package storage

import (
	"errors"
	"reflect"
	"testing"
)

// Автозагрузка вехи M5b (FR-39, RM5b-4, таблица startup_items): миграция 011 и методы.

// Ловит: миграция 011 на базе 0.5.x (001–010 с данными) теряет детей, события или настройки;
// создаёт startup_items с данными, без индекса по ребёнку или без UNIQUE и CHECK из ТЗ.
func TestMigration011OnFilled010Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	all = migrationsBefore(all, 12) // свой срез истории (PL-57)
	db := rawDB(t)
	if err := applyMigrations(db, migrationsBefore(all, 11), 100); err != nil {
		t.Fatalf("миграции 001–010: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, end_mode, created_at) VALUES (1, 'S-1', 'Миша', 'lock', 10)`,
		`INSERT INTO events (child_id, at, tz_offset_min, kind, details) VALUES (1, 100, 180, 'bypass', '{}')`,
		`INSERT INTO settings (key, value) VALUES ('bonus.autobonus_from_day', '2026-11-12')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 010: %s: %v", q, err)
		}
	}
	const childQ = `SELECT id, windows_sid, display_name FROM children ORDER BY id`
	const eventQ = `SELECT id, child_id, at, kind FROM events ORDER BY id`
	const settingsQ = `SELECT key, value FROM settings ORDER BY key`
	before := [][][]string{rowStrings(t, db, childQ), rowStrings(t, db, eventQ), rowStrings(t, db, settingsQ)}
	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 011: %v", err)
	}
	after := [][][]string{rowStrings(t, db, childQ), rowStrings(t, db, eventQ), rowStrings(t, db, settingsQ)}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("данные после миграции = %v, want %v", after, before)
	}
	if n := count(t, db, "startup_items"); n != 0 {
		t.Errorf("строк startup_items %d, want 0", n)
	}
	if got := rowStrings(t, db, `SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'startup_items' AND sql IS NOT NULL`); !reflect.DeepEqual(got, [][]string{{"startup_items_by_child"}}) {
		t.Errorf("индексы startup_items = %v, want startup_items_by_child", got)
	}
	const ins = `INSERT INTO startup_items (child_id, scope, source, name, command, decision, first_seen) VALUES (?, ?, ?, ?, 'c', ?, 1)`
	if _, err := db.Exec(ins, 1, "own", "run", "OneDrive", "on"); err != nil {
		t.Fatalf("первая строка: %v", err)
	}
	for _, bad := range []struct {
		what                string
		scope, source, name string
		decision            string
		child               int64
	}{
		{"повтор ключа", "own", "run", "OneDrive", "off", 1},
		{"чужая область", "user", "run", "X", "on", 1},
		{"чужое решение", "own", "run", "Y", "maybe", 1},
		{"нет ребёнка", "own", "run", "Z", "on", 99},
	} {
		if _, err := db.Exec(ins, bad.child, bad.scope, bad.source, bad.name, bad.decision); err == nil {
			t.Errorf("%s: вставка прошла, want ошибку схемы", bad.what)
		}
	}
	if _, err := db.Exec(ins, 1, "shared", "run", "OneDrive", "pending"); err != nil {
		t.Errorf("тот же name в другой области: %v, want без ошибки", err)
	}
	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := versions(t, db); len(got) != 11 || got[10] != [2]int64{11, 200} || got[9] != [2]int64{10, 100} {
		t.Errorf("schema_migrations = %v, want 1–10 в 100 и 11 в 200", got)
	}
}

func i64(v int64) *int64 { return &v }

// Ловит: первая проверка не берёт решение записи (состояние Windows), вставляет nil вместо [],
// путает поля, ставит decided_at без решения родителя или отдаёт строки чужого ребёнка.
func TestSyncStartupItemsInsertsAndLists(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")

	empty, err := s.ListStartupItems(misha)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("пустой список = %#v, %v; want [] без ошибки", empty, err)
	}
	found := []StartupItem{
		{Scope: "own", Source: "run", Name: "OneDrive", Command: `"C:\OneDrive.exe" /background`, Publisher: "Microsoft", Decision: "on"},
		{Scope: "own", Source: "task", Name: `\Updater`, Command: `C:\up.exe`, Decision: "off"},
		{Scope: "shared", Source: "run", Name: "SecurityHealth", Command: `C:\sh.exe`}, // без решения — newDecision
	}
	added, err := s.SyncStartupItems(misha, found, "on", 1000)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	want := []StartupItem{
		{ChildID: misha, Scope: "own", Source: "run", Name: "OneDrive", Command: `"C:\OneDrive.exe" /background`, Publisher: "Microsoft", Decision: "on", FirstSeen: 1000},
		{ChildID: misha, Scope: "own", Source: "task", Name: `\Updater`, Command: `C:\up.exe`, Decision: "off", FirstSeen: 1000},
		{ChildID: misha, Scope: "shared", Source: "run", Name: "SecurityHealth", Command: `C:\sh.exe`, Decision: "on", FirstSeen: 1000},
	}
	if len(added) != 3 {
		t.Fatalf("добавлено %d, want 3: %+v", len(added), added)
	}
	for i := range added {
		if added[i].ID == 0 {
			t.Errorf("добавленная %d без id", i)
		}
		added[i].ID = 0
	}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("добавлено = %+v\nwant %+v", added, want)
	}
	list, err := s.ListStartupItems(misha)
	if err != nil {
		t.Fatal(err)
	}
	for i := range list {
		list[i].ID = 0
	}
	if !reflect.DeepEqual(list, want) {
		t.Errorf("список = %+v\nwant %+v", list, want)
	}
	other, err := s.ListStartupItems(anya)
	if err != nil || len(other) != 0 || other == nil {
		t.Errorf("список Ани = %#v, %v; want [] (строки Миши не видны)", other, err)
	}
}

// Ловит: повторная проверка теряет решение родителя, не обновляет команду и издателя изменившейся
// записи, не удаляет пропавшую (или удаляет у другого ребёнка), считает новой уже известную,
// новая не получает newDecision (pending).
func TestSyncStartupItemsUpdatesAndRemoves(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	first := []StartupItem{
		{Scope: "own", Source: "run", Name: "OneDrive", Command: "old.exe", Publisher: "Old", Decision: "on"},
		{Scope: "own", Source: "runonce", Name: "Setup", Command: "setup.exe", Decision: "on"},
		{Scope: "shared", Source: "run", Name: "Tool", Command: "tool.exe", Decision: "on"},
		{Scope: "own", Source: "task", Name: `\Pub`, Command: "pub.exe", Publisher: "Old", Decision: "off"},
	}
	if _, err := s.SyncStartupItems(misha, first, "on", 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SyncStartupItems(anya, []StartupItem{{Scope: "shared", Source: "run", Name: "Tool", Command: "tool.exe", Decision: "on"}}, "on", 1000); err != nil {
		t.Fatal(err)
	}
	var oneDriveID, pubID int64
	list, _ := s.ListStartupItems(misha)
	for _, it := range list {
		switch it.Name {
		case "OneDrive":
			oneDriveID = it.ID
		case `\Pub`:
			pubID = it.ID
		}
	}
	if _, err := s.SetStartupDecision(misha, oneDriveID, "off", 1500); err != nil {
		t.Fatal(err)
	}

	second := []StartupItem{
		{Scope: "own", Source: "run", Name: "OneDrive", Command: "new.exe", Publisher: "Microsoft", Decision: "on"}, // решение Windows не важно: строка есть
		{Scope: "own", Source: "startup_folder", Name: "Notes.lnk", Command: "notes.exe"},
		{Scope: "own", Source: "task", Name: `\Pub`, Command: "pub.exe", Publisher: "New"}, // сменился только издатель
		// Setup (RunOnce) и Tool пропали
	}
	added, err := s.SyncStartupItems(misha, second, "pending", 2000)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(added) != 1 || added[0].Name != "Notes.lnk" || added[0].Decision != "pending" || added[0].FirstSeen != 2000 || added[0].DecidedAt != nil {
		t.Errorf("добавлено = %+v, want одна Notes.lnk pending first_seen 2000", added)
	}
	list, err = s.ListStartupItems(misha)
	if err != nil {
		t.Fatal(err)
	}
	want := []StartupItem{
		{ID: oneDriveID, ChildID: misha, Scope: "own", Source: "run", Name: "OneDrive", Command: "new.exe", Publisher: "Microsoft", Decision: "off", FirstSeen: 1000, DecidedAt: i64(1500)},
		{ID: pubID, ChildID: misha, Scope: "own", Source: "task", Name: `\Pub`, Command: "pub.exe", Publisher: "New", Decision: "off", FirstSeen: 1000},
		{ID: added[0].ID, ChildID: misha, Scope: "own", Source: "startup_folder", Name: "Notes.lnk", Command: "notes.exe", Decision: "pending", FirstSeen: 2000},
	}
	if !reflect.DeepEqual(list, want) {
		t.Errorf("список = %+v\nwant %+v", list, want)
	}
	if got, _ := s.ListStartupItems(anya); len(got) != 1 || got[0].Name != "Tool" {
		t.Errorf("у Ани = %+v, want Tool остался (удаляется только у Миши)", got)
	}

	// Пустой найденный список удаляет всё у этого ребёнка, added — [].
	added, err = s.SyncStartupItems(misha, []StartupItem{}, "pending", 3000)
	if err != nil || added == nil || len(added) != 0 {
		t.Errorf("пустая проверка: added=%#v err=%v; want []", added, err)
	}
	if got, _ := s.ListStartupItems(misha); len(got) != 0 {
		t.Errorf("после пустой проверки строк %d, want 0", len(got))
	}
}

// Ловит: недопустимые значения частично записываются (нет одной транзакции) или проходят мимо
// ErrInvalid; повтор ключа внутри найденного роняет базу посередине.
func TestSyncStartupItemsAllOrNothing(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	if _, err := s.SyncStartupItems(misha, []StartupItem{{Scope: "own", Source: "run", Name: "Keep", Command: "k.exe", Decision: "on"}}, "on", 1000); err != nil {
		t.Fatal(err)
	}
	good := StartupItem{Scope: "own", Source: "run", Name: "New", Command: "n.exe"}
	for name, tc := range map[string]struct {
		found    []StartupItem
		decision string
	}{
		"чужое решение по умолчанию":  {[]StartupItem{good}, "maybe"},
		"пустое решение по умолчанию": {[]StartupItem{good}, ""},
		"чужое решение записи":        {[]StartupItem{good, {Scope: "own", Source: "run", Name: "X", Command: "x", Decision: "yes"}}, "on"},
		"чужая область":               {[]StartupItem{good, {Scope: "all", Source: "run", Name: "X", Command: "x"}}, "on"},
		"пустой источник":             {[]StartupItem{good, {Scope: "own", Name: "X", Command: "x"}}, "on"},
		"пустое имя":                  {[]StartupItem{good, {Scope: "own", Source: "run", Command: "x"}}, "on"},
		"повтор ключа":                {[]StartupItem{good, good}, "on"},
	} {
		if _, err := s.SyncStartupItems(misha, tc.found, tc.decision, 2000); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err=%v, want ErrInvalid", name, err)
		}
		list, err := s.ListStartupItems(misha)
		if err != nil || len(list) != 1 || list[0].Name != "Keep" {
			t.Errorf("%s: после отказа список = %+v, %v; want только Keep", name, list, err)
		}
	}
	if _, err := s.SyncStartupItems(999, []StartupItem{good}, "on", 2000); err == nil {
		t.Errorf("Sync несуществующего ребёнка без ошибки")
	}
}

// Ловит: решение пишется в строку чужого ребёнка, принимает pending или мусор, не ставит
// decided_at, не возвращает обновлённую строку.
func TestSetStartupDecision(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	added, err := s.SyncStartupItems(misha, []StartupItem{{Scope: "shared", Source: "task", Name: `\Upd`, Command: "u.exe", Publisher: "P"}}, "pending", 1000)
	if err != nil {
		t.Fatal(err)
	}
	id := added[0].ID
	got, err := s.SetStartupDecision(misha, id, "on", 1700)
	want := StartupItem{ID: id, ChildID: misha, Scope: "shared", Source: "task", Name: `\Upd`, Command: "u.exe", Publisher: "P", Decision: "on", FirstSeen: 1000, DecidedAt: i64(1700)}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Set = %+v, %v\nwant %+v", got, err, want)
	}
	if _, err := s.SetStartupDecision(anya, id, "off", 1800); !errors.Is(err, ErrNotFound) {
		t.Errorf("Set чужой строки: %v, want ErrNotFound", err)
	}
	if _, err := s.SetStartupDecision(misha, id+100, "off", 1800); !errors.Is(err, ErrNotFound) {
		t.Errorf("Set несуществующей строки: %v, want ErrNotFound", err)
	}
	for _, d := range []string{"pending", "", "maybe"} {
		if _, err := s.SetStartupDecision(misha, id, d, 1800); !errors.Is(err, ErrInvalid) {
			t.Errorf("Set %q: %v, want ErrInvalid", d, err)
		}
	}
	list, _ := s.ListStartupItems(misha)
	if len(list) != 1 || !reflect.DeepEqual(list[0], want) {
		t.Errorf("после отказов строка = %+v, want %+v (не изменилась)", list, want)
	}
}

// Ловит: методы автозагрузки на закрытом хранилище возвращают nil (служба приняла бы сбой за
// «записей нет» и удалила бы решения, PL-46).
func TestStartupMethodsFailWhenClosed(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1", "Миша")
	_ = s.Close()
	if _, err := s.ListStartupItems(id); err == nil {
		t.Error("ListStartupItems на закрытом хранилище без ошибки")
	}
	if _, err := s.SyncStartupItems(id, []StartupItem{}, "on", 1); err == nil {
		t.Error("SyncStartupItems на закрытом хранилище без ошибки")
	}
	if _, err := s.SetStartupDecision(id, 1, "on", 1); err == nil {
		t.Error("SetStartupDecision на закрытом хранилище без ошибки")
	}
}
