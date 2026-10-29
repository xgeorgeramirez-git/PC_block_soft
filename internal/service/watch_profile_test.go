package service

import (
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Программы в профиле Миши (RM3-13).
const (
	pathRoblox111 = `c:\users\миша\appdata\local\roblox\versions\version-111\robloxplayerbeta.exe`
	pathRoblox222 = `c:\users\миша\appdata\local\roblox\versions\version-222\robloxplayerbeta.exe`
	pathRoblox333 = `c:\users\миша\appdata\local\roblox\versions\version-333\robloxplayerbeta.exe`
	pathSpotify   = `c:\users\миша\appdata\roaming\spotify\spotify.exe`
	pathDefender  = `c:\programdata\microsoft\windows defender\platform\4.18.26090.9-0\defendersessionhelper.exe`
	robloxCorp    = "Roblox Corporation"
)

// countOf — сколько раз путь встречается в списке.
func countOf(list []string, path string) int {
	n := 0
	for _, p := range list {
		if p == path {
			n++
		}
	}
	return n
}

// Ловит (RM3-13): обновление Roblox в новую папку версии закрывается как незнакомое; подменённый
// exe того же имени без издателя проходит; издатель читается на каждом проходе (опрос медленнее)
// или для программ без решений в профиле; Защитник закрыт; секунда новой версии не игра.
func TestWatchProfileUpdateKeepsDecision(t *testing.T) {
	w := newWatcher()
	meta := newMeta()
	meta.set(pathRoblox111, robloxCorp, "Roblox", "10:1")
	meta.set(pathRoblox222, robloxCorp, "Roblox", "11:2")
	meta.set(pathRoblox333, "", "", "12:3") // ребёнок положил свою игру под именем Roblox
	h := watchHarness(t, w, withMeta(meta))
	if err := h.store.UpsertProgram(Program{Path: pathRoblox111, Name: "Roblox", Publisher: robloxCorp, Category: "games",
		Source: "parent", Fingerprint: "10:1", AppRoot: `c:\users\миша\appdata\local\roblox`, DecidedAt: 1}); err != nil {
		t.Fatal(err)
	}
	h.svc.mu.Lock()
	h.svc.reloadPrograms()
	h.svc.mu.Unlock()
	procs := startAll(w, pathRoblox222, pathRoblox333, pathSpotify, pathDefender, pathBrave)
	for i := 0; i < 3; i++ {
		h.watch()
	}
	if got, want := sortedStrings(w.killedPaths()), sortedStrings([]string{pathRoblox333, pathSpotify}); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("закрыты %v, want %v", got, want)
	}
	for _, p := range []string{pathRoblox222, pathDefender, pathBrave} {
		if !w.alive(procs[p]) {
			t.Errorf("закрыта %s", p)
		}
	}
	asked := meta.askedPaths()
	if n := countOf(asked, pathRoblox222); n != 1 {
		t.Errorf("издатель новой версии прочитан %d раз за 3 прохода, want 1 (кэш по процессу)", n)
	}
	for _, p := range []string{pathBrave, pathDefender} {
		if n := countOf(asked, p); n != 0 {
			t.Errorf("свойства %s прочитаны %d раз: издатель нужен только в профиле с решениями", p, n)
		}
	}
	h.svc.mu.Lock()
	cat, _ := h.svc.foregroundOf(ipc.SensorSample{SessionActive: true, ForegroundPath: pathRoblox222})
	h.svc.mu.Unlock()
	if cat != core.CatGames {
		t.Errorf("секунда новой версии Roblox: %v, want игры", cat)
	}
}

// Ловит (RM3-13): решение родителя по программе в профиле не запоминает корень профиля и
// издателя: следующая версия снова «ждёт решения».
func TestDecideProfileProgramStoresRootAndPublisher(t *testing.T) {
	h := newProgHarness(t)
	h.meta.set(pathRoblox111, robloxCorp, "Roblox", "10:1")
	h.store.AddPendingAttempt(PendingProgram{Path: pathRoblox111, Name: "Roblox", Publisher: robloxCorp}, h.misha, h.real.Now().UnixMilli())
	h.decide(pathRoblox111, "games")
	p, _, _ := h.store.GetProgram(pathRoblox111)
	if p.AppRoot != `c:\users\миша\appdata\local\roblox` || p.Publisher != robloxCorp || p.Fingerprint != "10:1" {
		t.Errorf("решение в профиле: %+v", p)
	}
	h.svc.mu.Lock()
	r := h.svc.classifier.Classify(`c:\users\коля\appdata\local\roblox\versions\version-9\robloxplayerbeta.exe`, "", robloxCorp)
	h.svc.mu.Unlock()
	if r.Verdict != programs.VerdictAllow || r.Category != "games" {
		t.Errorf("новая версия у Коли: %+v", r)
	}
}
