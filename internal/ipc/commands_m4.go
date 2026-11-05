package ipc

// Контракт вехи M4 «Сайты и Brave» (R6, R14, R15, FR-16, FR-21, RM4-1…RM4-12). Зеркалится в
// frontend/src/types.ts и проверяется contract_test.go + contract.test.ts. Обработчики команд
// сайтов реализует служба (internal/service).

// WarnEventVideo — вид события предупреждения GuardWarning.Event: исчерпание лимита «Видео и
// соцсети» (R6). Страж считает плитки по остатку видео службы, как у игр.
const WarnEventVideo = "video"
