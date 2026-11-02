package core

import "time"

// InactivityLimitMs — порог бездействия R2а: секунда активна, если ввод был менее 300 с назад.
const InactivityLimitMs int64 = 300_000

// IsActive сообщает, засчитывается ли секунда по снимку датчиков (R2): сеанс активен и
// выполнено хотя бы одно: ввод менее inactivityMs назад, окно на весь экран, игра или
// сайт «Видео и соцсети» на переднем плане. Экран блокировки проверяет вызывающий.
func IsActive(s SessionSample, inactivityMs int64) bool {
	if !s.SessionActive {
		return false
	}
	return s.LastInputAgoMs < inactivityMs ||
		s.Fullscreen ||
		s.Foreground == CatGames ||
		s.Foreground == CatVideoSocial
}

// CategoryOf возвращает категорию секунды по переднему плану (R3); неизвестное — «Прочее».
func CategoryOf(s SessionSample) Category {
	switch s.Foreground {
	case CatGames, CatVideoSocial, CatStudy:
		return s.Foreground
	default:
		return CatOther
	}
}

// Accumulator — накопитель активных секунд одного ребёнка по дням и категориям.
// Тик засчитывает только свою секунду [now, now+1 с): разрыв между тиками любой длины
// не досчитывается (Ruling 13). Сутки меняются в местную полночь (R16).
type Accumulator struct {
	state  DayState
	closed []DayState
}

// NewAccumulator начинает учёт с состояния st (пустое Day — день возьмётся из первого тика).
func NewAccumulator(st DayState) *Accumulator {
	return &Accumulator{state: st, closed: []DayState{}}
}

// Advance переводит учёт на сутки момента now (R16): прошлый день уходит в закрытые
// (ClosedDays), новый начинается с нуля и без доп. времени. Возвращает true, если сутки
// сменились. Часы, ушедшие назад за полночь, прошлый день не открывают: учёт остаётся
// в текущем дне (иначе служба перезаписала бы итог прошлого дня меньшими числами).
func (a *Accumulator) Advance(now time.Time) bool {
	day := DayOf(now)
	if !laterDay(day, a.state.Day) {
		return false
	}
	if a.state.Day != "" {
		a.closed = append(a.closed, a.state)
	}
	a.state = DayState{Day: day}
	return true
}

// Tick засчитывает секунду [now, now+1 с) по снимку s: сначала смена суток, затем +1 с
// в категорию секунды, если секунда активна (R2) и не заблокирована: blocked — показан экран
// блокировки или программе этой категории сейчас нельзя работать (Decision.Allows, RM3-7).
// Возвращает true, если секунда засчитана.
func (a *Accumulator) Tick(now time.Time, s SessionSample, blocked bool) bool {
	a.Advance(now)
	if blocked || !IsActive(s, InactivityLimitMs) {
		return false
	}
	switch CategoryOf(s) {
	case CatGames:
		a.state.GamesS++
		a.state.TotalS++
	case CatVideoSocial:
		a.state.VideoS++
		a.state.TotalS++
	case CatStudy:
		a.state.StudyS++
	default:
		a.state.TotalS++
	}
	return true
}

// laterDay сообщает, что day позже текущего дня cur (пустой cur — дня ещё нет).
// Даты YYYY-MM-DD сравниваются как строки.
func laterDay(day, cur string) bool { return cur == "" || day > cur }

// AddExtra добавляет доп. время к текущему дню (R10); границы проверяет Engine.
func (a *Accumulator) AddExtra(min int) { a.state.ExtraMin += min }

// SetExtra устанавливает доп. время текущего дня (R10, D-43: убавление и обнуление);
// границы проверяет Engine.
func (a *Accumulator) SetExtra(min int) { a.state.ExtraMin = min }

// SetPause ставит момент конца паузы текущего дня (R11); nil снимает паузу.
func (a *Accumulator) SetPause(untilMs *int64) { a.state.PauseUntilMs = untilMs }

// SetLastMinute ставит момент «конец по часам» последней минуты (R2, D-60); nil снимает отсчёт.
func (a *Accumulator) SetLastMinute(endMs *int64) { a.state.LastMinuteEndMs = endMs }

// SetTotal поднимает общее время текущего дня до total (дотяжка последней минуты, R2, D-60);
// разница идёт в «Прочее» (общее минус игры и видео), остальные категории не меняются.
func (a *Accumulator) SetTotal(total int) { a.state.TotalS = total }

// SetNoForbidden снимает или возвращает запретные часы на текущий день (R24, D-51).
func (a *Accumulator) SetNoForbidden(on bool) { a.state.NoForbidden = on }

// State возвращает счётчики текущего дня.
func (a *Accumulator) State() DayState { return a.state }

// ClosedDays отдаёт закрытые сменой суток дни (для сохранения) и очищает список.
// Нет закрытых — пустой список, не nil.
func (a *Accumulator) ClosedDays() []DayState {
	out := a.closed
	a.closed = []DayState{}
	return out
}
