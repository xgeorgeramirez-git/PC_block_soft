package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Миграции схемы лежат в migrations/NNN_имя.sql. Номер берётся из имени файла; первая строка
// файла — комментарий «-- Назначение: …». Первая миграция создаёт и таблицу schema_migrations.
// Файл миграции, попавший в main, не переписывается (L-131): правки схемы идут новой миграцией.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration — один файл миграции.
type migration struct {
	version int
	name    string
	sql     string
}

var migrationName = regexp.MustCompile(`^(\d+)_[a-z0-9_]+\.sql$`)

// loadMigrations читает все *.sql из папки dir и возвращает их по возрастанию номера.
// Файл с неверным именем, повтор номера, пустой файл и пустой набор — ошибки: молча
// пропущенная миграция оставила бы базу без части схемы.
func loadMigrations(fsys fs.FS, dir string) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("storage: прочитать миграции: %w", err)
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		m := migrationName.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("storage: имя миграции %q не вида NNN_имя.sql", name)
		}
		version, err := strconv.Atoi(m[1])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("storage: номер миграции %q должен быть целым от 1", name)
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("storage: миграции %q и %q имеют один номер %d", prev, name, version)
		}
		seen[version] = name
		body, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("storage: прочитать миграцию %q: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("storage: миграция %q пуста", name)
		}
		out = append(out, migration{version: version, name: name, sql: string(body)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("storage: в папке %q нет миграций", dir)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// applyMigrations применяет миграции, которых нет в schema_migrations, по возрастанию номера.
// Каждая миграция и запись о ней идут в одной транзакции: при сбое откатывается всё.
// nowMs — момент, который записывается в applied_at.
func applyMigrations(db *sql.DB, ms []migration, nowMs int64) error {
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		if err := applyOne(db, m, nowMs); err != nil {
			return fmt.Errorf("storage: миграция %s: %w", m.name, err)
		}
	}
	return nil
}

// appliedVersions возвращает номера уже применённых миграций; пока таблицы
// schema_migrations нет (чистая база), применённых нет.
func appliedVersions(db *sql.DB) (map[int]bool, error) {
	var tables int
	err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&tables)
	if err != nil {
		return nil, fmt.Errorf("storage: проверить schema_migrations: %w", err)
	}
	applied := map[int]bool{}
	if tables == 0 {
		return applied, nil
	}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("storage: прочитать schema_migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("storage: прочитать schema_migrations: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: прочитать schema_migrations: %w", err)
	}
	return applied, nil
}

// applyOne выполняет файл миграции и записывает её номер в одной транзакции.
func applyOne(db *sql.DB, m migration, nowMs int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(m.sql); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.version, nowMs); err != nil {
		return err
	}
	return tx.Commit()
}
