package storage

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// rawDB открывает пустую базу напрямую, без Open: для проверки применения миграций.
func rawDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "raw.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Первая миграция тестового набора создаёт schema_migrations так же, как настоящая 001_init.sql.
const testFirst = "-- Назначение: проба\n" +
	"CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);\n" +
	"CREATE TABLE a (x INTEGER);\n" +
	"INSERT INTO a VALUES (1);\n"

func versions(t *testing.T, db *sql.DB) [][2]int64 {
	t.Helper()
	rows, err := db.Query(`SELECT version, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][2]int64
	for rows.Next() {
		var v, at int64
		if err := rows.Scan(&v, &at); err != nil {
			t.Fatal(err)
		}
		out = append(out, [2]int64{v, at})
	}
	return out
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// Ловит: настоящие миграции в бинарнике не встроены, названы без номера, номера идут с
// пропуском или без «-- Назначение:» в первой строке (правило плана T7).
func TestEmbeddedMigrationsAreWellFormed(t *testing.T) {
	ms, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(ms) < 1 {
		t.Fatal("встроенных миграций нет")
	}
	for i, m := range ms {
		if m.version != i+1 {
			t.Errorf("миграция %s: версия %d, want %d (без пропусков, с единицы)", m.name, m.version, i+1)
		}
		if !strings.HasPrefix(m.sql, "-- Назначение:") {
			t.Errorf("миграция %s: первая строка не начинается с «-- Назначение:»", m.name)
		}
	}
	if ms[0].name != "001_init.sql" {
		t.Errorf("первая миграция = %q, want %q", ms[0].name, "001_init.sql")
	}
}

// Ловит: порядок применения идёт по строке имени (10 раньше 2), неверное имя файла молча
// пропускается, два файла с одним номером применяются оба, пустой набор считается нормой.
func TestLoadMigrations(t *testing.T) {
	ok := fstest.MapFS{
		"m/10_ten.sql":  {Data: []byte("-- Назначение: десять\nSELECT 10;")},
		"m/2_two.sql":   {Data: []byte("-- Назначение: два\nSELECT 2;")},
		"m/001_one.sql": {Data: []byte("-- Назначение: один\nSELECT 1;")},
		"m/readme.txt":  {Data: []byte("не миграция")},
	}
	ms, err := loadMigrations(ok, "m")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var got []int
	for _, m := range ms {
		got = append(got, m.version)
	}
	if want := []int{1, 2, 10}; !reflect.DeepEqual(got, want) {
		t.Errorf("порядок версий = %v, want %v", got, want)
	}
	if ms[0].name != "001_one.sql" || ms[0].sql != "-- Назначение: один\nSELECT 1;" {
		t.Errorf("первая миграция = %+v", ms[0])
	}

	bad := map[string]fstest.MapFS{
		"имя без номера":     {"m/init.sql": {Data: []byte("SELECT 1;")}},
		"номер без имени":    {"m/001.sql": {Data: []byte("SELECT 1;")}},
		"нулевой номер":      {"m/000_zero.sql": {Data: []byte("SELECT 1;")}},
		"повтор номера":      {"m/001_a.sql": {Data: []byte("SELECT 1;")}, "m/1_b.sql": {Data: []byte("SELECT 2;")}},
		"нет миграций":       {"m/readme.txt": {Data: []byte("x")}},
		"нет такой папки":    {"other/001_a.sql": {Data: []byte("SELECT 1;")}},
		"огромный номер":     {"m/99999999999999999999_a.sql": {Data: []byte("SELECT 1;")}},
		"пустой файл":        {"m/001_a.sql": {Data: []byte("")}},
		"пробелы в названии": {"m/001_a b.sql": {Data: []byte("SELECT 1;")}},
	}
	for name, fsys := range bad {
		if ms, err := loadMigrations(fsys, "m"); err == nil {
			t.Errorf("%s: loadMigrations прошёл (%d шт.), want ошибка", name, len(ms))
		}
	}
}

// Ловит: при появлении второй миграции первая применяется снова (данные дублируются или
// «таблица уже есть»), а новая теряется; момент применения записан не тот.
func TestApplyMigrationsOnlyNewOnes(t *testing.T) {
	db := rawDB(t)
	v1 := fstest.MapFS{"m/001_a.sql": {Data: []byte(testFirst)}}
	ms1, err := loadMigrations(v1, "m")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(db, ms1, 111); err != nil {
		t.Fatalf("первое применение: %v", err)
	}
	if err := applyMigrations(db, ms1, 999); err != nil {
		t.Fatalf("повторное применение того же набора: %v", err)
	}

	v2 := fstest.MapFS{
		"m/001_a.sql": {Data: []byte(testFirst)},
		"m/002_b.sql": {Data: []byte("-- Назначение: вторая\nCREATE TABLE b (y INTEGER);\nINSERT INTO a VALUES (2);\n")},
	}
	ms2, err := loadMigrations(v2, "m")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(db, ms2, 222); err != nil {
		t.Fatalf("применение второй: %v", err)
	}
	if got, want := versions(t, db), [][2]int64{{1, 111}, {2, 222}}; !reflect.DeepEqual(got, want) {
		t.Errorf("schema_migrations = %v, want %v", got, want)
	}
	if n := count(t, db, "a"); n != 2 {
		t.Errorf("строк в a = %d, want 2 (первая миграция выполнена один раз)", n)
	}
	if n := count(t, db, "b"); n != 0 {
		t.Errorf("строк в b = %d, want 0 (таблица создана)", n)
	}
}

// Ловит: миграция идёт не в транзакции, и после сбоя на середине остаётся половина схемы
// и номер миграции не записан (или записан без схемы): служба не стартует ни с новой, ни со старой базой.
func TestApplyMigrationsRollsBackFailedStep(t *testing.T) {
	db := rawDB(t)
	fsys := fstest.MapFS{
		"m/001_a.sql":   {Data: []byte(testFirst)},
		"m/002_bad.sql": {Data: []byte("-- Назначение: сломанная\nCREATE TABLE b (y INTEGER);\nINSERT INTO no_such_table VALUES (1);\n")},
	}
	ms, err := loadMigrations(fsys, "m")
	if err != nil {
		t.Fatal(err)
	}
	err = applyMigrations(db, ms, 500)
	if err == nil {
		t.Fatal("applyMigrations со сломанной второй миграцией прошёл, want ошибка")
	}
	if !strings.Contains(err.Error(), "002_bad.sql") {
		t.Errorf("ошибка %q не называет файл 002_bad.sql", err)
	}
	var tables int
	if e := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'b'`).Scan(&tables); e != nil {
		t.Fatal(e)
	}
	if tables != 0 {
		t.Error("таблица b осталась после отката миграции 002")
	}
	if got, want := versions(t, db), [][2]int64{{1, 500}}; !reflect.DeepEqual(got, want) {
		t.Errorf("schema_migrations = %v, want %v (001 применена, 002 нет)", got, want)
	}
}

// Ловит: миграция без таблицы schema_migrations применяется «успешно» без отметки о
// применении, и при каждом запуске выполняется снова.
func TestApplyMigrationsFailsWithoutVersionTable(t *testing.T) {
	db := rawDB(t)
	ms, err := loadMigrations(fstest.MapFS{"m/001_a.sql": {Data: []byte("-- Назначение: x\nCREATE TABLE a (x INTEGER);\n")}}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(db, ms, 1); err == nil {
		t.Fatal("applyMigrations без таблицы schema_migrations прошёл, want ошибка")
	}
	var n int
	if e := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'a'`).Scan(&n); e != nil {
		t.Fatal(e)
	}
	if n != 0 {
		t.Error("таблица a осталась после отката")
	}
}
