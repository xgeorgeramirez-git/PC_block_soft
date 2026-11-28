//go:build windows

package guard

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Окно «Сообщение от родителя» (FR-41, вид - ТЗ 8.2, макет вариант Б): сверху по центру стикер,
// нижней частью на светлой карточке с синей рамкой и тенью; в карточке по центру подпись, текст
// родителя и синяя кнопка «Понял» во всю ширину. Поверх всех окон и поверх экрана блокировки PC Block:
// там окно не закрывает его заголовок и кнопки (FitNote, NFR-10); не выше 55 % рабочей области
// (Ruling FB28-1). Длинный текст виден целиком: карточка шире (до 80 %), над экраном блокировки он
// раздвигается, и только потом шрифт мельче, не меньше 14 px. Не берёт фокус (WS_EX_NOACTIVATE,
// MA_NOACTIVATE): экран блокировки держит ввод пароля и клавиатуру, игра - полный экран.
//
// Окно со слоем прозрачности (WS_EX_LAYERED, образец internal/systray/card_windows.go): основу
// (тень, карточка, кнопка) рисует чистый Go (RenderNoteBase), текст - шрифт системы прямо в тот же
// буфер, прозрачность после него возвращается из основы, и кадр уходит на экран UpdateLayeredWindow.
// Вне тени пиксели прозрачны: там щелчки проходят к окнам под ним (кнопки и поле пароля экрана
// блокировки). Основа собирается один раз на смену раскладки или подсветки кнопки; кадр стикера
// (30 в секунду по кругу, пока окно видно) кладётся поверх неё только в прямоугольник стикера
// (NoteStickerOver) и выводится UpdateLayeredWindowIndirect с prcDirty - только этот прямоугольник.
// Спрайт стикера разбирается в своей горутине (поток окон держит перехват клавиш) и лежит номерами
// палитры (NoteSprite, NFR-5); при скрытии окна спрайт, кадр и буферы освобождаются. Что показать и
// когда убрать, решает Model; окно рисует и сообщает нажатие.

var (
	procCreateDIBSection            = gdi32.NewProc("CreateDIBSection")
	procGdiFlush                    = gdi32.NewProc("GdiFlush")
	procUpdateLayeredWindow         = user32.NewProc("UpdateLayeredWindow")
	procUpdateLayeredWindowIndirect = user32.NewProc("UpdateLayeredWindowIndirect")
)

const (
	wsExLayered = 0x00080000
	ulwAlpha    = 0x00000002
	acSrcOver   = 0
	acSrcAlpha  = 1
	dibRGB      = 0
	// ANTIALIASED_QUALITY: ClearType на слое с прозрачностью даёт цветную кайму (card_windows.go).
	antialiasedQuality = 4

	wmNotePress  = wmApp + 31 // нажатие «Понял» разбирается не внутри сообщения мыши
	wmNoteSprite = wmApp + 32 // спрайт стикера сообщения разобран в своей горутине
)

// layerPoint, layerSize, layerBlend - POINT, SIZE и BLENDFUNCTION для UpdateLayeredWindow.
type (
	layerPoint struct{ x, y int32 }
	layerSize  struct{ cx, cy int32 }
	layerBlend struct{ op, flags, alpha, format byte }
)

// layerInfo - UPDATELAYEREDWINDOWINFO для UpdateLayeredWindowIndirect.
type layerInfo struct {
	size   uint32
	hdcDst uintptr
	pptDst *layerPoint
	psize  *layerSize
	hdcSrc uintptr
	pptSrc *layerPoint
	crKey  uint32
	pblend *layerBlend
	flags  uint32
	dirty  *rect
}

var (
	theNote     *noteWindow
	noteProcPtr uintptr
)

func init() { noteProcPtr = windows.NewCallback(noteProc) }

// noteColorRef - COLORREF цвета 0xRRGGBB.
func noteColorRef(c uint32) uintptr { return rgb(c>>16&0xFF, c>>8&0xFF, c&0xFF) }

// Цвета окна сообщения - те же константы, что у рисунка основы (noterender.go): рамка и заголовок
// синие, текст родителя почти чёрный, подпись кнопки белая (ТЗ 8.2).
var (
	noteFrameColor = noteColorRef(NoteBlueRGB)
	noteTitleColor = noteColorRef(NoteBlueRGB)
	noteInkColor   = noteColorRef(NoteInkRGB)
)

type noteWindow struct {
	hwnd, hInst uintptr
	dpi         int
	fonts       map[[2]int]uintptr // шрифт по размеру при 96 dpi и жирности
	onPress     func(Button)
	// onShown - что окно вывело: стикер ("" - без) и кадр (Controller.SetNoteShown, тестовый режим).
	onShown func(sticker string, frame int)
	// onFit - каким шрифтом (пиксели экрана, 0 - окна нет) окно рисует текст и виден ли он целиком
	// (NoteTextFits; Controller.SetNoteFit, тестовый режим).
	onFit func(fontPx int, fits bool)

	key     string
	title   string
	text    string // как прислала служба
	button  ButtonView
	view    NoteView  // последний вид: стикер, начало показа, анимации
	sticker string    // стикер, который окно рисует: v.Sticker, если его спрайт разбирается
	lock    *NoteLock // экран блокировки под окном (nil - нет)
	visible bool
	geom    rect
	fit     NoteFit
	fits    bool // текст виден целиком (NoteTextFits по раскладке и рабочей области)

	hover, down bool
	tracking    bool

	// Рисунок: DIB секция окна (bits) и основа с текстом без стикера (base) - по W×H×4.
	memDC, dib, oldBmp uintptr
	bits               []byte
	dibW, dibH         int32
	base               []byte

	// Стикер: спрайт номерами палитры, кадр, выведенный последним (-1 - ни один), шаг кадров.
	sprite  *NoteSprite
	pending string // разбор идёт для этого номера
	failed  string // разбор этого номера не удался: окно без стикера
	cur     int
	anim    *stickerAnim

	inbox noteSpriteInbox // разобранные горутинами и ещё не забранные потоком окон, по номеру
}

func newNoteWindow(onPress func(Button)) (*noteWindow, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	if onPress == nil {
		onPress = func(Button) {}
	}
	n := &noteWindow{hInst: hInst, dpi: systemDPI(), onPress: onPress, fonts: map[[2]int]uintptr{}, cur: -1,
		onShown: func(string, int) {}, onFit: func(int, bool) {}}
	theNote = n
	cls, err := registerPlainClass("PCBlockGuardNote", noteProcPtr, hInst, 0)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(сообщение): %w", err)
	}
	hwnd, _, err := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWin|wsExNoActivate|wsExLayered,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(utf16(textNoteTitle))),
		wsPopup,
		0, 0, 10, 10,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx(сообщение): %w", err)
	}
	n.hwnd = hwnd
	return n, nil
}

// font - шрифт размера px (при 96 dpi) и жирности weight, сглаженный без ClearType; создаётся один раз.
func (n *noteWindow) font(px, weight int) uintptr {
	k := [2]int{px, weight}
	if f, ok := n.fonts[k]; ok {
		return f
	}
	f, _, _ := procCreateFont.Call(
		uintptr(-Scaled(px, n.dpi)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		defaultCharset, 0, 0, antialiasedQuality, 0,
		uintptr(unsafe.Pointer(utf16("Segoe UI"))),
	)
	n.fonts[k] = f
	return f
}

// stickerFor - стикер, который окно нарисует для вида v: номер из вида, если его спрайт не отказал.
func (n *noteWindow) stickerFor(v *NoteView) string {
	if v.Sticker == "" || v.Sticker == n.failed {
		return ""
	}
	return v.Sticker
}

// apply показывает окно по View.Note или прячет его (nil). lock - экран блокировки под окном
// (nil - его нет): окно встаёт между его заголовком и кнопками.
func (n *noteWindow) apply(v *NoteView, lock *NoteLock) {
	if v == nil {
		n.hide()
		return
	}
	n.view = *v
	sticker := n.stickerFor(v)
	n.wantSprite(sticker)
	key := fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%v", v.Seq, v.Title, v.Text, sticker, lock)
	if key == n.key && n.visible {
		n.step() // анимации Windows могли выключиться: стикер встаёт на первый кадр
		n.syncAnim()
		n.raise()
		return
	}
	n.key, n.title, n.text, n.sticker = key, v.Title, v.Text, sticker
	if lock != nil {
		l := *lock
		n.lock = &l
	} else {
		n.lock = nil
	}
	n.button = ButtonView{}
	if len(v.Buttons) > 0 {
		n.button = v.Buttons[0]
	}
	// Заголовок окна - подпись и текст целиком: его читает экранный диктор (доступность).
	setWindowText(n.hwnd, v.Title+": "+v.Text)
	n.hover, n.down = false, false
	n.layout(lock)
	if !n.paintAll(true) {
		n.key = "" // буфер рисунка не создан: следующий кадр стража попробует снова
		n.onFit(0, false)
		return
	}
	n.onFit(int(Scaled(n.fit.FontPx, n.dpi)), n.fits)
	procSetWindowPos.Call(n.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	n.syncAnim()
}

// raise держит окно над экраном блокировки и полноэкранной игрой без активации. Зовут кадр стража
// и таймер экрана блокировки (он каждые 500 мс поднимает себя наверх).
func (n *noteWindow) raise() {
	if n.visible {
		procSetWindowPos.Call(n.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	}
}

// hide прячет окно и освобождает спрайт, кадр и буферы рисунка (NFR-5): окно может не понадобиться
// часами.
func (n *noteWindow) hide() {
	if !n.visible {
		return
	}
	n.visible, n.key = false, ""
	n.hover, n.down = false, false
	n.stopAnim()
	procShowWindow.Call(n.hwnd, swHide)
	n.freeSurface()
	n.sprite, n.cur = nil, -1
	n.onShown("", 0)
	n.onFit(0, false)
}

// layout подбирает окно под место (FitNote: стикер, ширина, плотность, шрифт, сдвиг экрана
// блокировки): на рабочем столе сверху по центру рабочей области, над экраном блокировки - между
// заголовком и кнопками. Текст мерится тем шрифтом, которым рисуется (полужирный 600); той же меркой
// проверяется, что он виден целиком (NoteTextFits, приёмка S57).
func (n *noteWindow) layout(lock *NoteLock) {
	wa := workArea()
	hdc, _, _ := procGetDC.Call(n.hwnd)
	measure := func(px int, width int32, s string) int32 {
		return measureHeight(hdc, n.font(px, 600), s, width)
	}
	area := Rect{Left: wa.left, Top: wa.top, Right: wa.right, Bottom: wa.bottom}
	n.fit = FitNote(n.dpi, area, lock, n.sticker != "", n.title, n.text, measure)
	n.fits = NoteTextFits(n.fit, area, measure)
	procReleaseDC.Call(n.hwnd, hdc)
	n.geom, n.visible = toRect(n.fit.Place), true
}

// ensureSurface держит DIB секцию 32 бита размера окна (строки сверху вниз) в своём DC.
func (n *noteWindow) ensureSurface(w, h int32) bool {
	if n.dib != 0 && n.dibW == w && n.dibH == h {
		return true
	}
	n.freeSurface()
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
	n.memDC, n.dib, n.oldBmp, n.dibW, n.dibH = mem, dib, old, w, h
	n.bits = unsafe.Slice(ptrTo[byte](ptr), int(w)*int(h)*4)
	return true
}

// freeSurface освобождает DIB секцию, её DC и основу.
func (n *noteWindow) freeSurface() {
	if n.memDC != 0 {
		procSelectObject.Call(n.memDC, n.oldBmp)
		procDeleteObject.Call(n.dib)
		procDeleteDC.Call(n.memDC)
	}
	n.memDC, n.dib, n.oldBmp, n.bits, n.dibW, n.dibH = 0, 0, 0, nil, 0, 0
	n.base = nil
}

// buildBase собирает основу: рисунок без текста (RenderNoteBase), поверх него текст шрифтом системы,
// затем прозрачность возвращается из рисунка (шрифт системы её не ведёт). Итог - в DIB и в base.
func (n *noteWindow) buildBase() bool {
	l := n.fit.Layout
	if !n.ensureSurface(l.W, l.H) {
		return false
	}
	base := RenderNoteBase(l, n.hover || n.down)
	if len(base) != len(n.bits) {
		return false
	}
	copy(n.bits, base)
	n.drawTexts()
	procGdiFlush.Call() // текст лёг в пиксели до чтения их из Go
	for i := 3; i < len(base); i += 4 {
		n.bits[i] = base[i]
	}
	copy(base, n.bits)
	n.base = base
	return true
}

// drawTexts пишет подпись (синим полужирным), текст родителя (полужирным почти чёрным, по центру, с
// переводами строк) и подпись кнопки (белым) в DC окна.
func (n *noteWindow) drawTexts() {
	l, d, hdc := n.fit.Layout, n.fit.Density, n.memDC
	procSetBkMode.Call(hdc, transparent)
	drawWrapped(hdc, n.font(d.TitlePx, 600), noteTitleColor, n.title, toRect(l.Title), dtCenter)
	drawWrapped(hdc, n.font(n.fit.FontPx, 600), noteInkColor, n.fit.Text, toRect(l.Text), dtCenter)
	if n.button.Button != ButtonNone {
		old, _, _ := procSelectObject.Call(hdc, n.font(d.BtnPx, 600))
		procSetTextColor.Call(hdc, colorWhite)
		rc := toRect(l.Button)
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(utf16(n.button.Label))), ^uintptr(0),
			uintptr(unsafe.Pointer(&rc)), dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
		procSelectObject.Call(hdc, old)
	}
}

// putFrame кладёт кадр k стикера поверх основы в прямоугольник стикера; false - стикера нет (не
// выбран или спрайт ещё не разобран): там остаётся основа.
func (n *noteWindow) putFrame(k int) bool {
	l := n.fit.Layout
	s := n.sprite
	if s == nil || n.sticker == "" || s.Sticker != n.sticker || l.StickerPx <= 0 || n.base == nil {
		return false
	}
	if !NoteStickerOver(n.bits, n.base, l, s.FrameBGRA(k, int(l.StickerPx))) {
		return false
	}
	n.cur = k
	n.onShown(n.sticker, k)
	return true
}

// paintAll собирает окно целиком (основа, текст, кадр стикера) и выводит его; move - окно встаёт
// на место раскладки. false - нечем рисовать (DIB не создан).
func (n *noteWindow) paintAll(move bool) bool {
	if !n.buildBase() {
		return false
	}
	n.cur = -1
	if !n.putFrame(NoteStickerFrame(&n.view, time.Now())) {
		n.onShown("", 0)
	}
	var at *layerPoint
	if move {
		at = &layerPoint{n.geom.left, n.geom.top}
	}
	n.update(at)
	return true
}

// update выводит DIB окна целиком (at != nil - и ставит окно в эту точку экрана).
func (n *noteWindow) update(at *layerPoint) {
	size := layerSize{n.dibW, n.dibH}
	src := layerPoint{}
	blend := layerBlend{op: acSrcOver, alpha: 255, format: acSrcAlpha}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	procUpdateLayeredWindow.Call(n.hwnd, screen, uintptr(unsafe.Pointer(at)), uintptr(unsafe.Pointer(&size)),
		n.memDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

// updateSticker выводит только прямоугольник стикера (prcDirty): остальное окно на экране не
// пересчитывается. Не вышло - окно целиком.
func (n *noteWindow) updateSticker() {
	s := n.fit.Layout.Sticker
	dirty := rect{max(s.Left, 0), max(s.Top, 0), min(s.Right, n.dibW), min(s.Bottom, n.dibH)}
	size := layerSize{n.dibW, n.dibH}
	src := layerPoint{}
	blend := layerBlend{op: acSrcOver, alpha: 255, format: acSrcAlpha}
	info := layerInfo{hdcSrc: n.memDC, psize: &size, pptSrc: &src, pblend: &blend, flags: ulwAlpha, dirty: &dirty}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, _ := procUpdateLayeredWindowIndirect.Call(n.hwnd, uintptr(unsafe.Pointer(&info))); r == 0 {
		n.update(nil)
	}
}

// step выводит кадр стикера на сейчас, если он сменился (шаг кадров и кадр стража).
func (n *noteWindow) step() {
	if !n.visible || n.sprite == nil || n.memDC == 0 {
		return
	}
	k := NoteStickerFrame(&n.view, time.Now())
	if k == n.cur {
		return
	}
	if n.putFrame(k) {
		n.updateSticker()
	}
}

// repaint пересобирает основу (подсветка кнопки сменилась) и выводит окно на месте.
func (n *noteWindow) repaint() {
	if n.visible {
		n.paintAll(false)
	}
}

// syncAnim держит шаг кадров, пока окно видно, стикер на экране и движется (анимации Windows
// включены); иначе останавливает: лишних пробуждений потока окон нет.
func (n *noteWindow) syncAnim() {
	want := n.visible && n.sticker != "" && n.sprite != nil && n.fit.Layout.StickerPx > 0 && !n.view.Still
	if want == (n.anim != nil) {
		return
	}
	if want {
		n.anim = startStickerAnim(n.hwnd)
	} else {
		n.stopAnim()
	}
}

func (n *noteWindow) stopAnim() {
	if n.anim != nil {
		n.anim.close()
		n.anim = nil
	}
}

// wantSprite готовит спрайт стикера sticker: разбирает его в своей горутине (доли секунды; поток
// окон с перехватом клавиш не ждёт), готовность приходит окну сообщением wmNoteSprite. "" - спрайт
// не нужен и отпускается.
func (n *noteWindow) wantSprite(sticker string) {
	if sticker == "" {
		n.sprite = nil
		return
	}
	if (n.sprite != nil && n.sprite.Sticker == sticker) || n.pending == sticker {
		return
	}
	n.pending = sticker
	hwnd := n.hwnd
	go safely(func() {
		s, err := LoadNoteSprite(sticker)
		if err != nil {
			s = nil
		}
		n.inbox.put(sticker, s)
		procPostMessage.Call(hwnd, wmNoteSprite, 0, 0)
	})
}

// spriteReady забирает разобранные спрайты (noteSpriteInbox: по номеру, порядок окончания разборов
// не важен): свой - кадр на экран и шаг кадров; чужие (стикер уже сменился, окно скрыто)
// выбрасываются; разбор не удался - окно без стикера.
func (n *noteWindow) spriteReady() {
	want := ""
	if n.visible {
		want = n.sticker
	}
	sprite, found, pending := n.inbox.take(want, n.pending)
	n.pending = pending
	if !found {
		return
	}
	if sprite == nil {
		n.failed = want
		n.key = "" // заново без стикера: место под картинку не пустует
		v := n.view
		n.apply(&v, n.lock)
		return
	}
	n.sprite = sprite
	n.cur = -1
	if n.putFrame(NoteStickerFrame(&n.view, time.Now())) {
		n.updateSticker()
	}
	n.syncAnim()
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
	n.stopAnim()
	if n.hwnd != 0 {
		procDestroyWindow.Call(n.hwnd)
		n.hwnd = 0
	}
	n.visible = false
	n.freeSurface()
	n.sprite = nil
	for _, f := range n.fonts {
		procDeleteObject.Call(f)
	}
	n.fonts = map[[2]int]uintptr{}
	theNote = nil
}

// raiseNoteAboveLock - таймер экрана блокировки поднял его наверх: окно сообщения снова над ним.
func raiseNoteAboveLock() {
	if n := theNote; n != nil {
		n.raise()
	}
}

func noteProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if n := theNote; n != nil && n.hwnd == hwnd {
		switch uint32(msg) {
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
				n.repaint()
			}
			return 0
		case wmMouseLeave:
			n.tracking = false
			if n.hover {
				n.hover = false
				n.repaint()
			}
			return 0
		case wmLButtonDown:
			if n.hit(lParam) {
				procSetCapture.Call(hwnd)
				n.down = true
				n.repaint()
			}
			return 0
		case wmLButtonUp:
			procReleaseCapture.Call()
			was := n.down
			n.down = false
			if was {
				n.repaint()
			}
			if was && n.hit(lParam) {
				procPostMessage.Call(hwnd, wmNotePress, uintptr(n.button.Button), 0)
			}
			return 0
		case wmNotePress:
			n.onPress(Button(wParam))
			return 0
		case wmAppStickerFrame:
			if n.anim != nil {
				n.anim.handled()
				safely(n.step)
			}
			return 0
		case wmNoteSprite:
			safely(n.spriteReady)
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
