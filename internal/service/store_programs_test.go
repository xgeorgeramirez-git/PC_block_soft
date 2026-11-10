package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Пути приложения магазина в трёх папках версий и его ключ (RM4-11).
const (
	storeWA    = `c:\program files\windowsapps`
	storeTool1 = storeWA + `\contoso.tool_1.0.0.0_x64__abcdef\tool.exe`
	storeTool2 = storeWA + `\contoso.tool_2.0.0.0_x64__abcdef\tool.exe`
	storeTool3 = storeWA + `\contoso.tool_3.0.0.0_x64__abcdef\tool.exe`
	storeTool  = storeWA + `\contoso.tool__abcdef\tool.exe`
)

// Ловит (RM4-11, D-66, S46, PL-40): «Ждут решения», плашка, решение родителя, учёт или скрытие
// пишут путь версии пакета, а не ключ: после обновления приложения — снова «ждёт решения», вторая
// строка «Программ», учёт по версиям; свойства файла спрашивают по ключу, а не по настоящему пути.
func TestStoreKeyEverywhere(t *testing.T) {
	meta := newMeta()
	w := newWatcher()
	h := watchHarness(t, w, withMeta(meta))
	w.start(childSession, storeTool1)
	h.watch()
	want := []ipc.BlockedPlate{{Kind: ipc.BlockedUnknown, Text: "Программа «tool» ждёт разрешения родителя", ShowS: 15, Path: storeTool, CanAsk: true}}
	if got := platesOf(h.poll(activeSample)); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("плашка версии 1:\n got %s\nwant %s", jsonOf(t, got), jsonOf(t, want))
	}
	h.real.Advance(10 * time.Second)
	w.start(childSession, storeTool2)
	h.watch()
	pend, _ := h.store.ListPending()
	if len(pend) != 1 || pend[0].Path != storeTool || pend[0].Attempts != 2 {
		t.Fatalf("«Ждут решения» после двух версий: %+v, want одна строка ключа с 2 попытками", pend)
	}
	if got := meta.askedPaths(); jsonOf(t, got) != jsonOf(t, []string{storeTool1, storeTool2}) {
		t.Errorf("свойства файла спрошены по %v, want настоящие пути версий", got)
	}
	if ev := h.store.eventsOf(evProgramPending); len(ev) != 1 || ev[0].Details["path"] != storeTool {
		t.Errorf("журнал «ждёт решения»: %+v, want одна запись с ключом", ev)
	}

	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: storeTool, Category: "games"}, nil)
	p3 := w.start(childSession, storeTool3)
	h.watch()
	if !w.alive(p3) {
		t.Fatal("версия 3 после решения «Игры» по ключу закрыта")
	}
	h.run(5, fgSample(storeTool3, ipc.CatNameOther))

	h.ok(adminPeer(), tok, ipc.CmdSetProgramsHidden, ipc.SetProgramsHiddenRequest{Paths: []string{storeTool2}, Hidden: true}, nil)
	if p, ok, _ := h.store.GetProgram(storeTool); !ok || !p.Hidden {
		t.Errorf("скрытие по пути версии 2: строка ключа %+v (есть %v), want скрыта", p, ok)
	}
	h.svc.Shutdown(h.real.Now())
	id := h.childID(childSID)
	h.store.mu.Lock()
	got := h.store.usage[fmt.Sprintf("%d|2026-10-19|games|program|%s", id, storeTool)]
	h.store.mu.Unlock()
	if got != 5 {
		t.Errorf("учёт по ключу: %d с, want 5 (usage %v)", got, h.store.usage)
	}
	list, _ := h.store.ListPrograms()
	for _, p := range list {
		if p.Path == storeTool1 || p.Path == storeTool2 || p.Path == storeTool3 {
			t.Errorf("в списке программ путь версии: %+v", p)
		}
	}
}

// Ловит (RM4-11, R13): приложение магазина по ключу не считается приложением Магазина (ручной
// запуск через брокер стал бы «запускалась сама» или наоборот).
func TestLaunchStoreAppKey(t *testing.T) {
	if !launchStoreApp(storeTool, storeWA) {
		t.Error("launchStoreApp(ключ) = false, want true")
	}
	if launchStoreApp(`c:\program files\geogebra\geogebra.exe`, storeWA) {
		t.Error("launchStoreApp(GeoGebra) = true, want false")
	}
	if !launchStoreApp(`c:\temp\pcb-programs\store\contoso.tool_2.0.0.0_x64__abcdef\tool.exe`, `c:\temp\pcb-programs\store`) {
		t.Error("launchStoreApp в тестовой папке пакетов = false, want true")
	}
}

// Ловит (RM4-11, T6-8): признак «приложение Магазина» у процесса берётся не от папки пакетов, а от
// Program Files: любая программа Program Files считалась бы приложением Магазина.
func TestWatchEntryStoreFact(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	w.start(childSession, pathGeoGebra)
	w.start(childSession, storeTool1)
	h.watch()
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	seen := map[string]bool{}
	for _, e := range h.svc.watch.procs {
		seen[e.norm] = e.facts.Store
	}
	if seen[pathGeoGebra] || !seen[storeTool1] {
		t.Errorf("признак Магазина: GeoGebra %v, пакет %v; want false, true", seen[pathGeoGebra], seen[storeTool1])
	}
}

// Ловит (PL-41, T6-12, S46): тестовая команда засева программ «до обновления» не доходит до
// хранилища, пишет ключ вместо пути версии (S46 не проверит слияние), не сбрасывает флаг слияния,
// работает без тестового режима, принимает чужую категорию или неизвестного ребёнка; после
// перезапуска службы засеянное не сливается.
func TestTestSeedPrograms(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()), withTestMode(TestMode{}))
	h.start()
	id := h.childID(childSID)
	req := ipc.TestSeedProgramsRequest{
		Programs: []ipc.TestSeedProgram{
			{Path: strings.ToUpper(edge99), Name: "msedge_proxy", Category: "blocked", Source: "parent", DecidedAt: 50},
			{Path: edge130, Name: "msedge_proxy", Category: "other", Source: "snapshot"},
			{Path: edge131, Name: "Microsoft Edge", Publisher: "Microsoft Corporation", Category: "other", Source: "parent", Hidden: true, DecidedAt: 100},
		},
		Usage:   []ipc.TestSeedProgramUsage{{ChildID: id, Day: "2026-10-12", Path: edge130, Category: "other", Seconds: 600}},
		Pending: []ipc.TestSeedPending{{Path: edge131, Name: "msedge_proxy", Publisher: "Microsoft Corporation", ChildID: id, Attempts: 2, LastAttemptAt: 1000}},
	}
	h.ok(adminPeer(), "", ipc.CmdTestSeedPrograms, req, nil)
	for _, p := range []string{edge99, edge130, edge131} {
		if _, ok := h.store.programs[p]; !ok {
			t.Errorf("нет строки по пути версии %s", p)
		}
	}
	if _, ok := h.store.programs[edgeKey]; ok {
		t.Error("засев записал ключ: «база 0.3.3» должна быть по путям версий")
	}
	if got := h.store.usage[fmt.Sprintf("%d|2026-10-12|other|program|%s", id, edge130)]; got != 600 {
		t.Errorf("учёт по версии 130: %d, want 600", got)
	}
	if p := h.store.pending[edge131]; p == nil || p.Attempts != 2 || len(p.Children) != 1 || p.Children[0].ChildID != id {
		t.Errorf("«Ждут решения» по версии 131: %+v", p)
	}
	if h.store.settings[settingStoreMerged] != "0" {
		t.Errorf("флаг слияния после засева %q, want 0", h.store.settings[settingStoreMerged])
	}
	if !strings.Contains(h.logText(), "Тестовые программы: 3, учёт 1, ждут решения 1") {
		t.Errorf("журнал работы: %s", h.logText())
	}
	h.svc.Shutdown(h.real.Now())

	// Перезапуск службы: повторы слиты.
	h2 := newHarness(t, withStore(h.store), configured(), withPrograms(typicalScan()), withTestMode(TestMode{}))
	h2.start()
	if p, ok := h2.store.programs[edgeKey]; !ok || !p.Hidden || p.Name != "Microsoft Edge" {
		t.Errorf("после перезапуска строка ключа: %+v (есть %v)", p, ok)
	}
	for _, p := range []string{edge99, edge130, edge131} {
		if _, ok := h2.store.programs[p]; ok {
			t.Errorf("после перезапуска осталась строка версии %s", p)
		}
	}
	if got := h2.store.usage[fmt.Sprintf("%d|2026-10-12|other|program|%s", id, edgeKey)]; got != 600 || len(h2.store.pending) != 0 {
		t.Errorf("после перезапуска учёт ключа %d, ждут решения %v; want 600 и пусто", got, h2.store.pending)
	}

	for name, bad := range map[string]ipc.TestSeedProgramsRequest{
		"категория":       {Programs: []ipc.TestSeedProgram{{Path: edge130, Name: "x", Category: "x", Source: "parent"}}},
		"источник":        {Programs: []ipc.TestSeedProgram{{Path: edge130, Name: "x", Category: "other", Source: "x"}}},
		"путь":            {Programs: []ipc.TestSeedProgram{{Path: "x.exe", Name: "x", Category: "other", Source: "parent"}}},
		"категория учёта": {Usage: []ipc.TestSeedProgramUsage{{ChildID: id, Day: "2026-10-12", Path: edge130, Category: "blocked", Seconds: 1}}},
		"день":            {Usage: []ipc.TestSeedProgramUsage{{ChildID: id, Day: "12.10.2026", Path: edge130, Category: "other", Seconds: 1}}},
		"будущий день":    {Usage: []ipc.TestSeedProgramUsage{{ChildID: id, Day: "2026-10-20", Path: edge130, Category: "other", Seconds: 1}}},
		"попытки":         {Pending: []ipc.TestSeedPending{{Path: edge130, Name: "x", ChildID: id, Attempts: 0, LastAttemptAt: 1}}},
	} {
		if rep := h2.do(adminPeer(), "", ipc.CmdTestSeedPrograms, bad); rep.OK || rep.Error != ipc.TextBadRequest {
			t.Errorf("%s: ответ %+v, want %q", name, rep, ipc.TextBadRequest)
		}
	}
	for name, bad := range map[string]ipc.TestSeedProgramsRequest{
		"учёт":  {Usage: []ipc.TestSeedProgramUsage{{ChildID: 999, Day: "2026-10-12", Path: edge130, Category: "other", Seconds: 1}}},
		"ждущ.": {Pending: []ipc.TestSeedPending{{Path: edge130, Name: "x", ChildID: 999, Attempts: 1, LastAttemptAt: 1}}},
	} {
		if rep := h2.do(adminPeer(), "", ipc.CmdTestSeedPrograms, bad); rep.OK || rep.Error != textNoSuchChild {
			t.Errorf("ребёнок 999 (%s): ответ %+v, want %q", name, rep, textNoSuchChild)
		}
	}

	plain := newHarness(t, configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()))
	plain.start()
	if rep := plain.do(adminPeer(), "", ipc.CmdTestSeedPrograms, req); rep.OK {
		t.Error("без тестового режима команда работает")
	}
}
