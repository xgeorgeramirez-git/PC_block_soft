package service

import (
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
	"pcblock/internal/sites"
)

// Обработчики команд вехи M4 «Сайты и Brave» (R14, FR-21, RM4-4, RM4-5). Права проверены в
// Handle (parentToken). Всё, что меняет списки, вызывает sitesChanged: служба сразу закрывает
// по новым спискам, правила Brave переписываются (RM4-10).

// sitesView собирает экран «Сайты» (FR-21, 8.3.5): списки в порядке хранилища (по списку и
// домену), переключатели «18+» всех детей, предел «Запрещённых», подпись источника «18+».
// ok = false: списки не прочитаны (в ответе пустые списки).
func (s *Service) sitesView() (ipc.ListSitesReply, bool) {
	out := ipc.ListSitesReply{Sites: []ipc.SiteDTO{}, Children: []ipc.AdultFilterDTO{}, BlockedMax: ipc.MaxBlockedSites, AdultSource: textAdultSource}
	for _, id := range s.childOrder() {
		ch := s.children[id].child
		out.Children = append(out.Children, ipc.AdultFilterDTO{ChildID: id, Name: ch.Name, On: ch.AdultFilter})
	}
	list, err := s.cfg.Store.ListSiteRules()
	if err != nil {
		s.logRate("sites", "не удалось прочитать списки сайтов: %v", err)
		return out, false
	}
	for _, r := range list {
		out.Sites = append(out.Sites, ipc.SiteDTO{Domain: r.Domain, Kind: r.Kind})
	}
	return out, true
}

// hListSites — «Сайты»: списки и переключатели 18+ (FR-21).
func (s *Service) hListSites(*callCtx) ipc.Reply {
	view, ok := s.sitesView()
	if !ok {
		return ipc.Fail(textRetry)
	}
	return ipc.OK(view)
}

// hAddSite — добавить домен в список или перенести из другого (RM4-5: один домен — один список).
// Ввод родителя — домен или вставленный адрес (RM4-4). В «Запрещённых» не больше 1000 доменов
// (R14, URLBlocklist правил Brave); повтор в тот же список — отказ у поля раньше предела.
func (s *Service) hAddSite(c *callCtx) ipc.Reply {
	var req ipc.AddSiteRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if _, ok := sites.KindFromString(req.Kind); !ok {
		return bad()
	}
	domain, err := sites.NormalizeInput(req.Domain)
	if err != nil {
		return ipc.FailField(textNotDomain, ipc.FieldDomain)
	}
	if req.Kind == ipc.SiteKindBlocked {
		full, err := s.blockedFull(domain)
		if err != nil {
			s.logRate("sites-save", "сайт не записан: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		if full {
			return ipc.FailField(textBlockedFull, ipc.FieldDomain)
		}
	}
	moved, prev, err := s.cfg.Store.AddSiteRule(domain, req.Kind)
	if err != nil {
		s.logRate("sites-save", "сайт не записан: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	if !moved && prev == req.Kind {
		return ipc.FailField(textSiteDuplicate, ipc.FieldDomain)
	}
	reply := ipc.AddSiteReply{Domain: domain, Moved: moved}
	details := map[string]any{"domain": domain, "kind": req.Kind}
	if moved {
		reply.Message = "Сайт перенесён в " + siteListNames[req.Kind].acc
		s.journal.Write(0, evSiteMoved, details)
	} else {
		reply.Message = "Добавлено: " + domain
		s.journal.Write(0, evSiteAdded, details)
	}
	s.sitesChanged(c.real)
	reply.Sites, _ = s.sitesView()
	return ipc.OK(reply)
}

// blockedFull — в «Запрещённых» уже ipc.MaxBlockedSites доменов и domain среди них нет (повтор
// получает свой отказ «уже в списке», перенос в полный список — отказ о пределе).
func (s *Service) blockedFull(domain string) (bool, error) {
	n, err := s.cfg.Store.CountSiteRules(ipc.SiteKindBlocked)
	if err != nil || n < ipc.MaxBlockedSites {
		return false, err
	}
	list, err := s.cfg.Store.ListSiteRules()
	if err != nil {
		return false, err
	}
	for _, r := range list {
		if r.Domain == domain && r.Kind == ipc.SiteKindBlocked {
			return false, nil
		}
	}
	return true, nil
}

// hRemoveSite — удалить домен из своего списка. «Отменить» до UndoUntilMs — AddSite(Domain, Kind)
// окна (S-15): отдельной записи отмены служба не ведёт.
func (s *Service) hRemoveSite(c *callCtx) ipc.Reply {
	var req ipc.RemoveSiteRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	domain, err := sites.NormalizeInput(req.Domain)
	if err != nil {
		return ipc.FailField(textNotDomain, ipc.FieldDomain)
	}
	prev, ok, err := s.cfg.Store.RemoveSiteRule(domain)
	if err != nil {
		s.logRate("sites-save", "сайт не удалён: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	if !ok {
		return ipc.Fail(textSiteGone)
	}
	s.journal.Write(0, evSiteRemoved, map[string]any{"domain": domain, "kind": prev})
	s.sitesChanged(c.real)
	reply := ipc.RemoveSiteReply{Message: "Удалён " + domain, Domain: domain, Kind: prev, UndoUntilMs: c.real.Add(undoLife).UnixMilli()}
	reply.Sites, _ = s.sitesView()
	return ipc.OK(reply)
}

// hSetAdultFilter — «Блокировать сайты 18+» ребёнку (R14). Решение действует сразу: служба
// закрывает вкладки по cs.child.AdultFilter, не дожидаясь перезапуска.
func (s *Service) hSetAdultFilter(c *callCtx) ipc.Reply {
	var req ipc.SetAdultFilterRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if err := s.cfg.Store.SetAdultFilter(cs.child.ID, req.On); err != nil {
		s.logRate("sites-save", "переключатель 18+ не записан: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	cs.child.AdultFilter = req.On
	s.journal.Write(cs.child.ID, evSiteAdultFilter, map[string]any{"on": req.On})
	text := textAdultOff
	if req.On {
		text = textAdultOn
	}
	reply := ipc.SetAdultFilterReply{Message: cs.child.Name + ": " + text}
	reply.Sites, _ = s.sitesView()
	return ipc.OK(reply)
}

// hTestSeedPrograms записывает программы, учёт и «Ждут решения» как есть, без ключа магазина
// (только тестовый режим, права в commands.go; RM4-11, S46, PL-41): это «база 0.3.3» с повторами
// приложения магазина по версиям. Флаг слияния сбрасывается: повторы сольются при следующем
// старте службы (приёмка перезапускает службу). Сначала проверяется весь запрос, потом пишется.
func (s *Service) hTestSeedPrograms(c *callCtx) ipc.Reply {
	var req ipc.TestSeedProgramsRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	roots := s.classifier.Roots()
	progs := make([]Program, 0, len(req.Programs))
	for _, p := range req.Programs {
		path := programs.Normalize(p.Path)
		if path == "" || !seedProgramCats[p.Category] || !seedProgramSources[p.Source] || p.DecidedAt < 0 {
			return bad()
		}
		progs = append(progs, Program{Path: path, Name: programName(p.Name, path), Publisher: programs.ClipName(p.Publisher),
			Category: p.Category, Source: p.Source, AppRoot: roots.AppRoot(path), DecidedAt: p.DecidedAt,
			Hidden: p.Hidden, CustomName: p.CustomName})
	}
	today := core.DayOf(s.cfg.Clock.Now().In(s.cfg.Loc))
	for _, u := range req.Usage {
		t, err := time.Parse(time.DateOnly, u.Day)
		if programs.Normalize(u.Path) == "" || !seedCategories[u.Category] || u.Seconds < 0 || err != nil ||
			t.Format(time.DateOnly) != u.Day || u.Day > today {
			return bad()
		}
		if s.children[u.ChildID] == nil {
			return ipc.Fail(textNoSuchChild)
		}
	}
	for _, p := range req.Pending {
		if programs.Normalize(p.Path) == "" || p.Attempts < 1 || p.LastAttemptAt < 0 {
			return bad()
		}
		if s.children[p.ChildID] == nil {
			return ipc.Fail(textNoSuchChild)
		}
	}
	for _, p := range progs {
		if err := s.cfg.Store.UpsertProgram(p); err != nil {
			s.cfg.Logf("тестовая программа %s не записана: %v", p.Path, err)
			return ipc.Fail(textSaveFailed)
		}
	}
	for _, u := range req.Usage {
		if err := s.cfg.Store.AddUsage(u.ChildID, u.Day, u.Category, usageKindProgram, programs.Normalize(u.Path), u.Seconds); err != nil {
			s.cfg.Logf("тестовый учёт %s не записан: %v", u.Path, err)
			return ipc.Fail(textSaveFailed)
		}
	}
	for _, p := range req.Pending {
		path := programs.Normalize(p.Path)
		pend := PendingProgram{Path: path, Name: programName(p.Name, path), Publisher: programs.ClipName(p.Publisher),
			FirstChildID: p.ChildID, FirstAttemptAt: p.LastAttemptAt, LastAttemptAt: p.LastAttemptAt, Attempts: p.Attempts,
			Children: []PendingChild{{ChildID: p.ChildID, Attempts: p.Attempts, LastAttemptAt: p.LastAttemptAt}}}
		if err := s.cfg.Store.RestorePending(pend); err != nil {
			s.cfg.Logf("тестовая попытка %s не записана: %v", path, err)
			return ipc.Fail(textSaveFailed)
		}
	}
	if err := s.cfg.Store.SetSetting(settingStoreMerged, "0"); err != nil {
		s.cfg.Logf("флаг слияния повторов магазина не сброшен: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	s.reloadPrograms()
	s.cfg.Logf("Тестовые программы: %d, учёт %d, ждут решения %d", len(req.Programs), len(req.Usage), len(req.Pending))
	return ipc.OK(ipc.Empty{})
}

// seedProgramCats и seedProgramSources — категории и источники, которые принимает TestSeedPrograms.
var (
	seedProgramCats = map[string]bool{ipc.ProgramCatGames: true, ipc.ProgramCatStudy: true, ipc.ProgramCatOther: true,
		ipc.ProgramCatBlocked: true, ipc.ProgramCatSystem: true}
	seedProgramSources = map[string]bool{ipc.ProgramSourceCatalog: true, ipc.ProgramSourceSnapshot: true, ipc.ProgramSourceParent: true}
)
