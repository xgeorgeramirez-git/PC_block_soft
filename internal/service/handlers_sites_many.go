package service

import (
	"fmt"
	"strings"

	"pcblock/internal/ipc"
	"pcblock/internal/sites"
)

// Несколько сайтов за раз (8.3.5 п.2, B-49): вставка адресов по одному в строке и удаление
// отмеченных строк. Каждый сайт пишется теми же вызовами хранилища и журнала, что AddSite и
// RemoveSite (один домен — один список, предел «Запрещённых», запись журнала по сайту); правила
// Brave и закрытие по новым спискам — один раз на команду (sitesChanged).

// pasteLines — строки вставки без пробелов по краям, пустые пропущены.
func pasteLines(text string) []string {
	out := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(strings.TrimSuffix(line, "\r")); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// rejectedText — отказ у поля для строк, не похожих на адрес: одна — как у AddSite.
func rejectedText(n int) string {
	if n == 1 {
		return textNotDomain
	}
	return textSitesRejected
}

// hAddSites — вставить несколько адресов в список. Строки, не похожие на адрес, возвращаются окну
// (оно оставляет их в поле); повтор в пачке — один раз; уже бывший в списке — «уже были»;
// не поместившиеся в «Запрещённые» — счётом в итоге. Ни одного адреса — отказ у поля.
func (s *Service) hAddSites(c *callCtx) ipc.Reply {
	var req ipc.AddSitesRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if _, ok := sites.KindFromString(req.Kind); !ok {
		return bad()
	}
	lines := pasteLines(req.Text)
	if len(lines) > ipc.MaxSitesPerCall {
		return ipc.FailField(textSitesTooMany, ipc.FieldDomain)
	}
	reply := ipc.AddSitesReply{Added: []string{}, Rejected: []string{}}
	domains := []string{}
	seen := map[string]bool{}
	for _, line := range lines {
		d, err := sites.NormalizeInput(line)
		if err != nil {
			reply.Rejected = append(reply.Rejected, line)
			continue
		}
		if !seen[d] {
			seen[d] = true
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		return ipc.FailField(rejectedText(max(len(reply.Rejected), 1)), ipc.FieldDomain)
	}
	if len(reply.Rejected) > 0 {
		reply.FieldError = rejectedText(len(reply.Rejected))
	}
	list, err := s.cfg.Store.ListSiteRules()
	if err != nil {
		s.logRate("sites-save", "сайты не записаны: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	kindOf := map[string]string{}
	blocked := 0
	for _, r := range list {
		kindOf[r.Domain] = r.Kind
		if r.Kind == ipc.SiteKindBlocked {
			blocked++
		}
	}
	var added, moved, same, full int
	failed := false
	for _, d := range domains {
		if kindOf[d] == req.Kind {
			same++
			continue
		}
		if req.Kind == ipc.SiteKindBlocked && blocked >= ipc.MaxBlockedSites {
			full++
			continue
		}
		wasMoved, _, err := s.cfg.Store.AddSiteRule(d, req.Kind)
		if err != nil {
			s.logRate("sites-save", "сайт не записан: %v", err)
			failed = true
			break
		}
		if req.Kind == ipc.SiteKindBlocked {
			blocked++
		}
		kindOf[d] = req.Kind
		details := map[string]any{"domain": d, "kind": req.Kind}
		if wasMoved {
			moved++
			s.journal.Write(0, evSiteMoved, details)
		} else {
			added++
			s.journal.Write(0, evSiteAdded, details)
		}
		reply.Added = append(reply.Added, d)
	}
	if failed && len(reply.Added) == 0 {
		return ipc.Fail(textSaveFailed)
	}
	parts := []string{}
	for _, p := range []struct {
		n    int
		text string
	}{{added, textSitesAdded}, {moved, textSitesMoved}, {same, textSitesSame}, {full, textSitesFull}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf(p.text, p.n))
		}
	}
	if failed {
		parts = append(parts, textSitesAddRest)
	}
	reply.Message = strings.Join(parts, ". ")
	if len(reply.Added) > 0 {
		s.sitesChanged(c.real)
	}
	reply.Sites, _ = s.sitesView()
	return ipc.OK(reply)
}

// hRemoveSites — удалить отмеченные домены из их списков. «Отменить» до UndoUntilMs — AddSites
// окна по спискам из Removed. Уже удалённые пропускаются; не удалено ни одного — отказ.
func (s *Service) hRemoveSites(c *callCtx) ipc.Reply {
	var req ipc.RemoveSitesRequest
	if c.msg.Into(&req) != nil || len(req.Domains) == 0 || len(req.Domains) > ipc.MaxSitesPerCall {
		return bad()
	}
	reply := ipc.RemoveSitesReply{Removed: []ipc.SiteDTO{}}
	failed := false
	for _, raw := range req.Domains {
		d, err := sites.NormalizeInput(raw)
		if err != nil {
			continue
		}
		prev, ok, err := s.cfg.Store.RemoveSiteRule(d)
		if err != nil {
			s.logRate("sites-save", "сайт не удалён: %v", err)
			failed = true
			break
		}
		if !ok {
			continue
		}
		s.journal.Write(0, evSiteRemoved, map[string]any{"domain": d, "kind": prev})
		reply.Removed = append(reply.Removed, ipc.SiteDTO{Domain: d, Kind: prev})
	}
	switch {
	case len(reply.Removed) == 0 && failed:
		return ipc.Fail(textSaveFailed)
	case len(reply.Removed) == 0:
		return ipc.Fail(textSiteGone)
	case len(reply.Removed) == 1:
		reply.Message = "Удалён " + reply.Removed[0].Domain
	default:
		reply.Message = fmt.Sprintf(textSitesRemoved, len(reply.Removed))
	}
	if failed {
		reply.Message += ". " + textSitesRemoveRest
	}
	s.sitesChanged(c.real)
	reply.UndoUntilMs = c.real.Add(undoLife).UnixMilli()
	reply.Sites, _ = s.sitesView()
	return ipc.OK(reply)
}
