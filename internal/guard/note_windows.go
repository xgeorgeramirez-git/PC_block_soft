//go:build windows

package guard

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Окно плашки «Сообщение от родителя» (FR-41): сверху по центру, поверх всех окон и поверх экрана
// блокировки PC Block. Не берёт фокус (WS_EX_NOACTIVATE, MA_NOACTIVATE): экран блокировки держит
// ввод пароля и клавиатуру, игра — полный экран. Кнопка «Понял» — область окна, как у плашки
// (warnings_windows.go). Что показать и когда убрать, решает Model; окно рисует и сообщает нажатие.

var (
	theNote     *noteWindow
	noteProcPtr uintptr
)

const wmNotePress = wmApp + 31 // нажатие «Понял» разбирается не внутри сообщения мыши

func init() { noteProcPtr = windows.NewCallback(noteProc) }

type noteWindow struct {
	hwnd, hInst  uintptr
	dpi          int
	titleFont    uintptr
	textFont     uintptr
	btnFont      uintptr
	surfaceBrush uintptr
	accentBrush  uintptr
	onPress      func(Button)

	key     string
	title   string
	text    string
	button  ButtonView
	visible bool
	geom    rect
	lay     NoteLayout

	hover, down bool
	tracking    bool
}

func newNoteWindow(onPress func(Button)) (*noteWindow, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	if onPress == nil {
		onPress = func(Button) {}
	}
	n := &noteWindow{hInst: hInst, dpi: systemDPI(), onPress: onPress}
	n.surfaceBrush, _, _ = procCreateSolidBr.Call(colorSurface)
	n.accentBrush, _, _ = procCreateSolidBr.Call(colorNoteAccent)
	n.titleFont = makeFont(Scaled(20, n.dpi), 600)
	n.textFont = makeFont(Scaled(28, n.dpi), 400)
	n.btnFont = makeFont(Scaled(22, n.dpi), 600)
	theNote = n
	cls, err := registerPlainClass("PCBlockGuardNote", noteProcPtr, hInst, n.surfaceBrush)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(сообщение): %w", err)
	}
	hwnd, _, err := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWin|wsExNoActivate,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(utf16(textNoteTitle))),
		wsPopup|wsClipChildren,
		0, 0, 10, 10,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx(сообщение): %w", err)
	}
	n.hwnd = hwnd
	return n, nil
}

// colorNoteAccent — полоса плашки сообщения: спокойный синий, не «время кончается» (янтарь).
var colorNoteAccent = rgb(0x3D, 0x7E, 0xC9)

// apply показывает плашку по View.Note или прячет её (nil).
func (n *noteWindow) apply(v *NoteView) {
	if v == nil {
		n.hide()
		return
	}
	key := fmt.Sprintf("%d\x00%s\x00%s", v.Seq, v.Title, v.Text)
	if key == n.key && n.visible {
		n.raise()
		return
	}
	n.key, n.title, n.text = key, v.Title, v.Text
	n.button = ButtonView{}
	if len(v.Buttons) > 0 {
		n.button = v.Buttons[0]
	}
	// Заголовок окна — подпись и текст: его читает экранный диктор (доступность).
	setWindowText(n.hwnd, v.Title+": "+v.Text)
	n.hover, n.down = false, false
	n.layout()
	invalidate(n.hwnd)
}

// raise держит плашку над экраном блокировки и полноэкранной игрой без активации. Зовут кадр стража
// и таймер экрана блокировки (он каждые 500 мс поднимает себя наверх).
func (n *noteWindow) raise() {
	if n.visible {
		procSetWindowPos.Call(n.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	}
}

func (n *noteWindow) hide() {
	if !n.visible {
		return
	}
	n.visible, n.key = false, ""
	n.hover, n.down = false, false
	procShowWindow.Call(n.hwnd, swHide)
}

// layout считает размер по тексту (NoteLayout) и ставит окно сверху по центру рабочей области.
func (n *noteWindow) layout() {
	wa := workArea()
	w := NoteWidth(n.dpi, wa.right-wa.left)
	tw := NoteTextWidth(n.dpi, w)
	hdc, _, _ := procGetDC.Call(n.hwnd)
	titleH := measureHeight(hdc, n.titleFont, n.title, tw)
	textH := measureHeight(hdc, n.textFont, n.text, tw)
	procReleaseDC.Call(n.hwnd, hdc)
	n.lay = NewNoteLayout(n.dpi, w, titleH, textH)
	p := NotePlace(n.dpi, Rect{Left: wa.left, Top: wa.top, Right: wa.right, Bottom: wa.bottom}, w, n.lay.H)
	g := toRect(p)
	n.geom, n.visible = g, true
	procSetWindowPos.Call(n.hwnd, hwndTopmost, uintptr(g.left), uintptr(g.top), uintptr(g.right-g.left), uintptr(g.bottom-g.top),
		swpNoActivate|swpShowWindow)
}

func (n *noteWindow) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	roundBox(hdc, rc, colorSurface, colorNoteAccent, 2, 0)
	bar := rect{rc.left + 1, rc.top + 1, rc.left + 1 + Scaled(noteBarPx, n.dpi), rc.bottom - 1}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&bar)), n.accentBrush)
	procSetBkMode.Call(hdc, transparent)
	// Текст родителя — по левому краю: переводы строк и списки читаются как написаны.
	drawWrapped(hdc, n.titleFont, colorSecondary, n.title, toRect(n.lay.Title), 0)
	drawWrapped(hdc, n.textFont, colorText, n.text, toRect(n.lay.Text), 0)
	if n.button.Button != ButtonNone {
		look := lookOf(n.button.Main, n.button.Enabled, n.down || n.hover)
		drawButtonRect(hdc, toRect(n.lay.Button), n.button.Label, n.btnFont, look, Scaled(8, n.dpi), n.surfaceBrush, false)
	}
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

// hit: точка lParam (клиентские координаты) на кнопке «Понял».
func (n *noteWindow) hit(lParam uintptr) bool {
	x, y := int32(int16(lParam&0xFFFF)), int32(int16(lParam>>16&0xFFFF))
	r := n.lay.Button
	return n.button.Button != ButtonNone && n.button.Enabled && x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// click нажимает «Понял» мышью тем же путём, что человек (тестовый режим, TestGuard click).
func (n *noteWindow) click() bool {
	if !n.visible || n.button.Button == ButtonNone {
		return false
	}
	r := n.lay.Button
	x, y := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	lp := uintptr(uint32(y)<<16 | uint32(x)&0xFFFF)
	procSendMessage.Call(n.hwnd, wmLButtonDown, 1, lp)
	procSendMessage.Call(n.hwnd, wmLButtonUp, 0, lp)
	return true
}

func (n *noteWindow) close() {
	if n.hwnd != 0 {
		procDestroyWindow.Call(n.hwnd)
		n.hwnd = 0
	}
	for _, b := range []uintptr{n.surfaceBrush, n.accentBrush, n.titleFont, n.textFont, n.btnFont} {
		if b != 0 {
			procDeleteObject.Call(b)
		}
	}
	theNote = nil
}

// raiseNoteAboveLock — таймер экрана блокировки поднял его наверх: плашка сообщения снова над ним.
func raiseNoteAboveLock() {
	if n := theNote; n != nil {
		n.raise()
	}
}

func noteProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if n := theNote; n != nil && n.hwnd == hwnd {
		switch uint32(msg) {
		case wmPaint:
			n.paint(hwnd)
			return 0
		case wmMouseActivate:
			return maNoActivate // щелчок не забирает фокус у экрана блокировки и игры
		case wmMouseMove:
			if !n.tracking {
				tme := trackMouseEvent{flags: tmeLeave, hwnd: hwnd}
				tme.size = uint32(unsafe.Sizeof(tme))
				procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				n.tracking = true
			}
			if h := n.hit(lParam); h != n.hover {
				n.hover = h
				invalidate(hwnd)
			}
			return 0
		case wmMouseLeave:
			n.tracking = false
			if n.hover {
				n.hover = false
				invalidate(hwnd)
			}
			return 0
		case wmLButtonDown:
			if n.hit(lParam) {
				procSetCapture.Call(hwnd)
				n.down = true
				invalidate(hwnd)
			}
			return 0
		case wmLButtonUp:
			procReleaseCapture.Call()
			was := n.down
			n.down = false
			invalidate(hwnd)
			if was && n.hit(lParam) {
				procPostMessage.Call(hwnd, wmNotePress, uintptr(n.button.Button), 0)
			}
			return 0
		case wmNotePress:
			n.onPress(Button(wParam))
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
