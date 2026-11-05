package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"pcblock/internal/core"
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

// foreground — foregroundOf ребёнка Миши под s.mu.
func (h *harness) foreground(sm ipc.SensorSample) (core.Category, usageSubject, bool) {
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	return h.svc.foregroundOf(h.svc.children[h.childID(childSID)], sm)
}

// Ловит (RM4-7, S-7, RM4-8): секунда Brave не по сайту, а по программе; поддомен учитывается своим
// доменом, а не записью списка; запрещённый и «18+» сайт получает строку учёта; домен у не-Brave
// меняет категорию игры; служба верит слову стража «видео».
func TestForegroundSiteCategory(t *testing.T) {
	h := siteHarness(t)
	mc := fgSample(pathMinecraft, ipc.CatNameOther)
	mc.ForegroundDomain = "youtube.com"
	cases := []struct {
		name   string
		sample ipc.SensorSample
		cat    core.Category
		u      usageSubject
	}{
		{"служебная страница", braveSample(""), core.CatOther, usageSubject{usageKindProgram, pathBrave}},
		{"видео", braveSample("youtube.com"), core.CatVideoSocial, usageSubject{usageKindSite, "youtube.com"}},
		{"поддомен видео — запись", braveSample("m.youtube.com"), core.CatVideoSocial, usageSubject{usageKindSite, "youtube.com"}},
		{"учёба", braveSample("school.example"), core.CatStudy, usageSubject{usageKindSite, "school.example"}},
		{"запрещённый", braveSample("m.example.com"), core.CatOther, usageSubject{}},
		{"18+", braveSample("adult-probe.test"), core.CatOther, usageSubject{}},
		{"прочий сайт", braveSample("news.example"), core.CatOther, usageSubject{usageKindSite, "news.example"}},
		{"домен у не-Brave", mc, core.CatGames, usageSubject{usageKindProgram, pathMinecraft}},
		{"слово стража «видео»", ipc.SensorSample{SessionActive: true, Foreground: ipc.CatNameVideoSocial}, core.CatOther, usageSubject{usageKindProgram, ""}},
	}
	for _, c := range cases {
		cat, u, hidden := h.foreground(c.sample)
		if cat != c.cat || u != c.u || hidden {
			t.Errorf("%s: (%v, %+v, %v), want (%v, %+v, false)", c.name, cat, u, hidden, c.cat, c.u)
		}
	}
}

// Ловит (RM4-7, PL-38): секунды видео не идут в учёт (usageCats без видео), сайт пишется видом
// «программа», запрещённый сайт получает свою строку, секунды Brave на служебной странице теряются.
func TestSiteUsageSaved(t *testing.T) {
	h := siteHarness(t, withVideo(childSID, 60))
	h.run(10, braveSample("youtube.com"))
	h.run(5, braveSample(""))
	h.run(3, braveSample("news.example"))
	h.run(4, braveSample("m.example.com"))
	h.svc.Shutdown(h.real.Now())
	id := h.childID(childSID)
	want := map[string]int{
		fmt.Sprintf("%d|2026-10-19|video_social|site|youtube.com", id): 10,
		fmt.Sprintf("%d|2026-10-19|other|program|%s", id, pathBrave):   5,
		fmt.Sprintf("%d|2026-10-19|other|site|news.example", id):       3,
	}
	h.store.mu.Lock()
	got := map[string]int{}
	for k, v := range h.store.usage {
		got[k] = v
	}
	st := h.store.days[dayKey(id, "2026-10-19")]
	h.store.mu.Unlock()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("учёт %v, want %v", got, want)
	}
	if st.VideoS != 10 || st.TotalS != 22 {
		t.Errorf("день: видео %d, всего %d; want 10, 22", st.VideoS, st.TotalS)
	}
}

// Ловит (R23, S-5): адрес страницы или мусор от стража принимаются за домен, попадают в журнал
// работы или родителя, домен в другом регистре закрывает вкладку.
func TestGuardDomainRejected(t *testing.T) {
	h := siteHarness(t)
	for _, d := range []string{"example.com/secret?token=1", strings.Repeat("a", 250) + ".com", "Example.COM"} {
		rep := h.poll(braveSample(d))
		for _, c := range rep.Commands {
			if c.Kind == ipc.GuardCmdCloseTab {
				t.Errorf("%q: команда closeTab %+v", d, c)
			}
		}
		h.svc.mu.Lock()
		got := h.svc.children[h.childID(childSID)].sample.ForegroundDomain
		h.svc.mu.Unlock()
		if got != "" {
			t.Errorf("%q: в снимке домен %q, want «»", d, got)
		}
	}
	logs := h.logText()
	if strings.Contains(logs, "secret") || strings.Contains(logs, "token") {
		t.Errorf("журнал работы содержит адрес страницы: %q", logs)
	}
	for _, e := range h.store.events {
		if s := fmt.Sprint(e.Details); strings.Contains(s, "secret") || strings.Contains(s, "token") {
			t.Errorf("журнал родителя содержит адрес страницы: %s %s", e.Kind, s)
		}
	}
	if n := strings.Count(logs, "страж прислал домен, не похожий на домен"); n != 1 {
		t.Errorf("строк о мусорном домене %d, want 1:\n%s", n, logs)
	}
}
