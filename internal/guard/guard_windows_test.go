//go:build windows

package guard

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/launch"
	"pcblock/internal/systray"
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
		t.Fatalf("категория переднего плана %q, ждали %q: категорию по пути определяет служба", s.Foreground, ipc.CatNameOther)
	}
	if n := len([]rune(s.ForegroundPath)); n > ipc.MaxForegroundPathLen {
		t.Fatalf("путь переднего плана длиннее %d символов: %d", ipc.MaxForegroundPathLen, n)
	}
	fast := ReadSensors(now, 10)
	if fast.LastInputAgoMs < s.LastInputAgoMs {
		t.Fatalf("при ускорении простой не может стать меньше: %d < %d", fast.LastInputAgoMs, s.LastInputAgoMs)
	}
}

var procGetShellWindow = user32.NewProc("GetShellWindow")

// Ловит (R3, R23, PL-9): путь программы окна не читается или читается с 8.3-именами, окно самого
// стража уходит службе как программа ребёнка, нет окна — не пусто.
func TestForegroundPathOfWindows(t *testing.T) {
	if got := foregroundPath(0); got != "" {
		t.Fatalf("нет окна: %q", got)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := newPlateWindow(nil)
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer func() {
		p.close()
		pump(20 * time.Millisecond)
	}()
	if got := foregroundPath(p.hwnd); got != "" {
		t.Fatalf("окно самого стража ушло службе: %q", got)
	}
	shell, _, _ := procGetShellWindow.Call()
	if shell == 0 {
		t.Skip("нет окна рабочего стола (explorer не запущен)")
	}
	got := foregroundPath(shell)
	if !strings.HasSuffix(strings.ToLower(got), `\explorer.exe`) || strings.Contains(got, "~") {
		t.Fatalf("путь окна рабочего стола: %q", got)
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

// Ловит: текстовая плашка 1 мин не показывается или не прячется.
func TestWarningPlateShowsAndHides(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	p, err := newPlateWindow(nil)
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer func() {
		p.close()
		pump(20 * time.Millisecond)
	}()

	p.apply(&WarningView{ID: 1, Threshold: 1, Text: "Осталась 1 минута: 0:59", Buttons: []ButtonView{}})
	pump(20 * time.Millisecond)
	if r, _, _ := procIsWindowVisible.Call(p.hwnd); r == 0 {
		t.Fatal("плашка должна быть видна")
	}
	p.hide()
	if r, _, _ := procIsWindowVisible.Call(p.hwnd); r != 0 {
		t.Fatal("плашка должна прятаться")
	}
}

// Ловит: плашка-табло забирает фокус у окна на переднем плане при показе или щелчке по крестику
// (класс B); крестик не доходит до стража.
func TestWarningCardKeepsFocusAndCloses(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	closed := 0
	c, err := systray.NewWarningCard(func() { closed++ })
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer func() {
		c.Close()
		pump(20 * time.Millisecond)
	}()
	before, _, _ := procGetForegroundWin.Call()
	v := systray.TimeCountdown(1800)
	v.Line = "Через 30 минут время на сегодня закончится"
	c.Show(v)
	pump(30 * time.Millisecond)
	if r, _, _ := procIsWindowVisible.Call(c.HWND()); r == 0 {
		t.Fatal("табло должно быть видно")
	}
	if after, _, _ := procGetForegroundWin.Call(); after == c.HWND() || after != before {
		t.Fatalf("фокус ушёл: до %x, после %x, табло %x", before, after, c.HWND())
	}
	c.ClickClose()
	pump(30 * time.Millisecond)
	if closed != 1 {
		t.Fatalf("крестик дошёл %d раз, ждали 1", closed)
	}
	if after, _, _ := procGetForegroundWin.Call(); after == c.HWND() {
		t.Fatal("щелчок по крестику активировал табло")
	}
	c.Hide()
	if r, _, _ := procIsWindowVisible.Call(c.HWND()); r != 0 {
		t.Fatal("табло должно прятаться")
	}
}

var procFindWindow = user32.NewProc("FindWindowW")

func findHost(class string) uintptr {
	h, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(utf16(class))), 0)
	return h
}

// Ловит: страж не запускается без службы (падает, не создаёт окно, не занимает мьютекс),
// второй страж в сеансе не выходит молча, закрытие окна не завершает Run. Без службы стражу
// нечего показывать, и экран блокировки не появляется (NFR-10).
func TestRunStartsOneGuardAndStopsOnClose(t *testing.T) {
	if c, err := ipc.Dial(ipc.PipeName, 100*time.Millisecond); err == nil {
		_ = c.Close()
		t.Skip("на этой машине служба PCBlock уже работает")
	}
	opts := launch.Options{"session": "4242422"}
	runStage.Store(0)
	done := make(chan error, 1)
	go func() { done <- Run(opts) }()

	var host uintptr
	for i := 0; i < 100 && host == 0; i++ {
		host = findHost("PCBlockGuardHost")
		time.Sleep(50 * time.Millisecond)
	}
	if host == 0 {
		select {
		case err := <-done:
			t.Skipf("страж не запустился (нет рабочего стола?): %v", err)
		default:
			t.Fatal("окно стража не появилось за 5 секунд")
		}
	}

	second := make(chan error, 1)
	go func() { second <- Run(opts) }()
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("второй страж в сеансе должен выйти молча, а вернул: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("второй страж не вышел")
	}

	time.Sleep(600 * time.Millisecond) // один оборот таймера окон
	if w := findHost("PCBlockLockWindow"); w != 0 {
		if r, _, _ := procIsWindowVisible.Call(w); r != 0 {
			t.Fatal("без службы экран блокировки показываться не должен")
		}
	}
	// Закрываем окно стража. Единичный PostMessage изредка приходил раньше, чем окно начинало
	// качать очередь сообщений, и терялся — тогда Run не завершался и тест падал по таймауту
	// (B-16). Шлём WM_CLOSE повторно, пока Run не вернётся, с общим запасом времени.
	// Падение называет этап Run и число опросов службы после закрытия (B-20): этап 0 — WM_QUIT
	// не дошёл до цикла окон; этап 1 и опросов 2 и больше — опрос не выходит по стопу; этап 1 и
	// опросов не больше 1 — завис сам опрос (подключение к каналу); этап 2 — уборка окон.
	deadline := time.After(15 * time.Second)
	retry := time.NewTicker(250 * time.Millisecond)
	defer retry.Stop()
	closedAt := time.Now()
	before := pollRounds.Load()
	procPostMessage.Call(host, wmClose, 0, 0)
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run завершился с ошибкой: %v", err)
			}
			t.Logf("Run вернулся через %v, опросов после закрытия %d", time.Since(closedAt).Round(time.Millisecond), pollRounds.Load()-before)
			return
		case <-retry.C:
			procPostMessage.Call(host, wmClose, 0, 0)
		case <-deadline:
			t.Fatalf("Run не завершился за 15 с после закрытия окна: этап %d, опросов службы после закрытия %d", runStage.Load(), pollRounds.Load()-before)
		}
	}
}

// Ловит (S2, S35): при живом табло «5» порог 1 не сменяет его плашкой 1 мин в настоящих окнах
// (окно табло не прячется, текстовая плашка не создаётся или поток окон встаёт), а GuardState
// после опроса показывает старый кадр.
func TestBoardThenOneMinuteRealWindows(t *testing.T) {
	for _, c := range []struct {
		balance int
		buttons string
	}{{0, TargetAskMorePlate}, {45, TargetSpendBonusPlate + "," + TargetAskMorePlate}} {
		t.Run(fmt.Sprintf("копилка %d", c.balance), func(t *testing.T) { boardThenOneMinute(t, c.balance, c.buttons) })
	}
}

// boardThenOneMinute: табло «5», затем плашка 1 мин в настоящих окнах; кнопки плашки 1 мин по
// балансу копилки (RM3-5), нажатие кнопки через TestGuard доходит до службы без пароля.
func boardThenOneMinute(t *testing.T, balance int, buttons string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	r := newCtRig()
	r.svc.test = true
	r.ctl.SetSoundPlayers(func() bool { return true }, nil, nil)
	a := &app{ctl: r.ctl}
	defer a.shutdown()
	ctRunningStatus := func(rem int) ipc.ChildStatus {
		st := ipc.ChildStatusFrom("Миша", runningDecision(rem))
		st.BonusBalance = balance
		return st
	}

	r.svc.status = ctRunningStatus(300)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(5)}
	r.ctl.PollOnce()
	a.tick()
	pump(30 * time.Millisecond)
	if a.board == nil {
		t.Skip("окно табло создать нельзя (нет рабочего стола)")
	}
	if !a.board.Visible() {
		t.Fatal("табло «5» не показано")
	}
	for i := 1; i <= 4; i++ {
		r.clk.Advance(time.Second)
		r.svc.status = ctRunningStatus(300 - 60*i)
		if i == 4 {
			r.svc.status = ctRunningStatus(60)
			r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
		}
		r.ctl.PollOnce()
		r.ctl.PushState()
		a.tick()
		pump(20 * time.Millisecond)
	}
	if a.board.Visible() {
		t.Fatal("табло «5» осталось после порога 1")
	}
	if a.plate == nil || !a.plate.visible {
		t.Fatal("плашка 1 мин не показана")
	}
	if r2, _, _ := procIsWindowVisible.Call(a.plate.hwnd); r2 == 0 {
		t.Fatal("окно плашки 1 мин не видно")
	}
	if v := r.ctl.View(); v.Warning == nil || v.Warning.Threshold != 1 || buttonNames(v.Warning) != buttons {
		t.Fatalf("вид после порога 1: %+v", v.Warning)
	}
	r.ctl.PushState()
	calls := r.svc.called(ipc.CmdGuardState)
	var st ipc.GuardStateReply
	if err := calls[len(calls)-1].Into(&st); err != nil {
		t.Fatal(err)
	}
	if strings.Join(st.Buttons, ",") != buttons || st.PlateKind != ipc.PlateOneMinute || len(st.Sounds) != 2 || st.UI.Ticks < 5 || st.UI.Panic != "" {
		t.Fatalf("GuardState: кнопки %v, вид %q, звуки %v, поток окон %+v", st.Buttons, st.PlateKind, st.Sounds, st.UI)
	}
	// Первая кнопка плашки нажимается щелчком в окне (TestGuard click), команда идёт службе.
	first, _, _ := strings.Cut(buttons, ",")
	cmd := map[string]string{TargetAskMorePlate: ipc.CmdOpenWindow, TargetSpendBonusPlate: ipc.CmdSpendBonus}[first]
	a.runTestAction(ipc.TestGuardAction{Action: ipc.TestClick, Target: first})
	deadline := time.Now().Add(2 * time.Second)
	for len(r.svc.called(cmd)) == 0 && time.Now().Before(deadline) {
		pump(20 * time.Millisecond)
	}
	if n := len(r.svc.called(cmd)); n != 1 || len(r.svc.called(ipc.CmdLogin)) != 0 {
		t.Fatalf("щелчок по «%s»: %s вызвана %d раз", first, cmd, n)
	}
}

var procGetWindowLong = user32.NewProc("GetWindowLongW")

const (
	gwlExStyle   = ^uintptr(19) // GWL_EXSTYLE = -20
	exTopmostBit = 0x00000008
)

// Ловит (класс B, Review Focus 1): плашка о программе с кнопками забирает фокус при показе или при
// щелчке по кнопке, щелчок не доходит до стража, повторные кадры снимают «поверх всех»; крестик
// и «Попросить сейчас» отличаются (не та кнопка).
func TestBlockedPlateKeepsFocusAndPresses(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var pressed []Button
	p, err := newPlateWindow(func(b Button) { pressed = append(pressed, b) })
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer func() {
		p.close()
		pump(20 * time.Millisecond)
	}()
	m := NewModel()
	m.OnDecision(time.Now(), runningDecision(3600), false, nil)
	m.OnBlocked(time.Now(), ipc.BlockedPlate{Kind: ipc.BlockedUnknown, Text: "Программа «SuperGame» новая. Родитель должен решить, можно ли её запускать", ShowS: 15, Path: `c:\x.exe`, CanAsk: true})
	w := m.View(time.Now()).Warning
	before, _, _ := procGetForegroundWin.Call()
	for i := 0; i < 3; i++ {
		p.apply(w) // кадры окна: вид тот же
		pump(20 * time.Millisecond)
	}
	if r, _, _ := procIsWindowVisible.Call(p.hwnd); r == 0 {
		t.Fatal("плашка не видна")
	}
	if ex, _, _ := procGetWindowLong.Call(p.hwnd, gwlExStyle); ex&exTopmostBit == 0 {
		t.Fatal("плашка не поверх всех окон после повторных кадров")
	}
	if after, _, _ := procGetForegroundWin.Call(); after == p.hwnd || after != before {
		t.Fatalf("фокус ушёл при показе: до %x, после %x, плашка %x", before, after, p.hwnd)
	}
	if !p.click(ButtonAskNow) || !p.click(ButtonPlateClose) {
		t.Fatal("кнопок нет на плашке")
	}
	if p.click(ButtonPlateSpendBonus) {
		t.Fatal("нажалась кнопка, которой на плашке нет")
	}
	pump(50 * time.Millisecond)
	if len(pressed) != 2 || pressed[0] != ButtonAskNow || pressed[1] != ButtonPlateClose {
		t.Fatalf("нажатия: %v", pressed)
	}
	if after, _, _ := procGetForegroundWin.Call(); after == p.hwnd {
		t.Fatal("щелчок по кнопке активировал плашку")
	}
	if title := windowText(p.hwnd); title != w.Text {
		t.Fatalf("заголовок окна для диктора: %q", title)
	}
}

// Ловит: экран перерыва (8.2.3 п.12) без картинки и плиток на обычном экране, картинка не над
// заголовком или налезает на него, плитки не под заголовком; сообщение родителя длиннее двух
// строк или длинное слово вылезает за ширину текста; новая секунда таймера перестраивает окно
// (кнопки создаются заново, фокус прыгает); на экране «Попросить ещё».
func TestRestScreenDrawsCatTilesAndMessage(t *testing.T) {
	r := newLockRig(t)
	now := time.Now()
	long := "Разомнись" + strings.Repeat("ааааааааааааааааааааааааааааааа", 6) + " попей воды, посмотри в окно и потом снова садись играть, если захочешь"
	r.model.OnDecision(now, core.Decision{Lock: core.LockRest, Reason: core.ReasonRest, Until: "Можно снова в 16:20",
		RestLeftS: 15 * 60, RestMessage: long}, false, nil)
	v := r.show()
	if v.Rest == nil || !r.ui.rest.showTiles {
		t.Fatalf("плиток таймера нет: %+v", r.ui.rest)
	}
	area := r.ui.area()
	title := r.ui.texts[0]
	if title.text != core.ReasonRest {
		t.Fatalf("первый текст %q, ждали заголовок", title.text)
	}
	if area.bottom-area.top >= 900 && !r.ui.rest.showCat {
		t.Error("на экране выше 900 точек картинки нет")
	}
	if r.ui.rest.showCat && r.ui.rest.catRc.bottom > title.rc.top {
		t.Errorf("картинка %v налезает на заголовок %v", r.ui.rest.catRc, title.rc)
	}
	if r.ui.rest.tilesRc.top < title.rc.bottom {
		t.Errorf("плитки %v выше конца заголовка %v", r.ui.rest.tilesRc, title.rc)
	}
	hdc, _, _ := procGetDC.Call(r.w.hwnd)
	defer procReleaseDC.Call(r.w.hwnd, hdc)
	lines := 0
	for _, tx := range r.ui.texts {
		if tx.rc.top < r.ui.rest.tilesRc.bottom || tx.text == "Можно снова в 16:20" || tx.font != r.ui.fonts.body {
			continue
		}
		lines++
		if w := measureWidth(hdc, tx.font, tx.text); w > tx.rc.right-tx.rc.left {
			t.Errorf("строка сообщения %q шире места: %d > %d", tx.text, w, tx.rc.right-tx.rc.left)
		}
	}
	if lines < 1 || lines > 2 {
		t.Errorf("строк сообщения %d, ждали 1–2", lines)
	}
	if r.ui.buttonHandle(ButtonAskMore) != 0 || r.ui.buttonHandle(ButtonParentPassword) == 0 {
		t.Error("на экране перерыва «Попросить ещё» есть или нет «Пароль родителя»")
	}
	h, sig := r.ui.buttonHandle(ButtonParentPassword), r.ui.sig
	time.Sleep(1100 * time.Millisecond)
	v2 := r.show()
	if !TimerChanged(v, v2) {
		t.Fatalf("таймер не изменился: %v → %v", v.Rest.Tiles, v2.Rest.Tiles)
	}
	if r.ui.buttonHandle(ButtonParentPassword) != h || r.ui.sig != sig || r.ui.view.Rest.Tiles != v2.Rest.Tiles {
		t.Error("новая секунда таймера перестроила окно или не дошла до плиток")
	}
}

// Ловит: слово без пробелов длиннее строки выходит за край плашки (DrawText без DT_EDITCONTROL не
// переносит его), и расчёт высоты не совпадает с рисованием.
func TestMeasureLongWordWraps(t *testing.T) {
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)
	font := makeFont(Scaled(NoteFontPx, 96), 400)
	defer procDeleteObject.Call(font)
	word := strings.Repeat("ужинать", 12)
	w, h := measureSize(hdc, font, word, 200)
	_, one := measureSize(hdc, font, "у", 200)
	if w > 200 || h < 3*one {
		t.Fatalf("слово %d знаков в ширину 200: %d×%d (строка %d) — не перенесено по буквам", len([]rune(word)), w, h, one)
	}
	if got := measureHeight(hdc, font, word, 200); got != h {
		t.Fatalf("measureHeight %d, measureSize %d", got, h)
	}
}

// Ловит: настоящее окно сообщения (мерка Windows) с длинным текстом над экраном блокировки
// закрывает его заголовок или кнопки, выходит за рабочую область или выше 55 % её (окно со стикером
// и тенью), текст обрезан.
func TestNoteWindowOverLockFits(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	n, err := newNoteWindow(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer n.close()
	fitPx, fits := -1, false
	n.onFit = func(px int, ok bool) { fitPx, fits = px, ok } // GuardState.NoteFontPx, NoteTextFits (S57)
	wa := workArea()
	text := strings.Repeat(strings.Repeat("Иди ужинать скорее ", 2)+"\n", 5) + strings.Repeat("ж", 110)
	lock := &NoteLock{Title: Rect{Left: wa.left + 62, Top: wa.top + 65, Right: wa.left + 962, Bottom: wa.top + 115}, ControlsTop: wa.top + 272,
		MaxUp: 65 - 16, MaxDown: 768 - 16 - 680}
	for i, c := range []struct {
		lk      *NoteLock
		sticker string
	}{{nil, ""}, {lock, ""}, {nil, "033"}, {lock, "033"}} {
		lk := c.lk
		v := &NoteView{Seq: i + 1, ID: "a", Title: textNoteTitle, Text: text,
			Buttons: []ButtonView{{Button: ButtonNoteOK, Label: textNoteOK, Main: true, Enabled: true}}}
		v.Sticker, v.Frames = noteSticker(c.sticker)
		v.Started = time.Now()
		n.apply(v, lk)
		pump(50 * time.Millisecond)
		g := n.geom
		if g.top < wa.top || g.bottom > wa.bottom || g.left < wa.left || g.right > wa.right {
			t.Fatalf("плашка %+v вне рабочей области %+v", g, wa)
		}
		if h := g.bottom - g.top; h > (wa.bottom-wa.top)*55/100 {
			t.Fatalf("окно %d px выше 55 %% области (Ruling FB28-1)", h)
		}
		if lk != nil {
			f := n.fit
			up, down := f.ShiftUp, f.ShiftDown
			if up > lk.MaxUp || down > lk.MaxDown || g.bottom > lk.ControlsTop+down || g.top < lk.Title.Bottom-up && g.bottom > lk.Title.Top-up {
				t.Fatalf("плашка %+v закрывает заголовок %+v или кнопки (с %d), сдвиг %d/%d", g, lk.Title, lk.ControlsTop, up, down)
			}
		}
		// Текст целиком (FR-41): без «…», все строки в раскладке (мерка Windows), шрифт не мельче 14 px.
		hdc, _, _ := procGetDC.Call(n.hwnd)
		tw := NoteTextWidth(n.dpi, n.fit.Layout.Card.Right-n.fit.Layout.Card.Left, n.fit.Density)
		need := measureHeight(hdc, n.font(n.fit.FontPx, 600), text, tw)
		procReleaseDC.Call(n.hwnd, hdc)
		if n.fit.Text != text || n.fit.FontPx < 14 || need > n.fit.Layout.Text.Bottom-n.fit.Layout.Text.Top {
			t.Fatalf("текст не целиком: шрифт %d, нужно %d px, место %+v, %q", n.fit.FontPx, need, n.fit.Layout.Text, n.fit.Text)
		}
		if fitPx != int(Scaled(n.fit.FontPx, n.dpi)) || !fits {
			t.Fatalf("окно сообщило шрифт %d, поместился %v; ждали %d и да", fitPx, fits, Scaled(n.fit.FontPx, n.dpi))
		}
		n.hide()
		if fitPx != 0 || fits {
			t.Fatalf("после скрытия: шрифт %d, поместился %v; ждали 0 и нет", fitPx, fits)
		}
	}
}

// contrastRatio — отношение контраста двух цветов COLORREF по WCAG 2.
func contrastRatio(a, b uintptr) float64 {
	lum := func(c uintptr) float64 {
		ch := func(v uintptr) float64 {
			x := float64(v&0xFF) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c) + 0.7152*ch(c>>8) + 0.0722*ch(c>>16)
	}
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Ловит (ТЗ 8.0 п. 3, 8.2): окно сообщения не своего синего (рамка, заголовок и кнопка #2F6DB5 на
// фоне #EEF4FC - одни константы с рисунком noterender.go); заголовок тусклый (контраст ниже 4,5:1) или
// не 18 px; белая подпись «Понял» на синей кнопке ниже 4,5:1.
func TestNoteColorsBlue(t *testing.T) {
	ref := func(v uint32) uintptr { return rgb(v>>16&0xFF, v>>8&0xFF, v&0xFF) }
	if NoteBlueRGB != 0x2F6DB5 || NoteTintRGB != 0xEEF4FC || NoteInkRGB != 0x1B1F23 {
		t.Fatalf("цвета %06x/%06x/%06x, want 2F6DB5/EEF4FC/1B1F23", NoteBlueRGB, NoteTintRGB, NoteInkRGB)
	}
	if r := contrastRatio(ref(NoteBlueRGB), ref(NoteTintRGB)); r < 4.5 {
		t.Fatalf("заголовок на карточке: контраст %.2f", r)
	}
	if r := contrastRatio(ref(0xFFFFFF), ref(NoteBlueRGB)); r < 4.5 {
		t.Fatalf("«Понял» на кнопке: контраст %.2f", r)
	}
	if r := contrastRatio(ref(NoteInkRGB), ref(NoteTintRGB)); r < 4.5 {
		t.Fatalf("текст на карточке: контраст %.2f", r)
	}
	if NoteNormal.TitlePx != 18 {
		t.Fatalf("заголовок %d px, want 18", NoteNormal.TitlePx)
	}
	// Окно рисует текст теми же цветами, что основа: не прежними нейтральными colorLine/colorText.
	if noteFrameColor != ref(NoteBlueRGB) || noteTitleColor != ref(NoteBlueRGB) || noteInkColor != ref(NoteInkRGB) {
		t.Fatalf("окно: рамка %06x, заголовок %06x, текст %06x (COLORREF); want синий %06x и %06x",
			noteFrameColor, noteTitleColor, noteInkColor, ref(NoteBlueRGB), ref(NoteInkRGB))
	}
}

// Ловит (FR-41, NFR-5, план 0.8.3): стикер сообщения в настоящем окне не разобран, не движется
// (тикер не запущен или кадр не выводится), движется при выключенных анимациях Windows; после
// скрытия окна тикер идёт дальше, а спрайт и буферы рисунка остаются в памяти; без стикера окно
// всё равно держит спрайт.
func TestNoteWindowStickerAnimates(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	n, err := newNoteWindow(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer n.close()
	var shownSticker string
	var frames []int
	n.onShown = func(s string, k int) { shownSticker = s; frames = append(frames, k) }
	v := &NoteView{Seq: 1, ID: "a", Title: textNoteTitle, Text: "Иди ужинать",
		Buttons: []ButtonView{{Button: ButtonNoteOK, Label: textNoteOK, Main: true, Enabled: true}}}
	v.Sticker, v.Frames = noteSticker("033")
	v.Started = time.Now()
	if v.Sticker != "033" || v.Frames == 0 {
		t.Fatalf("стикера 033 нет среди встроенных: %q, %d кадров", v.Sticker, v.Frames)
	}
	n.apply(v, nil)
	for end := time.Now().Add(5 * time.Second); n.sprite == nil && time.Now().Before(end); {
		pump(20 * time.Millisecond)
	}
	if n.sprite == nil || n.anim == nil || n.fit.Layout.StickerPx == 0 {
		t.Fatalf("спрайт %v, шаг кадров %v, стикер %d px", n.sprite != nil, n.anim != nil, n.fit.Layout.StickerPx)
	}
	frames = nil
	pump(time.Second)
	distinct := map[int]bool{}
	for _, k := range frames {
		distinct[k] = true
	}
	if shownSticker != "033" || len(distinct) < StickerFPS/3 {
		t.Fatalf("за 1 с выведено %d разных кадров стикера %q, ждали не меньше %d", len(distinct), shownSticker, StickerFPS/3)
	}
	// Анимации Windows выключены: первый кадр, шаг кадров остановлен.
	still := *v
	still.Still = true
	n.apply(&still, nil)
	if n.anim != nil || n.cur != 0 {
		t.Fatalf("без анимаций: шаг кадров %v, кадр %d (ждали первый)", n.anim != nil, n.cur)
	}
	n.apply(nil, nil)
	if n.anim != nil || n.sprite != nil || n.bits != nil || n.base != nil || n.dib != 0 || shownSticker != "" {
		t.Fatalf("после скрытия: шаг %v, спрайт %v, DIB %v, основа %v, стикер %q",
			n.anim != nil, n.sprite != nil, n.dib != 0, n.base != nil, shownSticker)
	}
	// Без стикера: окно не держит спрайт и не шагает кадры.
	plain := &NoteView{Seq: 2, ID: "b", Title: textNoteTitle, Text: "Иди ужинать", Buttons: v.Buttons}
	n.apply(plain, nil)
	pump(100 * time.Millisecond)
	if n.sprite != nil || n.anim != nil || n.fit.Layout.StickerPx != 0 || n.pending != "" {
		t.Fatalf("без стикера: спрайт %v, шаг %v, стикер %d px, разбор %q", n.sprite != nil, n.anim != nil, n.fit.Layout.StickerPx, n.pending)
	}
}

var procWindowFromPoint = user32.NewProc("WindowFromPoint")

// windowAt - окно в точке экрана, как его выберет щелчок мыши (POINT по значению одним регистром на
// amd64 и arm64).
func windowAt(x, y int32) uintptr {
	h, _, _ := procWindowFromPoint.Call(uintptr(uint64(uint32(x)) | uint64(uint32(y))<<32))
	return h
}

// Ловит (ревью M3 0.8.3, NFR-10): прозрачные пиксели окна сообщения (угол рядом со стикером, вне
// тени) ловят щелчки, и окно под ним (поле пароля, кнопки экрана блокировки) там не нажать; или
// карточка с текстом и кнопка «Понял» щелчков не ловят.
func TestNoteWindowClicksPassThroughTransparent(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	n, err := newNoteWindow(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer n.close()
	v := &NoteView{Seq: 1, ID: "a", Title: textNoteTitle, Text: "Иди ужинать",
		Buttons: []ButtonView{{Button: ButtonNoteOK, Label: textNoteOK, Main: true, Enabled: true}}}
	v.Sticker, v.Frames = noteSticker("033")
	v.Started = time.Now()
	n.apply(v, nil)
	for end := time.Now().Add(5 * time.Second); n.sprite == nil && time.Now().Before(end); {
		pump(20 * time.Millisecond)
	}
	pump(100 * time.Millisecond)
	l := n.fit.Layout
	if n.sprite == nil || l.StickerPx <= 0 || n.bits == nil || !n.visible {
		t.Fatalf("окно со стикером не показано: спрайт %v, стикер %d px, DIB %v", n.sprite != nil, l.StickerPx, n.bits != nil)
	}
	alpha := func(x, y int32) byte { return n.bits[(int(y)*int(n.dibW)+int(x))*4+3] }
	// Угол окна левее стикера и выше тени карточки: альфа 0.
	cx, cy := l.Sticker.Left/2, int32(2)
	if cy >= l.Shadow.Top || cx <= 0 || alpha(cx, cy) != 0 {
		t.Fatalf("точка (%d, %d) окна не прозрачна: альфа %d, стикер с %d, тень с %d", cx, cy, alpha(cx, cy), l.Sticker.Left, l.Shadow.Top)
	}
	if h := windowAt(n.geom.left+cx, n.geom.top+cy); h == n.hwnd {
		t.Fatalf("щелчок в прозрачной точке (%d, %d) окна ловит окно сообщения: под ним ничего не нажать", cx, cy)
	}
	for _, p := range []struct {
		name string
		r    Rect
	}{{"текст на карточке", l.Text}, {"кнопка «Понял»", l.Button}} {
		x, y := (p.r.Left+p.r.Right)/2, (p.r.Top+p.r.Bottom)/2
		if a := alpha(x, y); a != 255 {
			t.Fatalf("%s (%d, %d): альфа %d, ждали непрозрачную", p.name, x, y, a)
		}
		if h := windowAt(n.geom.left+x, n.geom.top+y); h != n.hwnd {
			t.Fatalf("%s (%d, %d): щелчок попадает в окно %#x, а не в окно сообщения %#x", p.name, x, y, h, n.hwnd)
		}
	}
}

// Ловит (ревью M4 0.8.3): над экраном блокировки настоящее окно (мерка Windows) никогда не оставляет
// стикер, хотя места хватает: путь «стикер над блокировкой» не проверялся ни одним настоящим окном
// (в TestNoteWindowOverLockFits длинный текст отдаёт стикер). Высокий экран блокировки: заголовок
// сверху, кнопки внизу, короткое сообщение встаёт между ними со стикером и без сдвига экрана.
func TestNoteWindowStickerOverLock(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	n, err := newNoteWindow(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer n.close()
	var shown string
	n.onShown = func(s string, _ int) { shown = s }
	wa := workArea()
	lock := &NoteLock{Title: Rect{Left: wa.left + 62, Top: wa.top + 20, Right: wa.right - 62, Bottom: wa.top + 60}, ControlsTop: wa.bottom - 60}
	v := &NoteView{Seq: 1, ID: "a", Title: textNoteTitle, Text: "Иди ужинать",
		Buttons: []ButtonView{{Button: ButtonNoteOK, Label: textNoteOK, Main: true, Enabled: true}}}
	v.Sticker, v.Frames = noteSticker("033")
	v.Started = time.Now()
	n.apply(v, lock)
	for end := time.Now().Add(5 * time.Second); n.sprite == nil && time.Now().Before(end); {
		pump(20 * time.Millisecond)
	}
	pump(50 * time.Millisecond)
	f, g := n.fit, n.geom
	if f.Layout.StickerPx <= 0 {
		t.Fatalf("над экраном блокировки (рабочая область %d×%d, %d dpi) окно без стикера: %+v",
			wa.right-wa.left, wa.bottom-wa.top, n.dpi, f.Layout)
	}
	if n.sprite == nil || shown != "033" {
		t.Fatalf("стикер над экраном блокировки не выведен: спрайт %v, выведен %q", n.sprite != nil, shown)
	}
	if f.ShiftUp != 0 || f.ShiftDown != 0 || g.top < lock.Title.Bottom || g.bottom > lock.ControlsTop {
		t.Fatalf("окно %+v закрывает заголовок %+v или кнопки (с %d), сдвиг %d/%d", g, lock.Title, lock.ControlsTop, f.ShiftUp, f.ShiftDown)
	}
}

// plaqueView — плашка «Ждёт разрешения» со стикером 031 и кнопкой «Попросить сейчас» (как у Model).
func plaqueView(id int) *WarningView {
	m := NewModel()
	m.OnDecision(time.Now(), runningDecision(3600), false, nil)
	m.OnBlocked(time.Now(), ipc.BlockedPlate{Kind: ipc.BlockedUnknown, Title: "Ждёт разрешения",
		Text: "Программа «SuperGame» новая. Родитель должен решить, можно ли её запускать", ShowS: 15, Path: `c:\x.exe`, CanAsk: true})
	w := m.View(time.Now()).Warning
	w.ID = id
	return w
}

// showPlaque показывает плашку и ждёт разбора спрайта стикера.
func showPlaque(t *testing.T, p *plaqueWindow, w *WarningView) {
	t.Helper()
	p.apply(w)
	for end := time.Now().Add(5 * time.Second); p.sprite == nil && time.Now().Before(end); {
		pump(20 * time.Millisecond)
	}
	pump(100 * time.Millisecond)
	if !p.visible || p.sprite == nil || p.bits == nil {
		t.Fatalf("плашка не показана: видна %v, спрайт %v, DIB %v", p.visible, p.sprite != nil, p.bits != nil)
	}
}

// Ловит (8.2.4, PL-86): прозрачный угол плашки ловит щелчки (под ней ничего не нажать), а панель с
// текстом не ловит; плашка не в правом нижнем углу рабочей области.
func TestPlaqueWindowHitTest(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := newPlaqueWindow(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	showPlaque(t, p, plaqueView(1))
	wa, l := workArea(), p.lay
	if p.geom.right != wa.right-Scaled(16, p.dpi) || p.geom.bottom != wa.bottom-Scaled(16, p.dpi) {
		t.Fatalf("плашка %+v не в правом нижнем углу %+v", p.geom, wa)
	}
	alpha := func(x, y int32) byte { return p.bits[(int(y)*int(p.dibW)+int(x))*4+3] }
	if alpha(0, l.H-1) != 0 {
		t.Fatal("нижний левый угол окна не прозрачен")
	}
	if h := windowAt(p.geom.left, p.geom.bottom-1); h == p.hwnd {
		t.Fatal("щелчок в прозрачном углу ловит окно плашки")
	}
	x, y := (l.Text.Left+l.Text.Right)/2, (l.Text.Top+l.Text.Bottom)/2
	if a := alpha(x, y); a != 255 {
		t.Fatalf("текст на панели: альфа %d", a)
	}
	if h := windowAt(p.geom.left+x, p.geom.top+y); h != p.hwnd {
		t.Fatalf("щелчок по панели попадает в окно %#x, а не в плашку %#x", h, p.hwnd)
	}
}

// Ловит (RM3-4, 8.2.4): щелчок в «Попросить сейчас» или крестик не доходит до стража нужной кнопкой.
func TestPlaqueWindowButtons(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var pressed []Button
	p, err := newPlaqueWindow(func(b Button) { pressed = append(pressed, b) })
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	showPlaque(t, p, plaqueView(1))
	if !p.click(ButtonAskNow) || !p.click(ButtonPlateClose) {
		t.Fatal("кнопки нет на плашке")
	}
	pump(100 * time.Millisecond)
	if len(pressed) != 2 || pressed[0] != ButtonAskNow || pressed[1] != ButtonPlateClose {
		t.Fatalf("нажато %v, ждали «Попросить сейчас» и крестик", pressed)
	}
}

// Ловит (NFR-5, PL-89): тикер стикера идёт после скрытия плашки, спрайт и буферы остаются в памяти,
// при выключенных анимациях стикер движется.
func TestPlaqueTickerOnlyVisible(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := newPlaqueWindow(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	var frames []int
	p.onShown = func(s string, k int) { frames = append(frames, k) }
	w := plaqueView(1)
	showPlaque(t, p, w)
	if p.anim == nil {
		t.Fatal("тикер стикера не запущен")
	}
	frames = nil
	pump(time.Second)
	distinct := map[int]bool{}
	for _, k := range frames {
		distinct[k] = true
	}
	if len(distinct) < StickerFPS/3 {
		t.Fatalf("за 1 с %d разных кадров", len(distinct))
	}
	still := *w
	still.Still = true
	p.apply(&still)
	if p.anim != nil || p.cur != 0 {
		t.Fatalf("без анимаций: тикер %v, кадр %d", p.anim != nil, p.cur)
	}
	p.apply(nil)
	if p.anim != nil || p.sprite != nil || p.bits != nil || p.patch != nil || p.dib != 0 {
		t.Fatalf("после скрытия: тикер %v, спрайт %v, DIB %v", p.anim != nil, p.sprite != nil, p.dib != 0)
	}
}
