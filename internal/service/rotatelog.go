package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RotatingLog — журнал работы службы (ТЗ 3.4): файл <base>.log и до files-1 предыдущих
// <base>.log.1 … по кругу, каждый не больше maxBytes. Безопасен для вызова из нескольких
// горутин. В журнал пишутся только события службы: паролей, нажатий клавиш, заголовков окон
// и адресов страниц в нём нет (R23).
type RotatingLog struct {
	dir, base string
	maxBytes  int64
	files     int

	mu     sync.Mutex
	f      *os.File
	size   int64
	closed bool
}

// OpenRotatingLog открывает журнал в папке dir (создаёт её), дописывая в существующий файл.
func OpenRotatingLog(dir, base string, maxBytes int64, files int) (*RotatingLog, error) {
	if maxBytes < 1 || files < 1 {
		return nil, errors.New("журнал: размер файла и число файлов должны быть положительными")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("журнал: создать папку: %w", err)
	}
	l := &RotatingLog{dir: dir, base: base, maxBytes: maxBytes, files: files}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *RotatingLog) path(n int) string {
	p := filepath.Join(l.dir, l.base+".log")
	if n > 0 {
		p += fmt.Sprintf(".%d", n)
	}
	return p
}

func (l *RotatingLog) open() error {
	f, err := os.OpenFile(l.path(0), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("журнал: открыть файл: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("журнал: размер файла: %w", err)
	}
	l.f, l.size = f, fi.Size()
	return nil
}

// Write дописывает запись. Если запись не помещается в текущий файл, журнал сдвигается:
// самый старый файл удаляется, остальные получают следующий номер. Запись больше предела
// целиком попадает в новый файл.
func (l *RotatingLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, errors.New("журнал закрыт")
	}
	if l.size > 0 && l.size+int64(len(p)) > l.maxBytes {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

func (l *RotatingLog) rotate() error {
	if err := l.f.Close(); err != nil {
		return fmt.Errorf("журнал: закрыть файл: %w", err)
	}
	_ = os.Remove(l.path(l.files - 1))
	for n := l.files - 2; n >= 0; n-- {
		if err := os.Rename(l.path(n), l.path(n+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("журнал: сдвинуть файл: %w", err)
		}
	}
	return l.open()
}

// Close закрывает журнал; повторный вызов безопасен.
func (l *RotatingLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	return l.f.Close()
}
