package service

import (
	"pcblock/internal/ipc"
	"pcblock/internal/sites"
)

// Обработчики команд вехи M4 «Сайты и Brave» (R14, FR-21, RM4-4, RM4-5). Права проверены в
// Handle (parentToken). Всё, что меняет списки, вызывает sitesChanged: служба сразу закрывает
// по новым спискам, правила Brave переписываются (RM4-10). TestSeedPrograms — T6.

// textNotReadyM4 — ответ заглушки, пока обработчик не написан.
const textNotReadyM4 = "Пока не готово"

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

// hTestSeedPrograms — программы «до обновления» в тестовом режиме (RM4-11, S46). Заглушка T3;
// обработчик — T6.
func (s *Service) hTestSeedPrograms(*callCtx) ipc.Reply { return ipc.Fail(textNotReadyM4) }
