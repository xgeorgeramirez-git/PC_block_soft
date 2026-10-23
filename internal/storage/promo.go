package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"pcblock/internal/core"
)

// Промокоды на время (R26, D-50, FR-35, таблица promo_codes). Родитель создаёт код и минуты,
// ребёнок вводит один раз. Нормализация кода (без регистра и пробелов по краям) — core.NormalizePromoCode;
// применение минут к сегодняшнему дню (как доп. время) делает служба, здесь — только хранение
// и одноразовая пометка «использован».

// Promo — промокод в списке родителя (действующий или использованный).
type Promo struct {
	ID          int64
	CodeNorm    string
	CodeDisplay string
	Minutes     int
	CreatedAtMs int64
	UsedAtMs    *int64 // nil — действующий; иначе момент использования (UTC мс)
	UsedByChild *int64 // кто использовал; nil — ещё не использован
}

// CreatePromo добавляет промокод (R26, D-50): код как ввёл родитель + минуты 1..600. Код
// хранится в двух видах — отображаемый (code_display) и нормализованный для сравнения
// (code_norm). Пустой код или минуты вне 1..600 — ErrInvalid; код с таким же
// нормализованным видом уже есть — ErrInvalid («такой код уже есть»).
func (s *Store) CreatePromo(codeDisplay string, minutes int) (Promo, error) {
	norm := core.NormalizePromoCode(codeDisplay)
	if norm == "" {
		return Promo{}, fmt.Errorf("%w: пустой промокод", ErrInvalid)
	}
	if !core.PromoMinutesValid(minutes) {
		return Promo{}, fmt.Errorf("%w: минуты промокода %d, допустимо от 1 до 600", ErrInvalid, minutes)
	}
	at := nowMs()
	var id int64
	err := s.db.QueryRow(`INSERT INTO promo_codes (code_norm, code_display, minutes, created_at)
		VALUES (?, ?, ?, ?) RETURNING id`, norm, codeDisplay, minutes, at).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return Promo{}, fmt.Errorf("%w: такой код уже есть", ErrInvalid)
		}
		return Promo{}, fmt.Errorf("storage: создать промокод: %w", err)
	}
	return Promo{ID: id, CodeNorm: norm, CodeDisplay: codeDisplay, Minutes: minutes, CreatedAtMs: at}, nil
}

// RedeemPromo одноразово помечает действующий промокод использованным ребёнком childID и
// возвращает его минуты и отображаемый код (R26, D-50). Поиск и пометка — одним атомарным
// UPDATE по условию used_at IS NULL, поэтому повторный ввод того же кода (или гонка двух
// вводов) второй раз ничего не помечает: ok=false. Кода нет или уже использован — ok=false
// без ошибки (служба покажет «Код не подходит»). code — как ввёл ребёнок, нормализуется здесь.
func (s *Store) RedeemPromo(childID int64, code string, atMs int64) (minutes int, display string, ok bool, err error) {
	norm := core.NormalizePromoCode(code)
	if norm == "" {
		return 0, "", false, nil
	}
	err = s.db.QueryRow(`UPDATE promo_codes SET used_at = ?, used_by_child = ?
		WHERE code_norm = ? AND used_at IS NULL
		RETURNING minutes, code_display`, atMs, childID, norm).Scan(&minutes, &display)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("storage: применить промокод: %w", err)
	}
	return minutes, display, true, nil
}

// ListPromos возвращает все промокоды, новые сверху (FR-35): родитель видит действующие
// (UsedAtMs == nil) и использованные (с моментом и ребёнком). Пусто — пустой список, не nil.
func (s *Store) ListPromos() ([]Promo, error) {
	rows, err := s.db.Query(`SELECT id, code_norm, code_display, minutes, created_at, used_at, used_by_child
		FROM promo_codes ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("storage: список промокодов: %w", err)
	}
	defer rows.Close()
	out := []Promo{}
	for rows.Next() {
		var p Promo
		var usedAt, usedBy sql.NullInt64
		if err := rows.Scan(&p.ID, &p.CodeNorm, &p.CodeDisplay, &p.Minutes, &p.CreatedAtMs, &usedAt, &usedBy); err != nil {
			return nil, fmt.Errorf("storage: список промокодов: %w", err)
		}
		if usedAt.Valid {
			v := usedAt.Int64
			p.UsedAtMs = &v
		}
		if usedBy.Valid {
			v := usedBy.Int64
			p.UsedByChild = &v
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: список промокодов: %w", err)
	}
	return out, nil
}

// isUniqueViolation сообщает, что ошибка записи — нарушение UNIQUE (повтор code_norm).
// Драйвер modernc.org/sqlite не экспортирует типизированный код, поэтому смотрим текст
// ошибки: он содержит «UNIQUE constraint failed».
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
