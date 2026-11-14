package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// Автозагрузка учёток детей (FR-39, D-76, RM5b-4, таблица startup_items). Что найдено в Windows,
// служба передаёт целиком в SyncStartupItems; решения родителя — SetStartupDecision. Граница
// «найдено при первой проверке» — ключ settings startup.baseline.<childID> (GetSetting/SetSetting).

// Значения startup_items.scope и decision (ТЗ раздел 4).
const (
	StartupOwn     = "own"
	StartupShared  = "shared"
	StartupPending = "pending"
	StartupOn      = "on"
	StartupOff     = "off"
)

// StartupItem — строка startup_items. Ключ записи у ребёнка — Scope + Source + Name.
type StartupItem struct {
	ID, ChildID                                       int64
	Scope, Source, Name, Command, Publisher, Decision string
	FirstSeen                                         int64  // UTC мс первой проверки, нашедшей запись
	DecidedAt                                         *int64 // nil — родитель ещё не решал
}

type startupKey struct{ scope, source, name string }

func startupDecisionValid(d string) bool {
	return d == StartupPending || d == StartupOn || d == StartupOff
}

const startupCols = `id, child_id, scope, source, name, command, publisher, decision, first_seen, decided_at`

func scanStartup(row interface{ Scan(...any) error }) (StartupItem, error) {
	var it StartupItem
	var decided sql.NullInt64
	if err := row.Scan(&it.ID, &it.ChildID, &it.Scope, &it.Source, &it.Name, &it.Command, &it.Publisher, &it.Decision, &it.FirstSeen, &decided); err != nil {
		return StartupItem{}, err
	}
	if decided.Valid {
		v := decided.Int64
		it.DecidedAt = &v
	}
	return it, nil
}

// ListStartupItems — записи автозагрузки ребёнка в порядке появления (id); нет — [].
func (s *Store) ListStartupItems(childID int64) ([]StartupItem, error) {
	rows, err := s.db.Query(`SELECT `+startupCols+` FROM startup_items WHERE child_id = ? ORDER BY id`, childID)
	if err != nil {
		return nil, fmt.Errorf("storage: автозагрузка: %w", err)
	}
	defer rows.Close()
	out := []StartupItem{}
	for rows.Next() {
		it, err := scanStartup(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: автозагрузка: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: автозагрузка: %w", err)
	}
	return out, nil
}

// SyncStartupItems сводит записи ребёнка с найденными в Windows одной транзакцией (RM5b-4):
// новые по ключу Scope+Source+Name вставляются с решением записи (found[i].Decision, если оно
// задано: первая проверка — по состоянию Windows) или newDecision; у известных обновляются
// command и publisher, решение и first_seen не меняются; отсутствующие в found удаляются.
// Недопустимая область, решение, пустой источник или имя, повтор ключа в found — ErrInvalid,
// ничего не изменилось. Возвращает вставленные в порядке found; нет — [].
//
// found — полный список найденного: служба вызывает метод только после удачной проверки
// (сбой датчика ≠ «записей нет», PL-46).
func (s *Store) SyncStartupItems(childID int64, found []StartupItem, newDecision string, now int64) ([]StartupItem, error) {
	if !startupDecisionValid(newDecision) {
		return nil, fmt.Errorf("%w: решение автозагрузки %q", ErrInvalid, newDecision)
	}
	seen := make(map[startupKey]bool, len(found))
	for _, f := range found {
		if f.Scope != StartupOwn && f.Scope != StartupShared {
			return nil, fmt.Errorf("%w: область автозагрузки %q", ErrInvalid, f.Scope)
		}
		if f.Source == "" || f.Name == "" {
			return nil, fmt.Errorf("%w: запись автозагрузки без источника или имени", ErrInvalid)
		}
		if f.Decision != "" && !startupDecisionValid(f.Decision) {
			return nil, fmt.Errorf("%w: решение автозагрузки %q", ErrInvalid, f.Decision)
		}
		k := startupKey{f.Scope, f.Source, f.Name}
		if seen[k] {
			return nil, fmt.Errorf("%w: запись автозагрузки %s/%s/%q повторяется", ErrInvalid, f.Scope, f.Source, f.Name)
		}
		seen[k] = true
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stored := map[startupKey]StartupItem{}
	rows, err := tx.Query(`SELECT `+startupCols+` FROM startup_items WHERE child_id = ?`, childID)
	if err != nil {
		return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
	}
	for rows.Next() {
		it, err := scanStartup(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
		}
		stored[startupKey{it.Scope, it.Source, it.Name}] = it
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
	}
	rows.Close()

	added := []StartupItem{}
	for _, f := range found {
		k := startupKey{f.Scope, f.Source, f.Name}
		if old, ok := stored[k]; ok {
			if old.Command != f.Command || old.Publisher != f.Publisher {
				if _, err := tx.Exec(`UPDATE startup_items SET command = ?, publisher = ? WHERE id = ?`, f.Command, f.Publisher, old.ID); err != nil {
					return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
				}
			}
			continue
		}
		d := f.Decision
		if d == "" {
			d = newDecision
		}
		it := StartupItem{ChildID: childID, Scope: f.Scope, Source: f.Source, Name: f.Name, Command: f.Command, Publisher: f.Publisher, Decision: d, FirstSeen: now}
		err := tx.QueryRow(`INSERT INTO startup_items (child_id, scope, source, name, command, publisher, decision, first_seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`, childID, f.Scope, f.Source, f.Name, f.Command, f.Publisher, d, now).Scan(&it.ID)
		if err != nil {
			return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
		}
		added = append(added, it)
	}
	for k, old := range stored {
		if seen[k] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM startup_items WHERE id = ?`, old.ID); err != nil {
			return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("storage: проверка автозагрузки: %w", err)
	}
	return added, nil
}

// SetStartupDecision — решение родителя «on» или «off» по строке id ребёнка childID (decided_at =
// now); возвращает обновлённую строку. Другое решение — ErrInvalid; строки нет или она другого
// ребёнка — ErrNotFound.
func (s *Store) SetStartupDecision(childID, id int64, decision string, now int64) (StartupItem, error) {
	if decision != StartupOn && decision != StartupOff {
		return StartupItem{}, fmt.Errorf("%w: решение родителя %q, допустимо %q или %q", ErrInvalid, decision, StartupOn, StartupOff)
	}
	it, err := scanStartup(s.db.QueryRow(`UPDATE startup_items SET decision = ?, decided_at = ?
		WHERE id = ? AND child_id = ? RETURNING `+startupCols, decision, now, id, childID))
	if errors.Is(err, sql.ErrNoRows) {
		return StartupItem{}, fmt.Errorf("storage: решение автозагрузки: %w", ErrNotFound)
	}
	if err != nil {
		return StartupItem{}, fmt.Errorf("storage: решение автозагрузки: %w", err)
	}
	return it, nil
}
