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

		GamesRemainingS: d.GamesRemainingS,
		GamesLimitS:     d.GamesLimitS,
		GamesUnlimited:  d.GamesUnlimited,
		CloseGames:      d.CloseGames,
		StudyOnly:       d.StudyOnly,
		// Правило ребёнка ядро в решение не кладёт: служба ставит его из Rules (statusOf); здесь —
		// то, что видно по решению («только учёба» сейчас действует или нет).
		EndMode: EndModeLock,
	}
	if d.StudyOnly {
		s.EndMode = EndModeStudyOnly
	}
	s.StatusText = statusWords[s.Status]
	// Остаток в минуты — единым правилом ядра (B-5): то же, что считает значок у часов.
	remMin := core.RemainingMinutes(d.RemainingS)
	switch s.Status {
	case StatusOK, StatusLow:
		s.Label = fmt.Sprint(remMin)
		s.Tooltip = "Осталось " + core.FormatDuration(remMin*60)
		if hasGamesLimit(d) {
			// Вторая строка подсказки (ТЗ 8.2.1 п.2): остаток игр тем же правилом минут (B-5).
			s.Tooltip += "\nИгры: " + core.FormatDuration(core.RemainingMinutes(d.GamesRemainingS)*60)
		}
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
	if d.StudyOnly && d.CloseReason != "" {
		// «Оставить только учёбу» (R8): экрана блокировки нет (Lock из решения ядра), значок и
		// мини-панель говорят, что доступна только учёба.
		s.Tooltip, s.Summary = d.CloseReason, d.CloseReason
	}
	return s
}

// hasGamesLimit: у решения есть отдельный лимит игр. Ядро всегда заполняет GamesUnlimited; у
// нулевого решения (старый код, тесты) его нет — тогда лимит виден по лимиту игр или закрытию игр
// (лимит 0 ядро отдаёт как CloseGames).
func hasGamesLimit(d core.Decision) bool {
	return !d.GamesUnlimited && (d.GamesLimitS > 0 || d.CloseGames)
}

// GamesCardText — строка игр карточки «Сегодня» (ТЗ 8.3.2 п.3): «Игры 40 мин из 1 ч». usedS —
// игры за день (прошедшее время, вниз), limitS — лимит игр с доп. временем; без отдельного
// лимита игр — пусто.
func GamesCardText(usedS, limitS int, unlimited bool) string {
	if unlimited {
		return ""
	}
	return "Игры " + core.FormatDuration(usedS) + " из " + core.FormatLimit(limitS/60)
}

// ChildStatusAdmin — статус учётки, которая стала администратором (R1): ограничения не действуют.
func ChildStatusAdmin(name string) ChildStatus {
	return ChildStatus{
		Name: name, Status: StatusAdmin, StatusText: statusWords[StatusAdmin], Lock: LockNone,
		Tooltip: textAdmin, Summary: textAdmin, Unlimited: true, GamesUnlimited: true, EndMode: EndModeLock,
	}
}

// ChildStatusNotChild — ответ GetChildStatus из сеанса, где работает не ребёнок: значку
// нечего показывать, кроме пункта «Настройки для родителя».
func ChildStatusNotChild() ChildStatus {
	return ChildStatus{Status: StatusNotChild, Lock: LockNone, Unlimited: true, GamesUnlimited: true, EndMode: EndModeLock}
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
	return NewGuardWarningFor(threshold, core.WarnLimit, threshold*60)
}

// NewGuardWarningFor собирает предупреждение для порога в минутах, вида ближайшего события
// (R9) и секунд до него eventS: исчерпание времени, начало запретных часов или конец паузы.
// Текст говорит, что именно кончится (FR-9); звук на каждом пороге (D-55).
func NewGuardWarningFor(threshold int, ev core.WarnEvent, eventS int) GuardWarning {
	w := GuardWarning{Threshold: threshold, Sound: true, Event: warnEventName(ev), EventS: eventS}
	what, whatShort := warnSubject(ev)
	switch threshold {
	case 30, 15:
		w.Text = fmt.Sprintf("Через %d минут %s", threshold, what)
	case 5:
		switch ev {
		case core.WarnForbidden, core.WarnPauseEnd:
			w.Text = fmt.Sprintf("Через 5 минут %s", what)
		case core.WarnGames:
			w.Text = "Сохрани игру: через 5 минут " + what
		default:
			w.Text = "Через 5 минут время закончится. Сохрани игру"
		}
	case 1:
		w.Text = warnOneMinute(ev)
	default:
		w.Text = fmt.Sprintf("Через %d мин %s", threshold, whatShort)
	}
	return w
}

// warnEventName — вид события для канала связи; неизвестный вид считается лимитом.
func warnEventName(ev core.WarnEvent) string {
	switch ev {
	case core.WarnForbidden:
		return WarnEventForbidden
	case core.WarnPauseEnd:
		return WarnEventPause
	case core.WarnGames:
		return WarnEventGames
	default:
		return WarnEventLimit
	}
}

// warnSubject — что кончится, для порогов 15/5 мин и запасного текста.
func warnSubject(ev core.WarnEvent) (full, short string) {
	switch ev {
	case core.WarnForbidden:
		return "начнутся запретные часы", "начнутся запретные часы"
	case core.WarnPauseEnd:
		return "закончится пауза", "закончится пауза"
	case core.WarnGames:
		return "время на игры закончится", "время на игры закончится"
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
	case core.WarnGames:
		return "Осталась 1 минута на игры" // страж дописывает отсчёт: «…: 0:59»
	default:
		return "Осталась 1 минута"
	}
}
