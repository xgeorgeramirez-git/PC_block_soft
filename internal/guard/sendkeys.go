package guard

import "errors"

// Решение «слать Ctrl+W или нет» (RM4-2) без Windows: нажатия уходят окну переднего плана, поэтому
// непосредственно перед отправкой окно переднего плана сверяется с окном, которое контроллер
// только что прочитал (Look). Исполнитель — sendkeys_windows.go, в тестах — подделка.

// errForegroundChanged — на переднем плане уже другое окно: Ctrl+W закрыл бы вкладку чужой
// программы; шаг не засчитывается, ничего не послано (как errModifierHeld).
var errForegroundChanged = errors.New("окно переднего плана сменилось")

// ctrlWPort — что нужно от Windows для Ctrl+W.
type ctrlWPort struct {
	foreground func() uintptr // GetForegroundWindow
	held       func() bool    // зажат Shift, Alt или Win
	send       func() error   // SendInput: Ctrl+W
}

// sendCtrlW нажимает Ctrl+W, только если ничего не зажато и window всё ещё на переднем плане.
func sendCtrlW(p ctrlWPort, window uintptr) error {
	if p.held() {
		return errModifierHeld
	}
	if window == 0 || p.foreground() != window {
		return errForegroundChanged
	}
	return p.send()
}
