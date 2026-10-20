package service

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// OpenResult — итог открытия базы при старте службы.
type OpenResult struct {
	Store      Store
	Recovered  bool   // база была испорчена: открыта новая пустая (безопасный режим, R18)
	BrokenCopy string // путь сохранённой копии испорченной базы
}

// OpenStore открывает базу. Файл испорчен (ErrStoreCorrupt): копия сохраняется
// (Quarantine; не вышло, например нет места, файл переименовывается), испорченные файлы
// убираются, открывается новая пустая база, служба работает без ограничений, а родителю
// остаётся предупреждение (R18, NFR-8, Ruling 25). Любая другая ошибка (файл занят, нет прав)
// возвращается как есть: здоровую базу в карантин отправлять нельзя.
func OpenStore(o StoreOpener, path string, now func() time.Time, logf func(string, ...any)) (OpenResult, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	st, err := o.Open(path)
	if err == nil {
		return OpenResult{Store: st}, nil
	}
	if !errors.Is(err, ErrStoreCorrupt) {
		return OpenResult{}, err
	}
	logf("база данных повреждена: %v", err)
	cp, qerr := o.Quarantine(path)
	if qerr != nil {
		logf("копия испорченной базы не сохранена: %v; файл будет переименован", qerr)
		cp = path + ".broken-" + now().Format("20060102-150405")
		if rerr := os.Rename(path, cp); rerr != nil {
			return OpenResult{}, fmt.Errorf("испорченную базу не убрать: %w", rerr)
		}
	} else if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		return OpenResult{}, fmt.Errorf("испорченную базу не убрать: %w", rerr)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if rerr := os.Remove(path + suffix); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return OpenResult{}, fmt.Errorf("журнал испорченной базы не убрать: %w", rerr)
		}
	}
	st, err = o.Open(path)
	if err != nil {
		return OpenResult{}, fmt.Errorf("новая база не открылась: %w", err)
	}
	logf("создана новая база, копия испорченной: %s", cp)
	return OpenResult{Store: st, Recovered: true, BrokenCopy: cp}, nil
}
