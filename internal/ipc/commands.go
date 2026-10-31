package ipc

// Имена команд M1. Права на каждую команду проверяет служба по Peer и родительской сессии
// (таблица в internal/service); канал только доставляет вызов.
const (
	// Общие.
	CmdHello          = "Hello"          // версия, тестовый режим, настроена ли программа
	CmdGetChildStatus = "GetChildStatus" // статус ребёнка своего сеанса для значка и мини-панели
	CmdOpenWindow     = "OpenWindow"     // попросить службу открыть окно (Ruling 19)
	CmdWindowClosing  = "WindowClosing"  // окно закрыто штатно

	// Мастер и администратор.
	CmdListAccounts   = "ListAccounts"   // локальные учётки
	CmdSetup          = "Setup"          // первоначальная настройка
	CmdResetPassword  = "ResetPassword"  // сброс пароля родителя (R20)
	CmdUninstallCheck = "UninstallCheck" // можно ли удалять программу

	// Родительская сессия (R19).
	CmdLogin          = "Login"
	CmdExtendSession  = "ExtendSession"
	CmdLogout         = "Logout"
	CmdSessionState   = "SessionState"
	CmdGetToday       = "GetToday"
	CmdGetChildRules  = "GetChildRules"
	CmdSaveLimits     = "SaveLimits"
	CmdAddExtra       = "AddExtra"
	CmdReduceExtra    = "ReduceExtra" // убавить доп. время (R10, D-43)
	CmdClearExtra     = "ClearExtra"  // обнулить доп. время (R10, D-43)
	CmdUndoExtra      = "UndoExtra"
	CmdClearBlock     = "ClearBlock"
	CmdListEvents     = "ListEvents"
	CmdChangePassword = "ChangePassword"
	CmdSetChildren    = "SetChildren"
	CmdParentStats    = "ParentStats" // статистика выбранного ребёнка за сегодня для родителя (FR-23, D-44)

	// Диагностика: подробные логи (FR-25, D-42).
	CmdGetDiag  = "GetDiag"  // прочитать, включены ли подробные логи
	CmdSetDiag  = "SetDiag"  // включить/выключить подробные логи
	CmdOpenLogs = "OpenLogs" // открыть папку с логами в проводнике (FR-25, D-48)

	// Фидбек-раунд 0.2.2.
	CmdSetNoForbidden = "SetNoForbidden" // снять/вернуть запретные часы на сегодня (R24, D-51)
	CmdClearBonus     = "ClearBonus"     // обнулить копилку (FR-22, D-46)
	CmdReduceBonus    = "ReduceBonus"    // убавить копилку на N минут (FR-22, D-46)
	CmdCreatePromo    = "CreatePromo"    // родитель создаёт промокод на время (R26, D-50)
	CmdListPromos     = "ListPromos"     // списки действующих и использованных кодов (R26, D-50)
	CmdRedeemPromo    = "RedeemPromo"    // ребёнок вводит промокод (R26, D-50)
	CmdGetBonusRate   = "GetBonusRate"   // прочитать настройку автобонуса N (R25, D-49)
	CmdSetBonusRate   = "SetBonusRate"   // задать настройку автобонуса N (R25, D-49)

	// Расписание, копилка, пауза (M2).
	CmdGetSchedule  = "GetSchedule"  // запретные интервалы ребёнка (R7)
	CmdSaveSchedule = "SaveSchedule" // сохранить интервалы, вернуть нормализованные (R7)
	CmdPause        = "Pause"        // пауза одному ребёнку (R11)
	CmdPauseAll     = "PauseAll"     // пауза всем детям (FR-25)
	CmdGrantBonus   = "GrantBonus"   // начислить в копилку (FR-22)
	CmdUndoBonus    = "UndoBonus"    // отменить начисление на 10 с
	CmdGetBonus     = "GetBonus"     // баланс и история копилки
	CmdCopyRules    = "CopyRules"    // скопировать правила с другого ребёнка (FR-33)

	// Ребёнок о себе (только свой сеанс, только чтение).
	CmdMyToday      = "MyToday"
	CmdFirstRunSeen = "FirstRunSeen"

	// Ребёнок просит ещё / берёт из копилки (M2).
	CmdAskMore    = "AskMore"    // «Попросить ещё» ребёнком с паролем родителя (FR-10)
	CmdSpendBonus = "SpendBonus" // «Взять из копилки» ребёнком без пароля (FR-11)

	// Страж (только процесс системы в том же сеансе).
	CmdGuardPoll       = "GuardPoll"
	CmdGuardLogoff     = "GuardLogoff"
	CmdGuardShutdown   = "GuardShutdown"   // «Выключить компьютер» после подтверждения
	CmdGuardSwitchUser = "GuardSwitchUser" // «Смена пользователя» и автосмена: сеанс отключается, не завершаясь
	CmdGuardClosing    = "GuardClosing"    // страж закрывается вместе с сеансом (выход, выключение): это не обход (R17, FR-27)

	// Значок (только процесс пользователя своего сеанса).
	CmdTrayClosing = "TrayClosing"

	// Только тестовый режим (ТЗ 10.8).
	CmdTestGuard   = "TestGuard"
	CmdTestClock   = "TestClock"
	CmdTestSession = "TestSession"
	CmdGuardState  = "GuardState"
)

// AllCommands — все команды M1 в порядке групп. По этому списку служба проверяет, что у
// каждой команды есть строка в таблице прав, а contract_test.go — что у каждой есть
// пример в testdata/contract.json: новая команда не пройдёт без того и другого.
var AllCommands = []string{
	CmdHello, CmdGetChildStatus, CmdOpenWindow, CmdWindowClosing,
	CmdListAccounts, CmdSetup, CmdResetPassword, CmdUninstallCheck,
	CmdLogin, CmdExtendSession, CmdLogout, CmdSessionState, CmdGetToday, CmdGetChildRules,
	CmdSaveLimits, CmdAddExtra, CmdReduceExtra, CmdClearExtra, CmdUndoExtra, CmdClearBlock, CmdListEvents, CmdChangePassword,
	CmdSetChildren, CmdParentStats, CmdGetDiag, CmdSetDiag, CmdOpenLogs,
	CmdSetNoForbidden, CmdClearBonus, CmdReduceBonus, CmdCreatePromo, CmdListPromos, CmdGetBonusRate, CmdSetBonusRate,
	CmdGetSchedule, CmdSaveSchedule, CmdPause, CmdPauseAll,
	CmdGrantBonus, CmdUndoBonus, CmdGetBonus, CmdCopyRules,
	CmdMyToday, CmdFirstRunSeen, CmdAskMore, CmdSpendBonus, CmdRedeemPromo,
	CmdGuardPoll, CmdGuardLogoff, CmdGuardShutdown, CmdGuardSwitchUser, CmdGuardClosing,
	CmdTrayClosing,
	CmdTestGuard, CmdTestClock, CmdTestSession, CmdGuardState,
	CmdListPrograms, CmdDecideProgram, CmdSetProgramsCategory, CmdUndoProgram, CmdScanPrograms, // M3 (commands_m3.go)
	CmdSetProgramsHidden, // фидбек-раунд 0.3.1 (D-57)
}

// Имена полей, к которым служба привязывает ошибку (Reply.Field, L-121).
const (
	FieldPassword    = "password"
	FieldOldPassword = "oldPassword"
	FieldNewPassword = "newPassword"
	FieldChildren    = "children"
	FieldMinutes     = "minutes"
	FieldState       = "state"
	FieldReason      = "reason"  // причина начисления копилки (FR-22)
	FieldStart       = "start"   // начало запретного интервала (R7)
	FieldEnd         = "end"     // конец запретного интервала (R7)
	FieldCode        = "code"    // промокод (R26, D-50)
	FieldEndMode     = "endMode" // режим «Когда время вышло» (R8, FR-18)
)

// LimitField — имя поля лимита дня недели (1 = пн … 7 = вс) для ошибок SaveLimits и Setup.
func LimitField(weekday int) string { return "limits." + itoa(weekday) }

// GamesField — имя поля лимита игр дня недели (1 = пн … 7 = вс) для ошибок SaveLimits и Setup (R5, FR-16).
func GamesField(weekday int) string { return "games." + itoa(weekday) }

// Режимы «Когда время вышло» (R8, D-9) в ChildRules.EndMode, SaveLimitsRequest.EndMode и
// ChildStatus.EndMode: те же ключи, что core.EndMode.String и children.end_mode.
const (
	EndModeLock      = "lock"       // экран блокировки
	EndModeStudyOnly = "study_only" // оставить только учёбу
)

// Empty — нагрузка команд без данных; уходит как «{}».
type Empty struct{}

// ProtocolVersion — версия протокола, её возвращает Hello.
const ProtocolVersion = 1

// --- Общие -----------------------------------------------------------------

// HelloReply — ответ Hello.
type HelloReply struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`  // версия программы, например «0.1.0»
	TestMode bool   `json:"testMode"` // включён тестовый режим (ТЗ 10.8)
	// AutoSwitchS — срок автоматической смены пользователя в секундах; только в тестовом
	// режиме и только если задан в реестре (короче 15 минут). 0: обычные 15 минут.
	AutoSwitchS int  `json:"autoSwitchS,omitempty"`
	Configured  bool `json:"configured"` // пароль родителя задан, мастер пройден
}

// Виды окон для OpenWindow и WindowClosing (Ruling 19).
const (
	WindowParent   = "parent"
	WindowMyStats  = "mystats"
	WindowFirstRun = "firstrun"
)

// OpenWindowRequest — запрос окна. Из родительской сессии (страж после верного пароля)
// токен сессии идёт в Msg.Token: служба передаёт сессию окну.
type OpenWindowRequest struct {
	Kind string `json:"kind"` // WindowParent, WindowMyStats или WindowFirstRun
	// Screen — экран окна родителя, на котором оно откроется (RM3-4): ScreenPrograms; «» — главный.
	// Другое значение — отказ TextBadRequest.
	Screen string `json:"screen,omitempty"`
}

// OpenWindowReply — ответ OpenWindow.
type OpenWindowReply struct {
	Opened bool `json:"opened"` // окно запущено или уже открыто
}

// WindowClosingRequest — окно закрыто штатно (код 0, FR-27): служба не считает это обходом.
type WindowClosingRequest struct {
	Kind string `json:"kind"`
}

// --- Статус ребёнка: один источник для значка, мини-панели, «Моей статистики», карточки и стража ---

// Значения ChildStatus.Status. Первые шесть повторяют core.Status, admin и notChild
// добавляет служба: учётка стала администратором (R1) и «в этом сеансе не ребёнок».
const (
	StatusOK        = "ok"
	StatusLow       = "low"
	StatusTimeUp    = "timeUp"
	StatusForbidden = "forbidden" // запретные часы (R7)
	StatusBypass    = "bypass"
	StatusUnlimited = "unlimited"
	StatusPaused    = "paused" // пауза: ограничения не действуют, учёт идёт (R11)
	StatusSafe      = "safe"
	StatusAdmin     = "admin"
	StatusNotChild  = "notChild"
)

// Значения ChildStatus.Lock: какой экран блокировки показать.
const (
	LockNone      = "none"
	LockTimeUp    = "timeUp"
	LockForbidden = "forbidden" // запретные часы (R7)
	LockBypass    = "bypass"
)

// ChildStatus — состояние ребёнка. Числа и тексты экрана блокировки берутся из
// core.Decision без пересчёта (ChildStatusFrom); надпись, подсказка и слово статуса
// собираются в одном месте, поэтому на всех экранах они одинаковы (Review Focus 5).
type ChildStatus struct {
	Name          string `json:"name"`          // имя ребёнка
	Status        string `json:"status"`        // Status*; цвет определяет только он (ТЗ 8.0 п.3)
	StatusText    string `json:"statusText"`    // статус словом для карточки родителя: «Можно», «Время вышло»
	Label         string `json:"label"`         // надпись на значке: минуты остатка «85», «0», «∞»
	Tooltip       string `json:"tooltip"`       // подсказка значка: «Осталось 1 ч 25 мин»
	Summary       string `json:"summary"`       // строка мини-панели: «Сегодня осталось 1 ч 25 мин из 2 ч»
	Day           string `json:"day"`           // день цифр, YYYY-MM-DD
	RemainingS    int    `json:"remainingS"`    // остаток общего времени, с (0 без лимита)
	LimitS        int    `json:"limitS"`        // лимит дня с доп. временем, с (0 без лимита)
	UsedS         int    `json:"usedS"`         // общее время за день, с
	Unlimited     bool   `json:"unlimited"`     // лимита нет или правила не прочитаны
	Lock          string `json:"lock"`          // Lock*
	Reason        string `json:"reason"`        // заголовок экрана блокировки
	Detail        string `json:"detail"`        // что произошло (только при обходе)
	Until         string `json:"until"`         // «Можно снова: завтра»
	PauseUntil    string `json:"pauseUntil"`    // «18:30» — конец активной паузы (R11); «» — нет паузы
	Used          string `json:"used"`          // «Сегодня: 2 ч 00 мин из 2 ч»
	BonusBalance  int    `json:"bonusBalance"`  // баланс копилки, мин (M2, для мини-панели ребёнка)
	ForbiddenNote string `json:"forbiddenNote"` // «Сегодня нельзя с 21:00 до 07:00»; «» — нет ближайшего интервала

	// Игры и режим окончания (M3: R5, R8) — из того же core.Decision.
	GamesRemainingS int    `json:"gamesRemainingS"` // остаток игр, с; 0 без лимита игр
	GamesLimitS     int    `json:"gamesLimitS"`     // лимит игр дня с доп. временем, с; 0 без лимита игр
	GamesUnlimited  bool   `json:"gamesUnlimited"`  // отдельного лимита игр нет (или правила не прочитаны)
	EndMode         string `json:"endMode"`         // EndModeLock или EndModeStudyOnly (правило ребёнка, R8)
	CloseGames      bool   `json:"closeGames"`      // игры закрыты: лимит игр исчерпан или экран блокировки
	StudyOnly       bool   `json:"studyOnly"`       // действует «только учёба»: экрана нет, открыта только учёба
}

// --- Мастер и администратор --------------------------------------------------

// Account — локальная учётка Windows для мастера и «Детских учёток».
type Account struct {
	SID        string `json:"sid"`
	Name       string `json:"name"`
	Admin      bool   `json:"admin"`      // входит в Администраторы
	Child      bool   `json:"child"`      // уже отмечена детской
	Current    bool   `json:"current"`    // учётка того, кто спрашивает
	CanBeChild bool   `json:"canBeChild"` // можно отметить детской (D-38: администратора нельзя)
}

// ListAccountsReply — ответ ListAccounts.
type ListAccountsReply struct {
	Accounts []Account `json:"accounts"`
}

// WeekdayLimit — лимит одного дня недели. Weekday 1 = пн … 7 = вс; 0 минут — в этот день
// пользоваться нельзя (R4); Unlimited — без лимита (тогда TotalMin = 0 и не читается).
// GamesMin — лимит игр (R5, FR-16): 0 — игры в этот день нельзя; GamesUnlimited — игры без
// отдельного лимита (ограничены только общим, тогда GamesMin = 0 и не читается). Лимит игр может
// быть больше общего: тогда раньше кончится общий.
type WeekdayLimit struct {
	Weekday        int  `json:"weekday"`
	TotalMin       int  `json:"totalMin"`
	Unlimited      bool `json:"unlimited"`
	GamesMin       int  `json:"gamesMin"`
	GamesUnlimited bool `json:"gamesUnlimited"`
}

// SetupChild — ребёнок при первоначальной настройке.
type SetupChild struct {
	SID    string         `json:"sid"`
	Name   string         `json:"name"`
	Limits []WeekdayLimit `json:"limits"` // семь дней
}

// SetupRequest — первоначальная настройка (только повышенный администратор и только пока
// программа не настроена). Ошибки по полям: FieldPassword, FieldChildren, LimitField(n).
type SetupRequest struct {
	Password string       `json:"password"`
	Children []SetupChild `json:"children"`
	// Programs — категории программ компьютера, выбранные на шаге «Программы» (FR-2, ScanPrograms);
	// пусто — снимок службы с категориями по умолчанию (RM3-10).
	Programs []SetupProgram `json:"programs,omitempty"`
}

// ResetPasswordRequest — сброс пароля (только повышенный администратор, R20).
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

// UninstallCheckRequest — можно ли удалять программу: верный пароль родителя или «забыл
// пароль» от повышенного администратора; пока программа не настроена, пароль не нужен.
type UninstallCheckRequest struct {
	Password string `json:"password"`
	Forgot   bool   `json:"forgot"`
}

// UninstallCheckReply — ответ UninstallCheck. Неверный пароль приходит отказом с LoginDenied.
type UninstallCheckReply struct {
	Allowed bool `json:"allowed"`
}

// --- Родительская сессия ------------------------------------------------------

// LoginRequest — вход родителя.
type LoginRequest struct {
	Password string `json:"password"`
}

// LoginReply — родительская сессия открыта; токен идёт в Msg.Token следующих команд.
type LoginReply struct {
	Token        string `json:"token"`
	ExpiresMs    int64  `json:"expiresMs"`    // когда закончится при бездействии, UTC мс
	IdleTimeoutS int    `json:"idleTimeoutS"` // длина окна бездействия (R19: 600)
}

// LoginDenied — нагрузка отказа Login и UninstallCheck (Reply.OK = false): сколько попыток
// осталось и сколько секунд ввод закрыт после серии ошибок (R19).
type LoginDenied struct {
	AttemptsLeft int `json:"attemptsLeft"`
	PauseS       int `json:"pauseS"`
}

// SessionStateReply — состояние родительской сессии (ExtendSession, SessionState).
type SessionStateReply struct {
	Active     bool  `json:"active"`
	ExpiresMs  int64 `json:"expiresMs"`
	RemainingS int   `json:"remainingS"`
}

// TodayCard — карточка ребёнка на «Сегодня» (ТЗ 8.3.2).
type TodayCard struct {
	ChildID          int64       `json:"childId"`
	SID              string      `json:"sid"`
	Name             string      `json:"name"`
	Status           ChildStatus `json:"status"`
	ExtraMin         int         `json:"extraMin"`         // доп. время на сегодня, мин
	BonusBalance     int         `json:"bonusBalance"`     // баланс копилки, мин (M2)
	CanClearBlock    bool        `json:"canClearBlock"`    // есть блокировка за обход, кнопка «Снять блокировку»
	NoForbiddenToday bool        `json:"noForbiddenToday"` // запретные часы сняты на сегодня (R24, D-51): показать «Вернуть»
	GamesText        string      `json:"gamesText"`        // «Игры 40 мин из 1 ч» (8.3.2 п.3); «» — отдельного лимита игр нет
	PendingPrograms  int         `json:"pendingPrograms"`  // сколько программ ждут решения: ссылка «Ждут решения: N» (8.3.2 п.5)
}

// GetTodayReply — ответ GetToday. Notices — важные сообщения при входе, например
// «База данных была повреждена и восстановлена» (Ruling 25).
type GetTodayReply struct {
	Children []TodayCard `json:"children"`
	Notices  []string    `json:"notices"`
}

// ChildRequest — запрос про одного ребёнка (GetChildRules, ClearBlock).
type ChildRequest struct {
	ChildID int64 `json:"childId"`
}

// ChildRules — недельные лимиты ребёнка и режим «Когда время вышло».
type ChildRules struct {
	ChildID int64          `json:"childId"`
	Name    string         `json:"name"`
	Limits  []WeekdayLimit `json:"limits"`  // семь дней, пн … вс
	EndMode string         `json:"endMode"` // EndModeLock или EndModeStudyOnly (R8, FR-18)
}

// SaveLimitsRequest — сохранить недельные лимиты одного ребёнка и (необязательно) режим «Когда
// время вышло». EndMode пусто — режим не меняется; Limits пусто при непустом EndMode — меняется
// только режим. Ошибки по полям: LimitField(n), GamesField(n), FieldEndMode.
type SaveLimitsRequest struct {
	ChildID int64          `json:"childId"`
	Limits  []WeekdayLimit `json:"limits"`
	EndMode string         `json:"endMode,omitempty"`
}

// SaveLimitsReply — ответ SaveLimits: правила уже действуют (FR-19).
type SaveLimitsReply struct {
	Rules   ChildRules `json:"rules"`
	Card    TodayCard  `json:"card"`
	Message string     `json:"message"` // «Сохранено. Новые правила уже действуют»
}

// AddExtraRequest — добавить родительское время на сегодня (R10): 1…240 мин.
type AddExtraRequest struct {
	ChildID int64 `json:"childId"`
	Minutes int   `json:"minutes"`
}

// AddExtraReply — ответ AddExtra, ReduceExtra и ClearExtra: карточка с новым лимитом,
// сообщение и «Отменить» на 10 с.
type AddExtraReply struct {
	Card        TodayCard `json:"card"`
	Message     string    `json:"message"`     // «Добавлено 30 минут» / «Убрано 30 минут» / «Доп. время убрано»
	ExtraID     string    `json:"extraId"`     // для UndoExtra
	UndoUntilMs int64     `json:"undoUntilMs"` // до какого момента работает «Отменить», UTC мс
}

// ReduceExtraRequest — убавить родительское доп. время на сегодня (R10, D-43): 1…240 мин.
// Суммарный дневной лимит не опускается ниже 0.
type ReduceExtraRequest struct {
	ChildID int64 `json:"childId"`
	Minutes int   `json:"minutes"`
}

// ClearExtraRequest — обнулить сегодняшнее доп. время ребёнка (R10, D-43, «Убрать доп. время»).
type ClearExtraRequest struct {
	ChildID int64 `json:"childId"`
}

// ParentStatsRequest — статистика выбранного ребёнка за сегодня для родителя (FR-23, D-44).
type ParentStatsRequest struct {
	ChildID int64 `json:"childId"`
}

// DiagReply — состояние переключателя «Подробные логи для диагностики» (FR-25, D-42).
type DiagReply struct {
	Verbose  bool   `json:"verbose"`
	LogsPath string `json:"logsPath"` // папка с логами, показывается в настройках (D-48)
}

// SetDiagRequest — включить или выключить подробные логи (FR-25, D-42).
type SetDiagRequest struct {
	Verbose bool `json:"verbose"`
}

// UndoExtraRequest — отменить только что добавленное время.
type UndoExtraRequest struct {
	ChildID int64  `json:"childId"`
	ExtraID string `json:"extraId"`
}

// CardReply — ответ команд, которые меняют состояние ребёнка и возвращают его карточку
// (UndoExtra, ClearBlock).
type CardReply struct {
	Card    TodayCard `json:"card"`
	Message string    `json:"message"`
}

// Пределы ListEvents: ответ не должен приближаться к кадру в 1 МБ (1000 записей журнала
// занимают около 0,4 МБ). Потолок равен тому, что просит экран журнала для вкладок
// «За месяц» и «Всё» (FR-24, D-47), иначе старые события пропадали бы молча.
const (
	DefaultEventsLimit = 200
	MaxEventsLimit     = 1000
)

// ListEventsRequest — журнал событий. ChildID 0 — все дети; FromMs/ToMs 0 — без границы;
// Limit 0 — DefaultEventsLimit, не больше MaxEventsLimit; сначала новые.
type ListEventsRequest struct {
	ChildID int64 `json:"childId"`
	FromMs  int64 `json:"fromMs"`
	ToMs    int64 `json:"toMs"`
	Limit   int   `json:"limit"`
}

// JournalEntry — событие журнала простыми словами (R22, ТЗ 8.3.7).
type JournalEntry struct {
	AtMs        int64  `json:"atMs"` // UTC мс
	TzOffsetMin int    `json:"tzOffsetMin"`
	ChildID     int64  `json:"childId"` // 0 — событие не про ребёнка
	ChildName   string `json:"childName"`
	Kind        string `json:"kind"`
	Text        string `json:"text"` // «Миша: попытка обойти защиту. Экран заблокирован»
}

// ListEventsReply — ответ ListEvents.
type ListEventsReply struct {
	Events []JournalEntry `json:"events"`
}

// ChangePasswordRequest — смена пароля родителя из родительской сессии.
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// ChildAdd — добавить обычную учётку ребёнком.
type ChildAdd struct {
	SID  string `json:"sid"`
	Name string `json:"name"`
}

// ChildRename — переименовать ребёнка.
type ChildRename struct {
	ChildID int64  `json:"childId"`
	Name    string `json:"name"`
}

// SetChildrenRequest — изменить состав детей одним вызовом: добавить, убрать, переименовать.
// Службе все три списка применять вместе или не применять ничего.
type SetChildrenRequest struct {
	Add    []ChildAdd    `json:"add"`
	Remove []int64       `json:"remove"`
	Rename []ChildRename `json:"rename"`
}

// ChildInfo — ребёнок в списке «Детские учётки».
type ChildInfo struct {
	ChildID int64  `json:"childId"`
	SID     string `json:"sid"`
	Name    string `json:"name"`
}

// SetChildrenReply — ответ SetChildren: список детей после изменения.
type SetChildrenReply struct {
	Children []ChildInfo `json:"children"`
}

// --- Ребёнок о себе -----------------------------------------------------------------

// MyRule — строка «Моих правил»: день недели и лимит текстом. Строки после семи дней — общие
// правила (Weekday 0, Day пусто): лимит игр «Игры: до 1 ч в день» и «Когда время вышло: …» (R21).
type MyRule struct {
	Weekday int    `json:"weekday"` // 1 = пн … 7 = вс; 0 — правило не про день
	Day     string `json:"day"`     // «пн»; «» у общих правил
	Text    string `json:"text"`    // «2 ч», «Без лимита», «Нельзя», «Игры: до 1 ч в день»
}

// MyTodayReply — ответ MyToday: «Сегодня» и «Мои правила» текстом, только чтение.
type MyTodayReply struct {
	Status ChildStatus `json:"status"`
	Today  []string    `json:"today"` // строки «Сегодня»
	Rules  []MyRule    `json:"rules"`
	// GamesS, StudyS — игры и учёба за сегодня, с (итоги «Моей статистики», S13); счётчики ядра.
	GamesS int `json:"gamesS"`
	StudyS int `json:"studyS"`
}

// --- Страж --------------------------------------------------------------------------

// Названия категорий программы на переднем плане в снимке датчиков.
const (
	CatNameOther       = "other"
	CatNameGames       = "games"
	CatNameVideoSocial = "videoSocial"
	CatNameStudy       = "study"
)

// SensorSample — снимок датчиков сеанса (core.SessionSample в JSON). Заголовков окон и
// нажатий клавиш здесь нет (R23).
type SensorSample struct {
	AtMs           int64  `json:"atMs"` // UTC мс
	TzOffsetMin    int    `json:"tzOffsetMin"`
	SessionActive  bool   `json:"sessionActive"`
	LastInputAgoMs int64  `json:"lastInputAgoMs"`
	Fullscreen     bool   `json:"fullscreen"`
	Foreground     string `json:"foreground"` // CatName*
	// ForegroundPath — путь exe программы на переднем плане (R3, M3); «» — не определён. Длиннее
	// MaxForegroundPathLen символов служба отбрасывает.
	ForegroundPath string `json:"foregroundPath"`
}

// Виды событий стража в GuardPollRequest.Events.
const (
	GuardEvWarningShown = "warningShown" // показана плашка предупреждения (Threshold)
	GuardEvWarningAck   = "warningAck"   // ребёнок нажал «Понятно»
	GuardEvLockShown    = "lockShown"    // экран блокировки показан
	GuardEvLockHidden   = "lockHidden"   // экран блокировки снят
	GuardEvButton       = "button"       // нажата кнопка экрана или плашки (Button)
	GuardEvBypass       = "bypass"       // страж заметил попытку обхода (Detail простыми словами)
)

// Кнопки экрана блокировки и плашек для GuardEvButton и TestGuard.
const (
	ButtonParentPassword = "parentPassword" // «Пароль родителя»
	ButtonLogoff         = "logoff"         // «Выйти из учётной записи»
	ButtonShutdown       = "shutdown"       // «Выключить компьютер»
	ButtonSwitchUser     = "switchUser"     // «Смена пользователя»
	ButtonWarningClose   = "warningClose"   // крестик «Закрыть» на плашке-табло предупреждения
)

// GuardEvent — событие, которое страж сообщает службе.
type GuardEvent struct {
	Kind      string `json:"kind"`      // GuardEv*
	AtMs      int64  `json:"atMs"`      // UTC мс
	Threshold int    `json:"threshold"` // для warningShown и warningAck: 30, 15, 5 или 1
	Button    string `json:"button"`    // для button: Button*
	Detail    string `json:"detail"`    // для bypass: что произошло, без заголовков окон (R23)
}

// GuardPollRequest — раз в реальную секунду: снимок датчиков и накопленные события.
type GuardPollRequest struct {
	Sample SensorSample `json:"sample"`
	Events []GuardEvent `json:"events"`
}

// GuardWarning — предупреждение, которое страж показывает один раз (R9).
type GuardWarning struct {
	Threshold int    `json:"threshold"` // 30, 15, 5 или 1 минута (D-54)
	Text      string `json:"text"`      // что закончится: строка под плитками табло или текст плашки 1 мин
	Sound     bool   `json:"sound"`     // звук заказчика в момент показа: на всех порогах (D-55)
	Event     string `json:"event"`     // вид события: WarnEvent*
	EventS    int    `json:"eventS"`    // секунды до события в момент срабатывания
}

// Виды события предупреждения в GuardWarning.Event (R9): от вида зависит, как страж считает
// секунды на плитках табло. Лимит: остаток службы (стоит, когда время не идёт, R2); запретные
// часы и конец паузы: настоящие часы.
const (
	WarnEventLimit     = "limit"
	WarnEventForbidden = "forbidden"
	WarnEventPause     = "pause"
	WarnEventGames     = "games" // исчерпание лимита игр (R5): остаток игр службы, как у лимита
)

// Виды команд стражу в GuardPollReply.Commands.
const (
	GuardCmdHideLock = "hideLock" // спрятать экран блокировки: открыта родительская сессия (Ruling 19)
	GuardCmdNotice   = "notice"   // показать короткое сообщение (Text), например «Добавлено 30 минут»
	GuardCmdTest     = "test"     // действие TestGuard, только в тестовом режиме (Ruling 5)
	// GuardCmdBlocked = "blocked" (commands_m3.go): плашка о закрытой программе (Plate, FR-13).
)

// Действия TestGuard (Ruling 5): страж выполняет их через настоящие сообщения своих окон.
const (
	TestClick   = "click"   // нажать кнопку Target (Button*)
	TestType    = "type"    // ввести Text в поле Target
	TestWMClose = "wmClose" // послать окну экрана WM_CLOSE
	TestAltF4   = "altF4"   // нажать Alt+F4
	TestEsc     = "esc"     // нажать Esc
	// TestEndSession: страж проходит тот же путь, что при конце сеанса Windows (WM_ENDSESSION):
	// сообщает службе GuardClosing и завершается. Приёмка проверяет, что это не обход (FB-1).
	TestEndSession = "endSession"
	// TestForeground: страж подменяет SensorSample.ForegroundPath путём Text во всех следующих
	// снимках, пока не придёт новое действие foreground (M3, приёмка S12…S15); пустой Text
	// возвращает настоящее чтение переднего плана.
	TestForeground = "foreground"
)

// TestGuardAction — одно действие стражу.
type TestGuardAction struct {
	Action string `json:"action"`           // Test*
	Target string `json:"target,omitempty"` // кнопка или поле
	Text   string `json:"text,omitempty"`   // вводимый текст
}

// GuardCommand — команда службы стражу в ответе на GuardPoll.
type GuardCommand struct {
	Kind string           `json:"kind"` // GuardCmd*
	Text string           `json:"text,omitempty"`
	Test *TestGuardAction `json:"test,omitempty"`
	// Plate — плашка о закрытой программе для GuardCmdBlocked (FR-13, M3).
	Plate *BlockedPlate `json:"plate,omitempty"`
}

// GuardPollReply — ответ GuardPoll: решение для экрана, новые предупреждения, команды стражу.
type GuardPollReply struct {
	Status   ChildStatus    `json:"status"`
	Warnings []GuardWarning `json:"warnings"`
	Commands []GuardCommand `json:"commands"`
}

// --- Тестовый режим ---------------------------------------------------------------------

// TestGuardRequest — действие стражу в тестовом режиме. SessionID 0 — единственный сеанс
// стража.
type TestGuardRequest struct {
	SessionID uint32          `json:"sessionId,omitempty"`
	Action    TestGuardAction `json:"action"`
}

// TestSessionRequest — событие сеанса Windows, которое служба принимает тем же путём, что от
// диспетчера служб (только тестовый режим, приёмка FB5): в CI нет второго сеанса, выход из
// которого можно было бы сделать по-настоящему, а сеанс раннера сам возвращается на экран.
type TestSessionRequest struct {
	Event     string `json:"event"`     // TestSession*
	SessionID uint32 `json:"sessionId"` // номер сеанса
	// SID — пользователь сеанса, который выходит, если служба этот сеанс не знает (в CI нет второго
	// пользователя): выход засчитывается как конец сеанса этого пользователя (FB-5).
	SID string `json:"sid,omitempty"`
}

// События TestSession.
const (
	TestSessionDisconnect = "disconnect" // смена пользователя: сеанс отключён
	TestSessionLogoff     = "logoff"     // выход из учётки
)

// TestClockRequest — поставить часы ядра на паузу или продолжить их (Ruling 4).
type TestClockRequest struct {
	Paused bool `json:"paused"`
}

// TestClockReply — часы ядра после команды.
type TestClockReply struct {
	Paused bool  `json:"paused"`
	NowMs  int64 `json:"nowMs"` // часы ядра, UTC мс
	Speed  int   `json:"speed"` // ClockSpeed
}

// ShownWarning — предупреждение, показанное стражем.
type ShownWarning struct {
	AtMs      int64  `json:"atMs"`
	Threshold int    `json:"threshold"`
	Text      string `json:"text"`
}

// ShownSound — звук предупреждения, который страж сыграл (тестовый режим): порог и ответ
// Windows (PlaySoundW принял звук; на раннере без звуковой карты бывает false).
type ShownSound struct {
	AtMs      int64 `json:"atMs"`
	Threshold int   `json:"threshold"`
	OK        bool  `json:"ok"`
}

// GuardUI — жив ли поток окон стража: момент и число его кадров, последняя паника, пойманная в
// потоке окон или горутинах стража. Приёмка отличает «окно стоит» от «окно падает».
type GuardUI struct {
	TickMs int64  `json:"tickMs"` // UTC мс последнего кадра; 0 — кадров не было
	Ticks  int64  `json:"ticks"`
	Panic  string `json:"panic"` // «» — паник не было
}

// GuardField — поле ввода стража и его текст.
type GuardField struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// GuardStateReply — что сейчас показывает страж (GuardState, только тестовый режим).
type GuardStateReply struct {
	Warnings  []ShownWarning `json:"warnings"`  // журнал показанных предупреждений
	Sounds    []ShownSound   `json:"sounds"`    // журнал сыгранных звуков предупреждений (D-55)
	UI        GuardUI        `json:"ui"`        // диагностика потока окон стража (тестовый режим)
	LockShown bool           `json:"lockShown"` // экран блокировки показан
	LockKind  string         `json:"lockKind"`  // Lock*
	Plate     string         `json:"plate"`     // текст видимой плашки или табло предупреждения; пусто — плашки нет
	PlateKind string         `json:"plateKind"` // вид видимой плашки: Plate* или Blocked*; пусто — плашки нет
	Buttons   []string       `json:"buttons"`   // видимые кнопки (Button*)
	Fields    []GuardField   `json:"fields"`    // поля ввода и их текст
	// Exits — выходы, о которых страж просил службу в тестовом режиме (Exit*), по порядку.
	// В тестовом режиме служба их не выполняет, а записывает: раннер CI нельзя выключить или
	// отключить. Заполняет служба при чтении; то, что прислал страж, отбрасывается.
	Exits []string `json:"exits"`
	// ForegroundPath — путь переднего плана, который страж отправил службе последним снимком
	// (M3; подмена TestForeground тоже видна здесь); «» — не определён.
	ForegroundPath string `json:"foregroundPath"`
}

// Виды плашек стража для GuardStateReply.PlateKind, кроме плашек о закрытых программах (их вид —
// BlockedPlate.Kind).
const (
	PlateBoard     = "board"     // табло за 30, 15 и 5 мин
	PlateOneMinute = "oneMinute" // плашка 1 мин с отсчётом
	PlateAdded     = "added"     // «Добавлено N минут»
	PlateNotice    = "notice"    // короткое сообщение стража (команда службе не прошла)
)

// Выходы с экрана блокировки для GuardStateReply.Exits (тестовый режим).
const (
	ExitShutdown   = "shutdown"   // выключение компьютера
	ExitSwitchUser = "switchUser" // смена пользователя, в том числе автоматическая
)
