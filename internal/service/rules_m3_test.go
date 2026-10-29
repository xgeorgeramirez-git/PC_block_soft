package service

import (
	"strings"
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

// withGames задаёт ребёнку лимит игр games минут на каждый день (до Start).
func withGames(sid string, games int) hopt {
	return func(h *harness) {
		id := h.childID(sid)
		week := h.store.limits[id]
		for i := range week {
			g := games
			week[i].GamesMin = &g
		}
		h.store.SetDayLimits(id, week)
	}
}

// Ловит: игры не доходят до карточки «Сегодня» (8.3.2 п.3), подсказки значка (8.2.1 п.2) и
// «Моей статистики» одними цифрами (Review Focus 5); строка игр есть без лимита игр; число
// «Ждут решения» не из хранилища.
func TestGamesInStatusCardAndMyToday(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withGames(childSID, 60), withChild(otherSID, "Даша", 120))
	misha := h.childID(childSID)
	h.store.days[dayKey(misha, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 1200, GamesS: 1200}
	h.store.AddPendingAttempt(PendingProgram{Path: `c:\a.exe`, Name: "A"}, misha, 1)
	h.store.AddPendingAttempt(PendingProgram{Path: `c:\b.exe`, Name: "B"}, misha, 2)
	h.start()
	tok := h.login(adminPeer())
	var today ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	if len(today.Children) != 2 {
		t.Fatalf("карточек %d", len(today.Children))
	}
	m, d := today.Children[0], today.Children[1]
	if m.GamesText != "Игры 20 мин из 1 ч" || m.PendingPrograms != 2 {
		t.Errorf("карточка Миши: игры %q, ждут %d; want «Игры 20 мин из 1 ч», 2", m.GamesText, m.PendingPrograms)
	}
	if d.GamesText != "" || d.PendingPrograms != 2 {
		t.Errorf("карточка Даши: игры %q, ждут %d; want пусто, 2", d.GamesText, d.PendingPrograms)
	}
	st := h.status()
	if st.GamesRemainingS != 2400 || st.GamesLimitS != 3600 || st.GamesUnlimited || st.Tooltip != "Осталось 1 ч 40 мин\nИгры: 40 мин" {
		t.Errorf("статус значка: %+v", st)
	}
	if m.Status.GamesRemainingS != st.GamesRemainingS || m.Status.Tooltip != st.Tooltip {
		t.Errorf("карточка и значок расходятся: %d/%q и %d/%q", m.Status.GamesRemainingS, m.Status.Tooltip, st.GamesRemainingS, st.Tooltip)
	}
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if my.Status.GamesRemainingS != 2400 {
		t.Errorf("«Моя статистика»: остаток игр %d", my.Status.GamesRemainingS)
	}
	tail := my.Rules[7:]
	if jsonOf(t, tail) != jsonOf(t, []ipc.MyRule{{Text: "Игры: до 1 ч в день"}, {Text: "Когда время вышло: экран блокировки"}}) {
		t.Errorf("общие правила %s", jsonOf(t, tail))
	}
}

// Ловит: «Мои правила» (R21) не говорят о режиме «Оставить только учёбу» и о разных лимитах игр
// по дням; у ребёнка без лимита игр появляется строка игр.
func TestMyRulesGamesByDayAndStudyOnly(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(otherSID, "Даша", 120))
	misha := h.childID(childSID)
	week := h.store.limits[misha]
	g60, g0 := 60, 0
	week[0].GamesMin, week[2].GamesMin, week[5].GamesMin = &g60, &g0, &g60
	h.store.SetDayLimits(misha, week)
	h.store.SetEndMode(misha, "study_only")
	h.start()
	if st := h.status(); st.EndMode != ipc.EndModeStudyOnly {
		t.Errorf("режим в статусе %q, want study_only", st.EndMode)
	}
	tok := h.login(adminPeer())
	var my ipc.MyTodayReply
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: misha}, &my)
	want := []ipc.MyRule{{Text: "Игры: пн до 1 ч, ср нельзя, сб до 1 ч"}, {Text: "Когда время вышло: остаётся только учёба"}}
	if jsonOf(t, my.Rules[7:]) != jsonOf(t, want) {
		t.Errorf("общие правила Миши %s, want %s", jsonOf(t, my.Rules[7:]), jsonOf(t, want))
	}
	// Все дни с лимитом игр, но разным: по дням, не «в день».
	for i := range week {
		g := 30
		if i >= 5 {
			g = 90
		}
		week[i].GamesMin = &g
	}
	h.store.SetDayLimits(misha, week)
	h.ok(adminPeer(), tok, ipc.CmdSaveSchedule, ipc.SaveScheduleRequest{ChildID: misha, Intervals: []ipc.Interval{}}, nil)
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: misha}, &my)
	if got := my.Rules[7].Text; got != "Игры: пн до 30 мин, вт до 30 мин, ср до 30 мин, чт до 30 мин, пт до 30 мин, сб до 1 ч 30 мин, вс до 1 ч 30 мин" {
		t.Errorf("разные лимиты игр: %q", got)
	}
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: h.childID(otherSID)}, &my)
	if jsonOf(t, my.Rules[7:]) != jsonOf(t, []ipc.MyRule{{Text: "Когда время вышло: экран блокировки"}}) {
		t.Errorf("общие правила Даши %s", jsonOf(t, my.Rules[7:]))
	}
}

// Ловит: строка игр «Моих правил» путает «нельзя» и «до N», пропускает дни или говорит «в день»,
// когда игры ограничены не каждый день.
func TestGamesRuleText(t *testing.T) {
	week := func(gs ...*int) core.Rules {
		var r core.Rules
		for i, g := range gs {
			r.Week[i].GamesMin = g
		}
		return r
	}
	z, h1, h2 := 0, 60, 120
	cases := []struct {
		r    core.Rules
		want string
	}{
		{core.Rules{}, ""},
		{week(&z, &z, &z, &z, &z, &z, &z), "Игры: нельзя"},
		{week(&h1, &h1, &h1, &h1, &h1, &h1, &h1), "Игры: до 1 ч в день"},
		{week(&h1, &h1, &h1, &h1, &h1, &h1, nil), "Игры: пн до 1 ч, вт до 1 ч, ср до 1 ч, чт до 1 ч, пт до 1 ч, сб до 1 ч"},
		{week(nil, nil, nil, nil, nil, &z, &h2), "Игры: сб нельзя, вс до 2 ч"},
	}
	for _, c := range cases {
		if got := gamesRuleText(c.r); got != c.want {
			t.Errorf("gamesRuleText = %q, want %q", got, c.want)
		}
	}
}

// Ловит (TODO T3b): команды программ M3 отвечают как настоящие, хотя обработчиков ещё нет, или
// заглушка отвечает отказом по правам. T3b заменяет заглушки и этот тест.
func TestM3ProgramCommandsAreStubs(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	for _, cmd := range []string{ipc.CmdListPrograms, ipc.CmdDecideProgram, ipc.CmdSetProgramsCategory, ipc.CmdUndoProgram} {
		if rep := h.fail(adminPeer(), tok, cmd, nil); rep.Error != textNotReady {
			t.Errorf("%s: %q, want заглушку %q", cmd, rep.Error, textNotReady)
		}
	}
	h2 := newHarness(t)
	h2.start()
	if rep := h2.fail(elevatedPeer(), "", ipc.CmdScanPrograms, nil); rep.Error != textNotReady {
		t.Errorf("ScanPrograms: %q, want заглушку", rep.Error)
	}
}

// Ловит: OpenWindow принимает неизвестный экран окна (RM3-4: допустим только «programs»).
func TestOpenWindowScreen(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	if rep := h.fail(childPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent, Screen: "settings"}); rep.Error != ipc.TextBadRequest {
		t.Errorf("неизвестный экран: %q, want %q", rep.Error, ipc.TextBadRequest)
	}
	h.ok(childPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowParent, Screen: ipc.ScreenPrograms}, nil)
}

// Ловит: путь программы на переднем плане длиннее 520 символов (мусор или атака на память
// службы) принимается в снимок; нормальный путь теряется.
func TestGuardPollForegroundPathLimit(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	cs := h.svc.children[h.childID(childSID)]
	ok := `c:\users\миша\downloads\` + strings.Repeat("я", ipc.MaxForegroundPathLen-24-4) + ".exe"
	s := activeSample
	s.ForegroundPath = ok
	h.poll(s)
	if cs.sample.ForegroundPath != ok {
		t.Errorf("путь %d символов потерян", len([]rune(ok)))
	}
	s.ForegroundPath = ok + "x"
	h.poll(s)
	if cs.sample.ForegroundPath != "" {
		t.Errorf("путь %d символов принят, want «не определён»", len([]rune(s.ForegroundPath)))
	}
}
