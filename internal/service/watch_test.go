package service

import (
	"context"
	"errors"
	"sort"
	"strings"
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

// Ловит (PL-13, Review Focus 3 и 8): наблюдатель закрывает компоненты PC Block — по пути каталога
// установки, по решению родителя «Запретить» на pcblock.exe или когда страж запущен не из каталога
// установки (сборка разработчика): гибель окон и стража записалась бы «обходом защиты».
func TestWatchNeverTouchesPCBlock(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w, withRules(0, -1, "study_only"))
	if err := h.store.UpsertProgram(Program{Path: pathPCBlock, Name: "PC Block", Category: "blocked", Source: "parent"}); err != nil {
		t.Fatal(err)
	}
	h.svc.mu.Lock()
	h.svc.reloadPrograms()
	guardPID := h.svc.sessions[childSession].guard.pid
	trayPID := h.svc.sessions[childSession].tray.pid
	h.svc.mu.Unlock()
	keep := []ProcInfo{
		w.start(childSession, pathPCBlock), w.start(childSession, pathPCGuard), w.start(childSession, pathWebView),
		w.start(childSession, pathNotepad),
	}
	own := []ProcInfo{
		{PID: guardPID, Stamp: 1, SessionID: childSession, Path: pathDevBuild, PathOK: true, UserSID: childSID, UserOK: true},
		{PID: trayPID, Stamp: 2, SessionID: childSession, Path: pathDevBuild, PathOK: true, UserSID: childSID, UserOK: true},
	}
	for _, p := range own {
		w.add(p)
	}
	h.watch()
	for _, p := range append(keep, own...) {
		if !w.alive(p) {
			t.Errorf("закрыт %s (PID %d): компоненты PC Block и система не трогаются", p.Path, p.PID)
		}
	}
	h.run(int(bypassGrace/time.Second)+2, activeSample)
	if ev := h.store.eventsOf(core.EvBypass); len(ev) != 0 {
		t.Errorf("записи «обход защиты»: %+v", ev)
	}
}

// Ловит (PL-9, Review Focus 2 и 3): «не знаю» закрывается: нечитаемый путь (отказ в доступе),
// пустой путь, процесс чужого сеанса в ответе адаптера, процесс в сеансе родителя (R19), ошибка
// списка процессов; нечитаемое не пишется в журнал работы.
func TestWatchUnknownIsUntouched(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	h.launcher.setSessions(
		SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID},
		SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID},
	)
	h.svc.OnSession(SessionLogon, 1, h.real.Now())
	keep := []ProcInfo{
		{PID: 7001, Stamp: 1, SessionID: childSession, Path: pathSuperGame, PathOK: false, UserSID: childSID, UserOK: true},
		{PID: 7002, Stamp: 1, SessionID: childSession, Path: "", PathOK: true, UserSID: childSID, UserOK: true},
		{PID: 7003, Stamp: 1, SessionID: childSession, Path: `relative\game.exe`, PathOK: true, UserSID: childSID, UserOK: true},
		{PID: 7004, Stamp: 1, SessionID: 9, Path: pathSuperGame, PathOK: true, UserSID: childSID, UserOK: true},
	}
	for _, p := range keep {
		w.add(p)
	}
	w.mu.Lock()
	w.listAll[childSession] = true // адаптер ошибся и вернул процесс сеанса 9
	w.mu.Unlock()
	parentGame := w.start(1, pathSuperGame)
	h.watch()
	for _, p := range append(keep, parentGame) {
		if !w.alive(p) {
			t.Errorf("закрыт процесс %+v: «не знаю» = «не трогать»", p)
		}
	}
	if calls := w.listCalls(); calls[1] != 0 || calls[childSession] != 1 {
		t.Errorf("опрос сеансов %v, want только сеанс ребёнка", calls)
	}
	if !strings.Contains(h.logText(), "путь не прочитан") {
		t.Errorf("нечитаемые пути не записаны в журнал работы:\n%s", h.logText())
	}
	if n := len(h.store.events); n != 0 {
		t.Errorf("в журнале родителя %d событий, want 0: %v", n, h.store.eventKinds())
	}

	w.mu.Lock()
	w.listAll[childSession] = false
	w.listErr[childSession] = errors.New("отказано в доступе")
	w.mu.Unlock()
	game := w.start(childSession, pathSuperGame)
	h.real.Advance(time.Second)
	h.watch()
	if !w.alive(game) || !strings.Contains(h.logText(), "отказано в доступе") {
		t.Errorf("ошибка списка: игра жива %v, журнал работы:\n%s", w.alive(game), h.logText())
	}
}

// Ловит (R1, R11, R18, RM3-10): наблюдатель закрывает программы у ребёнка-администратора, во время
// паузы, в безопасном режиме (правила не прочитаны) или пока список программ не готов.
func TestWatchHoldsOff(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h *harness)
		opts  []hopt
	}{
		{"администратор", nil, []hopt{func(h *harness) { h.admins.set(childSID, true) }}},
		{"безопасный режим", nil, []hopt{func(h *harness) { h.store.failLimits = true }}},
		{"пауза", func(h *harness) {
			tok := h.login(adminPeer())
			h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: h.childID(childSID), Preset: ipc.PauseHour1}, nil)
		}, nil},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			w := newWatcher()
			h := watchHarness(t, w, append([]hopt{withRules(0, 0, "study_only")}, k.opts...)...)
			if k.setup != nil {
				k.setup(h)
			}
			procs := startAll(w, pathSuperGame, pathMinecraft, pathBrave)
			h.watch()
			for p, pi := range procs {
				if !w.alive(pi) {
					t.Errorf("закрыт %s", p)
				}
			}
			if n := w.listCount(); n != 0 {
				t.Errorf("сеанс опрошен %d раз, want 0", n)
			}
		})
	}
}

// Ловит (RM3-10, Review Focus 8): обновление 0.2.5 → 0.3.0: снимок программ ещё не сделан (сканер
// не ответил), а наблюдатель уже закрывает всё «незнакомое», включая Brave и его помощников.
// После снимка Brave и вспомогательные процессы не «незнакомые», незнакомая — закрыта.
func TestWatchFirstStartAfterUpdate(t *testing.T) {
	sc := typicalScan()
	sc.err = errors.New("сканер не ответил")
	w := newWatcher()
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(sc), withWatcher(w))
	h.start()
	h.childLogsIn()
	procs := startAll(w, pathBrave, pathBraveUpd, pathSuperGame)
	h.watch()
	if n := len(w.killedPaths()); n != 0 || w.listCount() != 0 {
		t.Fatalf("до снимка: закрыто %d, опросов %d; want 0, 0", n, w.listCount())
	}
	sc.mu.Lock()
	sc.err = nil
	sc.mu.Unlock()
	h.real.Advance(programsRetryEvery)
	h.svc.Housekeep(h.real.Now())
	h.svc.waitSeed()
	h.watch()
	if !w.alive(procs[pathBrave]) || !w.alive(procs[pathBraveUpd]) || w.alive(procs[pathSuperGame]) {
		t.Errorf("после снимка: закрыты %v, want только SuperGame", w.killedPaths())
	}
}

// Ловит (RM3-11): кэш пути по одному PID: Windows повторно отдала номер завершившегося Brave
// новой незнакомой программе, а наблюдатель помнит «Brave» и не закрывает её.
func TestWatchPIDReuse(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	brave := ProcInfo{PID: 7100, Stamp: 1, SessionID: childSession, Path: pathBrave, PathOK: true, UserSID: childSID, UserOK: true}
	w.add(brave)
	h.watch()
	w.exit(brave)
	game := ProcInfo{PID: 7100, Stamp: 2, SessionID: childSession, Path: pathSuperGame, PathOK: true, UserSID: childSID, UserOK: true}
	w.add(game)
	h.watch()
	if w.alive(game) {
		t.Error("новая программа с тем же PID не закрыта: кэш по одному PID")
	}
}

// Ловит: защищённый процесс (Kill не удался) записывается «Ждут решения» и плашкой, хотя
// программа работает; повторной попытки закрыть нет; сбой не попадает в журнал работы.
func TestWatchKillRefused(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	game := w.start(childSession, pathSuperGame)
	w.mu.Lock()
	w.protected[game.PID] = true
	w.mu.Unlock()
	h.watch()
	if !w.alive(game) || !strings.Contains(h.logText(), "не удалось закрыть") {
		t.Fatalf("защищённый процесс: жив %v, журнал работы:\n%s", w.alive(game), h.logText())
	}
	if pend, _ := h.store.ListPending(); len(pend) != 0 {
		t.Errorf("незакрытая программа в «Ждут решения»: %+v", pend)
	}
	w.mu.Lock()
	delete(w.protected, game.PID)
	w.mu.Unlock()
	h.watch()
	if w.alive(game) {
		t.Error("второй проход не закрыл программу")
	}
	if pend, _ := h.store.ListPending(); len(pend) != 1 || pend[0].Attempts != 1 {
		t.Errorf("после закрытия «Ждут решения»: %+v, want одна попытка", pend)
	}
}

// Ловит (R19, PL-9): наблюдатель отбирает процессы только по сеансу и закрывает программу, которую
// администратор запустил через UAC в сеансе ребёнка (установщик из «Загрузок»); процесс, чей
// пользователь не прочитан, закрывается («не знаю» = «не трогать»); регистр SID мешает узнать
// ребёнка; запись о чужом процессе попадает в журнал родителя, а не в журнал работы.
func TestWatchSkipsOtherUserInChildSession(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	admin := w.startAs(childSession, `c:\users\миша\downloads\setup.exe`, parentSID, true)
	unread := w.startAs(childSession, `c:\users\миша\downloads\other.exe`, "", false)
	// Адаптер заполнил SID, но сам пометил его непрочитанным: доверять нельзя.
	half := w.startAs(childSession, `c:\users\миша\downloads\other.exe`, childSID, false)
	lower := w.startAs(childSession, `c:\users\миша\downloads\lower.exe`, strings.ToLower(childSID), true)
	child := w.startAs(childSession, pathSuperGame, childSID, true)
	h.watch()
	if !w.alive(admin) || !w.alive(unread) || !w.alive(half) {
		t.Errorf("закрыт процесс другого пользователя или с непрочитанным пользователем: закрыты %v", w.killedPaths())
	}
	if w.alive(child) || w.alive(lower) {
		t.Errorf("процесс ребёнка не закрыт: закрыты %v, want supergame.exe и lower.exe", w.killedPaths())
	}
	pend, _ := h.store.ListPending()
	paths := []string{}
	for _, p := range pend {
		paths = append(paths, p.Path)
	}
	if want := []string{`c:\users\миша\downloads\lower.exe`, pathSuperGame}; !slicesEqual(sortedStrings(paths), want) {
		t.Errorf("«Ждут решения» %v, want %v", paths, want)
	}
	for _, ev := range h.store.events {
		if d, _ := ev.Details["path"].(string); strings.Contains(d, "setup.exe") || strings.Contains(d, "other.exe") {
			t.Errorf("в журнале родителя запись о чужом процессе: %+v", ev)
		}
	}
	if !strings.Contains(h.logText(), "другого пользователя") {
		t.Errorf("процессы другого пользователя не записаны в журнал работы:\n%s", h.logText())
	}
}

// Ловит (PL-9): SID ребёнка в базе пуст, а наблюдатель всё равно закрывает программы его сеанса
// (сравнение с пустой строкой или пропуск проверки).
func TestWatchChildWithoutSIDClosesNothing(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	h.svc.mu.Lock()
	h.svc.children[h.childID(childSID)].child.SID = ""
	h.svc.mu.Unlock()
	game := w.startAs(childSession, pathSuperGame, "", false)
	h.watch()
	if !w.alive(game) {
		t.Error("программа закрыта, хотя SID ребёнка неизвестен")
	}
	if !strings.Contains(h.logText(), "SID ребёнка неизвестен") {
		t.Errorf("журнал работы молчит о неизвестном SID:\n%s", h.logText())
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Ловит (PL-16): TerminateProcess сработал, а процесс исчез не сразу (завершается дольше 300 мс):
// следующий проход видит его живым. Записи о закрытии (журнал родителя, «Ждут решения», плашка)
// должны быть ровно одни, а журнал работы не должен говорить «не удалось закрыть».
func TestWatchKilledButAliveOnePass(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	game := w.start(childSession, pathSuperGame)
	w.mu.Lock()
	w.linger[game.PID] = true
	w.mu.Unlock()
	h.watch()
	h.real.Advance(time.Second)
	h.watch()
	h.real.Advance(time.Second)
	h.watch()
	if w.alive(game) {
		t.Fatal("процесс не завершился")
	}
	if ev := h.store.eventsOf(evProgramPending); len(ev) != 1 {
		t.Errorf("записей «ждёт решения» в журнале родителя %d, want 1: %v", len(ev), h.store.eventKinds())
	}
	if pend, _ := h.store.ListPending(); len(pend) != 1 || pend[0].Attempts != 1 {
		t.Errorf("«Ждут решения»: %+v, want одна попытка", pend)
	}
	if strings.Contains(h.logText(), "не удалось закрыть") {
		t.Errorf("журнал работы пишет о неудаче, хотя процесс завершается:\n%s", h.logText())
	}
}
