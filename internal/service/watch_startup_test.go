package service

import (
	"reflect"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/startup"
)

// Правило наблюдателя для автозагрузки (M5b, RM5b-3, уточнения плана 1 и 17, Review Focus 1;
// задача T4b): выключенная или ждущая решения запись, запустившаяся сама при входе, закрывается
// без плашки и нарушения, родителю — строка журнала раз в день.

// filetime — момент t в единицах ProcInfo.Stamp (FILETIME: 100 нс с 1601 года), как у адаптера.
func filetime(t time.Time) uint64 { return uint64(t.UnixNano()/100) + 116444736000000000 }

// startCreated запускает процесс path от имени sid в сеансе session, созданный в момент created,
// с родителем parent.
func (w *fakeWatcher) startCreated(session uint32, path, sid string, parent ProcInfo, created time.Time) ProcInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextPID++
	p := ProcInfo{PID: w.nextPID, ParentPID: parent.PID, Stamp: filetime(created), SessionID: session, Path: path, PathOK: true,
		UserSID: sid, UserOK: true}
	w.procs = append(w.procs, p)
	return p
}

// setCommand меняет команду и exe записи name (sid «» — общей).
func (f *fakeStartupSource) setCommand(sid, name, exe string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.shared
	if sid != "" {
		list = f.own[sid]
	}
	for i := range list {
		if list[i].Name == name {
			list[i].Command, list[i].Exe = `"`+exe+`" --tray`, exe
		}
	}
}

// startupWatchHarness — служба с Мишей и Колей, снимком программ, наблюдателем и автозагрузкой
// src; первая проверка при старте прошла, родитель вошёл. Дети ещё не вошли.
func startupWatchHarness(t *testing.T, src *fakeStartupSource, w *fakeWatcher) (*harness, string) {
	t.Helper()
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120),
		withPrograms(typicalScan()), withWatcher(w), withStartup(src))
	h.start()
	h.svc.waitStartup()
	return h, h.login(adminPeer())
}

// startupLogIn — Миша и Коля вошли сейчас, проверка их автозагрузки прошла; Проводники сеансов.
func (h *harness) startupLogIn(w *fakeWatcher) (ProcInfo, ProcInfo) {
	h.t.Helper()
	h.childLogsInBoth()
	h.svc.waitStartup()
	return w.start(childSession, pathExplorer), w.startAs(child2Session, pathExplorer, child2SID, true)
}

func (h *harness) setStartup(tok string, id int64, name string, on bool) {
	h.t.Helper()
	it := h.startupItem(tok, id, name)
	h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: id, ID: it.ID, On: on}, nil)
	h.svc.waitStartup()
}

// noViolations — закрытие правилом автозагрузки не дало ребёнку плашку и не записало нарушение,
// попытку незнакомой программы или «запрещённая запускалась сама».
func (h *harness) noViolations() {
	h.t.Helper()
	for _, k := range []string{evProgramPending, evProgramForbiddenTry, evProgramAutoClosed, evProgramClosedGames, evProgramClosedStudyOnly} {
		if ev := h.store.eventsOf(k); len(ev) != 0 {
			h.t.Errorf("журнал %s: %+v", k, ev)
		}
	}
	if got := platesOf(h.poll(inputSample(1000, pathExplorer))); len(got) != 0 {
		h.t.Errorf("плашки ребёнку: %+v", got)
	}
	if list, err := h.store.ListPending(); err != nil || len(list) != 0 {
		h.t.Errorf("«Ждут решения» программ: %+v (%v)", list, err)
	}
}

func geogebraEntry(name string) startup.Entry {
	return startup.Entry{Source: startup.SrcRun, Name: name, Command: `"C:\Program Files\GeoGebra\GeoGebra.exe" --tray`, Exe: `C:\Program Files\GeoGebra\GeoGebra.exe`, Enabled: true}
}

// Ловит (RM5b-3, уточнение 1): общая запись, выключенная родителем у Миши, при входе не
// закрывается (или закрывается с плашкой и нарушением, хотя ребёнок ничего не делал); у Коли,
// которому она включена, закрывается; повтор в тот же день пишет вторую строку журнала.
func TestWatchStartupSharedOffPerChild(t *testing.T) {
	src := newStartupSource()
	src.addShared(geogebraEntry("GeoGebra"))
	w := newWatcher()
	h, tok := startupWatchHarness(t, src, w)
	ex1, ex2 := h.startupLogIn(w)
	misha, kolya := h.childID(childSID), h.childID(child2SID)
	h.setStartup(tok, misha, "GeoGebra", false)
	if it := h.startupItem(tok, kolya, "GeoGebra"); it.Decision != ipc.StartupOn {
		t.Fatalf("у Коли решение %q, want on", it.Decision)
	}

	h.real.Advance(5 * time.Second)
	p1 := w.startCreated(childSession, pathGeoGebra, childSID, ex1, h.real.Now())
	p2 := w.startCreated(child2Session, pathGeoGebra, child2SID, ex2, h.real.Now())
	h.watch()
	if w.alive(p1) {
		t.Error("у Миши выключенная общая запись не закрыта")
	}
	if !w.alive(p2) {
		t.Error("у Коли включённая общая запись закрыта")
	}
	want := []string{"Миша: запись автозагрузки «GeoGebra» запускалась сама и закрыта"}
	if got := h.journalTexts(evStartupClosed); !reflect.DeepEqual(got, want) {
		t.Errorf("журнал = %q, want %q", got, want)
	}

	// Повтор в тот же день: закрыт снова, строка журнала одна.
	h.real.Advance(10 * time.Second)
	p3 := w.startCreated(childSession, pathGeoGebra, childSID, ex1, h.real.Now())
	h.watch()
	if w.alive(p3) {
		t.Error("повтор не закрыт")
	}
	if got := h.journalTexts(evStartupClosed); !reflect.DeepEqual(got, want) {
		t.Errorf("после повтора журнал = %q, want %q", got, want)
	}
	h.noViolations()
}

// Ловит (RM5b-3): своя запись, ждущая решения (Проводник запустил её раньше, чем служба
// выключила) или выключенная, при входе не закрывается или закрывается как незнакомая программа:
// с плашкой «Попросить разрешение» и строкой «Ждут решения».
func TestWatchStartupOwnPendingAndOff(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Steam", false))
	w := newWatcher()
	h, tok := startupWatchHarness(t, src, w)
	ex1, _ := h.startupLogIn(w)
	misha := h.childID(childSID)
	src.addOwn(childSID, runE("Roblox", true))
	h.listStartup(tok, misha, true)
	if it := h.startupItem(tok, misha, "Roblox"); it.Decision != ipc.StartupPending {
		t.Fatalf("Roblox = %q, want pending", it.Decision)
	}
	if it := h.startupItem(tok, misha, "Steam"); it.Decision != ipc.StartupOff {
		t.Fatalf("Steam = %q, want off", it.Decision)
	}

	h.real.Advance(3 * time.Second)
	roblox := w.startCreated(childSession, `C:\Apps\Roblox.exe`, childSID, ex1, h.real.Now())
	steam := w.startCreated(childSession, `C:\Apps\Steam.exe`, childSID, ex1, h.real.Now())
	h.watch()
	if w.alive(roblox) || w.alive(steam) {
		t.Errorf("не закрыты: Roblox %v, Steam %v", w.alive(roblox), w.alive(steam))
	}
	want := []string{
		"Миша: запись автозагрузки «Roblox» запускалась сама и закрыта",
		"Миша: запись автозагрузки «Steam» запускалась сама и закрыта",
	}
	got := h.journalTexts(evStartupClosed)
	if len(got) == 2 && got[0] > got[1] {
		got[0], got[1] = got[1], got[0]
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("журнал = %q, want %q", got, want)
	}
	h.noViolations()
}

// Ловит (RM5b-3): правило автозагрузки закрывает программу, которую ребёнок запустил сам (из
// программы на переднем плане сразу после входа или из Проводника позже), или автоматический
// запуск позже первых 60 с после входа (выключена только автозагрузка).
func TestWatchStartupManualOrLateUntouched(t *testing.T) {
	cases := []struct {
		name   string
		after  time.Duration
		input  int64 // простой ребёнка до запуска, мс
		fromFG bool  // родитель — программа на переднем плане (Brave), иначе Проводник
	}{
		{"из программы на переднем плане через 5 с", 5 * time.Second, 0, true},
		{"из Проводника через 60 с при вводе", manualLogonGrace, 1000, false},
		{"сам через 61 с", manualLogonGrace + time.Second, 30000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := newStartupSource()
			src.addShared(geogebraEntry("GeoGebra"))
			w := newWatcher()
			h, tok := startupWatchHarness(t, src, w)
			ex1, _ := h.startupLogIn(w)
			h.setStartup(tok, h.childID(childSID), "GeoGebra", false)
			parent, fg := ex1, pathExplorer
			if c.fromFG {
				parent, fg = w.start(childSession, pathBrave), pathBrave
			}
			h.real.Advance(c.after)
			h.poll(inputSample(c.input, fg))
			p := w.startCreated(childSession, pathGeoGebra, childSID, parent, h.real.Now())
			h.watch()
			if !w.alive(p) {
				t.Error("закрыт правилом автозагрузки")
			}
			if ev := h.store.eventsOf(evStartupClosed); len(ev) != 0 {
				t.Errorf("журнал: %+v", ev)
			}
		})
	}
}

// Ловит (Review Focus 1, перезапуск службы в сеансе ребёнка): после перезапуска службы первые
// 60 с считаются «после входа», и программа, которую ребёнок давно запустил сам, закрывается как
// автозагрузка. Процесс, созданный незадолго до того, как служба увидела вход (служба увидела
// сеанс позже), закрывается.
func TestWatchStartupProcessOlderThanLogon(t *testing.T) {
	src := newStartupSource()
	src.addShared(geogebraEntry("GeoGebra"))
	w := newWatcher()
	h, tok := startupWatchHarness(t, src, w)
	ex1, _ := h.startupLogIn(w)
	h.setStartup(tok, h.childID(childSID), "GeoGebra", false)
	logon := h.real.Now()
	old := w.startCreated(childSession, pathGeoGebra, childSID, ex1, logon.Add(-10*time.Minute))
	h.real.Advance(2 * time.Second)
	h.watch()
	if !w.alive(old) {
		t.Fatal("программа, запущенная за 10 мин до входа, закрыта")
	}
	fresh := w.startCreated(childSession, pathGeoGebra, childSID, ex1, logon.Add(-30*time.Second))
	h.watch()
	if w.alive(fresh) {
		t.Error("программа, созданная за 30 с до того, как служба увидела вход, не закрыта")
	}
}

// Ловит (RM5b-3, Review Focus): правило автозагрузки закрывает Компоненты Windows или PC Block,
// если путь записи совпал; закрывает программу, у которой есть включённая запись с тем же exe.
func TestWatchStartupSystemAndOnUntouched(t *testing.T) {
	src := newStartupSource()
	src.addShared(geogebraEntry("GeoGebra"))
	src.addOwn(childSID, geogebraEntry("GeoGebraMine"))
	w := newWatcher()
	h, tok := startupWatchHarness(t, src, w)
	ex1, _ := h.startupLogIn(w)
	misha := h.childID(childSID)
	h.setStartup(tok, misha, "GeoGebra", false) // общая выключена, своя с тем же exe — включена
	src.addOwn(childSID, startup.Entry{Source: startup.SrcRun, Name: "Notes", Command: pathNotepad, Exe: pathNotepad, Enabled: true})
	src.addOwn(childSID, startup.Entry{Source: startup.SrcRun, Name: "Guard", Command: pathPCGuard, Exe: pathPCGuard, Enabled: true})
	h.listStartup(tok, misha, true)
	for _, n := range []string{"Notes", "Guard"} {
		if it := h.startupItem(tok, misha, n); it.Decision != ipc.StartupPending {
			t.Fatalf("%s = %q, want pending", n, it.Decision)
		}
	}
	h.real.Advance(time.Second)
	procs := []ProcInfo{
		w.startCreated(childSession, pathNotepad, childSID, ex1, h.real.Now()),
		w.startCreated(childSession, pathPCGuard, childSID, ex1, h.real.Now()),
		w.startCreated(childSession, pathGeoGebra, childSID, ex1, h.real.Now()),
	}
	h.watch()
	for _, p := range procs {
		if !w.alive(p) {
			t.Errorf("%s закрыт", p.Path)
		}
	}
	if ev := h.store.eventsOf(evStartupClosed); len(ev) != 0 {
		t.Errorf("журнал: %+v", ev)
	}
}

// Ловит (решение оркестратора по Open T4a): у включённой записи сменилась команда (другой exe
// под тем же именем), а решение осталось «on» — запись запускается без ведома родителя. Своя
// должна вернуться в «Ждут решения» и выключиться в Windows, общая — ждать решения у каждого
// ребёнка; выключенная с новой командой остаётся выключенной без строки журнала.
func TestStartupChangedCommandPendingAgain(t *testing.T) {
	src := newStartupSource()
	src.addOwn(childSID, runE("Discord", true))
	src.addOwn(childSID, runE("Steam", false))
	src.addShared(runE("OneDrive", true))
	w := newWatcher()
	h, tok := startupWatchHarness(t, src, w)
	ex1, _ := h.startupLogIn(w)
	misha, kolya := h.childID(childSID), h.childID(child2SID)

	src.setCommand(childSID, "Discord", `C:\Apps\Other.exe`)
	src.setCommand(childSID, "Steam", `C:\Apps\Steam2.exe`)
	src.setCommand("", "OneDrive", `C:\Apps\OneDrive2.exe`)
	n := src.callN()
	rep := h.listStartup(tok, misha, true)
	if got := decisions(rep.Pending); !reflect.DeepEqual(got, []string{"Discord=pending", "OneDrive=pending"}) {
		t.Errorf("Миша ждут решения = %v", got)
	}
	if got := decisions(rep.Own); !reflect.DeepEqual(got, []string{"Steam=off"}) {
		t.Errorf("Миша свои = %v", got)
	}
	if c := setCalls(src.callsFrom(n)); !reflect.DeepEqual(c, []string{"set " + childSID + " Discord false"}) {
		t.Errorf("Windows = %v; want выключение Discord", c)
	}
	if got := decisions(h.listStartup(tok, kolya, true).Pending); !reflect.DeepEqual(got, []string{"OneDrive=pending"}) {
		t.Errorf("Коля ждут решения = %v", got)
	}
	want := []string{
		"Миша: запись автозагрузки «Discord» изменилась, выключена и ждёт решения",
		"Миша: запись автозагрузки «OneDrive» изменилась, выключена и ждёт решения",
		"Коля: запись автозагрузки «OneDrive» изменилась, выключена и ждёт решения",
	}
	if got := h.journalTexts(evStartupNew); !reflect.DeepEqual(got, want) {
		t.Errorf("журнал = %q\nwant %q", got, want)
	}
	// Набор наблюдателя пересобран с новым exe.
	h.real.Advance(time.Second)
	p := w.startCreated(childSession, `C:\Apps\OneDrive2.exe`, childSID, ex1, h.real.Now())
	h.watch()
	if w.alive(p) {
		t.Error("новый exe изменившейся общей записи не закрыт")
	}
	// Повторная проверка: ничего нового.
	h.listStartup(tok, misha, true)
	if got := len(h.store.eventsOf(evStartupNew)); got != 3 {
		t.Errorf("строк журнала %d, want 3", got)
	}
}

// Ловит (RM5b-3): набор наблюдателя меняется только после следующей проверки Windows, а не сразу
// после решения родителя: пока проверка идёт (или висит), выключенная запись запускается.
func TestWatchStartupSetAfterDecision(t *testing.T) {
	src := newStartupSource()
	src.addShared(geogebraEntry("GeoGebra"))
	w := newWatcher()
	h, tok := startupWatchHarness(t, src, w)
	ex1, _ := h.startupLogIn(w)
	misha := h.childID(childSID)
	it := h.startupItem(tok, misha, "GeoGebra")
	block := make(chan struct{})
	src.mu.Lock()
	src.block = block
	src.mu.Unlock()
	h.ok(adminPeer(), tok, ipc.CmdSetStartup, ipc.SetStartupRequest{ChildID: misha, ID: it.ID, On: false}, nil)

	h.real.Advance(time.Second)
	p := w.startCreated(childSession, pathGeoGebra, childSID, ex1, h.real.Now())
	h.watch()
	src.mu.Lock()
	src.block = nil
	src.mu.Unlock()
	close(block)
	h.svc.waitStartup()
	if w.alive(p) {
		t.Error("выключенная общая запись не закрыта, пока идёт проверка")
	}
}

// Ловит (возврат 1 ревью T4b): решение «выключить» или смена команды у записи, чья программа
// запустилась при входе и уже работает, молча закрывает её позже — набор пересобран, а момент
// запуска по-прежнему «в первые 60 с после входа».
func TestWatchStartupRunningNotClosedLater(t *testing.T) {
	cases := []struct {
		name  string
		after time.Duration // через сколько после входа меняется набор
		mod   func(h *harness, src *fakeStartupSource, tok string)
	}{
		{"решение off через 20 с", 20 * time.Second, func(h *harness, src *fakeStartupSource, tok string) {
			h.setStartup(tok, h.childID(childSID), "GeoGebra", false)
		}},
		{"решение off через час", time.Hour, func(h *harness, src *fakeStartupSource, tok string) {
			h.setStartup(tok, h.childID(childSID), "GeoGebra", false)
		}},
		{"смена команды через 10 мин", 10 * time.Minute, func(h *harness, src *fakeStartupSource, tok string) {
			src.mu.Lock()
			src.own[childSID][0].Command += " --hidden" // тот же exe, другая команда
			src.mu.Unlock()
			h.listStartup(tok, h.childID(childSID), true)
			if it := h.startupItem(tok, h.childID(childSID), "GeoGebra"); it.Decision != ipc.StartupPending {
				h.t.Fatalf("GeoGebra = %q, want pending", it.Decision)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := newStartupSource()
			src.addOwn(childSID, geogebraEntry("GeoGebra"))
			w := newWatcher()
			h, _ := startupWatchHarness(t, src, w)
			ex1, _ := h.startupLogIn(w)
			h.real.Advance(5 * time.Second)
			p := w.startCreated(childSession, pathGeoGebra, childSID, ex1, h.real.Now())
			h.watch()
			h.real.Advance(c.after - 5*time.Second)
			c.mod(h, src, h.login(adminPeer()))
			h.watch()
			if !w.alive(p) {
				t.Error("работающая с входа программа закрыта правилом автозагрузки")
			}
			// Её дочерний процесс того же exe, появившийся после решения, тоже оставлен.
			q := w.startCreated(childSession, pathGeoGebra, childSID, p, h.real.Now())
			h.watch()
			if !w.alive(q) {
				t.Error("дочерний процесс оставленной программы закрыт")
			}
			if ev := h.store.eventsOf(evStartupClosed); len(ev) != 0 {
				t.Errorf("журнал: %+v", ev)
			}
		})
	}
}

// Ловит (RM5b-3, страховка от гонки; возврат 1 ревью T4b): программа, запущенная Проводником при
// входе, пока первая проверка автозагрузки ещё идёт, остаётся жить («оставлена» до готовности
// набора) или закрывается, когда проверка закончилась много позже входа.
func TestWatchStartupBeforeFirstCheck(t *testing.T) {
	for _, c := range []struct {
		name   string
		check  time.Duration // когда после входа закончилась первая проверка
		closed bool
	}{
		{"проверка через 2 мин", 2 * time.Minute, true},
		{"проверка через 10 мин", 10 * time.Minute, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := newStartupSource()
			src.addOwn(childSID, runE("Discord", true))
			w := newWatcher()
			h, _ := startupWatchHarness(t, src, w)
			h.childLogsInBoth()
			h.svc.waitStartup() // первая полная проверка: Discord on, граница записана
			src.addOwn(childSID, geogebraEntry("GeoGebra"))
			h.real.Advance(time.Hour)
			block := make(chan struct{})
			src.mu.Lock()
			src.block = block
			src.mu.Unlock()
			// Выход и новый вход Миши; проверка после входа висит.
			h.launcher.setSessions()
			h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
			h.childLogsInBoth()
			ex1 := w.start(childSession, pathExplorer)
			h.real.Advance(3 * time.Second)
			p := w.startCreated(childSession, pathGeoGebra, childSID, ex1, h.real.Now())
			h.watch() // набор ещё прежний: GeoGebra в нём нет
			h.real.Advance(c.check - 3*time.Second)
			src.mu.Lock()
			src.block = nil
			src.mu.Unlock()
			close(block)
			h.svc.waitStartup()
			if it := h.startupItem(h.login(adminPeer()), h.childID(childSID), "GeoGebra"); it.Decision != ipc.StartupPending {
				t.Fatalf("GeoGebra = %q, want pending", it.Decision)
			}
			h.watch()
			if w.alive(p) == c.closed {
				t.Errorf("закрыт = %v, want %v", !w.alive(p), c.closed)
			}
			if n := len(h.store.eventsOf(evStartupClosed)); (n == 1) != c.closed {
				t.Errorf("строк startup_closed %d", n)
			}
		})
	}
}
