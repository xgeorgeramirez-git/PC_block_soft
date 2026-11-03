package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"pcblock/internal/core"
)

// Перерыв на отдых (FB-14, D-62, R27, FR-37): настройки break_rules и счёт break_state,
// миграция 008.

// GetBreakRules читает настройки «Отдых» ребёнка. Строки нет — настройки по умолчанию
// (перерывы выключены, 60 / всё, кроме учёбы / 5 / 15 / 15), без ошибки.
func (s *Store) GetBreakRules(childID int64) (core.BreakRules, error) {
	b := core.DefaultBreakRules()
	var enabled int
	var counts string
	var msg sql.NullString
	err := s.db.QueryRow(`SELECT enabled, after_min, counts, warn_min, rest_min, self_rest_min, message FROM break_rules WHERE child_id = ?`, childID).
		Scan(&enabled, &b.AfterMin, &counts, &b.WarnMin, &b.RestMin, &b.SelfRestMin, &msg)
	if errors.Is(err, sql.ErrNoRows) {
		return core.DefaultBreakRules(), nil
	}
	if err != nil {
		return core.BreakRules{}, fmt.Errorf("storage: прочитать настройки отдыха: %w", err)
	}
	b.Enabled, b.Counts, b.Message = enabled != 0, core.BreakCountsFromString(counts), msg.String
	return b, nil
}

// SetBreakRules записывает настройки «Отдых» ребёнка (вставка или замена). Пределы FR-37
// проверяются до записи (ErrInvalid); сообщение без пробелов по краям, пустое — NULL (стандартное).
func (s *Store) SetBreakRules(childID int64, b core.BreakRules) error {
	b.Message = strings.TrimSpace(b.Message)
	if err := b.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var msg any
	if b.Message != "" {
		msg = b.Message
	}
	_, err := s.db.Exec(`INSERT INTO break_rules (child_id, enabled, after_min, counts, warn_min, rest_min, self_rest_min, message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (child_id) DO UPDATE SET enabled = excluded.enabled, after_min = excluded.after_min, counts = excluded.counts,
			warn_min = excluded.warn_min, rest_min = excluded.rest_min, self_rest_min = excluded.self_rest_min, message = excluded.message`,
		childID, boolToInt(b.Enabled), b.AfterMin, b.Counts.String(), b.WarnMin, b.RestMin, b.SelfRestMin, msg)
	if err != nil {
		return fmt.Errorf("storage: записать настройки отдыха: %w", err)
	}
	return nil
}

// LoadBreakState читает счёт к перерыву ребёнка. Строки нет — пустой счёт, без ошибки.
// warned без break_at (чужая правка базы) считается «плашки не было».
func (s *Store) LoadBreakState(childID int64) (core.BreakState, error) {
	var b core.BreakState
	var quietSince, breakAt, restUntil sql.NullInt64
	var warned int
	err := s.db.QueryRow(`SELECT counted_s, quiet_s, quiet_since, warned, break_at, rest_until FROM break_state WHERE child_id = ?`, childID).
		Scan(&b.CountedS, &b.QuietS, &quietSince, &warned, &breakAt, &restUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return core.BreakState{}, nil
	}
	if err != nil {
		return core.BreakState{}, fmt.Errorf("storage: прочитать счёт к перерыву: %w", err)
	}
	b.QuietSinceMs, b.RestUntilMs = nullMs(quietSince), nullMs(restUntil)
	if warned != 0 {
		b.BreakAtMs = nullMs(breakAt)
	}
	return b, nil
}

// SaveBreakState записывает счёт к перерыву ребёнка (вставка или замена); nil уходит как NULL.
func (s *Store) SaveBreakState(childID int64, b core.BreakState) error {
	_, err := s.db.Exec(`INSERT INTO break_state (child_id, counted_s, quiet_s, quiet_since, warned, break_at, rest_until)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (child_id) DO UPDATE SET counted_s = excluded.counted_s, quiet_s = excluded.quiet_s, quiet_since = excluded.quiet_since,
			warned = excluded.warned, break_at = excluded.break_at, rest_until = excluded.rest_until`,
		childID, b.CountedS, b.QuietS, msArg(b.QuietSinceMs), boolToInt(b.Warned()), msArg(b.BreakAtMs), msArg(b.RestUntilMs))
	if err != nil {
		return fmt.Errorf("storage: записать счёт к перерыву: %w", err)
	}
	return nil
}

func nullMs(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	ms := v.Int64
	return &ms
}

func msArg(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
