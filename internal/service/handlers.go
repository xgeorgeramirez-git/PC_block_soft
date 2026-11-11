package service

import (
	"path/filepath"
	"strings"
	"time"

	"pcblock/internal/auth"
	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/sites"
)

// Обработчики команд. Права уже проверены в Handle (commands.go): здесь разбор запроса и
// действие. Блокировка s.mu снята с вызывающего, обработчик её держит.

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
	if req.Screen != "" && (req.Screen != ipc.ScreenPrograms || req.Kind != ipc.WindowParent) {
		return bad() // экран бывает только у окна родителя (RM3-4)
	}
	ss := s.sessions[c.peer.SessionID]
	if c.viaBind && req.Kind == ipc.WindowParent {
		if ss == nil {
			return ipc.Fail(textRetry)
		}
		// Страж просит окно родителя после верного пароля: окно получает родительскую сессию.
		s.spendBind(c.peer.SessionID)
		opened, err := s.openWindow(ss, ipc.WindowParent, req.Screen, true, c.real)
		if err != nil {
			s.logRate("openwin", "окно родителя не открыто: %v", err)
			return ipc.Fail(textRetry)
		}
		return ipc.OK(ipc.OpenWindowReply{Opened: opened})
	}
	cs := s.childBySID(c.peer.SID)
	if c.kind == kindGuard {
		// Страж без пароля (кнопки плашек): окно родителя без сессии — пароль спросит окно
		// («Попросить сейчас», RM3-4) — и «Моя статистика» ребёнка своего сеанса («Попросить ещё»
		// на плашке 1 мин, RM3-5). Окно первого входа открывает значок, не страж.
		if req.Kind == ipc.WindowFirstRun || ss == nil || ss.childID == 0 {
			return ipc.Fail(textDenyKind)
		}
		cs = s.children[ss.childID]
	}
	if req.Kind != ipc.WindowParent && cs == nil {
		return ipc.Fail(textNotAChild)
	}
	if ss == nil {
		return ipc.Fail(textRetry)
	}
	if req.Kind == ipc.WindowFirstRun && s.firstRunSeen(cs) {
		return ipc.OK(ipc.OpenWindowReply{Opened: false})
	}
	opened, err := s.openWindow(ss, req.Kind, req.Screen, false, c.real)
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

func (s *Service) hGuardClosing(c *callCtx) ipc.Reply {
	s.guardClosing(c.peer)
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

// weekFromLimits проверяет семь дней лимитов (общий, игр и видео) и собирает массив для ядра. При
// ошибке возвращает поле формы и текст у поля (L-121).
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
		if !l.GamesUnlimited {
			// Лимит игр (R5, FR-16): не ограничен общим — если больше, раньше кончится общий.
			switch g := ipc.GamesField(l.Weekday); {
			case l.GamesMin < 0:
				return week, g, textLimitNegative
			case l.GamesMin > 1440:
				return week, g, textLimitTooBig
			}
			games := l.GamesMin
			week[l.Weekday-1].GamesMin = &games
		}
		if !l.VideoUnlimited {
			// Лимит «Видео и соцсети» (R6, FR-16) — по образцу игр; день без ключа videoUnlimited
			// канал читает «без лимита» (S-3, ipc.WeekdayLimit.UnmarshalJSON).
			switch v := ipc.VideoField(l.Weekday); {
			case l.VideoMin < 0:
				return week, v, textLimitNegative
			case l.VideoMin > 1440:
				return week, v, textLimitTooBig
			}
			video := l.VideoMin
			week[l.Weekday-1].VideoMin = &video
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
	video, siteDenial := setupVideoSites(req.Sites)
	if siteDenial != nil {
		return *siteDenial
	}
	progs, denial := s.setupPrograms(req.Programs, c.real.UnixMilli())
	switch denial {
	case "":
	case ipc.TextBadRequest:
		return bad()
	default:
		return ipc.FailField(denial, fieldPrograms)
	}
	// Свойства программ читались вне блокировки службы: за это время служба могла начать
	// останавливаться или её настроил другой мастер.
	if s.stopping {
		return ipc.Fail(textRetry)
	}
	if s.configured() {
		return ipc.Fail(textAlreadySetup)
	}
	hash, err := s.cfg.Hasher.Hash(req.Password)
	if err != nil {
		return ipc.FailField(err.Error(), ipc.FieldPassword)
	}
	// Дети записываются раньше пароля: оборвавшаяся настройка остаётся «не настроено», и
	// мастер можно пройти заново.
	ids := make(map[string]int64, len(plans))
	for _, p := range plans {
		id, err := s.cfg.Store.UpsertChild(p.sid, p.name)
		if err == nil {
			err = s.cfg.Store.SetDayLimits(id, p.week)
		}
		if err != nil {
			s.logRate("setup", "настройка не записана: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		ids[p.sid] = id
	}
	// Программы мастера (FR-2) — тоже до пароля: оборвавшаяся настройка проходится заново.
	if err := s.writeSetupPrograms(progs); err != nil {
		s.logRate("setup", "программы мастера не записаны: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	// Сайты мастера (S-4 пакета T3) — тоже до пароля.
	if err := s.writeSetupSites(req.Sites, video, ids); err != nil {
		s.logRate("setup", "сайты мастера не записаны: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	if err := s.cfg.Store.SetAuth(Auth{Hash: hash, ChangedAtMs: c.real.UnixMilli()}); err != nil {
		s.logRate("setup", "пароль родителя не записан: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	s.loadChildren(c.real)
	s.sitesChanged(c.real)
	s.afterSetupPrograms(c)
	for _, p := range plans {
		s.applyPolicy(p.sid, true)
	}
	s.journal.Write(0, evSetupDone, map[string]any{"text": "Программа настроена: пароль родителя задан"})
	s.syncSessions(c.real)
	return ipc.OK(ipc.Empty{})
}

// setupVideoSites проверяет «Видео и соцсети» шага мастера «Сайты» до записи (RM4-4): каждый
// ввод сводится к домену, повторы схлопываются. Не домен — отказ у поля домена, больше
// ipc.MaxBlockedSites доменов — неверный запрос. Шага нет (nil) — nil, nil.
func setupVideoSites(st *ipc.SetupSites) ([]string, *ipc.Reply) {
	if st == nil {
		return nil, nil
	}
	out := make([]string, 0, len(st.Video))
	seen := map[string]bool{}
	for _, raw := range st.Video {
		d, err := sites.NormalizeInput(raw)
		if err != nil {
			r := ipc.FailField(textNotDomain, ipc.FieldDomain)
			return nil, &r
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	if len(out) > ipc.MaxBlockedSites {
		r := bad()
		return nil, &r
	}
	return out, nil
}

// writeSetupSites записывает шаг мастера «Сайты»: «Видео и соцсети» целиком и «18+» каждому
// ребёнку мастера (выключен у SID из AdultOff, у остальных включён: повтор оборванной настройки
// не оставляет прежнего выбора). Чужие SID в AdultOff пропускаются. Шага нет — ничего.
func (s *Service) writeSetupSites(st *ipc.SetupSites, video []string, ids map[string]int64) error {
	if st == nil {
		return nil
	}
	if err := s.cfg.Store.ReplaceSiteRules(ipc.SiteKindVideo, video); err != nil {
		return err
	}
	off := map[string]bool{}
	for _, sid := range st.AdultOff {
		off[sid] = true
	}
	for sid, id := range ids {
		if err := s.cfg.Store.SetAdultFilter(id, !off[sid]); err != nil {
			return err
		}
	}
	return nil
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
	s.journal.Write(0, evPasswordReset, map[string]any{"text": "Пароль родителя сброшен администратором"})
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
		s.journal.Write(0, evUninstallForgot, map[string]any{"text": "Удаление программы без пароля: пароль забыт"})
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
	if req.EndMode != "" && req.EndMode != ipc.EndModeLock && req.EndMode != ipc.EndModeStudyOnly {
		return ipc.FailField(textEndModeBad, ipc.FieldEndMode)
	}
	var brk core.BreakRules
	if req.Break != nil {
		var field, text string
		if brk, field, text = breakFromIPC(*req.Break); field != "" {
			return ipc.FailField(text, field)
		}
	}
	// Только режим «Когда время вышло» (вкладка FR-18) или только «Отдых» (FR-37): лимиты не
	// присланы и не меняются.
	saveLimits := len(req.Limits) > 0 || (req.EndMode == "" && req.Break == nil)
	var week [7]core.DayLimit
	if saveLimits {
		var field, text string
		if week, field, text = weekFromLimits(req.Limits); field != "" {
			return ipc.FailField(text, field)
		}
	}
	modeChanged := req.EndMode != "" && req.EndMode != cs.rules.EndMode.String()
	if saveLimits {
		if err := s.cfg.Store.SetDayLimits(cs.child.ID, week); err != nil {
			s.logRate("limits", "лимиты не записаны: %v", err)
			return ipc.Fail(textSaveFailed)
		}
	}
	if modeChanged {
		if err := s.cfg.Store.SetEndMode(cs.child.ID, req.EndMode); err != nil {
			s.logRate("endmode", "режим «Когда время вышло» не записан: %v", err)
			return ipc.Fail(textSaveFailed)
		}
	}
	breakChanged := req.Break != nil && brk != cs.rules.Break
	if breakChanged {
		if err := s.cfg.Store.SetBreakRules(cs.child.ID, brk); err != nil {
			s.logRate("break", "настройки отдыха не записаны: %v", err)
			return ipc.Fail(textSaveFailed)
		}
	}
	// FR-19: правила действуют сразу, ближайший ответ стражу уже несёт новое решение. Правила
	// берутся из базы целиком (RM3-9): лимиты, игры, расписание и режим.
	cs.rulesAt = c.real
	s.applyRules(cs, s.readRules(cs.child.ID))
	if saveLimits {
		s.journal.Write(cs.child.ID, evSettingsChanged, map[string]any{"text": "изменены лимиты времени"})
	}
	if modeChanged {
		s.journal.Write(cs.child.ID, evSettingsChanged, map[string]any{"text": "изменён режим, когда время вышло"})
	}
	if breakChanged {
		s.journal.Write(cs.child.ID, evSettingsChanged, map[string]any{"text": "изменены настройки отдыха"})
	}
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
	rec := &undoRec{id: s.cfg.Rand(), minutes: req.Minutes, extraDelta: req.Minutes, undoMsg: "Добавленное время отменено", day: d.Day, until: c.real.Add(undoLife)}
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
	if st.Day != rec.day {
		return ipc.Fail(textUndoGone)
	}
	// Откат знакового изменения доп. времени (R10, D-43): ядро пересоздаётся из состояния.
	// Пороги R9 взводятся заново по новому остатку, как при любом сдвиге.
	st.ExtraMin -= rec.extraDelta
	cs.eng = core.NewEngine(s.cfg.Clock, s.cfg.Loc, cs.rules, st, bypass)
	cs.undo = nil
	s.saveState(cs, st)
	s.journal.Write(cs.child.ID, evExtraUndone, map[string]any{
		"minutes": rec.minutes, "text": "Изменение доп. времени отменено",
	})
	return ipc.OK(ipc.CardReply{Card: s.cardOf(cs), Message: rec.undoMsg})
}

// hReduceExtra убавляет доп. время ребёнку (R10, D-43, кнопки «−15/−30/−60» и «своё»).
func (s *Service) hReduceExtra(c *callCtx) ipc.Reply {
	var req ipc.ReduceExtraRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.childArg(c, req.ChildID)
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if req.Minutes < 1 || req.Minutes > core.MaxExtraMin {
		return ipc.FailField(textReduceRange, ipc.FieldMinutes)
	}
	d, removed, err := cs.eng.ReduceExtra(req.Minutes)
	if err != nil {
		return ipc.FailField(textReduceRange, ipc.FieldMinutes)
	}
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	reply := ipc.AddExtraReply{Card: s.cardOf(cs), Message: "Убирать нечего"}
	if removed != 0 {
		rec := &undoRec{id: s.cfg.Rand(), minutes: removed, extraDelta: -removed, undoMsg: "Убранное время возвращено", day: d.Day, until: c.real.Add(undoLife)}
		cs.undo = rec
		reply.Message = "Убрано " + minutesText(removed)
		reply.ExtraID, reply.UndoUntilMs = rec.id, rec.until.UnixMilli()
		s.noticeGuards(cs, reply.Message)
	}
	return ipc.OK(reply)
}

// hClearExtra обнуляет сегодняшнее доп. время ребёнку (R10, D-43, «Убрать доп. время»).
func (s *Service) hClearExtra(c *callCtx) ipc.Reply {
	var req ipc.ClearExtraRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.childArg(c, req.ChildID)
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	d, removed := cs.eng.ClearExtra()
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	reply := ipc.AddExtraReply{Card: s.cardOf(cs), Message: "Доп. времени нет"}
	if removed != 0 {
		mins := removed
		if mins < 0 {
			mins = -mins
		}
		rec := &undoRec{id: s.cfg.Rand(), minutes: mins, extraDelta: -removed, undoMsg: "Доп. время возвращено", day: d.Day, until: c.real.Add(undoLife)}
		cs.undo = rec
		reply.Message = "Доп. время убрано"
		reply.ExtraID, reply.UndoUntilMs = rec.id, rec.until.UnixMilli()
		s.noticeGuards(cs, reply.Message)
	}
	return ipc.OK(reply)
}

// hGetDiag возвращает состояние переключателя подробных логов (FR-25, D-42).
func (s *Service) hGetDiag(c *callCtx) ipc.Reply {
	return ipc.OK(ipc.DiagReply{Verbose: s.verbose, LogsPath: s.logsDir()})
}

// logsDir — папка с логами службы, стража и значка (FR-25, D-48): показывается в настройках.
func (s *Service) logsDir() string {
	return filepath.Join(s.cfg.DataDir, "logs")
}

// hSetDiag включает или выключает подробные логи для диагностики (FR-25, D-42).
func (s *Service) hSetDiag(c *callCtx) ipc.Reply {
	var req ipc.SetDiagRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	s.verbose = req.Verbose
	val := "0"
	state := "выключены"
	if req.Verbose {
		val, state = "1", "включены"
	}
	if err := s.cfg.Store.SetSetting(settingVerboseLogs, val); err != nil {
		s.logRate("diag", "не сохранена настройка подробных логов: %v", err)
		return ipc.Fail(textRetry)
	}
	s.cfg.Logf("Подробные логи для диагностики %s (FR-25, D-42)", state)
	return ipc.OK(ipc.DiagReply{Verbose: s.verbose, LogsPath: s.logsDir()})
}

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

// hListEvents — журнал (FR-24, RM5-5): ребёнок, период, группа «Что» и курсор «Показать ещё»
// уходят в запрос хранилища; служба после чтения ничего не отбирает. More — есть записи старше
// страницы: хранилище просят на одну строку больше страницы.
func (s *Service) hListEvents(c *callCtx) ipc.Reply {
	var req ipc.ListEventsRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	var kinds []string // «Все события»: без отбора по виду, в том числе виды старых версий
	if req.Group != ipc.JournalGroupAll {
		var ok bool
		if kinds, ok = journalGroups[req.Group]; !ok {
			return bad()
		}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = ipc.DefaultEventsLimit
	}
	limit = min(limit, ipc.MaxEventsLimit)
	q := EventQuery{ChildID: req.ChildID, FromMs: req.FromMs, ToMs: req.ToMs, Kinds: kinds, Limit: limit + 1}
	if req.BeforeAtMs != 0 {
		q.BeforeAtMs, q.BeforeID = req.BeforeAtMs, req.BeforeID
	}
	evs, err := s.cfg.Store.ListEvents(q)
	if err != nil {
		s.logRate("events", "журнал не прочитан: %v", err)
		return ipc.Fail(textRetry)
	}
	more := len(evs) > limit
	if more {
		evs = evs[:limit]
	} else if q.Limit > storeEventLimit && len(evs) == limit {
		// Хранилище больше storeEventLimit строк за раз не отдаёт: есть ли старше — одной строкой.
		last := evs[len(evs)-1]
		probe := q
		probe.BeforeAtMs, probe.BeforeID, probe.Limit = last.AtMs, last.ID, 1
		older, err := s.cfg.Store.ListEvents(probe)
		if err != nil {
			s.logRate("events", "журнал не прочитан: %v", err)
			return ipc.Fail(textRetry)
		}
		more = len(older) > 0
	}
	out := ipc.ListEventsReply{Events: make([]ipc.JournalEntry, 0, len(evs)), More: more}
	for _, e := range evs {
		out.Events = append(out.Events, s.journalEntryOf(e))
	}
	return ipc.OK(out)
}

// journalEntryOf — запись журнала для окна: текст простыми словами с именем ребёнка (eventText).
func (s *Service) journalEntryOf(e StoredEvent) ipc.JournalEntry {
	name := s.names[e.ChildID]
	return ipc.JournalEntry{
		ID: e.ID, AtMs: e.AtMs, TzOffsetMin: e.TzOffsetMin, ChildID: e.ChildID, ChildName: name,
		Kind: e.Kind, Text: eventText(e.Kind, name, e.Details),
	}
}

// storeEventLimit — наибольший отбор событий, который отдаёт хранилище за раз (storage.maxEventLimit).
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
	s.journal.Write(0, evPasswordChanged, map[string]any{"text": "Пароль родителя изменён"})
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
		s.removeBrave(cs)
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
		cs := s.addChild(Child{ID: id, SID: a.sid, Name: a.name}, c.real)
		s.applyPolicy(a.sid, true)
		s.applyBrave(cs)
	}
	s.journal.Write(0, evChildrenChanged, map[string]any{
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

// --- ребёнок о себе -------------------------------------------------------------------------------

var weekdayShort = [7]string{"пн", "вт", "ср", "чт", "пт", "сб", "вс"}

func (s *Service) hMyToday(c *callCtx) ipc.Reply {
	var req ipc.MyTodayRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if _, ok := periodFrom(req.Period, time.Time{}); !ok {
		return bad()
	}
	return ipc.OK(s.myTodayOf(s.childBySID(c.peer.SID), req.Period))
}

// hParentStats возвращает статистику выбранного ребёнка за период для родительской сессии
// (FR-23, D-44, D-61): та же «Моя статистика», что видит ребёнок, только для просмотра.
func (s *Service) hParentStats(c *callCtx) ipc.Reply {
	var req ipc.ParentStatsRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if _, ok := periodFrom(req.Period, time.Time{}); !ok {
		return bad()
	}
	cs := s.childArg(c, req.ChildID)
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	return ipc.OK(s.myTodayOf(cs, req.Period))
}

// myTodayOf собирает «Моя статистика» (FR-12, FR-23): статус, строки «Сегодня» и «Мои правила»
// текстом за сегодня и «Итоги» за период period (D-61; «» — сегодня, период уже проверен). Один
// источник для ребёнка (hMyToday) и родителя (hParentStats).
func (s *Service) myTodayOf(cs *childState, period string) ipc.MyTodayReply {
	if period == "" {
		period = ipc.PeriodToday
	}
	if cs == nil {
		return ipc.MyTodayReply{Status: ipc.ChildStatusNotChild(), Today: []string{}, Rules: []ipc.MyRule{}, Period: period, Programs: []ipc.ProgramTime{},
			Bars: []ipc.DayBar{}, BarsBy: ipc.BarsByDay, Violations: []ipc.JournalEntry{}, Bonus: []ipc.BonusEntry{}}
	}
	st := s.statusOf(cs)
	out := ipc.MyTodayReply{Status: st, Today: []string{st.Summary, "Использовано " + core.FormatDuration(st.UsedS)}, Rules: []ipc.MyRule{}, Period: period}
	// Сегодня — счётчики дня ядра; счётчики прошлого дня (сутки ещё не сменились в ядре)
	// сегодняшними не считаются. Прошлые дни периода — из базы (RF13-1).
	now := s.cfg.Clock.Now().In(s.cfg.Loc)
	today := core.DayOf(now)
	var live core.DayState
	if day, _ := cs.eng.State(); day.Day == today {
		live = day
	}
	from, _ := periodFrom(period, now)
	// «Итоги» и столбики — из одного чтения ряда дней и одних счётчиков ядра (RM5-4).
	pd := s.periodTotals(cs, from, today, live)
	sum := pd.sum
	out.UsedS, out.GamesS, out.StudyS, out.VideoS = sum.TotalS, sum.GamesS, sum.StudyS, sum.VideoS
	out.OtherS = max(0, sum.TotalS-sum.GamesS-sum.VideoS)
	out.AllS = sum.TotalS + sum.StudyS
	out.ExtraMin = sum.ExtraMin
	out.Bars, out.BarsBy = buildBars(period, from, today, pd.days, pd.live, s.cfg.Loc)
	fromMs := s.dayStartMs(from)
	spentFrom := fromMs
	if pd.resetOK {
		spentFrom = max(spentFrom, pd.reset.ClearedAtMs)
	}
	out.BonusSpentMin, out.Bonus = s.periodBonus(cs, fromMs, spentFrom)
	out.Violations = s.periodViolations(cs, fromMs)
	out.StatsSinceDay, out.StatsSinceText = s.statsSince(cs, today, pd.reset, pd.resetOK)
	out.Programs = s.programTimes(cs, sum, pd.usage)
	// Строка «Можно снова…» нужна при любой блокировке: показываем, когда блокировка есть и
	// ядро назвало срок (B-6).
	if st.Lock != "" && st.Lock != ipc.LockNone && st.Until != "" {
		out.Today = append(out.Today, st.Until)
	}
	if cs.rules.Broken {
		return out
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
	// Общие правила (R21): лимиты игр и видео и что будет, когда время вышло.
	for _, text := range []string{gamesRuleText(cs.rules), videoRuleText(cs.rules)} {
		if text != "" {
			out.Rules = append(out.Rules, ipc.MyRule{Text: text})
		}
	}
	out.Rules = append(out.Rules, ipc.MyRule{Text: endModeRuleText(cs.rules.EndMode)})
	return out
}

// gamesRuleText — строка «Моих правил» о лимите игр (R5, R21): одинаковый каждый день — «Игры: до
// 1 ч в день» («Игры: нельзя»), иначе по дням с лимитом — «Игры: пн до 1 ч, ср нельзя». Лимита
// игр нет ни в один день — пусто.
func gamesRuleText(r core.Rules) string {
	return catRuleText(r, "Игры", func(l core.DayLimit) *int { return l.GamesMin })
}

// videoRuleText — то же для лимита «Видео и соцсети» (R6, R21): «Видео и соцсети: до 20 мин в день».
func videoRuleText(r core.Rules) string {
	return catRuleText(r, "Видео и соцсети", func(l core.DayLimit) *int { return l.VideoMin })
}

// catRuleText — строка «Моих правил» об отдельном лимите категории: title — начало строки, limit —
// минуты категории дня (nil — без отдельного лимита).
func catRuleText(r core.Rules, title string, limit func(core.DayLimit) *int) string {
	word := func(min int) string {
		if min == 0 {
			return "нельзя"
		}
		return "до " + core.FormatLimit(min)
	}
	same, parts := true, []string{}
	first := limit(r.Week[0])
	for i := 0; i < 7; i++ {
		g := limit(r.Week[i])
		if g == nil {
			same = false
			continue
		}
		if first == nil || *first != *g {
			same = false
		}
		parts = append(parts, weekdayShort[i]+" "+word(*g))
	}
	switch {
	case len(parts) == 0:
		return ""
	case same && *first == 0:
		return title + ": нельзя"
	case same:
		return title + ": " + word(*first) + " в день"
	}
	return title + ": " + strings.Join(parts, ", ")
}

// endModeRuleText — строка «Моих правил» о режиме «Когда время вышло» (R8, R21).
func endModeRuleText(m core.EndMode) string {
	if m == core.EndStudyOnly {
		return "Когда время вышло: остаётся только учёба"
	}
	return "Когда время вышло: экран блокировки"
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
	if len([]rune(req.Sample.ForegroundPath)) > ipc.MaxForegroundPathLen {
		req.Sample.ForegroundPath = "" // мусор от стража: путь не определён
	}
	if d := req.Sample.ForegroundDomain; len(d) > ipc.MaxDomainLen || sites.FromOmnibox(d) != d {
		// Не домен: мог быть адресом страницы, поэтому в журнал только длина (R23, S-5).
		s.logRate("bad-domain|"+itoa(int(session)), "страж прислал домен, не похожий на домен (%d символов), он не учитывается", len(d))
		req.Sample.ForegroundDomain = ""
	}
	cs.sample, cs.sampleAt, cs.sampleSess, cs.haveSample = req.Sample, c.real, session, true
	ss.resetLockBySample(req.Sample)
	// Вкладки Brave решаются в этом же ответе (S-1): команды встают в очередь перед её сливом.
	s.siteGuard(cs, ss, session, req.Sample, c.real)
	for _, ev := range req.Events {
		if ev.Kind == ipc.GuardEvBypass && !cs.admin {
			s.markBypass(cs, ev.Detail, "guard", c.real)
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

// testActions — действия стражу, которые служба передаёт в тестовом режиме (TestGuard, Ruling 5);
// TestForeground — подмена переднего плана (приёмка S38, S44, S45); TestDomain — подмена домена
// вкладки Brave (RM4-12, приёмка S16…S18).
var testActions = map[string]bool{
	ipc.TestClick: true, ipc.TestType: true, ipc.TestWMClose: true, ipc.TestAltF4: true, ipc.TestEsc: true,
	ipc.TestEndSession: true, ipc.TestForeground: true, ipc.TestDomain: true,
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

// hTestSession подаёт службе событие сеанса Windows тем же путём, что диспетчер служб (только
// тестовый режим, приёмка FB5).
func (s *Service) hTestSession(c *callCtx) ipc.Reply {
	var req ipc.TestSessionRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	var ev SessionEvent
	switch req.Event {
	case ipc.TestSessionDisconnect:
		ev = SessionDisconnect
	case ipc.TestSessionLogoff:
		ev = SessionLogoff
	default:
		return bad()
	}
	s.cfg.Logf("Тестовое событие сеанса %d: %s", req.SessionID, req.Event)
	if ev == SessionLogoff && req.SID != "" && s.sessions[req.SessionID] == nil {
		s.noteLogoff(req.SID, c.real)
	}
	s.onSession(ev, req.SessionID, c.real)
	return ipc.OK(ipc.Empty{})
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
