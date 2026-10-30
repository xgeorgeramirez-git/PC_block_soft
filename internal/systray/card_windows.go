//go:build windows

package systray

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Окно карточки-табло (D-53, ТЗ 8.2.1 п.3-6). Окно со слоем прозрачности: рисунок карточки
// (card_render.go: сглаженные углы, переход цвета, тени) целиком кладётся на экран через
// UpdateLayeredWindow, цифры и подписи рисует шрифт системы прямо в тот же буфер. Окно ни
// разу не стирается и не перерисовывается кусками, поэтому секундная смена цифр не мерцает.
//
// Один код окна у двух мест (R-2 пакета 0.2.5): табло у значка (над значком, берёт фокус,
// Esc, стрелка) и плашка-табло предупреждения стража (WarningCard: правый нижний угол рабочей
// области, без активации и без клавиатуры, крестик, строка текста под плитками).

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

	wsExNoActivate  = 0x08000000 // WS_EX_NOACTIVATE: окно не становится активным, игра остаётся в фокусе
	wmMouseActivate = 0x0021
	maNoActivate    = 3
	swpNoActivate   = 0x0010
	swpShowWindow   = 0x0040
	cornerGap       = 16 // отступ плашки-табло от края рабочей области, px при 100 %
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
	opts        CardOptions
	noActivate  bool // плашка предупреждения: не берёт фокус, клавиатуры нет, не прячется по потере активации
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

// cards — окна карточек этого процесса по hwnd; трогает только поток окон.
var cards = map[uintptr]*card{}

func newCard(onPress func(CardTarget)) (*card, error) {
	return newCardWith(onPress, CardOptions{}, false)
}

func newCardWith(onPress func(CardTarget), o CardOptions, noActivate bool) (*card, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	c := &card{hInst: hInst, dpi: systemDPI(), opts: o, noActivate: noActivate, onPress: onPress, fonts: map[fontKey]uintptr{}}
	c.layout = NewCardLayoutFor(CardScale(c.dpi), o)
	cls, err := registerPlainClass("PCBlockTrayCard", cardProcPtr, hInst, 0)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(табло): %w", err)
	}
	ex := uintptr(wsExToolWin | wsExTopmost | wsExLayered)
	if noActivate {
		ex |= wsExNoActivate
	}
	hwnd, _, err := procCreateWindowEx.Call(
		ex,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(wstr("PC Block: сколько осталось времени"))),
		wsPopup,
		0, 0, uintptr(c.layout.W), uintptr(c.layout.H),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx(табло): %w", err)
	}
	c.hwnd = hwnd
	cards[hwnd] = c
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
	key := cardBaseKey(c.view, c.state)
	if key == c.baseKey && c.baseBGRA != nil {
		return c.baseBGRA
	}
	img := RenderCardBase(c.layout, c.view, c.state)
	c.baseBGRA = toBGRA(img, c.baseBGRA)
	c.baseKey = key
	return c.baseBGRA
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
	flags := textRunFlags(r)
	text := uintptr(unsafe.Pointer(wstr(r.Text)))
	if r.Wrap {
		// Несколько строк по центру высоты: сначала меряем, потом сдвигаем.
		calc := rc
		procDrawText.Call(c.memDC, text, ^uintptr(0), uintptr(unsafe.Pointer(&calc)), flags|dtCalcRect)
		rc.top, rc.bottom = centerRows(rc.top, rc.bottom, calc.bottom-calc.top)
	}
	procDrawText.Call(c.memDC, text, ^uintptr(0), uintptr(unsafe.Pointer(&rc)), flags)
}

// setView принимает новый вид; кадр уходит на экран, только если вид изменился.
func (c *card) setView(v CountdownView) {
	if v == c.view && c.baseBGRA != nil {
		return
	}
	c.view = v
	c.state.Focus = focusFor(v.Kind, c.state.Focus)
	if c.visible {
		c.render(nil)
	}
}

// show открывает карточку над значком (или у указателя, если оболочка не назвала место).
func (c *card) show(anchor Rect, haveAnchor bool) {
	if tooSoon(c.hiddenAt, time.Now(), cardReopenGuard) {
		return
	}
	if !haveAnchor {
		var pt point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		anchor = Rect{int(pt.x), int(pt.y), int(pt.x), int(pt.y)}
	}
	work := monitorWork(rect{int32(anchor.Left), int32(anchor.Top), int32(anchor.Right), int32(anchor.Bottom)}).toRect()
	x, y := cardOriginAt(c.layout, anchor, work)
	at := point{int32(x), int32(y)}
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
	if !canPress(c.busy, t, c.onPress != nil) {
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
	x, y := pointFromLParam(lParam)
	return c.layout.HitTest(x, y, c.view.Kind)
}

func (c *card) onKey(vk uintptr) bool {
	st, act := cardKey(vk, c.view.Kind, c.state, shiftDown)
	switch act {
	case keyHide:
		c.hide()
	case keyMove:
		c.setState(st)
	case keyPress:
		c.setState(st)
		procPostMessage.Call(c.hwnd, wmCardPress, uintptr(st.Focus), 0)
	default:
		return false
	}
	return true
}

// shiftDown: Shift нажат сейчас (Shift+Tab идёт назад).
func shiftDown() bool {
	s, _, _ := procGetKeyState.Call(vkShift)
	return int16(s) < 0
}

func (c *card) close() {
	if c.hwnd != 0 {
		delete(cards, c.hwnd)
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
}

func cardProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	c := cards[hwnd]
	if c != nil && c.hwnd == hwnd {
		switch uint32(msg) {
		case wmMouseActivate:
			if c.noActivate {
				return maNoActivate // щелчок по крестику не забирает фокус у игры
			}
		case wmActivate:
			if hidesOnDeactivate(c.noActivate, wParam) {
				c.hide()
			}
			return 0
		case wmTimer:
			if shownTooLong(c.visible, c.shownAt, time.Now(), cardMaxShown) {
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
			if cardHandCursor(c.state, c.busy) {
				hand, _, _ := procLoadCursor.Call(0, idcHand)
				procSetCursor.Call(hand)
				return 1
			}
		case wmLButtonDown:
			if st, capture := cardDown(c.state, c.hit(lParam)); capture {
				procSetCapture.Call(hwnd)
				c.setState(st)
			}
			return 0
		case wmLButtonUp:
			procReleaseCapture.Call()
			st, fire := cardUp(c.state, c.hit(lParam))
			c.setState(st)
			if fire != TargetNone {
				procPostMessage.Call(hwnd, wmCardPress, uintptr(fire), 0)
			}
			return 0
		case wmKeyDown:
			if !c.noActivate && c.onKey(wParam) {
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

// WarningCard — плашка-табло предупреждения стража за 30, 15 и 5 минут (FR-9, D-55): та же
// карточка, что табло значка (рисунок, раскладка, окно), но в правом нижнем углу рабочей
// области монитора, где сейчас окно на переднем плане (игра на весь экран), поверх всех окон и
// без перехвата фокуса; крестик вместо стрелки и строка текста под плитками. Что показать и
// когда закрыть, решает модель стража; окно только рисует и сообщает о крестике. Работает в
// потоке окон.
type WarningCard struct{ c *card }

// NewWarningCard создаёт скрытое окно плашки; onClose — нажат крестик.
func NewWarningCard(onClose func()) (*WarningCard, error) {
	c, err := newCardWith(func(t CardTarget) {
		if t == TargetClose && onClose != nil {
			onClose()
		}
	}, CardOptions{Button: CardButtonClose, Line: true}, true)
	if err != nil {
		return nil, err
	}
	return &WarningCard{c: c}, nil
}

// Show показывает плашку с видом v или обновляет цифры. Первый показ ставит окно в угол
// рабочей области монитора окна на переднем плане; каждый вызов подтверждает «поверх всех» без
// активации (игра на весь экран могла поднять себя выше).
func (w *WarningCard) Show(v CountdownView) {
	c := w.c
	if c.hwnd == 0 {
		return
	}
	if c.visible && v == c.view && c.baseBGRA != nil {
		// Вид тот же: слой не перерисовывается, только «поверх всех» без активации (игра на весь
		// экран могла подняться выше).
		procSetWindowPos.Call(c.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		return
	}
	c.view = v
	if !c.visible {
		fg, _, _ := procGetForegroundWindow.Call()
		var wr rect
		if fg == 0 || func() bool { r, _, _ := procGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&wr))); return r == 0 }() {
			wr = rect{0, 0, 1, 1}
		}
		work := monitorWork(wr).toRect()
		x, y := cardOriginCorner(c.layout, work, int(scaled(cornerGap, c.dpi)))
		at := point{int32(x), int32(y)}
		c.state = CardState{}
		c.render(&at)
		c.visible, c.shownAt = true, time.Now()
	} else {
		c.render(nil)
	}
	procSetWindowPos.Call(c.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
}

// Hide прячет плашку.
func (w *WarningCard) Hide() {
	c := w.c
	if !c.visible {
		return
	}
	c.visible = false
	procShowWindow.Call(c.hwnd, swHide)
}

// Visible: плашка на экране.
func (w *WarningCard) Visible() bool { return w.c.visible }

// HWND — окно плашки (тестовые помощники приёмки).
func (w *WarningCard) HWND() uintptr { return w.c.hwnd }

// ClickClose нажимает крестик настоящими сообщениями мыши окну (TestGuard, Ruling 5): тот же
// путь, что у щелчка человека, без активации окна.
func (w *WarningCard) ClickClose() {
	c := w.c
	if !c.visible {
		return
	}
	x, y := c.layout.CloseCenter()
	lp := lParamFromPoint(x, y)
	procSendMessage.Call(c.hwnd, wmLButtonDown, 1, lp)
	procSendMessage.Call(c.hwnd, wmLButtonUp, 0, lp)
}

// Close уничтожает окно.
func (w *WarningCard) Close() { w.c.close() }
