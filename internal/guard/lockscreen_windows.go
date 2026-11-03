//go:build windows

package guard

import (
	"fmt"
	"strings"
	"unsafe"

	"pcblock/internal/systray"
)

// Идентификаторы дочерних окон экрана блокировки: кнопка получает idButtonBase плюс номер
// Button, поле пароля idEdit.
const (
	idButtonBase = 1000
	idEdit       = 2000

	wmAppPress = wmApp + 10 // нажатие кнопки обрабатывается не внутри её же сообщения
)

// lockText — строка текста экрана блокировки с готовым положением.
type lockText struct {
	text  string
	rc    rect
	font  uintptr
	color uintptr
}

// lockUI рисует Model.View() в окне экрана блокировки: тексты причины, кнопки, панель пароля
// (поле ES_PASSWORD, ошибка под ним, отсчёт паузы). Логики здесь нет: что показать, решает
// Model, а нажатия кнопок уходят обратно через onPress. Работает только в потоке окна.
type lockUI struct {
	w            *LockWindow
	dpi          int
	fonts        fontSet
	surfaceBrush uintptr

	view     View
	notice   string
	sig      string
	btnSig   string
	clearSeq int

	texts    []lockText
	frame    rect
	hasFrame bool
	frameErr bool
	edit     uintptr
	maskChar uintptr // символ точек поля пароля (EM_GETPASSWORDCHAR при создании)
	masked   bool    // поле сейчас прячет текст точками
	maskSet  bool    // режим поля уже задан
	buttons  map[Button]uintptr
	views    map[Button]ButtonView

	rest restPaint // экран перерыва: картинка и плитки таймера

	onPress func(Button)
	onEdit  func(string)
}

// restPaint — готовые пиксели экрана перерыва (8.2.3 п.12): картинка и плитки таймера уже на
// фоне экрана (OverBackground), цифры рисует шрифт системы поверх плиток каждую секунду.
type restPaint struct {
	showCat, showTiles bool
	catRc, tilesRc     rect
	catSize            int32
	catPx              []byte
	tiles              systray.CardLayout
	tilesPx            []byte
	fonts              map[[2]int]uintptr // шрифт цифр и подписей по размеру и жирности
}

func newLockUI(w *LockWindow, onPress func(Button), onEdit func(string)) *lockUI {
	u := &lockUI{
		w: w, dpi: systemDPI(), onPress: onPress, onEdit: onEdit,
		buttons: map[Button]uintptr{}, views: map[Button]ButtonView{},
	}
	u.fonts = newFontSet(u.dpi)
	u.surfaceBrush, _, _ = procCreateSolidBr.Call(colorSurface)
	w.onMsg = u.handle
	w.onPaint = u.paint
	return u
}

func metric(index uintptr) int32 {
	r, _, _ := procGetSystemMetrics.Call(index)
	return int32(uint32(r))
}

// area возвращает основной монитор в координатах окна: текст и кнопки стоят по его центру,
// а не по центру всего виртуального экрана, где середина может попасть на стык мониторов.
func (u *lockUI) area() rect {
	left, top := -metric(smXVirtScreen), -metric(smYVirtScreen)
	return rect{left, top, left + metric(0), top + metric(1)}
}

func toRect(r Rect) rect { return rect{r.Left, r.Top, r.Right, r.Bottom} }

// apply показывает вид v и короткое сообщение notice под заголовком. Вызывается часто:
// если вид тот же, ничего не перерисовывается; у экрана перерыва новая секунда таймера
// перерисовывает только плитки (LayoutKey, TimerChanged).
func (u *lockUI) apply(v View, notice string) {
	key := LayoutKey(v, notice)
	if key == u.sig {
		if TimerChanged(u.view, v) {
			u.view.Rest = v.Rest
			invalidateRect(u.w.hwnd, u.rest.tilesRc)
		}
		return
	}
	u.sig = key
	u.view, u.notice = v, notice
	u.layout()
}

type entryKind int

const (
	entText entryKind = iota
	entEdit
	entButton
	entCat   // картинка экрана перерыва
	entTiles // плитки таймера экрана перерыва
)

// entry — блок столбца экрана до раскладки.
type entry struct {
	kind  entryKind
	item  FlowItem
	text  string
	font  uintptr
	color uintptr
	btn   ButtonView
}

// layout раскладывает вид в один столбец по центру, создаёт и двигает кнопки и поле пароля.
func (u *lockUI) layout() {
	hwnd := u.w.hwnd
	if hwnd == 0 {
		return
	}
	hdc, _, _ := procGetDC.Call(hwnd)
	defer procReleaseDC.Call(hwnd, hdc)

	v := u.view
	area := u.area()
	s := func(px int) int32 { return Scaled(px, u.dpi) }
	textW := min(s(900), area.right-area.left-2*s(32))

	var es []entry
	addText := func(text string, font, color uintptr, gap int32) {
		if text == "" {
			return
		}
		h := measureHeight(hdc, font, text, textW)
		es = append(es, entry{kind: entText, item: FlowItem{W: textW, H: h, Gap: gap}, text: text, font: font, color: color})
	}
	untilGap := s(16)
	addText(v.Lock.Reason, u.fonts.title, colorText, 0)
	if v.Rest != nil {
		// Экран перерыва (8.2.3 п.12): под заголовком крупный таймер плитками табло остатка.
		tl := RestTiles(u.dpi)
		es = append(es, entry{kind: entTiles, item: FlowItem{W: int32(tl.W), H: int32(tl.H), Gap: s(restTilesGap)}})
		untilGap = s(restUntilGap)
	}
	addText(v.Lock.Detail, u.fonts.body, colorSecondary, s(12))
	addText(v.Lock.Until, u.fonts.body, colorSecondary, untilGap)
	if v.Rest != nil {
		// Сообщение родителя — не больше двух строк, длинное слово переносится по буквам.
		fits := func(line string) bool { return measureWidth(hdc, u.fonts.body, line) <= textW }
		for i, line := range WrapMessage(v.Rest.Message, restMessageLines, fits) {
			gap := s(restMsgLineGp)
			if i == 0 {
				gap = s(restMsgGap)
			}
			addText(line, u.fonts.body, colorText, gap)
		}
	}
	addText(v.Lock.Used, u.fonts.body, colorSecondary, s(6))
	addText(v.Lock.Hint, u.fonts.body, colorSecondary, s(16))
	addText(v.PanelText, u.fonts.body, colorText, s(32))
	addText(u.notice, u.fonts.small, colorErrorText, s(16))

	if v.Panel == PanelPassword || v.Panel == PanelPromo {
		// Подпись над полем: без неё непонятно, что вводить (B-12, H6): «Пароль родителя»
		// (видны только точки) или «Промокод» (текст виден, FR-35).
		addText(v.FieldLabel, u.fonts.small, colorSecondary, s(32))
		es = append(es, entry{kind: entEdit, item: FlowItem{W: s(380), H: s(52), Gap: s(8)}})
		addText(v.PasswordError, u.fonts.small, colorErrorText, s(10))
	}
	list := v.Buttons
	if len(v.Choices) > 0 {
		list = v.Choices
	}
	for i, b := range list {
		gap := s(14)
		switch {
		case i == 0:
			gap = s(32)
		case isExitButton(b.Button) && !isExitButton(list[i-1].Button):
			// Кнопки выхода (выход из учётки, выключение, смена пользователя) отделяем от
			// кнопок доступа к времени, чтобы случайно не нажать выход (B-10).
			gap = s(28)
		}
		es = append(es, entry{kind: entButton, item: FlowItem{W: s(380), H: s(54), Gap: gap}, btn: b})
		// Пояснение недоступной кнопки — сразу под ней, а не в конце списка (L-17, B-10):
		// «Из копилки можно взять с 07:00» должно стоять у «Взять из копилки».
		if !b.Enabled && b.Hint != "" && !strings.Contains(v.PasswordError, b.Hint) {
			addText(b.Hint, u.fonts.small, colorSecondary, s(6))
		}
	}

	items := make([]FlowItem, len(es))
	for i, e := range es {
		items[i] = e.item
	}
	if v.Rest != nil {
		// Картинка над заголовком; на низком экране меньше или без неё (RestCatSize).
		if cat := RestCatSize(u.dpi, area.bottom-area.top, FlowHeight(items)); cat > 0 && len(es) > 0 {
			es[0].item.Gap = s(restCatGapPx)
			es = append([]entry{{kind: entCat, item: FlowItem{W: cat, H: cat}}}, es...)
			items = append([]FlowItem{es[0].item}, items...)
			items[1] = es[1].item
		}
	}
	top := FlowTop(Rect{area.left, area.top, area.right, area.bottom}, FlowHeight(items), s(24))
	rects := Flow(Rect{area.left, area.top, area.right, area.bottom}, top, items)

	u.texts, u.hasFrame = u.texts[:0], false
	u.rest.showCat, u.rest.showTiles = false, false
	want := map[Button]rect{}
	var order []Button
	for i, e := range es {
		rc := toRect(rects[i])
		switch e.kind {
		case entText:
			u.texts = append(u.texts, lockText{text: e.text, rc: rc, font: e.font, color: e.color})
		case entEdit:
			u.frame, u.hasFrame = rc, true
		case entCat:
			u.rest.showCat, u.rest.catRc = u.prepareCat(rc.right-rc.left), rc
		case entTiles:
			u.prepareTiles()
			u.rest.showTiles, u.rest.tilesRc = true, rc
		case entButton:
			want[e.btn.Button] = rc
			order = append(order, e.btn.Button)
			u.views[e.btn.Button] = e.btn
		}
	}
	u.frameErr = v.PasswordError != "" && v.PasswordEnabled

	u.layoutEdit()
	u.layoutButtons(want, order)

	invalidate(hwnd)
}

// layoutButtons создаёт кнопки, если набор изменился (порядок обхода Tab совпадает с порядком
// на экране), иначе только двигает и включает или отключает.
func (u *lockUI) layoutButtons(want map[Button]rect, order []Button) {
	ids := make([]string, len(order))
	for i, b := range order {
		ids[i] = fmt.Sprint(int(b))
	}
	sig := strings.Join(ids, ",")
	changed := sig != u.btnSig
	if changed {
		for b, h := range u.buttons {
			procDestroyWindow.Call(h)
			delete(u.buttons, b)
		}
		for b := range u.views {
			if _, keep := want[b]; !keep {
				delete(u.views, b)
			}
		}
		for _, b := range order {
			h := createChild(u.w.hwnd, u.w.hInst, "BUTTON", u.views[b].Label, wsTabStop|bsOwnerDraw, idButtonBase+int(b), want[b])
			u.buttons[b] = h
		}
		u.btnSig = sig
	}
	for _, b := range order {
		h := u.buttons[b]
		if h == 0 {
			continue
		}
		if !changed {
			moveWindow(h, want[b])
		}
		enable := uintptr(0)
		if u.views[b].Enabled {
			enable = 1
		}
		procEnableWindow.Call(h, enable)
		invalidate(h)
	}
	if changed {
		u.focusFirst(order)
	}
}

// focusFirst отдаёт фокус полю пароля, если оно доступно, иначе главной кнопке.
func (u *lockUI) focusFirst(order []Button) {
	if (u.view.Panel == PanelPassword || u.view.Panel == PanelPromo) && u.view.PasswordEnabled && u.edit != 0 {
		procSetFocus.Call(u.edit)
		return
	}
	for _, b := range order {
		if u.views[b].Main && u.views[b].Enabled {
			procSetFocus.Call(u.buttons[b])
			return
		}
	}
}

// layoutEdit показывает поле пароля внутри рамки или прячет его; очищает по команде модели.
func (u *lockUI) layoutEdit() {
	v := u.view
	if u.edit == 0 {
		u.edit = createChild(u.w.hwnd, u.w.hInst, "EDIT", "", wsTabStop|esPassword|esAutoHScroll, idEdit, rect{0, 0, 10, 10})
		setFont(u.edit, u.fonts.body)
		procSendMessage.Call(u.edit, emSetLimitTxt, 256, 0)
		// Символ точек поля пароля запоминаем: на панели промокода поле показывает текст
		// (EM_SETPASSWORDCHAR 0), на панели пароля точки возвращаются.
		u.maskChar, _, _ = procSendMessage.Call(u.edit, emGetPasswordChar, 0, 0)
		procShowWindow.Call(u.edit, swHide)
	}
	u.setMasked(!v.FieldPlain)
	if v.PasswordClearSeq != u.clearSeq {
		u.clearSeq = v.PasswordClearSeq
		setWindowText(u.edit, "")
		if u.onEdit != nil {
			u.onEdit("")
		}
		if (v.Panel == PanelPassword || v.Panel == PanelPromo) && v.PasswordEnabled {
			procSetFocus.Call(u.edit)
		}
	}
	if !u.hasFrame {
		procShowWindow.Call(u.edit, swHide)
		return
	}
	pad := Scaled(16, u.dpi)
	h := Scaled(26, u.dpi)
	mid := (u.frame.top + u.frame.bottom) / 2
	moveWindow(u.edit, rect{u.frame.left + pad, mid - h/2, u.frame.right - pad, mid - h/2 + h})
	enable := uintptr(0)
	if v.PasswordEnabled {
		enable = 1
	}
	procEnableWindow.Call(u.edit, enable)
	procShowWindow.Call(u.edit, swShow)
}

// setMasked прячет набранное точками (пароль) или показывает текст (промокод). Меняет поле только
// при смене режима: EM_SETPASSWORDCHAR перерисовывает поле.
func (u *lockUI) setMasked(masked bool) {
	if u.edit == 0 || u.masked == masked && u.maskSet {
		return
	}
	ch := uintptr(0)
	if masked {
		ch = u.maskChar
		if ch == 0 {
			ch = '●'
		}
	}
	procSendMessage.Call(u.edit, emSetPasswordChar, ch, 0)
	invalidate(u.edit)
	u.masked, u.maskSet = masked, true
}

// bgBytes — цвет фона экрана байтами r, g, b (COLORREF хранит 0x00BBGGRR).
func bgBytes() (r, g, b byte) { return byte(colorBg), byte(colorBg >> 8), byte(colorBg >> 16) }

// prepareCat готовит пиксели картинки стороной size на фоне экрана; false — картинки нет.
func (u *lockUI) prepareCat(size int32) bool {
	if u.rest.catSize != size || u.rest.catPx == nil {
		px, err := RestCatBGRA(int(size))
		if err != nil || len(px) != int(size*size*4) {
			return false
		}
		r, g, b := bgBytes()
		u.rest.catPx, u.rest.catSize = OverBackground(px, r, g, b), size
	}
	return true
}

// prepareTiles готовит пиксели плиток таймера один раз: dpi окна не меняется.
func (u *lockUI) prepareTiles() {
	if u.rest.tilesPx != nil {
		return
	}
	r, g, b := bgBytes()
	u.rest.tiles = RestTiles(u.dpi)
	u.rest.tilesPx = RestTimerPixels(u.rest.tiles, r, g, b)
}

// paintRest рисует картинку и плитки с цифрами экрана перерыва.
func (u *lockUI) paintRest(hdc uintptr) {
	if u.rest.showCat {
		blitBGRA(hdc, u.rest.catRc.left, u.rest.catRc.top, u.rest.catSize, u.rest.catSize, u.rest.catPx)
	}
	if !u.rest.showTiles || u.view.Rest == nil {
		return
	}
	l, at := u.rest.tiles, u.rest.tilesRc
	blitBGRA(hdc, at.left, at.top, int32(l.W), int32(l.H), u.rest.tilesPx)
	for _, r := range systray.TileTexts(l, RestTimerView(*u.view.Rest)) {
		rc := rect{at.left + int32(r.Rect.Left), at.top + int32(r.Rect.Top), at.left + int32(r.Rect.Right), at.top + int32(r.Rect.Bottom)}
		old, _, _ := procSelectObject.Call(hdc, u.restFont(r.Px, r.Bold))
		procSetTextColor.Call(hdc, rgb(uint32(r.Color.R), uint32(r.Color.G), uint32(r.Color.B)))
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(utf16(r.Text))), ^uintptr(0), uintptr(unsafe.Pointer(&rc)),
			dtCenter|dtSingleLine|dtVCenter|dtNoPrefix)
		procSelectObject.Call(hdc, old)
	}
}

// restFont — шрифт цифр и подписей плиток (как у табло значка: Bahnschrift).
func (u *lockUI) restFont(px int, bold bool) uintptr {
	weight := 400
	if bold {
		weight = 700
	}
	k := [2]int{px, weight}
	if f, ok := u.rest.fonts[k]; ok {
		return f
	}
	if u.rest.fonts == nil {
		u.rest.fonts = map[[2]int]uintptr{}
	}
	f := makeFontFace(int32(px), weight, "Bahnschrift")
	u.rest.fonts[k] = f
	return f
}

// paint рисует тексты и рамку поля пароля; фон уже залит окном.
func (u *lockUI) paint(hdc uintptr, rc *rect) {
	u.paintRest(hdc)
	for _, t := range u.texts {
		drawWrapped(hdc, t.font, t.color, t.text, t.rc, dtCenter)
	}
	if u.hasFrame {
		line, lw := colorLine, int32(1)
		if u.frameErr {
			line, lw = colorErrorText, 2
		}
		roundBox(hdc, u.frame, colorSurface, line, lw, Scaled(6, u.dpi))
	}
}

// handle разбирает сообщения окна, которые само окно не разбирает.
func (u *lockUI) handle(msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch msg {
	case wmCommand:
		id, code := int(wParam&0xFFFF), int((wParam>>16)&0xFFFF)
		switch {
		case id == idEdit && code == enChange:
			if u.onEdit != nil {
				u.onEdit(windowText(u.edit))
			}
			return 0, true
		case id >= idButtonBase && id < idButtonBase+64 && code == bnClicked:
			// Нажатие обрабатывается следующим сообщением: пока кнопка ещё внутри своего
			// обработчика щелчка, перестраивать окно и удалять её нельзя.
			procPostMessage.Call(u.w.hwnd, wmAppPress, uintptr(id-idButtonBase), 0)
			return 0, true
		}
	case wmAppPress:
		if u.onPress != nil {
			u.onPress(Button(wParam))
		}
		return 0, true
	case wmDrawItem:
		di := ptrTo[drawItemStruct](lParam)
		b := Button(int(di.ctlID) - idButtonBase)
		bv, ok := u.views[b]
		if !ok {
			return 0, false
		}
		look := lookOf(bv.Main, bv.Enabled, di.itemState&odsSelected != 0)
		drawButton(di, bv.Label, u.fonts.button, look, Scaled(6, u.dpi), colorBg)
		return 1, true
	case wmCtlColorEdit, wmCtlColorStatic:
		if lParam != u.edit || u.edit == 0 {
			return 0, false
		}
		color := colorText
		if !u.view.PasswordEnabled {
			color = colorDimText
		}
		procSetTextColor.Call(wParam, color)
		procSetBkColor.Call(wParam, colorSurface)
		return u.surfaceBrush, true
	case wmDisplayChange:
		u.w.FitVirtualScreen()
		return 0, true
	case wmSize:
		u.layout()
		return 0, false
	}
	return 0, false
}

// passwordText возвращает то, что набрано в поле пароля.
func (u *lockUI) passwordText() string {
	if u.edit == 0 {
		return ""
	}
	return windowText(u.edit)
}

// setPasswordText записывает текст в поле (TestGuard type): так же, как ввод с клавиатуры, это
// вызывает EN_CHANGE.
func (u *lockUI) setPasswordText(s string) {
	if u.edit != 0 {
		setWindowText(u.edit, s)
	}
}

// buttonHandle возвращает ручку кнопки, если она сейчас на экране.
func (u *lockUI) buttonHandle(b Button) uintptr { return u.buttons[b] }

// focusedButton возвращает кнопку, у которой сейчас фокус; ButtonNone, если фокус не на кнопке.
func (u *lockUI) focusedButton() Button {
	f, _, _ := procGetFocus.Call()
	if f == 0 {
		return ButtonNone
	}
	for b, h := range u.buttons {
		if h == f {
			return b
		}
	}
	return ButtonNone
}

// mainButton возвращает доступную главную кнопку вида (её нажимает Enter).
func (u *lockUI) mainButton() Button {
	list := u.view.Buttons
	if len(u.view.Choices) > 0 {
		list = u.view.Choices
	}
	for _, b := range list {
		if b.Main && b.Enabled {
			return b.Button
		}
	}
	return ButtonNone
}

// close освобождает шрифты и кисть.
func (u *lockUI) close() {
	u.fonts.free()
	for k, f := range u.rest.fonts {
		procDeleteObject.Call(f)
		delete(u.rest.fonts, k)
	}
	if u.surfaceBrush != 0 {
		procDeleteObject.Call(u.surfaceBrush)
		u.surfaceBrush = 0
	}
}
