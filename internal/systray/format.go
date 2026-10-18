package systray

import (
	"image/color"
	"strconv"

	"pcblock/internal/core"
)

// Status — статус значка; цвет значка его носит (ТЗ 8.0 п.3, урок L-14).
type Status int

const (
	// StatusNormal — обычный: остаток больше 15 минут или лимита нет.
	StatusNormal Status = iota
	// StatusLow — осталось 15 минут и меньше.
	StatusLow
	// StatusOut — время вышло или включена блокировка.
	StatusOut
	// StatusPaused — ограничения не действуют (пауза, M2; заводится сразу).
	StatusPaused
	// StatusNotChild — учётка не детская: значок без таймера.
	StatusNotChild
)

// lowThresholdMin — порог янтарного значка (FR-7, R9): осталось не больше 15 минут.
const lowThresholdMin = 15

// maxLabelHours — предел часов в надписи: «99ч» — три знака, больше в значок 16 px не влезает,
// а остаток в сутки и больше бывает только от ошибки данных.
const maxLabelHours = 99

// Тексты подсказки (ТЗ 8.2.1 п.2 и 4); время вышло берётся из ядра (core.ReasonTimeUp).
const (
	tooltipUnlimited = "Сегодня без ограничений по времени"
	tooltipParent    = "PC Block: настройки для родителя"
)

// Палитра ТЗ 8.0 п.9. Цвета статусов только отсюда; янтарный, красный и зелёный
// больше ни для чего не используются (п.3).
var (
	colorGraphite = color.RGBA{R: 0x2B, G: 0x31, B: 0x38, A: 0xFF}
	colorAmber    = color.RGBA{R: 0xA1, G: 0x5C, B: 0x00, A: 0xFF}
	colorRed      = color.RGBA{R: 0xB3, G: 0x26, B: 0x1E, A: 0xFF}
	colorGreen    = color.RGBA{R: 0x2C, G: 0x7A, B: 0x4B, A: 0xFF}
	// colorText — цвет надписи на значке; на каждом цвете статуса контраст не меньше 4,5:1 (NFR-14).
	colorText = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// Color — цвет фона значка для статуса; неизвестный статус рисуется графитом.
func (s Status) Color() color.RGBA {
	switch s {
	case StatusLow:
		return colorAmber
	case StatusOut:
		return colorRed
	case StatusPaused:
		return colorGreen
	default:
		return colorGraphite
	}
}

// String — короткое имя статуса для журнала и сообщений тестов.
func (s Status) String() string {
	switch s {
	case StatusNormal:
		return "normal"
	case StatusLow:
		return "low"
	case StatusOut:
		return "out"
	case StatusPaused:
		return "paused"
	case StatusNotChild:
		return "not-child"
	default:
		return "unknown"
	}
}

// TrayLabel — надпись на значке (FR-7): без лимита «∞»; до 99 минут цифрами; от 100 минут
// целые часы вниз со знаком «ч» («1ч», «2ч»), не больше «99ч»; отрицательное — «0».
func TrayLabel(remainingMin int, unlimited bool) string {
	if unlimited {
		return "∞"
	}
	if remainingMin <= 0 {
		return "0"
	}
	if remainingMin < 100 {
		return strconv.Itoa(remainingMin)
	}
	hours := remainingMin / 60
	if hours > maxLabelHours {
		hours = maxLabelHours
	}
	return strconv.Itoa(hours) + "ч"
}

// TrayStatus — статус значка. Пауза важнее всего; затем блокировка; без лимита всегда
// обычный; 0 и меньше — время вышло; не больше 15 минут — янтарный; иначе обычный.
// StatusNotChild сюда не попадает: не детскую учётку определяет вызывающий.
func TrayStatus(remainingMin int, unlimited, locked, paused bool) Status {
	switch {
	case paused:
		return StatusPaused
	case locked:
		return StatusOut
	case unlimited:
		return StatusNormal
	case remainingMin <= 0:
		return StatusOut
	case remainingMin <= lowThresholdMin:
		return StatusLow
	default:
		return StatusNormal
	}
}

// TrayTooltip — подсказка при наведении (ТЗ 8.2.1 п.2, без игр в M1). Длительности
// форматирует ядро (ТЗ 4): остаток «1 ч 25 мин», лимит «2 ч». Не детская учётка и блокировка
// перекрывают остаток; остаток 0 при лимите читается как «время закончилось», как красный
// значок. Лимит не меньше минуты дописывается через «из».
func TrayTooltip(remainingS int, limitS int, unlimited, locked, notChild bool) string {
	switch {
	case notChild:
		return tooltipParent
	case locked:
		return core.ReasonTimeUp
	case unlimited:
		return tooltipUnlimited
	case remainingS <= 0:
		return core.ReasonTimeUp
	}
	text := "Осталось " + core.FormatDuration(remainingS)
	if limitS >= 60 {
		text += " из " + core.FormatLimit(limitS/60)
	}
	return text
}
