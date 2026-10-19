package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// Ловит: журнал работы растёт без предела (ТЗ 3.4: 5 файлов по 10 МБ по кругу) или в нём
// остаётся больше файлов, чем положено; самый свежий файл не тот, что без номера.
func TestRotatingLogKeepsFiveFiles(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenRotatingLog(dir, "pcblock", 100, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	for i := 0; i < 40; i++ {
		line := strings.Repeat(string(rune('a'+i%26)), 49) + "\n" // 50 байт: два на файл
		if n, err := l.Write([]byte(line)); err != nil || n != len(line) {
			t.Fatalf("Write: n=%d err=%v", n, err)
		}
	}
	names := listDir(t, dir)
	want := []string{"pcblock.log", "pcblock.log.1", "pcblock.log.2", "pcblock.log.3", "pcblock.log.4"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("файлы %v, ждали %v", names, want)
	}
	for _, n := range names {
		fi, _ := os.Stat(filepath.Join(dir, n))
		if fi.Size() > 100 {
			t.Errorf("%s: %d байт, предел 100", n, fi.Size())
		}
	}
	cur, _ := os.ReadFile(filepath.Join(dir, "pcblock.log"))
	if !strings.HasSuffix(string(cur), strings.Repeat(string(rune('a'+39%26)), 49)+"\n") {
		t.Errorf("в текущем файле нет последней записи: %q", cur)
	}
}

// Ловит: после перезапуска службы журнал начинается заново и затирает прошлые записи
// (файл надо дополнять, а размер брать с диска).
func TestRotatingLogAppendsAfterReopen(t *testing.T) {
	dir := t.TempDir()
	l, _ := OpenRotatingLog(dir, "pcblock", 1000, 5)
	l.Write([]byte("первая\n"))
	l.Close()
	l, err := OpenRotatingLog(dir, "pcblock", 1000, 5)
	if err != nil {
		t.Fatal(err)
	}
	l.Write([]byte("вторая\n"))
	l.Close()
	got, _ := os.ReadFile(filepath.Join(dir, "pcblock.log"))
	if string(got) != "первая\nвторая\n" {
		t.Fatalf("содержимое %q", got)
	}
}

// Ловит: одна запись больше предела зацикливает вращение или теряется; запись после Close
// паникует вместо ошибки.
func TestRotatingLogHugeRecordAndClose(t *testing.T) {
	dir := t.TempDir()
	l, _ := OpenRotatingLog(dir, "pcblock", 10, 3)
	big := strings.Repeat("я", 30)
	if n, err := l.Write([]byte(big)); err != nil || n != len(big) {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "pcblock.log"))
	if string(got) != big {
		t.Fatalf("запись потеряна: %q", got)
	}
	l.Close()
	if _, err := l.Write([]byte("x")); err == nil {
		t.Fatal("запись после Close должна вернуть ошибку")
	}
}

// Ловит: плохие параметры принимаются молча, папка журнала не создаётся.
func TestOpenRotatingLogValidation(t *testing.T) {
	if _, err := OpenRotatingLog(t.TempDir(), "x", 0, 5); err == nil {
		t.Error("размер 0 должен быть ошибкой")
	}
	if _, err := OpenRotatingLog(t.TempDir(), "x", 100, 0); err == nil {
		t.Error("0 файлов должен быть ошибкой")
	}
	dir := filepath.Join(t.TempDir(), "logs", "вложенная")
	l, err := OpenRotatingLog(dir, "x", 100, 2)
	if err != nil {
		t.Fatalf("папка должна создаваться: %v", err)
	}
	l.Close()
}
