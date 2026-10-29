package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Корни компьютера в тестах службы (как их даст адаптер Windows, уже нормализованные).
var svcRoots = programs.Roots{
	SystemRoot:      `c:\windows`,
	ProgramFiles:    `c:\program files`,
	ProgramFilesX86: `c:\program files (x86)`,
	InstallDir:      `c:\program files\pcblock`,
	ProgramData:     `c:\programdata`,
	UserProfiles:    `c:\users`,
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
	upd := h.svc.classifier.Classify(pathBraveUpd, "", "")
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
	if r := h2.svc.classifier.Classify(pathBraveUpd, "", ""); r.Category != "games" {
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
	h.svc.waitSeed()
	if sc.callCount() != 1 {
		t.Errorf("повтор раньше срока: вызовов %d", sc.callCount())
	}
	h.real.Advance(programsRetryEvery)
	h.svc.Housekeep(h.real.Now())
	h.svc.waitSeed()
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

// Ловит (Д-3): пакеты Microsoft Store в снимке не попадают в список (их считают «системой»), игра
// из Store по семейству пакета становится «Прочим», корень приложения хранится с версией.
func TestSeedProgramsStorePackages(t *testing.T) {
	const wa = `c:\program files\windowsapps\`
	calc := wa + `microsoft.windowscalculator_11.2405.2.0_x64__8wekyb3d8bbwe\calculatorapp.exe`
	java := wa + `microsoft.minecraftjavaedition_1.0.0.0_x64__8wekyb3d8bbwe\minecraft.exe.launcher.exe`
	widgets := wa + `microsoftwindows.client.webexperience_524.33200.0.0_x64__cw5n1h2txyewy\widgets.exe`
	sc := &fakeScanner{list: []FoundProgram{{Path: calc, Name: "Калькулятор"}, {Path: java, Name: "Minecraft"}, {Path: widgets, Name: "Виджеты"}}}
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	h.start()
	got := map[string]string{}
	for _, p := range h.store.programs {
		got[p.Path] = p.Category + "|" + p.Source + "|" + p.AppRoot
	}
	want := map[string]string{
		calc: "other|snapshot|" + wa + `microsoft.windowscalculator__8wekyb3d8bbwe`,
		java: "games|catalog|" + wa + `microsoft.minecraftjavaedition__8wekyb3d8bbwe`,
	}
	if jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("снимок пакетов Store:\n got %v\nwant %v", got, want)
	}
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	if r := h.svc.classifier.Classify(wa+`microsoft.windowscalculator_11.2502.0.0_x64__8wekyb3d8bbwe\calculatorapp.exe`, "", ""); r.Verdict != programs.VerdictAllow {
		t.Errorf("калькулятор новой версии: %+v, want разрешён", r)
	}
}

// within выполняет f и ждёт его не дольше d: иначе тест падает (вызов ждёт блокировку службы).
func within(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); f() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s не ответил за %v: ждёт блокировку службы", what, d)
	}
}

// Ловит (Д-2): сканирование Program Files (до 10 с на настоящем компьютере) идёт под блокировкой
// службы: канал связи, значок и страж ждут его. Повтор в Housekeep и мастер ScanPrograms; второй
// ScanPrograms во время первого не запускает второе сканирование.
func TestScanProgramsOutsideServiceLock(t *testing.T) {
	sc := typicalScan()
	sc.err = errors.New("занято")
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	h.start()
	h.childLogsIn()
	t.Cleanup(sc.release)

	sc.mu.Lock()
	sc.err = nil
	sc.mu.Unlock()
	sc.hold()
	h.real.Advance(programsRetryEvery)
	hk := make(chan struct{})
	go func() { defer close(hk); h.svc.Housekeep(h.real.Now()) }()
	if !sc.waitEntered(2 * time.Second) {
		t.Fatal("Housekeep не начал повтор сканирования")
	}
	within(t, time.Second, "GetChildStatus во время сканирования", func() {
		h.svc.Handle(childPeer(), ipc.Msg{Cmd: ipc.CmdGetChildStatus}, h.real.Now())
	})
	within(t, time.Second, "GuardPoll во время сканирования", func() {
		h.svc.Handle(guardPeer(), ipc.Msg{Cmd: ipc.CmdGuardPoll, Payload: ipc.Payload(ipc.GuardPollRequest{Sample: activeSample})}, h.real.Now())
	})
	sc.release()
	<-hk
	h.svc.waitSeed()
	h.svc.mu.Lock()
	ready := h.svc.programsReady()
	h.svc.mu.Unlock()
	if !ready || sc.callCount() != 2 {
		t.Fatalf("после повтора: готово %v, вызовов %d; want true, 2", ready, sc.callCount())
	}
}

// Ловит (Д-2): мастер ScanPrograms держит блокировку службы на время сканирования; второй вызов
// во время первого запускает второе сканирование.
func TestWizardScanOutsideServiceLock(t *testing.T) {
	sc := typicalScan()
	h := newHarness(t, withPrograms(sc))
	h.start()
	t.Cleanup(sc.release)
	n := sc.callCount()
	sc.hold()
	replies := make(chan ipc.Reply, 2)
	for i := 0; i < 2; i++ {
		go func() {
			replies <- h.svc.Handle(elevatedPeer(), ipc.Msg{Cmd: ipc.CmdScanPrograms}, h.real.Now())
		}()
	}
	if !sc.waitEntered(2 * time.Second) {
		t.Fatal("ScanPrograms не начал сканирование")
	}
	within(t, time.Second, "GetChildStatus во время сканирования мастера", func() {
		h.svc.Handle(childPeer(), ipc.Msg{Cmd: ipc.CmdGetChildStatus}, h.real.Now())
	})
	time.Sleep(100 * time.Millisecond) // второй ScanPrograms успевает дойти до сканера
	sc.release()
	for i := 0; i < 2; i++ {
		select {
		case rep := <-replies:
			var out ipc.ScanProgramsReply
			if !rep.OK || rep.Into(&out) != nil || len(out.Programs) != 4 {
				t.Errorf("ответ ScanPrograms %d: ok %v, %s", i, rep.OK, rep.Payload)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("ScanPrograms не ответил после сканирования")
		}
	}
	if got := sc.callCount() - n; got != 1 {
		t.Errorf("сканирований во время двух ScanPrograms: %d, want 1", got)
	}
}

// Ловит (возврат T4a): Start ждёт сканер (на большом компьютере до 60 с), служба не отвечает
// диспетчеру служб «запущена» за WaitHint 20 с, первый старт после обновления срывается; канал
// во время снимка не отвечает; после снимка список не готов.
func TestStartDoesNotWaitForSnapshot(t *testing.T) {
	sc := typicalScan()
	sc.hold()
	t.Cleanup(sc.release)
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	within(t, 200*time.Millisecond, "Start во время снимка программ", func() {
		if err := h.svc.Start(h.real.Now()); err != nil {
			t.Error(err)
		}
	})
	if !sc.waitEntered(2 * time.Second) {
		t.Fatal("снимок программ не начат")
	}
	within(t, 200*time.Millisecond, "GetChildStatus во время снимка", func() {
		h.svc.Handle(childPeer(), ipc.Msg{Cmd: ipc.CmdGetChildStatus}, h.real.Now())
	})
	h.svc.mu.Lock()
	ready := h.svc.programsReady()
	h.svc.mu.Unlock()
	if ready {
		t.Fatal("список готов до конца снимка")
	}
	sc.release()
	h.svc.waitSeed()
	h.svc.mu.Lock()
	ready = h.svc.programsReady()
	h.svc.mu.Unlock()
	if !ready || h.store.settings["programs.seeded"] != "1" {
		t.Errorf("после снимка: готово %v, флаг %q; want true, «1»", ready, h.store.settings["programs.seeded"])
	}
}

// Ловит (возврат T4a): повтор снимка в Housekeep держит цикл учёта (Tick и Housekeep идут из одной
// горутины RunLoop) до конца сканирования; второй повтор запускается, пока идёт первый.
func TestHousekeepRetryDoesNotBlockTick(t *testing.T) {
	sc := typicalScan()
	sc.err = errors.New("занято")
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	h.start()
	h.childLogsIn()
	sc.mu.Lock()
	sc.err = nil
	sc.mu.Unlock()
	sc.hold()
	t.Cleanup(sc.release)
	h.real.Advance(programsRetryEvery)
	within(t, 200*time.Millisecond, "Housekeep с повтором снимка", func() { h.svc.Housekeep(h.real.Now()) })
	if !sc.waitEntered(2 * time.Second) {
		t.Fatal("повтор снимка не начат")
	}
	before := h.usedS()
	for i := 0; i < 3; i++ {
		within(t, 200*time.Millisecond, "шаг учёта во время снимка", func() { h.step(activeSample) })
	}
	if got := h.usedS(); got != before+3 {
		t.Errorf("учёт во время снимка: %d → %d, want +3", before, got)
	}
	h.real.Advance(programsRetryEvery)
	h.svc.Housekeep(h.real.Now())
	sc.release()
	h.svc.waitSeed()
	h.svc.mu.Lock()
	ready := h.svc.programsReady()
	h.svc.mu.Unlock()
	if !ready || sc.callCount() != 2 {
		t.Errorf("после повтора: готово %v, сканирований %d; want true, 2 (второй не запущен, пока идёт первый)", ready, sc.callCount())
	}
	if n := strings.Count(h.logText(), "Снимок программ: найдено"); n != 1 {
		t.Errorf("снимок записан %d раз, want 1 (одна горутина снимка)", n)
	}
}

// Ловит (возврат T4a): Shutdown ждёт сканер, а сканер, закончив после остановки, пишет в уже
// закрытую базу (снимок, флаг) или гоняется с остановкой (-race).
func TestShutdownDuringSnapshot(t *testing.T) {
	sc := typicalScan()
	sc.hold()
	t.Cleanup(sc.release)
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc))
	if err := h.svc.Start(h.real.Now()); err != nil { // без ожидания снимка
		t.Fatal(err)
	}
	if !sc.waitEntered(2 * time.Second) {
		t.Fatal("снимок программ не начат")
	}
	within(t, 200*time.Millisecond, "Shutdown во время снимка", func() { h.svc.Shutdown(h.real.Now()) })
	h.store.Close()
	sc.release()
	h.svc.waitSeed()
	if n, flag := len(h.store.programs), h.store.settings["programs.seeded"]; n != 0 || flag != "" {
		t.Errorf("после остановки записано: программ %d, флаг %q; want ничего", n, flag)
	}
}
