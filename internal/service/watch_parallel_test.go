package service

import (
	"fmt"
	"testing"
	"time"

	"pcblock/internal/ipc"
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

// Ловит (B-32): чтение свойств закрытой незнакомой (издатель и описание для «Ждут решения») идёт
// в цикле Kill — медленный сетевой путь первой программы держит закрытие второй. Свойства
// читаются после всех Kill и попадают в «Ждут решения».
func TestWatchKillsBeforeReadingMeta(t *testing.T) {
	// Для этих путей свойства до решения не читаются (нет решения родителя в профиле, RM3-12,
	// RM3-13): каждый путь читается ровно один раз — после закрытия.
	{
		w, meta := newWatcher(), newMeta()
		h := watchHarness(t, w, withMeta(meta))
		w.start(childSession, pathSuperGame)
		w.start(childSession, pathHomework)
		h.watch()
		if got, want := jsonOf(t, sortedStrings(meta.askedPaths())), jsonOf(t, []string{pathHomework, pathSuperGame}); got != want {
			t.Fatalf("чтения свойств %s, want %s (одно на закрытую незнакомую, до решения — ни одного)", got, want)
		}
	}
	w, meta := newWatcher(), newMeta()
	meta.set(pathSuperGame, "Супер Студия", "Super Game", "")
	meta.set(pathHomework, "Школа №1", "Домашка", "")
	h := watchHarness(t, w, withMeta(meta))
	t.Cleanup(meta.release)
	w.start(childSession, pathSuperGame)
	w.start(childSession, pathHomework)
	meta.hold()
	done := make(chan struct{})
	go func() { defer close(done); h.watch() }()
	select {
	case <-meta.in:
	case <-time.After(2 * time.Second):
		t.Fatal("наблюдатель не дошёл до чтения свойств")
	}
	if got := w.killedPaths(); len(got) != 2 {
		t.Errorf("к началу чтения свойств закрыто %v, want обе программы", got)
	}
	meta.release()
	<-done
	pend, _ := h.store.ListPending()
	got := map[string]string{}
	for _, p := range pend {
		got[p.Path] = p.Publisher + "|" + p.Description
	}
	if want := map[string]string{pathSuperGame: "Супер Студия|Super Game", pathHomework: "Школа №1|Домашка"}; jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("«Ждут решения» %s, want %s", jsonOf(t, got), jsonOf(t, want))
	}
}

// Ловит (B-32, PL-17): зависшее чтение свойств (сетевой путь) держит проход наблюдателя и все
// следующие; повторный запуск того же exe, пока первое чтение висит, начинает второе чтение
// (горутины копятся).
func TestWatchMetaBudget(t *testing.T) {
	w, meta := newWatcher(), newMeta()
	meta.set(pathSuperGame, "Супер Студия", "Super Game", "")
	h := watchHarness(t, w, withMeta(meta))
	t.Cleanup(meta.release)
	w.start(childSession, pathSuperGame)
	meta.hold()
	within(t, time.Second, "проход при зависшем чтении свойств", h.watch)
	if got := w.killedPaths(); len(got) != 1 {
		t.Fatalf("закрыто %v, want SuperGame", got)
	}
	pend, _ := h.store.ListPending()
	if len(pend) != 1 || pend[0].Path != pathSuperGame || publisherOrStub(pend[0].Publisher) != "Без подписи издателя" || pend[0].Description != "" {
		t.Errorf("«Ждут решения» %+v, want SuperGame «Без подписи издателя» без описания", pend)
	}
	w.start(childSession, pathSuperGame) // новый PID того же exe
	within(t, time.Second, "второй проход при зависшем чтении свойств", h.watch)
	if got := w.killedPaths(); len(got) != 2 {
		t.Errorf("после второго запуска закрыто %d, want 2", len(got))
	}
	if n := meta.enteredCount(); n != 1 {
		t.Errorf("чтений свойств начато %d, want 1 (одно на путь, пока первое висит)", n)
	}
}

// Ловит (B-32): отпечаток файла до решения (игра родителя в «Загрузках», RM3-12) читается прямо в
// проходе: зависший сетевой путь держит проход, и незнакомая рядом не закрывается. Чтение до решения
// ждётся не дольше metaBudget; пока оно не готово, процесс не трогается («не знаю» — не трогать,
// PL-9), повторный проход не начинает второе чтение, а готовый отпечаток доходит до решения и до
// категории секунды в следующем проходе.
func TestWatchFingerprintBudget(t *testing.T) {
	w, meta := newWatcher(), newMeta()
	meta.set(pathSuperGame, "", "", "1024:1700000000")
	h := watchHarness(t, w, withMeta(meta))
	t.Cleanup(meta.release)
	if err := h.store.UpsertProgram(Program{Path: pathSuperGame, Name: "SuperGame", Category: "games", Source: "parent", Fingerprint: "1024:1700000000"}); err != nil {
		t.Fatal(err)
	}
	h.svc.mu.Lock()
	h.svc.reloadPrograms()
	h.svc.mu.Unlock()
	game := w.start(childSession, pathSuperGame)
	homework := w.start(childSession, pathHomework)
	meta.hold()
	within(t, time.Second, "проход при зависшем чтении отпечатка", h.watch)
	if !w.alive(game) || w.alive(homework) {
		t.Fatalf("при зависшем отпечатке: игра жива %v, незнакомая жива %v; want true, false", w.alive(game), w.alive(homework))
	}
	h.real.Advance(time.Second)
	within(t, time.Second, "второй проход при зависшем чтении отпечатка", h.watch)
	if !w.alive(game) {
		t.Fatal("второй проход: игра с непрочитанным отпечатком закрыта")
	}
	if n := meta.enteredCount(); n != 2 {
		t.Errorf("чтений свойств начато %d, want 2 (отпечаток игры один раз, незнакомая после закрытия)", n)
	}
	meta.release()
	if !waitFor(2*time.Second, func() bool { return len(meta.askedPaths()) == 2 }) {
		t.Fatalf("чтения свойств не закончились: %v", meta.askedPaths())
	}
	h.real.Advance(time.Second)
	h.watch()
	h.run(5, fgSample(pathSuperGame, ipc.CatNameOther))
	id := h.childID(childSID)
	if st, _ := h.svc.children[id].eng.State(); !w.alive(game) || st.GamesS != 5 {
		t.Errorf("отпечаток готов: игра жива %v, игры %d с; want true, 5", w.alive(game), st.GamesS)
	}
	if n := meta.enteredCount(); n != 2 {
		t.Errorf("после готового отпечатка чтений %d, want 2 (результат не читается заново)", n)
	}
}
