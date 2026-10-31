package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Скрытые программы (FB-9, D-57, RF9-1…RF9-4): служба, наблюдатель и учёт.

// Пути двух процессов одной игры: запускалка и сама игра (сценарий 8.3.4 п.4).
const (
	pathLauncher = `c:\program files\epic games\stalker2\stalker2.exe`
	pathShipping = `c:\program files\epic games\stalker2\binaries\win64\stalker2-win64-shipping.exe`
)

// withStalker — запускалка и игра в списке программ как «Игры» (до Start).
func withStalker() hopt {
	return func(h *harness) {
		for _, p := range []Program{
			{Path: pathLauncher, Name: "Stalker2", Publisher: "Epic Games", Category: "games", Source: "parent", DecidedAt: 5},
			{Path: pathShipping, Name: "Stalker2-Win64-Shipping", Publisher: "GSC Game World", Category: "games", Source: "catalog"},
		} {
			if err := h.store.UpsertProgram(p); err != nil {
				h.t.Fatal(err)
			}
		}
	}
}

// hide — команда SetProgramsHidden от родителя.
func (h *harness) hide(tok string, hidden bool, paths ...string) ipc.DecideProgramReply {
	h.t.Helper()
	var rep ipc.DecideProgramReply
	h.ok(adminPeer(), tok, ipc.CmdSetProgramsHidden, ipc.SetProgramsHiddenRequest{Paths: paths, Hidden: hidden}, &rep)
	return rep
}

// programDTO — строка «Все программы» по пути; ok = false — строки нет.
func programDTO(rep ipc.ProgramsReply, path string) (ipc.ProgramDTO, bool) {
	for _, p := range rep.Programs {
		if p.Path == path {
			return p, true
		}
	}
	return ipc.ProgramDTO{}, false
}

// Ловит: скрытие меняет категорию (RF9-1: «Показать» вернёт прежнюю), не доходит до
// классификатора без перезапуска, список не отдаёт флаг (фильтр «Скрытые» пуст), неделя скрытой
// видна, журнал не «вы скрыли «X»» / «вы снова показали «X»», сообщение не «Stalker2 скрыта»,
// «Отменить» не возвращает видимость, неизвестные пути — успех, больше 200 путей принимается,
// скрытие ставит плашки ребёнку (PL-16).
func TestSetProgramsHidden(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withStalker())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.store.AddUsage(id, "2026-10-19", "games", "program", pathLauncher, 120)

	rep := h.hide(tok, true, strings.ToUpper(pathLauncher), pathLauncher, `c:\nope\x.exe`)
	if rep.Message != "Stalker2 скрыта" || rep.UndoID == "" || rep.UndoUntilMs != h.real.Now().Add(10*time.Second).UnixMilli() {
		t.Errorf("ответ: %+v", rep)
	}
	got, ok := programDTO(rep.Programs, pathLauncher)
	want := ipc.ProgramDTO{Path: pathLauncher, Name: "Stalker2", Publisher: "Epic Games", Category: "games", Source: "parent", WeekS: 0, Hidden: true}
	if !ok || got != want {
		t.Errorf("строка скрытой = %+v (есть %v), want %+v", got, ok, want)
	}
	if p, _, _ := h.store.GetProgram(pathLauncher); !p.Hidden || p.Category != "games" || p.Source != "parent" || p.DecidedAt != 5 {
		t.Errorf("запись после скрытия: %+v", p)
	}
	if p, _ := programDTO(rep.Programs, pathShipping); p.Hidden {
		t.Errorf("игра скрыта вместе с запускалкой: %+v", p)
	}
	h.svc.mu.Lock()
	r := h.svc.classifier.Classify(pathLauncher, "", "")
	h.svc.mu.Unlock()
	if r.Verdict != programs.VerdictHidden {
		t.Errorf("классификатор после скрытия: %+v, want «скрыта» без перезапуска", r)
	}
	if got := journalTexts(h, tok, "program_hidden"); !reflect.DeepEqual(got, []string{"Вы скрыли «Stalker2»"}) {
		t.Errorf("журнал скрытия: %q", got)
	}
	if pl := guardPlates(h); len(pl) != 0 {
		t.Errorf("скрытие поставило плашки ребёнку: %+v", pl)
	}

	// «Отменить» — снова видна.
	h.real.Advance(9 * time.Second)
	var undo ipc.UndoProgramReply
	h.ok(adminPeer(), tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: rep.UndoID}, &undo)
	if p, _ := programDTO(undo.Programs, pathLauncher); p.Hidden || p.WeekS != 120 {
		t.Errorf("после отмены скрытия: %+v, want видна и неделя 120 с", p)
	}
	h.svc.mu.Lock()
	r = h.svc.classifier.Classify(pathLauncher, "", "")
	h.svc.mu.Unlock()
	if r.Verdict != programs.VerdictAllow || r.Category != "games" {
		t.Errorf("классификатор после отмены: %+v", r)
	}

	// Две сразу, затем «Показать» одну.
	rep = h.hide(tok, true, pathLauncher, pathShipping, pathBrave)
	if rep.Message != "Скрыто программ: 3" {
		t.Errorf("сообщение для трёх: %q", rep.Message)
	}
	rep = h.hide(tok, false, pathLauncher)
	if rep.Message != "Stalker2 снова в списке" || rep.UndoID == "" {
		t.Errorf("«Показать»: %+v", rep)
	}
	if p, _, _ := h.store.GetProgram(pathLauncher); p.Hidden || p.Category != "games" {
		t.Errorf("после «Показать»: %+v, want видна с категорией «Игры»", p)
	}
	rep = h.hide(tok, false, pathShipping, pathBrave)
	if rep.Message != "Снова в списке программ: 2" {
		t.Errorf("«Показать» двух: %q", rep.Message)
	}
	want2 := []string{"Вы скрыли «Stalker2»", "Вы скрыли 3 программы", "Вы снова показали «Stalker2»", "Вы снова показали 2 программы"}
	if got := journalTexts(h, tok, "program_hidden", "program_shown"); !reflect.DeepEqual(got, want2) {
		t.Errorf("журнал = %q, want %q", got, want2)
	}

	// Отказы.
	if f := h.fail(adminPeer(), tok, ipc.CmdSetProgramsHidden, ipc.SetProgramsHiddenRequest{Paths: []string{`c:\nope\x.exe`}, Hidden: true}); f.Error != "Такой программы нет в списке" {
		t.Errorf("только неизвестные пути: %q", f.Error)
	}
	h.store.UpsertProgram(Program{Path: `c:\program files\vendor\driver.exe`, Name: "Драйвер", Category: "system", Source: "parent"})
	if f := h.fail(adminPeer(), tok, ipc.CmdSetProgramsHidden, ipc.SetProgramsHiddenRequest{Paths: []string{`c:\program files\vendor\driver.exe`}, Hidden: true}); f.Error != "Такой программы нет в списке" {
		t.Errorf("только системная: %q (системные не скрывают, RF9-1)", f.Error)
	}
	many := make([]string, 201)
	for i := range many {
		many[i] = fmt.Sprintf(`c:\program files\app%d\app.exe`, i)
	}
	if f := h.fail(adminPeer(), tok, ipc.CmdSetProgramsHidden, ipc.SetProgramsHiddenRequest{Paths: many, Hidden: true}); f.Error != "Можно изменить не больше 200 программ за раз" {
		t.Errorf("201 путь: %q", f.Error)
	}
}

// journalTexts — строки журнала родителя указанных видов в порядке записи (старые первыми).
func journalTexts(h *harness, tok string, kinds ...string) []string {
	h.t.Helper()
	var rep ipc.ListEventsReply
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{}, &rep)
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	out := []string{}
	for i := len(rep.Events) - 1; i >= 0; i-- {
		if e := rep.Events[i]; want[e.Kind] {
			out = append(out, e.Text)
		}
	}
	return out
}

// guardPlates — плашки о программах в очереди стража сеанса ребёнка.
func guardPlates(h *harness) []ipc.BlockedPlate {
	h.t.Helper()
	out := []ipc.BlockedPlate{}
	for _, c := range h.poll(activeSample).Commands {
		if c.Kind == ipc.GuardCmdBlocked && c.Plate != nil {
			out = append(out, *c.Plate)
		}
	}
	return out
}

// Ловит (RF9-2, S38): скрытая программа закрывается — по лимиту игр, в режиме «только учёба»
// или под экраном блокировки; скрытие запускалки «разрешает» игру рядом (Review Focus 1);
// после «Показать» запускалка снова закрывается как игра.
func TestWatchLeavesHidden(t *testing.T) {
	cases := []struct {
		name   string
		rules  hopt
		killed []string
	}{
		{"игры кончились: скрытая не закрыта, игра закрыта", withRules(120, 0, "lock"), []string{pathShipping}},
		{"только учёба: скрытая не закрыта", withRules(0, -1, "study_only"), []string{pathShipping}},
		{"экран блокировки: скрытая не закрыта", withRules(0, -1, "lock"), []string{pathShipping}},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			w := newWatcher()
			h := watchHarness(t, w, withStalker(), k.rules)
			tok := h.login(adminPeer())
			h.hide(tok, true, pathLauncher)
			procs := startAll(w, pathLauncher, pathShipping)
			h.watch()
			if got := w.killedPaths(); !reflect.DeepEqual(got, k.killed) {
				t.Errorf("закрыты %v, want %v", got, k.killed)
			}
			if !w.alive(procs[pathLauncher]) {
				t.Error("скрытая запускалка закрыта")
			}
			for _, kind := range []string{evProgramClosedGames, evProgramClosedStudyOnly} {
				for _, e := range h.store.eventsOf(kind) {
					if e.Details["path"] == pathLauncher {
						t.Errorf("журнал о закрытии скрытой: %+v", e)
					}
				}
			}
			// «Показать» — уже запущенная и следующий запуск запускалки закрыты (S38).
			h.hide(tok, false, pathLauncher)
			again := w.start(childSession, pathLauncher)
			h.real.Advance(time.Second)
			h.watch()
			if w.alive(procs[pathLauncher]) || w.alive(again) {
				t.Errorf("после «Показать»: запущенная жива %v, новый запуск жив %v; want обе закрыты", w.alive(procs[pathLauncher]), w.alive(again))
			}
		})
	}
}

// Ловит (RF9-2, Review Focus 2): скрытая программа вне доверенных каталогов после подмены файла
// остаётся скрытой (подмена файла важнее скрытия: снова «ждёт решения» и закрыта).
func TestHiddenSubstitutedFileWaits(t *testing.T) {
	meta := newMeta()
	meta.set(pathSuperGame, "", "", "1024:1700000000")
	w := newWatcher()
	h := watchHarness(t, w, withMeta(meta))
	if err := h.store.UpsertProgram(Program{Path: pathSuperGame, Name: "SuperGame", Category: "games", Source: "parent", Fingerprint: "1024:1700000000"}); err != nil {
		t.Fatal(err)
	}
	tok := h.login(adminPeer())
	h.hide(tok, true, pathSuperGame)
	game := w.start(childSession, pathSuperGame)
	h.watch()
	if !w.alive(game) {
		t.Fatal("скрытая с верным отпечатком закрыта")
	}
	meta.set(pathSuperGame, "", "", "2048:1700000500")
	w.exit(game)
	game2 := w.start(childSession, pathSuperGame)
	h.real.Advance(time.Second)
	h.watch()
	if w.alive(game2) {
		t.Error("скрытая с подменённым файлом не закрыта")
	}
	if pend, _ := h.store.ListPending(); len(pend) != 1 || pend[0].Path != pathSuperGame || pend[0].WasCategory != "games" {
		t.Errorf("«Ждут решения» после подмены скрытой: %+v", pend)
	}
}

// Ловит (RF9-3, R3): секунда скрытой игры на переднем плане идёт в игры, не в общее время, или
// пишется в учёт по программе (Ruling: не пишется — скрытая «никак не фигурирует», D-57).
func TestHiddenForegroundCountsAsOther(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withStalker())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.hide(tok, true, pathLauncher)
	h.run(10, fgSample(pathLauncher, ipc.CatNameGames))
	h.run(5, fgSample(pathShipping, ipc.CatNameOther))
	st, _ := h.svc.children[id].eng.State()
	if h.usedS() != 15 || st.GamesS != 5 {
		t.Errorf("общее %d с, игры %d с; want 15, 5 (скрытая — «Прочее»)", h.usedS(), st.GamesS)
	}
	h.svc.Shutdown(h.real.Now())
	const day = "2026-10-19"
	for _, cat := range []string{"games", "other"} {
		if n := h.usageOf(day, cat, pathLauncher); n != 0 {
			t.Errorf("учёт скрытой (%s) = %d с, want 0", cat, n)
		}
	}
	if n := h.usageOf(day, "games", pathShipping); n != 5 {
		t.Errorf("учёт игры = %d с, want 5", n)
	}
}
