package service

import (
	"strings"
	"testing"

	"pcblock/internal/ipc"
)

// Тесты системных инструментов в сеансе ребёнка (веха M6, R17г, FR-29, D-86, S54).

const (
	pathCmd      = `c:\windows\system32\cmd.exe`
	pathPS       = `c:\windows\system32\windowspowershell\v1.0\powershell.exe`
	pathTerminal = `c:\program files\windowsapps\microsoft.windowsterminal_1.21.2361.0_x64__8wekyb3d8bbwe\windowsterminal.exe`
)

func toolEvents(h *harness) []StoredEvent {
	out := []StoredEvent{}
	for _, e := range h.store.eventsOf("bypass") {
		if e.Details["subtype"] == "system_tool" {
			out = append(out, e)
		}
	}
	return out
}

func toolPlates(rep ipc.GuardPollReply) []string {
	out := []string{}
	for _, p := range platesOf(rep) {
		if strings.HasSuffix(p.Text, "недоступна") {
			out = append(out, p.Kind+": "+p.Text)
		}
	}
	return out
}

// Ловит: командная строка, открытая ребёнком из Проводника, не закрывается, закрывается без
// плашки или без записи обхода, или попадает в «Ждут решения»; экран блокируется.
func TestSysToolManualFromExplorerClosed(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	explorer := h.manualReady(w)
	cmd := w.startFrom(childSession, pathCmd, explorer)
	h.watch()
	if w.alive(cmd) {
		t.Fatal("cmd из Проводника при вводе ребёнка не закрыта")
	}
	want := []string{"forbidden: Программа «Командная строка» недоступна"}
	if got := toolPlates(h.poll(inputSample(0, pathExplorer))); jsonOf(t, got) != jsonOf(t, want) {
		t.Fatalf("плашки %v, ждали %v", got, want)
	}
	ev := toolEvents(h)
	if len(ev) != 1 || ev[0].Details["detail"] != "Открыта программа «Командная строка», она закрыта" || ev[0].ChildID != h.childID(childSID) {
		t.Fatalf("записи обхода %v", ev)
	}
	if p, _ := h.store.ListPending(); len(p) != 0 {
		t.Fatalf("инструмент в «Ждут решения»: %v", p)
	}
	if got := h.eventTexts(evProgramForbiddenTry); len(got) != 0 {
		t.Fatalf("лишнее нарушение «попытка открыть запрещённое»: %v", got)
	}
	if h.status().Lock == "bypass" {
		t.Fatal("экран заблокирован, а не должен (D-41)")
	}
	// Второй раз через 10 с — снова закрыта, запись не повторяется (60 с).
	h.real.Advance(10_000_000_000)
	h.poll(inputSample(0, pathExplorer))
	again := w.startFrom(childSession, pathCmd, explorer)
	h.watch()
	if w.alive(again) {
		t.Fatal("второй запуск cmd не закрыт")
	}
	if n := len(toolEvents(h)); n != 1 {
		t.Fatalf("записей обхода после повтора через 10 с: %d, ждали 1", n)
	}
}

// Ловит: инструмент, запущенный игрой на переднем плане, обновлятором без ввода или самим
// Windows (Терминал без родителя в сеансе), закрывается, пишется обходом или ждёт решения
// родителя (D-86).
func TestSysToolNotByChildUntouched(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	h.manualReady(w)
	steam := w.start(childSession, pathSteam)
	h.poll(inputSample(0, pathSteam)) // игра на переднем плане, ребёнок только что нажимал
	byGame := w.startFrom(childSession, pathCmd, steam)
	updater := w.start(childSession, pathBraveUpd)
	byUpdater := w.startFrom(childSession, pathPS, updater)
	term := w.start(childSession, pathTerminal) // Windows 11 открывает Терминал для консоли сама
	h.watch()
	for _, p := range []ProcInfo{byGame, byUpdater, term} {
		if !w.alive(p) {
			t.Fatalf("%s закрыта, а запустил не ребёнок", p.Path)
		}
	}
	if n := len(toolEvents(h)); n != 0 {
		t.Fatalf("записей обхода: %d, ждали 0", n)
	}
	if p, _ := h.store.ListPending(); len(p) != 0 {
		t.Fatalf("инструменты в «Ждут решения»: %v", p)
	}
}

// Ловит: командная строка, которую ребёнок открыл из окна PowerShell (оба инструмента), остаётся.
func TestSysToolFromToolClosed(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	explorer := h.manualReady(w)
	ps := w.startFrom(childSession, pathPS, explorer)
	h.watch()
	if w.alive(ps) {
		t.Fatal("PowerShell из Проводника не закрыт")
	}
	// Окно PowerShell успело открыть cmd до закрытия (на переднем плане PowerShell).
	ps2 := w.start(childSession, pathPS)
	h.poll(inputSample(0, pathPS))
	cmd := w.startFrom(childSession, pathCmd, ps2)
	h.watch()
	if w.alive(cmd) {
		t.Fatal("cmd из окна PowerShell при вводе не закрыта")
	}
}

// Ловит: в тестовом режиме копия cmd.exe в папке тестовых программ не считается инструментом
// (S54 не проверит правило), а настоящая cmd.exe вне папки трогается (RM3-8).
func TestSysToolTestMode(t *testing.T) {
	const dir = `c:\pcb-test\apps`
	w := newWatcher()
	h := watchHarness(t, w, withTestMode(TestMode{WatchDir: dir, ClockSpeed: 1}))
	explorer := h.manualReady(w)
	copyCmd := w.startFrom(childSession, dir+`\cmd.exe`, explorer)
	realCmd := w.startFrom(childSession, pathCmd, explorer)
	h.watch()
	if w.alive(copyCmd) {
		t.Fatal("тестовая копия cmd.exe из Проводника не закрыта")
	}
	if !w.alive(realCmd) {
		t.Fatal("настоящая cmd.exe закрыта в тестовом режиме (RM3-8)")
	}
	if ev := toolEvents(h); len(ev) != 1 {
		t.Fatalf("записей обхода: %d, ждали 1", len(ev))
	}
}
