package core

import (
	"testing"
	"time"
)

// msk — фиксированный пояс UTC+3 для тестов (эталоны в местном времени).
var msk = time.FixedZone("UTC+3", 3*60*60)

// fakeClock — поддельные часы: время двигает тест.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time      { return c.t }
func (c *fakeClock) Set(t time.Time)     { c.t = t }
func (c *fakeClock) Add(d time.Duration) { c.t = c.t.Add(d) }

// Ловит: ускоренные часы тестового режима (Ruling 4) идут не с той скоростью
// или скорость 1 перестаёт быть обычными часами.
func TestScaledClock(t *testing.T) {
	start := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	base := time.Date(2026, 10, 19, 15, 0, 0, 0, msk)

	fast := ScaledClock{Base: base, Speed: 60, Start: start}
	if got, want := fast.At(start.Add(time.Second)), time.Date(2026, 10, 19, 15, 1, 0, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 60, через 1 с: %s, want %s", got, want)
	}
	if got, want := fast.At(start.Add(90*time.Second)), time.Date(2026, 10, 19, 16, 30, 0, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 60, через 90 с: %s, want %s", got, want)
	}

	normal := ScaledClock{Base: base, Speed: 1, Start: start}
	if got, want := normal.At(start.Add(5*time.Second)), time.Date(2026, 10, 19, 15, 0, 5, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 1, через 5 с: %s, want %s", got, want)
	}

	broken := ScaledClock{Base: base, Speed: 0, Start: start}
	if got, want := broken.At(start.Add(5*time.Second)), time.Date(2026, 10, 19, 15, 0, 5, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 0 должна читаться как 1: %s, want %s", got, want)
	}
}

// Ловит: Now() у часов не опирается на настоящее время (служба считала бы по стоящим часам).
func TestClocksFollowRealTime(t *testing.T) {
	before := time.Now()
	sys := SystemClock{}.Now()
	if sys.Before(before) || sys.Sub(before) > time.Second {
		t.Fatalf("SystemClock.Now() = %s, а настоящее время %s", sys, before)
	}
	sc := NewScaledClock(1)
	if d := sc.Now().Sub(time.Now()); d < -time.Second || d > time.Second {
		t.Fatalf("NewScaledClock(1).Now() отстоит от настоящего времени на %s", d)
	}
	if sc.Speed != 1 {
		t.Fatalf("NewScaledClock(1).Speed = %d", sc.Speed)
	}
}
