package service

import (
	"fmt"
	"sort"
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
			{Domain: "discord.com", Kind: "video_social"}, {Domain: "ok.ru", Kind: "video_social"}, {Domain: "rutube.ru", Kind: "video_social"},
			{Domain: "telegram.org", Kind: "video_social"}, {Domain: "tiktok.com", Kind: "video_social"}, {Domain: "twitch.tv", Kind: "video_social"},
			{Domain: "vk.com", Kind: "video_social"}, {Domain: "web.telegram.org", Kind: "video_social"}, {Domain: "youtube.com", Kind: "video_social"},
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

// videoSites — домены «Видео и соцсетей» фейка по алфавиту.
func (h *harness) videoSites() []string {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	out := []string{}
	for d, k := range h.store.sites {
		if k == ipc.SiteKindVideo {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// Ловит (FR-2, R14, S-4 пакета T3): шаг мастера «Сайты» не заменяет «Видео и соцсети», не сводит
// ввод к домену, оставляет повторы, не выключает «18+» выбранному ребёнку или выключает другому;
// не домен записывает пароль (настройка закончена с ошибкой) или трогает списки; без шага списки
// и «18+» меняются.
func TestSetupSites(t *testing.T) {
	setup := func(sites *ipc.SetupSites) (*harness, ipc.Reply) {
		h := newHarness(t)
		h.start()
		req := ipc.SetupRequest{Password: goodPassword, Children: []ipc.SetupChild{
			{SID: childSID, Name: "Миша", Limits: sevenDays(120)}, {SID: otherSID, Name: "Даша", Limits: sevenDays(120)},
		}, Sites: sites}
		return h, h.do(elevatedPeer(), "", ipc.CmdSetup, req)
	}
	h, rep := setup(&ipc.SetupSites{AdultOff: []string{childSID, "S-1-5-21-9-9-9-9999"}, Video: []string{"youtube.com", "https://www.Kids.example/x", "vk.com", "vk.com"}})
	if !rep.OK {
		t.Fatalf("Setup: %q", rep.Error)
	}
	if got := h.videoSites(); jsonOf(t, got) != jsonOf(t, []string{"kids.example", "vk.com", "youtube.com"}) {
		t.Errorf("видео %q", got)
	}
	if !h.store.adultOff[h.childID(childSID)] || h.store.adultOff[h.childID(otherSID)] || len(h.store.adultOff) != 1 {
		t.Errorf("18+ выключен у %v, want только у Миши", h.store.adultOff)
	}
	list := h.listSites(h.login(adminPeer()))
	want := []ipc.AdultFilterDTO{{ChildID: h.childID(childSID), Name: "Миша", On: false}, {ChildID: h.childID(otherSID), Name: "Даша", On: true}}
	if jsonOf(t, list.Children) != jsonOf(t, want) {
		t.Errorf("переключатели %s", jsonOf(t, list.Children))
	}
	h.svc.mu.Lock()
	due := h.svc.braveDue
	c, _ := h.svc.classifySite("m.kids.example", true)
	h.svc.mu.Unlock()
	if !due || c != siteVideo {
		t.Errorf("после мастера правила Brave %v, kids.example — %v; want true, siteVideo", due, c)
	}

	h, rep = setup(&ipc.SetupSites{Video: []string{"youtube.com", "просто текст"}})
	if rep.OK || rep.Error != "Это не похоже на адрес сайта" || rep.Field != "domain" {
		t.Errorf("не домен: %+v", rep)
	}
	if h.store.auth != nil || len(h.videoSites()) != 9 || len(h.store.children) != 0 {
		t.Errorf("не домен: пароль %v, видео %d, детей %d; want ничего не записано", h.store.auth, len(h.videoSites()), len(h.store.children))
	}

	many := make([]string, 0, 1001)
	for i := 0; i < 1000; i++ {
		many = append(many, fmt.Sprintf("v%d.example", i))
	}
	if _, rep = setup(&ipc.SetupSites{Video: append(many, "v0.example")}); !rep.OK {
		t.Errorf("1000 доменов и повтор: %q, want успех (повторы схлопываются до предела)", rep.Error)
	}
	if _, rep = setup(&ipc.SetupSites{Video: append(many, "v1000.example")}); rep.OK || rep.Error != ipc.TextBadRequest {
		t.Errorf("1001 домен: %+v, want неверный запрос", rep)
	}

	h, rep = setup(nil)
	if !rep.OK || len(h.videoSites()) != 9 || len(h.store.adultOff) != 0 {
		t.Errorf("без шага: %q, видео %d, 18+ выключен %v", rep.Error, len(h.videoSites()), h.store.adultOff)
	}
}

// Ловит (S-4 пакета T3): списки мастера пишутся после пароля, и оборванная настройка (пароль не
// записан) оставляет стартовый набор, хотя мастер проходится заново.
func TestSetupSitesBeforeAuth(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.store.failSetAuth = true
	req := ipc.SetupRequest{Password: goodPassword, Children: []ipc.SetupChild{{SID: childSID, Name: "Миша", Limits: sevenDays(120)}},
		Sites: &ipc.SetupSites{Video: []string{"kids.example"}}}
	h.fail(elevatedPeer(), "", ipc.CmdSetup, req)
	if got := h.videoSites(); jsonOf(t, got) != jsonOf(t, []string{"kids.example"}) {
		t.Errorf("видео при оборванной настройке %q, want записано до пароля", got)
	}
}

// Ловит (8.3.5 п.2, B-49): вставка нескольких адресов по одному в строке не делится по строкам,
// адрес страницы не сводится к домену, повтор в пачке пишется дважды, уже бывший в списке
// считается добавленным, перенос не отмечен, не-адрес теряется (окно должно вернуть его в поле),
// журнал не по сайту, списки службы не перечитаны (вкладка не закрывается).
func TestAddSites(t *testing.T) {
	h := siteHarness(t)
	tok := h.login(adminPeer())
	var rep ipc.AddSitesReply
	text := "https://www.New.example/page?x=1\r\nyoutube.com\n\n  просто текст  \nexample.com\nnew.example\n"
	h.ok(adminPeer(), tok, ipc.CmdAddSites, ipc.AddSitesRequest{Text: text, Kind: "blocked"}, &rep)
	want := ipc.AddSitesReply{
		Message:    "Добавлено сайтов: 1. Перенесено сюда из других списков: 1. Уже были в списке: 1",
		Added:      []string{"new.example", "youtube.com"},
		Rejected:   []string{"просто текст"},
		FieldError: "Это не похоже на адрес сайта",
	}
	rep.Sites = ipc.ListSitesReply{}
	if jsonOf(t, rep) != jsonOf(t, want) {
		t.Errorf("AddSites\n got %s\nwant %s", jsonOf(t, rep), jsonOf(t, want))
	}
	if h.store.sites["new.example"] != "blocked" || h.store.sites["youtube.com"] != "blocked" || h.store.sites["example.com"] != "blocked" {
		t.Errorf("списки после вставки %v", h.store.sites)
	}
	if got := h.journalTexts(evSiteAdded); jsonOf(t, got) != jsonOf(t, []string{"Вы добавили сайт new.example в «Запрещённые»"}) {
		t.Errorf("журнал добавления %q", got)
	}
	if got := h.journalTexts(evSiteMoved); jsonOf(t, got) != jsonOf(t, []string{"Вы перенесли сайт youtube.com в «Запрещённые»"}) {
		t.Errorf("журнал переноса %q", got)
	}
	if p := h.poll(braveSample("new.example")); !hasCloseTab(p, "new.example") {
		t.Errorf("после вставки вкладка не закрывается: %+v", p.Commands)
	}
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSites, ipc.AddSitesRequest{Text: "просто текст\nещё текст", Kind: "blocked"}),
		"Эти строки не похожи на адрес сайта", "domain")
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSites, ipc.AddSitesRequest{Text: "просто текст\n", Kind: "blocked"}),
		"Это не похоже на адрес сайта", "domain")
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSites, ipc.AddSitesRequest{Text: " \n\n", Kind: "blocked"}),
		"Это не похоже на адрес сайта", "domain")
	if r := h.fail(adminPeer(), tok, ipc.CmdAddSites, ipc.AddSitesRequest{Text: "x.ru", Kind: "video"}); r.Error != ipc.TextBadRequest {
		t.Errorf("неизвестный список: %q", r.Error)
	}
	many := ""
	for i := 0; i <= ipc.MaxSitesPerCall; i++ {
		many += fmt.Sprintf("m%d.example\n", i)
	}
	expectFieldFail(t, h.fail(adminPeer(), tok, ipc.CmdAddSites, ipc.AddSitesRequest{Text: many, Kind: "study"}),
		"Можно вставить не больше 1000 адресов за раз", "domain")
	if _, ok := h.store.sites["m0.example"]; ok {
		t.Error("слишком длинная вставка частично записана")
	}
}

// Ловит (R14): вставка в почти полные «Запрещённые» проходит за 1000 или не говорит, сколько
// не поместилось; повтор в пачке занимает место дважды.
func TestAddSitesBlockedLimit(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), func(h *harness) {
		for i := 0; i < 998; i++ {
			h.store.sites[fmt.Sprintf("s%d.example", i)] = "blocked"
		}
	})
	h.start()
	tok := h.login(adminPeer())
	var rep ipc.AddSitesReply
	h.ok(adminPeer(), tok, ipc.CmdAddSites, ipc.AddSitesRequest{Text: "a.example\na.example\ns1.example\nb.example\nc.example\nd.example", Kind: "blocked"}, &rep)
	if rep.Message != "Добавлено сайтов: 2. Уже были в списке: 1. Не поместились (2): в списке запрещённых уже 1000 сайтов" ||
		jsonOf(t, rep.Added) != jsonOf(t, []string{"a.example", "b.example"}) {
		t.Errorf("вставка у предела: %q, %v", rep.Message, rep.Added)
	}
	if n, _ := h.store.CountSiteRules("blocked"); n != 1000 {
		t.Errorf("в «Запрещённых» %d", n)
	}
}

// Ловит (8.3.5 п.2, B-49): отмеченные сайты не удаляются одним действием, ответ не говорит, из
// каких списков (для «Отменить»), журнал не по сайту, удалённый ещё закрывается; пустой запрос
// принимается; все уже удалены — нет отказа.
func TestRemoveSites(t *testing.T) {
	h := siteHarness(t)
	tok := h.login(adminPeer())
	var rm ipc.RemoveSitesReply
	h.ok(adminPeer(), tok, ipc.CmdRemoveSites, ipc.RemoveSitesRequest{Domains: []string{"example.com", "school.example", "gone.example"}}, &rm)
	if rm.Message != "Удалено сайтов: 2" || rm.UndoUntilMs != realT0.Add(10*time.Second).UnixMilli() ||
		jsonOf(t, rm.Removed) != jsonOf(t, []ipc.SiteDTO{{Domain: "example.com", Kind: "blocked"}, {Domain: "school.example", Kind: "study"}}) {
		t.Errorf("удаление: %+v", rm)
	}
	if siteKindOf(rm.Sites, "example.com") != "" || siteKindOf(rm.Sites, "school.example") != "" {
		t.Error("в ответе удалённые всё ещё в списках")
	}
	if p := h.poll(braveSample("example.com")); len(p.Commands) != 0 {
		t.Errorf("после удаления команды %+v", p.Commands)
	}
	want := []string{"Вы удалили сайт example.com из «Запрещённых»", "Вы удалили сайт school.example из «Учёбы»"}
	if got := h.journalTexts(evSiteRemoved); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("журнал удаления %q", got)
	}
	if r := h.fail(adminPeer(), tok, ipc.CmdRemoveSites, ipc.RemoveSitesRequest{Domains: []string{"example.com"}}); r.Error != "Этого сайта уже нет в списках" {
		t.Errorf("повтор удаления: %q", r.Error)
	}
	if r := h.fail(adminPeer(), tok, ipc.CmdRemoveSites, ipc.RemoveSitesRequest{Domains: []string{}}); r.Error != ipc.TextBadRequest {
		t.Errorf("пустой запрос: %q", r.Error)
	}
	h.ok(adminPeer(), tok, ipc.CmdRemoveSites, ipc.RemoveSitesRequest{Domains: []string{"youtube.com"}}, &rm)
	if rm.Message != "Удалён youtube.com" {
		t.Errorf("один сайт: %q", rm.Message)
	}
}
