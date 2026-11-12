package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"pcblock/internal/auth"
	"pcblock/internal/core"
)

// Child — детская учётка Windows, которую родитель отметил в программе (таблица children).
type Child struct {
	ID           int64
	SID          string // идентификатор учётки Windows (windows_sid)
	Name         string // имя, которое видит родитель (display_name)
	BypassDetail string // блокировка за обход (R17): что произошло; пусто — блокировки нет
	EndMode      string // «Когда время вышло» (R8, end_mode): EndModeLock или EndModeStudyOnly
	AdultFilter  bool   // список 18+ включён (R14, adult_filter); новому ребёнку — включён
	CreatedAtMs  int64  // UTC, миллисекунды
}

// Режимы «Когда время вышло» (R8, CHECK children.end_mode). Те же ключи даёт core.EndMode.String.
const (
	EndModeLock      = "lock"       // экран блокировки (по умолчанию)
	EndModeStudyOnly = "study_only" // оставить только учёбу
)

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
	rows, err := s.db.Query(`SELECT id, windows_sid, display_name, COALESCE(bypass_detail, ''), end_mode, adult_filter <> 0, created_at
		FROM children WHERE removed_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage: список детей: %w", err)
	}
	defer rows.Close()
	out := []Child{}
	for rows.Next() {
		var c Child
		if err := rows.Scan(&c.ID, &c.SID, &c.Name, &c.BypassDetail, &c.EndMode, &c.AdultFilter, &c.CreatedAtMs); err != nil {
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

// SetEndMode записывает режим «Когда время вышло» ребёнка (R8, FR-18): EndModeLock или
// EndModeStudyOnly; другое значение — ErrInvalid (в базу не попадает), нет такого ребёнка — ErrNotFound.
func (s *Store) SetEndMode(childID int64, mode string) error {
	if mode != EndModeLock && mode != EndModeStudyOnly {
		return fmt.Errorf("%w: режим «Когда время вышло» %q, допустимо %q или %q", ErrInvalid, mode, EndModeLock, EndModeStudyOnly)
	}
	res, err := s.db.Exec(`UPDATE children SET end_mode = ? WHERE id = ?`, mode, childID)
	return affectedOne(res, err, "записать режим «Когда время вышло»")
}

// SetAdultFilter включает или выключает ребёнку список сайтов 18+ («Блокировать сайты 18+», R14,
// FR-21). Снятие и возврат отметки «детская» выбор не сбрасывают. Нет такого ребёнка — ErrNotFound.
func (s *Store) SetAdultFilter(childID int64, on bool) error {
	res, err := s.db.Exec(`UPDATE children SET adult_filter = ? WHERE id = ?`, boolToInt(on), childID)
	return affectedOne(res, err, "записать переключатель 18+")
}

// GetEndMode читает режим «Когда время вышло» ребёнка (R8). Нет такого ребёнка: EndModeLock и nil
// («пусто — норма», безопасный режим по умолчанию).
func (s *Store) GetEndMode(childID int64) (string, error) {
	var mode string
	err := s.db.QueryRow(`SELECT end_mode FROM children WHERE id = ?`, childID).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return EndModeLock, nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: прочитать режим «Когда время вышло»: %w", err)
	}
	return mode, nil
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

// SetDayLimits сохраняет лимиты семи дней недели (индекс 0 = пн, в базе weekday 1…7): общий
// (total_min, R4), игр (games_min, R5) и видео и соцсетей (video_min, R6). Лимит nil записывается
// как NULL («без лимита» / «без отдельного лимита»), 0 значит «нельзя». Три лимита дня пишутся
// одной строкой, все семь строк — в одной транзакции (PL-21); значения вне 0…1440 отклоняются до
// записи (ErrInvalid).
func (s *Store) SetDayLimits(childID int64, week [7]core.DayLimit) error {
	for i, l := range week {
		if l.TotalMin != nil && (*l.TotalMin < 0 || *l.TotalMin > maxLimitMin) {
			return fmt.Errorf("%w: лимит дня %d равен %d мин, допустимо от 0 до %d", ErrInvalid, i+1, *l.TotalMin, maxLimitMin)
		}
		if l.GamesMin != nil && (*l.GamesMin < 0 || *l.GamesMin > maxLimitMin) {
			return fmt.Errorf("%w: лимит игр дня %d равен %d мин, допустимо от 0 до %d", ErrInvalid, i+1, *l.GamesMin, maxLimitMin)
		}
		if l.VideoMin != nil && (*l.VideoMin < 0 || *l.VideoMin > maxLimitMin) {
			return fmt.Errorf("%w: лимит видео дня %d равен %d мин, допустимо от 0 до %d", ErrInvalid, i+1, *l.VideoMin, maxLimitMin)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage: сохранить лимиты: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for i, l := range week {
		_, err := tx.Exec(`INSERT INTO day_limits (child_id, weekday, total_min, games_min, video_min) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (child_id, weekday) DO UPDATE SET total_min = excluded.total_min, games_min = excluded.games_min,
			video_min = excluded.video_min`,
			childID, i+1, nullableMin(l.TotalMin), nullableMin(l.GamesMin), nullableMin(l.VideoMin))
		if err != nil {
			return fmt.Errorf("storage: сохранить лимиты: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: сохранить лимиты: %w", err)
	}
	return nil
}

// nullableMin — минуты для INTEGER-столбца: nil уходит как NULL.
func nullableMin(p *int) any {
	if p == nil {
		return nil
	}
	return int64(*p)
}

// GetDayLimits возвращает лимиты семи дней недели (индекс 0 = пн): общий, игр и видео. Дни без
// строки или с NULL — «без лимита»: у ребёнка без сохранённых лимитов ограничений по времени нет,
// игры и видео без отдельного лимита (база 0.3.3 после миграции 009 — тоже, video_min NULL).
func (s *Store) GetDayLimits(childID int64) ([7]core.DayLimit, error) {
	var week [7]core.DayLimit
	rows, err := s.db.Query(`SELECT weekday, total_min, games_min, video_min FROM day_limits WHERE child_id = ?`, childID)
	if err != nil {
		return [7]core.DayLimit{}, fmt.Errorf("storage: прочитать лимиты: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var weekday int
		var total, games, video sql.NullInt64
		if err := rows.Scan(&weekday, &total, &games, &video); err != nil {
			return [7]core.DayLimit{}, fmt.Errorf("storage: прочитать лимиты: %w", err)
		}
		if weekday < 1 || weekday > 7 {
			continue
		}
		if total.Valid {
			v := int(total.Int64)
			week[weekday-1].TotalMin = &v
		}
		if games.Valid {
			v := int(games.Int64)
			week[weekday-1].GamesMin = &v
		}
		if video.Valid {
			v := int(video.Int64)
			week[weekday-1].VideoMin = &v
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
	var pause, lastMinute sql.NullInt64
	var bonusGranted, noForbidden int
	err := s.db.QueryRow(`SELECT total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden, last_minute_end FROM day_state
		WHERE child_id = ? AND day = ?`, childID, day).Scan(&st.TotalS, &st.GamesS, &st.VideoS, &st.StudyS, &st.ExtraMin, &pause, &bonusGranted, &noForbidden, &lastMinute)
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
	if lastMinute.Valid {
		v := lastMinute.Int64
		st.LastMinuteEndMs = &v
	}
	st.BonusGranted = bonusGranted != 0
	st.NoForbidden = noForbidden != 0
	return st, nil
}

// SaveDayState записывает состояние дня ребёнка (вставка или замена по паре ребёнок и день).
// Сохраняет и pause_until (R11), и last_minute_end (R2, D-60): nil уходит как NULL. День должен быть датой YYYY-MM-DD
// (ErrInvalid). Вместе с учётом по программам — SaveDayAndUsage (одна транзакция).
func (s *Store) SaveDayState(childID int64, st core.DayState) error {
	if err := checkDay(st.Day); err != nil {
		return err
	}
	if err := saveDayState(s.db, childID, st); err != nil {
		return fmt.Errorf("storage: сохранить состояние дня: %w", err)
	}
	return nil
}

// saveDayState пишет строку day_state через ex (база или транзакция); день уже проверен.
func saveDayState(ex execer, childID int64, st core.DayState) error {
	var pause any
	if st.PauseUntilMs != nil {
		pause = *st.PauseUntilMs
	}
	var lastMinute any
	if st.LastMinuteEndMs != nil {
		lastMinute = *st.LastMinuteEndMs
	}
	_, err := ex.Exec(`INSERT INTO day_state (child_id, day, total_s, games_s, video_s, study_s, extra_min, pause_until, bonus_granted, no_forbidden, last_minute_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (child_id, day) DO UPDATE SET total_s = excluded.total_s, games_s = excluded.games_s,
			video_s = excluded.video_s, study_s = excluded.study_s, extra_min = excluded.extra_min,
			pause_until = excluded.pause_until, bonus_granted = excluded.bonus_granted, no_forbidden = excluded.no_forbidden,
			last_minute_end = excluded.last_minute_end`,
		childID, st.Day, st.TotalS, st.GamesS, st.VideoS, st.StudyS, st.ExtraMin, pause, boolToInt(st.BonusGranted), boolToInt(st.NoForbidden), lastMinute)
	return err
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

// EventFilter — отбор событий для ListEvents (FR-24, RM5-5): фильтр уходит в запрос, страница —
// по курсору (at, id) сверху вниз.
type EventFilter struct {
	ChildID    *int64   // только события этого ребёнка; nil — все
	FromMs     int64    // 0 — без нижней границы; at >= FromMs
	ToMs       int64    // 0 — без верхней; at < ToMs
	Kinds      []string // пусто — все виды
	BeforeAtMs int64    // курсор: строго раньше (at, id) = (BeforeAtMs, BeforeID); 0 — с новых
	BeforeID   int64
	Limit      int // не больше строк; 0 и меньше — defaultEventLimit, выше maxEventLimit не бывает
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
	var where []string
	var args []any
	if f.ChildID != nil {
		where = append(where, `child_id = ?`)
		args = append(args, *f.ChildID)
	}
	if f.FromMs != 0 {
		where = append(where, `at >= ?`)
		args = append(args, f.FromMs)
	}
	if f.ToMs != 0 {
		where = append(where, `at < ?`)
		args = append(args, f.ToMs)
	}
	if len(f.Kinds) > 0 {
		where = append(where, `kind IN (?`+strings.Repeat(`, ?`, len(f.Kinds)-1)+`)`)
		for _, k := range f.Kinds {
			args = append(args, k)
		}
	}
	if f.BeforeAtMs != 0 {
		where = append(where, `(at < ? OR (at = ? AND id < ?))`)
		args = append(args, f.BeforeAtMs, f.BeforeAtMs, f.BeforeID)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
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

// DeleteChildEvents удаляет события ребёнка (child_id = childID): «Очистить журнал» (FR-38, RM5-9).
// События без ребёнка (пароль, промокоды, работа программы) не трогает. Возвращает число удалённых.
func (s *Store) DeleteChildEvents(childID int64) (int, error) {
	res, err := s.db.Exec(`DELETE FROM events WHERE child_id = ?`, childID)
	if err != nil {
		return 0, fmt.Errorf("storage: очистить журнал: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("storage: очистить журнал: %w", err)
	}
	return int(n), nil
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
