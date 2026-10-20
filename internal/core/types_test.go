package core

import (
	"testing"
	"time"
)

// Ловит: категория показывается ребёнку и родителю не тем словом, пустой строкой
// или числом (неизвестное значение должно читаться как «Прочее», R3).
func TestCategoryString(t *testing.T) {
	cases := []struct {
		c    Category
		want string
	}{
		{CatOther, "Прочее"},
		{CatGames, "Игры"},
		{CatStudy, "Учёба"},
		{CatVideoSocial, "Видео и соцсети"},
		{Category(42), "Прочее"},
		{Category(-1), "Прочее"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("Category(%d).String() = %q, want %q", int(tc.c), got, tc.want)
		}
	}
}

// Ловит: пустой лимит (NULL в базе) читается как «0 минут» и блокирует день
// вместо «без лимита» (R4, ТЗ 4: total_min NULL = без лимита).
func TestDayLimitNoLimit(t *testing.T) {
	if !(DayLimit{}).Unlimited() {
		t.Fatal("DayLimit{} должен означать «без лимита»")
	}
	zero := LimitOf(0)
	if zero.Unlimited() {
		t.Fatal("лимит 0 мин — это ограничение, а не «без лимита»")
	}
	if *zero.TotalMin != 0 {
		t.Fatalf("LimitOf(0).TotalMin = %d, want 0", *zero.TotalMin)
	}
	two := LimitOf(120)
	if two.Unlimited() || *two.TotalMin != 120 {
		t.Fatalf("LimitOf(120) = %+v, want 120 мин", two)
	}
}

// Ловит: день недели считается по-американски (воскресенье = 0) и лимит берётся
// не того дня (ТЗ 4: weekday 1 = пн … 7 = вс).
func TestWeekdayOf(t *testing.T) {
	cases := []struct {
		at   time.Time
		want Weekday
	}{
		{time.Date(2026, 10, 19, 15, 0, 0, 0, msk), Monday},
		{time.Date(2026, 10, 23, 23, 59, 59, 0, msk), Friday},
		{time.Date(2026, 10, 24, 0, 0, 0, 0, msk), Saturday},
		{time.Date(2026, 10, 25, 12, 0, 0, 0, msk), Sunday},
	}
	for _, tc := range cases {
		if got := WeekdayOf(tc.at); got != tc.want {
			t.Errorf("WeekdayOf(%s) = %d, want %d", tc.at.Format(time.DateTime), got, tc.want)
		}
	}
}

// Ловит: правила дня недели берутся со сдвигом на день (индекс 0 вместо 1)
// или день вне 1…7 роняет службу паникой.
func TestRulesFor(t *testing.T) {
	var r Rules
	r.Week[0] = LimitOf(120) // индекс 0 = пн
	r.Week[6] = LimitOf(240) // индекс 6 = вс
	if got := r.For(Monday); got.Unlimited() || *got.TotalMin != 120 {
		t.Fatalf("For(Monday) = %+v, want 120", got)
	}
	if got := r.For(Sunday); got.Unlimited() || *got.TotalMin != 240 {
		t.Fatalf("For(Sunday) = %+v, want 240", got)
	}
	if got := r.For(Tuesday); !got.Unlimited() {
		t.Fatalf("For(Tuesday) = %+v, want без лимита", got)
	}
	if got := r.For(Weekday(9)); !got.Unlimited() {
		t.Fatalf("For(9) = %+v, want без лимита (безопасно, R18)", got)
	}
}
