package core

import (
	"sync"
	"time"
)

// Clock — порт часов ядра. Ядро узнаёт время только через него: в работе это
// SystemClock, в тестовом режиме (ТЗ 10.8) — ScaledClock, в тестах — поддельные часы.
type Clock interface {
	Now() time.Time
}

// SystemClock — настоящие часы компьютера.
type SystemClock struct{}

// Now возвращает текущее время.
func (SystemClock) Now() time.Time { return time.Now() }

// ScaledClock — ускоренные часы тестового режима (Ruling 4 плана M1): виртуальное время
// идёт от virtualStart в speed раз быстрее настоящего, начиная с настоящего момента
// realStart. Служба в тестовом режиме тикает раз в секунду этих часов (настоящий период
// 1 с / speed), поэтому правило разрыва тиков (Ruling 13) считается в часах ядра.
// Пауза останавливает виртуальное время (снимки экранов приёмки). Безопасны для
// одновременного вызова из нескольких горутин.
type ScaledClock struct {
	mu        sync.Mutex
	realNow   func() time.Time // настоящие часы; в тестах подменяются
	virtual   time.Time        // виртуальное время в момент realStart (или на паузе)
	realStart time.Time
	speed     int
	paused    bool
}

// NewScaledClock возвращает часы, у которых сейчас = virtualStart + (настоящее сейчас −
// realStart) × speed. Так время продолжается после перезапуска службы: служба передаёт
// сохранённые virtualStart и realStart. speed меньше 1 читается как 1.
func NewScaledClock(virtualStart, realStart time.Time, speed int) *ScaledClock {
	if speed < 1 {
		speed = 1
	}
	return &ScaledClock{realNow: time.Now, virtual: virtualStart, realStart: realStart, speed: speed}
}

// Now возвращает виртуальное время.
func (c *ScaledClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nowLocked()
}

func (c *ScaledClock) nowLocked() time.Time {
	if c.paused {
		return c.virtual
	}
	return c.virtual.Add(c.realNow().Sub(c.realStart) * time.Duration(c.speed))
}

// Pause останавливает виртуальное время; повторный вызов ничего не меняет.
func (c *ScaledClock) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused {
		return
	}
	c.virtual = c.nowLocked()
	c.paused = true
}

// Resume продолжает виртуальное время с момента паузы; без паузы ничего не меняет.
func (c *ScaledClock) Resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.paused {
		return
	}
	c.realStart = c.realNow()
	c.paused = false
}
