package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Пн 19.10.2026 и вт 20.10.2026 — константами (грабля «тесты с датами»).
const (
	monday  = "2026-10-19"
	tuesday = "2026-10-20"
)

// at — момент понедельника 19.10.2026 по UTC+3.
func at(h, m, s int) time.Time { return time.Date(2026, 10, 19, h, m, s, 0, msk) }

// rulesMon — лимит min минут в понедельник, остальные дни без лимита.
func rulesMon(min int) Rules {
	var r Rules
	r.Week[0] = LimitOf(min)
	return r
}

// tickLog — что показало ядро за прогон.
type tickLog struct {
	warnings []string // «16:45:00 15»
	lockAt   string   // момент первой блокировки, «» — не было
	lock     Decision // решение в момент первой блокировки
}

// run тикает каждую секунду в [from, to) со снимком s.
func run(e *Engine, c *fakeClock, from, to time.Time, s SessionSample) tickLog {
	log := tickLog{warnings: []string{}}
	for t := from; t.Before(to); t = t.Add(time.Second) {
		c.Set(t)
		d := e.Tick(s)
		if d.Warning != 0 {
			log.warnings = append(log.warnings, fmt.Sprintf("%s %d", t.Format(time.TimeOnly), d.Warning))
		}
		if d.Lock != LockNone && log.lockAt == "" {
			log.lockAt = t.Format(time.TimeOnly)
			log.lock = d
		}
	}
	return log
}

func kinds(evs []Event) []string {
	out := []string{}
	for _, ev := range evs {
		out = append(out, ev.Kind)
	}
	return out
}

// Ловит: предупреждения не в свои моменты (порог «<» вместо «<=»), повторный показ
// порога, блокировка не ровно по исчерпании, счёт секунд под экраном блокировки (G1).
func TestEngineSimpleLimit(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{}, "")
	log := run(e, c, at(15, 0, 0), at(17, 0, 5), activeSample())

	if want := []string{"16:45:00 15", "16:55:00 5", "16:59:00 1"}; !slices.Equal(log.warnings, want) {
		t.Fatalf("предупреждения %v, want %v", log.warnings, want)
	}
	if log.lockAt != "17:00:00" {
		t.Fatalf("блокировка в %q, want 17:00:00", log.lockAt)
	}
	d := log.lock
	if d.Lock != LockTimeUp || d.Status != StatusTimeUp || d.Reason != "Время на сегодня закончилось" ||
		d.Until != "Можно снова: завтра" || d.Used != "Сегодня: 2 ч 00 мин из 2 ч" || d.Detail != "" {
		t.Fatalf("решение при блокировке %+v", d)
	}
	if d.Day != monday || d.RemainingS != 0 || d.LimitS != 7200 || d.UsedS != 7200 || d.Unlimited {
		t.Fatalf("цифры при блокировке: день %s, остаток %d, лимит %d, использовано %d", d.Day, d.RemainingS, d.LimitS, d.UsedS)
	}
	st, detail := e.State()
	if st != (DayState{Day: monday, TotalS: 7200}) || detail != "" {
		t.Fatalf("State() = %+v, %q, want 7200 с и без обхода", st, detail)
	}
}

// Ловит: при входе ближе порога показываются уже прошедшие пороги (Ruling 10, R9),
// а порог, взведённый ровно на границе, не срабатывает.
func TestEngineLateLogin(t *testing.T) {
	cases := []struct {
		name   string
		usedS  int
		login  time.Time
		want   []string
		lockAt string
	}{
		{"вход за 3 мин до конца", 7020, at(16, 57, 0), []string{"16:59:00 1"}, "17:00:00"},
		{"вход за 10 мин до конца", 6600, at(16, 50, 0), []string{"16:55:00 5", "16:59:00 1"}, "17:00:00"},
		{"вход ровно за 15 мин", 6300, at(16, 45, 0), []string{"16:45:00 15", "16:55:00 5", "16:59:00 1"}, "17:00:00"},
		{"вход за 30 с до конца", 7170, at(16, 59, 30), []string{}, "17:00:00"},
	}
	for _, tc := range cases {
		c := &fakeClock{t: tc.login}
		e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: tc.usedS}, "")
		log := run(e, c, tc.login, at(17, 0, 2), activeSample())
		if !slices.Equal(log.warnings, tc.want) || log.lockAt != tc.lockAt {
			t.Errorf("%s: предупреждения %v, блокировка %q; want %v, %q", tc.name, log.warnings, log.lockAt, tc.want, tc.lockAt)
		}
	}
}

// Ловит: без лимита появляются предупреждения или блокировка (R4, Ruling 10).
func TestEngineUnlimited(t *testing.T) {
	c := &fakeClock{t: at(12, 0, 0)}
	e := NewEngine(c, msk, Rules{}, DayState{}, "")
	log := run(e, c, at(12, 0, 0), at(15, 0, 0), activeSample())
	if len(log.warnings) != 0 || log.lockAt != "" {
		t.Fatalf("без лимита: предупреждения %v, блокировка %q", log.warnings, log.lockAt)
	}
	d := e.Current()
	if d.Status != StatusUnlimited || !d.Unlimited || d.LimitS != 0 || d.RemainingS != 0 || d.UsedS != 10800 || d.Used != "Сегодня: 3 ч 00 мин" {
		t.Fatalf("Current() без лимита = %+v", d)
	}
}

// Ловит: лимит 0 не блокирует сразу или «Можно снова» указывает день с лимитом 0 (R4).
func TestEngineZeroLimit(t *testing.T) {
	var r Rules
	for i := range r.Week {
		r.Week[i] = LimitOf(0)
	}
	r.Week[2] = LimitOf(60) // среда
	c := &fakeClock{t: at(10, 0, 0)}
	e := NewEngine(c, msk, r, DayState{}, "")
	log := run(e, c, at(10, 0, 0), at(10, 0, 3), activeSample())
	if log.lockAt != "10:00:00" || len(log.warnings) != 0 {
		t.Fatalf("лимит 0: блокировка %q, предупреждения %v; want 10:00:00 и без предупреждений", log.lockAt, log.warnings)
	}
	if log.lock.Until != "Можно снова: в среду" || log.lock.Used != "Сегодня: 0 мин из 0 мин" {
		t.Fatalf("тексты: %q, %q", log.lock.Until, log.lock.Used)
	}
	if st, _ := e.State(); st.TotalS != 0 {
		t.Fatalf("при лимите 0 засчитано %d с", st.TotalS)
	}
}

// Ловит: при непрочитанных правилах ядро блокирует (в том числе за обход) без выхода,
// не сообщает об этом в журнал или сообщает на каждом тике (R18, NFR-10).
func TestEngineBrokenRulesSafeMode(t *testing.T) {
	c := &fakeClock{t: at(18, 0, 0)}
	e := NewEngine(c, msk, Rules{Broken: true}, DayState{Day: monday, TotalS: 7200}, "Закрыт значок программы")
	d := e.Tick(activeSample())
	if d.Lock != LockNone || d.Status != StatusSafe || !d.Unlimited || d.Warning != 0 {
		t.Fatalf("безопасный режим: %+v", d)
	}
	if got := kinds(d.Events); !slices.Equal(got, []string{EvSafeMode}) {
		t.Fatalf("события %v, want [safe_mode]", got)
	}
	c.Add(time.Second)
	d = e.Tick(activeSample())
	if len(d.Events) != 0 || d.UsedS != 7201 {
		t.Fatalf("второй тик: события %v, использовано %d; want без событий и учёт идёт (7201)", kinds(d.Events), d.UsedS)
	}
	d = e.SetRules(Rules{Broken: true})
	if len(d.Events) != 0 {
		t.Fatalf("повторные битые правила дали события %v", kinds(d.Events))
	}
	d = e.SetRules(rulesMon(120))
	if d.Lock != LockBypass || len(d.Events) != 0 {
		t.Fatalf("правила прочитаны: блокировка %v, события %v; want обход снова действует", d.Lock, kinds(d.Events))
	}
	d = e.ClearBlock()
	if d.Lock != LockTimeUp {
		t.Fatalf("после снятия обхода блокировка %v, want время вышло", d.Lock)
	}
	d = e.SetRules(Rules{Broken: true})
	if d.Lock != LockNone || d.Status != StatusSafe || !slices.Equal(kinds(d.Events), []string{EvSafeMode}) {
		t.Fatalf("правила снова сломались при блокировке: %+v", d)
	}
}

// Ловит: доп. время не снимает блокировку «время вышло», не сдвигает предупреждения
// и блокировку, не пишется в журнал (G7, R10, R9).
func TestEngineAddExtraAfterLock(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{}, "")
	run(e, c, at(15, 0, 0), at(17, 5, 0), activeSample())

	c.Set(at(17, 5, 0))
	d, err := e.AddExtra(30)
	if err != nil {
		t.Fatalf("AddExtra(30): %v", err)
	}
	if d.Lock != LockNone || d.RemainingS != 1800 || d.LimitS != 9000 || d.UsedS != 7200 {
		t.Fatalf("после +30: %+v", d)
	}
	if len(d.Events) != 1 || d.Events[0].Kind != EvExtraTime || d.Events[0].Details["minutes"] != 30 || d.Events[0].Details["text"] != "Доп. время +30 мин" {
		t.Fatalf("события после +30: %+v", d.Events)
	}
	log := run(e, c, at(17, 5, 0), at(17, 35, 5), activeSample())
	if want := []string{"17:20:00 15", "17:30:00 5", "17:34:00 1"}; !slices.Equal(log.warnings, want) {
		t.Fatalf("предупреждения после +30: %v, want %v", log.warnings, want)
	}
	if log.lockAt != "17:35:00" || log.lock.Used != "Сегодня: 2 ч 30 мин из 2 ч 30 мин" {
		t.Fatalf("блокировка %q, %q", log.lockAt, log.lock.Used)
	}
	if st, _ := e.State(); st != (DayState{Day: monday, TotalS: 9000, ExtraMin: 30}) {
		t.Fatalf("State() = %+v, want 150:00 и доп. 30 мин", st)
	}
}

// Ловит: доп. время взводит заново порог, до которого новый остаток уже не дотягивает
// (повторный показ «15 мин» при остатке 11 мин), или теряет непоказанные пороги.
func TestEngineAddExtraRearmsReachableOnly(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{}, "")
	run(e, c, at(15, 0, 0), at(16, 50, 0), activeSample())
	c.Set(at(16, 50, 0))
	if _, err := e.AddExtra(1); err != nil {
		t.Fatal(err)
	}
	log := run(e, c, at(16, 50, 0), at(17, 1, 2), activeSample())
	if want := []string{"16:56:00 5", "17:00:00 1"}; !slices.Equal(log.warnings, want) || log.lockAt != "17:01:00" {
		t.Fatalf("предупреждения %v, блокировка %q; want %v и 17:01:00", log.warnings, log.lockAt, want)
	}
}

// Ловит: доп. время вне 1…240 принимается или портит состояние (R10).
func TestEngineAddExtraRange(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday}, "")
	for _, bad := range []int{0, -1, 241} {
		d, err := e.AddExtra(bad)
		if !errors.Is(err, ErrExtraRange) {
			t.Fatalf("AddExtra(%d): ошибка %v, want ErrExtraRange", bad, err)
		}
		if d.Events == nil || len(d.Events) != 0 || d.LimitS != 7200 {
			t.Fatalf("AddExtra(%d) изменил состояние: %+v", bad, d)
		}
	}
	for _, ok := range []int{1, 240} {
		if _, err := e.AddExtra(ok); err != nil {
			t.Fatalf("AddExtra(%d): %v", ok, err)
		}
	}
	if st, _ := e.State(); st.ExtraMin != 241 {
		t.Fatalf("ExtraMin = %d, want 241", st.ExtraMin)
	}
}

// Ловит: блокировка за обход снимается доп. временем, сменой суток или перезапуском
// службы; секунды под ней засчитываются; снятие не пишется в журнал (R17в, пакет п.9).
func TestEngineBypass(t *testing.T) {
	c := &fakeClock{t: at(23, 59, 50)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 600}, "")
	d := e.MarkBypass("Закрыт значок программы")
	if d.Lock != LockBypass || d.Status != StatusBypass || d.Reason != "Попытка обойти защиту" ||
		d.Detail != "Закрыт значок программы" || d.Until != "Можно снова: когда родитель снимет блокировку" {
		t.Fatalf("MarkBypass: %+v", d)
	}
	if len(d.Events) != 1 || d.Events[0].Kind != EvBypass || d.Events[0].Details["detail"] != "Закрыт значок программы" {
		t.Fatalf("события обхода: %+v", d.Events)
	}
	log := run(e, c, at(23, 59, 50), at(23, 59, 55), activeSample())
	if log.lockAt != "23:59:50" {
		t.Fatalf("блокировка за обход на тике: %q", log.lockAt)
	}
	if st, _ := e.State(); st.TotalS != 600 {
		t.Fatalf("под экраном обхода засчитано: %d с, want 600", st.TotalS)
	}
	if d, _ := e.AddExtra(15); d.Lock != LockBypass {
		t.Fatalf("доп. время сняло блокировку за обход: %+v", d)
	}

	st, detail := e.State()
	restarted := NewEngine(c, msk, rulesMon(120), st, detail)
	if d := restarted.Current(); d.Lock != LockBypass || d.Detail != "Закрыт значок программы" ||
		d.Until != "Можно снова: когда родитель снимет блокировку" {
		t.Fatalf("после перезапуска: %+v", d)
	}

	c.Set(time.Date(2026, 10, 20, 0, 0, 0, 0, msk))
	if d := e.Tick(activeSample()); d.Lock != LockBypass || d.Day != tuesday {
		t.Fatalf("после полуночи: блокировка %v, день %s; want обход во вторник", d.Lock, d.Day)
	}
	d = e.ClearBlock()
	if d.Lock != LockNone || !slices.Equal(kinds(d.Events), []string{EvBlockCleared}) {
		t.Fatalf("ClearBlock: %+v", d)
	}
	if _, detail := e.State(); detail != "" {
		t.Fatalf("после снятия detail = %q", detail)
	}
	if d := e.ClearBlock(); len(d.Events) != 0 {
		t.Fatalf("снятие без блокировки дало события %v", kinds(d.Events))
	}
	if d := e.MarkBypass(""); d.Lock != LockBypass || d.Detail == "" {
		t.Fatalf("обход без описания: %+v; want блокировка с описанием по умолчанию", d)
	}
}

// Ловит: в полночь блокировка «время вышло» не снимается, доп. время переносится,
// итог прошлого дня не отдаётся службе на сохранение (R16).
func TestEngineMidnightRollover(t *testing.T) {
	var r Rules
	r.Week[4] = LimitOf(120) // пятница
	r.Week[5] = LimitOf(240) // суббота
	c := &fakeClock{t: midnight.Add(-2 * time.Second)}
	e := NewEngine(c, msk, r, DayState{Day: friday, TotalS: 8100, ExtraMin: 15}, "")
	log := run(e, c, midnight.Add(-2*time.Second), midnight, activeSample())
	if log.lockAt != "23:59:58" {
		t.Fatalf("пятница не заблокирована: %q", log.lockAt)
	}
	c.Set(midnight)
	d := e.Tick(activeSample())
	if d.ClosedDay == nil || *d.ClosedDay != (DayState{Day: friday, TotalS: 8100, ExtraMin: 15}) {
		t.Fatalf("ClosedDay = %+v, want пятница 8100 с, доп. 15", d.ClosedDay)
	}
	if !slices.Equal(kinds(d.Events), []string{EvDayClosed}) || d.Events[0].Details["day"] != friday || d.Events[0].Details["total_s"] != 8100 {
		t.Fatalf("события смены суток: %+v", d.Events)
	}
	if d.Day != saturday || d.Lock != LockNone || d.LimitS != 14400 || d.RemainingS != 14400 {
		t.Fatalf("суббота: %+v", d)
	}
	c.Add(time.Second)
	d = e.Tick(activeSample())
	if d.ClosedDay != nil || len(d.Events) != 0 || d.UsedS != 1 {
		t.Fatalf("второй тик субботы: %+v", d)
	}
}

// Ловит: сохранённые правила применяются только со следующего тика или не снимают
// и не ставят блокировку сразу (FR-19).
func TestEngineSetRulesAppliesNow(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{}, "")
	run(e, c, at(15, 0, 0), at(17, 0, 10), activeSample())
	d := e.SetRules(rulesMon(150))
	if d.Lock != LockNone || d.RemainingS != 1800 {
		t.Fatalf("лимит увеличен: %+v", d)
	}
	log := run(e, c, at(17, 0, 10), at(17, 15, 11), activeSample())
	if want := []string{"17:15:10 15"}; !slices.Equal(log.warnings, want) {
		t.Fatalf("после увеличения лимита предупреждения %v, want %v", log.warnings, want)
	}
	d = e.SetRules(rulesMon(60))
	if d.Lock != LockTimeUp || d.Until != "Можно снова: завтра" {
		t.Fatalf("лимит уменьшен ниже использованного: %+v", d)
	}
}

// Ловит: Current() засчитывает секунду, гасит порог или отдаёт события вместо Tick;
// у границы порога статус не янтарный (ТЗ 8.0 п.3).
func TestEngineCurrentIsReadOnly(t *testing.T) {
	c := &fakeClock{t: at(16, 44, 59)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 6299}, "")
	for range 2 {
		if d := e.Current(); d.UsedS != 6299 || d.Warning != 0 || d.Status != StatusOK || d.Events == nil || len(d.Events) != 0 {
			t.Fatalf("Current() = %+v", d)
		}
	}
	if d := e.Tick(activeSample()); d.Warning != 0 || d.RemainingS != 901 {
		t.Fatalf("тик при остатке 901 с: %+v", d)
	}
	if d := e.Current(); d.Status != StatusLow || d.RemainingS != 900 || d.Warning != 0 {
		t.Fatalf("Current() при остатке 900 с: %+v", d)
	}
	c.Set(at(16, 45, 0))
	if d := e.Tick(activeSample()); d.Warning != 15 {
		t.Fatalf("Current() погасил порог: Warning = %d", d.Warning)
	}

	c.Set(time.Date(2026, 10, 20, 0, 0, 0, 0, msk))
	d := e.Current()
	if d.Day != tuesday || d.UsedS != 0 || d.ClosedDay != nil || len(d.Events) != 0 {
		t.Fatalf("Current() после полуночи до тика: %+v", d)
	}
	if st, _ := e.State(); st.Day != monday {
		t.Fatalf("Current() сменил сутки в состоянии: %+v", st)
	}
	if d := e.Tick(activeSample()); d.ClosedDay == nil || d.ClosedDay.Day != monday {
		t.Fatalf("тик после полуночи не закрыл понедельник: %+v", d.ClosedDay)
	}
}

// Ловит: сохранение реже 30 с, отсчёт не от начала учёта или несколько сохранений
// в одном 30-секундном отрезке (G10, NFR-7).
func TestEngineDueSave(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{}, "")
	if e.DueSave(at(15, 0, 30)) {
		t.Fatal("сохранение до начала учёта")
	}
	e.Tick(activeSample())
	steps := []struct {
		at   time.Time
		want bool
	}{
		{at(15, 0, 29), false},
		{at(15, 0, 30), true},
		{at(15, 0, 30), false},
		{at(15, 0, 59), false},
		{at(15, 1, 0), true},
		{at(15, 2, 15), true},
		{at(15, 2, 20), false},
		{at(15, 2, 30), true},
	}
	for _, s := range steps {
		if got := e.DueSave(s.at); got != s.want {
			t.Fatalf("DueSave(%s) = %v, want %v", s.at.Format(time.TimeOnly), got, s.want)
		}
	}
}

// Ловит: в тестовом режиме (часы ×60, служба тикает раз в секунду часов ядра) секунды
// теряются из-за правила разрыва (Ruling 4 и Ruling 13 вместе).
func TestEngineScaledClockTicksCountFully(t *testing.T) {
	realStart := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	real := &realTime{t: realStart}
	c := scaled(at(15, 0, 0), real, realStart, 60)
	e := NewEngine(c, msk, rulesMon(120), DayState{}, "")
	for i := range 120 {
		real.t = realStart.Add(time.Duration(i) * time.Second / 60)
		e.Tick(activeSample())
	}
	if st, _ := e.State(); st != (DayState{Day: monday, TotalS: 120}) {
		t.Fatalf("за 2 с настоящего времени ×60: %+v, want 120 с", st)
	}
}

// Ловит: пустые события уходят во фронт и стражу как null (L-22, CLAUDE.md правило 6).
func TestDecisionJSONHasNoNullEvents(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{}, "")
	extra, _ := e.AddExtra(0)
	for name, d := range map[string]Decision{
		"Tick":       e.Tick(activeSample()),
		"Current":    e.Current(),
		"AddExtra":   extra,
		"ClearBlock": e.ClearBlock(),
		"SetRules":   e.SetRules(rulesMon(120)),
	} {
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"Events":[]`) {
			t.Errorf("%s: %s", name, b)
		}
	}
}

// Ловит: ядро без явного пояса падает на nil или считает сутки в UTC, а не в местном поясе.
func TestEngineDefaultsToLocalZone(t *testing.T) {
	c := &fakeClock{t: time.Date(2026, 10, 19, 23, 30, 0, 0, time.UTC)}
	e := NewEngine(c, nil, Rules{}, DayState{}, "")
	if got, want := e.Current().Day, c.t.In(time.Local).Format(time.DateOnly); got != want {
		t.Fatalf("Current().Day = %s, want %s (местный пояс)", got, want)
	}
}

// Ловит: гонку между тиком службы и командами родителя из канала связи.
func TestEngineConcurrentUse(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday}, "")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			e.Current()
			e.State()
		}
	}()
	for range 200 {
		e.Tick(activeSample())
	}
	wg.Wait()
	if st, _ := e.State(); st.TotalS != 200 {
		t.Fatalf("TotalS = %d, want 200", st.TotalS)
	}
}
