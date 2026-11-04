package service

import (
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Тесты наблюдателя: запрещённая программа, запустившаяся сама (R13, D-65, FB-18, S45).

// inputSample — снимок стража: ребёнок за компьютером, последний ввод agoMs назад, на переднем
// плане программа fg («» — не определена).
func inputSample(agoMs int64, fg string) ipc.SensorSample {
	return ipc.SensorSample{SessionActive: true, LastInputAgoMs: agoMs, Foreground: ipc.CatNameOther, ForegroundPath: fg}
}

// manualReady — ребёнок вошёл больше минуты назад и только что двигал мышью, на переднем плане
// Проводник: запуски из него ручные (R13, D-65). Возвращает процесс Проводника.
func (h *harness) manualReady(w *fakeWatcher) ProcInfo {
	h.t.Helper()
	explorer := w.start(childSession, pathExplorer)
	h.real.Advance(manualLogonGrace)
	h.poll(inputSample(0, pathExplorer))
	return explorer
}

// startManual запускает по процессу каждой программы из Проводника explorer (ручной запуск).
func startManual(w *fakeWatcher, explorer ProcInfo, paths ...string) map[string]ProcInfo {
	out := map[string]ProcInfo{}
	for _, p := range paths {
		out[p] = w.startFrom(childSession, p, explorer)
	}
	return out
}

// autoHarness — служба с Мишей и наблюдателем; Проводник сеанса запущен при входе, chrome запрещена.
func autoHarness(t *testing.T) (*harness, *fakeWatcher, ProcInfo) {
	t.Helper()
	w := newWatcher()
	h := watchHarness(t, w)
	explorer := w.start(childSession, pathExplorer)
	h.setCategory(pathChrome, "blocked")
	return h, w, explorer
}

// forbiddenPlates — плашки «запрещена родителем» в ответе стража.
func forbiddenPlates(rep ipc.GuardPollReply) []ipc.BlockedPlate {
	out := []ipc.BlockedPlate{}
	for _, p := range platesOf(rep) {
		if p.Kind == ipc.BlockedForbidden {
			out = append(out, p)
		}
	}
	return out
}

// Ловит (R13, D-65, FB-18, S45 (а)): запрещённая программа, запущенная автозагрузкой при входе
// и потом обновлятором или Steam на переднем плане, пока ребёнок ничего не нажимал, даёт ребёнку плашку «запрещена родителем»
// и нарушение «попытка открыть запрещённое»; не закрывается (молчание вместо закрытия); журнал
// пишет по записи на каждый автозапуск (заказчик выбрал «одна запись в день») или не пишет
// вовсе; на следующий день записи нет.
func TestWatchForbiddenAutostartSilent(t *testing.T) {
	h, w, explorer := autoHarness(t)
	updater := w.start(childSession, pathBraveUpd)

	// Автозагрузка: Проводник через 5 с после входа, ребёнок двигает мышь (ввод 1 с назад).
	h.real.Advance(5 * time.Second)
	h.poll(inputSample(1000, pathExplorer))
	first := w.startFrom(childSession, pathChrome, explorer)
	h.watch()
	if w.alive(first) {
		t.Fatal("автозапуск из Проводника в первые 60 с: chrome не закрыта")
	}
	if got := forbiddenPlates(h.poll(inputSample(1000, pathExplorer))); len(got) != 0 {
		t.Errorf("автозапуск при входе: плашки %s, want нет", jsonOf(t, got))
	}

	// Через 2 мин её запускает обновлятор не на переднем плане; ребёнок 30 с ничего не нажимал.
	h.real.Advance(2 * time.Minute)
	h.poll(inputSample(30000, pathGeoGebra))
	second := w.startFrom(childSession, pathChrome, updater)
	h.watch()
	if w.alive(second) {
		t.Fatal("автозапуск обновлятором: chrome не закрыта")
	}
	if got := forbiddenPlates(h.poll(inputSample(30000, pathGeoGebra))); len(got) != 0 {
		t.Errorf("автозапуск обновлятором: плашки %s, want нет", jsonOf(t, got))
	}
	// Steam на переднем плане сам запускает её (ребёнок 30 с ничего не нажимал): тоже сама.
	steam := w.start(childSession, pathSteam)
	h.real.Advance(time.Minute)
	h.poll(inputSample(30000, pathSteam))
	third := w.startFrom(childSession, pathChrome, steam)
	h.watch()
	if w.alive(third) {
		t.Fatal("автозапуск из Steam без ввода: chrome не закрыта")
	}
	if got := forbiddenPlates(h.poll(inputSample(30000, pathSteam))); len(got) != 0 {
		t.Errorf("автозапуск из Steam на переднем плане без ввода: плашки %s, want нет", jsonOf(t, got))
	}
	if got := h.eventTexts(evProgramForbiddenTry); len(got) != 0 {
		t.Errorf("нарушения «попытка открыть запрещённое» при автозапуске: %v, want нет", got)
	}
	want := []string{"Миша: программа «chrome» запускалась сама и закрыта"}
	if got := h.eventTexts(evProgramAutoClosed); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("журнал за три автозапуска в один день: %v, want %v", got, want)
	}
	if ev := h.store.eventsOf(evProgramAutoClosed); len(ev) == 1 && (ev[0].Details["path"] != pathChrome || ev[0].ChildID != h.childID(childSID)) {
		t.Errorf("подробности записи: ребёнок %d, %v; want Миша и путь %s", ev[0].ChildID, ev[0].Details, pathChrome)
	}

	// На следующий день учёта — новая запись.
	h.clock.Set(mon1500.Add(20 * time.Hour)) // вторник 11:00
	h.svc.Tick(h.clock.Now())
	h.real.Advance(time.Minute)
	h.poll(inputSample(30000, ""))
	w.startFrom(childSession, pathChrome, updater)
	h.watch()
	if n := len(h.store.eventsOf(evProgramAutoClosed)); n != 2 {
		t.Errorf("записей «запускалась сама» после смены дня: %d, want 2", n)
	}
	if n := len(h.store.eventsOf(evProgramForbiddenTry)); n != 0 {
		t.Errorf("нарушений после смены дня: %d, want 0", n)
	}
}

// Ловит (R13, D-65, S45 (б)): ручной запуск запрещённой программы из Проводника (через минуту
// после входа, ребёнок только что нажал клавишу) или из программы на переднем плане («Играть» в
// Steam) больше не даёт плашку и нарушение; ручной запуск пишется как «запускалась сама».
func TestWatchForbiddenManualStillWarns(t *testing.T) {
	cases := []struct {
		name   string
		parent func(w *fakeWatcher, explorer ProcInfo) ProcInfo
		fg     string
	}{
		{"из Проводника", func(w *fakeWatcher, explorer ProcInfo) ProcInfo { return explorer }, pathExplorer},
		{"из Steam на переднем плане", func(w *fakeWatcher, _ ProcInfo) ProcInfo { return w.start(childSession, pathSteam) }, pathSteam},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			h, w, explorer := autoHarness(t)
			parent := k.parent(w, explorer)
			h.real.Advance(manualLogonGrace + time.Second)
			h.poll(inputSample(300, k.fg))
			p := w.startFrom(childSession, pathChrome, parent)
			h.watch()
			if w.alive(p) {
				t.Fatal("ручной запуск: chrome не закрыта")
			}
			want := []ipc.BlockedPlate{{Kind: ipc.BlockedForbidden, Text: "Программа «chrome» запрещена родителем", ShowS: 10, Path: pathChrome}}
			if got := forbiddenPlates(h.poll(inputSample(300, k.fg))); jsonOf(t, got) != jsonOf(t, want) {
				t.Errorf("плашка ручного запуска: %s, want %s", jsonOf(t, got), jsonOf(t, want))
			}
			if got := h.eventTexts(evProgramForbiddenTry); jsonOf(t, got) != jsonOf(t, []string{"Миша: попытка открыть запрещённое: «chrome»"}) {
				t.Errorf("нарушение ручного запуска: %v", got)
			}
			if n := len(h.store.eventsOf(evProgramAutoClosed)); n != 0 {
				t.Errorf("ручной запуск записан как «запускалась сама»: %d записей", n)
			}
		})
	}
}

// Ловит (R13, D-65): правило «сама или вручную» задело не только запрещённые: незнакомая программа
// при автозапуске перестала попадать в «Ждут решения» с плашкой, игра при кончившихся играх —
// закрываться с записью в журнал.
func TestWatchAutostartOnlyForForbidden(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(120, 0, "lock"))
	updater := w.start(childSession, pathBraveUpd)
	h.real.Advance(2 * time.Minute)
	h.poll(inputSample(30000, ""))
	w.startFrom(childSession, pathSuperGame, updater)
	w.startFrom(childSession, pathMinecraft, updater)
	h.watch()
	got := platesOf(h.poll(inputSample(30000, "")))
	if len(got) != 2 || got[0].Kind != ipc.BlockedGamesOver || got[1].Kind != ipc.BlockedUnknown {
		t.Errorf("плашки незнакомой и игры при автозапуске: %s, want «игры кончились» и «ждёт разрешения»", jsonOf(t, got))
	}
	if n, m := len(h.store.eventsOf(evProgramPending)), len(h.store.eventsOf(evProgramClosedGames)); n != 1 || m != 1 {
		t.Errorf("журнал: «ждёт решения» %d, «игры кончились» %d; want по 1", n, m)
	}
	if n := len(h.store.eventsOf(evProgramAutoClosed)); n != 0 {
		t.Errorf("«запускалась сама» у незапрещённых: %d записей, want 0", n)
	}
}
