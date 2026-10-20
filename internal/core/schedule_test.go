package core

import (
	"reflect"
	"testing"
	"time"
)

// Эталонные дни недели константами (грабля «тесты с датами», L-59):
// пт 23.10.2026, сб 24.10.2026, пн 19.10.2026.
func tFri(h, m int) time.Time { return time.Date(2026, 10, 23, h, m, 0, 0, msk) }
func tSat(h, m int) time.Time { return time.Date(2026, 10, 24, h, m, 0, 0, msk) }
func tMon(h, m int) time.Time { return time.Date(2026, 10, 19, h, m, 0, 0, msk) }

// iv — короткий конструктор интервала.
func iv(wd Weekday, startMin, endMin int) Interval {
	return Interval{Weekday: wd, StartMin: startMin, EndMin: endMin}
}

// rulesSched собирает правила с интервалами (без лимитов).
func rulesSched(in ...Interval) Rules {
	return Rules{Schedule: in}
}

func TestScheduleInForbidden(t *testing.T) {
	// Один интервал пн 21:00–07:00 (через полночь, принадлежит пн).
	r := rulesSched(iv(Monday, 21*60, 7*60))
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"пн 21:00 начало", tMon(21, 0), true},
		{"пн 20:59 до начала", tMon(20, 59), false},
		{"пн 23:30 внутри", tMon(23, 30), true},
		{"вт 06:59 перенос с пн", time.Date(2026, 10, 20, 6, 59, 0, 0, msk), true},
		{"вт 07:00 конец", time.Date(2026, 10, 20, 7, 0, 0, 0, msk), false},
		{"пн 12:00 день", tMon(12, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := InForbidden(r, c.t)
			if got != c.want {
				t.Fatalf("InForbidden(%s) = %v, want %v", c.t.Format(time.RFC3339), got, c.want)
			}
		})
	}
}

func TestScheduleForbiddenUntil(t *testing.T) {
	// В интервале конец интервала — «когда снова можно».
	r := rulesSched(iv(Monday, 21*60, 7*60))
	in, until := InForbidden(r, tMon(23, 30))
	if !in {
		t.Fatal("ожидался запретный интервал в пн 23:30")
	}
	wantUntil := time.Date(2026, 10, 20, 7, 0, 0, 0, msk) // вт 07:00
	if !until.Equal(wantUntil) {
		t.Fatalf("конец интервала = %s, want %s", until.Format(time.RFC3339), wantUntil.Format(time.RFC3339))
	}
}

func TestScheduleFridayCrossMidnight(t *testing.T) {
	// Интервал пт 21:00–07:00 действует до сб 07:00 (принадлежит пятнице, R7).
	r := rulesSched(iv(Friday, 21*60, 7*60))
	if in, _ := InForbidden(r, tSat(3, 0)); !in {
		t.Fatal("сб 03:00 должно попадать в интервал пятницы (перенос через полночь)")
	}
	if in, _ := InForbidden(r, tSat(7, 0)); in {
		t.Fatal("сб 07:00 — конец интервала, уже можно")
	}
	if in, _ := InForbidden(r, tFri(20, 59)); in {
		t.Fatal("пт 20:59 — до начала интервала")
	}
}

func TestScheduleNextForbiddenStart(t *testing.T) {
	r := rulesSched(iv(Monday, 21*60, 7*60))
	// В пн 20:30 ближайшее начало запрета — пн 21:00.
	got, ok := NextForbiddenStart(r, tMon(20, 30))
	if !ok {
		t.Fatal("ожидалось ближайшее начало интервала")
	}
	want := tMon(21, 0)
	if !got.Equal(want) {
		t.Fatalf("NextForbiddenStart = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	// Уже внутри интервала — следующего начала сегодня нет (или оно уже в силе).
	if _, ok := NextForbiddenStart(r, tMon(22, 0)); ok {
		t.Fatal("внутри интервала следующего начала в тех же сутках быть не должно")
	}
}

func TestNormalizeIntervalsMergeOverlap(t *testing.T) {
	// Пересекающиеся пн 21:00–23:00 и 22:00–07:00 → один 21:00–07:00.
	got := NormalizeIntervals([]Interval{
		iv(Monday, 21*60, 23*60),
		iv(Monday, 22*60, 7*60),
	})
	want := []Interval{iv(Monday, 21*60, 7*60)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("объединение пересечения = %+v, want %+v", got, want)
	}
}

func TestNormalizeIntervalsMergeAdjacent(t *testing.T) {
	// Смежные 08:00–09:00 и 09:00–10:00 → один 08:00–10:00.
	got := NormalizeIntervals([]Interval{
		iv(Monday, 8*60, 9*60),
		iv(Monday, 9*60, 10*60),
	})
	want := []Interval{iv(Monday, 8*60, 10*60)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("объединение смежных = %+v, want %+v", got, want)
	}
}

func TestNormalizeIntervalsSeparateDaysKept(t *testing.T) {
	// Разные дни не объединяются; порядок детерминирован (по дню, затем началу).
	got := NormalizeIntervals([]Interval{
		iv(Tuesday, 8*60, 9*60),
		iv(Monday, 10*60, 11*60),
		iv(Monday, 8*60, 9*60),
	})
	want := []Interval{
		iv(Monday, 8*60, 9*60),
		iv(Monday, 10*60, 11*60),
		iv(Tuesday, 8*60, 9*60),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("сортировка по дням = %+v, want %+v", got, want)
	}
}

func TestNormalizeIntervalsMaxSix(t *testing.T) {
	// Больше 6 непересекающихся интервалов на день после объединения обрезается до 6 (R7).
	in := []Interval{}
	for i := 0; i < 8; i++ {
		start := i * 120 // 0,120,240,... минут — непересекающиеся
		in = append(in, iv(Monday, start, start+60))
	}
	got := NormalizeIntervals(in)
	count := 0
	for _, x := range got {
		if x.Weekday == Monday {
			count++
		}
	}
	if count > 6 {
		t.Fatalf("на день оставлено %d интервалов, допустимо не больше 6 (R7)", count)
	}
}

func TestNormalizeIntervalsEmpty(t *testing.T) {
	// Пустой и nil вход — пустой результат, не паника (R18).
	if got := NormalizeIntervals(nil); len(got) != 0 {
		t.Fatalf("nil → %+v, want пусто", got)
	}
	if got := NormalizeIntervals([]Interval{}); len(got) != 0 {
		t.Fatalf("[] → %+v, want пусто", got)
	}
}

func TestIntervalsForWithCarry(t *testing.T) {
	// IntervalsFor(вт) включает перенос интервала пн 21:00–07:00 в сутки вторника.
	r := rulesSched(iv(Monday, 21*60, 7*60), iv(Tuesday, 12*60, 13*60))
	got := r.IntervalsFor(Tuesday)
	// Во вторник действуют: перенос пн 21:00–07:00 (часть после полуночи) и вт 12:00–13:00.
	if len(got) == 0 {
		t.Fatal("IntervalsFor(вт) не вернул действующих интервалов")
	}
	hasCarry := false
	for _, x := range got {
		if x.Weekday == Monday {
			hasCarry = true
		}
	}
	if !hasCarry {
		t.Fatal("IntervalsFor(вт) должен включать перенос интервала понедельника")
	}
}
