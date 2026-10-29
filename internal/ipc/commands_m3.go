package ipc

// Контракт вехи M3 «Программы и игры» (R5, R8, R12, FR-2, FR-13, FR-20, RM3-4). Зеркалится в
// frontend/src/types.ts и проверяется contract_test.go + contract.test.ts. Обработчики команд
// программ реализует служба (internal/service).

// Команды M3.
const (
	CmdListPrograms        = "ListPrograms"        // «Программы»: ждут решения и все программы (родитель)
	CmdDecideProgram       = "DecideProgram"       // решение по ждущей программе (родитель)
	CmdSetProgramsCategory = "SetProgramsCategory" // смена категории одной или нескольких программ (родитель)
	CmdUndoProgram         = "UndoProgram"         // «Отменить» решения 10 с (родитель)
	CmdScanPrograms        = "ScanPrograms"        // мастер до первого входа: программы компьютера (FR-2)
)

// Категории программ в канале (как programs.category). «system» родителю как выбор не
// показывается: такие программы наблюдатель не трогает.
const (
	ProgramCatGames   = "games"
	ProgramCatStudy   = "study"
	ProgramCatOther   = "other"
	ProgramCatBlocked = "blocked"
	ProgramCatSystem  = "system"
)

// Откуда категория программы (как programs.source).
const (
	ProgramSourceCatalog  = "catalog"
	ProgramSourceSnapshot = "snapshot"
	ProgramSourceParent   = "parent"
)

// ScreenPrograms — экран окна родителя для OpenWindowRequest.Screen: «Программы → Ждут решения»
// (кнопка «Попросить сейчас» на плашке, RM3-4).
const ScreenPrograms = "programs"

// MaxForegroundPathLen — наибольшая длина SensorSample.ForegroundPath в символах: длиннее служба
// отбрасывает (путь считается неопределённым).
const MaxForegroundPathLen = 520

// --- страж: плашки закрытых программ (FR-13) ---------------------------------------------

// GuardCmdBlocked — команда стражу показать плашку о закрытой программе (Plate).
const GuardCmdBlocked = "blocked"

// Виды плашек BlockedPlate.Kind.
const (
	BlockedUnknown   = "unknown"   // незнакомая программа ждёт разрешения родителя (R12)
	BlockedForbidden = "forbidden" // программа запрещена родителем
	BlockedGamesOver = "gamesOver" // время на игры закончилось (R5)
	BlockedStudyOnly = "studyOnly" // время вышло, доступна только учёба (R8)
	BlockedDecided   = "decided"   // родитель решил: «Родитель разрешил «X»: это игра. Запусти её снова»
)

// BlockedPlate — плашка о закрытой программе (FR-13): текст ребёнку, сколько показывать, путь
// программы и можно ли «Попросить сейчас» (RM3-4, только для BlockedUnknown).
type BlockedPlate struct {
	Kind   string `json:"kind"`   // Blocked*
	Text   string `json:"text"`   // «Программа «SuperGame» ждёт разрешения родителя»
	ShowS  int    `json:"showS"`  // секунд показа; 0 — пока ребёнок не закроет
	Path   string `json:"path"`   // путь exe; «» — не про одну программу
	CanAsk bool   `json:"canAsk"` // кнопка «Попросить сейчас»
}

// --- окно родителя: «Программы» (FR-20) ---------------------------------------------------

// ChildName — ребёнок в списках «Программ» (кто запускал).
type ChildName struct {
	ChildID int64  `json:"childId"`
	Name    string `json:"name"`
}

// PendingChildDTO — попытки одного ребёнка запустить ждущую программу.
type PendingChildDTO struct {
	ChildID  int64  `json:"childId"`
	Name     string `json:"name"`
	Attempts int    `json:"attempts"`
}

// PendingProgramDTO — строка «Ждут решения» (FR-20): «SuperGame — Загрузки — 3 попытки — вчера 18:40».
type PendingProgramDTO struct {
	Path          string            `json:"path"`
	Name          string            `json:"name"`
	Publisher     string            `json:"publisher"`   // «Без подписи издателя», если нет (RM3-3)
	Description   string            `json:"description"` // описание из файла
	Attempts      int               `json:"attempts"`
	LastAttemptAt int64             `json:"lastAttemptAt"` // UTC мс
	WasCategory   string            `json:"wasCategory"`   // «раньше: Игра» (RM3-12); «» — нет
	ByChild       []PendingChildDTO `json:"byChild"`
}

// ProgramDTO — строка списка всех программ (FR-20) с временем за неделю.
type ProgramDTO struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Publisher string `json:"publisher"`
	Category  string `json:"category"` // ProgramCat*
	Source    string `json:"source"`   // ProgramSource*
	WeekS     int    `json:"weekS"`    // время всех детей за 7 дней, с
}

// ProgramsReply — ответ ListPrograms и состояние «Программ» после решения.
type ProgramsReply struct {
	Pending  []PendingProgramDTO `json:"pending"`  // новые сверху
	Programs []ProgramDTO        `json:"programs"` // по имени
	Children []ChildName         `json:"children"` // дети для подписей и фильтров
}

// DecideProgramRequest — решение по ждущей программе: games|study|other|blocked.
type DecideProgramRequest struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}

// SetProgramsCategoryRequest — смена категории отмеченных программ (FR-20, массовое изменение).
type SetProgramsCategoryRequest struct {
	Paths    []string `json:"paths"`
	Category string   `json:"category"`
}

// DecideProgramReply — ответ DecideProgram и SetProgramsCategory: сообщение, «Отменить» 10 с и
// «Программы» после изменения.
type DecideProgramReply struct {
	Message     string        `json:"message"`     // «SuperGame: игра»
	UndoID      string        `json:"undoId"`      // для UndoProgram
	UndoUntilMs int64         `json:"undoUntilMs"` // до какого момента работает «Отменить», UTC мс
	Programs    ProgramsReply `json:"programs"`
}

// UndoProgramRequest — отменить последнее решение по программам.
type UndoProgramRequest struct {
	UndoID string `json:"undoId"`
}

// UndoProgramReply — «Программы» после отмены.
type UndoProgramReply struct {
	Message  string        `json:"message"` // «Решение отменено»
	Programs ProgramsReply `json:"programs"`
}

// --- мастер: программы компьютера (FR-2) ----------------------------------------------------

// ScanProgramDTO — программа компьютера для шага мастера «Программы» с предложенной категорией.
type ScanProgramDTO struct {
	Path              string `json:"path"`
	Name              string `json:"name"`
	Publisher         string `json:"publisher"`
	SuggestedCategory string `json:"suggestedCategory"` // ProgramCat*
}

// ScanProgramsReply — ответ ScanPrograms.
type ScanProgramsReply struct {
	Programs []ScanProgramDTO `json:"programs"`
}

// SetupProgram — категория программы, выбранная в мастере (SetupRequest.Programs).
type SetupProgram struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}
