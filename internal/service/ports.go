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
	AdultFilter  bool   // список сайтов 18+ включён (R14); новому ребёнку — включён
}

// SiteRule — запись списков сайтов (storage.SiteRule без id, R14, FR-21, RM4-5): у домена одна
// запись. Kind: blocked | study | video_social (те же ключи, что sites.Kind.String).
type SiteRule struct {
	Domain string
	Kind   string
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
	Hidden      bool   // родитель скрыл программу (D-57, RF9-1); категория сохраняется
	CustomName  string // своё название родителя (D-59); пусто — название из файла (Name). Как storage.Program
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

// ProgramMerge — слияние повторов приложения магазина на ключ без версии (RM4-11, T6-11):
// storage.ProgramMerge. Хранилище пишет весь набор одной транзакцией: строка Into по ключу Key,
// строки From (кроме Key) удаляются; учёт usage_daily вида «программа» с предметом из From и
// PendingFrom суммируется на Key; строки «Ждут решения» PendingFrom удаляются, и, если Pending не
// nil, пишется он (с попытками детей).
type ProgramMerge struct {
	Key         string
	Into        *Program // nil — строк программ в группе нет
	From        []string
	Pending     *PendingProgram // nil — «Ждут решения» группы только удаляются
	PendingFrom []string
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

// EventQuery — отбор событий журнала (FR-24, RM5-5): фильтр уходит в базу (storage.EventFilter).
type EventQuery struct {
	ChildID              int64    // 0: все дети
	FromMs, ToMs         int64    // 0 — без границы; FromMs <= at < ToMs
	Kinds                []string // пусто — все виды
	BeforeAtMs, BeforeID int64    // курсор: строго раньше (at, id); BeforeAtMs 0 — с новых
	Limit                int
}

// DayRow — итоги одного дня ребёнка (storage.DayRow): ряд дней для столбиков и «Итогов» (RM5-4).
type DayRow struct {
	Day                            string
	TotalS, GamesS, VideoS, StudyS int
	ExtraMin                       int
}

// StatsReset — снимок последней очистки статистики ребёнка (storage.StatsReset, RM5-8).
type StatsReset struct {
	ClearedAtMs                    int64
	Day                            string // день ядра на момент очистки (RM5-8)
	TotalS, GamesS, VideoS, StudyS int
	ExtraMin                       int
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
	SetEndMode(childID int64, mode string) error           // "lock" | "study_only", иное — ошибка
	GetEndMode(childID int64) (string, error)              // нет ребёнка: "lock"
	GetBreakRules(childID int64) (core.BreakRules, error)  // «Отдых» (FR-37); нет строки — по умолчанию
	SetBreakRules(childID int64, b core.BreakRules) error  // вне пределов FR-37 — ошибка
	LoadBreakState(childID int64) (core.BreakState, error) // счёт к перерыву (R27); нет строки — пустой
	SaveBreakState(childID int64, b core.BreakState) error
	GetSchedule(childID int64) ([]core.Interval, error)
	SetSchedule(childID int64, intervals []core.Interval) error
	AppendBonus(childID int64, deltaMin int, reason string) (balanceAfter int, err error)
	GetBonusBalance(childID int64) (int, error)
	// GrantDayBonus — автобонус за день одной транзакцией (R25 «один раз»): начисление deltaMin в
	// копилку и строка дня day с bonus_granted = 1; сбой — ни того, ни другого.
	GrantDayBonus(childID int64, day core.DayState, deltaMin int, reason string) (balanceAfter int, err error)
	ListBonus(childID int64) ([]BonusRecord, error)
	CreatePromo(codeDisplay string, minutes int) (PromoRecord, error)
	RedeemPromo(childID int64, code string, atMs int64) (minutes int, display string, ok bool, err error)
	ListPromos() ([]PromoRecord, error) // действующие новые сверху, затем использованные по used_at (RM5-11)
	// CreatePromos — пачка кодов одной транзакцией (RM5-12): любой повтор или минуты вне 1..600 — ошибка, ничего не создано.
	CreatePromos(codesDisplay []string, minutes int) ([]PromoRecord, error)
	ClearUsedPromos() (int, error) // удаляет использованные коды, действующие остаются (RM5-14)
	LoadDayState(childID int64, day string) (core.DayState, error)
	SaveDayState(childID int64, st core.DayState) error
	// SaveDayAndUsage — состояние дня и порции учёта по программам и сайтам одной транзакцией (RM3-6, G10).
	SaveDayAndUsage(childID int64, st core.DayState, usage []UsageRow) error
	AddUsage(childID int64, day, category, kind, subject string, seconds int) error
	ProgramSeconds(fromDay string) (map[string]int, error)  // секунды по пути exe с fromDay, все дети
	DayUsage(childID int64, day string) ([]UsageRow, error) // учёт по программам и сайтам ребёнка за день (D-58, RM4-7); нет — []
	// DaysSeries — строки дней ребёнка с fromDay включительно до beforeDay не включительно, по
	// возрастанию дня; только дни со строкой; нет — [] (RM5-4).
	DaysSeries(childID int64, fromDay, beforeDay string) ([]DayRow, error)
	FirstStatsDay(childID int64) (string, bool, error)     // самый ранний день ребёнка; нет строк — false
	GetStatsReset(childID int64) (StatsReset, bool, error) // снимок последней очистки; не очищали — false
	// ClearStats — одной транзакцией (RM5-8): весь учёт по программам и сайтам ребёнка, дни до r.Day
	// с bonus_granted = 1 (ждущие автобонуса остаются, D-82; «Итоги» их отсекают по r.Day),
	// строка дня ядра day (day.Day == r.Day) со всеми флагами и снимок r; ошибка — ничего не изменилось.
	ClearStats(childID int64, day core.DayState, r StatsReset) error
	// UngrantedDays — дни ребёнка без автобонуса (bonus_granted = 0) строго до beforeDay, по
	// возрастанию, все поля как LoadDayState (FB-21, D-82); нет — [].
	UngrantedDays(childID int64, beforeDay string) ([]core.DayState, error)
	// PeriodUsage — учёт по программам и сайтам ребёнка за те же дни, сложенный по категории, виду и
	// предмету (Day пусто); скрытые тоже (их прячет служба); нет — [].
	PeriodUsage(childID int64, fromDay, beforeDay string) ([]UsageRow, error)

	// Программы и «Ждут решения» (M3: R12, FR-20, RM3-10).
	UpsertProgram(p Program) error
	SeedPrograms(ps []Program) (added int, err error) // только отсутствующие; решения родителя не трогает
	GetProgram(path string) (Program, bool, error)
	ListPrograms() ([]Program, error)
	// SetProgramsCategory — решение родителя: возвращает прежние значения изменённых (для «Отменить»).
	SetProgramsCategory(paths []string, category string, at int64) (changed []Program, err error)
	// SetProgramsHidden — скрыть или снова показать (D-57): меняется только флаг; прежние значения изменённых.
	SetProgramsHidden(paths []string, hidden bool) (changed []Program, err error)
	// SetProgramName — своё название (D-59): меняется только оно; прежнее значение; нет программы — ok = false.
	SetProgramName(path, name string) (prev Program, ok bool, err error)
	RestoreProgram(p Program) error
	DeleteProgram(path string) error
	AddPendingAttempt(p PendingProgram, childID, at int64) (attempts int, err error)
	ListPending() ([]PendingProgram, error)
	ResolvePending(path string) (PendingProgram, bool, error)
	RestorePending(p PendingProgram) error
	// MergePrograms — слияние повторов приложений магазина (RM4-11) одной транзакцией: ошибка —
	// ничего не изменилось.
	MergePrograms(ms []ProgramMerge) error

	// Сайты (M4: R14, FR-21, RM4-5). Домен нормализует служба (sites.NormalizeInput); хранилище
	// отклоняет пустой, длиннее 253 и чужой список. Предел 1000 запрещённых проверяет служба.
	ListSiteRules() ([]SiteRule, error) // по списку и домену; нет — []
	// AddSiteRule — новая запись: (false, "", nil); уже в этом списке: (false, kind, nil); перенос из
	// другого списка: (true, прежний, nil).
	AddSiteRule(domain, kind string) (moved bool, prevKind string, err error)
	RemoveSiteRule(domain string) (prevKind string, ok bool, err error) // нет домена — ok = false
	CountSiteRules(kind string) (int, error)
	// ReplaceSiteRules — список kind целиком (мастер, S-4) одной транзакцией; домены из других
	// списков переносятся; ошибка — ничего не изменилось.
	ReplaceSiteRules(kind string, domains []string) error
	SetAdultFilter(childID int64, on bool) error // «Блокировать сайты 18+» (R14); нет ребёнка — ошибка
	AppendEvent(e StoredEvent) error
	ListEvents(q EventQuery) ([]StoredEvent, error)
	DeleteChildEvents(childID int64) (int, error) // события ребёнка; без ребёнка не трогает (RM5-9)
	GetAuth() (Auth, bool, error)
	SetAuth(a Auth) error
	GetSetting(key string) (string, bool, error)
	SetSetting(key, value string) error
	Close() error
}

// FoundProgram — программа, найденная сканером в Program Files (снимок RM3-10, мастер FR-2).
// Name — описание из файла или имя файла; Publisher — CompanyName (RM3-3), пусто — не известен.
type FoundProgram struct {
	Path      string
	Name      string
	Publisher string
}

// Scanner — снимок программ компьютера: главные exe корней приложений Program Files и известные
// игры по каталогу (Windows-реализация в winhost). Ошибка — снимок не сделан: служба не считает
// компьютер пустым и ничего не закрывает (RM3-10, R18).
type Scanner interface {
	Scan() ([]FoundProgram, error)
}

// FileMeta читает свойства файла exe: издатель (CompanyName), описание (FileDescription) и
// отпечаток «размер:время изменения unix» (RM3-12). Не прочиталось — пустые строки.
type FileMeta func(path string) (publisher, description, fingerprint string)

// ProcInfo — процесс сеанса для наблюдателя (M3: R12, R13, NFR-6, RM3-11). Пара (PID, Stamp)
// не повторяется, в отличие от PID: Stamp — момент создания процесса. Path — полный путь exe
// (адаптер разворачивает 8.3-имена и junction); PathOK = false — путь не прочитан (защищённый
// процесс, отказ в доступе): такой процесс наблюдатель не трогает (PL-9). UserSID — SID
// пользователя, от имени которого идёт процесс (маркер доступа); UserOK = false — не прочитан:
// процесс не трогается (программа администратора через UAC в сеансе ребёнка, R19). ParentPID —
// номер родительского процесса из снимка Toolhelp32 (0 — нет); номер мог достаться другому
// процессу, поэтому родитель узнаётся ещё и по моменту создания (launchParentOf, R13, D-65).
type ProcInfo struct {
	PID       uint32
	ParentPID uint32
	Stamp     uint64
	SessionID uint32
	Path      string
	PathOK    bool
	UserSID   string
	UserOK    bool
}

// ProcessWatcher — процессы сеансов Windows для наблюдателя (Windows-реализация в winhost: Toolhelp32,
// ProcessIdToSessionId, QueryFullProcessImageName, TerminateProcess). Служба зовёт его вне s.mu.
type ProcessWatcher interface {
	// List возвращает процессы сеанса sessionID; нечитаемый путь — PathOK=false.
	List(sessionID uint32) ([]ProcInfo, error)
	// Kill завершает процесс, если PID и Stamp всё ещё совпадают (ручка открыта при проверке); true — завершён.
	Kill(p ProcInfo) bool
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

// Policy ставит и снимает запрет Диспетчера задач (FR-29, Ruling 20) и правила Brave (R15, FR-31,
// RM4-10) в учётке ребёнка. Если пользователь не вошёл, адаптер подгружает раздел его профиля;
// профиля ещё нет: запись откладывается до первого входа и ошибкой не считается.
type Policy interface {
	SetTaskMgrDisabled(sid string, disabled bool) error
	// SetBraveRules приводит ветку HKU\<sid>\Software\Policies\BraveSoftware\Brave к r, не стирая
	// её (BravePlan): ветка не загружена — через профиль; профиля нет — nil.
	SetBraveRules(sid string, r BraveRules) error
	// BraveRulesOK — ветка совпадает с r. Ветка пользователя не загружена (не вошёл) — (true, nil):
	// профиль ради сверки не подгружается (T5-4).
	BraveRulesOK(sid string, r BraveRules) (bool, error)
	// RemoveBraveRules снимает ветку правил Brave целиком; её нет — nil.
	RemoveBraveRules(sid string) error
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
	//eventkind:dynamic параметр Journal.Write: вид проверяют места вызова (TestEventKindsSourceScan)
	err := j.Store.AppendEvent(StoredEvent{
		ChildID: childID, AtMs: now.UnixMilli(), TzOffsetMin: off / 60, Kind: kind, Details: details,
	})
	if err != nil && j.Logf != nil {
		j.Logf("журнал: не записано событие %s: %v", kind, err)
	}
}
