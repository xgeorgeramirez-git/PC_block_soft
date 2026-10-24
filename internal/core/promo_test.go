package core

import "testing"

// Ловит: неверное сравнение промокода (R26) — регистр и пробелы должны игнорироваться.
func TestNormalizePromoCode(t *testing.T) {
	cases := map[string]string{
		"a1234":     "a1234",
		"A1234":     "a1234",
		"  a1234  ": "a1234",
		"AbC-99":    "abc-99",
		"":          "",
	}
	for in, want := range cases {
		if got := NormalizePromoCode(in); got != want {
			t.Errorf("NormalizePromoCode(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

// Ловит: неверные границы минут промокода (R26, FR-35: 1…600).
func TestPromoMinutesValid(t *testing.T) {
	valid := []int{1, 30, 600}
	invalid := []int{0, -5, 601, 1000}
	for _, m := range valid {
		if !PromoMinutesValid(m) {
			t.Errorf("PromoMinutesValid(%d) = false, ожидалось true", m)
		}
	}
	for _, m := range invalid {
		if PromoMinutesValid(m) {
			t.Errorf("PromoMinutesValid(%d) = true, ожидалось false", m)
		}
	}
}

// Ловит: бонус поднимает копилку выше потолка 600 или уходит в минус (R25, R26).
func TestAddBonusWithinCeiling(t *testing.T) {
	cases := []struct {
		balance, add, want int
	}{
		{0, 30, 30},   // влезает целиком
		{580, 30, 20}, // до потолка только 20
		{600, 30, 0},  // уже у потолка
		{650, 10, 0},  // выше потолка (на всякий случай) — ничего
		{100, -5, 0},  // отрицательное начисление не вычитает
		{0, 600, 600}, // ровно потолок
		{0, 700, 600}, // больше потолка — только до 600
	}
	for _, c := range cases {
		if got := AddBonusWithinCeiling(c.balance, c.add); got != c.want {
			t.Errorf("AddBonusWithinCeiling(%d, %d) = %d, ожидалось %d", c.balance, c.add, got, c.want)
		}
	}
}
