package storage

import (
	"database/sql"
	"errors"
	"fmt"

	"pcblock/internal/core"
)

// maxBonusBalance — наибольший баланс копилки в минутах (CHECK в схеме, FR-22).
const maxBonusBalance = 600

// GetSchedule возвращает запретные интервалы ребёнка (R7), отсортированные по дню недели
// и началу. Интервалов нет — пустой список, не nil (L-22).
func (s *Store) GetSchedule(childID int64) ([]core.Interval, error) {
	out := []core.Interval{}
	rows, err := s.db.Query(`SELECT weekday, start_min, end_min FROM schedule_intervals
		WHERE child_id = ? ORDER BY weekday, start_min`, childID)
	if err != nil {
		return nil, fmt.Errorf("storage: прочитать расписание: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var wd, start, end int
		if err := rows.Scan(&wd, &start, &end); err != nil {
			return nil, fmt.Errorf("storage: прочитать расписание: %w", err)
		}
		out = append(out, core.Interval{Weekday: core.Weekday(wd), StartMin: start, EndMin: end})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: прочитать расписание: %w", err)
	}
	return out, nil
}

// SetSchedule заменяет все запретные интервалы ребёнка переданными (R7): удаляет прежние
// и вставляет новые в одной транзакции. На вход — нормализованные интервалы (служба
// нормализует до записи); значения проверяются CHECK схемы. Пустой список очищает.
func (s *Store) SetSchedule(childID int64, intervals []core.Interval) error {
	for i, iv := range intervals {
		if iv.Weekday < core.Monday || iv.Weekday > core.Sunday {
			return fmt.Errorf("%w: интервал %d: день недели %d вне 1..7", ErrInvalid, i, iv.Weekday)
		}
		if iv.StartMin < 0 || iv.StartMin > 1439 || iv.EndMin < 0 || iv.EndMin > 1439 {
			return fmt.Errorf("%w: интервал %d: минуты вне 0..1439", ErrInvalid, i)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage: сохранить расписание: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM schedule_intervals WHERE child_id = ?`, childID); err != nil {
		return fmt.Errorf("storage: сохранить расписание: %w", err)
	}
	for _, iv := range intervals {
		if _, err := tx.Exec(`INSERT INTO schedule_intervals (child_id, weekday, start_min, end_min)
			VALUES (?, ?, ?, ?)`, childID, int(iv.Weekday), iv.StartMin, iv.EndMin); err != nil {
			return fmt.Errorf("storage: сохранить расписание: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: сохранить расписание: %w", err)
	}
	return nil
}

// BonusEntry — запись истории копилки (FR-22).
type BonusEntry struct {
	ID           int64
	AtMs         int64
	DeltaMin     int
	Reason       string
	BalanceAfter int
}

// GetBonusBalance возвращает текущий баланс копилки ребёнка (последний balance_after) или 0.
func (s *Store) GetBonusBalance(childID int64) (int, error) {
	var bal int
	err := s.db.QueryRow(`SELECT balance_after FROM bonus_ledger WHERE child_id = ?
		ORDER BY id DESC LIMIT 1`, childID).Scan(&bal)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("storage: прочитать баланс копилки: %w", err)
	}
	return bal, nil
}

// AppendBonus записывает начисление (deltaMin > 0) или трату (deltaMin < 0) и возвращает
// новый баланс (R10, FR-22). Баланс считается в той же транзакции, что и вставка (L-80):
// трата сверх баланса и начисление выше 600 отклоняются, строка не пишется. reason
// обязателен (R22).
func (s *Store) AppendBonus(childID int64, deltaMin int, reason string) (int, error) {
	if reason == "" {
		return 0, fmt.Errorf("%w: у начисления копилки нет причины", ErrInvalid)
	}
	if deltaMin == 0 {
		return 0, fmt.Errorf("%w: изменение копилки не может быть нулевым", ErrInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("storage: копилка: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var cur int
	err = tx.QueryRow(`SELECT balance_after FROM bonus_ledger WHERE child_id = ?
		ORDER BY id DESC LIMIT 1`, childID).Scan(&cur)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("storage: копилка: %w", err)
	}
	after := cur + deltaMin
	if after < 0 {
		return 0, fmt.Errorf("%w: в копилке %d мин, нельзя взять %d", ErrInvalid, cur, -deltaMin)
	}
	if after > maxBonusBalance {
		return 0, fmt.Errorf("%w: можно начислить не больше %d мин", ErrInvalid, maxBonusBalance-cur)
	}
	if _, err := tx.Exec(`INSERT INTO bonus_ledger (child_id, at, delta_min, reason, balance_after)
		VALUES (?, ?, ?, ?, ?)`, childID, timeNow().UnixMilli(), deltaMin, reason, after); err != nil {
		return 0, fmt.Errorf("storage: копилка: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("storage: копилка: %w", err)
	}
	return after, nil
}

// ListBonus возвращает историю копилки ребёнка, новые записи первыми (FR-22). Пусто —
// пустой список, не nil.
func (s *Store) ListBonus(childID int64) ([]BonusEntry, error) {
	out := []BonusEntry{}
	rows, err := s.db.Query(`SELECT id, at, delta_min, reason, balance_after FROM bonus_ledger
		WHERE child_id = ? ORDER BY id DESC`, childID)
	if err != nil {
		return nil, fmt.Errorf("storage: история копилки: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e BonusEntry
		if err := rows.Scan(&e.ID, &e.AtMs, &e.DeltaMin, &e.Reason, &e.BalanceAfter); err != nil {
			return nil, fmt.Errorf("storage: история копилки: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: история копилки: %w", err)
	}
	return out, nil
}
