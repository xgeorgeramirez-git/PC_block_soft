package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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

// siteCmds — команды стража о сайтах (closeTab, closeBraveWindow) и плашки, по порядку ответа.
func siteCmds(rep ipc.GuardPollReply) []ipc.GuardCommand {
	out := []ipc.GuardCommand{}
	for _, c := range rep.Commands {
		switch c.Kind {
		case ipc.GuardCmdCloseTab, ipc.GuardCmdCloseBraveWindow, ipc.GuardCmdBlocked:
			out = append(out, c)
		}
	}
	return out
}

// closes — команды закрытия вкладки или окна в ответе.
func closes(rep ipc.GuardPollReply) []ipc.GuardCommand {
	out := []ipc.GuardCommand{}
	for _, c := range rep.Commands {
		if c.Kind == ipc.GuardCmdCloseTab || c.Kind == ipc.GuardCmdCloseBraveWindow {
			out = append(out, c)
		}
	}
	return out
}

// Ловит (R14, 8.2.5 п.1–2, RM4-9, PL-16, S-9, S-10): запрещённая и «18+» вкладка не закрывается в
// том же ответе, плашка раньше команды, не тот текст (домен у «18+» виден ребёнку, у поддомена —
// не запись списка), повтор команды не идёт каждый опрос, плашка и нарушение повторяются чаще
// своих сроков, ограничитель прячет первое событие, разрешённое закрывается.
func TestSiteTabClosedWithPlate(t *testing.T) {
	h := siteHarness(t)
	rep := h.step(braveSample("m.example.com"))
	cmds := siteCmds(rep)
	if len(cmds) != 2 || cmds[0].Kind != ipc.GuardCmdCloseTab || cmds[0].Domain != "m.example.com" || cmds[1].Kind != ipc.GuardCmdBlocked {
		t.Fatalf("команды %s, want closeTab{m.example.com}, затем плашка", jsonOf(t, cmds))
	}
	wantPlate := []ipc.BlockedPlate{{Kind: "site", Text: "Сайт example.com закрыт: он в списке запрещённых. Если он нужен, попроси родителей", ShowS: 10, Path: ""}}
	if got := platesOf(rep); jsonOf(t, got) != jsonOf(t, wantPlate) {
		t.Errorf("плашка %s, want %s", jsonOf(t, got), jsonOf(t, wantPlate))
	}
	if got := h.eventTexts("site_forbidden_try"); jsonOf(t, got) != jsonOf(t, []string{"Миша: попытка открыть запрещённое: сайт m.example.com"}) {
		t.Errorf("журнал %q", got)
	}
	evs := h.store.eventsOf("site_forbidden_try")
	if len(evs) != 1 || jsonOf(t, evs[0].Details) != jsonOf(t, map[string]any{"domain": "m.example.com", "kind": "blocked"}) {
		t.Fatalf("подробности %s, want {domain: m.example.com, kind: blocked}", jsonOf(t, evs))
	}
	rep = h.step(braveSample("m.example.com")) // через 1 с
	if len(closes(rep)) != 1 || len(platesOf(rep)) != 0 || len(h.store.eventsOf("site_forbidden_try")) != 1 {
		t.Errorf("через 1 с: закрытий %d, плашек %d, записей %d; want 1, 0, 1", len(closes(rep)), len(platesOf(rep)), len(h.store.eventsOf("site_forbidden_try")))
	}
	h.run(3, braveSample("m.example.com"))
	rep = h.step(braveSample("m.example.com")) // 5 с от первой
	if len(platesOf(rep)) != 1 {
		t.Errorf("через 5 с плашек %d, want 1 (plateEvery)", len(platesOf(rep)))
	}
	h.run(54, braveSample("m.example.com"))
	if n := len(h.store.eventsOf("site_forbidden_try")); n != 1 {
		t.Errorf("через 59 с записей %d, want 1", n)
	}
	h.step(braveSample("m.example.com")) // 60 с от первой
	if n := len(h.store.eventsOf("site_forbidden_try")); n != 2 {
		t.Errorf("через 60 с записей %d, want 2 (watchRepeatEvery)", n)
	}

	rep = h.step(braveSample("adult-probe.test"))
	if c := closes(rep); len(c) != 1 || c[0].Domain != "adult-probe.test" {
		t.Errorf("18+: закрытия %s, want closeTab{adult-probe.test}", jsonOf(t, c))
	}
	wantAdult := []ipc.BlockedPlate{{Kind: "adult", Text: "Сайт закрыт: он в списке сайтов для взрослых", ShowS: 10}}
	if got := platesOf(rep); jsonOf(t, got) != jsonOf(t, wantAdult) {
		t.Errorf("плашка 18+ %s, want %s (плашка запрещённого сайта её не глушит)", jsonOf(t, got), jsonOf(t, wantAdult))
	}
	want := []string{"Миша: попытка открыть запрещённое: сайт m.example.com", "Миша: попытка открыть запрещённое: сайт m.example.com",
		"Миша: попытка открыть запрещённое: сайт для взрослых adult-probe.test"}
	if got := h.eventTexts("site_forbidden_try"); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("журнал %q, want %q", got, want)
	}
	for _, d := range []string{"news.example", "youtube.com", ""} {
		if c := siteCmds(h.step(braveSample(d))); len(c) != 0 {
			t.Errorf("%q: команды %s, want нет", d, jsonOf(t, c))
		}
	}
}

// Ловит (R6, 8.2.5 п.3): при исчерпанном видео закрываются и другие сайты, вкладка видео не
// закрывается или пишется нарушением, секунда видео засчитывается.
func TestSiteVideoOverClosesOnlyVideo(t *testing.T) {
	h := siteHarness(t, withVideo(childSID, 0))
	h.run(1, idleSample)
	rep := h.step(braveSample("m.youtube.com"))
	if c := closes(rep); len(c) != 1 || c[0].Kind != ipc.GuardCmdCloseTab || c[0].Domain != "m.youtube.com" {
		t.Errorf("закрытия %s, want closeTab{m.youtube.com}", jsonOf(t, c))
	}
	want := []ipc.BlockedPlate{{Kind: "videoOver", Text: "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 2 ч 00 мин", ShowS: 10}}
	if got := platesOf(rep); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("плашка %s, want %s", jsonOf(t, got), jsonOf(t, want))
	}
	if got := h.eventTexts("site_closed_video"); jsonOf(t, got) != jsonOf(t, []string{"Миша: закрыт сайт m.youtube.com: время на видео и соцсети закончилось"}) {
		t.Errorf("журнал %q", got)
	}
	if n := len(h.store.eventsOf("site_forbidden_try")); n != 0 {
		t.Errorf("нарушений %d, want 0", n)
	}
	if c := siteCmds(h.step(braveSample("news.example"))); len(c) != 0 {
		t.Errorf("news.example: команды %s, want нет (другие сайты работают)", jsonOf(t, c))
	}
	h.svc.Shutdown(h.real.Now())
	if st := h.store.days[dayKey(h.childID(childSID), "2026-10-19")]; st.VideoS != 0 {
		t.Errorf("видео засчитано %d с, want 0", st.VideoS)
	}
}

// Ловит (R8, RM4-6, S-14, RM4-1): в «только учёбе» не закрываются сайты не из «Учёбы», закрываются
// сайты учёбы или служебная страница (Brave стал бы непригоден), наблюдатель убивает Brave целиком.
func TestSiteStudyOnly(t *testing.T) {
	w := newWatcher()
	h := siteHarness(t, withRules(0, -1, "study_only"), withWatcher(w))
	rep := h.step(braveSample("news.example"))
	if c := closes(rep); len(c) != 1 || c[0].Domain != "news.example" {
		t.Errorf("закрытия %s, want closeTab{news.example}", jsonOf(t, c))
	}
	want := []ipc.BlockedPlate{{Kind: "studyOnly", Text: "Время вышло. Сейчас доступна только учёба", ShowS: 10}}
	if got := platesOf(rep); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("плашка %s, want %s", jsonOf(t, got), jsonOf(t, want))
	}
	if got := h.eventTexts("site_closed_study_only"); jsonOf(t, got) != jsonOf(t, []string{"Миша: закрыт сайт news.example: время вышло, доступна только учёба"}) {
		t.Errorf("журнал %q", got)
	}
	for _, d := range []string{"school.example", ""} {
		if c := siteCmds(h.step(braveSample(d))); len(c) != 0 {
			t.Errorf("%q: команды %s, want нет", d, jsonOf(t, c))
		}
	}
	brave := w.start(childSession, pathBrave)
	mc := w.start(childSession, pathMinecraft)
	h.watch()
	if !w.alive(brave) || w.alive(mc) {
		t.Errorf("наблюдатель: Brave жив %v, Minecraft жив %v; want true, false", w.alive(brave), w.alive(mc))
	}
}

// Ловит (RM4-1, S-3, RK5): окно Brave без адреса закрывается сразу (холодное дерево доступности),
// отсчёт не сбрасывается прочитанным адресом, окно закрывается без «только учёбы» и исчерпанного
// видео, запись в журнал родителя чаще раза в день, в журнал работы — чаще раза в 10 мин.
func TestBraveWindowWithoutAddress(t *testing.T) {
	h := siteHarness(t, withRules(0, -1, "study_only"))
	unread := braveSample("")
	unread.DomainUnread, unread.DomainNoAddress = true, true
	for i := 1; i <= 5; i++ {
		if c := closes(h.step(unread)); len(c) != 0 {
			t.Fatalf("шаг %d: закрытия %s, want нет до 5 с", i, jsonOf(t, c))
		}
	}
	rep := h.step(unread)
	if c := siteCmds(rep); jsonOf(t, c) != jsonOf(t, []ipc.GuardCommand{{Kind: ipc.GuardCmdCloseBraveWindow}}) {
		t.Fatalf("шаг 6: команды %s, want closeBraveWindow без плашки", jsonOf(t, c))
	}
	h.step(braveSample("school.example"))
	for i := 1; i <= 5; i++ {
		if c := closes(h.step(unread)); len(c) != 0 {
			t.Fatalf("после прочитанного адреса шаг %d: закрытия %s, want нет (отсчёт сброшен)", i, jsonOf(t, c))
		}
	}
	if c := closes(h.step(unread)); len(c) != 1 {
		t.Errorf("после сброса шаг 6: закрытий %d, want 1", len(c))
	}

	h2 := siteHarness(t)
	for i := 0; i < 10; i++ {
		if c := siteCmds(h2.step(unread)); len(c) != 0 {
			t.Fatalf("без «только учёбы» шаг %d: команды %s, want нет", i+1, jsonOf(t, c))
		}
	}
	if got := h2.eventTexts("site_unread"); jsonOf(t, got) != jsonOf(t, []string{"Миша: не удалось прочитать адрес сайта в Brave"}) {
		t.Errorf("журнал родителя %q, want одна запись", got)
	}
	if n := strings.Count(h2.logText(), "не удалось прочитать адрес сайта в Brave"); n != 1 {
		t.Errorf("строк в журнале работы %d за 10 с, want 1", n)
	}
	h2.run(9*60+48, unread)
	if n := strings.Count(h2.logText(), "не удалось прочитать адрес сайта в Brave"); n != 1 {
		t.Errorf("строк в журнале работы %d до 10 мин, want 1", n)
	}
	h2.run(3, unread)
	if n := strings.Count(h2.logText(), "не удалось прочитать адрес сайта в Brave"); n != 2 {
		t.Errorf("строк в журнале работы %d после 10 мин, want 2", n)
	}
	h2.clock.Advance(24 * time.Hour)
	h2.step(unread)
	if n := len(h2.eventTexts("site_unread")); n != 2 {
		t.Errorf("записей в журнале родителя на следующий день %d, want 2", n)
	}
	if h2.svc.bypassWait != nil {
		t.Errorf("закрытие окна Brave ждёт решения об обходе: %+v", h2.svc.bypassWait)
	}
}

// Ловит (S-13, PL-9, PL-18, R1, R8, R11, R18): вкладка закрывается у администратора, в отключённом
// сеансе, на паузе, в безопасном режиме, у другого браузера, под экраном блокировки; скрытый
// родителем Brave перестаёт закрывать запрещённое (PL-30).
func TestSiteGuardHoldsOff(t *testing.T) {
	blocked := braveSample("m.example.com")
	chrome := fgSample(pathChrome, ipc.CatNameOther)
	chrome.ForegroundDomain = "example.com"
	cases := []struct {
		name   string
		opts   []hopt
		setup  func(h *harness)
		sample ipc.SensorSample
	}{
		{"администратор", nil, func(h *harness) {
			h.admins.set(childSID, true)
			h.svc.mu.Lock()
			h.svc.checkAdmin(h.svc.children[h.childID(childSID)], h.real.Now())
			h.svc.mu.Unlock()
		}, blocked},
		{"сеанс отключён", nil, func(h *harness) { h.svc.OnSession(SessionDisconnect, childSession, h.real.Now()) }, blocked},
		{"пауза", nil, func(h *harness) {
			tok := h.login(adminPeer())
			h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: h.childID(childSID), Preset: ipc.PauseHour1}, nil)
		}, blocked},
		{"безопасный режим", []hopt{func(h *harness) { h.store.failLimits = true }}, nil, blocked},
		{"другой браузер", nil, nil, chrome},
		{"экран блокировки", []hopt{withRules(0, -1, "lock")}, nil, blocked},
	}
	for _, c := range cases {
		h := siteHarness(t, c.opts...)
		if c.setup != nil {
			c.setup(h)
		}
		for i := 0; i < 3; i++ {
			rep := h.do(guardPeer(), "", ipc.CmdGuardPoll, ipc.GuardPollRequest{Sample: c.sample})
			var r ipc.GuardPollReply
			if rep.OK {
				rep.Into(&r)
			}
			if cl := closes(r); len(cl) != 0 {
				t.Errorf("%s: закрытия %s, want нет", c.name, jsonOf(t, cl))
			}
			h.real.Advance(time.Second)
		}
	}
	h := siteHarness(t)
	h.hide(h.login(adminPeer()), true, pathBrave)
	if c := closes(h.step(blocked)); len(c) != 1 {
		t.Errorf("скрытый Brave: закрытий %d, want 1 (запрет сильнее скрытия)", len(c))
	}
	// Скрытую программу правила времени не закрывают (D-57): в «только учёбе» прочий сайт открыт.
	h = siteHarness(t, withRules(0, -1, "study_only"))
	h.hide(h.login(adminPeer()), true, pathBrave)
	if c := closes(h.step(braveSample("news.example"))); len(c) != 0 {
		t.Errorf("скрытый Brave в «только учёбе»: закрытия %s, want нет", jsonOf(t, c))
	}
}

// Ловит (R6, D-54, PL-14): видео-предупреждения не доходят до стража, повторяются, идут без видео
// на переднем плане (игры на переднем плане при лимите видео).
func TestVideoWarningsOnlyOnVideoForeground(t *testing.T) {
	type got struct{ threshold, remaining int }
	h := siteHarness(t, withVideo(childSID, 31))
	var warns []got
	var text30 string
	for i := 0; i < 31*60+2; i++ {
		rep := h.step(braveSample("youtube.com"))
		for _, w := range rep.Warnings {
			if w.Event != "video" || !w.Sound {
				t.Fatalf("предупреждение %+v: want видео со звуком", w)
			}
			if w.Threshold == 30 {
				text30 = w.Text
			}
			warns = append(warns, got{w.Threshold, h.status().VideoRemainingS})
		}
	}
	want := []got{{30, 1799}, {15, 899}, {5, 299}, {1, 59}}
	if jsonOf(t, warns) != jsonOf(t, want) {
		t.Errorf("предупреждения %v, want %v", warns, want)
	}
	if text30 != "Через 30 минут время на видео и соцсети закончится" {
		t.Errorf("текст 30 мин %q", text30)
	}
	if !h.status().CloseVideo {
		t.Error("после лимита видео CloseVideo = false")
	}

	h2 := siteHarness(t, withVideo(childSID, 31))
	for i := 0; i < 31*60+2; i++ {
		for _, w := range h2.step(fgSample(pathMinecraft, ipc.CatNameOther)).Warnings {
			if w.Event == "video" {
				t.Fatalf("шаг %d: видео-предупреждение %+v при игре на переднем плане", i, w)
			}
		}
	}
}

// Ловит (Review Focus 7): наблюдатель и опрос стража одновременно трогают ограничитель повторов
// и очередь стража без s.mu (виден только под -race).
func TestSiteGuardAndWatchConcurrent(t *testing.T) {
	w := newWatcher()
	h := siteHarness(t, withRules(0, -1, "study_only"), withWatcher(w))
	w.start(childSession, pathMinecraft)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			h.svc.Watch(realT0.Add(time.Duration(i) * time.Second))
		}
	}()
	for i := 0; i < 50; i++ {
		h.do(guardPeer(), "", ipc.CmdGuardPoll, ipc.GuardPollRequest{Sample: braveSample("news.example")})
	}
	<-done
}

// Ловит (B-55, PL-46): 5 с отсчитываются по «адреса нет», а не по любому сбою чтения. 4 с сбоев
// UIA, затем «адреса нет» — окно не закрывается на первом «нет адреса», только через 5 с «нет»
// подряд; сбой посреди «нет» начинает отсчёт заново; одни сбои окно не закрывают никогда, но
// запись «не удалось прочитать адрес» в журнал родителя дают (адрес и правда не читается).
func TestBraveCloseCountsNoAddressOnly(t *testing.T) {
	h := siteHarness(t, withRules(0, -1, "study_only"))
	failed := braveSample("")
	failed.DomainUnread = true
	none := braveSample("")
	none.DomainUnread, none.DomainNoAddress = true, true
	for i := 1; i <= 5; i++ {
		if c := closes(h.step(failed)); len(c) != 0 {
			t.Fatalf("сбой %d: закрытия %s, want нет", i, jsonOf(t, c))
		}
	}
	for i := 1; i <= 5; i++ {
		if c := closes(h.step(none)); len(c) != 0 {
			t.Fatalf("после 4 с сбоев «нет адреса» %d: закрытия %s, want нет до 5 с «нет» подряд", i, jsonOf(t, c))
		}
	}
	if c := closes(h.step(none)); len(c) != 1 {
		t.Fatalf("5 с «нет адреса» подряд: закрытий %d, want 1", len(c))
	}
	h.step(braveSample("school.example"))
	h.run(3, none)
	h.step(failed)
	for i := 1; i <= 5; i++ {
		if c := closes(h.step(none)); len(c) != 0 {
			t.Fatalf("сбой посреди «нет адреса», шаг %d: закрытия %s, want нет (отсчёт заново)", i, jsonOf(t, c))
		}
	}
	if c := closes(h.step(none)); len(c) != 1 {
		t.Errorf("после сбоя 5 с «нет адреса»: закрытий %d, want 1", len(c))
	}

	h2 := siteHarness(t, withRules(0, -1, "study_only"))
	for i := 1; i <= 20; i++ {
		if c := closes(h2.step(failed)); len(c) != 0 {
			t.Fatalf("только сбои, шаг %d: закрытия %s, want нет", i, jsonOf(t, c))
		}
	}
	if got := h2.eventTexts("site_unread"); jsonOf(t, got) != jsonOf(t, []string{"Миша: не удалось прочитать адрес сайта в Brave"}) {
		t.Errorf("только сбои: журнал родителя %q, want одна запись", got)
	}
}

// Ловит (ревью M4 F1-5): «не удалось прочитать адрес сайта в Brave» в журнале родителя после
// короткого перерыва чтения (свежее окно строит дерево доступности до 3,5 с), а не только когда
// адрес не читается подряд 5 с.
func TestBraveUnreadJournalOnlyAfterCloseDelay(t *testing.T) {
	h := siteHarness(t)
	unread := braveSample("")
	unread.DomainUnread = true
	h.run(3, unread)
	h.step(braveSample("school.example"))
	if got := h.eventTexts("site_unread"); len(got) != 0 {
		t.Fatalf("3 с без адреса, затем адрес прочитан: журнал родителя %q, want пусто", got)
	}
	h.run(5, unread)
	if got := h.eventTexts("site_unread"); len(got) != 0 {
		t.Fatalf("5 опросов (4 с подряд) без адреса: журнал родителя %q, want пусто", got)
	}
	h.step(unread)
	if got := h.eventTexts("site_unread"); jsonOf(t, got) != jsonOf(t, []string{"Миша: не удалось прочитать адрес сайта в Brave"}) {
		t.Errorf("6 опросов (5 с подряд) без адреса: журнал родителя %q, want одна запись", got)
	}
	h.run(10, unread)
	if n := len(h.eventTexts("site_unread")); n != 1 {
		t.Errorf("ещё 10 с без адреса: записей %d, want 1 (раз в день)", n)
	}
}
