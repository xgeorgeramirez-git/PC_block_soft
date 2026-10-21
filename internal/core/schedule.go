package core

import (
	"sort"
	"time"
)

// maxIntervalsPerDay — до 6 запретных интервалов на день недели (R7).
const maxIntervalsPerDay = 6

// dayMinutes — минут в сутках.
const dayMinutes = 24 * 60

// Interval — запретный интервал одного дня недели (R7). StartMin и EndMin — минуты от
// полуночи дня начала (Weekday). EndMin <= StartMin означает перенос через полночь:
// интервал идёт до EndMin следующих суток, но принадлежит дню своего начала
// (пример: пт 21:00–07:00 действует с пятницы 21:00 до субботы 07:00).
type Interval struct {
	Weekday  Weekday
	StartMin int
	EndMin   int
}

// crossesMidnight сообщает, что интервал переходит через полночь.
func (iv Interval) crossesMidnight() bool { return iv.EndMin <= iv.StartMin }

// absEnd возвращает конец интервала на оси [0, 2880) минут от полуночи дня начала:
// для переноса через полночь к концу прибавляются сутки.
func (iv Interval) absEnd() int {
	if iv.crossesMidnight() {
		return iv.EndMin + dayMinutes
	}
	return iv.EndMin
}

// NormalizeIntervals сортирует интервалы по дню недели и началу, объединяет
// пересекающиеся и смежные в пределах одного дня начала (ось [0, 2880) с учётом
// переноса через полночь) и оставляет не больше 6 на день (R7). Детерминирована;
// nil и пустой вход дают пустой результат (R18). Интервал с нулевой или
// неположительной длиной отбрасывается.
func NormalizeIntervals(in []Interval) []Interval {
	byDay := map[Weekday][]Interval{}
	for _, iv := range in {
		if iv.Weekday < Monday || iv.Weekday > Sunday {
			continue
		}
		if iv.StartMin < 0 || iv.StartMin >= dayMinutes || iv.EndMin < 0 || iv.EndMin >= dayMinutes {
			continue
		}
		byDay[iv.Weekday] = append(byDay[iv.Weekday], iv)
	}
	out := []Interval{}
	for wd := Monday; wd <= Sunday; wd++ {
		out = append(out, mergeDay(wd, byDay[wd])...)
	}
	return out
}

// mergeDay объединяет интервалы одного дня начала и обрезает до 6 (R7).
func mergeDay(wd Weekday, ivs []Interval) []Interval {
	if len(ivs) == 0 {
		return nil
	}
	// Каждый интервал — отрезок [start, absEnd) на оси [0, 2880).
	type seg struct{ start, end int }
	segs := make([]seg, 0, len(ivs))
	for _, iv := range ivs {
		segs = append(segs, seg{iv.StartMin, iv.absEnd()})
	}
	sort.Slice(segs, func(i, j int) bool {
		if segs[i].start != segs[j].start {
			return segs[i].start < segs[j].start
		}
		return segs[i].end < segs[j].end
	})
	merged := []seg{segs[0]}
	for _, s := range segs[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end { // пересечение или смежность
			if s.end > last.end {
				last.end = s.end
			}
			continue
		}
		merged = append(merged, s)
	}
	if len(merged) > maxIntervalsPerDay {
		merged = merged[:maxIntervalsPerDay]
	}
	out := make([]Interval, 0, len(merged))
	for _, s := range merged {
		end := s.end
		if end >= dayMinutes {
			end -= dayMinutes
		}
		out = append(out, Interval{Weekday: wd, StartMin: s.start, EndMin: end})
	}
	return out
}

// intervalWindow возвращает абсолютный отрезок [start, end) интервала как моменты
// времени: start — в сутки дня недели интервала на неделе момента ref, end — start плюс
// длительность (через полночь end попадает в следующие сутки). ref задаёт неделю.
func intervalWindow(iv Interval, ref time.Time) (time.Time, time.Time) {
	loc := ref.Location()
	// Полночь дня недели интервала в ту же неделю, что ref (ближайшая не позже ref+запас).
	refMidnight := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, loc)
	deltaDays := (int(iv.Weekday) - int(WeekdayOf(ref)) + 7) % 7
	// Берём как текущую, так и предыдущую неделю, чтобы поймать перенос: вернём «главное»
	// окно для этой недели; вызывающий перебирает недели отдельно.
	dayStart := refMidnight.AddDate(0, 0, deltaDays)
	start := dayStart.Add(time.Duration(iv.StartMin) * time.Minute)
	end := dayStart.Add(time.Duration(iv.absEnd()) * time.Minute)
	return start, end
}

// InForbidden сообщает, попадает ли момент t в запретный интервал, и возвращает момент
// конца действующего интервала (для «Можно снова в 07:00»). Проверяются интервалы, чьё
// окно (с учётом переноса через полночь и соседних недель) накрывает t.
func InForbidden(r Rules, t time.Time) (bool, time.Time) {
	for _, iv := range r.Schedule {
		// Окна интервала в текущую и предыдущую неделю (перенос через полночь может
		// начаться в прошлых сутках).
		for _, wkShift := range []int{0, -7} {
			start, end := intervalWindow(iv, t.AddDate(0, 0, wkShift*0)) // база — t
			start = start.AddDate(0, 0, wkShift)
			end = end.AddDate(0, 0, wkShift)
			if !t.Before(start) && t.Before(end) {
				return true, end
			}
		}
	}
	return false, time.Time{}
}

// NextForbiddenStart возвращает ближайшее начало запретного интервала не раньше t в
// пределах ближайших суток (для предупреждений R9 и экрана блокировки). Если t уже
// внутри интервала, его начало в прошлом и здесь не возвращается; начала за горизонтом
// суток (следующая неделя того же дня) не считаются «ближайшим событием».
func NextForbiddenStart(r Rules, t time.Time) (time.Time, bool) {
	horizon := t.AddDate(0, 0, 1) // ближайшие сутки
	var best time.Time
	found := false
	for _, iv := range r.Schedule {
		start, _ := intervalWindow(iv, t)
		if start.Before(t) || !start.Before(horizon) {
			continue
		}
		if !found || start.Before(best) {
			best, found = start, true
		}
	}
	return best, found
}

// IntervalsFor возвращает интервалы, действующие в сутки дня недели wd, включая перенос
// интервала предыдущего дня через полночь. Порядок — как в NormalizeIntervals.
func (r Rules) IntervalsFor(wd Weekday) []Interval {
	prev := Weekday((int(wd)+5)%7 + 1) // предыдущий день недели
	out := []Interval{}
	for _, iv := range NormalizeIntervals(r.Schedule) {
		switch {
		case iv.Weekday == wd:
			out = append(out, iv)
		case iv.Weekday == prev && iv.crossesMidnight():
			out = append(out, iv) // часть после полуночи действует в сутки wd
		}
	}
	return out
}
