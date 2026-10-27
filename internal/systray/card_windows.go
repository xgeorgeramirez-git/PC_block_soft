//go:build windows

package systray

import (
	"fmt"
	"image"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Окно карточки-табло (D-53, ТЗ 8.2.1 п.3-6). Окно со слоем прозрачности: рисунок карточки
// (card_render.go: сглаженные углы, переход цвета, тени) целиком кладётся на экран через
// UpdateLayeredWindow, цифры и подписи рисует шрифт системы прямо в тот же буфер. Окно ни
// разу не стирается и не перерисовывается кусками, поэтому секундная смена цифр не мерцает.

const (
	wmCardPress = wmApp + 21 // нажатие обрабатывается следующим сообщением, не внутри щелчка

	cardTimerID    = 12
	cardTimerEvery = 10000 // мс: проверка предельного срока показа
	// cardMaxShown: забытая открытой карточка закрывается сама (потеря активации обычно
	// закрывает её раньше).
	cardMaxShown = 5 * time.Minute
	// cardReopenGuard: щелчок по значку, закрывшему карточку потерей активации, не открывает
	// её заново: для человека это одно нажатие.
	cardReopenGuard = 400 * time.Millisecond

	idcHand    = 32649
	tmeLeave   = 0x00000002
	ulwAlpha   = 0x00000002
	acSrcOver  = 0
	acSrcAlpha = 1
	biRGB      = 0
	dibRGB     = 0

	fwBold   = 700
	fwNormal = 400
	// ANTIALIASED_QUALITY: ClearType на слое с прозрачностью даёт цветную кайму.
	antialiasedQuality = 4

	fontBahnschrift = "Bahnschrift"
	fontFallback    = "Segoe UI"
)

var cardProcPtr uintptr

func init() { cardProcPtr = windows.NewCallback(cardProc) }

type bitmapInfoHeader struct {
	size          uint32
	width, height int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

type blendFunction struct{ op, flags, alpha, format byte }

type trackMouse struct {
	size    uint32
	flags   uint32
	hwnd    uintptr
	hoverMs uint32
}

type sizeT struct{ cx, cy int32 }

type fontKey struct {
	px   int
	bold bool
}

// card — окно табло. Что показать, решает CountdownFor, что где лежит — NewCardLayout;
// окно кладёт рисунок на экран и принимает мышь и клавиши. Работает только в потоке окон.
type card struct {
	hwnd, hInst uintptr
	dpi         int
	layout      CardLayout
	onPress     func(CardTarget)
	onHide      func() // карточка закрылась (щелчком мимо, Esc, потерей активации)

	view     CountdownView
	state    CardState
	busy     bool
	visible  bool
	shownAt  time.Time
	hiddenAt time.Time
	tracking bool

	baseKey  string
	baseBGRA []byte // рисунок без текста в порядке байтов Windows (синий, зелёный, красный, прозрачность)

	memDC, dib, oldBmp uintptr
	bits               []byte
	face               string
	fonts              map[fontKey]uintptr
}

var theCard *card

func newCard(onPress func(CardTarget)) (*card, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	c := &card{hInst: hInst, dpi: systemDPI(), onPress: onPress, fonts: map[fontKey]uintptr{}}
	c.layout = NewCardLayout(CardScale(c.dpi))
	cls, err := registerPlainClass("PCBlockTrayCard", cardProcPtr, hInst, 0)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(табло): %w", err)
	}
	theCard = c
	hwnd, _, err := procCreateWindowEx.Call(
		wsExToolWin|wsExTopmost|wsExLayered,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(wstr("PC Block: сколько осталось времени"))),
		wsPopup,
		0, 0, uintptr(c.layout.W), uintptr(c.layout.H),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		theCard = nil
		return nil, fmt.Errorf("CreateWindowEx(табло): %w", err)
	}
	c.hwnd = hwnd
	return c, nil
}

// pickFace выбирает шрифт: Bahnschrift, если он есть в системе, иначе Segoe UI. CreateFont
// молча подставляет другой шрифт, поэтому имя проверяется по выбранному в контекст шрифту.
func (c *card) pickFace(hdc uintptr) {
	if c.face != "" {
		return
	}
	c.face = fontFallback
	f := c.rawFont(fontBahnschrift, 20, fwNormal)
	if f == 0 {
		return
	}
	old, _, _ := procSelectObject.Call(hdc, f)
	buf := make([]uint16, 64)
	n, _, _ := procGetTextFace.Call(hdc, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	procSelectObject.Call(hdc, old)
	procDeleteObject.Call(f)
	if n > 0 && strings.HasPrefix(windows.UTF16ToString(buf), fontBahnschrift) {
		c.face = fontBahnschrift
	}
}

func (c *card) rawFont(face string, px, weight int) uintptr {
	f, _, _ := procCreateFont.Call(
		uintptr(-int32(px)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		defaultCharset, 0, 0, antialiasedQuality, 0,
		uintptr(unsafe.Pointer(wstr(face))),
	)
	return f
}

func (c *card) font(px int, bold bool) uintptr {
	k := fontKey{px, bold}
	if f, ok := c.fonts[k]; ok {
		return f
	}
	w := fwNormal
	if bold {
		w = fwBold
	}
	f := c.rawFont(c.face, px, w)
	c.fonts[k] = f
	return f
}

// ensureSurface создаёт буфер рисунка нужного размера (32 бита на пиксель, строки сверху вниз).
func (c *card) ensureSurface() bool {
	if c.dib != 0 {
		return true
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return false
	}
	bi := bitmapInfoHeader{
		width: int32(c.layout.W), height: -int32(c.layout.H),
		planes: 1, bitCount: 32, compression: biRGB,
	}
	bi.size = uint32(unsafe.Sizeof(bi))
	var ptr uintptr
	dib, _, _ := procCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), dibRGB, uintptr(unsafe.Pointer(&ptr)), 0, 0)
	if dib == 0 || ptr == 0 {
		procDeleteDC.Call(mem)
		return false
	}
	old, _, _ := procSelectObject.Call(mem, dib)
	c.memDC, c.dib, c.oldBmp = mem, dib, old
	c.bits = unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&ptr))), c.layout.W*c.layout.H*4)
	c.pickFace(mem)
	return true
}

// baseFor возвращает рисунок без текста для текущего вида и подсветки; пересчитывается только
// при смене вида карточки, наведения или фокуса, а не каждую секунду.
func (c *card) baseFor() []byte {
	key := fmt.Sprintf("%d|%v", c.view.Kind, c.state)
	if key == c.baseKey && c.baseBGRA != nil {
		return c.baseBGRA
	}
	img := RenderCardBase(c.layout, c.view, c.state)
	c.baseBGRA = toBGRA(img, c.baseBGRA)
	c.baseKey = key
	return c.baseBGRA
}

// toBGRA переставляет красный и синий байты: DIB Windows хранит пиксель как B, G, R, A.
func toBGRA(img *image.RGBA, dst []byte) []byte {
	if len(dst) != len(img.Pix) {
		dst = make([]byte, len(img.Pix))
	}
	for i := 0; i+3 < len(img.Pix); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
	return dst
}

// render собирает кадр: рисунок, поверх него текст, затем прозрачность возвращается из
// рисунка (шрифт системы прозрачность не ведёт) и кадр уходит на экран. at != nil: новое
// положение окна.
func (c *card) render(at *point) {
	if c.hwnd == 0 || !c.ensureSurface() {
		return
	}
	base := c.baseFor()
	copy(c.bits, base)
	for _, r := range CardTexts(c.layout, c.view) {
		c.drawRun(r)
	}
	for i := 3; i < len(c.bits); i += 4 {
		c.bits[i] = base[i]
	}
	size := sizeT{int32(c.layout.W), int32(c.layout.H)}
	src := point{}
	blend := blendFunction{op: acSrcOver, alpha: 255, format: acSrcAlpha}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	var dst uintptr
	if at != nil {
		dst = uintptr(unsafe.Pointer(at))
	}
	procUpdateLayeredWindow.Call(c.hwnd, screen, dst, uintptr(unsafe.Pointer(&size)),
		c.memDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

func (c *card) drawRun(r TextRun) {
	old, _, _ := procSelectObject.Call(c.memDC, c.font(r.Px, r.Bold))
	defer procSelectObject.Call(c.memDC, old)
	procSetBkMode.Call(c.memDC, transparent)
	procSetTextColor.Call(c.memDC, rgb(uint32(r.Color.R), uint32(r.Color.G), uint32(r.Color.B)))
	rc := rect{int32(r.Rect.Left), int32(r.Rect.Top), int32(r.Rect.Right), int32(r.Rect.Bottom)}
	flags := uintptr(dtNoPrefix)
	if r.Align == AlignCenter {
		flags |= dtCenter
	}
	text := uintptr(unsafe.Pointer(wstr(r.Text)))
	if r.Wrap {
		// Несколько строк по центру высоты: сначала меряем, потом сдвигаем.
		calc := rc
		procDrawText.Call(c.memDC, text, ^uintptr(0), uintptr(unsafe.Pointer(&calc)), flags|dtWordBreak|dtCalcRect)
		if h := calc.bottom - calc.top; h < rc.bottom-rc.top {
			shift := (rc.bottom - rc.top - h) / 2
			rc.top += shift
			rc.bottom = rc.top + h
		}
		procDrawText.Call(c.memDC, text, ^uintptr(0), uintptr(unsafe.Pointer(&rc)), flags|dtWordBreak)
		return
	}
	procDrawText.Call(c.memDC, text, ^uintptr(0), uintptr(unsafe.Pointer(&rc)), flags|dtSingleLine|dtVCenter)
}

// setView принимает новый вид; кадр уходит на экран, только если вид изменился.
func (c *card) setView(v CountdownView) {
	if v == c.view && c.baseBGRA != nil {
		return
	}
	c.view = v
	if order := FocusOrder(v.Kind); !contains(order, c.state.Focus) {
		c.state.Focus = order[0]
	}
	if c.visible {
		c.render(nil)
	}
}

func contains(order []CardTarget, t CardTarget) bool {
	for _, o := range order {
		if o == t {
			return true
		}
	}
	return false
}

// show открывает карточку над значком (или у указателя, если оболочка не назвала место).
func (c *card) show(anchor Rect, haveAnchor bool) {
	if time.Since(c.hiddenAt) < cardReopenGuard {
		return
	}
	if !haveAnchor {
		var pt point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		anchor = Rect{int(pt.x), int(pt.y), int(pt.x), int(pt.y)}
	}
	work := monitorWork(rect{int32(anchor.Left), int32(anchor.Top), int32(anchor.Right), int32(anchor.Bottom)}).toRect()
	x, y := PlacePanel(anchor, work, c.layout.Card.Dx(), c.layout.Card.Dy())
	at := point{int32(x - c.layout.Margin), int32(y - c.layout.Margin)}
	c.state = CardState{Focus: FocusOrder(c.view.Kind)[0]}
	c.busy = false
	c.render(&at)
	procSetWindowPos.Call(c.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|0x0040) // SWP_SHOWWINDOW
	procSetForegroundWindow.Call(c.hwnd)
	procSetFocus.Call(c.hwnd)
	c.visible, c.shownAt = true, time.Now()
	procSetTimer.Call(c.hwnd, cardTimerID, cardTimerEvery, 0)
}

func (c *card) hide() {
	if !c.visible {
		return
	}
	c.visible, c.hiddenAt = false, time.Now()
	procKillTimer.Call(c.hwnd, cardTimerID)
	procShowWindow.Call(c.hwnd, swHide)
	if c.onHide != nil {
		c.onHide()
	}
}

func (c *card) setBusy(b bool) { c.busy = b }

// press выполняет нажатие кнопки карточки.
func (c *card) press(t CardTarget) {
	if c.busy || t == TargetNone || c.onPress == nil {
		return
	}
	c.onPress(t)
}

func (c *card) setState(s CardState) {
	if s == c.state {
		return
	}
	c.state = s
	c.render(nil)
}

func (c *card) hit(lParam uintptr) CardTarget {
	x, y := int(int16(lParam&0xFFFF)), int(int16((lParam>>16)&0xFFFF))
	return c.layout.HitTest(x, y, c.view.Kind)
}

func (c *card) onKey(vk uintptr) bool {
	order := FocusOrder(c.view.Kind)
	st := c.state
	st.FocusVisible = true
	switch vk {
	case vkEscape:
		c.hide()
		return true
	case vkTab, vkRight, vkDown, vkLeft, vkUp:
		back := vk == vkLeft || vk == vkUp
		if vk == vkTab {
			s, _, _ := procGetKeyState.Call(vkShift)
			back = int16(s) < 0
		}
		st.Focus = NextFocus(order, st.Focus, back)
		c.setState(st)
		return true
	case vkReturn, vkSpace:
		if !contains(order, st.Focus) {
			st.Focus = order[0]
		}
		c.setState(st)
		procPostMessage.Call(c.hwnd, wmCardPress, uintptr(st.Focus), 0)
		return true
	}
	return false
}

func (c *card) close() {
	if c.hwnd != 0 {
		procDestroyWindow.Call(c.hwnd)
		c.hwnd = 0
	}
	for _, f := range c.fonts {
		if f != 0 {
			procDeleteObject.Call(f)
		}
	}
	if c.memDC != 0 {
		procSelectObject.Call(c.memDC, c.oldBmp)
		procDeleteObject.Call(c.dib)
		procDeleteDC.Call(c.memDC)
		c.memDC, c.dib, c.bits = 0, 0, nil
	}
	theCard = nil
}

func cardProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	c := theCard
	if c != nil && c.hwnd == hwnd {
		switch uint32(msg) {
		case wmActivate:
			if wParam&0xFFFF == waInactive {
				c.hide()
			}
			return 0
		case wmTimer:
			if c.visible && time.Since(c.shownAt) > cardMaxShown {
				c.hide()
			}
			return 0
		case wmMouseMove:
			if !c.tracking {
				tme := trackMouse{flags: tmeLeave, hwnd: hwnd}
				tme.size = uint32(unsafe.Sizeof(tme))
				procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				c.tracking = true
			}
			st := c.state
			st.Hover = c.hit(lParam)
			c.setState(st)
			return 0
		case wmMouseLeave:
			c.tracking = false
			st := c.state
			st.Hover = TargetNone
			c.setState(st)
			return 0
		case wmSetCursor:
			if c.state.Hover != TargetNone && !c.busy {
				hand, _, _ := procLoadCursor.Call(0, idcHand)
				procSetCursor.Call(hand)
				return 1
			}
		case wmLButtonDown:
			if t := c.hit(lParam); t != TargetNone {
				procSetCapture.Call(hwnd)
				st := c.state
				st.Pressed = t
				c.setState(st)
			}
			return 0
		case wmLButtonUp:
			procReleaseCapture.Call()
			pressed := c.state.Pressed
			st := c.state
			st.Pressed = TargetNone
			c.setState(st)
			if pressed != TargetNone && c.hit(lParam) == pressed {
				procPostMessage.Call(hwnd, wmCardPress, uintptr(pressed), 0)
			}
			return 0
		case wmKeyDown:
			if c.onKey(wParam) {
				return 0
			}
		case wmCardPress:
			c.press(CardTarget(wParam))
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
