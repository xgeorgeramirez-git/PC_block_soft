package storage

import (
	"database/sql"
	"errors"
	"fmt"

	"pcblock/internal/core"
)

// Статистика вехи M5: ряд дней для столбиков и «Итогов» (RM5-4), очистка статистики (FR-38, RM5-8),
// дни без автобонуса (FB-21, D-82). Очистка журнала — DeleteChildEvents в data.go.

// DayRow — итоги одного дня ребёнка (строка day_state без флагов учёта лимита).
type DayRow struct {
	Day                            string
	TotalS, GamesS, VideoS, StudyS int
	ExtraMin                       int
}

// DaysSeries — строки day_state ребёнка с fromDay включительно до beforeDay не включительно, по
// возрастанию дня; только дни, у которых есть строка; нет строк — []DayRow{}. Сегодняшний день
// служба передаёт в beforeDay: его счётчики ведёт ядро (PL-31). Не дата — ErrInvalid.
func (s *Store) DaysSeries(childID int64, fromDay, beforeDay string) ([]DayRow, error) {
	if err := checkDay(fromDay); err != nil {
		return nil, err
	}
	if err := checkDay(beforeDay); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT day, total_s, games_s, video_s, study_s, extra_min FROM day_state
		WHERE child_id = ? AND day >= ? AND day < ? ORDER BY day`, childID, fromDay, beforeDay)
	if err != nil {
		return nil, fmt.Errorf("storage: ряд дней: %w", err)
	}
	defer rows.Close()
	out := []DayRow{}
	for rows.Next() {
		var r DayRow
		if err := rows.Scan(&r.Day, &r.TotalS, &r.GamesS, &r.VideoS, &r.StudyS, &r.ExtraMin); err != nil {
			return nil, fmt.Errorf("storage: ряд дней: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: ряд дней: %w", err)
	}
	return out, nil
}

// FirstStatsDay — самый ранний день day_state ребёнка («Статистика ведётся с», RM5-8); строк нет —
// ok = false.
func (s *Store) FirstStatsDay(childID int64) (day string, ok bool, err error) {
	var first sql.NullString
	if err := s.db.QueryRow(`SELECT min(day) FROM day_state WHERE child_id = ?`, childID).Scan(&first); err != nil {
		return "", false, fmt.Errorf("storage: первый день статистики: %w", err)
	}
	if !first.Valid {
		return "", false, nil
	}
	return first.String, true, nil
}

// StatsReset — снимок последней очистки статистики ребёнка (таблица stats_reset, RM5-8).
type StatsReset struct {
	ClearedAtMs                    int64
	Day                            string // день ядра на момент очистки (RM5-8)
	TotalS, GamesS, VideoS, StudyS int
	ExtraMin                       int
}

// GetStatsReset — снимок последней очистки; не очищали — ok = false.
func (s *Store) GetStatsReset(childID int64) (r StatsReset, ok bool, err error) {
	err = s.db.QueryRow(`SELECT cleared_at, day, total_s, games_s, video_s, study_s, extra_min FROM stats_reset
		WHERE child_id = ?`, childID).Scan(&r.ClearedAtMs, &r.Day, &r.TotalS, &r.GamesS, &r.VideoS, &r.StudyS, &r.ExtraMin)
	if errors.Is(err, sql.ErrNoRows) {
		return StatsReset{}, false, nil
	}
	if err != nil {
		return StatsReset{}, false, fmt.Errorf("storage: снимок очистки статистики: %w", err)
	}
	return r, true, nil
}

// ClearStats одной транзакцией (RM5-8): DELETE usage_daily ребёнка (все дни); DELETE day_state ребёнка
// с day < r.Day и bonus_granted = 1 (ждущие автобонуса остаются, D-82); saveDayState(day) — строка
// дня ядра текущими счётчиками и всеми флагами (day.Day == r.Day, иначе ErrInvalid); INSERT … ON CONFLICT (child_id) DO UPDATE stats_reset = r (r.ClearedAtMs —
// часы ядра, передаёт служба). Не дата — ErrInvalid; сбой — откат всего.
func (s *Store) ClearStats(childID int64, day core.DayState, r StatsReset) error {
	if err := checkDay(r.Day); err != nil {
		return err
	}
	if day.Day != r.Day {
		return fmt.Errorf("%w: день строки %q не совпадает с днём очистки %q", ErrInvalid, day.Day, r.Day)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage: очистить статистику: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM usage_daily WHERE child_id = ?`, childID); err != nil {
		return fmt.Errorf("storage: очистить статистику: %w", err)
	}
	// Дни, ждущие автобонуса (bonus_granted = 0, D-82), остаются: бонус за них начислит служба при
	// запуске с правилами; статистика после очистки их не показывает (отсекает по r.Day).
	if _, err := tx.Exec(`DELETE FROM day_state WHERE child_id = ? AND day < ? AND bonus_granted = 1`, childID, r.Day); err != nil {
		return fmt.Errorf("storage: очистить статистику: %w", err)
	}
	if err := saveDayState(tx, childID, day); err != nil {
		return fmt.Errorf("storage: очистить статистику: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO stats_reset (child_id, cleared_at, day, total_s, games_s, video_s, study_s, extra_min)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (child_id) DO UPDATE SET cleared_at = excluded.cleared_at, day = excluded.day,
			total_s = excluded.total_s, games_s = excluded.games_s, video_s = excluded.video_s,
			study_s = excluded.study_s, extra_min = excluded.extra_min`,
		childID, r.ClearedAtMs, r.Day, r.TotalS, r.GamesS, r.VideoS, r.StudyS, r.ExtraMin); err != nil {
		return fmt.Errorf("storage: очистить статистику: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: очистить статистику: %w", err)
	}
	return nil
}

// UngrantedDays — строки day_state ребёнка с bonus_granted = 0 и day < beforeDay, по возрастанию
// дня, все поля как LoadDayState (FB-21, R25: автобонус за полночь при выключенном компьютере).
// Нет — []; не дата — ErrInvalid.
func (s *Store) UngrantedDays(childID int64, beforeDay string) ([]core.DayState, error) {
	if err := checkDay(beforeDay); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT day, total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden, last_minute_end
		FROM day_state WHERE child_id = ? AND bonus_granted = 0 AND day < ? ORDER BY day`, childID, beforeDay)
	if err != nil {
		return nil, fmt.Errorf("storage: дни без автобонуса: %w", err)
	}
	defer rows.Close()
	out := []core.DayState{}
	for rows.Next() {
		var st core.DayState
		var pause, lastMinute sql.NullInt64
		var bonusGranted, noForbidden int
		if err := rows.Scan(&st.Day, &st.TotalS, &st.GamesS, &st.VideoS, &st.StudyS, &st.ExtraMin, &pause, &bonusGranted, &noForbidden, &lastMinute); err != nil {
			return nil, fmt.Errorf("storage: дни без автобонуса: %w", err)
		}
		if pause.Valid {
			v := pause.Int64
			st.PauseUntilMs = &v
		}
		if lastMinute.Valid {
			v := lastMinute.Int64
			st.LastMinuteEndMs = &v
		}
		st.BonusGranted = bonusGranted != 0
		st.NoForbidden = noForbidden != 0
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: дни без автобонуса: %w", err)
	}
	return out, nil
}
