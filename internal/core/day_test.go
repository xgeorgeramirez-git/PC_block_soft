package core

import (
	"testing"
	"time"
)

// Ловит: остаток считается без доп. времени, уходит в минус на экранах, лимит 0
// не даёт нулевой остаток, «без лимита» превращается в ноль (R4).
func TestRemainingTotalS(t *testing.T) {
	cases := []struct {
		name          string
		limit         DayLimit
		extraMin      int
		totalS        int
		wantRemaining int
		wantUnlimited bool
	}{
		{"без лимита", DayLimit{}, 0, 5000, 0, true},
		{"120 мин, ничего не использовано", LimitOf(120), 0, 0, 7200, false},
		{"120 мин, осталась секунда", LimitOf(120), 0, 7199, 1, false},
		{"120 мин, всё использовано", LimitOf(120), 0, 7200, 0, false},
		{"120 мин и доп. 30 мин, использовано 120", LimitOf(120), 30, 7200, 1800, false},
		{"лимит 0", LimitOf(0), 0, 0, 0, false},
		{"лимит уменьшили ниже использованного", LimitOf(10), 0, 900, 0, false},
	}
	for _, tc := range cases {
		rem, unl := RemainingTotalS(tc.limit, tc.extraMin, tc.totalS)
		if rem != tc.wantRemaining || unl != tc.wantUnlimited {
			t.Errorf("%s: RemainingTotalS = (%d, %v), want (%d, %v)", tc.name, rem, unl, tc.wantRemaining, tc.wantUnlimited)
		}
	}
}

// Ловит: дата дня берётся в UTC, а не по местному времени (ТЗ 4), и секунды после
// местной полуночи уходят во вчерашний день.
func TestDayOf(t *testing.T) {
	if got := DayOf(midnight.Add(-time.Second)); got != friday {
		t.Fatalf("23:59:59 → %s, want %s", got, friday)
	}
	if got := DayOf(midnight); got != saturday {
		t.Fatalf("00:00:00 → %s, want %s", got, saturday)
	}
	// Та же полночь по UTC — ещё пятница 21:00, но день — по поясу момента.
	if got := DayOf(midnight.UTC()); got != friday {
		t.Fatalf("21:00 UTC → %s, want %s", got, friday)
	}
}
