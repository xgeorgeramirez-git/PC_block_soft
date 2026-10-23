package core

import (
	"fmt"
	"time"
)

// Тексты экрана блокировки (ТЗ 8.2.3): ребёнку на «ты», без длинных тире (NFR-11).
const (
	ReasonTimeUp        = "Время на сегодня закончилось"
	ReasonForbidden     = "Сейчас нельзя пользоваться компьютером"
	ReasonBypass        = "Попытка обойти защиту"
	defaultBypassDetail = "Защита программы была нарушена"
	untilPrefix         = "Можно снова: "
	untilNever          = "когда родитель изменит правила"
	// UntilBypass — «Можно снова» при блокировке за обход: её снимает только родитель паролем.
	UntilBypass = untilPrefix + "когда родитель снимет блокировку"
)

// FormatClock — момент времени как «21:00» по местному поясу (ТЗ 4).
func FormatClock(t time.Time) string { return t.Format("15:04") }

// UntilForbidden — «Можно снова: …» для блокировки по запретному интервалу (R7, G6):
// конец интервала относительно now. Сегодня → «сегодня в 07:00», завтра → «завтра в
// 07:00», дальше — «в среду в 07:00».
func UntilForbidden(now, end time.Time) string {
	return untilPrefix + dayWord(now, end) + " в " + FormatClock(end)
}

// RemainingMinutes — ЕДИНОЕ правило перевода остатка времени в минуты (B-5, класс D).
// Остаток округляется ВВЕРХ: «осталось 5 с» показывает «1 мин», а не «0», пока время ещё
// есть; ноль и отрицательное дают 0. Этим правилом остаток считают и значок у часов, и
// строки окна родителя (ipc), чтобы они не расходились на минуту. Прошедшее время (сколько
// использовано) округляется ВНИЗ через FormatDuration — это разные величины.
func RemainingMinutes(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	return (seconds + 59) / 60
}

// FormatDuration — длительность по ТЗ 4: меньше часа «40 мин», иначе «1 ч 05 мин».
// Неполная минута отбрасывается, отрицательное — «0 мин».
func FormatDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	min := seconds / 60
	if min < 60 {
		return fmt.Sprintf("%d мин", min)
	}
	return fmt.Sprintf("%d ч %02d мин", min/60, min%60)
}

// FormatLimit — лимит в минутах: «20 мин», «2 ч» для целых часов, иначе «1 ч 30 мин».
func FormatLimit(min int) string {
	if min < 60 {
		return fmt.Sprintf("%d мин", min)
	}
	if min%60 == 0 {
		return fmt.Sprintf("%d ч", min/60)
	}
	return fmt.Sprintf("%d ч %02d мин", min/60, min%60)
}

// UsedText — строка экрана блокировки «Сегодня: 2 ч 00 мин из 2 ч»; limitMin — лимит
// дня вместе с доп. временем; без лимита — «Сегодня: 40 мин».
func UsedText(totalS, limitMin int, unlimited bool) string {
	if unlimited {
		return "Сегодня: " + FormatDuration(totalS)
	}
	return "Сегодня: " + FormatDuration(totalS) + " из " + FormatLimit(limitMin)
}

// weekdayAcc — день недели в винительном падеже с предлогом: «в среду», «во вторник».
var weekdayAcc = [7]string{"в понедельник", "во вторник", "в среду", "в четверг", "в пятницу", "в субботу", "в воскресенье"}

// dayWord — «сегодня» / «завтра» / «в среду» для момента t относительно now (по датам).
func dayWord(now, t time.Time) string {
	days := daysBetween(now, t)
	switch days {
	case 0:
		return "сегодня"
	case 1:
		return "завтра"
	default:
		return weekdayAcc[int(WeekdayOf(t))-1]
	}
}

// daysBetween — разница в календарных днях (по местным датам) между now и t.
func daysBetween(now, t time.Time) int {
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	a := time.Date(y1, m1, d1, 0, 0, 0, 0, now.Location())
	b := time.Date(y2, m2, d2, 0, 0, 0, 0, t.Location())
	return int(b.Sub(a).Hours() / 24)
}

// UntilText — строка «Можно снова: …» для блокировки «время вышло» (пакет, п.7):
// ближайший следующий день с ненулевым лимитом или без лимита — «завтра», дальше —
// «в среду»; если начало этого дня попадает в запретный интервал, пишется конец интервала
// («завтра в 07:00», G1 примечание, G6); если подходящего дня нет во всей неделе —
// «когда родитель изменит правила». now — текущий момент (для «сегодня/завтра»).
func UntilText(today Weekday, r Rules, now time.Time) string {
	for k := 1; k <= 7; k++ {
		wd := Weekday((int(today)-1+k)%7 + 1)
		lim := r.For(wd)
		if !(lim.Unlimited() || *lim.TotalMin > 0) {
			continue
		}
		// Полночь этого дня: если она в запретном интервале, разрешено лишь с конца интервала.
		dayStart := startOfDayAfter(now, k)
		if in, end := InForbidden(r, dayStart); in {
			return UntilForbidden(now, end)
		}
		if k == 1 {
			return untilPrefix + "завтра"
		}
		return untilPrefix + weekdayAcc[wd-1]
	}
	return untilPrefix + untilNever
}

// startOfDayAfter возвращает местную полночь через k суток от даты now.
func startOfDayAfter(now time.Time, k int) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, k)
}

// ExtraText — запись журнала о доп. времени, как в эталоне G7: «Доп. время +30 мин».
func ExtraText(min int) string { return fmt.Sprintf("Доп. время +%d мин", min) }

// PromoRedeemText — строка журнала о применённом промокоде (R26, D-50).
func PromoRedeemText(min int) string { return fmt.Sprintf("Промокод: +%d мин", min) }

// ExtraRemovedText — запись журнала об убавлении доп. времени (R10, D-43): «Доп. время −30 мин».
func ExtraRemovedText(min int) string { return fmt.Sprintf("Доп. время −%d мин", min) }

// ExtraClearedText — запись журнала об обнулении доп. времени (R10, D-43).
const ExtraClearedText = "Доп. время убрано"

// PauseText — запись журнала о паузе, как в эталоне G11: «Пауза 1 ч». Длительность —
// от now до until, округляется к ближайшей читаемой величине (минуты/часы).
func PauseText(now, until time.Time) string {
	min := int(until.Sub(now).Minutes())
	if min < 0 {
		min = 0
	}
	return "Пауза " + FormatLimit(min)
}

// PauseEndText — запись журнала об окончании паузы (R11).
const PauseEndText = "Пауза закончилась"

// NoForbiddenOnText и NoForbiddenOffText — записи журнала о снятии и возврате запретных
// часов на сегодня (R24, D-51).
const (
	NoForbiddenOnText  = "Запретные часы сняты на сегодня"
	NoForbiddenOffText = "Запретные часы снова действуют сегодня"
)

// BonusSpentText — запись журнала о трате из копилки (FR-11): «Взято из копилки 30 мин».
func BonusSpentText(min int) string {
	return fmt.Sprintf("Взято из копилки %d мин", min)
}
