package service

import (
	"fmt"
	"time"

	"pcblock/internal/ipc"
)

// clientKind — вид клиента канала по Peer (NFR-9, Review Focus 7). Права команды зависят от
// вида клиента, родительской сессии и тестового режима; канал сам права не проверяет.
type clientKind int

const (
	kindChild  clientKind = iota // процесс обычного пользователя, отмеченного детским
	kindUser                     // процесс другого обычного пользователя
	kindAdmin                    // администратор без повышения
	kindElev                     // администратор с повышением (UAC)
	kindGuard                    // страж: система в сеансе ребёнка
	kindSystem                   // система в другом сеансе или не в сеансе ребёнка
)

func (k clientKind) String() string {
	return [...]string{"ребёнок", "другой пользователь", "администратор", "повышенный администратор", "страж", "система"}[k]
}

// callCtx — один вызов команды.
type callCtx struct {
	s       *Service
	peer    ipc.Peer
	msg     ipc.Msg
	real    time.Time
	kind    clientKind
	parent  *parentSession // действующая родительская сессия вызывающего, если есть
	viaBind bool           // вызов разрешён привязкой стража (после верного пароля)
	session *sessState     // сеанс стража
}

// check возвращает "" если вызов разрешён, иначе готовый текст отказа.
type check func(c *callCtx) string

// classify определяет вид клиента.
func (s *Service) classify(p ipc.Peer) clientKind {
	switch {
	case p.System:
		if ss := s.sessions[p.SessionID]; ss != nil && ss.childID != 0 {
			return kindGuard
		}
		return kindSystem
	case p.Admin && p.Elevated:
		return kindElev
	case p.Admin:
		return kindAdmin
	case s.childBySID(p.SID) != nil:
		return kindChild
	}
	return kindUser
}

// --- проверки ---------------------------------------------------------------------------

func anyone() check { return func(*callCtx) string { return "" } }

// kinds разрешает вызов указанным видам клиента.
func kinds(ks ...clientKind) check {
	return func(c *callCtx) string {
		for _, k := range ks {
			if c.kind == k {
				return ""
			}
		}
		return textDenyKind
	}
}

// users: любой процесс пользователя (не система).
func users() check { return kinds(kindChild, kindUser, kindAdmin, kindElev) }

// elevated: только администратор с повышением (R20).
func elevated() check {
	return func(c *callCtx) string {
		if c.kind == kindElev {
			return ""
		}
		return textNeedAdmin
	}
}

// admins: администратор, с повышением или без (тестовые команды: режим сам включает администратор).
func admins() check { return kinds(kindAdmin, kindElev) }

func notConfigured() check {
	return func(c *callCtx) string {
		if c.s.configured() {
			return textAlreadySetup
		}
		return ""
	}
}

// parentToken: нужна действующая родительская сессия вызывающего процесса. touch = true:
// действие продлевает сессию.
func parentToken(touch bool) check {
	return func(c *callCtx) string {
		if c.kind == kindGuard || c.kind == kindSystem {
			return textNeedParent
		}
		if c.msg.Token == "" {
			return textNeedParent
		}
		c.parent = c.s.parentFor(c.peer, c.msg.Token, c.real, touch)
		if c.parent == nil {
			return textSessionEnded
		}
		return ""
	}
}

// bound: страж, только что проведший верную проверку пароля (одно действие, 30 с).
func bound() check {
	return func(c *callCtx) string {
		if c.kind != kindGuard {
			return textDenyKind
		}
		if !c.s.guardBound(c.peer.SessionID, c.real) {
			return textNeedParent
		}
		c.viaBind = true
		c.session = c.s.sessions[c.peer.SessionID]
		return ""
	}
}

func testMode(inner check) check {
	return func(c *callCtx) string {
		if !c.s.cfg.Test.Enabled {
			return textTestOnly
		}
		return inner(c)
	}
}

func and(cs ...check) check {
	return func(c *callCtx) string {
		for _, f := range cs {
			if d := f(c); d != "" {
				return d
			}
		}
		return ""
	}
}

// or разрешает вызов, если разрешает любая проверка; иначе отказ последней из подходящих по виду.
func or(cs ...check) check {
	return func(c *callCtx) string {
		first := ""
		for _, f := range cs {
			d := f(c)
			if d == "" {
				return ""
			}
			if first == "" {
				first = d
			}
		}
		return first
	}
}

// rule — строка таблицы прав.
type rule struct {
	allow   check
	handler func(*Service, *callCtx) ipc.Reply
}

// commandRules — таблица прав на каждую команду из ipc.AllCommands. Команды без строки
// отклоняются как неразобранные, а тест следит, чтобы строка была у каждой команды канала.
var commandRules map[string]rule

func init() {
	commandRules = map[string]rule{
		ipc.CmdHello:          {anyone(), (*Service).hHello},
		ipc.CmdGetChildStatus: {users(), (*Service).hGetChildStatus},
		ipc.CmdOpenWindow:     {or(users(), bound()), (*Service).hOpenWindow},
		ipc.CmdWindowClosing:  {users(), (*Service).hWindowClosing},

		ipc.CmdListAccounts:   {or(and(elevated(), unconfiguredCheck()), parentToken(false)), (*Service).hListAccounts},
		ipc.CmdSetup:          {and(elevated(), notConfigured()), (*Service).hSetup},
		ipc.CmdResetPassword:  {elevated(), (*Service).hResetPassword},
		ipc.CmdUninstallCheck: {elevated(), (*Service).hUninstallCheck},

		ipc.CmdLogin:         {or(users(), kinds(kindGuard)), (*Service).hLogin},
		ipc.CmdExtendSession: {users(), (*Service).hExtendSession},
		ipc.CmdLogout:        {users(), (*Service).hLogout},
		ipc.CmdSessionState:  {users(), (*Service).hSessionState},

		ipc.CmdGetToday:       {parentToken(false), (*Service).hGetToday},
		ipc.CmdGetChildRules:  {parentToken(false), (*Service).hGetChildRules},
		ipc.CmdSaveLimits:     {parentToken(true), (*Service).hSaveLimits},
		ipc.CmdAddExtra:       {or(parentToken(true), bound()), (*Service).hAddExtra},
		ipc.CmdUndoExtra:      {parentToken(true), (*Service).hUndoExtra},
		ipc.CmdClearBlock:     {or(parentToken(true), bound()), (*Service).hClearBlock},
		ipc.CmdListEvents:     {parentToken(false), (*Service).hListEvents},
		ipc.CmdChangePassword: {parentToken(true), (*Service).hChangePassword},
		ipc.CmdSetChildren:    {parentToken(true), (*Service).hSetChildren},

		ipc.CmdGetHelperState:  {or(parentToken(false), users()), (*Service).hGetHelperState},
		ipc.CmdSaveHelperState: {or(parentToken(true), users()), (*Service).hSaveHelperState},

		ipc.CmdMyToday:      {users(), (*Service).hMyToday},
		ipc.CmdFirstRunSeen: {users(), (*Service).hFirstRunSeen},

		ipc.CmdGuardPoll:   {kinds(kindGuard), (*Service).hGuardPoll},
		ipc.CmdGuardLogoff: {kinds(kindGuard), (*Service).hGuardLogoff},

		ipc.CmdTrayClosing: {users(), (*Service).hTrayClosing},

		ipc.CmdTestGuard:  {testMode(admins()), (*Service).hTestGuard},
		ipc.CmdTestClock:  {testMode(admins()), (*Service).hTestClock},
		ipc.CmdGuardState: {testMode(kinds(kindGuard, kindAdmin, kindElev)), (*Service).hGuardState},
	}
}

// unconfiguredCheck разрешает вызов, пока программа не настроена (мастер до пароля).
func unconfiguredCheck() check {
	return func(c *callCtx) string {
		if c.s.configured() {
			return textNeedParent
		}
		return ""
	}
}

// Handle разбирает одну команду канала: определяет вид клиента, сверяется с таблицей прав и
// вызывает обработчик. real: настоящий момент. Внутренняя ошибка обработчика не роняет службу.
func (s *Service) Handle(peer ipc.Peer, msg ipc.Msg, real time.Time) (rep ipc.Reply) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.real = real
	if s.stopping {
		return ipc.Fail(textRetry)
	}
	r, ok := commandRules[msg.Cmd]
	if !ok {
		return ipc.Fail(ipc.TextBadRequest)
	}
	c := &callCtx{s: s, peer: peer, msg: msg, real: real, kind: s.classify(peer)}
	if denial := r.allow(c); denial != "" {
		s.logRate(fmt.Sprintf("deny %s %d %s", msg.Cmd, peer.PID, c.kind), "Отказ: команда %s от процесса %s (%s): %s", msg.Cmd, peer, c.kind, denial)
		return ipc.Fail(denial)
	}
	defer func() {
		if p := recover(); p != nil {
			s.cfg.Logf("Внутренняя ошибка в команде %s: %v", msg.Cmd, p)
			rep = ipc.Fail(ipc.TextInternal)
		}
	}()
	return r.handler(s, c)
}
