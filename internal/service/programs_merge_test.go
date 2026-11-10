package service

import (
	"fmt"
	"strings"
	"testing"

	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Три версии msedge_proxy и ключ (RM4-11, S46).
const (
	edge130 = storeWA + `\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
	edge131 = storeWA + `\microsoft.microsoftedge.stable_131.0.2903.51_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
	edge99  = storeWA + `\microsoft.microsoftedge.stable_99.0.1.1_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
	edgeKey = storeWA + `\microsoft.microsoftedge.stable__8wekyb3d8bbwe\msedge_proxy.exe`
	edgeApp = storeWA + `\microsoft.microsoftedge.stable__8wekyb3d8bbwe`
)

// edgeRows — «база 0.3.3»: три строки одной программы магазина.
func edgeRows() []Program {
	return []Program{
		{Path: edge99, Name: "msedge_proxy", Category: "blocked", Source: ProgramSourceParent, DecidedAt: 50},
		{Path: edge130, Name: "msedge_proxy", Category: "other", Source: ProgramSourceSnapshot, Fingerprint: "1:1"},
		{Path: edge131, Name: "Microsoft Edge", Publisher: "Microsoft Corporation", Category: "other", Source: ProgramSourceParent, DecidedAt: 100, Hidden: true},
	}
}

// Ловит (RM4-11, PL-21, PL-30): слияние берёт строку целиком, а не поле по правилу; «последнее
// решение» — по пути или версии, а не по моменту решения; скрытие теряется; своё название
// пустое затирает непустое; «новейшая версия» сравнивается строкой («99» новее «131»);
// повторное слияние снова что-то меняет; в план попадают программы не из магазина.
func TestPlanStoreMerge(t *testing.T) {
	geo := Program{Path: pathGeoGebra, Name: "GeoGebra", Category: "study", Source: ProgramSourceParent, DecidedAt: 7}
	got := planStoreMerge(svcRoots, append(edgeRows(), geo), nil)
	want := []ProgramMerge{{
		Key: edgeKey,
		Into: &Program{Path: edgeKey, Name: "Microsoft Edge", Publisher: "Microsoft Corporation", Category: "other",
			Source: ProgramSourceParent, AppRoot: edgeApp, DecidedAt: 100, Hidden: true},
		From:        []string{edge130, edge131, edge99},
		PendingFrom: []string{},
	}}
	if jsonOf(t, got) != jsonOf(t, want) {
		t.Fatalf("план:\n got %s\nwant %s", jsonOf(t, got), jsonOf(t, want))
	}

	// Последнее решение — запрет (DecidedAt 200): категория «Запрещено», скрытие остаётся.
	rows := edgeRows()
	rows[0].DecidedAt = 200
	if m := planStoreMerge(svcRoots, rows, nil); len(m) != 1 || m[0].Into.Category != "blocked" || m[0].Into.Source != ProgramSourceParent ||
		!m[0].Into.Hidden || m[0].Into.DecidedAt != 200 {
		t.Errorf("последний запрет: %s", jsonOf(t, m))
	}
	// Без решений родителя — категория и источник новейшей версии (131 новее 99 по числам).
	rows = edgeRows()
	for i := range rows {
		rows[i].Source, rows[i].DecidedAt, rows[i].Hidden = ProgramSourceSnapshot, 0, false
	}
	rows[0].Category = "games"
	rows[2].Category = "study"
	if m := planStoreMerge(svcRoots, rows, nil); len(m) != 1 || m[0].Into.Category != "study" || m[0].Into.Source != ProgramSourceSnapshot || m[0].Into.Hidden {
		t.Errorf("без решений: %s", jsonOf(t, m))
	}
	// Скрыта старая версия — строка ключа тоже скрыта (скрытие переживает обновление).
	rows = edgeRows()
	rows[2].Hidden, rows[1].Hidden = false, true
	if m := planStoreMerge(svcRoots, rows, nil); len(m) != 1 || !m[0].Into.Hidden {
		t.Errorf("скрыта старая версия: %s", jsonOf(t, m))
	}
	// Своё название — непустое у самого позднего решения.
	rows = edgeRows()
	rows[1].CustomName, rows[1].DecidedAt = "Эдж", 10
	rows[2].CustomName = ""
	if m := planStoreMerge(svcRoots, rows, nil); len(m) != 1 || m[0].Into.CustomName != "Эдж" {
		t.Errorf("своё название: %s", jsonOf(t, m))
	}

	// «Ждут решения» при программе в группе — удаляются.
	pend := []PendingProgram{
		{Path: edge131, Name: "msedge_proxy", Attempts: 2, FirstChildID: 1, FirstAttemptAt: 1000, LastAttemptAt: 3000,
			Children: []PendingChild{{ChildID: 1, Attempts: 2, LastAttemptAt: 3000}}},
		{Path: edge130, Name: "msedge_proxy", Publisher: "Microsoft Corporation", Attempts: 1, FirstChildID: 2, FirstAttemptAt: 500, LastAttemptAt: 4000,
			Children: []PendingChild{{ChildID: 2, Attempts: 1, LastAttemptAt: 4000}}},
	}
	m := planStoreMerge(svcRoots, edgeRows(), pend)
	if len(m) != 1 || m[0].Pending != nil || jsonOf(t, m[0].PendingFrom) != jsonOf(t, []string{edge130, edge131}) {
		t.Errorf("ждут решения при программе: %s", jsonOf(t, m))
	}
	// Без программы — одна строка ключа: попытки складываются по детям.
	pend[0].Children = append(pend[0].Children, PendingChild{ChildID: 2, Attempts: 3, LastAttemptAt: 2000})
	pend[0].Attempts = 5
	m = planStoreMerge(svcRoots, nil, pend)
	wantPend := []ProgramMerge{{
		Key: edgeKey,
		Pending: &PendingProgram{Path: edgeKey, Name: "msedge_proxy", Publisher: "Microsoft Corporation", Attempts: 6,
			FirstChildID: 2, FirstAttemptAt: 500, LastAttemptAt: 4000,
			Children: []PendingChild{{ChildID: 2, Attempts: 4, LastAttemptAt: 4000}, {ChildID: 1, Attempts: 2, LastAttemptAt: 3000}}},
		From:        []string{},
		PendingFrom: []string{edge130, edge131},
	}}
	if jsonOf(t, m) != jsonOf(t, wantPend) {
		t.Errorf("ждут решения без программы:\n got %s\nwant %s", jsonOf(t, m), jsonOf(t, wantPend))
	}
	// Одна строка с путём версии — тоже на ключ.
	m = planStoreMerge(svcRoots, edgeRows()[1:2], nil)
	if len(m) != 1 || m[0].Into.Path != edgeKey || jsonOf(t, m[0].From) != jsonOf(t, []string{edge130}) {
		t.Errorf("одна строка версии: %s", jsonOf(t, m))
	}

	// Повтор на слитом — пусто; программы не из магазина в план не идут.
	merged := []Program{*want[0].Into, geo}
	if m := planStoreMerge(svcRoots, merged, []PendingProgram{*wantPend[0].Pending}); len(m) != 0 {
		t.Errorf("повтор на слитой базе: %s, want []", jsonOf(t, m))
	}
	if m := planStoreMerge(svcRoots, []Program{geo}, nil); m == nil || len(m) != 0 {
		t.Errorf("без магазина: %#v, want [] (не nil)", m)
	}
}

// storeBase033 — хранилище «0.3.3»: снимок сделан, три строки msedge_proxy, учёт по версиям и
// попытка в «Ждут решения» по пути версии; флага слияния нет.
func storeBase033(t *testing.T) *fakeStore {
	t.Helper()
	st := newFakeStore()
	id, err := st.UpsertChild(childSID, "Миша")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range edgeRows() {
		if err := st.UpsertProgram(p); err != nil {
			t.Fatal(err)
		}
	}
	st.AddUsage(id, "2026-10-14", "other", usageKindProgram, edge130, 600)
	st.AddUsage(id, "2026-10-14", "other", usageKindProgram, edge131, 300)
	st.AddUsage(id, "2026-10-19", "other", usageKindProgram, edge99, 60)
	st.AddUsage(id, "2026-10-19", "games", usageKindProgram, pathMinecraft, 70)
	st.RestorePending(PendingProgram{Path: edge131, Name: "msedge_proxy", Attempts: 2, FirstChildID: id, FirstAttemptAt: 1, LastAttemptAt: 2,
		Children: []PendingChild{{ChildID: id, Attempts: 2, LastAttemptAt: 2}}})
	st.settings[settingProgramsSeeded] = "1"
	return st
}

// Ловит (RM4-11, Review Focus 7 плана, PL-41): повторы магазина не сливаются при старте 0.4.0 или
// сливаются при каждом старте; учёт не суммируется на ключ (или задевает чужие программы);
// «Ждут решения» остаются по пути версии; ошибка слияния ставит флаг (повтора не будет) или
// оставляет службу без списка программ; новая установка без флага сливает впустую.
func TestStoreMergeOnceAtStart(t *testing.T) {
	st := storeBase033(t)
	h := newHarness(t, withStore(st), configured(), withPrograms(typicalScan()))
	h.start()
	id := h.childID(childSID)
	if len(st.programs) != 1 {
		t.Fatalf("строк программ после слияния %d: %v, want 1", len(st.programs), st.programs)
	}
	if p := st.programs[edgeKey]; p.Name != "Microsoft Edge" || !p.Hidden || p.Category != "other" || p.Source != ProgramSourceParent {
		t.Errorf("строка ключа: %+v", p)
	}
	wantUsage := map[string]int{
		fmt.Sprintf("%d|2026-10-14|other|program|%s", id, edgeKey):       900,
		fmt.Sprintf("%d|2026-10-19|other|program|%s", id, edgeKey):       60,
		fmt.Sprintf("%d|2026-10-19|games|program|%s", id, pathMinecraft): 70,
	}
	if jsonOf(t, st.usage) != jsonOf(t, wantUsage) {
		t.Errorf("учёт после слияния:\n got %v\nwant %v", st.usage, wantUsage)
	}
	if len(st.pending) != 0 {
		t.Errorf("«Ждут решения» после слияния: %v, want пусто (программа есть в списке)", st.pending)
	}
	if st.settings[settingStoreMerged] != "1" {
		t.Errorf("флаг слияния %q, want 1", st.settings[settingStoreMerged])
	}
	if !strings.Contains(h.logText(), "Повторы приложений магазина слиты: 3 строк в 1") {
		t.Errorf("журнал работы: %s", h.logText())
	}
	h.svc.mu.Lock()
	ready, res := h.svc.progReady, h.svc.classifier.Classify(edge131, "", "")
	h.svc.mu.Unlock()
	if !ready || res.Verdict != programs.VerdictHidden {
		t.Errorf("после слияния: progReady %v, Classify(131) %+v, want скрыта", ready, res)
	}
	tok := h.login(adminPeer())
	var list ipc.ProgramsReply
	h.ok(adminPeer(), tok, ipc.CmdListPrograms, nil, &list)
	if p, ok := programDTO(list, edgeKey); !ok || p.Publisher != "Microsoft Corporation" || !p.Hidden || len(list.Programs) != 1 {
		t.Errorf("«Программы»: %+v (есть %v, строк %d), want одна скрытая строка ключа с издателем Microsoft", p, ok, len(list.Programs))
	}
	h.svc.Shutdown(h.real.Now())

	// Перезапуск: слияние не повторяется.
	h2 := newHarness(t, withStore(st), configured(), withPrograms(typicalScan()))
	h2.start()
	if st.mergeCalls != 1 {
		t.Errorf("MergePrograms вызван %d раз за два старта, want 1", st.mergeCalls)
	}
}

// Ловит (RM4-11, T6-11): сбой записи слияния ставит флаг (повтора не будет) или оставляет
// службу без списка программ; следующий старт не сливает.
func TestStoreMergeRetriedAfterFailure(t *testing.T) {
	st := storeBase033(t)
	st.failMerge = true
	h := newHarness(t, withStore(st), configured(), withPrograms(typicalScan()))
	h.start()
	if _, ok := st.settings[settingStoreMerged]; ok {
		t.Errorf("флаг слияния поставлен после сбоя: %q", st.settings[settingStoreMerged])
	}
	h.svc.mu.Lock()
	ready := h.svc.progReady
	h.svc.mu.Unlock()
	if !ready || len(st.programs) != 3 {
		t.Errorf("после сбоя: progReady %v, строк %d; want true и 3 (база не тронута)", ready, len(st.programs))
	}
	if !strings.Contains(h.logText(), "Повторы приложений магазина не слиты") {
		t.Errorf("журнал работы после сбоя: %s", h.logText())
	}
	h.svc.Shutdown(h.real.Now())
	st.failMerge = false
	h2 := newHarness(t, withStore(st), configured(), withPrograms(typicalScan()))
	h2.start()
	if len(st.programs) != 1 || st.settings[settingStoreMerged] != "1" {
		t.Errorf("второй старт: строк %d, флаг %q; want 1 и 1", len(st.programs), st.settings[settingStoreMerged])
	}
}

// Ловит (T6-11): новая установка (снимок по ключам) не ставит флаг слияния и при следующем
// старте зря строит план.
func TestStoreMergeFlagOnFreshInstall(t *testing.T) {
	h := newHarness(t, configured(), withPrograms(typicalScan()))
	h.start()
	if h.store.settings[settingStoreMerged] != "1" || h.store.mergeCalls != 0 {
		t.Errorf("новая установка: флаг %q, вызовов слияния %d; want 1 и 0", h.store.settings[settingStoreMerged], h.store.mergeCalls)
	}
}
