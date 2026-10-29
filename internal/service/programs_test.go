package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"pcblock/internal/programs"
)

// Корни компьютера в тестах службы (как их даст адаптер Windows, уже нормализованные).
var svcRoots = programs.Roots{
	SystemRoot:      `c:\windows`,
	ProgramFiles:    `c:\program files`,
	ProgramFilesX86: `c:\program files (x86)`,
	InstallDir:      `c:\program files\pcblock`,
}

// Пути программ в тестах.
const (
	pathBrave     = `c:\program files\brave software\brave-browser\application\brave.exe`
	pathBraveUpd  = `c:\program files\brave software\brave-browser\application\123.0\updater.exe`
	pathMinecraft = `c:\program files (x86)\minecraft launcher\minecraftlauncher.exe`
	pathChrome    = `c:\program files\google\chrome\application\chrome.exe`
	pathGeoGebra  = `c:\program files\geogebra\geogebra.exe`
	pathSuperGame = `c:\users\миша\downloads\supergame.exe`
	pathHomework  = `c:\users\миша\downloads\homework.exe`
)

// withPrograms подключает сканер программ (и корни компьютера) до New.
func withPrograms(sc *fakeScanner) hopt {
	return func(h *harness) { h.cfg.Scanner = sc; h.cfg.Roots = svcRoots }
}

// withMeta подключает свойства файлов.
func withMeta(m *fakeMeta) hopt {
	return func(h *harness) { h.cfg.FileMeta = m.meta }
}

// typicalScan — что находит сканер на обычном компьютере.
func typicalScan() *fakeScanner {
	return &fakeScanner{list: []FoundProgram{
		{Path: `C:\Program Files\Brave Software\Brave-Browser\Application\brave.exe`, Name: "Brave", Publisher: "Brave Software, Inc."},
		{Path: `C:\Program Files (x86)\Minecraft Launcher\MinecraftLauncher.exe`, Name: "Minecraft Launcher", Publisher: "Mojang"},
		{Path: pathChrome, Name: "", Publisher: "Google LLC"},
		{Path: pathGeoGebra, Name: "GeoGebra Classic"},
		{Path: pathBrave, Name: "Brave (повтор)"},                                  // повтор того же пути
		{Path: `c:\users\миша\downloads\x.exe`, Name: "Не из Program Files"},       // вне доверенных: не снимок
		{Path: `c:\program files\pcblock\pcblock.exe`, Name: "PC Block"},           // система: не нужен
		{Path: `c:\program files games\steam.exe`, Name: "Похож на Program Files"}, // не доверенный
		{Path: `relative.exe`, Name: "Непригодный"},
	}}
}

// Ловит: снимок не делается при первом старте (обновление 0.2.5 → 0.3.0, RM3-10), игры из
// каталога становятся «Прочим», в снимок попадают программы вне Program Files (их ребёнок мог
// положить сам) или PC Block, повтор пути даёт две записи, флаг не ставится.
func TestSeedProgramsOnFirstStart(t *testing.T) {
	sc := typicalScan()
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	h.start()
	if sc.callCount() != 1 {
		t.Fatalf("сканер вызван %d раз, want 1", sc.callCount())
	}
	want := map[string]Program{
		pathBrave: {Path: pathBrave, Name: "Brave", Publisher: "Brave Software, Inc.", Category: "other", Source: "catalog",
			AppRoot: `c:\program files\brave software`},
		pathMinecraft: {Path: pathMinecraft, Name: "Minecraft Launcher", Publisher: "Mojang", Category: "games", Source: "catalog",
			AppRoot: `c:\program files (x86)\minecraft launcher`},
		pathChrome: {Path: pathChrome, Name: "chrome", Publisher: "Google LLC", Category: "other", Source: "catalog",
			AppRoot: `c:\program files\google`},
		pathGeoGebra: {Path: pathGeoGebra, Name: "GeoGebra Classic", Category: "other", Source: "snapshot",
			AppRoot: `c:\program files\geogebra`},
	}
	got, _ := h.store.ListPrograms()
	if len(got) != len(want) {
		t.Fatalf("в снимке %d программ, want %d: %+v", len(got), len(want), got)
	}
	for _, p := range got {
		w := want[p.Path]
		w.ID = p.ID
		if p != w {
			t.Errorf("снимок:\n got %+v\nwant %+v", p, w)
		}
	}
	if v := h.store.settings["programs.seeded"]; v != "1" {
		t.Errorf("флаг programs.seeded = %q, want «1»", v)
	}
	h.svc.mu.Lock()
	ready := h.svc.programsReady()
	upd := h.svc.classifier.Classify(pathBraveUpd, "")
	h.svc.mu.Unlock()
	if !ready {
		t.Error("после снимка programsReady = false")
	}
	if upd.Verdict != programs.VerdictAllow || upd.Category != "other" {
		t.Errorf("обновление Brave после снимка: %+v, want разрешена как «Прочее»", upd)
	}
}

// Ловит: снимок повторяется при каждом старте (затирает решения или тратит 10 с сканера) и
// повторное сидирование меняет решения родителя.
func TestSeedProgramsOnceAndKeepsDecisions(t *testing.T) {
	st := newFakeStore()
	st.programs[pathBrave] = Program{ID: 1, Path: pathBrave, Name: "Brave", Category: "games", Source: "parent", DecidedAt: 5}
	st.nextProg = 2
	sc := typicalScan()
	h := newHarness(t, withStore(st), configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	h.start()
	if p := st.programs[pathBrave]; p.Category != "games" || p.Source != "parent" || p.DecidedAt != 5 {
		t.Errorf("решение родителя после снимка: %+v", p)
	}
	sc2 := typicalScan()
	h2 := newHarness(t, withStore(st), withPrograms(sc2))
	h2.start()
	if sc2.callCount() != 0 {
		t.Errorf("второй старт: сканер вызван %d раз, want 0", sc2.callCount())
	}
	h2.svc.mu.Lock()
	defer h2.svc.mu.Unlock()
	if !h2.svc.programsReady() {
		t.Error("второй старт: programsReady = false")
	}
	if r := h2.svc.classifier.Classify(pathBraveUpd, ""); r.Category != "games" {
		t.Errorf("второй старт: список не загружен в классификатор: %+v", r)
	}
}

// Ловит: ошибка сканирования считается «пустым компьютером» (всё незнакомое, наблюдатель закроет
// Brave), флаг ставится без снимка, повтора нет до перезапуска (RM3-10, R18).
func TestSeedProgramsScanError(t *testing.T) {
	sc := typicalScan()
	sc.err = errors.New("доступ запрещён")
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	h.start()
	h.svc.mu.Lock()
	ready := h.svc.programsReady()
	h.svc.mu.Unlock()
	if ready {
		t.Fatal("ошибка сканирования: programsReady = true")
	}
	if _, ok := h.store.settings["programs.seeded"]; ok {
		t.Error("ошибка сканирования: флаг поставлен")
	}
	if len(h.store.programs) != 0 {
		t.Errorf("ошибка сканирования: в списке %d программ", len(h.store.programs))
	}
	if !strings.Contains(h.logText(), "доступ запрещён") {
		t.Errorf("ошибки нет в журнале работы:\n%s", h.logText())
	}
	// Через минуту повтора ещё нет, через 10 минут — есть и удаётся.
	sc.mu.Lock()
	sc.err = nil
	sc.mu.Unlock()
	h.real.Advance(time.Minute)
	h.svc.Housekeep(h.real.Now())
	if sc.callCount() != 1 {
		t.Errorf("повтор раньше срока: вызовов %d", sc.callCount())
	}
	h.real.Advance(programsRetryEvery)
	h.svc.Housekeep(h.real.Now())
	h.svc.mu.Lock()
	ready = h.svc.programsReady()
	h.svc.mu.Unlock()
	if sc.callCount() != 2 || !ready {
		t.Errorf("повтор через %v: вызовов %d, готово %v; want 2, true", programsRetryEvery, sc.callCount(), ready)
	}
}

// Ловит: без сканера (адаптер не подключён) или при сбое чтения списка программ служба считает
// список готовым: наблюдатель закрыл бы всё, чего нет в пустом классификаторе.
func TestProgramsNotReadyWithoutScannerOrList(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	h.svc.mu.Lock()
	if h.svc.programsReady() {
		t.Error("без сканера: programsReady = true")
	}
	h.svc.mu.Unlock()

	st := newFakeStore()
	st.settings["programs.seeded"] = "1"
	st.failPrograms = true
	h2 := newHarness(t, withStore(st), withPrograms(typicalScan()))
	h2.start()
	h2.svc.mu.Lock()
	if h2.svc.programsReady() {
		t.Error("список программ не прочитан: programsReady = true")
	}
	h2.svc.mu.Unlock()
}
