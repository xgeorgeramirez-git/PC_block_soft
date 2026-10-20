package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// opener — поддельный открыватель базы: первые corruptOpens открытий дают порчу.
type opener struct {
	corruptOpens  int
	openErr       error
	quarantineErr error
	opens         int
	quarantined   []string
}

func (o *opener) Open(path string) (Store, error) {
	o.opens++
	if o.openErr != nil {
		return nil, o.openErr
	}
	if o.opens <= o.corruptOpens {
		return nil, fmt.Errorf("файл испорчен: %w", ErrStoreCorrupt)
	}
	return newFakeStore(), nil
}

func (o *opener) Quarantine(path string) (string, error) {
	if o.quarantineErr != nil {
		return "", o.quarantineErr
	}
	o.quarantined = append(o.quarantined, path)
	cp := path + ".broken-test"
	data, _ := os.ReadFile(path)
	return cp, os.WriteFile(cp, data, 0o600)
}

func fixedNow() time.Time { return time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC) }

func writeFiles(t *testing.T, path string, names ...string) {
	t.Helper()
	for _, suffix := range names {
		if err := os.WriteFile(path+suffix, []byte("мусор"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// Ловит: здоровая база уходит в карантин или открывается дважды; результат помечен как восстановленный.
func TestOpenStoreHealthy(t *testing.T) {
	o := &opener{}
	res, err := OpenStore(o, filepath.Join(t.TempDir(), "pcblock.db"), fixedNow, nil)
	if err != nil || res.Store == nil || res.Recovered || res.BrokenCopy != "" || o.opens != 1 || len(o.quarantined) != 0 {
		t.Fatalf("res=%+v err=%v opens=%d quarantined=%v", res, err, o.opens, o.quarantined)
	}
}

// Ловит (R18, NFR-8, Ruling 25): испорченная база не уходит в карантин, не заменяется новой пустой
// или оставляет старый файл (с журналом -wal) на месте, из-за чего новая база снова «испорчена».
func TestOpenStoreCorruptQuarantinesAndStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcblock.db")
	writeFiles(t, path, "", "-wal", "-shm")
	o := &opener{corruptOpens: 1}
	res, err := OpenStore(o, path, fixedNow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Recovered || res.Store == nil || res.BrokenCopy != path+".broken-test" || o.opens != 2 {
		t.Fatalf("res=%+v opens=%d", res, o.opens)
	}
	for _, s := range []string{"", "-wal", "-shm"} {
		if exists(path + s) {
			t.Errorf("остался файл %s", path+s)
		}
	}
	if !exists(res.BrokenCopy) {
		t.Error("копия испорченной базы не сохранена")
	}
}

// Ловит: при ошибке не порчи (файл занят, нет прав) здоровая база отправляется в карантин и
// теряется; ошибка должна дойти до вызывающего как есть.
func TestOpenStoreOtherErrorsAreNotQuarantined(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcblock.db")
	writeFiles(t, path, "")
	boom := errors.New("файл занят")
	o := &opener{openErr: boom}
	_, err := OpenStore(o, path, fixedNow, nil)
	if !errors.Is(err, boom) || len(o.quarantined) != 0 || !exists(path) {
		t.Fatalf("err=%v quarantined=%v exists=%v", err, o.quarantined, exists(path))
	}
}

// Ловит: не удалась копия (нет места), а испорченный файл удалён: данные потеряны безвозвратно;
// запасной путь должен переименовать файл.
func TestOpenStoreQuarantineFallbackRenames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcblock.db")
	writeFiles(t, path, "")
	o := &opener{corruptOpens: 1, quarantineErr: errors.New("нет места")}
	res, err := OpenStore(o, path, fixedNow, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := path + ".broken-20261019-150000"
	if res.BrokenCopy != want || !exists(want) || exists(path) {
		t.Fatalf("BrokenCopy=%q exists(want)=%v exists(path)=%v", res.BrokenCopy, exists(want), exists(path))
	}
}

// Ловит: если и новая база не открылась, служба считает, что всё хорошо, и работает без хранилища.
func TestOpenStoreSecondFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcblock.db")
	writeFiles(t, path, "")
	o := &opener{corruptOpens: 2}
	if res, err := OpenStore(o, path, fixedNow, nil); err == nil || res.Store != nil {
		t.Fatalf("res=%+v err=%v, ждали ошибку", res, err)
	}
}
