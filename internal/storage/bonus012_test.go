package storage

import (
	"reflect"
	"strings"
	"testing"
)

// Ловит (D-113): миграция 012 теряет строки копилки или их id, задевает детей, журнал и настройки,
// оставляет старый CHECK 0…600, снимает проверку «не ниже нуля» или внешний ключ на ребёнка,
// создаёт лишние индексы.
func TestMigration012OnFilled011Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	all = migrationsBefore(all, 13) // свой срез истории (PL-57)
	db := rawDB(t)
	if err := applyMigrations(db, migrationsBefore(all, 12), 100); err != nil {
		t.Fatalf("миграции 001–011: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, end_mode, created_at) VALUES (1, 'S-1', 'Миша', 'lock', 10)`,
		`INSERT INTO bonus_ledger (id, child_id, at, delta_min, reason, balance_after) VALUES (5, 1, 100, 300, 'за уборку', 300)`,
		`INSERT INTO bonus_ledger (id, child_id, at, delta_min, reason, balance_after) VALUES (7, 1, 200, 300, 'за уроки', 600)`,
		`INSERT INTO bonus_ledger (id, child_id, at, delta_min, reason, balance_after) VALUES (9, 1, 300, -15, 'взято из копилки', 585)`,
		`INSERT INTO events (child_id, at, tz_offset_min, kind, details) VALUES (1, 100, 180, 'bonus_grant', '{}')`,
		`INSERT INTO settings (key, value) VALUES ('bonus.autobonus_from_day', '2026-11-12')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 011: %s: %v", q, err)
		}
	}
	// До 012 старый CHECK не пускает баланс выше 600.
	if _, err := db.Exec(`INSERT INTO bonus_ledger (child_id, at, delta_min, reason, balance_after) VALUES (1, 400, 615, 'x', 1200)`); err == nil {
		t.Fatal("до миграции 012 баланс 1200 прошёл, want ошибку CHECK 0…600")
	}
	const ledgerQ = `SELECT id, child_id, at, delta_min, reason, balance_after FROM bonus_ledger ORDER BY id`
	const childQ = `SELECT id, windows_sid, display_name FROM children ORDER BY id`
	const eventQ = `SELECT id, child_id, at, kind FROM events ORDER BY id`
	const settingsQ = `SELECT key, value FROM settings ORDER BY key`
	const indexQ = `SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'bonus_ledger' AND sql IS NOT NULL`
	snap := func() [][][]string {
		return [][][]string{rowStrings(t, db, ledgerQ), rowStrings(t, db, childQ), rowStrings(t, db, eventQ), rowStrings(t, db, settingsQ)}
	}
	before := snap()
	if got := rowStrings(t, db, indexQ); len(got) != 0 {
		t.Fatalf("индексы bonus_ledger до 012 = %v, want нет", got)
	}

	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 012: %v", err)
	}
	if after := snap(); !reflect.DeepEqual(after, before) {
		t.Errorf("данные после миграции = %v, want %v", after, before)
	}
	if got := rowStrings(t, db, indexQ); len(got) != 0 {
		t.Errorf("индексы bonus_ledger после 012 = %v, want нет", got)
	}
	var schema string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'bonus_ledger'`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schema, "balance_after >= 0") || strings.Contains(schema, "600") {
		t.Errorf("схема bonus_ledger = %s, want CHECK (balance_after >= 0) без 600", schema)
	}
	const ins = `INSERT INTO bonus_ledger (child_id, at, delta_min, reason, balance_after) VALUES (?, 500, 615, 'за лето', ?)`
	if _, err := db.Exec(ins, 1, 1200); err != nil {
		t.Errorf("баланс 1200 после 012: %v, want без ошибки", err)
	}
	if _, err := db.Exec(ins, 1, -1); err == nil {
		t.Error("баланс -1 прошёл, want ошибку CHECK")
	}
	if _, err := db.Exec(ins, 99, 10); err == nil {
		t.Error("строка чужого ребёнка 99 прошла, want ошибку внешнего ключа")
	}
	var maxID int64
	if err := db.QueryRow(`SELECT max(id) FROM bonus_ledger`).Scan(&maxID); err != nil || maxID != 10 {
		t.Errorf("id новой строки = %d, %v; want 10 (после переноса id 9)", maxID, err)
	}
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); len(got) != 12 || got[11] != [2]int64{12, 200} || got[10] != [2]int64{11, 100} {
		t.Errorf("schema_migrations = %v, want 1–11 в 100 и 12 в 200", got)
	}
}
