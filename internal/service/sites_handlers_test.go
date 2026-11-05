package service

import (
	"fmt"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// journalTexts — строки журнала родителя вида kind, как их видит родитель: событие ребёнка с
// его именем, событие не про ребёнка — без имени.
func (h *harness) journalTexts(kind string) []string {
	out := []string{}
	for _, e := range h.store.eventsOf(kind) {
		name := ""
		if e.ChildID != 0 {
			name = h.store.children[e.ChildID].Name
		}
		out = append(out, eventText(e.Kind, name, e.Details))
	}
	return out
}

// listSites — ListSites от родителя.
func (h *harness) listSites(tok string) ipc.ListSitesReply {
	h.t.Helper()
	var rep ipc.ListSitesReply
	h.ok(adminPeer(), tok, ipc.CmdListSites, nil, &rep)
	return rep
}

// hasCloseTab — в ответе на опрос есть closeTab с доменом domain.
func hasCloseTab(rep ipc.GuardPollReply, domain string) bool {
	for _, c := range rep.Commands {
		if c.Kind == ipc.GuardCmdCloseTab && c.Domain == domain {
			return true
		}
	}
	return false
}

func siteKindOf(rep ipc.ListSitesReply, domain string) string {
	for _, s := range rep.Sites {
		if s.Domain == domain {
			return s.Kind
		}
	}
	return ""
}

// expectFieldFail — отказ text у поля field.
func expectFieldFail(t *testing.T, rep ipc.Reply, text, field string) {
	t.Helper()
	if rep.Error != text || rep.Field != field {
		t.Errorf("отказ %q (поле %q), want %q (поле %q)", rep.Error, rep.Field, text, field)
	}
}

// Ловит (FR-21, 8.3.5): экран «Сайты» без стартового набора, не по алфавиту, без переключателя
// «18+» ребёнка, без предела «Запрещённых» и подписи источника списка «18+».
func TestListSites(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	rep := h.listSites(h.login(adminPeer()))
	want := ipc.ListSitesReply{
		Sites: []ipc.SiteDTO{
			{"discord.com", "video_social"}, {"ok.ru", "video_social"}, {"rutube.ru", "video_social"},
			{"telegram.org", "video_social"}, {"tiktok.com", "video_social"}, {"twitch.tv", "video_social"},
			{"vk.com", "video_social"}, {"web.telegram.org", "video_social"}, {"youtube.com", "video_social"},
		},
		Children:    []ipc.AdultFilterDTO{{ChildID: h.childID(childSID), Name: "Миша", On: true}},
		BlockedMax:  1000,
		AdultSource: "Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0",
	}
	if jsonOf(t, rep) != jsonOf(t, want) {
		t.Errorf("ListSites\n got %s\nwant %s", jsonOf(t, rep), jsonOf(t, want))
	}
}

// Ловит (FR-21, RM4-4, RM4-5, S-12): вставленный адрес не сводится к домену, повтор в тот же
// список принимается, не домен проходит, перенос не сообщается, неизвестный список принимается,
// журнал родителя не пишется или с именем ребёнка, списки службы не перечитаны (вкладка не
// закрывается), флаг правил Brave не ставится.
func TestAddSite(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withAdult())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	var add ipc.AddSiteReply
	h.ok(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "https://www.Example.com/page?x=1", Kind: "blocked"}, &add)
	if add.Message != "Добавлено: example.com" || add.Domain != "example.com" || add.Moved {
		t.Errorf("добавление: %+v", add)
	}
	if siteKindOf(add.Sites, "example.com") != "blocked" {
		t.Errorf("в ответе списки без example.com: %s", jsonOf(t, add.Sites.Sites))
	}
	h.svc.mu.Lock()
	due := h.svc.braveDue
	h.svc.mu.Unlock()
	if !due {
		t.Error("правила Brave не отмечены к записи")
	}
	if rep := h.poll(braveSample("example.com")); !hasCloseTab(rep, "example.com") {
		t.Errorf("после добавления вкладка не закрывается: %+v", rep.Commands)
	}
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "example.com", Kind: "blocked"}), "Этот сайт уже в списке", "domain")
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "просто текст", Kind: "blocked"}), "Это не похоже на адрес сайта", "domain")
	h.ok(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "youtube.com", Kind: "study"}, &add)
	if add.Message != "Сайт перенесён в «Учёбу»" || add.Domain != "youtube.com" || !add.Moved {
		t.Errorf("перенос: %+v", add)
	}
	if rep := h.fail(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "x.ru", Kind: "video"}); rep.Error != ipc.TextBadRequest {
		t.Errorf("неизвестный список: %q", rep.Error)
	}
	if _, ok := h.store.sites["x.ru"]; ok {
		t.Error("x.ru записан в неизвестный список")
	}
	if got := h.journalTexts(evSiteAdded); jsonOf(t, got) != jsonOf(t, []string{"Вы добавили сайт example.com в «Запрещённые»"}) {
		t.Errorf("журнал добавления %q", got)
	}
	if got := h.journalTexts(evSiteMoved); jsonOf(t, got) != jsonOf(t, []string{"Вы перенесли сайт youtube.com в «Учёбу»"}) {
		t.Errorf("журнал переноса %q", got)
	}
}

// Ловит (R14, RM4-10): в «Запрещённые» уходит 1001-й домен (в том числе переносом), предел
// проверяется раньше повтора, предел мешает другим спискам.
func TestAddSiteBlockedLimit(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), func(h *harness) {
		for i := 0; i < 1000; i++ {
			h.store.sites[fmt.Sprintf("s%d.example", i)] = "blocked"
		}
	})
	h.start()
	tok := h.login(adminPeer())
	const full = "В списке запрещённых уже 1000 сайтов. Удалите ненужные"
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "one-more.example", Kind: "blocked"}), full, "domain")
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "s5.example", Kind: "blocked"}), "Этот сайт уже в списке", "domain")
	h.ok(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "one-more.example", Kind: "study"}, nil)
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "youtube.com", Kind: "blocked"}), full, "domain")
	if n, _ := h.store.CountSiteRules("blocked"); n != 1000 {
		t.Errorf("запрещённых %d, want 1000", n)
	}
	if h.store.sites["youtube.com"] != "video_social" {
		t.Errorf("youtube.com в %q, want video_social", h.store.sites["youtube.com"])
	}
}

// Ловит (FR-21, S-15): удаление не отвечает прежним списком и сроком «Отменить», списки службы
// не перечитаны (вкладка всё ещё закрывается), повтор удаления — успех, «Отменить» (AddSite) не
// возвращает запрет, журнал родителя не пишется.
func TestRemoveSite(t *testing.T) {
	h := siteHarness(t)
	tok := h.login(adminPeer())
	var rm ipc.RemoveSiteReply
	h.ok(adminPeer(), tok, ipc.CmdRemoveSite, ipc.RemoveSiteRequest{Domain: "example.com"}, &rm)
	if rm.Message != "Удалён example.com" || rm.Domain != "example.com" || rm.Kind != "blocked" || rm.UndoUntilMs != realT0.Add(10*time.Second).UnixMilli() {
		t.Errorf("удаление: %+v", rm)
	}
	if siteKindOf(rm.Sites, "example.com") != "" {
		t.Error("в ответе example.com всё ещё в списках")
	}
	if rep := h.poll(braveSample("example.com")); len(rep.Commands) != 0 {
		t.Errorf("после удаления команды %+v", rep.Commands)
	}
	if rep := h.fail(adminPeer(), tok, ipc.CmdRemoveSite, ipc.RemoveSiteRequest{Domain: "example.com"}); rep.Error != "Этого сайта уже нет в списках" {
		t.Errorf("повтор удаления: %q", rep.Error)
	}
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdRemoveSite, ipc.RemoveSiteRequest{Domain: "просто текст"}), "Это не похоже на адрес сайта", "domain")
	h.ok(adminPeer(), tok, ipc.CmdAddSite, ipc.AddSiteRequest{Domain: "example.com", Kind: "blocked"}, nil)
	h.real.Advance(10 * time.Second) // плашка той же вкладки не чаще plateEvery: проверяем только закрытие
	if rep := h.poll(braveSample("example.com")); !hasCloseTab(rep, "example.com") {
		t.Errorf("после «Отменить» вкладка не закрывается: %+v", rep.Commands)
	}
	if got := h.journalTexts(evSiteRemoved); jsonOf(t, got) != jsonOf(t, []string{"Вы удалили сайт example.com из «Запрещённых»"}) {
		t.Errorf("журнал удаления %q", got)
	}
}

// Ловит (R14): переключатель «18+» не доходит до службы без перезапуска, не отражается в списке,
// сообщение и журнал не о ребёнке, чужой ребёнок принимается.
func TestSetAdultFilter(t *testing.T) {
	h := siteHarness(t)
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	var rep ipc.SetAdultFilterReply
	h.ok(adminPeer(), tok, ipc.CmdSetAdultFilter, ipc.SetAdultFilterRequest{ChildID: id, On: false}, &rep)
	if rep.Message != "Миша: сайты 18+ не блокируются" || len(rep.Sites.Children) != 1 || rep.Sites.Children[0].On {
		t.Errorf("выключение: %+v", rep)
	}
	if !h.store.adultOff[id] {
		t.Error("выключение не записано")
	}
	if p := h.poll(braveSample("adult-probe.test")); len(p.Commands) != 0 {
		t.Errorf("«18+» выключен, а команды %+v", p.Commands)
	}
	h.ok(adminPeer(), tok, ipc.CmdSetAdultFilter, ipc.SetAdultFilterRequest{ChildID: id, On: true}, &rep)
	if rep.Message != "Миша: сайты 18+ блокируются" || !rep.Sites.Children[0].On {
		t.Errorf("включение: %+v", rep)
	}
	if p := h.poll(braveSample("adult-probe.test")); !hasCloseTab(p, "adult-probe.test") {
		t.Errorf("«18+» включён, а команды %+v", p.Commands)
	}
	if r := h.fail(adminPeer(), tok, ipc.CmdSetAdultFilter, ipc.SetAdultFilterRequest{ChildID: 999, On: false}); r.Error != textNoSuchChild {
		t.Errorf("чужой ребёнок: %q", r.Error)
	}
	want := []string{"Миша: сайты 18+ не блокируются", "Миша: сайты 18+ блокируются"}
	if got := h.journalTexts(evSiteAdultFilter); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("журнал %q, want %q", got, want)
	}
}

// Ловит (R18): сбой хранилища при чтении списков отвечает успехом с пустыми списками.
func TestListSitesStoreFailure(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	h.store.mu.Lock()
	h.store.failSites = true
	h.store.mu.Unlock()
	if rep := h.fail(adminPeer(), tok, ipc.CmdListSites, nil); rep.Error != textRetry {
		t.Errorf("сбой чтения: %q", rep.Error)
	}
}
