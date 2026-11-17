package service

import (
	"fmt"
	"strconv"
	"time"

	"pcblock/internal/core"
)

// Перевод часов компьютера (веха M6, R17а, FR-28, D-87). Обычная учётка менять время не может,
// поэтому опасны два случая: часы перевели при выключенном компьютере (BIOS) и часы перевёл
// ребёнок, у которого права всё же нашлись. Реакция — только журнал (D-41).
//
// Во время работы служба раз в секунду сравнивает, на сколько сдвинулись настоящие часы
// (RealNow) и её внутренний счётчик (Config.Mono). Счётчик часам не подчиняется: расхождение
// больше clockJumpLimit — часы перевели. Сон и гибернация сдвигают обоих одинаково (Go на Windows
// считает счётчик по времени прерываний, сон в нём есть), поправки службы времени Windows меньше
// порога. При включении служба сверяет часы с границей settingClockLastSeen — последним моментом,
// когда она работала (пишется раз в минуту и при остановке): часы позади границы больше чем на
// clockJumpLimit — их перевели назад, пока компьютер был выключен. Часы впереди — компьютер был
// выключен, записи нет (D-87).

const (
	clockJumpLimit       = 120 * time.Second // перевод больше этого — обход (строго больше)
	clockSaveEvery       = time.Minute       // как часто пишется граница settingClockLastSeen
	settingClockLastSeen = "clock.last_seen_ms"
)

// clockState — что служба помнит о часах между проходами обслуживания.
type clockState struct {
	wall  time.Time     // настоящее время прошлого прохода
	mono  time.Duration // счётчик прошлого прохода
	known bool          // прошлый проход был
	saved time.Time     // когда граница записана в последний раз
}

// monoClock — счётчик процесса по умолчанию: монотонная часть time.Now не зависит от перевода часов.
func monoClock() func() time.Duration {
	start := time.Now()
	return func() time.Duration { return time.Since(start) }
}

// checkBootClock — сверка при включении (Start, под s.mu): часы позади границы больше чем на
// clockJumpLimit — запись обхода без ребёнка (кто перевёл, неизвестно). Затем граница = real.
func (s *Service) checkBootClock(real time.Time) {
	if v, ok, err := s.cfg.Store.GetSetting(settingClockLastSeen); err != nil {
		s.cfg.Logf("граница часов не прочитана: %v", err)
	} else if ok {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			last := time.UnixMilli(ms)
			if last.Sub(real) > clockJumpLimit {
				s.cfg.Logf("часы компьютера ушли назад: служба работала до %s, сейчас %s", last.Format(time.RFC3339), real.Format(time.RFC3339))
				s.journal.Write(0, core.EvBypass, map[string]any{"detail": bootBackDetail(last.In(s.cfg.Loc), real.In(s.cfg.Loc)), "subtype": "clock_back_boot"})
			}
		}
	}
	s.saveClock(real)
	s.clock = clockState{wall: real.Round(0), mono: s.cfg.Mono(), known: true, saved: real}
}

// checkClock — сверка часов со счётчиком (Housekeep, под s.mu).
func (s *Service) checkClock() {
	// Стена и счётчик — одним мгновением (PL-15): real снят до обслуживания, и сон между ним и
	// счётчиком дал бы ложный скачок. Стена через Round(0): у time.Now есть монотонная часть, и Sub
	// считал бы по ней (PL-71).
	wall := s.cfg.RealNow().Round(0)
	mono := s.cfg.Mono()
	c := &s.clock
	if c.known {
		ran := mono - c.mono
		drift := wall.Sub(c.wall) - ran
		if drift > clockJumpLimit || drift < -clockJumpLimit {
			s.onClockJump(c.wall.Add(ran), wall)
			c.saved = time.Time{} // граница сразу по новым часам
		}
	}
	c.wall, c.mono, c.known = wall, mono, true
	if c.saved.IsZero() || wall.Sub(c.saved) >= clockSaveEvery || wall.Before(c.saved) {
		s.saveClock(wall)
		c.saved = wall
	}
}

// onClockJump — часы перевели с expected на real: обход у ребёнка на экране. Назад, когда на
// экране нет ни одного сеанса (выход из гибернации или быстрого запуска: служба не
// перезапускалась, Start не было), — запись как при включении. Иначе журнал работы.
func (s *Service) onClockJump(expected, real time.Time) {
	detail := clockChangedDetail(expected.In(s.cfg.Loc), real.In(s.cfg.Loc))
	// Сессия родителя — по часам до перевода: перевод вперёд сам «состарил» бы её.
	if ss := s.activeChildSession(expected); ss != nil {
		s.cfg.Logf("часы компьютера переведены в сеансе ребёнка %d: %s", ss.id, detail)
		s.bypassIn(ss, detail, "clock_changed", real)
		return
	}
	if expected.Sub(real) > clockJumpLimit && !s.anySessionOnScreen() {
		s.cfg.Logf("%s; на экране никого: как при включении", detail)
		s.journal.Write(0, core.EvBypass, map[string]any{"detail": resumeBackDetail(expected.In(s.cfg.Loc), real.In(s.cfg.Loc)), "subtype": "clock_back_boot"})
		return
	}
	s.cfg.Logf("%s; на экране не ребёнок: обходом не считается", detail)
}

// anySessionOnScreen — на экране чей-то сеанс (ребёнка или взрослого), не отключённый и не
// заблокированный.
func (s *Service) anySessionOnScreen() bool {
	for _, ss := range s.sessions {
		if !ss.offScreen() {
			return true
		}
	}
	return false
}

// saveClock пишет границу settingClockLastSeen.
func (s *Service) saveClock(real time.Time) {
	if err := s.cfg.Store.SetSetting(settingClockLastSeen, strconv.FormatInt(real.UnixMilli(), 10)); err != nil {
		s.logRate("clock-save", "граница часов не записана: %v", err)
	}
}

// activeChildSession — сеанс ребёнка на экране: не отключён, не заблокирован и без сессии родителя
// PC Block в нём на момент at; nil — на экране не ребёнок. Сеансов на экране один (консоль); при нескольких — с меньшим номером.
func (s *Service) activeChildSession(at time.Time) *sessState {
	var best *sessState
	for _, ss := range s.sessions {
		if ss.childID == 0 || ss.offScreen() || s.parentOpenIn(ss.id, at) {
			continue // за экраном ребёнка родитель вошёл в PC Block: перевод его (M6 ревью)
		}
		if best == nil || ss.id < best.id {
			best = ss
		}
	}
	return best
}

// clockChangedDetail — «Часы компьютера переведены: было 19.10 15:00, стало 19.10 15:10».
func clockChangedDetail(was, now time.Time) string {
	return fmt.Sprintf("Часы компьютера переведены: было %s, стало %s", was.Format("02.01 15:04"), now.Format("02.01 15:04"))
}

// resumeBackDetail — «Часы компьютера ушли назад, пока компьютер спал или был выключен: было
// 19.10 16:00, стало 19.10 15:00».
func resumeBackDetail(was, now time.Time) string {
	return fmt.Sprintf("Часы компьютера ушли назад, пока компьютер спал или был выключен: было %s, стало %s", was.Format("02.01 15:04"), now.Format("02.01 15:04"))
}

// bootBackDetail — «Часы компьютера ушли назад: служба последний раз работала 19.10 в 16:00,
// после включения на часах 19.10 в 15:00».
func bootBackDetail(last, now time.Time) string {
	return fmt.Sprintf("Часы компьютера ушли назад: служба последний раз работала %s в %s, после включения на часах %s в %s",
		last.Format("02.01"), last.Format("15:04"), now.Format("02.01"), now.Format("15:04"))
}
