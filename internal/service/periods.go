package service

import (
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Периоды «Итогов» статистики (FB-13, D-61, 8.2.6 п.2): «Сегодня», «Неделя» с понедельника, «Месяц» с
// 1-го числа, «Год» с 1 января, «Всё время» с установки. Все календарные по местному времени службы и
// включают сегодняшний день (RF13-1).

// allFromDay — первый день периода «Всё время»: раньше любого дня учёта.
const allFromDay = "0001-01-01"

// Причины записей копилки о трате ребёнком (FR-11) и её откате: по ним считается «Взято из копилки».
const (
	reasonBonusSpent  = "взято из копилки"
	reasonBonusRefund = "возврат: трата не применена"
)

// periodFrom возвращает первый день (YYYY-MM-DD) периода period для момента now, уже переведённого в
// местный пояс службы. «» — сегодня (окно прежней версии периода не шлёт). Неизвестный период — ok = false.
func periodFrom(period string, now time.Time) (from string, ok bool) {
	y, m, d := now.Date()
	// Полдень, а не полночь: переход на летнее время в полночь не сдвигает дату.
	day := func(y int, m time.Month, d int) string {
		return time.Date(y, m, d, 12, 0, 0, 0, now.Location()).Format(time.DateOnly)
	}
	switch period {
	case "", ipc.PeriodToday:
		return day(y, m, d), true
	case ipc.PeriodWeek:
		back := (int(now.Weekday()) + 6) % 7 // понедельник = 0
		return day(y, m, d-back), true
	case ipc.PeriodMonth:
		return day(y, m, 1), true
	case ipc.PeriodYear:
		return day(y, time.January, 1), true
	case ipc.PeriodAll:
		return allFromDay, true
	}
	return "", false
}

// bonusSpentSince — сколько минут ребёнок взял из копилки (FR-11) с момента fromMs включительно:
// траты минус откаты неприменённых трат. Начисления и убавление родителем сюда не входят.
func bonusSpentSince(list []BonusRecord, fromMs int64) int {
	n := 0
	for _, r := range list {
		if r.AtMs < fromMs {
			continue
		}
		switch r.Reason {
		case reasonBonusSpent:
			n -= r.DeltaMin
		case reasonBonusRefund:
			n -= r.DeltaMin
		}
	}
	return max(0, n)
}

// periodTotals собирает «Итоги» за период from…сегодня: live — счётчики ядра за сегодня (нули, если в
// ядре ещё вчерашний день), прошлые дни — суммы базы строго до сегодняшнего дня (сегодняшняя строка
// базы — сохранённая копия счётчиков ядра, второй раз её не считать). Возвращает суммы и учёт по
// программам прошлых дней. Ошибка чтения базы — только сегодня: показать меньше, но согласованно
// (итог и строки из одного набора дней, класс A).
func (s *Service) periodTotals(cs *childState, from, today string, live core.DayState) (core.DayState, []UsageRow) {
	sum := core.DayState{Day: today, TotalS: live.TotalS, GamesS: live.GamesS, VideoS: live.VideoS, StudyS: live.StudyS, ExtraMin: live.ExtraMin}
	if from >= today {
		return sum, nil
	}
	past, err := s.cfg.Store.PeriodDays(cs.child.ID, from, today)
	if err != nil {
		s.logRate("perioddays", "не удалось прочитать итоги дней с %s у %s: %v", from, cs.child.Name, err)
		return sum, nil
	}
	rows, err := s.cfg.Store.PeriodUsage(cs.child.ID, from, today)
	if err != nil {
		// Итоги прошлых дней есть, строк нет: их время уйдёт в «Остальное время» (время не пропадает).
		s.logRate("periodusage", "не удалось прочитать время по программам с %s у %s: %v", from, cs.child.Name, err)
		rows = nil
	}
	sum.TotalS += past.TotalS
	sum.GamesS += past.GamesS
	sum.VideoS += past.VideoS
	sum.StudyS += past.StudyS
	sum.ExtraMin += past.ExtraMin
	return sum, rows
}

// periodBonusSpent — «Взято из копилки» за период с первого дня from (местная полночь).
func (s *Service) periodBonusSpent(cs *childState, from string) int {
	start, err := time.ParseInLocation(time.DateOnly, from, s.cfg.Loc)
	if err != nil {
		return 0
	}
	list, err := s.cfg.Store.ListBonus(cs.child.ID)
	if err != nil {
		s.logRate("bonusspent", "не удалось прочитать копилку у %s: %v", cs.child.Name, err)
		return 0
	}
	return bonusSpentSince(list, start.UnixMilli())
}

// seedCategories — категории учёта, которые принимает TestSeedDay.
var seedCategories = map[string]bool{programs.CatGames: true, programs.CatStudy: true, programs.CatOther: true}

// hTestSeedDay записывает итоги прошлого дня и учёт по программам за него (только тестовый режим,
// приёмка S42): тем же путём, что служба сохраняет день (SaveDayAndUsage). Сегодняшний и будущий дни
// не принимаются: сегодняшний ведёт ядро, иначе он посчитался бы дважды.
func (s *Service) hTestSeedDay(c *callCtx) ipc.Reply {
	var req ipc.TestSeedDayRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if t, err := time.Parse(time.DateOnly, req.Day); err != nil || t.Format(time.DateOnly) != req.Day ||
		req.Day >= core.DayOf(s.cfg.Clock.Now().In(s.cfg.Loc)) || req.TotalS < 0 || req.GamesS < 0 || req.StudyS < 0 {
		return bad()
	}
	rows := make([]UsageRow, 0, len(req.Usage))
	for _, u := range req.Usage {
		path := programs.Normalize(u.Path)
		if path == "" || !seedCategories[u.Category] || u.Seconds < 0 {
			return bad()
		}
		rows = append(rows, UsageRow{Day: req.Day, Category: u.Category, Kind: usageKindProgram, Subject: path, Seconds: u.Seconds})
	}
	st := core.DayState{Day: req.Day, TotalS: req.TotalS, GamesS: req.GamesS, StudyS: req.StudyS}
	if err := s.cfg.Store.SaveDayAndUsage(cs.child.ID, st, rows); err != nil {
		s.cfg.Logf("тестовый день %s у %s не записан: %v", req.Day, cs.child.Name, err)
		return ipc.Fail(textSaveFailed)
	}
	s.cfg.Logf("Тестовый день %s у %s: общее %d с, игры %d с, учёба %d с, программ %d", req.Day, cs.child.Name, req.TotalS, req.GamesS, req.StudyS, len(rows))
	return ipc.OK(ipc.Empty{})
}
