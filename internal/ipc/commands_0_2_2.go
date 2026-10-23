package ipc

// DTO фидбек-раунда 0.2.2. Контракт зеркалится в frontend/src/types.ts и проверяется
// contract_test.go + contract.test.ts.

// SetNoForbiddenRequest — снять или вернуть запретные часы на сегодня (R24, D-51). Только
// родитель. On=true — запретные интервалы сегодня не действуют; общий лимит остаётся.
type SetNoForbiddenRequest struct {
	ChildID int64 `json:"childId"`
	On      bool  `json:"on"`
}

// SetNoForbiddenReply — карточка «Сегодня» с новым состоянием и сообщение.
type SetNoForbiddenReply struct {
	Card    TodayCard `json:"card"`
	Message string    `json:"message"` // «Сегодня запретные часы не действуют» / «Запретные часы снова действуют»
}

// ClearBonusRequest — обнулить копилку ребёнка (FR-22, D-46). Только родитель.
type ClearBonusRequest struct {
	ChildID int64 `json:"childId"`
}

// ReduceBonusRequest — убавить копилку на Minutes минут, не ниже нуля (FR-22, D-46). Только
// родитель.
type ReduceBonusRequest struct {
	ChildID int64 `json:"childId"`
	Minutes int   `json:"minutes"`
}

// BonusChangeReply — новый баланс копилки и сообщение (для обнуления и убавления).
type BonusChangeReply struct {
	BalanceAfter int    `json:"balanceAfter"`
	Message      string `json:"message"`
}

// CreatePromoRequest — родитель создаёт промокод: код (буквы/цифры) и минуты 1..600 (R26, D-50).
type CreatePromoRequest struct {
	Code    string `json:"code"`
	Minutes int    `json:"minutes"`
}

// CreatePromoReply — созданный код и сообщение.
type CreatePromoReply struct {
	Promo   PromoView `json:"promo"`
	Message string    `json:"message"`
}

// PromoView — промокод в списке родителя (R26, D-50).
type PromoView struct {
	CodeDisplay string `json:"codeDisplay"`           // как ввёл родитель
	Minutes     int    `json:"minutes"`               //
	CreatedAtMs int64  `json:"createdAtMs"`           //
	UsedAtMs    int64  `json:"usedAtMs,omitempty"`    // 0 — действующий; иначе момент использования
	UsedByChild int64  `json:"usedByChild,omitempty"` // 0 — не использован
	UsedByName  string `json:"usedByName,omitempty"`  // имя ребёнка, кто использовал (для списка)
}

// ListPromosReply — действующие и использованные коды, новые сверху (R26, D-50).
type ListPromosReply struct {
	Active []PromoView `json:"active"`
	Used   []PromoView `json:"used"`
}

// RedeemPromoRequest — ребёнок вводит промокод (R26, D-50). childSelf (экран блокировки и
// раздел «Бонусы»). Код нормализуется службой.
type RedeemPromoRequest struct {
	ChildID int64  `json:"childId"`
	Code    string `json:"code"`
}

// RedeemPromoReply — результат: время добавлено (как доп. время, расширяет все три лимита) и
// карточка; неверный/использованный код — отказ с «Код не подходит» у поля.
type RedeemPromoReply struct {
	Card     TodayCard `json:"card"`
	AddedMin int       `json:"addedMin"`
	Message  string    `json:"message"` // «Добавлено 30 минут» (FR-35)
}

// OpenLogsReply — путь к папке логов и подтверждение открытия (FR-25, D-48).
type OpenLogsReply struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// BonusRateReply — настройка автобонуса N (R25, D-49): минут бонуса за каждые полные 10 минут
// неизрасходованного времени (по умолчанию 5).
type BonusRateReply struct {
	Minutes int `json:"minutes"`
}

// SetBonusRateRequest — задать N (0..600; 0 отключает автобонус). Только родитель.
type SetBonusRateRequest struct {
	Minutes int `json:"minutes"`
}
