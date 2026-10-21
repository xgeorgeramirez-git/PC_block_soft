package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Ловит: неверное склонение «минута» в сообщениях родителю («Добавлено 21 минут», «11 минуты»).
func TestMinutesText(t *testing.T) {
	cases := map[int]string{
		1: "1 минуту", 2: "2 минуты", 4: "4 минуты", 5: "5 минут", 11: "11 минут", 12: "12 минут", 14: "14 минут",
		15: "15 минут", 21: "21 минуту", 22: "22 минуты", 25: "25 минут", 30: "30 минут", 60: "60 минут",
		101: "101 минуту", 111: "111 минут", 240: "240 минут",
	}
	for n, want := range cases {
		if got := minutesText(n); got != want {
			t.Errorf("%d: %q, ждали %q", n, got, want)
		}
	}
}

// Ловит: отсчёт паузы пароля выводится не в виде м:сс («0:60» вместо «1:00») и уходит в минус.
func TestClockText(t *testing.T) {
	for in, want := range map[int]string{0: "0:00", 5: "0:05", 59: "0:59", 60: "1:00", 125: "2:05", -3: "0:00", 900: "15:00"} {
		if got := clockText(in); got != want {
			t.Errorf("%d: %q, ждали %q", in, got, want)
		}
	}
}

// Ловит: секунды паузы округляются вниз (человек увидел бы «0:00» при закрытом вводе).
func TestSecondsLeftRoundsUp(t *testing.T) {
	for _, c := range []struct {
		until, now int64
		want       int
	}{{1000, 0, 1}, {1001, 0, 2}, {60000, 0, 60}, {5, 10, 0}, {10, 10, 0}} {
		if got := secondsLeft(c.until, c.now); got != c.want {
			t.Errorf("%d-%d: %d, ждали %d", c.until, c.now, got, c.want)
		}
	}
}

// Ловит: события журнала выводятся без нужных слов (R22: простые слова для каждого вида события).
func TestEventTextKinds(t *testing.T) {
	cases := []struct {
		kind string
		d    map[string]any
		want string
	}{
		{"extra_undone", map[string]any{"minutes": 30}, "Миша: изменение доп. времени отменено"},
		{"block_cleared", nil, "Миша: блокировка снята родителем"},
		{"safe_mode", nil, "Миша: правила не прочитаны, ограничения не действуют"},
		{"child_admin", nil, "Миша: учётка получила права администратора, ограничения не действуют"},
		{"child_admin_off", nil, "Миша: права администратора сняты, ограничения снова действуют"},
		{"bypass", map[string]any{}, "Миша: попытка обойти защиту"},
		{"day_closed", map[string]any{"day": "не-дата", "total_s": 60}, "Миша: итог дня не-дата: 1 мин"},
		{"password_changed", nil, "Миша: пароль родителя изменён"},
		{"password_reset", nil, "Миша: пароль родителя сброшен администратором"},
		{"setup_done", nil, "Миша: программа настроена"},
		{"db_recovered", nil, "Миша: не удалось прочитать правила, ограничения не действовали, прежние данные сохранены"},
		{"uninstall_forgot", nil, "Миша: удаление программы без пароля, пароль забыт"},
	}
	for _, c := range cases {
		if got := eventText(c.kind, "Миша", c.d); got != c.want {
			t.Errorf("%s: %q, ждали %q", c.kind, got, c.want)
		}
	}
	if got := detInt(map[string]any{"a": int64(7), "b": 8, "c": 9.0, "d": "x"}, "a") + detInt(map[string]any{"b": 8}, "b") + detInt(map[string]any{"c": 9.0}, "c") + detInt(map[string]any{"d": "x"}, "d"); got != 24 {
		t.Errorf("detInt: %d", got)
	}
}

// Ловит: New принимает неполную настройку (нет хранилища, часов и так далее) и падает позже
// на первом тике; значения по умолчанию не заданы.
func TestNewValidatesConfig(t *testing.T) {
	st := newFakeStore()
	full := Config{Store: st, Hasher: &fakeHasher{}, Launcher: newLauncher(), Clock: newClock(mon1500)}
	for name, mutate := range map[string]func(*Config){
		"нет хранилища": func(c *Config) { c.Store = nil },
		"нет хеша":      func(c *Config) { c.Hasher = nil },
		"нет запуска":   func(c *Config) { c.Launcher = nil },
		"нет часов":     func(c *Config) { c.Clock = nil },
	} {
		cfg := full
		mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: ждали ошибку", name)
		}
	}
	s, err := New(full)
	if err != nil {
		t.Fatal(err)
	}
	// значения по умолчанию рабочие: служба стартует и отвечает без необязательных портов
	if err := s.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	rep := s.Handle(ipc.Peer{PID: 1, SID: parentSID, Admin: true}, ipc.Msg{Cmd: ipc.CmdHello}, time.Now())
	if !rep.OK {
		t.Fatalf("Hello: %+v", rep)
	}
	if s.today() == "" {
		t.Error("день не определён без заданного пояса")
	}
	if ok, err := (noAdmins{}).IsAdmin("x"); ok || err != nil {
		t.Error("noAdmins")
	}
	if l, err := (noAccounts{}).List(); len(l) != 0 || err != nil {
		t.Error("noAccounts")
	}
	if (noPolicy{}).SetTaskMgrDisabled("x", true) != nil {
		t.Error("noPolicy")
	}
}

// Ловит: токены случайной строки пустые или одинаковые.
func TestRandomToken(t *testing.T) {
	a, b := randomToken(), randomToken()
	if len(a) != 64 || a == b {
		t.Fatalf("токены %q и %q", a, b)
	}
}

// Ловит: после сбоя чтения списка детей служба не пробует снова (дети появляются только после
// перезапуска, ограничений нет).
func TestChildrenListRetriedAfterFailure(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1))
	failing := &listFailStore{fakeStore: h.store, fail: true}
	h.cfg.Store = failing
	h.cfg.Journal = nil
	h.svc, _ = New(h.cfg)
	h.start()
	h.childLogsIn()
	h.hk(70)
	if st := h.status(); st.Status != ipc.StatusNotChild {
		t.Fatalf("до починки: %+v", st)
	}
	failing.fail = false
	h.hk(2)
	if st := h.status(); st.Name != "Миша" {
		t.Fatalf("после починки ребёнок не появился: %+v", st)
	}
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
		t.Fatal("страж ребёнку не запущен после починки базы")
	}
}

type listFailStore struct {
	*fakeStore
	fail bool
}

func (l *listFailStore) ListChildren() ([]Child, error) {
	if l.fail {
		return nil, errFake
	}
	return l.fakeStore.ListChildren()
}

// Ловит: ошибка чтения пароля открывает мастер заново (Setup разрешён поверх существующего пароля).
func TestAuthReadErrorCountsAsConfigured(t *testing.T) {
	h := newHarness(t, configured())
	h.store.failAuth = true
	h.start()
	var hr ipc.HelloReply
	h.ok(adminPeer(), "", ipc.CmdHello, nil, &hr)
	if !hr.Configured {
		t.Fatal("при ошибке чтения пароль считается незаданным")
	}
	h.fail(elevatedPeer(), "", ipc.CmdSetup, setupReq("секрет-123", childSID))
	rep := h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword})
	if rep.Error != textRetry {
		t.Fatalf("вход при ошибке чтения: %q", rep.Error)
	}
}

// Ловит: открытое окно без передачи сессии остаётся, когда страж просит окно с сессией
// (родитель введёт пароль второй раз), или открытое окно убито как обход.
func TestGuardHandoverReplacesOpenWindow(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	tray := h.trayPeer(childSession, childSID)
	h.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}, nil)
	first := h.launcher.aliveOf(ProcWindow, childSession)[0]
	h.guardLogin(goodPassword)
	h.ok(guardPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}, nil)
	if first.alive || !first.terminated {
		t.Fatal("старое окно не закрыто")
	}
	h.hk(2)
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("замена окна засчитана обходом: %+v", st)
	}
	if n := len(h.launcher.aliveOf(ProcWindow, childSession)); n != 1 {
		t.Fatalf("окон после замены %d", n)
	}
}

// Ловит: просроченная привязка стража остаётся в памяти (мелочь, но привязки копятся) или
// продолжает действовать до обслуживания.
func TestExpiredBindingIsRemoved(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	h.guardLogin(goodPassword)
	h.hk(31)
	if len(h.svc.binds) != 0 {
		t.Fatalf("привязки: %v", h.svc.binds)
	}
}

// Ловит: две родительские сессии видели сообщение, одна закончилась, а сообщение погасло для другой.
func TestNoticeSurvivesWhileAnotherSessionSawIt(t *testing.T) {
	h := newHarness(t, configured(), withRecovered("pcblock.db.broken-1"))
	h.start()
	a := h.login(adminPeer())
	p2 := elevatedPeer()
	b := h.login(p2)
	h.ok(adminPeer(), a, ipc.CmdGetToday, nil, nil)
	h.ok(p2, b, ipc.CmdGetToday, nil, nil)
	h.ok(adminPeer(), a, ipc.CmdLogout, nil, nil)
	if v, _, _ := h.store.GetSetting(settingDBNotice); v == "" {
		t.Fatal("сообщение погашено, хотя вторая сессия его ещё показывает")
	}
	h.ok(p2, b, ipc.CmdLogout, nil, nil)
	if v, _, _ := h.store.GetSetting(settingDBNotice); v != "" {
		t.Fatalf("сообщение осталось: %q", v)
	}
}

// Ловит: RunLoop не тикает по периоду тестового режима или не останавливается по отмене.
func TestRunLoopTicksAndStops(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 200}))
	h.cfg.RealNow = time.Now
	svc, err := New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.svc = svc
	if err := svc.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	svc.OnSession(SessionLogon, childSession, time.Now())
	if got := svc.TickInterval(); got != 5*time.Millisecond {
		t.Fatalf("период тика %v", got)
	}
	h.svc.Handle(guardPeer(), ipc.Msg{Cmd: ipc.CmdGuardPoll, Payload: ipc.Payload(ipc.GuardPollRequest{Sample: activeSample})}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { svc.RunLoop(ctx); close(done) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunLoop не остановился")
	}
	if got := h.usedS(); got < 3 {
		t.Fatalf("за 150 мс тиков по 5 мс засчитано %d с", got)
	}
}

// Ловит: ошибка выхода из сеанса возвращает технический текст.
func TestGuardLogoffFailureIsFriendly(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	bad := &logoffFailLauncher{fakeLauncher: h.launcher}
	h.svc.cfg.Launcher = bad
	rep := h.fail(guardPeer(), "", ipc.CmdGuardLogoff, nil)
	if rep.Error != textRetry || strings.Contains(rep.Error, "logoff") {
		t.Fatalf("ответ %q", rep.Error)
	}
}

// Ловит: ошибка выключения или смены пользователя возвращает ребёнку технический текст WTS.
func TestGuardExitFailuresAreFriendly(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.svc.cfg.Launcher = &exitsFailLauncher{fakeLauncher: h.launcher}
	for _, cmd := range []string{ipc.CmdGuardShutdown, ipc.CmdGuardSwitchUser} {
		rep := h.fail(guardPeer(), "", cmd, nil)
		if rep.Error != textRetry || strings.Contains(rep.Error, "WTS") || strings.Contains(rep.Error, "отказано") {
			t.Fatalf("%s: ответ %q", cmd, rep.Error)
		}
	}
	if !strings.Contains(h.logText(), "отказано") {
		t.Errorf("причина сбоя не попала в журнал работы:\n%s", h.logText())
	}
}

type exitsFailLauncher struct{ *fakeLauncher }

func (l *exitsFailLauncher) Shutdown() error {
	return errors.New("InitiateSystemShutdownEx: отказано")
}

func (l *exitsFailLauncher) SwitchUser(uint32) error {
	return errors.New("WTSDisconnectSession: отказано")
}

// Ловит: срок автосмены из тестового режима не доходит до стража через Hello или попадает в
// обычный режим.
func TestHelloCarriesAutoSwitchOnlyInTestMode(t *testing.T) {
	h := newHarness(t, configured(), withTestMode(TestMode{ClockSpeed: 1, LockAutoSwitchS: 20}))
	h.start()
	var hr ipc.HelloReply
	h.ok(guardPeer(), "", ipc.CmdHello, nil, &hr)
	if hr.AutoSwitchS != 20 {
		t.Fatalf("AutoSwitchS = %d, ждали 20", hr.AutoSwitchS)
	}
	h2 := newHarness(t, configured(), func(h *harness) { h.cfg.Test = TestMode{LockAutoSwitchS: 20} })
	h2.start()
	var hr2 ipc.HelloReply
	h2.ok(guardPeer(), "", ipc.CmdHello, nil, &hr2)
	if hr2.AutoSwitchS != 0 {
		t.Fatalf("вне тестового режима AutoSwitchS = %d, ждали 0", hr2.AutoSwitchS)
	}
}

type logoffFailLauncher struct{ *fakeLauncher }

func (l *logoffFailLauncher) Logoff(uint32) error {
	return errors.New("WTSLogoffSession: отказано")
}

// Ловит: отметка первого входа не записывается, а ошибка записи молчит.
func TestFirstRunSeenStoredPerChild(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120))
	h.start()
	h.ok(childPeer(), "", ipc.CmdFirstRunSeen, nil, nil)
	if v, _, _ := h.store.GetSetting(settingFirstRunPrefix + "1"); v != "1" {
		t.Fatal("отметка не записана")
	}
	if v, _, _ := h.store.GetSetting(settingFirstRunPrefix + "2"); v != "" {
		t.Fatal("отметка записана чужому ребёнку")
	}
	h.ok(otherPeer(), "", ipc.CmdFirstRunSeen, nil, nil) // не ребёнок: ничего не меняется
}

// Ловит: неразобранный запрос выполняется с нулевыми значениями (SaveLimits с мусором
// сбросил бы лимиты в «без лимита»).
func TestGarbagePayloadIsBadRequest(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	for _, cmd := range []string{ipc.CmdSaveLimits, ipc.CmdAddExtra, ipc.CmdUndoExtra, ipc.CmdClearBlock, ipc.CmdGetChildRules,
		ipc.CmdSetChildren, ipc.CmdListEvents, ipc.CmdChangePassword, ipc.CmdSaveHelperState} {
		rep := h.svc.Handle(adminPeer(), ipc.Msg{Cmd: cmd, Token: tok, Payload: []byte(`[1]`)}, h.real.Now())
		if rep.OK || rep.Error != ipc.TextBadRequest {
			t.Errorf("%s: %+v", cmd, rep)
		}
	}
	for cmd, who := range map[string]ipc.Peer{
		ipc.CmdSetup: elevatedPeer(), ipc.CmdResetPassword: elevatedPeer(), ipc.CmdUninstallCheck: elevatedPeer(),
		ipc.CmdOpenWindow: childPeer(), ipc.CmdWindowClosing: childPeer(), ipc.CmdLogin: adminPeer(),
		ipc.CmdTestClock: adminPeer(), ipc.CmdTestGuard: adminPeer(), ipc.CmdGuardPoll: guardPeer(),
	} {
		if cmd == ipc.CmdSetup {
			h2 := newHarness(t)
			h2.start()
			rep := h2.svc.Handle(who, ipc.Msg{Cmd: cmd, Payload: []byte(`[1]`)}, h2.real.Now())
			if rep.OK || rep.Error != ipc.TextBadRequest {
				t.Errorf("%s: %+v", cmd, rep)
			}
			continue
		}
		hh := h
		if cmd == ipc.CmdTestClock || cmd == ipc.CmdTestGuard {
			hh = newHarness(t, withTestMode(TestMode{ClockSpeed: 1}))
			hh.start()
		}
		rep := hh.svc.Handle(who, ipc.Msg{Cmd: cmd, Payload: []byte(`[1]`)}, hh.real.Now())
		if rep.OK || rep.Error != ipc.TextBadRequest {
			t.Errorf("%s: %+v", cmd, rep)
		}
	}
}

// Ловит: учёт дня ребёнка читается с ошибкой, а ядро стартует с чужим днём.
func TestDayStateLoadErrorStartsFreshDay(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.cfg.Store = &loadFailStore{fakeStore: h.store}
	h.cfg.Journal = nil
	h.svc, _ = New(h.cfg)
	h.start()
	if st := h.status(); st.Day != "2026-10-19" || st.UsedS != 0 {
		t.Fatalf("статус: %+v", st)
	}
}

type loadFailStore struct{ *fakeStore }

func (l *loadFailStore) LoadDayState(int64, string) (core.DayState, error) {
	return core.DayState{}, errFake
}

// Ловит: каталог данных не задан, а служба пишет память о процессах в текущую папку.
func TestNoDataDirNoProcessMemory(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), func(h *harness) { h.cfg.DataDir = "" })
	h.start()
	h.childLogsIn()
	if h.svc.procsPath() != "" {
		t.Fatal("путь памяти о процессах задан без папки данных")
	}
}
