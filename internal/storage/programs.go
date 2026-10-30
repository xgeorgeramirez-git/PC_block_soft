package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"pcblock/internal/core"
)

// Программы, «Ждут решения» и учёт по программам (веха M3: R5, R12, RM3-1, RM3-6, RM3-12, FR-20).
// Хранилище только хранит: путь exe нормализует (нижний регистр, длинное имя), отпечаток
// «размер:время изменения» и корень приложения считает служба (internal/programs).

// Категории программ (CHECK programs.category). «Системное» служба не закрывает и не считает.
const (
	ProgramGames   = "games"
	ProgramStudy   = "study"
	ProgramOther   = "other"
	ProgramBlocked = "blocked"
	ProgramSystem  = "system"
)

// Откуда взялась категория программы (CHECK programs.source).
const (
	SourceCatalog  = "catalog"  // встроенный каталог игр и браузеров
	SourceSnapshot = "snapshot" // снимок Program Files при установке или первом старте (RM3-10)
	SourceParent   = "parent"   // решение родителя (FR-20)
)

// Категории и виды учёта usage_daily (CHECK): «Системного» здесь нет, его секунды не пишутся.
const (
	UsageGames       = "games"
	UsageVideoSocial = "video_social"
	UsageStudy       = "study"
	UsageOther       = "other"

	SubjectProgram = "program" // subject — путь exe в нижнем регистре
	SubjectSite    = "site"    // subject — домен (M4)
)

var (
	programCategories = map[string]bool{ProgramGames: true, ProgramStudy: true, ProgramOther: true, ProgramBlocked: true, ProgramSystem: true}
	programSources    = map[string]bool{SourceCatalog: true, SourceSnapshot: true, SourceParent: true}
	usageCategories   = map[string]bool{UsageGames: true, UsageVideoSocial: true, UsageStudy: true, UsageOther: true}
	usageKinds        = map[string]bool{SubjectProgram: true, SubjectSite: true}
)

// execer — то, что умеет выполнить запрос: *sql.DB или *sql.Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// --- программы ----------------------------------------------------------------------------

// Program — программа из списка (таблица programs): одна категория для всех детей (R12).
type Program struct {
	ID          int64
	Path        string // exe_path: нормализованный полный путь в нижнем регистре
	Name        string // display_name: описание из файла или имя файла
	Publisher   string // CompanyName из свойств файла (RM3-3); пусто — не известен
	Category    string // Program*
	Source      string // Source*
	Fingerprint string // «размер:время изменения» для решения родителя вне доверенных каталогов (RM3-12); пусто — не нужен
	AppRoot     string // корень приложения «<доверенный корень>\<первая папка>» (RM3-1 п.3); пусто — нет
	DecidedAt   int64  // момент решения родителя, UTC мс; 0 — нет (NULL)
}

func checkProgram(p Program) error {
	switch {
	case p.Path == "":
		return fmt.Errorf("%w: у программы нет пути", ErrInvalid)
	case p.Name == "":
		return fmt.Errorf("%w: у программы %q нет имени", ErrInvalid, p.Path)
	case !programCategories[p.Category]:
		return fmt.Errorf("%w: категория программы %q", ErrInvalid, p.Category)
	case !programSources[p.Source]:
		return fmt.Errorf("%w: источник категории программы %q", ErrInvalid, p.Source)
	case p.DecidedAt < 0:
		return fmt.Errorf("%w: момент решения %d", ErrInvalid, p.DecidedAt)
	}
	return nil
}

func nullableMs(ms int64) any {
	if ms == 0 {
		return nil
	}
	return ms
}

const programColumns = `id, exe_path, display_name, publisher, category, source, fingerprint, app_root, decided_at`

// scanProgram читает строку programColumns.
func scanProgram(sc interface{ Scan(...any) error }) (Program, error) {
	var p Program
	var decided sql.NullInt64
	if err := sc.Scan(&p.ID, &p.Path, &p.Name, &p.Publisher, &p.Category, &p.Source, &p.Fingerprint, &p.AppRoot, &decided); err != nil {
		return Program{}, err
	}
	p.DecidedAt = decided.Int64
	return p, nil
}

// upsertProgram записывает программу целиком (вставка или замена всех полей по пути).
func upsertProgram(ex execer, p Program) error {
	_, err := ex.Exec(`INSERT INTO programs (exe_path, display_name, publisher, category, source, fingerprint, app_root, decided_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (exe_path) DO UPDATE SET display_name = excluded.display_name, publisher = excluded.publisher,
			category = excluded.category, source = excluded.source, fingerprint = excluded.fingerprint,
			app_root = excluded.app_root, decided_at = excluded.decided_at`,
		p.Path, p.Name, p.Publisher, p.Category, p.Source, p.Fingerprint, p.AppRoot, nullableMs(p.DecidedAt))
	return err
}

// UpsertProgram записывает программу по пути (вставка или замена всех полей, кроме ID).
// Путь и имя обязательны, категория и источник — из списков схемы (ErrInvalid).
func (s *Store) UpsertProgram(p Program) error {
	if err := checkProgram(p); err != nil {
		return err
	}
	if err := upsertProgram(s.db, p); err != nil {
		return fmt.Errorf("storage: записать программу: %w", err)
	}
	return nil
}

// RestoreProgram возвращает программу, как она была до решения родителя («Отменить» 10 с,
// FR-20): все поля по пути, строка с тем же путём сохраняет свой номер.
func (s *Store) RestoreProgram(p Program) error { return s.UpsertProgram(p) }

// SeedPrograms добавляет программы, которых в списке ещё нет (снимок и каталог, RM3-10); уже
// известные (в том числе решения родителя) не меняются. Весь набор проверяется до записи и пишется
// одной транзакцией. Возвращает, сколько добавлено.
func (s *Store) SeedPrograms(ps []Program) (int, error) {
	for _, p := range ps {
		if err := checkProgram(p); err != nil {
			return 0, err
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("storage: снимок программ: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	added := 0
	for _, p := range ps {
		res, err := tx.Exec(`INSERT INTO programs (exe_path, display_name, publisher, category, source, fingerprint, app_root, decided_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (exe_path) DO NOTHING`,
			p.Path, p.Name, p.Publisher, p.Category, p.Source, p.Fingerprint, p.AppRoot, nullableMs(p.DecidedAt))
		if err != nil {
			return 0, fmt.Errorf("storage: снимок программ: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("storage: снимок программ: %w", err)
		}
		added += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("storage: снимок программ: %w", err)
	}
	return added, nil
}

// GetProgram читает программу по пути. Нет такой: Program{}, false, nil.
func (s *Store) GetProgram(path string) (Program, bool, error) {
	p, err := scanProgram(s.db.QueryRow(`SELECT `+programColumns+` FROM programs WHERE exe_path = ?`, path))
	if errors.Is(err, sql.ErrNoRows) {
		return Program{}, false, nil
	}
	if err != nil {
		return Program{}, false, fmt.Errorf("storage: прочитать программу: %w", err)
	}
	return p, true, nil
}

// ListPrograms возвращает все программы по имени (при равных именах — по пути). Нет программ: []Program{}.
func (s *Store) ListPrograms() ([]Program, error) {
	rows, err := s.db.Query(`SELECT ` + programColumns + ` FROM programs ORDER BY display_name, exe_path`)
	if err != nil {
		return nil, fmt.Errorf("storage: список программ: %w", err)
	}
	defer rows.Close()
	out := []Program{}
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: список программ: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: список программ: %w", err)
	}
	return out, nil
}

// SetProgramsCategory — решение родителя по списку программ (FR-20, массовая смена): категория
// category, источник «родитель», момент at (UTC мс). Пути, которых нет в списке, и программы, уже
// отнесённые родителем к этой категории, не трогаются. Одной транзакцией; возвращает прежние
// значения изменённых программ (для «Отменить»), без изменений — []Program{}.
func (s *Store) SetProgramsCategory(paths []string, category string, at int64) ([]Program, error) {
	if !programCategories[category] {
		return nil, fmt.Errorf("%w: категория программы %q", ErrInvalid, category)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("storage: сменить категорию программ: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	changed := []Program{}
	for _, path := range paths {
		p, err := scanProgram(tx.QueryRow(`SELECT `+programColumns+` FROM programs WHERE exe_path = ?`, path))
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("storage: сменить категорию программ: %w", err)
		}
		if p.Category == category && p.Source == SourceParent {
			continue
		}
		if _, err := tx.Exec(`UPDATE programs SET category = ?, source = ?, decided_at = ? WHERE id = ?`,
			category, SourceParent, nullableMs(at), p.ID); err != nil {
			return nil, fmt.Errorf("storage: сменить категорию программ: %w", err)
		}
		changed = append(changed, p)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("storage: сменить категорию программ: %w", err)
	}
	return changed, nil
}

// DeleteProgram удаляет программу из списка («Отменить» решения по ждущей программе). Нет такой — ErrNotFound.
func (s *Store) DeleteProgram(path string) error {
	res, err := s.db.Exec(`DELETE FROM programs WHERE exe_path = ?`, path)
	return affectedOne(res, err, "удалить программу")
}

// --- ждут решения -------------------------------------------------------------------------

// PendingChild — кто из детей запускал ждущую решения программу (таблица pending_attempts).
type PendingChild struct {
	ChildID       int64
	Attempts      int
	LastAttemptAt int64 // UTC мс
}

// PendingProgram — программа в «Ждут решения» (R12, FR-20).
type PendingProgram struct {
	Path           string
	Name           string
	Publisher      string
	Description    string
	WasCategory    string // прежняя категория при смене отпечатка («раньше: Игра», RM3-12); пусто — нет
	FirstChildID   int64  // кто запустил первым; 0 — не известно
	FirstAttemptAt int64  // UTC мс
	LastAttemptAt  int64  // UTC мс
	Attempts       int    // всего попыток всех детей
	Children       []PendingChild
}

// AddPendingAttempt записывает попытку запуска незнакомой программы ребёнком childID в момент
// at (UTC мс): создаёт строку «Ждут решения» или увеличивает её счётчик и время последней попытки,
// ведёт попытки этого ребёнка. Имя, издатель, описание и прежняя категория обновляются, если
// пришли непустыми (файл могли заменить). childID 0 — ребёнок не известен. Путь и имя обязательны
// (ErrInvalid). Возвращает число попыток программы после записи.
func (s *Store) AddPendingAttempt(p PendingProgram, childID, at int64) (int, error) {
	if p.Path == "" || p.Name == "" {
		return 0, fmt.Errorf("%w: у ждущей программы должны быть путь и имя", ErrInvalid)
	}
	var child any
	if childID != 0 {
		child = childID
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("storage: записать попытку запуска: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	err = tx.QueryRow(`INSERT INTO pending_programs (exe_path, display_name, publisher, description, was_category,
			first_child_id, first_attempt_at, last_attempt_at, attempts)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT (exe_path) DO UPDATE SET attempts = attempts + 1,
			last_attempt_at = max(last_attempt_at, excluded.last_attempt_at),
			display_name = excluded.display_name,
			publisher = CASE WHEN excluded.publisher <> '' THEN excluded.publisher ELSE publisher END,
			description = CASE WHEN excluded.description <> '' THEN excluded.description ELSE description END,
			was_category = CASE WHEN excluded.was_category <> '' THEN excluded.was_category ELSE was_category END
		RETURNING attempts`,
		p.Path, p.Name, p.Publisher, p.Description, p.WasCategory, child, at, at).Scan(&attempts)
	if err != nil {
		return 0, fmt.Errorf("storage: записать попытку запуска: %w", err)
	}
	if childID != 0 {
		if _, err := tx.Exec(`INSERT INTO pending_attempts (exe_path, child_id, attempts, last_attempt_at) VALUES (?, ?, 1, ?)
			ON CONFLICT (exe_path, child_id) DO UPDATE SET attempts = attempts + 1,
				last_attempt_at = max(last_attempt_at, excluded.last_attempt_at)`, p.Path, childID, at); err != nil {
			return 0, fmt.Errorf("storage: записать попытку запуска: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("storage: записать попытку запуска: %w", err)
	}
	return attempts, nil
}

// querier — то, что умеет выполнить запрос со строками: *sql.DB или *sql.Tx.
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// listPending читает ждущие программы (все или одну по пути, если path непуст) с попытками детей:
// новые сверху по последней попытке, дети — тоже по последней попытке. Строки читаются до
// следующего запроса: соединение одно (store.go).
func listPending(q querier, path string) ([]PendingProgram, error) {
	query := `SELECT exe_path, display_name, publisher, description, was_category, COALESCE(first_child_id, 0),
		first_attempt_at, last_attempt_at, attempts FROM pending_programs`
	var args []any
	if path != "" {
		query += ` WHERE exe_path = ?`
		args = append(args, path)
	}
	rows, err := q.Query(query+` ORDER BY last_attempt_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	out := []PendingProgram{}
	index := map[string]int{}
	for rows.Next() {
		p := PendingProgram{Children: []PendingChild{}}
		if err := rows.Scan(&p.Path, &p.Name, &p.Publisher, &p.Description, &p.WasCategory, &p.FirstChildID,
			&p.FirstAttemptAt, &p.LastAttemptAt, &p.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		index[p.Path] = len(out)
		out = append(out, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	query = `SELECT exe_path, child_id, attempts, last_attempt_at FROM pending_attempts`
	if path != "" {
		query += ` WHERE exe_path = ?`
	}
	rows, err = q.Query(query+` ORDER BY last_attempt_at DESC, child_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var exe string
		var c PendingChild
		if err := rows.Scan(&exe, &c.ChildID, &c.Attempts, &c.LastAttemptAt); err != nil {
			return nil, err
		}
		if i, ok := index[exe]; ok {
			out[i].Children = append(out[i].Children, c)
		}
	}
	return out, rows.Err()
}

// ListPending возвращает «Ждут решения» с попытками по детям, новые сверху. Нет: []PendingProgram{}.
func (s *Store) ListPending() ([]PendingProgram, error) {
	out, err := listPending(s.db, "")
	if err != nil {
		return nil, fmt.Errorf("storage: список «Ждут решения»: %w", err)
	}
	return out, nil
}

// ResolvePending убирает программу из «Ждут решения» (решение родителя) вместе с попытками детей
// (каскад) и возвращает её, как была, для «Отменить». Нет такой: PendingProgram{}, false, nil.
func (s *Store) ResolvePending(path string) (PendingProgram, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return PendingProgram{}, false, fmt.Errorf("storage: убрать из «Ждут решения»: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	list, err := listPending(tx, path)
	if err != nil {
		return PendingProgram{}, false, fmt.Errorf("storage: убрать из «Ждут решения»: %w", err)
	}
	if len(list) == 0 {
		return PendingProgram{}, false, nil
	}
	if _, err := tx.Exec(`DELETE FROM pending_programs WHERE exe_path = ?`, path); err != nil {
		return PendingProgram{}, false, fmt.Errorf("storage: убрать из «Ждут решения»: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PendingProgram{}, false, fmt.Errorf("storage: убрать из «Ждут решения»: %w", err)
	}
	return list[0], true, nil
}

// RestorePending возвращает программу в «Ждут решения» («Отменить» решения, FR-20). Если ребёнок
// успел снова её запустить, попытки складываются: счётчики суммируются, первая попытка — более
// ранняя, последняя — более поздняя. Одной транзакцией; путь и имя обязательны (ErrInvalid).
func (s *Store) RestorePending(p PendingProgram) error {
	if p.Path == "" || p.Name == "" {
		return fmt.Errorf("%w: у ждущей программы должны быть путь и имя", ErrInvalid)
	}
	var first any
	if p.FirstChildID != 0 {
		first = p.FirstChildID
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage: вернуть в «Ждут решения»: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO pending_programs (exe_path, display_name, publisher, description, was_category,
			first_child_id, first_attempt_at, last_attempt_at, attempts)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (exe_path) DO UPDATE SET attempts = attempts + excluded.attempts,
			first_child_id = CASE WHEN excluded.first_attempt_at < first_attempt_at THEN excluded.first_child_id ELSE first_child_id END,
			first_attempt_at = min(first_attempt_at, excluded.first_attempt_at),
			last_attempt_at = max(last_attempt_at, excluded.last_attempt_at),
			was_category = CASE WHEN was_category = '' THEN excluded.was_category ELSE was_category END`,
		p.Path, p.Name, p.Publisher, p.Description, p.WasCategory, first, p.FirstAttemptAt, p.LastAttemptAt, p.Attempts); err != nil {
		return fmt.Errorf("storage: вернуть в «Ждут решения»: %w", err)
	}
	for _, c := range p.Children {
		if _, err := tx.Exec(`INSERT INTO pending_attempts (exe_path, child_id, attempts, last_attempt_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (exe_path, child_id) DO UPDATE SET attempts = attempts + excluded.attempts,
				last_attempt_at = max(last_attempt_at, excluded.last_attempt_at)`, p.Path, c.ChildID, c.Attempts, c.LastAttemptAt); err != nil {
			return fmt.Errorf("storage: вернуть в «Ждут решения»: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: вернуть в «Ждут решения»: %w", err)
	}
	return nil
}

// --- учёт по программам ---------------------------------------------------------------------

// UsageRow — порция учёта usage_daily (RM3-6): секунды категории по программе или сайту за день.
// Day пусто — день состояния, с которым порция пишется (SaveDayAndUsage).
type UsageRow struct {
	Day      string
	Category string // Usage*
	Kind     string // SubjectProgram или SubjectSite
	Subject  string // путь exe в нижнем регистре или домен
	Seconds  int
}

func checkDay(day string) error {
	if t, err := time.Parse(time.DateOnly, day); err != nil || t.Format(time.DateOnly) != day {
		return fmt.Errorf("%w: день %q не дата вида ГГГГ-ММ-ДД", ErrInvalid, day)
	}
	return nil
}

func checkUsage(u UsageRow) error {
	switch {
	case !usageCategories[u.Category]:
		return fmt.Errorf("%w: категория учёта %q", ErrInvalid, u.Category)
	case !usageKinds[u.Kind]:
		return fmt.Errorf("%w: вид учёта %q", ErrInvalid, u.Kind)
	case u.Subject == "":
		return fmt.Errorf("%w: у учёта нет программы или сайта", ErrInvalid)
	case u.Seconds < 0:
		return fmt.Errorf("%w: секунд учёта %d", ErrInvalid, u.Seconds)
	}
	return checkDay(u.Day)
}

// addUsage прибавляет секунды порции к строке usage_daily (вставка или сложение). Ноль — ничего.
func addUsage(ex execer, childID int64, u UsageRow) error {
	if u.Seconds == 0 {
		return nil
	}
	_, err := ex.Exec(`INSERT INTO usage_daily (child_id, day, category, subject_kind, subject, seconds) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (child_id, day, category, subject_kind, subject) DO UPDATE SET seconds = seconds + excluded.seconds`,
		childID, u.Day, u.Category, u.Kind, u.Subject, u.Seconds)
	return err
}

// AddUsage прибавляет seconds секунд категории category по программе или сайту (kind, subject)
// за день day (статистика «за всё время»). Ноль секунд — ничего не пишется; «Системное», чужой
// вид, пустой предмет, отрицательные секунды и не дата — ErrInvalid.
func (s *Store) AddUsage(childID int64, day, category, kind, subject string, seconds int) error {
	u := UsageRow{Day: day, Category: category, Kind: kind, Subject: subject, Seconds: seconds}
	if err := checkUsage(u); err != nil {
		return err
	}
	if err := addUsage(s.db, childID, u); err != nil {
		return fmt.Errorf("storage: записать учёт: %w", err)
	}
	return nil
}

// SaveDayAndUsage записывает состояние дня и накопленные службой порции учёта по программам
// одной транзакцией (RM3-6, G10): после сбоя не бывает дня без учёта или учёта без дня. Всё
// проверяется до записи (ErrInvalid); сбой базы откатывает и день, и учёт.
func (s *Store) SaveDayAndUsage(childID int64, st core.DayState, usage []UsageRow) error {
	if err := checkDay(st.Day); err != nil {
		return err
	}
	rows := make([]UsageRow, 0, len(usage))
	for _, u := range usage {
		if u.Day == "" {
			u.Day = st.Day
		}
		if err := checkUsage(u); err != nil {
			return err
		}
		rows = append(rows, u)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage: сохранить день и учёт: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := saveDayState(tx, childID, st); err != nil {
		return fmt.Errorf("storage: сохранить день и учёт: %w", err)
	}
	for _, u := range rows {
		if err := addUsage(tx, childID, u); err != nil {
			return fmt.Errorf("storage: сохранить день и учёт: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: сохранить день и учёт: %w", err)
	}
	return nil
}

// ProgramSeconds возвращает секунды по каждой программе (путь exe) у всех детей за дни с fromDay
// включительно: «время за неделю» в «Программах» (FR-20). Нет учёта: пустая карта, не nil.
func (s *Store) ProgramSeconds(fromDay string) (map[string]int, error) {
	if err := checkDay(fromDay); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT subject, sum(seconds) FROM usage_daily WHERE subject_kind = ? AND day >= ? GROUP BY subject`,
		SubjectProgram, fromDay)
	if err != nil {
		return nil, fmt.Errorf("storage: время по программам: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var subject string
		var sec int
		if err := rows.Scan(&subject, &sec); err != nil {
			return nil, fmt.Errorf("storage: время по программам: %w", err)
		}
		out[subject] = sec
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: время по программам: %w", err)
	}
	return out, nil
}
