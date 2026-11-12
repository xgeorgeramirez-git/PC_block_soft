package service

import (
	"fmt"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Столбики «По дням» и снимок очистки статистики (M5: RM5-2, RM5-3, RM5-4, RM5-8). Столбики и
// «Итоги» строятся из одного чтения ряда дней (DaysSeries) и одних счётчиков ядра за сегодня, поэтому
// сумма столбиков равна «Итогам» до секунды (класс A).

// monthsShort — подпись месяца столбика: «ноя».
var monthsShort = [12]string{"янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}

// monthsNom — месяц в именительном падеже для подсказки «ноябрь 2026: …».
var monthsNom = [12]string{"январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"}

// barTitle — подсказка столбика (RM5-3): «<когда>: 2 ч 20 мин, учёба 0 мин».
func barTitle(when string, totalS, studyS int) string {
	return when + ": " + core.FormatDuration(totalS) + ", учёба " + core.FormatDuration(studyS)
}

// noonOf — полдень дня day (YYYY-MM-DD) в поясе loc: шаг по дням от полудня не сбивается переходом
// на летнее время в полночь. Не дата — ok = false.
func noonOf(day string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation(time.DateOnly, day, loc)
	if err != nil {
		return time.Time{}, false
	}
	y, m, d := t.Date()
	return time.Date(y, m, d, 12, 0, 0, 0, loc), true
}

// buildBars — столбики периода (RM5-2, RM5-3). from — исходное начало периода (календарь «Недели»,
// «Месяца», «Года» строится от него; дни до очистки — пустые столбики); days — строки базы с начала
// данных до today не включительно (уже с учётом очистки, applyReset); live — сегодня из ядра (уже
// минус снимок; нули, если ядро ещё во вчерашнем дне). «Сегодня», «Неделя», «Месяц» — по дню;
// «Год» — по месяцу с января; «Всё время» (from == allFromDay) — по месяцу с месяца первой строки
// days, нет строк — текущий месяц (после очистки первая строка — день очистки). Дни и месяцы без
// данных внутри диапазона — нулевые столбики. Высота — только время (D-72, D-78): лимита здесь нет.
func buildBars(period, from, today string, days []DayRow, live core.DayState, loc *time.Location) (bars []ipc.DayBar, by string) {
	bars = []ipc.DayBar{}
	end, ok := noonOf(today, loc)
	if !ok {
		return bars, ipc.BarsByDay
	}
	type sum struct{ total, study int }
	byKey := map[string]sum{}
	add := func(key string, total, study int) {
		s := byKey[key]
		byKey[key] = sum{s.total + total, s.study + study}
	}
	monthly := period == ipc.PeriodYear || period == ipc.PeriodAll
	keyOf := func(day string) string {
		if monthly && len(day) >= 7 {
			return day[:7]
		}
		return day
	}
	for _, d := range days {
		if d.Day < today {
			add(keyOf(d.Day), d.TotalS, d.StudyS)
		}
	}
	add(keyOf(today), live.TotalS, live.StudyS)

	if !monthly {
		start, ok := noonOf(from, loc)
		if !ok || start.After(end) {
			start = end
		}
		for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
			day := t.Format(time.DateOnly)
			s := byKey[day]
			wd := weekdayShort[(int(t.Weekday())+6)%7]
			bars = append(bars, ipc.DayBar{
				Key: day, Label: fmt.Sprintf("%s %d", wd, t.Day()),
				Title:  barTitle(fmt.Sprintf("%s, %d %s", wd, t.Day(), months[t.Month()-1]), s.total, s.study),
				TotalS: s.total, StudyS: s.study, Today: day == today,
			})
		}
		return bars, ipc.BarsByDay
	}

	first := from
	if period == ipc.PeriodAll {
		first = today
		if len(days) > 0 && days[0].Day < today {
			first = days[0].Day
		}
	}
	start, ok := noonOf(first, loc)
	if !ok || start.After(end) {
		start = end
	}
	cur := time.Date(start.Year(), start.Month(), 1, 12, 0, 0, 0, loc)
	last := time.Date(end.Year(), end.Month(), 1, 12, 0, 0, 0, loc)
	withYear := cur.Year() != last.Year()
	for t := cur; !t.After(last); t = t.AddDate(0, 1, 0) {
		key := t.Format("2006-01")
		s := byKey[key]
		label := monthsShort[t.Month()-1]
		if withYear {
			label += fmt.Sprintf(" %d", t.Year())
		}
		bars = append(bars, ipc.DayBar{
			Key: key, Label: label,
			Title:  barTitle(fmt.Sprintf("%s %d", monthsNom[t.Month()-1], t.Year()), s.total, s.study),
			TotalS: s.total, StudyS: s.study, Today: key == keyOf(today),
		})
	}
	return bars, ipc.BarsByMonth
}

// minusFloor — a − b, не меньше нуля (доп. время могли убавить после очистки, RM5-8).
func minusFloor(a, b int) int { return max(0, a-b) }

// applyReset вычитает снимок очистки (RM5-8): строки с Day < r.Day выбрасываются, строка дня
// очистки — минус снимок с нижней границей 0 по каждому полю; ok = false — строки как есть.
func applyReset(days []DayRow, r StatsReset, ok bool) []DayRow {
	if !ok {
		return days
	}
	out := make([]DayRow, 0, len(days))
	for _, d := range days {
		switch {
		case d.Day < r.Day:
			continue
		case d.Day == r.Day:
			d = DayRow{Day: d.Day, TotalS: minusFloor(d.TotalS, r.TotalS), GamesS: minusFloor(d.GamesS, r.GamesS),
				VideoS: minusFloor(d.VideoS, r.VideoS), StudyS: minusFloor(d.StudyS, r.StudyS), ExtraMin: minusFloor(d.ExtraMin, r.ExtraMin)}
		}
		out = append(out, d)
	}
	return out
}

// liveMinusReset — сегодняшние счётчики ядра для статистики (RM5-8): если очистка была в этот же
// день ядра — минус снимок с нижней границей 0; иначе как есть. Учёт лимита (остаток) этим не
// меняется: здесь только «Итоги» и столбик.
func liveMinusReset(live core.DayState, r StatsReset, ok bool) core.DayState {
	if !ok || live.Day == "" || live.Day != r.Day {
		return live
	}
	live.TotalS = minusFloor(live.TotalS, r.TotalS)
	live.GamesS = minusFloor(live.GamesS, r.GamesS)
	live.VideoS = minusFloor(live.VideoS, r.VideoS)
	live.StudyS = minusFloor(live.StudyS, r.StudyS)
	live.ExtraMin = minusFloor(live.ExtraMin, r.ExtraMin)
	return live
}

// dayStartMs — местная полночь дня from в мс UTC; «Всё время» и не дата — 0 (без нижней границы).
func (s *Service) dayStartMs(from string) int64 {
	if from == allFromDay {
		return 0
	}
	t, err := time.ParseInLocation(time.DateOnly, from, s.cfg.Loc)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// violationKinds — виды событий-нарушений за период: группа «Нарушения» журнала (RM5-5).
var violationKinds = journalGroups[ipc.JournalGroupViolations]

// periodViolations — нарушения ребёнка с fromMs, новые сверху, не больше ipc.StatsListLimit; текст —
// как в журнале (eventText). От очистки статистики не зависят (журнал отдельно, D-73). Ошибка — [].
func (s *Service) periodViolations(cs *childState, fromMs int64) []ipc.JournalEntry {
	out := []ipc.JournalEntry{}
	evs, err := s.cfg.Store.ListEvents(EventQuery{ChildID: cs.child.ID, FromMs: fromMs, Kinds: violationKinds, Limit: ipc.StatsListLimit})
	if err != nil {
		s.logRate("violations", "не удалось прочитать нарушения у %s: %v", cs.child.Name, err)
		return out
	}
	for _, e := range evs {
		if e.ChildID != cs.child.ID {
			continue
		}
		out = append(out, s.journalEntryOf(e))
		if len(out) >= ipc.StatsListLimit {
			break
		}
	}
	return out
}

// bonusEntryOf — запись копилки для окна: пояс службы на момент записи (окно показывает время
// записи по местным часам). Один вид записи у экрана «Копилка» и у статистики.
func (s *Service) bonusEntryOf(r BonusRecord) ipc.BonusEntry {
	_, off := time.UnixMilli(r.AtMs).In(s.cfg.Loc).Zone()
	return ipc.BonusEntry{AtMs: r.AtMs, TzOffset: off / 60, DeltaMin: r.DeltaMin, Reason: r.Reason, Balance: r.Balance}
}

// periodBonus — копилка за период из одного чтения истории: «Взято из копилки» с spentFromMs (начало
// периода или очистка статистики, что позже, RM5-8) и записи с fromMs (история копилки от очистки не
// зависит), новые сверху, не больше ipc.StatsListLimit. Ошибка — 0 и [].
func (s *Service) periodBonus(cs *childState, fromMs, spentFromMs int64) (spent int, entries []ipc.BonusEntry) {
	entries = []ipc.BonusEntry{}
	list, err := s.cfg.Store.ListBonus(cs.child.ID)
	if err != nil {
		s.logRate("bonusspent", "не удалось прочитать копилку у %s: %v", cs.child.Name, err)
		return 0, entries
	}
	for _, r := range list { // новые первыми
		if r.AtMs < fromMs {
			continue
		}
		entries = append(entries, s.bonusEntryOf(r))
		if len(entries) >= ipc.StatsListLimit {
			break
		}
	}
	return bonusSpentSince(list, spentFromMs), entries
}

// statsSince — с какого дня ведётся статистика (RM5-8): местная дата очистки; без очистки — первый
// день учёта ребёнка; без строк (или ошибка чтения) — сегодня. Текст — «Статистика ведётся с 11 ноября 2026».
func (s *Service) statsSince(cs *childState, today string, reset StatsReset, resetOK bool) (day, text string) {
	day = today
	if resetOK {
		day = time.UnixMilli(reset.ClearedAtMs).In(s.cfg.Loc).Format(time.DateOnly)
	} else if first, ok, err := s.cfg.Store.FirstStatsDay(cs.child.ID); err != nil {
		s.logRate("statssince", "не удалось прочитать первый день статистики у %s: %v", cs.child.Name, err)
	} else if ok && first < today {
		day = first
	}
	t, err := time.ParseInLocation(time.DateOnly, day, s.cfg.Loc)
	if err != nil {
		return day, ""
	}
	return day, fmt.Sprintf("Статистика ведётся с %d %s %d", t.Day(), months[t.Month()-1], t.Year())
}
