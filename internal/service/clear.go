package service

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Очистка статистики и журнала ребёнка (FR-38, D-73, D-79; RM5-8, RM5-9). Учёт лимита (ядро,
// остаток, доп. время, копилка, флаги дня) очистка не трогает.

// hClearStats — «Очистить статистику» (RM5-8): одной транзакцией хранилища весь учёт по программам
// и сайтам ребёнка, прошлые дни до дня ядра, строка дня ядра текущими счётчиками ядра со всеми
// флагами (база отстаёт от ядра до 30 с) и снимок счётчиков; несохранённый накопитель — время до
// очистки, обнуляется. Момент — по часам ядра (как журнал), день — день ядра (до полуночи ядра
// может быть ещё вчерашним, PL-31). Ошибка — ничего не изменилось, накопитель на месте.
func (s *Service) hClearStats(c *callCtx) ipc.Reply {
	var req ipc.ClearChildRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	// Прошлые дни, ждущие автобонуса (D-82), сначала начисляются: очистка удаляет только
	// рассмотренные дни. В безопасном режиме catchUpBonus ничего не делает, строки остаются.
	s.catchUpBonus(cs)
	day, _ := cs.eng.State()
	snap := day
	if day.Day == "" {
		// Ядро ещё не начало день: снимок нулевой, строка сегодняшнего дня — как в базе.
		today := s.today()
		loaded, err := s.cfg.Store.LoadDayState(cs.child.ID, today)
		if err != nil {
			s.logRate(fmt.Sprint("clearstats", cs.child.ID), "очистка статистики у %s: не прочитан день: %v", cs.child.Name, err)
			return ipc.Fail(textSaveFailed)
		}
		day, snap = loaded, core.DayState{}
		day.Day = today
	}
	r := StatsReset{
		ClearedAtMs: s.cfg.Clock.Now().UnixMilli(), Day: day.Day,
		TotalS: snap.TotalS, GamesS: snap.GamesS, VideoS: snap.VideoS, StudyS: snap.StudyS, ExtraMin: snap.ExtraMin,
	}
	if err := s.cfg.Store.ClearStats(cs.child.ID, day, r); err != nil {
		s.logRate(fmt.Sprint("clearstats", cs.child.ID), "очистка статистики у %s не удалась: %v", cs.child.Name, err)
		return ipc.Fail(textSaveFailed)
	}
	cs.usage = nil
	s.journal.Write(cs.child.ID, evStatsCleared, map[string]any{"text": "Статистика очищена"})
	return ipc.OK(ipc.ClearReply{Message: "Статистика " + genitiveName(s.names[cs.child.ID]) + " очищена"})
}

// hClearJournal — «Очистить журнал» (RM5-9): события ребёнка удаляются, события без ребёнка
// (пароль, промокоды, работа программы) остаются; затем ребёнку пишется «Журнал очищен».
// Флаг обхода и блокировка — не журнал, не трогаются.
func (s *Service) hClearJournal(c *callCtx) ipc.Reply {
	var req ipc.ClearChildRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if _, err := s.cfg.Store.DeleteChildEvents(cs.child.ID); err != nil {
		s.logRate(fmt.Sprint("clearjournal", cs.child.ID), "очистка журнала у %s не удалась: %v", cs.child.Name, err)
		return ipc.Fail(textSaveFailed)
	}
	s.journal.Write(cs.child.ID, evJournalCleared, map[string]any{"text": "Журнал очищен"})
	return ipc.OK(ipc.ClearReply{Message: "Журнал " + genitiveName(s.names[cs.child.ID]) + " очищен"})
}

// genitiveName — имя в родительном падеже для «Статистика Миши очищена»: те же правила, что
// genitiveName окна (frontend/src/lib/format.ts); составные и нерусские имена — как есть.
func genitiveName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return n
	}
	for _, r := range n {
		if !(r >= 'А' && r <= 'я' || r == 'Ё' || r == 'ё' || r == '-') {
			return n
		}
	}
	last, size := utf8.DecodeLastRuneInString(n)
	stem := n[:len(n)-size]
	prev, _ := utf8.DecodeLastRuneInString(stem)
	switch unicode.ToLower(last) {
	case 'а':
		if strings.ContainsRune("гкхжчшщ", unicode.ToLower(prev)) {
			return stem + "и"
		}
		return stem + "ы"
	case 'я':
		return stem + "и"
	case 'й', 'ь':
		return stem + "я"
	case 'е', 'и', 'о', 'у', 'ы', 'ю', 'э':
		return n
	}
	return n + "а"
}
