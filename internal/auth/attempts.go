package auth

import (
	"errors"
	"unicode/utf8"
)

// MinLength — наименьшая длина пароля родителя в символах (R19).
const MinLength = 6

// ErrTooShort — пароль короче MinLength символов; текст показывается у поля ввода.
var ErrTooShort = errors.New("Не меньше 6 символов")

// CheckLength проверяет длину пароля в символах (не в байтах: кириллица — 2 байта на букву).
func CheckLength(pw string) error {
	if utf8.RuneCountInString(pw) < MinLength {
		return ErrTooShort
	}
	return nil
}

// Политика пауз R19.
const (
	FailsPerSeries = 5       // неверных попыток подряд в одной серии
	firstPauseMs   = 60_000  // пауза после первой серии
	maxPauseMs     = 900_000 // потолок паузы: 15 мин
)

// Attempts — состояние неверных вводов пароля родителя (R19). Хранится в parent_auth
// (failed_count — FailedInRow, blocked_until — BlockedUntilMs), поэтому структура плоская.
// Время — миллисекунды реальных часов UTC: пауза идёт по реальным часам и в тестовом режиме.
type Attempts struct {
	Failed         int   // неверных в текущей серии, 0…4
	Series         int   // завершённых серий (каждая включила паузу)
	BlockedUntilMs int64 // ввод закрыт до этого момента; 0 — паузы не было
}

// Fail засчитывает неверный ввод в момент nowMs. Пятая неверная попытка серии включает
// паузу: 60 с, каждая следующая серия вдвое дольше, не больше 15 мин. Возвращает момент
// конца паузы или 0, если ввод открыт. Во время паузы ввод не засчитывается и паузу
// не продлевает: служба такой ввод и не проверяет.
func (a *Attempts) Fail(nowMs int64) (blockedUntilMs int64) {
	if blocked, until := a.Blocked(nowMs); blocked {
		return until
	}
	a.Failed++
	if a.Failed < FailsPerSeries {
		return 0
	}
	a.Failed = 0
	a.Series++
	a.BlockedUntilMs = nowMs + pauseMs(a.Series)
	return a.BlockedUntilMs
}

// pauseMs — длина паузы после серии номер series (с 1): 60 с · 2^(series−1), не больше 15 мин.
func pauseMs(series int) int64 {
	p := int64(firstPauseMs)
	for i := 1; i < series && p < maxPauseMs; i++ {
		p *= 2
	}
	return min(p, maxPauseMs)
}

// Reset сбрасывает счётчик, серию и паузу после верного пароля.
func (a *Attempts) Reset() { *a = Attempts{} }

// Blocked сообщает, закрыт ли ввод в момент nowMs, и момент конца паузы.
func (a Attempts) Blocked(nowMs int64) (bool, int64) {
	if nowMs < a.BlockedUntilMs {
		return true, a.BlockedUntilMs
	}
	return false, 0
}

// FailedInRow — неверных подряд с последнего верного пароля (parent_auth.failed_count):
// серия и счётчик в серии хранятся одним числом без потерь.
func (a Attempts) FailedInRow() int { return a.Series*FailsPerSeries + a.Failed }

// AttemptsFromRow восстанавливает состояние из parent_auth: серия = inRow / 5,
// счётчик в серии = inRow % 5. Отрицательное число из базы считается нулём.
func AttemptsFromRow(inRow int, blockedUntilMs int64) Attempts {
	if inRow < 0 {
		inRow = 0
	}
	return Attempts{
		Failed:         inRow % FailsPerSeries,
		Series:         inRow / FailsPerSeries,
		BlockedUntilMs: blockedUntilMs,
	}
}
