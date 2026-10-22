package core

import "testing"

// Ловит: расхождение размера автобонуса с правилом R25 (D-49) — формула N×(остаток div 10),
// дно 0, «неполные 10 минут не считаются», без бонуса в неразрешённые к игре дни.
func TestDailyBonusMinutes(t *testing.T) {
	lim := func(min int) DayLimit { return LimitOf(min) }
	cases := []struct {
		name       string
		limit      DayLimit
		extraMin   int
		totalS     int
		ratioN     int
		allowsPlay bool
		want       int
	}{
		{"играл половину лимита", lim(60), 0, 30 * 60, 5, true, 15},     // остаток 30 → 3×5
		{"совсем не играл", lim(60), 0, 0, 5, true, 30},                 // остаток 60 → 6×5
		{"потратил всё", lim(60), 0, 60 * 60, 5, true, 0},               // остаток 0
		{"остаток 5 мин не считается", lim(60), 0, 55 * 60, 5, true, 0}, // остаток 5 → 0 блоков
		{"остаток 15 мин", lim(60), 0, 45 * 60, 5, true, 5},             // остаток 15 → 1×5
		{"ratio 10", lim(60), 0, 0, 10, true, 60},                       // остаток 60 → 6×10
		{"с доп. временем", lim(60), 30, 30 * 60, 5, true, 30},          // остаток 90-30=60 → 6×5
		{"без лимита — нет бонуса", DayLimit{}, 0, 0, 5, true, 0},
		{"лимит 0 — нет бонуса", lim(0), 0, 0, 5, true, 0},
		{"запретный день — нет бонуса", lim(60), 0, 0, 5, false, 0},
		{"ratio 0 — нет бонуса", lim(60), 0, 0, 0, true, 0},
		{"остаток 29 мин — 2 блока", lim(60), 0, 31 * 60, 5, true, 10}, // остаток 29 → 2×5
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DailyBonusMinutes(c.limit, c.extraMin, c.totalS, c.ratioN, c.allowsPlay)
			if got != c.want {
				t.Errorf("DailyBonusMinutes = %d, ожидалось %d", got, c.want)
			}
		})
	}
}

// Ловит: неверное определение «день полностью запретный» (R24/R25) — покрытие суток
// запретными интервалами, в том числе переход через полночь и перенос с прошлого дня.
func TestDayFullyForbidden(t *testing.T) {
	iv := func(wd Weekday, s, e int) Interval { return Interval{Weekday: wd, StartMin: s, EndMin: e} }
	cases := []struct {
		name  string
		sched []Interval
		wd    Weekday
		want  bool
	}{
		{"нет интервалов", nil, Monday, false},
		{"только вечер — не весь день", []Interval{iv(Monday, 21*60, 7*60)}, Monday, false},
		{"весь день одним интервалом", []Interval{iv(Monday, 0, 0)}, Monday, true},
		{"весь день двумя половинами", []Interval{iv(Monday, 0, 12*60), iv(Monday, 12*60, 0)}, Monday, true},
		{"перенос с пн не покрывает вт целиком", []Interval{iv(Monday, 21*60, 7*60)}, Tuesday, false},
		{"разрыв в середине", []Interval{iv(Monday, 0, 10*60), iv(Monday, 11*60, 0)}, Monday, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DayFullyForbidden(Rules{Schedule: c.sched}, c.wd)
			if got != c.want {
				t.Errorf("DayFullyForbidden = %v, ожидалось %v", got, c.want)
			}
		})
	}
}
