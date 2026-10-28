package service

import (
	"testing"

	"pcblock/internal/ipc"
)

// Тесты FB-5 (фидбек-раунд 0.2.5; R17, FR-27): заказчик вошёл ребёнком, сменил пользователя
// (сеанс ребёнка отключён, но жив), родитель настроил программу и вышел из своей учётки, а в
// журнале появилась «попытка обойти защиту». Выход другого сеанса, совпавший по времени с
// гибелью стража, значка или окна в отключённом сеансе ребёнка, обходом не считается: ребёнка
// за компьютером нет, а части программы перезапускаются сразу (FR-27). Принудительное
// завершение без выхода другого сеанса по-прежнему пишется.

const parentSessionFB5 = 1

// childAwayParentIn: ребёнок вошёл в сеанс 2, затем смена пользователя: сеанс 2 отключён,
// родитель вошёл в сеанс 1.
func (h *harness) childAwayParentIn() {
	h.t.Helper()
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOn})
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.hk(2)
	h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
	h.launcher.setSessions(
		SessionInfo{ID: parentSessionFB5, User: `PC\Анна`, SID: parentSID, Screen: ScreenOn},
		SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOff},
	)
	h.svc.OnSession(SessionLogon, parentSessionFB5, h.real.Now())
	h.hk(10) // родитель настраивает программу; удержание отключения (5 с) давно прошло
}

// parentLogsOff: родитель вышел, его сеанс исчез из списка, сеанс ребёнка остался отключённым.
func (h *harness) parentLogsOff() {
	h.t.Helper()
	h.svc.OnSession(SessionLogoff, parentSessionFB5, h.real.Now())
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOff})
}

// Ловит: гибель стража, значка или окна в отключённом сеансе ребёнка во время выхода родителя
// (до события выхода и после него, с сообщением стража «закрываюсь» и без) записывается обходом.
// Перезапуск при этом не откладывается (FR-27).
func TestParentLogoffWithChildDisconnectedIsNotBypass(t *testing.T) {
	orders := []struct {
		name string
		do   func(h *harness, d death)
	}{
		{"гибель до события выхода", func(h *harness, d death) {
			d.kill(h)
			h.hk(1)
			h.parentLogsOff()
		}},
		{"гибель после события выхода", func(h *harness, d death) {
			h.parentLogsOff()
			h.hk(3)
			d.kill(h)
		}},
	}
	for _, d := range deaths() {
		for _, o := range orders {
			t.Run(d.name+"/"+o.name, func(t *testing.T) {
				h := newHarness(t, withChild(childSID, "Миша", 120))
				h.start()
				h.childAwayParentIn()
				d.prepare(h)
				o.do(h, d)
				h.hk(1)
				if got := d.alive(h); got != 1 {
					t.Fatalf("после гибели живых частей %d; want 1 (перезапуск сразу, FR-27)", got)
				}
				h.hk(60)
				if n := h.bypassCount(); n != 0 {
					t.Fatalf("записей об обходе %d; want 0: %+v", n, h.store.eventsOf("bypass"))
				}
				if n := len(h.svc.bypassWait); n != 0 {
					t.Fatalf("ожиданий осталось %d; want 0", n)
				}
			})
		}
	}
	t.Run("страж сообщил «закрываюсь» и погиб при выходе родителя", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childAwayParentIn()
		h.ok(h.guardProcPeer(childSession), "", ipc.CmdGuardClosing, nil, nil)
		h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
		h.hk(1)
		h.parentLogsOff()
		h.hk(60)
		if n := h.bypassCount(); n != 0 {
			t.Fatalf("записей об обходе %d; want 0: %+v", n, h.store.eventsOf("bypass"))
		}
	})
}

// Ловит: исправление FB-5 гасит настоящие записи. Принудительное завершение стража пишется: в
// сеансе на экране (даже если в это время вышел другой сеанс), в отключённом сеансе без выхода
// другого сеанса и задолго до выхода родителя.
func TestForcedGuardKillStillJournaledFB5(t *testing.T) {
	kill := func(h *harness) { h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0]) }
	t.Run("ребёнок на экране, рядом вышел другой сеанс", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.launcher.setSessions(
			SessionInfo{ID: parentSessionFB5, User: `PC\Анна`, SID: parentSID, Screen: ScreenOff},
			SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOn},
		)
		h.svc.OnSession(SessionLogon, childSession, h.real.Now())
		h.hk(2)
		kill(h)
		h.hk(1)
		h.svc.OnSession(SessionLogoff, parentSessionFB5, h.real.Now())
		h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOn})
		h.hk(10)
		if n := h.bypassCount(); n != 1 {
			t.Fatalf("записей об обходе %d; want 1", n)
		}
	})
	t.Run("сеанс ребёнка отключён, другой сеанс не выходил", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childAwayParentIn()
		kill(h)
		h.hk(10)
		evs := h.store.eventsOf("bypass")
		if len(evs) != 1 || evs[0].Details["subtype"] != "guard_closed" {
			t.Fatalf("записи: %+v; want одна guard_closed", evs)
		}
	})
	t.Run("гибель задолго до выхода родителя", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childAwayParentIn()
		kill(h)
		h.hk(20)
		h.parentLogsOff()
		h.hk(10)
		if n := h.bypassCount(); n != 1 {
			t.Fatalf("записей об обходе %d; want 1", n)
		}
	})
	t.Run("выход родителя задолго до гибели", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childAwayParentIn()
		h.parentLogsOff()
		h.hk(20)
		kill(h)
		h.hk(10)
		if n := h.bypassCount(); n != 1 {
			t.Fatalf("записей об обходе %d; want 1", n)
		}
	})
}

// Ловит: тестовая команда TestSession (приёмка FB5 в CI) не доходит до того же пути, что событие
// Windows, работает вне тестового режима или принимает неизвестное событие.
func TestTestSessionCommandFeedsSessionEvents(t *testing.T) {
	t.Run("вне тестового режима отказ", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.fail(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionLogoff, SessionID: 99})
	})
	t.Run("неизвестное событие: отказ", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}))
		h.start()
		h.childLogsIn()
		rep := h.fail(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: "reboot", SessionID: childSession})
		if rep.Error != ipc.TextBadRequest {
			t.Fatalf("отказ %q; want %q", rep.Error, ipc.TextBadRequest)
		}
	})
	t.Run("отключение и выход другого сеанса гасят запись, как события Windows", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}))
		h.start()
		h.childLogsIn()
		h.hk(2)
		h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionDisconnect, SessionID: childSession}, nil)
		h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
		h.hk(1)
		h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionLogoff, SessionID: 99, SID: parentSID}, nil)
		h.hk(40)
		if n := h.bypassCount(); n != 0 {
			t.Fatalf("записей об обходе %d; want 0", n)
		}
	})
	t.Run("выход незнакомого сеанса без SID пользователя: запись есть", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}))
		h.start()
		h.childLogsIn()
		h.hk(2)
		h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionDisconnect, SessionID: childSession}, nil)
		h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
		h.hk(1)
		h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionLogoff, SessionID: 99}, nil)
		h.hk(40)
		if n := h.bypassCount(); n != 1 {
			t.Fatalf("записей об обходе %d; want 1", n)
		}
	})
	t.Run("контроль: только отключение, без выхода другого сеанса, запись есть", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}))
		h.start()
		h.childLogsIn()
		h.hk(2)
		h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionDisconnect, SessionID: childSession}, nil)
		h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
		h.hk(10)
		if n := h.bypassCount(); n != 1 {
			t.Fatalf("записей об обходе %d; want 1", n)
		}
	})
}

// Ловит лазейку (ревью FB-5): ребёнок запускает отложенное закрытие значка, жмёт «Сменить
// пользователя» и возвращается в свою учётку. Временный сеанс экрана входа (без пользователя)
// завершается событием выхода с другим номером, а выход с неизвестным номером не говорит, кто
// вышел: ни то ни другое не выход другого пользователя, запись остаётся.
func TestLogoffWithoutOtherUserKeepsBypass(t *testing.T) {
	for _, c := range []struct {
		name string
		id   uint32
	}{{"экран входа без пользователя", 7}, {"номер сеанса неизвестен", SessionUnknown}} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOn})
			h.svc.OnSession(SessionLogon, childSession, h.real.Now())
			h.hk(2)
			h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
			h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOff})
			h.hk(1)
			h.launcher.kill(h.launcher.aliveOf(ProcTray, childSession)[0])
			h.hk(1)
			h.svc.OnSession(SessionLogoff, c.id, h.real.Now())
			h.hk(10)
			if n := h.bypassCount(); n != 1 {
				t.Fatalf("записей об обходе %d; want 1", n)
			}
		})
	}
}

// Ловит: сеанс родителя исчез из списка Windows раньше, чем пришло событие выхода (или событие
// потерялось), и исправление FB-5 не срабатывает.
func TestParentSessionVanishedCountsAsLogoff(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childAwayParentIn()
	h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: ScreenOff})
	h.hk(1)
	h.svc.OnSession(SessionLogoff, parentSessionFB5, h.real.Now())
	h.hk(60)
	if n := h.bypassCount(); n != 0 {
		t.Fatalf("записей об обходе %d; want 0: %+v", n, h.store.eventsOf("bypass"))
	}
}
