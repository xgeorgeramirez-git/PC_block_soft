package core

import "fmt"

// Тексты экрана блокировки (ТЗ 8.2.3): ребёнку на «ты», без длинных тире (NFR-11).
const (
	ReasonTimeUp        = "Время на сегодня закончилось"
	ReasonBypass        = "Попытка обойти защиту"
	defaultBypassDetail = "Защита программы была нарушена"
	untilPrefix         = "Можно снова: "
	untilNever          = "когда родитель изменит правила"
)

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

// UntilText — строка «Можно снова: …» для блокировки «время вышло» (пакет, п.7):
// ближайший следующий день с ненулевым лимитом или без лимита — «завтра», дальше —
// «в среду»; если такого дня нет во всей неделе — «когда родитель изменит правила».
func UntilText(today Weekday, r Rules) string {
	for k := 1; k <= 7; k++ {
		wd := Weekday((int(today)-1+k)%7 + 1)
		lim := r.For(wd)
		if lim.Unlimited() || *lim.TotalMin > 0 {
			if k == 1 {
				return untilPrefix + "завтра"
			}
			return untilPrefix + weekdayAcc[wd-1]
		}
	}
	return untilPrefix + untilNever
}

// ExtraText — запись журнала о доп. времени, как в эталоне G7: «Доп. время +30 мин».
func ExtraText(min int) string { return fmt.Sprintf("Доп. время +%d мин", min) }
