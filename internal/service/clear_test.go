package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Очистка статистики и журнала (FR-38, RM5-8, RM5-9, задача T2c).

// sun1500 — воскресенье 22.11.2026 15:00 (часы ядра); настоящие часы в тестах — realT0 (19.10.2026).
var sun1500 = time.Date(2026, 11, 22, 15, 0, 0, 0, tzMSK)

// engState — счётчики и флаги дня ядра ребёнка (учёт лимита).
func (h *harness) engState(id int64) core.DayState {
	h.t.Helper()
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	st, _ := h.svc.children[id].eng.State()
	return st
}

// dayRow — строка day_state из базы (fake).
func (h *harness) dayRow(id int64, day string) (core.DayState, bool) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	st, ok := h.store.days[dayKey(id, day)]
	return st, ok
}

// resetOf — снимок очистки ребёнка в базе.
func (h *harness) resetOf(id int64) (StatsReset, bool) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	r, ok := h.store.resets[id]
	return r, ok
}

// usageLen — сколько строк учёта по программам и сайтам у ребёнка в базе.
func (h *harness) usageLen(id int64) int {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	n := 0
	for k := range h.store.usage {
		if strings.HasPrefix(k, fmt.Sprint(id)+"|") {
			n++
		}
	}
	return n
}

// clearStats — ClearStats родителем; ответ.
func (h *harness) clearStats(tok string, id int64) ipc.ClearReply {
	h.t.Helper()
	var rep ipc.ClearReply
	h.ok(adminPeer(), tok, ipc.CmdClearStats, ipc.ClearChildRequest{ChildID: id}, &rep)
	return rep
}

// parentPeriod — «Итоги» ребёнка id за период у родителя.
func (h *harness) parentPeriod(tok string, id int64, period string) ipc.MyTodayReply {
	h.t.Helper()
	var my ipc.MyTodayReply
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id, Period: period}, &my)
	return my
}

// Ловит (S48, D-73, RM5-10, Review Focus 3): очистка меняет учёт лимита: остаток, доп. время,
// флаги дня (bonus_granted, no_forbidden, пауза) в ядре или в строке базы; после очистки остаток
// считается от нуля; «Итоги» сегодня считают время до очистки.
func TestClearStatsKeepsLimitAccounting(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withClockAt(sun1500))
	id := h.childID(childSID)
	h.store.days[dayKey(id, "2026-11-22")] = core.DayState{Day: "2026-11-22", TotalS: 40 * 60, ExtraMin: 15, BonusGranted: true, NoForbidden: true}
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	before := h.status()
	if before.RemainingS != 95*60 {
		t.Fatalf("до очистки остаток %d, want 5700 (120 + 15 − 40 мин)", before.RemainingS)
	}
	stBefore := h.engState(id)

	if rep := h.clearStats(tok, id); rep.Message != "Статистика Миши очищена" {
		t.Errorf("сообщение %q", rep.Message)
	}
	if st := h.status(); st.RemainingS != 95*60 || st.UsedS != 40*60 {
		t.Errorf("после очистки остаток %d, использовано %d; want 5700 и 2400", st.RemainingS, st.UsedS)
	}
	if st := h.engState(id); !reflect.DeepEqual(st, stBefore) {
		t.Errorf("день ядра изменился: %+v, был %+v", st, stBefore)
	}
	if row, _ := h.dayRow(id, "2026-11-22"); !row.BonusGranted || !row.NoForbidden || row.ExtraMin != 15 || row.TotalS != 40*60 {
		t.Errorf("строка дня после очистки %+v; want учёт и флаги прежние", row)
	}
	if my := h.periodReply(ipc.PeriodToday); my.UsedS != 0 || my.ExtraMin != 0 || my.AllS != 0 {
		t.Errorf("«Итоги» сразу после очистки: %d с, доп. %d; want 0", my.UsedS, my.ExtraMin)
	}

	h.run(60, activeSample)
	if st := h.status(); st.RemainingS != 95*60-60 {
		t.Errorf("через 60 с остаток %d, want 5640", st.RemainingS)
	}
	my := h.periodReply(ipc.PeriodToday)
	if my.UsedS != 60 || my.OtherS != 60 {
		t.Errorf("«Итоги» сегодня через 60 с: %d (прочее %d); want 60", my.UsedS, my.OtherS)
	}
	if len(my.Today) < 2 || my.Today[1] != "Использовано 41 мин" {
		t.Errorf("строки «Сегодня» (учёт лимита, RM5-10): %q; want «Использовано 41 мин»", my.Today)
	}

	// Пауза — тоже флаг дня: переживает очистку в ядре и в строке базы.
	h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: id, Preset: ipc.PauseMin15}, nil)
	stPaused := h.engState(id)
	if stPaused.PauseUntilMs == nil {
		t.Fatal("пауза не поставлена: проверка ничего не проверяет")
	}
	h.clearStats(tok, id)
	if st := h.engState(id); !reflect.DeepEqual(st, stPaused) {
		t.Errorf("пауза: день ядра изменился: %+v, был %+v", st, stPaused)
	}
	if row, _ := h.dayRow(id, "2026-11-22"); row.PauseUntilMs == nil || *row.PauseUntilMs != *stPaused.PauseUntilMs || !row.BonusGranted || !row.NoForbidden {
		t.Errorf("пауза: строка дня %+v; want пауза и флаги ядра", row)
	}
}

// Ловит (RM5-8, Review Focus 3): cleared_at — настоящее время, а не часы ядра (в приёмке часы ядра
// сдвинуты TestClock); «Статистика ведётся с» не дата очистки.
func TestClearStatsClearedAtByCoreClock(t *testing.T) {
	h, _, id := g15Harness(t)
	tok := h.login(adminPeer())
	at := h.clock.Now().UnixMilli()
	h.clearStats(tok, id)
	r, ok := h.resetOf(id)
	if !ok || r.ClearedAtMs != at || r.Day != "2026-11-22" {
		t.Fatalf("снимок %+v (есть %v); want cleared_at %d (часы ядра), день 2026-11-22", r, ok, at)
	}
	if r.TotalS != 600 || r.GamesS != 600 {
		t.Errorf("снимок счётчиков %+v; want 600 с игр (счётчики ядра)", r)
	}
	my := h.periodReply(ipc.PeriodWeek)
	if my.StatsSinceDay != "2026-11-22" || my.StatsSinceText != "Статистика ведётся с 22 ноября 2026" {
		t.Errorf("ведётся с: %q, %q", my.StatsSinceDay, my.StatsSinceText)
	}
}

// Ловит (FR-38, RM5-8): очистка затронула другого ребёнка (его дни, учёт программ или «Итоги»).
func TestClearStatsOtherChildUntouched(t *testing.T) {
	h, _, id := g15Harness(t, withChild(child2SID, "Коля", 240))
	kolya := h.childID(child2SID)
	st := core.DayState{Day: "2026-11-18", TotalS: 50 * 60, GamesS: 30 * 60, StudyS: 10 * 60, BonusGranted: true}
	if err := h.store.SaveDayAndUsage(kolya, st, []UsageRow{{Day: "2026-11-18", Category: "games", Kind: usageKindProgram, Subject: pathMinecraft, Seconds: 30 * 60}}); err != nil {
		t.Fatal(err)
	}
	tok := h.login(adminPeer())
	periods := []string{ipc.PeriodToday, ipc.PeriodWeek, ipc.PeriodMonth, ipc.PeriodYear, ipc.PeriodAll}
	was := map[string]string{}
	for _, p := range periods {
		was[p] = jsonOf(t, h.parentPeriod(tok, kolya, p))
	}
	usage := h.usageLen(kolya)
	h.clearStats(tok, id)
	for _, p := range periods {
		if got := jsonOf(t, h.parentPeriod(tok, kolya, p)); got != was[p] {
			t.Errorf("Коля, %s: ответ изменился\n%s\nбыл\n%s", p, got, was[p])
		}
	}
	if n := h.usageLen(kolya); n != usage || n == 0 {
		t.Errorf("строк учёта у Коли %d, было %d", n, usage)
	}
	if _, ok := h.resetOf(kolya); ok {
		t.Error("у Коли появился снимок очистки")
	}
	if my := h.parentPeriod(tok, id, ipc.PeriodAll); my.UsedS != 0 {
		t.Errorf("Миша «Всё время» после очистки %d с, want 0", my.UsedS)
	}
}

// Ловит (RM5-8, класс A): несохранённый накопитель cs.usage (до 30 с) вносит в «Итоги» секунды до
// очистки; строки программ не сходятся с итогом категории.
func TestClearStatsTodayFromClearMoment(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withClockAt(sun1500))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	tok := h.login(adminPeer())
	h.run(40, fgSample(pathMinecraft, ipc.CatNameOther)) // сохранение на 30-й секунде, 10 с в накопителе
	h.svc.mu.Lock()
	unsaved := len(h.svc.children[id].usage)
	h.svc.mu.Unlock()
	if unsaved == 0 {
		t.Fatal("накопитель пуст: проверка ничего не проверяет")
	}
	h.clearStats(tok, id)
	if n := h.usageLen(id); n != 0 {
		t.Errorf("после очистки строк учёта %d, want 0", n)
	}
	h.run(20, fgSample(pathMinecraft, ipc.CatNameOther))
	my := h.periodReply(ipc.PeriodToday)
	if my.UsedS != 20 || my.GamesS != 20 {
		t.Errorf("«Итоги» сегодня %d с, игры %d; want 20", my.UsedS, my.GamesS)
	}
	var mc int
	for _, p := range my.Programs {
		if p.Path == pathMinecraft {
			mc += p.Seconds
		}
	}
	if mc != 20 {
		t.Errorf("Minecraft за сегодня %d с, want 20 (секунды до очистки не считаются)", mc)
	}
	if sumOf(my.Programs, "games") != my.GamesS || sumOf(my.Programs, "") != my.AllS {
		t.Errorf("строки %s не сходятся с итогами: игры %d, всего %d", jsonOf(t, my.Programs), my.GamesS, my.AllS)
	}
}

// Ловит (RM5-2, RM5-8): после очистки в «Неделе» и «Всём времени» остались прошлые дни; календарь
// «Недели» и «Года» обрезан до дня очистки; «Всё время» начинается не с месяца очистки; строка дня
// в базе после очистки — устаревшая копия (база отстаёт от ядра до 30 с).
func TestClearStatsPastDaysAndBars(t *testing.T) {
	h, _, id := g15Harness(t)
	tok := h.login(adminPeer())
	h.run(25, fgSample(pathMinecraft, ipc.CatNameOther))
	live := h.engState(id)
	if row, _ := h.dayRow(id, "2026-11-22"); row.TotalS == live.TotalS {
		t.Fatalf("строка базы уже равна ядру (%d с): проверка 4а ничего не проверяет", row.TotalS)
	}
	h.clearStats(tok, id)
	if row, _ := h.dayRow(id, "2026-11-22"); row.TotalS != live.TotalS || row.GamesS != live.GamesS {
		t.Errorf("строка дня ядра в базе %+v; want счётчики ядра %+v", row, live)
	}
	for k := range h.store.days {
		if d, ok := fakeDayOf(k, id); ok && d < "2026-11-22" {
			t.Errorf("прошлый день %s не удалён", d)
		}
	}
	week := h.periodReply(ipc.PeriodWeek)
	if week.UsedS != 0 || len(week.Bars) != 7 || week.Bars[0].Key != "2026-11-16" || week.StatsSinceDay != "2026-11-22" {
		t.Errorf("неделя: итог %d, столбики %s, с %q; want 0, 7 с понедельника, 2026-11-22", week.UsedS, jsonOf(t, week.Bars), week.StatsSinceDay)
	}
	for _, b := range week.Bars {
		if b.TotalS != 0 || b.StudyS != 0 {
			t.Errorf("неделя: столбик с данными %+v", b)
		}
	}
	year := h.periodReply(ipc.PeriodYear)
	if len(year.Bars) != 11 || year.Bars[0].Key != "2026-01" || year.UsedS != 0 {
		t.Errorf("год: %d столбиков с %v, итог %d; want 11 с января, 0", len(year.Bars), year.Bars, year.UsedS)
	}
	all := h.periodReply(ipc.PeriodAll)
	if len(all.Bars) != 1 || all.Bars[0].Key != "2026-11" || all.UsedS != 0 || all.BarsBy != ipc.BarsByMonth {
		t.Errorf("всё время: столбики %s, итог %d; want один ноябрь 2026", jsonOf(t, all.Bars), all.UsedS)
	}
}

// Ловит (RM5-8): повторная очистка не заменяет снимок — «Итоги» после неё не с нуля.
func TestClearStatsTwice(t *testing.T) {
	h, _, id := g15Harness(t)
	tok := h.login(adminPeer())
	h.clearStats(tok, id)
	h.run(30, fgSample(pathMinecraft, ipc.CatNameOther))
	if my := h.periodReply(ipc.PeriodToday); my.UsedS != 30 {
		t.Fatalf("после первой очистки %d с, want 30", my.UsedS)
	}
	h.clearStats(tok, id)
	if r, _ := h.resetOf(id); r.TotalS != 630 {
		t.Errorf("снимок второй очистки %+v; want 630 с", r)
	}
	if my := h.periodReply(ipc.PeriodAll); my.UsedS != 0 || my.GamesS != 0 {
		t.Errorf("после второй очистки «Всё время» %d с, want 0", my.UsedS)
	}
}

// Ловит (RM5-8, Review Focus 3): после перезапуска службы снимок забыт (сегодня снова всё время дня)
// или остаток изменился.
func TestClearStatsSurvivesRestart(t *testing.T) {
	h, _, id := g15Harness(t)
	tok := h.login(adminPeer())
	h.clearStats(tok, id)
	h.run(20, fgSample(pathMinecraft, ipc.CatNameOther))
	remaining := h.status().RemainingS
	h.svc.Shutdown(h.real.Now())

	h2 := newHarness(t, withStore(h.store), withPrograms(typicalScan()), withClockAt(h.clock.Now()))
	h2.start()
	h2.childLogsIn()
	if st := h2.status(); st.RemainingS != remaining {
		t.Errorf("после перезапуска остаток %d, want %d", st.RemainingS, remaining)
	}
	my := h2.periodReply(ipc.PeriodToday)
	if my.UsedS != 20 || my.GamesS != 20 {
		t.Errorf("после перезапуска «Итоги» сегодня %d с (игры %d), want 20 (счётчики ядра − снимок)", my.UsedS, my.GamesS)
	}
	if all := h2.periodReply(ipc.PeriodAll); all.UsedS != 20 || all.StatsSinceDay != "2026-11-22" {
		t.Errorf("после перезапуска «Всё время» %d с с %q", all.UsedS, all.StatsSinceDay)
	}
}

// Ловит (PL-31, RM5-8, Review Focus 1): очистка в 00:00:02, пока ядро во вчерашнем дне: снимок взят
// не с дня ядра (вчерашний день после закрытия ядром идёт в «Неделю» целиком); вчерашняя строка
// удалена; «Статистика ведётся с» — день ядра, а не дата очистки; автобонус за вчера дважды.
func TestClearStatsWhileCoreYesterday(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()),
		withClockAt(time.Date(2026, 11, 24, 23, 59, 0, 0, tzMSK)))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	tok := h.login(adminPeer())
	h.run(30, activeSample) // вт 23:59:30, 30 с за вторник
	h.clock.Advance(32 * time.Second)
	if st := h.engState(id); st.Day != "2026-11-24" {
		t.Fatalf("ядро уже в %q: проверка ничего не проверяет", st.Day)
	}
	h.clearStats(tok, id)
	r, _ := h.resetOf(id)
	if r.Day != "2026-11-24" || r.TotalS != 30 {
		t.Errorf("снимок %+v; want день ядра 2026-11-24 и 30 с", r)
	}
	if my := h.periodReply(ipc.PeriodWeek); my.StatsSinceDay != "2026-11-25" {
		t.Errorf("ведётся с %q, want 2026-11-25 (дата очистки)", my.StatsSinceDay)
	}
	h.step(activeSample) // ядро закрывает вторник
	if st := h.engState(id); st.Day != "2026-11-25" {
		t.Fatalf("ядро не перешло в среду: %q", st.Day)
	}
	row, ok := h.dayRow(id, "2026-11-24")
	if !ok || row.TotalS < 30 || !row.BonusGranted {
		t.Errorf("строка вторника %+v (есть %v); want не удалена, бонус рассмотрен", row, ok)
	}
	week := h.periodReply(ipc.PeriodWeek)
	if len(week.Bars) != 3 || week.Bars[1].Key != "2026-11-24" || week.Bars[1].TotalS > 3 {
		t.Errorf("неделя после смены суток: %s; want вторник ≈ 0 (счётчики − снимок)", jsonOf(t, week.Bars))
	}
	if n := len(h.store.eventsOf(core.EvBonusGranted)); n != 1 {
		t.Errorf("автобонусов за вторник %d, want 1", n)
	}
	// Перезапуск: вторник уже рассмотрен, второго бонуса нет.
	h.svc.Shutdown(h.real.Now())
	h2 := newHarness(t, withStore(h.store), withPrograms(typicalScan()), withClockAt(h.clock.Now()))
	h2.start()
	if n := len(h2.store.eventsOf(core.EvBonusGranted)); n != 1 {
		t.Errorf("после перезапуска автобонусов %d, want 1", n)
	}
}

// Ловит (RM5-8): день ядра пуст (ядро ещё не начало день) — очистка падает или пишет снимок с пустым днём.
func TestClearStatsEmptyCoreDay(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withClockAt(sun1500))
	h.start()
	id := h.childID(childSID)
	tok := h.login(adminPeer())
	h.svc.mu.Lock()
	cs := h.svc.children[id]
	cs.eng = core.NewEngine(h.clock, tzMSK, cs.rules, core.DayState{}, "")
	h.svc.mu.Unlock()
	h.clearStats(tok, id)
	r, ok := h.resetOf(id)
	if !ok || r.Day != "2026-11-22" || r.TotalS != 0 || r.GamesS != 0 || r.StudyS != 0 || r.ExtraMin != 0 {
		t.Errorf("снимок %+v; want день 2026-11-22 и нули", r)
	}
}

// Ловит (RM5-9, S48): очистка журнала удаляет события другого ребёнка или события без ребёнка;
// после очистки у ребёнка нет записи «Журнал очищен»; ClearStats не пишет «Статистика очищена».
func TestClearJournal(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120), withClockAt(sun1500))
	h.start()
	misha, kolya := h.childID(childSID), h.childID(child2SID)
	for _, e := range []StoredEvent{
		{ChildID: misha, AtMs: 1, Kind: core.EvBypass, Details: map[string]any{}},
		{ChildID: misha, AtMs: 2, Kind: core.EvExtraTime, Details: map[string]any{"minutes": 15}},
		{ChildID: kolya, AtMs: 3, Kind: core.EvBypass, Details: map[string]any{}},
		{ChildID: 0, AtMs: 4, Kind: "password_changed", Details: map[string]any{}},
	} {
		if err := h.store.AppendEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	tok := h.login(adminPeer())
	h.clearStats(tok, misha)
	if evs := h.store.eventsOf(evStatsCleared); len(evs) != 1 || evs[0].ChildID != misha {
		t.Fatalf("«Статистика очищена»: %+v", evs)
	} else if txt := eventText(evs[0].Kind, "Миша", evs[0].Details); txt != "Миша: статистика очищена" {
		t.Errorf("текст журнала %q", txt)
	}
	before := map[int64]int{}
	for _, e := range h.store.events {
		before[e.ChildID]++
	}
	var rep ipc.ClearReply
	h.ok(adminPeer(), tok, ipc.CmdClearJournal, ipc.ClearChildRequest{ChildID: misha}, &rep)
	if rep.Message != "Журнал Миши очищен" {
		t.Errorf("сообщение %q", rep.Message)
	}
	after := map[int64][]StoredEvent{}
	for _, e := range h.store.events {
		after[e.ChildID] = append(after[e.ChildID], e)
	}
	if m := after[misha]; len(m) != 1 || m[0].Kind != evJournalCleared || eventText(m[0].Kind, "Миша", m[0].Details) != "Миша: журнал очищен" {
		t.Errorf("журнал Миши после очистки: %+v; want одна запись «журнал очищен»", m)
	}
	if len(after[kolya]) != before[kolya] || len(after[0]) != before[0] || before[0] == 0 {
		t.Errorf("события Коли %d (было %d), без ребёнка %d (было %d)", len(after[kolya]), before[kolya], len(after[0]), before[0])
	}
}

// Ловит (R21, Review Focus 4, RM5-8): очистку можно вызвать без родительской сессии (из окна
// ребёнка, стража); несуществующий ребёнок не отказ; сбой хранилища теряет накопитель или пишет
// событие об очистке, которой не было.
func TestClearRightsAndErrors(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withPrograms(typicalScan()), withClockAt(sun1500))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	for _, cmd := range []string{ipc.CmdClearStats, ipc.CmdClearJournal} {
		for name, p := range map[string]ipc.Peer{"ребёнок": childPeer(), "страж": guardPeer()} {
			if rep := h.fail(p, "", cmd, ipc.ClearChildRequest{ChildID: id}); rep.Error != textNeedParent {
				t.Errorf("%s от «%s»: %q, want %q", cmd, name, rep.Error, textNeedParent)
			}
		}
	}
	tok := h.login(adminPeer())
	for _, cmd := range []string{ipc.CmdClearStats, ipc.CmdClearJournal} {
		if rep := h.fail(adminPeer(), tok, cmd, ipc.ClearChildRequest{ChildID: 999}); rep.Error != textNoSuchChild {
			t.Errorf("%s чужого ребёнка: %q, want %q", cmd, rep.Error, textNoSuchChild)
		}
	}
	h.run(40, fgSample(pathMinecraft, ipc.CatNameOther))
	h.svc.mu.Lock()
	unsaved := len(h.svc.children[id].usage)
	h.svc.mu.Unlock()
	h.store.mu.Lock()
	h.store.failSave = true
	h.store.mu.Unlock()
	if rep := h.fail(adminPeer(), tok, ipc.CmdClearStats, ipc.ClearChildRequest{ChildID: id}); rep.Error != textSaveFailed {
		t.Errorf("сбой хранилища: %q, want %q", rep.Error, textSaveFailed)
	}
	h.svc.mu.Lock()
	kept := len(h.svc.children[id].usage)
	h.svc.mu.Unlock()
	if unsaved == 0 || kept != unsaved {
		t.Errorf("накопитель при сбое: %d порций, было %d; want не тронут", kept, unsaved)
	}
	if _, ok := h.resetOf(id); ok || len(h.store.eventsOf(evStatsCleared)) != 0 {
		t.Error("при сбое записан снимок или событие очистки")
	}
}

// Ловит: «Статистика Миша очищена» вместо «Миши» (тот же падеж, что genitiveName окна).
func TestGenitiveName(t *testing.T) {
	for name, want := range map[string]string{
		"Миша": "Миши", "Даша": "Даши", "Анна": "Анны", "Ольга": "Ольги", "Ника": "Ники", "Катя": "Кати",
		"Мария": "Марии", "Иван": "Ивана", "Артём": "Артёма", "Андрей": "Андрея", "Игорь": "Игоря",
		"Дмитрий": "Дмитрия", "Женя": "Жени", "Tom": "Tom", "Лиза Петрова": "Лиза Петрова", "": "", " Коля ": "Коли",
	} {
		if got := genitiveName(name); got != want {
			t.Errorf("genitiveName(%q) = %q, want %q", name, got, want)
		}
	}
}

// Ловит (D-82, R25, ревью M5): очистка статистики удаляет прошлый день, ждущий автобонуса, и бонус
// за него теряется навсегда; в безопасном режиме (правила не прочитаны) строка удаляется или
// попадает в «Итоги» после очистки; после восстановления правил бонус не начисляется.
func TestClearStatsKeepsPendingBonus(t *testing.T) {
	// Правила прочитаны: перед очисткой служба начисляет ждущий бонус.
	st, id := catchupStore(t)
	h := startAt(t, st, thu1000)
	st.mu.Lock()
	st.days[dayKey(id, wed25)] = core.DayState{Day: wed25, TotalS: 60 * 60} // ждёт бонуса (появился после запуска)
	st.mu.Unlock()
	tok := h.login(adminPeer())
	h.clearStats(tok, id)
	bal, evs := bonusOf(t, st, id)
	if bal != 70 || len(evs) != 2 || evs[1].Details["day"] != wed25 {
		t.Errorf("после очистки: копилка %d, записи %+v; want 70 (40 за вторник при запуске и 30 за среду перед очисткой)", bal, evs)
	}
	if _, ok := h.dayRow(id, wed25); ok {
		t.Error("строка среды после начисления и очистки осталась")
	}

	// Безопасный режим: начислить нельзя — строка остаётся с bonus_granted = 0, в «Итоги» не попадает.
	st, id = catchupStore(t)
	st.failSchedule = true
	h = startAt(t, st, thu1000)
	tok = h.login(adminPeer())
	h.clearStats(tok, id)
	if row, ok := h.dayRow(id, tue24); !ok || row.BonusGranted || row.TotalS != 40*60 {
		t.Fatalf("безопасный режим: строка вторника %+v (есть %v); want сохранена без отметки", row, ok)
	}
	st.failSchedule = false
	for _, p := range []string{ipc.PeriodWeek, ipc.PeriodMonth, ipc.PeriodAll} {
		if my := h.parentPeriod(tok, id, p); my.UsedS != 0 {
			t.Errorf("%s после очистки: %d с; want 0 (вторник до очистки)", p, my.UsedS)
		}
	}
	startAt(t, st, thu1000.Add(time.Hour))
	if bal, evs := bonusOf(t, st, id); bal != 40 || len(evs) != 1 || evs[0].Details["day"] != tue24 {
		t.Errorf("запуск с правилами после очистки: копилка %d, записи %+v; want 40 за вторник", bal, evs)
	}
}
