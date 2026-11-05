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

// settle — проходит срок уточнения автозапуска (launchSettle): наблюдатель записывает отложенное.
func (h *harness) settle() {
	h.t.Helper()
	h.real.Advance(launchSettle)
	h.watch()
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

// forbiddenPlates — плашки «запрещена родителем» и «Этот браузер запрещён» (chrome, RM4-8) в
// ответе стража.
func forbiddenPlates(rep ipc.GuardPollReply) []ipc.BlockedPlate {
	out := []ipc.BlockedPlate{}
	for _, p := range platesOf(rep) {
		if p.Kind == ipc.BlockedForbidden || p.Kind == ipc.BlockedBrowser {
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
	h.settle()
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
	h.settle()
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
			want := []ipc.BlockedPlate{{Kind: ipc.BlockedBrowser, Text: "Этот браузер запрещён. Пользуйся Brave", ShowS: 10, Path: pathChrome}}
			if got := forbiddenPlates(h.poll(inputSample(300, k.fg))); jsonOf(t, got) != jsonOf(t, want) {
				t.Errorf("плашка ручного запуска: %s, want %s", jsonOf(t, got), jsonOf(t, want))
			}
			if got := h.eventTexts(evProgramForbiddenTry); jsonOf(t, got) != jsonOf(t, []string{"Миша: попытка открыть запрещённое: «chrome»"}) {
				t.Errorf("нарушение ручного запуска: %v", got)
			}
			h.settle()
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

// Ловит (R13, D-65, ревью FB-18): в тестовом режиме страж присылает простой в часах ядра (ScaleIdle,
// ×60), а служба сравнивает его с настоящими часами: ввод 3 с назад выглядит как 3 мин назад, и
// ручной запуск в приёмке считается автоматическим.
func TestWatchManualWithClockSpeed(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withTestMode(TestMode{WatchDir: `c:\program files\google\chrome`, ClockSpeed: 60}))
	h.setCategory(pathChrome, "blocked")
	explorer := w.start(childSession, pathExplorer)
	h.real.Advance(manualLogonGrace)
	h.poll(inputSample(3000*60, pathExplorer)) // 3 настоящие секунды простоя в часах ядра
	p := w.startFrom(childSession, pathChrome, explorer)
	h.watch()
	if w.alive(p) {
		t.Fatal("ручной запуск при ускорении часов: chrome не закрыта")
	}
	if got := forbiddenPlates(h.poll(inputSample(0, pathExplorer))); len(got) != 1 {
		t.Errorf("ручной запуск при ускорении 60 (ввод 3 с назад): плашек «запрещена» %d, want 1", len(got))
	}
	// Простой 30 настоящих секунд и при ускорении остаётся простоем.
	h.real.Advance(time.Minute)
	h.poll(inputSample(30000*60, pathExplorer))
	w.startFrom(childSession, pathChrome, explorer)
	h.watch()
	h.settle()
	if n, m := len(h.store.eventsOf(evProgramForbiddenTry)), len(h.store.eventsOf(evProgramAutoClosed)); n != 1 || m != 1 {
		t.Errorf("нарушений %d, «запускалась сама» %d; want 1 и 1 (второй запуск после 30 с простоя)", n, m)
	}
}

// Ловит (R13, D-65, ревью FB-18): ребёнок вручную запускает многопроцессную запрещённую программу
// (браузер, Discord): дочерние процессы того же exe считаются «перезапустила сама себя», а в
// следующем проходе (главный уже закрыт) — «родителя нет», и в журнале рядом с нарушением
// появляется «запускалась сама». Порядок процессов в списке не важен.
func TestWatchForbiddenMultiProcessManual(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		w := newWatcher()
		h, explorer := watchHarness(t, w), ProcInfo{}
		h.setCategory(pathChrome, "blocked")
		explorer = h.manualReady(w)
		main := w.startFrom(childSession, pathChrome, explorer)
		renderer := w.startFrom(childSession, pathChrome, main)
		if reversed {
			w.mu.Lock()
			for i, j := 0, len(w.procs)-1; i < j; i, j = i+1, j-1 {
				w.procs[i], w.procs[j] = w.procs[j], w.procs[i]
			}
			w.mu.Unlock()
		}
		h.watch()
		if w.alive(main) || w.alive(renderer) {
			t.Fatalf("обратный порядок %v: главный жив %v, дочерний жив %v", reversed, w.alive(main), w.alive(renderer))
		}
		if got := forbiddenPlates(h.poll(activeSample)); len(got) != 1 {
			t.Errorf("обратный порядок %v: плашек «запрещена» %d, want 1", reversed, len(got))
		}
		h.svc.watch.mu.Lock()
		e := h.svc.watch.procs[procKey{pid: renderer.PID, stamp: renderer.Stamp}]
		h.svc.watch.mu.Unlock()
		if e == nil || !e.manual || !e.quiet {
			t.Errorf("обратный порядок %v: дочерний процесс того же exe: %+v; want ручной от главного, записи у главного (quiet)", reversed, e)
		}
		// Следующий проход: ещё один процесс того же exe, его родитель уже закрыт.
		h.real.Advance(time.Second)
		h.poll(activeSample)
		w.startFrom(childSession, pathChrome, renderer)
		h.watch()
		h.settle()
		if n, m := len(h.store.eventsOf(evProgramForbiddenTry)), len(h.store.eventsOf(evProgramAutoClosed)); n != 1 || m != 0 {
			t.Errorf("обратный порядок %v: нарушений %d, «запускалась сама» %d; want 1 и 0", reversed, n, m)
		}
	}
}

// Ловит (R13, D-65, ревью FB-18): решение по снимку стража, сделанному до запуска: ребёнок был
// без ввода, нажал на ярлык, и программа запустилась раньше, чем страж прислал следующий снимок
// (раз в секунду), — ручной запуск записан как «сама», без плашки. Следующий снимок без ввода
// или с вводом только после того, как процесс увиден (он не мог вызвать запуск), оставляет «сама».
func TestWatchForbiddenDecidedByNextSample(t *testing.T) {
	for _, k := range []struct {
		name    string
		nextAgo int64
		plates  int
		tries   int
		autos   int
	}{
		{"щелчок до запуска виден в следующем снимке: ручной", 700, 1, 1, 0},
		{"следующий снимок без ввода: сама", 31000, 0, 0, 1},
		{"ввод только после запуска: сама", 100, 0, 0, 1},
	} {
		t.Run(k.name, func(t *testing.T) {
			h, w, explorer := autoHarness(t)
			h.real.Advance(manualLogonGrace)
			h.poll(inputSample(30000, pathExplorer)) // снимок до щелчка: ребёнок 30 с без ввода
			h.real.Advance(500 * time.Millisecond)
			p := w.startFrom(childSession, pathChrome, explorer)
			h.watch()
			if w.alive(p) {
				t.Fatal("chrome не закрыта сразу (решение о плашке ждёт снимка, закрытие — нет)")
			}
			h.real.Advance(500 * time.Millisecond)
			plates := len(forbiddenPlates(h.poll(inputSample(k.nextAgo, pathExplorer))))
			h.watch()
			plates += len(forbiddenPlates(h.poll(inputSample(k.nextAgo, pathExplorer))))
			h.settle()
			plates += len(forbiddenPlates(h.poll(inputSample(k.nextAgo, pathExplorer))))
			if n, m := len(h.store.eventsOf(evProgramForbiddenTry)), len(h.store.eventsOf(evProgramAutoClosed)); plates != k.plates || n != k.tries || m != k.autos {
				t.Errorf("плашек %d, нарушений %d, «сама» %d; want %d, %d, %d", plates, n, m, k.plates, k.tries, k.autos)
			}
		})
	}
}

// Ловит (R13, ревью FB-18): процесс, запущенный, пока программа была разрешена, после запрета
// родителем закрывается молча как «запускалась сама», а не с плашкой «запрещена родителем».
func TestWatchForbiddenAfterStartWarns(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	h.setCategory(pathChrome, "other") // родитель разрешил chrome (сильнее каталога, RM4-8)
	updater := w.start(childSession, pathBraveUpd)
	h.real.Advance(2 * time.Minute)
	h.poll(inputSample(30000, ""))
	p := w.startFrom(childSession, pathChrome, updater) // запуск «сам», но chrome ещё разрешена
	h.watch()
	if !w.alive(p) {
		t.Fatal("разрешённая chrome закрыта")
	}
	h.setCategory(pathChrome, "blocked")
	h.real.Advance(time.Second)
	h.poll(inputSample(30000, ""))
	h.watch()
	if w.alive(p) {
		t.Fatal("после запрета chrome не закрыта")
	}
	if got := forbiddenPlates(h.poll(inputSample(30000, ""))); len(got) != 1 {
		t.Errorf("плашек «запрещена» после запрета запущенной: %d, want 1", len(got))
	}
	h.settle()
	if n, m := len(h.store.eventsOf(evProgramForbiddenTry)), len(h.store.eventsOf(evProgramAutoClosed)); n != 1 || m != 0 {
		t.Errorf("нарушений %d, «сама» %d; want 1 и 0", n, m)
	}
}

// Ловит (R13, D-65, ревью FB-18): приложение Магазина (Program Files\WindowsApps) Windows запускает
// через svchost вне сеанса ребёнка: родителя в сеансе нет, и ручной запуск запрещённой игры
// Магазина считается «сама».
func TestWatchForbiddenStoreAppManual(t *testing.T) {
	const roblox = `c:\program files\windowsapps\robloxcorporation.robloxapp_2.6_x64__55nm5eh3cm0pr\windows10universal.exe`
	sc := typicalScan()
	sc.list = append(sc.list, FoundProgram{Path: roblox, Name: "Roblox"})
	w := newWatcher()
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc), withWatcher(w))
	h.start()
	h.childLogsIn()
	h.setCategory(roblox, "blocked")
	h.manualReady(w)
	p := w.start(childSession, roblox) // родителя в сеансе нет
	h.watch()
	if w.alive(p) {
		t.Fatal("запрещённая игра Магазина не закрыта")
	}
	want := []ipc.BlockedPlate{{Kind: ipc.BlockedForbidden, Text: "Программа «Roblox» запрещена родителем", ShowS: 10, Path: roblox}}
	if got := forbiddenPlates(h.poll(activeSample)); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("плашка игры Магазина: %s, want %s", jsonOf(t, got), jsonOf(t, want))
	}
}
