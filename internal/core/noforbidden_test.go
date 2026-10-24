package core

import (
	"testing"
	"time"
)

func hasEventKind(evs []Event, kind string) bool {
	for _, e := range evs {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

// Ловит: «снять запрет на сегодня» (R24, D-51) не убирает блокировку запретных часов,
// не пишет событие, или не держится после тика.
func TestEngineNoForbiddenToday(t *testing.T) {
	r := rulesMon(180)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 130 * 60}, "")

	if d := e.Tick(activeSample()); d.Lock != LockForbidden {
		t.Fatalf("до снятия: Lock = %v, want LockForbidden", d.Lock)
	}
	d := e.SetNoForbiddenToday(true)
	if d.Lock != LockNone {
		t.Fatalf("после снятия: Lock = %v, want LockNone", d.Lock)
	}
	if !hasEventKind(d.Events, EvNoForbidden) {
		t.Fatalf("нет события %s", EvNoForbidden)
	}
	if d2 := e.Tick(activeSample()); d2.Lock != LockNone {
		t.Fatalf("тик после снятия: Lock = %v, want LockNone", d2.Lock)
	}
}

// Ловит: снятие запрета ошибочно снимает и блокировку «время вышло» — лимит времени
// должен действовать и при снятом запрете (R24).
func TestEngineNoForbiddenKeepsTimeLimit(t *testing.T) {
	r := rulesMon(120)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 120 * 60}, "") // время вышло
	d := e.SetNoForbiddenToday(true)
	if d.Lock != LockTimeUp {
		t.Fatalf("Lock = %v, want LockTimeUp (лимит действует даже при снятом запрете)", d.Lock)
	}
}

// Ловит: флаг «запрет снят» не сбрасывается в новый день (R24: привязан к дню).
func TestEngineNoForbiddenResetsNextDay(t *testing.T) {
	r := rulesMon(180)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60), iv(Tuesday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 0}, "")
	e.SetNoForbiddenToday(true)
	if d := e.Tick(activeSample()); d.Lock != LockNone {
		t.Fatalf("пн после снятия: Lock = %v, want LockNone", d.Lock)
	}
	c.Set(time.Date(2026, 10, 20, 21, 0, 0, 0, msk)) // вторник 21:00
	d := e.Tick(activeSample())
	if d.Lock != LockForbidden {
		t.Fatalf("вт: Lock = %v, want LockForbidden (флаг должен сброситься в новый день)", d.Lock)
	}
}
