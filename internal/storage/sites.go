package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// Списки сайтов (таблица site_rules, R14, FR-21, RM4-5): общие для всех детей, у домена одна
// запись (UNIQUE(domain), миграция 009). Ключи — те же, что sites.Kind.String и CHECK схемы.
const (
	SiteBlocked     = "blocked"      // «Запрещённые»
	SiteStudy       = "study"        // «Учёба»
	SiteVideoSocial = "video_social" // «Видео и соцсети»
)

// maxSiteDomainLen — наибольшая длина домена (CHECK site_rules.domain, как sites.MaxDomainLen).
const maxSiteDomainLen = 253

var siteKinds = map[string]bool{SiteBlocked: true, SiteStudy: true, SiteVideoSocial: true}

// SiteRule — запись списка сайтов.
type SiteRule struct {
	ID     int64
	Domain string // без www, нижний регистр, punycode (нормализует служба: sites.NormalizeInput)
	Kind   string // SiteBlocked | SiteStudy | SiteVideoSocial
}

// checkSiteKind отклоняет список вне перечня до записи.
func checkSiteKind(kind string) error {
	if !siteKinds[kind] {
		return fmt.Errorf("%w: список сайтов %q", ErrInvalid, kind)
	}
	return nil
}

// checkSiteDomain отклоняет пустой и слишком длинный домен до записи. Хранилище домен не
// нормализует: это делает служба (единственный писатель).
func checkSiteDomain(domain string) error {
	if domain == "" || len(domain) > maxSiteDomainLen {
		return fmt.Errorf("%w: домен длиной %d, допустимо от 1 до %d", ErrInvalid, len(domain), maxSiteDomainLen)
	}
	return nil
}

// ListSiteRules возвращает все записи списков по порядку списка и домена. Нет записей: [], не nil.
func (s *Store) ListSiteRules() ([]SiteRule, error) {
	rows, err := s.db.Query(`SELECT id, domain, kind FROM site_rules ORDER BY kind, domain`)
	if err != nil {
		return nil, fmt.Errorf("storage: списки сайтов: %w", err)
	}
	defer rows.Close()
	out := []SiteRule{}
	for rows.Next() {
		var r SiteRule
		if err := rows.Scan(&r.ID, &r.Domain, &r.Kind); err != nil {
			return nil, fmt.Errorf("storage: списки сайтов: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: списки сайтов: %w", err)
	}
	return out, nil
}

// AddSiteRule кладёт домен в список kind (S-1, S-6). Записи нет — вставка: (false, "", nil). Домен
// уже в этом списке — ничего: (false, kind, nil). Домен в другом списке — перенос той же записи
// (id сохраняется): (true, прежний список, nil). Пустой или длиннее 253 домен, чужой список — ErrInvalid.
// Предел 1000 запрещённых хранилище не проверяет: это делает служба через CountSiteRules.
func (s *Store) AddSiteRule(domain, kind string) (moved bool, prevKind string, err error) {
	if err := checkSiteDomain(domain); err != nil {
		return false, "", err
	}
	if err := checkSiteKind(kind); err != nil {
		return false, "", err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, "", fmt.Errorf("storage: добавить сайт: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.QueryRow(`SELECT kind FROM site_rules WHERE domain = ?`, domain).Scan(&prevKind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`INSERT INTO site_rules (domain, kind) VALUES (?, ?)`, domain, kind); err != nil {
			return false, "", fmt.Errorf("storage: добавить сайт: %w", err)
		}
	case err != nil:
		return false, "", fmt.Errorf("storage: добавить сайт: %w", err)
	case prevKind == kind:
		return false, kind, nil
	default:
		if _, err := tx.Exec(`UPDATE site_rules SET kind = ? WHERE domain = ?`, kind, domain); err != nil {
			return false, "", fmt.Errorf("storage: перенести сайт: %w", err)
		}
		moved = true
	}
	if err := tx.Commit(); err != nil {
		return false, "", fmt.Errorf("storage: добавить сайт: %w", err)
	}
	return moved, prevKind, nil
}

// RemoveSiteRule удаляет домен из его списка и возвращает список, где он был (для «Отменить»:
// служба вызывает AddSiteRule с ним, S-5). Домена нет: ("", false, nil). Пустой домен — ErrInvalid.
func (s *Store) RemoveSiteRule(domain string) (prevKind string, ok bool, err error) {
	if domain == "" {
		return "", false, fmt.Errorf("%w: пустой домен", ErrInvalid)
	}
	err = s.db.QueryRow(`DELETE FROM site_rules WHERE domain = ? RETURNING kind`, domain).Scan(&prevKind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("storage: удалить сайт: %w", err)
	}
	return prevKind, true, nil
}

// CountSiteRules — число доменов в списке kind («N из 1000» и предел запрещённых, R14). Чужой
// список — ErrInvalid.
func (s *Store) CountSiteRules(kind string) (int, error) {
	if err := checkSiteKind(kind); err != nil {
		return 0, err
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM site_rules WHERE kind = ?`, kind).Scan(&n); err != nil {
		return 0, fmt.Errorf("storage: число сайтов в списке: %w", err)
	}
	return n, nil
}

// ReplaceSiteRules заменяет список kind целиком (мастер FR-2, S-4) одной транзакцией: прежние
// домены списка удаляются, каждый домен domains записывается в kind; домен из другого списка
// переносится, дубли входа дают одну запись. Ошибка любого домена (ErrInvalid) или базы — ничего
// не изменилось. Другие списки, кроме перенесённых доменов, не трогаются.
func (s *Store) ReplaceSiteRules(kind string, domains []string) error {
	if err := checkSiteKind(kind); err != nil {
		return err
	}
	for _, d := range domains {
		if err := checkSiteDomain(d); err != nil {
			return err
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage: заменить список сайтов: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM site_rules WHERE kind = ?`, kind); err != nil {
		return fmt.Errorf("storage: заменить список сайтов: %w", err)
	}
	for _, d := range domains {
		if _, err := tx.Exec(`INSERT INTO site_rules (domain, kind) VALUES (?, ?)
			ON CONFLICT (domain) DO UPDATE SET kind = excluded.kind`, d, kind); err != nil {
			return fmt.Errorf("storage: заменить список сайтов: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: заменить список сайтов: %w", err)
	}
	return nil
}
