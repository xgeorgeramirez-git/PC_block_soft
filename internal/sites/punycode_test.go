package sites

import (
	"strings"
	"testing"
)

// Векторы RFC 3492, раздел 7.1 (сверены с кодеком Python). Регистр в RFC — флаги, поэтому
// сравнение без учёта регистра.
func TestEncodePunycodeRFC3492(t *testing.T) {
	cases := []struct{ in, want string }{
		{"他们为什么不说中文", "ihqwcrb4cv8a8dqg056pqjye"},
		{"Pročprostěnemluvíčesky", "Proprostnemluvesky-uyb24dma41a"},
		{"почемужеонинеговорятпорусски", "b1abfaaepdrnnbgefbadotcwatmq2g4l"},
		{"PorquénopuedensimplementehablarenEspañol", "PorqunopuedensimplementehablarenEspaol-fmd56a"},
		{"3年B組金八先生", "3B-ww4c5e180e575a65lsy2b"},
		{"なぜみんな日本語を話してくれないのか", "n8jok5ay5dzabd5bym9f0cm5685rrjetr6pdxa"},
		{"-> $1.00 <-", "-> $1.00 <--"},
		// Не из RFC: вектор, на котором видна ошибка в постоянной damp (сверен с кодеком Python).
		{"功文", "xfrw35b"},
	}
	for _, c := range cases {
		got, ok := encodePunycode([]rune(c.in))
		if !ok || !strings.EqualFold(got, c.want) {
			t.Errorf("encodePunycode(%q) = %q, %v; want %q", c.in, got, ok, c.want)
		}
	}
}

func TestToASCII(t *testing.T) {
	cases := []struct{ in, want string }{
		{"пример", "xn--e1afmkfd"},
		{"рф", "xn--p1ai"},
		{"example", "example"},
		{"xn--p1ai", "xn--p1ai"},
		{"ПРИМЕР", "xn--e1afmkfd"},
	}
	for _, c := range cases {
		if got, ok := toASCII(c.in); !ok || got != c.want {
			t.Errorf("toASCII(%q) = %q, %v; want %q", c.in, got, ok, c.want)
		}
	}
}
