//go:build windows

package guard

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

var procPeekMessage = user32.NewProc("PeekMessageW")

const pmRemove = 0x0001

// pump разбирает очередь сообщений потока: окна живут в потоке теста.
func pump(d time.Duration) {
	var m tagMSG
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		if r == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func TestReadSensorsIsSane(t *testing.T) {
	now := time.Now()
	s := ReadSensors(now, 1)
	if s.AtMs != now.UnixMilli() {
		t.Fatalf("AtMs = %d, ждали %d", s.AtMs, now.UnixMilli())
	}
	if s.LastInputAgoMs < 0 {
		t.Fatalf("время простоя отрицательное: %d", s.LastInputAgoMs)
	}
	if s.Foreground != ipc.CatNameOther {
		t.Fatalf("категория переднего плана %q, в M1 всегда %q", s.Foreground, ipc.CatNameOther)
	}
	fast := ReadSensors(now, 10)
	if fast.LastInputAgoMs < s.LastInputAgoMs {
		t.Fatalf("при ускорении простой не может стать меньше: %d < %d", fast.LastInputAgoMs, s.LastInputAgoMs)
	}
}

func TestTestClockSpeedWithoutRegistryIsNormal(t *testing.T) {
	if n := testClockSpeed(); n < 1 || n > MaxClockSpeed {
		t.Fatalf("ускорение часов %d вне 1..%d", n, MaxClockSpeed)
	}
}

// lockRig — окно блокировки с настоящей отрисовкой поверх Model, как в стороже.
type lockRig struct {
	w       *LockWindow
	ui      *lockUI
	model   *Model
	pressed []Button
	typed   string
}

func newLockRig(t *testing.T) *lockRig {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)

	r := &lockRig{model: NewModel()}
	w, err := NewLockWindow(LockView{Reason: core.ReasonTimeUp})
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	r.w = w
	r.ui = newLockUI(w, func(b Button) { r.pressed = append(r.pressed, b) }, func(s string) { r.typed = s })
	t.Cleanup(func() {
		w.RemoveKeyboardGuard()
		procDestroyWindow.Call(w.hwnd)
		r.ui.close()
		w.Close()
		pump(20 * time.Millisecond)
	})

	now := time.Now()
	r.model.OnDecision(now, core.Decision{Lock: core.LockTimeUp, Reason: core.ReasonTimeUp, Until: "Можно снова: завтра", Used: "Сегодня: 2 ч 00 мин из 2 ч"}, false, nil)
	return r
}

func (r *lockRig) show() View {
	v := r.model.View(time.Now())
	r.ui.apply(v, "")
	pump(20 * time.Millisecond)
	return v
}

func TestLockScreenClickReachesModel(t *testing.T) {
	r := newLockRig(t)
	v := r.show()
	if !v.LockShown {
		t.Fatal("после решения о блокировке экран должен быть показан")
	}
	h := r.ui.buttonHandle(ButtonParentPassword)
	if h == 0 {
		t.Fatal("на экране нет кнопки «Пароль родителя»")
	}
	procSendMessage.Call(h, bmClick, 0, 0)
	pump(50 * time.Millisecond)
	if len(r.pressed) != 1 || r.pressed[0] != ButtonParentPassword {
		t.Fatalf("нажатие не дошло до обработчика: %v", r.pressed)
	}

	r.model.Press(time.Now(), ButtonParentPassword, "")
	v = r.show()
	if !v.PasswordEnabled {
		t.Fatal("после «Пароль родителя» поле пароля должно быть доступно")
	}
	if r.ui.edit == 0 {
		t.Fatal("поле пароля не создано")
	}
	r.ui.setPasswordText("пароль")
	pump(20 * time.Millisecond)
	if r.typed != "пароль" || r.ui.passwordText() != "пароль" {
		t.Fatalf("текст пароля не дошёл: typed=%q, поле=%q", r.typed, r.ui.passwordText())
	}
}

func TestLockScreenStaysAfterCloseAttempts(t *testing.T) {
	r := newLockRig(t)
	r.show()
	res := r.w.SelfTest()
	if !res.Passed {
		t.Fatalf("окно блокировки закрылось от штатных сообщений: %+v", res)
	}
	if !r.w.Visible() {
		t.Fatal("окно должно остаться видимым")
	}
	r.w.Hide()
	if r.w.Visible() {
		t.Fatal("Hide должен прятать окно")
	}
	pump(600 * time.Millisecond)
	if r.w.Visible() {
		t.Fatal("спрятанное окно не должно возвращаться наверх по таймеру")
	}
	r.w.Show()
	if !r.w.Visible() {
		t.Fatal("Show должен показывать окно")
	}
}

func TestWarningPlateShowsAndAcknowledges(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	acked := 0
	p, err := newPlateWindow(func() { acked++ })
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer func() {
		p.close()
		pump(20 * time.Millisecond)
	}()

	p.apply(&WarningView{ID: 1, Text: "Осталось 15 минут", Buttons: []ButtonView{{Button: ButtonWarningOK, Label: textWarningOK, Enabled: true}}})
	pump(20 * time.Millisecond)
	if r, _, _ := procIsWindowVisible.Call(p.hwnd); r == 0 {
		t.Fatal("плашка должна быть видна")
	}
	procSendMessage.Call(p.btn, bmClick, 0, 0)
	pump(20 * time.Millisecond)
	if acked != 1 {
		t.Fatalf("нажатие «Понятно» дошло %d раз, ждали 1", acked)
	}
	p.hide()
	if r, _, _ := procIsWindowVisible.Call(p.hwnd); r != 0 {
		t.Fatal("плашка должна прятаться")
	}
}
