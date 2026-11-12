package storage

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"pcblock/internal/core"
)

// Статистика вехи M5 (FR-38, FR-24, RM5-4, RM5-5, RM5-8, RM5-9): миграция 010, ряд дней,
// очистка статистики и журнала, фильтры журнала, дни без автобонуса (T7c, D-82).

// Ловит: миграция 010 на базе 0.4.x (001–009 с данными) теряет детей, дни или события; создаёт
// stats_reset с данными или без индексов журнала; меняет bonus_granted по часам базы (date('now',
// 'localtime') — не пояс службы; границу «до установки 0.5.0» ставит служба, D-82).
func TestMigration010OnFilled009Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var v009 []migration
	for _, m := range all {
		if m.version < 10 {
			v009 = append(v009, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, v009, 100); err != nil {
		t.Fatalf("миграции 001–009: %v", err)
	}
	// Вчера и сегодня по часам базы: миграция не должна их различать (границу ставит служба).
	var today, yesterday string
	if err := db.QueryRow(`SELECT date('now','localtime'), date('now','localtime','-1 day')`).Scan(&today, &yesterday); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, end_mode, created_at) VALUES (1, 'S-1', 'Миша', 'lock', 10)`,
		`INSERT INTO children (id, windows_sid, display_name, end_mode, created_at) VALUES (2, 'S-2', 'Аня', 'lock', 10)`,
		`INSERT INTO day_state (child_id, day, total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden, last_minute_end) VALUES
			(1, '2026-11-02', 7141, 1800, 600, 300, 15, NULL, 0, 1, 1792418400000),
			(1, '` + yesterday + `', 600, 0, 0, 0, 0, NULL, 0, 0, NULL),
			(1, '` + today + `', 300, 0, 0, 0, 0, 1792418400000, 0, 0, NULL),
			(2, '2026-11-03', 100, 0, 0, 0, 0, NULL, 1, 0, NULL)`,
		`INSERT INTO events (child_id, at, tz_offset_min, kind, details) VALUES (1, 100, 180, 'bypass', '{}'), (NULL, 200, 180, 'password_changed', '{}')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 009: %s: %v", q, err)
		}
	}
	const childQ = `SELECT id, windows_sid, display_name FROM children ORDER BY id`
	const dayQ = `SELECT child_id, day, total_s, games_s, video_s, study_s, extra_min, ifnull(pause_until, 'NULL'), no_forbidden, ifnull(last_minute_end, 'NULL') FROM day_state ORDER BY child_id, day`
	const eventQ = `SELECT id, ifnull(child_id, 'NULL'), at, kind FROM events ORDER BY id`
	before := [][][]string{rowStrings(t, db, childQ), rowStrings(t, db, dayQ), rowStrings(t, db, eventQ)}
	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 010: %v", err)
	}
	after := [][][]string{rowStrings(t, db, childQ), rowStrings(t, db, dayQ), rowStrings(t, db, eventQ)}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("данные после миграции = %v, want %v", after, before)
	}
	wantBonus := [][]string{{"1", "2026-11-02", "0"}, {"1", yesterday, "0"}, {"1", today, "0"}, {"2", "2026-11-03", "1"}}
	if got := rowStrings(t, db, `SELECT child_id, day, bonus_granted FROM day_state ORDER BY child_id, day`); !reflect.DeepEqual(got, wantBonus) {
		t.Errorf("bonus_granted после миграции = %v, want как до неё: %v", got, wantBonus)
	}
	if n := count(t, db, "stats_reset"); n != 0 {
		t.Errorf("строк stats_reset %d, want 0", n)
	}
	if got := rowStrings(t, db, `SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'events' ORDER BY name`); !reflect.DeepEqual(got, [][]string{{"events_by_at"}, {"events_by_child_at"}}) {
		t.Errorf("индексы events = %v, want events_by_at и events_by_child_at", got)
	}
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 100}, {6, 100}, {7, 100}, {8, 100}, {9, 100}, {10, 200}}) {
		t.Errorf("schema_migrations = %v, want 1–9 в 100 и 10 в 200", got)
	}
}

// Ловит: ряд дней берёт день before или теряет from; отдаёт чужого ребёнка, nil вместо [] или
// путает поля; принимает мусорный день.
func TestDaysSeries(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	for _, d := range []struct {
		child int64
		st    core.DayState
	}{
		{misha, core.DayState{Day: "2026-10-18", TotalS: 7}}, // до from
		{misha, core.DayState{Day: "2026-10-20", TotalS: 900, GamesS: 300, VideoS: 100, StudyS: 120, ExtraMin: 15, BonusGranted: true}},
		{misha, core.DayState{Day: "2026-10-19", TotalS: 1800, GamesS: 1800, ExtraMin: 30}},
		{misha, core.DayState{Day: "2026-10-21", TotalS: 600}}, // before: не входит
		{anya, core.DayState{Day: "2026-10-20", TotalS: 5000}},
	} {
		if err := s.SaveDayState(d.child, d.st); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.DaysSeries(misha, "2026-10-19", "2026-10-21")
	if err != nil {
		t.Fatal(err)
	}
	want := []DayRow{
		{Day: "2026-10-19", TotalS: 1800, GamesS: 1800, ExtraMin: 30},
		{Day: "2026-10-20", TotalS: 900, GamesS: 300, VideoS: 100, StudyS: 120, ExtraMin: 15},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DaysSeries = %+v, want %+v", got, want)
	}
	empty, err := s.DaysSeries(misha, "2027-01-01", "2027-02-01")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("DaysSeries без дней = %#v, %v; want [] не nil", empty, err)
	}
	if _, err := s.DaysSeries(misha, "19.10.2026", "2026-10-21"); !errors.Is(err, ErrInvalid) {
		t.Errorf("мусорный from: %v, want ErrInvalid", err)
	}
	if _, err := s.DaysSeries(misha, "2026-10-19", "2026-13-01"); !errors.Is(err, ErrInvalid) {
		t.Errorf("мусорный before: %v, want ErrInvalid", err)
	}
}

// Ловит: первый день статистики не самый ранний, чужой ребёнок или ok при пустой таблице.
func TestFirstStatsDay(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	if day, ok, err := s.FirstStatsDay(misha); err != nil || ok || day != "" {
		t.Errorf("FirstStatsDay без строк = %q, %v, %v; want \"\", false, nil", day, ok, err)
	}
	for _, d := range []struct {
		child int64
		day   string
	}{{misha, "2026-10-20"}, {misha, "2026-09-01"}, {anya, "2025-01-01"}, {misha, "2026-11-11"}} {
		if err := s.SaveDayState(d.child, core.DayState{Day: d.day, TotalS: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if day, ok, err := s.FirstStatsDay(misha); err != nil || !ok || day != "2026-09-01" {
		t.Errorf("FirstStatsDay = %q, %v, %v; want 2026-09-01, true", day, ok, err)
	}
}

// migrationsBefore — миграции с номером меньше v (база прошлой версии).
func migrationsBefore(all []migration, v int) []migration {
	var out []migration
	for _, m := range all {
		if m.version < v {
			out = append(out, m)
		}
	}
	return out
}

// statsFixture — два ребёнка с днями и учётом для проверки очистки статистики.
func statsFixture(t *testing.T) (*Store, int64, int64) {
	t.Helper()
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	for _, c := range []int64{misha, anya} {
		for _, d := range []string{"2026-11-09", "2026-11-10", "2026-11-11"} {
			if err := s.SaveDayState(c, core.DayState{Day: d, TotalS: 600, GamesS: 300, BonusGranted: true}); err != nil {
				t.Fatal(err)
			}
			if err := s.AddUsage(c, d, UsageGames, SubjectProgram, pathGame, 300); err != nil {
				t.Fatal(err)
			}
			if err := s.AddUsage(c, d, UsageVideoSocial, SubjectSite, "youtube.com", 100); err != nil {
				t.Fatal(err)
			}
		}
	}
	return s, misha, anya
}

func countWhere(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// Ловит (RM5-8): очистка оставляет учёт по программам за какой-то день (в том числе сегодня),
// прошлые строки дня; переписывает строку дня ядра не теми счётчиками или теряет её флаги
// (bonus_granted, no_forbidden, pause_until, last_minute_end — защита R25, R24, R11, R2);
// пишет cleared_at временем базы, а не часами ядра; задевает другого ребёнка; повтор не
// заменяет снимок.
func TestClearStats(t *testing.T) {
	s, misha, anya := statsFixture(t)
	fixClock(t, 999_999) // время базы: не должно попасть в cleared_at
	if _, ok, err := s.GetStatsReset(misha); err != nil || ok {
		t.Fatalf("GetStatsReset до очистки: ok=%v, %v; want false", ok, err)
	}
	pause, last := int64(1_792_418_400_000), int64(1_792_418_460_000)
	day := core.DayState{Day: "2026-11-10", TotalS: 1234, GamesS: 600, VideoS: 200, StudyS: 100, ExtraMin: 20,
		PauseUntilMs: &pause, BonusGranted: true, NoForbidden: true, LastMinuteEndMs: &last}
	r := StatsReset{ClearedAtMs: 1_762_800_000_000, Day: "2026-11-10", TotalS: 1234, GamesS: 600, VideoS: 200, StudyS: 100, ExtraMin: 20}
	if err := s.ClearStats(misha, day, r); err != nil {
		t.Fatalf("ClearStats: %v", err)
	}
	if n := countWhere(t, s, `SELECT count(*) FROM usage_daily WHERE child_id = ?`, misha); n != 0 {
		t.Errorf("строк usage_daily Миши %d, want 0 (все дни, и сегодня тоже)", n)
	}
	if got := rowStrings(t, s.db, fmt.Sprintf(`SELECT day FROM day_state WHERE child_id = %d ORDER BY day`, misha)); !reflect.DeepEqual(got, [][]string{{"2026-11-10"}, {"2026-11-11"}}) {
		t.Errorf("дни Миши после очистки = %v, want 2026-11-10 и 2026-11-11 (до дня ядра удалены)", got)
	}
	if got, err := s.LoadDayState(misha, "2026-11-10"); err != nil || !reflect.DeepEqual(got, day) {
		t.Errorf("строка дня ядра = %+v, %v; want %+v", got, err, day)
	}
	got, ok, err := s.GetStatsReset(misha)
	if err != nil || !ok || got != r {
		t.Errorf("GetStatsReset = %+v, %v, %v; want %+v", got, ok, err, r)
	}
	if n := countWhere(t, s, `SELECT count(*) FROM usage_daily WHERE child_id = ?`, anya); n != 6 {
		t.Errorf("строк usage_daily Ани %d, want 6 (не тронуты)", n)
	}
	if n := countWhere(t, s, `SELECT count(*) FROM day_state WHERE child_id = ?`, anya); n != 3 {
		t.Errorf("дней Ани %d, want 3 (не тронуты)", n)
	}
	if _, ok, _ := s.GetStatsReset(anya); ok {
		t.Error("у Ани появился снимок очистки")
	}
	if d, ok, err := s.FirstStatsDay(misha); err != nil || !ok || d != "2026-11-10" {
		t.Errorf("FirstStatsDay после очистки = %q, %v, %v; want 2026-11-10", d, ok, err)
	}

	// Повтор: снимок заменяется (одна строка на ребёнка).
	day2 := core.DayState{Day: "2026-11-11", TotalS: 60}
	r2 := StatsReset{ClearedAtMs: 1_762_900_000_000, Day: "2026-11-11", TotalS: 60}
	if err := s.ClearStats(misha, day2, r2); err != nil {
		t.Fatalf("повторный ClearStats: %v", err)
	}
	if got, _, _ := s.GetStatsReset(misha); got != r2 {
		t.Errorf("снимок после повтора = %+v, want %+v", got, r2)
	}
	if n := countWhere(t, s, `SELECT count(*) FROM stats_reset`); n != 1 {
		t.Errorf("строк stats_reset %d, want 1", n)
	}
	if got := rowStrings(t, s.db, fmt.Sprintf(`SELECT day, total_s FROM day_state WHERE child_id = %d`, misha)); !reflect.DeepEqual(got, [][]string{{"2026-11-11", "60"}}) {
		t.Errorf("дни Миши после повтора = %v, want только 2026-11-11 с 60 с", got)
	}
}

// Ловит (D-82, ревью M5): очистка удаляет прошлый день, ждущий автобонуса (bonus_granted = 0,
// безопасный режим), — бонус за него теряется; или оставляет рассмотренные дни.
func TestClearStatsKeepsUngrantedDays(t *testing.T) {
	s, misha, _ := statsFixture(t)
	pending := core.DayState{Day: "2026-11-08", TotalS: 2400, GamesS: 600}
	if err := s.SaveDayState(misha, pending); err != nil {
		t.Fatal(err)
	}
	r := StatsReset{ClearedAtMs: 1, Day: "2026-11-11"}
	if err := s.ClearStats(misha, core.DayState{Day: "2026-11-11"}, r); err != nil {
		t.Fatalf("ClearStats: %v", err)
	}
	if got := rowStrings(t, s.db, fmt.Sprintf(`SELECT day, bonus_granted FROM day_state WHERE child_id = %d ORDER BY day`, misha)); !reflect.DeepEqual(got, [][]string{{"2026-11-08", "0"}, {"2026-11-11", "0"}}) {
		t.Errorf("дни Миши после очистки = %v, want ждущий 2026-11-08 и день ядра 2026-11-11", got)
	}
	if got, err := s.UngrantedDays(misha, "2026-11-11"); err != nil || !reflect.DeepEqual(got, []core.DayState{pending}) {
		t.Errorf("UngrantedDays после очистки = %+v, %v; want %+v", got, err, pending)
	}
}

// Ловит (RM5-8): ошибка в начале или посередине очистки удаляет часть данных (нет одной транзакции).
func TestClearStatsAllOrNothing(t *testing.T) {
	s, misha, _ := statsFixture(t)
	snapshot := func() [][][]string {
		return [][][]string{
			rowStrings(t, s.db, `SELECT child_id, day, total_s FROM day_state ORDER BY child_id, day`),
			rowStrings(t, s.db, `SELECT child_id, day, subject, seconds FROM usage_daily ORDER BY child_id, day, subject`),
		}
	}
	before := snapshot()
	for _, tc := range []struct {
		name string
		day  core.DayState
		r    StatsReset
	}{
		{"не дата", core.DayState{Day: "10.11.2026"}, StatsReset{ClearedAtMs: 1, Day: "10.11.2026"}},
		{"день строки не день снимка", core.DayState{Day: "2026-11-11"}, StatsReset{ClearedAtMs: 1, Day: "2026-11-10"}},
	} {
		if err := s.ClearStats(misha, tc.day, tc.r); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", tc.name, err)
		}
	}
	if got := snapshot(); !reflect.DeepEqual(got, before) {
		t.Errorf("после отказа данные изменились: %v, want %v", got, before)
	}
	// Сбой на последнем шаге (запись снимка) откатывает удаление дней и учёта.
	if _, err := s.db.Exec(`DROP TABLE stats_reset`); err != nil {
		t.Fatal(err)
	}
	err := s.ClearStats(misha, core.DayState{Day: "2026-11-11"}, StatsReset{ClearedAtMs: 1, Day: "2026-11-11"})
	if err == nil {
		t.Fatal("ClearStats без таблицы снимка прошёл без ошибки")
	}
	if got := snapshot(); !reflect.DeepEqual(got, before) {
		t.Errorf("после сбоя посередине данные изменились: %v, want %v", got, before)
	}
}

// Ловит (RM5-9): очистка журнала ребёнка удаляет события другого ребёнка или без ребёнка
// (пароль, промокоды, работа программы) или неверно считает удалённые.
func TestDeleteChildEvents(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	for i, c := range []*int64{&misha, &anya, nil, &misha, &misha} {
		if err := s.AppendEvent(Event{ChildID: c, AtMs: int64(100 + i), Kind: "k"}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.DeleteChildEvents(misha)
	if err != nil || n != 3 {
		t.Fatalf("DeleteChildEvents = %d, %v; want 3", n, err)
	}
	if got := rowStrings(t, s.db, `SELECT ifnull(child_id, 'NULL'), at FROM events ORDER BY id`); !reflect.DeepEqual(got, [][]string{{fmt.Sprint(anya), "101"}, {"NULL", "102"}}) {
		t.Errorf("события после очистки = %v, want Ани и без ребёнка", got)
	}
	if n, err := s.DeleteChildEvents(misha); err != nil || n != 0 {
		t.Errorf("повтор = %d, %v; want 0", n, err)
	}
}

// Ловит (RM5-5): фильтр видов, границы времени (from включительно, to не включительно) и
// ребёнок не уходят в запрос; курсор (at, id) при повторяющихся at теряет или дублирует
// события на стыке страниц.
func TestListEventsFiltersAndCursor(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	// 450 событий Миши: по 3 на каждый at (150 разных моментов), виды по кругу a, b, c.
	kinds := []string{"a", "b", "c"}
	for i := 0; i < 450; i++ {
		if err := s.AppendEvent(Event{ChildID: &misha, AtMs: int64(1000 + i/3), Kind: kinds[i%3]}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AppendEvent(Event{ChildID: &anya, AtMs: 1100, Kind: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(Event{AtMs: 1100, Kind: "a"}); err != nil {
		t.Fatal(err)
	}

	// Курсор: 200 + 200 + 50, без дублей и пропусков, строго по (at, id) сверху вниз.
	seen := map[int64]bool{}
	var sizes []int
	f := EventFilter{ChildID: &misha, Limit: 200}
	var prevAt, prevID int64 = 1 << 62, 1 << 62
	for page := 0; page < 5; page++ {
		evs, err := s.ListEvents(f)
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) == 0 {
			break
		}
		sizes = append(sizes, len(evs))
		for _, e := range evs {
			if seen[e.ID] {
				t.Fatalf("событие %d повторилось на странице %d", e.ID, page)
			}
			seen[e.ID] = true
			if e.AtMs > prevAt || (e.AtMs == prevAt && e.ID >= prevID) {
				t.Fatalf("порядок нарушен: (%d,%d) после (%d,%d)", e.AtMs, e.ID, prevAt, prevID)
			}
			prevAt, prevID = e.AtMs, e.ID
		}
		last := evs[len(evs)-1]
		f.BeforeAtMs, f.BeforeID = last.AtMs, last.ID
	}
	if !reflect.DeepEqual(sizes, []int{200, 200, 50}) || len(seen) != 450 {
		t.Errorf("страницы %v, всего %d; want [200 200 50] и 450", sizes, len(seen))
	}

	ids := func(f EventFilter) string {
		t.Helper()
		evs, err := s.ListEvents(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range evs {
			out = append(out, fmt.Sprintf("%d:%s", e.AtMs, e.Kind))
		}
		return strings.Join(out, " ")
	}
	// Время: from включительно, to не включительно. at 1001 — события 3,4,5 (a b c), 1002 — 6,7,8.
	if got, want := ids(EventFilter{ChildID: &misha, FromMs: 1001, ToMs: 1003}), "1002:c 1002:b 1002:a 1001:c 1001:b 1001:a"; got != want {
		t.Errorf("FromMs/ToMs: %q, want %q", got, want)
	}
	// Виды: только a и c.
	if got, want := ids(EventFilter{ChildID: &misha, FromMs: 1001, ToMs: 1003, Kinds: []string{"a", "c"}}), "1002:c 1002:a 1001:c 1001:a"; got != want {
		t.Errorf("Kinds a,c: %q, want %q", got, want)
	}
	// Все дети, один момент: Миша (3), Аня, без ребёнка.
	if got := ids(EventFilter{FromMs: 1100, ToMs: 1101, Kinds: []string{"a"}}); got != "1100:a 1100:a 1100:a" {
		t.Errorf("все дети, вид a в 1100: %q, want три события (Миша, Аня, без ребёнка)", got)
	}
	// Курсор вместе с фильтрами: строго раньше (1002, id первого события 1002).
	evs, err := s.ListEvents(EventFilter{ChildID: &misha, FromMs: 1001, ToMs: 1003})
	if err != nil {
		t.Fatal(err)
	}
	mid := evs[2] // 1002:a
	if got, want := ids(EventFilter{ChildID: &misha, FromMs: 1001, ToMs: 1003, BeforeAtMs: mid.AtMs, BeforeID: mid.ID}), "1001:c 1001:b 1001:a"; got != want {
		t.Errorf("курсор с фильтром: %q, want %q", got, want)
	}
	// Пустые виды — все; несуществующий вид — пустой список, не nil.
	none, err := s.ListEvents(EventFilter{Kinds: []string{"нет_такого"}})
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("несуществующий вид: %#v, %v; want []", none, err)
	}
}

// Ловит (T7c, D-82): дни без автобонуса берут сегодняшний (before) или уже помеченные, чужого
// ребёнка; теряют поля строки (доп. время, флаги); порядок не по дню; nil вместо [].
func TestUngrantedDays(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	last := int64(1_792_418_460_000)
	tue := core.DayState{Day: "2026-11-10", TotalS: 2400, GamesS: 600, ExtraMin: 10, NoForbidden: true, LastMinuteEndMs: &last}
	mon := core.DayState{Day: "2026-11-09", TotalS: 100}
	for _, d := range []struct {
		child int64
		st    core.DayState
	}{
		{misha, tue},
		{misha, mon},
		{misha, core.DayState{Day: "2026-11-08", TotalS: 50, BonusGranted: true}}, // уже начислен
		{misha, core.DayState{Day: "2026-11-11", TotalS: 70}},                     // before: не входит
		{anya, core.DayState{Day: "2026-11-09", TotalS: 5}},
	} {
		if err := s.SaveDayState(d.child, d.st); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.UngrantedDays(misha, "2026-11-11")
	if err != nil {
		t.Fatal(err)
	}
	if want := []core.DayState{mon, tue}; !reflect.DeepEqual(got, want) {
		t.Errorf("UngrantedDays = %+v, want %+v", got, want)
	}
	empty, err := s.UngrantedDays(misha, "2026-11-09")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("UngrantedDays до первого дня = %#v, %v; want []", empty, err)
	}
	if _, err := s.UngrantedDays(misha, "11.11.2026"); !errors.Is(err, ErrInvalid) {
		t.Errorf("мусорный день: %v, want ErrInvalid", err)
	}
}
