package service

import (
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
	"pcblock/internal/sites"
)

// Периоды «Итогов» статистики (FB-13, D-61, 8.2.6 п.2): «Сегодня», «Неделя» с понедельника, «Месяц» с
// 1-го числа, «Год» с 1 января, «Всё время» с установки. Все календарные по местному времени службы и
// включают сегодняшний день (RF13-1).

// allFromDay — первый день периода «Всё время»: раньше любого дня учёта.
const allFromDay = "0001-01-01"

// Причины записей копилки о трате ребёнком (FR-11) и её откате: их пишет hSpendBonus, по ним же
// считается «Взято из копилки» за период — одна строка на оба места. Значения не менять: по ним
// же читаются записи, уже лежащие в базе.
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

// periodData — «Итоги» и столбики периода из одного чтения (RM5-4): суммы (Day — сегодня), строки
// прошлых дней (уже с учётом очистки), учёт по программам и сайтам прошлых дней, снимок очистки.
type periodData struct {
	sum     core.DayState
	live    core.DayState // сегодня из ядра минус снимок очистки (для столбика сегодня)
	days    []DayRow
	usage   []UsageRow
	reset   StatsReset
	resetOK bool
}

// periodTotals собирает «Итоги» за период from…сегодня одним чтением ряда дней (RM5-4): live —
// счётчики ядра за сегодня (нули, если в ядре ещё вчерашний день), прошлые дни — строки базы строго
// до сегодняшнего дня (сегодняшняя строка базы — сохранённая копия счётчиков ядра, второй раз её не
// считать, PL-31). После очистки статистики (RM5-8) прошлые дни — не раньше дня очистки, день
// очистки и сегодня — минус снимок. Ошибка чтения снимка или дней — только сегодня: показать меньше,
// но согласованно (итог, столбики и строки из одного набора дней, класс A, PL-28).
func (s *Service) periodTotals(cs *childState, from, today string, live core.DayState) periodData {
	pd := periodData{days: []DayRow{}}
	readOK := true
	if r, ok, err := s.cfg.Store.GetStatsReset(cs.child.ID); err != nil {
		s.logRate("statsreset", "не удалось прочитать снимок очистки статистики у %s: %v", cs.child.Name, err)
		readOK = false
	} else {
		pd.reset, pd.resetOK = r, ok
	}
	pd.live = liveMinusReset(live, pd.reset, pd.resetOK)
	pd.sum = core.DayState{Day: today, TotalS: pd.live.TotalS, GamesS: pd.live.GamesS, VideoS: pd.live.VideoS, StudyS: pd.live.StudyS, ExtraMin: pd.live.ExtraMin}
	if pd.resetOK && pd.reset.Day > from {
		from = pd.reset.Day
	}
	if !readOK || from >= today {
		return pd
	}
	rows, err := s.cfg.Store.DaysSeries(cs.child.ID, from, today)
	if err != nil {
		s.logRate("daysseries", "не удалось прочитать итоги дней с %s у %s: %v", from, cs.child.Name, err)
		return pd
	}
	pd.days = applyReset(rows, pd.reset, pd.resetOK)
	usage, err := s.cfg.Store.PeriodUsage(cs.child.ID, from, today)
	if err != nil {
		// Итоги прошлых дней есть, строк нет: их время уйдёт в «Остальное время» (время не пропадает).
		s.logRate("periodusage", "не удалось прочитать время по программам с %s у %s: %v", from, cs.child.Name, err)
		usage = nil
	}
	pd.usage = usage
	for _, d := range pd.days {
		pd.sum.TotalS += d.TotalS
		pd.sum.GamesS += d.GamesS
		pd.sum.VideoS += d.VideoS
		pd.sum.StudyS += d.StudyS
		pd.sum.ExtraMin += d.ExtraMin
	}
	return pd
}

// seedCategories — категории учёта, которые принимает TestSeedDay.
var seedCategories = map[string]bool{programs.CatGames: true, programs.CatStudy: true, programs.CatOther: true, usageCatVideo: true}

// hTestSeedDay записывает итоги прошлого дня и учёт по программам и сайтам за него (только тестовый
// режим, приёмка S42, S21 по G15, S51): тем же путём, что служба сохраняет день (SaveDayAndUsage).
// Сегодняшний и будущий дни не принимаются: сегодняшний ведёт ядро, иначе он посчитался бы дважды.
// Путь программы — ключ, как у службы (programKey); сайт — домен, как служба пишет сайт (R23).
// День помечается рассмотренным для автобонуса (bonus_granted), если в запросе нет bonusPending;
// с bonusPending граница пропущенных дней settingAutobonusFrom сдвигается на этот день, если он раньше.
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
		req.Day >= core.DayOf(s.cfg.Clock.Now().In(s.cfg.Loc)) ||
		req.TotalS < 0 || req.GamesS < 0 || req.StudyS < 0 || req.VideoS < 0 || req.ExtraMin < 0 {
		return bad()
	}
	rows := make([]UsageRow, 0, len(req.Usage))
	for _, u := range req.Usage {
		if !seedCategories[u.Category] || u.Seconds < 0 {
			return bad()
		}
		var kind, subject string
		switch u.Kind {
		case "", ipc.SeedKindProgram:
			kind, subject = usageKindProgram, s.programKey(u.Path)
		case ipc.SeedKindSite:
			kind, subject = usageKindSite, sites.FromOmnibox(u.Path)
		default:
			return bad()
		}
		if subject == "" {
			return bad()
		}
		rows = append(rows, UsageRow{Day: req.Day, Category: u.Category, Kind: kind, Subject: subject, Seconds: u.Seconds})
	}
	st := core.DayState{Day: req.Day, TotalS: req.TotalS, GamesS: req.GamesS, VideoS: req.VideoS, StudyS: req.StudyS,
		ExtraMin: req.ExtraMin, BonusGranted: !req.BonusPending}
	if err := s.cfg.Store.SaveDayAndUsage(cs.child.ID, st, rows); err != nil {
		s.cfg.Logf("тестовый день %s у %s не записан: %v", req.Day, cs.child.Name, err)
		return ipc.Fail(textSaveFailed)
	}
	// День ждёт автобонуса (S51): граница пропущенных дней (D-82) сдвигается на него, иначе засев
	// после первого запуска оказался бы «до установки» и бонуса не дал бы.
	if req.BonusPending {
		if from, ok := s.autobonusFrom(); ok && req.Day < from {
			if err := s.cfg.Store.SetSetting(settingAutobonusFrom, req.Day); err != nil {
				s.cfg.Logf("тестовый день %s у %s: граница автобонуса не сдвинута: %v", req.Day, cs.child.Name, err)
				return ipc.Fail(textSaveFailed)
			}
		}
	}
	s.cfg.Logf("Тестовый день %s у %s: общее %d с, игры %d с, видео %d с, учёба %d с, доп. %d мин, ждёт бонуса %v, строк учёта %d",
		req.Day, cs.child.Name, req.TotalS, req.GamesS, req.VideoS, req.StudyS, req.ExtraMin, req.BonusPending, len(rows))
	return ipc.OK(ipc.Empty{})
}
