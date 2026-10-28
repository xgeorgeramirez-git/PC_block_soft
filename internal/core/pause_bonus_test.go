package core

import (
	"slices"
	"testing"
	"time"
)

// Ловит: пауза не снимает блокировку, учёт во время паузы не идёт, предупреждения не на
// конец паузы, блокировка не возвращается по окончании (R11, G11).
func TestEnginePauseLikeG11(t *testing.T) {
	// Лимит 300 мин, интервал пн 21:00–07:00. Блокировка в 21:00.
	r := rulesMon(300)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 90 * 60}, "")

	// В 21:00 — блокировка по интервалу.
	if d := e.Tick(activeSample()); d.Lock != LockForbidden {
		t.Fatalf("в 21:00 Lock = %v, want LockForbidden", d.Lock)
	}
	// В 21:10 родитель ставит паузу на 1 ч (до 22:10).
	c.Set(at(21, 10, 0))
	d, err := e.Pause(at(22, 10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if d.Lock != LockNone || d.Status != StatusPaused {
		t.Fatalf("после паузы Lock=%v Status=%v, want LockNone/StatusPaused", d.Lock, d.Status)
	}
	if d.PauseUntil != "22:10" {
		t.Fatalf("PauseUntil = %q, want 22:10", d.PauseUntil)
	}
	// Событие паузы в журнал.
	if !slices.Contains(kinds(d.Events), EvPause) {
		t.Fatalf("нет события pause: %v", kinds(d.Events))
	}
	// Учёт идёт во время паузы: 21:10…22:10 добавляет 3600 с к общему.
	log := warnEvLog(e, c, at(21, 10, 0), at(22, 10, 2), activeSample())
	want := []string{
		"21:40:00 30 pause_end",
		"21:55:00 15 pause_end",
		"22:05:00 5 pause_end",
		"22:09:00 1 pause_end",
	}
	if !slices.Equal(log, want) {
		t.Fatalf("предупреждения конца паузы %v, want %v", log, want)
	}
	// После 22:10 блокировка интервала вернулась.
	c.Set(at(22, 10, 1))
	if d := e.Current(); d.Lock != LockForbidden {
		t.Fatalf("после паузы в 22:10 Lock = %v, want LockForbidden (интервал)", d.Lock)
	}
	// Итог: за 21:10–22:10 в общее добавлено 3600 с (90*60 + 3600 = 9000).
	st, _ := e.State()
	if st.TotalS != 90*60+3600 {
		t.Fatalf("общее время = %d, want %d (учёт идёт в паузу)", st.TotalS, 90*60+3600)
	}
}

// Ловит: трата из копилки не снимает блокировку, не добавляет доп. время, не взводит
// предупреждения заново (R10, FR-11, G8).
func TestEngineSpendBonusLikeG8(t *testing.T) {
	// Лимит 120 мин, время вышло в 17:00; интервал пн 21:00–07:00.
	r := rulesMon(120)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(17, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 120 * 60}, "")
	if d := e.Current(); d.Lock != LockTimeUp {
		t.Fatalf("в 17:00 Lock = %v, want LockTimeUp", d.Lock)
	}
	// В 18:00 ребёнок тратит 30 мин из копилки (служба уже проверила баланс).
	c.Set(at(18, 0, 0))
	d, err := e.SpendBonus(30)
	if err != nil {
		t.Fatal(err)
	}
	if d.Lock != LockNone {
		t.Fatalf("после траты Lock = %v, want LockNone", d.Lock)
	}
	if !slices.Contains(kinds(d.Events), EvBonusSpent) {
		t.Fatalf("нет события bonus_spent: %v", kinds(d.Events))
	}
	st, _ := e.State()
	if st.ExtraMin != 30 {
		t.Fatalf("ExtraMin = %d, want 30", st.ExtraMin)
	}
	// Новый конец — 18:30; предупреждения 18:15/18:25/18:29.
	log := warnEvLog(e, c, at(18, 0, 0), at(18, 30, 2), activeSample())
	want := []string{
		"18:15:00 15 limit",
		"18:25:00 5 limit",
		"18:29:00 1 limit",
	}
	if !slices.Equal(log, want) {
		t.Fatalf("предупреждения после траты %v, want %v", log, want)
	}
	c.Set(at(18, 30, 1))
	if d := e.Current(); d.Lock != LockTimeUp {
		t.Fatalf("в 18:30 Lock = %v, want LockTimeUp", d.Lock)
	}
}

// Ловит: пауза «до конца дня» не сбрасывается в полночь (R11, R16).
func TestEnginePauseEndsAtMidnight(t *testing.T) {
	r := rulesMon(60)
	c := &fakeClock{t: at(23, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 60 * 60}, "")
	// Пауза до конца дня = местная полночь следующего дня (вт 00:00).
	eod := time.Date(2026, 10, 20, 0, 0, 0, 0, msk)
	if _, err := e.Pause(eod); err != nil {
		t.Fatal(err)
	}
	// 23:30 — пауза активна.
	c.Set(at(23, 30, 0))
	if d := e.Current(); d.Status != StatusPaused {
		t.Fatalf("в 23:30 статус %v, want StatusPaused", d.Status)
	}
	// После полуночи — новый день, пауза сброшена, состояние чистое.
	c.Set(time.Date(2026, 10, 20, 0, 0, 30, 0, msk))
	e.Tick(activeSample())
	st, _ := e.State()
	if st.PauseUntilMs != nil {
		t.Fatalf("после смены суток пауза не сброшена: %v", *st.PauseUntilMs)
	}
	if st.Day != tuesday {
		t.Fatalf("день %s, want %s", st.Day, tuesday)
	}
}
