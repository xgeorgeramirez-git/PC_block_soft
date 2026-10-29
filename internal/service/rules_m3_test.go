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
		out = append(out, ipc.WeekdayLimit{Weekday: d, TotalMin: total, GamesUnlimited: true})
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

// Ловит: лимит игр по дням (R5, FR-16) и режим «Когда время вышло» (R8, FR-18) не сохраняются
// через SaveLimits, не возвращаются в правилах, не действуют сразу (FR-19) или не пишутся в
// журнал словами родителю; пустой режим меняет сохранённый.
func TestSaveLimitsGamesAndEndMode(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	limits := sevenDays(120)
	limits[0].GamesUnlimited, limits[0].GamesMin = false, 60  // пн: игры 1 ч
	limits[2].GamesUnlimited, limits[2].GamesMin = false, 0   // ср: игр нельзя
	limits[5].GamesUnlimited, limits[5].GamesMin = false, 300 // сб: больше общего — можно
	var rep ipc.SaveLimitsReply
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: limits, EndMode: ipc.EndModeStudyOnly}, &rep)
	if rep.Rules.EndMode != ipc.EndModeStudyOnly {
		t.Errorf("режим в ответе %q, want study_only", rep.Rules.EndMode)
	}
	want := []ipc.WeekdayLimit{
		{Weekday: 1, TotalMin: 120, GamesMin: 60}, {Weekday: 2, TotalMin: 120, GamesUnlimited: true},
		{Weekday: 3, TotalMin: 120, GamesMin: 0}, {Weekday: 4, TotalMin: 120, GamesUnlimited: true},
		{Weekday: 5, TotalMin: 120, GamesUnlimited: true}, {Weekday: 6, TotalMin: 120, GamesMin: 300},
		{Weekday: 7, TotalMin: 120, GamesUnlimited: true},
	}
	if jsonOf(t, rep.Rules.Limits) != jsonOf(t, want) {
		t.Errorf("лимиты в ответе %s, want %s", jsonOf(t, rep.Rules.Limits), jsonOf(t, want))
	}
	if m, _ := h.store.GetEndMode(id); m != "study_only" {
		t.Errorf("режим в базе %q, want study_only", m)
	}
	if g := h.store.limits[id][0].GamesMin; g == nil || *g != 60 {
		t.Errorf("игры пн в базе %v, want 60", g)
	}
	if cs := h.svc.children[id]; cs.rules.EndMode != core.EndStudyOnly || cs.eng.Current().GamesLimitS != 3600 {
		t.Errorf("правила в памяти: режим %v, лимит игр %d с; want study_only, 3600", cs.rules.EndMode, cs.eng.Current().GamesLimitS)
	}
	texts := []string{}
	for _, e := range h.store.eventsOf("settings_changed") {
		texts = append(texts, e.Details["text"].(string))
	}
	if jsonOf(t, texts) != jsonOf(t, []string{"изменены лимиты времени", "изменён режим, когда время вышло"}) {
		t.Errorf("журнал %v", texts)
	}

	// Пустой режим не меняет сохранённый; режим без изменения не пишется в журнал.
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: limits}, &rep)
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: limits, EndMode: ipc.EndModeStudyOnly}, &rep)
	if m, _ := h.store.GetEndMode(id); m != "study_only" || rep.Rules.EndMode != ipc.EndModeStudyOnly {
		t.Errorf("после сохранения без режима: %q / %q, want study_only", m, rep.Rules.EndMode)
	}
	if n := len(h.store.eventsOf("settings_changed")); n != 4 {
		t.Errorf("записей журнала %d, want 4 (лимиты ×3, режим ×1)", n)
	}

	// Только режим: лимиты не нужны и не меняются.
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, EndMode: ipc.EndModeLock}, &rep)
	if m, _ := h.store.GetEndMode(id); m != "lock" {
		t.Errorf("режим после смены одного режима %q, want lock", m)
	}
	if jsonOf(t, rep.Rules.Limits) != jsonOf(t, want) {
		t.Errorf("лимиты после смены режима %s, want прежние", jsonOf(t, rep.Rules.Limits))
	}
	evs := h.store.eventsOf("settings_changed")
	if last := evs[len(evs)-1].Details["text"]; last != "изменён режим, когда время вышло" || len(evs) != 5 {
		t.Errorf("журнал после смены режима: %d записей, последняя %v", len(evs), last)
	}
}

// Ловит: лимит игр вне 0…1440 или неизвестный режим доходят до базы, ошибка не у поля (L-121),
// или отказ всё-таки меняет правила.
func TestSaveLimitsRejectsBadGamesAndEndMode(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	bad := func(edit func([]ipc.WeekdayLimit), mode, text, field string) {
		t.Helper()
		l := sevenDays(60)
		edit(l)
		rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: l, EndMode: mode})
		if rep.Error != text || rep.Field != field {
			t.Errorf("отказ %q у %q, want %q у %q", rep.Error, rep.Field, text, field)
		}
	}
	bad(func(l []ipc.WeekdayLimit) { l[3].GamesUnlimited, l[3].GamesMin = false, 1441 }, "", "Не больше 24 ч", "games.4")
	bad(func(l []ipc.WeekdayLimit) { l[6].GamesUnlimited, l[6].GamesMin = false, -1 }, "", "Не меньше 0 мин", "games.7")
	bad(func([]ipc.WeekdayLimit) {}, "kill", "Выберите, что делать, когда время вышло", ipc.FieldEndMode)
	if rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id}); rep.Field != "limits" {
		t.Errorf("пустой запрос: %+v, want отказ у limits", rep)
	}
	if g := h.store.limits[id][0]; *g.TotalMin != 120 || g.GamesMin != nil {
		t.Errorf("после отказов лимиты изменены: %v/%v", *g.TotalMin, g.GamesMin)
	}
	if m, _ := h.store.GetEndMode(id); m != "lock" {
		t.Errorf("после отказов режим %q", m)
	}
}

// Ловит: «Скопировать правила с…» (FR-33) переносит только общий лимит: игры и режим «Когда
// время вышло» у приёмника остаются прежними.
func TestCopyRulesCarriesGamesAndEndMode(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(otherSID, "Даша", 90))
	src := h.childID(otherSID)
	games := 45
	week := h.store.limits[src]
	week[4].GamesMin = &games
	h.store.SetDayLimits(src, week)
	h.store.SetEndMode(src, "study_only")
	h.start()
	tok := h.login(adminPeer())
	var rep ipc.CopyRulesReply
	h.ok(adminPeer(), tok, ipc.CmdCopyRules, ipc.CopyRulesRequest{FromChildID: src, ToChildID: h.childID(childSID)}, &rep)
	if rep.EndMode != ipc.EndModeStudyOnly {
		t.Errorf("режим %q, want study_only", rep.EndMode)
	}
	if l := rep.Limits[4]; l.TotalMin != 90 || l.GamesMin != 45 || l.GamesUnlimited {
		t.Errorf("пятница %+v, want 90 и игры 45", l)
	}
	if l := rep.Limits[0]; !l.GamesUnlimited || l.GamesMin != 0 {
		t.Errorf("понедельник %+v, want игры без лимита", l)
	}
}

// Ловит: мастер (Setup, FR-2) теряет лимит игр детей или ставит «игры нельзя» тем, у кого его нет.
func TestSetupStoresGamesLimits(t *testing.T) {
	h := newHarness(t)
	h.start()
	limits := sevenDays(120)
	limits[6].GamesUnlimited, limits[6].GamesMin = false, 90
	h.ok(elevatedPeer(), "", ipc.CmdSetup, ipc.SetupRequest{Password: goodPassword, Children: []ipc.SetupChild{{SID: childSID, Name: "Миша", Limits: limits}}}, nil)
	week := h.store.limits[h.childID(childSID)]
	if week[0].GamesMin != nil {
		t.Errorf("пн игры %v, want без отдельного лимита", *week[0].GamesMin)
	}
	if week[6].GamesMin == nil || *week[6].GamesMin != 90 {
		t.Errorf("вс игры %v, want 90", week[6].GamesMin)
	}
}
