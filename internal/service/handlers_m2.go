package service

import (
	"fmt"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// --- расписание (R7, FR-17) -------------------------------------------------------------

func (s *Service) hGetSchedule(c *callCtx) ipc.Reply {
	var req ipc.GetScheduleRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	ivs, err := s.cfg.Store.GetSchedule(cs.child.ID)
	if err != nil {
		s.logRate(fmt.Sprint("getsched", cs.child.ID), "расписание не прочитано: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	return ipc.OK(ipc.GetScheduleReply{ChildID: cs.child.ID, Intervals: toIPCIntervals(core.NormalizeIntervals(ivs))})
}

func (s *Service) hSaveSchedule(c *callCtx) ipc.Reply {
	var req ipc.SaveScheduleRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	// Проверка полей: конец равен началу — пустой интервал (ТЗ 8.3.3).
	for _, iv := range req.Intervals {
		if iv.StartMin == iv.EndMin {
			return ipc.FailField(textIntervalSame, ipc.FieldEnd)
		}
		if iv.StartMin < 0 || iv.StartMin > 1439 || iv.EndMin < 0 || iv.EndMin > 1439 {
			return ipc.FailField(textIntervalRange, ipc.FieldStart)
		}
	}
	// Нормализация (объединение пересекающихся, до 6 на день, R7) — родитель видит итог.
	norm := core.NormalizeIntervals(fromIPCIntervals(req.Intervals))
	if err := s.cfg.Store.SetSchedule(cs.child.ID, norm); err != nil {
		s.logRate(fmt.Sprint("savesched", cs.child.ID), "расписание не записано: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	cs.rulesAt = c.real
	s.applyRules(cs, s.readRules(cs.child.ID))
	s.journal.Write(cs.child.ID, "settings_changed", map[string]any{"text": "изменены запретные часы"})
	return ipc.OK(ipc.SaveScheduleReply{Intervals: toIPCIntervals(norm), Card: s.cardOf(cs), Message: textSaved})
}

// --- пауза (R11, FR-25) -----------------------------------------------------------------

func (s *Service) hPause(c *callCtx) ipc.Reply {
	var req ipc.PauseRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	until, dur, ok := s.pauseUntil(req.Preset)
	if !ok {
		return bad()
	}
	d, err := cs.eng.Pause(until)
	if err != nil {
		return ipc.Fail(textRetry)
	}
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	msg := "Пауза " + core.FormatLimit(int(dur.Minutes()))
	s.noticeGuards(cs, msg)
	return ipc.OK(ipc.PauseReply{Card: s.cardOf(cs), Message: msg})
}

func (s *Service) hPauseAll(c *callCtx) ipc.Reply {
	var req ipc.PauseAllRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	until, dur, ok := s.pauseUntil(req.Preset)
	if !ok {
		return bad()
	}
	cards := []ipc.TodayCard{}
	for _, id := range s.childOrder() {
		cs := s.children[id]
		d, err := cs.eng.Pause(until)
		if err != nil {
			continue
		}
		s.afterDecision(cs, d, nil)
		s.saveDay(cs)
		s.noticeGuards(cs, "Пауза "+core.FormatLimit(int(dur.Minutes())))
		cards = append(cards, s.cardOf(cs))
	}
	return ipc.OK(ipc.PauseAllReply{Children: cards, Message: "Пауза для всех детей на " + core.FormatLimit(int(dur.Minutes()))})
}

// pauseUntil переводит preset в абсолютный момент конца паузы и её длительность (R11).
// «До конца дня» — местная полночь следующего дня.
func (s *Service) pauseUntil(preset ipc.PausePreset) (time.Time, time.Duration, bool) {
	now := s.cfg.Clock.Now().In(s.cfg.Loc)
	switch preset {
	case ipc.PauseMin15:
		return now.Add(15 * time.Minute), 15 * time.Minute, true
	case ipc.PauseHour1:
		return now.Add(time.Hour), time.Hour, true
	case ipc.PauseHour2:
		return now.Add(2 * time.Hour), 2 * time.Hour, true
	case ipc.PauseEndOfDay:
		y, m, d := now.Date()
		eod := time.Date(y, m, d, 0, 0, 0, 0, s.cfg.Loc).AddDate(0, 0, 1)
		return eod, eod.Sub(now), true
	default:
		return time.Time{}, 0, false
	}
}

// --- копилка (FR-22) --------------------------------------------------------------------

func (s *Service) hGrantBonus(c *callCtx) ipc.Reply {
	var req ipc.GrantBonusRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if req.Minutes < 1 || req.Minutes > 600 {
		return ipc.FailField(textBonusRange, ipc.FieldMinutes)
	}
	if req.Reason == "" {
		return ipc.FailField(textBonusReason, ipc.FieldReason)
	}
	bal, err := s.cfg.Store.AppendBonus(cs.child.ID, req.Minutes, req.Reason)
	if err != nil {
		// Превышение 600 — объяснить у поля, сколько можно (FR-22).
		cur, _ := s.cfg.Store.GetBonusBalance(cs.child.ID)
		return ipc.FailField(fmt.Sprintf("Можно начислить не больше %d мин", 600-cur), ipc.FieldMinutes)
	}
	rec := &undoRec{id: s.cfg.Rand(), minutes: req.Minutes, until: c.real.Add(undoLife)}
	cs.bonusUndo = rec
	s.journal.Write(cs.child.ID, "bonus_grant", map[string]any{
		"minutes": req.Minutes, "reason": req.Reason, "balance": bal,
		"text": fmt.Sprintf("Начислено в копилку +%d мин: %s", req.Minutes, req.Reason),
	})
	return ipc.OK(ipc.GrantBonusReply{
		BalanceAfter: bal, Message: fmt.Sprintf("Начислено %s. Баланс %d мин", minutesText(req.Minutes), bal),
		BonusID: rec.id, UndoUntilMs: rec.until.UnixMilli(),
	})
}

func (s *Service) hUndoBonus(c *callCtx) ipc.Reply {
	var req ipc.UndoBonusRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	rec := cs.bonusUndo
	if rec == nil || rec.id == "" || rec.id != req.BonusID || !c.real.Before(rec.until) {
		return ipc.Fail(textUndoGone)
	}
	cs.bonusUndo = nil
	bal, err := s.cfg.Store.AppendBonus(cs.child.ID, -rec.minutes, "отмена начисления")
	if err != nil {
		return ipc.Fail(textRetry)
	}
	s.journal.Write(cs.child.ID, "bonus_undo", map[string]any{
		"minutes": rec.minutes, "text": fmt.Sprintf("Начисление копилки отменено: -%d мин", rec.minutes),
	})
	return ipc.OK(ipc.UndoBonusReply{BalanceAfter: bal, Message: "Начисление отменено"})
}

func (s *Service) hGetBonus(c *callCtx) ipc.Reply {
	var req ipc.GetBonusRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	// Ребёнок своего сеанса читает только свою копилку (childSelf); родитель — любую.
	// Номер 0 от ребёнка значит «моя копилка»: окно «Моя статистика» своего номера не знает (FB-5).
	if c.self != nil && req.ChildID == 0 {
		req.ChildID = c.self.child.ID
	}
	cs := s.children[req.ChildID]
	if c.self != nil && (cs == nil || cs.child.ID != c.self.child.ID) {
		return ipc.Fail(textNoSuchChild)
	}
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	bal, err := s.cfg.Store.GetBonusBalance(cs.child.ID)
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	recs, err := s.cfg.Store.ListBonus(cs.child.ID)
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	entries := make([]ipc.BonusEntry, 0, len(recs))
	for _, r := range recs {
		entries = append(entries, ipc.BonusEntry{AtMs: r.AtMs, DeltaMin: r.DeltaMin, Reason: r.Reason, Balance: r.Balance})
	}
	return ipc.OK(ipc.GetBonusReply{ChildID: cs.child.ID, Balance: bal, Entries: entries})
}

// --- копирование правил (FR-33) ---------------------------------------------------------

func (s *Service) hCopyRules(c *callCtx) ipc.Reply {
	var req ipc.CopyRulesRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	from := s.children[req.FromChildID]
	to := s.children[req.ToChildID]
	if from == nil || to == nil {
		return ipc.Fail(textNoSuchChild)
	}
	rules := s.readRules(from.child.ID)
	// Копирование возвращает правила для черновика приёмника (FR-19): запись делает родитель
	// кнопкой «Сохранить», сразу в базу не пишем.
	limits := make([]ipc.WeekdayLimit, 0, 7)
	for i := 0; i < 7; i++ {
		l := rules.Week[i]
		w := ipc.WeekdayLimit{Weekday: i + 1, Unlimited: l.Unlimited()}
		if !w.Unlimited {
			w.TotalMin = *l.TotalMin
		}
		limits = append(limits, w)
	}
	return ipc.OK(ipc.CopyRulesReply{
		Limits: limits, Intervals: toIPCIntervals(rules.Schedule), Message: "Правила скопированы. Проверьте и сохраните",
	})
}

// --- ребёнок: попросить ещё и взять из копилки (FR-10, FR-11) ---------------------------

func (s *Service) hAskMore(c *callCtx) ipc.Reply {
	var req ipc.AskMoreRequest
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
	// Пароль родителя проверяется здесь (ребёнок зовёт родителя, FR-10).
	if ok, rep := s.checkPassword(req.Password, c.real, cs.child.ID); !ok {
		return rep
	}
	d, err := cs.eng.AddExtra(req.Minutes)
	if err != nil {
		return ipc.FailField(textMinutesRange, ipc.FieldMinutes)
	}
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	msg := "Добавлено " + minutesText(req.Minutes)
	s.journal.Write(cs.child.ID, core.EvExtraTime, map[string]any{"minutes": req.Minutes, "text": core.ExtraText(req.Minutes)})
	return ipc.OK(ipc.AskMoreReply{Card: s.cardOf(cs), Message: msg})
}

func (s *Service) hSpendBonus(c *callCtx) ipc.Reply {
	var req ipc.SpendBonusRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	// childSelf гарантировал c.self — ребёнок своего сеанса. Тратить можно только свою копилку.
	// Значок своего сеанса childID не знает и шлёт 0: берём ребёнка из c.self; если childID
	// задан, он обязан совпадать (минор m3 ревью: не тронуть чужую копилку).
	cs := c.self
	if cs == nil || (req.ChildID != 0 && req.ChildID != cs.child.ID) {
		return ipc.Fail(textNoSuchChild)
	}
	if req.Minutes < 1 {
		return ipc.FailField(textMinutesRange, ipc.FieldMinutes)
	}
	// В запретный интервал из копилки взять нельзя (FR-11): время не поможет.
	now := s.cfg.Clock.Now().In(s.cfg.Loc)
	if in, end := core.InForbidden(cs.rules, now); in {
		return ipc.FailField("Из копилки можно взять с "+core.FormatClock(end), ipc.FieldMinutes)
	}
	// Проверить баланс и списать атомарно (storage): трата сверх баланса отклоняется.
	bal, err := s.cfg.Store.AppendBonus(cs.child.ID, -req.Minutes, "взято из копилки")
	if err != nil {
		cur, _ := s.cfg.Store.GetBonusBalance(cs.child.ID)
		return ipc.FailField(fmt.Sprintf("В копилке %d мин, столько взять нельзя", cur), ipc.FieldMinutes)
	}
	d, err := cs.eng.SpendBonus(req.Minutes)
	if err != nil {
		// Откат списания, если ядро не приняло (не должно случиться — границы те же).
		_, _ = s.cfg.Store.AppendBonus(cs.child.ID, req.Minutes, "возврат: трата не применена")
		return ipc.FailField(textMinutesRange, ipc.FieldMinutes)
	}
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	s.journal.Write(cs.child.ID, core.EvBonusSpent, map[string]any{
		"minutes": req.Minutes, "balance": bal, "text": core.BonusSpentText(req.Minutes),
	})
	return ipc.OK(ipc.SpendBonusReply{Card: s.cardOf(cs), Message: core.BonusSpentText(req.Minutes), BalanceAfter: bal})
}

// --- преобразование интервалов между каналом и ядром ------------------------------------

func fromIPCIntervals(in []ipc.Interval) []core.Interval {
	out := make([]core.Interval, 0, len(in))
	for _, iv := range in {
		out = append(out, core.Interval{Weekday: core.Weekday(iv.Weekday), StartMin: iv.StartMin, EndMin: iv.EndMin})
	}
	return out
}

func toIPCIntervals(in []core.Interval) []ipc.Interval {
	out := make([]ipc.Interval, 0, len(in))
	for _, iv := range in {
		out = append(out, ipc.Interval{Weekday: int(iv.Weekday), StartMin: iv.StartMin, EndMin: iv.EndMin})
	}
	return out
}
