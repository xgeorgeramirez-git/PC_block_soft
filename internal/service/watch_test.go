package service

import (
	"context"
	"sort"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Пути процессов в тестах наблюдателя (кроме путей programs_test.go).
const (
	pathNotepad  = `c:\windows\system32\notepad.exe`
	pathPCBlock  = `c:\program files\pcblock\pcblock.exe`
	pathPCGuard  = `c:\program files\pcblock\pcblock-guard.exe`
	pathWebView  = `c:\program files (x86)\microsoft\edgewebview\application\1.2.3\msedgewebview2.exe`
	pathDevBuild = `c:\users\миша\source\pcblock\pcblock.exe`
)

// withWatcher подключает наблюдатель процессов до New.
func withWatcher(w *fakeWatcher) hopt {
	return func(h *harness) { h.cfg.Watcher = w }
}

// withRules задаёт ребёнку childSID лимиты на все дни: общий total минут (<0 — без лимита), игры
// games минут (<0 — без отдельного лимита) и режим «Когда время вышло» (до Start).
func withRules(total, games int, mode string) hopt {
	return func(h *harness) {
		id := h.childID(childSID)
		var week [7]core.DayLimit
		for i := range week {
			if total >= 0 {
				t := total
				week[i].TotalMin = &t
			}
			if games >= 0 {
				g := games
				week[i].GamesMin = &g
			}
		}
		if err := h.store.SetDayLimits(id, week); err != nil {
			h.t.Fatal(err)
		}
		if err := h.store.SetEndMode(id, mode); err != nil {
			h.t.Fatal(err)
		}
	}
}

// watchHarness — служба с ребёнком Мишей (120 мин), снимком программ и наблюдателем; Миша вошёл
// в сеанс 2. opts применяются после ребёнка (withRules и т.п.).
func watchHarness(t *testing.T, w *fakeWatcher, opts ...hopt) *harness {
	t.Helper()
	all := append([]hopt{configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withWatcher(w)}, opts...)
	h := newHarness(t, all...)
	h.start()
	h.childLogsIn()
	return h
}

// watch — один проход наблюдателя в текущий настоящий момент.
func (h *harness) watch() { h.svc.Watch(h.real.Now()) }

// setCategory — решение родителя о категории программы прямо в хранилище и перезагрузка списка.
func (h *harness) setCategory(path, cat string) {
	h.t.Helper()
	if _, err := h.store.SetProgramsCategory([]string{path}, cat, 1); err != nil {
		h.t.Fatal(err)
	}
	h.svc.mu.Lock()
	h.svc.reloadPrograms()
	h.svc.mu.Unlock()
}

// Ловит (RM3-11, NFR-6): наблюдатель живёт на тике учёта (в тестовом ускорении опрашивал бы
// процессы тысячу раз в секунду, а при обычном темпе — раз в секунду, дольше NFR-6) или вообще
// не запущен в RunLoop; незнакомая программа закрывается позже одного периода опроса.
func TestRunLoopWatchTickerOwnPeriod(t *testing.T) {
	for _, speed := range []int{1, 1000} {
		w := newWatcher()
		h := watchHarness(t, w, withTestMode(TestMode{ClockSpeed: speed, WatchDir: `c:\users\миша\downloads`}))
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { h.svc.RunLoop(ctx); close(done) }()
		began := time.Now()
		game := w.start(childSession, pathSuperGame)
		deadline := time.Now().Add(2 * time.Second)
		for w.alive(game) && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		took := time.Since(began)
		time.Sleep(time.Second - took)
		cancel()
		<-done
		n := w.listCount()
		if w.alive(game) {
			t.Fatalf("ускорение %d: незнакомая программа не закрыта за 2 с", speed)
		}
		// Один период опроса (250 мс) плюс запас на планировщик тестовой машины.
		if took > watchEvery+250*time.Millisecond {
			t.Errorf("ускорение %d: программа закрыта через %v, want не позже %v + запас", speed, took, watchEvery)
		}
		// За ~1 с при периоде 250 мс — около 4 опросов, при любом ускорении часов.
		if n < 2 || n > 6 {
			t.Errorf("ускорение %d: опросов процессов за секунду %d, want 2…6 (период %v)", speed, n, watchEvery)
		}
	}
}

// Ловит: без наблюдателя (Watcher nil) Watch падает; наблюдатель опрашивает до Start или после
// Shutdown.
func TestWatchNilWatcherAndStopped(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()))
	h.start()
	h.childLogsIn()
	h.watch() // не падает

	w := newWatcher()
	h2 := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withWatcher(w))
	h2.svc.Watch(h2.real.Now()) // до Start
	h2.start()
	h2.childLogsIn()
	game := w.start(childSession, pathSuperGame)
	h2.svc.Shutdown(h2.real.Now())
	h2.svc.Watch(h2.real.Now())
	if n := w.listCount(); n != 0 || !w.alive(game) {
		t.Errorf("до Start и после Shutdown: опросов %d, игра жива %v; want 0, true", n, w.alive(game))
	}
}

// Ловит: вызов канала ждёт, пока наблюдатель завершает процесс (Kill под s.mu): на настоящем
// компьютере TerminateProcess и ожидание ручки идут сотни миллисекунд.
func TestWatchKillOutsideServiceLock(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	t.Cleanup(w.releaseKill)
	w.start(childSession, pathSuperGame)
	w.holdKill()
	done := make(chan struct{})
	go func() { defer close(done); h.watch() }()
	select {
	case <-w.killIn:
	case <-time.After(2 * time.Second):
		t.Fatal("наблюдатель не дошёл до Kill")
	}
	within(t, time.Second, "GetChildStatus во время Kill", func() {
		h.svc.Handle(childPeer(), ipc.Msg{Cmd: ipc.CmdGetChildStatus}, h.real.Now())
	})
	within(t, time.Second, "Tick во время Kill", func() { h.svc.Tick(h.clock.Now()) })
	w.releaseKill()
	<-done
	if got := w.killedPaths(); len(got) != 1 {
		t.Errorf("завершено %v, want SuperGame", got)
	}
}

// startAll запускает в сеансе ребёнка по процессу каждой программы набора и возвращает их по путям.
func startAll(w *fakeWatcher, paths ...string) map[string]ProcInfo {
	out := map[string]ProcInfo{}
	for _, p := range paths {
		out[p] = w.start(childSession, p)
	}
	return out
}

// sortedStrings — копия по возрастанию (для сравнения наборов).
func sortedStrings(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// Ловит (R5, R8, R12, R13): наблюдатель закрывает не то: незнакомая или запрещённая программа
// работает, игра при исчерпанных играх не закрыта, в режиме «только учёба» остались игры и
// прочее, учёба или система закрыты, под экраном блокировки закрыто что-то кроме игр, при лимите
// игр закрыта учёба во втором окне (Review Focus 7). Решение — настоящее ядро на поддельных часах.
func TestWatchVerdicts(t *testing.T) {
	all := []string{pathSuperGame, pathMinecraft, pathGeoGebra, pathBrave, pathChrome, pathNotepad, pathPCBlock, pathWebView}
	cases := []struct {
		name  string
		rules hopt
		want  []string
	}{
		{"обычный день: незнакомая и запрещённая", withRules(120, -1, "lock"), []string{pathSuperGame, pathChrome}},
		{"игры кончились: ещё и игра", withRules(120, 0, "lock"), []string{pathSuperGame, pathMinecraft, pathChrome}},
		{"только учёба: всё, кроме учёбы и системы", withRules(0, -1, "study_only"), []string{pathSuperGame, pathMinecraft, pathBrave, pathChrome}},
		{"экран блокировки: только игры (и незнакомое, запрещённое)", withRules(0, -1, "lock"), []string{pathSuperGame, pathMinecraft, pathChrome}},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			w := newWatcher()
			h := watchHarness(t, w, k.rules)
			h.setCategory(pathGeoGebra, "study")
			h.setCategory(pathChrome, "blocked")
			procs := startAll(w, all...)
			h.watch()
			if got, want := sortedStrings(w.killedPaths()), sortedStrings(k.want); jsonOf(t, got) != jsonOf(t, want) {
				t.Errorf("закрыты %v, want %v", got, want)
			}
			for _, p := range []string{pathGeoGebra, pathNotepad, pathPCBlock, pathWebView} {
				if !w.alive(procs[p]) {
					t.Errorf("закрыта %s: учёбу и систему наблюдатель не трогает", p)
				}
			}
			pend, _ := h.store.ListPending()
			if len(pend) != 1 || pend[0].Path != pathSuperGame || pend[0].Attempts != 1 || pend[0].Name != "supergame" {
				t.Errorf("«Ждут решения»: %+v, want SuperGame с одной попыткой", pend)
			}
			if n := len(h.store.eventsOf(evProgramForbiddenTry)); n != 1 {
				t.Errorf("событий program_forbidden_try %d, want 1", n)
			}
			// Повторный проход по тем же (уже закрытым) не закрывает ничего нового.
			h.watch()
			if n := len(w.killedPaths()); n != len(k.want) {
				t.Errorf("после второго прохода закрыто %d, want %d", n, len(k.want))
			}
		})
	}
}

// Ловит (R5, R8, Review Focus 6): включение запрета не закрывает уже запущенное — игра,
// открытая до исчерпания лимита игр, работает дальше; «Запретить» запущенной сейчас программы
// не закрывает её; смена категории действует только на новые процессы.
func TestWatchClosesRunningWhenRuleTurnsOn(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	game := w.start(childSession, pathMinecraft)
	brave := w.start(childSession, pathBrave)
	h.watch()
	if !w.alive(game) || !w.alive(brave) {
		t.Fatal("в обычный день закрыта игра или браузер")
	}
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	limits := sevenDays(120)
	for i := range limits {
		limits[i].GamesUnlimited, limits[i].GamesMin = false, 0
	}
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: limits}, nil)
	h.watch()
	if w.alive(game) || !w.alive(brave) {
		t.Fatalf("лимит игр 0: игра жива %v, браузер жив %v; want закрыта только игра", w.alive(game), w.alive(brave))
	}
	if ev := h.store.eventsOf(evProgramClosedGames); len(ev) != 1 || ev[0].ChildID != id {
		t.Errorf("событие закрытия игры: %+v", ev)
	}
	h.ok(adminPeer(), tok, ipc.CmdSetProgramsCategory, ipc.SetProgramsCategoryRequest{Paths: []string{pathBrave}, Category: "blocked"}, nil)
	h.watch()
	if w.alive(brave) {
		t.Error("«Запретить» запущенной программы: браузер жив")
	}
}
