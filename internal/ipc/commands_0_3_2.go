package ipc

import "pcblock/internal/core"

// Перерыв на отдых (фидбек-раунд 0.3.2: FB-14, D-62, R27, FR-37).

// CmdEndBreak — родитель паролем закончил перерыв (окно родителя или экран перерыва стража).
// Нагрузка — ChildRequest, ответ — CardReply.
const CmdEndBreak = "EndBreak"

// LockRest — экран «Перерыв на отдых» (ChildStatus.Lock): без «Попросить ещё», копилки и промокода.
const LockRest = "rest"

// WarnEventRest — плашка «ПОРА ОТДОХНУТЬ» (GuardWarning.Event): плитки считают по настоящим часам
// до начала перерыва, плашка закрывается сама через 5 с и крестиком при любых минутах.
const WarnEventRest = "rest"

// Виды счёта к перерыву (BreakRules.Counts): те же ключи, что core.BreakCounts.String и break_rules.counts.
const (
	BreakCountsAll   = "all"   // «Всё время, кроме учёбы»
	BreakCountsGames = "games" // «Только игры»
)

// Имена полей вкладки «Отдых» для ошибок SaveLimits (Reply.Field).
const (
	FieldBreakAfterMin    = "break.afterMin"
	FieldBreakCounts      = "break.counts"
	FieldBreakWarnMin     = "break.warnMin"
	FieldBreakRestMin     = "break.restMin"
	FieldBreakSelfRestMin = "break.selfRestMin"
	FieldBreakMessage     = "break.message"
)

// BreakRules — настройки «Отдых» ребёнка (FR-37). Message «» — стандартный текст экрана перерыва.
type BreakRules struct {
	Enabled     bool   `json:"enabled"`
	AfterMin    int    `json:"afterMin"`    // «Через сколько», 15–240
	Counts      string `json:"counts"`      // BreakCounts*
	WarnMin     int    `json:"warnMin"`     // «Предупредить за», 1–15
	RestMin     int    `json:"restMin"`     // «Отдых», 5–60
	SelfRestMin int    `json:"selfRestMin"` // «Засчитать как отдых, если не играл подряд», 5–120, не меньше RestMin
	Message     string `json:"message"`     // «Сообщение на экране перерыва», до 120 символов
}

// BreakRulesFrom переводит настройки ядра в вид канала.
func BreakRulesFrom(b core.BreakRules) BreakRules {
	return BreakRules{
		Enabled: b.Enabled, AfterMin: b.AfterMin, Counts: b.Counts.String(), WarnMin: b.WarnMin,
		RestMin: b.RestMin, SelfRestMin: b.SelfRestMin, Message: b.Message,
	}
}

// Core переводит настройки канала в вид ядра (пределы проверяет служба).
func (b BreakRules) Core() core.BreakRules {
	return core.BreakRules{
		Enabled: b.Enabled, AfterMin: b.AfterMin, Counts: core.BreakCountsFromString(b.Counts), WarnMin: b.WarnMin,
		RestMin: b.RestMin, SelfRestMin: b.SelfRestMin, Message: b.Message,
	}
}

// NewGuardRestWarning — плашка «ПОРА ОТДОХНУТЬ» (8.2.2 п.7): звук заказчика, плитки до начала
// перерыва (inS секунд), строка под плитками text. Threshold — минуты до перерыва для журнала
// стража; вид плашки страж выбирает по Event, а не по порогу (1 мин здесь не плашка 1 мин).
func NewGuardRestWarning(inS int, text string) GuardWarning {
	return GuardWarning{Threshold: inS / 60, Text: text, Sound: true, Event: WarnEventRest, EventS: inS}
}
