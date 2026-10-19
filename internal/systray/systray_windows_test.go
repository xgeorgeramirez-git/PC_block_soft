//go:build windows

package systray

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
)

// Ловит: структура уведомления не совпала с Windows (оболочка отвергает значок молча).
// NOTIFYICONDATAW с версией 4 занимает 976 байт на 64-разрядной Windows.
func TestNotifyIconDataSize(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("размер проверяется для 64-разрядной Windows")
	}
	if got := unsafe.Sizeof(notifyIconData{}); got != 976 {
		t.Fatalf("sizeof(notifyIconData) = %d, ждали 976", got)
	}
	var d notifyIconData
	if off := unsafe.Offsetof(d.szTip); off != 40 {
		t.Fatalf("szTip на смещении %d, ждали 40", off)
	}
	if off := unsafe.Offsetof(d.uVersion); off != 816 {
		t.Fatalf("uVersion на смещении %d, ждали 816", off)
	}
}

// Ловит: значок какого-то статуса или надписи не создаётся (ни из PNG, ни из пикселей).
func TestMakeIconForEveryStatus(t *testing.T) {
	for _, st := range []Status{StatusNormal, StatusLow, StatusOut, StatusPaused, StatusNotChild} {
		for _, label := range []string{"", "0", "85", "2ч", "∞"} {
			h, err := makeIcon(label, st)
			if err != nil || h == 0 {
				t.Fatalf("значок %q (%v): %v", label, st, err)
			}
			procDestroyIcon.Call(h)
		}
	}
}

func TestIconFromPixelsFallbackWorks(t *testing.T) {
	for _, side := range IconSides {
		h, err := iconFromPixels(RenderIcon("85", StatusLow, side))
		if err != nil || h == 0 {
			t.Fatalf("сторона %d: %v", side, err)
		}
		procDestroyIcon.Call(h)
	}
}

func TestTipFieldIsTruncatedNotOverflowed(t *testing.T) {
	ti := newTrayIcon(0, wmApp+1)
	long := make([]rune, 400)
	for i := range long {
		long[i] = 'я'
	}
	d := ti.data(nifTip, 0, string(long))
	if d.szTip[tipMaxChar] != 0 {
		t.Fatal("подсказка должна заканчиваться нулём в последней ячейке")
	}
	if d.szTip[tipMaxChar-1] == 0 {
		t.Fatal("подсказка обрезана слишком рано")
	}
}

// Ловит: паника при добавлении, обновлении и удалении значка. Принял ли значок рабочий стол
// раннера, зависит от оболочки, поэтому результат не проверяется.
func TestTrayIconLifecycleDoesNotPanic(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hInst, _, _ := procGetModuleHandle.Call(0)
	cls, err := registerPlainClass("PCBlockTrayTestHost", procDefWindowProc.Addr(), hInst, 0)
	if err != nil {
		t.Skipf("класс окна не зарегистрирован: %v", err)
	}
	hwnd, _, _ := procCreateWindowEx.Call(wsExToolWin, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(wstr("t"))), wsPopup, 0, 0, 0, 0, 0, 0, hInst, 0)
	if hwnd == 0 {
		t.Skip("окно создать нельзя (нет рабочего стола)")
	}
	defer procDestroyWindow.Call(hwnd)

	ti := newTrayIcon(hwnd, wmApp+1)
	ti.apply(Display{Label: "85", Status: StatusNormal, Tooltip: "Осталось 1 ч 25 мин из 2 ч"})
	ti.apply(Display{Label: "15", Status: StatusLow, Tooltip: "Осталось 15 мин из 2 ч"})
	ti.forget()
	ti.apply(Display{Label: "0", Status: StatusOut, Tooltip: "Время на сегодня закончилось"})
	_, _ = ti.rect()
	ti.remove()
	ti.remove()
}

func TestClickedUnderBothNotificationVersions(t *testing.T) {
	v4 := &trayIcon{v4: true}
	for ev, want := range map[uint32]bool{ninSelect: true, ninKeySelect: true, wmContextMenu: true, wmLButtonUp: false, 0x0200: false} {
		if got := v4.clicked(uintptr(ev) | 1<<16); got != want {
			t.Errorf("версия 4, событие %#x: %v, ждали %v", ev, got, want)
		}
	}
	old := &trayIcon{}
	for ev, want := range map[uint32]bool{wmLButtonUp: true, wmRButtonUp: true, ninSelect: false, 0x0200: false} {
		if got := old.clicked(uintptr(ev)); got != want {
			t.Errorf("старая версия, событие %#x: %v, ждали %v", ev, got, want)
		}
	}
}

func TestMonitorWorkIsNotEmpty(t *testing.T) {
	w := monitorWork(rect{0, 0, 10, 10})
	if w.right <= w.left || w.bottom <= w.top {
		t.Fatalf("рабочая область пуста: %+v", w)
	}
}

// Ловит: панель не собирает кнопки по виду, не умеет закрываться и открывается снова
// сразу после закрытия (щелчок по значку закрыл бы её и тут же открыл).
func TestPanelBuildsButtonsAndHides(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pressed := []PanelKind{}
	p, err := newPanel(func(b PanelButton) { pressed = append(pressed, b.Kind) })
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer p.close()

	p.setView(BuildPanel(freshSnapshot(okStatus(85*60, 7200)), ""))
	if len(p.buttons) != 2 || p.height <= 0 {
		t.Fatalf("ребёнок: кнопок %d, высота %d", len(p.buttons), p.height)
	}
	p.setView(BuildPanel(Snapshot{}, "Не получилось открыть окно. Попробуй ещё раз"))
	if len(p.buttons) != 1 {
		t.Fatalf("ошибка: кнопок %d, ждали одну («Обновить»)", len(p.buttons))
	}
	withNote := p.height
	p.setView(BuildPanel(Snapshot{}, ""))
	if p.height >= withNote {
		t.Fatalf("без строки итога панель должна быть ниже: %d и %d", p.height, withNote)
	}

	p.pressIndex(0)
	if len(pressed) != 1 || pressed[0] != PanelRefresh {
		t.Fatalf("нажатие не дошло: %v", pressed)
	}
	p.setBusy(true)
	p.pressIndex(0)
	if len(pressed) != 1 {
		t.Fatal("пока команда в пути, нажатия игнорируются")
	}
	p.setBusy(false)

	p.show(Rect{900, 1048, 916, 1072}, true)
	if !p.visible {
		t.Fatal("панель должна открыться")
	}
	p.hide()
	if p.visible {
		t.Fatal("панель должна закрыться")
	}
	p.show(Rect{900, 1048, 916, 1072}, true)
	if p.visible {
		t.Fatal("сразу после закрытия панель не открывается заново")
	}
}

func TestSessionOfPrefersFlag(t *testing.T) {
	if got := sessionOf(map[string]string{"session": "7"}); got != 7 {
		t.Fatalf("сеанс %d, ждали 7", got)
	}
	if got := sessionOf(map[string]string{"session": "x"}); got != sessionOf(nil) {
		t.Fatalf("с плохим ключом должен взяться свой сеанс: %d", got)
	}
}

func TestLazyCallerWithoutServiceReturnsError(t *testing.T) {
	// Канал службы не поднят: опрос возвращает ошибку, а не зависает и не паникует.
	l := &lazyCaller{}
	defer l.close()
	var st ipc.ChildStatus
	if err := ipc.Call(l, ipc.CmdGetChildStatus, "", nil, &st); err == nil {
		t.Skip("на этой машине служба PCBlock уже работает")
	}
}

var procFindWindow = user32.NewProc("FindWindowW")

func findHost(class string) uintptr {
	h, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(wstr(class))), 0)
	return h
}

// Ловит: значок не запускается (не создаёт окно, не занимает мьютекс), второй значок в том же
// сеансе не выходит молча, а закрытие окна не завершает Run.
func TestRunStartsOneTrayAndStopsOnClose(t *testing.T) {
	if c, err := ipc.Dial(ipc.PipeName, 100*time.Millisecond); err == nil {
		_ = c.Close()
		t.Skip("на этой машине служба PCBlock уже работает")
	}
	opts := launch.Options{"session": "4242421"}
	done := make(chan error, 1)
	go func() { done <- Run(opts) }()

	var host uintptr
	for i := 0; i < 100 && host == 0; i++ {
		host = findHost("PCBlockTrayHost")
		time.Sleep(50 * time.Millisecond)
	}
	if host == 0 {
		select {
		case err := <-done:
			t.Skipf("значок не запустился (нет рабочего стола?): %v", err)
		default:
			t.Fatal("окно значка не появилось за 5 секунд")
		}
	}

	second := make(chan error, 1)
	go func() { second <- Run(opts) }()
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("второй значок в сеансе должен выйти молча, а вернул: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("второй значок не вышел")
	}

	time.Sleep(300 * time.Millisecond)       // дать циклу опроса и значку один оборот
	procPostMessage.Call(host, 0x0010, 0, 0) // WM_CLOSE
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run завершился с ошибкой: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run не завершился после закрытия окна")
	}
}

// Ловит: паника при рисовании панели и её кнопок во всех видах.
func TestPanelPaintAndOwnerDraw(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := newPanel(nil)
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer p.close()

	for _, v := range []PanelView{
		BuildPanel(freshSnapshot(okStatus(85*60, 7200)), "Не получилось открыть окно. Попробуй ещё раз"),
		BuildPanel(freshSnapshot(okStatus(10*60, 7200)), ""),
		BuildPanel(freshSnapshot(ipc.ChildStatusNotChild()), ""),
		BuildPanel(Snapshot{}, ""),
	} {
		for _, busy := range []bool{false, true} {
			p.busy = busy
			p.sig = ""
			p.setView(v)
			p.paint()
			hdc, _, _ := procGetDC.Call(p.hwnd)
			for i := range v.Buttons {
				for _, state := range []uint32{0, odsFocus, odsSelected | odsFocus} {
					di := drawItemStruct{ctlID: uint32(idPanelButtonBase + i), hdc: hdc, rcItem: rect{0, 0, 280, 40}, itemState: state}
					p.drawButton(&di)
				}
			}
			procReleaseDC.Call(p.hwnd, hdc)
		}
	}
	di := drawItemStruct{ctlID: 9999}
	p.drawButton(&di) // чужой номер кнопки: молча пропускается
}
