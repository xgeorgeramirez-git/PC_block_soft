package storage

import (
	"database/sql"
	"reflect"
	"testing"
)

// rowStrings читает строки запроса как текст (для сравнения «до и после миграции»).
func rowStrings(t *testing.T, db *sql.DB, query string) [][]string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	out := [][]string{}
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = "NULL"
			if v.Valid {
				row[i] = v.String
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// Ловит: миграция 004 (M3) на базе заказчика версии 0.2.5 (применены 001 и 002) теряет детей,
// лимиты, состояние дней, настройки или пароль; новые колонки программ и «Ждут решения» без
// значений по умолчанию (NOT NULL сломал бы старые строки); таблица попыток по детям и индекс
// учёта по программам не созданы; повторное применение что-то меняет.
func TestMigration004OnFilled025Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var v025 []migration
	for _, m := range all {
		if m.version < 3 {
			v025 = append(v025, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, v025, 100); err != nil {
		t.Fatalf("миграции 0.2.5: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, end_mode, created_at) VALUES (1, 'S-1', 'Миша', 'study_only', 10), (2, 'S-2', 'Аня', 'lock', 11)`,
		`INSERT INTO day_limits (child_id, weekday, total_min, games_min) VALUES (1, 1, 120, 60), (1, 2, NULL, NULL), (2, 7, 0, NULL)`,
		`INSERT INTO day_state (child_id, day, total_s, games_s, extra_min, bonus_granted) VALUES (1, '2026-10-28', 3600, 600, 15, 1)`,
		`INSERT INTO settings (key, value) VALUES ('firstrun.seen.1', '1'), ('bonus.rate_per_10', '5'), ('helper.parent', '{}')`,
		`INSERT INTO parent_auth (id, hash, failed_count, changed_at) VALUES (1, 'hash', 2, 50)`,
		`INSERT INTO programs (exe_path, display_name, category, source, decided_at) VALUES ('c:\program files\brave\brave.exe', 'Brave', 'other', 'snapshot', NULL)`,
		`INSERT INTO pending_programs (exe_path, display_name, first_child_id, first_attempt_at, attempts) VALUES ('c:\users\миша\downloads\supergame.exe', 'SuperGame', 1, 777, 3)`,
		`INSERT INTO usage_daily (child_id, day, category, subject_kind, subject, seconds) VALUES (1, '2026-10-28', 'games', 'program', 'c:\игра.exe', 600)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 0.2.5: %s: %v", q, err)
		}
	}
	keep := map[string]string{
		"children":    `SELECT id, windows_sid, display_name, end_mode, created_at FROM children ORDER BY id`,
		"day_limits":  `SELECT child_id, weekday, total_min, games_min FROM day_limits ORDER BY child_id, weekday`,
		"day_state":   `SELECT child_id, day, total_s, games_s, extra_min, bonus_granted FROM day_state`,
		"parent_auth": `SELECT id, hash, failed_count, changed_at FROM parent_auth`,
		"usage_daily": `SELECT child_id, day, category, subject, seconds FROM usage_daily`,
	}
	before := map[string][][]string{}
	for name, q := range keep {
		before[name] = rowStrings(t, db, q)
	}

	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграции 003 и 004: %v", err)
	}
	for name, q := range keep {
		if got := rowStrings(t, db, q); !reflect.DeepEqual(got, before[name]) {
			t.Errorf("%s после миграции = %v, want %v", name, got, before[name])
		}
	}
	if got, want := rowStrings(t, db, `SELECT key, value FROM settings ORDER BY key`), [][]string{{"bonus.rate_per_10", "5"}, {"firstrun.seen.1", "1"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("settings = %v, want %v (003 убрала только helper.*)", got, want)
	}
	if got, want := rowStrings(t, db, `SELECT exe_path, display_name, category, source, decided_at, publisher, fingerprint, app_root FROM programs`),
		[][]string{{`c:\program files\brave\brave.exe`, "Brave", "other", "snapshot", "NULL", "", "", ""}}; !reflect.DeepEqual(got, want) {
		t.Errorf("programs = %v, want %v", got, want)
	}
	if got, want := rowStrings(t, db, `SELECT exe_path, display_name, first_child_id, first_attempt_at, attempts, publisher, description, last_attempt_at, was_category FROM pending_programs`),
		[][]string{{`c:\users\миша\downloads\supergame.exe`, "SuperGame", "1", "777", "3", "", "", "0", ""}}; !reflect.DeepEqual(got, want) {
		t.Errorf("pending_programs = %v, want %v", got, want)
	}
	if n := count(t, db, "pending_attempts"); n != 0 {
		t.Errorf("строк pending_attempts = %d, want 0", n)
	}
	var idx string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'usage_by_subject'`).Scan(&idx); err != nil {
		t.Fatalf("индекс usage_by_subject: %v", err)
	}
	if want := "CREATE INDEX usage_by_subject ON usage_daily(subject_kind, subject, day)"; idx != want {
		t.Errorf("индекс = %q, want %q", idx, want)
	}

	// Попытки ребёнка уходят каскадом вместе с ждущей программой (решение родителя).
	if _, err := db.Exec(`INSERT INTO pending_attempts (exe_path, child_id, attempts, last_attempt_at) VALUES ('c:\users\миша\downloads\supergame.exe', 1, 3, 777)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM pending_programs`); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, "pending_attempts"); n != 0 {
		t.Errorf("после удаления ждущей программы попыток %d, want 0 (ON DELETE CASCADE)", n)
	}

	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 200}, {4, 200}, {5, 200}, {6, 200}}) {
		t.Errorf("schema_migrations = %v, want 1,2 в 100 и 3–6 в 200", got)
	}
}
