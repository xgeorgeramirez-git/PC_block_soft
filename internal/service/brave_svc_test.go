package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// braveURLs — домены чёрного списка Brave в ветке учётки sid по номерам.
func (h *harness) braveURLs(sid string) []string {
	h.t.Helper()
	vals, _ := h.policy.braveOf(sid)
	out := []string{}
	for _, v := range vals {
		if v.Key == braveKeyURLs {
			out = append(out, v.Str)
		}
	}
	return out
}

// braveFull — ветка учётки sid совпадает с правилами Brave для доменов blocked.
func (h *harness) braveFull(sid string, blocked ...string) bool {
	vals, ok := h.policy.braveOf(sid)
	return ok && reflect.DeepEqual(vals, BraveRules{Blocked: blocked}.Values())
}

// houseFor — n секунд настоящих часов с обслуживанием, без опроса стража (ребёнок не вошёл).
func (h *harness) houseFor(n int) {
	for i := 0; i < n; i++ {
		h.real.Advance(time.Second)
		h.svc.Housekeep(h.real.Now())
	}
}

func braveHarness(t *testing.T, opts ...hopt) *harness {
	t.Helper()
	all := append([]hopt{configured(), withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 60),
		func(h *harness) { h.store.sites["example.com"] = "blocked"; h.store.sites["school.example"] = "study" }}, opts...)
	return newHarness(t, all...)
}

// Ловит (RM4-10, Review Focus 2, PL-18): правила не пишутся при старте, пишутся не всем детям,
// попадают родителю или учётке без отметки, в чёрный список уходят «Учёба» и «Видео», первая запись
// пишется в журнал родителя как восстановление.
func TestBraveRulesAtStart(t *testing.T) {
	h := braveHarness(t)
	h.start()
	for _, sid := range []string{childSID, child2SID} {
		if !h.braveFull(sid, "example.com") {
			vals, _ := h.policy.braveOf(sid)
			t.Errorf("ветка %s после старта: %+v", sid, vals)
		}
	}
	for _, sid := range []string{parentSID, otherSID} {
		if _, ok := h.policy.braveOf(sid); ok {
			t.Errorf("правила Brave записаны учётке без отметки %s", sid)
		}
	}
	for _, c := range h.policy.braveCalls(0) {
		if strings.Contains(c, parentSID) || strings.Contains(c, otherSID) {
			t.Errorf("вызов правил Brave для учётки без отметки: %q", c)
		}
	}
	if got := h.journalTexts(evBraveRestored); len(got) != 0 {
		t.Errorf("первая запись попала в журнал родителя: %q", got)
	}
}

// Ловит (RM4-10, S-12): изменение «Запрещённых» не доходит до Brave без перезапуска, флаг записи
// не сбрасывается (запись каждую секунду), запись по изменению списков пишется в журнал родителя.
func TestBraveRulesFollowLists(t *testing.T) {
	h := siteHarness(t)
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "other.example", Kind: "blocked"}, nil)
	h.step(activeSample)
	if got := h.braveURLs(childSID); !reflect.DeepEqual(got, []string{"example.com", "other.example"}) {
		t.Errorf("после добавления URLBlocklist %q", got)
	}
	h.ok(adminPeer(), tok, ipc.CmdRemoveSite, ipc.RemoveSiteRequest{Domain: "example.com"}, nil)
	n := h.policy.braveCallN()
	h.step(activeSample)
	if got := h.braveURLs(childSID); !reflect.DeepEqual(got, []string{"other.example"}) {
		t.Errorf("после удаления URLBlocklist %q", got)
	}
	if got := h.policy.braveCalls(n); !reflect.DeepEqual(got, []string{"set " + childSID + " 5"}) {
		t.Errorf("вызовы после удаления %q, want одна запись", got)
	}
	h.svc.mu.Lock()
	due := h.svc.braveDue
	h.svc.mu.Unlock()
	if due {
		t.Error("braveDue не сброшен после записи")
	}
	n = h.policy.braveCallN()
	h.run(5, activeSample)
	if got := h.policy.braveCalls(n); len(got) != 0 {
		t.Errorf("запись повторяется без изменения списков: %q", got)
	}
	if got := h.journalTexts(evBraveRestored); len(got) != 0 {
		t.Errorf("запись по изменению списков попала в журнал родителя: %q", got)
	}
}

// Ловит (RM4-10, T5-7, Review Focus 6): ветку, изменённую чужими руками, не возвращают за минуту,
// сверяют чаще раза в 60 с, журнал родителя пишется на каждое восстановление или по ограничителю,
// который забывает ключи раньше 10 мин; сверка без расхождения пишет журнал или переписывает ветку.
func TestBraveRulesRestoredOncePer10Min(t *testing.T) {
	h := siteHarness(t)
	h.policy.tamper(childSID)
	n := h.policy.braveCallN()
	h.run(59, activeSample)
	if got := h.policy.braveCalls(n); len(got) != 0 {
		t.Fatalf("сверка раньше 60 с: %q", got)
	}
	if h.braveFull(childSID, "example.com") {
		t.Fatal("ветка восстановлена раньше 60 с")
	}
	h.step(activeSample)
	if !h.braveFull(childSID, "example.com") {
		t.Fatal("ветка не восстановлена через 60 с")
	}
	want := []string{"Миша: правила Brave восстановлены"}
	if got := h.journalTexts(evBraveRestored); !reflect.DeepEqual(got, want) {
		t.Fatalf("журнал родителя %q, want %q", got, want)
	}
	if got := h.logText(); !strings.Contains(got, "правила Brave у Миша восстановлены") {
		t.Errorf("журнал работы без восстановления: %q", got)
	}
	h.policy.tamper(childSID)
	h.run(60, activeSample)
	if !h.braveFull(childSID, "example.com") {
		t.Fatal("ветка не восстановлена второй раз")
	}
	if got := h.journalTexts(evBraveRestored); len(got) != 1 {
		t.Fatalf("второе восстановление за 10 мин записано родителю: %q", got)
	}
	h.run(480, activeSample) // 600 с после первого восстановления — на следующей сверке
	h.policy.tamper(childSID)
	h.run(60, activeSample)
	if got := h.journalTexts(evBraveRestored); len(got) != 2 {
		t.Errorf("восстановление через 10 мин не записано родителю: %q", got)
	}
	if n := strings.Count(h.logText(), "правила Brave у Миша восстановлены"); n != 3 {
		t.Errorf("в журнале работы %d восстановлений, want 3", n)
	}

	quiet := siteHarness(t)
	n = quiet.policy.braveCallN()
	quiet.run(180, activeSample)
	got := quiet.policy.braveCalls(n)
	if !reflect.DeepEqual(got, []string{"ok " + childSID, "ok " + childSID, "ok " + childSID}) {
		t.Errorf("сверка без расхождения: %q, want три ok", got)
	}
	if got := quiet.journalTexts(evBraveRestored); len(got) != 0 {
		t.Errorf("журнал родителя без расхождения: %q", got)
	}
}

// Ловит (T5-4): сверка раз в 60 с читает ветку невошедшего ребёнка (загрузка профиля каждые 60 с),
// вход ребёнка не пишет правила в его ветку.
func TestBraveRulesUnloadedNotChecked(t *testing.T) {
	h := braveHarness(t, func(h *harness) { h.policy.unloaded[childSID] = true })
	h.start()
	if !h.braveFull(childSID, "example.com") {
		t.Fatal("при старте правила не записаны через профиль")
	}
	h.policy.tamper(childSID)
	n := h.policy.braveCallN()
	h.houseFor(120)
	for _, c := range h.policy.braveCalls(n) {
		if strings.HasPrefix(c, "set "+childSID) {
			t.Fatalf("ветка невошедшего ребёнка переписана сверкой: %q", h.policy.braveCalls(n))
		}
	}
	h.policy.mu.Lock()
	delete(h.policy.unloaded, childSID)
	h.policy.mu.Unlock()
	h.childLogsIn()
	if !h.braveFull(childSID, "example.com") {
		t.Error("при входе ребёнка правила не записаны")
	}
}

// Ловит (T5-5, PL-30): сбой чтения списков на старте пишет пустой чёрный список, после первого
// удачного чтения правила не пишутся, сбой повторного чтения стирает «Запрещённые» в Brave.
func TestBraveRulesWaitForLists(t *testing.T) {
	t.Run("первое чтение", func(t *testing.T) {
		// Дети не вошли: сверка их ветки не читает, правила пишет только первое удачное чтение.
		h := braveHarness(t, func(h *harness) {
			h.store.failSites = true
			h.policy.unloaded[childSID], h.policy.unloaded[child2SID] = true, true
		})
		h.start()
		h.houseFor(61)
		if got := h.policy.braveCalls(0); len(got) != 0 {
			t.Fatalf("правила записаны без списков: %q", got)
		}
		if got := h.logText(); !strings.Contains(got, "правила Brave не пишутся: списки сайтов ещё не прочитаны") {
			t.Errorf("журнал работы: %q", got)
		}
		h.store.mu.Lock()
		h.store.failSites = false
		h.store.mu.Unlock()
		h.real.Advance(rulesRetryEvery)
		h.svc.Housekeep(h.real.Now())
		if !h.braveFull(childSID, "example.com") || !h.braveFull(child2SID, "example.com") {
			t.Error("после первого удачного чтения правила не записаны")
		}
	})
	t.Run("сбой повторного чтения", func(t *testing.T) {
		h := braveHarness(t)
		h.start()
		h.store.mu.Lock()
		h.store.failSites = true
		h.store.mu.Unlock()
		h.svc.mu.Lock()
		h.svc.sitesChanged(h.real.Now())
		h.svc.mu.Unlock()
		h.houseFor(1)
		if got := h.braveURLs(childSID); !reflect.DeepEqual(got, []string{"example.com"}) {
			t.Errorf("после сбоя повторного чтения URLBlocklist %q, want прежний example.com", got)
		}
	})
}

// Ловит (T5-6, Review Focus 2): снятая отметка «детская» оставляет правила Brave (Tor и VPN
// выключены у взрослой учётки), новый ребёнок ждёт сверки, а не получает правила сразу.
func TestBraveRulesRemovedWithChild(t *testing.T) {
	h := braveHarness(t)
	h.start()
	tok := h.login(adminPeer())
	n := h.policy.braveCallN()
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Remove: []int64{h.childID(child2SID)}}, nil)
	if got := h.policy.braveCalls(n); !reflect.DeepEqual(got, []string{"rm " + child2SID}) {
		t.Errorf("снятие отметки: вызовы %q", got)
	}
	if _, ok := h.policy.braveOf(child2SID); ok {
		t.Error("у снятой учётки остались правила Brave")
	}
	n = h.policy.braveCallN()
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Add: []ipc.ChildAdd{{SID: otherSID, Name: "Даша"}}}, nil)
	if got := h.policy.braveCalls(n); !reflect.DeepEqual(got, []string{"set " + otherSID + " 5"}) {
		t.Errorf("новый ребёнок: вызовы %q", got)
	}
}

// Ловит (R18): ошибка адаптера правил Brave останавливает учёт или пишется в журнал работы каждую
// секунду.
func TestBraveRulesErrorDoesNotStopService(t *testing.T) {
	h := siteHarness(t, func(h *harness) { h.policy.braveErr = errors.New("нет доступа") })
	h.run(59, activeSample)
	if used := h.usedS(); used < 59 {
		t.Errorf("учёт при ошибке правил Brave: %d с", used)
	}
	if n := strings.Count(h.logText(), "правила Brave для Миша не записаны"); n != 1 {
		t.Errorf("ошибка в журнале работы %d раз за 59 с, want 1", n)
	}
}
