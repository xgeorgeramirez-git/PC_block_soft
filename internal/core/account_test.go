package core

import (
	"testing"
	"time"
)

// Границы суток в тестах — константами (грабля «тесты с датами»): пт 23.10 → сб 24.10.
const (
	friday   = "2026-10-23"
	saturday = "2026-10-24"
)

// midnight — полночь с пятницы на субботу по UTC+3.
var midnight = time.Date(2026, 10, 24, 0, 0, 0, 0, msk)

// activeSample — ребёнок за компьютером: ввод секунду назад, обычное окно, «Прочее».
func activeSample() SessionSample {
	return SessionSample{SessionActive: true, LastInputAgoMs: 1000, Foreground: CatOther}
}

// Ловит: неверная граница бездействия (300 с — уже не активна), неучтённые правила
// R2б…R2г (полный экран, игра, видео без ввода) и счёт в заблокированном или спящем сеансе.
func TestIsActiveR2(t *testing.T) {
	const hour = int64(3600 * 1000)
	cases := []struct {
		name string
		s    SessionSample
		want bool
	}{
		{"ввод 299 с назад", SessionSample{SessionActive: true, LastInputAgoMs: 299_000}, true},
		{"ввод 299,999 с назад", SessionSample{SessionActive: true, LastInputAgoMs: 299_999}, true},
		{"ввод ровно 300 с назад", SessionSample{SessionActive: true, LastInputAgoMs: 300_000}, false},
		{"ввод 301 с назад", SessionSample{SessionActive: true, LastInputAgoMs: 301_000}, false},
		{"полный экран без ввода", SessionSample{SessionActive: true, LastInputAgoMs: hour, Fullscreen: true}, true},
		{"игра без ввода", SessionSample{SessionActive: true, LastInputAgoMs: hour, Foreground: CatGames}, true},
		{"видео-сайт без ввода", SessionSample{SessionActive: true, LastInputAgoMs: hour, Foreground: CatVideoSocial}, true},
		{"учёба без ввода", SessionSample{SessionActive: true, LastInputAgoMs: hour, Foreground: CatStudy}, false},
		{"сеанс заблокирован, ввод только что", SessionSample{SessionActive: false, LastInputAgoMs: 0}, false},
		{"сеанс заблокирован, полный экран, игра", SessionSample{SessionActive: false, Fullscreen: true, Foreground: CatGames}, false},
	}
	for _, tc := range cases {
		if got := IsActive(tc.s, InactivityLimitMs); got != tc.want {
			t.Errorf("%s: IsActive = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Ловит: секунда уходит не в ту категорию или неизвестная категория теряет секунду (R3).
func TestCategoryOf(t *testing.T) {
	cases := []struct {
		fg   Category
		want Category
	}{
		{CatOther, CatOther},
		{CatGames, CatGames},
		{CatVideoSocial, CatVideoSocial},
		{CatStudy, CatStudy},
		{Category(7), CatOther},
	}
	for _, tc := range cases {
		if got := CategoryOf(SessionSample{Foreground: tc.fg}); got != tc.want {
			t.Errorf("CategoryOf(%d) = %d, want %d", int(tc.fg), int(got), int(tc.want))
		}
	}
}

// Ловит: учёба попадает в общее время (D-7), игры и видео не попадают в общее,
// или за тик засчитывается больше секунды.
func TestAccumulatorCategories(t *testing.T) {
	a := NewAccumulator(DayState{Day: friday})
	at := time.Date(2026, 10, 23, 15, 0, 0, 0, msk)
	for i, fg := range []Category{CatGames, CatVideoSocial, CatStudy, CatOther, CatGames} {
		s := activeSample()
		s.Foreground = fg
		if !a.Tick(at.Add(time.Duration(i)*time.Second), s, false) {
			t.Fatalf("тик %d (%s) не засчитан", i, fg)
		}
	}
	want := DayState{Day: friday, TotalS: 4, GamesS: 2, VideoS: 1, StudyS: 1}
	if got := a.State(); got != want {
		t.Fatalf("State() = %+v, want %+v", got, want)
	}
}

// Ловит: засчитывается секунда под экраном блокировки (R2, D-24) или неактивная секунда.
func TestAccumulatorSkipsLockAndInactive(t *testing.T) {
	a := NewAccumulator(DayState{Day: friday, TotalS: 10})
	at := time.Date(2026, 10, 23, 17, 0, 0, 0, msk)
	if a.Tick(at, activeSample(), true) {
		t.Fatal("секунда под экраном блокировки засчитана")
	}
	idle := SessionSample{SessionActive: true, LastInputAgoMs: 600_000}
	if a.Tick(at.Add(time.Second), idle, false) {
		t.Fatal("неактивная секунда засчитана")
	}
	if got := a.State().TotalS; got != 10 {
		t.Fatalf("TotalS = %d, want 10", got)
	}
}

// Ловит: секунды после 00:00 остаются в старом дне, доп. время переносится на новый день
// (R16, G9), или закрытый день теряется для сохранения.
func TestTickRollover(t *testing.T) {
	a := NewAccumulator(DayState{Day: friday, TotalS: 100, ExtraMin: 30})
	for _, at := range []time.Time{
		midnight.Add(-2 * time.Second), // 23:59:58
		midnight.Add(-1 * time.Second), // 23:59:59
		midnight,                       // 00:00:00
		midnight.Add(1 * time.Second),  // 00:00:01
	} {
		a.Tick(at, activeSample(), false)
	}
	closed := a.ClosedDays()
	if len(closed) != 1 || closed[0] != (DayState{Day: friday, TotalS: 102, ExtraMin: 30}) {
		t.Fatalf("закрытые дни = %+v, want пятница 102 с и доп. 30 мин", closed)
	}
	if got := a.State(); got != (DayState{Day: saturday, TotalS: 2}) {
		t.Fatalf("новый день = %+v, want суббота 2 с без доп. времени", got)
	}
	if again := a.ClosedDays(); again == nil || len(again) != 0 {
		t.Fatalf("повторный ClosedDays() = %#v, want пустой список (не nil)", again)
	}
}

// Ловит: после сна через полночь (23:50 → 00:10) время сна засчитывается или день не
// сменяется, потому что смену суток проверяли только при непрерывных тиках (Ruling 13).
func TestRolloverAfterSleep(t *testing.T) {
	a := NewAccumulator(DayState{Day: friday})
	beforeSleep := time.Date(2026, 10, 23, 23, 49, 58, 0, msk)
	afterSleep := time.Date(2026, 10, 24, 0, 10, 0, 0, msk)
	for _, at := range []time.Time{beforeSleep, beforeSleep.Add(time.Second), afterSleep, afterSleep.Add(time.Second)} {
		a.Tick(at, activeSample(), false)
	}
	closed := a.ClosedDays()
	if len(closed) != 1 || closed[0] != (DayState{Day: friday, TotalS: 2}) {
		t.Fatalf("пятница = %+v, want 2 с", closed)
	}
	if got := a.State(); got != (DayState{Day: saturday, TotalS: 2}) {
		t.Fatalf("суббота = %+v, want 2 с", got)
	}
}

// Ловит: разрыв между тиками (перегрузка, остановленная служба) досчитывается задним
// числом (Ruling 13): тик засчитывает только свою секунду, разрыв не засчитывается.
func TestGapNotBackfilled(t *testing.T) {
	start := time.Date(2026, 10, 23, 15, 0, 0, 0, msk)
	cases := []struct {
		name string
		gap  time.Duration
		want int
	}{
		{"разрыв 5 с", 5 * time.Second, 4},
		{"разрыв 6 с", 6 * time.Second, 4},
		{"разрыв 1 ч", time.Hour, 4},
	}
	for _, tc := range cases {
		a := NewAccumulator(DayState{Day: friday})
		second := start.Add(time.Second)
		resume := second.Add(tc.gap)
		for _, at := range []time.Time{start, second, resume, resume.Add(time.Second)} {
			a.Tick(at, activeSample(), false)
		}
		if got := a.State().TotalS; got != tc.want {
			t.Errorf("%s: TotalS = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// Ловит: перевод часов назад через полночь открывает прошлый день с нулевыми
// счётчиками, и служба перезаписала бы его итог меньшими числами.
func TestClockBackDoesNotReopenDay(t *testing.T) {
	a := NewAccumulator(DayState{Day: saturday, TotalS: 5})
	a.Tick(midnight.Add(-2*time.Second), activeSample(), false)
	if got := a.State(); got != (DayState{Day: saturday, TotalS: 6}) {
		t.Fatalf("State() = %+v, want суббота 6 с", got)
	}
	if closed := a.ClosedDays(); len(closed) != 0 {
		t.Fatalf("закрыты дни %+v, want ни одного", closed)
	}
}

// Ловит: новый накопитель без дня закрывает пустой день «» и служба сохранила бы мусор.
func TestAccumulatorStartsWithoutDay(t *testing.T) {
	a := NewAccumulator(DayState{})
	a.Tick(time.Date(2026, 10, 23, 9, 0, 0, 0, msk), activeSample(), false)
	if got := a.State(); got != (DayState{Day: friday, TotalS: 1}) {
		t.Fatalf("State() = %+v, want пятница 1 с", got)
	}
	if closed := a.ClosedDays(); len(closed) != 0 {
		t.Fatalf("закрыт пустой день: %+v", closed)
	}
}
