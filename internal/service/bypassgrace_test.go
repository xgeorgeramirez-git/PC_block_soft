package service

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Тесты отложенного решения об обходе (FB-1, фидбек-раунд 0.2.3; R17, FR-27): гибель стража,
// значка или окна в сеансе ребёнка записывается в журнал не сразу, а через 5 с, если за это
// время не пришли конец сеанса или остановка службы. Ожидания времени записаны литералами:
// секунды обслуживания (hk) считаются от гибели, которую служба заметила.

// guardProcPeer — настоящий страж сеанса: PID запущенного службой процесса.
func (h *harness) guardProcPeer(session uint32) ipc.Peer {
	h.t.Helper()
	l := h.launcher.aliveOf(ProcGuard, session)
	if len(l) == 0 {
		h.t.Fatalf("в сеансе %d нет живого стража", session)
	}
	return ipc.Peer{PID: l[0].pid, SessionID: session, SID: systemSID, System: true}
}

func (h *harness) bypassCount() int { return len(h.store.eventsOf("bypass")) }

// death — способ «убить» часть программы в сеансе ребёнка и что за ней должно быть записано.
type death struct {
	name    string
	subtype string
	detail  string
	prepare func(h *harness)
	kill    func(h *harness)
	alive   func(h *harness) int // сколько таких частей живо после перезапуска
}

func deaths() []death {
	return []death{
		{
			name: "страж", subtype: "guard_closed", detail: "Защита программы была остановлена",
			prepare: func(h *harness) {},
			kill:    func(h *harness) { h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0]) },
			alive:   func(h *harness) int { return len(h.launcher.aliveOf(ProcGuard, childSession)) },
		},
		{
			name: "значок", subtype: "tray_closed", detail: "Значок программы был закрыт",
			prepare: func(h *harness) {},
			kill:    func(h *harness) { h.launcher.kill(h.launcher.aliveOf(ProcTray, childSession)[0]) },
			alive:   func(h *harness) int { return len(h.launcher.aliveOf(ProcTray, childSession)) },
		},
		{
			name: "окно", subtype: "window_closed", detail: "Окно программы было закрыто не кнопкой окна",
			prepare: func(h *harness) {
				h.ok(h.trayPeer(childSession, childSID), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowMyStats}, nil)
			},
			kill:  func(h *harness) { h.launcher.kill(h.launcher.aliveOf(ProcWindow, childSession)[0]) },
			alive: func(h *harness) int { return len(h.launcher.aliveOf(ProcWindow, childSession)) },
		},
	}
}

// Ловит: гибель в живом сеансе не превращается в запись (ожидание теряется), записывается
// раньше срока (до 5 с) или задерживает перезапуск (FR-27: часть возвращается не позже
// следующего обслуживания). Без прерывающего события запись ровно одна и с прежним текстом.
func TestDeathInLiveSessionIsJournaledAfterFiveSeconds(t *testing.T) {
	for _, d := range deaths() {
		t.Run(d.name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.childLogsIn()
			d.prepare(h)
			d.kill(h)
			h.hk(1) // служба заметила гибель
			if got := d.alive(h); got != 1 {
				t.Fatalf("после гибели живых частей %d; want 1 (перезапуск не позже следующего такта)", got)
			}
			if n := h.bypassCount(); n != 0 {
				t.Fatalf("запись об обходе появилась сразу: %d; want 0 до истечения 5 с", n)
			}
			if !strings.Contains(h.logText(), "завершил") {
				t.Errorf("строки журнала работы про гибель нет:\n%s", h.logText())
			}
			h.hk(4) // прошло 4 с с гибели
			if n := h.bypassCount(); n != 0 {
				t.Fatalf("запись об обходе появилась через 4 с: %d; want 0", n)
			}
			h.hk(1) // прошло 5 с
			evs := h.store.eventsOf("bypass")
			if len(evs) != 1 {
				t.Fatalf("через 5 с записей об обходе %d; want 1: %+v", len(evs), evs)
			}
			if evs[0].Details["subtype"] != d.subtype || evs[0].Details["detail"] != d.detail || evs[0].ChildID != h.childID(childSID) {
				t.Fatalf("запись: %+v; want подтип %q и подробность %q", evs[0], d.subtype, d.detail)
			}
			if got := eventText("bypass", "Миша", evs[0].Details); got != "Миша: попытка обойти защиту. "+d.detail {
				t.Errorf("текст записи %q", got)
			}
			h.hk(30)
			if n := h.bypassCount(); n != 1 {
				t.Fatalf("записей об обходе %d; want 1 (ожидание сработало один раз)", n)
			}
			if n := len(h.svc.bypassWait); n != 0 {
				t.Fatalf("ожиданий осталось %d; want 0", n)
			}
		})
	}
}

// Ловит: конец сеанса или остановка службы за 5 с после гибели всё же даёт запись об обходе —
// ложная «попытка обойти защиту» при выключении и выходе из учётки, замечание заказчика FB-1.
func TestDeathBeforeSessionEndIsNotBypass(t *testing.T) {
	actions := []struct {
		name string
		do   func(h *harness)
		want int
	}{
		{"сеанс жив: запись есть", func(h *harness) {}, 1},
		{"остановка службы", func(h *harness) { h.svc.Shutdown(h.real.Now()) }, 0},
		{"выход из учётной записи", func(h *harness) { h.svc.OnSession(SessionLogoff, childSession, h.real.Now()) }, 0},
		{"отключение сеанса (смена пользователя)", func(h *harness) { h.svc.OnSession(SessionDisconnect, childSession, h.real.Now()) }, 0},
		{"сеанс пропал из списка Windows", func(h *harness) { h.launcher.setSessions(); h.hk(1) }, 0},
		{"в сеансе другой пользователь, потом снова ребёнок", func(h *harness) {
			h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Анна`, SID: parentSID})
			h.hk(1)
			h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
		}, 0},
	}
	for _, d := range deaths() {
		for _, a := range actions {
			t.Run(d.name+"/"+a.name, func(t *testing.T) {
				h := newHarness(t, withChild(childSID, "Миша", 120))
				h.start()
				h.childLogsIn()
				d.prepare(h)
				d.kill(h)
				h.hk(1)
				a.do(h)
				h.hk(10)
				if n := h.bypassCount(); n != a.want {
					t.Fatalf("записей об обходе %d; want %d: %+v", n, a.want, h.store.eventsOf("bypass"))
				}
			})
		}
	}
}

// Ловит: страж умер, перезапущен и умер снова за 5 с (выключение: Windows закрывает и нового),
// а служба ставит второе ожидание или сдвигает срок первого. Без выключения запись одна и
// ровно через 5 с после первой гибели; с выключением записей нет.
func TestRepeatedDeathKeepsSingleWait(t *testing.T) {
	setup := func(t *testing.T) *harness {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.childLogsIn()
		h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
		h.hk(1) // первая гибель, t=1
		if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
			t.Fatal("страж не перезапущен после первой гибели")
		}
		h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
		h.hk(1) // вторая гибель, t=2
		if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
			t.Fatal("страж не перезапущен после второй гибели")
		}
		if n := len(h.svc.bypassWait); n != 1 {
			t.Fatalf("ожиданий %d; want 1 (повторная гибель не создаёт второе)", n)
		}
		return h
	}
	t.Run("выключение", func(t *testing.T) {
		h := setup(t)
		h.svc.Shutdown(h.real.Now())
		if n := len(h.svc.bypassWait); n != 0 {
			t.Fatalf("после остановки службы ожиданий %d; want 0 (все снимаются)", n)
		}
		h.hk(10)
		if n := h.bypassCount(); n != 0 {
			t.Fatalf("записей об обходе %d; want 0", n)
		}
	})
	t.Run("сеанс жив", func(t *testing.T) {
		h := setup(t)
		h.hk(3) // t=5: с первой гибели 4 с
		if n := h.bypassCount(); n != 0 {
			t.Fatalf("запись через 4 с после первой гибели: %d; want 0", n)
		}
		h.hk(1) // t=6: ровно 5 с с первой гибели (срок не сдвигается второй гибелью)
		if n := h.bypassCount(); n != 1 {
			t.Fatalf("записей об обходе %d; want 1", n)
		}
		h.hk(30)
		if n := h.bypassCount(); n != 1 {
			t.Fatalf("записей об обходе %d; want 1 (одна на обе гибели)", n)
		}
	})
}

// Ловит: GuardClosing принимается от любого процесса (ребёнок «закрылся штатно» и убил
// стража без следа) или не принимается от настоящего стража (ложная запись при выключении).
func TestGuardClosingFromRealGuardIsNotBypass(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.ok(h.guardProcPeer(childSession), "", ipc.CmdGuardClosing, nil, nil)
	h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
	h.hk(1)
	if len(h.svc.bypassWait) != 0 {
		t.Fatalf("штатное закрытие стража поставило ожидание: %+v", h.svc.bypassWait)
	}
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 0 {
		t.Fatal("страж вернулся сразу после штатного закрытия (сеанс может заканчиваться)")
	}
	h.hk(10)
	if n := h.bypassCount(); n != 0 {
		t.Fatalf("штатное закрытие стража записано обходом: %d; want 0", n)
	}
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
		t.Fatal("страж не вернулся через 10 с, хотя сеанс остался")
	}
}

// Ловит: GuardClosing от чужого PID (не от стража, запущенного службой) гасит запись об обходе.
func TestGuardClosingFromForeignPIDIsIgnored(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.ok(guardPeer(), "", ipc.CmdGuardClosing, nil, nil) // PID 600: не тот процесс, что запустила служба
	h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
	h.hk(1)
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
		t.Fatal("страж не перезапущен сразу: чужой GuardClosing отложил перезапуск")
	}
	h.hk(5)
	if n := h.bypassCount(); n != 1 {
		t.Fatalf("записей об обходе %d; want 1 (чужой GuardClosing не должен гасить запись)", n)
	}
}

// Ловит: метка «закрываюсь» от стража, который не закрылся (выключение отменили), живёт вечно:
// настоящая гибель этого стража потом не записывается, а перезапуск откладывается на 10 с.
func TestGuardClosingMarkExpiresIfGuardKeepsRunning(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.ok(h.guardProcPeer(childSession), "", ipc.CmdGuardClosing, nil, nil) // выключение, которое потом отменили
	h.hk(31)                                                               // страж жив, сеанс остался
	h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
	h.hk(1)
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
		t.Fatal("страж не перезапущен сразу: старая метка закрытия задерживает перезапуск")
	}
	h.hk(5)
	if n := h.bypassCount(); n != 1 {
		t.Fatalf("записей об обходе %d; want 1 (метка закрытия старше 30 с не гасит запись)", n)
	}
}

// Ловит: выход одного ребёнка снимает ожидание другого (событие сеанса относится только к своему).
func TestLogoffOfOtherSessionKeepsWait(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120))
	h.start()
	h.child2LogsIn()
	h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
	h.hk(1)
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.svc.OnSession(SessionLogoff, child2Session, h.real.Now())
	h.hk(6)
	evs := h.store.eventsOf("bypass")
	if len(evs) != 1 || evs[0].ChildID != h.childID(childSID) {
		t.Fatalf("события обхода: %+v; want одно, у Миши", evs)
	}
}

// Ловит: у взрослого и у ребёнка-администратора (R1) гибель частей ставит ожидание или пишет обход.
func TestNoWaitOrBypassForAdultAndChildAdmin(t *testing.T) {
	t.Run("взрослый", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.start()
		h.bothLogIn()
		h.launcher.kill(h.launcher.aliveOf(ProcTray, 1)[0])
		h.hk(1)
		if len(h.svc.bypassWait) != 0 {
			t.Fatalf("у взрослого поставлено ожидание: %+v", h.svc.bypassWait)
		}
		h.hk(10)
		if n := h.bypassCount(); n != 0 {
			t.Fatalf("у взрослого записано обходов: %d", n)
		}
		if len(h.launcher.aliveOf(ProcTray, 1)) != 1 {
			t.Fatal("значок взрослого не вернулся")
		}
	})
	t.Run("ребёнок-администратор", func(t *testing.T) {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		h.admins.set(childSID, true)
		h.start()
		h.childLogsIn()
		h.launcher.kill(h.launcher.aliveOf(ProcTray, childSession)[0])
		h.hk(10)
		if n := h.bypassCount(); n != 0 {
			t.Fatalf("у ребёнка-администратора записано обходов: %d", n)
		}
	})
}

// Ловит: снятое ожидание уходит молча (в подробном режиме должна быть строка, почему записи нет),
// а при выключенном режиме журнал работы засорён.
func TestCancelledWaitIsLoggedOnlyInVerboseMode(t *testing.T) {
	run := func(t *testing.T, verbose bool) string {
		h := newHarness(t, withChild(childSID, "Миша", 120))
		if verbose {
			h.store.settings[settingVerboseLogs] = "1"
		}
		h.start()
		h.childLogsIn()
		h.launcher.kill(h.launcher.aliveOf(ProcGuard, childSession)[0])
		h.hk(1)
		h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
		h.hk(10)
		if n := h.bypassCount(); n != 0 {
			t.Fatalf("записей об обходе %d; want 0", n)
		}
		return h.logText()
	}
	if log := run(t, true); !strings.Contains(log, "[подробно] Обход не записан") {
		t.Errorf("в подробном режиме нет строки о снятом ожидании:\n%s", log)
	}
	if log := run(t, false); strings.Contains(log, "Обход не записан") {
		t.Errorf("без подробного режима строка о снятом ожидании попала в журнал работы:\n%s", log)
	}
}

// Ловит: созревшее ожидание превращается в запись, хотя сеанса уже нет или в нём другой
// пользователь (запись попала бы чужому ребёнку или службу уронила бы пустая ссылка). Обычные
// пути снимают такое ожидание раньше (dropSession); этот тест держит последнюю проверку при самом
// превращении. Случаи по отдельности: сбой первого скрыл бы второй (guard глотает панику).
func TestMatureWaitIsDroppedIfSessionGoneOrOtherUser(t *testing.T) {
	cases := []struct {
		name string
		wait pendingBypass
	}{
		{"сеанса нет", pendingBypass{sessionID: 77, sid: childSID, detail: detailGuardClosed, subtype: "guard_closed"}},
		{"в сеансе другой пользователь", pendingBypass{sessionID: childSession, sid: otherSID, detail: detailGuardClosed, subtype: "guard_closed"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, withChild(childSID, "Миша", 120))
			h.start()
			h.childLogsIn()
			c.wait.at = h.real.Now().Add(-10 * time.Second)
			h.svc.bypassWait = []pendingBypass{c.wait}
			h.hk(1)
			if n := h.bypassCount(); n != 0 {
				t.Fatalf("записей об обходе %d; want 0: %+v", n, h.store.eventsOf("bypass"))
			}
			if n := len(h.store.eventsOf(evInternalError)); n != 0 {
				t.Fatalf("внутренних ошибок %d; want 0 (служба не должна падать на пустом сеансе)", n)
			}
			if n := len(h.svc.bypassWait); n != 0 {
				t.Fatalf("ожиданий осталось %d; want 0", n)
			}
		})
	}
}
