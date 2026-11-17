package service

import (
	"fmt"
	"time"
)

// Часовой пояс и безопасный режим (веха M6, R17б, R17е, FR-28, FR-30). Обычная учётка Windows
// может сменить часовой пояс: служба проверяет его раз в секунду (обслуживание) и пишет смену
// обходом ребёнку, сеанс которого на экране (не позже 2 с, S23); смена при родителе или без
// ребёнка на экране — только журнал работы. Реакция — только журнал (D-41).
//
// Службе пояс нужен только для этой записи: учёт дня идёт по поясу, который был при её запуске
// (time.Local кешируется при старте процесса).

// TimeZoneInfo — часовой пояс компьютера: ключ Windows («Russian Standard Time»), название для
// людей («(UTC+03:00) Москва, Санкт-Петербург»; пусто — ключ), стандартное смещение от UTC в
// минутах со знаком Windows (Bias: UTC = местное + смещение) и «летнее время выключено». Текущее
// смещение (ActiveTimeBias) не сравнивается: переход на летнее время — не смена пояса.
type TimeZoneInfo struct {
	Key     string
	Display string
	BiasMin int
	NoDST   bool
}

func (z TimeZoneInfo) same(o TimeZoneInfo) bool {
	return z.Key == o.Key && z.BiasMin == o.BiasMin && z.NoDST == o.NoDST
}

func (z TimeZoneInfo) name() string {
	if z.Display != "" {
		return z.Display
	}
	return z.Key
}

// tzState — последний прочитанный пояс.
type tzState struct {
	last  TimeZoneInfo
	known bool
}

// checkTZ читает пояс и сравнивает с прошлым чтением (Housekeep, под s.mu).
func (s *Service) checkTZ(real time.Time) {
	if s.cfg.TimeZone == nil {
		return
	}
	z, err := s.cfg.TimeZone()
	if err != nil {
		s.logRate("tz", "часовой пояс не прочитан: %v", err)
		return
	}
	if !s.tz.known {
		s.tz = tzState{last: z, known: true}
		return
	}
	if z.same(s.tz.last) {
		return
	}
	was := s.tz.last
	s.tz.last = z
	detail := fmt.Sprintf("Часовой пояс изменён: был «%s», стал «%s»", was.name(), z.name())
	if was.name() == z.name() {
		state := "включён"
		if z.NoDST {
			state = "выключен"
		}
		detail = fmt.Sprintf("Часовой пояс «%s»: переход на летнее время %s", z.name(), state)
	}
	if ss := s.activeChildSession(); ss != nil {
		s.cfg.Logf("часовой пояс изменён в сеансе ребёнка %d: %s", ss.id, detail)
		s.bypassIn(ss, detail, "tz_changed", real)
		return
	}
	s.cfg.Logf("%s; на экране не ребёнок: обходом не считается", detail)
}
