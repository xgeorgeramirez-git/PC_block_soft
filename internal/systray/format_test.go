package systray

import (
	"image/color"
	"math"
	"testing"
	"unicode/utf16"
)

// Ловит: надпись у часов считается не по FR-7: минуты до 99 цифрами, от 100 минут
// целые часы вниз со знаком «ч» (125 мин это «2ч», а не «1ч» и не «2»), без лимита «∞»,
// отрицательное «0»; огромный остаток не даёт надпись длиннее трёх знаков (иначе она
// не поместится в значок 16 px).
func TestTrayLabel(t *testing.T) {
	cases := []struct {
		name      string
		min       int
		unlimited bool
		want      string
	}{
		{"ноль", 0, false, "0"},
		{"одна цифра", 7, false, "7"},
		{"порог предупреждения", 15, false, "15"},
		{"две цифры", 85, false, "85"},
		{"последние минуты", 99, false, "99"},
		{"сто минут это час", 100, false, "1ч"},
		{"почти два часа", 119, false, "1ч"},
		{"ровно два часа", 120, false, "2ч"},
		{"пример из плана", 125, false, "2ч"},
		{"почти три часа", 179, false, "2ч"},
		{"три часа", 180, false, "3ч"},
		{"десять часов", 600, false, "10ч"},
		{"сутки", 1440, false, "24ч"},
		{"предел надписи", 99 * 60, false, "99ч"},
		{"выше предела", 100 * 60, false, "99ч"},
		{"огромное значение", 1 << 40, false, "99ч"},
		{"минус одна", -1, false, "0"},
		{"минус много", -500, false, "0"},
		{"без лимита", 85, true, "∞"},
		{"без лимита и ноль", 0, true, "∞"},
		{"без лимита и минус", -10, true, "∞"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrayLabel(tc.min, tc.unlimited); got != tc.want {
				t.Errorf("TrayLabel(%d, %v) = %q, want %q", tc.min, tc.unlimited, got, tc.want)
			}
		})
	}
}

// Ловит: статус значка берётся не в том порядке: пауза важнее всего, потом блокировка,
// без лимита всегда обычный, 0 это «вышло», ≤ 15 мин янтарный, 16 уже обычный.
func TestTrayStatus(t *testing.T) {
	cases := []struct {
		name      string
		min       int
		unlimited bool
		locked    bool
		paused    bool
		want      Status
	}{
		{"обычный", 85, false, false, false, StatusNormal},
		{"16 минут ещё обычный", 16, false, false, false, StatusNormal},
		{"15 минут уже янтарный", 15, false, false, false, StatusLow},
		{"1 минута", 1, false, false, false, StatusLow},
		{"0 минут вышло", 0, false, false, false, StatusOut},
		{"минус вышло", -3, false, false, false, StatusOut},
		{"блокировка при большом остатке", 100, false, true, false, StatusOut},
		{"без лимита обычный", 0, true, false, false, StatusNormal},
		{"без лимита и малый остаток", 10, true, false, false, StatusNormal},
		{"блокировка за обход при безлимите", 0, true, true, false, StatusOut},
		{"пауза при большом остатке", 85, false, false, true, StatusPaused},
		{"пауза важнее нуля", 0, false, false, true, StatusPaused},
		{"пауза важнее блокировки", 0, false, true, true, StatusPaused},
		{"пауза при безлимите", 0, true, false, true, StatusPaused},
		{"пауза важнее всего", -5, true, true, true, StatusPaused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrayStatus(tc.min, tc.unlimited, tc.locked, tc.paused); got != tc.want {
				t.Errorf("TrayStatus(%d, unlimited=%v, locked=%v, paused=%v) = %v, want %v",
					tc.min, tc.unlimited, tc.locked, tc.paused, got, tc.want)
			}
		})
	}
}

// Ловит: цвета статусов не из палитры ТЗ 8.0 п.9 (графит #2B3138, янтарный #A15C00,
// красный #B3261E, зелёный #2C7A4B) или два статуса сливаются в один цвет (цвет носитель
// статуса, L-14); неизвестный статус не должен давать прозрачный значок.
func TestStatusColors(t *testing.T) {
	cases := []struct {
		st   Status
		want color.RGBA
	}{
		{StatusNormal, color.RGBA{0x2B, 0x31, 0x38, 0xFF}},
		{StatusLow, color.RGBA{0xA1, 0x5C, 0x00, 0xFF}},
		{StatusOut, color.RGBA{0xB3, 0x26, 0x1E, 0xFF}},
		{StatusPaused, color.RGBA{0x2C, 0x7A, 0x4B, 0xFF}},
		{StatusNotChild, color.RGBA{0x2B, 0x31, 0x38, 0xFF}},
		{Status(99), color.RGBA{0x2B, 0x31, 0x38, 0xFF}},
		{Status(-1), color.RGBA{0x2B, 0x31, 0x38, 0xFF}},
	}
	for _, tc := range cases {
		if got := tc.st.Color(); got != tc.want {
			t.Errorf("Status(%d).Color() = %v, want %v", int(tc.st), got, tc.want)
		}
	}
	distinct := []Status{StatusNormal, StatusLow, StatusOut, StatusPaused}
	for i := range distinct {
		for j := i + 1; j < len(distinct); j++ {
			if distinct[i].Color() == distinct[j].Color() {
				t.Errorf("статусы %v и %v одного цвета", distinct[i], distinct[j])
			}
		}
	}
}

// Ловит: имя статуса пустое или числом там, где оно попадает в журнал и в сообщения тестов.
func TestStatusString(t *testing.T) {
	cases := []struct {
		st   Status
		want string
	}{
		{StatusNormal, "normal"},
		{StatusLow, "low"},
		{StatusOut, "out"},
		{StatusPaused, "paused"},
		{StatusNotChild, "not-child"},
		{Status(42), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.st.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", int(tc.st), got, tc.want)
		}
	}
}

// wcagLuminance — относительная яркость по WCAG 2.x, считается в тесте независимо от кода.
func wcagLuminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func wcagContrast(a, b color.RGBA) float64 {
	la, lb := wcagLuminance(a), wcagLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Ловит: проверка контраста, которая проходит всегда (NFR-14): помощник считает на
// известных парах (белое на чёрном 21:1, #767676 на белом 4,54:1, #777777 уже 4,48:1).
func TestContrastHelperKnownPairs(t *testing.T) {
	white := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	cases := []struct {
		name     string
		fg       color.RGBA
		min, max float64
	}{
		{"чёрное на белом", color.RGBA{0, 0, 0, 0xFF}, 20.99, 21.01},
		{"#767676 на белом", color.RGBA{0x76, 0x76, 0x76, 0xFF}, 4.53, 4.55},
		{"#777777 на белом", color.RGBA{0x77, 0x77, 0x77, 0xFF}, 4.47, 4.49},
	}
	for _, tc := range cases {
		got := wcagContrast(tc.fg, white)
		if got < tc.min || got > tc.max {
			t.Errorf("%s: контраст %.3f вне [%.2f, %.2f]", tc.name, got, tc.min, tc.max)
		}
	}
}

// Ловит: белая надпись нечитаема на цвете статуса (NFR-14, контраст текста ≥ 4,5:1),
// например янтарь посветлел или красный ушёл в розовый.
func TestStatusColorContrastWithWhiteText(t *testing.T) {
	white := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for _, st := range []Status{StatusNormal, StatusLow, StatusOut, StatusPaused, StatusNotChild} {
		got := wcagContrast(st.Color(), white)
		if got < 4.5 {
			t.Errorf("статус %v: контраст белого на %v равен %.2f, нужно не меньше 4,5", st, st.Color(), got)
		}
	}
}

// Ловит: подсказка у значка не по ТЗ 8.2.1 п.2 и не в форматах ТЗ 4 («1 ч 05 мин», «40 мин»,
// лимит «2 ч»); блокировка, «без лимита» и не детская учётка перекрывают остаток;
// остаток 0 при лимите не пишется «Осталось 0 мин» (красный значок и такая подсказка спорят).
func TestTrayTooltip(t *testing.T) {
	cases := []struct {
		name      string
		remaining int
		limit     int
		unlimited bool
		locked    bool
		notChild  bool
		want      string
	}{
		{"пример из ТЗ", 5100, 7200, false, false, false, "Осталось 1 ч 25 мин из 2 ч"},
		{"меньше часа", 2400, 7200, false, false, false, "Осталось 40 мин из 2 ч"},
		{"час и пять минут", 3900, 7200, false, false, false, "Осталось 1 ч 05 мин из 2 ч"},
		{"полный лимит", 7200, 7200, false, false, false, "Осталось 2 ч 00 мин из 2 ч"},
		{"лимит с минутами", 2400, 5400, false, false, false, "Осталось 40 мин из 1 ч 30 мин"},
		{"лимит меньше часа", 600, 1200, false, false, false, "Осталось 10 мин из 20 мин"},
		{"неполная минута отбрасывается", 59, 7200, false, false, false, "Осталось 0 мин из 2 ч"},
		{"лимит неизвестен", 2400, 0, false, false, false, "Осталось 40 мин"},
		{"лимит меньше минуты", 2400, 30, false, false, false, "Осталось 40 мин"},
		{"без лимита", 0, 0, true, false, false, "Сегодня без ограничений по времени"},
		{"без лимита с числами", 5100, 7200, true, false, false, "Сегодня без ограничений по времени"},
		{"блокировка", 5100, 7200, false, true, false, "Время на сегодня закончилось"},
		{"блокировка при безлимите", 0, 0, true, true, false, "Время на сегодня закончилось"},
		{"остаток вышел", 0, 7200, false, false, false, "Время на сегодня закончилось"},
		{"остаток отрицательный", -40, 7200, false, false, false, "Время на сегодня закончилось"},
		{"не детская учётка", 5100, 7200, false, false, true, "PC Block: настройки для родителя"},
		{"не детская учётка важнее блокировки", 0, 0, true, true, true, "PC Block: настройки для родителя"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TrayTooltip(tc.remaining, tc.limit, tc.unlimited, tc.locked, tc.notChild)
			if got != tc.want {
				t.Errorf("TrayTooltip(%d, %d, unlimited=%v, locked=%v, notChild=%v) = %q, want %q",
					tc.remaining, tc.limit, tc.unlimited, tc.locked, tc.notChild, got, tc.want)
			}
		})
	}
}

// Ловит: подсказка длиннее поля szTip значка Windows (128 знаков UTF-16 вместе с нулём):
// длинный текст обрежется на полуслове (L-10, L-11).
func TestTrayTooltipFitsWindowsTipField(t *testing.T) {
	for _, s := range []string{
		TrayTooltip(5100, 7200, false, false, false),
		TrayTooltip(1<<30, 1<<30, false, false, false),
		TrayTooltip(0, 0, true, false, false),
		TrayTooltip(0, 0, false, true, false),
		TrayTooltip(0, 0, false, false, true),
	} {
		if n := len(utf16.Encode([]rune(s))); n > 127 {
			t.Errorf("подсказка %q занимает %d знаков UTF-16, поле вмещает 127", s, n)
		}
	}
}
