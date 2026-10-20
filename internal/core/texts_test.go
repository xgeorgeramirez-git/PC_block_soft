package core

import "testing"

// Ловит: длительность не по ТЗ 4 («40 мин», «1 ч 05 мин»): без ведущего нуля минут,
// с округлением вверх (ребёнку показали бы больше, чем есть) или с минусом.
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		s    int
		want string
	}{
		{-5, "0 мин"},
		{0, "0 мин"},
		{59, "0 мин"},
		{60, "1 мин"},
		{2400, "40 мин"},
		{3599, "59 мин"},
		{3600, "1 ч 00 мин"},
		{3900, "1 ч 05 мин"},
		{7200, "2 ч 00 мин"},
		{9000, "2 ч 30 мин"},
	}
	for _, tc := range cases {
		if got := FormatDuration(tc.s); got != tc.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", tc.s, got, tc.want)
		}
	}
}

// Ловит: лимит пишется «2 ч 00 мин» вместо «2 ч» или «1 ч 30» без «мин» (пакет, п.7).
func TestFormatLimit(t *testing.T) {
	cases := []struct {
		min  int
		want string
	}{
		{0, "0 мин"},
		{20, "20 мин"},
		{60, "1 ч"},
		{65, "1 ч 05 мин"},
		{90, "1 ч 30 мин"},
		{120, "2 ч"},
		{150, "2 ч 30 мин"},
	}
	for _, tc := range cases {
		if got := FormatLimit(tc.min); got != tc.want {
			t.Errorf("FormatLimit(%d) = %q, want %q", tc.min, got, tc.want)
		}
	}
}

// Ловит: строка «Сегодня: …» без «из» при лимите или с «из 0 мин» без лимита (ТЗ 8.2.3).
func TestUsedText(t *testing.T) {
	if got, want := UsedText(7200, 120, false), "Сегодня: 2 ч 00 мин из 2 ч"; got != want {
		t.Errorf("UsedText(7200, 120) = %q, want %q", got, want)
	}
	if got, want := UsedText(9000, 150, false), "Сегодня: 2 ч 30 мин из 2 ч 30 мин"; got != want {
		t.Errorf("UsedText(9000, 150) = %q, want %q", got, want)
	}
	if got, want := UsedText(2400, 0, true), "Сегодня: 40 мин"; got != want {
		t.Errorf("UsedText без лимита = %q, want %q", got, want)
	}
}

// Ловит: «Можно снова» указывает день с лимитом 0, не тот падеж дня недели, считает
// сегодняшний день «завтра» или обещает время, когда во всей неделе лимит 0.
func TestUntilText(t *testing.T) {
	only := func(allowed ...Weekday) Rules {
		var r Rules
		for i := range r.Week {
			r.Week[i] = LimitOf(0)
		}
		for _, wd := range allowed {
			r.Week[wd-1] = LimitOf(60)
		}
		return r
	}
	allUnlimited := Rules{}
	cases := []struct {
		name  string
		today Weekday
		rules Rules
		want  string
	}{
		{"завтра без лимита", Monday, allUnlimited, "Можно снова: завтра"},
		{"завтра лимит", Monday, only(Tuesday), "Можно снова: завтра"},
		{"через воскресенье", Saturday, only(Monday), "Можно снова: в понедельник"},
		{"вторник", Sunday, only(Tuesday), "Можно снова: во вторник"},
		{"среда", Monday, only(Wednesday), "Можно снова: в среду"},
		{"четверг", Monday, only(Thursday), "Можно снова: в четверг"},
		{"пятница", Monday, only(Friday), "Можно снова: в пятницу"},
		{"суббота", Monday, only(Saturday), "Можно снова: в субботу"},
		{"воскресенье", Monday, only(Sunday), "Можно снова: в воскресенье"},
		{"только сегодняшний день недели", Monday, only(Monday), "Можно снова: в понедельник"},
		{"ближайший из нескольких", Monday, only(Thursday, Saturday), "Можно снова: в четверг"},
		{"вся неделя 0", Wednesday, only(), "Можно снова: когда родитель изменит правила"},
	}
	for _, tc := range cases {
		if got := UntilText(tc.today, tc.rules); got != tc.want {
			t.Errorf("%s: UntilText = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Ловит: запись журнала о доп. времени не в формате эталона G7 («Доп. время +30 мин»).
func TestExtraText(t *testing.T) {
	if got, want := ExtraText(15), "Доп. время +15 мин"; got != want {
		t.Fatalf("ExtraText(15) = %q, want %q", got, want)
	}
}
