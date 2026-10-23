package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Обработчики фидбек-раунда 0.2.2: снятие запрета на день (FB-7), копилку обнулить/убавить
// (FB-2), промокоды (FB-6), открыть папку логов (FB-4), автобонус за прошедший день (FB-5).

// defaultBonusRate — N по умолчанию: 5 мин бонуса за каждые полные 10 мин остатка (R25, D-49).
const defaultBonusRate = 5

// --- FB-7: снять/вернуть запретные часы на сегодня (R24, D-51) -------------------------

func (s *Service) hSetNoForbidden(c *callCtx) ipc.Reply {
	var req ipc.SetNoForbiddenRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	d := cs.eng.SetNoForbiddenToday(req.On)
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	msg := "Сегодня запретные часы не действуют"
	if !req.On {
		msg = "Запретные часы снова действуют сегодня"
	}
	return ipc.OK(ipc.SetNoForbiddenReply{Card: s.cardOf(cs), Message: msg})
}

// --- FB-2: копилку обнулить и убавить (FR-22, D-46) ------------------------------------

func (s *Service) hClearBonus(c *callCtx) ipc.Reply {
	var req ipc.ClearBonusRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	bal, err := s.cfg.Store.GetBonusBalance(cs.child.ID)
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	if bal <= 0 {
		return ipc.OK(ipc.BonusChangeReply{BalanceAfter: 0, Message: "Копилка уже пуста"})
	}
	after, err := s.cfg.Store.AppendBonus(cs.child.ID, -bal, "Обнулено родителем")
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	s.journal.Write(cs.child.ID, "bonus_clear", map[string]any{
		"was": bal, "text": fmt.Sprintf("Копилка обнулена родителем (было %d мин)", bal),
	})
	return ipc.OK(ipc.BonusChangeReply{BalanceAfter: after, Message: "Копилка обнулена"})
}

func (s *Service) hReduceBonus(c *callCtx) ipc.Reply {
	var req ipc.ReduceBonusRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if req.Minutes < 1 {
		return ipc.FailField("Укажите, на сколько убавить", ipc.FieldMinutes)
	}
	bal, err := s.cfg.Store.GetBonusBalance(cs.child.ID)
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	take := req.Minutes
	if take > bal {
		take = bal // не ниже нуля (D-46)
	}
	if take <= 0 {
		return ipc.OK(ipc.BonusChangeReply{BalanceAfter: 0, Message: "Копилка уже пуста"})
	}
	after, err := s.cfg.Store.AppendBonus(cs.child.ID, -take, "Убавлено родителем")
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	s.journal.Write(cs.child.ID, "bonus_reduce", map[string]any{
		"minutes": take, "balance": after, "text": fmt.Sprintf("Копилка убавлена родителем на %d мин", take),
	})
	return ipc.OK(ipc.BonusChangeReply{
		BalanceAfter: after, Message: fmt.Sprintf("Убавлено %s. Баланс %d мин", minutesText(take), after),
	})
}

// --- FB-6: промокоды на время (R26, D-50, FR-35) --------------------------------------

func (s *Service) hCreatePromo(c *callCtx) ipc.Reply {
	var req ipc.CreatePromoRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if strings.TrimSpace(req.Code) == "" {
		return ipc.FailField("Введите код", ipc.FieldCode)
	}
	if !core.PromoMinutesValid(req.Minutes) {
		return ipc.FailField("Можно от 1 до 600 минут", ipc.FieldMinutes)
	}
	p, err := s.cfg.Store.CreatePromo(req.Code, req.Minutes)
	if err != nil {
		// Самая частая причина — такой код уже есть (UNIQUE); показываем у поля кода.
		return ipc.FailField("Такой код уже есть, придумайте другой", ipc.FieldCode)
	}
	s.journal.Write(0, "promo_create", map[string]any{
		"code": p.CodeDisplay, "minutes": p.Minutes,
		"text": fmt.Sprintf("Создан промокод «%s» на %d мин", p.CodeDisplay, p.Minutes),
	})
	return ipc.OK(ipc.CreatePromoReply{
		Promo:   promoViewOf(p, ""),
		Message: fmt.Sprintf("Промокод «%s» на %s готов", p.CodeDisplay, minutesText(p.Minutes)),
	})
}

func (s *Service) hListPromos(c *callCtx) ipc.Reply {
	list, err := s.cfg.Store.ListPromos()
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	active := []ipc.PromoView{}
	used := []ipc.PromoView{}
	for _, p := range list {
		if p.UsedAtMs == 0 {
			active = append(active, promoViewOf(p, ""))
		} else {
			used = append(used, promoViewOf(p, s.names[p.UsedByChild]))
		}
	}
	return ipc.OK(ipc.ListPromosReply{Active: active, Used: used})
}

func (s *Service) hRedeemPromo(c *callCtx) ipc.Reply {
	var req ipc.RedeemPromoRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	// childSelf: ребёнок вводит код только для себя (экран блокировки 8.2.3 и раздел «Бонусы»).
	cs := c.self
	if cs == nil || (req.ChildID != 0 && req.ChildID != cs.child.ID) {
		return ipc.Fail(textNoSuchChild)
	}
	if strings.TrimSpace(req.Code) == "" {
		return ipc.FailField("Введите код", ipc.FieldCode)
	}
	minutes, display, ok, err := s.cfg.Store.RedeemPromo(cs.child.ID, req.Code, c.real.UnixMilli())
	if err != nil {
		return ipc.Fail(textSaveFailed)
	}
	if !ok {
		return ipc.FailField("Код не подходит", ipc.FieldCode)
	}
	// Применяем минуты к сегодняшнему дню как доп. время (расширяет все три лимита, R10/R26).
	d, err := cs.eng.RedeemPromoMinutes(minutes)
	if err != nil {
		// Код уже помечен использованным; на всякий случай сообщаем мягко.
		return ipc.Fail(textRetry)
	}
	s.afterDecision(cs, d, map[string]any{"code": display})
	s.saveDay(cs)
	// FR-35: ребёнок видит «Добавлено N минут». Код с экрана блокировки — плашку страж
	// показывает сам (дублей нет); код из окна ребёнка — стражу команда-плашка.
	msg := "Добавлено " + minutesText(minutes)
	if c.kind != kindGuard {
		s.noticeGuards(cs, msg)
	}
	return ipc.OK(ipc.RedeemPromoReply{Card: s.cardOf(cs), AddedMin: minutes, Message: msg})
}

// promoViewOf приводит запись промокода к виду для окна; usedByName — имя ребёнка в
// использованных (пусто для действующих).
func promoViewOf(p PromoRecord, usedByName string) ipc.PromoView {
	return ipc.PromoView{
		CodeDisplay: p.CodeDisplay, Minutes: p.Minutes, CreatedAtMs: p.CreatedAtMs,
		UsedAtMs: p.UsedAtMs, UsedByChild: p.UsedByChild, UsedByName: usedByName,
	}
}

// --- FB-4: открыть папку логов (FR-25, D-48) ------------------------------------------

func (s *Service) hOpenLogs(c *callCtx) ipc.Reply {
	path := s.logsDir()
	if !s.cfg.Test.Enabled {
		// В тестовом режиме реальный проводник не запускаем (раннер CI без рабочего стола).
		if err := s.cfg.OpenFolder(path); err != nil {
			s.logRate("openlogs", "не удалось открыть папку логов: %v", err)
			return ipc.Fail(textRetry)
		}
	}
	return ipc.OK(ipc.OpenLogsReply{Path: path, Message: "Открываю папку с логами"})
}

// --- FB-5: настройка автобонуса и начисление за прошедший день (R25, D-49) -------------

func (s *Service) hGetBonusRate(c *callCtx) ipc.Reply {
	return ipc.OK(ipc.BonusRateReply{Minutes: s.bonusRate()})
}

func (s *Service) hSetBonusRate(c *callCtx) ipc.Reply {
	var req ipc.SetBonusRateRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if req.Minutes < 0 || req.Minutes > 600 {
		return ipc.FailField("Можно от 0 до 600 минут", ipc.FieldMinutes)
	}
	if err := s.cfg.Store.SetSetting(settingBonusRate, strconv.Itoa(req.Minutes)); err != nil {
		s.logRate("bonusrate", "не сохранена настройка автобонуса: %v", err)
		return ipc.Fail(textRetry)
	}
	s.cfg.Logf("Автобонус настроен: %d мин за каждые 10 мин остатка (R25, D-49)", req.Minutes)
	return ipc.OK(ipc.BonusRateReply{Minutes: req.Minutes})
}

// bonusRate читает настройку N автобонуса из хранилища; нет записи или мусор — значение по
// умолчанию 5 (R25, D-49).
func (s *Service) bonusRate() int {
	v, ok, err := s.cfg.Store.GetSetting(settingBonusRate)
	if err != nil || !ok {
		return defaultBonusRate
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 600 {
		return defaultBonusRate
	}
	return n
}

// grantDailyBonus начисляет автобонус за прошедший (закрытый) день (R25, D-49): один раз за
// день, только в дни, когда игра была разрешена, с потолком копилки 600. Вызывается из
// afterDecision на смене суток; помечает day.BonusGranted, чтобы повторно не начислять.
func (s *Service) grantDailyBonus(cs *childState, day *core.DayState) {
	if day == nil || day.Day == "" || day.BonusGranted {
		return
	}
	t, err := time.Parse(time.DateOnly, day.Day)
	if err != nil {
		return
	}
	wd := core.WeekdayOf(t)
	limit := cs.rules.Week[int(wd)-1]
	allows := !core.DayFullyForbidden(cs.rules, wd)
	bonus := core.DailyBonusMinutes(limit, day.ExtraMin, day.TotalS, s.bonusRate(), allows)
	if bonus <= 0 {
		day.BonusGranted = true // за этот день начислять нечего — помечаем как обработанный
		return
	}
	bal, err := s.cfg.Store.GetBonusBalance(cs.child.ID)
	if err != nil {
		s.logRate(fmt.Sprint("autobonus", cs.child.ID), "автобонус: не прочитан баланс копилки %d: %v", cs.child.ID, err)
		return // не помечаем — транзиентная ошибка, R18
	}
	add := core.AddBonusWithinCeiling(bal, bonus)
	if add <= 0 {
		day.BonusGranted = true // копилка полна — начислять некуда, день обработан
		return
	}
	after, err := s.cfg.Store.AppendBonus(cs.child.ID, add, fmt.Sprintf("Бонус за свободное время: %d мин", add))
	if err != nil {
		s.logRate(fmt.Sprint("autobonus2", cs.child.ID), "автобонус: не начислено в копилку %d: %v", cs.child.ID, err)
		return
	}
	day.BonusGranted = true
	s.journal.Write(cs.child.ID, core.EvBonusGranted, map[string]any{
		"minutes": add, "balance": after, "day": day.Day,
		"text": fmt.Sprintf("Бонус за свободное время %s: +%d мин", day.Day, add),
	})
}
