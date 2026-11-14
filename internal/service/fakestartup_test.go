package service

import (
	"reflect"
	"testing"
)

// Ловит: фейк хранилища расходится со storage.SyncStartupItems/SetStartupDecision (тесты службы
// автозагрузки M5b тогда проверяли бы не то поведение): решение записи против решения по
// умолчанию, обновление команды без смены решения, удаление пропавших только у этого ребёнка,
// отказ без изменений, [] вместо nil, сбой по флагу.
func TestFakeStoreStartupLikeStorage(t *testing.T) {
	f := newFakeStore()
	for _, sid := range []string{"S-1", "S-2"} {
		if _, err := f.UpsertChild(sid, sid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.SyncStartupItems(99, nil, "on", 1); err == nil {
		t.Error("Sync несуществующего ребёнка без ошибки")
	}
	if got, err := f.ListStartupItems(1); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("пустой список = %#v, %v; want []", got, err)
	}
	first := []StartupRecord{
		{Scope: "own", Source: "run", Name: "A", Command: "a.exe", Decision: "off"},
		{Scope: "shared", Source: "run", Name: "B", Command: "b.exe"},
	}
	added, err := f.SyncStartupItems(1, first, "on", 100)
	if err != nil || len(added) != 2 || added[0].Decision != "off" || added[1].Decision != "on" || added[0].ID == 0 || added[0].ID == added[1].ID || added[1].FirstSeen != 100 || added[1].ChildID != 1 {
		t.Fatalf("первая проверка = %+v, %v", added, err)
	}
	if _, err := f.SyncStartupItems(2, []StartupRecord{{Scope: "shared", Source: "run", Name: "B", Command: "b.exe"}}, "on", 100); err != nil {
		t.Fatal(err)
	}
	set, err := f.SetStartupDecision(1, added[1].ID, "off", 150)
	if err != nil || set.Decision != "off" || set.DecidedAt != 150 {
		t.Fatalf("Set = %+v, %v", set, err)
	}
	if _, err := f.SetStartupDecision(2, added[1].ID, "on", 150); err == nil {
		t.Error("Set чужой строки без ошибки")
	}
	if _, err := f.SetStartupDecision(1, added[1].ID, "pending", 150); err == nil {
		t.Error("Set pending без ошибки")
	}
	second := []StartupRecord{
		{Scope: "shared", Source: "run", Name: "B", Command: "b2.exe", Publisher: "P", Decision: "on"},
		{Scope: "own", Source: "task", Name: `\C`, Command: "c.exe"},
	}
	added2, err := f.SyncStartupItems(1, second, "pending", 200)
	if err != nil || len(added2) != 1 || added2[0].Name != `\C` || added2[0].Decision != "pending" {
		t.Fatalf("вторая проверка = %+v, %v", added2, err)
	}
	got, _ := f.ListStartupItems(1)
	want := []StartupRecord{
		{ID: added[1].ID, ChildID: 1, Scope: "shared", Source: "run", Name: "B", Command: "b2.exe", Publisher: "P", Decision: "off", FirstSeen: 100, DecidedAt: 150},
		{ID: added2[0].ID, ChildID: 1, Scope: "own", Source: "task", Name: `\C`, Command: "c.exe", Decision: "pending", FirstSeen: 200},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("список = %+v\nwant %+v", got, want)
	}
	if other, _ := f.ListStartupItems(2); len(other) != 1 {
		t.Errorf("у ребёнка 2 строк %d, want 1", len(other))
	}
	for name, bad := range map[string][]StartupRecord{
		"повтор ключа":  {second[1], second[1]},
		"чужая область": {{Scope: "all", Source: "run", Name: "X"}},
		"пустое имя":    {{Scope: "own", Source: "run"}},
		"чужое решение": {{Scope: "own", Source: "run", Name: "X", Decision: "maybe"}},
	} {
		if _, err := f.SyncStartupItems(1, bad, "pending", 300); err == nil {
			t.Errorf("%s: без ошибки", name)
		}
	}
	if _, err := f.SyncStartupItems(1, second, "maybe", 300); err == nil {
		t.Error("чужое решение по умолчанию без ошибки")
	}
	if again, _ := f.ListStartupItems(1); !reflect.DeepEqual(again, want) {
		t.Errorf("после отказов = %+v, want без изменений", again)
	}
	f.failStartup = true
	if _, err := f.ListStartupItems(1); err == nil {
		t.Error("failStartup: List без ошибки")
	}
	if _, err := f.SyncStartupItems(1, second, "pending", 300); err == nil {
		t.Error("failStartup: Sync без ошибки")
	}
	if _, err := f.SetStartupDecision(1, added[1].ID, "on", 300); err == nil {
		t.Error("failStartup: Set без ошибки")
	}
}
