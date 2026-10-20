//go:build windows

package systray

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	idPanelButtonBase = 2000
	wmPanelPress      = wmApp + 20 // нажатие кнопки обрабатывается не внутри её же сообщения

	panelTimerID    = 11
	panelTimerEvery = 1000 // мс
	panelMaxShown   = 45 * time.Second
	// panelReopenGuard: щелчок по значку, закрывшему панель потерей активации, не открывает её
	// заново: для человека это одно и то же нажатие («значок закрывает панель»).
	panelReopenGuard = 400 * time.Millisecond
)

var panelProcPtr uintptr

func init() { panelProcPtr = windows.NewCallback(panelProc) }

// panelFonts — шрифты панели с учётом dpi.
type panelFonts struct{ title, body, button uintptr }

func newPanelFonts(dpi int) panelFonts {
	return panelFonts{
		title:  makeFont(scaled(17, dpi), 600),
		body:   makeFont(scaled(14, dpi), 400),
		button: makeFont(scaled(14, dpi), 600),
	}
}

func (f panelFonts) free() {
	for _, h := range []uintptr{f.title, f.body, f.button} {
		if h != 0 {
			procDeleteObject.Call(h)
		}
	}
}

// panel — мини-панель у часов (ТЗ 8.2.1). Что показать, решает BuildPanel; окно только
// рисует и сообщает нажатия. Работает только в потоке окон.
type panel struct {
	hwnd, hInst uintptr
	dpi         int
	fonts       panelFonts
	onPress     func(PanelButton)

	view     PanelView
	sig      string
	busy     bool // команда службе в пути: кнопки не нажимаются
	visible  bool
	shownAt  time.Time
	hiddenAt time.Time

	buttons []uintptr
	titleRc rect
	barRc   rect
	noteRc  rect
	height  int32
}

func newPanel(onPress func(PanelButton)) (*panel, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	p := &panel{hInst: hInst, dpi: systemDPI(), onPress: onPress}
	p.fonts = newPanelFonts(p.dpi)
	brush, _, _ := procCreateSolidBrush.Call(uiSurface)
	cls, err := registerPlainClass("PCBlockTrayPanel", panelProcPtr, hInst, brush)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(панель): %w", err)
	}
	thePanel = p
	hwnd, _, err := procCreateWindowEx.Call(
		wsExToolWin|wsExTopmost,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(wstr("PC Block"))),
		wsPopup|wsClipChildren,
		0, 0, 10, 10,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx(панель): %w", err)
	}
	p.hwnd = hwnd
	return p, nil
}

var thePanel *panel

// width — ширина панели в пикселях экрана.
func (p *panel) width() int32 { return scaled(320, p.dpi) }

// setView принимает новый вид и перестраивает окно, если он изменился.
func (p *panel) setView(v PanelView) {
	sig := fmt.Sprintf("%+v|%t", v, p.busy)
	if sig == p.sig {
		return
	}
	p.sig, p.view = sig, v
	p.layout()
	invalidate(p.hwnd)
}

func invalidate(hwnd uintptr) { procInvalidateRect.Call(hwnd, 0, 1) }

// layout считает размеры по тексту, пересоздаёт кнопки и меняет размер окна.
func (p *panel) layout() {
	for _, b := range p.buttons {
		procDestroyWindow.Call(b)
	}
	p.buttons = p.buttons[:0]

	hdc, _, _ := procGetDC.Call(p.hwnd)
	defer procReleaseDC.Call(p.hwnd, hdc)
	s := func(px int) int32 { return scaled(px, p.dpi) }
	w, pad := p.width(), s(16)
	inner := w - 2*pad
	y := pad

	th := measureHeight(hdc, p.fonts.title, p.view.Title, inner)
	p.titleRc = rect{pad, y, pad + inner, y + th}
	y += th + s(12)

	p.barRc = rect{}
	if p.view.Bar != nil {
		p.barRc = rect{pad, y, pad + inner, y + s(8)}
		y += s(8) + s(14)
	}

	p.noteRc = rect{}
	if p.view.Note != "" {
		nh := measureHeight(hdc, p.fonts.body, p.view.Note, inner)
		p.noteRc = rect{pad, y, pad + inner, y + nh}
		y += nh + s(12)
	}

	for i, b := range p.view.Buttons {
		h := s(40)
		if b.Small {
			h = s(28)
		}
		rc := rect{pad, y, pad + inner, y + h}
		hb := createChild(p.hwnd, p.hInst, "BUTTON", b.Label, wsTabStop|bsOwnerDraw, idPanelButtonBase+i, rc)
		p.buttons = append(p.buttons, hb)
		y += h + s(8)
	}
	y += pad - s(8)
	p.height = y

	procSetWindowPos.Call(p.hwnd, 0, 0, 0, uintptr(w), uintptr(p.height), swpNoMove|swpNoActive|0x0004) // SWP_NOZORDER
}

// show открывает панель рядом с anchor.
func (p *panel) show(anchor Rect, haveAnchor bool) {
	if time.Since(p.hiddenAt) < panelReopenGuard {
		return
	}
	if !haveAnchor {
		var pt point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		anchor = Rect{int(pt.x), int(pt.y), int(pt.x), int(pt.y)}
	}
	work := monitorWork(rect{int32(anchor.Left), int32(anchor.Top), int32(anchor.Right), int32(anchor.Bottom)}).toRect()
	x, y := PlacePanel(anchor, work, int(p.width()), int(p.height))
	procSetWindowPos.Call(p.hwnd, hwndTopmost, uintptr(x), uintptr(y), uintptr(p.width()), uintptr(p.height), 0x0040) // SWP_SHOWWINDOW
	procShowWindow.Call(p.hwnd, swShow)
	procSetForegroundWindow.Call(p.hwnd)
	p.visible, p.shownAt = true, time.Now()
	procSetTimer.Call(p.hwnd, panelTimerID, panelTimerEvery, 0)
	if len(p.buttons) > 0 {
		procSetFocus.Call(p.buttons[0])
	}
}

func (p *panel) hide() {
	if !p.visible {
		return
	}
	p.visible, p.hiddenAt = false, time.Now()
	procKillTimer.Call(p.hwnd, panelTimerID)
	procShowWindow.Call(p.hwnd, swHide)
}

// setBusy включает и выключает состояние «команда в пути».
func (p *panel) setBusy(b bool) {
	if p.busy == b {
		return
	}
	p.busy = b
	p.sig = ""
	p.setView(p.view)
}

// focusedIndex возвращает номер кнопки, у которой фокус (-1: фокус не на кнопке).
func (p *panel) focusedIndex() int {
	f, _, _ := procGetFocus.Call()
	for i, h := range p.buttons {
		if h == f {
			return i
		}
	}
	return -1
}

// pressIndex нажимает кнопку по номеру.
func (p *panel) pressIndex(i int) {
	if p.busy || i < 0 || i >= len(p.view.Buttons) || p.onPress == nil {
		return
	}
	p.onPress(p.view.Buttons[i])
}

// activateFocused: Enter нажимает кнопку с фокусом или первую, главную.
func (p *panel) activateFocused() {
	i := p.focusedIndex()
	if i < 0 {
		i = 0
	}
	p.pressIndex(i)
}

func (p *panel) paint() {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(p.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(p.hwnd, uintptr(unsafe.Pointer(&ps)))

	w := p.width()
	roundBox(hdc, rect{0, 0, w, p.height}, uiSurface, uiLine, 1, 0)
	drawWrapped(hdc, p.fonts.title, uiText, p.view.Title, p.titleRc)
	if p.view.Bar != nil {
		fillSolid(hdc, p.barRc, uiLine)
		fill := p.barRc
		fill.right = fill.left + int32(float64(fill.right-fill.left)*p.view.Bar.Fraction)
		if fill.right > fill.left {
			c := p.view.Bar.Status.Color()
			fillSolid(hdc, fill, rgb(uint32(c.R), uint32(c.G), uint32(c.B)))
		}
	}
	if p.view.Note != "" {
		drawWrapped(hdc, p.fonts.body, uiRed, p.view.Note, p.noteRc)
	}
}

func (p *panel) drawButton(di *drawItemStruct) {
	i := int(di.ctlID) - idPanelButtonBase
	if i < 0 || i >= len(p.view.Buttons) {
		return
	}
	b := p.view.Buttons[i]
	rc := di.rcItem
	fillSolid(di.hdc, rc, uiSurface)
	pressed := di.itemState&odsSelected != 0

	text := uiSecondary
	switch {
	case b.Small:
		if pressed {
			text = uiText
		}
	case p.busy:
		roundBox(di.hdc, rect{rc.left + 1, rc.top + 1, rc.right - 1, rc.bottom - 1}, uiLine, uiLine, 1, scaled(6, p.dpi))
	case b.Main:
		fill := uiGraphite
		if pressed {
			fill = uiSecondary
		}
		roundBox(di.hdc, rect{rc.left + 1, rc.top + 1, rc.right - 1, rc.bottom - 1}, fill, fill, 1, scaled(6, p.dpi))
		text = uiWhite
	default:
		roundBox(di.hdc, rect{rc.left + 1, rc.top + 1, rc.right - 1, rc.bottom - 1}, uiSurface, uiLine, 1, scaled(6, p.dpi))
		text = uiText
	}
	old, _, _ := procSelectObject.Call(di.hdc, p.fonts.button)
	procSetBkMode.Call(di.hdc, transparent)
	procSetTextColor.Call(di.hdc, text)
	procDrawText.Call(di.hdc, uintptr(unsafe.Pointer(wstr(b.Label))), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	procSelectObject.Call(di.hdc, old)
	if di.itemState&odsFocus != 0 {
		focus := rect{rc.left + 4, rc.top + 4, rc.right - 4, rc.bottom - 4}
		procDrawFocusRect.Call(di.hdc, uintptr(unsafe.Pointer(&focus)))
	}
}

func (p *panel) close() {
	if p.hwnd != 0 {
		procDestroyWindow.Call(p.hwnd)
		p.hwnd = 0
	}
	p.fonts.free()
	thePanel = nil
}

func panelProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	p := thePanel
	if p != nil && p.hwnd == hwnd {
		switch uint32(msg) {
		case wmPaint:
			p.paint()
			return 0
		case wmActivate:
			if wParam&0xFFFF == waInactive {
				p.hide()
			}
			return 0
		case wmTimer:
			if p.visible && time.Since(p.shownAt) > panelMaxShown {
				p.hide()
			}
			return 0
		case wmDrawItem:
			p.drawButton(ptrTo[drawItemStruct](lParam))
			return 1
		case wmCommand:
			id, code := int(wParam&0xFFFF), int((wParam>>16)&0xFFFF)
			if code == bnClicked && id >= idPanelButtonBase && id < idPanelButtonBase+16 {
				// Нажатие обрабатывается следующим сообщением: пока кнопка внутри своего
				// щелчка, перестраивать окно и удалять её нельзя.
				procPostMessage.Call(hwnd, wmPanelPress, uintptr(id-idPanelButtonBase), 0)
				return 0
			}
		case wmPanelPress:
			p.pressIndex(int(wParam))
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
