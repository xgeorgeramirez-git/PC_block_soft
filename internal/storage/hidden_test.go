package storage

import (
	"reflect"
	"testing"
)

// Скрытые программы (D-57, RF9-1, FR-20): флаг programs.hidden, категория сохраняется.

// Ловит: миграция 005 на базе 0.3.0 (001–004 с данными) теряет программы, «Ждут решения»,
// учёт или детей; колонка hidden без значения по умолчанию (NOT NULL сломал бы старые строки)
// или со значением «скрыта»; повторное применение что-то меняет.
func TestMigration005OnFilled030Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var v030 []migration
	for _, m := range all {
		if m.version < 5 {
			v030 = append(v030, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, v030, 100); err != nil {
		t.Fatalf("миграции 0.3.0: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, end_mode, created_at) VALUES (1, 'S-1', 'Миша', 'study_only', 10)`,
		`INSERT INTO programs (exe_path, display_name, publisher, category, source, fingerprint, app_root, decided_at) VALUES
			('c:\program files\epic games\stalker2\stalker2.exe', 'Stalker2', 'Epic Games', 'games', 'parent', '', 'c:\program files\epic games', 500),
			('c:\program files\bravesoftware\brave.exe', 'Brave', '', 'other', 'snapshot', '', 'c:\program files\bravesoftware', NULL)`,
		`INSERT INTO pending_programs (exe_path, display_name, first_child_id, first_attempt_at, attempts, last_attempt_at) VALUES ('c:\users\миша\downloads\supergame.exe', 'SuperGame', 1, 777, 3, 778)`,
		`INSERT INTO usage_daily (child_id, day, category, subject_kind, subject, seconds) VALUES (1, '2026-10-30', 'games', 'program', 'c:\program files\epic games\stalker2\stalker2.exe', 600)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 0.3.0: %s: %v", q, err)
		}
	}
	keep := map[string]string{
		"children":         `SELECT id, windows_sid, display_name, end_mode, created_at FROM children`,
		"programs":         `SELECT id, exe_path, display_name, publisher, category, source, fingerprint, app_root, decided_at FROM programs ORDER BY id`,
		"pending_programs": `SELECT exe_path, display_name, first_child_id, first_attempt_at, attempts, last_attempt_at FROM pending_programs`,
		"usage_daily":      `SELECT child_id, day, category, subject_kind, subject, seconds FROM usage_daily`,
	}
	before := map[string][][]string{}
	for name, q := range keep {
		before[name] = rowStrings(t, db, q)
	}
	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 005: %v", err)
	}
	for name, q := range keep {
		if got := rowStrings(t, db, q); !reflect.DeepEqual(got, before[name]) {
			t.Errorf("%s после миграции = %v, want %v", name, got, before[name])
		}
	}
	if got, want := rowStrings(t, db, `SELECT display_name, hidden FROM programs ORDER BY id`), [][]string{{"Stalker2", "0"}, {"Brave", "0"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("hidden после миграции = %v, want %v", got, want)
	}
	if _, err := db.Exec(`INSERT INTO programs (exe_path, display_name, category, source) VALUES ('c:\x.exe', 'X', 'other', 'snapshot')`); err != nil {
		t.Fatalf("вставка без hidden: %v", err)
	}
	if got := rowStrings(t, db, `SELECT hidden FROM programs WHERE exe_path = 'c:\x.exe'`); !reflect.DeepEqual(got, [][]string{{"0"}}) {
		t.Errorf("hidden по умолчанию = %v, want 0", got)
	}
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 200}}) {
		t.Errorf("schema_migrations = %v, want 1–4 в 100 и 5 в 200", got)
	}
}

const (
	pathStalker  = `c:\program files\epic games\stalker2\stalker2.exe`
	pathShipping = `c:\program files\epic games\stalker2\stalker2\binaries\win64\stalker2-win64-shipping.exe`
)

// Ловит: флаг «скрыта» не читается и не пишется (Get/List/Upsert), скрытие меняет категорию
// или источник (RF9-1: «Показать» возвращает прежнюю категорию), трогает программы, которых
// нет, системные или уже скрытые, не отдаёт прежние значения для «Отменить».
func TestSetProgramsHiddenAndUndo(t *testing.T) {
	s := openTemp(t)
	stalker := Program{Path: pathStalker, Name: "Stalker2", Publisher: "Epic Games", Category: ProgramGames, Source: SourceParent,
		AppRoot: `c:\program files\epic games`, DecidedAt: 500}
	shipping := Program{Path: pathShipping, Name: "Stalker2-Win64-Shipping", Category: ProgramGames, Source: SourceCatalog,
		AppRoot: `c:\program files\epic games`}
	sys := Program{Path: `c:\program files\windows defender\msmpeng.exe`, Name: "Defender", Category: ProgramSystem, Source: SourceSnapshot}
	for _, p := range []Program{stalker, shipping, sys} {
		if err := s.UpsertProgram(p); err != nil {
			t.Fatal(err)
		}
	}
	st0, _, _ := s.GetProgram(pathStalker)
	if st0.Hidden {
		t.Fatalf("новая программа скрыта: %+v", st0)
	}

	changed, err := s.SetProgramsHidden([]string{pathStalker, `c:\нет.exe`, sys.Path, pathStalker}, true)
	if err != nil {
		t.Fatalf("SetProgramsHidden: %v", err)
	}
	if want := []Program{st0}; !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed = %+v, want %+v (только Stalker2: третьей нет, системную не скрывают)", changed, want)
	}
	st1, _, _ := s.GetProgram(pathStalker)
	want1 := st0
	want1.Hidden = true
	if st1 != want1 {
		t.Errorf("Stalker2 после скрытия = %+v, want %+v (категория, источник и момент решения прежние)", st1, want1)
	}
	if p, _, _ := s.GetProgram(sys.Path); p.Hidden {
		t.Errorf("системная скрыта: %+v", p)
	}
	list, err := s.ListPrograms()
	if err != nil {
		t.Fatal(err)
	}
	hidden := map[string]bool{}
	for _, p := range list {
		hidden[p.Path] = p.Hidden
	}
	if want := map[string]bool{pathStalker: true, pathShipping: false, sys.Path: false}; !reflect.DeepEqual(hidden, want) {
		t.Errorf("ListPrograms hidden = %v, want %v", hidden, want)
	}
	if got, err := s.SetProgramsHidden([]string{pathStalker}, true); err != nil || got == nil || len(got) != 0 {
		t.Errorf("повторное скрытие = %#v, %v; want [], nil", got, err)
	}

	// «Показать»: флаг снят, категория прежняя.
	shown, err := s.SetProgramsHidden([]string{pathStalker, pathShipping}, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Program{st1}; !reflect.DeepEqual(shown, want) {
		t.Errorf("показано = %+v, want %+v (Shipping и так не скрыта)", shown, want)
	}
	if p, _, _ := s.GetProgram(pathStalker); p != st0 {
		t.Errorf("после «Показать» = %+v, want %+v", p, st0)
	}

	// «Отменить» скрытия: RestoreProgram возвращает флаг как был.
	if err := s.RestoreProgram(st1); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := s.GetProgram(pathStalker); p != st1 {
		t.Errorf("RestoreProgram скрытой = %+v, want %+v", p, st1)
	}
	// Снимок (SeedPrograms) не снимает скрытие.
	if _, err := s.SeedPrograms([]Program{{Path: pathStalker, Name: "Stalker2", Category: ProgramOther, Source: SourceSnapshot}}); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := s.GetProgram(pathStalker); !p.Hidden {
		t.Errorf("снимок снял скрытие: %+v", p)
	}
}

// Ловит: «за неделю» в «Программах» (ProgramSeconds) показывает время скрытой программы (FR-20,
// RF9-3); после «Показать» время снова видно (учёт не удалён).
func TestProgramSecondsSkipsHidden(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	for _, p := range []Program{
		{Path: pathStalker, Name: "Stalker2", Category: ProgramGames, Source: SourceParent},
		{Path: pathShipping, Name: "Shipping", Category: ProgramGames, Source: SourceCatalog},
	} {
		if err := s.UpsertProgram(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{pathStalker, pathShipping} {
		if err := s.AddUsage(misha, "2026-10-30", UsageGames, SubjectProgram, path, 60); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SetProgramsHidden([]string{pathStalker}, true); err != nil {
		t.Fatal(err)
	}
	got, err := s.ProgramSeconds("2026-10-24")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{pathShipping: 60}; !reflect.DeepEqual(got, want) {
		t.Errorf("ProgramSeconds со скрытой = %v, want %v", got, want)
	}
	if _, err := s.SetProgramsHidden([]string{pathStalker}, false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ProgramSeconds("2026-10-24")
	if want := map[string]int{pathShipping: 60, pathStalker: 60}; !reflect.DeepEqual(got, want) {
		t.Errorf("ProgramSeconds после «Показать» = %v, want %v", got, want)
	}
}
