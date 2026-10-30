//go:build windows

package guard

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	thePlate     *plateWindow
	plateProcPtr uintptr

	procTrackMouseEvent = user32.NewProc("TrackMouseEvent")
	procSetCapture      = user32.NewProc("SetCapture")
	procReleaseCapture  = user32.NewProc("ReleaseCapture")
	procMoveToEx        = gdi32.NewProc("MoveToEx")
	procLineTo          = gdi32.NewProc("LineTo")
)

const (
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmMouseLeave  = 0x02A3
	tmeLeave      = 0x00000002
	wmPlatePress  = wmApp + 30 // нажатие кнопки плашки разбирается не внутри сообщения мыши

	plateNone  = -2 // мимо кнопок (PlateLayout.Hit)
	plateClose = -1 // крестик
)

func init() { plateProcPtr = windows.NewCallback(plateProc) }

type trackMouseEvent struct {
	size, flags uint32
	hwnd        uintptr
	hover       uint32
}

// plateWindow — текстовая плашка справа внизу (FR-9, FR-13, ТЗ 8.2.2, 8.2.4): «Осталась 1 минута:
// 0:59» с кнопками «Взять из копилки» и «Попросить ещё», «Добавлено N минут», плашки о закрытых
// программах с крестиком и «Попросить сейчас». Кнопки — области окна (как крестик табло), а не
// дочерние окна: щелчок не активирует плашку. Окно не берёт фокус: WS_EX_NOACTIVATE, показ без
// активации, MA_NOACTIVATE на щелчок, поэтому игра остаётся в фокусе и на весь экран. Плашки
// 30/15/5 мин рисует общая карточка (systray.WarningCard). Что показать, когда убрать и что
// делает кнопка, решает Model; окно только рисует и сообщает нажатия (onPress).
type plateWindow struct {
	hwnd, hInst  uintptr
	dpi          int
	fonts        fontSet
	btnFont      uintptr
	surfaceBrush uintptr
	accentBrush  uintptr
	onPress      func(Button)

	text     string
	buttons  []ButtonView // кнопки столбцом (без крестика)
	hasClose bool         // есть крестик
	key      string
	visible  bool
	geom     rect
	lay      PlateLayout

	hover, down int // кнопка под мышью и нажатая: номер, plateClose или plateNone
	tracking    bool
}

func newPlateWindow(onPress func(Button)) (*plateWindow, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	if onPress == nil {
		onPress = func(Button) {}
	}
	p := &plateWindow{hInst: hInst, dpi: systemDPI(), onPress: onPress, hover: plateNone, down: plateNone}
	p.surfaceBrush, _, _ = procCreateSolidBr.Call(colorSurface)
	p.accentBrush, _, _ = procCreateSolidBr.Call(colorAmber)
	p.fonts = newFontSet(p.dpi)
	p.btnFont = makeFont(Scaled(16, p.dpi), 600)
	thePlate = p
	cls, err := registerPlainClass("PCBlockGuardPlate", plateProcPtr, hInst, p.surfaceBrush)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(плашка): %w", err)
	}
	hwnd, _, err := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWin|wsExNoActivate,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(utf16("PC Block"))),
		wsPopup|wsClipChildren,
		0, 0, 10, 10,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx(плашка): %w", err)
	}
	p.hwnd = hwnd
	return p, nil
}

// plateKey — подпись плашки для сравнения кадров: текст и кнопки.
func plateKey(w *WarningView) string {
	var b strings.Builder
	b.WriteString(w.Text)
	for _, v := range w.Buttons {
		fmt.Fprintf(&b, "\x00%d:%s:%t", v.Button, v.Label, v.Enabled)
	}
	return b.String()
}

// apply показывает плашку по View.Warning или прячет её (nil).
func (p *plateWindow) apply(w *WarningView) {
	if w == nil {
		p.hide()
		return
	}
	key := plateKey(w)
	if key == p.key && p.visible {
		// Поверх полноэкранной игры: «поверх всех» подтверждается без активации каждый кадр.
		procSetWindowPos.Call(p.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		return
	}
	textChanged := w.Text != p.text
	p.key, p.text = key, w.Text
	p.buttons, p.hasClose = p.buttons[:0], false
	for _, b := range w.Buttons {
		if b.Button == ButtonPlateClose {
			p.hasClose = true
			continue
		}
		p.buttons = append(p.buttons, b)
	}
	if textChanged {
		// Заголовок окна — текст плашки: его читает экранный диктор (доступность).
		setWindowText(p.hwnd, w.Text)
	}
	p.layout()
	invalidate(p.hwnd)
}

func (p *plateWindow) hide() {
	if !p.visible {
		return
	}
	p.visible, p.key = false, ""
	p.hover, p.down = plateNone, plateNone
	procShowWindow.Call(p.hwnd, swHide)
}

// layout считает размер по тексту и кнопкам (PlateLayout) и ставит окно в правый нижний угол
// рабочей области поверх всех окон без активации.
func (p *plateWindow) layout() {
	hdc, _, _ := procGetDC.Call(p.hwnd)
	textH := measureHeight(hdc, p.fonts.body, p.text, PlateTextWidth(p.dpi, p.hasClose))
	procReleaseDC.Call(p.hwnd, hdc)
	p.lay = NewPlateLayout(p.dpi, textH, len(p.buttons), p.hasClose)
	s := func(px int) int32 { return Scaled(px, p.dpi) }
	width, h := p.lay.W, p.lay.H
	wa := workArea()
	g := rect{wa.right - width - s(16), wa.bottom - h - s(16), wa.right - s(16), wa.bottom - s(16)}
	if p.visible && g == p.geom {
		return
	}
	p.geom, p.visible = g, true
	procSetWindowPos.Call(p.hwnd, hwndTopmost, uintptr(g.left), uintptr(g.top), uintptr(width), uintptr(h),
		swpNoActivate|swpShowWindow)
}

func (p *plateWindow) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	roundBox(hdc, rc, colorSurface, colorLine, 1, 0)
	bar := rect{rc.left + 1, rc.top + 1, rc.left + 1 + Scaled(6, p.dpi), rc.bottom - 1}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&bar)), p.accentBrush)
	procSetBkMode.Call(hdc, transparent)
	drawWrapped(hdc, p.fonts.body, colorText, p.text, toRect(p.lay.Text), 0)
	for i, b := range p.buttons {
		if i >= len(p.lay.Buttons) {
			break
		}
		look := lookOf(b.Main, b.Enabled, p.down == i || (p.down == plateNone && p.hover == i))
		drawButtonRect(hdc, toRect(p.lay.Buttons[i]), b.Label, p.btnFont, look, Scaled(6, p.dpi), p.surfaceBrush, false)
	}
	if p.hasClose {
		p.paintClose(hdc)
	}
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

// paintClose рисует крестик «Закрыть»: подложка при наведении и нажатии, две линии.
func (p *plateWindow) paintClose(hdc uintptr) {
	r := toRect(p.lay.Close)
	if p.hover == plateClose || p.down == plateClose {
		roundBox(hdc, r, colorGraphite, colorLine, 1, Scaled(6, p.dpi))
	}
	in := Scaled(9, p.dpi)
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(Scaled(2, p.dpi)), colorText)
	old, _, _ := procSelectObject.Call(hdc, pen)
	procMoveToEx.Call(hdc, uintptr(r.left+in), uintptr(r.top+in), 0)
	procLineTo.Call(hdc, uintptr(r.right-in), uintptr(r.bottom-in))
	procMoveToEx.Call(hdc, uintptr(r.right-in-1), uintptr(r.top+in), 0)
	procLineTo.Call(hdc, uintptr(r.left+in-1), uintptr(r.bottom-in))
	procSelectObject.Call(hdc, old)
	procDeleteObject.Call(pen)
}

// hit — что под точкой lParam (клиентские координаты): номер кнопки, plateClose или plateNone.
func (p *plateWindow) hit(lParam uintptr) int {
	x, y := int32(int16(lParam&0xFFFF)), int32(int16(lParam>>16&0xFFFF))
	h := p.lay.Hit(x, y)
	if h == plateClose && !p.hasClose || h >= len(p.buttons) {
		return plateNone
	}
	if h >= 0 && !p.buttons[h].Enabled {
		return plateNone
	}
	return h
}

func (p *plateWindow) buttonAt(i int) Button {
	switch {
	case i == plateClose:
		return ButtonPlateClose
	case i >= 0 && i < len(p.buttons):
		return p.buttons[i].Button
	}
	return ButtonNone
}

// click нажимает кнопку b мышью тем же путём, что человек (тестовый режим, TestGuard click):
// WM_LBUTTONDOWN и WM_LBUTTONUP в её центре. Кнопки нет на плашке — ничего.
func (p *plateWindow) click(b Button) bool {
	if !p.visible {
		return false
	}
	var r Rect
	switch {
	case b == ButtonPlateClose && p.hasClose:
		r = p.lay.Close
	default:
		found := false
		for i, v := range p.buttons {
			if v.Button == b && i < len(p.lay.Buttons) {
				r, found = p.lay.Buttons[i], true
			}
		}
		if !found {
			return false
		}
	}
	x, y := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	lp := uintptr(uint32(y)<<16 | uint32(x)&0xFFFF)
	procSendMessage.Call(p.hwnd, wmLButtonDown, 1, lp)
	procSendMessage.Call(p.hwnd, wmLButtonUp, 0, lp)
	return true
}

// close уничтожает окно и освобождает ресурсы.
func (p *plateWindow) close() {
	if p.hwnd != 0 {
		procDestroyWindow.Call(p.hwnd)
		p.hwnd = 0
	}
	p.fonts.free()
	for _, b := range []uintptr{p.surfaceBrush, p.accentBrush, p.btnFont} {
		if b != 0 {
			procDeleteObject.Call(b)
		}
	}
	thePlate = nil
}

func plateProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if p := thePlate; p != nil && p.hwnd == hwnd {
		switch uint32(msg) {
		case wmPaint:
			p.paint(hwnd)
			return 0
		case wmMouseActivate:
			return maNoActivate // щелчок по кнопке не забирает фокус у игры
		case wmMouseMove:
			if !p.tracking {
				tme := trackMouseEvent{flags: tmeLeave, hwnd: hwnd}
				tme.size = uint32(unsafe.Sizeof(tme))
				procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				p.tracking = true
			}
			if h := p.hit(lParam); h != p.hover {
				p.hover = h
				invalidate(hwnd)
			}
			return 0
		case wmMouseLeave:
			p.tracking = false
			if p.hover != plateNone {
				p.hover = plateNone
				invalidate(hwnd)
			}
			return 0
		case wmLButtonDown:
			if h := p.hit(lParam); h != plateNone {
				procSetCapture.Call(hwnd)
				p.down = h
				invalidate(hwnd)
			}
			return 0
		case wmLButtonUp:
			procReleaseCapture.Call()
			down := p.down
			p.down = plateNone
			invalidate(hwnd)
			if down != plateNone && p.hit(lParam) == down {
				if b := p.buttonAt(down); b != ButtonNone {
					procPostMessage.Call(hwnd, wmPlatePress, uintptr(b), 0)
				}
			}
			return 0
		case wmPlatePress:
			p.onPress(Button(wParam))
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
