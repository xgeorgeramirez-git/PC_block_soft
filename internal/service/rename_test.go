package service

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Свои названия программ (FB-11, D-59, FR-20): команда RenameProgram, журнал, «Отменить» и
// одно название во всех показах (класс A, PL-22).

// rename — команда RenameProgram от родителя.
func (h *harness) rename(tok, path, name string) ipc.DecideProgramReply {
	h.t.Helper()
	var rep ipc.DecideProgramReply
	h.ok(adminPeer(), tok, ipc.CmdRenameProgram, ipc.RenameProgramRequest{Path: path, Name: name}, &rep)
	return rep
}

// Ловит: переименование меняет название из файла (сброс тогда не вернёт его), категорию или
// скрытие; не доходит до классификатора без перезапуска; список не отдаёт название из файла
// (родитель теряется); журнал не «вы переименовали «X» в «Y»»; «Отменить» не возвращает прежнее;
// пустое не сбрасывает; 61 символ и управляющие символы принимаются или отказ не у поля; нет
// программы — успех; переименование ставит плашки ребёнку.
func TestRenameProgram(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withStalker())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())

	rep := h.rename(tok, strings.ToUpper(pathShipping), "  Сталкер 2  ")
	if rep.Message != "Название изменено" || rep.UndoID == "" || rep.UndoUntilMs != h.real.Now().Add(10*time.Second).UnixMilli() {
		t.Errorf("ответ: %+v", rep)
	}
	got, _ := programDTO(rep.Programs, pathShipping)
	want := ipc.ProgramDTO{Path: pathShipping, Name: "Сталкер 2", FileName: "Stalker2-Win64-Shipping", Publisher: "GSC Game World", Category: "games", Source: "catalog"}
	if got != want {
		t.Errorf("строка = %+v, want %+v", got, want)
	}
	if p, _, _ := h.store.GetProgram(pathShipping); p.CustomName != "Сталкер 2" || p.Name != "Stalker2-Win64-Shipping" || p.Category != "games" || p.Source != "catalog" {
		t.Errorf("запись: %+v", p)
	}
	h.svc.mu.Lock()
	r := h.svc.classifier.Classify(pathShipping, "", "")
	h.svc.mu.Unlock()
	if r.Name != "Сталкер 2" || r.Verdict != programs.VerdictAllow {
		t.Errorf("классификатор: %+v", r)
	}
	if pl := guardPlates(h); len(pl) != 0 {
		t.Errorf("переименование поставило плашки: %+v", pl)
	}

	// «Отменить» — снова название из файла.
	h.real.Advance(9 * time.Second)
	var undo ipc.UndoProgramReply
	h.ok(adminPeer(), tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: rep.UndoID}, &undo)
	if p, _ := programDTO(undo.Programs, pathShipping); p.Name != "Stalker2-Win64-Shipping" || p.FileName != "" {
		t.Errorf("после отмены: %+v", p)
	}

	// Снова, затем сброс пустым.
	h.rename(tok, pathShipping, "Сталкер 2")
	rep = h.rename(tok, pathShipping, "   ")
	if p, _ := programDTO(rep.Programs, pathShipping); p.Name != "Stalker2-Win64-Shipping" || p.FileName != "" || rep.Message != "Название изменено" {
		t.Errorf("сброс: %+v, %q", p, rep.Message)
	}
	if p, _, _ := h.store.GetProgram(pathShipping); p.CustomName != "" {
		t.Errorf("сброс в базе: %+v", p)
	}
	wantJ := []string{
		"Вы переименовали «Stalker2-Win64-Shipping» в «Сталкер 2»",
		"Вы переименовали «Stalker2-Win64-Shipping» в «Сталкер 2»",
		"Вы вернули «Сталкер 2» название из файла «Stalker2-Win64-Shipping»",
	}
	if got := journalTexts(h, tok, "program_renamed"); !reflect.DeepEqual(got, wantJ) {
		t.Errorf("журнал = %q, want %q", got, wantJ)
	}
	// То же название ещё раз: журнал не пишется (ничего не изменилось).
	h.rename(tok, pathShipping, "")
	if n := len(journalTexts(h, tok, "program_renamed")); n != 3 {
		t.Errorf("записей после повтора %d, want 3", n)
	}

	// Отказы.
	for _, bad := range []string{strings.Repeat("я", 61), "Моя\nигра"} {
		f := h.fail(adminPeer(), tok, ipc.CmdRenameProgram, ipc.RenameProgramRequest{Path: pathShipping, Name: bad})
		if f.Error != "Название до 60 символов" || f.Field != "name" {
			t.Errorf("%q: %q поле %q", bad, f.Error, f.Field)
		}
	}
	if f := h.fail(adminPeer(), tok, ipc.CmdRenameProgram, ipc.RenameProgramRequest{Path: `c:\nope\x.exe`, Name: "X"}); f.Error != "Такой программы нет в списке" {
		t.Errorf("нет программы: %q", f.Error)
	}
	h.rename(tok, pathShipping, strings.Repeat("я", 60))
}

// Инвариант класса A (D-59, PL-22): переименованная программа выглядит одинаково во всех местах,
// где показывается её название. Каждая строка таблицы — одно место; старые записи журнала
// остаются как записаны.
func TestRenamedNameEverywhere(t *testing.T) {
	const custom = "Моя игра"
	meta := newMeta()
	meta.set(pathSuperGame, "", "", "1024:1700000000")
	w := newWatcher()
	h := watchHarness(t, w, withMeta(meta), withStalker(), withRules(120, 0, "lock"))
	tok := h.login(adminPeer())
	if err := h.store.UpsertProgram(Program{Path: pathSuperGame, Name: "SuperGame", Category: "games", Source: "parent", Fingerprint: "1024:1700000000"}); err != nil {
		t.Fatal(err)
	}
	h.rename(tok, pathSuperGame, "Супер")
	// Старая запись журнала с названием из файла: не переписывается.
	h.svc.mu.Lock()
	h.svc.journal.Write(h.childID(childSID), evProgramClosedGames, map[string]any{"name": "Stalker2-Win64-Shipping", "path": pathShipping})
	h.svc.mu.Unlock()
	h.rename(tok, pathShipping, custom)

	startAll(w, pathShipping)
	h.watch()
	h.real.Advance(time.Second)
	// Подменённый файл (RM3-12): плашка ребёнку о незнакомой и «Ждут решения».
	meta.set(pathSuperGame, "", "", "2048:1")
	startAll(w, pathSuperGame)
	h.watch()
	plates := platesOf(h.poll(activeSample))
	pend, _ := h.store.ListPending()

	view := h.list(tok)
	row, _ := programDTO(view, pathShipping)
	rep := h.decideCat(tok, pathShipping, "study")
	hid := h.hide(tok, true, pathShipping)
	h.svc.mu.Lock()
	res := h.svc.classifier.Classify(pathShipping, "", "")
	h.svc.mu.Unlock()
	closed := h.eventTexts(evProgramClosedGames)

	places := []struct{ place, got, want string }{
		{"список программ", row.Name, custom},
		{"классификатор Result.Name", res.Name, custom},
		{"журнал: закрыта по лимиту игр", closed[len(closed)-1], "Миша: закрыта «" + custom + "»: время на игры закончилось"},
		{"журнал: старая запись как записана", closed[0], "Миша: закрыта «Stalker2-Win64-Shipping»: время на игры закончилось"},
		{"журнал: решение родителя", lastOf(journalTexts(h, tok, "program_decided")), "Вы отнесли «" + custom + "» к категории «Учёба»"},
		{"сообщение о решении", rep.Message, custom + ": учёба"},
		{"сообщение о скрытии", hid.Message, custom + " скрыта"},
		{"журнал: скрытие", lastOf(journalTexts(h, tok, "program_hidden")), "Вы скрыли «" + custom + "»"},
		{"плашка ребёнку о подменённой", plateText(plates, ipc.BlockedUnknown), "Программа «Супер» ждёт разрешения родителя"},
		{"«Ждут решения» — название из файла", firstName(pend), "SuperGame"},
	}
	for _, p := range places {
		if p.got != p.want {
			t.Errorf("%s: %q, want %q", p.place, p.got, p.want)
		}
	}

	// Плашка «Родитель разрешил «X»»: решение по подменённой переименованной программе.
	var dec ipc.DecideProgramReply
	h.ok(adminPeer(), tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: pathSuperGame, Category: "games"}, &dec)
	if got := plateText(platesOf(h.poll(activeSample)), ipc.BlockedDecided); got != "Родитель разрешил «Супер»: это игра. Запусти её снова" || dec.Message != "Супер: игра" {
		t.Errorf("плашка о решении %q, сообщение %q", got, dec.Message)
	}
	if p, _, _ := h.store.GetProgram(pathSuperGame); p.CustomName != "Супер" || p.Name != "SuperGame" {
		t.Errorf("решение по ждущей стёрло своё название или записало его как название из файла: %+v", p)
	}
}

func (h *harness) list(tok string) ipc.ProgramsReply {
	h.t.Helper()
	var rep ipc.ProgramsReply
	h.ok(adminPeer(), tok, ipc.CmdListPrograms, nil, &rep)
	return rep
}

func (h *harness) decideCat(tok, path, cat string) ipc.DecideProgramReply {
	h.t.Helper()
	var rep ipc.DecideProgramReply
	h.ok(adminPeer(), tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: path, Category: cat}, &rep)
	return rep
}

func lastOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

func plateText(ps []ipc.BlockedPlate, kind string) string {
	for _, p := range ps {
		if p.Kind == kind {
			return p.Text
		}
	}
	return ""
}

func firstName(ps []PendingProgram) string {
	if len(ps) == 0 {
		return ""
	}
	return ps[0].Name
}

// Ловит: строки журнала о переименовании не по ТЗ: не на «вы», сброс не говорит, что вернулось
// название из файла.
func TestRenameEventTexts(t *testing.T) {
	cases := []struct {
		d    map[string]any
		want string
	}{
		{map[string]any{"from": "Stalker2-Win64-Shipping", "to": "Сталкер 2"}, "Вы переименовали «Stalker2-Win64-Shipping» в «Сталкер 2»"},
		{map[string]any{"from": "Сталкер 2", "to": "Stalker2-Win64-Shipping", "reset": true}, "Вы вернули «Сталкер 2» название из файла «Stalker2-Win64-Shipping»"},
	}
	for _, c := range cases {
		if got := eventText("program_renamed", "", c.d); got != c.want {
			t.Errorf("eventText(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
