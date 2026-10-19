package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Ловит: записи о запущенных процессах теряются при записи и чтении (после падения службы
// оставшиеся страж и значок не находятся и стоят рядом с новыми, Ruling 15).
func TestProcRecordsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "procs.json")
	want := []procRecord{{Role: "guard", Session: 2, PID: 400, Stamp: 99}, {Role: "tray", Session: 2, PID: 401, Stamp: 100}}
	if err := writeProcRecords(path, want); err != nil {
		t.Fatal(err)
	}
	if got := readProcRecords(path); !reflect.DeepEqual(got, want) {
		t.Fatalf("получили %+v, ждали %+v", got, want)
	}
}

// Ловит: нет файла или он испорчен, а чтение паникует или возвращает мусор; пустой список
// оставляет файл на диске (старые PID потом убили бы чужие процессы).
func TestProcRecordsMissingBrokenEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := readProcRecords(filepath.Join(dir, "нет.json")); len(got) != 0 {
		t.Errorf("нет файла: %+v", got)
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{не json"), 0o600)
	if got := readProcRecords(bad); len(got) != 0 {
		t.Errorf("испорченный файл: %+v", got)
	}
	path := filepath.Join(dir, "procs.json")
	writeProcRecords(path, []procRecord{{Role: "tray", PID: 1}})
	if err := writeProcRecords(path, nil); err != nil {
		t.Fatal(err)
	}
	if exists(path) {
		t.Error("пустой список должен удалять файл")
	}
	if err := writeProcRecords(path, nil); err != nil {
		t.Errorf("повторное удаление несуществующего файла: %v", err)
	}
}
