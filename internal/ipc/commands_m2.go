package ipc

// DTO вехи M2 (расписание, пауза, доп. время, копилка). Контракт зеркалится в
// frontend/src/types.ts и проверяется contract_test.go + contract.test.ts.

// Interval — запретный интервал расписания (R7): день недели 1=пн..7=вс, минуты от полуночи.
type Interval struct {
	Weekday  int `json:"weekday"`
	StartMin int `json:"startMin"`
	EndMin   int `json:"endMin"`
}

// GetScheduleRequest / GetScheduleReply — запретные интервалы ребёнка (R7, FR-17).
type GetScheduleRequest struct {
	ChildID int64 `json:"childId"`
}

// GetScheduleReply — интервалы ребёнка, нормализованные (R7).
type GetScheduleReply struct {
	ChildID   int64      `json:"childId"`
	Intervals []Interval `json:"intervals"`
}

// SaveScheduleRequest — сохранить интервалы ребёнка (R7). Служба нормализует (объединяет
// пересекающиеся, до 6 на день) и возвращает итог (родитель видит результат).
type SaveScheduleRequest struct {
	ChildID   int64      `json:"childId"`
	Intervals []Interval `json:"intervals"`
}

// SaveScheduleReply — нормализованные интервалы и карточка (правила уже действуют, FR-19).
type SaveScheduleReply struct {
	Intervals []Interval `json:"intervals"`
	Card      TodayCard  `json:"card"`
	Message   string     `json:"message"`
}

// PausePreset — срок паузы (R11): 15 мин, 1 ч, 2 ч, до конца дня.
type PausePreset string

// Сроки паузы.
const (
	PauseMin15    PausePreset = "min15"
	PauseHour1    PausePreset = "hour1"
	PauseHour2    PausePreset = "hour2"
	PauseEndOfDay PausePreset = "endOfDay"
)

// PauseRequest — пауза одному ребёнку (R11).
type PauseRequest struct {
	ChildID int64       `json:"childId"`
	Preset  PausePreset `json:"preset"`
}

// PauseReply — карточка с паузой и сообщение «Пауза до 18:30».
type PauseReply struct {
	Card    TodayCard `json:"card"`
	Message string    `json:"message"`
}

// PauseAllRequest — пауза всем детям (FR-25).
type PauseAllRequest struct {
	Preset PausePreset `json:"preset"`
}

// PauseAllReply — карточки всех детей и сообщение.
type PauseAllReply struct {
	Children []TodayCard `json:"children"`
	Message  string      `json:"message"`
}

// GrantBonusRequest — начислить в копилку (FR-22): минуты 1..600 и причина.
type GrantBonusRequest struct {
	ChildID int64  `json:"childId"`
	Minutes int    `json:"minutes"`
	Reason  string `json:"reason"`
}

// GrantBonusReply — новый баланс, сообщение и «Отменить» на 10 с.
type GrantBonusReply struct {
	BalanceAfter int    `json:"balanceAfter"`
	Message      string `json:"message"`     // «Начислено 30 минут. Баланс 75 мин»
	BonusID      string `json:"bonusId"`     // для UndoBonus
	UndoUntilMs  int64  `json:"undoUntilMs"` // до какого момента работает «Отменить», UTC мс
}

// UndoBonusRequest — отменить только что начисленное.
type UndoBonusRequest struct {
	ChildID int64  `json:"childId"`
	BonusID string `json:"bonusId"`
}

// UndoBonusReply — новый баланс после отмены.
type UndoBonusReply struct {
	BalanceAfter int    `json:"balanceAfter"`
	Message      string `json:"message"`
}

// BonusEntry — запись истории копилки (FR-22).
type BonusEntry struct {
	AtMs     int64  `json:"atMs"`
	TzOffset int    `json:"tzOffsetMin"`
	DeltaMin int    `json:"deltaMin"` // + начисление, - трата
	Reason   string `json:"reason"`
	Balance  int    `json:"balanceAfter"`
}

// GetBonusRequest / GetBonusReply — баланс и история копилки (FR-22).
type GetBonusRequest struct {
	ChildID int64 `json:"childId"`
}

// GetBonusReply — баланс и история, новые записи первыми.
type GetBonusReply struct {
	ChildID int64        `json:"childId"`
	Balance int          `json:"balance"`
	Entries []BonusEntry `json:"entries"`
}

// CopyRulesRequest — скопировать правила с другого ребёнка (FR-33).
type CopyRulesRequest struct {
	FromChildID int64 `json:"fromChildId"`
	ToChildID   int64 `json:"toChildId"`
}

// CopyRulesReply — правила источника для вставки в черновик приёмника (FR-19): фронт
// кладёт их в несохранённые изменения, пишет сам родитель.
type CopyRulesReply struct {
	Limits    []WeekdayLimit `json:"limits"`
	Intervals []Interval     `json:"intervals"`
	Message   string         `json:"message"`
}

// AskMoreRequest — «Попросить ещё» ребёнком (FR-10): пароль родителя и минуты (1..240).
type AskMoreRequest struct {
	ChildID  int64  `json:"childId"`
	Minutes  int    `json:"minutes"`
	Password string `json:"password"`
}

// AskMoreReply — результат: доп. время добавлено (как AddExtra) или ошибка пароля у поля.
type AskMoreReply struct {
	Card    TodayCard `json:"card"`
	Message string    `json:"message"` // «Добавлено 30 минут»
}

// SpendBonusRequest — «Взять из копилки» ребёнком без пароля (FR-11).
type SpendBonusRequest struct {
	ChildID int64 `json:"childId"`
	Minutes int   `json:"minutes"`
}

// SpendBonusReply — результат: время добавлено и новый баланс, либо отказ (интервал/не
// хватает) с понятным текстом.
type SpendBonusReply struct {
	Card         TodayCard `json:"card"`
	Message      string    `json:"message"`      // «Взято из копилки 30 мин»
	BalanceAfter int       `json:"balanceAfter"` // остаток копилки
}
