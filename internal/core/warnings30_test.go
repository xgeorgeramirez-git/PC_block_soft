package core

import (
	"slices"
	"testing"
)

// Тесты порогов R9 по ТЗ 1.7 (D-54): 30, 15, 5 и 1 минута; порог взводится, только если до
// события строго больше его минут; повторный пересчёт без сдвига события повторов не даёт.

// idleSample — сеанс активен, но ввода давно нет: секунда не засчитывается (R2а), остаток стоит.
func idleSample() SessionSample {
	return SessionSample{SessionActive: true, LastInputAgoMs: 3600 * 1000, Foreground: CatOther}
}

// Ловит: порога 30 нет, пороги срабатывают не в свои секунды или не по одному разу.
func TestWarn30EntryAt40Min(t *testing.T) {
	c := &fakeClock{t: at(16, 20, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 80 * 60}, "")
	got := warnEvLog(e, c, at(16, 20, 0), at(17, 0, 5), activeSample())
	want := []string{
		"16:30:00 30 limit",
		"16:45:00 15 limit",
		"16:55:00 5 limit",
		"16:59:00 1 limit",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("предупреждения %v, want %v", got, want)
	}
}

// Ловит: порог, равный расстоянию до события при входе, всё же показывается (R9: «строго больше»).
func TestWarn30EntryExactly30(t *testing.T) {
	c := &fakeClock{t: at(16, 30, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 90 * 60}, "")
	got := warnEvLog(e, c, at(16, 30, 0), at(17, 0, 5), activeSample())
	want := []string{"16:45:00 15 limit", "16:55:00 5 limit", "16:59:00 1 limit"}
	if !slices.Equal(got, want) {
		t.Fatalf("вход ровно за 30 мин: %v, want %v", got, want)
	}
}

// Ловит: «+30» при закончившемся времени даёт предупреждение «за 30» (S9).
func TestWarn30AfterExtra30AtZero(t *testing.T) {
	c := &fakeClock{t: at(17, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 120 * 60}, "")
	c.Set(at(17, 5, 0))
	if _, err := e.AddExtra(30); err != nil {
		t.Fatal(err)
	}
	got := warnEvLog(e, c, at(17, 5, 0), at(17, 35, 5), activeSample())
	want := []string{"17:20:00 15 limit", "17:30:00 5 limit", "17:34:00 1 limit"}
	if !slices.Equal(got, want) {
		t.Fatalf("после +30: %v, want %v", got, want)
	}
}

// Ловит: копилка, отодвинувшая конец за 30 мин, не взводит «30» снова или взводит его дважды.
func TestWarn30BonusMovesEventPast30(t *testing.T) {
	c := &fakeClock{t: at(16, 20, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 80 * 60}, "")
	first := warnEvLog(e, c, at(16, 20, 0), at(16, 32, 0), activeSample())
	if want := []string{"16:30:00 30 limit"}; !slices.Equal(first, want) {
		t.Fatalf("до копилки: %v, want %v", first, want)
	}
	c.Set(at(16, 32, 0)) // осталось 28 мин
	if _, err := e.SpendBonus(5); err != nil {
		t.Fatal(err)
	}
	got := warnEvLog(e, c, at(16, 32, 0), at(17, 5, 5), activeSample())
	want := []string{
		"16:35:00 30 limit",
		"16:50:00 15 limit",
		"17:00:00 5 limit",
		"17:04:00 1 limit",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("после копилки +5: %v, want %v", got, want)
	}
}

// Ловит: во время паузы на 1 ч нет порога 30 до её конца (S11, G11).
func TestWarn30Pause1h(t *testing.T) {
	r := rulesMon(300)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 10, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 90 * 60}, "")
	if _, err := e.Pause(at(22, 10, 0)); err != nil {
		t.Fatal(err)
	}
	got := warnEvLog(e, c, at(21, 10, 0), at(22, 10, 2), activeSample())
	want := []string{
		"21:40:00 30 pause_end",
		"21:55:00 15 pause_end",
		"22:05:00 5 pause_end",
		"22:09:00 1 pause_end",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("пауза 1 ч: %v, want %v", got, want)
	}
}

// Ловит: перед запретными часами нет порога 30 или вид события не «запретные часы» (S8).
func TestWarn30ForbiddenIn35Min(t *testing.T) {
	r := rulesMon(600)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(20, 25, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 60 * 60}, "")
	got := warnEvLog(e, c, at(20, 25, 0), at(21, 0, 2), activeSample())
	want := []string{
		"20:30:00 30 forbidden",
		"20:45:00 15 forbidden",
		"20:55:00 5 forbidden",
		"20:59:00 1 forbidden",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("перед запретными часами: %v, want %v", got, want)
	}
}

// Ловит: порог, сработавший ровно на границе, взводится снова сохранением тех же правил в ту же
// секунду и показывается второй раз (класс A, дубль уведомления). Запретные часы: дельта по
// настоящим часам; лимит: ребёнок отошёл, остаток стоит.
func TestWarnNoRepeatAfterSameSecondRearm(t *testing.T) {
	t.Run("запретные часы", func(t *testing.T) {
		r := rulesMon(600)
		r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
		c := &fakeClock{t: at(20, 29, 58)}
		e := NewEngine(c, msk, r, DayState{Day: monday}, "")
		got := warnEvLog(e, c, at(20, 29, 58), at(20, 30, 1), activeSample())
		if want := []string{"20:30:00 30 forbidden"}; !slices.Equal(got, want) {
			t.Fatalf("до сохранения правил: %v, want %v", got, want)
		}
		c.Set(at(20, 30, 0))
		e.SetRules(r)
		got = warnEvLog(e, c, at(20, 30, 1), at(20, 44, 0), activeSample())
		if len(got) != 0 {
			t.Fatalf("после сохранения тех же правил повтор: %v", got)
		}
	})
	t.Run("лимит, время стоит", func(t *testing.T) {
		c := &fakeClock{t: at(16, 30, 0)}
		e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 90*60 - 1}, "")
		// 16:30:00 засчитана: остаток ровно 30:00. Дальше ребёнок отошёл, остаток стоит.
		if got := warnEvLog(e, c, at(16, 30, 0), at(16, 30, 1), activeSample()); len(got) != 0 {
			t.Fatalf("при остатке 30:01: %v", got)
		}
		got := warnEvLog(e, c, at(16, 30, 1), at(16, 30, 2), idleSample())
		if want := []string{"16:30:01 30 limit"}; !slices.Equal(got, want) {
			t.Fatalf("до сохранения правил: %v, want %v", got, want)
		}
		e.SetRules(rulesMon(120))
		got = warnEvLog(e, c, at(16, 30, 2), at(16, 31, 0), idleSample())
		if len(got) != 0 {
			t.Fatalf("после сохранения тех же правил при стоящем остатке повтор: %v", got)
		}
	})
	t.Run("порог ещё не сработал", func(t *testing.T) {
		// Правила сохранены в ту секунду, когда остаток стал ровно 30:00, а тика с порогом ещё не
		// было: порог не теряется (он был взведён, событие не сдвинулось).
		c := &fakeClock{t: at(16, 30, 0)}
		e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 90*60 - 1}, "")
		warnEvLog(e, c, at(16, 30, 0), at(16, 30, 1), activeSample())
		e.SetRules(rulesMon(120))
		got := warnEvLog(e, c, at(16, 30, 1), at(16, 30, 3), activeSample())
		if want := []string{"16:30:01 30 limit"}; !slices.Equal(got, want) {
			t.Fatalf("порог потерян или повторён: %v, want %v", got, want)
		}
	})
}

// Ловит: обход и снятие блокировки без сдвига события дают повтор порога.
func TestWarnNoRepeatAfterBypassAndClear(t *testing.T) {
	c := &fakeClock{t: at(16, 29, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 89 * 60}, "")
	got := warnEvLog(e, c, at(16, 29, 0), at(16, 30, 1), activeSample())
	if want := []string{"16:30:00 30 limit"}; !slices.Equal(got, want) {
		t.Fatalf("до обхода: %v, want %v", got, want)
	}
	c.Set(at(16, 30, 1))
	e.MarkBypass("тест")
	e.ClearBlock()
	got = warnEvLog(e, c, at(16, 30, 1), at(16, 44, 0), activeSample())
	if len(got) != 0 {
		t.Fatalf("после обхода и снятия блокировки повтор: %v", got)
	}
}
