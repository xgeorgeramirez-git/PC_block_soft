package storage

import (
	"database/sql"
	"reflect"
	"testing"
)

// allSettings читает таблицу settings целиком: ключ → значение.
func allSettings(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// Ловит: миграция 003 (D-56) оставляет в базе состояние удалённого Помощника или задевает
// чужие настройки: окно первого входа (FR-32), автобонус, подробные логи, пароль родителя.
func TestMigration003DropsHelperStateOnly(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var before []migration
	for _, m := range all {
		if m.version < 3 {
			before = append(before, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, before, 100); err != nil {
		t.Fatalf("миграции 001 и 002: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES
		('helper.parent', '{"enabled":true,"seen":["H-M1-01"]}'),
		('helper.child.1', '{"enabled":false}'),
		('helper.child.12', '{}'),
		('firstrun.seen.1', '1'),
		('bonus.rate_per_10', '2'),
		('diag.verbose', '1'),
		('notice.db_recovered', '1'),
		('helperless', 'чужой ключ с похожим началом')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO parent_auth (id, hash, failed_count, changed_at) VALUES (1, 'hash', 0, 100)`); err != nil {
		t.Fatal(err)
	}
	if n := len(allSettings(t, db)); n != 8 {
		t.Fatalf("до миграции 003 строк настроек %d, want 8", n)
	}

	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 003: %v", err)
	}
	want := map[string]string{
		"firstrun.seen.1":     "1",
		"bonus.rate_per_10":   "2",
		"diag.verbose":        "1",
		"notice.db_recovered": "1",
		"helperless":          "чужой ключ с похожим началом",
	}
	if got := allSettings(t, db); !reflect.DeepEqual(got, want) {
		t.Errorf("настройки после миграции 003 = %v, want %v", got, want)
	}
	if n := count(t, db, "parent_auth"); n != 1 {
		t.Errorf("строк пароля родителя %d, want 1", n)
	}

	// Повторное применение набора ничего не меняет: 003 записана и второй раз не идёт.
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); len(got) < 3 || got[2] != [2]int64{3, 200} {
		t.Errorf("schema_migrations = %v, want третья запись {3 200}", got)
	}
}
