package service

import (
	"strings"
	"testing"

	"pcblock/internal/ipc"
	"pcblock/internal/sites"
)

// withAdult подставляет список «18+» из доменов domains вместо встроенного (до New).
func withAdult(domains ...string) hopt {
	return func(h *harness) {
		h.cfg.Adult = func() (sites.AdultList, error) { return sites.OpenAdult(string(sites.EncodeAdult(domains))) }
	}
}

// withVideo задаёт ребёнку лимит «Видео и соцсети» min минут на каждый день (до Start), как withGames.
func withVideo(sid string, min int) hopt {
	return func(h *harness) {
		id := h.childID(sid)
		week := h.store.limits[id]
		for i := range week {
			v := min
			week[i].VideoMin = &v
		}
		h.store.SetDayLimits(id, week)
	}
}

// braveSample — снимок стража: Brave на переднем плане, активная вкладка с доменом domain.
func braveSample(domain string) ipc.SensorSample {
	return ipc.SensorSample{SessionActive: true, Foreground: ipc.CatNameOther, ForegroundPath: pathBrave, ForegroundDomain: domain}
}

// siteHarness — служба с Мишей (120 мин), снимком программ, списком «18+» из adult-probe.test,
// example.com в «Запрещённых» и school.example в «Учёбе»; Миша вошёл в сеанс 2. opts — после
// ребёнка и до Start (withRules, withVideo, withWatcher…).
func siteHarness(t *testing.T, opts ...hopt) *harness {
	t.Helper()
	all := append([]hopt{configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withAdult("adult-probe.test"),
		func(h *harness) { h.store.sites["example.com"] = "blocked"; h.store.sites["school.example"] = "study" }}, opts...)
	h := newHarness(t, all...)
	h.start()
	h.childLogsIn()
	return h
}

// classify — classifySite под s.mu.
func (h *harness) classify(domain string, adultOn bool) (siteClass, string) {
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	return h.svc.classifySite(domain, adultOn)
}

// Ловит (RM4-5, R14): поддомен не находит запись списка, «18+» сильнее своих списков родителя,
// совпавшая запись не возвращается (плашка и учёт по записи), служебная страница получает класс,
// флаг правил Brave не ставится после изменения списков.
func TestClassifySite(t *testing.T) {
	h := siteHarness(t)
	cases := []struct {
		domain  string
		adultOn bool
		cls     siteClass
		entry   string
	}{
		{"youtube.com", true, siteVideo, "youtube.com"},
		{"m.youtube.com", true, siteVideo, "youtube.com"},
		{"a.school.example", true, siteStudy, "school.example"},
		{"m.example.com", true, siteBlocked, "example.com"},
		{"adult-probe.test", true, siteAdult, ""},
		{"adult-probe.test", false, siteOther, "adult-probe.test"},
		{"news.example", true, siteOther, "news.example"},
		{"", true, siteNone, ""},
	}
	for _, c := range cases {
		if cls, entry := h.classify(c.domain, c.adultOn); cls != c.cls || entry != c.entry {
			t.Errorf("%q (18+ %v): (%v, %q), want (%v, %q)", c.domain, c.adultOn, cls, entry, c.cls, c.entry)
		}
	}
	if h.svc.braveDue {
		t.Fatal("braveDue взведён без изменения списков")
	}
	h.store.sites["adult-probe.test"] = "study"
	h.svc.mu.Lock()
	h.svc.sitesChanged(h.real.Now())
	h.svc.mu.Unlock()
	if cls, entry := h.classify("adult-probe.test", true); cls != siteStudy || entry != "adult-probe.test" {
		t.Errorf("adult-probe.test в «Учёбе»: (%v, %q), want (siteStudy, adult-probe.test): свои списки сильнее 18+", cls, entry)
	}
	if !h.svc.braveDue {
		t.Error("braveDue не взведён после sitesChanged (правила Brave, T5)")
	}
}

// Ловит (R18, S-11): сбой чтения списков роняет «18+» или открывает прежде запрещённое,
// чтение не повторяется в обслуживании, повтор не ждёт rulesRetryEvery.
func TestSitesReloadFailure(t *testing.T) {
	h := siteHarness(t, func(h *harness) { h.store.failSites = true })
	if h.svc.siteListsOK {
		t.Fatal("siteListsOK при сбое чтения")
	}
	if cls, _ := h.classify("m.example.com", true); cls != siteOther {
		t.Errorf("m.example.com без списков: %v, want siteOther", cls)
	}
	if cls, _ := h.classify("adult-probe.test", true); cls != siteAdult {
		t.Errorf("adult-probe.test без списков: %v, want siteAdult (18+ действует)", cls)
	}
	h.store.mu.Lock()
	h.store.failSites = false
	h.store.mu.Unlock()
	h.real.Advance(rulesRetryEvery - 1e9)
	h.svc.Housekeep(h.real.Now())
	if h.svc.siteListsOK {
		t.Fatal("списки перечитаны раньше rulesRetryEvery")
	}
	h.real.Advance(1e9)
	h.svc.Housekeep(h.real.Now())
	if !h.svc.siteListsOK {
		t.Fatal("списки не перечитаны в Housekeep через rulesRetryEvery")
	}
	if cls, _ := h.classify("m.example.com", true); cls != siteBlocked {
		t.Errorf("m.example.com после повтора: %v, want siteBlocked", cls)
	}
	if got := h.logText(); !strings.Contains(got, "не удалось прочитать списки сайтов") {
		t.Errorf("журнал работы без строки о сбое списков: %q", got)
	}
	// Сбой после удачного чтения: прежние списки остаются.
	h.store.mu.Lock()
	h.store.failSites = true
	h.store.mu.Unlock()
	h.svc.mu.Lock()
	h.svc.sitesChanged(h.real.Now())
	h.svc.mu.Unlock()
	if h.svc.siteListsOK {
		t.Fatal("siteListsOK при сбое повторного чтения")
	}
	if cls, _ := h.classify("m.example.com", true); cls != siteBlocked {
		t.Errorf("m.example.com после сбоя повторного чтения: %v, want siteBlocked (прежние списки)", cls)
	}
}
