package service

import (
	"encoding/json"
	"strings"
	"time"

	"pcblock/internal/auth"
	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Обработчики команд. Права уже проверены в Handle (commands.go): здесь разбор запроса и
// действие. Блокировка s.mu снята с вызывающего, обработчик её держит.

// maxHelperState — предел размера состояния Помощника: канал и база не для картинок.
const maxHelperState = 64 << 10

func bad() ipc.Reply { return ipc.Fail(ipc.TextBadRequest) }

// childIDOf возвращает id ребёнка, чей процесс звонит (по SID), или 0.
func (s *Service) childIDOf(p ipc.Peer) int64 {
	if p.System {
		if ss := s.sessions[p.SessionID]; ss != nil {
			return ss.childID
		}
		return 0
	}
	if cs := s.childBySID(p.SID); cs != nil {
		return cs.child.ID
	}
	return 0
}

// --- общие -------------------------------------------------------------------------------

func (s *Service) hHello(c *callCtx) ipc.Reply {
	return ipc.OK(ipc.HelloReply{
		Protocol: ipc.ProtocolVersion, Version: s.cfg.Version,
		TestMode: s.cfg.Test.Enabled, Configured: s.configured(),
		AutoSwitchS: s.autoSwitchS(),
	})
}

// autoSwitchS: срок автосмены пользователя для стража в секундах, только в тестовом режиме.
func (s *Service) autoSwitchS() int {
	if !s.cfg.Test.Enabled {
		return 0
	}
	return s.cfg.Test.LockAutoSwitchS
}

func (s *Service) hGetChildStatus(c *callCtx) ipc.Reply {
	cs := s.childBySID(c.peer.SID)
	if cs == nil {
		return ipc.OK(ipc.ChildStatusNotChild())
	}
	return ipc.OK(s.statusOf(cs))
}

func (s *Service) hOpenWindow(c *callCtx) ipc.Reply {
	var req ipc.OpenWindowRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if _, ok := windowFlag(req.Kind); !ok {
		return ipc.Fail(textWindowKind)
	}
	ss := s.sessions[c.peer.SessionID]
	if c.viaBind {
		if ss == nil {
			return ipc.Fail(textRetry)
		}
		// Страж просит окно родителя после верного пароля: окно получает родительскую сессию.
		if req.Kind != ipc.WindowParent {
			return ipc.Fail(textDenyKind)
		}
		s.spendBind(c.peer.SessionID)
		opened, err := s.openWindow(ss, ipc.WindowParent, true, c.real)
		if err != nil {
			s.logRate("openwin", "окно родителя не открыто: %v", err)
			return ipc.Fail(textRetry)
		}
		return ipc.OK(ipc.OpenWindowReply{Opened: opened})
	}
	cs := s.childBySID(c.peer.SID)
	if req.Kind != ipc.WindowParent && cs == nil {
		return ipc.Fail(textNotAChild)
	}
	if ss == nil {
		return ipc.Fail(textRetry)
	}
	if req.Kind == ipc.WindowFirstRun && s.firstRunSeen(cs) {
		return ipc.OK(ipc.OpenWindowReply{Opened: false})
	}
	opened, err := s.openWindow(ss, req.Kind, false, c.real)
	if err != nil {
		s.logRate("openwin", "окно %s не открыто: %v", req.Kind, err)
		return ipc.Fail(textRetry)
	}
	return ipc.OK(ipc.OpenWindowReply{Opened: opened})
}

func (s *Service) hWindowClosing(c *callCtx) ipc.Reply {
	var req ipc.WindowClosingRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	s.windowClosing(c.peer, req.Kind)
	return ipc.OK(ipc.Empty{})
}

func (s *Service) hTrayClosing(c *callCtx) ipc.Reply {
	s.trayClosing(c.peer)
	return ipc.OK(ipc.Empty{})
}

// --- мастер и администратор ---------------------------------------------------------------

// canBeChild: детской может быть только обычная учётка (A2); в тестовом режиме запрет снят
// для учётки раннера (ТЗ 10.8).
func (s *Service) canBeChild(a AccountInfo) bool { return !a.Admin || s.exempt(a.SID) }

func (s *Service) hListAccounts(c *callCtx) ipc.Reply {
	list, err := s.cfg.Accounts.List()
	if err != nil {
		s.logRate("accounts", "не удалось получить список учёток: %v", err)
		return ipc.Fail(textRetry)
	}
	out := ipc.ListAccountsReply{Accounts: make([]ipc.Account, 0, len(list))}
	for _, a := range list {
		out.Accounts = append(out.Accounts, ipc.Account{
			SID: a.SID, Name: a.Name, Admin: a.Admin, Child: s.childBySID(a.SID) != nil,
			Current: a.SID == c.peer.SID, CanBeChild: s.canBeChild(a),
		})
	}
	return ipc.OK(out)
}

// weekFromLimits проверяет семь дней лимитов и собирает массив для ядра. При ошибке
// возвращает поле формы и текст у поля (L-121).
func weekFromLimits(ls []ipc.WeekdayLimit) (week [7]core.DayLimit, field, text string) {
	seen := [7]bool{}
	for _, l := range ls {
		if l.Weekday < 1 || l.Weekday > 7 {
			return week, "limits", textLimitsCount
		}
		f := ipc.LimitField(l.Weekday)
		switch {
		case seen[l.Weekday-1]:
			return week, f, textLimitsCount
		case !l.Unlimited && l.TotalMin < 0:
			return week, f, textLimitNegative
		case !l.Unlimited && l.TotalMin > 1440:
			return week, f, textLimitTooBig
		}
		seen[l.Weekday-1] = true
		if !l.Unlimited {
			week[l.Weekday-1] = core.LimitOf(l.TotalMin)
		}
	}
	for _, ok := range seen {
		if !ok {
			return week, "limits", textLimitsCount
		}
	}
	return week, "", ""
}

func (s *Service) hSetup(c *callCtx) ipc.Reply {
	var req ipc.SetupRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if err := auth.CheckLength(req.Password); err != nil {
		return ipc.FailField(err.Error(), ipc.FieldPassword)
	}
	if len(req.Children) == 0 {
		return ipc.FailField(textNoChildren, ipc.FieldChildren)
	}
	accounts, err := s.cfg.Accounts.List()
	if err != nil {
		s.logRate("accounts", "не удалось получить список учёток: %v", err)
		return ipc.Fail(textRetry)
	}
	type plan struct {
		sid, name string
		week      [7]core.DayLimit
	}
	var plans []plan
	seen := map[string]bool{}
	for _, ch := range req.Children {
		acc, ok := findAccount(accounts, ch.SID)
		switch {
		case !ok:
			return ipc.FailField(textNoAccount, ipc.FieldChildren)
		case !s.canBeChild(acc):
			return ipc.FailField(textAdminAccount, ipc.FieldChildren)
		}
		week, field, text := weekFromLimits(ch.Limits)
		if field != "" {
			return ipc.FailField(text, field)
		}
		if seen[ch.SID] {
			continue
		}
		seen[ch.SID] = true
		name := strings.TrimSpace(ch.Name)
		if name == "" {
			name = acc.Name
		}
		plans = append(plans, plan{ch.SID, name, week})
	}
	hash, err := s.cfg.Hasher.Hash(req.Password)
	if err != nil {
		return ipc.FailField(err.Error(), ipc.FieldPassword)
	}
	// Дети записываются раньше пароля: оборвавшаяся настройка остаётся «не настроено», и
	// мастер можно пройти заново.
	for _, p := range plans {
		id, err := s.cfg.Store.UpsertChild(p.sid, p.name)
		if err == nil {
			err = s.cfg.Store.SetDayLimits(id, p.week)
		}
		if err != nil {
			s.logRate("setup", "настройка не записана: %v", err)
			return ipc.Fail(textSaveFailed)
		}
	}
	if err := s.cfg.Store.SetAuth(Auth{Hash: hash, ChangedAtMs: c.real.UnixMilli()}); err != nil {
		s.logRate("setup", "пароль родителя не записан: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	s.loadChildren(c.real)
	for _, p := range plans {
		s.applyPolicy(p.sid, true)
	}
	s.journal.Write(0, "setup_done", map[string]any{"text": "Программа настроена: пароль родителя задан"})
	s.syncSessions(c.real)
	return ipc.OK(ipc.Empty{})
}

func findAccount(list []AccountInfo, sid string) (AccountInfo, bool) {
	for _, a := range list {
		if a.SID == sid {
			return a, true
		}
	}
	return AccountInfo{}, false
}

func (s *Service) hResetPassword(c *callCtx) ipc.Reply {
	var req ipc.ResetPasswordRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if err := auth.CheckLength(req.NewPassword); err != nil {
		return ipc.FailField(err.Error(), ipc.FieldNewPassword)
	}
	if !s.configured() {
		return ipc.Fail(textNotConfigured)
	}
	hash, err := s.cfg.Hasher.Hash(req.NewPassword)
	if err != nil {
		return ipc.FailField(err.Error(), ipc.FieldNewPassword)
	}
	if err := s.cfg.Store.SetAuth(Auth{Hash: hash, ChangedAtMs: c.real.UnixMilli()}); err != nil {
		s.logRate("setauth", "новый пароль не записан: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	for _, ps := range s.parents {
		s.endParent(ps)
	}
	s.journal.Write(0, "password_reset", map[string]any{"text": "Пароль родителя сброшен администратором"})
	return ipc.OK(ipc.Empty{})
}

func (s *Service) hUninstallCheck(c *callCtx) ipc.Reply {
	var req ipc.UninstallCheckRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if !s.configured() {
		return ipc.OK(ipc.UninstallCheckReply{Allowed: true})
	}
	if req.Forgot {
		s.journal.Write(0, "uninstall_forgot", map[string]any{"text": "Удаление программы без пароля: пароль забыт"})
		return ipc.OK(ipc.UninstallCheckReply{Allowed: true})
	}
	if ok, rep := s.checkPassword(req.Password, c.real, 0); !ok {
		return rep
	}
	return ipc.OK(ipc.UninstallCheckReply{Allowed: true})
}

// --- родительская сессия -----------------------------------------------------------------------

func (s *Service) hLogin(c *callCtx) ipc.Reply {
	var req ipc.LoginRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if ok, rep := s.checkPassword(req.Password, c.real, s.childIDOf(c.peer)); !ok {
		return rep
	}
	if c.kind == kindGuard {
		// Страж токена не получает: служба запоминает, что пароль только что проверен.
		until := s.bindGuard(c.peer.SessionID, c.real)
		return ipc.OK(ipc.LoginReply{ExpiresMs: until.UnixMilli(), IdleTimeoutS: int(guardBindLife / time.Second)})
	}
	ps := s.newParent(c.peer, c.real)
	if ps == nil {
		return ipc.Fail(textRetry)
	}
	return ipc.OK(ipc.LoginReply{
		Token: ps.token, ExpiresMs: ps.last.Add(parentIdle).UnixMilli(), IdleTimeoutS: int(parentIdle / time.Second),
	})
}

func (s *Service) hExtendSession(c *callCtx) ipc.Reply {
	ps := s.parentFor(c.peer, c.msg.Token, c.real, true)
	return ipc.OK(s.parentState(ps, c.real))
}

func (s *Service) hSessionState(c *callCtx) ipc.Reply {
	ps := s.parentFor(c.peer, c.msg.Token, c.real, false)
	return ipc.OK(s.parentState(ps, c.real))
}

func (s *Service) hLogout(c *callCtx) ipc.Reply {
	if ps := s.parentFor(c.peer, c.msg.Token, c.real, false); ps != nil {
		s.endParent(ps)
	}
	return ipc.OK(ipc.Empty{})
}

// --- окно родителя -----------------------------------------------------------------------------

func (s *Service) hGetToday(c *callCtx) ipc.Reply {
	out := ipc.GetTodayReply{Children: []ipc.TodayCard{}, Notices: []string{}}
	for _, id := range s.childOrder() {
		out.Children = append(out.Children, s.cardOf(s.children[id]))
	}
	if v, ok, err := s.cfg.Store.GetSetting(settingDBNotice); err == nil && ok && v != "" {
		out.Notices = append(out.Notices, v)
		if c.parent != nil {
			c.parent.sawNotices = true
		}
	}
	return ipc.OK(out)
}

// childArg находит ребёнка по номеру из запроса; для стража после пароля это ребёнок его
// сеанса (номер 0 или свой), чужого ребёнка страж менять не может.
func (s *Service) childArg(c *callCtx, id int64) *childState {
	if c.viaBind {
		if c.session == nil {
			return nil
		}
		cs := s.children[c.session.childID]
		if cs == nil || (id != 0 && id != cs.child.ID) {
			return nil
		}
		return cs
	}
	return s.children[id]
}

func (s *Service) hGetChildRules(c *callCtx) ipc.Reply {
	var req ipc.ChildRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	return ipc.OK(rulesView(cs))
}

func (s *Service) hSaveLimits(c *callCtx) ipc.Reply {
	var req ipc.SaveLimitsRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	week, field, text := weekFromLimits(req.Limits)
	if field != "" {
		return ipc.FailField(text, field)
	}
	if err := s.cfg.Store.SetDayLimits(cs.child.ID, week); err != nil {
		s.logRate("limits", "лимиты не записаны: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	// FR-19: правила действуют сразу, ближайший ответ стражу уже несёт новое решение.
	cs.rulesAt = c.real
	s.applyRules(cs, core.Rules{Week: week})
	s.journal.Write(cs.child.ID, "settings_changed", map[string]any{"text": "изменены лимиты времени"})
	return ipc.OK(ipc.SaveLimitsReply{Rules: rulesView(cs), Card: s.cardOf(cs), Message: textSaved})
}

func (s *Service) hAddExtra(c *callCtx) ipc.Reply {
	var req ipc.AddExtraRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.childArg(c, req.ChildID)
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if req.Minutes < 1 || req.Minutes > core.MaxExtraMin {
		return ipc.FailField(textMinutesRange, ipc.FieldMinutes)
	}
	if c.viaBind {
		s.spendBind(c.peer.SessionID)
	}
	d, err := cs.eng.AddExtra(req.Minutes)
	if err != nil {
		return ipc.FailField(textMinutesRange, ipc.FieldMinutes)
	}
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	rec := &undoRec{id: s.cfg.Rand(), minutes: req.Minutes, day: d.Day, until: c.real.Add(undoLife)}
	cs.undo = rec
	msg := "Добавлено " + minutesText(req.Minutes)
	if !c.viaBind {
		s.noticeGuards(cs, msg)
	}
	return ipc.OK(ipc.AddExtraReply{
		Card: s.cardOf(cs), Message: msg, ExtraID: rec.id, UndoUntilMs: rec.until.UnixMilli(),
	})
}

// noticeGuards просит стражей сеансов ребёнка показать короткое сообщение.
func (s *Service) noticeGuards(cs *childState, text string) {
	for _, id := range s.sessionOrder() {
		if ss := s.sessions[id]; ss.childID == cs.child.ID {
			s.guardCmd[id] = append(s.guardCmd[id], ipc.GuardCommand{Kind: ipc.GuardCmdNotice, Text: text})
		}
	}
}

func (s *Service) hUndoExtra(c *callCtx) ipc.Reply {
	var req ipc.UndoExtraRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	rec := cs.undo
	if rec == nil || rec.id == "" || rec.id != req.ExtraID || !c.real.Before(rec.until) {
		return ipc.Fail(textUndoGone)
	}
	st, bypass := cs.eng.State()
	if st.Day != rec.day || st.ExtraMin < rec.minutes {
		return ipc.Fail(textUndoGone)
	}
	st.ExtraMin -= rec.minutes
	// У ядра нет «убрать доп. время»: ядро пересоздаётся из состояния без него. Пороги R9
	// при этом взводятся заново по новому остатку, как при любом сдвиге.
	cs.eng = core.NewEngine(s.cfg.Clock, s.cfg.Loc, cs.rules, st, bypass)
	cs.undo = nil
	s.saveState(cs, st)
	s.journal.Write(cs.child.ID, "extra_undone", map[string]any{
		"minutes": rec.minutes, "text": "Доп. время отменено: -" + minutesTextShort(rec.minutes),
	})
	return ipc.OK(ipc.CardReply{Card: s.cardOf(cs), Message: "Добавленное время отменено"})
}

func minutesTextShort(n int) string { return itoa(n) + " мин" }

func (s *Service) hClearBlock(c *callCtx) ipc.Reply {
	var req ipc.ChildRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.childArg(c, req.ChildID)
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if c.viaBind {
		s.spendBind(c.peer.SessionID)
	}
	d := cs.eng.ClearBlock()
	if err := s.cfg.Store.SetBypass(cs.child.ID, ""); err != nil {
		s.logRate("bypass", "снятие блокировки не записано в базу: %v", err)
	}
	s.afterDecision(cs, d, nil)
	return ipc.OK(ipc.CardReply{Card: s.cardOf(cs), Message: textBlockCleared})
}

func (s *Service) hListEvents(c *callCtx) ipc.Reply {
	var req ipc.ListEventsRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = ipc.DefaultEventsLimit
	}
	limit = min(limit, ipc.MaxEventsLimit)
	q := EventQuery{ChildID: req.ChildID, Limit: limit}
	if req.FromMs != 0 || req.ToMs != 0 {
		// Хранилище отбирает по ребёнку, а границы времени служба накладывает сама.
		q.Limit = storeEventLimit
	}
	evs, err := s.cfg.Store.ListEvents(q)
	if err != nil {
		s.logRate("events", "журнал не прочитан: %v", err)
		return ipc.Fail(textRetry)
	}
	out := ipc.ListEventsReply{Events: make([]ipc.JournalEntry, 0, len(evs))}
	for _, e := range evs {
		if (req.FromMs != 0 && e.AtMs < req.FromMs) || (req.ToMs != 0 && e.AtMs >= req.ToMs) {
			continue
		}
		name := s.names[e.ChildID]
		out.Events = append(out.Events, ipc.JournalEntry{
			AtMs: e.AtMs, TzOffsetMin: e.TzOffsetMin, ChildID: e.ChildID, ChildName: name,
			Kind: e.Kind, Text: eventText(e.Kind, name, e.Details),
		})
		if len(out.Events) >= limit {
			break
		}
	}
	return ipc.OK(out)
}

// storeEventLimit — наибольший отбор событий, который отдаёт хранилище за раз.
const storeEventLimit = 1000

func (s *Service) hChangePassword(c *callCtx) ipc.Reply {
	var req ipc.ChangePasswordRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if err := auth.CheckLength(req.NewPassword); err != nil {
		return ipc.FailField(err.Error(), ipc.FieldNewPassword)
	}
	if ok, rep := s.checkPassword(req.OldPassword, c.real, s.childIDOf(c.peer)); !ok {
		if rep.Field == ipc.FieldPassword {
			rep.Field = ipc.FieldOldPassword
		}
		return rep
	}
	hash, err := s.cfg.Hasher.Hash(req.NewPassword)
	if err != nil {
		return ipc.FailField(err.Error(), ipc.FieldNewPassword)
	}
	if err := s.cfg.Store.SetAuth(Auth{Hash: hash, ChangedAtMs: c.real.UnixMilli()}); err != nil {
		s.logRate("setauth", "новый пароль не записан: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	s.journal.Write(0, "password_changed", map[string]any{"text": "Пароль родителя изменён"})
	return ipc.OK(ipc.Empty{})
}

func (s *Service) hSetChildren(c *callCtx) ipc.Reply {
	var req ipc.SetChildrenRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	accounts, err := s.cfg.Accounts.List()
	if err != nil {
		s.logRate("accounts", "не удалось получить список учёток: %v", err)
		return ipc.Fail(textRetry)
	}
	type addPlan struct{ sid, name string }
	var adds []addPlan
	for _, a := range req.Add {
		acc, ok := findAccount(accounts, a.SID)
		switch {
		case !ok:
			return ipc.FailField(textNoAccount, ipc.FieldChildren)
		case !s.canBeChild(acc):
			return ipc.FailField(textAdminAccount, ipc.FieldChildren)
		}
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = acc.Name
		}
		if s.childBySID(a.SID) == nil {
			adds = append(adds, addPlan{a.SID, name})
		}
	}
	for _, id := range req.Remove {
		if s.children[id] == nil {
			return ipc.FailField(textNoSuchChild, ipc.FieldChildren)
		}
	}
	for _, r := range req.Rename {
		if s.children[r.ChildID] == nil {
			return ipc.FailField(textNoSuchChild, ipc.FieldChildren)
		}
		if strings.TrimSpace(r.Name) == "" {
			return ipc.FailField(textNameEmpty, ipc.FieldChildren)
		}
	}
	for _, id := range req.Remove {
		cs := s.children[id]
		// Блокировка за обход снимается вместе с отметкой: иначе она вернулась бы, если
		// учётку отметят детской заново (эскалация T7).
		if err := s.cfg.Store.SetBypass(id, ""); err != nil {
			s.logRate("setchildren", "блокировка не снята при удалении ребёнка: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		if err := s.cfg.Store.RemoveChild(id); err != nil {
			s.logRate("setchildren", "ребёнок не убран: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		delete(s.children, id)
		s.applyPolicy(cs.child.SID, false)
	}
	for _, r := range req.Rename {
		cs := s.children[r.ChildID]
		name := strings.TrimSpace(r.Name)
		if _, err := s.cfg.Store.UpsertChild(cs.child.SID, name); err != nil {
			s.logRate("setchildren", "имя не записано: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		cs.child.Name = name
		s.names[cs.child.ID] = name
	}
	for _, a := range adds {
		id, err := s.cfg.Store.UpsertChild(a.sid, a.name)
		if err != nil {
			s.logRate("setchildren", "ребёнок не добавлен: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		s.addChild(Child{ID: id, SID: a.sid, Name: a.name}, c.real)
		s.applyPolicy(a.sid, true)
	}
	s.journal.Write(0, "children_changed", map[string]any{
		"text": "изменён состав детских учёток", "added": len(adds), "removed": len(req.Remove), "renamed": len(req.Rename),
	})
	s.syncSessions(c.real)
	out := ipc.SetChildrenReply{Children: make([]ipc.ChildInfo, 0, len(s.children))}
	for _, id := range s.childOrder() {
		cs := s.children[id]
		out.Children = append(out.Children, ipc.ChildInfo{ChildID: id, SID: cs.child.SID, Name: cs.child.Name})
	}
	return ipc.OK(out)
}

// --- Помощник ----------------------------------------------------------------------------------

// helperKey возвращает ключ состояния Помощника: профиль родителя (из родительской сессии)
// или профиль ребёнка (из его сеанса); ни того ни другого нет: отказ.
func (s *Service) helperKey(c *callCtx) (string, bool) {
	if c.parent != nil {
		return settingHelperParent, true
	}
	if id := s.childIDOf(c.peer); id != 0 && !c.peer.System {
		return settingHelperChildPrefix + itoa(int(id)), true
	}
	return "", false
}

func (s *Service) hGetHelperState(c *callCtx) ipc.Reply {
	key, ok := s.helperKey(c)
	if !ok {
		return ipc.Fail(textNeedParent)
	}
	v, found, err := s.cfg.Store.GetSetting(key)
	if err != nil {
		s.logRate("helper", "состояние Помощника не прочитано: %v", err)
		return ipc.Fail(textRetry)
	}
	if !found || !json.Valid([]byte(v)) {
		return ipc.OK(ipc.HelperState{})
	}
	return ipc.OK(ipc.HelperState{State: json.RawMessage(v)})
}

func (s *Service) hSaveHelperState(c *callCtx) ipc.Reply {
	key, ok := s.helperKey(c)
	if !ok {
		return ipc.Fail(textNeedParent)
	}
	var req ipc.HelperState
	if c.msg.Into(&req) != nil || len(req.State) > maxHelperState {
		return bad()
	}
	state := strings.TrimSpace(string(req.State))
	if state == "" {
		state = "{}"
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(state), &obj) != nil {
		return bad()
	}
	if err := s.cfg.Store.SetSetting(key, state); err != nil {
		s.logRate("helper", "состояние Помощника не записано: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	return ipc.OK(ipc.Empty{})
}

// --- ребёнок о себе -------------------------------------------------------------------------------

var weekdayShort = [7]string{"пн", "вт", "ср", "чт", "пт", "сб", "вс"}

func (s *Service) hMyToday(c *callCtx) ipc.Reply {
	cs := s.childBySID(c.peer.SID)
	if cs == nil {
		return ipc.OK(ipc.MyTodayReply{Status: ipc.ChildStatusNotChild(), Today: []string{}, Rules: []ipc.MyRule{}})
	}
	st := s.statusOf(cs)
	out := ipc.MyTodayReply{Status: st, Today: []string{st.Summary, "Использовано " + core.FormatDuration(st.UsedS)}, Rules: []ipc.MyRule{}}
	if st.Lock == ipc.LockTimeUp && st.Until != "" {
		out.Today = append(out.Today, st.Until)
	}
	if cs.rules.Broken {
		return ipc.OK(out)
	}
	for i := 0; i < 7; i++ {
		l := cs.rules.Week[i]
		text := "Без лимита"
		switch {
		case l.Unlimited():
		case *l.TotalMin == 0:
			text = "Нельзя"
		default:
			text = core.FormatLimit(*l.TotalMin)
		}
		out.Rules = append(out.Rules, ipc.MyRule{Weekday: i + 1, Day: weekdayShort[i], Text: text})
	}
	return ipc.OK(out)
}

func (s *Service) firstRunSeen(cs *childState) bool {
	if cs == nil {
		return true
	}
	v, ok, err := s.cfg.Store.GetSetting(settingFirstRunPrefix + itoa(int(cs.child.ID)))
	return err == nil && ok && v == "1"
}

func (s *Service) hFirstRunSeen(c *callCtx) ipc.Reply {
	if cs := s.childBySID(c.peer.SID); cs != nil {
		if err := s.cfg.Store.SetSetting(settingFirstRunPrefix+itoa(int(cs.child.ID)), "1"); err != nil {
			s.logRate("firstrun", "отметка первого входа не записана: %v", err)
			return ipc.Fail(textRetry)
		}
	}
	return ipc.OK(ipc.Empty{})
}

// --- страж -----------------------------------------------------------------------------------------

func (s *Service) hGuardPoll(c *callCtx) ipc.Reply {
	var req ipc.GuardPollRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	session := c.peer.SessionID
	ss := s.sessions[session]
	cs := s.children[ss.childID]
	if cs == nil {
		return ipc.OK(ipc.GuardPollReply{Status: ipc.ChildStatusNotChild(), Warnings: []ipc.GuardWarning{}, Commands: []ipc.GuardCommand{}})
	}
	cs.sample, cs.sampleAt, cs.haveSample = req.Sample, c.real, true
	for _, ev := range req.Events {
		if ev.Kind == ipc.GuardEvBypass && !cs.admin {
			s.markBypass(cs, ev.Detail, "guard")
		}
	}
	rep := ipc.GuardPollReply{Status: s.statusOf(cs), Warnings: []ipc.GuardWarning{}, Commands: []ipc.GuardCommand{}}
	cs.expireWarnings(c.real)
	for _, w := range cs.warns {
		rep.Warnings = append(rep.Warnings, w.w)
	}
	cs.warns = nil
	rep.Commands = append(rep.Commands, s.guardCmd[session]...)
	delete(s.guardCmd, session)
	if s.parentOpenIn(session, c.real) {
		rep.Commands = append(rep.Commands, ipc.GuardCommand{Kind: ipc.GuardCmdHideLock})
	}
	return ipc.OK(rep)
}

func (s *Service) hGuardLogoff(c *callCtx) ipc.Reply {
	if err := s.cfg.Launcher.Logoff(c.peer.SessionID); err != nil {
		s.logRate("logoff", "не удалось выйти из сеанса %d: %v", c.peer.SessionID, err)
		return ipc.Fail(textRetry)
	}
	return ipc.OK(ipc.Empty{})
}

// hGuardShutdown выключает компьютер по просьбе стража после подтверждения на экране. В
// тестовом режиме компьютер не выключается: раннер CI нельзя выключать, служба только
// записывает, что выключение запрошено (GuardState.Exits).
func (s *Service) hGuardShutdown(c *callCtx) ipc.Reply {
	if s.cfg.Test.Enabled {
		s.testExits = append(s.testExits, ipc.ExitShutdown)
		return ipc.OK(ipc.Empty{})
	}
	if err := s.cfg.Launcher.Shutdown(); err != nil {
		s.logRate("shutdown", "не удалось выключить компьютер: %v", err)
		return ipc.Fail(textRetry)
	}
	return ipc.OK(ipc.Empty{})
}

// hGuardSwitchUser отключает сеанс ребёнка без завершения по просьбе стража: нажатие кнопки
// «Смена пользователя» или автосмена через 15 минут. В тестовом режиме сеанс раннера не
// отключается, служба только записывает запрос (GuardState.Exits).
func (s *Service) hGuardSwitchUser(c *callCtx) ipc.Reply {
	if s.cfg.Test.Enabled {
		s.testExits = append(s.testExits, ipc.ExitSwitchUser)
		return ipc.OK(ipc.Empty{})
	}
	if err := s.cfg.Launcher.SwitchUser(c.peer.SessionID); err != nil {
		s.logRate("switchuser", "не удалось сменить пользователя в сеансе %d: %v", c.peer.SessionID, err)
		return ipc.Fail(textRetry)
	}
	return ipc.OK(ipc.Empty{})
}

// --- тестовый режим -------------------------------------------------------------------------------

var testActions = map[string]bool{
	ipc.TestClick: true, ipc.TestType: true, ipc.TestWMClose: true, ipc.TestAltF4: true, ipc.TestEsc: true,
}

func (s *Service) hTestGuard(c *callCtx) ipc.Reply {
	var req ipc.TestGuardRequest
	if c.msg.Into(&req) != nil || !testActions[req.Action.Action] {
		return bad()
	}
	session := req.SessionID
	if session == 0 {
		var found []uint32
		for _, id := range s.sessionOrder() {
			if ss := s.sessions[id]; ss.childID != 0 && ss.guard != nil && ss.guard.proc != nil {
				found = append(found, id)
			}
		}
		switch len(found) {
		case 0:
			return ipc.Fail(textNoGuard)
		case 1:
			session = found[0]
		default:
			return ipc.Fail(textManyGuards)
		}
	} else if ss := s.sessions[session]; ss == nil || ss.childID == 0 {
		return ipc.Fail(textNoGuard)
	}
	action := req.Action
	s.guardCmd[session] = append(s.guardCmd[session], ipc.GuardCommand{Kind: ipc.GuardCmdTest, Test: &action})
	return ipc.OK(ipc.Empty{})
}

// pausable — часы ядра, которые можно остановить (ScaledClock тестового режима).
type pausable interface {
	Pause()
	Resume()
}

func (s *Service) hTestClock(c *callCtx) ipc.Reply {
	var req ipc.TestClockRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	pc, ok := s.cfg.Clock.(pausable)
	if !ok {
		return ipc.Fail(textNoPause)
	}
	if req.Paused {
		pc.Pause()
	} else {
		pc.Resume()
	}
	s.clockPaused = req.Paused
	return ipc.OK(ipc.TestClockReply{
		Paused: req.Paused, NowMs: s.cfg.Clock.Now().UnixMilli(), Speed: max(s.cfg.Test.ClockSpeed, 1),
	})
}

// guardView — что страж сеанса показывает сейчас; страж в тестовом режиме присылает свой вид,
// проверяющий читает его пустым запросом.
type guardView struct {
	state ipc.GuardStateReply
	at    time.Time
}

func (s *Service) hGuardState(c *callCtx) ipc.Reply {
	if c.kind == kindGuard {
		var st ipc.GuardStateReply
		if c.msg.Into(&st) != nil {
			return bad()
		}
		s.guardUI[c.peer.SessionID] = &guardView{state: st, at: c.real}
		return ipc.OK(ipc.Empty{})
	}
	var best *guardView
	for _, id := range s.sessionOrder() {
		if v := s.guardUI[id]; v != nil && (best == nil || v.at.After(best.at)) {
			best = v
		}
	}
	// След выходов принадлежит службе: что прислал страж, отбрасывается.
	if best == nil {
		return ipc.OK(ipc.GuardStateReply{Exits: append([]string{}, s.testExits...)})
	}
	out := best.state
	out.Exits = append([]string{}, s.testExits...)
	return ipc.OK(out)
}
