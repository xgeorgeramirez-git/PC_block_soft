package service

import (
	"sync"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Ловит (PL-5, PL-13, Review Focus 2): закрытие игры и незнакомой программы наблюдателем пишется
// «обходом защиты» или задевает стража и значок (их перезапуск и запись о гибели).
func TestWatchClosingIsNotBypass(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(120, 0, "lock"))
	guards, trays := len(h.launcher.launched(ProcGuard, childSession)), len(h.launcher.launched(ProcTray, childSession))
	startAll(w, pathMinecraft, pathSuperGame)
	for i := 0; i < int(bypassGrace/time.Second)+3; i++ {
		h.watch()
		h.step(fgSample(pathMinecraft, ipc.CatNameOther))
	}
	if n := len(w.killedPaths()); n != 2 {
		t.Fatalf("закрыто %d, want 2", n)
	}
	if ev := h.store.eventsOf(core.EvBypass); len(ev) != 0 {
		t.Errorf("закрытие наблюдателем записано обходом: %+v", ev)
	}
	if g, tr := len(h.launcher.launched(ProcGuard, childSession)), len(h.launcher.launched(ProcTray, childSession)); g != guards || tr != trays {
		t.Errorf("страж или значок перезапущены: стражей %d→%d, значков %d→%d", guards, g, trays, tr)
	}
}

// Ловит (PL-5, PL-13, FB-1, FB-5): выход из учётки, смена пользователя и остановка службы, пока
// работает наблюдатель, дают запись «обход защиты», панику или опрос исчезнувшего сеанса.
func TestWatchDuringSessionEndsAndStop(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(120, 0, "lock"))
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				w.start(childSession, pathMinecraft)
				h.svc.Watch(h.real.Now())
				time.Sleep(time.Millisecond)
			}
		}
	}()
	h.run(3, activeSample)
	h.svc.OnSession(SessionDisconnect, childSession, h.real.Now()) // смена пользователя
	h.run(3, activeSample)
	h.svc.OnSession(SessionConnect, childSession, h.real.Now())
	for _, p := range h.launcher.aliveOf(ProcGuard, childSession) {
		h.launcher.kill(p) // Windows закрывает стража при выходе
	}
	h.launcher.setSessions()
	h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
	for i := 0; i < int(bypassGrace/time.Second)+2; i++ { // сеанса нет: страж не опрашивает
		h.clock.Advance(time.Second)
		h.real.Advance(time.Second)
		h.svc.Tick(h.clock.Now())
		h.svc.Housekeep(h.real.Now())
	}
	before := w.listCalls()[childSession]
	time.Sleep(20 * time.Millisecond)
	if after := w.listCalls()[childSession]; after != before {
		t.Errorf("после выхода сеанс %d ещё опрашивается: %d → %d", childSession, before, after)
	}
	h.svc.Shutdown(h.real.Now())
	close(stop)
	wg.Wait()
	if ev := h.store.eventsOf(core.EvBypass); len(ev) != 0 {
		t.Errorf("штатные завершения записаны обходом: %+v", ev)
	}
}

// Ловит (Review Focus 5): гонки между наблюдателем, тиком, обслуживанием и каналом (go test -race):
// память наблюдателя, ограничитель, «Ждут решения», перезагрузка классификатора.
func TestWatchConcurrentWithTickAndHandle(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(120, 0, "lock"))
	tok := h.login(adminPeer())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			w.start(childSession, pathSuperGame)
			w.start(childSession, pathMinecraft)
			h.svc.Watch(h.real.Now())
		}
	}()
	for i := 0; i < 50; i++ {
		h.step(fgSample(pathMinecraft, ipc.CatNameOther))
		h.do(adminPeer(), tok, ipc.CmdListPrograms, nil)
		cat := "games"
		if i%2 == 0 {
			cat = "blocked"
		}
		h.do(adminPeer(), tok, ipc.CmdSetProgramsCategory, ipc.SetProgramsCategoryRequest{Paths: []string{pathBrave}, Category: cat})
	}
	wg.Wait()
}
