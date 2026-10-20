package guard

import (
	"math"
	"strings"
	"time"

	"pcblock/internal/ipc"
)

// RawSensors — что страж прочитал из Windows за один раз. Здесь нет заголовков окон, имён
// программ и нажатий клавиш: страж их не читает и не хранит (R23).
type RawSensors struct {
	IdleMs     int64 // сколько прошло с последнего ввода с клавиатуры или мыши, мс
	Fullscreen bool  // окно переднего плана занимает весь монитор (рабочий стол не в счёт, Ruling 24)
	Locked     bool  // сеанс заблокирован (экран входа Windows)
}

// ScaleIdle переводит время с последнего ввода в часы ядра. В тестовом режиме часы ядра идут
// быстрее настоящих в speed раз (Ruling 4), и 3 настоящие секунды простоя должны выглядеть для
// ядра как 3*speed секунд; в обычном режиме speed равен 1. Отрицательное время считается
// нулём, переполнения нет.
func ScaleIdle(ms int64, speed int) int64 {
	if ms < 0 {
		return 0
	}
	if speed <= 1 {
		return ms
	}
	if ms > math.MaxInt64/int64(speed) {
		return math.MaxInt64
	}
	return ms * int64(speed)
}

// MaxClockSpeed — наибольшее ускорение часов в тестовом режиме (то же, что у службы).
const MaxClockSpeed = 1000

// ClampSpeed приводит ускорение часов из ключа реестра к допустимому: от 1 до MaxClockSpeed.
func ClampSpeed(n int64) int {
	switch {
	case n < 1:
		return 1
	case n > MaxClockSpeed:
		return MaxClockSpeed
	}
	return int(n)
}

// BuildSample собирает снимок датчиков для службы. now берётся по настоящим часам с местным
// поясом; категория программы на переднем плане в M1 всегда «Прочее» (игры и сайты приходят в
// M3 и M4), заголовки окон не читаются.
func BuildSample(now time.Time, raw RawSensors, speed int) ipc.SensorSample {
	_, offset := now.Zone()
	return ipc.SensorSample{
		AtMs:           now.UnixMilli(),
		TzOffsetMin:    offset / 60,
		SessionActive:  !raw.Locked,
		LastInputAgoMs: ScaleIdle(raw.IdleMs, speed),
		Fullscreen:     raw.Fullscreen,
		Foreground:     ipc.CatNameOther,
	}
}

// Rect — прямоугольник в экранных координатах Windows.
type Rect struct{ Left, Top, Right, Bottom int32 }

// CoversMonitor: окно win закрывает монитор mon целиком (выходить за него можно).
func CoversMonitor(win, mon Rect) bool {
	if mon.Right <= mon.Left || mon.Bottom <= mon.Top {
		return false
	}
	return win.Left <= mon.Left && win.Top <= mon.Top && win.Right >= mon.Right && win.Bottom >= mon.Bottom
}

// styleCaption — WS_CAPTION: у окна есть строка заголовка. Развёрнутое обычное окно
// с автоскрытием панели задач тоже закрывает монитор, но заголовок у него есть; полноэкранный
// плеер или игра рисуются без него.
const styleCaption = 0x00C00000

// IsFullscreenWindow: окно переднего плана полноэкранное (R2б) — закрывает монитор целиком и
// не имеет заголовка.
func IsFullscreenWindow(win, mon Rect, style uint32) bool {
	return CoversMonitor(win, mon) && style&styleCaption != styleCaption
}

// IsDesktopClass: класс окна — рабочий стол Windows (Progman или WorkerW). Такое окно
// закрывает монитор, но полноэкранным не считается (Ruling 24).
func IsDesktopClass(class string) bool {
	switch strings.ToLower(class) {
	case "progman", "workerw":
		return true
	}
	return false
}
