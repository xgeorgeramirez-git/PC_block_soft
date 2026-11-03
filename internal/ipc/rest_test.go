package ipc

import (
	"testing"

	"pcblock/internal/core"
)

// Ловит: экран перерыва уходит стражу как «нет экрана» или без конца и сообщения (R27, 8.2.3 п.12);
// плашка отдыха без звука или с видом события лимита (страж показал бы плашку 1 мин).
func TestRestStatusAndWarning(t *testing.T) {
	st := ChildStatusFrom("Миша", core.Decision{Status: core.StatusOK, Lock: core.LockRest, Reason: core.ReasonRest,
		Until: "Можно снова в 16:20", RestUntilMs: 1792419600000, RestMessage: "Пойди погуляй с собакой"})
	if st.Lock != "rest" || st.RestUntilMs != 1792419600000 || st.RestMessage != "Пойди погуляй с собакой" || st.Until != "Можно снова в 16:20" {
		t.Fatalf("статус перерыва %+v", st)
	}
	w := NewGuardRestWarning(60, "Через 1 минуту перерыв на 5 минут. Сохрани игру")
	if w != (GuardWarning{Threshold: 1, Text: "Через 1 минуту перерыв на 5 минут. Сохрани игру", Sound: true, Event: "rest", EventS: 60}) {
		t.Fatalf("плашка отдыха %+v", w)
	}
	b := BreakRules{Enabled: true, AfterMin: 60, Counts: "games", WarnMin: 5, RestMin: 15, SelfRestMin: 30, Message: "м"}
	if got := BreakRulesFrom(b.Core()); got != b {
		t.Fatalf("туда и обратно %+v, want %+v", got, b)
	}
}
