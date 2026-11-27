package service

import (
	"strings"
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Окно при входе в Windows (FR-46, S63, R-13): служба по событию входа открывает ребёнку
// «Моя статистика» (первый вход после установки — сначала окно FR-32), другой учётке — окно
// родителя с вводом пароля; экран блокировки главнее окна; одно окно на вход.

// logonHarness — служба с настроенным ребёнком Мишей (2 ч) и родителем Анной, без сеансов.
func logonHarness(t *testing.T, seen bool, opts ...hopt) *harness {
	t.Helper()
	h := newHarness(t, append([]hopt{withChild(childSID, "Миша", 120), configured()}, opts...)...)
	if seen {
		if err := h.store.SetSetting(settingFirstRunPrefix+itoa(int(h.childID(childSID))), "1"); err != nil {
			t.Fatal(err)
		}
	}
	h.start()
	return h
}

// childSessionUp: сеанс ребёнка уже есть, служба нашла его сверкой без события входа (как после
// перезапуска службы, R-13): окна при входе нет. Для проверок окон, открытых по запросу.
func (h *harness) childSessionUp() {
	h.t.Helper()
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.svc.Housekeep(h.real.Now())
}

// windowArgs — ключи всех окон, открытых в сеансе, по порядку.
func (h *harness) windowArgs(session uint32) [][]string {
	out := [][]string{}
	for _, r := range h.launcher.launched(ProcWindow, session) {
		out = append(out, r.args)
	}
	return out
}

func wantWindows(t *testing.T, h *harness, session uint32, want ...string) {
	t.Helper()
	got := h.windowArgs(session)
	if len(got) != len(want) {
		t.Fatalf("окна в сеансе %d = %v, ждали %v", session, got, want)
	}
	for i, w := range want {
		if strings.Join(got[i], " ") != w {
			t.Fatalf("окно %d в сеансе %d = %v, ждали %q (все: %v)", i, session, got[i], w, got)
		}
	}
}

// Ловит: вход ребёнка не открывает «Моя статистика» сразу, в том же вызове (не позже 10 с, FR-46).
func TestLogonChildOpensMyStats(t *testing.T) {
	h := logonHarness(t, true)
	h.childLogsIn()
	wantWindows(t, h, childSession, "--mystats")
	h.hk(3)
	wantWindows(t, h, childSession, "--mystats")
}

// Ловит: первый вход после установки сразу «Моя статистика» мимо окна FR-32; после «Понятно»
// окна статистики нет; второе «Понятно» открывает ещё окно.
func TestLogonFirstRunThenMyStats(t *testing.T) {
	h := logonHarness(t, false)
	h.childLogsIn()
	wantWindows(t, h, childSession, "--firstrun")
	h.ok(childPeer(), "", ipc.CmdFirstRunSeen, nil, nil)
	wantWindows(t, h, childSession, "--firstrun", "--mystats")
	h.ok(childPeer(), "", ipc.CmdFirstRunSeen, nil, nil)
	h.hk(2)
	wantWindows(t, h, childSession, "--firstrun", "--mystats")
	// Окно первого входа закрыто штатно после «Понятно»: нового первого входа и обхода нет.
	fr := h.launcher.launched(ProcWindow, childSession)[0]
	h.ok(ipc.Peer{PID: fr.proc.pid, SessionID: childSession, SID: childSID}, "", ipc.CmdWindowClosing,
		ipc.WindowClosingRequest{Kind: ipc.WindowFirstRun}, nil)
	h.launcher.kill(fr.proc)
	h.hk(10)
	wantWindows(t, h, childSession, "--firstrun", "--mystats")
	if evs := h.store.eventsOf("bypass"); len(evs) != 0 {
		t.Errorf("после «Понятно» записан обход: %+v", evs)
	}
}

// Ловит: «Понятно» без окна при входе (сеанс был до старта службы, R-13) открывает статистику.
func TestFirstRunSeenWithoutLogonNoMyStats(t *testing.T) {
	h := logonHarness(t, false)
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.hk(1) // сеанс найден сверкой, события входа не было (R-13)
	h.ok(childPeer(), "", ipc.CmdFirstRunSeen, nil, nil)
	wantWindows(t, h, childSession)
}

// Ловит: окно поверх экрана блокировки — время вышло, запретные часы (экран блокировки главнее).
func TestLogonLockScreenNoWindow(t *testing.T) {
	t.Run("время вышло", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 0), configured())
		h.start()
		h.childLogsIn()
		h.hk(12)
		wantWindows(t, h, childSession)
	})
	t.Run("запретные часы", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120), configured())
		h.store.SetSchedule(h.childID(childSID), []core.Interval{{Weekday: core.Monday, StartMin: 14 * 60, EndMin: 16 * 60}})
		h.start()
		h.childLogsIn()
		h.hk(12)
		wantWindows(t, h, childSession)
	})
}

// Ловит: вход учётки не ребёнка открывает окно с сессией (без пароля) или не открывает ничего.
func TestLogonParentAccount(t *testing.T) {
	h := logonHarness(t, true)
	h.launcher.setSessions(SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID})
	h.svc.OnSession(SessionLogon, 1, h.real.Now())
	wantWindows(t, h, 1, "--parent")
	h.svc.mu.Lock()
	n := len(h.svc.parents)
	h.svc.mu.Unlock()
	if n != 0 {
		t.Errorf("окно при входе получило сессию родителя: %d", n)
	}
}

// Ловит: программа не настроена (нет пароля) — окно родителя при входе открывало бы мастер.
func TestLogonNotConfiguredNoParentWindow(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.launcher.setSessions(SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID})
	h.svc.OnSession(SessionLogon, 1, h.real.Now())
	h.hk(12)
	wantWindows(t, h, 1)
}

// Ловит: разблокировка и возврат через «Смену пользователя» считаются входом.
func TestUnlockConnectNoWindow(t *testing.T) {
	h := logonHarness(t, true)
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.hk(1)
	for _, ev := range []SessionEvent{SessionLock, SessionUnlock, SessionDisconnect, SessionConnect} {
		h.svc.OnSession(ev, childSession, h.real.Now())
		h.hk(1)
	}
	wantWindows(t, h, childSession)
}

// Ловит (S63): тестовые события блокировки и разблокировки не доходят до службы или открывают окно.
func TestTestSessionLockUnlockNoWindow(t *testing.T) {
	h := logonHarness(t, true, withTestMode(TestMode{}))
	h.childSessionUp()
	h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionLock, SessionID: childSession}, nil)
	h.svc.mu.Lock()
	locked := h.svc.sessions[childSession].locked
	h.svc.mu.Unlock()
	if !locked {
		t.Fatal("тестовая блокировка не дошла до сеанса")
	}
	h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionUnlock, SessionID: childSession}, nil)
	h.hk(12)
	h.svc.mu.Lock()
	locked = h.svc.sessions[childSession].locked
	h.svc.mu.Unlock()
	if locked {
		t.Fatal("тестовая разблокировка не дошла до сеанса")
	}
	wantWindows(t, h, childSession)
}

// Ловит: дубль события входа (настоящее и тестовое) — второе окно; окно, закрытое крестиком,
// открывается снова или пишется обходом; после выхода и нового входа окна нет.
func TestLogonOnceAndReopenAfterLogoff(t *testing.T) {
	h := logonHarness(t, true, withTestMode(TestMode{}))
	h.childLogsIn()
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionLogon, SessionID: childSession}, nil)
	h.hk(2)
	wantWindows(t, h, childSession, "--mystats")
	// Крестик: окно сообщает о закрытии и завершается.
	w := h.launcher.launched(ProcWindow, childSession)[0]
	h.ok(ipc.Peer{PID: w.proc.pid, SessionID: childSession, SID: childSID}, "", ipc.CmdWindowClosing,
		ipc.WindowClosingRequest{Kind: ipc.WindowMyStats}, nil)
	h.launcher.kill(w.proc)
	h.hk(12)
	wantWindows(t, h, childSession, "--mystats")
	if evs := h.store.eventsOf("bypass"); len(evs) != 0 {
		t.Errorf("закрытие крестиком записано обходом: %+v", evs)
	}
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.hk(1)
	wantWindows(t, h, childSession, "--mystats")
	// Выход и новый вход того же пользователя в том же сеансе — снова одно окно.
	h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.hk(2)
	wantWindows(t, h, childSession, "--mystats", "--mystats")
}

// Ловит: окно, уже открытое с табло значка, открывается при входе второй раз.
func TestLogonWindowAlreadyOpen(t *testing.T) {
	h := logonHarness(t, true)
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.hk(1)
	tray := h.trayPeer(childSession, childSID)
	h.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowMyStats}, nil)
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.hk(2)
	wantWindows(t, h, childSession, "--mystats")
}

// Ловит: событие входа раньше, чем сеанс в списке Windows, теряется; или окно открывается
// позже 10 с после входа.
func TestLogonSessionUnknownWaitsUpTo10s(t *testing.T) {
	t.Run("сеанс через 3 с", func(t *testing.T) {
		h := logonHarness(t, true)
		h.svc.OnSession(SessionLogon, childSession, h.real.Now())
		h.hk(2)
		wantWindows(t, h, childSession)
		h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
		h.hk(1)
		wantWindows(t, h, childSession, "--mystats")
	})
	t.Run("сеанс через 11 с", func(t *testing.T) {
		h := logonHarness(t, true)
		h.svc.OnSession(SessionLogon, childSession, h.real.Now())
		h.hk(10)
		h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
		h.hk(1)
		h.hk(3)
		wantWindows(t, h, childSession)
		if !strings.Contains(h.logText(), "Окно при входе в сеанс 2 не открыто") {
			t.Errorf("нет строки журнала работы:\n%s", h.logText())
		}
	})
	t.Run("окно не запустилось", func(t *testing.T) {
		h := logonHarness(t, true)
		h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
		h.hk(1) // значок и страж уже запущены
		h.launcher.failNext = 2
		h.svc.OnSession(SessionLogon, childSession, h.real.Now())
		h.hk(1)
		wantWindows(t, h, childSession)
		h.hk(1)
		wantWindows(t, h, childSession, "--mystats")
	})
}

// Ловит (R-13): перезапуск службы или обновление программы открывает окна в уже открытых сеансах.
func TestServiceRestartNoLogonWindows(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.launcher.setSessions(
		SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID},
		SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID},
	)
	h.start()
	h.hk(12)
	wantWindows(t, h, childSession)
	wantWindows(t, h, 1)
}

// Ловит: окно при входе, закрытое крестиком (сообщение и завершение процесса), считается
// обходом или открывается заново; убитое без сообщения — не открывается заново (Ruling 19).
func TestLogonWindowWithinDeadlineNoBypass(t *testing.T) {
	h := logonHarness(t, true)
	h.childLogsIn()
	h.hk(1)
	w := h.launcher.launched(ProcWindow, childSession)[0]
	h.ok(ipc.Peer{PID: w.proc.pid, SessionID: childSession, SID: childSID}, "", ipc.CmdWindowClosing,
		ipc.WindowClosingRequest{Kind: ipc.WindowMyStats}, nil)
	h.launcher.kill(w.proc)
	h.run(20, activeSample)
	wantWindows(t, h, childSession, "--mystats")
	if evs := h.store.eventsOf("bypass"); len(evs) != 0 {
		t.Errorf("закрытие крестиком записано обходом: %+v", evs)
	}
	if h.usedS() == 0 {
		t.Error("после закрытия окна учёт не идёт")
	}
}
