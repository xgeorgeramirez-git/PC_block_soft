package service

import (
	"context"
	"time"

	"pcblock/internal/core"
)

// Tick — один тик учёта по часам ядра, now: их текущий момент. Для каждого ребёнка сначала
// сохранение по DueSave (снимок отражает учёт ровно до now, эталон G10), затем тик ядра со
// снимком датчиков стража. Ребёнок-администратор (R1) не учитывается. Остановленные часы
// тестового режима тик пропускает: иначе каждый тик засчитывал бы секунду на месте.
func (s *Service) Tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping || s.clockPaused {
		return
	}
	// У тика есть только часы ядра, а возраст снимка стража считается по настоящим часам.
	s.real = s.cfg.RealNow()
	for _, id := range s.childOrder() {
		cs := s.children[id]
		if cs.admin {
			continue
		}
		if cs.eng.DueSave(now) {
			s.saveDay(cs)
		}
		d := cs.eng.Tick(s.sampleFor(cs))
		s.afterDecision(cs, d, nil)
	}
}

// sampleFor возвращает снимок датчиков для тика: свежий снимок стража, иначе «неактивен».
// Служба ничего не додумывает за стража: нет вестей дольше sampleMaxAge, секунда не идёт.
func (s *Service) sampleFor(cs *childState) core.SessionSample {
	if !cs.haveSample || s.real.Sub(cs.sampleAt) > sampleMaxAge {
		return core.SessionSample{}
	}
	return cs.sample.ToCore()
}

// Housekeep — обслуживание раз в настоящую секунду: сеансы и процессы, проверка R1 раз в
// 60 с, повторное чтение правил, окончание родительских сессий.
func (s *Service) Housekeep(real time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.real = real
	if s.stopping {
		return
	}
	if !s.loaded {
		s.loadChildren(real)
	}
	for _, id := range s.childOrder() {
		cs := s.children[id]
		if real.Sub(cs.adminAt) >= adminCheckEvery {
			s.checkAdmin(cs, real)
		}
		if cs.rules.Broken && real.Sub(cs.rulesAt) >= rulesRetryEvery {
			s.retryRules(cs, real)
		}
		cs.expireWarnings(real)
	}
	s.expireParents(real)
	s.expireBinds(real)
	s.syncSessions(real)
}

// retryRules повторно читает правила, которые не удалось прочитать (R18).
func (s *Service) retryRules(cs *childState, real time.Time) {
	cs.rulesAt = real
	r := s.readRules(cs.child.ID)
	if r.Broken {
		return
	}
	s.applyRules(cs, r)
}

// applyRules ставит правила ядру сразу (FR-19) и разбирает решение.
func (s *Service) applyRules(cs *childState, r core.Rules) {
	cs.rules = r
	d := cs.eng.SetRules(r)
	s.afterDecision(cs, d, nil)
}

// expireWarnings выбрасывает предупреждения, которые никто не забрал: показывать на экране
// «через 5 минут» спустя минуту поздно.
func (cs *childState) expireWarnings(real time.Time) {
	keep := cs.warns[:0]
	for _, w := range cs.warns {
		if real.Sub(w.at) <= warnLife {
			keep = append(keep, w)
		}
	}
	cs.warns = keep
}

// TickInterval возвращает настоящий период тика: секунда часов ядра (Ruling 4).
func (s *Service) TickInterval() time.Duration { return s.cfg.Test.TickInterval() }

// RunLoop крутит тики и обслуживание, пока не отменён ctx. Тик идёт раз в секунду часов ядра, обслуживание раз в настоящую секунду.
func (s *Service) RunLoop(ctx context.Context) {
	tick := time.NewTicker(s.TickInterval())
	house := time.NewTicker(time.Second)
	defer tick.Stop()
	defer house.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.Tick(s.cfg.Clock.Now())
		case <-house.C:
			s.Housekeep(s.cfg.RealNow())
		}
	}
}
