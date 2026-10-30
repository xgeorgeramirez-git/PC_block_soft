package guard

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Тесты плашки-табло (FR-9, D-55, пакет 0.2.5 T3) на поддельном времени.

func hms(w *WarningView) string {
	if w == nil || w.Board == nil {
		return ""
	}
	return w.Board.Hours + " : " + w.Board.Minutes + " : " + w.Board.Seconds
}

// Ловит: секунды на табло не идут вживую или идут не по остатку службы; табло живёт дольше 5 с.
func TestBoardTicksAndCloses(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(1800), false, []int{30})
	if got := hms(m.View(t0).Warning); got != "00 : 30 : 00" {
		t.Fatalf("за 30: %q", got)
	}
	feed(m, 1, 3, 1800)
	if got := hms(m.View(at(3 * time.Second)).Warning); got != "00 : 29 : 57" {
		t.Fatalf("через 3 с: %q", got)
	}
	feed(m, 4, 5, 1800)
	if w := m.View(at(5 * time.Second)).Warning; w != nil {
		t.Fatalf("через 5 с плашка осталась: %+v", *w)
	}
}

// Ловит: новый порог при видимой плашке не сменяет её или не даёт новый звук (новый ID).
func TestBoardReplacedByNextThreshold(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(901), false, []int{30})
	a := m.View(t0).Warning
	m.OnDecision(at(time.Second), runningDecision(900), false, []int{15})
	b := m.View(at(time.Second)).Warning
	if b == nil || b.Threshold != 15 || hms(b) != "00 : 15 : 00" || b.ID == a.ID || !SoundDue(a.ID, b) {
		t.Fatalf("табло 15 не сменило 30: %+v (прежнее %+v)", b, a)
	}
}

// Ловит: для запретных часов плитки стоят на остатке лимита (RemainingS), а не идут по настоящим
// часам до начала интервала; плашка «устаревает», хотя событие не сдвинулось (Review Focus 4).
func TestBoardForbiddenCountsByClock(t *testing.T) {
	m := NewModel()
	d := runningDecision(5000) // остаток лимита большой и стоит (ребёнок отошёл)
	m.OnWarnings(t0, d, false, []Incoming{{Threshold: 15, Text: "Через 15 минут начнутся запретные часы", Event: ipc.WarnEventForbidden, EventS: 900}})
	w := m.View(t0).Warning
	if hms(w) != "00 : 15 : 00" || w.Text != "Через 15 минут начнутся запретные часы" || w.Board.Line != w.Text {
		t.Fatalf("табло перед запретными часами: %+v", w)
	}
	keepAlive(m, d, 1, 3)
	if got := hms(m.View(at(3 * time.Second)).Warning); got != "00 : 14 : 57" {
		t.Fatalf("через 3 с при неизменном остатке лимита: %q, ждали 00 : 14 : 57", got)
	}

	one := NewModel()
	one.OnWarnings(t0, d, false, []Incoming{{Threshold: 1, Text: "Через минуту начнутся запретные часы", Event: ipc.WarnEventForbidden, EventS: 60}})
	keepAlive(one, d, 1, 20)
	if w := one.View(at(20 * time.Second)).Warning; w == nil || w.Text != "Через минуту начнутся запретные часы: 0:40" {
		t.Fatalf("плашка 1 мин перед запретными часами: %+v", w)
	}
	// Начало интервала прошло, а экрана блокировки нет (родитель снял запрет): плашка снимается.
	keepAlive(one, d, 21, 75)
	if w := one.View(at(75 * time.Second)).Warning; w != nil {
		t.Fatalf("плашка 1 мин висит после отменённого события: %+v", *w)
	}
}

// Ловит: конец паузы считается по остатку лимита; плашка остаётся, когда паузу сняли.
func TestBoardPauseEnd(t *testing.T) {
	m := NewModel()
	d := runningDecision(5000)
	d.Status = core.StatusPaused
	m.OnWarnings(t0, d, false, []Incoming{{Threshold: 5, Text: "Через 5 минут закончится пауза", Event: ipc.WarnEventPause, EventS: 300}})
	keepAlive(m, d, 1, 2)
	if got := hms(m.View(at(2 * time.Second)).Warning); got != "00 : 04 : 58" {
		t.Fatalf("конец паузы: %q", got)
	}
	d.Status = core.StatusOK
	m.OnDecision(at(3*time.Second), d, false, nil)
	if w := m.View(at(3 * time.Second)).Warning; w != nil {
		t.Fatalf("пауза снята, плашка осталась: %+v", *w)
	}
}

// Ловит: служба прислала тот же порог дважды (повторный опрос, перезапуск службы), и страж
// показал две плашки или сыграл звук дважды (класс A).
func TestBoardDuplicateWarningOnePlate(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(300), false, []int{5})
	a := m.View(t0).Warning
	m.OnDecision(at(time.Second), runningDecision(299), false, []int{5})
	b := m.View(at(time.Second)).Warning
	if b == nil || b.ID != a.ID || SoundDue(a.ID, b) {
		t.Fatalf("дубль порога дал новую плашку: %+v -> %+v", a, b)
	}
	// Крестиком закрыто, тот же порог пришёл снова: плашка не возвращается.
	m.Press(at(2*time.Second), ButtonWarningClose, "")
	m.OnDecision(at(3*time.Second), runningDecision(297), false, []int{5})
	if w := m.View(at(3 * time.Second)).Warning; w != nil {
		t.Fatalf("закрытая плашка вернулась от дубля: %+v", *w)
	}
	// Событие отодвинулось (остаток больше порога), затем тот же порог: показ законный.
	m.OnDecision(at(4*time.Second), runningDecision(1000), false, nil)
	m.OnDecision(at(5*time.Second), runningDecision(300), false, []int{5})
	if w := m.View(at(5 * time.Second)).Warning; w == nil || w.Threshold != 5 {
		t.Fatalf("после сдвига события порог не показан: %+v", w)
	}

	f := NewModel()
	in := Incoming{Threshold: 15, Text: "Через 15 минут начнутся запретные часы", Event: ipc.WarnEventForbidden, EventS: 900}
	f.OnWarnings(t0, runningDecision(5000), false, []Incoming{in})
	first := f.View(t0).Warning
	in.EventS = 899
	f.OnWarnings(at(time.Second), runningDecision(5000), false, []Incoming{in})
	if w := f.View(at(time.Second)).Warning; w == nil || w.ID != first.ID {
		t.Fatalf("дубль порога запретных часов дал новую плашку: %+v", w)
	}
}

// Ловит: звук играет не один раз на плашку (при перерисовке, на «Добавлено»), не пишется в
// GuardState в тестовом режиме, отказ звука ломает показ.
func TestControllerSoundOncePerPlate(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	var playedN atomic.Int32
	r.ctl.SetSoundPlayer(func() bool { playedN.Add(1); return false })
	played := func() int { return waitPlayed(&playedN) }
	r.svc.status = ctRunningStatus(1800)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(30)}
	if v := r.poll(); v.Warning == nil || v.Warning.Board == nil {
		t.Fatalf("табло не показано при отказе звука: %+v", v.Warning)
	}
	r.ctl.View()
	r.clk.Advance(time.Second)
	r.svc.status = ctRunningStatus(1799)
	r.poll()
	if played() != 1 {
		t.Fatalf("звук сыгран %d раз за одну плашку", played())
	}
	r.clk.Advance(time.Second)
	r.svc.status = ctRunningStatus(3600)
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdNotice, Text: "Добавлено 30 минут"}}
	if v := r.poll(); v.Warning == nil || v.Warning.Text != "Добавлено 30 минут" {
		t.Fatalf("«Добавлено» не показано: %+v", v.Warning)
	}
	if played() != 1 {
		t.Fatalf("звук на «Добавлено»: %d", played())
	}
	r.clk.Advance(time.Second)
	r.svc.status = ctRunningStatus(60)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
	r.poll()
	if waitPlayedN(&playedN, 2) != 2 {
		t.Fatalf("звук за 1 мин не сыгран: %d", playedN.Load())
	}
	r.ctl.mu.Lock()
	st := StateFromView(r.ctl.last, r.ctl.lastLock, r.ctl.shown, r.ctl.sounds, r.ctl.fields)
	r.ctl.mu.Unlock()
	if len(st.Sounds) != 2 || st.Sounds[0].Threshold != 30 || st.Sounds[1].Threshold != 1 || st.Sounds[0].OK {
		t.Fatalf("журнал звуков %+v", st.Sounds)
	}
	if len(st.Warnings) != 2 || st.Warnings[0].Threshold != 30 || st.Warnings[1].Threshold != 1 {
		t.Fatalf("журнал предупреждений %+v", st.Warnings)
	}
}

// Ловит: крестик табло не виден приёмке (кнопки нет в GuardState) или не шлёт подтверждение
// с порогом.
func TestControllerCloseButtonAcks(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctRunningStatus(900)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(15)}
	v := r.poll()
	st := StateFromView(v, "", nil, nil, nil)
	if len(st.Buttons) != 1 || st.Buttons[0] != ipc.ButtonWarningClose {
		t.Fatalf("кнопки табло в GuardState: %v", st.Buttons)
	}
	r.ctl.Press(ButtonWarningClose, "")
	if v := r.ctl.View(); v.Warning != nil {
		t.Fatal("крестик не закрыл табло")
	}
	r.ctl.PollOnce()
	var req ipc.GuardPollRequest
	if err := r.svc.called(ipc.CmdGuardPoll)[1].Into(&req); err != nil {
		t.Fatal(err)
	}
	ack := 0
	for _, e := range req.Events {
		if e.Kind == ipc.GuardEvWarningAck && e.Threshold == 15 {
			ack++
		}
	}
	if ack != 1 {
		t.Fatalf("подтверждений %d, события %+v", ack, req.Events)
	}
}

// Ловит (CI 40751015930, S2 и S35): табло «за 5» ещё видно, через 4 с приходит порог 1 — плашка 1 мин
// должна сменить табло (без крестика, с «Попросить ещё» при пустой копилке, новый ID) и звук за 1
// играет один раз.
func TestBoard5ReplacedByOneMinute(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	var playedN atomic.Int32
	r.ctl.SetSoundPlayer(func() bool { playedN.Add(1); return true })
	r.svc.status = ctRunningStatus(300)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(5)}
	five := r.poll().Warning
	for i := 1; i <= 4; i++ {
		r.clk.Advance(time.Second)
		r.svc.status = ctRunningStatus(300 - 60*i)
		if i == 4 {
			r.svc.status = ctRunningStatus(60)
			r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
		}
		r.poll()
	}
	v := r.ctl.View()
	if v.Warning == nil || v.Warning.Threshold != 1 || v.Warning.Board != nil || v.Warning.ID == five.ID || buttonNames(v.Warning) != TargetAskMorePlate {
		t.Fatalf("после порога 1: %+v", v.Warning)
	}
	n := waitPlayedN(&playedN, 2)
	r.ctl.mu.Lock()
	st := StateFromView(r.ctl.last, "", r.ctl.shown, r.ctl.sounds, r.ctl.fields)
	r.ctl.mu.Unlock()
	if strings.Join(st.Buttons, ",") != TargetAskMorePlate || Get(st.Sounds) != "5,1" || n != 2 {
		t.Fatalf("кнопки %v, звуки %v, сыграно %d", st.Buttons, st.Sounds, n)
	}
}

// Get — пороги звуков строкой.
func Get(ss []ipc.ShownSound) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += itoa(s.Threshold)
	}
	return out
}

func itoa(n int) string { return fmt.Sprint(n) }

// Ловит (корень CI 40751015930): PlaySoundW, который не вернулся сразу (раннер без звуковой карты,
// предыдущий звук в памяти), держит поток окон: вид стража не обновляется, плашка 1 мин не
// показывается, табло висит. Звук не должен блокировать View.
func TestSoundDoesNotBlockView(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	block := make(chan struct{})
	defer close(block)
	r.ctl.SetSoundPlayer(func() bool { <-block; return true })
	r.svc.status = ctRunningStatus(300)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(5)}
	r.ctl.PollOnce()
	done := make(chan View, 1)
	go func() { done <- r.ctl.View() }()
	select {
	case v := <-done:
		if v.Warning == nil || v.Warning.Threshold != 5 {
			t.Fatalf("вид %+v", v.Warning)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("View ждёт окончания звука: поток окон заблокирован")
	}
	r.ctl.mu.Lock()
	n := len(r.ctl.sounds)
	r.ctl.mu.Unlock()
	if n != 1 {
		t.Fatalf("звук не записан в журнал до окончания: %d", n)
	}
}

// Ловит: решение «какое окно показать» для табло и плашки 1 мин (run_windows.go) путает их.
func TestSplitPlate(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(300), false, []int{5})
	b, x := SplitPlate(m.View(t0).Warning)
	if b == nil || x != nil {
		t.Fatalf("табло: %v %v", b, x)
	}
	m.OnDecision(at(4*time.Second), runningDecision(60), false, []int{1})
	b, x = SplitPlate(m.View(at(4 * time.Second)).Warning)
	if b != nil || x == nil || x.Threshold != 1 {
		t.Fatalf("плашка 1 мин: %v %v", b, x)
	}
	if b, x = SplitPlate(nil); b != nil || x != nil {
		t.Fatal("нет плашки")
	}
}

// waitPlayedN ждёт (до 2 с), пока исполнитель звука сыграет want звуков; возвращает, сколько сыграно.
func waitPlayedN(n *atomic.Int32, want int32) int {
	for end := time.Now().Add(2 * time.Second); n.Load() < want && time.Now().Before(end); {
		time.Sleep(5 * time.Millisecond)
	}
	return int(n.Load())
}

// waitPlayed даёт исполнителю звука доиграть очередь и возвращает, сколько сыграно.
func waitPlayed(n *atomic.Int32) int {
	time.Sleep(50 * time.Millisecond)
	return int(n.Load())
}

// lastPushed — последний вид, отправленный службе (GuardState).
func lastPushed(t *testing.T, r *ctRig) ipc.GuardStateReply {
	t.Helper()
	calls := r.svc.called(ipc.CmdGuardState)
	if len(calls) == 0 {
		t.Fatal("вид не отправлен")
	}
	var st ipc.GuardStateReply
	if err := calls[len(calls)-1].Into(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

// Ловит (корень CI 40751015930 и 40754071886, S2 и S35): горутина опроса после PollOnce сразу
// отправляет GuardState (pollLoop), а поток окон ещё не перерисовал вид. Журнал предупреждений уже
// с порогом 1, а кнопки и звуки от прошлого кадра (табло «5» с крестиком, звука за 1 нет): приёмка
// видит несогласованный снимок. Порядок здесь тот же, что в pollLoop: PollOnce, PushState, кадра
// окна между ними нет.
func TestPushStateConsistentWithoutWindowFrame(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	r.ctl.SetSoundPlayer(func() bool { return true })
	r.svc.status = ctRunningStatus(300)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(5)}
	r.ctl.PollOnce()
	r.ctl.View() // кадр окна: табло «5»
	r.ctl.PushState()
	r.clk.Advance(4 * time.Second)
	r.svc.status = ctRunningStatus(60)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
	r.ctl.PollOnce()
	r.ctl.PushState() // кадра окна после опроса ещё не было
	st := lastPushed(t, r)
	thr := ""
	for i, w := range st.Warnings {
		if i > 0 {
			thr += ","
		}
		thr += itoa(w.Threshold)
	}
	if thr != "5,1" || strings.Join(st.Buttons, ",") != TargetAskMorePlate || Get(st.Sounds) != "5,1" {
		t.Fatalf("несогласованный снимок: предупреждения %s, кнопки %v, звуки %v", thr, st.Buttons, st.Sounds)
	}
}

// Ловит: GuardState не говорит, жив ли поток окон и была ли паника (приёмке нечем отличить
// «стоит» от «падает»).
func TestPushStateCarriesUIDiag(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	r.svc.status = ctRunningStatus(600)
	r.ctl.PollOnce()
	r.ctl.NoteUITick(t0.Add(time.Second))
	r.ctl.NoteUITick(t0.Add(2 * time.Second))
	// lastPanic общая на процесс: после теста вернуть пустую, иначе «проба» увидят другие тесты
	// пакета (TestBoardThenOneMinuteRealWindows на Windows проверяет, что паник нет).
	t.Cleanup(func() { lastPanic.Store("") })
	safely(func() { panic("проба") })
	r.ctl.PushState()
	st := lastPushed(t, r)
	if st.UI.Ticks != 2 || st.UI.TickMs != t0.Add(2*time.Second).UnixMilli() || st.UI.Panic != "проба" {
		t.Fatalf("диагностика %+v", st.UI)
	}
}

// Ловит (снимок s35-added в CI был копией плашки 1 мин): приёмка не видит, какой текст сейчас на
// плашке, и снимает «Добавлено 30 минут», пока на экране ещё «Осталась 1 минута». GuardState
// называет текст видимой плашки; плашки нет — пусто.
func TestPushStateNamesVisiblePlateText(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	r.svc.status = ctRunningStatus(60)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
	r.ctl.PollOnce()
	r.ctl.PushState()
	if got := lastPushed(t, r).Plate; got != "Осталась 1 минута: 1:00" {
		t.Fatalf("плашка %q; ждали «Осталась 1 минута: 1:00»", got)
	}
	r.clk.Advance(time.Second)
	r.svc.status = ctRunningStatus(1860)
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdNotice, Text: "Добавлено 30 минут"}}
	r.ctl.PollOnce()
	r.ctl.PushState()
	if got := lastPushed(t, r).Plate; got != "Добавлено 30 минут" {
		t.Fatalf("плашка %q; ждали «Добавлено 30 минут»", got)
	}
	r.clk.Advance(6 * time.Second)
	r.ctl.PollOnce()
	r.ctl.PushState()
	if got := lastPushed(t, r).Plate; got != "" {
		t.Fatalf("через 6 с плашка %q; ждали пусто", got)
	}
}
