package service

import (
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// myTodayPrograms — строки «Итогов» за сегодня в окне ребёнка.
func (h *harness) myTodayPrograms() []ipc.ProgramTime {
	h.t.Helper()
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	return my.Programs
}

// sumByCat — сумма строк по категориям (PL-28: сумма строк категории равна её итогу).
func sumByCat(rows []ipc.ProgramTime) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		out[r.Category] += r.Seconds
	}
	return out
}

// Ловит (RM4-7, PL-38, PL-28, S-8): строки сайтов не показываются или идут видом «программа»,
// сайт, запрещённый сейчас, виден строкой, перенос из «Видео» в «Учёбу» прячет прошлые секунды,
// сбой чтения списков показывает сайты по устаревшим спискам, сумма строк не равна итогу.
func TestStatsSiteRows(t *testing.T) {
	h2 := siteHarness(t, func(h *harness) {
		id := h.childID(childSID)
		h.store.days[dayKey(id, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 1180, VideoS: 760}
		h.store.AddUsage(id, "2026-10-19", "video_social", "site", "youtube.com", 600)
		h.store.AddUsage(id, "2026-10-19", "video_social", "site", "example.com", 60)     // записан до запрета
		h.store.AddUsage(id, "2026-10-19", "video_social", "site", "school.example", 100) // тогда было видео, сейчас «Учёба»
		h.store.AddUsage(id, "2026-10-19", "other", "program", pathBrave, 300)
		h.store.AddUsage(id, "2026-10-19", "other", "site", "news.example", 120)
	})
	want := []ipc.ProgramTime{
		{Path: "youtube.com", Name: "youtube.com", Category: "video_social", Seconds: 600, Kind: "site"},
		{Path: "school.example", Name: "school.example", Category: "video_social", Seconds: 100, Kind: "site"},
		{Name: textOtherTime, Category: "video_social", Seconds: 60, Kind: ""},
		{Path: pathBrave, Name: "Brave", Category: "other", Seconds: 300, Kind: "program"},
		{Path: "news.example", Name: "news.example", Category: "other", Seconds: 120, Kind: "site"},
	}
	got := h2.myTodayPrograms()
	if jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("строки\n%s\nwant\n%s", jsonOf(t, got), jsonOf(t, want))
	}
	if s := sumByCat(got); s["video_social"] != 760 || s["other"] != 420 {
		t.Errorf("суммы строк %v, want видео 760, прочее 420", s)
	}

	// Списки не прочитаны: все сайты — в «Остальное время» своей категории (безопасный вид).
	h2.store.mu.Lock()
	h2.store.failSites = true
	h2.store.mu.Unlock()
	h2.svc.mu.Lock()
	h2.svc.reloadSites(h2.real.Now())
	h2.svc.mu.Unlock()
	want = []ipc.ProgramTime{
		{Name: textOtherTime, Category: "video_social", Seconds: 760, Kind: ""},
		{Path: pathBrave, Name: "Brave", Category: "other", Seconds: 300, Kind: "program"},
		{Name: textOtherTime, Category: "other", Seconds: 120, Kind: ""},
	}
	if got := h2.myTodayPrograms(); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("без списков строки\n%s\nwant\n%s", jsonOf(t, got), jsonOf(t, want))
	}
}

// Ловит (S-8, PL-38): строка сайта и строка программы с одинаковым предметом сливаются в одну;
// сайт из «18+» при включённом переключателе ребёнка виден строкой.
func TestStatsSiteAndProgramSameSubject(t *testing.T) {
	h2 := siteHarness(t, func(h *harness) {
		id := h.childID(childSID)
		h.store.days[dayKey(id, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 90}
		h.store.AddUsage(id, "2026-10-19", "other", "program", `c:\x`, 40)
		h.store.AddUsage(id, "2026-10-19", "other", "site", `c:\x`, 30)
		h.store.AddUsage(id, "2026-10-19", "other", "site", "adult-probe.test", 20)
	})
	want := []ipc.ProgramTime{
		{Path: `c:\x`, Name: "x", Category: "other", Seconds: 40, Kind: "program"},
		{Path: `c:\x`, Name: `c:\x`, Category: "other", Seconds: 30, Kind: "site"},
		{Name: textOtherTime, Category: "other", Seconds: 20, Kind: ""},
	}
	if got := h2.myTodayPrograms(); jsonOf(t, got) != jsonOf(t, want) {
		t.Errorf("строки\n%s\nwant\n%s", jsonOf(t, got), jsonOf(t, want))
	}
}
