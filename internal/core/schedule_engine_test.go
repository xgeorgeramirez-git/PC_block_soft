package core

import (
	"slices"
	"testing"
	"time"
)

// warnEvLog тикает в [from,to) и собирает «момент порог вид» для проверки R9 на разные
// события (исчерпание лимита, интервал, конец паузы).
func warnEvLog(e *Engine, c *fakeClock, from, to time.Time, s SessionSample) []string {
	out := []string{}
	for t := from; t.Before(to); t = t.Add(time.Second) {
		c.Set(t)
		d := e.Tick(s)
		if d.Warning != 0 {
			out = append(out, t.Format(time.TimeOnly)+" "+itoa(d.Warning)+" "+d.WarningEvent.String())
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// Ловит: блокировка по запретному интервалу не ставится, не та причина, или остаток
// лимита мешает (R7, G6).
func TestEngineForbiddenInterval(t *testing.T) {
	// Лимит 180 мин, интервал пн 21:00–07:00. Остаток в интервале не важен.
	r := rulesMon(180)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 130 * 60}, "")
	d := e.Tick(activeSample())
	if d.Lock != LockForbidden {
		t.Fatalf("Lock = %v, want LockForbidden", d.Lock)
	}
	if d.Reason != "Сейчас нельзя пользоваться компьютером" {
		t.Fatalf("Reason = %q", d.Reason)
	}
	if !contains(d.Until, "07:00") {
		t.Fatalf("Until = %q, want содержит 07:00", d.Until)
	}
}

// Ловит: предупреждения R9 взводятся на исчерпание лимита, а не на более ранний интервал
// (G6: интервал в 21:00 ближе, чем исчерпание в 21:20 при остатке 50 мин).
func TestEngineWarnsOnNearestEventInterval(t *testing.T) {
	r := rulesMon(180)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	// К 20:30 использовано 130 мин (остаток 50 → исчерпание в 21:20). Интервал в 21:00 раньше.
	c := &fakeClock{t: at(20, 30, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 130 * 60}, "")
	got := warnEvLog(e, c, at(20, 30, 0), at(21, 0, 2), activeSample())
	want := []string{
		"20:45:00 15 forbidden",
		"20:55:00 5 forbidden",
		"20:59:00 1 forbidden",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("предупреждения %v, want %v", got, want)
	}
}

// Ловит: при исчерпании лимита раньше интервала вид события — лимит.
func TestEngineWarnsOnNearestEventLimit(t *testing.T) {
	r := rulesMon(120)
	r.Schedule = []Interval{iv(Monday, 22*60, 7*60)} // интервал в 22:00, позже исчерпания в 17:00
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday}, "")
	got := warnEvLog(e, c, at(15, 0, 0), at(17, 0, 2), activeSample())
	want := []string{
		"16:45:00 15 limit",
		"16:55:00 5 limit",
		"16:59:00 1 limit",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("предупреждения %v, want %v", got, want)
	}
}

// Ловит: приоритет блокировок интервал > время вышло; обход не блокирует (D-41).
func TestEngineLockPriority(t *testing.T) {
	r := rulesMon(1) // лимит почти исчерпан
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 30, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 3600}, "")
	// В интервале и лимит исчерпан — должна быть блокировка интервала, не «время вышло».
	d := e.Current()
	if d.Lock != LockForbidden {
		t.Fatalf("в интервале при исчерпанном лимите Lock = %v, want LockForbidden", d.Lock)
	}
	// Обход больше не блокирует (D-41): он лишь пишет событие в журнал, блокировка интервала остаётся.
	e.MarkBypass("тест")
	if d := e.Current(); d.Lock != LockForbidden {
		t.Fatalf("обход не должен менять блокировку: Lock = %v, want LockForbidden (D-41)", d.Lock)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
