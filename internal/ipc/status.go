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
	case core.StatusForbidden:
		return StatusForbidden
	case core.StatusBypass:
		return StatusBypass
	case core.StatusUnlimited:
		return StatusUnlimited
	case core.StatusPaused:
		return StatusPaused
	default:
		return StatusSafe
	}
}

// statusWords — статус словом для карточки родителя (ТЗ 8.0 п.3, 8.3.2).
var statusWords = map[string]string{
	StatusOK:        "Можно",
	StatusLow:       "Осталось мало",
	StatusTimeUp:    "Время вышло",
	StatusForbidden: "Запретные часы",
	StatusBypass:    "Заблокирован за обход защиты",
	StatusUnlimited: "Без ограничений",
	StatusPaused:    "Пауза",
	StatusSafe:      "Ограничения не действуют",
	StatusAdmin:     "Администратор: ограничения не действуют",
	StatusNotChild:  "",
}

// Тексты значка и мини-панели (ТЗ 8.2.1).
const (
	textUnlimited = "Сегодня без ограничений по времени"
	textSafe      = "Ограничения сейчас не действуют"
	textAdmin     = "Администратор: ограничения не действуют"
	textPaused    = "Пауза. Ограничения не действуют"
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
		PauseUntil: d.PauseUntil,
		Used:       d.Used,
	}
	s.StatusText = statusWords[s.Status]
	// Остаток в минуты — единым правилом ядра (B-5): то же, что считает значок у часов.
	remMin := core.RemainingMinutes(d.RemainingS)
	switch s.Status {
	case StatusOK, StatusLow:
		s.Label = fmt.Sprint(remMin)
		s.Tooltip = "Осталось " + core.FormatDuration(remMin*60)
		s.Summary = "Сегодня осталось " + core.FormatDuration(remMin*60) + " из " + core.FormatLimit(d.LimitS/60)
	case StatusTimeUp:
		s.Label = "0"
		s.Tooltip = core.ReasonTimeUp
		s.Summary = core.ReasonTimeUp
	case StatusForbidden:
		s.Label = "0"
		s.Tooltip = core.ReasonForbidden
		s.Summary = core.ReasonForbidden
	case StatusBypass:
		s.Label = "!"
		s.Tooltip = core.ReasonBypass
		s.Summary = core.ReasonBypass
	case StatusUnlimited:
		s.Label = "∞"
		s.Tooltip = textUnlimited
		s.Summary = textUnlimited
	case StatusPaused:
		// Пауза: значок показывает остаток, мини-панель — конец паузы (ТЗ 8.2.1 п.4).
		s.Label = fmt.Sprint(remMin)
		s.Tooltip = textPaused
		s.Summary = textPaused
		if d.PauseUntil != "" {
			s.Summary = "Пауза до " + d.PauseUntil + ". Ограничения не действуют"
		}
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
	case core.LockForbidden:
		return LockForbidden
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

// NewGuardWarning собирает предупреждение для порога об исчерпании общего времени (ТЗ
// 8.2.2) — частый случай; для события другого вида — NewGuardWarningFor.
func NewGuardWarning(threshold int) GuardWarning {
	return NewGuardWarningFor(threshold, core.WarnLimit)
}

// NewGuardWarningFor собирает предупреждение для порога в минутах и вида ближайшего
// события (R9): исчерпание времени, начало запретных часов или конец паузы — текст
// говорит, что именно кончится (FR-9). За 1 минуту страж играет звук и показывает
// обратный отсчёт по ChildStatus.RemainingS.
func NewGuardWarningFor(threshold int, ev core.WarnEvent) GuardWarning {
	w := GuardWarning{Threshold: threshold}
	what, whatShort := warnSubject(ev)
	switch threshold {
	case 15:
		w.Text = fmt.Sprintf("Через 15 минут %s", what)
	case 5:
		if ev == core.WarnLimit {
			w.Text = "Через 5 минут время закончится. Сохрани игру"
		} else {
			w.Text = fmt.Sprintf("Через 5 минут %s", what)
		}
	case 1:
		w.Text = warnOneMinute(ev)
		w.Sound = true
	default:
		w.Text = fmt.Sprintf("Через %d мин %s", threshold, whatShort)
	}
	return w
}

// warnSubject — что кончится, для порогов 15/5 мин и запасного текста.
func warnSubject(ev core.WarnEvent) (full, short string) {
	switch ev {
	case core.WarnForbidden:
		return "начнутся запретные часы", "начнутся запретные часы"
	case core.WarnPauseEnd:
		return "закончится пауза", "закончится пауза"
	default:
		return "время на сегодня закончится", "время закончится"
	}
}

// warnOneMinute — текст предупреждения за 1 минуту по виду события.
func warnOneMinute(ev core.WarnEvent) string {
	switch ev {
	case core.WarnForbidden:
		return "Через минуту начнутся запретные часы"
	case core.WarnPauseEnd:
		return "Через минуту закончится пауза"
	default:
		return "Осталась 1 минута"
	}
}
