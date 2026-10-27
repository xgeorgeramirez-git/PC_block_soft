package guard

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// ctClock — настоящие часы стража в тестах: время стоит, пока тест его не сдвинет.
type ctClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *ctClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *ctClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// ctService — поддельная служба: отвечает на команды стража так, как скажет тест, и помнит,
// что ей прислали.
type ctService struct {
	mu     sync.Mutex
	calls  []ipc.Msg
	status ipc.ChildStatus
	warns  []ipc.GuardWarning
	cmds   []ipc.GuardCommand
	test   bool
	down   bool                               // обрыв связи: Do возвращает ошибку
	reply  map[string]func(ipc.Msg) ipc.Reply // особые ответы по имени команды
}

func newCtService() *ctService {
	return &ctService{reply: map[string]func(ipc.Msg) ipc.Reply{}}
}

func (s *ctService) Do(m ipc.Msg) (ipc.Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, m)
	if s.down {
		return ipc.Reply{}, errors.New("обрыв")
	}
	if f := s.reply[m.Cmd]; f != nil {
		return f(m), nil
	}
	switch m.Cmd {
	case ipc.CmdHello:
		return ipc.OK(ipc.HelloReply{Protocol: ipc.ProtocolVersion, TestMode: s.test}), nil
	case ipc.CmdGuardPoll:
		rep := ipc.GuardPollReply{Status: s.status, Warnings: s.warns, Commands: s.cmds}
		s.warns, s.cmds = nil, nil
		return ipc.OK(rep), nil
	}
	return ipc.OK(ipc.Empty{}), nil
}

// setDown обрывает связь или возвращает её.
func (s *ctService) setDown(down bool) {
	s.mu.Lock()
	s.down = down
	s.mu.Unlock()
}

// called возвращает вызовы команды cmd по порядку.
func (s *ctService) called(cmd string) []ipc.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ipc.Msg
	for _, m := range s.calls {
		if m.Cmd == cmd {
			out = append(out, m)
		}
	}
	return out
}

func ctTimeUpStatus() ipc.ChildStatus {
	return ipc.ChildStatusFrom("Миша", timeUpDecision())
}

func ctRunningStatus(remainingS int) ipc.ChildStatus {
	return ipc.ChildStatusFrom("Миша", runningDecision(remainingS))
}

type ctRig struct {
	svc  *ctService
	clk  *ctClock
	ctl  *Controller
	wake int
	mu   sync.Mutex
}

func newCtRig() *ctRig {
	r := &ctRig{svc: newCtService(), clk: &ctClock{t: t0}}
	read := func(now time.Time) ipc.SensorSample {
		return BuildSample(now, RawSensors{IdleMs: 1500}, 1)
	}
	r.ctl = NewController(NewModel(), r.svc, r.clk.Now, read, func() {
		r.mu.Lock()
		r.wake++
		r.mu.Unlock()
	})
	return r
}

func (r *ctRig) wakes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wake
}

// poll делает опрос и возвращает вид сразу после него.
func (r *ctRig) poll() View {
	r.ctl.PollOnce()
	return r.ctl.View()
}

// unlockedPanel доводит экран блокировки до выбора действий родителя: пароль принят.
func (r *ctRig) choices(t *testing.T) {
	t.Helper()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonParentPassword, "")
	act := r.ctl.Press(ButtonSubmitPassword, "верный")
	if act.Kind != ActCheckPassword {
		t.Fatalf("действие %v, ждали проверку пароля", act.Kind)
	}
	r.ctl.Run(act)
	if v := r.ctl.View(); v.Panel != PanelParentChoices {
		t.Fatalf("панель %v после верного пароля, ждали выбор действий", v.Panel)
	}
}

func TestPollSendsSampleAndEventsAndShowsLock(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	res := r.ctl.PollOnce()
	if res.Err != nil {
		t.Fatalf("опрос: %v", res.Err)
	}
	polls := r.svc.called(ipc.CmdGuardPoll)
	if len(polls) != 1 {
		t.Fatalf("GuardPoll отправлен %d раз, ждали 1", len(polls))
	}
	var req ipc.GuardPollRequest
	if err := polls[0].Into(&req); err != nil {
		t.Fatal(err)
	}
	if req.Sample.AtMs != t0.UnixMilli() || !req.Sample.SessionActive || req.Sample.LastInputAgoMs != 1500 || req.Sample.Foreground != ipc.CatNameOther {
		t.Fatalf("снимок датчиков: %+v", req.Sample)
	}
	if req.Events == nil {
		t.Fatal("события ушли как null, ждали []")
	}
	if got := polls[0].Token; got != "" {
		t.Fatalf("у GuardPoll токен %q, ждали пустой", got)
	}
	v := r.ctl.View()
	if !v.LockShown || v.Lock.Reason != core.ReasonTimeUp || !v.SuppressKeys {
		t.Fatalf("экран блокировки не показан: %+v", v)
	}
	if r.wakes() == 0 {
		t.Fatal("окно не позвали перерисовать после ответа службы")
	}
	// Событие «экран показан» уходит со следующим опросом, один раз.
	r.ctl.PollOnce()
	var req2 ipc.GuardPollRequest
	if err := r.svc.called(ipc.CmdGuardPoll)[1].Into(&req2); err != nil {
		t.Fatal(err)
	}
	if len(req2.Events) != 1 || req2.Events[0].Kind != ipc.GuardEvLockShown {
		t.Fatalf("события второго опроса: %+v", req2.Events)
	}
	r.ctl.PollOnce()
	var req3 ipc.GuardPollRequest
	if err := r.svc.called(ipc.CmdGuardPoll)[2].Into(&req3); err != nil {
		t.Fatal(err)
	}
	if len(req3.Events) != 0 {
		t.Fatalf("событие ушло дважды: %+v", req3.Events)
	}
}

func TestNoAnswerKeepsEventsAndHidesScreenAfterTenSeconds(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.View() // событие «экран показан» уходит со следующим опросом
	r.svc.setDown(true)
	for i := 0; i < 5; i++ {
		r.clk.Advance(2 * time.Second)
		if res := r.ctl.PollOnce(); res.Err == nil {
			t.Fatal("ждали ошибку связи")
		}
	}
	// Ровно 10 с без ответа экран ещё виден, на миллисекунду дольше уже нет (Ruling 15).
	if v := r.ctl.View(); !v.LockShown {
		t.Fatal("экран снят раньше, чем через 10 с без ответа")
	}
	r.clk.Advance(time.Millisecond)
	if v := r.ctl.View(); v.LockShown {
		t.Fatalf("экран остался после 10 с без ответа: %+v", v)
	}
	r.svc.setDown(false)
	r.ctl.PollOnce()
	r.ctl.PollOnce()
	var kinds []string
	for _, m := range r.svc.called(ipc.CmdGuardPoll) {
		var req ipc.GuardPollRequest
		if err := m.Into(&req); err != nil {
			t.Fatal(err)
		}
		for _, e := range req.Events {
			kinds = append(kinds, e.Kind)
		}
	}
	joined := strings.Join(kinds, ",")
	if !strings.Contains(joined, ipc.GuardEvLockShown) {
		t.Fatalf("событие «экран показан» потеряно при обрыве связи: %s", joined)
	}
	if !strings.Contains(joined, EvKindOffline) {
		t.Fatalf("после возвращения связи нет события «страж был без связи»: %s", joined)
	}
}

func TestEventQueueIsBounded(t *testing.T) {
	r := newCtRig()
	r.svc.setDown(true)
	for i := 0; i < maxQueuedEvents+50; i++ {
		r.ctl.mu.Lock()
		r.ctl.addEventLocked(ipc.GuardEvent{Kind: ipc.GuardEvButton, Button: "x"})
		r.ctl.mu.Unlock()
		r.ctl.PollOnce()
	}
	r.ctl.mu.Lock()
	n := len(r.ctl.events)
	r.ctl.mu.Unlock()
	if n > maxQueuedEvents {
		t.Fatalf("в очереди %d событий, предел %d", n, maxQueuedEvents)
	}
}

func TestHideLockCommandHidesScreen(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdHideLock}}
	if v := r.poll(); v.LockShown {
		t.Fatalf("экран виден при открытой родительской сессии: %+v", v)
	}
	if v := r.poll(); !v.LockShown {
		t.Fatal("экран не вернулся, когда команда hideLock перестала приходить")
	}
}

func TestThresholdWarningShownOnceWithEvents(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctRunningStatus(14*60 + 59)
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(15)}
	v := r.poll()
	if v.Warning == nil || v.Warning.Text != "Через 15 минут время на сегодня закончится" {
		t.Fatalf("плашка: %+v", v.Warning)
	}
	r.ctl.Press(ButtonWarningOK, "")
	if v := r.ctl.View(); v.Warning != nil {
		t.Fatal("плашка осталась после «Понятно»")
	}
	r.ctl.PollOnce()
	var req ipc.GuardPollRequest
	if err := r.svc.called(ipc.CmdGuardPoll)[1].Into(&req); err != nil {
		t.Fatal(err)
	}
	var shown, ack, btn int
	for _, e := range req.Events {
		switch e.Kind {
		case ipc.GuardEvWarningShown:
			shown++
			if e.Threshold != 15 {
				t.Fatalf("порог показанной плашки %d, ждали 15", e.Threshold)
			}
		case ipc.GuardEvWarningAck:
			ack++
			if e.Threshold != 15 {
				t.Fatalf("порог подтверждённой плашки %d, ждали 15", e.Threshold)
			}
		case ipc.GuardEvButton:
			btn++
			if e.Button != TargetAck {
				t.Fatalf("кнопка в событии %q, ждали %q", e.Button, TargetAck)
			}
		}
	}
	if shown != 1 || ack != 1 || btn != 1 {
		t.Fatalf("событий: показана %d, понятно %d, кнопка %d; все: %+v", shown, ack, btn, req.Events)
	}
}

func TestNoticeCommandShowsAddedPlateWithoutThresholdEvent(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctRunningStatus(3600)
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdNotice, Text: "Добавлено 30 минут"}}
	v := r.poll()
	if v.Warning == nil || v.Warning.Text != "Добавлено 30 минут" || len(v.Warning.Buttons) != 0 {
		t.Fatalf("плашка «Добавлено»: %+v", v.Warning)
	}
	r.ctl.PollOnce()
	var req ipc.GuardPollRequest
	if err := r.svc.called(ipc.CmdGuardPoll)[1].Into(&req); err != nil {
		t.Fatal(err)
	}
	for _, e := range req.Events {
		if e.Kind == ipc.GuardEvWarningShown {
			t.Fatalf("плашка «Добавлено» записана как предупреждение: %+v", e)
		}
	}
}

func TestNoticeMinutes(t *testing.T) {
	cases := []struct {
		text string
		want int
		ok   bool
	}{
		{"Добавлено 30 минут", 30, true},
		{"Добавлена 1 минута", 1, true},
		{"Добавлено 240 минут", 240, true},
		{"Добавлено 0 минут", 0, false},
		{"Без числа", 0, false},
		{"", 0, false},
		{"7 и 8", 7, true},
		{"99999999999999999999 минут", 1000000, true},
	}
	for _, c := range cases {
		n, ok := NoticeMinutes(c.text)
		if ok != c.ok || (ok && n != c.want && c.want != 1000000) {
			t.Errorf("NoticeMinutes(%q) = %d, %v; ждали %d, %v", c.text, n, ok, c.want, c.ok)
		}
	}
}

func TestClassifyCommandsIgnoresUnknownAndEmptyTest(t *testing.T) {
	cmds := ClassifyCommands([]ipc.GuardCommand{
		{Kind: "что-то новое"},
		{Kind: ipc.GuardCmdTest},
		{Kind: ipc.GuardCmdTest, Test: &ipc.TestGuardAction{Action: ipc.TestEsc}},
		{Kind: ipc.GuardCmdNotice, Text: "без числа"},
		{Kind: ipc.GuardCmdHideLock},
	})
	if !cmds.HideLock || len(cmds.Tests) != 1 || cmds.Tests[0].Action != ipc.TestEsc || len(cmds.NoticeMinutes) != 0 {
		t.Fatalf("команды: %+v", cmds)
	}
}

func TestDecisionFromStatusKeepsEverythingTheScreenNeeds(t *testing.T) {
	for name, d := range map[string]core.Decision{
		"время вышло": timeUpDecision(),
		"обход":       bypassDecision(),
		"идёт":        runningDecision(900),
	} {
		got := DecisionFromStatus(ipc.ChildStatusFrom("Миша", d))
		if got.Lock != d.Lock || got.Reason != d.Reason || got.Detail != d.Detail || got.Until != d.Until ||
			got.Used != d.Used || got.RemainingS != d.RemainingS || got.LimitS != d.LimitS ||
			got.UsedS != d.UsedS || got.Unlimited != d.Unlimited || got.Day != d.Day {
			t.Errorf("%s: решение изменилось по дороге:\nбыло  %+v\nстало %+v", name, d, got)
		}
		if got.Events == nil {
			t.Errorf("%s: Events nil", name)
		}
	}
	if got := DecisionFromStatus(ipc.ChildStatusNotChild()); got.Lock != core.LockNone {
		t.Errorf("не детский сеанс получил блокировку: %+v", got)
	}
	if got := DecisionFromStatus(ipc.ChildStatusAdmin("Миша")); got.Lock != core.LockNone || got.Status != core.StatusSafe {
		t.Errorf("администратор: %+v", got)
	}
}

func TestLoginOutcomes(t *testing.T) {
	now := t0
	if o := LoginOutcomeOf(nil, now); !o.Decided || !o.Result.OK || o.Notice != "" {
		t.Errorf("верный пароль: %+v", o)
	}
	wrong := &ipc.ReplyError{Text: "Неверный пароль. Осталось попыток: 3", Field: ipc.FieldPassword,
		Payload: ipc.Payload(ipc.LoginDenied{AttemptsLeft: 3})}
	if o := LoginOutcomeOf(wrong, now); !o.Decided || o.Result.OK || o.Result.AttemptsLeft != 3 || !o.Result.BlockedUntil.IsZero() {
		t.Errorf("неверный пароль: %+v", o)
	}
	pause := &ipc.ReplyError{Text: "Слишком много попыток", Field: ipc.FieldPassword,
		Payload: ipc.Payload(ipc.LoginDenied{PauseS: 60})}
	if o := LoginOutcomeOf(pause, now); !o.Decided || !o.Result.BlockedUntil.Equal(now.Add(time.Minute)) {
		t.Errorf("пауза: %+v", o)
	}
	notConfigured := &ipc.ReplyError{Text: "Программа ещё не настроена", Field: ipc.FieldPassword}
	if o := LoginOutcomeOf(notConfigured, now); o.Decided || o.Notice != "Программа ещё не настроена" {
		t.Errorf("отказ без данных о паролях: %+v", o)
	}
	busy := &ipc.ReplyError{Text: "Служба занята. Попробуйте ещё раз"}
	if o := LoginOutcomeOf(busy, now); o.Decided || o.Notice != "Служба занята. Попробуйте ещё раз" {
		t.Errorf("отказ службы: %+v", o)
	}
	if o := LoginOutcomeOf(errors.New("что-то странное"), now); o.Decided || o.Notice != textNoticeRetry {
		t.Errorf("чужая ошибка: %+v", o)
	}
}

func TestPasswordFlowThroughService(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonParentPassword, "")

	// Неверный пароль: счётчик под полем.
	r.svc.reply[ipc.CmdLogin] = func(m ipc.Msg) ipc.Reply {
		return ipc.FailWith("Неверный пароль. Осталось попыток: 3", ipc.FieldPassword, ipc.LoginDenied{AttemptsLeft: 3})
	}
	act := r.ctl.Press(ButtonSubmitPassword, "не тот")
	r.ctl.Run(act)
	v := r.ctl.View()
	if v.Panel != PanelPassword || v.PasswordError != "Неверный пароль. Осталось попыток: 3" {
		t.Fatalf("после неверного пароля: панель %v, ошибка %q", v.Panel, v.PasswordError)
	}
	var login ipc.LoginRequest
	if err := r.svc.called(ipc.CmdLogin)[0].Into(&login); err != nil || login.Password != "не тот" {
		t.Fatalf("Login получил %+v, %v", login, err)
	}

	// Пауза: поле закрыто, идёт отсчёт; опросы идут, связь жива.
	r.svc.reply[ipc.CmdLogin] = func(m ipc.Msg) ipc.Reply {
		return ipc.FailWith("Слишком много попыток. Повторить можно через 1:00", ipc.FieldPassword, ipc.LoginDenied{PauseS: 60})
	}
	r.ctl.Run(r.ctl.Press(ButtonSubmitPassword, "ещё раз"))
	v = r.ctl.View()
	if v.PasswordEnabled || v.PasswordCountdown != "1:00" {
		t.Fatalf("пауза не показана: %+v", v)
	}
	for i := 0; i < 13; i++ {
		r.clk.Advance(5 * time.Second)
		r.poll()
	}
	if v := r.ctl.View(); !v.PasswordEnabled || v.Panel != PanelPassword {
		t.Fatalf("пауза не кончилась через 65 с: %+v", v)
	}
	r.svc.reply[ipc.CmdLogin] = func(m ipc.Msg) ipc.Reply {
		return ipc.OK(ipc.LoginReply{IdleTimeoutS: 30})
	}
	r.ctl.Run(r.ctl.Press(ButtonSubmitPassword, "верный"))
	if v := r.ctl.View(); v.Panel != PanelParentChoices {
		t.Fatalf("панель %v после верного пароля", v.Panel)
	}
}

func TestPasswordIsNeverInEvents(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonParentPassword, "")
	r.ctl.Run(r.ctl.Press(ButtonSubmitPassword, "СекретныйПароль"))
	r.ctl.PollOnce()
	for _, m := range r.svc.called(ipc.CmdGuardPoll) {
		if strings.Contains(string(m.Payload), "СекретныйПароль") {
			t.Fatalf("пароль попал в опрос: %s", m.Payload)
		}
	}
}

func TestChoicesSendCommandsToService(t *testing.T) {
	t.Run("добавить время", func(t *testing.T) {
		r := newCtRig()
		r.choices(t)
		r.ctl.Run(r.ctl.Press(ButtonAdd30, ""))
		var req ipc.AddExtraRequest
		calls := r.svc.called(ipc.CmdAddExtra)
		if len(calls) != 1 || calls[0].Into(&req) != nil || req.Minutes != 30 || req.ChildID != 0 {
			t.Fatalf("AddExtra: %+v, %+v", calls, req)
		}
		// Экран исчезает по решению службы, плашка «Добавлено» появляется следом.
		r.svc.status = ctRunningStatus(1800)
		v := r.poll()
		if v.LockShown || v.Warning == nil || v.Warning.Text != "Добавлено 30 минут" {
			t.Fatalf("после доп. времени: %+v", v)
		}
	})
	t.Run("снять блокировку", func(t *testing.T) {
		r := newCtRig()
		r.svc.status = ipc.ChildStatusFrom("Миша", bypassDecision())
		r.poll()
		r.ctl.Press(ButtonParentPassword, "")
		r.ctl.Run(r.ctl.Press(ButtonSubmitPassword, "верный"))
		r.ctl.Run(r.ctl.Press(ButtonClearBlock, ""))
		if n := len(r.svc.called(ipc.CmdClearBlock)); n != 1 {
			t.Fatalf("ClearBlock отправлен %d раз", n)
		}
	})
	t.Run("открыть настройки", func(t *testing.T) {
		r := newCtRig()
		r.choices(t)
		r.ctl.Run(r.ctl.Press(ButtonOpenSettings, ""))
		var req ipc.OpenWindowRequest
		calls := r.svc.called(ipc.CmdOpenWindow)
		if len(calls) != 1 || calls[0].Into(&req) != nil || req.Kind != ipc.WindowParent {
			t.Fatalf("OpenWindow: %+v, %+v", calls, req)
		}
	})
	t.Run("выйти из учётной записи", func(t *testing.T) {
		r := newCtRig()
		r.svc.status = ctTimeUpStatus()
		r.poll()
		r.ctl.Press(ButtonLogoff, "")
		r.ctl.Run(r.ctl.Press(ButtonConfirmLogoff, ""))
		if n := len(r.svc.called(ipc.CmdGuardLogoff)); n != 1 {
			t.Fatalf("GuardLogoff отправлен %d раз", n)
		}
	})
	t.Run("выключить компьютер", func(t *testing.T) {
		r := newCtRig()
		r.svc.status = ctTimeUpStatus()
		r.poll()
		r.ctl.Run(r.ctl.Press(ButtonShutdown, ""))
		if n := len(r.svc.called(ipc.CmdGuardShutdown)); n != 0 {
			t.Fatalf("GuardShutdown отправлен без подтверждения (%d раз)", n)
		}
		r.ctl.Run(r.ctl.Press(ButtonConfirmShutdown, ""))
		if n := len(r.svc.called(ipc.CmdGuardShutdown)); n != 1 {
			t.Fatalf("GuardShutdown отправлен %d раз", n)
		}
		if n := len(r.svc.called(ipc.CmdGuardLogoff)) + len(r.svc.called(ipc.CmdGuardSwitchUser)); n != 0 {
			t.Fatalf("вместе с выключением ушли другие выходы (%d)", n)
		}
	})
	t.Run("смена пользователя", func(t *testing.T) {
		r := newCtRig()
		r.svc.status = ctTimeUpStatus()
		r.poll()
		r.ctl.Run(r.ctl.Press(ButtonSwitchUser, ""))
		if n := len(r.svc.called(ipc.CmdGuardSwitchUser)); n != 1 {
			t.Fatalf("GuardSwitchUser отправлен %d раз (подтверждения нет, нужно одно нажатие)", n)
		}
		if n := len(r.svc.called(ipc.CmdGuardLogoff)) + len(r.svc.called(ipc.CmdGuardShutdown)); n != 0 {
			t.Fatalf("вместе со сменой пользователя ушли другие выходы (%d)", n)
		}
	})
}

func TestShutdownAndSwitchFailureSpeakToChild(t *testing.T) {
	// Ловит: отказ службы при выключении или смене пользователя молчит или показывает
	// технический текст; тексты ребёнку на «ты», без длинных тире.
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonShutdown, "")
	r.svc.reply[ipc.CmdGuardShutdown] = func(m ipc.Msg) ipc.Reply { return ipc.Fail("") }
	r.ctl.Run(r.ctl.Press(ButtonConfirmShutdown, ""))
	if got := r.ctl.Notice(); got != "Не получилось выключить. Попробуй ещё раз" {
		t.Fatalf("сообщение о выключении %q", got)
	}
	r.ctl.Press(ButtonCancel, "")
	r.clk.Advance(noticeLife + time.Second)
	r.svc.reply[ipc.CmdGuardSwitchUser] = func(m ipc.Msg) ipc.Reply { return ipc.Fail("") }
	r.ctl.Run(r.ctl.Press(ButtonSwitchUser, ""))
	if got := r.ctl.Notice(); got != "Не получилось сменить пользователя. Попробуй ещё раз" {
		t.Fatalf("сообщение о смене пользователя %q", got)
	}
	for _, s := range []string{textNoticeShutdown, textNoticeSwitchUser} {
		if strings.ContainsAny(s, "—–") {
			t.Errorf("длинное тире в %q", s)
		}
	}
}

func TestExitButtonsIgnoredWhileCommandRuns(t *testing.T) {
	// Ловит: двойное нажатие «Смена пользователя» или «Выключить» отправляет команду дважды.
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	first := r.ctl.Press(ButtonSwitchUser, "")
	if first.Kind != ActSwitchUser {
		t.Fatalf("первое нажатие: %+v", first)
	}
	if second := r.ctl.Press(ButtonSwitchUser, ""); second.Kind != ActNone {
		t.Fatalf("повторное нажатие во время команды: %+v", second)
	}
	r.ctl.Run(first)
	if n := len(r.svc.called(ipc.CmdGuardSwitchUser)); n != 1 {
		t.Fatalf("GuardSwitchUser отправлен %d раз", n)
	}
	r.ctl.Press(ButtonShutdown, "")
	a := r.ctl.Press(ButtonConfirmShutdown, "")
	if a.Kind != ActShutdown {
		t.Fatalf("«Выключить»: %+v", a)
	}
	if b := r.ctl.Press(ButtonConfirmShutdown, ""); b.Kind != ActNone {
		t.Fatalf("повторное «Выключить» во время команды: %+v", b)
	}
	r.ctl.Run(a)
}

// pollFor опрашивает службу раз в 5 с, пока не пройдёт d, и возвращает действие, которое
// предложила автосмена в каждый момент; пустые не копятся.
func (r *ctRig) pollFor(d time.Duration) []Action {
	var out []Action
	for end := r.clk.Now().Add(d); r.clk.Now().Before(end); {
		step := 5 * time.Second
		if rest := end.Sub(r.clk.Now()); rest < step {
			step = rest
		}
		r.clk.Advance(step)
		r.ctl.PollOnce()
		if a := r.ctl.AutoSwitch(); a.Kind != ActNone {
			out = append(out, a)
		}
	}
	return out
}

func TestControllerAutoSwitchAfterFifteenMinutes(t *testing.T) {
	// Ловит: автосмена не доходит до службы; срабатывает раньше 15 минут или дважды за срок;
	// после неё экран остаётся (сеанс ребёнка не завершён).
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	if got := r.pollFor(testAutoSwitch - 5*time.Second); len(got) != 0 {
		t.Fatalf("смена раньше срока: %+v", got)
	}
	got := r.pollFor(5 * time.Second)
	if len(got) != 1 || got[0].Kind != ActSwitchUser {
		t.Fatalf("через 15 минут: %+v", got)
	}
	r.ctl.Run(got[0])
	if n := len(r.svc.called(ipc.CmdGuardSwitchUser)); n != 1 {
		t.Fatalf("GuardSwitchUser отправлен %d раз", n)
	}
	if n := len(r.svc.called(ipc.CmdGuardLogoff)) + len(r.svc.called(ipc.CmdGuardShutdown)); n != 0 {
		t.Fatalf("автосмена отправила другой выход (%d)", n)
	}
	if got := r.pollFor(testAutoSwitch - 5*time.Second); len(got) != 0 {
		t.Fatalf("лишняя смена в следующие 15 минут: %+v", got)
	}
	if v := r.ctl.View(); !v.LockShown {
		t.Fatal("экран блокировки пропал после автосмены")
	}
}

func TestControllerAutoSwitchBlocksRepeatedPressUntilDone(t *testing.T) {
	// Ловит: пока автосмена идёт, ребёнок нажимает «Смена пользователя» ещё раз (команда
	// уходит дважды); после конца команды кнопки снова работают.
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.pollFor(testAutoSwitch - 5*time.Second)
	r.clk.Advance(5 * time.Second)
	r.ctl.PollOnce()
	auto := r.ctl.AutoSwitch()
	if auto.Kind != ActSwitchUser {
		t.Fatalf("автосмена: %+v", auto)
	}
	if again := r.ctl.Press(ButtonSwitchUser, ""); again.Kind != ActNone {
		t.Fatalf("нажатие во время автосмены: %+v", again)
	}
	r.ctl.Run(auto)
	if again := r.ctl.Press(ButtonSwitchUser, ""); again.Kind != ActSwitchUser {
		t.Fatalf("нажатие после автосмены: %+v", again)
	}
}

func TestControllerAutoSwitchWaitsForRunningCommand(t *testing.T) {
	// Ловит: автосмена вклинивается, пока идёт другая команда стража (проверка пароля родителя):
	// срок не пропадает, а переносится на следующий опрос.
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.pollFor(testAutoSwitch - 5*time.Second)
	r.ctl.Press(ButtonParentPassword, "")
	busy := r.ctl.Press(ButtonSubmitPassword, "пароль")
	if busy.Kind != ActCheckPassword {
		t.Fatalf("проверка пароля: %+v", busy)
	}
	r.clk.Advance(5 * time.Second)
	r.ctl.PollOnce()
	if a := r.ctl.AutoSwitch(); a.Kind != ActNone {
		t.Fatalf("автосмена во время другой команды: %+v", a)
	}
	r.svc.reply[ipc.CmdLogin] = func(m ipc.Msg) ipc.Reply { return ipc.Fail("") }
	r.ctl.Run(busy)
	r.clk.Advance(time.Second)
	r.ctl.PollOnce()
	if a := r.ctl.AutoSwitch(); a.Kind != ActSwitchUser {
		t.Fatalf("после команды автосмена не вернулась: %+v", a)
	}
}

func TestControllerAutoSwitchShortenedOnlyInTestMode(t *testing.T) {
	// Ловит: срок из Hello действует вне тестового режима (ребёнок не должен менять срок);
	// в тестовом режиме срок не сокращается (приёмка ждала бы 15 минут настоящих часов).
	const short = 20
	for _, test := range []bool{true, false} {
		r := newCtRig()
		r.svc.test = test
		r.svc.reply[ipc.CmdHello] = func(m ipc.Msg) ipc.Reply {
			return ipc.OK(ipc.HelloReply{Protocol: ipc.ProtocolVersion, TestMode: test, AutoSwitchS: short})
		}
		r.svc.status = ctTimeUpStatus()
		r.poll()
		got := r.pollFor(short * time.Second)
		if test && (len(got) != 1 || got[0].Kind != ActSwitchUser) {
			t.Fatalf("тестовый режим: через %d с %+v", short, got)
		}
		if !test && len(got) != 0 {
			t.Fatalf("вне тестового режима срок сокращён: %+v", got)
		}
	}
}

func TestFailedActionShowsNoticeAndLetsTryAgain(t *testing.T) {
	r := newCtRig()
	r.choices(t)
	r.svc.reply[ipc.CmdAddExtra] = func(m ipc.Msg) ipc.Reply {
		return ipc.Fail("Служба занята. Попробуйте ещё раз")
	}
	r.ctl.Run(r.ctl.Press(ButtonAdd15, ""))
	if got := r.ctl.Notice(); got != "Служба занята. Попробуйте ещё раз" {
		t.Fatalf("сообщение об ошибке %q", got)
	}
	// Родитель снова на экране с кнопками: пароль спросят заново, время не добавлено.
	if v := r.ctl.View(); !v.LockShown {
		t.Fatalf("экран пропал после неудачной команды: %+v", v)
	}
	r.clk.Advance(noticeLife + time.Second)
	r.svc.setDown(false)
	if got := r.ctl.Notice(); got != "" {
		t.Fatalf("сообщение не исчезло: %q", got)
	}
}

func TestFailureTextsAreHuman(t *testing.T) {
	transport := &ipc.Error{Kind: ipc.ErrTimeout, Cause: errors.New("i/o timeout, pipe 0xc000")}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"связи нет", transport, "Программа не отвечает. Попробуйте ещё раз"},
		{"служба отказала с текстом", &ipc.ReplyError{Text: "Служба занята. Попробуйте ещё раз"}, "Служба занята. Попробуйте ещё раз"},
		{"служба отказала без текста", &ipc.ReplyError{}, textNoticeRetry},
		{"чужая ошибка", errors.New("runtime error: index out of range"), textNoticeRetry},
	}
	for _, c := range cases {
		if got := failureText(c.err, textNoticeRetry); got != c.want {
			t.Errorf("%s: %q, ждали %q", c.name, got, c.want)
		}
	}
	r := newCtRig()
	r.choices(t)
	r.svc.reply[ipc.CmdAddExtra] = func(m ipc.Msg) ipc.Reply { return ipc.Fail("") }
	r.ctl.Run(r.ctl.Press(ButtonAdd15, ""))
	if got := r.ctl.Notice(); got != textNoticeRetry {
		t.Fatalf("сообщение %q, ждали %q", got, textNoticeRetry)
	}
}

func TestLogoffFailureSpeaksToChild(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonLogoff, "")
	r.svc.reply[ipc.CmdGuardLogoff] = func(m ipc.Msg) ipc.Reply { return ipc.Fail("") }
	r.ctl.Run(r.ctl.Press(ButtonConfirmLogoff, ""))
	if got := r.ctl.Notice(); got != textNoticeLogoff {
		t.Fatalf("сообщение %q, ждали %q", got, textNoticeLogoff)
	}
}

func TestServiceCommandRunsOncePerPressWhileBusy(t *testing.T) {
	r := newCtRig()
	r.choices(t)
	first := r.ctl.Press(ButtonAdd15, "")
	if first.Kind != ActAddExtra {
		t.Fatalf("первое нажатие: %+v", first)
	}
	// Команда ещё в пути: повторное нажатие любой кнопки службы игнорируется.
	if second := r.ctl.Press(ButtonAdd30, ""); second.Kind != ActNone {
		t.Fatalf("повторное нажатие во время команды дало действие %+v", second)
	}
	r.ctl.Run(first)
	if n := len(r.svc.called(ipc.CmdAddExtra)); n != 1 {
		t.Fatalf("AddExtra отправлен %d раз", n)
	}
	// Когда команда закончилась, кнопки снова работают.
	r.ctl.Press(ButtonParentPassword, "")
	if act := r.ctl.Press(ButtonSubmitPassword, "x"); act.Kind != ActCheckPassword {
		t.Fatalf("после команды кнопки не работают: %+v", act)
	}
	r.ctl.Run(Action{})
}

func TestTestGuardCommandsOnlyInTestMode(t *testing.T) {
	for _, test := range []bool{false, true} {
		r := newCtRig()
		r.svc.test = test
		r.svc.status = ctRunningStatus(3600)
		r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdTest, Test: &ipc.TestGuardAction{Action: ipc.TestClick, Target: TargetAck}}}
		r.poll()
		got := r.ctl.QueuedTests()
		if test && (len(got) != 1 || got[0].Target != TargetAck) {
			t.Errorf("тестовый режим: действия %+v", got)
		}
		if !test && len(got) != 0 {
			t.Errorf("обычный режим выполнил тестовую команду: %+v", got)
		}
		if again := r.ctl.QueuedTests(); len(again) != 0 {
			t.Errorf("действие отдано дважды: %+v", again)
		}
	}
}

func TestStateIsPushedOnlyInTestModeAndOnlyOnChange(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.PushState()
	if n := len(r.svc.called(ipc.CmdGuardState)); n != 0 {
		t.Fatalf("вне тестового режима вид отправлен %d раз", n)
	}

	r = newCtRig()
	r.svc.test = true
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.PushState()
	r.ctl.PushState()
	calls := r.svc.called(ipc.CmdGuardState)
	if len(calls) != 1 {
		t.Fatalf("вид отправлен %d раз, ждали 1 (повтор без изменений не нужен)", len(calls))
	}
	var st ipc.GuardStateReply
	if err := calls[0].Into(&st); err != nil {
		t.Fatal(err)
	}
	if !st.LockShown || st.LockKind != ipc.LockTimeUp || strings.Join(st.Buttons, ",") != strings.Join([]string{TargetAskMore, TargetPromo, TargetParentPassword, TargetShutdown, TargetLogoff, TargetSwitchUser}, ",") {
		t.Fatalf("вид стража: %+v", st)
	}
	r.ctl.Press(ButtonParentPassword, "")
	r.ctl.View()
	r.ctl.SetField(FieldPasswordID, "abc")
	r.ctl.PushState()
	calls = r.svc.called(ipc.CmdGuardState)
	if len(calls) != 2 {
		t.Fatalf("после изменения вид отправлен %d раз, ждали 2", len(calls))
	}
	if err := calls[1].Into(&st); err != nil {
		t.Fatal(err)
	}
	if len(st.Fields) != 1 || st.Fields[0].ID != FieldPasswordID || st.Fields[0].Text != "abc" {
		t.Fatalf("поля: %+v", st.Fields)
	}
}

func TestFieldTextIsKeptOnlyInTestMode(t *testing.T) {
	r := newCtRig()
	r.poll()
	r.ctl.SetField(FieldPasswordID, "секрет")
	r.ctl.mu.Lock()
	n := len(r.ctl.fields)
	r.ctl.mu.Unlock()
	if n != 0 {
		t.Fatal("текст поля запомнен вне тестового режима")
	}
}

func TestStateFromViewKeepsSlicesNonNil(t *testing.T) {
	st := StateFromView(View{Buttons: []ButtonView{}, Choices: []ButtonView{}}, "", nil, nil)
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("в виде стража есть null: %s", raw)
	}
	if st.LockKind != ipc.LockNone || st.LockShown {
		t.Fatalf("пустой вид: %+v", st)
	}
}

func TestOfflineEventCarriesSeconds(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctRunningStatus(3600)
	r.poll()
	r.svc.setDown(true)
	r.clk.Advance(30 * time.Second)
	r.ctl.PollOnce()
	r.svc.setDown(false)
	r.ctl.PollOnce()
	r.ctl.PollOnce()
	for _, m := range r.svc.called(ipc.CmdGuardPoll) {
		var req ipc.GuardPollRequest
		if err := m.Into(&req); err != nil {
			t.Fatal(err)
		}
		for _, e := range req.Events {
			if e.Kind == EvKindOffline {
				if e.Threshold != 30 || !strings.Contains(e.Detail, "30") {
					t.Fatalf("событие без связи: %+v", e)
				}
				return
			}
		}
	}
	t.Fatal("события «страж был без связи» нет ни в одном опросе")
}

func TestKickIsNonBlockingAndCoalesces(t *testing.T) {
	r := newCtRig()
	for i := 0; i < 5; i++ {
		r.ctl.Kick()
	}
	select {
	case <-r.ctl.Kicked():
	default:
		t.Fatal("сигнал опроса не пришёл")
	}
	select {
	case <-r.ctl.Kicked():
		t.Fatal("сигналы не склеились")
	default:
	}
}

func TestControllerIsSafeForConcurrentUse(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	r.svc.status = ctTimeUpStatus()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					f()
				}
			}
		}()
	}
	run(func() { r.ctl.PollOnce(); r.ctl.PushState() })
	run(func() { r.ctl.View(); r.ctl.Notice() })
	run(func() {
		r.ctl.Press(ButtonParentPassword, "")
		r.ctl.Run(r.ctl.Press(ButtonSubmitPassword, "x"))
		r.ctl.Press(ButtonCancel, "")
		r.ctl.SetField(FieldPasswordID, "x")
		r.ctl.QueuedTests()
	})
	time.Sleep(80 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// --- штатное закрытие стража при конце сеанса (FB-1) ---------------------------------------------

// closingCaller — клиент канала для проверки NotifyClosing: запоминает команды, отвечает сразу
// или только после release, может вернуть ошибку или отказ службы.
type closingCaller struct {
	mu      sync.Mutex
	calls   []ipc.Msg
	release chan struct{} // не nil: ответ только после закрытия канала
	err     error
	refuse  bool
}

func (c *closingCaller) Do(m ipc.Msg) (ipc.Reply, error) {
	c.mu.Lock()
	c.calls = append(c.calls, m)
	rel, err, refuse := c.release, c.err, c.refuse
	c.mu.Unlock()
	if rel != nil {
		<-rel
	}
	if err != nil {
		return ipc.Reply{}, err
	}
	if refuse {
		return ipc.Fail("Нельзя"), nil
	}
	return ipc.OK(ipc.Empty{}), nil
}

func (c *closingCaller) sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{}
	for _, m := range c.calls {
		out = append(out, m.Cmd)
	}
	return out
}

func newClosingController(c ipc.Caller) *Controller {
	return NewController(NewModel(), c, (&ctClock{t: t0}).Now, func(time.Time) ipc.SensorSample { return ipc.SensorSample{} }, nil)
}

// Ловит: страж при конце сеанса не сообщает службе GuardClosing (она запишет «Защита программы
// была остановлена»), либо шлёт другую команду, либо несколько.
func TestNotifyClosingSendsGuardClosing(t *testing.T) {
	cc := &closingCaller{}
	if !newClosingController(cc).NotifyClosing(time.Second) {
		t.Fatal("NotifyClosing вернул false, хотя служба приняла сообщение")
	}
	if got := cc.sent(); len(got) != 1 || got[0] != "GuardClosing" {
		t.Fatalf("команды службе: %v; want одна GuardClosing", got)
	}
}

// Ловит: ошибка канала или отказ службы при конце сеанса роняют стража или всплывают наружу:
// сеанс заканчивается, служба может быть уже остановлена.
func TestNotifyClosingIgnoresChannelErrorAndRefusal(t *testing.T) {
	for name, cc := range map[string]*closingCaller{
		"обрыв связи":  {err: errors.New("обрыв")},
		"отказ службы": {refuse: true},
	} {
		if newClosingController(cc).NotifyClosing(time.Second) {
			t.Errorf("%s: NotifyClosing вернул true", name)
		}
		if got := cc.sent(); len(got) != 1 {
			t.Errorf("%s: команд %d; want 1 (повторов нет)", name, len(got))
		}
	}
}

// Ловит: страж ждёт зависшую службу и задерживает конец сеанса (выключение компьютера): вызов
// обязан вернуться через заданный срок, не дожидаясь ответа.
func TestNotifyClosingDoesNotWaitLongerThanLimit(t *testing.T) {
	cc := &closingCaller{release: make(chan struct{})}
	defer close(cc.release)
	start := time.Now()
	if newClosingController(cc).NotifyClosing(30 * time.Millisecond) {
		t.Fatal("NotifyClosing вернул true, хотя служба не ответила")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("NotifyClosing ждал %v; want около 30 мс (не ждать зависшую службу)", d)
	}
}

// Ловит: паника в канале связи при конце сеанса убивает процесс стража до выхода из сеанса.
func TestNotifyClosingSurvivesPanic(t *testing.T) {
	if newClosingController(panicCaller{}).NotifyClosing(time.Second) {
		t.Fatal("NotifyClosing вернул true после паники")
	}
}

type panicCaller struct{}

func (panicCaller) Do(ipc.Msg) (ipc.Reply, error) { panic("сломалось") }

// Ловит: уведомление GuardClosing уходит и в WM_QUERYENDSESSION, и в WM_ENDSESSION, и каждый раз
// поток окон ждёт до 2 с (при зависшей службе конец сеанса тормозится вдвое дольше). Исход
// первого вызова, удачный или нет, запоминается на весь этот конец сеанса; отмена конца сеанса
// (ResetClosing) разрешает уведомить снова при следующем.
func TestNotifyClosingIsNotRepeatedInSameSessionEnd(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		cc := &closingCaller{}
		c := newClosingController(cc)
		if !c.NotifyClosing(time.Second) || !c.NotifyClosing(time.Second) {
			t.Fatal("оба вызова должны вернуть исход первого: true")
		}
		if got := cc.sent(); len(got) != 1 {
			t.Fatalf("команд службе %d; want 1", len(got))
		}
	})
	t.Run("неудача не повторяется и не ждёт снова", func(t *testing.T) {
		cc := &closingCaller{release: make(chan struct{})}
		defer close(cc.release)
		c := newClosingController(cc)
		if c.NotifyClosing(30 * time.Millisecond) {
			t.Fatal("первый вызов: служба не ответила, want false")
		}
		start := time.Now()
		if c.NotifyClosing(time.Second) {
			t.Fatal("второй вызов должен вернуть исход первого: false")
		}
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Fatalf("второй вызов ждал %v; want без ожидания", d)
		}
		if got := cc.sent(); len(got) != 1 {
			t.Fatalf("команд службе %d; want 1", len(got))
		}
	})
	t.Run("после отмены конца сеанса уведомляет снова", func(t *testing.T) {
		cc := &closingCaller{}
		c := newClosingController(cc)
		c.NotifyClosing(time.Second)
		c.ResetClosing()
		c.NotifyClosing(time.Second)
		if got := cc.sent(); len(got) != 2 {
			t.Fatalf("команд службе %d; want 2", len(got))
		}
	})
}
