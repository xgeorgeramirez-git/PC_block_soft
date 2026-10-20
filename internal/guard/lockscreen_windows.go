//go:build windows

package guard

import (
	"fmt"
	"strings"
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
	buttons  map[Button]uintptr
	views    map[Button]ButtonView

	onPress func(Button)
	onEdit  func(string)
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
// если вид тот же, ничего не перерисовывается.
func (u *lockUI) apply(v View, notice string) {
	shown := v
	shown.Warning = nil
	sig := fmt.Sprintf("%+v|%s", shown, notice)
	if sig == u.sig {
		return
	}
	u.sig = sig
	u.view, u.notice = v, notice
	u.layout()
}

type entryKind int

const (
	entText entryKind = iota
	entEdit
	entButton
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
	addText(v.Lock.Reason, u.fonts.title, colorText, 0)
	addText(v.Lock.Detail, u.fonts.body, colorSecondary, s(12))
	addText(v.Lock.Until, u.fonts.body, colorSecondary, s(16))
	addText(v.Lock.Used, u.fonts.body, colorSecondary, s(6))
	addText(v.PanelText, u.fonts.body, colorText, s(32))
	addText(u.notice, u.fonts.small, colorErrorText, s(16))

	if v.Panel == PanelPassword {
		es = append(es, entry{kind: entEdit, item: FlowItem{W: s(380), H: s(52), Gap: s(32)}})
		addText(v.PasswordError, u.fonts.small, colorErrorText, s(10))
	}
	list := v.Buttons
	if len(v.Choices) > 0 {
		list = v.Choices
	}
	for i, b := range list {
		gap := s(14)
		if i == 0 {
			gap = s(32)
		}
		es = append(es, entry{kind: entButton, item: FlowItem{W: s(380), H: s(54), Gap: gap}, btn: b})
	}
	for _, b := range list {
		if !b.Enabled && b.Hint != "" && !strings.Contains(v.PasswordError, b.Hint) {
			addText(b.Hint, u.fonts.small, colorSecondary, s(12))
		}
	}

	items := make([]FlowItem, len(es))
	for i, e := range es {
		items[i] = e.item
	}
	top := FlowTop(Rect{area.left, area.top, area.right, area.bottom}, FlowHeight(items), s(24))
	rects := Flow(Rect{area.left, area.top, area.right, area.bottom}, top, items)

	u.texts, u.hasFrame = u.texts[:0], false
	want := map[Button]rect{}
	var order []Button
	for i, e := range es {
		rc := toRect(rects[i])
		switch e.kind {
		case entText:
			u.texts = append(u.texts, lockText{text: e.text, rc: rc, font: e.font, color: e.color})
		case entEdit:
			u.frame, u.hasFrame = rc, true
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
	if u.view.Panel == PanelPassword && u.view.PasswordEnabled && u.edit != 0 {
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
		procShowWindow.Call(u.edit, swHide)
	}
	if v.PasswordClearSeq != u.clearSeq {
		u.clearSeq = v.PasswordClearSeq
		setWindowText(u.edit, "")
		if u.onEdit != nil {
			u.onEdit("")
		}
		if v.Panel == PanelPassword && v.PasswordEnabled {
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

// paint рисует тексты и рамку поля пароля; фон уже залит окном.
func (u *lockUI) paint(hdc uintptr, rc *rect) {
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
	if u.surfaceBrush != 0 {
		procDeleteObject.Call(u.surfaceBrush)
		u.surfaceBrush = 0
	}
}
