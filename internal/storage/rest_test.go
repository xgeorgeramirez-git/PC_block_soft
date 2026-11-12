package storage

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"pcblock/internal/core"
)

// Перерыв на отдых (FB-14, D-62): таблицы break_rules и break_state, миграция 008.

// Ловит: миграция 008 на базе 0.3.1 (001–007 с данными) теряет детей, день, программы со своим
// названием; не создаёт таблицы или создаёт их с данными; повторное применение что-то меняет.
func TestMigration008OnFilled007Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var v007 []migration
	for _, m := range all {
		if m.version < 8 {
			v007 = append(v007, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, v007, 100); err != nil {
		t.Fatalf("миграции 001–007: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, end_mode, created_at) VALUES (1, 'S-1', 'Миша', 'study_only', 10)`,
		`INSERT INTO day_state (child_id, day, total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden, last_minute_end) VALUES
			(1, '2026-11-02', 7141, 1800, 600, 300, 15, NULL, 1, 0, 1792418400000)`,
		`INSERT INTO programs (exe_path, display_name, category, source, hidden, custom_name) VALUES ('c:\games\x.exe', 'X', 'games', 'parent', 0, 'Майнкрафт')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 007: %s: %v", q, err)
		}
	}
	const childQ = `SELECT id, windows_sid, display_name, end_mode FROM children`
	const dayQ = `SELECT child_id, day, total_s, games_s, extra_min, bonus_granted, ifnull(last_minute_end, 'NULL') FROM day_state`
	const progQ = `SELECT exe_path, display_name, category, hidden, custom_name FROM programs`
	before := [][][]string{rowStrings(t, db, childQ), rowStrings(t, db, dayQ), rowStrings(t, db, progQ)}
	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 008: %v", err)
	}
	after := [][][]string{rowStrings(t, db, childQ), rowStrings(t, db, dayQ), rowStrings(t, db, progQ)}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("данные после миграции = %v, want %v", after, before)
	}
	if got := rowStrings(t, db, `SELECT (SELECT count(*) FROM break_rules), (SELECT count(*) FROM break_state)`); !reflect.DeepEqual(got, [][]string{{"0", "0"}}) {
		t.Errorf("строк break_rules и break_state = %v, want 0 и 0", got)
	}
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 100}, {6, 100}, {7, 100}, {8, 200}, {9, 200}, {10, 200}}) {
		t.Errorf("schema_migrations = %v, want 1–7 в 100 и 8–9 в 200", got)
	}
}

// Ловит: строки нет — не настройки по умолчанию; поля не сохраняются или путаются; пустое
// сообщение не уходит в NULL; пробелы по краям сохраняются.
func TestBreakRulesSaveLoad(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	got, err := s.GetBreakRules(id)
	if err != nil {
		t.Fatal(err)
	}
	if want := (core.BreakRules{AfterMin: 60, Counts: core.BreakAll, WarnMin: 5, RestMin: 15, SelfRestMin: 15}); got != want {
		t.Fatalf("без строки %+v, want %+v", got, want)
	}
	b := core.BreakRules{Enabled: true, AfterMin: 45, Counts: core.BreakGames, WarnMin: 3, RestMin: 20, SelfRestMin: 30, Message: "  Пойди погуляй с собакой "}
	if err := s.SetBreakRules(id, b); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetBreakRules(id)
	if err != nil {
		t.Fatal(err)
	}
	if want := (core.BreakRules{Enabled: true, AfterMin: 45, Counts: core.BreakGames, WarnMin: 3, RestMin: 20, SelfRestMin: 30, Message: "Пойди погуляй с собакой"}); got != want {
		t.Fatalf("после записи %+v, want %+v", got, want)
	}
	b.Message, b.Enabled = "", false
	if err := s.SetBreakRules(id, b); err != nil {
		t.Fatal(err)
	}
	if n := rawNull(t, s, `SELECT count(*) FROM break_rules WHERE child_id = ? AND message IS NULL AND enabled = 0 AND after_min = 45`, id); n.Int64 != 1 {
		t.Fatalf("пустое сообщение и выключенный переключатель: строк %d, want 1 (message NULL, значения сохранены)", n.Int64)
	}
}

// Ловит: вне пределов FR-37 (и «засчитать как отдых» меньше «отдыха») попадает в базу.
func TestBreakRulesRejectsOutOfRange(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	for name, b := range map[string]core.BreakRules{
		"через 14":     {AfterMin: 14, WarnMin: 5, RestMin: 15, SelfRestMin: 15},
		"сам < отдыха": {AfterMin: 60, WarnMin: 5, RestMin: 20, SelfRestMin: 15},
		"121 символ":   {AfterMin: 60, WarnMin: 5, RestMin: 15, SelfRestMin: 15, Message: strings.Repeat("я", 121)},
	} {
		if err := s.SetBreakRules(id, b); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if n := rawNull(t, s, `SELECT count(*) FROM break_rules`); n.Int64 != 0 {
		t.Fatalf("строк break_rules %d, want 0", n.Int64)
	}
	// CHECK базы держит порог и в обход проверки (чужая правка, D-45).
	if _, err := s.db.Exec(`INSERT INTO break_rules (child_id, rest_min, self_rest_min) VALUES (?, 20, 19)`, id); err == nil {
		t.Fatal("CHECK self_rest_min >= rest_min не сработал")
	}
}

// Ловит: момент начала и конец перерыва, тишина и счёт не сохраняются или не читаются после
// перезапуска (RF14-2); nil не пишет NULL; warned не следует за break_at.
func TestBreakStateSaveLoad(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	if got, err := s.LoadBreakState(id); err != nil || got != (core.BreakState{}) {
		t.Fatalf("без строки %+v, %v; want пусто", got, err)
	}
	quiet, at, until := int64(1792418400000), int64(1792418700000), int64(1792419600000)
	if err := s.SaveBreakState(id, core.BreakState{CountedS: 3600, QuietS: 120, QuietSinceMs: &quiet, BreakAtMs: &at}); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadBreakState(id)
	if err != nil || got.CountedS != 3600 || got.QuietS != 120 || got.QuietSinceMs == nil || *got.QuietSinceMs != quiet ||
		got.BreakAtMs == nil || *got.BreakAtMs != at || got.RestUntilMs != nil {
		t.Fatalf("плашка показана: %+v, %v", got, err)
	}
	if n := rawNull(t, s, `SELECT warned FROM break_state WHERE child_id = ?`, id); n.Int64 != 1 {
		t.Fatalf("warned = %d, want 1", n.Int64)
	}
	if err := s.SaveBreakState(id, core.BreakState{RestUntilMs: &until}); err != nil {
		t.Fatal(err)
	}
	got, err = s.LoadBreakState(id)
	if err != nil || got.RestUntilMs == nil || *got.RestUntilMs != until || got.BreakAtMs != nil || got.QuietSinceMs != nil || got.CountedS != 0 {
		t.Fatalf("перерыв идёт: %+v, %v", got, err)
	}
	if n := rawNull(t, s, `SELECT warned FROM break_state WHERE child_id = ?`, id); n.Int64 != 0 {
		t.Fatalf("warned = %d, want 0", n.Int64)
	}
}
