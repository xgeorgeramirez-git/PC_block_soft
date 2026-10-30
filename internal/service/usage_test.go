package service

import (
	"fmt"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// fgSample — снимок стража с программой path на переднем плане; Foreground — строка стража.
func fgSample(path, foreground string) ipc.SensorSample {
	return ipc.SensorSample{SessionActive: true, Foreground: foreground, ForegroundPath: path}
}

// usageOf — секунды учёта по программе за день.
func (h *harness) usageOf(day, cat, path string) int {
	id := h.childID(childSID)
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	return h.store.usage[fmt.Sprintf("%d|%s|%s|program|%s", id, day, cat, path)]
}

// Ловит (RM3-1, RM3-7, S-3): категория секунды берётся из слов стража, а не из пути программы на
// переднем плане (страж «games» без пути делает игрой что угодно), незнакомая программа успевает
// засчитаться игрой, учёба входит в общее время, нечитаемый передний план не идёт «Прочим» (PL-7/8).
func TestSampleCategoryFromForegroundPath(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()))
	h.start()
	h.childLogsIn()
	h.setCategory(pathGeoGebra, "study")
	id := h.childID(childSID)
	steps := []struct {
		name          string
		sample        ipc.SensorSample
		usedS, gamesS int
	}{
		{"игра из каталога, страж говорит «прочее»", fgSample(pathMinecraft, ipc.CatNameOther), 10, 10},
		{"учёба не идёт в общее время", fgSample(pathGeoGebra, ipc.CatNameOther), 10, 10},
		{"незнакомая со словом стража «игры» — прочее", fgSample(pathSuperGame, ipc.CatNameGames), 20, 10},
		{"путь не определён, страж «игры» — прочее", fgSample("", ipc.CatNameGames), 30, 10},
		{"система — прочее", fgSample(pathNotepad, ipc.CatNameStudy), 40, 10},
		{"обновление Brave по корню — прочее", fgSample(pathBraveUpd, ipc.CatNameOther), 50, 10},
	}
	for _, k := range steps {
		h.run(10, k.sample)
		st, _ := h.svc.children[id].eng.State()
		if got := h.usedS(); got != k.usedS || st.GamesS != k.gamesS {
			t.Fatalf("%s: общее %d с, игры %d с; want %d, %d", k.name, got, st.GamesS, k.usedS, k.gamesS)
		}
	}
}

// Ловит (RM3-6, S-4): учёт по программам не пишется, пишется не той категорией или с путём не в
// нижнем регистре, системные программы попадают в учёт, сохранение идёт не вместе с состоянием
// дня (SaveDayAndUsage), повторный сброс удваивает секунды.
func TestUsageByProgramSavedWithDay(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()))
	h.start()
	h.childLogsIn()
	h.setCategory(pathGeoGebra, "study")
	h.run(40, fgSample(`C:\Program Files (x86)\Minecraft Launcher\MinecraftLauncher.exe`, ipc.CatNameOther))
	h.run(5, fgSample(pathGeoGebra, ipc.CatNameOther))
	h.run(5, fgSample(pathNotepad, ipc.CatNameOther))
	h.run(5, fgSample(pathBrave, ipc.CatNameOther))
	h.svc.Shutdown(h.real.Now())
	const day = "2026-10-19"
	if g, s, o := h.usageOf(day, "games", pathMinecraft), h.usageOf(day, "study", pathGeoGebra), h.usageOf(day, "other", pathBrave); g != 40 || s != 5 || o != 5 {
		t.Errorf("учёт: игра %d, учёба %d, Brave %d; want 40, 5, 5", g, s, o)
	}
	if n := h.usageOf(day, "other", pathNotepad); n != 0 {
		t.Errorf("система в учёте: %d с", n)
	}
	h.store.mu.Lock()
	logs := len(h.store.usageLog)
	h.store.mu.Unlock()
	if logs < 2 {
		t.Errorf("SaveDayAndUsage вызван %d раз, want сброс по DueSave и при остановке", logs)
	}
}

// Ловит (RM3-6, R18): ошибка записи теряет накопленные секунды (накопитель очищен до успеха) или
// останавливает учёт; после восстановления базы секунды дописываются один раз.
func TestUsageKeptOnSaveError(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()))
	h.start()
	h.childLogsIn()
	h.store.mu.Lock()
	h.store.failSave = true
	h.store.mu.Unlock()
	h.run(35, fgSample(pathMinecraft, ipc.CatNameOther))
	h.store.mu.Lock()
	h.store.failSave = false
	h.store.mu.Unlock()
	h.run(30, fgSample(pathMinecraft, ipc.CatNameOther))
	h.svc.Shutdown(h.real.Now())
	if n := h.usageOf("2026-10-19", "games", pathMinecraft); n != 65 {
		t.Errorf("учёт после сбоя записи: %d с, want 65", n)
	}
}

// Ловит (PL-15, Review Focus 6): секунды накопителя привязаны к дню сброса, а не к дню, когда
// они засчитаны: игра через полночь пишет всё в новый день (или в старый).
func TestUsageAcrossMidnight(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600), withPrograms(typicalScan()))
	h.clock.Set(time.Date(2026, 10, 19, 23, 59, 40, 0, tzMSK))
	h.start()
	h.childLogsIn()
	h.run(40, fgSample(pathMinecraft, ipc.CatNameOther))
	h.svc.Shutdown(h.real.Now())
	mon, tue := h.usageOf("2026-10-19", "games", pathMinecraft), h.usageOf("2026-10-20", "games", pathMinecraft)
	if mon != 19 || tue != 21 {
		t.Errorf("игра через полночь: пн %d с, вт %d с; want 19, 21", mon, tue)
	}
}

// Ловит (RM3-12, Д-1): решение родителя об игре в «Загрузках» (с отпечатком) не засчитывается
// игрой на переднем плане — категория секунды не знает отпечатка, который прочитал наблюдатель;
// подменённый файл засчитывается по старому решению.
func TestForegroundUsesWatcherFingerprint(t *testing.T) {
	meta := newMeta()
	meta.set(pathSuperGame, "", "", "1024:1700000000")
	w := newWatcher()
	h := watchHarness(t, w, withMeta(meta))
	if err := h.store.UpsertProgram(Program{Path: pathSuperGame, Name: "SuperGame", Category: "games", Source: "parent", Fingerprint: "1024:1700000000"}); err != nil {
		t.Fatal(err)
	}
	h.svc.mu.Lock()
	h.svc.reloadPrograms()
	h.svc.mu.Unlock()
	id := h.childID(childSID)
	game := w.start(childSession, pathSuperGame)
	h.watch()
	h.run(5, fgSample(pathSuperGame, ipc.CatNameOther))
	if st, _ := h.svc.children[id].eng.State(); !w.alive(game) || st.GamesS != 5 {
		t.Fatalf("разрешённая игра с верным отпечатком: жива %v, игры %d с; want true, 5", w.alive(game), st.GamesS)
	}
	meta.set(pathSuperGame, "", "", "2048:1700000500") // ребёнок подменил файл
	w.exit(game)
	game2 := w.start(childSession, pathSuperGame)
	h.watch()
	h.run(5, fgSample(pathSuperGame, ipc.CatNameOther))
	if st, _ := h.svc.children[id].eng.State(); w.alive(game2) || st.GamesS != 5 || h.usedS() != 10 {
		t.Errorf("подменённый файл: жив %v, игры %d с, общее %d; want закрыт, 5, 10", w.alive(game2), st.GamesS, h.usedS())
	}
}
