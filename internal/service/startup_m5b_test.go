package service

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/startup"
)

// Автозагрузка M5b (FR-39, RM5b-2…RM5b-5, Review Focus 1–4, уточнения плана 1–4, 8, 16; задача T4a).

func runE(name string, on bool) startup.Entry {
	return startup.Entry{Source: startup.SrcRun, Name: name, Command: `C:\Apps\` + name + `.exe`, Exe: `C:\Apps\` + name + `.exe`, Enabled: on}
}

// startupHarness — служба с Мишей и Колей, автозагрузкой src и вошедшим родителем.
func startupHarness(t *testing.T, src *fakeStartupSource, opts ...hopt) (*harness, string) {
	t.Helper()
	all := append([]hopt{configured(), withChild(childSID, "Миша", 600), withChild(child2SID, "Коля", 600), withStartup(src)}, opts...)
	h := newHarness(t, all...)
	h.start()
	h.svc.waitStartup()
	return h, h.login(adminPeer())
}

// listStartup — ListStartup родителем (refresh — запустить проверку и дождаться её).
func (h *harness) listStartup(tok string, id int64, refresh bool) ipc.ListStartupReply {
	h.t.Helper()
	var rep ipc.ListStartupReply
	h.ok(adminPeer(), tok, ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: id, Refresh: refresh}, &rep)
	if refresh {
		h.svc.waitStartup()
		h.ok(adminPeer(), tok, ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: id}, &rep)
	}
	return rep
}

// decisions — «имя=решение» блока по порядку.
func decisions(list []ipc.StartupItemView) []string {
	out := []string{}
	for _, it := range list {
		out = append(out, it.Name+"="+it.Decision)
	}
	return out
}

func (h *harness) startupItem(tok string, id int64, name string) ipc.StartupItemView {
	h.t.Helper()
	rep := h.listStartup(tok, id, false)
	for _, l := range [][]ipc.StartupItemView{rep.Pending, rep.Own, rep.Shared} {
		for _, it := range l {
			if it.Name == name {
				return it
			}
		}
	}
	h.t.Fatalf("нет строки %q: %+v", name, rep)
	return ipc.StartupItemView{}
}

func setCalls(calls []string) []string {
	out := []string{}
	for _, c := range calls {
		if strings.HasPrefix(c, "set ") {
			out = append(out, c)
		}
	}
	return out
}

// Ловит (RM5b-4): первая проверка меняет Windows, пишет журнал или делает записи «новыми»;
// выключенная своя запись получила решение «on»; блоки не по ТЗ; null вместо [].
func TestStartupFirstCheckKeepsState(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	src.addOwn(childSID, runE("Steam", false))
	src.addShared(runE("OneDrive", true))
	src.addShared(runE("Updater", false))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)

	rep := h.listStartup(tok, id, false)
	if got := decisions(rep.Own); !reflect.DeepEqual(got, []string{"Discord=on", "Steam=off"}) {
		t.Errorf("свои = %v", got)
	}
	if got := decisions(rep.Shared); !reflect.DeepEqual(got, []string{"OneDrive=on", "Updater=off"}) {
		t.Errorf("общие = %v", got)
	}
	if rep.Pending == nil || len(rep.Pending) != 0 {
		t.Errorf("ждут решения = %#v, want []", rep.Pending)
	}
	if rep.CheckedAt != h.real.Now().UnixMilli() || rep.Checking {
		t.Errorf("checkedAt %d checking %v; want %d false", rep.CheckedAt, rep.Checking, h.real.Now().UnixMilli())
	}
	if c := setCalls(src.callsFrom(0)); len(c) != 0 {
		t.Errorf("первая проверка меняла Windows: %v", c)
	}
	if ev := h.store.eventsOf(evStartupNew); len(ev) != 0 {
		t.Errorf("первая проверка написала в журнал: %+v", ev)
	}
	if it := rep.Own[0]; it.Source != startup.SrcRun || it.From != "Реестр, при входе" || it.Scope != "own" || it.ID == 0 {
		t.Errorf("строка = %+v", it)
	}
	// Пустой ребёнок: все блоки [], проверки не было.
	empty := h.listStartup(tok, h.childID(child2SID), false)
	if empty.Own == nil || empty.Pending == nil || len(empty.Own) != 0 {
		t.Errorf("Коля = %#v", empty)
	}
}

// Ловит (RM5b-4, RM5b-2, уточнения 1, 16): новая своя запись не стала pending, не выключена в
// Windows, нет строки журнала; RunOnce или общая запись выключаются через Windows; новая общая
// не pending.
func TestStartupNewEntriesPending(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)

	src.addOwn(childSID, runE("Roblox", true))
	src.addOwn(childSID, startup.Entry{Source: startup.SrcRunOnce, Name: "Setup", Command: "setup.exe", Enabled: true})
	src.addShared(runE("Teams", true))
	n := src.callN()
	rep := h.listStartup(tok, id, true)

	if got := decisions(rep.Pending); !reflect.DeepEqual(got, []string{"Roblox=pending", "Setup=pending", "Teams=pending"}) {
		t.Errorf("ждут решения = %v", got)
	}
	if got := decisions(rep.Own); !reflect.DeepEqual(got, []string{"Discord=on"}) {
		t.Errorf("свои = %v", got)
	}
	if rep.Shared == nil || len(rep.Shared) != 0 {
		t.Errorf("общие = %#v, want [] (Teams — в «Ждут решения»)", rep.Shared)
	}
	if c := setCalls(src.callsFrom(n)); !reflect.DeepEqual(c, []string{"set " + childSID + " Roblox false"}) {
		t.Errorf("Windows = %v; want только выключение Roblox", c)
	}
	if src.ownEnabled(childSID, "Roblox") {
		t.Error("Roblox остался включён в Windows")
	}
	want := []string{
		"Миша: новая запись автозагрузки «Roblox» выключена и ждёт решения",
		"Миша: новая запись автозагрузки «Setup» выключена и ждёт решения",
		"Миша: новая запись автозагрузки «Teams» выключена и ждёт решения",
	}
	if got := h.journalTexts(evStartupNew); !reflect.DeepEqual(got, want) {
		t.Errorf("журнал = %q\nwant %q", got, want)
	}
	// Повторная проверка ничего не добавляет и не выключает.
	n = src.callN()
	h.listStartup(tok, id, true)
	if c := setCalls(src.callsFrom(n)); len(c) != 0 {
		t.Errorf("повторная проверка: %v", c)
	}
	if got := len(h.store.eventsOf(evStartupNew)); got != 3 {
		t.Errorf("строк журнала %d, want 3", got)
	}
}

// Ловит (уточнение 3): ребёнок включил запись обратно в Параметрах — служба оставила её
// включённой; решение «on» и выключенная ребёнком запись включаются обратно.
func TestStartupChildReenabledIsOffAgain(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	src.addOwn(childSID, runE("Steam", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	src.addOwn(childSID, runE("Roblox", true))
	h.listStartup(tok, id, true) // Roblox → pending, выключен
	steam := h.startupItem(tok, id, "Steam")
	h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: steam.ID, On: false}, nil)
	h.svc.waitStartup()

	src.setOwnEnabled(childSID, "Roblox", true)
	src.setOwnEnabled(childSID, "Steam", true)
	src.setOwnEnabled(childSID, "Discord", false)
	n := src.callN()
	h.listStartup(tok, id, true)
	got := setCalls(src.callsFrom(n))
	want := []string{"set " + childSID + " Steam false", "set " + childSID + " Roblox false"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Windows = %v, want %v", got, want)
	}
	if src.ownEnabled(childSID, "Roblox") || src.ownEnabled(childSID, "Steam") || src.ownEnabled(childSID, "Discord") {
		t.Error("Roblox и Steam должны быть выключены, Discord — как выключил ребёнок")
	}
}

// Ловит (PL-46, Review Focus 2): сбой чтения Windows удаляет строки или делает их «новыми» при
// следующей удачной проверке; сбой хранилища теряет записи.
func TestStartupListErrorKeepsRows(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	src.addShared(runE("OneDrive", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	before := h.listStartup(tok, id, false)

	src.setFail(errors.New("куст не читается"), nil)
	rep := h.listStartup(tok, id, true)
	if !reflect.DeepEqual(rep.Own, before.Own) || !reflect.DeepEqual(rep.Shared, before.Shared) || rep.CheckedAt != before.CheckedAt {
		t.Errorf("после сбоя = %+v\nwant %+v", rep, before)
	}
	h.store.failStartup = true
	src.setFail(nil, nil)
	h.real.Advance(time.Minute)
	h.do(adminPeer(), tok, ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: id, Refresh: true}) // список не читается, проверка идёт
	h.svc.waitStartup()
	h.store.failStartup = false
	rep = h.listStartup(tok, id, false)
	if !reflect.DeepEqual(rep.Own, before.Own) || rep.CheckedAt != before.CheckedAt {
		t.Errorf("после сбоя хранилища = %+v", rep)
	}
	src.removeOwn(childSID, "Discord")
	rep = h.listStartup(tok, id, true)
	if len(rep.Own) != 0 || len(rep.Pending) != 0 || !reflect.DeepEqual(decisions(rep.Shared), []string{"OneDrive=on"}) {
		t.Errorf("после удачной проверки = %+v", rep)
	}
	if ev := h.store.eventsOf(evStartupNew); len(ev) != 0 {
		t.Errorf("сбой сделал записи новыми: %+v", ev)
	}
	if !strings.Contains(h.logText(), "куст не читается") {
		t.Error("сбой чтения не в журнале работы")
	}
}

// Ловит (решение оркестратора, уточнение 2, Review Focus 1): при старте службы подгружается
// профиль невошедшего ребёнка; общая запись, появившаяся после установки до первого входа
// ребёнка, при входе получает «on»; свои записи при первом входе — «новые».
func TestStartupSharedBoundaryBeforeFirstLogon(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	src.addShared(runE("OneDrive", true))
	h, tok := startupHarness(t, src)
	for _, c := range src.callsFrom(0) {
		if c != "shared" {
			t.Errorf("при старте без входа вызвано %q: профиль ребёнка не трогается", c)
		}
	}
	if n := src.callN(); n != 2 {
		t.Errorf("при старте %d проверок, want 2 (общие для Миши и Коли)", n)
	}
	id := h.childID(childSID)
	if got := decisions(h.listStartup(tok, id, false).Shared); !reflect.DeepEqual(got, []string{"OneDrive=on"}) {
		t.Errorf("общие до входа = %v", got)
	}

	src.addShared(runE("Teams", true)) // поставили программу до первого входа ребёнка
	h.childLogsIn()
	h.svc.waitStartup()
	rep := h.listStartup(tok, id, false)
	if got := decisions(rep.Pending); !reflect.DeepEqual(got, []string{"Teams=pending"}) {
		t.Errorf("ждут решения = %v, want [Teams=pending]", got)
	}
	if got := decisions(rep.Own); !reflect.DeepEqual(got, []string{"Discord=on"}) {
		t.Errorf("свои при первом входе = %v, want [Discord=on]", got)
	}
	if got := decisions(rep.Shared); !reflect.DeepEqual(got, []string{"OneDrive=on"}) {
		t.Errorf("общие = %v", got)
	}
	// Коля не входил: раз в 10 минут — только общие, Teams у него тоже ждёт решения.
	n := src.callN()
	h.hk(int(startupCheckEvery / time.Second))
	h.svc.waitStartup()
	calls := src.callsFrom(n)
	sort.Strings(calls) // дети проверяются параллельно
	if !reflect.DeepEqual(calls, []string{"list " + childSID, "shared"}) {
		t.Errorf("проверки через 10 мин = %v", calls)
	}
	tok = h.login(adminPeer()) // сессия родителя закончилась за 10 мин без действий
	if got := decisions(h.listStartup(tok, h.childID(child2SID), false).Pending); !reflect.DeepEqual(got, []string{"Teams=pending"}) {
		t.Errorf("Коля: ждут решения = %v", got)
	}
}

// Ловит (RM5b-5): проверки раньше 10 минут или не идут совсем.
func TestStartupHousekeepEveryTenMinutes(t *testing.T) {
	src := newStartupSource()
	h, _ := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	n := src.callN()
	h.hk(int(startupCheckEvery/time.Second) - 2)
	h.svc.waitStartup()
	if c := src.callsFrom(n); len(c) != 0 {
		t.Fatalf("до 10 мин: %v", c)
	}
	h.hk(2)
	h.svc.waitStartup()
	if c := src.callsFrom(n); len(c) != 2 {
		t.Errorf("через 10 мин: %v, want 2 проверки", c)
	}
}

// Ловит (Review Focus 1, RM5b-5): две проверки одного ребёнка идут одновременно; просьба о
// проверке во время идущей теряется; ответ не говорит «проверка идёт».
func TestStartupOneCheckPerChild(t *testing.T) {
	src := newStartupSource()
	h, tok := startupHarness(t, src)
	id := h.childID(childSID)
	block := make(chan struct{})
	src.mu.Lock()
	src.block, src.maxIn = block, 0
	src.mu.Unlock()
	n := src.callN()
	h.childLogsIn() // проверка встала на List
	within(t, 2*time.Second, "List вызван", func() {
		for src.callN() == n {
			time.Sleep(time.Millisecond)
		}
	})
	var rep ipc.ListStartupReply
	h.ok(adminPeer(), tok, ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: id, Refresh: true}, &rep)
	h.ok(adminPeer(), tok, ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: id, Refresh: true}, &rep)
	if !rep.Checking {
		t.Error("checking = false во время проверки")
	}
	close(block)
	h.svc.waitStartup()
	src.mu.Lock()
	maxIn := src.maxIn
	src.mu.Unlock()
	if maxIn != 1 {
		t.Errorf("одновременно %d проверок, want 1", maxIn)
	}
	if c := src.callsFrom(n); !reflect.DeepEqual(c, []string{"list " + childSID, "list " + childSID}) {
		t.Errorf("проверки = %v, want две по очереди", c)
	}
	h.ok(adminPeer(), tok, ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: id}, &rep)
	if rep.Checking {
		t.Error("checking = true после проверки")
	}
}

// Ловит (Review Focus 3, уточнения 4, 8): решение сохраняется до записи в Windows или при её
// сбое; сообщение не по ТЗ; запись из «Ждут решения» не переходит в блок; журнал без строки.
func TestSetStartupOwn(t *testing.T) {
	src := newStartupSource()
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	src.addOwn(childSID, runE("Roblox", true))
	rob := h.listStartup(tok, id, true).Pending[0]

	var rep ipc.SetStartupReply
	h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: rob.ID, On: true}, &rep)
	if rep.Message != "Будет запускаться при входе Миши" || rep.Item.Decision != ipc.StartupOn || rep.Item.ID != rob.ID || rep.Item.Name != "Roblox" {
		t.Errorf("ответ = %+v", rep)
	}
	if !src.ownEnabled(childSID, "Roblox") {
		t.Error("Roblox не включён в Windows")
	}
	h.svc.waitStartup()
	list := h.listStartup(tok, id, false)
	if len(list.Pending) != 0 || !reflect.DeepEqual(decisions(list.Own), []string{"Roblox=on"}) {
		t.Errorf("после включения = %+v", list)
	}

	src.setFail(nil, errors.New("отказано в доступе"))
	f := h.fail(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: rob.ID, On: false})
	if f.Error != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Errorf("сбой Windows: %q", f.Error)
	}
	if it := h.startupItem(tok, id, "Roblox"); it.Decision != ipc.StartupOn {
		t.Errorf("при сбое Windows решение = %s, want on", it.Decision)
	}
	src.setFail(nil, nil)
	h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: rob.ID, On: false}, &rep)
	if rep.Message != "Не будет запускаться при входе Миши" || rep.Item.Decision != ipc.StartupOff || src.ownEnabled(childSID, "Roblox") {
		t.Errorf("выключение = %+v", rep)
	}
	want := []string{"Миша: автозагрузка «Roblox» включена", "Миша: автозагрузка «Roblox» выключена"}
	if got := h.journalTexts(evStartupDecided); !reflect.DeepEqual(got, want) {
		t.Errorf("журнал = %q, want %q", got, want)
	}

	// Запись пропала из Windows до нажатия: свежий List её не нашёл.
	h.svc.waitStartup()
	src.removeOwn(childSID, "Roblox")
	f = h.fail(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: rob.ID, On: true})
	if f.Error != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Errorf("пропавшая запись: %q", f.Error)
	}
	f = h.fail(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: 999, On: true})
	if f.Error != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Errorf("нет строки: %q", f.Error)
	}
}

// Ловит (уточнения 1, 16): решение по общей записи или RunOnce пишется в Windows или не
// сохраняется; решение Миши меняет запись Коли.
func TestSetStartupSharedAndRunOnceOnlyBase(t *testing.T) {
	src := newStartupSource()
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	src.addShared(runE("Teams", true))
	src.addOwn(childSID, startup.Entry{Source: startup.SrcRunOnce, Name: "Setup", Command: "setup.exe", Enabled: true})
	rep := h.listStartup(tok, id, true)
	h.listStartup(tok, h.childID(child2SID), true)
	n := src.callN()
	for _, it := range rep.Pending {
		var r ipc.SetStartupReply
		h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: it.ID, On: false}, &r)
		if r.Item.Decision != ipc.StartupOff {
			t.Errorf("%s: решение %s", it.Name, r.Item.Decision)
		}
	}
	h.svc.waitStartup()
	for _, c := range src.callsFrom(n) {
		if strings.Contains(c, "Teams") && strings.HasPrefix(c, "set ") {
			t.Errorf("общая запись записана в Windows: %q", c)
		}
	}
	if got := decisions(h.listStartup(tok, id, false).Shared); !reflect.DeepEqual(got, []string{"Teams=off"}) {
		t.Errorf("общие Миши = %v", got)
	}
	if got := decisions(h.listStartup(tok, h.childID(child2SID), false).Pending); !reflect.DeepEqual(got, []string{"Teams=pending"}) {
		t.Errorf("Коля = %v, want [Teams=pending]", got)
	}
}

// Ловит: ребёнок, другой пользователь, страж или администратор без входа родителя читают или
// меняют автозагрузку; чужой id ребёнка проходит.
func TestStartupCommandsNeedParent(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	it := h.listStartup(tok, id, false).Own[0]
	for _, pk := range peerKinds {
		for _, c := range []struct {
			cmd string
			req any
		}{
			{ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: id}},
			{ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: it.ID, On: false}},
		} {
			if r := h.do(pk.peer, "", c.cmd, c.req); r.OK {
				t.Errorf("%s без токена: %s прошла", pk.name, c.cmd)
			}
		}
	}
	if r := h.do(childPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: it.ID, On: false}); r.OK {
		t.Error("ребёнок с чужим токеном выключил запись")
	}
	if !src.ownEnabled(childSID, "Discord") {
		t.Error("запись выключена отказанным вызовом")
	}
	if f := h.fail(adminPeer(), tok, ipc.CmdListStartup, ipc.ListStartupRequest{ChildID: 77}); f.Error != textNoSuchChild {
		t.Errorf("чужой ребёнок: %q", f.Error)
	}
	// Строка Миши от имени Коли.
	if f := h.fail(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: h.childID(child2SID), ID: it.ID, On: false}); f.Error != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Errorf("строка другого ребёнка: %q", f.Error)
	}
}

// Ловит (S50): TestSession logon не сбрасывает момент входа или не проверяет автозагрузку.
func TestTestSessionLogon(t *testing.T) {
	src := newStartupSource()
	h, tok := startupHarness(t, src, withTestMode(TestMode{}))
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	h.hk(90)
	src.addOwn(childSID, runE("Roblox", true))
	h.ok(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionLogon, SessionID: childSession}, nil)
	h.svc.waitStartup()
	h.svc.mu.Lock()
	since := h.svc.sessions[childSession].since
	h.svc.mu.Unlock()
	if !since.Equal(h.real.Now()) {
		t.Errorf("since = %v, want %v", since, h.real.Now())
	}
	if got := decisions(h.listStartup(tok, id, false).Pending); !reflect.DeepEqual(got, []string{"Roblox=pending"}) {
		t.Errorf("после logon ждут решения = %v", got)
	}
	if r := h.do(adminPeer(), "", ipc.CmdTestSession, ipc.TestSessionRequest{Event: ipc.TestSessionLogon, SessionID: 42}); r.OK {
		t.Error("logon неизвестного сеанса прошёл")
	}
}

// Ловит: строка приложения магазина показывает имя ключа; нет службы автозагрузки — ошибка.
func TestStartupStoreNameAndNoSource(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, startup.Entry{Source: startup.SrcStore, Name: "SpotifyAB.SpotifyMusic_zpdnekdrzrea0!Spotify",
		Command: "SpotifyAB.SpotifyMusic_zpdnekdrzrea0!Spotify", Package: "SpotifyAB.SpotifyMusic_zpdnekdrzrea0", Task: "Spotify", Enabled: true})
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	it := h.listStartup(tok, h.childID(childSID), false).Own[0]
	if it.Name != "SpotifyMusic" || it.Source != startup.SrcStore || it.Publisher != "" || it.From != "Приложение из магазина" {
		t.Errorf("строка магазина = %+v", it)
	}

	h2 := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h2.start()
	tok2 := h2.login(adminPeer())
	rep := h2.listStartup(tok2, h2.childID(childSID), true)
	if rep.Own == nil || rep.Shared == nil || rep.Pending == nil || rep.Checking {
		t.Errorf("без источника = %#v", rep)
	}
}

// Ловит: тексты журнала автозагрузки (группа «Программы») не по плану.
func TestStartupJournalTexts(t *testing.T) {
	for _, c := range []struct {
		kind string
		d    map[string]any
		want string
	}{
		{evStartupNew, map[string]any{"name": "Roblox"}, "Миша: новая запись автозагрузки «Roblox» выключена и ждёт решения"},
		{evStartupNew, map[string]any{"name": "Roblox", "changed": true}, "Миша: запись автозагрузки «Roblox» изменилась, выключена и ждёт решения"},
		{evStartupDecided, map[string]any{"name": "Roblox", "on": true}, "Миша: автозагрузка «Roblox» включена"},
		{evStartupDecided, map[string]any{"name": "Roblox", "on": false}, "Миша: автозагрузка «Roblox» выключена"},
		{evStartupClosed, map[string]any{"name": "Teams"}, "Миша: запись автозагрузки «Teams» запускалась сама и закрыта"},
	} {
		if got := eventText(c.kind, "Миша", c.d); got != c.want {
			t.Errorf("%s = %q, want %q", c.kind, got, c.want)
		}
	}
	for _, k := range []string{evStartupNew, evStartupDecided, evStartupClosed} {
		found := false
		for _, x := range journalGroups[ipc.JournalGroupPrograms] {
			found = found || x == k
		}
		if !found {
			t.Errorf("%s не в группе «Программы»", k)
		}
	}
}

// Ловит: проверка только общих записей (ребёнок вышел) удаляет его свои строки или меняет решения.
func TestStartupSharedOnlyKeepsOwnRows(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	src.addShared(runE("OneDrive", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	before := h.listStartup(tok, id, false)
	h.launcher.setSessions()
	h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
	n := src.callN()
	rep := h.listStartup(tok, id, true)
	if c := src.callsFrom(n); !reflect.DeepEqual(c, []string{"shared"}) {
		t.Errorf("проверка вышедшего = %v, want [shared]", c)
	}
	if !reflect.DeepEqual(rep.Own, before.Own) || !reflect.DeepEqual(rep.Shared, before.Shared) {
		t.Errorf("после проверки общих = %+v\nwant %+v", rep, before)
	}
}

// Ловит (уточнение 4): SetStartup ищет запись в свежем списке только по имени и переключает
// одноимённую запись другого вида.
func TestSetStartupFindsByScopeSourceName(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	src.addOwn(childSID, startup.Entry{Source: startup.SrcFolder, Name: "Discord", Command: "discord.lnk", Enabled: true})
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	var folder ipc.StartupItemView
	for _, it := range h.listStartup(tok, id, false).Own {
		if it.Source == startup.SrcFolder {
			folder = it
		}
	}
	n := src.callN()
	h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: folder.ID, On: false}, nil)
	h.svc.waitStartup()
	if !src.ownEnabled(childSID, "Discord") { // первая по имени — запись реестра
		t.Error("выключена запись реестра вместо ярлыка папки")
	}
	src.mu.Lock()
	folderOn := src.own[childSID][1].Enabled
	src.mu.Unlock()
	if folderOn {
		t.Errorf("ярлык папки не выключен: %v", src.callsFrom(n))
	}
}

// Ловит (возврат 1 T4a, п.1): Windows переключён, а решение в базу не записалось — Windows
// остаётся в новом состоянии, база в старом, и проверка этого не выравнивает.
func TestSetStartupDecisionFailRollsBackWindows(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	it := h.listStartup(tok, id, false).Own[0]

	h.store.failStartupSet = true
	n := src.callN()
	f := h.fail(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: it.ID, On: false})
	if f.Error != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Errorf("ответ %q", f.Error)
	}
	want := []string{"set " + childSID + " Discord false", "set " + childSID + " Discord true"}
	if got := setCalls(src.callsFrom(n)); !reflect.DeepEqual(got, want) {
		t.Errorf("Windows = %v, want %v (выключить и вернуть)", got, want)
	}
	if !src.ownEnabled(childSID, "Discord") {
		t.Error("Windows не возвращён: Discord выключен, а в базе «on»")
	}
	h.store.failStartupSet = false
	if d := h.startupItem(tok, id, "Discord").Decision; d != ipc.StartupOn {
		t.Errorf("решение %s, want on", d)
	}
}

// Ловит (возврат 1 T4a, п.2, уточнение 4): пока SetStartup писал в Windows, проверка удалила и
// вставила строку заново (новый id) — решение сохраняется по старому id или теряется; строку
// удалили совсем — Windows остаётся переключённым.
func TestSetStartupRereadsRowAfterWindows(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	old := h.listStartup(tok, id, false).Own[0]
	rec := StartupRecord{Scope: "own", Source: startup.SrcRun, Name: "Discord", Command: `C:\Apps\Discord.exe`}

	// Строку пересоздали (как смена команды): новый id, решение pending.
	src.mu.Lock()
	src.onList = func() {
		src.onList = nil
		if _, err := h.store.SyncStartupItems(id, []StartupRecord{}, "pending", 1); err != nil {
			t.Error(err)
		}
		if _, err := h.store.SyncStartupItems(id, []StartupRecord{rec}, "pending", 2); err != nil {
			t.Error(err)
		}
	}
	src.mu.Unlock()
	var rep ipc.SetStartupReply
	h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: old.ID, On: false}, &rep)
	h.svc.waitStartup()
	now := h.listStartup(tok, id, false)
	if len(now.Own) != 1 || now.Own[0].ID == old.ID || now.Own[0].Decision != ipc.StartupOff || rep.Item.ID != now.Own[0].ID {
		t.Errorf("после пересоздания: ответ %+v, список %+v; want решение off у новой строки", rep.Item, now)
	}

	// Строку удалили совсем: «Не удалось сохранить», Windows возвращён.
	src.mu.Lock()
	src.onList = func() {
		src.onList = nil
		if _, err := h.store.SyncStartupItems(id, []StartupRecord{}, "pending", 3); err != nil {
			t.Error(err)
		}
	}
	src.mu.Unlock()
	cur := now.Own[0]
	f := h.fail(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: cur.ID, On: true})
	if f.Error != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Errorf("строка удалена: %q", f.Error)
	}
	if src.ownEnabled(childSID, "Discord") {
		t.Error("Windows не возвращён после удаления строки")
	}
}

// Ловит (возврат 1 T4a, п.3): «Проверено в …» обновляется проверкой только общих записей
// (ребёнок не входил — его свои записи не проверялись).
func TestStartupCheckedAtOnlyFullCheck(t *testing.T) {
	src := newStartupSource()
	src.addShared(runE("OneDrive", true))
	h, tok := startupHarness(t, src)
	id := h.childID(childSID)
	if rep := h.listStartup(tok, id, true); rep.CheckedAt != 0 {
		t.Errorf("до входа checkedAt = %d, want 0", rep.CheckedAt)
	}
	h.real.Advance(time.Minute)
	h.childLogsIn()
	h.svc.waitStartup()
	at := h.real.Now().UnixMilli()
	if rep := h.listStartup(tok, id, false); rep.CheckedAt != at {
		t.Errorf("после входа checkedAt = %d, want %d", rep.CheckedAt, at)
	}
	h.launcher.setSessions()
	h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
	h.real.Advance(time.Minute)
	if rep := h.listStartup(tok, id, true); rep.CheckedAt != at {
		t.Errorf("после проверки общих checkedAt = %d, want %d", rep.CheckedAt, at)
	}
}

// Ловит (возврат 1 T4a, п.5, PL-46): сбой проверки автозагрузки не виден в журнале работы, пишется
// без текста ошибки или чаще раза в час на ребёнка.
func TestStartupListErrorLoggedHourly(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	h, tok := startupHarness(t, src)
	h.childLogsIn()
	h.svc.waitStartup()
	id := h.childID(childSID)
	src.setFail(errors.New("файл задания повреждён"), nil)
	count := func() int {
		return strings.Count(h.logText(), "автозагрузка Миша (все записи) не прочитана, строки не меняются: файл задания повреждён")
	}
	tick := func(d time.Duration) {
		h.real.Advance(d)
		h.svc.Housekeep(h.real.Now())
		tok = h.login(adminPeer())
	}
	h.listStartup(tok, id, true)
	if count() != 1 {
		t.Fatalf("строк о сбое %d, want 1\n%s", count(), h.logText())
	}
	tick(59 * time.Minute)
	h.listStartup(tok, id, true)
	if count() != 1 {
		t.Errorf("через 59 мин строк %d, want 1\n%s", count(), h.logText())
	}
	tick(time.Minute)
	h.listStartup(tok, id, true)
	if count() != 2 {
		t.Errorf("через час строк %d, want 2", count())
	}
	if d := h.startupItem(tok, id, "Discord").Decision; d != ipc.StartupOn {
		t.Errorf("сбой поменял решение: %s", d)
	}
}
