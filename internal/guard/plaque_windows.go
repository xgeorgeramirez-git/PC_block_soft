//go:build windows

package guard

import (
	"fmt"
	"math"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Окно плашки «Игрового лобби» про программы и сайты (ТЗ 8.2.4, 8.2.5, D-111; макет plaques.py): в
// правом нижнем углу рабочей области поверх всех окон, без фокуса (WS_EX_NOACTIVATE, MA_NOACTIVATE:
// игра остаётся в фокусе и на весь экран). Окно со слоем прозрачности, как «Сообщение от родителя»
// (note_windows.go): основу (тень, панель, кнопку, крестик) рисует чистый Go прямо в DIB окна
// (RenderPlaqueInto), текст — шрифт системы в тот же DIB (заголовок жирностью 900 с обводкой
// #0B1D5C и тенью вниз: шрифт с сглаживанием, выведенный кольцом сдвигов, — путь GDI не сглаживается),
// непрозрачность пикселям текста возвращает PlaqueFixTextAlpha; кадр уходит на экран
// UpdateLayeredWindow. Вне панели и тени пиксели прозрачны: щелчки проходят к окнам под плашкой.
// Кадр стикера (30 в секунду по кругу, пока окно видно и анимации Windows включены; выключены —
// первый кадр, тикер не запускается, PL-89) кладётся поверх заплатки основы только в прямоугольник
// стикера и выводится UpdateLayeredWindowIndirect с prcDirty. Спрайт — номерами палитры (NoteSprite),
// разбирается в своей горутине; при скрытии окна спрайт, кадр и буферы освобождаются (NFR-5). Что
// показать и когда убрать, решает Model; окно рисует и сообщает нажатия.

const (
	wmPlaquePress  = wmApp + 33 // нажатие кнопки плашки разбирается не внутри сообщения мыши
	wmPlaqueSprite = wmApp + 34 // спрайт стикера плашки разобран в своей горутине
)

var (
	thePlaque     *plaqueWindow
	plaqueProcPtr uintptr
)

func init() { plaqueProcPtr = windows.NewCallback(plaqueProc) }

type plaqueWindow struct {
	hwnd, hInst uintptr
	dpi         int
	fonts       map[[2]int]uintptr // шрифт по размеру при 96 dpi и жирности
	onPress     func(Button)
	// onShown — что окно вывело: стикер ("" — без) и кадр (Controller.SetPlaqueShown, тестовый режим).
	onShown func(sticker string, frame int)

	key      string
	view     WarningView
	style    PlaqueStyle
	ask      ButtonView // «Попросить сейчас»; Button == ButtonNone — кнопки нет
	hasClose bool       // есть крестик
	sticker  string     // стикер, который окно рисует ("" — спрайт не разобрался)
	visible  bool
	geom     rect
	lay      PlaqueLayout

	hover, down Button // под мышью и нажатая: ButtonAskNow, ButtonPlateClose или ButtonNone
	tracking    bool

	memDC, dib, oldBmp uintptr
	bits               []byte
	dibW, dibH         int32
	patch              []byte // основа под стикером (PlaquePatch)

	sprite  *NoteSprite
	pending string
	failed  string
	cur     int
	anim    *stickerAnim
	inbox   noteSpriteInbox
}

func newPlaqueWindow(onPress func(Button)) (*plaqueWindow, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	if onPress == nil {
		onPress = func(Button) {}
	}
	p := &plaqueWindow{hInst: hInst, dpi: systemDPI(), onPress: onPress, fonts: map[[2]int]uintptr{}, cur: -1,
		hover: ButtonNone, down: ButtonNone, onShown: func(string, int) {}}
	thePlaque = p
	cls, err := registerPlainClass("PCBlockGuardPlaque", plaqueProcPtr, hInst, 0)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(плашка): %w", err)
	}
	hwnd, _, err := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWin|wsExNoActivate|wsExLayered,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(utf16("PC Block"))),
		wsPopup,
		0, 0, 10, 10,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx(плашка): %w", err)
	}
	p.hwnd = hwnd
	return p, nil
}

// font — шрифт размера px (при 96 dpi) и жирности weight, сглаженный без ClearType (на слое с
// прозрачностью ClearType даёт цветную кайму); создаётся один раз.
func (p *plaqueWindow) font(px, weight int) uintptr {
	k := [2]int{px, weight}
	if f, ok := p.fonts[k]; ok {
		return f
	}
	f, _, _ := procCreateFont.Call(
		uintptr(-Scaled(px, p.dpi)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		defaultCharset, 0, 0, antialiasedQuality, 0,
		uintptr(unsafe.Pointer(utf16("Segoe UI"))),
	)
	p.fonts[k] = f
	return f
}

// plaqueKey — подпись плашки для сравнения кадров: номер, заголовок, текст, стикер и кнопки.
func plaqueKey(w *WarningView, sticker string) string {
	k := fmt.Sprintf("%d\x00%s\x00%s\x00%s", w.ID, w.Title, w.Text, sticker)
	for _, b := range w.Buttons {
		k += fmt.Sprintf("\x00%d:%s:%t", b.Button, b.Label, b.Enabled)
	}
	return k
}

// apply показывает плашку по View.Warning вида «Лобби» или прячет её (nil).
func (p *plaqueWindow) apply(w *WarningView) {
	if w == nil {
		p.hide()
		return
	}
	p.view = *w
	style, _ := PlaqueStyleOf(w.Kind, w.Allowed)
	sticker := w.Sticker
	if sticker == p.failed {
		sticker = ""
	}
	p.wantSprite(sticker)
	key := plaqueKey(w, sticker)
	if key == p.key && p.visible {
		p.step() // анимации Windows могли выключиться: стикер встаёт на первый кадр
		p.syncAnim()
		p.raise()
		return
	}
	p.key, p.style, p.sticker = key, style, sticker
	p.ask, p.hasClose = ButtonView{}, false
	for _, b := range w.Buttons {
		switch b.Button {
		case ButtonAskNow:
			p.ask = b
		case ButtonPlateClose:
			p.hasClose = true
		}
	}
	// Заголовок окна — заголовок и текст плашки: его читает экранный диктор (доступность).
	setWindowText(p.hwnd, w.Title+": "+w.Text)
	p.hover, p.down = ButtonNone, ButtonNone
	p.layout()
	if !p.paintAll(true) {
		p.key = "" // буфер рисунка не создан: следующий кадр стража попробует снова
		return
	}
	procSetWindowPos.Call(p.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	p.syncAnim()
}

// raise держит плашку над полноэкранной игрой без активации.
func (p *plaqueWindow) raise() {
	if p.visible {
		procSetWindowPos.Call(p.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	}
}

// hide прячет плашку и освобождает спрайт, кадр и буферы (NFR-5).
func (p *plaqueWindow) hide() {
	if !p.visible {
		return
	}
	p.visible, p.key = false, ""
	p.hover, p.down = ButtonNone, ButtonNone
	p.stopAnim()
	procShowWindow.Call(p.hwnd, swHide)
	p.freeSurface()
	p.sprite, p.cur = nil, -1
	p.onShown("", 0)
}

// layout раскладывает плашку (FitPlaque) тем шрифтом, которым она рисуется, в правом нижнем углу.
func (p *plaqueWindow) layout() {
	wa := workArea()
	hdc, _, _ := procGetDC.Call(p.hwnd)
	measure := func(px, weight int, w int32, s string) (int32, int32) {
		return measureSize(hdc, p.font(px, weight), s, w)
	}
	label := ""
	if p.ask.Button != ButtonNone {
		label = p.ask.Label
	}
	l, place := FitPlaque(p.dpi, Rect{Left: wa.left, Top: wa.top, Right: wa.right, Bottom: wa.bottom}, p.view.Title, p.view.Text, label, measure)
	procReleaseDC.Call(p.hwnd, hdc)
	p.lay, p.geom, p.visible = l, toRect(place), true
}

// ensureSurface держит DIB секцию 32 бита размера окна (строки сверху вниз) в своём DC.
func (p *plaqueWindow) ensureSurface(w, h int32) bool {
	if p.dib != 0 && p.dibW == w && p.dibH == h {
		return true
	}
	p.freeSurface()
	if w <= 0 || h <= 0 {
		return false
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return false
	}
	bi := dibHeader{width: w, height: -h, planes: 1, bitCount: 32}
	bi.size = uint32(unsafe.Sizeof(bi))
	var ptr uintptr
	dib, _, _ := procCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), dibRGB, uintptr(unsafe.Pointer(&ptr)), 0, 0)
	if dib == 0 || ptr == 0 {
		procDeleteDC.Call(mem)
		return false
	}
	old, _, _ := procSelectObject.Call(mem, dib)
	p.memDC, p.dib, p.oldBmp, p.dibW, p.dibH = mem, dib, old, w, h
	p.bits = unsafe.Slice(ptrTo[byte](ptr), int(w)*int(h)*4)
	return true
}

// freeSurface освобождает DIB секцию, её DC и заплатку основы.
func (p *plaqueWindow) freeSurface() {
	if p.memDC != 0 {
		procSelectObject.Call(p.memDC, p.oldBmp)
		procDeleteObject.Call(p.dib)
		procDeleteDC.Call(p.memDC)
	}
	p.memDC, p.dib, p.oldBmp, p.bits, p.dibW, p.dibH = 0, 0, 0, nil, 0, 0
	p.patch = nil
}

// buildBase рисует основу прямо в DIB, поверх — текст шрифтом системы, возвращает непрозрачность
// пикселям текста и запоминает основу под стикером.
func (p *plaqueWindow) buildBase() bool {
	l := p.lay
	if !p.ensureSurface(l.W, l.H) {
		return false
	}
	if !RenderPlaqueInto(p.bits, l, p.hover == ButtonAskNow || p.down == ButtonAskNow, p.hover == ButtonPlateClose || p.down == ButtonPlateClose) {
		return false
	}
	p.drawTexts()
	procGdiFlush.Call() // текст лёг в пиксели до чтения их из Go
	PlaqueFixTextAlpha(p.bits, l, p.dpi)
	p.patch = PlaquePatch(p.bits, l)
	return p.patch != nil
}

// drawTexts пишет заголовок (цвет вида, обводка и тень #0B1D5C), текст (#DCE4FF) и подпись кнопки
// (#0B1D5C по центру).
func (p *plaqueWindow) drawTexts() {
	l, hdc := p.lay, p.memDC
	procSetBkMode.Call(hdc, transparent)
	ink := noteColorRef(PlaqueInkRGB)
	if p.view.Title != "" {
		f := p.font(l.TitlePx, 900)
		// Обводка 3 px (видна снаружи 1,5, как -webkit-text-stroke с paint-order) и тень 2 px вниз —
		// кольцом сдвигов того же сглаженного текста; сверху — текст цветом вида.
		ring := math.Max(1, 1.5*float64(p.dpi)/96)
		drop := Scaled(2, p.dpi)
		for _, dy := range []int32{drop, 0} {
			for i := 0; i < 12; i++ {
				a := float64(i) * math.Pi / 6
				dx, ddy := int32(math.Round(ring*math.Cos(a))), int32(math.Round(ring*math.Sin(a)))
				r := toRect(l.Title)
				r.left, r.right, r.top, r.bottom = r.left+dx, r.right+dx, r.top+ddy+dy, r.bottom+ddy+dy
				drawWrapped(hdc, f, ink, p.view.Title, r, 0)
			}
		}
		drawWrapped(hdc, f, noteColorRef(p.style.Accent), p.view.Title, toRect(l.Title), 0)
	}
	drawWrapped(hdc, p.font(l.TextPx, 600), noteColorRef(PlaqueTextRGB), p.view.Text, toRect(l.Text), 0)
	if p.ask.Button != ButtonNone && l.Btn != (Rect{}) {
		old, _, _ := procSelectObject.Call(hdc, p.font(l.BtnPx, 900))
		procSetTextColor.Call(hdc, ink)
		rc := toRect(l.Btn)
		rc.bottom -= l.BtnStrip / 2 // подпись над нижней полосой, как на макете
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(utf16(p.ask.Label))), ^uintptr(0),
			uintptr(unsafe.Pointer(&rc)), dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
		procSelectObject.Call(hdc, old)
	}
}

// putFrame кладёт кадр k стикера поверх заплатки основы; false — стикера нет (спрайт не разобран).
func (p *plaqueWindow) putFrame(k int) bool {
	s, l := p.sprite, p.lay
	side := int(l.Sticker.Right - l.Sticker.Left)
	if s == nil || p.sticker == "" || s.Sticker != p.sticker || side <= 0 || p.patch == nil {
		return false
	}
	if !PlaqueStickerOver(p.bits, p.patch, l, s.FrameBGRA(k, side), side) {
		return false
	}
	p.cur = k
	p.onShown(p.sticker, k)
	return true
}

// paintAll собирает плашку целиком и выводит её; move — окно встаёт на место раскладки.
func (p *plaqueWindow) paintAll(move bool) bool {
	if !p.buildBase() {
		return false
	}
	p.cur = -1
	if !p.putFrame(PlaqueStickerFrame(&p.view, time.Now())) {
		p.onShown("", 0)
	}
	var at *layerPoint
	if move {
		at = &layerPoint{p.geom.left, p.geom.top}
	}
	p.update(at)
	return true
}

// update выводит DIB окна целиком (at != nil — и ставит окно в эту точку экрана).
func (p *plaqueWindow) update(at *layerPoint) {
	size := layerSize{p.dibW, p.dibH}
	src := layerPoint{}
	blend := layerBlend{op: acSrcOver, alpha: 255, format: acSrcAlpha}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	procUpdateLayeredWindow.Call(p.hwnd, screen, uintptr(unsafe.Pointer(at)), uintptr(unsafe.Pointer(&size)),
		p.memDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

// updateSticker выводит только прямоугольник стикера (prcDirty); не вышло — окно целиком.
func (p *plaqueWindow) updateSticker() {
	s := p.lay.Sticker
	dirty := rect{max(s.Left, 0), max(s.Top, 0), min(s.Right, p.dibW), min(s.Bottom, p.dibH)}
	size := layerSize{p.dibW, p.dibH}
	src := layerPoint{}
	blend := layerBlend{op: acSrcOver, alpha: 255, format: acSrcAlpha}
	info := layerInfo{hdcSrc: p.memDC, psize: &size, pptSrc: &src, pblend: &blend, flags: ulwAlpha, dirty: &dirty}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, _ := procUpdateLayeredWindowIndirect.Call(p.hwnd, uintptr(unsafe.Pointer(&info))); r == 0 {
		p.update(nil)
	}
}

// step выводит кадр стикера на сейчас, если он сменился.
func (p *plaqueWindow) step() {
	if !p.visible || p.sprite == nil || p.memDC == 0 {
		return
	}
	k := PlaqueStickerFrame(&p.view, time.Now())
	if k == p.cur {
		return
	}
	if p.putFrame(k) {
		p.updateSticker()
	}
}

// repaint пересобирает основу (подсветка кнопки или крестика сменилась) и выводит окно на месте.
func (p *plaqueWindow) repaint() {
	if p.visible {
		p.paintAll(false)
	}
}

// syncAnim держит шаг кадров, только пока плашка видна, спрайт есть и анимации Windows включены.
func (p *plaqueWindow) syncAnim() {
	want := p.visible && p.sticker != "" && p.sprite != nil && !p.view.Still && p.view.Frames > 1
	if want == (p.anim != nil) {
		return
	}
	if want {
		p.anim = startStickerAnim(p.hwnd)
	} else {
		p.stopAnim()
	}
}

func (p *plaqueWindow) stopAnim() {
	if p.anim != nil {
		p.anim.close()
		p.anim = nil
	}
}

// wantSprite готовит спрайт стикера в своей горутине (поток окон с перехватом клавиш не ждёт);
// готовность приходит окну сообщением wmPlaqueSprite. "" — спрайт не нужен и отпускается.
func (p *plaqueWindow) wantSprite(sticker string) {
	if sticker == "" {
		p.sprite = nil
		return
	}
	if (p.sprite != nil && p.sprite.Sticker == sticker) || p.pending == sticker {
		return
	}
	p.pending = sticker
	hwnd := p.hwnd
	go safely(func() {
		s, err := LoadNoteSprite(sticker)
		if err != nil {
			s = nil
		}
		p.inbox.put(sticker, s)
		procPostMessage.Call(hwnd, wmPlaqueSprite, 0, 0)
	})
}

// spriteReady забирает разобранные спрайты: свой — кадр на экран и шаг кадров; чужие выбрасываются;
// разбор не удался — плашка без стикера (место под ним остаётся панелью).
func (p *plaqueWindow) spriteReady() {
	want := ""
	if p.visible {
		want = p.sticker
	}
	sprite, found, pending := p.inbox.take(want, p.pending)
	p.pending = pending
	if !found {
		return
	}
	if sprite == nil {
		p.failed = want
		p.key = ""
		v := p.view
		p.apply(&v)
		return
	}
	p.sprite = sprite
	p.cur = -1
	if p.putFrame(PlaqueStickerFrame(&p.view, time.Now())) {
		p.updateSticker()
	}
	p.syncAnim()
}

// hit — кнопка под точкой lParam (клиентские координаты): ButtonAskNow, ButtonPlateClose или
// ButtonNone (прозрачные места, панель, недоступная кнопка).
func (p *plaqueWindow) hit(lParam uintptr) Button {
	x, y := int32(int16(lParam&0xFFFF)), int32(int16(lParam>>16&0xFFFF))
	switch b := p.lay.Hit(x, y); {
	case b == ButtonPlateClose && p.hasClose:
		return b
	case b == ButtonAskNow && p.ask.Button == ButtonAskNow && p.ask.Enabled:
		return b
	}
	return ButtonNone
}

// click нажимает кнопку b мышью тем же путём, что человек (тестовый режим, TestGuard click).
func (p *plaqueWindow) click(b Button) bool {
	if !p.visible {
		return false
	}
	var r Rect
	switch {
	case b == ButtonPlateClose && p.hasClose:
		r = p.lay.Close
	case b == ButtonAskNow && p.ask.Button == ButtonAskNow:
		r = p.lay.Btn
	default:
		return false
	}
	x, y := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	lp := uintptr(uint32(y)<<16 | uint32(x)&0xFFFF)
	procSendMessage.Call(p.hwnd, wmLButtonDown, 1, lp)
	procSendMessage.Call(p.hwnd, wmLButtonUp, 0, lp)
	return true
}

func (p *plaqueWindow) close() {
	p.stopAnim()
	if p.hwnd != 0 {
		procDestroyWindow.Call(p.hwnd)
		p.hwnd = 0
	}
	p.visible = false
	p.freeSurface()
	p.sprite = nil
	for _, f := range p.fonts {
		procDeleteObject.Call(f)
	}
	p.fonts = map[[2]int]uintptr{}
	thePlaque = nil
}

func plaqueProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if p := thePlaque; p != nil && p.hwnd == hwnd {
		switch uint32(msg) {
		case wmMouseActivate:
			return maNoActivate // щелчок не забирает фокус у игры
		case wmMouseMove:
			if !p.tracking {
				tme := trackMouseEvent{flags: tmeLeave, hwnd: hwnd}
				tme.size = uint32(unsafe.Sizeof(tme))
				procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				p.tracking = true
			}
			if h := p.hit(lParam); h != p.hover {
				p.hover = h
				p.repaint()
			}
			return 0
		case wmMouseLeave:
			p.tracking = false
			if p.hover != ButtonNone {
				p.hover = ButtonNone
				p.repaint()
			}
			return 0
		case wmLButtonDown:
			if h := p.hit(lParam); h != ButtonNone {
				procSetCapture.Call(hwnd)
				p.down = h
				p.repaint()
			}
			return 0
		case wmLButtonUp:
			procReleaseCapture.Call()
			was := p.down
			p.down = ButtonNone
			if was != ButtonNone {
				p.repaint()
			}
			if was != ButtonNone && p.hit(lParam) == was {
				procPostMessage.Call(hwnd, wmPlaquePress, uintptr(was), 0)
			}
			return 0
		case wmPlaquePress:
			p.onPress(Button(wParam))
			return 0
		case wmAppStickerFrame:
			if p.anim != nil {
				p.anim.handled()
				safely(p.step)
			}
			return 0
		case wmPlaqueSprite:
			safely(p.spriteReady)
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
