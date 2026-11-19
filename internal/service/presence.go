package service

import (
	"strconv"
	"time"

	"pcblock/internal/core"
)

// «За компьютером» для телефона (FR-43, D-99): когда ребёнок сегодня впервые сел за компьютер и
// сидит ли сейчас («За компьютером с 18:20» / «Не за компьютером с 19:05»). Считается в одном
// месте — тике учёта (notePresence); отчёт телефону только читает.

// settingPresenceFirst — ключ настройки (+ id ребёнка): UTC мс первого входа за день по часам
// ядра. Переживает перезапуск службы в пределах дня; новый день — новое значение.
const settingPresenceFirst = "presence.first."

// presence — «за компьютером» ребёнка. Под s.mu.
type presence struct {
	known   bool  // замерено хотя бы раз с запуска службы
	at      bool  // сейчас за компьютером
	sinceMs int64 // с какого момента at не менялось (после перезапуска службы — с первого замера)
	// firstDay, firstMs — первый вход за день firstDay; firstLoaded — значение прочитано из настроек.
	firstDay    string
	firstMs     int64
	firstLoaded bool
}

// childOnScreen — ребёнок за компьютером: свежий снимок стража из его сеанса, сеанс не отключён и
// не заблокирован. Те же условия, при которых sampleFor пускает секунду в учёт (FB-3, R2), без
// оценки программы переднего плана и простоя: за компьютером он и при экране «время вышло».
func (s *Service) childOnScreen(cs *childState) bool {
	if !cs.haveSample || s.real.Sub(cs.sampleAt) > sampleMaxAge {
		return false
	}
	ss := s.sessions[cs.sampleSess]
	return ss != nil && ss.childID == cs.child.ID && !ss.offScreen()
}

// notePresence — замер «за компьютером» на тике учёта в момент now (часы ядра): смена состояния
// двигает sinceMs; первый за день момент на экране записывается в настройки.
func (s *Service) notePresence(cs *childState, now time.Time) {
	on := s.childOnScreen(cs)
	p := &cs.presence
	ms := now.UnixMilli()
	if !p.known || p.at != on {
		p.known, p.at, p.sinceMs = true, on, ms
	}
	if !on {
		return
	}
	s.loadPresenceFirst(cs)
	if day := core.DayOf(now.In(s.cfg.Loc)); p.firstDay != day {
		p.firstDay, p.firstMs = day, ms
		if err := s.cfg.Store.SetSetting(settingPresenceFirst+strconv.FormatInt(cs.child.ID, 10), strconv.FormatInt(ms, 10)); err != nil {
			s.logRate("presence-save", "не удалось записать первый вход за день у %s: %v", cs.child.Name, err)
		}
	}
}

// loadPresenceFirst читает первый вход за день из настроек один раз за жизнь службы.
func (s *Service) loadPresenceFirst(cs *childState) {
	p := &cs.presence
	if p.firstLoaded {
		return
	}
	p.firstLoaded = true
	v, ok, err := s.cfg.Store.GetSetting(settingPresenceFirst + strconv.FormatInt(cs.child.ID, 10))
	if err != nil {
		s.logRate("presence-load", "не удалось прочитать первый вход за день у %s: %v", cs.child.Name, err)
		return
	}
	if !ok {
		return
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
		p.firstDay, p.firstMs = core.DayOf(time.UnixMilli(ms).In(s.cfg.Loc)), ms
	}
}

// presenceToday — для отчёта: первый вход сегодня (0 — сегодня не садился), сейчас ли за
// компьютером и с какого момента. Ни одного замера с запуска — «не за компьютером» с этого момента.
func (s *Service) presenceToday(cs *childState) (firstMs int64, at bool, sinceMs int64) {
	s.loadPresenceFirst(cs)
	now := s.cfg.Clock.Now().In(s.cfg.Loc)
	p := &cs.presence
	if !p.known {
		p.known, p.at, p.sinceMs = true, false, now.UnixMilli()
	}
	if p.firstDay == core.DayOf(now) {
		firstMs = p.firstMs
	}
	return firstMs, p.at, p.sinceMs
}
