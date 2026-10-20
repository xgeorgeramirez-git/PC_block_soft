// Package storage — хранилище программы: SQLite в режиме WAL (ТЗ 3.1, 3.4, 4), драйвер на чистом Go.
//
// Схема создаётся миграциями из migrations/NNN_имя.sql (встроены в бинарник, применяются по
// таблице schema_migrations, каждая в своей транзакции). Соединение одно (MaxOpenConns(1)):
// записей параллельно не бывает, методы безопасны для вызова из нескольких горутин и
// выстраиваются в очередь. Метод не вызывает другие методы Store внутри своей транзакции:
// второе обращение ждало бы единственное соединение вечно.
//
// «Пусто — норма»: чтение отсутствующей записи даёт нулевое значение и nil, а не sql.ErrNoRows;
// списки возвращаются как []T{}, не nil (в JSON это [], а не null). Ошибки базы не подменяются:
// их можно разобрать через errors.Is/As (L-129).
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // драйвер database/sql под именем "sqlite"
)

// Ошибки, по которым вызывающий выбирает поведение.
var (
	// ErrCorrupt — файл не база SQLite или проверка целостности не прошла. Служба сохраняет
	// копию (Quarantine), не применяет ограничения и пишет событие в журнал работы (R18, NFR-8).
	ErrCorrupt = errors.New("база данных повреждена")
	// ErrNotFound — запись, которую просили изменить, не существует (например, нет такого ребёнка).
	ErrNotFound = errors.New("запись не найдена")
	// ErrInvalid — значение не подходит для записи; в базу оно не попало.
	ErrInvalid = errors.New("недопустимое значение")
)

// timeNow — часы хранилища (created_at, removed_at, applied_at, имя копии при карантине).
// Подменяются только в тестах.
var timeNow = time.Now

func nowMs() int64 { return timeNow().UnixMilli() }

// Параметры соединения (ТЗ 3.1, NFR-8): журнал WAL, ожидание занятой базы 5 с, внешние ключи.
// Драйвер выполняет их на каждом новом соединении, поэтому они не слетают при переподключении.
const dsnPragmas = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

// Store — открытая база программы.
type Store struct {
	db *sql.DB
}

// Open открывает (создаёт, если файла нет) базу по пути path, проверяет целостность и
// применяет недостающие миграции. Файл не база или PRAGMA integrity_check не вернул «ok» —
// ошибка, для которой errors.Is(err, ErrCorrupt). Прочие сбои (нет папки, нет прав, файл
// занят) ErrCorrupt не содержат: здоровую базу в карантин отправлять нельзя.
func Open(path string) (*Store, error) {
	migrations, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		return nil, fmt.Errorf("storage: открыть базу: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := checkIntegrity(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := applyMigrations(db, migrations, nowMs()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close закрывает базу; повторный вызов безопасен.
func (s *Store) Close() error { return s.db.Close() }

// checkIntegrity выполняет PRAGMA integrity_check. Первое же обращение открывает соединение,
// поэтому файл «не база» обнаруживается здесь же.
func checkIntegrity(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA integrity_check`)
	if err != nil {
		return openError(err)
	}
	defer rows.Close()
	var first string
	if rows.Next() {
		if err := rows.Scan(&first); err != nil {
			return openError(err)
		}
	}
	if err := rows.Err(); err != nil {
		return openError(err)
	}
	if first != "ok" {
		return fmt.Errorf("storage: проверка целостности: %w (ответ: %q)", ErrCorrupt, first)
	}
	return nil
}

// openError превращает ошибку драйвера при открытии в ErrCorrupt, если она значит порчу файла.
func openError(err error) error {
	if isCorruptErr(err) {
		return fmt.Errorf("storage: файл базы испорчен: %w (%w)", ErrCorrupt, err)
	}
	return fmt.Errorf("storage: открыть базу: %w", err)
}

// Основные коды результата SQLite, означающие порчу: SQLITE_CORRUPT и SQLITE_NOTADB.
const (
	sqliteCorrupt = 11
	sqliteNotADB  = 26
)

// isCorruptErr узнаёт порчу файла по коду результата SQLite, а если драйвер кода не
// даёт, по тексту сообщения.
func isCorruptErr(err error) bool {
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		switch coded.Code() & 0xff {
		case sqliteCorrupt, sqliteNotADB:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"file is not a database", "disk image is malformed", "malformed database schema"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// Quarantine копирует файл базы (и журнал -wal, если он есть) в <path>.broken-<ГГГГММДД-ЧЧММСС>
// (местное время) и возвращает путь копии; журнал копируется как <копия>-wal. Исходные файлы не меняются.
// Если копия с таким именем уже есть (два сбоя за секунду), к имени добавляется -1, -2 и так далее.
func Quarantine(path string) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("storage: карантин: %w", err)
	}
	defer src.Close()
	dstPath, dst, err := createUnique(path + ".broken-" + timeNow().Format("20060102-150405"))
	if err != nil {
		return "", fmt.Errorf("storage: карантин: %w", err)
	}
	if err := copyAndClose(dst, src); err != nil {
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("storage: карантин: %w", err)
	}
	wal := path + "-wal"
	switch _, err := os.Stat(wal); {
	case err == nil:
		if err := copyFile(wal, dstPath+"-wal"); err != nil {
			return "", fmt.Errorf("storage: карантин журнала: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("storage: карантин журнала: %w", err)
	}
	return dstPath, nil
}

// createUnique создаёт новый файл base, а если он занят, base-1, base-2 и так далее.
func createUnique(base string) (string, *os.File, error) {
	for i := 0; i < 1000; i++ {
		name := base
		if i > 0 {
			name = base + "-" + strconv.Itoa(i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return name, f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("слишком много копий с именем %s", base)
}

// copyFile копирует src в новый файл dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := copyAndClose(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// copyAndClose переносит содержимое in в out, сбрасывает на диск и закрывает out.
func copyAndClose(out *os.File, in io.Reader) error {
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
