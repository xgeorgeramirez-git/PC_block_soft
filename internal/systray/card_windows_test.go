//go:build windows

package systray

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"pcblock/internal/ipc"
)

var procPeekMessage = user32.NewProc("PeekMessageW")

// pump разбирает очередь сообщений потока: окна живут в потоке теста.
func pump(d time.Duration) {
	var m tagMSG
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1) // PM_REMOVE
		if r == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func send(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessage.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// Ловит: табло значка не открывается и не рисуется (пустой кадр, прозрачность потеряна), не
// закрывается по Esc и потере активации, мышь и клавиши не доходят до кнопок, нажатие
// проходит, пока команда в пути (D-53, S34).
func TestCardWindowShowsAndHandlesInput(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var pressed []CardTarget
	c, err := newCard(func(tg CardTarget) { pressed = append(pressed, tg) })
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer c.close()
	hidden := 0
	c.onHide = func() { hidden++ }

	c.setView(CountdownFor(freshSnap(okStatus(5025, 7200))))
	if c.visible || c.bits != nil {
		t.Fatal("до показа табло не рисуется")
	}
	c.show(Rect{1800, 1048, 1816, 1072}, true)
	if !c.visible || len(c.bits) != c.layout.W*c.layout.H*4 {
		t.Fatalf("табло не открылось: видно %v, буфер %d", c.visible, len(c.bits))
	}
	if c.face != fontBahnschrift && c.face != fontFallback {
		t.Fatalf("шрифт %q", c.face)
	}
	for i := 3; i < len(c.bits); i += 4 {
		if c.bits[i] != c.baseBGRA[i] {
			t.Fatalf("прозрачность пикселя %d не из рисунка", i/4)
		}
	}

	// Ошибка службы: стрелка есть и тут, фокус остаётся на ней; текст в несколько строк.
	c.setView(CountdownFor(Snapshot{}))
	if c.state.Focus != TargetArrow {
		t.Fatalf("фокус %v, ждали стрелку", c.state.Focus)
	}
	if !c.onKey(vkRight) || c.state.Focus != TargetRefresh || !c.state.FocusVisible {
		t.Fatalf("стрелка вправо: %+v", c.state)
	}
	if !c.onKey(vkTab) || c.state.Focus == TargetNone {
		t.Fatalf("Tab: %+v", c.state)
	}
	if c.onKey(0x41) {
		t.Fatal("буква не табло")
	}
	c.state.Focus = TargetRefresh
	c.onKey(vkReturn)
	pump(30 * time.Millisecond)
	if len(pressed) != 1 || pressed[0] != TargetRefresh {
		t.Fatalf("Enter: %v", pressed)
	}
	c.setView(CountdownView{Kind: CountdownMessage, Title: "СЕЙЧАС НЕЛЬЗЯ", Big: "Можно снова в 07:00"})
	c.setView(CountdownView{Kind: CountdownMessage, Title: "СЕЙЧАС НЕЛЬЗЯ", Big: "Можно снова в 07:00"}) // тот же вид

	// Мышь по стрелке: наведение, рука, нажатие и отпускание над ней.
	lp := lParamFromPoint(c.layout.CloseCenter())
	send(c.hwnd, wmMouseMove, 0, lp)
	if c.state.Hover != TargetArrow || !c.tracking {
		t.Fatalf("наведение: %+v", c.state)
	}
	if send(c.hwnd, wmSetCursor, c.hwnd, 0) != 1 {
		t.Fatal("над кнопкой указатель рукой")
	}
	send(c.hwnd, wmLButtonDown, 1, lp)
	if c.state.Pressed != TargetArrow {
		t.Fatalf("нажатие: %+v", c.state)
	}
	send(c.hwnd, wmLButtonUp, 0, lp)
	pump(30 * time.Millisecond)
	if len(pressed) != 2 || pressed[1] != TargetArrow || c.state.Pressed != TargetNone {
		t.Fatalf("щелчок: %v, %+v", pressed, c.state)
	}
	send(c.hwnd, wmLButtonDown, 1, 0) // мимо кнопок
	send(c.hwnd, wmMouseLeave, 0, 0)
	if c.state.Hover != TargetNone {
		t.Fatal("уход мыши снимает наведение")
	}
	c.setBusy(true)
	c.press(TargetArrow)
	c.setBusy(false)
	if len(pressed) != 2 {
		t.Fatal("пока команда в пути, нажатия игнорируются")
	}
	if c.visible { // табло могла закрыть потеря активации: другое окно раннера на переднем плане
		send(c.hwnd, wmTimer, cardTimerID, 0)
		if !c.visible {
			t.Fatal("таймер не закрывает табло до предельного срока")
		}
	}
	send(c.hwnd, wmMouseActivate, 0, 0)

	send(c.hwnd, wmKeyDown, vkEscape, 0)
	if c.visible || hidden == 0 {
		t.Fatalf("Esc: видно %v, закрытий %d", c.visible, hidden)
	}
	c.show(Rect{}, false)
	if c.visible {
		t.Fatal("сразу после закрытия табло не открывается заново")
	}
	c.hiddenAt = time.Time{}
	c.show(Rect{}, false) // место у указателя
	if !c.visible {
		t.Fatal("табло у указателя не открылось")
	}
	c.shownAt = time.Now().Add(-cardMaxShown - time.Second)
	before := hidden
	send(c.hwnd, wmTimer, cardTimerID, 0)
	if c.visible {
		t.Fatal("забытое табло закрывается само")
	}
	c.hiddenAt = time.Time{}
	c.show(Rect{1800, 1048, 1816, 1072}, true)
	send(c.hwnd, wmActivate, waInactive, 0)
	if c.visible || hidden <= before {
		t.Fatalf("потеря активации: видно %v, закрытий %d", c.visible, hidden)
	}
	after := hidden
	c.hide() // уже закрыто: молча
	if hidden != after {
		t.Fatal("повторное закрытие не сообщает")
	}
}

// Ловит: плашка-табло стража не показывается, перерисовывает слой без смены вида, крестик не
// доходит до стража, плашка забирает активацию щелчком (R-2, FR-9).
func TestWarningCardInSystrayPackage(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	closed := 0
	w, err := NewWarningCard(func() { closed++ })
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	defer w.Close()
	if w.HWND() == 0 || w.Visible() {
		t.Fatal("плашка создаётся скрытой")
	}
	w.ClickClose() // скрытая: ничего
	v := TimeCountdown(1800)
	v.Line = "Через 30 минут время на сегодня закончится"
	w.Show(v)
	if !w.Visible() || !w.c.layout.Close {
		t.Fatal("плашка с крестиком должна быть видна")
	}
	base := w.c.baseBGRA
	w.Show(v) // тот же вид: только «поверх всех»
	if &w.c.baseBGRA[0] != &base[0] {
		t.Fatal("тот же вид не пересобирает рисунок")
	}
	v2 := TimeCountdown(900)
	v2.Line = "Через 15 минут время на сегодня закончится"
	w.Show(v2)
	if w.c.view != v2 {
		t.Fatal("новый вид не принят")
	}
	if send(w.c.hwnd, wmMouseActivate, 0, 0) != maNoActivate {
		t.Fatal("щелчок по плашке не должен активировать её")
	}
	send(w.c.hwnd, wmActivate, waInactive, 0)
	if !w.Visible() {
		t.Fatal("плашка не прячется по потере активации")
	}
	send(w.c.hwnd, wmKeyDown, vkEscape, 0)
	if !w.Visible() {
		t.Fatal("у плашки нет клавиатуры")
	}
	w.ClickClose()
	pump(30 * time.Millisecond)
	if closed != 1 {
		t.Fatalf("крестик дошёл %d раз, ждали 1", closed)
	}
	w.Hide()
	w.Hide()
	if w.Visible() {
		t.Fatal("плашка должна прятаться")
	}
}

// Ловит: щелчок по значку в детском сеансе открывает не табло, во взрослом не мини-панель;
// кнопки табло и панели не доходят до своих действий; ответ службы «окно открыто» не закрывает
// табло и панель (ТЗ 8.2.1 п.7, D-53).
func TestAppTogglesCardAndPanel(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c, err := newCard(nil)
	if err != nil {
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	p, err := newPanel(nil)
	if err != nil {
		c.close()
		t.Skipf("окно создать нельзя (нет рабочего стола): %v", err)
	}
	a := &app{caller: &lazyCaller{}, kick: make(chan struct{}, 1), icon: newTrayIcon(0, wmTrayCallback), card: c, panel: p}
	c.onPress = a.onCardPress
	p.onPress = a.onPanelButton
	c.onHide = func() { a.cardOpen.Store(false) } // как в Run
	theApp = a
	// Окна уничтожаются в своём потоке: defer, а не t.Cleanup (тот идёт после UnlockOSThread).
	defer func() {
		theApp = nil
		a.shutdown() // убирает и значок, если оболочка его приняла
		a.caller.close()
	}()

	a.tracker.OnStatus(time.Now(), okStatus(5025, 7200))
	a.refresh()
	a.togglePanel()
	if !c.visible || p.visible || !a.cardOpen.Load() {
		t.Fatalf("ребёнок: табло %v, панель %v, секундный опрос %v", c.visible, p.visible, a.cardOpen.Load())
	}
	if len(a.kick) != 1 {
		t.Fatal("к открытию нужен свежий опрос")
	}
	<-a.kick
	a.setNote("Не получилось открыть окно. Попробуй ещё раз")
	a.refresh()
	if c.view.Kind != CountdownError || c.view.Big != "Не получилось открыть окно. Попробуй ещё раз" {
		t.Fatalf("отказ открыть окно: %+v", c.view)
	}
	a.onCardPress(TargetRefresh)
	if a.currentNote() != "" || len(a.kick) != 1 {
		t.Fatal("«Обновить» снимает отказ и просит опрос")
	}
	<-a.kick
	a.onCardPress(TargetNone)
	a.onPanelButton(PanelButton{Kind: PanelSpendBonus, Disabled: true, DisabledNote: "Из копилки можно взять, когда закончатся запретные часы"})
	if a.currentNote() != "Из копилки можно взять, когда закончатся запретные часы" {
		t.Fatalf("недоступная кнопка объясняет: %q", a.currentNote())
	}
	a.onPanelButton(PanelButton{Kind: PanelKind(99)})
	a.setBusy(true)
	if !c.busy || !p.busy {
		t.Fatal("команда в пути запрещает нажатия везде")
	}
	hostProc(0, wmOpenDone, 1, 0)
	if c.visible || c.busy || a.cardOpen.Load() {
		t.Fatalf("окно открыто: табло %v, занято %v", c.visible, c.busy)
	}

	// Взрослый сеанс: мини-панель.
	a.tracker.OnStatus(time.Now(), ipc.ChildStatusNotChild())
	a.togglePanel()
	if !p.visible || c.visible {
		t.Fatalf("взрослый: панель %v, табло %v", p.visible, c.visible)
	}
	if a.filter(&tagMSG{hwnd: p.hwnd, message: wmKeyDown, wParam: vkEscape}) != true || p.visible {
		t.Fatal("Esc закрывает панель")
	}
	if a.filter(&tagMSG{hwnd: p.hwnd, message: wmKeyDown, wParam: vkEscape}) {
		t.Fatal("закрытая панель клавиши не разбирает")
	}
	p.hiddenAt = time.Time{}
	a.togglePanel()
	a.togglePanel()
	if p.visible {
		t.Fatal("второй щелчок закрывает панель")
	}
	hostProc(0, wmStatus, 0, 0)
	hostProc(0, wmOpenDone, 0, 0)
	hostProc(0, wmTrayCallback, 0, 0x0200) // движение мыши: не щелчок
	if hostProc(0, wmQueryEndSession, 0, 0) != 1 {
		t.Fatal("значок не мешает завершить сеанс")
	}
	if a.isNotChild() {
		t.Fatal("флаг взрослого ставит только опрос службы")
	}
}

// Ловит: кнопки панели и табло без окон делают не своё: недоступная не объясняет, «Обновить» не
// просит опрос, табло с отказом не показывает его; взрослый щелчок без панели падает; сеанс не
// отпускается при выходе. Окна не нужны: проверяется и без рабочего стола.
func TestAppActionsWithoutWindows(t *testing.T) {
	a := &app{caller: &lazyCaller{}, kick: make(chan struct{}, 1), icon: newTrayIcon(0, wmTrayCallback)}
	theApp = a
	defer func() {
		theApp = nil
		a.caller.close()
	}()
	a.onPanelButton(PanelButton{Kind: PanelSpendBonus, Disabled: true, DisabledNote: "позже"})
	if a.currentNote() != "позже" {
		t.Fatalf("пояснение: %q", a.currentNote())
	}
	a.onCardPress(TargetRefresh)
	if a.currentNote() != "" || len(a.kick) != 1 {
		t.Fatal("«Обновить» снимает пояснение и просит опрос")
	}
	a.kickPoll() // очередь занята: второй запрос не блокирует
	<-a.kick
	a.onCardPress(TargetClose)
	a.onPanelButton(PanelButton{Kind: PanelKind(99)})
	if len(a.kick) != 0 || a.currentNote() != "" {
		t.Fatal("кнопка без действия ничего не делает")
	}
	a.tracker.OnStatus(time.Now(), okStatus(5025, 7200))
	a.setNote("Не получилось открыть окно. Попробуй ещё раз")
	if v := a.cardView(time.Now()); v.Kind != CountdownError || v.Big != "Не получилось открыть окно. Попробуй ещё раз" {
		t.Fatalf("табло с отказом: %+v", v)
	}
	a.setBusy(true) // без окон: молча
	a.togglePanelMini()
	if a.isNotChild() {
		t.Fatal("флаг взрослого ставит только опрос службы")
	}
	if hostProc(0, wmQueryEndSession, 0, 0) != 1 {
		t.Fatal("значок не мешает завершить сеанс")
	}
	if hostProc(0, wmStatus, 0, 0) != 0 || hostProc(0, wmTestOpenCard, 0, 0) != 0 {
		t.Fatal("без значка сообщения разбираются молча")
	}
	done := make(chan struct{})
	go func() { safely(func() { panic("кадр") }); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("паника кадра убила горутину")
	}
}
