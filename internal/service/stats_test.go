package service

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Вкладки «Итогов» (FB-10, D-58, S39): время каждой программы за сегодня в ответе MyToday и
// ParentStats; сумма строк категории сходится с итогом категории (класс A).

// sumOf — секунды строк категории cat («» — все).
func sumOf(rows []ipc.ProgramTime, cat string) int {
	n := 0
	for _, r := range rows {
		if cat == "" || r.Category == cat {
			n += r.Seconds
		}
	}
	return n
}

// rowsOf — строки категории cat как «название=секунды» по порядку ответа.
func rowsOf(rows []ipc.ProgramTime, cat string) []string {
	out := []string{}
	for _, r := range rows {
		if r.Category == cat {
			out = append(out, r.Name+"="+strconv.Itoa(r.Seconds))
		}
	}
	return out
}

// Ловит: в ответе нет времени программ; секунды, ещё не сохранённые в базу (накопитель службы
// сбрасывается раз в 30 с), не попадают в список, и сумма игр меньше итога «Игры»; список не по
// убыванию; имя не то, что в списке программ и журнале (своё название, D-59); время рабочего
// стола и системных программ («Прочее» без программы, дотяжка последней минуты) пропадает, а не
// идёт строкой «Другое»; скрытая программа (D-57) видна по имени; родитель видит другое, чем ребёнок.
func TestMyTodayProgramsByCategory(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withStalker())
	h.start()
	h.childLogsIn()
	h.setCategory(pathGeoGebra, "study")
	tok := h.login(adminPeer())
	h.rename(tok, pathShipping, "Сталкер")
	h.run(50, fgSample(pathShipping, ipc.CatNameOther))
	h.run(25, fgSample(pathLauncher, ipc.CatNameOther))
	h.run(15, fgSample(pathGeoGebra, ipc.CatNameOther))
	h.run(10, fgSample(pathBrave, ipc.CatNameOther))
	h.run(5, fgSample(pathNotepad, ipc.CatNameOther)) // система: «Прочее» без программы
	h.run(15, fgSample("", ipc.CatNameOther))         // рабочий стол
	if n := len(h.svc.children[h.childID(childSID)].usage); n == 0 {
		t.Fatalf("накопитель службы пуст: проверка несохранённых секунд ничего не проверяет")
	}

	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if my.GamesS != 75 || my.StudyS != 15 || my.OtherS != 30 || my.AllS != 120 {
		t.Fatalf("итоги: игры %d, учёба %d, прочее %d, всего %d; want 75, 15, 30, 120", my.GamesS, my.StudyS, my.OtherS, my.AllS)
	}
	want := map[string][]string{
		"games": {"Сталкер=50", "Stalker2=25"},
		"study": {"GeoGebra Classic=15"},
		"other": {"Brave=10", "Другое=20"}, // «Другое» последней, хотя дольше Brave
	}
	for cat, w := range want {
		if got := rowsOf(my.Programs, cat); strings.Join(got, ", ") != strings.Join(w, ", ") {
			t.Errorf("%s: %v, want %v", cat, got, w)
		}
	}
	if len(my.Programs) != 5 {
		t.Errorf("строк %d, want 5: %+v", len(my.Programs), my.Programs)
	}
	if s := sumOf(my.Programs, "games"); s != my.GamesS {
		t.Errorf("сумма игр %d, итог «Игры» %d", s, my.GamesS)
	}
	if s := sumOf(my.Programs, ""); s != my.AllS {
		t.Errorf("сумма всех строк %d, итог «Всего» %d", s, my.AllS)
	}
	for _, r := range my.Programs {
		if (r.Name == "Другое") != (r.Path == "") {
			t.Errorf("строка без программы не «Другое» или программа без пути: %+v", r)
		}
	}

	var ps ipc.MyTodayReply
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: h.childID(childSID)}, &ps)
	if jsonOf(t, ps.Programs) != jsonOf(t, my.Programs) || ps.AllS != my.AllS || ps.OtherS != my.OtherS {
		t.Errorf("родитель видит другое:\n%s\nребёнок:\n%s", jsonOf(t, ps.Programs), jsonOf(t, my.Programs))
	}

	// Скрыли запускалку после игры: её имени нет, её секунды в играх остаются строкой «Другое».
	h.hide(tok, true, pathLauncher)
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if got := strings.Join(rowsOf(my.Programs, "games"), ", "); got != "Сталкер=50, Другое=25" {
		t.Errorf("игры после скрытия Stalker2: %s, want «Сталкер=50, Другое=25»", got)
	}
	for _, r := range my.Programs {
		if r.Path == pathLauncher || r.Name == "Stalker2" {
			t.Errorf("скрытая видна: %+v", r)
		}
	}
	if s := sumOf(my.Programs, "games"); s != my.GamesS {
		t.Errorf("после скрытия сумма игр %d, итог «Игры» %d", s, my.GamesS)
	}
}

// Ловит: список сегодня содержит вчерашние программы (после полуночи до тика ядра), у чужого
// пользователя или без программ — null вместо [].
func TestMyTodayProgramsEmptyAndYesterday(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()))
	h.start()
	h.childLogsIn()
	rep := h.do(childPeer(), "", ipc.CmdMyToday, nil)
	if !strings.Contains(string(rep.Payload), `"programs":[]`) {
		t.Errorf("без программ: %s", rep.Payload)
	}
	rep = h.do(otherPeer(), "", ipc.CmdMyToday, nil)
	if !strings.Contains(string(rep.Payload), `"programs":[]`) {
		t.Errorf("чужой пользователь: %s", rep.Payload)
	}
	h.run(40, fgSample(pathMinecraft, ipc.CatNameOther))
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if got := strings.Join(rowsOf(my.Programs, "games"), ", "); got != "Minecraft Launcher=40" {
		t.Fatalf("игры: %s", got)
	}
	h.clock.Advance(10 * time.Hour) // полночь прошла, тика ещё не было
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if len(my.Programs) != 0 || my.AllS != 0 || my.OtherS != 0 {
		t.Errorf("после полуночи: %+v, всего %d, прочее %d", my.Programs, my.AllS, my.OtherS)
	}
}
