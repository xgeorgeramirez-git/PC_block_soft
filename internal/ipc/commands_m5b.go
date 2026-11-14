package ipc

// Контракт вехи M5b «Автозагрузка» (FR-39, ТЗ 8.3.11, RM5b-1…RM5b-5). Зеркалится в
// frontend/src/types.ts и проверяется contract_test.go + contract.test.ts.

// Команды M5b.
const (
	CmdListStartup = "ListStartup" // вкладка «Автозагрузка» ребёнка (родитель)
	CmdSetStartup  = "SetStartup"  // включить или выключить запись автозагрузки ребёнку (родитель)
)

// Решения по записи автозагрузки (startup_items.decision).
const (
	StartupPending = "pending" // новая запись: выключена и ждёт решения родителя (RM5b-4)
	StartupOn      = "on"      // запускается при входе ребёнка
	StartupOff     = "off"     // не запускается при входе ребёнка
)

// TestSessionLogon — событие TestSession «ребёнок снова вошёл» (только тестовый режим, S50):
// служба забывает момент входа (sessState.since = сейчас) и проходит путь «ребёнок появился в
// сеансе» (проверка автозагрузки, R1, политика), как после настоящего входа.
const TestSessionLogon = "logon"

// StartupItemView — строка списка «Автозагрузка»: Scope "own" | "shared"; Source — вид записи
// для значка строки (run, run32, runonce, startup_folder, task, store_app, RM5b-6); Name —
// понятное название; Publisher — издатель exe ("" — неизвестен); From — «откуда» словами;
// Decision — StartupPending | StartupOn | StartupOff.
type StartupItemView struct {
	ID        int64  `json:"id"`
	Scope     string `json:"scope"`
	Source    string `json:"source"`
	Name      string `json:"name"`
	Publisher string `json:"publisher"`
	From      string `json:"from"`
	Decision  string `json:"decision"`
}

// ListStartupRequest — список автозагрузки ребёнка; Refresh — запустить проверку Windows (ответ
// не ждёт её: строки — то, что уже известно, Checking = true).
type ListStartupRequest struct {
	ChildID int64 `json:"childId"`
	Refresh bool  `json:"refresh,omitempty"`
}

// ListStartupReply — блоки вкладки: «Ждут решения», «Учётка ребёнка», «Для всех пользователей».
// CheckedAt — момент последней удачной проверки (UTC мс; 0 — ещё не было); Checking — проверка
// идёт сейчас.
type ListStartupReply struct {
	Pending   []StartupItemView `json:"pending"`
	Own       []StartupItemView `json:"own"`
	Shared    []StartupItemView `json:"shared"`
	CheckedAt int64             `json:"checkedAt"`
	Checking  bool              `json:"checking"`
}

// SetStartupRequest — решение родителя по строке ID ребёнка ChildID: On — запускать при входе.
type SetStartupRequest struct {
	ChildID int64 `json:"childId"`
	ID      int64 `json:"id"`
	On      bool  `json:"on"`
}

// SetStartupReply — строка с решением службы и «Будет запускаться при входе Миши».
type SetStartupReply struct {
	Item    StartupItemView `json:"item"`
	Message string          `json:"message"`
}
