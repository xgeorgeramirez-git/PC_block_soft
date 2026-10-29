package core

import (
	"slices"
	"testing"
	"time"
)

// Тесты M3 ядра: лимит игр (R5), режим окончания (R8), игровые предупреждения (R9, PL-14),
// учёт запрещённой секунды (RM3-7). Ожидания — литералы, посчитанные вручную.

func intp(n int) *int { return &n }

// gamesMon — понедельник: общий лимит total мин (nil — без лимита), лимит игр games (nil —
// без лимита игр), режим окончания mode; остальные дни без лимитов.
func gamesMon(total, games *int, mode EndMode) Rules {
	var r Rules
	r.Week[0] = DayLimit{TotalMin: total, GamesMin: games}
	r.EndMode = mode
	return r
}

func gameSample() SessionSample {
	return SessionSample{SessionActive: true, LastInputAgoMs: 1000, Foreground: CatGames}
}

func studySample() SessionSample {
	return SessionSample{SessionActive: true, LastInputAgoMs: 1000, Foreground: CatStudy}
}

// tue — момент вторника 20.10.2026 по UTC+3.
func tue(h, m, s int) time.Time { return time.Date(2026, 10, 20, h, m, s, 0, msk) }

// Ловит: остаток игр без доп. времени, отрицательный остаток, лимит игр 0 не закрывает игры
// с первой секунды, «без лимита игр» закрывает игры.
func TestRemainingGamesS(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lim           DayLimit
		extra, gamesS int
		rem           int
		unl           bool
	}{
		{"нет лимита игр", DayLimit{TotalMin: intp(120)}, 30, 100, 0, true},
		{"60 мин, 20 мин игры", DayLimit{GamesMin: intp(60)}, 0, 1200, 2400, false},
		{"60 + 30, 20 мин игры", DayLimit{GamesMin: intp(60)}, 30, 1200, 4200, false},
		{"0 мин", DayLimit{GamesMin: intp(0)}, 0, 0, 0, false},
		{"0 + 30", DayLimit{GamesMin: intp(0)}, 30, 0, 1800, false},
		{"перебор", DayLimit{GamesMin: intp(10)}, -5, 900, 0, false},
	} {
		rem, unl := RemainingGamesS(tc.lim, tc.extra, tc.gamesS)
		if rem != tc.rem || unl != tc.unl {
			t.Errorf("%s: (%d, %v), want (%d, %v)", tc.name, rem, unl, tc.rem, tc.unl)
		}
	}
}

// Ловит (тест 1): доп. время не расширяет лимит игр; остаток игр после игры; лимит 0 не
// закрывает игры или закрывает и общее; «без лимита игр» что-то закрывает; убавление доп.
// времени не убавляет лимит игр или уводит его в минус.
func TestEngineGamesRemaining(t *testing.T) {
	t.Run("60 + 30, 20 мин игры", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(120), intp(60), EndBlock), DayState{Day: monday}, "")
		d, err := e.AddExtra(30)
		if err != nil {
			t.Fatal(err)
		}
		if d.GamesLimitS != 5400 || d.LimitS != 9000 || d.GamesUnlimited {
			t.Fatalf("после +30: лимит игр %d, общий %d, без лимита игр %v; want 5400, 9000, false", d.GamesLimitS, d.LimitS, d.GamesUnlimited)
		}
		run(e, c, at(15, 0, 0), at(15, 20, 0), gameSample())
		cur := e.Current()
		if cur.GamesRemainingS != 4200 || cur.RemainingS != 7800 || cur.CloseGames {
			t.Fatalf("после 20 мин игры: игры %d, общее %d, закрыто %v; want 4200, 7800, false", cur.GamesRemainingS, cur.RemainingS, cur.CloseGames)
		}
		d, removed, err := e.ReduceExtra(60)
		if err != nil || removed != 60 {
			t.Fatalf("−60: убрано %d, err %v", removed, err)
		}
		if d.GamesLimitS != 1800 || d.GamesRemainingS != 600 {
			t.Fatalf("после −60: лимит игр %d, остаток %d; want 1800, 600", d.GamesLimitS, d.GamesRemainingS)
		}
	})
	t.Run("лимит игр 0", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(120), intp(0), EndBlock), DayState{Day: monday}, "")
		d := e.Tick(gameSample())
		if !d.CloseGames || d.Lock != LockNone || d.RemainingS != 7200 || d.Counted {
			t.Fatalf("первая секунда игры: закрыто %v, экран %v, общее %d, засчитано %v; want true, нет, 7200, false", d.CloseGames, d.Lock, d.RemainingS, d.Counted)
		}
		if d.CloseReason != "Время на игры на сегодня закончилось" || d.GamesLimitS != 0 || d.GamesRemainingS != 0 || d.Status != StatusOK {
			t.Fatalf("решение %+v", d)
		}
		c.Set(at(15, 0, 1))
		d = e.Tick(activeSample())
		if !d.Counted || d.CountedCat != CatOther {
			t.Fatalf("«Прочее» при закрытых играх: засчитано %v (%v), want true, «Прочее»", d.Counted, d.CountedCat)
		}
		if st, _ := e.State(); st.TotalS != 1 || st.GamesS != 0 {
			t.Fatalf("итог %+v, want общее 1, игры 0", st)
		}
		if cur := e.Current(); cur.Counted {
			t.Fatal("Current: Counted = true, want false")
		}
		// Убавление доп. времени ниже нуля игр: лимит игр не уходит в минус.
		d, _, _ = e.ReduceExtra(30)
		if d.GamesLimitS != 0 || d.GamesRemainingS != 0 || !d.CloseGames {
			t.Fatalf("после −30: лимит игр %d, остаток %d, закрыто %v; want 0, 0, true", d.GamesLimitS, d.GamesRemainingS, d.CloseGames)
		}
		d, _ = e.AddExtra(45) // доп. время −30 + 45 = +15
		if d.GamesLimitS != 900 || d.CloseGames {
			t.Fatalf("после +45: лимит игр %d, закрыто %v; want 900, false", d.GamesLimitS, d.CloseGames)
		}
	})
	t.Run("без лимита игр", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(120), nil, EndBlock), DayState{Day: monday, GamesS: 7000, TotalS: 7000}, "")
		d := e.Tick(gameSample())
		if !d.GamesUnlimited || d.GamesLimitS != 0 || d.CloseGames || !d.Counted {
			t.Fatalf("решение %+v, want без лимита игр, ничего не закрыто, секунда засчитана", d)
		}
	})
	t.Run("правила не прочитаны", func(t *testing.T) {
		r := gamesMon(intp(0), intp(0), EndStudyOnly)
		r.Broken = true
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, r, DayState{Day: monday}, "")
		d := e.Tick(gameSample())
		if !d.GamesUnlimited || d.CloseGames || d.StudyOnly || d.Lock != LockNone || !d.Counted {
			t.Fatalf("Broken: %+v, want ничего не закрыто (R18)", d)
		}
		for _, cat := range []Category{CatOther, CatGames, CatVideoSocial, CatStudy} {
			if !d.Allows(cat) {
				t.Errorf("Broken: Allows(%v) = false", cat)
			}
		}
	})
}

// Ловит (тест 2): игры не идут в общее время или учёба идёт (R3, R4, R5; G4-подобный).
func TestEngineGamesAndStudyTotals(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, gamesMon(intp(120), intp(60), EndBlock), DayState{Day: monday}, "")
	c.Set(at(15, 0, 0))
	if d := e.Tick(studySample()); !d.Counted || d.CountedCat != CatStudy {
		t.Fatalf("секунда учёбы: засчитано %v (%v), want true, «Учёба»", d.Counted, d.CountedCat)
	}
	run(e, c, at(15, 0, 1), at(15, 10, 0), studySample())
	run(e, c, at(15, 10, 0), at(15, 20, 0), gameSample())
	st, _ := e.State()
	if st.TotalS != 600 || st.GamesS != 600 || st.StudyS != 600 {
		t.Fatalf("итог %+v, want общее 600, игры 600, учёба 600", st)
	}
	if cur := e.Current(); cur.RemainingS != 6600 || cur.GamesRemainingS != 3000 {
		t.Fatalf("остатки: общее %d, игры %d; want 6600, 3000", cur.RemainingS, cur.GamesRemainingS)
	}
}

// Ловит (тест 3): Allows и DeniedText не по таблице R5/R8; нулевое решение старого кода
// что-то запрещает.
func TestDecisionAllows(t *testing.T) {
	cats := []Category{CatOther, CatGames, CatVideoSocial, CatStudy}
	for _, tc := range []struct {
		name   string
		d      Decision
		allow  [4]bool
		denied [4]string
	}{
		{"экран блокировки", Decision{Lock: LockTimeUp, CloseGames: true}, [4]bool{false, false, false, false},
			[4]string{"", "Закрыто: время на игры на сегодня закончилось", "", ""}},
		{"только учёба", Decision{StudyOnly: true}, [4]bool{false, false, false, true},
			[4]string{"Закрыто: время вышло, доступна только учёба", "Закрыто: время вышло, доступна только учёба", "Закрыто: время вышло, доступна только учёба", ""}},
		{"игры закрыты", Decision{CloseGames: true}, [4]bool{true, false, true, true},
			[4]string{"", "Закрыто: время на игры на сегодня закончилось", "", ""}},
		{"свободно", Decision{}, [4]bool{true, true, true, true}, [4]string{"", "", "", ""}},
	} {
		for i, cat := range cats {
			if got := tc.d.Allows(cat); got != tc.allow[i] {
				t.Errorf("%s: Allows(%v) = %v, want %v", tc.name, cat, got, tc.allow[i])
			}
			if got := tc.d.DeniedText(cat); got != tc.denied[i] {
				t.Errorf("%s: DeniedText(%v) = %q, want %q", tc.name, cat, got, tc.denied[i])
			}
		}
	}
}

// Ловит (тест 4): в режиме «только учёба» показывается экран, статус не красный, пауза или
// доп. время не снимают режим, засчитывается не только учёба.
func TestEngineStudyOnly(t *testing.T) {
	t.Run("время вышло", func(t *testing.T) {
		c := &fakeClock{t: at(16, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(60), nil, EndStudyOnly), DayState{Day: monday, TotalS: 3600}, "")
		d := e.Tick(gameSample())
		if d.Lock != LockNone || !d.StudyOnly || d.CloseGames || d.Status != StatusTimeUp || d.Counted {
			t.Fatalf("игра: экран %v, только учёба %v, игры %v, статус %v, засчитано %v", d.Lock, d.StudyOnly, d.CloseGames, d.Status, d.Counted)
		}
		if d.CloseReason != "Время вышло. Сейчас доступна только учёба" || d.Until != "Можно снова: завтра" {
			t.Fatalf("тексты: %q, %q", d.CloseReason, d.Until)
		}
		c.Set(at(16, 0, 1))
		if d := e.Tick(studySample()); !d.Counted || d.CountedCat != CatStudy {
			t.Fatalf("учёба: засчитано %v (%v), want true", d.Counted, d.CountedCat)
		}
		c.Set(at(16, 0, 2))
		if d := e.Tick(activeSample()); d.Counted {
			t.Fatal("«Прочее» засчитано в режиме «только учёба»")
		}
		if st, _ := e.State(); st.TotalS != 3600 || st.StudyS != 1 || st.GamesS != 0 {
			t.Fatalf("итог %+v, want общее 3600, учёба 1, игры 0", st)
		}
		d, _ = e.AddExtra(15)
		if d.StudyOnly || d.Lock != LockNone || d.CloseReason != "" {
			t.Fatalf("после +15: %+v, want режим снят", d)
		}
		c.Set(at(16, 0, 3))
		if d := e.Tick(gameSample()); d.Status != StatusLow || !d.Counted || d.StudyOnly {
			t.Fatalf("после +15: статус %v, засчитано %v, только учёба %v; want янтарный, true, false", d.Status, d.Counted, d.StudyOnly)
		}
	})
	t.Run("запретный интервал", func(t *testing.T) {
		r := gamesMon(nil, nil, EndStudyOnly)
		r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
		c := &fakeClock{t: at(21, 30, 0)}
		e := NewEngine(c, msk, r, DayState{Day: monday}, "")
		d := e.Tick(studySample())
		if d.Lock != LockNone || !d.StudyOnly || d.Status != StatusForbidden || !d.Counted {
			t.Fatalf("интервал: экран %v, только учёба %v, статус %v, засчитано %v", d.Lock, d.StudyOnly, d.Status, d.Counted)
		}
		if d.CloseReason != "Сейчас нельзя пользоваться компьютером. Доступна только учёба" || d.Until != "Можно снова: завтра в 07:00" {
			t.Fatalf("тексты: %q, %q", d.CloseReason, d.Until)
		}
	})
	t.Run("пауза", func(t *testing.T) {
		c := &fakeClock{t: at(16, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(60), intp(0), EndStudyOnly), DayState{Day: monday, TotalS: 3600}, "")
		if _, err := e.Pause(at(17, 0, 0)); err != nil {
			t.Fatal(err)
		}
		d := e.Tick(gameSample())
		if d.StudyOnly || d.CloseGames || d.Lock != LockNone || d.Status != StatusPaused || !d.Counted {
			t.Fatalf("пауза: %+v, want ничего не закрыто, секунда засчитана", d)
		}
		for _, cat := range []Category{CatOther, CatGames, CatVideoSocial, CatStudy} {
			if !d.Allows(cat) {
				t.Errorf("пауза: Allows(%v) = false", cat)
			}
		}
	})
}

// Ловит (тест 5): в режиме «Блокировать экран» игры не закрываются под экраном (R8).
func TestEngineEndBlockClosesGames(t *testing.T) {
	c := &fakeClock{t: at(16, 0, 0)}
	e := NewEngine(c, msk, gamesMon(intp(60), nil, EndBlock), DayState{Day: monday, TotalS: 3600}, "")
	d := e.Tick(gameSample())
	if d.Lock != LockTimeUp || !d.CloseGames || d.StudyOnly || d.Reason != "Время на сегодня закончилось" || d.CloseReason != "Время на сегодня закончилось" {
		t.Fatalf("время вышло: %+v", d)
	}
	r := gamesMon(nil, nil, EndBlock)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c = &fakeClock{t: at(21, 30, 0)}
	e = NewEngine(c, msk, r, DayState{Day: monday}, "")
	d = e.Tick(gameSample())
	if d.Lock != LockForbidden || !d.CloseGames || d.StudyOnly || d.Status != StatusForbidden {
		t.Fatalf("запретный интервал: %+v", d)
	}
}

// Ловит (тест 6): игровые пороги нарушают PL-14 (взвод нестрого, повтор после SetRules),
// два порога в один тик дают два уведомления или не тот, игровой порог показан, когда общий
// лимит кончится раньше, пауза не гасит/не взводит игровые пороги, сутки не обнуляют игры,
// игровой порог срабатывает без игры на переднем плане.
func TestEngineGamesWarnings(t *testing.T) {
	t.Run("+30 при исчерпанных играх", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(120), intp(0), EndBlock), DayState{Day: monday}, "")
		if _, err := e.AddExtra(30); err != nil {
			t.Fatal(err)
		}
		got := warnEvLog(e, c, at(15, 0, 0), at(15, 30, 5), gameSample())
		want := []string{"15:15:00 15 games", "15:25:00 5 games", "15:29:00 1 games"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("SetRules на границе той же секунды", func(t *testing.T) {
		r := gamesMon(intp(120), intp(60), EndBlock)
		c := &fakeClock{t: at(15, 29, 59)}
		e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 1799, GamesS: 1799}, "")
		// 15:29:59 засчитана: остаток игр ровно 30:00; дальше игра на переднем плане, сеанс спит.
		stalled := SessionSample{Foreground: CatGames}
		if got := warnEvLog(e, c, at(15, 29, 59), at(15, 30, 0), gameSample()); len(got) != 0 {
			t.Fatalf("при остатке 30:01: %v", got)
		}
		got := warnEvLog(e, c, at(15, 30, 0), at(15, 30, 1), stalled)
		if want := []string{"15:30:00 30 games"}; !slices.Equal(got, want) {
			t.Fatalf("до сохранения правил: %v, want %v", got, want)
		}
		e.SetRules(r)
		if got := warnEvLog(e, c, at(15, 30, 1), at(15, 31, 0), stalled); len(got) != 0 {
			t.Fatalf("после сохранения тех же правил повтор: %v", got)
		}
	})
	t.Run("равные события в один тик", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(60), intp(60), EndBlock), DayState{Day: monday}, "")
		got := warnEvLog(e, c, at(15, 0, 0), at(16, 0, 5), gameSample())
		want := []string{"15:30:00 30 limit", "15:45:00 15 limit", "15:55:00 5 limit", "15:59:00 1 limit"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("два порога в один тик, меньший остаток", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(70), intp(60), EndBlock), DayState{Day: monday}, "")
		got := warnEvLog(e, c, at(15, 0, 0), at(16, 0, 5), gameSample())
		// 15:55: общий «15» (остаток 15:00) и игровой «5» (5:00) в один тик → только игровой.
		want := []string{"15:30:00 30 games", "15:40:00 30 limit", "15:45:00 15 games", "15:55:00 5 games", "15:59:00 1 games"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("общий лимит кончится раньше", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		// Общий остаток 10:00, игр 16:00.
		e := NewEngine(c, msk, gamesMon(intp(120), intp(60), EndBlock), DayState{Day: monday, TotalS: 110 * 60, GamesS: 44 * 60}, "")
		got := warnEvLog(e, c, at(15, 0, 0), at(15, 10, 5), gameSample())
		want := []string{"15:05:00 5 limit", "15:09:00 1 limit"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("только при игре на переднем плане", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(120), intp(60), EndBlock), DayState{Day: monday, TotalS: 1790, GamesS: 1790}, "")
		if got := warnEvLog(e, c, at(15, 0, 0), at(15, 0, 10), gameSample()); len(got) != 0 {
			t.Fatalf("до 30:00: %v", got)
		}
		if got := warnEvLog(e, c, at(15, 0, 10), at(15, 5, 0), activeSample()); len(got) != 0 {
			t.Fatalf("без игры на переднем плане: %v", got)
		}
		e.Current() // Current порогов не гасит
		got := warnEvLog(e, c, at(15, 5, 0), at(15, 5, 2), gameSample())
		if want := []string{"15:05:00 30 games"}; !slices.Equal(got, want) {
			t.Fatalf("вернулся в игру: %v, want %v", got, want)
		}
	})
	t.Run("пауза гасит, конец паузы взводит", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, gamesMon(intp(300), intp(60), EndBlock), DayState{Day: monday}, "")
		if _, err := e.Pause(at(15, 40, 0)); err != nil {
			t.Fatal(err)
		}
		got := warnEvLog(e, c, at(15, 0, 0), at(16, 0, 5), gameSample())
		want := []string{
			"15:10:00 30 pause_end", "15:25:00 15 pause_end", "15:35:00 5 pause_end", "15:39:00 1 pause_end",
			"15:45:00 15 games", "15:55:00 5 games", "15:59:00 1 games",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("смена суток обнуляет игры", func(t *testing.T) {
		r := gamesMon(intp(120), intp(60), EndBlock)
		r.Week[1] = DayLimit{TotalMin: intp(120), GamesMin: intp(60)}
		c := &fakeClock{t: at(23, 59, 59)}
		e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 3600, GamesS: 3600}, "")
		if d := e.Tick(gameSample()); !d.CloseGames {
			t.Fatal("пн 23:59:59: игры не закрыты")
		}
		c.Set(tue(0, 0, 0))
		if cur := e.Current(); cur.Day != tuesday || cur.CloseGames || cur.GamesRemainingS != 3600 {
			t.Fatalf("Current во вторник без тика: %+v", cur)
		}
		got := warnEvLog(e, c, tue(0, 0, 0), tue(0, 30, 1), gameSample())
		if want := []string{"00:30:00 30 games"}; !slices.Equal(got, want) {
			t.Fatalf("вторник: %v, want %v", got, want)
		}
	})
}

// Ловит (тест 7): SetRules теряет режим окончания или лимит игр, применяет их не сразу.
func TestEngineSetRulesEndModeAndGames(t *testing.T) {
	c := &fakeClock{t: at(16, 0, 0)}
	e := NewEngine(c, msk, gamesMon(intp(60), nil, EndBlock), DayState{Day: monday, TotalS: 3600, GamesS: 1800}, "")
	if d := e.Tick(gameSample()); d.Lock != LockTimeUp {
		t.Fatalf("до смены режима: экран %v, want «время вышло»", d.Lock)
	}
	d := e.SetRules(gamesMon(intp(60), nil, EndStudyOnly))
	if d.Lock != LockNone || !d.StudyOnly {
		t.Fatalf("режим «только учёба»: экран %v, только учёба %v", d.Lock, d.StudyOnly)
	}
	d = e.SetRules(gamesMon(intp(120), intp(30), EndStudyOnly))
	if d.StudyOnly || !d.CloseGames || d.GamesLimitS != 1800 || d.RemainingS != 3600 {
		t.Fatalf("лимит игр 30: %+v", d)
	}
}

// Ловит: ключи режима окончания и вида события расходятся с хранилищем и каналом.
func TestEndModeStrings(t *testing.T) {
	if EndBlock.String() != "lock" || EndStudyOnly.String() != "study_only" || WarnGames.String() != "games" {
		t.Fatalf("ключи: %q %q %q", EndBlock, EndStudyOnly, WarnGames)
	}
	for in, want := range map[string]EndMode{"lock": EndBlock, "study_only": EndStudyOnly, "": EndBlock, "Study_Only": EndBlock, "x": EndBlock} {
		if got := EndModeFromString(in); got != want {
			t.Errorf("EndModeFromString(%q) = %v, want %v", in, got, want)
		}
	}
	var zero Rules
	if zero.EndMode != EndBlock {
		t.Fatal("режим по умолчанию не «Блокировать экран»")
	}
}
