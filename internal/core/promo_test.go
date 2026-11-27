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

// Ловит (R26, D-75, RM5-11): новый код с кириллицей, полноширинными буквами, пробелом или знаками
// внутри принят; длина вне 3–16; код показан не заглавными.
func TestCheckNewPromoCode(t *testing.T) {
	good := map[string]string{
		"ab12":             "AB12",
		" A1234 ":          "A1234",
		"a1z":              "A1Z",              // 3 знака — можно
		"abcdefgh12345678": "ABCDEFGH12345678", // 16 знаков — можно
		"0000":             "0000",
	}
	for in, want := range good {
		got, ok := CheckNewPromoCode(in)
		if !ok || got != want {
			t.Errorf("CheckNewPromoCode(%q) = %q, %v; ожидалось %q, true", in, got, ok, want)
		}
	}
	bad := []string{
		"ПРИВЕТ",            // кириллица
		"ab 12",             // пробел внутри
		"код!",              // кириллица и знак
		"ab-12",             // дефис
		"ＡＢ１２",              // полноширинные буквы и цифры
		"ab",                // 2 знака
		"abcdefgh123456789", // 17 знаков
		"ﬁx12",              // лигатура: ToUpper дал бы «FIX12»
		"",
		"   ",
	}
	for _, in := range bad {
		if got, ok := CheckNewPromoCode(in); ok {
			t.Errorf("CheckNewPromoCode(%q) = %q, true; ожидался отказ", in, got)
		}
	}
}

// Ловит (R26): код не той длины из допустимых знаков получает текст «Только латинские буквы»
// (служба различает длину и знаки по PromoCodeLatinOnly); пустой код считается латинским.
func TestPromoCodeLatinOnly(t *testing.T) {
	for _, in := range []string{"ab", " ab ", "a1z", "abcdefgh123456789", "Z"} {
		if !PromoCodeLatinOnly(in) {
			t.Errorf("PromoCodeLatinOnly(%q) = false; ожидалось true", in)
		}
	}
	for _, in := range []string{"ПРИВЕТ", "ab 12", "a!", "ＡＢ１２", "ﬁx12", "", "   "} {
		if PromoCodeLatinOnly(in) {
			t.Errorf("PromoCodeLatinOnly(%q) = true; ожидалось false", in)
		}
	}
}
