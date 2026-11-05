package service

import (
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Пути Brave в профиле Миши (choco ставит Brave в %LOCALAPPDATA%, PL-20).
const (
	pathBraveProfile    = `c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe`
	pathBraveProfileUpd = `c:\users\миша\appdata\local\bravesoftware\update\braveupdate.exe`
	braveCompany        = "Brave Software, Inc."
)

// Ловит (RM4-8, 8.2.5 п. 4, R13, D-65): ребёнку о запрещённом браузере приходит плашка
// «Программа «chrome» запрещена родителем» вместо «Этот браузер запрещён. Пользуйся Brave»;
// браузер не закрыт или не записан нарушением; плашку браузера получает запрещённая родителем
// программа; автозапуск браузера даёт плашку или нарушение.
func TestBrowserPlate(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(120, -1, "lock"))
	explorer := h.manualReady(w)
	procs := startManual(w, explorer, pathChrome) // chrome запрещён каталогом, не родителем
	h.watch()
	want := []ipc.BlockedPlate{{Kind: "browser", Text: "Этот браузер запрещён. Пользуйся Brave", ShowS: 10, Path: pathChrome}}
	if got := platesOf(h.poll(activeSample)); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("плашка браузера:\n got %s\nwant %s", jsonOf(t, got), jsonOf(t, want))
	}
	if got := h.eventTexts(evProgramForbiddenTry); jsonOf(t, got) != jsonOf(t, []string{"Миша: попытка открыть запрещённое: «chrome»"}) {
		t.Errorf("нарушение: %v", got)
	}
	if w.alive(procs[pathChrome]) {
		t.Error("chrome не закрыт")
	}

	// Запрещённая родителем программа (не браузер) — обычная плашка.
	h.setCategory(pathGeoGebra, "blocked")
	h.poll(activeSample) // ребёнок только что нажал клавишу
	procs = startManual(w, explorer, pathGeoGebra)
	h.watch()
	want = []ipc.BlockedPlate{{Kind: ipc.BlockedForbidden, Text: "Программа «GeoGebra Classic» запрещена родителем", ShowS: 10, Path: pathGeoGebra}}
	if got := platesOf(h.poll(activeSample)); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("плашка запрещённой программы:\n got %s\nwant %s", jsonOf(t, got), jsonOf(t, want))
	}
	if w.alive(procs[pathGeoGebra]) {
		t.Error("запрещённая GeoGebra не закрыта")
	}
}

// Ловит (R13, D-65, PL-40): браузер, запущенный автозагрузкой при входе, даёт ребёнку плашку
// браузера или нарушение вместо молчаливого закрытия с записью «запускалась сама».
func TestBrowserAutostartSilent(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	explorer := w.start(childSession, pathExplorer)
	h.real.Advance(5 * time.Second)
	h.poll(inputSample(1000, pathExplorer))
	p := w.startFrom(childSession, pathChrome, explorer)
	h.watch()
	if w.alive(p) {
		t.Fatal("автозапуск chrome не закрыт")
	}
	if got := platesOf(h.poll(inputSample(1000, pathExplorer))); len(got) != 0 {
		t.Errorf("автозапуск браузера: плашки %s, want нет", jsonOf(t, got))
	}
	h.settle()
	if n := len(h.store.eventsOf(evProgramForbiddenTry)); n != 0 {
		t.Errorf("нарушений при автозапуске %d, want 0", n)
	}
	if got := h.eventTexts(evProgramAutoClosed); jsonOf(t, got) != jsonOf(t, []string{"Миша: программа «chrome» запускалась сама и закрыта"}) {
		t.Errorf("журнал автозапуска: %v", got)
	}
}

// Ловит (T6-5, PL-19, PL-20, RM4-8): в тестовом режиме издатель Brave из профиля раннера (вне
// WatchDir) не читается — Brave «незнакомая», сайты не различаются (isBrave ложь); или чтение
// свойств вне WatchDir приводит к закрытию процесса раннера.
func TestBraveProfileReadsPublisherInTestMode(t *testing.T) {
	video := func(h *harness) { h.store.sites["youtube.com"] = "video_social" }
	sample := ipc.SensorSample{SessionActive: true, Foreground: ipc.CatNameOther, ForegroundPath: pathBraveProfile, ForegroundDomain: "youtube.com"}

	meta := newMeta()
	meta.set(pathBraveProfile, braveCompany, "Brave Browser", "1:1")
	w := newWatcher()
	h := watchHarness(t, w, withTestMode(TestMode{WatchDir: `c:\temp\pcb-programs`}), withMeta(meta), video)
	p := w.start(childSession, pathBraveProfile)
	h.watch()
	if !w.alive(p) {
		t.Fatal("Brave вне WatchDir закрыт в тестовом режиме")
	}
	h.svc.mu.Lock()
	pub := h.svc.pubSeen[pathBraveProfile]
	h.svc.mu.Unlock()
	if pub != braveCompany {
		t.Errorf("pubSeen[Brave в профиле] = %q, want %q", pub, braveCompany)
	}
	if cat, u, _ := h.foreground(sample); cat != core.CatVideoSocial || u != (usageSubject{usageKindSite, "youtube.com"}) {
		t.Errorf("Brave в профиле на youtube.com: (%v, %+v), want видео по сайту", cat, u)
	}

	// Без чтения свойств издатель неизвестен: Brave — незнакомая, сайт не учитывается.
	w2 := newWatcher()
	h2 := watchHarness(t, w2, withTestMode(TestMode{WatchDir: `c:\temp\pcb-programs`}), video)
	p2 := w2.start(childSession, pathBraveProfile)
	h2.watch()
	if !w2.alive(p2) {
		t.Fatal("Brave без издателя вне WatchDir закрыт в тестовом режиме")
	}
	if cat, u, _ := h2.foreground(sample); cat != core.CatOther || u != (usageSubject{usageKindProgram, pathBraveProfile}) {
		t.Errorf("Brave без издателя: (%v, %+v), want «Прочее» по программе", cat, u)
	}
}

// Ловит (T6-3, PL-18, R8): в режиме «только учёба» обновлятор Brave из профиля закрывается как
// «Прочее» или «незнакомая»; игра при этом не закрыта.
func TestStudyOnlyKeepsBraveHelpers(t *testing.T) {
	meta := newMeta()
	meta.set(pathBraveProfileUpd, braveCompany, "Brave Update", "2:2")
	meta.set(pathBraveProfile, braveCompany, "Brave Browser", "1:1")
	w := newWatcher()
	h := watchHarness(t, w, withRules(0, -1, "study_only"), withMeta(meta))
	upd := w.start(childSession, pathBraveProfileUpd)
	brave := w.start(childSession, pathBraveProfile)
	game := w.start(childSession, pathMinecraft)
	h.watch()
	if !w.alive(upd) {
		t.Error("обновлятор Brave в профиле закрыт в «только учёбе»")
	}
	if !w.alive(brave) {
		t.Error("Brave в профиле закрыт в «только учёбе» (вкладки закрывает служба по домену)")
	}
	if w.alive(game) {
		t.Error("игра не закрыта в «только учёбе»")
	}
	if got := h.svc.classifier.Classify(pathBraveProfileUpd, "", braveCompany).Verdict; got != programs.VerdictSystem {
		t.Errorf("обновлятор Brave в профиле: %v, want система", got)
	}
}
