package ipc

import "encoding/json"

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
	CmdUndoExtra      = "UndoExtra"
	CmdClearBlock     = "ClearBlock"
	CmdListEvents     = "ListEvents"
	CmdChangePassword = "ChangePassword"
	CmdSetChildren    = "SetChildren"

	// Состояние Помощника: профиль родителя (из родительской сессии) или ребёнка
	// (из его сеанса); какой именно, решает служба по тому, кто спрашивает.
	CmdGetHelperState  = "GetHelperState"
	CmdSaveHelperState = "SaveHelperState"

	// Ребёнок о себе (только свой сеанс, только чтение).
	CmdMyToday      = "MyToday"
	CmdFirstRunSeen = "FirstRunSeen"

	// Страж (только процесс системы в том же сеансе).
	CmdGuardPoll   = "GuardPoll"
	CmdGuardLogoff = "GuardLogoff"

	// Значок (только процесс пользователя своего сеанса).
	CmdTrayClosing = "TrayClosing"

	// Только тестовый режим (ТЗ 10.8).
	CmdTestGuard  = "TestGuard"
	CmdTestClock  = "TestClock"
	CmdGuardState = "GuardState"
)

// AllCommands — все команды M1 в порядке групп. По этому списку служба проверяет, что у
// каждой команды есть строка в таблице прав, а contract_test.go — что у каждой есть
// пример в testdata/contract.json: новая команда не пройдёт без того и другого.
var AllCommands = []string{
	CmdHello, CmdGetChildStatus, CmdOpenWindow, CmdWindowClosing,
	CmdListAccounts, CmdSetup, CmdResetPassword, CmdUninstallCheck,
	CmdLogin, CmdExtendSession, CmdLogout, CmdSessionState, CmdGetToday, CmdGetChildRules,
	CmdSaveLimits, CmdAddExtra, CmdUndoExtra, CmdClearBlock, CmdListEvents, CmdChangePassword,
	CmdSetChildren, CmdGetHelperState, CmdSaveHelperState,
	CmdMyToday, CmdFirstRunSeen,
	CmdGuardPoll, CmdGuardLogoff,
	CmdTrayClosing,
	CmdTestGuard, CmdTestClock, CmdGuardState,
}

// Имена полей, к которым служба привязывает ошибку (Reply.Field, L-121).
const (
	FieldPassword    = "password"
	FieldOldPassword = "oldPassword"
	FieldNewPassword = "newPassword"
	FieldChildren    = "children"
	FieldMinutes     = "minutes"
	FieldState       = "state"
)

// LimitField — имя поля лимита дня недели (1 = пн … 7 = вс) для ошибок SaveLimits и Setup.
func LimitField(weekday int) string { return "limits." + itoa(weekday) }

// Empty — нагрузка команд без данных; уходит как «{}».
type Empty struct{}

// ProtocolVersion — версия протокола, её возвращает Hello.
const ProtocolVersion = 1

// --- Общие -----------------------------------------------------------------

// HelloReply — ответ Hello.
type HelloReply struct {
	Protocol   int    `json:"protocol"`
	Version    string `json:"version"`    // версия программы, например «0.1.0»
	TestMode   bool   `json:"testMode"`   // включён тестовый режим (ТЗ 10.8)
	Configured bool   `json:"configured"` // пароль родителя задан, мастер пройден
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
	StatusBypass    = "bypass"
	StatusUnlimited = "unlimited"
	StatusSafe      = "safe"
	StatusAdmin     = "admin"
	StatusNotChild  = "notChild"
)

// Значения ChildStatus.Lock: какой экран блокировки показать.
const (
	LockNone   = "none"
	LockTimeUp = "timeUp"
	LockBypass = "bypass"
)

// ChildStatus — состояние ребёнка. Числа и тексты экрана блокировки берутся из
// core.Decision без пересчёта (ChildStatusFrom); надпись, подсказка и слово статуса
// собираются в одном месте, поэтому на всех экранах они одинаковы (Review Focus 5).
type ChildStatus struct {
	Name       string `json:"name"`       // имя ребёнка
	Status     string `json:"status"`     // Status*; цвет определяет только он (ТЗ 8.0 п.3)
	StatusText string `json:"statusText"` // статус словом для карточки родителя: «Можно», «Время вышло»
	Label      string `json:"label"`      // надпись на значке: минуты остатка «85», «0», «∞»
	Tooltip    string `json:"tooltip"`    // подсказка значка: «Осталось 1 ч 25 мин»
	Summary    string `json:"summary"`    // строка мини-панели: «Сегодня осталось 1 ч 25 мин из 2 ч»
	Day        string `json:"day"`        // день цифр, YYYY-MM-DD
	RemainingS int    `json:"remainingS"` // остаток общего времени, с (0 без лимита)
	LimitS     int    `json:"limitS"`     // лимит дня с доп. временем, с (0 без лимита)
	UsedS      int    `json:"usedS"`      // общее время за день, с
	Unlimited  bool   `json:"unlimited"`  // лимита нет или правила не прочитаны
	Lock       string `json:"lock"`       // Lock*
	Reason     string `json:"reason"`     // заголовок экрана блокировки
	Detail     string `json:"detail"`     // что произошло (только при обходе)
	Until      string `json:"until"`      // «Можно снова: завтра»
	Used       string `json:"used"`       // «Сегодня: 2 ч 00 мин из 2 ч»
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
type WeekdayLimit struct {
	Weekday   int  `json:"weekday"`
	TotalMin  int  `json:"totalMin"`
	Unlimited bool `json:"unlimited"`
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
	ChildID       int64       `json:"childId"`
	SID           string      `json:"sid"`
	Name          string      `json:"name"`
	Status        ChildStatus `json:"status"`
	ExtraMin      int         `json:"extraMin"`      // доп. время на сегодня, мин
	CanClearBlock bool        `json:"canClearBlock"` // есть блокировка за обход, кнопка «Снять блокировку»
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

// ChildRules — недельные лимиты ребёнка.
type ChildRules struct {
	ChildID int64          `json:"childId"`
	Name    string         `json:"name"`
	Limits  []WeekdayLimit `json:"limits"` // семь дней, пн … вс
}

// SaveLimitsRequest — сохранить недельные лимиты одного ребёнка. Ошибки по полям: LimitField(n).
type SaveLimitsRequest struct {
	ChildID int64          `json:"childId"`
	Limits  []WeekdayLimit `json:"limits"`
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

// AddExtraReply — ответ AddExtra: карточка с новым лимитом и «Отменить» на 10 с.
type AddExtraReply struct {
	Card        TodayCard `json:"card"`
	Message     string    `json:"message"`     // «Добавлено 30 минут»
	ExtraID     string    `json:"extraId"`     // для UndoExtra
	UndoUntilMs int64     `json:"undoUntilMs"` // до какого момента работает «Отменить», UTC мс
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

// Пределы ListEvents: ответ не должен приближаться к кадру в 1 МБ.
const (
	DefaultEventsLimit = 200
	MaxEventsLimit     = 500
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

// HelperState — состояние Помощника профиля (ТЗ 8.4 п.7). Содержимое определяет каталог
// Помощника (internal/guidance), канал его не разбирает; пустое состояние — «{}».
type HelperState struct {
	State json.RawMessage `json:"state"`
}

// MarshalJSON не пускает в ответ null вместо объекта.
func (h HelperState) MarshalJSON() ([]byte, error) {
	state := h.State
	if len(state) == 0 {
		state = json.RawMessage(`{}`)
	}
	return json.Marshal(struct {
		State json.RawMessage `json:"state"`
	}{state})
}

// --- Ребёнок о себе -----------------------------------------------------------------

// MyRule — строка «Моих правил»: день недели и лимит текстом.
type MyRule struct {
	Weekday int    `json:"weekday"` // 1 = пн … 7 = вс
	Day     string `json:"day"`     // «пн»
	Text    string `json:"text"`    // «2 ч», «Без лимита», «Нельзя»
}

// MyTodayReply — ответ MyToday: «Сегодня» и «Мои правила» текстом, только чтение.
type MyTodayReply struct {
	Status ChildStatus `json:"status"`
	Today  []string    `json:"today"` // строки «Сегодня»
	Rules  []MyRule    `json:"rules"`
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
	ButtonAck            = "ack"            // «Понятно»
)

// GuardEvent — событие, которое страж сообщает службе.
type GuardEvent struct {
	Kind      string `json:"kind"`      // GuardEv*
	AtMs      int64  `json:"atMs"`      // UTC мс
	Threshold int    `json:"threshold"` // для warningShown и warningAck: 15, 5 или 1
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
	Threshold int    `json:"threshold"` // 15, 5 или 1 минута
	Text      string `json:"text"`
	Sound     bool   `json:"sound"` // за 1 минуту — звук и обратный отсчёт
}

// Виды команд стражу в GuardPollReply.Commands.
const (
	GuardCmdHideLock = "hideLock" // спрятать экран блокировки: открыта родительская сессия (Ruling 19)
	GuardCmdNotice   = "notice"   // показать короткое сообщение (Text), например «Добавлено 30 минут»
	GuardCmdTest     = "test"     // действие TestGuard, только в тестовом режиме (Ruling 5)
)

// Действия TestGuard (Ruling 5): страж выполняет их через настоящие сообщения своих окон.
const (
	TestClick   = "click"   // нажать кнопку Target (Button*)
	TestType    = "type"    // ввести Text в поле Target
	TestWMClose = "wmClose" // послать окну экрана WM_CLOSE
	TestAltF4   = "altF4"   // нажать Alt+F4
	TestEsc     = "esc"     // нажать Esc
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

// GuardField — поле ввода стража и его текст.
type GuardField struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// GuardStateReply — что сейчас показывает страж (GuardState, только тестовый режим).
type GuardStateReply struct {
	Warnings  []ShownWarning `json:"warnings"`  // журнал показанных предупреждений
	LockShown bool           `json:"lockShown"` // экран блокировки показан
	LockKind  string         `json:"lockKind"`  // Lock*
	Buttons   []string       `json:"buttons"`   // видимые кнопки (Button*)
	Fields    []GuardField   `json:"fields"`    // поля ввода и их текст
}
