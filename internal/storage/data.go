package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"pcblock/internal/auth"
	"pcblock/internal/core"
)

// Child — детская учётка Windows, которую родитель отметил в программе (таблица children).
type Child struct {
	ID           int64
	SID          string // идентификатор учётки Windows (windows_sid)
	Name         string // имя, которое видит родитель (display_name)
	BypassDetail string // блокировка за обход (R17): что произошло; пусто — блокировки нет
	CreatedAtMs  int64  // UTC, миллисекунды
}

// UpsertChild добавляет ребёнка или, если учётка с таким SID уже есть, обновляет имя и
// возвращает учётке отметку «детская» (FR-25: убранный ребёнок возвращается с прежним id и
// прежней статистикой). SID и имя обязательны. Поле ID у c игнорируется.
func (s *Store) UpsertChild(c Child) (int64, error) {
	if c.SID == "" || c.Name == "" {
		return 0, fmt.Errorf("%w: у ребёнка должны быть SID и имя", ErrInvalid)
	}
	var id int64
	err := s.db.QueryRow(`INSERT INTO children (windows_sid, display_name, created_at) VALUES (?, ?, ?)
		ON CONFLICT (windows_sid) DO UPDATE SET display_name = excluded.display_name, removed_at = NULL
		RETURNING id`, c.SID, c.Name, nowMs()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("storage: сохранить ребёнка: %w", err)
	}
	return id, nil
}

// ListChildren возвращает детей, отмеченных сейчас (removed_at пуст), по возрастанию id.
func (s *Store) ListChildren() ([]Child, error) {
	rows, err := s.db.Query(`SELECT id, windows_sid, display_name, COALESCE(bypass_detail, ''), created_at
		FROM children WHERE removed_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage: список детей: %w", err)
	}
	defer rows.Close()
	out := []Child{}
	for rows.Next() {
		var c Child
		if err := rows.Scan(&c.ID, &c.SID, &c.Name, &c.BypassDetail, &c.CreatedAtMs); err != nil {
			return nil, fmt.Errorf("storage: список детей: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: список детей: %w", err)
	}
	return out, nil
}

// RemoveChild снимает отметку «детская» (мягкое удаление, FR-25): ребёнок пропадает из
// ListChildren, а лимиты, состояние дней и события остаются. Повторное удаление момент не
// сдвигает; нет такого ребёнка — ErrNotFound.
func (s *Store) RemoveChild(id int64) error {
	res, err := s.db.Exec(`UPDATE children SET removed_at = COALESCE(removed_at, ?) WHERE id = ?`, nowMs(), id)
	return affectedOne(res, err, "снять отметку с ребёнка")
}

// SetBypass записывает блокировку за обход (R17, Ruling 18): detail говорит, что произошло.
// Пустая строка снимает блокировку (в базе NULL). Нет такого ребёнка — ErrNotFound.
func (s *Store) SetBypass(childID int64, detail string) error {
	var value any
	if detail != "" {
		value = detail
	}
	res, err := s.db.Exec(`UPDATE children SET bypass_detail = ? WHERE id = ?`, value, childID)
	return affectedOne(res, err, "записать блокировку за обход")
}

// affectedOne превращает результат UPDATE в ошибку: сбой базы или «ни одной строки» (ErrNotFound).
func affectedOne(res sql.Result, err error, what string) error {
	if err != nil {
		return fmt.Errorf("storage: %s: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: %s: %w", what, err)
	}
	if n == 0 {
		return fmt.Errorf("storage: %s: %w", what, ErrNotFound)
	}
	return nil
}

// boolToInt превращает флаг в 0/1 для хранения в INTEGER-столбце SQLite.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// maxLimitMin — наибольший лимит дня в минутах (CHECK в схеме).
const maxLimitMin = 1440

// SetDayLimits сохраняет общие лимиты семи дней недели (индекс 0 = пн, в базе weekday 1…7).
// Лимит nil записывается как NULL («без лимита»), 0 значит «нельзя». Все семь строк пишутся в
// одной транзакции; значения вне 0…1440 отклоняются до записи (ErrInvalid). Поля M2
// (games_min, video_min) не трогаются.
func (s *Store) SetDayLimits(childID int64, week [7]core.DayLimit) error {
	for i, l := range week {
		if l.TotalMin != nil && (*l.TotalMin < 0 || *l.TotalMin > maxLimitMin) {
			return fmt.Errorf("%w: лимит дня %d равен %d мин, допустимо от 0 до %d", ErrInvalid, i+1, *l.TotalMin, maxLimitMin)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage: сохранить лимиты: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for i, l := range week {
		var total any
		if l.TotalMin != nil {
			total = int64(*l.TotalMin)
		}
		_, err := tx.Exec(`INSERT INTO day_limits (child_id, weekday, total_min) VALUES (?, ?, ?)
			ON CONFLICT (child_id, weekday) DO UPDATE SET total_min = excluded.total_min`, childID, i+1, total)
		if err != nil {
			return fmt.Errorf("storage: сохранить лимиты: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: сохранить лимиты: %w", err)
	}
	return nil
}

// GetDayLimits возвращает лимиты семи дней недели (индекс 0 = пн). Дни без строки или с NULL —
// «без лимита»: у ребёнка без сохранённых лимитов ограничений по времени нет.
func (s *Store) GetDayLimits(childID int64) ([7]core.DayLimit, error) {
	var week [7]core.DayLimit
	rows, err := s.db.Query(`SELECT weekday, total_min FROM day_limits WHERE child_id = ?`, childID)
	if err != nil {
		return [7]core.DayLimit{}, fmt.Errorf("storage: прочитать лимиты: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var weekday int
		var total sql.NullInt64
		if err := rows.Scan(&weekday, &total); err != nil {
			return [7]core.DayLimit{}, fmt.Errorf("storage: прочитать лимиты: %w", err)
		}
		if weekday >= 1 && weekday <= 7 && total.Valid {
			week[weekday-1] = core.LimitOf(int(total.Int64))
		}
	}
	if err := rows.Err(); err != nil {
		return [7]core.DayLimit{}, fmt.Errorf("storage: прочитать лимиты: %w", err)
	}
	return week, nil
}

// LoadDayState читает состояние дня day (YYYY-MM-DD) ребёнка. Строки нет: пустое состояние
// с этим днём, без ошибки.
func (s *Store) LoadDayState(childID int64, day string) (core.DayState, error) {
	st := core.DayState{Day: day}
	var pause sql.NullInt64
	var bonusGranted, noForbidden int
	err := s.db.QueryRow(`SELECT total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden FROM day_state
		WHERE child_id = ? AND day = ?`, childID, day).Scan(&st.TotalS, &st.GamesS, &st.VideoS, &st.StudyS, &st.ExtraMin, &pause, &bonusGranted, &noForbidden)
	if errors.Is(err, sql.ErrNoRows) {
		return core.DayState{Day: day}, nil
	}
	if err != nil {
		return core.DayState{}, fmt.Errorf("storage: прочитать состояние дня: %w", err)
	}
	if pause.Valid {
		v := pause.Int64
		st.PauseUntilMs = &v
	}
	st.BonusGranted = bonusGranted != 0
	st.NoForbidden = noForbidden != 0
	return st, nil
}

// SaveDayState записывает состояние дня ребёнка (вставка или замена по паре ребёнок и день).
// Сохраняет и pause_until (R11): nil уходит как NULL. День должен быть датой YYYY-MM-DD
// (ErrInvalid).
func (s *Store) SaveDayState(childID int64, st core.DayState) error {
	if t, err := time.Parse(time.DateOnly, st.Day); err != nil || t.Format(time.DateOnly) != st.Day {
		return fmt.Errorf("%w: день %q не дата вида ГГГГ-ММ-ДД", ErrInvalid, st.Day)
	}
	var pause any
	if st.PauseUntilMs != nil {
		pause = *st.PauseUntilMs
	}
	_, err := s.db.Exec(`INSERT INTO day_state (child_id, day, total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (child_id, day) DO UPDATE SET total_s = excluded.total_s, games_s = excluded.games_s,
			video_s = excluded.video_s, study_s = excluded.study_s, extra_min = excluded.extra_min,
			pause_until = excluded.pause_until, bonus_granted = excluded.bonus_granted, no_forbidden = excluded.no_forbidden`,
		childID, st.Day, st.TotalS, st.GamesS, st.VideoS, st.StudyS, st.ExtraMin, pause, boolToInt(st.BonusGranted), boolToInt(st.NoForbidden))
	if err != nil {
		return fmt.Errorf("storage: сохранить состояние дня: %w", err)
	}
	return nil
}

// Event — запись журнала (R22, таблица events).
type Event struct {
	ID          int64
	ChildID     *int64         // nil: событие не про конкретного ребёнка (смена пароля и т. п.)
	AtMs        int64          // UTC, миллисекунды
	TzOffsetMin int            // смещение часового пояса на момент события, минуты
	Kind        string         // вид события (R22)
	Details     map[string]any // подробности; в базе JSON, числа при чтении приходят как float64
}

// EventFilter — отбор событий для ListEvents.
type EventFilter struct {
	ChildID *int64 // только события этого ребёнка; nil — все
	Limit   int    // не больше строк; 0 и меньше — defaultEventLimit, выше maxEventLimit не бывает
}

// Пределы числа строк журнала за один запрос.
const (
	defaultEventLimit = 200
	maxEventLimit     = 1000
)

// eventLimit приводит запрошенный предел к допустимому.
func eventLimit(n int) int {
	switch {
	case n <= 0:
		return defaultEventLimit
	case n > maxEventLimit:
		return maxEventLimit
	}
	return n
}

// AppendEvent дописывает событие в журнал. Вид обязателен; Details nil записывается как {}.
func (s *Store) AppendEvent(e Event) error {
	if e.Kind == "" {
		return fmt.Errorf("%w: у события нет вида", ErrInvalid)
	}
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("storage: подробности события %q: %w", e.Kind, err)
	}
	var child any
	if e.ChildID != nil {
		child = *e.ChildID
	}
	// Подробности передаются строкой: []byte попал бы в базу как BLOB, а не как текст.
	_, err = s.db.Exec(`INSERT INTO events (child_id, at, tz_offset_min, kind, details) VALUES (?, ?, ?, ?, ?)`,
		child, e.AtMs, e.TzOffsetMin, e.Kind, string(raw))
	if err != nil {
		return fmt.Errorf("storage: записать событие %q: %w", e.Kind, err)
	}
	return nil
}

// ListEvents возвращает события, новые сверху (при равном моменте позже записанные выше), не
// больше f.Limit. Событий нет: []Event{}.
func (s *Store) ListEvents(f EventFilter) ([]Event, error) {
	query := `SELECT id, child_id, at, tz_offset_min, kind, details FROM events`
	var args []any
	if f.ChildID != nil {
		query += ` WHERE child_id = ?`
		args = append(args, *f.ChildID)
	}
	query += ` ORDER BY at DESC, id DESC LIMIT ?`
	args = append(args, eventLimit(f.Limit))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: список событий: %w", err)
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var child sql.NullInt64
		var details string
		if err := rows.Scan(&e.ID, &child, &e.AtMs, &e.TzOffsetMin, &e.Kind, &details); err != nil {
			return nil, fmt.Errorf("storage: список событий: %w", err)
		}
		if child.Valid {
			id := child.Int64
			e.ChildID = &id
		}
		if err := json.Unmarshal([]byte(details), &e.Details); err != nil {
			return nil, fmt.Errorf("storage: подробности события %d: %w", e.ID, err)
		}
		if e.Details == nil {
			e.Details = map[string]any{}
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: список событий: %w", err)
	}
	return out, nil
}

// Auth — пароль родителя и состояние неверных вводов (таблица parent_auth, R19, NFR-9).
// Failed, Series и BlockedUntilMs — то же, что auth.Attempts.
type Auth struct {
	Hash           string // Argon2id, строка PHC (internal/auth/pwhash)
	Failed         int    // неверных вводов в текущей серии, 0…4
	Series         int    // завершённых серий по 5 неверных вводов
	BlockedUntilMs int64  // ввод закрыт до этого момента; 0 — паузы нет
	ChangedAtMs    int64  // когда сменён пароль
}

// GetAuth читает пароль родителя. Пароль не задан (мастер первого запуска не пройден):
// Auth{}, false, nil.
func (s *Store) GetAuth() (Auth, bool, error) {
	var a Auth
	var failedInRow int
	var blocked sql.NullInt64
	err := s.db.QueryRow(`SELECT hash, failed_count, blocked_until, changed_at FROM parent_auth WHERE id = 1`).
		Scan(&a.Hash, &failedInRow, &blocked, &a.ChangedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Auth{}, false, nil
	}
	if err != nil {
		return Auth{}, false, fmt.Errorf("storage: прочитать пароль: %w", err)
	}
	att := auth.AttemptsFromRow(failedInRow, blocked.Int64)
	a.Failed, a.Series, a.BlockedUntilMs = att.Failed, att.Series, att.BlockedUntilMs
	return a, true, nil
}

// SetAuth записывает пароль родителя и состояние попыток. Серия хранится в failed_count:
// неверных подряд с последнего верного пароля = Series*5 + Failed (auth.Attempts.FailedInRow).
// Хеш обязателен, Failed от 0 до 4, Series не меньше 0 (ErrInvalid). ChangedAtMs 0 значит
// «сейчас»; BlockedUntilMs 0 пишется как NULL.
func (s *Store) SetAuth(a Auth) error {
	if a.Hash == "" {
		return fmt.Errorf("%w: пустой хеш пароля", ErrInvalid)
	}
	if a.Failed < 0 || a.Failed >= auth.FailsPerSeries || a.Series < 0 {
		return fmt.Errorf("%w: попыток в серии %d, серий %d", ErrInvalid, a.Failed, a.Series)
	}
	att := auth.Attempts{Failed: a.Failed, Series: a.Series}
	var blocked any
	if a.BlockedUntilMs != 0 {
		blocked = a.BlockedUntilMs
	}
	changed := a.ChangedAtMs
	if changed == 0 {
		changed = nowMs()
	}
	_, err := s.db.Exec(`INSERT INTO parent_auth (id, hash, failed_count, blocked_until, changed_at) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET hash = excluded.hash, failed_count = excluded.failed_count,
			blocked_until = excluded.blocked_until, changed_at = excluded.changed_at`,
		a.Hash, att.FailedInRow(), blocked, changed)
	if err != nil {
		return fmt.Errorf("storage: записать пароль: %w", err)
	}
	return nil
}

// GetSetting читает настройку. Настройки нет: "", false, nil; пустое значение, записанное
// раньше, читается как "", true, nil.
func (s *Store) GetSetting(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("storage: прочитать настройку %q: %w", key, err)
	}
	return value, true, nil
}

// SetSetting записывает настройку (вставка или замена). Ключ обязателен (ErrInvalid).
func (s *Store) SetSetting(key, value string) error {
	if key == "" {
		return fmt.Errorf("%w: пустой ключ настройки", ErrInvalid)
	}
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("storage: записать настройку %q: %w", key, err)
	}
	return nil
}
