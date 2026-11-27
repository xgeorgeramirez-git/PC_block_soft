package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// promoRule — строка общей таблицы правил кода R26 для Go и Kotlin (testdata/remote/promo-rules.json):
// ввод, принят ли, код для хранения или текст ошибки окна.
type promoRule struct {
	In    string `json:"in"`
	OK    bool   `json:"ok"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

func readPromoRules(t *testing.T) []promoRule {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "remote", "promo-rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rules []promoRule
	if err := json.Unmarshal(raw, &rules); err != nil || len(rules) == 0 {
		t.Fatalf("таблица правил: %v", err)
	}
	return rules
}

// Ловит: правила кода в Go разошлись с общей таблицей, по которой телефон проверяет поле сам
// (протокол 5г: те же правила и тексты, что окно). Текст ошибки — как у «Создать код» окна:
// пусто — «Введите код»; только латиница и цифры, но не та длина — «Код от 3 до 16 знаков»;
// иначе — «Только латинские буквы A-Z и цифры».
func TestPromoRulesVector(t *testing.T) {
	for _, r := range readPromoRules(t) {
		code, ok := CheckNewPromoCode(r.In)
		errText := ""
		switch {
		case ok:
		case strings.TrimSpace(r.In) == "":
			errText = "Введите код"
		case PromoCodeLatinOnly(r.In):
			errText = "Код от 3 до 16 знаков"
		default:
			errText = "Только латинские буквы A-Z и цифры"
		}
		if ok != r.OK || ok && code != r.Code || errText != r.Error {
			t.Errorf("%q: ok %v код %q ошибка %q; в таблице %+v", r.In, ok, code, errText, r)
		}
	}
}
