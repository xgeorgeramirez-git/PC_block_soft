package core

import (
	"errors"
	"slices"
	"testing"
)

// pauseEndEvent ищет событие конца паузы в решении.
func pauseEndEvent(d Decision) (Event, bool) {
	for _, ev := range d.Events {
		if ev.Kind == EvPauseEnd {
			return ev, true
		}
	}
	return Event{}, false
}

// Ловит: EndPause не снимает паузу или не возвращает блокировку «время вышло» сразу
// (R11, D-96); событие конца паузы без отметки «раньше срока».
func TestEndPauseTimeUpLocksAtOnce(t *testing.T) {
	c := &fakeClock{t: at(17, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 120 * 60}, "")
	if d := e.Tick(activeSample()); d.Lock != LockTimeUp {
		t.Fatalf("в 17:00 Lock = %v, want LockTimeUp", d.Lock)
	}
	c.Set(at(17, 5, 0))
	if d, err := e.Pause(at(18, 5, 0)); err != nil || d.Lock != LockNone {
		t.Fatalf("Pause: Lock=%v err=%v", d.Lock, err)
	}
	c.Set(at(17, 20, 0))
	d, err := e.EndPause()
	if err != nil {
		t.Fatalf("EndPause: %v", err)
	}
	if d.Lock != LockTimeUp || d.Status != StatusTimeUp {
		t.Fatalf("после EndPause Lock=%v Status=%v, want LockTimeUp/StatusTimeUp", d.Lock, d.Status)
	}
	if d.PauseUntil != "" {
		t.Fatalf("PauseUntil = %q, want пусто", d.PauseUntil)
	}
	ev, ok := pauseEndEvent(d)
	if !ok {
		t.Fatalf("нет события pause_end: %v", kinds(d.Events))
	}
	if ev.Details["early"] != true || ev.Details["text"] != "Пауза закончена раньше срока" {
		t.Fatalf("подробности конца паузы %v, want early=true и текст «Пауза закончена раньше срока»", ev.Details)
	}
	st, _ := e.State()
	if st.PauseUntilMs != nil {
		t.Fatalf("PauseUntilMs = %v, want nil", *st.PauseUntilMs)
	}
	// Следующий тик: блокировка держится, второго события конца паузы нет.
	c.Set(at(17, 20, 1))
	d = e.Tick(activeSample())
	if d.Lock != LockTimeUp {
		t.Fatalf("тик после EndPause Lock = %v, want LockTimeUp", d.Lock)
	}
	if _, dup := pauseEndEvent(d); dup {
		t.Fatalf("второе событие pause_end на тике: %v", kinds(d.Events))
	}
}

// Ловит: в запретные часы EndPause не возвращает экран интервала сразу (R11, S11).
func TestEndPauseForbiddenLocksAtOnce(t *testing.T) {
	r := rulesMon(300)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(21, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 90 * 60}, "")
	if d := e.Tick(activeSample()); d.Lock != LockForbidden {
		t.Fatalf("в 21:00 Lock = %v, want LockForbidden", d.Lock)
	}
	c.Set(at(21, 10, 0))
	if _, err := e.Pause(at(22, 10, 0)); err != nil {
		t.Fatal(err)
	}
	c.Set(at(21, 30, 0))
	d, err := e.EndPause()
	if err != nil {
		t.Fatal(err)
	}
	if d.Lock != LockForbidden {
		t.Fatalf("после EndPause Lock = %v, want LockForbidden", d.Lock)
	}
}

// Ловит: EndPause при остатке ставит блокировку или оставляет статус «пауза».
func TestEndPauseWithTimeLeftNoLock(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 30 * 60}, "")
	e.Tick(activeSample())
	if _, err := e.Pause(at(16, 0, 0)); err != nil {
		t.Fatal(err)
	}
	c.Set(at(15, 10, 0))
	d, err := e.EndPause()
	if err != nil {
		t.Fatal(err)
	}
	if d.Lock != LockNone || d.Status == StatusPaused {
		t.Fatalf("Lock=%v Status=%v, want LockNone и не пауза", d.Lock, d.Status)
	}
	if d.RemainingS <= 0 {
		t.Fatalf("RemainingS = %d, want > 0", d.RemainingS)
	}
}

// Ловит: EndPause без паузы (не ставили или истекла) молча проходит или пишет событие.
func TestEndPauseWithoutPause(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday}, "")
	e.Tick(activeSample())
	d, err := e.EndPause()
	if !errors.Is(err, ErrNoPause) {
		t.Fatalf("err = %v, want ErrNoPause", err)
	}
	if slices.Contains(kinds(d.Events), EvPauseEnd) {
		t.Fatalf("событие pause_end без паузы: %v", kinds(d.Events))
	}
	// Истёкшая пауза: тоже «паузы нет».
	if _, err := e.Pause(at(15, 15, 0)); err != nil {
		t.Fatal(err)
	}
	c.Set(at(15, 16, 0))
	e.Tick(activeSample())
	if _, err := e.EndPause(); !errors.Is(err, ErrNoPause) {
		t.Fatalf("истёкшая пауза: err = %v, want ErrNoPause", err)
	}
}

// Ловит: после EndPause новая пауза не ставится или не снимает блокировку (D-96: значок
// включается снова).
func TestPauseAgainAfterEndPause(t *testing.T) {
	c := &fakeClock{t: at(17, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 120 * 60}, "")
	e.Tick(activeSample())
	if _, err := e.Pause(at(18, 0, 0)); err != nil {
		t.Fatal(err)
	}
	c.Set(at(17, 10, 0))
	if _, err := e.EndPause(); err != nil {
		t.Fatal(err)
	}
	c.Set(at(17, 11, 0))
	d, err := e.Pause(at(18, 11, 0))
	if err != nil {
		t.Fatal(err)
	}
	if d.Lock != LockNone || d.Status != StatusPaused || d.PauseUntil != "18:11" {
		t.Fatalf("повторная пауза Lock=%v Status=%v until=%q", d.Lock, d.Status, d.PauseUntil)
	}
	// Конец повторной паузы по сроку: обычное событие без «раньше срока», блокировка вернулась.
	c.Set(at(18, 11, 1))
	d = e.Tick(activeSample())
	if d.Lock != LockTimeUp {
		t.Fatalf("после срока Lock = %v, want LockTimeUp", d.Lock)
	}
	ev, ok := pauseEndEvent(d)
	if !ok || ev.Details["early"] == true || ev.Details["text"] != PauseEndText {
		t.Fatalf("конец по сроку: %v ok=%v", ev.Details, ok)
	}
}
