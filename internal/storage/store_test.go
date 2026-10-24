package storage

import (
	"bytes"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"pcblock/internal/core"
)

// openTemp открывает чистую базу во временной папке теста; закрытие регистрируется в Cleanup.
func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "pcblock.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// fixClock подменяет часы хранилища на неподвижный момент (UTC, миллисекунды).
func fixClock(t *testing.T, ms int64) {
	t.Helper()
	old := timeNow
	timeNow = func() time.Time { return time.UnixMilli(ms).UTC() }
	t.Cleanup(func() { timeNow = old })
}

// mustChild добавляет ребёнка и возвращает его id.
func mustChild(t *testing.T, s *Store, sid, name string) int64 {
	t.Helper()
	id, err := s.UpsertChild(Child{SID: sid, Name: name})
	if err != nil {
		t.Fatalf("UpsertChild(%s): %v", sid, err)
	}
	return id
}

// rawNull читает одно целое из базы напрямую, мимо методов хранилища.
func rawNull(t *testing.T, s *Store, query string, args ...any) sql.NullInt64 {
	t.Helper()
	var v sql.NullInt64
	if err := s.db.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// Ловит: таблица или поле ТЗ 4 потерялись или переименованы при переносе схемы в миграцию
// (в том числе поля M2+, bypass_detail и removed_at), и тогда следующая веха потребует
// повторной миграции.
func TestOpenCreatesFullSchema(t *testing.T) {
	s := openTemp(t)
	want := map[string][]string{
		"children":           {"id", "windows_sid", "display_name", "end_mode", "adult_filter", "bypass_detail", "removed_at", "created_at"},
		"day_limits":         {"child_id", "weekday", "total_min", "games_min", "video_min"},
		"schedule_intervals": {"id", "child_id", "weekday", "start_min", "end_min"},
		"programs":           {"id", "exe_path", "display_name", "category", "source", "decided_at"},
		"pending_programs":   {"id", "exe_path", "display_name", "first_child_id", "first_attempt_at", "attempts"},
		"site_rules":         {"id", "domain", "kind"},
		"usage_daily":        {"child_id", "day", "category", "subject_kind", "subject", "seconds"},
		"day_state":          {"child_id", "day", "total_s", "games_s", "video_s", "study_s", "extra_min", "pause_until", "bonus_granted", "no_forbidden"},
		"bonus_ledger":       {"id", "child_id", "at", "delta_min", "reason", "balance_after"},
		"events":             {"id", "child_id", "at", "tz_offset_min", "kind", "details"},
		"promo_codes":        {"id", "code_norm", "code_display", "minutes", "created_at", "used_at", "used_by_child"},
		"parent_auth":        {"id", "hash", "failed_count", "blocked_until", "changed_at"},
		"settings":           {"key", "value"},
		"schema_migrations":  {"version", "applied_at"},
	}
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("список таблиц: %v", err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, n)
	}
	rows.Close()
	var wantTables []string
	for n := range want {
		wantTables = append(wantTables, n)
	}
	sort.Strings(wantTables)
	if !reflect.DeepEqual(tables, wantTables) {
		t.Fatalf("таблицы = %v, want %v", tables, wantTables)
	}
	for _, table := range wantTables {
		cols, err := s.db.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
		if err != nil {
			t.Fatalf("поля %s: %v", table, err)
		}
		var got []string
		for cols.Next() {
			var n string
			if err := cols.Scan(&n); err != nil {
				t.Fatal(err)
			}
			got = append(got, n)
		}
		cols.Close()
		if !reflect.DeepEqual(got, want[table]) {
			t.Errorf("поля %s = %v, want %v", table, got, want[table])
		}
	}
}

// Ловит: ограничения CHECK из ТЗ 4 потеряны (одна строка пароля, дни недели 1…7, режим
// конца времени, потолок копилки), и в базу попадают значения, которые ядро не умеет читать.
func TestSchemaConstraints(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	bad := []struct{ name, query string }{
		{"parent_auth id=2", `INSERT INTO parent_auth (id, hash, changed_at) VALUES (2, 'h', 1)`},
		{"end_mode вне списка", `UPDATE children SET end_mode = 'kill' WHERE id = ` + itoa(id)},
		{"weekday 8", `INSERT INTO day_limits (child_id, weekday, total_min) VALUES (` + itoa(id) + `, 8, 10)`},
		{"лимит 1441 мин", `INSERT INTO day_limits (child_id, weekday, total_min) VALUES (` + itoa(id) + `, 1, 1441)`},
		{"копилка 601", `INSERT INTO bonus_ledger (child_id, at, delta_min, reason, balance_after) VALUES (` + itoa(id) + `, 1, 1, 'x', 601)`},
	}
	for _, tc := range bad {
		if _, err := s.db.Exec(tc.query); err == nil {
			t.Errorf("%s: запрос прошёл, want ошибка ограничения", tc.name)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Ловит: при открытии не включены журнал WAL (NFR-8), ожидание занятой базы 5 с или
// внешние ключи (без них каскад и проверка ссылок молча не работают).
func TestOpenPragmas(t *testing.T) {
	s := openTemp(t)
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
	if got := rawNull(t, s, `PRAGMA foreign_keys`); got.Int64 != 1 {
		t.Errorf("foreign_keys = %d, want 1", got.Int64)
	}
	if got := rawNull(t, s, `PRAGMA busy_timeout`); got.Int64 != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", got.Int64)
	}
	if n := s.db.Stats().MaxOpenConnections; n != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", n)
	}
}

// Ловит: внешние ключи объявлены в схеме, но не включены в соединении: состояние дня
// несуществующего ребёнка записалось бы без ошибки.
func TestForeignKeysAreEnforced(t *testing.T) {
	s := openTemp(t)
	if err := s.SaveDayState(999, core.DayState{Day: "2026-10-19"}); err == nil {
		t.Fatal("SaveDayState для ребёнка 999 прошёл, want ошибка внешнего ключа")
	}
	if got := rawNull(t, s, `SELECT count(*) FROM day_state`); got.Int64 != 0 {
		t.Fatalf("строк day_state = %d, want 0", got.Int64)
	}
}

// Ловит: повторное открытие применяет миграцию второй раз (ошибка «таблица уже есть» или
// потеря данных) либо переписывает момент применения; данные после перезапуска службы теряются.
func TestOpenTwiceKeepsDataAndMigrationOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcblock.db")
	fixClock(t, 1_000)
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("первое Open: %v", err)
	}
	id := mustChild(t, s1, "S-1-5-21-1", "Миша")
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	fixClock(t, 2_000)
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("второе Open: %v", err)
	}
	defer s2.Close()
	if n := rawNull(t, s2, `SELECT count(*) FROM schema_migrations`); n.Int64 != 2 {
		t.Errorf("строк schema_migrations = %d, want 2", n.Int64)
	}
	if v := rawNull(t, s2, `SELECT max(version) FROM schema_migrations`); v.Int64 != 2 {
		t.Errorf("последняя миграция = %d, want 2", v.Int64)
	}
	if at := rawNull(t, s2, `SELECT applied_at FROM schema_migrations WHERE version = 1`); at.Int64 != 1_000 {
		t.Errorf("applied_at миграции 1 = %d, want 1000 (момент первого применения)", at.Int64)
	}
	if at := rawNull(t, s2, `SELECT applied_at FROM schema_migrations WHERE version = 2`); at.Int64 != 1_000 {
		t.Errorf("applied_at миграции 2 = %d, want 1000 (обе применяются при первом открытии, не повторно)", at.Int64)
	}
	kids, err := s2.ListChildren()
	if err != nil {
		t.Fatal(err)
	}
	want := []Child{{ID: id, SID: "S-1-5-21-1", Name: "Миша", CreatedAtMs: 1_000}}
	if !reflect.DeepEqual(kids, want) {
		t.Fatalf("ListChildren после перезапуска = %+v, want %+v", kids, want)
	}
}

// Ловит: файл с мусором вместо базы открывается как рабочий или даёт обычную ошибку без
// ErrCorrupt, и служба не узнаёт, что нужно сохранить копию и перейти в безопасный режим (R18, NFR-8).
func TestOpenGarbageFileIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcblock.db")
	garbage := bytes.Repeat([]byte("это не база данных, а мусор. "), 300)
	if err := os.WriteFile(path, garbage, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err == nil {
		s.Close()
		t.Fatal("Open мусорного файла прошёл, want ErrCorrupt")
	}
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Open = %v, want ErrCorrupt (через errors.Is)", err)
	}
	if s != nil {
		t.Fatal("Open вернул хранилище вместе с ошибкой")
	}
	fixClock(t, 1_792_423_845_000) // 2026-10-19 15:30:45 UTC
	copyPath, err := Quarantine(path)
	if err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	if want := path + ".broken-20261019-153045"; copyPath != want {
		t.Errorf("путь копии = %q, want %q", copyPath, want)
	}
	got, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatalf("копия не прочитана: %v", err)
	}
	if !bytes.Equal(got, garbage) {
		t.Error("содержимое копии не равно исходному файлу")
	}
}

// Ловит: база с правильной шапкой, но разрушенными страницами проходит открытие, потому что
// проверяется только шапка файла, а PRAGMA integrity_check не вызывается (Ruling 25).
func TestOpenDamagedPagesIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcblock.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mustChild(t, s, "S-1-5-21-1", "Миша")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 5*4096 {
		t.Fatalf("файл базы %d байт, want не меньше %d", len(data), 5*4096)
	}
	for i := 4096; i < 4*4096; i++ {
		data[i] = 0xFF
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err == nil {
		s2.Close()
		t.Fatal("Open базы с разрушенными страницами прошёл, want ErrCorrupt")
	}
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Open = %v, want ErrCorrupt", err)
	}
}

// Ловит: любая ошибка открытия (нет папки, нет прав, файл занят) считается порчей базы, и
// служба отправляет в карантин здоровую базу, а ребёнок остаётся без ограничений.
func TestOpenMissingDirIsNotCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "нет", "такой", "папки", "pcblock.db")
	s, err := Open(path)
	if err == nil {
		s.Close()
		t.Fatal("Open в несуществующей папке прошёл, want ошибка")
	}
	if errors.Is(err, ErrCorrupt) {
		t.Fatalf("Open = %v: ошибка пути принята за порчу базы", err)
	}
}

// Ловит: карантин забывает журнал -wal (последние записи теряются для разбора), портит
// оригинал, перезаписывает прежнюю копию при двух сбоях за одну секунду или молча
// «успешно» работает с несуществующим файлом.
func TestQuarantine(t *testing.T) {
	fixClock(t, 1_792_423_845_000) // 2026-10-19 15:30:45 UTC
	dir := t.TempDir()
	path := filepath.Join(dir, "pcblock.db")
	if err := os.WriteFile(path, []byte("DB-BYTES"), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := Quarantine(path)
	if err != nil {
		t.Fatalf("Quarantine без -wal: %v", err)
	}
	if want := path + ".broken-20261019-153045"; first != want {
		t.Errorf("первая копия = %q, want %q", first, want)
	}
	if _, err := os.Stat(first + "-wal"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("копия -wal без исходного -wal: Stat = %v, want ErrNotExist", err)
	}

	if err := os.WriteFile(path+"-wal", []byte("WAL-BYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := Quarantine(path)
	if err != nil {
		t.Fatalf("Quarantine с -wal: %v", err)
	}
	if want := path + ".broken-20261019-153045-1"; second != want {
		t.Errorf("вторая копия в ту же секунду = %q, want %q", second, want)
	}
	for file, want := range map[string]string{
		first:           "DB-BYTES",
		second:          "DB-BYTES",
		second + "-wal": "WAL-BYTES",
		path:            "DB-BYTES",
		path + "-wal":   "WAL-BYTES",
	} {
		got, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("чтение %s: %v", file, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", filepath.Base(file), got, want)
		}
	}

	_, err = Quarantine(filepath.Join(dir, "нет-такого.db"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Quarantine несуществующего файла = %v, want fs.ErrNotExist", err)
	}
}

// Ловит: ошибка базы глотается и метод возвращает «пусто» вместо ошибки (L-129): служба
// приняла бы закрытое или сломанное хранилище за пустое и сняла бы с ребёнка лимиты.
func TestMethodsReturnErrorsWhenStoreClosed(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("повторный Close = %v, want nil", err)
	}
	calls := map[string]func() error{
		"UpsertChild":  func() error { _, err := s.UpsertChild(Child{SID: "S-2", Name: "Аня"}); return err },
		"ListChildren": func() error { _, err := s.ListChildren(); return err },
		"RemoveChild":  func() error { return s.RemoveChild(id) },
		"SetBypass":    func() error { return s.SetBypass(id, "x") },
		"SetDayLimits": func() error { return s.SetDayLimits(id, [7]core.DayLimit{}) },
		"GetDayLimits": func() error { _, err := s.GetDayLimits(id); return err },
		"LoadDayState": func() error { _, err := s.LoadDayState(id, "2026-10-19"); return err },
		"SaveDayState": func() error { return s.SaveDayState(id, core.DayState{Day: "2026-10-19"}) },
		"AppendEvent":  func() error { return s.AppendEvent(Event{Kind: "bypass"}) },
		"ListEvents":   func() error { _, err := s.ListEvents(EventFilter{}); return err },
		"GetAuth":      func() error { _, _, err := s.GetAuth(); return err },
		"SetAuth":      func() error { return s.SetAuth(Auth{Hash: "h"}) },
		"GetSetting":   func() error { _, _, err := s.GetSetting("k"); return err },
		"SetSetting":   func() error { return s.SetSetting("k", "v") },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s на закрытом хранилище вернул nil, want ошибку", name)
		}
	}
}
