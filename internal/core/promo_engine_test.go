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

// Ловит (регресс S36 в M3): промокод расширяет только общий лимит, а лимит игр остаётся прежним
// (R26: минуты промокода увеличивают и общий лимит, и лимит игр, как доп. время R10), или игры,
// закрытые по концу лимита игр, после промокода не открываются.
func TestRedeemPromoMinutesExtendsGames(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	// Общий лимит 120 мин, игры 30 мин; уже сыграно 30 мин игр, всего 30 мин.
	e := NewEngine(c, msk, gamesMon(intp(120), intp(30), EndBlock), DayState{Day: monday}, "")
	run(e, c, at(15, 0, 0), at(15, 30, 0), gameSample())
	if cur := e.Current(); !cur.CloseGames || cur.GamesRemainingS != 0 {
		t.Fatalf("до промокода: закрыты игры %v, остаток игр %d; want true, 0", cur.CloseGames, cur.GamesRemainingS)
	}
	d, err := e.RedeemPromoMinutes(20)
	if err != nil {
		t.Fatalf("RedeemPromoMinutes(20): %v", err)
	}
	if d.CloseGames || d.GamesRemainingS != 1200 || d.GamesLimitS != 3000 || d.LimitS != 8400 {
		t.Fatalf("после промокода 20 мин: закрыты игры %v, остаток игр %d, лимит игр %d, общий %d; want false, 1200, 3000, 8400",
			d.CloseGames, d.GamesRemainingS, d.GamesLimitS, d.LimitS)
	}
}
