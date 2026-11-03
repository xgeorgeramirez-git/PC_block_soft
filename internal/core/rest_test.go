package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// Тесты перерыва на отдых (R27, D-62, G14) сверх эталона: старшинство экранов, пауза, плашка
// один раз, выключение, безопасный режим, сохранение сразу, тексты и пределы. Ожидания — литералы.

// restRules — пн, лимит 180 мин, перерывы по умолчанию G14 (60 / всё, кроме учёбы / 5 / 15 / 15).
func restRules() Rules {
	r := rulesMon(180)
	r.Break = BreakRules{Enabled: true, AfterMin: 60, Counts: BreakAll, WarnMin: 5, RestMin: 15, SelfRestMin: 15}
	return r
}

// restEngine — ядро с 15:00:00 и счётом 59 мин игр к 15:59:00 (плашка в 16:00:00 впереди).
func restEngine(t *testing.T, r Rules) (*Engine, *fakeClock) {
	t.Helper()
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday}, "")
	run(e, c, at(15, 0, 0), at(15, 59, 0), gameSample())
	if got := e.Break().CountedS; got != 59*60 {
		t.Fatalf("счёт к 15:59:00 = %d, want 3540", got)
	}
	return e, c
}

// restLog тикает [from, to) и пишет моменты плашек отдыха и смены экрана.
func restLog(e *Engine, c *fakeClock, from, to time.Time, s SessionSample) []string {
	out := []string{}
	prev := e.Current().Lock
	for t := from; t.Before(to); t = t.Add(time.Second) {
		c.Set(t)
		d := e.Tick(s)
		if d.RestWarn {
			out = append(out, t.Format(time.TimeOnly)+" плашка")
		}
		if d.Lock != prev {
			out = append(out, t.Format(time.TimeOnly)+" "+lockName(d.Lock))
			prev = d.Lock
		}
	}
	return out
}

func lockName(k LockKind) string {
	return [...]string{"нет", "время", "запрет", "обход", "перерыв"}[k]
}

// Ловит: запретные часы, начавшиеся во время перерыва, не перекрывают экран перерыва (RF14-5);
// перерыв не идёт по часам под их экраном (после конца запрета перерыва уже нет).
func TestRestForbiddenStronger(t *testing.T) {
	r := restRules()
	r.Schedule = []Interval{{Weekday: Monday, StartMin: 16*60 + 10, EndMin: 16*60 + 30}}
	e, c := restEngine(t, r)
	got := restLog(e, c, at(15, 59, 0), at(16, 31, 0), gameSample())
	want := []string{"16:00:00 плашка", "16:05:00 перерыв", "16:10:00 запрет", "16:30:00 нет"}
	if !slices.Equal(got, want) {
		t.Fatalf("экраны %v, want %v", got, want)
	}
}

// Ловит: окончание времени не сильнее перерыва: лимит кончился ровно к началу перерыва.
func TestRestTimeUpStronger(t *testing.T) {
	r := restRules()
	r.Week[0] = LimitOf(62)
	e, c := restEngine(t, r)
	got := restLog(e, c, at(15, 59, 0), at(16, 6, 0), gameSample())
	want := []string{"16:00:00 плашка", "16:02:00 время"}
	if !slices.Equal(got, want) {
		t.Fatalf("экраны %v, want %v", got, want)
	}
	if d := e.Current(); d.Reason != ReasonTimeUp {
		t.Fatalf("причина %q, want %q", d.Reason, ReasonTimeUp)
	}
}

// Ловит: пауза не снимает идущий перерыв; в паузе простой засчитывается как «сам отошёл» или
// счёт растёт; после паузы счёт не продолжается с того же места.
func TestRestPauseRemovesBreakAndFreezes(t *testing.T) {
	e, c := restEngine(t, restRules())
	restLog(e, c, at(15, 59, 0), at(16, 6, 0), gameSample())
	c.Set(at(16, 6, 0))
	d, err := e.Pause(at(17, 6, 0))
	if err != nil || d.Lock != LockNone {
		t.Fatalf("пауза в перерыве: Lock %v, err %v; want без экрана", d.Lock, err)
	}
	if b := e.Break(); b.RestUntilMs != nil || b.BreakAtMs != nil || b.CountedS != 0 {
		t.Fatalf("после паузы в перерыве состояние %+v, want пустое", b)
	}
	// Новая серия: 40 мин счёта, пауза 1 ч с простоем — счёт стоит, не сбрасывается.
	c2 := &fakeClock{t: at(15, 0, 0)}
	e2 := NewEngine(c2, msk, restRules(), DayState{Day: monday}, "")
	run(e2, c2, at(15, 0, 0), at(15, 40, 0), gameSample())
	c2.Set(at(15, 40, 0))
	if _, err := e2.Pause(at(16, 40, 0)); err != nil {
		t.Fatal(err)
	}
	run(e2, c2, at(15, 40, 0), at(16, 40, 0), idleSample())
	run(e2, c2, at(16, 40, 0), at(16, 40, 1), gameSample())
	if got := e2.Break().CountedS; got != 2401 {
		t.Fatalf("после паузы с простоем счёт %d, want 2401 (2400 + секунда после паузы)", got)
	}
}

// Ловит: повтор плашки при сохранении тех же правил и после перезапуска службы (PL-14, PL-16).
func TestRestPlateOnce(t *testing.T) {
	e, c := restEngine(t, restRules())
	got := restLog(e, c, at(15, 59, 0), at(16, 1, 0), gameSample())
	e.SetRules(restRules())
	got = append(got, restLog(e, c, at(16, 1, 0), at(16, 2, 0), gameSample())...)
	saved, _ := e.State()
	e2 := NewEngine(c, msk, restRules(), saved, "")
	e2.RestoreBreak(e.Break())
	got = append(got, restLog(e2, c, at(16, 2, 0), at(16, 5, 1), gameSample())...)
	want := []string{"16:00:00 плашка", "16:05:00 перерыв"}
	if !slices.Equal(got, want) {
		t.Fatalf("плашки и экраны %v, want %v", got, want)
	}
}

// Ловит: момент начала и конец перерыва не сохраняются сразу (выключение в эти секунды их теряет).
func TestRestSavesNow(t *testing.T) {
	e, c := restEngine(t, restRules())
	from := at(15, 59, 0)
	for _, tc := range []struct {
		tick time.Time
		name string
	}{{at(16, 0, 0), "плашка"}, {at(16, 5, 0), "начало перерыва"}} {
		run(e, c, from, tc.tick, gameSample())
		e.DueSave(tc.tick) // отрезок 30 с на границе — не наш случай
		c.Set(tc.tick)
		e.Tick(gameSample())
		if !e.DueSave(tc.tick.Add(time.Second)) {
			t.Fatalf("%s: DueSave сразу после тика = false, want true", tc.name)
		}
		from = tc.tick.Add(time.Second)
	}
	if b := e.Break(); b.RestUntilMs == nil || *b.RestUntilMs != at(16, 20, 0).UnixMilli() {
		t.Fatalf("конец перерыва %+v, want 16:20:00", b.RestUntilMs)
	}
}

// Ловит: экран перерыва без своих строк: причина, «Можно снова в», конец по часам, сообщение.
func TestRestScreenDecision(t *testing.T) {
	r := restRules()
	r.Break.Message = "Пойди погуляй с собакой"
	e, c := restEngine(t, r)
	run(e, c, at(15, 59, 0), at(16, 5, 1), gameSample())
	c.Set(at(16, 10, 0))
	d := e.Current()
	if d.Lock != LockRest || d.Reason != "Перерыв на отдых" || d.Until != "Можно снова в 16:20" ||
		d.RestUntilMs != at(16, 20, 0).UnixMilli() || d.RestLeftS != 600 || d.RestMessage != "Пойди погуляй с собакой" {
		t.Fatalf("экран перерыва %+v", d)
	}
	if d.Allows(CatGames) || d.CloseGames || d.StudyOnly {
		t.Fatalf("под экраном перерыва: Allows игр %v, CloseGames %v, StudyOnly %v; want false, false, false", d.Allows(CatGames), d.CloseGames, d.StudyOnly)
	}
	r.Break.Message = ""
	e.SetRules(r)
	if d := e.Current(); d.RestMessage != "Разомнись, попей воды, посмотри в окно" {
		t.Fatalf("пустое сообщение: %q, want стандартное", d.RestMessage)
	}
}

// Ловит: Current между тиками не показывает перерыв, момент начала которого уже наступил.
func TestRestCurrentBetweenTicks(t *testing.T) {
	e, c := restEngine(t, restRules())
	run(e, c, at(15, 59, 0), at(16, 0, 1), gameSample())
	c.Set(at(16, 7, 0))
	if d := e.Current(); d.Lock != LockRest || d.Until != "Можно снова в 16:20" {
		t.Fatalf("Current в 16:07:00: Lock %v, %q; want перерыв до 16:20", d.Lock, d.Until)
	}
}

// Ловит: события журнала перерыва: текст начала, «Вы закончили перерыв», нет события без перерыва.
func TestRestEvents(t *testing.T) {
	e, c := restEngine(t, restRules())
	run(e, c, at(15, 59, 0), at(16, 5, 0), gameSample())
	c.Set(at(16, 4, 0))
	if _, ok := e.EndBreak(); ok {
		t.Fatal("EndBreak до перерыва = true, want false")
	}
	c.Set(at(16, 5, 0))
	d := e.Tick(gameSample())
	if len(d.Events) != 1 || d.Events[0].Kind != "rest_break" || d.Events[0].Details["text"] != "Перерыв на отдых: 15 мин после 60 мин за компьютером" {
		t.Fatalf("событие начала %+v", d.Events)
	}
	c.Set(at(16, 8, 0))
	d, ok := e.EndBreak()
	if !ok || d.Lock != LockNone || len(d.Events) != 1 || d.Events[0].Kind != "rest_ended" || d.Events[0].Details["text"] != "Вы закончили перерыв" {
		t.Fatalf("EndBreak: ok %v, Lock %v, события %+v", ok, d.Lock, d.Events)
	}
	if b := e.Break(); b.CountedS != 0 || b.RestUntilMs != nil {
		t.Fatalf("после EndBreak %+v, want счёт с нуля", b)
	}
	r := restRules()
	r.Break.Counts = BreakGames
	if got := RestBreakText(r.Break); got != "Перерыв на отдых: 15 мин после 60 мин игр" {
		t.Fatalf("текст «Только игры» %q", got)
	}
}

// Ловит: выключенные перерывы или безопасный режим всё равно считают и показывают экран;
// выключение во время перерыва его не снимает.
func TestRestDisabledAndSafeMode(t *testing.T) {
	off := rulesMon(180)
	off.Break = BreakRules{AfterMin: 60, WarnMin: 5, RestMin: 15, SelfRestMin: 15}
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, off, DayState{Day: monday}, "")
	if got := restLog(e, c, at(15, 0, 0), at(16, 30, 0), gameSample()); len(got) != 0 || e.Break().CountedS != 0 {
		t.Fatalf("выключено: %v, счёт %d; want ничего", got, e.Break().CountedS)
	}
	e2, c2 := restEngine(t, restRules())
	restLog(e2, c2, at(15, 59, 0), at(16, 6, 0), gameSample())
	if d := e2.SetRules(off); d.Lock != LockNone || e2.Break() != (BreakState{}) {
		t.Fatalf("выключили в перерыве: Lock %v, состояние %+v", d.Lock, e2.Break())
	}
	broken := restRules()
	broken.Broken = true
	e3, c3 := restEngine(t, restRules())
	e3.SetRules(broken)
	if got := restLog(e3, c3, at(15, 59, 0), at(16, 30, 0), gameSample()); len(got) != 0 {
		t.Fatalf("безопасный режим: %v, want ничего", got)
	}
}

// Ловит: «Всё время, кроме учёбы» считает учёбу или не считает видео; «Только игры» считает видео.
func TestRestCountsKinds(t *testing.T) {
	video := SessionSample{SessionActive: true, LastInputAgoMs: 1000, Foreground: CatVideoSocial}
	for _, tc := range []struct {
		name   string
		counts BreakCounts
		s      SessionSample
		want   int
	}{
		{"всё: видео", BreakAll, video, 600},
		{"всё: учёба", BreakAll, studySample(), 0},
		{"всё: прочее", BreakAll, activeSample(), 600},
		{"игры: видео", BreakGames, video, 0},
		{"игры: игра", BreakGames, gameSample(), 600},
	} {
		r := restRules()
		r.Break.Counts = tc.counts
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, r, DayState{Day: monday}, "")
		run(e, c, at(15, 0, 0), at(15, 10, 0), tc.s)
		if got := e.Break().CountedS; got != tc.want {
			t.Errorf("%s: счёт %d, want %d", tc.name, got, tc.want)
		}
	}
}

// Ловит: простой у включённого компьютера (без ввода дольше 5 мин, R2а) не засчитывается как
// «сам отошёл» по часам: тишина идёт с первой незасчитанной секунды.
func TestRestIdleAtComputerIsQuiet(t *testing.T) {
	e, c := restEngine(t, restRules()) // 59 мин к 15:59:00
	idle := SessionSample{SessionActive: true, LastInputAgoMs: InactivityLimitMs}
	run(e, c, at(15, 59, 0), at(16, 13, 59), idle)
	if got := e.Break().CountedS; got != 3540 {
		t.Fatalf("простой 14:59: счёт %d, want 3540", got)
	}
	run(e, c, at(16, 13, 59), at(16, 14, 1), idle)
	if got := e.Break().CountedS; got != 0 {
		t.Fatalf("простой 15 мин: счёт %d, want 0 (сам отошёл)", got)
	}
}

// Ловит: неверные формы слов в плашке и пределы настроек FR-37.
func TestRestTextsAndValidate(t *testing.T) {
	for _, tc := range []struct {
		warn, rest int
		want       string
	}{
		{5, 15, "Через 5 минут перерыв на 15 минут. Сохрани игру"},
		{1, 21, "Через 1 минуту перерыв на 21 минуту. Сохрани игру"},
		{2, 11, "Через 2 минуты перерыв на 11 минут. Сохрани игру"},
		{3, 44, "Через 3 минуты перерыв на 44 минуты. Сохрани игру"},
	} {
		if got := RestPlateText(tc.warn, tc.rest); got != tc.want {
			t.Errorf("RestPlateText(%d, %d) = %q, want %q", tc.warn, tc.rest, got, tc.want)
		}
	}
	ok := BreakRules{AfterMin: 15, Counts: BreakGames, WarnMin: 1, RestMin: 5, SelfRestMin: 5, Message: strings.Repeat("я", 120)}
	if err := ok.Validate(); err != nil {
		t.Fatalf("нижние пределы и 120 символов: %v", err)
	}
	if err := (BreakRules{AfterMin: 240, WarnMin: 15, RestMin: 60, SelfRestMin: 120}).Validate(); err != nil {
		t.Fatalf("верхние пределы: %v", err)
	}
	if err := DefaultBreakRules().Validate(); err != nil {
		t.Fatalf("по умолчанию: %v", err)
	}
	for name, b := range map[string]BreakRules{
		"через 14":     {AfterMin: 14, WarnMin: 5, RestMin: 15, SelfRestMin: 15},
		"через 241":    {AfterMin: 241, WarnMin: 5, RestMin: 15, SelfRestMin: 15},
		"предупр 0":    {AfterMin: 60, WarnMin: 0, RestMin: 15, SelfRestMin: 15},
		"предупр 16":   {AfterMin: 60, WarnMin: 16, RestMin: 15, SelfRestMin: 15},
		"отдых 4":      {AfterMin: 60, WarnMin: 5, RestMin: 4, SelfRestMin: 15},
		"отдых 61":     {AfterMin: 60, WarnMin: 5, RestMin: 61, SelfRestMin: 120},
		"сам 121":      {AfterMin: 60, WarnMin: 5, RestMin: 15, SelfRestMin: 121},
		"сам < отдыха": {AfterMin: 60, WarnMin: 5, RestMin: 20, SelfRestMin: 19},
		"121 символ":   {AfterMin: 60, WarnMin: 5, RestMin: 15, SelfRestMin: 15, Message: strings.Repeat("я", 121)},
		"вид счёта":    {AfterMin: 60, Counts: 7, WarnMin: 5, RestMin: 15, SelfRestMin: 15},
	} {
		if err := b.Validate(); !errors.Is(err, ErrBreakRules) {
			t.Errorf("%s: %v, want ErrBreakRules", name, err)
		}
	}
	if got := (BreakRules{Message: "   "}).MessageText(); got != "Разомнись, попей воды, посмотри в окно" {
		t.Fatalf("пробелы: %q", got)
	}
	if BreakCountsFromString("games") != BreakGames || BreakCountsFromString("x") != BreakAll || BreakGames.String() != "games" || BreakAll.String() != "all" {
		t.Fatal("ключи вида счёта")
	}
}

// Ловит: тишина до паузы теряется (пауза её не замораживает, а обнуляет): 10 мин простоя до
// паузы и 5 мин после неё — уже 15 мин подряд, сам отошёл.
func TestRestQuietBeforePauseKept(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, restRules(), DayState{Day: monday}, "")
	run(e, c, at(15, 0, 0), at(15, 40, 0), gameSample())
	run(e, c, at(15, 40, 0), at(15, 50, 0), awaySample())
	c.Set(at(15, 50, 0))
	if _, err := e.Pause(at(16, 50, 0)); err != nil {
		t.Fatal(err)
	}
	run(e, c, at(15, 50, 0), at(16, 54, 59), awaySample())
	if got := e.Break().CountedS; got != 2400 {
		t.Fatalf("14:59 тишины вне паузы: счёт %d, want 2400", got)
	}
	run(e, c, at(16, 54, 59), at(16, 55, 1), awaySample())
	if got := e.Break().CountedS; got != 0 {
		t.Fatalf("15 мин тишины вне паузы: счёт %d, want 0", got)
	}
}

// Ловит: немонотонность «сам отошёл» (R27): ребёнок ушёл сразу после плашки, тишина достигла
// порога «засчитать как отдых» раньше момента начала перерыва, но первый тик после возвращения
// пришёл уже после этого момента — и начался перерыв, хотя вернись он раньше, счёт сбросился бы.
// Граница: тишина с 16:00:01 достигает 5 мин в 16:05:01 — на секунду позже начала в 16:05:00: перерыв.
func TestRestSelfRestBeforeBreakAtWins(t *testing.T) {
	for _, tc := range []struct {
		name               string
		warn, self         int
		back               time.Time
		wantLock           LockKind
		wantCounted        int
		wantBreakAt, wantU bool
	}{
		{"вернулся до начала", 10, 5, at(16, 8, 0), LockNone, 1, false, false},
		{"вернулся после начала", 10, 5, at(16, 12, 0), LockNone, 1, false, false},
		{"порог через секунду после начала", 5, 5, at(16, 6, 0), LockRest, 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := restRules()
			r.Break.WarnMin, r.Break.RestMin, r.Break.SelfRestMin = tc.warn, 5, tc.self
			e, c := restEngine(t, r)
			if got := restLog(e, c, at(15, 59, 0), at(16, 0, 1), gameSample()); !slices.Equal(got, []string{"16:00:00 плашка"}) {
				t.Fatalf("плашка %v", got)
			}
			// Тишина с 16:00:01; тиков нет (компьютер выключен), первый — по возвращении.
			c.Set(tc.back)
			e.DueSave(tc.back)
			if cur := e.Current(); cur.Lock != tc.wantLock {
				t.Fatalf("Current в %s до тика: Lock %s, want %s", tc.back.Format(time.TimeOnly), lockName(cur.Lock), lockName(tc.wantLock))
			}
			d := e.Tick(gameSample())
			b := e.Break()
			if d.Lock != tc.wantLock || b.CountedS != tc.wantCounted || (b.BreakAtMs != nil) != tc.wantBreakAt || (b.RestUntilMs != nil) != tc.wantU {
				t.Fatalf("в %s: Lock %s, состояние %+v; want %s, счёт %d", tc.back.Format(time.TimeOnly), lockName(d.Lock), b, lockName(tc.wantLock), tc.wantCounted)
			}
			if !e.DueSave(tc.back.Add(time.Second)) {
				t.Fatal("сброс счёта или начало перерыва не сохранены сразу")
			}
		})
	}
}

// Ловит: выключение перерывов очищает счёт только в памяти — без сохранения сразу выключение
// компьютера в эти секунды вернёт старый счёт (и плашку) из базы; повторное выключение уже
// пустого счёта сохранения не требует.
func TestRestDisableSavesNow(t *testing.T) {
	e, c := restEngine(t, restRules())
	c.Set(at(15, 59, 0))
	e.DueSave(at(15, 59, 0)) // отрезок 30 с на границе — не наш случай
	off := restRules()
	off.Break.Enabled = false
	e.SetRules(off)
	if b := e.Break(); b != (BreakState{}) {
		t.Fatalf("после выключения состояние %+v, want пустое", b)
	}
	if !e.DueSave(at(15, 59, 0)) {
		t.Fatal("выключение перерывов: DueSave = false, want true (сохранить сразу)")
	}
	e.SetRules(off)
	if e.DueSave(at(15, 59, 0)) {
		t.Fatal("повторное выключение пустого счёта: DueSave = true, want false")
	}
}
