//go:build windows

package guard

import (
	"unsafe"

	"pcblock/internal/ipc"
)

// runTestAction выполняет действие TestGuard (Ruling 5). Окно стража работает от имени системы,
// и обычный процесс (даже администратора) не может послать ему ввод: это защита от ребёнка.
// Поэтому в тестовом режиме служба передаёт действие стражу, а страж исполняет его через
// настоящие сообщения собственных окон: нажатие кнопки это BM_CLICK, ввод в поле это
// WM_SETTEXT, закрытие это WM_CLOSE, WM_SYSCOMMAND/SC_CLOSE (Alt+F4) и WM_KEYDOWN Esc. Кнопки,
// которой сейчас нет на экране или которая недоступна, действие не касается, как и у человека.
// Действие endSession изображает конец сеанса Windows (FB-1): настоящий выход из сеанса раннера
// CI устроить нельзя. Вызывается только из потока окон.
func (a *app) runTestAction(t ipc.TestGuardAction) {
	switch t.Action {
	case ipc.TestClick:
		b, ok := ButtonByTarget(t.Target)
		if !ok {
			return
		}
		if b == ButtonWarningClose {
			// Крестик табло: щелчок мыши окну карточки, тот же путь, что у человека.
			if a.board != nil && a.board.Visible() {
				a.board.ClickClose()
			}
			return
		}
		if isPlateButton(b) {
			// Кнопки текстовой плашки: щелчок мыши в её окне (RM3-4, RM3-5).
			if a.plate != nil {
				a.plate.click(b)
			}
			return
		}
		var h uintptr
		if a.ui != nil && a.lock != nil && a.lock.Visible() {
			h = a.ui.buttonHandle(b)
		}
		if h != 0 {
			procSendMessage.Call(h, bmClick, 0, 0)
		}
	case ipc.TestType:
		// Поле одно: пароль на панели пароля, промокод на панели промокода (FR-35).
		want := FieldPasswordID
		if a.ui != nil && a.ui.view.Panel == PanelPromo {
			want = FieldPromoID
		}
		if t.Target == want && a.ui != nil && a.lock != nil && a.lock.Visible() && a.ui.edit != 0 {
			procSendMessage.Call(a.ui.edit, wmSetText, 0, uintptr(unsafe.Pointer(utf16(t.Text))))
		}
	case ipc.TestWMClose:
		if a.lock != nil {
			procSendMessage.Call(a.lock.hwnd, wmClose, 0, 0)
		}
	case ipc.TestAltF4:
		if a.lock != nil {
			procSendMessage.Call(a.lock.hwnd, wmSysCommand, scClose, 0)
		}
	case ipc.TestEsc:
		if a.lock != nil {
			procSendMessage.Call(a.lock.hwnd, wmKeyDown, vkEscape, 0)
		}
	case ipc.TestEndSession:
		// Тот же путь, что при WM_ENDSESSION: сообщить службе GuardClosing и завершиться.
		a.endSession()
	}
}
