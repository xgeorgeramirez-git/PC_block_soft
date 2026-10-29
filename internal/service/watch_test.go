package service

import (
	"context"
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
