package service

import (
	"fmt"
	"strings"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
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

// usageKindSite — вид предмета учёта usage_daily: сайт (домен или запись списка, RM4-7).
const usageKindSite = "site"

// usageSubject — предмет учёта секунды: программа (путь exe) или сайт; пустой subject — не писать.
type usageSubject struct {
	kind, subject string
}

// siteCategory — категория секунды и предмет учёта по домену вкладки Brave (S-7); ok = false —
// домена нет (служебная страница, адрес не прочитан): секунда идёт как программа Brave.
func (s *Service) siteCategory(cs *childState, domain string) (core.Category, usageSubject, bool) {
	cls, entry := s.classifySite(domain, cs.child.AdultFilter)
	switch cls {
	case siteVideo:
		return core.CatVideoSocial, usageSubject{kind: usageKindSite, subject: entry}, true
	case siteStudy:
		return core.CatStudy, usageSubject{kind: usageKindSite, subject: entry}, true
	case siteOther:
		return core.CatOther, usageSubject{kind: usageKindSite, subject: entry}, true
	case siteBlocked, siteAdult:
		return core.CatOther, usageSubject{}, true // до закрытия — «Остальное время» (PL-38)
	}
	return core.CatOther, usageSubject{}, false
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
	blocked := []string{}
	for _, r := range list {
		if k, ok := sites.KindFromString(r.Kind); ok {
			rules = append(rules, sites.Rule{Domain: r.Domain, Kind: k})
		}
		if r.Kind == ipc.SiteKindBlocked {
			blocked = append(blocked, r.Domain)
		}
	}
	s.siteLists, s.siteListsOK = sites.NewLists(rules), true
	// Правила Brave (T5-5): домены только из удачного чтения. Первое удачное чтение после старта
	// службы (на старте списки не прочитались) — записать правила в обслуживании.
	if !s.braveReady && s.started {
		s.braveDue = true
	}
	s.braveBlocked, s.braveReady = blocked, true
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

// siteListName — название списка в сообщениях и журнале: винительный («в «Учёбу»») и родительный
// («из «Запрещённых»») падежи (S-10).
type siteListName struct{ acc, gen string }

// siteListNames — названия списков по ключу site_rules.kind.
var siteListNames = map[string]siteListName{
	ipc.SiteKindBlocked: {"«Запрещённые»", "«Запрещённых»"},
	ipc.SiteKindStudy:   {"«Учёбу»", "«Учёбы»"},
	ipc.SiteKindVideo:   {"«Видео и соцсети»", "«Видео и соцсетей»"},
}

// Вид запрета в подробностях site_forbidden_try.
const (
	siteKindBlocked = "blocked"
	siteKindAdult   = "adult"
)

const (
	// unreadCloseAfter — сколько подряд у окна Brave нет адреса, прежде чем окно закрывается
	// в «только учёбе» и при исчерпанном видео (S-3): свежее окно до 3,5 с без дерева доступности.
	unreadCloseAfter = 5 * time.Second
	// unreadLogEvery — строка «не удалось прочитать адрес» в журнал работы не чаще (RM4-1).
	unreadLogEvery = 10 * time.Minute
)

// siteGuard решает по снимку стража sm, закрыть ли вкладку или окно Brave (S-1, S-3, S-13), и
// ставит команду, плашку и запись журнала в очередь стража сеанса — их получит этот же ответ на
// опрос. closeTab повторяется в каждом ответе, пока вкладка под запретом на переднем плане (страж
// ведёт лестницу RM4-2 по числу подряд пришедших команд); плашка — не чаще plateEvery, запись —
// не чаще watchRepeatEvery на домен вкладки (RM4-9). Ничего не закрывает у администратора, в
// сеансе не на экране, на паузе, в безопасном режиме и под экраном блокировки; служебная страница
// не закрывается никогда. У скрытого родителем Brave закрываются только запрещённые и «18+»
// (запрет сильнее скрытия, PL-30). Под s.mu.
func (s *Service) siteGuard(cs *childState, ss *sessState, session uint32, sm ipc.SensorSample, real time.Time) {
	if !sm.DomainUnread {
		cs.unreadSince = time.Time{}
	}
	noAddress := sm.DomainUnread && sm.DomainNoAddress
	if !noAddress {
		cs.noAddrSince = time.Time{}
	}
	if cs.admin || ss == nil || ss.childID != cs.child.ID || ss.offScreen() {
		return
	}
	norm := programs.Normalize(sm.ForegroundPath) // настоящий путь, не ключ: по нему fpSeen и pubSeen
	if norm == "" {
		return
	}
	res := s.classifier.Classify(norm, s.fpSeen[norm], s.pubSeen[norm])
	if !isBrave(norm, res) {
		return
	}
	if sm.DomainUnread {
		if cs.unreadSince.IsZero() {
			cs.unreadSince = real
		}
		s.noteUnread(cs, session, real)
	}
	if noAddress && cs.noAddrSince.IsZero() {
		cs.noAddrSince = real
	}
	d := cs.eng.Current()
	if d.Status == core.StatusPaused || d.Status == core.StatusSafe || d.Lock != core.LockNone {
		return
	}
	domain := sm.ForegroundDomain
	cls, entry := s.classifySite(domain, cs.child.AdultFilter)
	hidden := res.Verdict == programs.VerdictHidden
	plate := ipc.BlockedPlate{ShowS: blockedPlateShowS}
	var kind string
	details := map[string]any{"domain": domain}
	switch {
	case cls == siteBlocked:
		kind, details["kind"] = evSiteForbiddenTry, siteKindBlocked
		plate.Kind, plate.Text = ipc.BlockedSite, fmt.Sprintf(textPlateSite, entry)
	case cls == siteAdult:
		kind, details["kind"] = evSiteForbiddenTry, siteKindAdult
		plate.Kind, plate.Text = ipc.BlockedAdult, textPlateAdult
	case hidden:
		return
	case d.StudyOnly && cls != siteStudy && cls != siteNone:
		kind = evSiteClosedStudyOnly
		plate.Kind, plate.Text = ipc.BlockedStudyOnly, d.CloseReason
		if plate.Text == "" {
			plate.Text = core.StudyOnlyPlateText
		}
	case d.CloseVideo && cls == siteVideo:
		kind = evSiteClosedVideo
		plate.Kind, plate.Text = ipc.BlockedVideoOver, d.VideoCloseReason
		if plate.Text == "" {
			plate.Text = core.VideoOverPlateText(d.RemainingS, d.Unlimited)
		}
	case cls == siteNone && noAddress && (d.StudyOnly || d.CloseVideo):
		// Окно без адреса (S-3): закрыть, только если «адреса нет» подряд unreadCloseAfter. Сбои
		// чтения UIA отсчёт не ведут и окно не закрывают (B-55, PL-46): страж закроет только окно,
		// у которого и сам увидит «адреса нет».
		if real.Sub(cs.noAddrSince) >= unreadCloseAfter {
			s.guardCmd[session] = append(s.guardCmd[session], ipc.GuardCommand{Kind: ipc.GuardCmdCloseBraveWindow})
		}
		return
	default:
		return
	}
	s.guardCmd[session] = append(s.guardCmd[session], ipc.GuardCommand{Kind: ipc.GuardCmdCloseTab, Domain: domain})
	if s.watchOnce(fmt.Sprintf("ev|%d|%s|%s", cs.child.ID, kind, domain), watchRepeatEvery, real) {
		s.journal.Write(cs.child.ID, kind, details)
	}
	s.sitePlate(session, cs.child.ID, "plate|"+itoa(int(session))+"|site|"+domain, plate, real)
}

// sitePlate ставит плашку о закрытой вкладке в очередь стража сеанса: только ребёнку, чей сеанс
// на экране, и не чаще plateEvery по ключу key (домен вкладки, не путь: у всех плашек сайтов путь
// пустой, и ключ по пути глушил бы одну другой). Под s.mu.
func (s *Service) sitePlate(session uint32, childID int64, key string, plate ipc.BlockedPlate, real time.Time) {
	ss := s.sessions[session]
	if ss == nil || ss.childID != childID || ss.offScreen() {
		return
	}
	if !s.watchOnce(key, plateEvery, real) {
		return
	}
	s.guardCmd[session] = append(s.guardCmd[session], ipc.GuardCommand{Kind: ipc.GuardCmdBlocked, Plate: &plate})
}

// noteUnread записывает, что адрес вкладки Brave не читается (RM4-1, RK5): в журнал родителя раз в
// день учёта на ребёнка (память только в службе) и только если адрес не читается подряд не меньше
// unreadCloseAfter (свежее окно строит дерево доступности до 3,5 с — это не сбой), в журнал работы
// раз в unreadLogEvery на сеанс. cs.unreadSince уже поставлен. Под s.mu.
func (s *Service) noteUnread(cs *childState, session uint32, real time.Time) {
	if day := s.today(); s.unreadDay[cs.child.ID] != day && real.Sub(cs.unreadSince) >= unreadCloseAfter {
		if s.unreadDay == nil {
			s.unreadDay = map[int64]string{}
		}
		s.unreadDay[cs.child.ID] = day
		s.journal.Write(cs.child.ID, evSiteUnread, map[string]any{})
	}
	key := "brave-unread|" + itoa(int(session))
	if last, ok := s.errAt[key]; ok && real.Sub(last) < unreadLogEvery && !real.Before(last) {
		return
	}
	s.errAt[key] = real
	s.cfg.Logf("сеанс %d: %s", session, textSiteUnread)
}
