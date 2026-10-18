package ipc

import (
	"fmt"

	"pcblock/internal/core"
)

// StatusName переводит статус ядра в строку ChildStatus.Status. Неизвестное значение
// безопасно: «ограничения не действуют» (R18).
func StatusName(s core.Status) string {
	switch s {
	case core.StatusOK:
		return StatusOK
	case core.StatusLow:
		return StatusLow
	case core.StatusTimeUp:
		return StatusTimeUp
	case core.StatusBypass:
		return StatusBypass
	case core.StatusUnlimited:
		return StatusUnlimited
	default:
		return StatusSafe
	}
}

// statusWords — статус словом для карточки родителя (ТЗ 8.0 п.3, 8.3.2).
var statusWords = map[string]string{
	StatusOK:        "Можно",
	StatusLow:       "Осталось мало",
	StatusTimeUp:    "Время вышло",
	StatusBypass:    "Заблокирован за обход защиты",
	StatusUnlimited: "Без ограничений",
	StatusSafe:      "Ограничения не действуют",
	StatusAdmin:     "Администратор: ограничения не действуют",
	StatusNotChild:  "",
}

// Тексты значка и мини-панели (ТЗ 8.2.1).
const (
	textUnlimited = "Сегодня без ограничений по времени"
	textSafe      = "Ограничения сейчас не действуют"
	textAdmin     = "Администратор: ограничения не действуют"
)

// ChildStatusFrom строит ChildStatus из решения ядра: числа и тексты экрана блокировки
// копируются, ничего не пересчитывается; надпись и подсказки значка собираются здесь, чтобы
// быть одинаковыми на всех экранах. Статусы admin и notChild ядру неизвестны: для них
// ChildStatusAdmin и ChildStatusNotChild.
func ChildStatusFrom(name string, d core.Decision) ChildStatus {
	s := ChildStatus{
		Name:       name,
		Status:     StatusName(d.Status),
		Day:        d.Day,
		RemainingS: d.RemainingS,
		LimitS:     d.LimitS,
		UsedS:      d.UsedS,
		Unlimited:  d.Unlimited,
		Lock:       lockName(d.Lock),
		Reason:     d.Reason,
		Detail:     d.Detail,
		Until:      d.Until,
		Used:       d.Used,
	}
	s.StatusText = statusWords[s.Status]
	// Остаток округляется вверх до минуты: «осталось 5 с» не должно показывать «0» при
	// открытом экране.
	remMin := (d.RemainingS + 59) / 60
	switch s.Status {
	case StatusOK, StatusLow:
		s.Label = fmt.Sprint(remMin)
		s.Tooltip = "Осталось " + core.FormatDuration(remMin*60)
		s.Summary = "Сегодня осталось " + core.FormatDuration(remMin*60) + " из " + core.FormatLimit(d.LimitS/60)
	case StatusTimeUp:
		s.Label = "0"
		s.Tooltip = core.ReasonTimeUp
		s.Summary = core.ReasonTimeUp
	case StatusBypass:
		s.Label = "!"
		s.Tooltip = core.ReasonBypass
		s.Summary = core.ReasonBypass
	case StatusUnlimited:
		s.Label = "∞"
		s.Tooltip = textUnlimited
		s.Summary = textUnlimited
	default:
		s.Label = "?"
		s.Tooltip = textSafe
		s.Summary = textSafe
	}
	return s
}

// ChildStatusAdmin — статус учётки, которая стала администратором (R1): ограничения не действуют.
func ChildStatusAdmin(name string) ChildStatus {
	return ChildStatus{
		Name: name, Status: StatusAdmin, StatusText: statusWords[StatusAdmin], Lock: LockNone,
		Tooltip: textAdmin, Summary: textAdmin, Unlimited: true,
	}
}

// ChildStatusNotChild — ответ GetChildStatus из сеанса, где работает не ребёнок: значку
// нечего показывать, кроме пункта «Настройки для родителя».
func ChildStatusNotChild() ChildStatus {
	return ChildStatus{Status: StatusNotChild, Lock: LockNone, Unlimited: true}
}

func lockName(k core.LockKind) string {
	switch k {
	case core.LockTimeUp:
		return LockTimeUp
	case core.LockBypass:
		return LockBypass
	default:
		return LockNone
	}
}

// CategoryName переводит категорию ядра в имя для снимка датчиков.
func CategoryName(c core.Category) string {
	switch c {
	case core.CatGames:
		return CatNameGames
	case core.CatVideoSocial:
		return CatNameVideoSocial
	case core.CatStudy:
		return CatNameStudy
	default:
		return CatNameOther
	}
}

// CategoryFromName переводит имя из снимка в категорию ядра; незнакомое имя — «Прочее»
// (мусор от стража не должен засчитываться как игры).
func CategoryFromName(name string) core.Category {
	switch name {
	case CatNameGames:
		return core.CatGames
	case CatNameVideoSocial:
		return core.CatVideoSocial
	case CatNameStudy:
		return core.CatStudy
	default:
		return core.CatOther
	}
}

// SampleFromCore превращает снимок ядра в снимок канала.
func SampleFromCore(s core.SessionSample) SensorSample {
	return SensorSample{
		AtMs: s.AtMs, TzOffsetMin: s.TzOffsetMin, SessionActive: s.SessionActive,
		LastInputAgoMs: s.LastInputAgoMs, Fullscreen: s.Fullscreen, Foreground: CategoryName(s.Foreground),
	}
}

// ToCore превращает снимок канала в снимок для ядра.
func (s SensorSample) ToCore() core.SessionSample {
	return core.SessionSample{
		AtMs: s.AtMs, TzOffsetMin: s.TzOffsetMin, SessionActive: s.SessionActive,
		LastInputAgoMs: s.LastInputAgoMs, Fullscreen: s.Fullscreen, Foreground: CategoryFromName(s.Foreground),
	}
}

// NewGuardWarning собирает предупреждение для порога в минутах (ТЗ 8.2.2). За 1 минуту
// страж играет звук и показывает обратный отсчёт по ChildStatus.RemainingS.
func NewGuardWarning(threshold int) GuardWarning {
	w := GuardWarning{Threshold: threshold}
	switch threshold {
	case 15:
		w.Text = "Через 15 минут время на сегодня закончится"
	case 5:
		w.Text = "Через 5 минут время закончится. Сохрани игру"
	case 1:
		w.Text = "Осталась 1 минута"
		w.Sound = true
	default:
		w.Text = fmt.Sprintf("Через %d мин время закончится", threshold)
	}
	return w
}
