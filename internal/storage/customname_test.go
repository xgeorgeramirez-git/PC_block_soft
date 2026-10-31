package storage

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Свои названия программ (FB-11, D-59, FR-20): колонка programs.custom_name, миграция 007.

// Ловит: миграция 007 на базе 0.3.0 с FB-9 и FB-12 (001–006 с данными) теряет программы, их
// скрытие или день; custom_name без значения по умолчанию (NOT NULL сломал бы старые строки) или
// не пустое у старых программ; повторное применение что-то меняет.
func TestMigration007OnFilled006Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var v006 []migration
	for _, m := range all {
		if m.version < 7 {
			v006 = append(v006, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, v006, 100); err != nil {
		t.Fatalf("миграции 001–006: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, created_at) VALUES (1, 'S-1', 'Миша', 10)`,
		`INSERT INTO programs (exe_path, display_name, publisher, category, source, fingerprint, app_root, decided_at, hidden) VALUES
			('c:\program files\epic games\stalker2\stalker2.exe', 'Stalker2', 'Epic Games', 'games', 'parent', '', 'c:\program files\epic games', 500, 1),
			('c:\program files\bravesoftware\brave.exe', 'Brave', '', 'other', 'snapshot', '', 'c:\program files\bravesoftware', NULL, 0)`,
		`INSERT INTO day_state (child_id, day, total_s, last_minute_end) VALUES (1, '2026-10-31', 7140, 1792418400000)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 006: %s: %v", q, err)
		}
	}
	const progQ = `SELECT id, exe_path, display_name, publisher, category, source, fingerprint, app_root, ifnull(decided_at, 'NULL'), hidden FROM programs ORDER BY id`
	const dayQ = `SELECT child_id, day, total_s, last_minute_end FROM day_state`
	beforeProg, beforeDay := rowStrings(t, db, progQ), rowStrings(t, db, dayQ)
	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 007: %v", err)
	}
	if got := rowStrings(t, db, progQ); !reflect.DeepEqual(got, beforeProg) {
		t.Errorf("programs после миграции = %v, want %v", got, beforeProg)
	}
	if got := rowStrings(t, db, dayQ); !reflect.DeepEqual(got, beforeDay) {
		t.Errorf("day_state после миграции = %v, want %v", got, beforeDay)
	}
	if got, want := rowStrings(t, db, `SELECT display_name, '[' || custom_name || ']' FROM programs ORDER BY id`),
		[][]string{{"Stalker2", "[]"}, {"Brave", "[]"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("custom_name после миграции = %v, want %v", got, want)
	}
	if _, err := db.Exec(`INSERT INTO programs (exe_path, display_name, category, source, custom_name) VALUES ('c:\y.exe', 'Y', 'other', 'snapshot', ?)`,
		strings.Repeat("я", 61)); err == nil {
		t.Error("custom_name в 61 символ записался, want отказ CHECK (D-59: до 60)")
	}
	if _, err := db.Exec(`INSERT INTO programs (exe_path, display_name, category, source, custom_name) VALUES ('c:\z.exe', 'Z', 'other', 'snapshot', ?)`,
		strings.Repeat("я", 60)); err != nil {
		t.Errorf("custom_name в 60 букв кириллицы: %v, want записан (длина в символах, не в байтах)", err)
	}
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 100}, {6, 100}, {7, 200}}) {
		t.Errorf("schema_migrations = %v, want 1–6 в 100 и 7 в 200", got)
	}
}

// Ловит: своё название не читается и не пишется (Get/List/Upsert/Restore); SetProgramName трогает
// название из файла, категорию, скрытие или момент решения; не отдаёт прежнее значение для
// «Отменить»; программа, которой нет, — успех; длиннее 60 символов записывается; снимок программ
// (SeedPrograms) затирает своё название.
func TestSetProgramNameAndUndo(t *testing.T) {
	s := openTemp(t)
	ship := Program{Path: pathShipping, Name: "Stalker2-Win64-Shipping", Publisher: "GSC Game World", Category: ProgramGames,
		Source: SourceCatalog, AppRoot: `c:\program files\epic games`, DecidedAt: 700, Hidden: false}
	if err := s.UpsertProgram(ship); err != nil {
		t.Fatal(err)
	}
	p0, _, _ := s.GetProgram(pathShipping)
	if p0.CustomName != "" {
		t.Fatalf("новая программа со своим названием: %+v", p0)
	}

	prev, ok, err := s.SetProgramName(pathShipping, "Сталкер 2")
	if err != nil || !ok || prev != p0 {
		t.Fatalf("SetProgramName = %+v, %v, %v; want %+v, true, nil", prev, ok, err, p0)
	}
	p1, _, _ := s.GetProgram(pathShipping)
	want1 := p0
	want1.CustomName = "Сталкер 2"
	if p1 != want1 {
		t.Errorf("после переименования = %+v, want %+v (название из файла и остальное прежние)", p1, want1)
	}
	list, _ := s.ListPrograms()
	if len(list) != 1 || list[0].CustomName != "Сталкер 2" || list[0].Name != "Stalker2-Win64-Shipping" {
		t.Errorf("ListPrograms = %+v", list)
	}

	// Повторный снимок своё название не трогает.
	if _, err := s.SeedPrograms([]Program{{Path: pathShipping, Name: "Другое", Category: ProgramOther, Source: SourceSnapshot}}); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := s.GetProgram(pathShipping); p != p1 {
		t.Errorf("после снимка = %+v, want %+v", p, p1)
	}

	// Сброс пустым названием и «Отменить» (RestoreProgram прежнего значения).
	if prev, ok, err := s.SetProgramName(pathShipping, ""); err != nil || !ok || prev != p1 {
		t.Errorf("сброс = %+v, %v, %v; want %+v", prev, ok, err, p1)
	}
	if p, _, _ := s.GetProgram(pathShipping); p != p0 {
		t.Errorf("после сброса = %+v, want %+v", p, p0)
	}
	if err := s.RestoreProgram(p1); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := s.GetProgram(pathShipping); p != p1 {
		t.Errorf("RestoreProgram = %+v, want %+v", p, p1)
	}

	// Отказы.
	if _, ok, err := s.SetProgramName(`c:\нет.exe`, "X"); err != nil || ok {
		t.Errorf("нет программы: ok %v, err %v; want false, nil", ok, err)
	}
	if _, _, err := s.SetProgramName(pathShipping, strings.Repeat("я", 61)); !errors.Is(err, ErrInvalid) {
		t.Errorf("61 символ: %v, want ErrInvalid", err)
	}
	if err := s.UpsertProgram(Program{Path: `c:\a.exe`, Name: "A", Category: ProgramOther, Source: SourceSnapshot, CustomName: strings.Repeat("я", 61)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("Upsert с 61 символом: %v, want ErrInvalid", err)
	}
	if _, _, err := s.SetProgramName(pathShipping, strings.Repeat("я", 60)); err != nil {
		t.Errorf("60 символов кириллицы: %v", err)
	}
}
