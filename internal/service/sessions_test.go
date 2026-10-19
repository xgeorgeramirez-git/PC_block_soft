package service

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// adultLogsIn: взрослый вошёл в сеанс 1 вместе с ребёнком в сеансе 2.
func (h *harness) bothLogIn() {
	h.t.Helper()
	h.launcher.setSessions(
		SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID},
		SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID},
	)
	h.svc.OnSession(SessionLogon, 1, h.real.Now())
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
}

func (h *harness) hk(n int) {
	h.t.Helper()
	for i := 0; i < n; i++ {
		h.real.Advance(time.Second)
		h.clock.Advance(time.Second)
		h.svc.Housekeep(h.real.Now())
	}
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// --- значок и страж в сеансах (FR-3, Ruling 12, 15) ---------------------------------------------

// Ловит: значок запускается не в каждом сеансе, страж запускается не только в сеансе ребёнка
// (у взрослого экран блокировки недопустим, R1), ключи запуска и номер сеанса неверны.
func TestTrayEverywhereGuardOnlyForChild(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.bothLogIn()
	if n := len(h.launcher.launched(ProcTray, 1)); n != 1 {
		t.Errorf("значков у взрослого %d", n)
	}
	if n := len(h.launcher.launched(ProcGuard, 1)); n != 0 {
		t.Errorf("стражей у взрослого %d", n)
	}
	g := h.launcher.launched(ProcGuard, childSession)
	tr := h.launcher.launched(ProcTray, childSession)
	if len(g) != 1 || len(tr) != 1 {
		t.Fatalf("у ребёнка стражей %d, значков %d", len(g), len(tr))
	}
	if !hasArg(g[0].args, "--guard") || !hasArg(g[0].args, "--session=2") || !hasArg(tr[0].args, "--tray") || !hasArg(tr[0].args, "--session=2") {
		t.Fatalf("ключи запуска: %v и %v", g[0].args, tr[0].args)
	}
}

// Ловит: при каждом обслуживании запускается ещё один страж или значок (Ruling 15: в сеансе
// ровно один страж и один значок).
func TestExactlyOneGuardAndTrayPerSession(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.hk(20)
	h.svc.OnSession(SessionUnlock, childSession, h.real.Now())
	if g, tr := len(h.launcher.launched(ProcGuard, 0)), len(h.launcher.launched(ProcTray, 0)); g != 1 || tr != 1 {
		t.Fatalf("запущено стражей %d, значков %d", g, tr)
	}
}

// Ловит: служба запускает части до своего старта или после остановки; события сеанса
// до Start ничего не запускают.
func TestSessionEventBeforeStartDoesNothing(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	if len(h.launcher.launches) != 0 {
		t.Fatal("процессы запущены до Start")
	}
}

// Ловит: ошибка списка сеансов роняет службу или стирает запущенное; процессы живут дальше.
func TestSessionListErrorKeepsProcesses(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.launcher.listErr = errFake
	h.hk(5)
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
		t.Fatal("при ошибке списка страж завершён")
	}
	if !strings.Contains(h.logText(), "список сеансов") {
		t.Errorf("ошибка не записана в журнал работы:\n%s", h.logText())
	}
}

// Ловит: смена пользователя в том же номере сеанса оставляет стража прежнего ребёнка.
func TestUserChangeInSameSessionRestartsParts(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Анна`, SID: parentSID})
	h.hk(1)
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 0 {
		t.Fatal("страж ребёнка остался в сеансе взрослого")
	}
	if len(h.launcher.aliveOf(ProcTray, childSession)) != 1 {
		t.Fatal("в сеансе взрослого нет значка")
	}
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("смена пользователя привела к блокировке: %+v", st)
	}
}

// --- защита от обхода (FR-27, R17в, Ruling 15, 19) ---------------------------------------------------

// Ловит: убитый страж не возвращается не позже следующего обслуживания (FR-27: ≤ 3 с), не
// записывается обход и экран «Попытка обойти защиту» не ставится.
func TestGuardKilledIsRelaunchedAndBlocks(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	old := h.launcher.aliveOf(ProcGuard, childSession)[0]
	h.launcher.kill(old)
	h.hk(1)
	now := h.launcher.aliveOf(ProcGuard, childSession)
	if len(now) != 1 || now[0] == old {
		t.Fatalf("страж не перезапущен: %+v", now)
	}
	st := h.status()
	if st.Lock != ipc.LockBypass || st.Detail != detailGuardClosed {
		t.Fatalf("статус после гибели стража: %+v", st)
	}
	evs := h.store.eventsOf("bypass")
	if len(evs) != 1 || evs[0].Details["subtype"] != "guard_closed" || evs[0].ChildID != h.childID(childSID) {
		t.Fatalf("события обхода: %+v", evs)
	}
	cs, _ := h.store.ListChildren()
	if cs[0].BypassDetail == "" {
		t.Fatal("обход не записан в базу")
	}
}

// Ловит: значок, убитый в сеансе ребёнка, не считается обходом.
func TestTrayKilledInChildSessionIsBypass(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.launcher.kill(h.launcher.aliveOf(ProcTray, childSession)[0])
	h.hk(1)
	if len(h.launcher.aliveOf(ProcTray, childSession)) != 1 {
		t.Fatal("значок не перезапущен")
	}
	if st := h.status(); st.Lock != ipc.LockBypass || st.Detail != detailTrayClosed {
		t.Fatalf("статус: %+v", st)
	}
}

// Ловит: закрытый значок взрослого блокирует ребёнка или не возвращается (у взрослого нет
// обхода, но значок нужен: «Настройки для родителя»).
func TestTrayKilledInAdultSessionRestartedWithoutBypass(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.bothLogIn()
	h.launcher.kill(h.launcher.aliveOf(ProcTray, 1)[0])
	h.hk(1)
	if len(h.launcher.aliveOf(ProcTray, 1)) != 1 {
		t.Fatal("значок взрослого не вернулся")
	}
	if n := len(h.store.eventsOf("bypass")); n != 0 {
		t.Fatalf("событий обхода %d", n)
	}
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("ребёнок заблокирован из-за значка взрослого: %+v", st)
	}
}

// Ловит: штатное закрытие значка при выходе из сеанса засчитывается обходом (FR-27: штатное
// закрытие обходом не считается); значок возвращается раньше чем через 10 с.
func TestTrayClosingIsNotBypass(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	tray := h.trayPeer(childSession, childSID)
	h.ok(tray, "", ipc.CmdTrayClosing, nil, nil)
	h.launcher.kill(h.launcher.aliveOf(ProcTray, childSession)[0])
	h.hk(1)
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("штатное закрытие значка засчитано обходом: %+v", st)
	}
	if len(h.launcher.aliveOf(ProcTray, childSession)) != 0 {
		t.Fatal("значок вернулся сразу после штатного закрытия (сеанс может заканчиваться)")
	}
	h.hk(10)
	if len(h.launcher.aliveOf(ProcTray, childSession)) != 1 {
		t.Fatal("значок не вернулся, хотя сеанс остался")
	}
}

// Ловит: ребёнок присылает TrayClosing от чужого процесса и убивает значок без последствий.
func TestTrayClosingFromForeignProcessIsIgnored(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.ok(childPeer(), "", ipc.CmdTrayClosing, nil, nil) // PID 500, не процесс значка
	h.launcher.kill(h.launcher.aliveOf(ProcTray, childSession)[0])
	h.hk(1)
	if st := h.status(); st.Lock != ipc.LockBypass {
		t.Fatalf("обман TrayClosing сработал: %+v", st)
	}
}

// Ловит: выход ребёнка из сеанса засчитывается обходом (процессы исчезают вместе с сеансом).
func TestSessionGoneIsNotBypass(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	g := h.launcher.aliveOf(ProcGuard, childSession)[0]
	tr := h.launcher.aliveOf(ProcTray, childSession)[0]
	h.launcher.kill(g)
	h.launcher.kill(tr)
	h.launcher.setSessions()
	h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
	h.hk(2)
	if n := len(h.store.eventsOf("bypass")); n != 0 {
		t.Fatalf("выход из сеанса записан обходом: %d", n)
	}
	h.childLogsIn()
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("после нового входа блокировка: %+v", st)
	}
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
		t.Fatal("при новом входе страж не запущен")
	}
}

// Ловит: неудачные запуски повторяются каждую секунду без пауз (нагрузка и засорение журнала):
// паузы 1, 2, 4 с; страж в итоге запускается.
func TestLaunchFailuresBackOff(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.launcher.failNext = 4 // значок и страж: по два срыва
	h.childLogsIn()
	attempts := 0
	for i := 0; i < 10; i++ {
		h.hk(1)
		attempts = len(h.launcher.launches)
	}
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 || len(h.launcher.aliveOf(ProcTray, childSession)) != 1 {
		t.Fatalf("части не запущены за 10 с, запусков %d", attempts)
	}
}

// Ловит: бесконечный цикл падений (процесс умирает сразу после запуска) перезапускается
// каждую секунду вечно: после трёх быстрых падений пауза растёт.
func TestCrashLoopBacksOff(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	for i := 0; i < 8; i++ {
		for _, p := range h.launcher.aliveOf(ProcGuard, childSession) {
			h.launcher.kill(p)
		}
		h.hk(1)
	}
	if n := len(h.launcher.launched(ProcGuard, childSession)); n > 6 {
		t.Fatalf("за 8 секунд падений запущено %d стражей, ждали не больше 6 (пауза растёт)", n)
	}
}

// --- остановка и память о процессах (Ruling 15) -----------------------------------------------------

// Ловит: остановка службы оставляет экран блокировки без службы (NFR-10): все стражи, значки
// и окна должны быть завершены, память о них очищена.
func TestShutdownTerminatesAllProcesses(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.bothLogIn()
	tray := h.trayPeer(childSession, childSID)
	h.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}, nil)
	if !fileExists(h.procsFile()) {
		t.Fatal("память о процессах не записана")
	}
	h.svc.Shutdown(h.real.Now())
	for _, l := range h.launcher.launches {
		if l.proc.alive {
			t.Errorf("процесс %v в сеансе %d жив после остановки службы", l.kind, l.session)
		}
		if !l.proc.terminated || !l.proc.closed {
			t.Errorf("процесс %v: завершён=%v, ручка закрыта=%v", l.kind, l.proc.terminated, l.proc.closed)
		}
	}
	if fileExists(h.procsFile()) {
		t.Fatal("память о процессах осталась после остановки")
	}
}

// Ловит: память о процессах не содержит PID и момент создания (убить по одному PID нельзя:
// он может совпасть с чужим процессом).
func TestProcessMemoryHasPIDAndStamp(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	recs := readProcRecords(h.procsFile())
	if len(recs) != 2 {
		t.Fatalf("записей %d: %+v", len(recs), recs)
	}
	for _, r := range recs {
		if r.PID == 0 || r.Stamp != uint64(r.PID)*1000 || r.Session != childSession || (r.Role != "guard" && r.Role != "tray") {
			t.Errorf("запись %+v", r)
		}
	}
}

// Ловит: после падения службы (S27) стражи и значки прошлого запуска остаются в сеансе вместе
// с новыми, а чужой процесс с повторившимся PID завершается.
func TestStaleProcessesFromPreviousRunAreKilled(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	if err := writeProcRecords(h.procsFile(), []procRecord{
		{Role: "guard", Session: 2, PID: 41, Stamp: 4100},
		{Role: "tray", Session: 2, PID: 42, Stamp: 4200},
	}); err != nil {
		t.Fatal(err)
	}
	h.launcher.stale[41] = 4100 // тот же процесс
	h.launcher.stale[42] = 9999 // PID повторился, это уже чужой процесс
	h.start()
	if len(h.launcher.killed) != 1 || h.launcher.killed[0] != 41 {
		t.Fatalf("завершены %v, ждали только 41", h.launcher.killed)
	}
	if fileExists(h.procsFile()) {
		if recs := readProcRecords(h.procsFile()); len(recs) != 0 {
			t.Fatalf("старые записи не очищены: %+v", recs)
		}
	}
}

// --- окна и родительская сессия со стража (Ruling 19) -----------------------------------------------

func (h *harness) guardLogin(pw string) ipc.Reply {
	return h.do(guardPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: pw})
}

// Ловит: страж получает токен (модель стража его не хранит) или верный пароль стража не
// открывает короткое окно действий.
func TestGuardLoginBindsWithoutToken(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	rep := h.guardLogin(goodPassword)
	var lr ipc.LoginReply
	if err := rep.Into(&lr); err != nil || lr.Token != "" || lr.IdleTimeoutS != 30 {
		t.Fatalf("ответ %+v, %v", lr, err)
	}
	if want := h.real.Now().Add(30 * time.Second).UnixMilli(); lr.ExpiresMs != want {
		t.Fatalf("окно до %d, ждали %d", lr.ExpiresMs, want)
	}
}

// Ловит: неверный пароль на экране блокировки открывает окно действий или не считается в
// журнале ребёнка сеанса.
func TestGuardWrongPasswordDoesNotBind(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	rep := h.guardLogin("не тот")
	if rep.OK || deniedOf(t, rep).AttemptsLeft != 4 {
		t.Fatalf("ответ %+v", rep)
	}
	h.fail(guardPeer(), "", ipc.CmdAddExtra, ipc.AddExtraRequest{Minutes: 15})
	if evs := h.store.eventsOf("password_failed"); len(evs) != 1 || evs[0].ChildID != h.childID(childSID) {
		t.Fatalf("журнал: %+v", evs)
	}
}

// Ловит: после одного ввода пароля страж делает два действия (эскалация T10a: привязка на
// одно действие), или действует после 30 с, или трогает чужого ребёнка.
func TestGuardBindingOneActionThirtySeconds(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1), withChild(child2SID, "Коля", 1), configured())
	h.start()
	h.childLogsIn()
	h.run(70, activeSample)
	own := h.childID(childSID)

	h.guardLogin(goodPassword)
	h.fail(guardPeer(), "", ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: h.childID(child2SID), Minutes: 15}) // чужой ребёнок
	var ar ipc.AddExtraReply
	h.ok(guardPeer(), "", ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: 0, Minutes: 15}, &ar)
	if ar.Card.ChildID != own || ar.Card.Status.Lock != ipc.LockNone {
		t.Fatalf("действие стража: %+v", ar.Card)
	}
	if rep := h.fail(guardPeer(), "", ipc.CmdAddExtra, ipc.AddExtraRequest{Minutes: 15}); rep.Error != textNeedParent {
		t.Fatalf("второе действие: %q", rep.Error)
	}

	h.guardLogin(goodPassword)
	h.real.Advance(29 * time.Second)
	h.ok(guardPeer(), "", ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: own, Minutes: 15}, nil)
	h.guardLogin(goodPassword)
	h.real.Advance(31 * time.Second)
	h.fail(guardPeer(), "", ipc.CmdAddExtra, ipc.AddExtraRequest{Minutes: 15})
}

// Ловит: «Снять блокировку» со стража не работает или работает без привязки.
func TestGuardClearBlockNeedsBinding(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass})
	h.fail(guardPeer(), "", ipc.CmdClearBlock, ipc.ChildRequest{})
	h.guardLogin(goodPassword)
	var cr ipc.CardReply
	h.ok(guardPeer(), "", ipc.CmdClearBlock, ipc.ChildRequest{}, &cr)
	if cr.Card.Status.Lock != ipc.LockNone {
		t.Fatalf("блокировка не снята: %+v", cr.Card)
	}
	if rep := h.fail(guardPeer(), "", ipc.CmdClearBlock, ipc.ChildRequest{}); rep.Error != textNeedParent {
		t.Fatalf("второе снятие по одному паролю: %q", rep.Error)
	}
}

// Ловит: окно родителя со стража открывается без пароля, без передачи сессии, или страж
// продолжает показывать экран поверх окна; конец сессии не возвращает экран.
func TestGuardOpensParentWindowWithHandover(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	if rep := h.fail(guardPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}); rep.Error == "" {
		t.Fatal("без пароля окно открылось")
	}
	h.guardLogin(goodPassword)
	h.fail(guardPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowMyStats}) // со стража только окно родителя
	h.guardLogin(goodPassword)
	var ow ipc.OpenWindowReply
	h.ok(guardPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}, &ow)
	wins := h.launcher.launched(ProcWindow, childSession)
	if !ow.Opened || len(wins) != 1 || !hasArg(wins[0].args, "--parent") {
		t.Fatalf("окно: %+v, запуски %+v", ow, wins)
	}
	var handover string
	for _, a := range wins[0].args {
		handover = strings.TrimPrefix(a, "--handover=")
	}
	if !strings.HasPrefix(handover, "tok-") {
		t.Fatalf("токен не передан окну: %v", wins[0].args)
	}
	win := ipc.Peer{PID: wins[0].proc.pid, SessionID: childSession, SID: childSID}
	h.ok(win, handover, ipc.CmdGetToday, nil, nil)
	h.fail(childPeer(), handover, ipc.CmdGetToday, nil)
	h.fail(guardPeer(), handover, ipc.CmdGetToday, nil)

	rep := h.poll(activeSample)
	if len(rep.Commands) != 1 || rep.Commands[0].Kind != ipc.GuardCmdHideLock {
		t.Fatalf("пока сессия родителя открыта, страж должен прятать экран: %+v", rep.Commands)
	}
	h.ok(win, handover, ipc.CmdLogout, nil, nil)
	if rep := h.poll(activeSample); len(rep.Commands) != 0 {
		t.Fatalf("после выхода экран всё ещё спрятан: %+v", rep.Commands)
	}
}

// Ловит: сессия, переданная окну, не кончается через 10 мин или при закрытии окна (R19:
// сессия заканчивается при закрытии окна), и экран блокировки не возвращается.
func TestHandoverSessionEndsWithWindowAndTimeout(t *testing.T) {
	open := func(t *testing.T) (*harness, ipc.Peer, string) {
		h := newHarness(t, withChild(childSID, "Миша", 120), configured())
		h.start()
		h.childLogsIn()
		h.guardLogin(goodPassword)
		h.ok(guardPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}, nil)
		w := h.launcher.launched(ProcWindow, childSession)[0]
		var tok string
		for _, a := range w.args {
			tok = strings.TrimPrefix(a, "--handover=")
		}
		return h, ipc.Peer{PID: w.proc.pid, SessionID: childSession, SID: childSID}, tok
	}
	h, win, tok := open(t)
	h.ok(win, "", ipc.CmdWindowClosing, ipc.WindowClosingRequest{Kind: ipc.WindowParent}, nil)
	h.fail(win, tok, ipc.CmdGetToday, nil)
	if rep := h.poll(activeSample); len(rep.Commands) != 0 {
		t.Fatalf("после закрытия окна: %+v", rep.Commands)
	}

	h, win, tok = open(t)
	h.real.Advance(10 * time.Minute)
	h.svc.Housekeep(h.real.Now())
	h.fail(win, tok, ipc.CmdGetToday, nil)
	if rep := h.poll(activeSample); len(rep.Commands) != 0 {
		t.Fatalf("после 10 минут: %+v", rep.Commands)
	}
}

// Ловит: гибель окна родителя оставляет родительскую сессию (токен живёт без окна).
func TestParentWindowDeathEndsItsSession(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	h.guardLogin(goodPassword)
	h.ok(guardPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}, nil)
	w := h.launcher.launched(ProcWindow, childSession)[0]
	h.launcher.kill(w.proc)
	h.hk(1)
	if rep := h.poll(activeSample); len(rep.Commands) != 0 {
		t.Fatalf("после гибели окна экран спрятан: %+v", rep.Commands)
	}
}

// Ловит: окно, закрытое не кнопкой, в сеансе ребёнка не открывается заново и не считается
// обходом (Ruling 19); закрытое штатно считается обходом.
func TestWindowKilledInChildSessionReopensAndBlocks(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	tray := h.trayPeer(childSession, childSID)
	h.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowMyStats}, nil)
	h.launcher.kill(h.launcher.aliveOf(ProcWindow, childSession)[0])
	h.hk(1)
	if len(h.launcher.aliveOf(ProcWindow, childSession)) != 1 {
		t.Fatal("окно не открыто заново")
	}
	if st := h.status(); st.Lock != ipc.LockBypass || st.Detail != detailWindowClosed {
		t.Fatalf("статус: %+v", st)
	}

	h2 := newHarness(t, withChild(childSID, "Миша", 120))
	h2.start()
	h2.childLogsIn()
	tray = h2.trayPeer(childSession, childSID)
	h2.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowMyStats}, nil)
	win := ipc.Peer{PID: h2.launcher.aliveOf(ProcWindow, childSession)[0].pid, SessionID: childSession, SID: childSID}
	h2.ok(win, "", ipc.CmdWindowClosing, ipc.WindowClosingRequest{Kind: ipc.WindowMyStats}, nil)
	h2.launcher.kill(h2.launcher.aliveOf(ProcWindow, childSession)[0])
	h2.hk(1)
	if len(h2.launcher.aliveOf(ProcWindow, childSession)) != 0 {
		t.Fatal("штатно закрытое окно открыто заново")
	}
	if st := h2.status(); st.Lock != ipc.LockNone {
		t.Fatalf("штатное закрытие окна засчитано обходом: %+v", st)
	}
}

// Ловит: одно и то же окно запускается второй раз; повторный запрос «открыть окно» плодит процессы.
func TestOpenWindowTwiceLaunchesOnce(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	tray := h.trayPeer(childSession, childSID)
	for i := 0; i < 3; i++ {
		var ow ipc.OpenWindowReply
		h.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent}, &ow)
		if !ow.Opened {
			t.Fatal("окно не открыто")
		}
	}
	if n := len(h.launcher.launched(ProcWindow, 0)); n != 1 {
		t.Fatalf("запущено окон %d", n)
	}
}

// Ловит: при ошибке запуска окна ответ технический или успешный.
func TestOpenWindowLaunchFailure(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	tray := h.trayPeer(childSession, childSID)
	h.launcher.launchErr = errFake
	rep := h.fail(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent})
	if rep.Error != textRetry {
		t.Fatalf("ответ %q", rep.Error)
	}
}

// --- политика Диспетчера задач (FR-29) --------------------------------------------------------------

// Ловит: запрет Диспетчера задач ставится не при старте и не при входе ребёнка; ошибка
// политики останавливает службу.
func TestPolicySetAtStartAndLogon(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	if !h.policy.disabled(childSID) {
		t.Fatal("запрет не поставлен при старте")
	}
	h.policy.state[childSID] = false
	h.childLogsIn()
	if !h.policy.disabled(childSID) {
		t.Fatal("запрет не поставлен при входе ребёнка")
	}
	if h.policy.disabled(parentSID) {
		t.Fatal("запрет поставлен взрослому")
	}
}

func TestPolicyErrorDoesNotStopService(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.policy.err = errFake
	h.start()
	h.childLogsIn()
	h.run(3, activeSample)
	if h.usedS() != 3 {
		t.Fatal("учёт остановился из-за ошибки политики")
	}
	if !strings.Contains(h.logText(), "Диспетчера задач") {
		t.Errorf("ошибка политики не записана:\n%s", h.logText())
	}
}

// --- тестовый режим (Ruling 4, 5) --------------------------------------------------------------------------

// Ловит: действие стражу теряется (TestGuard) или уходит не тому сеансу; незнакомое действие принимается.
func TestTestGuardQueuesActionForTheGuard(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}))
	h.start()
	h.childLogsIn()
	h.ok(adminPeer(), "", ipc.CmdTestGuard, ipc.TestGuardRequest{Action: ipc.TestGuardAction{Action: ipc.TestClick, Target: ipc.ButtonParentPassword}}, nil)
	h.ok(adminPeer(), "", ipc.CmdTestGuard, ipc.TestGuardRequest{SessionID: childSession, Action: ipc.TestGuardAction{Action: ipc.TestType, Target: "password", Text: "x"}}, nil)
	rep := h.poll(activeSample)
	if len(rep.Commands) != 2 || rep.Commands[0].Test == nil || rep.Commands[0].Test.Action != ipc.TestClick || rep.Commands[1].Test.Text != "x" {
		t.Fatalf("команды: %+v", rep.Commands)
	}
	if rep := h.poll(activeSample); len(rep.Commands) != 0 {
		t.Fatalf("повтор: %+v", rep.Commands)
	}
	h.fail(adminPeer(), "", ipc.CmdTestGuard, ipc.TestGuardRequest{Action: ipc.TestGuardAction{Action: "взорвать"}})
	h.fail(adminPeer(), "", ipc.CmdTestGuard, ipc.TestGuardRequest{SessionID: 77, Action: ipc.TestGuardAction{Action: ipc.TestEsc}})
}

// Ловит: при нескольких стражах команда без сеанса уходит наугад; без стража принимается молча.
func TestTestGuardSessionAmbiguity(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120), withTestMode(TestMode{ClockSpeed: 1}))
	h.start()
	if rep := h.fail(adminPeer(), "", ipc.CmdTestGuard, ipc.TestGuardRequest{Action: ipc.TestGuardAction{Action: ipc.TestEsc}}); rep.Error != textNoGuard {
		t.Fatalf("без стражей: %q", rep.Error)
	}
	h.child2LogsIn()
	h.childLogsInBoth()
	if rep := h.fail(adminPeer(), "", ipc.CmdTestGuard, ipc.TestGuardRequest{Action: ipc.TestGuardAction{Action: ipc.TestEsc}}); rep.Error != textManyGuards {
		t.Fatalf("два стража: %q", rep.Error)
	}
	h.ok(adminPeer(), "", ipc.CmdTestGuard, ipc.TestGuardRequest{SessionID: child2Session, Action: ipc.TestGuardAction{Action: ipc.TestEsc}}, nil)
}

func (h *harness) childLogsInBoth() {
	h.launcher.setSessions(
		SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID},
		SessionInfo{ID: child2Session, User: `PC\Коля`, SID: child2SID},
	)
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
}

// Ловит: проверяющий не видит, что показывает страж (GuardState), или видит вид другого сеанса.
func TestGuardStateRoundTrip(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}))
	h.start()
	h.childLogsIn()
	var empty ipc.GuardStateReply
	rep := h.do(adminPeer(), "", ipc.CmdGuardState, nil)
	_ = rep.Into(&empty)
	if !rep.OK || strings.Contains(string(rep.Payload), "null") {
		t.Fatalf("пустой вид: %s", rep.Payload)
	}
	view := ipc.GuardStateReply{LockShown: true, LockKind: ipc.LockTimeUp, Buttons: []string{ipc.ButtonParentPassword, ipc.ButtonLogoff},
		Warnings: []ipc.ShownWarning{{Threshold: 1, Text: "Осталась 1 минута"}}, Fields: []ipc.GuardField{{ID: "password", Text: ""}}}
	h.ok(guardPeer(), "", ipc.CmdGuardState, view, nil)
	var got ipc.GuardStateReply
	h.ok(elevatedPeer(), "", ipc.CmdGuardState, nil, &got)
	if !got.LockShown || got.LockKind != ipc.LockTimeUp || len(got.Buttons) != 2 || got.Warnings[0].Threshold != 1 {
		t.Fatalf("вид: %+v", got)
	}
}

// Ловит: пауза часов на настоящих часах (без ускорения их остановить нельзя) притворяется удачной.
func TestTestClockNeedsPausableClock(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}), func(h *harness) { h.cfg.Clock = systemClockForTest{} })
	h.start()
	rep := h.fail(adminPeer(), "", ipc.CmdTestClock, ipc.TestClockRequest{Paused: true})
	if rep.Error != textNoPause {
		t.Fatalf("ответ %q", rep.Error)
	}
}

// Ловит: ответ TestClock без скорости и времени ядра.
func TestTestClockReply(t *testing.T) {
	h := newHarness(t, withTestMode(TestMode{ClockSpeed: 60}))
	h.start()
	var cr ipc.TestClockReply
	h.ok(adminPeer(), "", ipc.CmdTestClock, ipc.TestClockRequest{Paused: true}, &cr)
	if !cr.Paused || cr.Speed != 60 || cr.NowMs != h.clock.Now().UnixMilli() {
		t.Fatalf("ответ %+v", cr)
	}
}

// Ловит: Hello не сообщает версию, тестовый режим и «настроена».
func TestHello(t *testing.T) {
	h := newHarness(t, configured(), withTestMode(TestMode{ClockSpeed: 1}))
	h.start()
	var hr ipc.HelloReply
	h.ok(childPeer(), "", ipc.CmdHello, nil, &hr)
	if hr.Protocol != ipc.ProtocolVersion || hr.Version != "0.1.0-test" || !hr.TestMode || !hr.Configured {
		t.Fatalf("Hello: %+v", hr)
	}
}

// Ловит: выход из учётной записи по просьбе стража не доходит до WTSLogoffSession своего сеанса.
func TestGuardLogoffCallsLauncherForOwnSession(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.ok(guardPeer(), "", ipc.CmdGuardLogoff, nil, nil)
	if len(h.launcher.logoffs) != 1 || h.launcher.logoffs[0] != childSession {
		t.Fatalf("выходы из сеансов: %v", h.launcher.logoffs)
	}
	h.launcher.logoffs = nil
	h.fail(systemOtherPeer(), "", ipc.CmdGuardLogoff, nil)
	if len(h.launcher.logoffs) != 0 {
		t.Fatal("выход выполнен по просьбе постороннего")
	}
}

// Ловит: гонки при одновременных опросах и командах (go test -race): поток стража, поток
// окна и поток тиков работают одновременно.
func TestConcurrentCallsAreRaceFree(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			h.svc.Handle(guardPeer(), ipc.Msg{Cmd: ipc.CmdGuardPoll, Payload: ipc.Payload(ipc.GuardPollRequest{Sample: activeSample})}, h.real.Now())
		}
	}()
	for i := 0; i < 200; i++ {
		h.svc.Handle(adminPeer(), ipc.Msg{Cmd: ipc.CmdGetToday, Token: tok}, h.real.Now())
		h.svc.Tick(h.clock.Now())
		h.svc.Housekeep(h.real.Now())
	}
	<-done
}
