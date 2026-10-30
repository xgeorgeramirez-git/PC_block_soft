package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// EvKindOffline — вид события стража «был без связи со службой». В канале связи такого вида
// нет (ipc.GuardEv*): страж отправляет его как есть, Threshold несёт длительность в секундах,
// Detail несёт текст для журнала. Пока служба его не разбирает, событие ею просто пропускается.
const EvKindOffline = "guardOffline"

// Пределы и тексты контроллера.
const (
	maxQueuedEvents = 100             // очередь событий не растёт без связи со службой
	noticeLife      = 6 * time.Second // сколько виден короткий ответ под заголовком экрана
	maxShownLog     = 50              // журнал показанных предупреждений для GuardState
	maxSoundLog     = 50              // журнал сыгранных звуков для GuardState

	textNoticeRetry  = "Не удалось выполнить. Попробуйте ещё раз"
	textNoticeLogoff = "Не получилось выйти. Попробуй ещё раз"

	textNoticeShutdown   = "Не получилось выключить. Попробуй ещё раз"
	textNoticeSwitchUser = "Не получилось сменить пользователя. Попробуй ещё раз"
)

// Controller связывает модель стража с службой: раз в секунду собирает снимок датчиков и
// отправляет GuardPoll, прогоняет ответ через Model, исполняет нажатия (Login, AddExtra,
// ClearBlock, OpenWindow, GuardLogoff) и копит журнал показанного. Здесь нет Windows: окна
// (T10b, *_windows.go) только рисуют View и сообщают нажатия. Методы безопасны для вызова из
// нескольких горутин; PollOnce зовёт одна горутина опроса.
type Controller struct {
	model *Model
	call  ipc.Caller
	now   func() time.Time
	read  func(now time.Time) ipc.SensorSample
	wake  func()
	kick  chan struct{}

	mu        sync.Mutex
	events    []ipc.GuardEvent
	shown     []ipc.ShownWarning
	sounds    []ipc.ShownSound // сыгранные звуки (журнал для GuardState, только тестовый режим)
	soundID   int              // ID плашки, для которой звук уже сыгран
	play      func() bool      // проиграть звук предупреждения; true — Windows принял звук
	soundQ    chan int         // номера записей журнала звуков для исполнителя звука; nil — исполнитель не запущен
	speed     int              // ускорение часов ядра, сообщённое окном; модели передаётся только в тестовом режиме
	ui        ipc.GuardUI      // диагностика потока окон
	seenPlate int              // наибольший ID плашки, о которой уже известно (ID растут; вернувшаяся плашка не новая)
	lockWas   bool
	lastLock  string
	last      View
	helloDone bool
	testMode  bool
	busy      bool
	notice    string
	noticeEnd time.Time
	tests     []ipc.TestGuardAction
	fields    map[string]string
	sentState []byte
	// fgSet/fgPath — подмена пути переднего плана действием TestForeground (только тестовый
	// режим, RM3-8); lastFg — путь из последнего отправленного снимка (GuardState).
	fgSet  bool
	fgPath string
	lastFg string

	closingMu   sync.Mutex
	closingDone bool // GuardClosing в этом конце сеанса уже отправлен (или попытка не удалась)
	closingOK   bool // исход первой попытки
}

// NewController собирает контроллер. now — настоящие часы (Ruling 4), read — чтение датчиков,
// wake — «что-то изменилось, перерисуй» (может быть nil); вызывается из любой горутины.
func NewController(model *Model, call ipc.Caller, now func() time.Time, read func(time.Time) ipc.SensorSample, wake func()) *Controller {
	if wake == nil {
		wake = func() {}
	}
	return &Controller{
		model: model, call: call, now: now, read: read, wake: wake,
		kick:   make(chan struct{}, 1),
		fields: map[string]string{},
	}
}

// lastPanic — текст последней паники, пойманной safely (диагностика GuardState.UI).
var lastPanic atomic.Value

// safely выполняет f и не даёт панике убить страж: упавший страж означает «обход» для
// службы (R17в), а ошибка одного опроса, одного кадра или звука обходом не является. Паника
// запоминается для GuardState.UI.
func safely(f func()) {
	defer func() {
		if r := recover(); r != nil {
			lastPanic.Store(fmt.Sprint(r))
		}
	}()
	f()
}

// NoteUITick отмечает кадр потока окон (диагностика GuardState.UI).
func (c *Controller) NoteUITick(now time.Time) {
	c.mu.Lock()
	c.ui.TickMs, c.ui.Ticks = now.UnixMilli(), c.ui.Ticks+1
	c.mu.Unlock()
}

// SetSoundPlayer задаёт, чем играть звук предупреждения (окно: sound.PlayWarning). Без него
// звук не играет. Вызывать до первого View.
func (c *Controller) SetSoundPlayer(play func() bool) {
	c.mu.Lock()
	c.play = play
	c.mu.Unlock()
}

// SetClockSpeed запоминает ускорение часов ядра из реестра тестового режима: в тестовом режиме
// секунды до запретных часов и конца паузы на табло идут в том же темпе, что учёт.
func (c *Controller) SetClockSpeed(n int) {
	c.mu.Lock()
	c.speed = n
	c.mu.Unlock()
}

// Kicked возвращает канал, в который контроллер пишет, когда опрос нужен немедленно (после
// нажатия, которое меняет решение службы).
func (c *Controller) Kicked() <-chan struct{} { return c.kick }

// Kick просит горутину опроса не ждать секунду.
func (c *Controller) Kick() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// TestMode: служба сообщила, что включён тестовый режим (ТЗ 10.8).
func (c *Controller) TestMode() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.testMode
}

// Commands — команды службы стражу из ответа GuardPoll, разобранные по видам.
type Commands struct {
	HideLock      bool                  // родительская сессия открыта: экран спрятать (Ruling 19)
	NoticeMinutes []int                 // «Добавлено N минут»: плашка на 5 с
	Blocked       []ipc.BlockedPlate    // плашки о закрытых программах (FR-13), по порядку
	Tests         []ipc.TestGuardAction // действия TestGuard (только тестовый режим)
}

// ClassifyCommands раскладывает команды службы; незнакомые виды пропускаются.
func ClassifyCommands(cmds []ipc.GuardCommand) Commands {
	var out Commands
	for _, c := range cmds {
		switch c.Kind {
		case ipc.GuardCmdHideLock:
			out.HideLock = true
		case ipc.GuardCmdNotice:
			if n, ok := NoticeMinutes(c.Text); ok {
				out.NoticeMinutes = append(out.NoticeMinutes, n)
			}
		case ipc.GuardCmdTest:
			if c.Test != nil {
				out.Tests = append(out.Tests, *c.Test)
			}
		case ipc.GuardCmdBlocked:
			if c.Plate != nil {
				out.Blocked = append(out.Blocked, *c.Plate)
			}
		}
	}
	return out
}

// NoticeMinutes достаёт число минут из сообщения службы вида «Добавлено 30 минут» или
// «Добавлена 1 минута»: первое целое число в тексте. Нет числа или оно не больше нуля: false.
func NoticeMinutes(text string) (int, bool) {
	n, found := 0, false
	for _, r := range text {
		if r >= '0' && r <= '9' {
			if n > 100000 {
				break
			}
			n, found = n*10+int(r-'0'), true
			continue
		}
		if found {
			break
		}
	}
	return n, found && n > 0
}

// DecisionFromStatus превращает статус из ответа службы в решение для модели. Модели нужны вид
// блокировки, тексты экрана и цифры остатка; статус ядра ей не нужен, но восстанавливается.
func DecisionFromStatus(s ipc.ChildStatus) core.Decision {
	d := core.Decision{
		Day: s.Day, Reason: s.Reason, Detail: s.Detail, Until: s.Until, Used: s.Used,
		PauseUntil: s.PauseUntil, RemainingS: s.RemainingS, LimitS: s.LimitS, UsedS: s.UsedS,
		BonusBalance: s.BonusBalance, Unlimited: s.Unlimited, Events: []core.Event{},
		GamesRemainingS: s.GamesRemainingS, GamesLimitS: s.GamesLimitS, GamesUnlimited: s.GamesUnlimited,
		CloseGames: s.CloseGames, StudyOnly: s.StudyOnly,
	}
	switch s.Lock {
	case ipc.LockTimeUp:
		d.Lock = core.LockTimeUp
	case ipc.LockForbidden:
		d.Lock = core.LockForbidden
	case ipc.LockBypass:
		d.Lock = core.LockBypass
	}
	switch s.Status {
	case ipc.StatusOK:
		d.Status = core.StatusOK
	case ipc.StatusLow:
		d.Status = core.StatusLow
	case ipc.StatusTimeUp:
		d.Status = core.StatusTimeUp
	case ipc.StatusForbidden:
		d.Status = core.StatusForbidden
	case ipc.StatusBypass:
		d.Status = core.StatusBypass
	case ipc.StatusUnlimited:
		d.Status = core.StatusUnlimited
	case ipc.StatusPaused:
		d.Status = core.StatusPaused
	default:
		d.Status = core.StatusSafe
	}
	return d
}

// Thresholds достаёт пороги из предупреждений службы.
func Thresholds(ws []ipc.GuardWarning) []int {
	out := make([]int, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Threshold)
	}
	return out
}

// LoginOutcome — итог проверки пароля родителя для модели и для сообщения под заголовком.
type LoginOutcome struct {
	Result  PasswordResult // что сообщить модели; имеет смысл, только если Decided
	Decided bool           // служба дала определённый ответ: пароль верный, неверный или пауза
	Notice  string         // что сказать человеку, если ответа по паролю нет (связи нет, сбой службы)
}

// LoginOutcomeOf разбирает итог команды Login. Пауза после серии ошибок приходит отказом с
// LoginDenied.PauseS (R19); неверный пароль, с LoginDenied.AttemptsLeft. Отказ без этих данных
// (программа не настроена, служба занята) и обрыв связи пароля не оценивают: показывается
// текст службы или «Программа не отвечает».
func LoginOutcomeOf(err error, now time.Time) LoginOutcome {
	if err == nil {
		return LoginOutcome{Result: PasswordResult{OK: true}, Decided: true}
	}
	var re *ipc.ReplyError
	if errors.As(err, &re) && len(re.Payload) > 0 {
		var d ipc.LoginDenied
		if json.Unmarshal(re.Payload, &d) == nil && (d.PauseS > 0 || re.Field == ipc.FieldPassword) {
			if d.PauseS > 0 {
				return LoginOutcome{Decided: true, Result: PasswordResult{BlockedUntil: now.Add(time.Duration(d.PauseS) * time.Second)}}
			}
			return LoginOutcome{Decided: true, Result: PasswordResult{AttemptsLeft: d.AttemptsLeft}}
		}
	}
	return LoginOutcome{Notice: failureText(err, textNoticeRetry)}
}

// failureText возвращает текст ошибки для человека: ответ службы как есть, обрыв связи
// как «Программа не отвечает. Попробуйте ещё раз» (подробности канала не показываются, L-112),
// иначе запасной текст.
func failureText(err error, fallback string) string {
	var re *ipc.ReplyError
	if errors.As(err, &re) && re.Text != "" {
		return re.Text
	}
	var ie *ipc.Error
	if errors.As(err, &ie) {
		return ie.Error() + ". Попробуйте ещё раз"
	}
	return fallback
}

// PollResult — итог одного опроса.
type PollResult struct {
	Err      error // нет ответа службы: модель подведена к «без связи»
	Commands Commands
}

// PollOnce опрашивает службу один раз: снимок датчиков и накопленные события уходят в GuardPoll,
// ответ идёт в модель. Нет ответа: события возвращаются в очередь, модель узнаёт о потере связи
// (через 10 с экран и плашки снимаются, Ruling 15).
func (c *Controller) PollOnce() PollResult {
	c.ensureHello()
	now := c.now()
	req := ipc.GuardPollRequest{Sample: c.read(now), Events: c.takeEvents()}
	c.mu.Lock()
	if c.fgSet && c.testMode {
		req.Sample.ForegroundPath = c.fgPath
	}
	c.lastFg = req.Sample.ForegroundPath
	c.mu.Unlock()
	var rep ipc.GuardPollReply
	if err := ipc.Call(c.call, ipc.CmdGuardPoll, "", req, &rep); err != nil {
		c.requeue(req.Events)
		c.mu.Lock()
		c.helloDone = false
		c.mu.Unlock()
		c.model.OnServiceUnreachable(c.now())
		c.wake()
		return PollResult{Err: err}
	}

	now = c.now()
	cmds := ClassifyCommands(rep.Commands)
	c.model.OnWarnings(now, DecisionFromStatus(rep.Status), cmds.HideLock, IncomingOf(rep.Warnings))
	for _, minutes := range cmds.NoticeMinutes {
		c.model.OnExtraAdded(now, minutes)
	}
	for _, pl := range cmds.Blocked {
		c.model.OnBlocked(now, pl)
	}
	v := c.model.View(now)
	// Журнал показанного, звук и вид для GuardState берутся из одного кадра: горутина опроса
	// отправляет GuardState сразу после PollOnce, раньше, чем поток окон перерисуется по wake.
	// Иначе снимок несогласован: порог 1 уже в журнале, а кнопки и звуки от табло «5» (CI S2, S35).
	c.playDue(now, v.Warning)

	c.mu.Lock()
	c.lastLock = rep.Status.Lock
	c.last = v
	if v.Warning != nil && v.Warning.ID > c.seenPlate {
		c.seenPlate = v.Warning.ID
		if t := v.Warning.Threshold; t != 0 && len(cmds.NoticeMinutes) == 0 {
			c.addEventLocked(ipc.GuardEvent{Kind: ipc.GuardEvWarningShown, AtMs: now.UnixMilli(), Threshold: t})
			c.shown = append(c.shown, ipc.ShownWarning{AtMs: now.UnixMilli(), Threshold: t, Text: v.Warning.Text})
			if len(c.shown) > maxShownLog {
				c.shown = c.shown[len(c.shown)-maxShownLog:]
			}
		}
	}
	for _, e := range c.model.TakeEvents() {
		if e.Kind != EvGuardOffline {
			continue
		}
		secs, _ := e.Details["seconds"].(int)
		text, _ := e.Details["text"].(string)
		c.addEventLocked(ipc.GuardEvent{Kind: EvKindOffline, AtMs: now.UnixMilli(), Threshold: secs, Detail: text})
	}
	for _, t := range cmds.Tests {
		switch {
		case !c.testMode:
		case t.Action == ipc.TestForeground:
			// Подмена переднего плана — дело контроллера, не окна: действует со следующего снимка.
			c.fgSet, c.fgPath = t.Text != "", t.Text
		default:
			c.tests = append(c.tests, t)
		}
	}
	c.mu.Unlock()
	c.wake()
	return PollResult{Commands: cmds}
}

// ensureHello один раз за соединение узнаёт у службы, включён ли тестовый режим. Ошибка не
// важна: повторится при следующем опросе.
func (c *Controller) ensureHello() {
	c.mu.Lock()
	done := c.helloDone
	c.mu.Unlock()
	if done {
		return
	}
	var h ipc.HelloReply
	if err := ipc.Call(c.call, ipc.CmdHello, "", nil, &h); err != nil {
		return
	}
	c.mu.Lock()
	c.helloDone, c.testMode = true, h.TestMode
	speed := c.speed
	c.mu.Unlock()
	if h.TestMode && speed > 1 {
		c.model.SetClockSpeed(speed)
	}
	if h.TestMode && h.AutoSwitchS > 0 {
		// Срок автосмены сокращается только в тестовом режиме: приёмка не ждёт 15 минут.
		c.model.SetAutoSwitchAfter(time.Duration(h.AutoSwitchS) * time.Second)
	}
}

func (c *Controller) takeEvents() []ipc.GuardEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	if out == nil {
		out = []ipc.GuardEvent{}
	}
	return out
}

// requeue возвращает неотправленные события в начало очереди.
func (c *Controller) requeue(evs []ipc.GuardEvent) {
	if len(evs) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(append([]ipc.GuardEvent{}, evs...), c.events...)
	if over := len(c.events) - maxQueuedEvents; over > 0 {
		c.events = c.events[over:]
	}
}

func (c *Controller) addEventLocked(e ipc.GuardEvent) {
	c.events = append(c.events, e)
	if over := len(c.events) - maxQueuedEvents; over > 0 {
		c.events = c.events[over:]
	}
}

// View подводит модель к настоящему моменту и возвращает, что показать. Зовёт окно: по
// таймеру и по сигналу wake. Заодно ведёт события «экран показан/снят».
func (c *Controller) View() View {
	now := c.now()
	c.model.Tick(now)
	v := c.model.View(now)
	c.playDue(now, v.Warning)
	c.mu.Lock()
	defer c.mu.Unlock()
	if v.LockShown != c.lockWas {
		kind := ipc.GuardEvLockHidden
		if v.LockShown {
			kind = ipc.GuardEvLockShown
		}
		c.addEventLocked(ipc.GuardEvent{Kind: kind, AtMs: now.UnixMilli()})
		c.lockWas = v.LockShown
	}
	c.last = v
	return v
}

// playDue играет звук один раз на каждую новую плашку предупреждения (D-55): решение
// SoundDue модели, окно только исполняет. Звук уходит исполнителю в своей горутине: PlaySoundW
// может не вернуться сразу (раннер без звуковой карты, смена звука), а поток окон ждать не должен,
// иначе плашка 1 мин не сменит табло (CI 40751015930). Его отказ показ плашки не трогает. В
// тестовом режиме звук пишется в журнал сразу, ответ Windows (OK) дописывается, когда придёт.
func (c *Controller) playDue(now time.Time, w *WarningView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !SoundDue(c.soundID, w) {
		return
	}
	c.soundID = w.ID
	if c.play == nil {
		return
	}
	idx := -1
	if c.testMode {
		c.sounds = append(c.sounds, ipc.ShownSound{AtMs: now.UnixMilli(), Threshold: w.Threshold})
		if len(c.sounds) > maxSoundLog {
			c.sounds = c.sounds[len(c.sounds)-maxSoundLog:]
		}
		idx = len(c.sounds) - 1
	}
	if c.soundQ == nil {
		c.soundQ = make(chan int, 8)
		go c.soundLoop(c.play, c.soundQ)
	}
	select {
	case c.soundQ <- idx:
	default: // очередь полна: звуки идут чаще, чем Windows их принимает; лишний пропускается
	}
}

// soundLoop играет звуки по одному (PlaySoundW не любит одновременных вызовов) и дописывает
// ответ Windows в журнал. Записи журнала могли сдвинуться при обрезке до maxSoundLog: тогда
// ответ теряется, а не попадает к чужой записи.
func (c *Controller) soundLoop(play func() bool, q <-chan int) {
	for idx := range q {
		ok := false
		safely(func() { ok = play() })
		c.mu.Lock()
		if idx >= 0 && idx < len(c.sounds) && len(c.sounds) < maxSoundLog {
			c.sounds[idx].OK = ok
		}
		c.mu.Unlock()
		c.wake()
	}
}

// Notice возвращает короткий ответ под заголовком экрана («Не удалось выполнить…»); пусто,
// если ответа нет или он устарел.
func (c *Controller) Notice() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.notice == "" || !c.now().Before(c.noticeEnd) {
		c.notice = ""
		return ""
	}
	return c.notice
}

// fail показывает отказ команды: у кнопки плашки — плашкой (текст службы или «Не получилось.
// Попробуй ещё раз», ребёнку на «ты»), у экрана блокировки — под заголовком экрана.
func (c *Controller) fail(act Action, err error, fallback string) {
	if !act.Plate {
		c.setNotice(failureText(err, fallback))
		return
	}
	text := textPlateFailed
	var re *ipc.ReplyError
	if errors.As(err, &re) && re.Text != "" {
		text = re.Text
	}
	c.model.OnPlateNotice(c.now(), text)
}

func (c *Controller) setNotice(text string) {
	c.mu.Lock()
	c.notice, c.noticeEnd = text, c.now().Add(noticeLife)
	c.mu.Unlock()
}

// actsOnService: нажатие этой кнопки может отправить команду службе. Пока команда идёт,
// такие нажатия игнорируются: пароль не проверится дважды, время не добавится дважды.
func actsOnService(b Button) bool {
	switch b {
	case ButtonSubmitPassword, ButtonAdd15, ButtonAdd30, ButtonAdd60, ButtonClearBlock,
		ButtonOpenSettings, ButtonConfirmLogoff, ButtonConfirmShutdown, ButtonSwitchUser,
		ButtonSpendBonus, ButtonSubmitPromo, ButtonAskNow, ButtonPlateSpendBonus, ButtonPlateAskMore:
		return true
	}
	return false
}

// Press передаёт нажатие кнопки модели и записывает событие. Возвращённое действие надо
// выполнить через Run (в отдельной горутине: ответ службы может прийти не сразу), даже если оно
// пустое. password — текст поля: пароль для ButtonSubmitPassword или промокод для
// ButtonSubmitPromo; в события не попадает.
func (c *Controller) Press(b Button, password string) Action {
	c.mu.Lock()
	if c.busy && actsOnService(b) {
		c.mu.Unlock()
		return Action{}
	}
	c.mu.Unlock()
	now := c.now()
	// Подтверждение крестиком — только у табло, видимого в момент нажатия (не у плашки о
	// программе поверх него и не у закрытого раньше).
	ackThr := 0
	if w := c.model.View(now).Warning; b == ButtonWarningClose && w != nil && w.Board != nil {
		ackThr = w.Threshold
	}
	act := c.model.Press(now, b, password)
	c.mu.Lock()
	defer c.mu.Unlock()
	if name := TargetName(b); name != "" {
		c.addEventLocked(ipc.GuardEvent{Kind: ipc.GuardEvButton, AtMs: now.UnixMilli(), Button: name})
	}
	if ackThr != 0 {
		c.addEventLocked(ipc.GuardEvent{Kind: ipc.GuardEvWarningAck, AtMs: now.UnixMilli(), Threshold: ackThr})
	}
	if act.Kind != ActNone {
		c.busy = true
	}
	return act
}

// Run выполняет действие, которое вернул Press: команда службе и разбор ответа. Блокирует до
// ответа (до 10 с без связи), поэтому зовётся не из потока окна.
func (c *Controller) Run(act Action) {
	if act.Kind == ActNone {
		return
	}
	defer func() {
		c.mu.Lock()
		c.busy = false
		c.mu.Unlock()
		c.Kick()
		c.wake()
	}()
	switch act.Kind {
	case ActCheckPassword:
		err := ipc.Call(c.call, ipc.CmdLogin, "", ipc.LoginRequest{Password: act.Password}, nil)
		now := c.now()
		out := LoginOutcomeOf(err, now)
		if out.Decided {
			c.model.OnPasswordResult(now, out.Result)
		}
		if out.Notice != "" {
			c.setNotice(out.Notice)
		}
	case ActAddExtra:
		if err := ipc.Call(c.call, ipc.CmdAddExtra, "", ipc.AddExtraRequest{Minutes: act.Minutes}, nil); err != nil {
			c.setNotice(failureText(err, textNoticeRetry))
			return
		}
		c.model.OnExtraAdded(c.now(), act.Minutes)
	case ActSpendBonus:
		// Трата из копилки без пароля (FR-11); childId 0 — служба берёт ребёнка этого сеанса.
		var rep ipc.SpendBonusReply
		if err := ipc.Call(c.call, ipc.CmdSpendBonus, "", ipc.SpendBonusRequest{Minutes: act.Minutes}, &rep); err != nil {
			c.fail(act, err, textNoticeRetry)
			return
		}
		c.model.OnExtraAdded(c.now(), act.Minutes)
	case ActOpenPrograms:
		// «Попросить сейчас» (RM3-4): окно родителя на «Программах», пароль спросит окно. Токена нет.
		req := ipc.OpenWindowRequest{Kind: ipc.WindowParent, Screen: ipc.ScreenPrograms}
		if err := ipc.Call(c.call, ipc.CmdOpenWindow, "", req, nil); err != nil {
			c.fail(act, err, textPlateFailed)
		}
	case ActOpenMyStats:
		// «Попросить ещё» на плашке 1 мин: как значок, «Моя статистика» (RM3-5).
		if err := ipc.Call(c.call, ipc.CmdOpenWindow, "", ipc.OpenWindowRequest{Kind: ipc.WindowMyStats}, nil); err != nil {
			c.fail(act, err, textPlateFailed)
		}
	case ActRedeemPromo:
		// Промокод без пароля (R26); childId 0 — служба берёт ребёнка этого сеанса.
		var rep ipc.RedeemPromoReply
		err := ipc.Call(c.call, ipc.CmdRedeemPromo, "", ipc.RedeemPromoRequest{Code: act.Code}, &rep)
		var re *ipc.ReplyError
		switch {
		case err == nil:
			c.model.OnPromoResult(c.now(), PromoResult{OK: true, Minutes: rep.AddedMin})
			c.mu.Lock()
			forbidden := c.lastLock == ipc.LockForbidden
			c.mu.Unlock()
			if forbidden {
				// Из запретного интервала промокод не выводит (ТЗ 8.2.3 п.3): экран остаётся,
				// и ребёнок должен понять, что код сработал.
				pron := "Их"
				if n := rep.AddedMin; n%10 == 1 && n%100 != 11 {
					pron = "Её" // «Добавлена 1 минута. Её можно…»
				}
				c.setNotice(addedText(rep.AddedMin) + ". " + pron + " можно будет потратить, когда закончатся запретные часы")
			}
		case errors.As(err, &re) && re.Text != "":
			// Отказ службы («Код не подходит») — у поля, панель остаётся.
			c.model.OnPromoResult(c.now(), PromoResult{Error: re.Text})
		default:
			c.setNotice(failureText(err, textNoticeRetry))
		}
	case ActClearBlock:
		if err := ipc.Call(c.call, ipc.CmdClearBlock, "", ipc.ChildRequest{}, nil); err != nil {
			c.setNotice(failureText(err, textNoticeRetry))
		}
	case ActOpenSettings:
		if err := ipc.Call(c.call, ipc.CmdOpenWindow, "", ipc.OpenWindowRequest{Kind: ipc.WindowParent}, nil); err != nil {
			c.setNotice(failureText(err, textNoticeRetry))
		}
	case ActLogoff:
		if err := ipc.Call(c.call, ipc.CmdGuardLogoff, "", nil, nil); err != nil {
			c.setNotice(failureText(err, textNoticeLogoff))
		}
	case ActShutdown:
		if err := ipc.Call(c.call, ipc.CmdGuardShutdown, "", nil, nil); err != nil {
			c.setNotice(failureText(err, textNoticeShutdown))
		}
	case ActSwitchUser:
		if err := ipc.Call(c.call, ipc.CmdGuardSwitchUser, "", nil, nil); err != nil {
			c.setNotice(failureText(err, textNoticeSwitchUser))
		}
	}
}

// NotifyClosing сообщает службе, что страж закрывается вместе с сеансом Windows (выход из учётной
// записи, выключение, перезагрузка, смена пользователя): GuardClosing. Это не обход защиты
// (R17, FR-27): без сообщения служба записала бы в журнал «Защита программы была остановлена».
// Сеанс не должен ждать службу: вызов идёт в отдельной горутине, и ждёт его NotifyClosing не
// дольше wait; ошибка канала, отказ службы, паника и опоздавший ответ не важны. Возвращает true,
// если служба приняла сообщение вовремя.
func (c *Controller) NotifyClosing(wait time.Duration) bool {
	// Конец сеанса сообщает о себе дважды (WM_QUERYENDSESSION и WM_ENDSESSION), а ждать службу
	// до wait в потоке окон каждый раз не нужно: исход первой попытки, удачный или нет, остаётся
	// на весь этот конец сеанса. Блокировка держится на всё ожидание: второй вызов ждёт первый.
	c.closingMu.Lock()
	defer c.closingMu.Unlock()
	if c.closingDone {
		return c.closingOK
	}
	c.closingDone = true
	c.closingOK = c.sendClosing(wait)
	return c.closingOK
}

// ResetClosing разрешает уведомить службу снова: конец сеанса отменён (WM_ENDSESSION с wParam=0).
func (c *Controller) ResetClosing() {
	c.closingMu.Lock()
	c.closingDone = false
	c.closingMu.Unlock()
}

func (c *Controller) sendClosing(wait time.Duration) bool {
	done := make(chan bool, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- false
			}
		}()
		done <- ipc.Call(c.call, ipc.CmdGuardClosing, "", nil, nil) == nil
	}()
	select {
	case ok := <-done:
		return ok
	case <-time.After(wait):
		return false
	}
}

// AutoSwitch проверяет срок автоматической смены пользователя (ТЗ 8.2.3 п.10) и возвращает
// действие, которое надо выполнить через Run, как после Press. Зовёт горутина опроса после
// каждого PollOnce. Пока идёт другая команда стража (проверка пароля родителя), срок не
// расходуется: действие придёт со следующим опросом после её конца.
func (c *Controller) AutoSwitch() Action {
	c.mu.Lock()
	busy := c.busy
	c.mu.Unlock()
	if busy {
		return Action{}
	}
	act := c.model.AutoSwitch(c.now())
	if act.Kind != ActNone {
		c.mu.Lock()
		c.busy = true
		c.mu.Unlock()
	}
	return act
}

// QueuedTests отдаёт накопленные действия TestGuard (пусто вне тестового режима).
func (c *Controller) QueuedTests() []ipc.TestGuardAction {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.tests
	c.tests = nil
	return out
}

// SetField запоминает текст поля для GuardState; только в тестовом режиме, иначе то, что
// набрано в поле пароля, нигде не хранится.
func (c *Controller) SetField(id, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.testMode {
		return
	}
	c.fields[id] = text
}

// StateFromView собирает ответ GuardState: что страж показывает сейчас. Все срезы не nil.
func StateFromView(v View, lockKind string, shown []ipc.ShownWarning, sounds []ipc.ShownSound, fields map[string]string) ipc.GuardStateReply {
	st := ipc.GuardStateReply{
		Warnings:  append([]ipc.ShownWarning{}, shown...),
		Sounds:    append([]ipc.ShownSound{}, sounds...),
		LockShown: v.LockShown, LockKind: ipc.LockNone,
		Buttons: []string{}, Fields: []ipc.GuardField{}, Exits: []string{},
	}
	if v.LockShown && lockKind != "" {
		st.LockKind = lockKind
	}
	add := func(list []ButtonView) {
		for _, b := range list {
			if name := TargetName(b.Button); name != "" {
				st.Buttons = append(st.Buttons, name)
			}
		}
	}
	add(v.Buttons)
	add(v.Choices)
	if v.Warning != nil {
		add(v.Warning.Buttons)
		st.Plate, st.PlateKind = v.Warning.Text, v.Warning.Kind
	}
	switch v.Panel {
	case PanelPassword:
		st.Fields = append(st.Fields, ipc.GuardField{ID: FieldPasswordID, Text: fields[FieldPasswordID]})
	case PanelPromo:
		st.Fields = append(st.Fields, ipc.GuardField{ID: FieldPromoID, Text: fields[FieldPromoID]})
	}
	return st
}

// PushState в тестовом режиме отправляет службе вид стража (GuardState), если он изменился
// с прошлой отправки. Зовёт горутина опроса после PollOnce.
func (c *Controller) PushState() {
	c.mu.Lock()
	if !c.testMode {
		c.mu.Unlock()
		return
	}
	st := StateFromView(c.last, c.lastLock, c.shown, c.sounds, c.fields)
	st.UI = c.ui
	st.ForegroundPath = c.lastFg
	c.mu.Unlock()
	if p, ok := lastPanic.Load().(string); ok {
		st.UI.Panic = p
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return
	}
	c.mu.Lock()
	same := string(raw) == string(c.sentState)
	c.mu.Unlock()
	if same {
		return
	}
	if err := ipc.Call(c.call, ipc.CmdGuardState, "", st, nil); err != nil {
		return
	}
	c.mu.Lock()
	c.sentState = raw
	c.mu.Unlock()
}
