package core

import "time"

// Clock — порт часов ядра. Ядро узнаёт время только через него: в работе это
// SystemClock, в тестовом режиме (ТЗ 10.8) — ScaledClock, в тестах — поддельные часы.
// Пояс берётся из Location возвращённого момента (сутки и полночь — по местному времени).
type Clock interface {
	Now() time.Time
}

// SystemClock — настоящие часы компьютера.
type SystemClock struct{}

// Now возвращает текущее местное время.
func (SystemClock) Now() time.Time { return time.Now() }

// ScaledClock — ускоренные часы тестового режима (Ruling 4 плана M1): от момента Start
// настоящего времени виртуальное время идёт от Base в Speed раз быстрее.
// Speed меньше 1 читается как 1 (обычные часы).
type ScaledClock struct {
	Base  time.Time // виртуальное время в момент Start
	Speed int       // во сколько раз быстрее настоящего времени
	Start time.Time // настоящее время запуска
}

// NewScaledClock запускает ускоренные часы с текущего момента.
func NewScaledClock(speed int) ScaledClock {
	now := time.Now()
	return ScaledClock{Base: now, Speed: speed, Start: now}
}

// Now возвращает виртуальное время для текущего настоящего момента.
func (c ScaledClock) Now() time.Time { return c.At(time.Now()) }

// At возвращает виртуальное время для настоящего момента real.
func (c ScaledClock) At(real time.Time) time.Time {
	speed := c.Speed
	if speed < 1 {
		speed = 1
	}
	return c.Base.Add(real.Sub(c.Start) * time.Duration(speed))
}
