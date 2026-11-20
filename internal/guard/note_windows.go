//go:build windows

package guard

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Окно плашки «Сообщение от родителя» (FR-41): сверху по центру, поверх всех окон и поверх экрана
// блокировки PC Block — там между его заголовком и столбцом кнопок, не закрывая ни то, ни другое
// (FitNote, NFR-10); не выше 40 % рабочей области. Длинный текст переносится по словам и виден
// целиком: плашка шире (до 80 %), над экраном блокировки он раздвигается (кнопки ниже, заголовок
// выше), и только потом шрифт мельче, не меньше 14 px. Не берёт фокус (WS_EX_NOACTIVATE, MA_NOACTIVATE): экран блокировки держит
// ввод пароля и клавиатуру, игра — полный экран. Кнопка «Понял» — область окна, как у плашки
// (warnings_windows.go). Что показать и когда убрать, решает Model; окно рисует и сообщает нажатие.

var (
	theNote     *noteWindow
	noteProcPtr uintptr
)

const wmNotePress = wmApp + 31 // нажатие «Понял» разбирается не внутри сообщения мыши

func init() { noteProcPtr = windows.NewCallback(noteProc) }

// Цвета плашки сообщения: рамка нейтральная, как у плашки предупреждения (colorLine), без цветной
// полосы; заголовок — основным светлым цветом текста (контраст с поверхностью выше 4,5:1).
var (
	noteFrameColor = colorLine
	noteTitleColor = colorText
)

type noteWindow struct {
	hwnd, hInst  uintptr
	dpi          int
	fonts        map[[2]int]uintptr // шрифт по размеру при 96 dpi и жирности
	surfaceBrush uintptr
	onPress      func(Button)

	key     string
	title   string
	text    string // как прислала служба
	button  ButtonView
	visible bool
	geom    rect
	fit     NoteFit

	hover, down bool
	tracking    bool
}

func newNoteWindow(onPress func(Button)) (*noteWindow, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	if onPress == nil {
		onPress = func(Button) {}
	}
	n := &noteWindow{hInst: hInst, dpi: systemDPI(), onPress: onPress, fonts: map[[2]int]uintptr{}}
	n.surfaceBrush, _, _ = procCreateSolidBr.Call(colorSurface)
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

// font — шрифт размера px (при 96 dpi) и жирности weight; создаётся один раз.
func (n *noteWindow) font(px, weight int) uintptr {
	k := [2]int{px, weight}
	if f, ok := n.fonts[k]; ok {
		return f
	}
	f := makeFont(Scaled(px, n.dpi), weight)
	n.fonts[k] = f
	return f
}

// apply показывает плашку по View.Note или прячет её (nil). lock — экран блокировки под плашкой
// (nil — его нет): плашка встаёт между его заголовком и кнопками.
func (n *noteWindow) apply(v *NoteView, lock *NoteLock) {
	if v == nil {
		n.hide()
		return
	}
	key := fmt.Sprintf("%d\x00%s\x00%s\x00%v", v.Seq, v.Title, v.Text, lock)
	if key == n.key && n.visible {
		n.raise()
		return
	}
	n.key, n.title, n.text = key, v.Title, v.Text
	n.button = ButtonView{}
	if len(v.Buttons) > 0 {
		n.button = v.Buttons[0]
	}
	// Заголовок окна — подпись и текст целиком: его читает экранный диктор (доступность).
	setWindowText(n.hwnd, v.Title+": "+v.Text)
	n.hover, n.down = false, false
	n.layout(lock)
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

// layout подбирает плашку под место (FitNote: ширина, плотность, шрифт, сдвиг экрана блокировки) и ставит окно: на рабочем
// столе сверху по центру рабочей области, над экраном блокировки — между заголовком и кнопками.
func (n *noteWindow) layout(lock *NoteLock) {
	wa := workArea()
	hdc, _, _ := procGetDC.Call(n.hwnd)
	measure := func(px int, width int32, s string) int32 {
		weight := 400
		if s == n.title { // заголовок полужирный; текст родителя, совпавший с ним, мерится так же — не страшно
			weight = 600
		}
		return measureHeight(hdc, n.font(px, weight), s, width)
	}
	n.fit = FitNote(n.dpi, Rect{Left: wa.left, Top: wa.top, Right: wa.right, Bottom: wa.bottom}, lock, n.title, n.text, measure)
	procReleaseDC.Call(n.hwnd, hdc)
	g := toRect(n.fit.Place)
	n.geom, n.visible = g, true
	procSetWindowPos.Call(n.hwnd, hwndTopmost, uintptr(g.left), uintptr(g.top), uintptr(g.right-g.left), uintptr(g.bottom-g.top),
		swpNoActivate|swpShowWindow)
}

func (n *noteWindow) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	// Нейтральная рамка, как у остальных плашек стража: цвет — только для статусов (ТЗ 8.0).
	roundBox(hdc, rc, colorSurface, noteFrameColor, 1, 0)
	procSetBkMode.Call(hdc, transparent)
	l, d := n.fit.Layout, n.fit.Density
	// Заголовок — светлым и крупнее текста подписи (контраст как у основного текста); текст родителя —
	// по левому краю: переводы строк и списки читаются как написаны.
	drawWrapped(hdc, n.font(d.TitlePx, 600), noteTitleColor, n.title, toRect(l.Title), 0)
	drawWrapped(hdc, n.font(n.fit.FontPx, 400), colorText, n.fit.Text, toRect(l.Text), 0)
	if n.button.Button != ButtonNone {
		look := lookOf(n.button.Main, n.button.Enabled, n.down || n.hover)
		btnPx := 22
		if d == NoteCompact {
			btnPx = 18
		}
		drawButtonRect(hdc, toRect(l.Button), n.button.Label, n.font(btnPx, 600), look, Scaled(8, n.dpi), n.surfaceBrush, false)
	}
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

// hit: точка lParam (клиентские координаты) на кнопке «Понял».
func (n *noteWindow) hit(lParam uintptr) bool {
	x, y := int32(int16(lParam&0xFFFF)), int32(int16(lParam>>16&0xFFFF))
	r := n.fit.Layout.Button
	return n.button.Button != ButtonNone && n.button.Enabled && x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// click нажимает «Понял» мышью тем же путём, что человек (тестовый режим, TestGuard click).
func (n *noteWindow) click() bool {
	if !n.visible || n.button.Button == ButtonNone {
		return false
	}
	r := n.fit.Layout.Button
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
	if n.surfaceBrush != 0 {
		procDeleteObject.Call(n.surfaceBrush)
	}
	for _, f := range n.fonts {
		procDeleteObject.Call(f)
	}
	n.fonts = map[[2]int]uintptr{}
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
