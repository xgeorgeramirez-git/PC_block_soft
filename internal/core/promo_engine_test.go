package core

import (
	"errors"
	"testing"
)

// Ловит: промокод не добавляет время всем лимитам, не снимает блокировку «время вышло», или
// его диапазон ошибочно обрезан до 240 (как у «Попросить ещё») вместо 600.
func TestRedeemPromoMinutesAddsTimeLikeExtra(t *testing.T) {
	r := rulesMon(120) // общий лимит 120 мин
	c := &fakeClock{t: at(12, 0, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 120 * 60}, "") // время вышло

	if d := e.Current(); d.Lock == LockNone {
		t.Fatalf("до промокода: Lock=%v; ждали блокировку «время вышло»", d.Lock)
	}
	// Промокод на 300 мин (больше 240) — применяется целиком (R26 допускает до 600).
	d, err := e.RedeemPromoMinutes(300)
	if err != nil {
		t.Fatalf("RedeemPromoMinutes(300): %v", err)
	}
	if d.Lock != LockNone {
		t.Errorf("после промокода Lock=%v; ждали снятие блокировки (время появилось)", d.Lock)
	}
	if !hasEventKind(d.Events, EvPromoRedeem) {
		t.Errorf("нет события %q среди %+v", EvPromoRedeem, d.Events)
	}
	if st, _ := e.State(); st.ExtraMin != 300 {
		t.Errorf("доп. время после промокода = %d; want 300", st.ExtraMin)
	}
}

// Ловит: минуты промокода вне 1..600 принимаются (расходится с CHECK схемы и R26).
func TestRedeemPromoMinutesRange(t *testing.T) {
	c := &fakeClock{t: at(12, 0, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday}, "")
	for _, min := range []int{0, 601, -5} {
		if _, err := e.RedeemPromoMinutes(min); !errors.Is(err, ErrPromoRange) {
			t.Errorf("RedeemPromoMinutes(%d) err=%v; want ErrPromoRange", min, err)
		}
	}
}
