package storage

import (
	"reflect"
	"testing"

	"pcblock/internal/core"
)

// Последняя минута (FB-12, D-60, RF12-1): колонка day_state.last_minute_end, миграция 006.

// Ловит: миграция 006 на базе со схемой 005 и данными теряет или портит строки day_state
// (счётчики, пауза, флаги), ставит отсчёт старым дням (last_minute_end не NULL) или меняет
// что-то при повторном применении.
func TestMigration006OnFilled005Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var v005 []migration
	for _, m := range all {
		if m.version < 6 {
			v005 = append(v005, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, v005, 100); err != nil {
		t.Fatalf("миграции 001–005: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, created_at) VALUES (1, 'S-1', 'Миша', 10)`,
		`INSERT INTO day_state (child_id, day, total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden) VALUES
			(1, '2026-10-30', 7200, 1800, 600, 300, 15, 1792418400000, 1, 0),
			(1, '2026-10-31', 4000, 0, 0, 0, -5, NULL, 0, 1)`,
		`INSERT INTO programs (exe_path, display_name, category, source, hidden) VALUES ('c:\games\x.exe', 'X', 'games', 'parent', 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 005: %s: %v", q, err)
		}
	}
	const dayQ = `SELECT child_id, day, total_s, games_s, video_s, study_s, extra_min, ifnull(pause_until, 'NULL'), bonus_granted, no_forbidden FROM day_state ORDER BY day`
	const progQ = `SELECT exe_path, display_name, category, source, hidden FROM programs`
	beforeDay, beforeProg := rowStrings(t, db, dayQ), rowStrings(t, db, progQ)
	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 006: %v", err)
	}
	if got := rowStrings(t, db, dayQ); !reflect.DeepEqual(got, beforeDay) {
		t.Errorf("day_state после миграции = %v, want %v", got, beforeDay)
	}
	if got := rowStrings(t, db, progQ); !reflect.DeepEqual(got, beforeProg) {
		t.Errorf("programs после миграции = %v, want %v", got, beforeProg)
	}
	if got, want := rowStrings(t, db, `SELECT day, ifnull(last_minute_end, 'NULL') FROM day_state ORDER BY day`),
		[][]string{{"2026-10-30", "NULL"}, {"2026-10-31", "NULL"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("last_minute_end после миграции = %v, want %v", got, want)
	}
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 100}, {6, 200}, {7, 200}}) {
		t.Errorf("schema_migrations = %v, want 1–5 в 100 и 6–7 в 200", got)
	}
}

// Ловит: момент последней минуты не сохраняется или не читается (выключение питания в
// последнюю минуту его теряет, RF12-3), путается с паузой; снятие (nil) не пишет NULL.
func TestSaveLoadDayStateLastMinute(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	end, pause := int64(1792418400000), int64(1792422000000)
	st := core.DayState{Day: day1, TotalS: 7141, LastMinuteEndMs: &end, PauseUntilMs: &pause}
	if err := s.SaveDayState(id, st); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadDayState(id, day1)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastMinuteEndMs == nil || *got.LastMinuteEndMs != 1792418400000 || got.PauseUntilMs == nil || *got.PauseUntilMs != 1792422000000 || got.TotalS != 7141 {
		t.Fatalf("LoadDayState = %+v; want момент 1792418400000, пауза 1792422000000, 7141 с", got)
	}
	st.LastMinuteEndMs = nil
	if err := s.SaveDayAndUsage(id, st, nil); err != nil {
		t.Fatal(err)
	}
	if n := rawNull(t, s, `SELECT last_minute_end FROM day_state WHERE child_id = ? AND day = ?`, id, day1); n.Valid {
		t.Errorf("last_minute_end после снятия = %+v, want NULL", n)
	}
	if got, _ := s.LoadDayState(id, day1); got.LastMinuteEndMs != nil {
		t.Errorf("после снятия LastMinuteEndMs = %d, want nil", *got.LastMinuteEndMs)
	}
}
