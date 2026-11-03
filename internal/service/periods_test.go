package service

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Периоды «Итогов» (FB-13, D-61, RF13-1…RF13-3, S42): неделя с понедельника, месяц с 1-го, год с
// 1 января, всё время; все по местному времени и с сегодняшним днём; прошлые дни из базы, сегодня из
// счётчиков ядра, без двойного счёта сегодняшнего дня.

// Ловит: неделя с воскресенья или с «7 дней назад», а не с понедельника; граница по UTC, а не по
// местному времени (в 00:30 МСК 1 ноября по UTC ещё 31 октября); месяц или год со вчерашнего дня;
// неизвестный период принят молча; пустой период не равен «Сегодня».
func TestPeriodFrom(t *testing.T) {
	msk := func(y int, m time.Month, d, hh, mm, ss int) time.Time {
		return time.Date(y, m, d, hh, mm, ss, 0, tzMSK)
	}
	cases := []struct {
		what   string
		now    time.Time
		period string
		want   string
	}{
		{"среда: сегодня", msk(2026, 10, 21, 15, 0, 0), ipc.PeriodToday, "2026-10-21"},
		{"пустой — сегодня", msk(2026, 10, 21, 15, 0, 0), "", "2026-10-21"},
		{"среда: неделя с понедельника", msk(2026, 10, 21, 15, 0, 0), ipc.PeriodWeek, "2026-10-19"},
		{"среда: месяц с 1-го", msk(2026, 10, 21, 15, 0, 0), ipc.PeriodMonth, "2026-10-01"},
		{"среда: год с 1 января", msk(2026, 10, 21, 15, 0, 0), ipc.PeriodYear, "2026-01-01"},
		{"среда: всё время", msk(2026, 10, 21, 15, 0, 0), ipc.PeriodAll, allFromDay},
		{"понедельник 00:00: неделя с сегодня", msk(2026, 10, 19, 0, 0, 0), ipc.PeriodWeek, "2026-10-19"},
		{"воскресенье 23:59: неделя с прошлого понедельника", msk(2026, 10, 25, 23, 59, 59), ipc.PeriodWeek, "2026-10-19"},
		{"неделя через границу месяца", msk(2026, 11, 1, 12, 0, 0), ipc.PeriodWeek, "2026-10-26"},
		// 2026-10-31 21:30 UTC = 2026-11-01 00:30 МСК: по местному времени уже ноябрь.
		{"полночь по местному: месяц", time.Date(2026, 10, 31, 21, 30, 0, 0, time.UTC).In(tzMSK), ipc.PeriodMonth, "2026-11-01"},
		{"полночь по местному: сегодня", time.Date(2026, 10, 31, 21, 30, 0, 0, time.UTC).In(tzMSK), ipc.PeriodToday, "2026-11-01"},
		// 2025-12-31 21:00 UTC = 2026-01-01 00:00 МСК.
		{"1 января 00:00: год", time.Date(2025, 12, 31, 21, 0, 0, 0, time.UTC).In(tzMSK), ipc.PeriodYear, "2026-01-01"},
		{"1 января: неделя с понедельника прошлого года", msk(2026, 1, 1, 10, 0, 0), ipc.PeriodWeek, "2025-12-29"},
	}
	for _, c := range cases {
		got, ok := periodFrom(c.period, c.now)
		if !ok || got != c.want {
			t.Errorf("%s: periodFrom(%q, %s) = %q, %v; want %q", c.what, c.period, c.now, got, ok, c.want)
		}
	}
	if got, ok := periodFrom("decade", msk(2026, 10, 21, 15, 0, 0)); ok {
		t.Errorf("неизвестный период принят: %q", got)
	}
}

// Ловит: в «из копилки» попали начисления, обнуление родителем или трата до начала периода; возврат
// неприменённой траты не вычтен.
func TestBonusSpentSince(t *testing.T) {
	from := time.Date(2026, 10, 19, 0, 0, 0, 0, tzMSK).UnixMilli()
	list := []BonusRecord{ // новые первыми, как ListBonus
		{AtMs: from + 5000, DeltaMin: 10, Reason: reasonBonusRefund},
		{AtMs: from + 4000, DeltaMin: -10, Reason: reasonBonusSpent},
		{AtMs: from + 3000, DeltaMin: -20, Reason: reasonBonusSpent},
		{AtMs: from + 2000, DeltaMin: -5, Reason: "Убавлено родителем"},
		{AtMs: from + 1000, DeltaMin: 60, Reason: "Бонус за свободное время: 60 мин"},
		{AtMs: from, DeltaMin: -15, Reason: reasonBonusSpent},
		{AtMs: from - 1, DeltaMin: -30, Reason: reasonBonusSpent},
	}
	if got := bonusSpentSince(list, from); got != 35 {
		t.Errorf("взято из копилки %d мин, want 35 (15 + 20 + 10 − 10 возврат)", got)
	}
	if got := bonusSpentSince(nil, from); got != 0 {
		t.Errorf("пустая история: %d", got)
	}
}

// wed1500 — среда 21.10.2026 15:00: понедельник этой недели (19.10) уже прошёл, 1-е число и 2 января —
// раньше недели и месяца (S42 на заранее известной дате).
var wed1500 = time.Date(2026, 10, 21, 15, 0, 0, 0, tzMSK)

// withClockAt ставит часы ядра на момент at до запуска службы.
func withClockAt(at time.Time) hopt {
	return func(h *harness) { h.clock.Set(at) }
}

// seedS42 кладёт в базу прошлые дни S42: пн этой недели игры 30 мин (Minecraft), 1-е число игры 20
// мин, 2 января учёба 40 мин (GeoGebra), прошлый год прочее 50 мин без программы; и строку «завтра»
// (часы могли уйти назад), которой в периодах быть не должно.
func seedS42(t *testing.T, h *harness, id int64) {
	t.Helper()
	seed := func(st core.DayState, rows ...UsageRow) {
		if err := h.store.SaveDayAndUsage(id, st, rows); err != nil {
			t.Fatal(err)
		}
	}
	seed(core.DayState{Day: "2026-10-19", TotalS: 1800, GamesS: 1800, ExtraMin: 30},
		UsageRow{Category: "games", Kind: usageKindProgram, Subject: pathMinecraft, Seconds: 1800})
	seed(core.DayState{Day: "2026-10-01", TotalS: 1200, GamesS: 1200},
		UsageRow{Category: "games", Kind: usageKindProgram, Subject: pathMinecraft, Seconds: 1200})
	seed(core.DayState{Day: "2026-01-02", StudyS: 2400},
		UsageRow{Category: "study", Kind: usageKindProgram, Subject: pathGeoGebra, Seconds: 2400})
	seed(core.DayState{Day: "2025-06-15", TotalS: 3000, ExtraMin: 15})
	seed(core.DayState{Day: "2026-10-22", TotalS: 999, GamesS: 999},
		UsageRow{Category: "games", Kind: usageKindProgram, Subject: pathMinecraft, Seconds: 999})
}

// periodReply — MyToday ребёнка за период.
func (h *harness) periodReply(period string) ipc.MyTodayReply {
	h.t.Helper()
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, ipc.MyTodayRequest{Period: period}, &my)
	return my
}

// Ловит: период не меняет итоги (всегда сегодня); прошлые дни не попадают; сегодняшний день считается
// дважды (из базы, куда служба его уже сохранила, и из ядра); неделя тянет 1-е число, месяц —
// 2 января; «будущий» день из базы попадает в период; строки программ за период не сходятся с итогом
// вкладки или нет «Остальное время» за прошлый год; доп. время за период не суммируется; родитель
// видит иное, чем ребёнок; ответ не называет период.
func TestMyTodayPeriodsS42(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()), withClockAt(wed1500))
	h.start()
	h.childLogsIn()
	h.setCategory(pathGeoGebra, "study")
	id := h.childID(childSID)
	seedS42(t, h, id)
	h.run(600, fgSample(pathMinecraft, ipc.CatNameOther)) // сегодня игры 10 мин; служба сохраняет день в базу
	if st, _ := h.store.LoadDayState(id, "2026-10-21"); st.TotalS == 0 {
		t.Fatalf("сегодняшний день не сохранён в базу: проверка двойного счёта ничего не проверяет")
	}

	type want struct {
		used, games, study, other, all, extra int
		games1, study1, rest                  string
	}
	cases := map[string]want{
		ipc.PeriodToday: {600, 600, 0, 0, 600, 0, "Minecraft Launcher=600", "", ""},
		ipc.PeriodWeek:  {2400, 2400, 0, 0, 2400, 30, "Minecraft Launcher=2400", "", ""},
		ipc.PeriodMonth: {3600, 3600, 0, 0, 3600, 30, "Minecraft Launcher=3600", "", ""},
		ipc.PeriodYear:  {3600, 3600, 2400, 0, 6000, 30, "Minecraft Launcher=3600", "GeoGebra Classic=2400", ""},
		ipc.PeriodAll:   {6600, 3600, 2400, 3000, 9000, 45, "Minecraft Launcher=3600", "GeoGebra Classic=2400", "Остальное время=3000"},
	}
	tok := h.login(adminPeer())
	for _, period := range []string{ipc.PeriodToday, ipc.PeriodWeek, ipc.PeriodMonth, ipc.PeriodYear, ipc.PeriodAll} {
		w := cases[period]
		my := h.periodReply(period)
		if my.Period != period {
			t.Errorf("%s: в ответе период %q", period, my.Period)
		}
		if my.UsedS != w.used || my.GamesS != w.games || my.StudyS != w.study || my.OtherS != w.other || my.AllS != w.all || my.ExtraMin != w.extra {
			t.Errorf("%s: использовано %d, игры %d, учёба %d, прочее %d, всего %d, доп. %d мин; want %d, %d, %d, %d, %d, %d",
				period, my.UsedS, my.GamesS, my.StudyS, my.OtherS, my.AllS, my.ExtraMin, w.used, w.games, w.study, w.other, w.all, w.extra)
		}
		if got := strings.Join(rowsOf(my.Programs, "games"), ", "); got != w.games1 {
			t.Errorf("%s: игры %q, want %q", period, got, w.games1)
		}
		if got := strings.Join(rowsOf(my.Programs, "study"), ", "); got != w.study1 {
			t.Errorf("%s: учёба %q, want %q", period, got, w.study1)
		}
		if got := strings.Join(rowsOf(my.Programs, "other"), ", "); got != w.rest {
			t.Errorf("%s: прочее %q, want %q", period, got, w.rest)
		}
		for cat, total := range map[string]int{"games": my.GamesS, "study": my.StudyS, "other": my.OtherS} {
			if s := sumOf(my.Programs, cat); s != total {
				t.Errorf("%s: сумма строк %s %d, итог %d", period, cat, s, total)
			}
		}
		// Остаток и «Сегодня» от периода не зависят (8.2.6 п.2).
		if my.Status.UsedS != 600 || len(my.Today) == 0 || my.Today[1] != "Использовано 10 мин" {
			t.Errorf("%s: статус за сегодня изменился: usedS %d, сегодня %q", period, my.Status.UsedS, my.Today)
		}
		var ps ipc.MyTodayReply
		h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id, Period: period}, &ps)
		if jsonOf(t, ps.Programs) != jsonOf(t, my.Programs) || ps.AllS != my.AllS || ps.UsedS != my.UsedS || ps.Period != period {
			t.Errorf("%s: родитель видит иное:\n%s\nребёнок:\n%s", period, jsonOf(t, ps), jsonOf(t, my))
		}
	}

	// Старый вызов без периода — сегодня (окно прежней версии).
	var old ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &old)
	if old.Period != ipc.PeriodToday || old.GamesS != 600 || old.AllS != 600 {
		t.Errorf("без периода: период %q, игры %d, всего %d; want today, 600, 600", old.Period, old.GamesS, old.AllS)
	}
	// Неизвестный период — отказ, а не «сегодня» молча.
	if rep := h.do(childPeer(), "", ipc.CmdMyToday, ipc.MyTodayRequest{Period: "decade"}); rep.OK {
		t.Errorf("неизвестный период принят: %s", rep.Payload)
	}
	if rep := h.do(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id, Period: "decade"}); rep.OK {
		t.Errorf("ParentStats: неизвестный период принят: %s", rep.Payload)
	}
}

// Ловит: скрытая программа (D-57) видна по имени в периоде (только в «Сегодня» скрыта); своё название
// (D-59) не применяется к прошлым дням; за период без данных — null вместо [].
func TestMyTodayPeriodHiddenAndNames(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()), withClockAt(wed1500))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	if rep := h.do(childPeer(), "", ipc.CmdMyToday, ipc.MyTodayRequest{Period: ipc.PeriodAll}); !strings.Contains(string(rep.Payload), `"programs":[]`) {
		t.Errorf("пустое «Всё время»: %s", rep.Payload)
	}
	seedS42(t, h, id)
	tok := h.login(adminPeer())
	h.rename(tok, pathMinecraft, "Майнкрафт")
	if got := strings.Join(rowsOf(h.periodReply(ipc.PeriodWeek).Programs, "games"), ", "); got != "Майнкрафт=1800" {
		t.Errorf("своё название за неделю: %q", got)
	}
	h.hide(tok, true, pathMinecraft)
	my := h.periodReply(ipc.PeriodMonth)
	if got := strings.Join(rowsOf(my.Programs, "games"), ", "); got != "Остальное время=3000" {
		t.Errorf("скрытая за месяц: %q, want «Остальное время=3000»", got)
	}
}

// Ловит: «из копилки» за период не считается или берёт траты прошлой недели; доп. время сегодня не
// входит в итог периода.
func TestMyTodayPeriodBonusSpent(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()), withClockAt(wed1500))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	seedS42(t, h, id)
	h.store.bonus[id] = []BonusRecord{ // старые первыми, как пишет fake
		{AtMs: time.Date(2026, 10, 12, 10, 0, 0, 0, tzMSK).UnixMilli(), DeltaMin: 60, Reason: "Бонус", Balance: 60},
		{AtMs: time.Date(2026, 10, 12, 11, 0, 0, 0, tzMSK).UnixMilli(), DeltaMin: -20, Reason: reasonBonusSpent, Balance: 40},
		{AtMs: time.Date(2026, 10, 20, 11, 0, 0, 0, tzMSK).UnixMilli(), DeltaMin: -15, Reason: reasonBonusSpent, Balance: 25},
	}
	h.ok(childPeer(), "", ipc.CmdSpendBonus, ipc.SpendBonusRequest{Minutes: 10}, nil)
	today, week, month := h.periodReply(ipc.PeriodToday), h.periodReply(ipc.PeriodWeek), h.periodReply(ipc.PeriodMonth)
	if today.BonusSpentMin != 10 || week.BonusSpentMin != 25 || month.BonusSpentMin != 45 {
		t.Errorf("из копилки: сегодня %d, неделя %d, месяц %d; want 10, 25, 45", today.BonusSpentMin, week.BonusSpentMin, month.BonusSpentMin)
	}
	if today.ExtraMin != 10 || week.ExtraMin != 40 {
		t.Errorf("доп. время: сегодня %d, неделя %d; want 10, 40", today.ExtraMin, week.ExtraMin)
	}
}

// Ловит: тестовая команда пишет сегодняшний или будущий день (его ведёт ядро: двойной счёт), работает
// без тестового режима, не нормализует путь (строка не совпадёт с программой в списке).
func TestTestSeedDay(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()), withClockAt(wed1500), withTestMode(TestMode{}))
	h.start()
	id := h.childID(childSID)
	req := ipc.TestSeedDayRequest{ChildID: id, Day: "2026-10-19", TotalS: 1800, GamesS: 1800,
		Usage: []ipc.TestSeedUsage{{Path: `C:\Program Files (x86)\Minecraft Launcher\MinecraftLauncher.exe`, Category: "games", Seconds: 1800}}}
	h.ok(adminPeer(), "", ipc.CmdTestSeedDay, req, nil)
	if st, _ := h.store.LoadDayState(id, "2026-10-19"); st.TotalS != 1800 || st.GamesS != 1800 {
		t.Errorf("день не записан: %+v", st)
	}
	rows, _ := h.store.DayUsage(id, "2026-10-19")
	if len(rows) != 1 || rows[0].Subject != pathMinecraft || rows[0].Seconds != 1800 {
		t.Errorf("учёт: %+v", rows)
	}
	for _, bad := range []ipc.TestSeedDayRequest{
		{ChildID: id, Day: "2026-10-21", TotalS: 60},
		{ChildID: id, Day: "2026-10-22", TotalS: 60},
		{ChildID: id, Day: "21.10.2026", TotalS: 60},
		{ChildID: id, Day: "2026-10-19", TotalS: -1},
		{ChildID: id, Day: "2026-10-19", Usage: []ipc.TestSeedUsage{{Path: pathMinecraft, Category: "blocked", Seconds: 1}}},
		{ChildID: 999, Day: "2026-10-19", TotalS: 60},
	} {
		if rep := h.do(adminPeer(), "", ipc.CmdTestSeedDay, bad); rep.OK {
			t.Errorf("принят %+v", bad)
		}
	}

	plain := newHarness(t, configured(), withChild(childSID, "Миша", 240), withClockAt(wed1500))
	plain.start()
	if rep := plain.do(adminPeer(), "", ipc.CmdTestSeedDay, req); rep.OK {
		t.Errorf("без тестового режима команда работает")
	}
}

// Ловит: ошибка чтения прошлых дней роняет ответ или даёт итог без строк (сумма строк ≠ итог, класс
// A); ошибка чтения учёта по программам теряет время прошлых дней, а не переносит его в «Остальное время».
func TestMyTodayPeriodStoreErrors(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()), withClockAt(wed1500))
	h.start()
	h.childLogsIn()
	seedS42(t, h, h.childID(childSID))
	h.run(60, fgSample(pathMinecraft, ipc.CatNameOther))

	h.store.mu.Lock()
	h.store.failPeriod = true
	h.store.mu.Unlock()
	week := h.periodReply(ipc.PeriodWeek)
	if week.GamesS != 60 || strings.Join(rowsOf(week.Programs, "games"), ", ") != "Minecraft Launcher=60" {
		t.Errorf("без прошлых дней: игры %d, строки %v; want только сегодня 60", week.GamesS, rowsOf(week.Programs, "games"))
	}

	h.store.mu.Lock()
	h.store.failPeriod, h.store.failUsage = false, true
	h.store.mu.Unlock()
	week = h.periodReply(ipc.PeriodWeek)
	if week.GamesS != 1860 || sumOf(week.Programs, "games") != 1860 {
		t.Errorf("без учёта по программам: игры %d, сумма строк %d; want 1860 и 1860 (%v)", week.GamesS, sumOf(week.Programs, "games"), rowsOf(week.Programs, "games"))
	}
	if !strings.Contains(h.logText(), "не удалось прочитать время по программам") {
		t.Errorf("ошибка учёта не в журнале работы")
	}
}
