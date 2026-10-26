package service

import (
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Тесты FB-3 (фидбек-раунд 0.2.3, ТЗ R2, D-52): секунды ребёнка не считаются, пока его сеанс не
// на экране: отключён (смена пользователя) или заблокирован, — что бы ни прислал страж. Страж в
// отключённом сеансе не может открыть рабочий стол ввода и честно считает сеанс незаблокированным,
// а окно на весь экран (игра, видео) засчитывается по R2б без ввода.

// fullscreenSample — ребёнок давно не вводит, но окно на весь экран: по R2б секунда идёт.
var fullscreenSample = ipc.SensorSample{SessionActive: true, LastInputAgoMs: 900_000, Fullscreen: true, Foreground: ipc.CatNameOther}

// childSessionIs ставит список сеансов Windows: сеанс ребёнка в состоянии screen.
func (h *harness) childSessionIs(screen ScreenState) {
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID, Screen: screen})
}

// Ловит: время отключённого сеанса (смена пользователя) идёт, пока в нём окно на весь экран;
// после возврата в сеанс время не возобновляется. Ожидания записаны литералами в секундах.
func TestDisconnectedSessionTimeIsNotCounted(t *testing.T) {
	ways := []struct {
		name string
		off  func(h *harness)
		on   func(h *harness)
	}{
		{"список сеансов говорит: отключён",
			func(h *harness) { h.childSessionIs(ScreenOff); h.hk(1) },
			func(h *harness) { h.childSessionIs(ScreenOn); h.hk(1) }},
		{"событие отключения, в списке состояния нет",
			func(h *harness) { h.svc.OnSession(SessionDisconnect, childSession, h.real.Now()) },
			func(h *harness) { h.svc.OnSession(SessionConnect, childSession, h.real.Now()) }},
		{"и событие, и список",
			func(h *harness) {
				h.childSessionIs(ScreenOff)
				h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
			},
			func(h *harness) {
				h.childSessionIs(ScreenOn)
				h.svc.OnSession(SessionConnect, childSession, h.real.Now())
			}},
	}
	for _, w := range ways {
		t.Run(w.name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.childLogsIn()
			h.run(5, fullscreenSample)
			if got := h.usedS(); got != 5 {
				t.Fatalf("до отключения учтено %d с; want 5 (окно на весь экран засчитывается)", got)
			}
			w.off(h)
			h.run(10, fullscreenSample)
			if got := h.usedS(); got != 5 {
				t.Fatalf("за 10 с в отключённом сеансе учтено %d с; want 5 (время не идёт)", got)
			}
			w.on(h)
			h.run(3, fullscreenSample)
			if got := h.usedS(); got != 8 {
				t.Fatalf("после возврата в сеанс учтено %d с; want 8 (3 новых секунды)", got)
			}
		})
	}
}

// Ловит: блокировка (Win+L, экран входа) не останавливает время, если страж этого не заметил, и
// разблокировка не возвращает его.
func TestLockedSessionTimeIsNotCounted(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.run(5, fullscreenSample)
	h.svc.OnSession(SessionLock, childSession, h.real.Now())
	h.run(10, fullscreenSample)
	if got := h.usedS(); got != 5 {
		t.Fatalf("за 10 с блокировки учтено %d с; want 5", got)
	}
	h.svc.OnSession(SessionUnlock, childSession, h.real.Now())
	h.run(4, fullscreenSample)
	if got := h.usedS(); got != 9 {
		t.Fatalf("после разблокировки учтено %d с; want 9", got)
	}
}

// Ловит: пропущенное событие разблокировки навсегда останавливает время ребёнка: вход в
// сеанс (Logon) и подключение (Connect) сбрасывают признак блокировки, дальше решает страж.
func TestLockFlagIsResetByLogonAndConnect(t *testing.T) {
	for name, ev := range map[string]SessionEvent{"вход": SessionLogon, "подключение": SessionConnect} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.childLogsIn()
			h.svc.OnSession(SessionLock, childSession, h.real.Now())
			h.run(3, fullscreenSample)
			if got := h.usedS(); got != 0 {
				t.Fatalf("при блокировке учтено %d с; want 0", got)
			}
			h.svc.OnSession(ev, childSession, h.real.Now())
			h.run(4, fullscreenSample)
			if got := h.usedS(); got != 4 {
				t.Fatalf("после события %q учтено %d с; want 4", name, got)
			}
		})
	}
}

// Ловит: событие блокировки приходит раньше, чем служба узнала о сеансе (новый сеанс создаётся
// при сверке), и признак блокировки теряется.
func TestLockEventForNewSessionIsKept(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.svc.OnSession(SessionLock, childSession, h.real.Now()) // сеанс появляется только при этой сверке
	h.run(5, fullscreenSample)
	if got := h.usedS(); got != 0 {
		t.Fatalf("учтено %d с; want 0 (сеанс заблокирован с самого начала)", got)
	}
}

// Ловит: сведений о состоянии нет (список без состояния, событий не было), а служба всё равно
// не считает время: решать должен снимок стража, как до FB-3.
func TestUnknownScreenStateFollowsGuardSample(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn() // SessionInfo без состояния
	h.run(6, fullscreenSample)
	if got := h.usedS(); got != 6 {
		t.Fatalf("учтено %d с; want 6", got)
	}
	h.run(6, idleSample)
	if got := h.usedS(); got != 6 {
		t.Fatalf("неактивный снимок засчитан: учтено %d с; want 6", got)
	}
}

// Ловит: блокировка или отключение сеанса взрослого останавливает время ребёнка (признаки
// относятся к своему сеансу).
func TestOtherSessionStateDoesNotAffectChild(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.bothLogIn()
	h.svc.OnSession(SessionLock, 1, h.real.Now())
	h.svc.OnSession(SessionDisconnect, 1, h.real.Now())
	h.run(5, fullscreenSample)
	if got := h.usedS(); got != 5 {
		t.Fatalf("учтено %d с; want 5 (сеанс взрослого не влияет)", got)
	}
}

// Ловит: после выхода из учётки или смены учётки в сеансе служба ещё до 5 с засчитывает секунды
// по последнему снимку стража (запас sampleMaxAge), хотя ТЗ S33 требует остановки не позже 2 с.
func TestGoneSessionSampleIsNotTrusted(t *testing.T) {
	cases := []struct {
		name string
		gone func(h *harness)
	}{
		{"ребёнок вышел из учётки", func(h *harness) { h.launcher.setSessions() }},
		{"в том же сеансе другой пользователь", func(h *harness) {
			h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Анна`, SID: parentSID})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.childLogsIn()
			h.run(2, fullscreenSample)
			c.gone(h)
			h.real.Advance(time.Second)
			h.svc.Housekeep(h.real.Now())
			for i := 0; i < 3; i++ { // стража уже нет, снимок последний; за 3 с он не должен засчитаться
				h.clock.Advance(time.Second)
				h.real.Advance(time.Second)
				h.svc.Tick(h.clock.Now())
			}
			if got := h.usedS(); got != 2 {
				t.Fatalf("учтено %d с; want 2 (после ухода ребёнка секунды не идут)", got)
			}
		})
	}
}

// Ловит: состояние берётся из списка сеансов только один раз при создании сеанса: потом
// отключение, замеченное сверкой, не доходит до учёта.
func TestScreenStateFromListIsRefreshedOnEverySync(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childSessionIs(ScreenOn)
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.run(3, fullscreenSample)
	h.childSessionIs(ScreenOff)
	h.hk(1) // сверка без события
	h.run(3, fullscreenSample)
	if got := h.usedS(); got != 3 {
		t.Fatalf("учтено %d с; want 3", got)
	}
}

// Ловит: пометка «штатно завершается» остаётся на живых процессах после отключения сеанса
// (смены пользователя): потом настоящее принудительное завершение стража не попало бы в журнал,
// а перезапуск откладывался бы на 10 с. Отключённый сеанс остаётся, процессы в нём живут.
func TestDisconnectDoesNotMarkProcessesExpected(t *testing.T) {
	for _, back := range []bool{false, true} {
		name := "сеанс остался отключённым"
		if back {
			name = "ребёнок вернулся"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.childLogsIn()
			h.childSessionIs(ScreenOff)
			h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
			if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
				t.Fatal("страж отключённого сеанса завершён: сеанс остался, процессы живут")
			}
			if back {
				h.childSessionIs(ScreenOn)
				h.svc.OnSession(SessionConnect, childSession, h.real.Now())
			}
			h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
			h.hk(1)
			if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
				t.Fatal("страж не перезапущен сразу: пометка отключения задержала перезапуск")
			}
			h.hk(5)
			if n := h.bypassCount(); n != 1 {
				t.Fatalf("записей об обходе %d; want 1 (принудительное завершение стража записывается)", n)
			}
		})
	}
}

// recentInputActive — страж видит рабочий стол ввода и ввод был секунду назад: так выглядит
// ребёнок за незаблокированным сеансом.
var recentInputActive = ipc.SensorSample{SessionActive: true, LastInputAgoMs: 1000, Foreground: ipc.CatNameOther}

// Ловит: потерянное событие разблокировки (перезапуск службы, гонка) навсегда останавливает
// время в подключённом сеансе. Свежий снимок стража «активен, ввод 1 с назад» сбрасывает
// признак блокировки; ожидания в секундах литералами.
func TestLostUnlockIsRecoveredByGuardSample(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childSessionIs(ScreenOn)
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	h.svc.OnSession(SessionLock, childSession, h.real.Now())
	h.run(5, recentInputActive)
	if got := h.usedS(); got != 5 {
		t.Fatalf("учтено %d с; want 5 (стражу виден рабочий стол с вводом: блокировки нет)", got)
	}
}

// Ловит: признак блокировки сбрасывается снимком, который блокировке не противоречит:
// страж блокировку видит (SessionActive=false), либо ввода давно не было, либо сеанс не на экране.
func TestLockStaysWhileGuardSampleAgreesWithIt(t *testing.T) {
	cases := []struct {
		name   string
		screen ScreenState
		sample ipc.SensorSample
	}{
		{"страж видит блокировку", ScreenOn, ipc.SensorSample{SessionActive: false, LastInputAgoMs: 0, Fullscreen: true}},
		{"ввода давно не было", ScreenOn, fullscreenSample},
		{"сеанс не на экране", ScreenOff, recentInputActive},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.childSessionIs(c.screen)
			h.svc.OnSession(SessionLogon, childSession, h.real.Now())
			h.svc.OnSession(SessionLock, childSession, h.real.Now())
			h.run(5, c.sample)
			if got := h.usedS(); got != 0 {
				t.Fatalf("учтено %d с; want 0 (сеанс заблокирован)", got)
			}
		})
	}
}

// Ловит: событие с неизвестным номером сеанса (не удалось прочитать данные уведомления)
// обрабатывается вслепую: снимает ожидания всех сеансов или не обновляет состояние экрана.
// Безопасно: состояние экрана берётся из полной сверки сеансов, блокировка по такому событию
// не ставится, разблокировка, вход и подключение снимают признак у всех.
func TestUnknownSessionEventsUseFullSync(t *testing.T) {
	t.Run("отключение: экран перечитан из списка", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childSessionIs(ScreenOn)
		h.svc.OnSession(SessionLogon, childSession, h.real.Now())
		h.run(3, fullscreenSample)
		h.childSessionIs(ScreenOff)
		h.svc.OnSession(SessionDisconnect, SessionUnknown, h.real.Now())
		h.run(4, fullscreenSample)
		if got := h.usedS(); got != 3 {
			t.Fatalf("учтено %d с; want 3", got)
		}
	})
	t.Run("разблокировка снимает признак у всех сеансов", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childSessionIs(ScreenOn)
		h.svc.OnSession(SessionLogon, childSession, h.real.Now())
		h.svc.OnSession(SessionLock, childSession, h.real.Now())
		h.run(2, fullscreenSample)
		h.svc.OnSession(SessionUnlock, SessionUnknown, h.real.Now())
		h.run(4, fullscreenSample)
		if got := h.usedS(); got != 4 {
			t.Fatalf("учтено %d с; want 4", got)
		}
	})
	t.Run("блокировка с неизвестным номером не останавливает время вслепую", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childSessionIs(ScreenOn)
		h.svc.OnSession(SessionLogon, childSession, h.real.Now())
		h.svc.OnSession(SessionLock, SessionUnknown, h.real.Now())
		h.run(3, fullscreenSample)
		if got := h.usedS(); got != 3 {
			t.Fatalf("учтено %d с; want 3 (блокировку увидит страж)", got)
		}
	})
}
