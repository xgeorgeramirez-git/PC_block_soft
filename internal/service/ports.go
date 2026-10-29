package service

import (
	"errors"
	"time"

	"pcblock/internal/auth"
	"pcblock/internal/core"
)

// Порты службы (ТЗ 3.3, план M1 T9): всё, что связано с базой, Windows и паролем, служба
// видит только через эти интерфейсы. Настоящие реализации (storage.Store и pwhash, winsession
// и так далее) лежат в подпакете winhost; в тестах стоят поддельные.

// ErrStoreCorrupt — база не открылась из-за порчи файла (R18, NFR-8). Адаптер хранилища
// оборачивает в неё storage.ErrCorrupt; по ней OpenStore отличает порчу от занятого файла.
var ErrStoreCorrupt = errors.New("service: база данных повреждена")

// Child — детская учётка, отмеченная родителем (storage.Child без служебных полей).
type Child struct {
	ID           int64
	SID          string
	Name         string
	BypassDetail string // блокировка за обход (R17); пусто: блокировки нет
	EndMode      string // «Когда время вышло» (R8): "lock" или "study_only" (core.EndMode.String)
}

// Program — программа из списка (storage.Program): одна категория для всех детей (R12, FR-20).
// Category: games|study|other|blocked|system; Source: catalog|snapshot|parent.
type Program struct {
	ID          int64
	Path        string // нормализованный путь exe в нижнем регистре (RM3-1)
	Name        string
	Publisher   string // CompanyName из файла (RM3-3); пусто — не известен
	Category    string
	Source      string
	Fingerprint string // «размер:время изменения» (RM3-12); пусто — не нужен
	AppRoot     string // корень приложения (RM3-1 п.3); пусто — нет
	DecidedAt   int64  // момент решения родителя, UTC мс; 0 — нет
}

// PendingChild — попытки одного ребёнка запустить ждущую решения программу (storage.PendingChild).
type PendingChild struct {
	ChildID       int64
	Attempts      int
	LastAttemptAt int64 // UTC мс
}

// PendingProgram — программа в «Ждут решения» (storage.PendingProgram, R12, FR-20).
type PendingProgram struct {
	Path           string
	Name           string
	Publisher      string
	Description    string
	WasCategory    string // прежняя категория при смене отпечатка (RM3-12); пусто — нет
	FirstChildID   int64
	FirstAttemptAt int64 // UTC мс
	LastAttemptAt  int64 // UTC мс
	Attempts       int
	Children       []PendingChild // новые сверху; нет — [], не nil
}

// UsageRow — порция учёта usage_daily (storage.UsageRow, RM3-6). Day пусто — день состояния;
// Category: games|video_social|study|other («Системное» не пишется); Kind: program|site.
type UsageRow struct {
	Day      string
	Category string
	Kind     string
	Subject  string
	Seconds  int
}

// StoredEvent — запись журнала (R22), storage.Event с ChildID 0 вместо nil.
type StoredEvent struct {
	ID          int64
	ChildID     int64 // 0: событие не про ребёнка
	AtMs        int64 // UTC, миллисекунды
	TzOffsetMin int
	Kind        string
	Details     map[string]any
}

// EventQuery — отбор событий журнала.
type EventQuery struct {
	ChildID int64 // 0: все дети
	Limit   int
}

// BonusRecord — запись истории копилки (storage.BonusEntry) для службы (FR-22).
type BonusRecord struct {
	AtMs     int64
	DeltaMin int
	Reason   string
	Balance  int
}

// PromoRecord — промокод в списке родителя (storage.Promo) для службы (R26, D-50).
type PromoRecord struct {
	CodeDisplay string
	Minutes     int
	CreatedAtMs int64
	UsedAtMs    int64 // 0 — действующий; иначе момент использования (UTC мс)
	UsedByChild int64 // 0 — не использован
}

// Auth — пароль родителя и состояние неверных вводов (storage.Auth).
type Auth struct {
	Hash        string
	Attempts    auth.Attempts
	ChangedAtMs int64
}

// Store — подмножество методов storage.Store, нужное службе. Читающие методы «пусто — норма»
// (нет записи: нулевое значение и nil), как в storage.
type Store interface {
	UpsertChild(sid, name string) (int64, error)
	ListChildren() ([]Child, error)
	RemoveChild(id int64) error
	SetBypass(childID int64, detail string) error
	SetDayLimits(childID int64, week [7]core.DayLimit) error
	GetDayLimits(childID int64) ([7]core.DayLimit, error)
	SetEndMode(childID int64, mode string) error // "lock" | "study_only", иное — ошибка
	GetEndMode(childID int64) (string, error)    // нет ребёнка: "lock"
	GetSchedule(childID int64) ([]core.Interval, error)
	SetSchedule(childID int64, intervals []core.Interval) error
	AppendBonus(childID int64, deltaMin int, reason string) (balanceAfter int, err error)
	GetBonusBalance(childID int64) (int, error)
	ListBonus(childID int64) ([]BonusRecord, error)
	CreatePromo(codeDisplay string, minutes int) (PromoRecord, error)
	RedeemPromo(childID int64, code string, atMs int64) (minutes int, display string, ok bool, err error)
	ListPromos() ([]PromoRecord, error)
	LoadDayState(childID int64, day string) (core.DayState, error)
	SaveDayState(childID int64, st core.DayState) error
	// SaveDayAndUsage — состояние дня и порции учёта по программам одной транзакцией (RM3-6, G10).
	SaveDayAndUsage(childID int64, st core.DayState, usage []UsageRow) error
	AddUsage(childID int64, day, category, kind, subject string, seconds int) error
	ProgramSeconds(fromDay string) (map[string]int, error) // секунды по пути exe с fromDay, все дети

	// Программы и «Ждут решения» (M3: R12, FR-20, RM3-10).
	UpsertProgram(p Program) error
	SeedPrograms(ps []Program) (added int, err error) // только отсутствующие; решения родителя не трогает
	GetProgram(path string) (Program, bool, error)
	ListPrograms() ([]Program, error)
	// SetProgramsCategory — решение родителя: возвращает прежние значения изменённых (для «Отменить»).
	SetProgramsCategory(paths []string, category string, at int64) (changed []Program, err error)
	RestoreProgram(p Program) error
	DeleteProgram(path string) error
	AddPendingAttempt(p PendingProgram, childID, at int64) (attempts int, err error)
	ListPending() ([]PendingProgram, error)
	ResolvePending(path string) (PendingProgram, bool, error)
	RestorePending(p PendingProgram) error
	AppendEvent(e StoredEvent) error
	ListEvents(q EventQuery) ([]StoredEvent, error)
	GetAuth() (Auth, bool, error)
	SetAuth(a Auth) error
	GetSetting(key string) (string, bool, error)
	SetSetting(key, value string) error
	Close() error
}

// StoreOpener открывает базу и сохраняет копию испорченной. Open при порче файла
// возвращает ошибку, для которой errors.Is(err, ErrStoreCorrupt).
type StoreOpener interface {
	Open(path string) (Store, error)
	Quarantine(path string) (string, error)
}

// Hasher — хеш пароля родителя (обёртка над auth/pwhash): Hash проверяет длину пароля,
// Verify сравнивает за постоянное время; испорченный хеш — ошибка.
type Hasher interface {
	Hash(pw string) (string, error)
	Verify(hash, pw string) (bool, error)
}

// ProcKind — какой из экранных процессов запускается в сеансе.
type ProcKind int

// Виды процессов: страж от имени системы, значок и окна от имени пользователя сеанса.
const (
	ProcGuard ProcKind = iota
	ProcTray
	ProcWindow
)

// String возвращает имя вида для журнала работы.
func (k ProcKind) String() string {
	switch k {
	case ProcGuard:
		return "guard"
	case ProcTray:
		return "tray"
	case ProcWindow:
		return "window"
	}
	return "unknown"
}

// ScreenState — подключён ли сеанс к экрану (состояние WTS «активен»). Отключённый сеанс
// остаётся при смене пользователя: программы ребёнка в нём живут, но за компьютером его нет.
type ScreenState uint8

const (
	ScreenUnknown ScreenState = iota // состояние узнать не удалось: решает снимок стража
	ScreenOn                         // сеанс на экране
	ScreenOff                        // сеанс отключён (смена пользователя, экран выбора пользователя)
)

func (v ScreenState) String() string {
	switch v {
	case ScreenOn:
		return "на экране"
	case ScreenOff:
		return "не на экране"
	}
	return "неизвестно"
}

// SessionInfo — интерактивный сеанс Windows с пользователем.
type SessionInfo struct {
	ID       uint32
	User     string // «ДОМЕН\учётка»
	SID      string
	Screen   ScreenState // нулевое значение: неизвестно
	WTSState uint32      // состояние сеанса по WTS (0 активен, 4 отключён); только для журнала
}

// Proc — запущенный службой процесс (ручка процесса на Windows).
type Proc interface {
	PID() uint32
	Alive() bool
	Terminate() error
	Close()
}

// SessionLauncher — запуск и слежение за процессами в сеансах (на Windows: winsession).
type SessionLauncher interface {
	// Sessions перечисляет интерактивные сеансы с вошедшим пользователем, в том числе
	// отключённые (смена пользователя), с признаком, на экране ли сеанс (FB-3).
	Sessions() ([]SessionInfo, error)
	// Launch запускает программу (ключи в args) в сеансе: страж от имени системы, значок
	// и окна от имени пользователя сеанса.
	Launch(kind ProcKind, sessionID uint32, args []string) (Proc, error)
	// Logoff выводит пользователя из сеанса (WTSLogoffSession).
	Logoff(sessionID uint32) error
	// Shutdown штатно выключает компьютер (с запросом закрыть программы, без принуждения).
	Shutdown() error
	// SwitchUser отключает сеанс без завершения (WTSDisconnectSession): Windows показывает
	// экран выбора пользователя, программы и данные ребёнка остаются в сеансе.
	SwitchUser(sessionID uint32) error
	// Stamp возвращает момент создания процесса: пара (PID, Stamp) не повторяется,
	// в отличие от PID. ok = false: процесса нет.
	Stamp(pid uint32) (stamp uint64, ok bool)
	// KillStale завершает процесс, оставшийся от прошлого запуска службы, если он тот же
	// (совпали момент создания и путь программы). Возвращает true, если процесс завершён.
	KillStale(pid uint32, stamp uint64) bool
}

// AdminChecker сообщает, входит ли учётка в Администраторы, в том числе косвенно (R1).
type AdminChecker interface {
	IsAdmin(sid string) (bool, error)
}

// AccountInfo — локальная учётка Windows.
type AccountInfo struct {
	SID   string
	Name  string
	Admin bool
}

// AccountDirectory перечисляет локальные учётки для мастера и «Детских учёток».
type AccountDirectory interface {
	List() ([]AccountInfo, error)
}

// Policy ставит и снимает запрет Диспетчера задач в учётке ребёнка (FR-29, Ruling 20).
// Если пользователь не вошёл, адаптер подгружает раздел его профиля; профиля ещё нет:
// запись откладывается до первого входа и ошибкой не считается.
type Policy interface {
	SetTaskMgrDisabled(sid string, disabled bool) error
}

// Journal — запись событий журнала (R22). childID 0: событие не про ребёнка. Ошибки записи
// журнала службу не останавливают (R18), адаптер пишет их в журнал работы.
type Journal interface {
	Write(childID int64, kind string, details map[string]any)
}

// StoreJournal пишет события в хранилище: момент и смещение пояса берутся из часов ядра.
type StoreJournal struct {
	Store Store
	Clock core.Clock
	Loc   *time.Location
	Logf  func(format string, args ...any)
}

// Write дописывает событие в журнал.
func (j StoreJournal) Write(childID int64, kind string, details map[string]any) {
	now := j.Clock.Now()
	if j.Loc != nil {
		now = now.In(j.Loc)
	}
	_, off := now.Zone()
	err := j.Store.AppendEvent(StoredEvent{
		ChildID: childID, AtMs: now.UnixMilli(), TzOffsetMin: off / 60, Kind: kind, Details: details,
	})
	if err != nil && j.Logf != nil {
		j.Logf("журнал: не записано событие %s: %v", kind, err)
	}
}
