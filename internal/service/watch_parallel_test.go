package service

import (
	"fmt"
	"testing"
	"time"
)

// waitFor ждёт cond не дольше d, проверяя каждые 5 мс; итог — выполнилось ли условие.
func waitFor(d time.Duration, cond func() bool) bool {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// Ловит (B-37, NFR-6): Kill прохода идут по очереди — Windows-адаптер ждёт исчезновения каждого
// процесса до 300 мс, и пять закрываемых в «только учёбе» держат проход 1,5 с. И обратное (PL-33):
// пул без предела — 12 закрываемых получают 12 одновременных Kill вместо killWorkers = 8.
func TestWatchKillsInParallel(t *testing.T) {
	t.Run("пять незнакомых закрываются одновременно", func(t *testing.T) {
		w := newWatcher()
		h := watchHarness(t, w)
		t.Cleanup(w.releaseKill)
		for _, n := range []string{"a", "b", "c", "d", "e"} {
			w.start(childSession, `c:\users\миша\downloads\`+n+`.exe`)
		}
		w.holdKill()
		done := make(chan struct{})
		go func() { defer close(done); h.watch() }()
		if !waitFor(2*time.Second, func() bool { return w.killTries() == 5 }) {
			t.Fatalf("Kill идут по очереди: начато %d из 5", w.killTries())
		}
		w.releaseKill()
		<-done
		if got := w.killedPaths(); len(got) != 5 {
			t.Errorf("закрыто %v, want 5 программ", got)
		}
		if pend, _ := h.store.ListPending(); len(pend) != 5 {
			t.Errorf("«Ждут решения»: %d строк, want 5", len(pend))
		}
	})
	t.Run("двенадцать закрываемых: одновременно не больше восьми", func(t *testing.T) {
		w := newWatcher()
		h := watchHarness(t, w)
		t.Cleanup(w.releaseKill)
		for i := 0; i < 12; i++ {
			w.start(childSession, fmt.Sprintf(`c:\users\миша\downloads\g%02d.exe`, i))
		}
		w.holdKill()
		done := make(chan struct{})
		go func() { defer close(done); h.watch() }()
		if !waitFor(2*time.Second, func() bool { return w.killTries() >= 8 }) {
			t.Fatalf("за 2 с начато Kill %d, want 8 одновременно", w.killTries())
		}
		time.Sleep(200 * time.Millisecond) // девятый Kill ждёт свободного места в пуле
		if n := w.killTries(); n != 8 {
			t.Fatalf("при закрытых воротах начато Kill %d, want 8 (killWorkers)", n)
		}
		w.releaseKill()
		<-done
		if got := w.killedPaths(); len(got) != 12 {
			t.Errorf("закрыто %d, want 12", len(got))
		}
	})
}
