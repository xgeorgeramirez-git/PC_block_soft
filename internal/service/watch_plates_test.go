package service

import (
	"sort"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// platesOf — плашки о закрытых программах из ответа стража.
func platesOf(rep ipc.GuardPollReply) []ipc.BlockedPlate {
	out := []ipc.BlockedPlate{}
	for _, c := range rep.Commands {
		if c.Kind == ipc.GuardCmdBlocked && c.Plate != nil {
			out = append(out, *c.Plate)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// eventTexts — строки журнала родителя по событиям вида kind (как их видит родитель).
func (h *harness) eventTexts(kind string) []string {
	out := []string{}
	for _, e := range h.store.eventsOf(kind) {
		out = append(out, eventText(e.Kind, "Миша", e.Details))
	}
	return out
}

// Ловит (FR-13, 8.2.4, R22, S-5, S-7, PL-16): плашка ребёнку о закрытой программе не приходит,
// текст не тот или не на «ты», «Попросить сейчас» не только у «ждёт разрешения»; повторный запуск
// через секунду даёт вторую плашку и вторую запись журнала; ограничитель прячет первое событие
// или не пропускает событие после срока; попытки в «Ждут решения» не растут.
func TestWatchPlatesAndJournal(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(120, 0, "lock"))
	h.setCategory(pathChrome, "blocked")
	startAll(w, pathSuperGame, pathChrome, pathMinecraft)
	h.watch()
	want := []ipc.BlockedPlate{
		{Kind: ipc.BlockedForbidden, Text: "Программа «chrome» запрещена родителем", ShowS: 10, Path: pathChrome},
		{Kind: ipc.BlockedGamesOver, Text: "Время на игры на сегодня закончилось", ShowS: 10, Path: pathMinecraft},
		{Kind: ipc.BlockedUnknown, Text: "Программа «supergame» ждёт разрешения родителя", ShowS: 15, Path: pathSuperGame, CanAsk: true},
	}
	if got := platesOf(h.poll(activeSample)); jsonOf(t, got) != jsonOf(t, want) {
		t.Fatalf("плашки:\n got %s\nwant %s", jsonOf(t, got), jsonOf(t, want))
	}
	journal := map[string][]string{
		evProgramPending:      {"Миша: программа «supergame» ждёт решения"},
		evProgramForbiddenTry: {"Миша: попытка открыть запрещённое: «chrome»"},
		evProgramClosedGames:  {"Миша: закрыта «Minecraft Launcher»: время на игры закончилось"},
	}
	for kind, texts := range journal {
		if got := h.eventTexts(kind); jsonOf(t, got) != jsonOf(t, texts) {
			t.Errorf("журнал %s: %v, want %v", kind, got, texts)
		}
	}
	if ev := h.store.eventsOf(evProgramClosedGames); ev[0].Details["text"] != core.GamesDeniedText || ev[0].Details["path"] != pathMinecraft {
		t.Errorf("подробности закрытия игры: %v", ev[0].Details)
	}

	// Через секунду ребёнок запускает всё снова: закрыто, попытка записана, но ни плашки, ни журнала.
	h.real.Advance(time.Second)
	startAll(w, pathSuperGame, pathChrome, pathMinecraft)
	h.watch()
	if got := platesOf(h.poll(activeSample)); len(got) != 0 {
		t.Errorf("повтор через 1 с: плашки %s, want нет", jsonOf(t, got))
	}
	if pend, _ := h.store.ListPending(); len(pend) != 1 || pend[0].Attempts != 2 {
		t.Errorf("«Ждут решения» после второй попытки: %+v, want 2 попытки", pend)
	}
	// Через 6 с — снова плашки, журнал ещё молчит (60 с).
	h.real.Advance(6 * time.Second)
	startAll(w, pathSuperGame, pathChrome, pathMinecraft)
	h.watch()
	if got := platesOf(h.poll(activeSample)); len(got) != 3 {
		t.Errorf("повтор через 7 с: плашек %d, want 3", len(got))
	}
	if n := len(h.store.eventsOf(evProgramForbiddenTry)); n != 1 {
		t.Errorf("журнал через 7 с: запрещённых попыток %d, want 1", n)
	}
	// Через 60 с после первой записи — журнал снова пишет.
	h.real.Advance(watchRepeatEvery)
	startAll(w, pathSuperGame, pathChrome, pathMinecraft)
	h.watch()
	for kind := range journal {
		if n := len(h.store.eventsOf(kind)); n != 2 {
			t.Errorf("журнал %s через минуту: %d записей, want 2", kind, n)
		}
	}
	if n := len(w.killedPaths()); n != 12 {
		t.Errorf("закрыто %d процессов, want 12 (каждый запуск)", n)
	}
}

// Ловит (R8, S-5): в режиме «только учёба» плашка и журнал говорят не про учёбу; ребёнку в
// отключённом сеансе (смена пользователя) приходит плашка, а программа не закрывается.
func TestWatchStudyOnlyAndDisconnected(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(0, -1, "study_only"))
	w.start(childSession, pathBrave)
	h.watch()
	want := []ipc.BlockedPlate{{Kind: ipc.BlockedStudyOnly, Text: core.StudyOnlyPlateText, ShowS: 10, Path: pathBrave}}
	if got := platesOf(h.poll(activeSample)); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("плашка «только учёба»: %s, want %s", jsonOf(t, got), jsonOf(t, want))
	}
	if got := h.eventTexts(evProgramClosedStudyOnly); jsonOf(t, got) != jsonOf(t, []string{"Миша: закрыта «Brave»: время вышло, доступна только учёба"}) {
		t.Errorf("журнал «только учёба»: %v", got)
	}
	h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
	h.real.Advance(10 * time.Second)
	game := w.start(childSession, pathMinecraft)
	h.watch()
	h.svc.mu.Lock()
	queued := len(h.svc.guardCmd[childSession])
	h.svc.mu.Unlock()
	if w.alive(game) || queued != 0 {
		t.Errorf("отключённый сеанс: игра жива %v, команд стражу %d; want закрыта, 0", w.alive(game), queued)
	}
}

// Ловит (R5, R9, D-54, S-6): игровые предупреждения 30/15/5/1 не доходят до стража (очередь
// предупреждений) или до значка (остаток игр в статусе); порог повторяется или пропущен.
func TestGamesWarningsReachGuardAndTray(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withRules(120, 31, "lock"))
	h.start()
	h.childLogsIn()
	type got struct {
		threshold, remaining int
	}
	var warns []got
	for i := 0; i < 31*60+2; i++ {
		rep := h.step(fgSample(pathMinecraft, ipc.CatNameOther))
		for _, w := range rep.Warnings {
			if w.Event != "games" || w.Text == "" || !w.Sound {
				t.Fatalf("предупреждение %+v: want игровое с текстом и звуком", w)
			}
			warns = append(warns, got{w.Threshold, h.status().GamesRemainingS})
		}
	}
	// Предупреждение уходит стражу на следующем опросе: остаток на секунду меньше порога.
	want := []got{{30, 30*60 - 1}, {15, 15*60 - 1}, {5, 5*60 - 1}, {1, 60 - 1}}
	if jsonOf(t, warns) != jsonOf(t, want) {
		t.Errorf("игровые предупреждения %v, want %v", warns, want)
	}
	if st := h.status(); !st.CloseGames || st.GamesRemainingS != 0 {
		t.Errorf("после лимита игр: статус %+v, want игры закрыты, остаток 0", st)
	}
}

// Ловит (RM3-8, S-8): в тестовом режиме наблюдатель закрывает процессы вне папки WatchDir (на
// раннере он завершил бы всё незнакомое и игры) или папку с похожим именем; без WatchDir — что-либо.
func TestWatchTestModeOnlyWatchDir(t *testing.T) {
	const dir = `c:\users\миша\downloads\pcb`
	w := newWatcher()
	h := watchHarness(t, w, withTestMode(TestMode{WatchDir: dir}), withRules(120, 0, "lock"))
	startAll(w, dir+`\supergame.exe`, pathSuperGame, pathMinecraft, `c:\users\миша\downloads\pcbx\a.exe`)
	h.watch()
	if got := w.killedPaths(); jsonOf(t, got) != jsonOf(t, []string{dir + `\supergame.exe`}) {
		t.Errorf("тестовый режим: закрыты %v, want только программа из WatchDir", got)
	}

	w2 := newWatcher()
	h2 := watchHarness(t, w2, withTestMode(TestMode{}))
	w2.start(childSession, pathSuperGame)
	h2.watch()
	if n := w2.listCount(); n != 0 || len(w2.killedPaths()) != 0 {
		t.Errorf("тестовый режим без WatchDir: опросов %d, закрыто %v; want ничего", n, w2.killedPaths())
	}
}
