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

// realTime — подставное «настоящее время» для ScaledClock.
type realTime struct{ t time.Time }

func (r *realTime) now() time.Time { return r.t }

// scaled возвращает ускоренные часы, у которых настоящее время двигает тест.
func scaled(virtualStart time.Time, real *realTime, realStart time.Time, speed int) *ScaledClock {
	c := NewScaledClock(virtualStart, realStart, speed)
	c.realNow = real.now
	return c
}

// Ловит: ускоренные часы тестового режима (Ruling 4) идут не с той скоростью,
// скорость 1 перестаёт быть обычными часами, скорость 0 останавливает время.
func TestScaledClock(t *testing.T) {
	realStart := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	virtualStart := time.Date(2026, 10, 19, 15, 0, 0, 0, msk)
	real := &realTime{t: realStart}

	fast := scaled(virtualStart, real, realStart, 60)
	real.t = realStart.Add(time.Second)
	if got, want := fast.Now(), time.Date(2026, 10, 19, 15, 1, 0, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 60, через 1 с: %s, want %s", got, want)
	}
	real.t = realStart.Add(90 * time.Second)
	if got, want := fast.Now(), time.Date(2026, 10, 19, 16, 30, 0, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 60, через 90 с: %s, want %s", got, want)
	}

	real.t = realStart.Add(5 * time.Second)
	normal := scaled(virtualStart, real, realStart, 1)
	if got, want := normal.Now(), time.Date(2026, 10, 19, 15, 0, 5, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 1, через 5 с: %s, want %s", got, want)
	}
	broken := scaled(virtualStart, real, realStart, 0)
	if got, want := broken.Now(), time.Date(2026, 10, 19, 15, 0, 5, 0, msk); !got.Equal(want) {
		t.Fatalf("скорость 0 должна читаться как 1: %s, want %s", got, want)
	}
}

// Ловит: после перезапуска службы ускоренное время начинается заново, а не продолжается
// от сохранённого виртуального момента.
func TestScaledClockContinuesAfterRestart(t *testing.T) {
	realNow := time.Date(2026, 10, 19, 12, 0, 10, 0, time.UTC)
	realStart := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) // запуск 10 с назад
	virtualStart := time.Date(2026, 10, 19, 16, 0, 0, 0, msk)
	c := scaled(virtualStart, &realTime{t: realNow}, realStart, 60)
	if got, want := c.Now(), time.Date(2026, 10, 19, 16, 10, 0, 0, msk); !got.Equal(want) {
		t.Fatalf("Now() = %s, want %s", got, want)
	}
}

// Ловит: пауза для снимков не останавливает время или после продолжения время
// перескакивает на длительность паузы.
func TestScaledClockPauseResume(t *testing.T) {
	realStart := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	virtualStart := time.Date(2026, 10, 19, 15, 0, 0, 0, msk)
	real := &realTime{t: realStart}
	c := scaled(virtualStart, real, realStart, 60)

	real.t = realStart.Add(2 * time.Second) // 15:02:00
	c.Pause()
	c.Pause() // повторная пауза ничего не меняет
	real.t = realStart.Add(30 * time.Second)
	if got, want := c.Now(), time.Date(2026, 10, 19, 15, 2, 0, 0, msk); !got.Equal(want) {
		t.Fatalf("на паузе Now() = %s, want %s", got, want)
	}
	c.Resume()
	c.Resume() // повторное продолжение ничего не меняет
	real.t = realStart.Add(31 * time.Second)
	if got, want := c.Now(), time.Date(2026, 10, 19, 15, 3, 0, 0, msk); !got.Equal(want) {
		t.Fatalf("после продолжения Now() = %s, want %s", got, want)
	}
}

// Ловит: Now() у часов не опирается на настоящее время (служба считала бы по стоящим часам).
func TestClocksFollowRealTime(t *testing.T) {
	before := time.Now()
	sys := SystemClock{}.Now()
	if sys.Before(before) || sys.Sub(before) > time.Second {
		t.Fatalf("SystemClock.Now() = %s, а настоящее время %s", sys, before)
	}
	now := time.Now()
	sc := NewScaledClock(now, now, 1)
	if d := sc.Now().Sub(time.Now()); d < -time.Second || d > time.Second {
		t.Fatalf("NewScaledClock(сейчас, сейчас, 1).Now() отстоит от настоящего времени на %s", d)
	}
}
