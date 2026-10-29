package service

import (
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// sevenDays — лимиты всех семи дней для SaveLimits: общий total минут, игры без отдельного лимита.
func sevenDays(total int) []ipc.WeekdayLimit {
	out := make([]ipc.WeekdayLimit, 0, 7)
	for d := 1; d <= 7; d++ {
		out = append(out, ipc.WeekdayLimit{Weekday: d, TotalMin: total})
	}
	return out
}

// Ловит (RM3-9, найденная ошибка): сохранение одних лимитов пересобирает правила службы без
// расписания, и запретные часы перестают действовать до перезапуска службы (Review Focus 4:
// частичный путь сохранения теряет соседние поля).
func TestSaveLimitsKeepsForbiddenHours(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	id := h.childID(childSID)
	// Понедельник 14:00–16:00 запрещён, сейчас 15:00.
	if err := h.store.SetSchedule(id, []core.Interval{{Weekday: core.Monday, StartMin: 14 * 60, EndMin: 16 * 60}}); err != nil {
		t.Fatal(err)
	}
	h.start()
	if st := h.status(); st.Status != ipc.StatusForbidden || st.Lock != ipc.LockForbidden {
		t.Fatalf("до сохранения: статус %q, экран %q; want forbidden", st.Status, st.Lock)
	}
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: sevenDays(90)}, nil)
	if st := h.status(); st.Status != ipc.StatusForbidden || st.Lock != ipc.LockForbidden {
		t.Fatalf("после SaveLimits: статус %q, экран %q; want forbidden (запретные часы потеряны)", st.Status, st.Lock)
	}
	h.childLogsIn()
	h.step(activeSample)
	if st := h.status(); st.Lock != ipc.LockForbidden {
		t.Fatalf("после тика: экран %q; want forbidden", st.Lock)
	}
}

// Ловит: служба не передаёт ядру лимит игр и режим «Когда время вышло» из базы (readRules
// собирает Rules не целиком: R5, R8), и они не действуют ни при старте, ни после SaveSchedule.
func TestReadRulesCarriesGamesAndEndMode(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	id := h.childID(childSID)
	games := 30
	var week [7]core.DayLimit
	for i := range week {
		week[i] = core.LimitOf(0)
		week[i].GamesMin = &games
	}
	if err := h.store.SetDayLimits(id, week); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetEndMode(id, "study_only"); err != nil {
		t.Fatal(err)
	}
	h.start()
	cs := h.svc.children[id]
	r := cs.rules
	if r.EndMode != core.EndStudyOnly || r.For(core.Monday).GamesMin == nil || *r.For(core.Monday).GamesMin != 30 {
		t.Fatalf("правила после старта: режим %v, игры %v; want study_only и 30", r.EndMode, r.For(core.Monday).GamesMin)
	}
	d := cs.eng.Current()
	if d.Lock != core.LockNone || !d.StudyOnly {
		t.Fatalf("время вышло в режиме «только учёба»: Lock %v, StudyOnly %v; want экрана нет, только учёба", d.Lock, d.StudyOnly)
	}
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdSaveSchedule, ipc.SaveScheduleRequest{ChildID: id, Intervals: []ipc.Interval{}}, nil)
	if r := h.svc.children[id].rules; r.EndMode != core.EndStudyOnly || r.For(core.Monday).GamesMin == nil {
		t.Fatalf("после SaveSchedule режим или игры потеряны: %+v", r)
	}
}

// Ловит: режим «Когда время вышло» не прочитан, а служба всё равно применяет правила (R18):
// ошибка чтения любой части правил — безопасный режим, как для лимитов и расписания.
func TestReadRulesEndModeErrorIsBroken(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.store.failEndMode = true
	h.start()
	if st := h.status(); st.Status != ipc.StatusSafe {
		t.Fatalf("статус %q, want safe (правила не прочитаны)", st.Status)
	}
}
