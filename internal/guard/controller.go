package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	sounds    []ipc.ShownSound  // сыгранные звуки (журнал для GuardState, только тестовый режим)
	soundID   int               // ID плашки, для которой звук уже сыгран
	noteSound int               // номер показа сообщения с телефона, для которого звук уже сыгран (FR-41)
	playWarn  func() bool       // проиграть звук предупреждения; true — Windows принял звук
	playMsg   func(int) bool    // проиграть встроенный звук сообщения с номером; true — Windows принял звук
	playSys   func(string) bool // проиграть звук схемы Windows и дождаться конца (SND_SYNC, R-7); true — принят
	warm      func(string)      // заранее разобрать кадры стикера промокода (не в потоке окон)
	soundQ    chan soundItem    // очередь исполнителя звука (одна на все виды); nil — исполнитель не запущен
	speed     int               // ускорение часов ядра, сообщённое окном; модели передаётся только в тестовом режиме
	ui        ipc.GuardUI       // диагностика потока окон
	seenPlate int               // наибольший ID плашки, о которой уже известно (ID растут; вернувшаяся плашка не новая)
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
	// lastDomain — домен вкладки Brave из последнего отправленного снимка (GuardState).
	lastDomain string
	// domSet/domText — подмена домена вкладки Brave действием TestDomain (только тестовый режим,
	// RM4-12): в снимке — если путь переднего плана (после подмены foreground) brave.exe, при
	// исполнении closeTab — если настоящий передний план Brave.
	domSet  bool
	domText string
	// tabActor — исполнитель закрытия вкладки и окна Brave (SetTabActor; nil — не исполнять);
	// tabs — лестница закрытия (nextClose); closedTab/closedWay — последняя исполненная команда
	// (GuardState.ClosedTab, ClosedTabWay).
	tabActor  TabActor
	tabs      tabLadder
	closedTab string
	closedWay string
	// noteSticker/noteFrame — стикер и кадр, которые окно сообщения вывело последним (SetNoteShown;
	// GuardState.NoteSticker, NoteFrame; только тестовый режим).
	noteSticker string
	noteFrame   int
	// noteFontPx/noteTextFits — шрифт текста окна сообщения в пикселях экрана и признак «текст виден
	// целиком» по последней раскладке (SetNoteFit; GuardState.NoteFontPx, NoteTextFits; только тестовый режим).
	noteFontPx   int
	noteTextFits bool
	// plaqueSound — ID плашки «Игрового лобби», для которой звук схемы уже поставлен (D-111: один раз на
	// плашку); plaqueSticker/plaqueFrame — стикер и кадр, выведенные окном плашки последними
	// (SetPlaqueShown; GuardState.PlateSticker, PlateFrame; только тестовый режим).
	plaqueSound   int
	plaqueSticker string
	plaqueFrame   int

	// lenWarn/lenMsg - сколько звучит звук предупреждения и звук сообщения с номером (из шапки wav):
	// после принятого звука исполнитель ждёт его конца (nil или 0 - не ждёт). soundTimer - ожидание
	// (nil - time.NewTimer; тест подменяет). soundStop закрывает StopSounds; soundExit закрывает
	// исполнитель, выйдя.
	lenWarn    func() time.Duration
	lenMsg     func(int) time.Duration
	soundTimer func(time.Duration) (<-chan time.Time, func() bool)
	soundStop  chan struct{}
	stopOnce   sync.Once
	soundExit  chan struct{}

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
		kick:      make(chan struct{}, 1),
		fields:    map[string]string{},
		soundStop: make(chan struct{}),
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

// SetSoundPlayers задаёт, чем играть звук предупреждения (окно: sound.PlayWarning), звук сообщения
// с телефона по номеру (sound.PlayMessage) и звук схемы Windows плашки «Игрового лобби»
// (sound.PlaySystem: ждёт конца звука, R-7). Без своего плеера звук этого вида не играет и не
// пишется в журнал. Вызывать до первого View.
func (c *Controller) SetSoundPlayers(warning func() bool, message func(n int) bool, system func(alias string) bool) {
	c.mu.Lock()
	c.playWarn, c.playMsg, c.playSys = warning, message, system
	c.mu.Unlock()
}

// SetSoundLengths задаёт, сколько звучит звук предупреждения (окно: sound.WarningLength) и звук
// сообщения с номером (sound.MessageLength). PlaySoundW с SND_ASYNC возвращается сразу, а следующий
// вызов обрывает текущий звук: исполнитель после принятого звука ждёт его длину и только потом играет
// следующий (ревью M2 0.8.3). Без длины (nil или 0) не ждёт. Вызывать до первого View.
func (c *Controller) SetSoundLengths(warning func() time.Duration, message func(n int) time.Duration) {
	c.mu.Lock()
	c.lenWarn, c.lenMsg = warning, message
	c.mu.Unlock()
}

// StopSounds останавливает исполнителя звуков (конец работы стража): он выходит сразу, не дожидаясь
// конца звука; звук, который уже играет Windows, доигрывает сам. Не ждёт выхода исполнителя: плеер
// может не вернуться (раннер без звуковой карты). Повторный вызов ничего не делает.
func (c *Controller) StopSounds() {
	c.stopOnce.Do(func() {
		if c.soundStop != nil {
			close(c.soundStop)
		}
	})
}

// SetStickerWarmer задаёт, как заранее готовить кадры стикера промокода: Run зовёт его в своей
// горутине сразу после ответа службы, и поток окон берёт готовые кадры из кэша.
func (c *Controller) SetStickerWarmer(warm func(sticker string)) {
	c.mu.Lock()
	c.warm = warm
	c.mu.Unlock()
}

// warmSticker готовит кадры показанного стикера, если он есть, и начинает показ заново с первого
// кадра: разбор до 90 кадров занимает долю секунды, круг стикера должен пройти целиком.
func (c *Controller) warmSticker() {
	c.mu.Lock()
	warm := c.warm
	c.mu.Unlock()
	if p := c.model.View(c.now()).Promo; p != nil && warm != nil {
		safely(func() { warm(p.Sticker) })
		c.model.PromoReady(c.now())
	}
}

// SetTabActor задаёт исполнителя закрытия вкладки и окна Brave (RM4-2): страж на Windows —
// winTabs. Без него команды closeTab и closeBraveWindow не исполняются. Вызывать до первого
// PollOnce.
func (c *Controller) SetTabActor(a TabActor) {
	c.mu.Lock()
	c.tabActor = a
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
	Notices       []string              // другие сообщения службы («Пауза закончилась»): плашкой как есть
	Blocked       []ipc.BlockedPlate    // плашки о закрытых программах (FR-13), по порядку
	Tests         []ipc.TestGuardAction // действия TestGuard (только тестовый режим)
	CloseTab      string                // домен последней команды closeTab (RM4-2); «» — нет
	CloseWindow   bool                  // команда closeBraveWindow: окно Brave без адреса (RM4-1)
	PhoneNotes    []ipc.GuardNote       // сообщения с телефона (FR-41): своя плашка, не notice (PL-81)
}

// ClassifyCommands раскладывает команды службы; незнакомые виды пропускаются.
func ClassifyCommands(cmds []ipc.GuardCommand) Commands {
	var out Commands
	for _, c := range cmds {
		switch c.Kind {
		case ipc.GuardCmdHideLock:
			out.HideLock = true
		case ipc.GuardCmdNotice:
			// Число берётся только из «Добавлено N минут»: у «Пауза 1 ч» и «Убрано 15 минут» оно
			// значит другое (FB-25), такие сообщения показываются текстом службы.
			if n, ok := NoticeMinutes(c.Text); ok && strings.HasPrefix(c.Text, "Добавлен") {
				out.NoticeMinutes = append(out.NoticeMinutes, n)
			} else if t := strings.TrimSpace(c.Text); t != "" {
				out.Notices = append(out.Notices, t)
			}
		case ipc.GuardCmdTest:
			if c.Test != nil {
				out.Tests = append(out.Tests, *c.Test)
			}
		case ipc.GuardCmdBlocked:
			if c.Plate != nil {
				out.Blocked = append(out.Blocked, *c.Plate)
			}
		case ipc.GuardCmdCloseTab:
			if c.Domain != "" {
				out.CloseTab = c.Domain
			}
		case ipc.GuardCmdCloseBraveWindow:
			out.CloseWindow = true
		case ipc.GuardCmdNote:
			if c.Note != nil {
				out.PhoneNotes = append(out.PhoneNotes, *c.Note)
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
		VideoRemainingS: s.VideoRemainingS, VideoLimitS: s.VideoLimitS, VideoUnlimited: s.VideoUnlimited,
		CloseVideo:  s.CloseVideo,
		RestUntilMs: s.RestUntilMs, RestLeftS: s.RestLeftS, RestMessage: s.RestMessage,
	}
	switch s.Lock {
	case ipc.LockTimeUp:
		d.Lock = core.LockTimeUp
	case ipc.LockForbidden:
		d.Lock = core.LockForbidden
	case ipc.LockBypass:
		d.Lock = core.LockBypass
	case ipc.LockRest:
		d.Lock = core.LockRest
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
	if c.domSet && c.testMode && isBravePath(req.Sample.ForegroundPath) {
		req.Sample.ForegroundDomain, req.Sample.DomainUnread, req.Sample.DomainNoAddress = c.domText, false, false
	}
	c.lastDomain = req.Sample.ForegroundDomain
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
	for _, text := range cmds.Notices {
		c.model.OnPlateNotice(now, text)
	}
	for _, pl := range cmds.Blocked {
		c.model.OnBlocked(now, pl)
	}
	for _, n := range cmds.PhoneNotes {
		c.model.OnNote(now, n)
	}
	v := c.model.View(now)
	// Журнал показанного, звук и вид для GuardState берутся из одного кадра: горутина опроса
	// отправляет GuardState сразу после PollOnce, раньше, чем поток окон перерисуется по wake.
	// Иначе снимок несогласован: порог 1 уже в журнале, а кнопки и звуки от табло «5» (CI S2, S35).
	c.playDue(now, v.Warning)
	c.playPlaque(now, v.Warning)
	c.playNote(now, v.Note)
	noteEvs := c.model.TakeNoteEvents()

	c.mu.Lock()
	for _, e := range noteEvs {
		c.addEventLocked(e)
	}
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
		case t.Action == ipc.TestDomain:
			// Подмена домена — как подмена переднего плана: со следующего снимка; текст как есть.
			c.domSet, c.domText = t.Text != "", t.Text
		default:
			c.tests = append(c.tests, t)
		}
	}
	c.mu.Unlock()
	c.wake()
	c.runCloses(cmds, now)
	return PollResult{Commands: cmds}
}

// runCloses исполняет closeTab и closeBraveWindow из ответа службы (RM4-2, RM4-1) в горутине
// опроса, после модели и wake (плашка уже видна): перечитывает настоящий передний план (Look),
// решает по nextClose и закрывает. Подмена TestForeground к исполнению не применяется — Ctrl+W
// и WM_CLOSE уходят только настоящему окну Brave; подмена TestDomain — только если оно Brave.
// Ответ без обеих команд сбрасывает лестницу. Look и Close — без c.mu (UIA ждёт до 2 с).
func (c *Controller) runCloses(cmds Commands, now time.Time) {
	c.mu.Lock()
	actor := c.tabActor
	if cmds.CloseTab == "" && !cmds.CloseWindow {
		c.tabs = tabLadder{}
	}
	ladder := c.tabs
	domSet, domText, test := c.domSet, c.domText, c.testMode
	c.mu.Unlock()
	if actor == nil || (cmds.CloseTab == "" && !cmds.CloseWindow) {
		return
	}
	var look TabLook
	safely(func() { look = actor.Look() })
	if domSet && test && look.Brave {
		look.Domain, look.Unread, look.NoAddress = domText, false, false
	}
	way, next := nextClose(ladder, cmds.CloseTab, cmds.CloseWindow, look, now)
	if way == "" {
		return
	}
	err := errors.New("исполнитель упал")
	safely(func() { err = actor.Close(way, look.Window) })
	if errors.Is(err, errModifierHeld) || errors.Is(err, errForegroundChanged) {
		return // ничего не послано, шаг не засчитан: следующая команда службы попробует снова
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tabs = next
	if err != nil {
		return
	}
	c.closedTab, c.closedWay = next.domain, way
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
	c.playPlaque(now, v.Warning)
	c.playNote(now, v.Note)
	noteEvs := c.model.TakeNoteEvents()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range noteEvs {
		c.addEventLocked(e)
	}
	if len(noteEvs) > 0 {
		c.Kick() // «показано» и «Понял» — службе сразу, телефон ждёт отметку
	}
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
	c.enqueueSoundLocked(ipc.ShownSound{AtMs: now.UnixMilli(), Threshold: w.Threshold}, soundItem{kind: soundWarning})
}

// playNote играет выбранный родителем звук один раз на каждый показ плашки сообщения с телефона
// (FR-41): тем же исполнителем и той же очередью, что звук предупреждений. «Без звука» — ни одного
// вызова плеера.
func (c *Controller) playNote(now time.Time, n *NoteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !NoteSoundDue(c.noteSound, n) {
		return
	}
	c.noteSound = n.Seq
	c.enqueueSoundLocked(ipc.ShownSound{AtMs: now.UnixMilli(), Note: true, NoteSound: n.Sound}, soundItem{kind: soundNote, n: n.Sound})
}

// playPlaque ставит звук схемы Windows новой плашки «Игрового лобби» (ТЗ 8.2.4, D-111): один раз на
// плашку — та же плашка, повторённая службой, звука не даёт (класс A); сбой кнопки и предупреждения о
// времени — не «Лобби».
func (c *Controller) playPlaque(now time.Time, w *WarningView) {
	if w == nil || w.Sticker == "" {
		return
	}
	st, ok := PlaqueStyleOf(w.Kind, w.Allowed)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !ok || w.ID == c.plaqueSound {
		return
	}
	c.plaqueSound = w.ID
	c.enqueueSoundLocked(ipc.ShownSound{AtMs: now.UnixMilli(), System: st.Sound}, soundItem{kind: soundSystem, alias: st.Sound})
}

// playSystem ставит в общую очередь звук схемы Windows alias (плашка «Игрового лобби», ТЗ 8.2.4,
// 8.2.5): тот же исполнитель, что у предупреждений и сообщений, по одному звуку за раз.
func (c *Controller) playSystem(now time.Time, alias string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enqueueSoundLocked(ipc.ShownSound{AtMs: now.UnixMilli(), System: alias}, soundItem{kind: soundSystem, alias: alias})
}

// soundKind — вид звука в очереди исполнителя.
type soundKind int

const (
	soundWarning soundKind = iota // звук предупреждения о времени (D-55)
	soundNote                     // звук сообщения с телефона с номером (FR-41)
	soundSystem                   // звук схемы Windows плашки «Игрового лобби» (R-7)
)

// soundItem — звук в очереди исполнителя: вид, номер звука сообщения или имя звука схемы и запись
// журнала звуков, куда дописать ответ Windows.
type soundItem struct {
	kind  soundKind
	n     int    // номер звука сообщения
	alias string // имя звука схемы Windows
	idx   int    // запись журнала звуков (тестовый режим); -1 — не пишется
}

// enqueueSoundLocked отдаёт звук исполнителю (под c.mu); в тестовом режиме пишет его в журнал звуков.
// Плеер проверяется для своего вида: без плеера сообщений звук предупреждения всё равно играет.
func (c *Controller) enqueueSoundLocked(rec ipc.ShownSound, it soundItem) {
	switch {
	case it.kind == soundNote && c.playMsg == nil,
		it.kind == soundWarning && c.playWarn == nil,
		it.kind == soundSystem && (c.playSys == nil || it.alias == ""):
		return
	}
	idx := -1
	if c.testMode {
		c.sounds = append(c.sounds, rec)
		if len(c.sounds) > maxSoundLog {
			c.sounds = c.sounds[len(c.sounds)-maxSoundLog:]
		}
		idx = len(c.sounds) - 1
	}
	it.idx = idx
	if c.soundQ == nil {
		c.soundQ = make(chan soundItem, 8)
		c.soundExit = make(chan struct{})
		go c.soundLoop(c.soundQ, c.soundStop, c.soundExit)
	}
	select {
	case c.soundQ <- it:
	default: // очередь полна: звуки идут чаще, чем Windows их принимает; лишний пропускается
	}
}

// soundLoop - единственный исполнитель звуков всех видов: играет их по одному и дописывает ответ
// Windows в журнал. PlaySoundW с SND_ASYNC возвращается сразу, а следующий вызов обрывает текущий
// звук, поэтому после принятого звука исполнитель ждёт его длину (SetSoundLengths) и только потом
// берёт следующий из очереди. Звук схемы Windows играет синхронно (R-7): плеер возвращается после
// конца звука, ждать дальше нечего. stop (StopSounds) прерывает и ожидание: выход стража конца звука
// не ждёт. Записи журнала могли сдвинуться при обрезке до maxSoundLog: тогда ответ теряется, а не
// попадает к чужой записи.
func (c *Controller) soundLoop(q <-chan soundItem, stop <-chan struct{}, exit chan<- struct{}) {
	defer close(exit)
	for {
		var it soundItem
		select {
		case <-stop:
			return
		case it = <-q:
		}
		c.mu.Lock()
		warn, msg, sys, lenWarn, lenMsg, timer := c.playWarn, c.playMsg, c.playSys, c.lenWarn, c.lenMsg, c.soundTimer
		c.mu.Unlock()
		ok := false
		var length time.Duration
		safely(func() {
			switch {
			case it.kind == soundNote && msg != nil:
				if ok = msg(it.n); ok && lenMsg != nil {
					length = lenMsg(it.n)
				}
			case it.kind == soundWarning && warn != nil:
				if ok = warn(); ok && lenWarn != nil {
					length = lenWarn()
				}
			case it.kind == soundSystem && sys != nil:
				ok = sys(it.alias) // синхронно: уже доиграл
			}
		})
		c.mu.Lock()
		if idx := it.idx; idx >= 0 && idx < len(c.sounds) && len(c.sounds) < maxSoundLog {
			c.sounds[idx].OK = ok
		}
		c.mu.Unlock()
		c.wake()
		if length <= 0 {
			continue
		}
		if timer == nil {
			timer = func(d time.Duration) (<-chan time.Time, func() bool) {
				t := time.NewTimer(d)
				return t.C, t.Stop
			}
		}
		done, cancel := timer(length)
		select {
		case <-done:
		case <-stop:
			cancel()
			return
		}
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
		if b == ButtonSubmitPromo {
			c.model.OnFieldEdit(c.now()) // нажатие есть, хоть и без действия: срок без нажатий идёт заново
		}
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
		req := ipc.OpenWindowRequest{Kind: ipc.WindowParent, Screen: ipc.ScreenPrograms, Path: act.Path}
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
			c.warmSticker()
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
			// Отказ службы: по полю кода («Код не подходит») — стикер и надпись неудачи над полем
			// (RM5b-10), другой — текст у поля; панель остаётся.
			c.model.OnPromoResult(c.now(), PromoResult{Error: re.Text, BadCode: re.Field == ipc.FieldCode})
			c.warmSticker()
		default:
			c.setNotice(failureText(err, textNoticeRetry))
		}
	case ActClearBlock:
		if err := ipc.Call(c.call, ipc.CmdClearBlock, "", ipc.ChildRequest{}, nil); err != nil {
			c.setNotice(failureText(err, textNoticeRetry))
		}
	case ActEndBreak:
		// «Закончить перерыв» после пароля (R27): служба пишет журнал, экран снимет следующий опрос.
		if err := ipc.Call(c.call, ipc.CmdEndBreak, "", ipc.ChildRequest{}, nil); err != nil {
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

// SetNoteShown запоминает, что окно сообщения вывело сейчас: номер стикера ("" — без стикера) и кадр
// (GuardState.NoteSticker, NoteFrame). Только в тестовом режиме, как журнал звуков; зовёт поток окон
// на каждом выведенном кадре и при скрытии окна.
func (c *Controller) SetNoteShown(sticker string, frame int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.testMode {
		return
	}
	c.noteSticker, c.noteFrame = sticker, frame
}

// SetNoteFit запоминает, как окно сообщения разложило текст: размер шрифта в пикселях экрана (0 — окна
// нет) и поместился ли текст целиком (NoteTextFits). GuardState.NoteFontPx, NoteTextFits (S57). Только в
// тестовом режиме; зовёт поток окон после раскладки и при скрытии окна.
func (c *Controller) SetNoteFit(fontPx int, fits bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.testMode {
		return
	}
	c.noteFontPx, c.noteTextFits = fontPx, fits
}

// SetPlaqueShown запоминает, что окно плашки «Игрового лобби» вывело сейчас: стикер ("" — нет) и кадр
// (GuardState.PlateSticker, PlateFrame, S65). Только в тестовом режиме; зовёт поток окон.
func (c *Controller) SetPlaqueShown(sticker string, frame int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.testMode {
		return
	}
	c.plaqueSticker, c.plaqueFrame = sticker, frame
}

// SetAnimations передаёт модели, включены ли анимации Windows (стикер промокода без движения, R26).
func (c *Controller) SetAnimations(on bool) { c.model.SetAnimations(on) }

// FieldEdited: ребёнок изменил текст в поле экрана блокировки (EN_CHANGE). На панели промокода
// срок без нажатий идёт заново; ответ на прошлый неверный код остаётся (R26, D-85).
func (c *Controller) FieldEdited() {
	c.model.OnFieldEdit(c.now())
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
	if v.Rest != nil {
		st.RestTimer, st.RestMessage = timerText(v.Rest), v.Rest.Message
	}
	if v.Warning != nil {
		add(v.Warning.Buttons)
		st.Plate, st.PlateKind, st.PlateTitle = v.Warning.Text, v.Warning.Kind, v.Warning.Title
	}
	if v.Promo != nil {
		st.PromoSticker, st.PromoText = v.Promo.Sticker, v.Promo.Text
	}
	if v.Note != nil {
		add(v.Note.Buttons)
		st.Note, st.NoteID = v.Note.Text, v.Note.ID
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
	st.ForegroundDomain = c.lastDomain
	st.ClosedTab, st.ClosedTabWay = c.closedTab, c.closedWay
	if c.last.Note != nil { // окно сообщения могло ещё не узнать, что сообщение закрыто
		st.NoteSticker, st.NoteFrame = c.noteSticker, c.noteFrame
		st.NoteFontPx, st.NoteTextFits = c.noteFontPx, c.noteTextFits
	}
	if c.last.Warning != nil && c.last.Warning.Sticker != "" { // окно плашки могло ещё не узнать, что она закрыта
		st.PlateSticker, st.PlateFrame = c.plaqueSticker, c.plaqueFrame
	}
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
