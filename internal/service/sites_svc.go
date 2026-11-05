package service

import (
	"strings"
	"time"

	"pcblock/internal/programs"
	"pcblock/internal/sites"
)

// Сайты в Brave (веха M4: R6, R8, R13, R14, R23, RM4-1, RM4-5, RM4-7, RM4-9). Страж присылает в
// снимке только домен активной вкладки; служба по своим спискам родителя и встроенному списку
// «18+» решает категорию секунды, предмет учёта и что закрыть.

// siteClass — чем служба считает домен вкладки Brave (RM4-5).
type siteClass int

const (
	siteNone    siteClass = iota // домена нет: служебная страница, новая вкладка или адрес не прочитан
	siteOther                    // ни в одном списке: «Прочее»
	siteBlocked                  // «Запрещённые»: закрыть
	siteStudy                    // «Учёба»
	siteVideo                    // «Видео и соцсети»
	siteAdult                    // список «18+» при включённом переключателе ребёнка: закрыть
)

func (c siteClass) String() string {
	return [...]string{"siteNone", "siteOther", "siteBlocked", "siteStudy", "siteVideo", "siteAdult"}[c]
}

// isBrave — программа на переднем плане — Brave, которому разрешено работать (S-4): имя файла
// brave.exe и решение классификатора «разрешено» или «скрыто» (у скрытого вкладки всё равно
// закрываются, секунды не идут). Brave в другом месте («незнакомая») сайтами не управляется: его
// закрывает наблюдатель.
func isBrave(norm string, res programs.Result) bool {
	if !strings.HasSuffix(norm, `\brave.exe`) {
		return false
	}
	return res.Verdict == programs.VerdictAllow || res.Verdict == programs.VerdictHidden
}

// classifySite — класс домена вкладки и совпавшая запись (RM4-5). Самая длинная запись своих
// списков сильнее «18+»; без совпадения при включённом «18+» ребёнка (adultOn) — список «18+»;
// иначе «Прочее» с самим доменом. entry: запись списка (siteBlocked, siteStudy, siteVideo), домен
// (siteOther) или «» (siteNone, siteAdult: у «18+» записи для ребёнка нет). Под s.mu.
func (s *Service) classifySite(domain string, adultOn bool) (siteClass, string) {
	if domain == "" {
		return siteNone, ""
	}
	if r, ok := s.siteLists.Match(domain); ok {
		switch r.Kind {
		case sites.KindBlocked:
			return siteBlocked, r.Domain
		case sites.KindStudy:
			return siteStudy, r.Domain
		case sites.KindVideo:
			return siteVideo, r.Domain
		}
	}
	if adultOn && s.adult.Contains(domain) {
		return siteAdult, ""
	}
	return siteOther, domain
}

// loadAdult открывает список «18+» (Config.Adult, иначе встроенный). Ошибка — пустой список и
// строка в журнал работы: служба работает дальше (R18).
func (s *Service) loadAdult() {
	open := s.cfg.Adult
	if open == nil {
		open = sites.Embedded
	}
	a, err := open()
	if err != nil {
		s.cfg.Logf("не удалось открыть список сайтов для взрослых, он не действует: %v", err)
		a = sites.AdultList{}
	}
	s.adult = a
}

// reloadSites перечитывает свои списки родителя. Ошибка — прежние списки остаются (при первом
// чтении — пустые), siteListsOK = false, повтор в Housekeep через rulesRetryEvery (R18). Под s.mu.
func (s *Service) reloadSites(real time.Time) {
	s.sitesAt = real
	list, err := s.cfg.Store.ListSiteRules()
	if err != nil {
		s.siteListsOK = false
		s.logRate("sites", "не удалось прочитать списки сайтов, действуют прежние: %v", err)
		return
	}
	rules := make([]sites.Rule, 0, len(list))
	for _, r := range list {
		if k, ok := sites.KindFromString(r.Kind); ok {
			rules = append(rules, sites.Rule{Domain: r.Domain, Kind: k})
		}
	}
	s.siteLists, s.siteListsOK = sites.NewLists(rules), true
}

// sitesChanged — списки сайтов изменились (обработчики «Сайтов», мастер): перечитать и отметить,
// что правила Brave надо переписать. Под s.mu.
func (s *Service) sitesChanged(real time.Time) {
	s.reloadSites(real)
	s.braveDue = true // читает T5 (RM4-10): переписать правила Brave в Housekeep
}

// retrySites — повтор чтения списков, которые не прочитались (из Housekeep).
func (s *Service) retrySites(real time.Time) {
	if !s.siteListsOK && real.Sub(s.sitesAt) >= rulesRetryEvery {
		s.reloadSites(real)
	}
}
