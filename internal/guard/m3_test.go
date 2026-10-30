package guard

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Тесты стража вехи M3: путь переднего плана, плашки о закрытых программах, кнопки плашки 1 мин,
// игровые предупреждения (R3, R5, R9, R12, FR-13, RM3-4, RM3-5).

const realGame = `C:\Users\Миша\Downloads\SuperGame.exe`

// m3Rig — контроллер на поддельной службе, у которого датчики видят на переднем плане realGame.
func m3Rig(test bool) *ctRig {
	r := newCtRig()
	r.svc.test = test
	read := func(now time.Time) ipc.SensorSample {
		return BuildSample(now, RawSensors{IdleMs: 1500, ForegroundPath: realGame}, 1)
	}
	r.ctl = NewController(NewModel(), r.svc, r.clk.Now, read, nil)
	return r
}

// lastSample — снимок датчиков из последнего GuardPoll.
func (r *ctRig) lastSample(t *testing.T) ipc.SensorSample {
	t.Helper()
	calls := r.svc.called(ipc.CmdGuardPoll)
	if len(calls) == 0 {
		t.Fatal("опросов не было")
	}
	var req ipc.GuardPollRequest
	if err := calls[len(calls)-1].Into(&req); err != nil {
		t.Fatal(err)
	}
	return req.Sample
}

// lastState — последний отправленный GuardState.
func (r *ctRig) lastState(t *testing.T) ipc.GuardStateReply {
	t.Helper()
	r.ctl.PushState()
	calls := r.svc.called(ipc.CmdGuardState)
	if len(calls) == 0 {
		t.Fatal("GuardState не отправлен")
	}
	var st ipc.GuardStateReply
	if err := calls[len(calls)-1].Into(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func foregroundCmd(path string) ipc.GuardCommand {
	return ipc.GuardCommand{Kind: ipc.GuardCmdTest, Test: &ipc.TestGuardAction{Action: ipc.TestForeground, Text: path}}
}

// Ловит (RM3-8): подмена переднего плана для приёмки не доходит до снимка, держится один опрос,
// не возвращается к настоящему чтению по пустому тексту, уходит окну как действие click или
// работает вне тестового режима; GuardState не показывает, что ушло службе.
func TestForegroundOverrideInTestMode(t *testing.T) {
	const fake = `C:\Users\runneradmin\AppData\Local\Temp\pcb\game.exe`
	r := m3Rig(true)
	r.svc.status = ctRunningStatus(3600)
	r.poll()
	if got := r.lastSample(t).ForegroundPath; got != realGame {
		t.Fatalf("настоящий путь: %q", got)
	}
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd(fake)}
	r.poll()
	if q := r.ctl.QueuedTests(); len(q) != 0 {
		t.Fatalf("подмена ушла окну: %+v", q)
	}
	for i := 0; i < 3; i++ {
		r.poll()
		if got := r.lastSample(t).ForegroundPath; got != fake {
			t.Fatalf("опрос %d после подмены: %q", i, got)
		}
	}
	if st := r.lastState(t); st.ForegroundPath != fake {
		t.Fatalf("GuardState.foregroundPath = %q", st.ForegroundPath)
	}
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd("")}
	r.poll()
	r.poll()
	if got := r.lastSample(t).ForegroundPath; got != realGame {
		t.Fatalf("после пустой подмены: %q", got)
	}
	if st := r.lastState(t); st.ForegroundPath != realGame {
		t.Fatalf("GuardState.foregroundPath после пустой подмены = %q", st.ForegroundPath)
	}

	r = m3Rig(false)
	r.svc.status = ctRunningStatus(3600)
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd(fake)}
	r.poll()
	r.poll()
	if got := r.lastSample(t).ForegroundPath; got != realGame {
		t.Fatalf("вне тестового режима подмена сработала: %q", got)
	}
}

// --- плашки о закрытых программах (FR-13, 8.2.4) ---------------------------------------------

func unknownPlate() ipc.BlockedPlate {
	return ipc.BlockedPlate{Kind: ipc.BlockedUnknown, Text: "Программа «SuperGame» ждёт разрешения родителя",
		ShowS: 15, Path: `c:\users\миша\downloads\supergame.exe`, CanAsk: true}
}

func buttonNames(w *WarningView) string {
	if w == nil {
		return "<нет плашки>"
	}
	var out []string
	for _, b := range w.Buttons {
		out = append(out, TargetName(b.Button))
	}
	return strings.Join(out, ",")
}

// Ловит: плашка о закрытой программе не показывается, живёт не свой срок (ShowS, 8 с по
// умолчанию, 10 с у решения родителя), играет звук, кнопка «Попросить сейчас» есть не у
// «ждёт разрешения» или есть без CanAsk.
func TestBlockedPlateShowsForItsTime(t *testing.T) {
	cases := []struct {
		name    string
		plate   ipc.BlockedPlate
		life    time.Duration
		buttons string
	}{
		{"ждёт разрешения", unknownPlate(), 15 * time.Second, TargetAskNow + "," + TargetPlateClose},
		{"ждёт разрешения без CanAsk", ipc.BlockedPlate{Kind: ipc.BlockedUnknown, Text: "Программа «X» ждёт разрешения родителя", Path: `c:\x.exe`}, 8 * time.Second, TargetPlateClose},
		{"запрещена", ipc.BlockedPlate{Kind: ipc.BlockedForbidden, Text: "Программа «X» запрещена родителем", ShowS: 10, Path: `c:\x.exe`, CanAsk: true}, 10 * time.Second, TargetPlateClose},
		{"игры кончились, срок по умолчанию", ipc.BlockedPlate{Kind: ipc.BlockedGamesOver, Text: "Время на игры на сегодня закончилось", Path: `c:\g.exe`}, 8 * time.Second, TargetPlateClose},
		{"решение родителя, срок по умолчанию", ipc.BlockedPlate{Kind: ipc.BlockedDecided, Text: "Родитель разрешил «X»: это игра. Запусти её снова", Path: `c:\x.exe`}, 10 * time.Second, TargetPlateClose},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel()
			d := runningDecision(3600)
			m.OnDecision(t0, d, false, nil)
			m.OnBlocked(t0, c.plate)
			w := m.View(t0).Warning
			if w == nil || w.Text != c.plate.Text || w.Kind != c.plate.Kind || w.Sound || w.Board != nil || w.Threshold != 0 {
				t.Fatalf("плашка: %+v", w)
			}
			if got := buttonNames(w); got != c.buttons {
				t.Fatalf("кнопки %q, ждали %q", got, c.buttons)
			}
			keepAlive(m, d, 1, int(c.life/time.Second)-1)
			if w := m.View(at(c.life - time.Millisecond)).Warning; w == nil {
				t.Fatal("плашка закрылась раньше срока")
			}
			if w := m.View(at(c.life)).Warning; w != nil {
				t.Fatalf("плашка живёт дольше срока: %+v", *w)
			}
		})
	}
}

// Ловит (класс A, PL-13): тот же GuardCmdBlocked повторно даёт второй показ (новый ID) или
// продлевает срок; новая плашка той же программы с другим текстом не сменяет старую; плашка
// другой программы не заменяет прежнюю (одна плашка за раз).
func TestBlockedPlateDuplicatesAndReplacement(t *testing.T) {
	m := NewModel()
	d := runningDecision(3600)
	m.OnDecision(t0, d, false, nil)
	m.OnBlocked(t0, unknownPlate())
	first := m.View(t0).Warning
	keepAlive(m, d, 1, 5)
	m.OnBlocked(at(5*time.Second), unknownPlate())
	if w := m.View(at(5 * time.Second)).Warning; w == nil || w.ID != first.ID {
		t.Fatalf("дубль дал новую плашку: %+v -> %+v", first, w)
	}
	keepAlive(m, d, 6, 15)
	if w := m.View(at(15 * time.Second)).Warning; w != nil {
		t.Fatalf("дубль продлил срок: %+v", *w)
	}

	m.OnBlocked(at(16*time.Second), unknownPlate())
	again := m.View(at(16 * time.Second)).Warning
	if again == nil || again.ID == first.ID {
		t.Fatalf("после срока та же плашка не показана снова: %+v", again)
	}
	decided := ipc.BlockedPlate{Kind: ipc.BlockedDecided, Text: "Родитель разрешил «SuperGame»: это игра. Запусти её снова", ShowS: 10, Path: unknownPlate().Path}
	m.OnBlocked(at(17*time.Second), decided)
	if w := m.View(at(17 * time.Second)).Warning; w == nil || w.Text != decided.Text || buttonNames(w) != TargetPlateClose {
		t.Fatalf("решение родителя не сменило плашку той же программы: %+v", w)
	}
	other := ipc.BlockedPlate{Kind: ipc.BlockedForbidden, Text: "Программа «Y» запрещена родителем", ShowS: 10, Path: `c:\y.exe`}
	m.OnBlocked(at(18*time.Second), other)
	if w := m.View(at(18 * time.Second)).Warning; w == nil || w.Text != other.Text {
		t.Fatalf("плашка другой программы: %+v", w)
	}
}

// Ловит: плашка о программе видна поверх экрана блокировки или в родительской сессии; пустой
// текст рисует пустую плашку; экран блокировки не убирает её.
func TestBlockedPlateNotOverLockOrParent(t *testing.T) {
	m := locked(timeUpDecision())
	m.OnBlocked(t0, unknownPlate())
	if v := m.View(t0); v.Warning != nil {
		t.Fatalf("плашка поверх экрана блокировки: %+v", *v.Warning)
	}
	m.OnDecision(at(time.Second), runningDecision(600), false, nil)
	if w := m.View(at(time.Second)).Warning; w != nil {
		t.Fatalf("после снятия экрана всплыла плашка, пришедшая под ним: %+v", *w)
	}

	p := NewModel()
	p.OnDecision(t0, runningDecision(600), true, nil)
	p.OnBlocked(t0, unknownPlate())
	p.OnDecision(at(time.Second), runningDecision(600), false, nil)
	if w := p.View(at(time.Second)).Warning; w != nil {
		t.Fatalf("плашка из родительской сессии: %+v", *w)
	}

	e := NewModel()
	e.OnDecision(t0, runningDecision(600), false, nil)
	e.OnBlocked(t0, ipc.BlockedPlate{Kind: ipc.BlockedForbidden, Text: "  ", Path: `c:\x.exe`})
	if w := e.View(t0).Warning; w != nil {
		t.Fatalf("плашка без текста: %+v", *w)
	}

	s := NewModel()
	s.OnDecision(t0, runningDecision(600), false, nil)
	s.OnBlocked(t0, unknownPlate())
	s.OnDecision(at(time.Second), timeUpDecision(), false, nil)
	s.OnDecision(at(2*time.Second), runningDecision(600), false, nil)
	if w := s.View(at(2 * time.Second)).Warning; w != nil {
		t.Fatalf("экран блокировки не убрал плашку: %+v", *w)
	}
}

// Ловит (RM3-4): «Попросить сейчас» не открывает окно родителя на «Программах», не закрывает
// плашку, срабатывает на плашке без кнопки; крестик не закрывает плашку о программе.
func TestBlockedPlateButtons(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(3600), false, nil)
	m.OnBlocked(t0, unknownPlate())
	if a := m.Press(t0, ButtonAskNow, ""); a.Kind != ActOpenPrograms {
		t.Fatalf("«Попросить сейчас»: %+v", a)
	}
	if w := m.View(t0).Warning; w != nil {
		t.Fatalf("после «Попросить сейчас» плашка осталась: %+v", *w)
	}
	if a := m.Press(t0, ButtonAskNow, ""); a.Kind != ActNone {
		t.Fatalf("второе нажатие без плашки: %+v", a)
	}

	m.OnBlocked(at(time.Second), ipc.BlockedPlate{Kind: ipc.BlockedForbidden, Text: "Программа «X» запрещена родителем", Path: `c:\x.exe`, CanAsk: true})
	if a := m.Press(at(time.Second), ButtonAskNow, ""); a.Kind != ActNone {
		t.Fatalf("«Попросить сейчас» у запрещённой: %+v", a)
	}
	if a := m.Press(at(time.Second), ButtonWarningClose, ""); a.Kind != ActNone || m.View(at(time.Second)).Warning == nil {
		t.Fatal("крестик табло закрыл плашку о программе")
	}
	if a := m.Press(at(time.Second), ButtonPlateClose, ""); a.Kind != ActNone {
		t.Fatalf("крестик: %+v", a)
	}
	if w := m.View(at(time.Second)).Warning; w != nil {
		t.Fatalf("крестик не закрыл плашку: %+v", *w)
	}
}

// Ловит: плашка о программе навсегда прячет плашку 1 мин (та не вернулась после срока), табло
// закрывается крестиком плашки о программе, новый порог не показывается поверх.
func TestBlockedPlateAndWarningShareCorner(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(60), false, []int{1})
	one := m.View(t0).Warning
	m.OnBlocked(t0, ipc.BlockedPlate{Kind: ipc.BlockedForbidden, Text: "Программа «X» запрещена родителем", ShowS: 3, Path: `c:\x.exe`})
	if w := m.View(t0).Warning; w == nil || w.Kind != ipc.BlockedForbidden {
		t.Fatalf("новая плашка о программе не сверху: %+v", w)
	}
	feed(m, 1, 3, 60)
	w := m.View(at(3 * time.Second)).Warning
	if w == nil || w.ID != one.ID || w.Countdown != "0:57" {
		t.Fatalf("после плашки о программе плашка 1 мин не вернулась: %+v", w)
	}
	if SoundDue(one.ID, w) {
		t.Fatal("вернувшаяся плашка 1 мин сыграла бы звук ещё раз")
	}

	// Новый порог после плашки о программе — сверху (новее).
	n := NewModel()
	n.OnDecision(t0, runningDecision(310), false, nil)
	n.OnBlocked(t0, unknownPlate())
	n.OnDecision(at(time.Second), runningDecision(300), false, []int{5})
	if w := n.View(at(time.Second)).Warning; w == nil || w.Kind != ipc.PlateBoard {
		t.Fatalf("табло 5 после плашки о программе: %+v", w)
	}

	// Контроллер: вернувшаяся плашка 1 мин не пишется показанной второй раз и звук не повторяет.
	r := newCtRig()
	r.svc.test = true
	r.ctl.SetSoundPlayer(func() bool { return true })
	r.svc.status = ctRunningStatus(60)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
	r.poll()
	p := ipc.BlockedPlate{Kind: ipc.BlockedForbidden, Text: "Программа «X» запрещена родителем", ShowS: 2, Path: `c:\x.exe`}
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &p}}
	r.poll()
	for i := 0; i < 3; i++ {
		r.clk.Advance(time.Second)
		r.poll()
	}
	st := r.lastState(t)
	if st.PlateKind != ipc.PlateOneMinute || len(st.Warnings) != 1 || len(st.Sounds) != 1 {
		t.Fatalf("после плашки о программе: вид %q, показано %+v, звуки %+v", st.PlateKind, st.Warnings, st.Sounds)
	}
}

// Ловит: страж заводит плашку о программе без команды службы (ClassifyCommands теряет Plate)
// или запоминает команду без плашки.
func TestClassifyBlockedCommands(t *testing.T) {
	p := unknownPlate()
	got := ClassifyCommands([]ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &p}, {Kind: ipc.GuardCmdBlocked}})
	if len(got.Blocked) != 1 || got.Blocked[0] != p {
		t.Fatalf("плашки: %+v", got.Blocked)
	}
}

// Ловит (RM3-4, Review Focus 6): «Попросить сейчас» не доходит до службы как OpenWindow
// {parent, programs}, шлёт пароль, пишет событие обхода; отказ службы молча теряется.
func TestControllerAskNowOpensPrograms(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	r.svc.status = ctRunningStatus(3600)
	p := unknownPlate()
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &p}}
	r.poll()
	st := r.lastState(t)
	if st.Plate != p.Text || st.PlateKind != ipc.BlockedUnknown || strings.Join(st.Buttons, ",") != TargetAskNow+","+TargetPlateClose {
		t.Fatalf("GuardState: %+v", st)
	}
	act := r.ctl.Press(ButtonAskNow, "")
	if act.Kind != ActOpenPrograms {
		t.Fatalf("действие %+v", act)
	}
	if again := r.ctl.Press(ButtonAskNow, ""); again.Kind != ActNone {
		t.Fatalf("второе нажатие, пока команда идёт: %+v", again)
	}
	r.ctl.Run(act)
	calls := r.svc.called(ipc.CmdOpenWindow)
	var req ipc.OpenWindowRequest
	if len(calls) != 1 || calls[0].Into(&req) != nil || req != (ipc.OpenWindowRequest{Kind: ipc.WindowParent, Screen: ipc.ScreenPrograms}) || calls[0].Token != "" {
		t.Fatalf("OpenWindow: %+v (%+v)", calls, req)
	}
	r.poll()
	var req2 ipc.GuardPollRequest
	polls := r.svc.called(ipc.CmdGuardPoll)
	if err := polls[len(polls)-1].Into(&req2); err != nil {
		t.Fatal(err)
	}
	for _, e := range req2.Events {
		if e.Kind == ipc.GuardEvBypass || e.Kind == ipc.GuardEvWarningAck {
			t.Fatalf("лишнее событие: %+v", e)
		}
	}

	r.svc.reply[ipc.CmdOpenWindow] = func(ipc.Msg) ipc.Reply {
		return ipc.Fail("Не получилось открыть окно, попробуйте ещё раз")
	}
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &p}}
	r.clk.Advance(20 * time.Second)
	r.poll()
	r.ctl.Run(r.ctl.Press(ButtonAskNow, ""))
	if v := r.ctl.View(); v.Warning == nil || v.Warning.Kind != ipc.PlateNotice || v.Warning.Text != "Не получилось открыть окно, попробуйте ещё раз" {
		t.Fatalf("отказ службы не виден ребёнку: %+v", v.Warning)
	}
}

// --- кнопки плашки 1 мин (B-27, RM3-5, ТЗ 8.2.2 п.4) -----------------------------------------

func withBonus(d core.Decision, balance int) core.Decision {
	d.BonusBalance = balance
	return d
}

// Ловит: кнопки плашки 1 мин видны при пустой копилке или в запретный интервал (FR-11), нет
// «Попросить ещё», плашка закрывается кнопкой или сроком (D-55).
func TestOneMinutePlateButtons(t *testing.T) {
	cases := []struct {
		name string
		d    core.Decision
		ev   string
		want string
	}{
		{"копилка пуста", runningDecision(60), ipc.WarnEventLimit, TargetAskMorePlate},
		{"в копилке 45 мин", withBonus(runningDecision(60), 45), ipc.WarnEventLimit, TargetSpendBonusPlate + "," + TargetAskMorePlate},
		{"перед запретными часами копилка не поможет", withBonus(runningDecision(5000), 45), ipc.WarnEventForbidden, TargetAskMorePlate},
		{"игры", withBonus(runningDecision(5000), 10), ipc.WarnEventGames, TargetSpendBonusPlate + "," + TargetAskMorePlate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := c.d
			d.GamesRemainingS, d.GamesLimitS = 60, 3600
			m := NewModel()
			m.OnWarnings(t0, d, false, []Incoming{{Threshold: 1, Event: c.ev, EventS: 60}})
			w := m.View(t0).Warning
			if got := buttonNames(w); got != c.want {
				t.Fatalf("кнопки %q, ждали %q", got, c.want)
			}
			m.Press(t0, ButtonWarningClose, "")
			m.Press(t0, ButtonPlateClose, "")
			keepAlive(m, d, 1, 30)
			if w := m.View(at(30 * time.Second)).Warning; w == nil || w.Threshold != 1 {
				t.Fatalf("плашка 1 мин закрылась: %+v", w)
			}
		})
	}
	for _, s := range []core.Status{core.StatusForbidden} {
		d := withBonus(runningDecision(60), 45)
		d.Status = s
		m := NewModel()
		m.OnDecision(t0, d, false, []int{1})
		if got := buttonNames(m.View(t0).Warning); got != TargetAskMorePlate {
			t.Fatalf("статус %v: кнопки %q", s, got)
		}
	}
}

// Ловит (RM3-5): «Взять из копилки» на плашке берёт не 30 мин (или весь остаток, если меньше),
// требует экрана блокировки, работает без кнопки; «Попросить ещё» на плашке открывает панель
// пароля вместо «Моей статистики».
func TestOneMinutePlatePress(t *testing.T) {
	for _, c := range []struct{ balance, want int }{{45, 30}, {30, 30}, {12, 12}} {
		m := NewModel()
		m.OnDecision(t0, withBonus(runningDecision(60), c.balance), false, []int{1})
		if a := m.Press(t0, ButtonPlateSpendBonus, ""); a.Kind != ActSpendBonus || a.Minutes != c.want || !a.Plate {
			t.Fatalf("копилка %d: %+v, ждали %d мин", c.balance, a, c.want)
		}
	}
	m := NewModel()
	m.OnDecision(t0, runningDecision(60), false, []int{1})
	if a := m.Press(t0, ButtonPlateSpendBonus, ""); a.Kind != ActNone {
		t.Fatalf("копилка пуста, а трата ушла: %+v", a)
	}
	if a := m.Press(t0, ButtonPlateAskMore, ""); a.Kind != ActOpenMyStats {
		t.Fatalf("«Попросить ещё»: %+v", a)
	}
	if v := m.View(t0); v.Panel != PanelNone || v.Warning == nil || v.Warning.Threshold != 1 {
		t.Fatalf("после «Попросить ещё» плашка 1 мин должна остаться: %+v", v)
	}
	// Табло 5 мин: кнопок копилки нет.
	b := NewModel()
	b.OnDecision(t0, withBonus(runningDecision(300), 45), false, []int{5})
	if a := b.Press(t0, ButtonPlateSpendBonus, ""); a.Kind != ActNone {
		t.Fatalf("табло 5 мин: %+v", a)
	}
	// Кнопки экрана блокировки не работают на плашке и наоборот.
	if a := m.Press(t0, ButtonSpendBonus, ""); a.Kind != ActNone {
		t.Fatalf("кнопка экрана на плашке: %+v", a)
	}
	l := locked(withBonus(timeUpDecision(), 45))
	if a := l.Press(t0, ButtonPlateSpendBonus, ""); a.Kind != ActNone {
		t.Fatalf("кнопка плашки на экране: %+v", a)
	}
}

// Ловит (RM3-5, Review Focus 5, 6): «Взять из копилки» на плашке не доходит до службы без пароля,
// плашка не сменяется на «Добавлено N минут»; «Попросить ещё» не открывает «Мою статистику» или
// просит пароль; отказ службы не виден ребёнку.
func TestControllerOneMinuteButtons(t *testing.T) {
	r := newCtRig()
	st := ctRunningStatus(60)
	st.BonusBalance = 45 // баланс копилки служба кладёт в статус сама
	r.svc.status = st
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
	r.poll()
	act := r.ctl.Press(ButtonPlateSpendBonus, "")
	r.ctl.Run(act)
	calls := r.svc.called(ipc.CmdSpendBonus)
	var req ipc.SpendBonusRequest
	if len(calls) != 1 || calls[0].Into(&req) != nil || req.Minutes != 30 || calls[0].Token != "" {
		t.Fatalf("SpendBonus: %+v %+v", calls, req)
	}
	if w := r.ctl.View().Warning; w == nil || w.Kind != ipc.PlateAdded || w.Text != "Добавлено 30 минут" {
		t.Fatalf("после копилки: %+v", w)
	}

	r = newCtRig()
	r.svc.status = ctRunningStatus(60)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
	r.poll()
	r.ctl.Run(r.ctl.Press(ButtonPlateAskMore, ""))
	ow := r.svc.called(ipc.CmdOpenWindow)
	var oreq ipc.OpenWindowRequest
	if len(ow) != 1 || ow[0].Into(&oreq) != nil || oreq != (ipc.OpenWindowRequest{Kind: ipc.WindowMyStats}) || len(r.svc.called(ipc.CmdLogin)) != 0 {
		t.Fatalf("«Попросить ещё»: %+v %+v", ow, oreq)
	}

	r = newCtRig()
	r.svc.status = st
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(1)}
	r.svc.reply[ipc.CmdSpendBonus] = func(ipc.Msg) ipc.Reply {
		return ipc.FailField("Из копилки можно взять с 07:00", ipc.FieldMinutes)
	}
	r.poll()
	r.ctl.Run(r.ctl.Press(ButtonPlateSpendBonus, ""))
	if w := r.ctl.View().Warning; w == nil || w.Kind != ipc.PlateNotice || w.Text != "Из копилки можно взять с 07:00" {
		t.Fatalf("отказ копилки: %+v", w)
	}
}

// --- игровые предупреждения (R9, G5) ---------------------------------------------------------

func gamesDecision(remainingS, gamesS int) core.Decision {
	d := runningDecision(remainingS)
	d.GamesRemainingS, d.GamesLimitS = gamesS, 3600
	return d
}

func gamesWarn(thr, eventS int) Incoming {
	w := ipc.NewGuardWarningFor(thr, core.WarnGames, eventS)
	return Incoming{Threshold: w.Threshold, Text: w.Text, Event: w.Event, EventS: w.EventS}
}

// Ловит: табло игр считает общий остаток вместо остатка игр, заголовок общий, звука нет; плашка
// 1 мин игр подменяет текст службы общим «Осталась 1 минута».
func TestGamesBoardCountsGames(t *testing.T) {
	m := NewModel()
	d := gamesDecision(5000, 300)
	m.OnWarnings(t0, d, false, []Incoming{gamesWarn(5, 300)})
	w := m.View(t0).Warning
	if w == nil || hms(w) != "00 : 05 : 00" || !w.Sound || w.Kind != ipc.PlateBoard ||
		w.Board.Title != "ОСТАЛОСЬ НА ИГРЫ:" || w.Text != "Сохрани игру: через 5 минут время на игры закончится" {
		t.Fatalf("табло игр: %+v %+v", w, w.Board)
	}
	m.OnWarnings(at(2*time.Second), gamesDecision(4998, 298), false, nil)
	if got := hms(m.View(at(2 * time.Second)).Warning); got != "00 : 04 : 58" {
		t.Fatalf("табло игр через 2 с: %q", got)
	}

	one := NewModel()
	one.OnWarnings(t0, gamesDecision(5000, 60), false, []Incoming{gamesWarn(1, 60)})
	one.OnWarnings(at(time.Second), gamesDecision(4999, 59), false, nil)
	if w := one.View(at(time.Second)).Warning; w == nil || w.Text != "Осталась 1 минута на игры: 0:59" {
		t.Fatalf("плашка 1 мин игр: %+v", w)
	}
	for s := 2; s <= 60; s++ {
		one.OnWarnings(at(time.Duration(s)*time.Second), gamesDecision(5000-s, max(0, 60-s)), false, nil)
	}
	if w := one.View(at(60 * time.Second)).Warning; w == nil || w.Countdown != "0:00" {
		t.Fatalf("на нуле игр до закрытия плашка 1 мин должна быть: %+v", w)
	}
	// Игры кончились, экрана нет (остальное работает): плашка 1 мин снимается.
	over := gamesDecision(4940, 0)
	over.CloseGames = true
	one.OnWarnings(at(61*time.Second), over, false, nil)
	if w := one.View(at(61 * time.Second)).Warning; w != nil {
		t.Fatalf("игры кончились, а плашка 1 мин висит: %+v", *w)
	}
	// Без текста службы — свой текст про игры.
	f := NewModel()
	f.OnWarnings(t0, gamesDecision(5000, 900), false, []Incoming{{Threshold: 15, Event: ipc.WarnEventGames, EventS: 900}})
	if w := f.View(t0).Warning; w == nil || w.Text != "Через 15 минут время на игры закончится" {
		t.Fatalf("запасной текст игр: %+v", w)
	}
}

// Ловит (Review Focus 1 плана): два порога в одном опросе дают две плашки или не тот (нужен
// меньший остаток, при равных — общий); повтор порога игр при следующем опросе даёт второй показ
// и звук (PL-14, seenPlate); сдвиг остатка игр (доп. время) не взводит порог снова.
func TestGamesAndLimitInOnePoll(t *testing.T) {
	limit5 := Incoming{Threshold: 5, Text: "Через 5 минут время закончится. Сохрани игру", Event: ipc.WarnEventLimit, EventS: 300}
	cases := []struct {
		name string
		d    core.Decision
		in   []Incoming
		want string
	}{
		{"игры 5, общий 15", gamesDecision(900, 300), []Incoming{{Threshold: 15, Event: ipc.WarnEventLimit, EventS: 900}, gamesWarn(5, 300)}, ipc.WarnEventGames},
		{"игры 15, общий 5", gamesDecision(300, 900), []Incoming{gamesWarn(15, 900), limit5}, ipc.WarnEventLimit},
		{"поровну 5: меньший остаток игр", gamesDecision(300, 290), []Incoming{limit5, gamesWarn(5, 290)}, ipc.WarnEventGames},
		{"поровну 5 и 300 с: общий", gamesDecision(300, 300), []Incoming{gamesWarn(5, 300), limit5}, ipc.WarnEventLimit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel()
			m.OnWarnings(t0, c.d, false, c.in)
			w := m.View(t0).Warning
			title := "ОСТАЛОСЬ ВРЕМЕНИ:"
			if c.want == ipc.WarnEventGames {
				title = "ОСТАЛОСЬ НА ИГРЫ:"
			}
			if w == nil || w.Board == nil || w.Board.Title != title {
				t.Fatalf("плашка %+v", w)
			}
		})
	}

	r := newCtRig()
	r.svc.test = true
	r.ctl.SetSoundPlayer(func() bool { return true })
	r.svc.status = ipc.ChildStatusFrom("Миша", gamesDecision(5000, 300))
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarningFor(5, core.WarnGames, 300)}
	r.poll()
	r.clk.Advance(time.Second)
	r.svc.status = ipc.ChildStatusFrom("Миша", gamesDecision(4999, 299))
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarningFor(5, core.WarnGames, 299)}
	r.poll()
	st := r.lastState(t)
	if len(st.Warnings) != 1 || st.Warnings[0].Threshold != 5 || len(st.Sounds) != 1 {
		t.Fatalf("повтор порога игр: показано %+v, звуки %+v", st.Warnings, st.Sounds)
	}
	// Доп. время: остаток игр снова больше порога, потом порог законно показывается снова.
	r.clk.Advance(time.Second)
	r.svc.status = ipc.ChildStatusFrom("Миша", gamesDecision(6800, 2100))
	r.poll()
	r.clk.Advance(time.Second)
	r.svc.status = ipc.ChildStatusFrom("Миша", gamesDecision(5000, 300))
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarningFor(5, core.WarnGames, 300)}
	r.poll()
	if st := r.lastState(t); len(st.Warnings) != 2 {
		t.Fatalf("после сдвига остатка игр порог не показан снова: %+v", st.Warnings)
	}
}
