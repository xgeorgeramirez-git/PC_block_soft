package ipc

// DTO вехи M5: промокоды «Придумать коды» и «Очистить историю» (FR-35, D-75a, RM5-12, RM5-14).
// Контракт зеркалится в frontend/src/types.ts и проверяется contract_test.go + contract.test.ts.

const (
	CmdGeneratePromos    = "GeneratePromos"    // придумать пачку промокодов (FR-35, RM5-12)
	CmdClearPromoHistory = "ClearPromoHistory" // удалить использованные промокоды (FR-35, RM5-14)
)

// Поля «Придумать коды», к которым служба привязывает ошибку.
const (
	FieldCount  = "count"  // сколько кодов (1–50)
	FieldLength = "length" // длина кода (4–12 знаков)
)

// Состав знаков «Придумать коды».
const (
	CharsetDigits  = "digits"  // 0–9
	CharsetLetters = "letters" // A–Z
	CharsetMixed   = "mixed"   // A–Z и 0–9
)

// GeneratePromosRequest — «Придумать коды» (FR-35, RM5-12): Count кодов 1–50 длиной Length 4–12
// из состава Charset (digits, letters, mixed) по Minutes минут 1–600. Только родитель.
type GeneratePromosRequest struct {
	Count   int    `json:"count"`
	Length  int    `json:"length"`
	Charset string `json:"charset"`
	Minutes int    `json:"minutes"`
}

// GeneratePromosReply — новые коды (заглавными) и сообщение «Готово: 20 кодов на 15 мин».
type GeneratePromosReply struct {
	Promos  []PromoView `json:"promos"`
	Message string      `json:"message"`
}

// ClearPromoHistoryReply — «Очистить историю» промокодов: «История промокодов очищена».
type ClearPromoHistoryReply struct {
	Message string `json:"message"`
}
