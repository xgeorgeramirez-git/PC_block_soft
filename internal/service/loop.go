package service

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"pcblock/internal/core"
)

// evInternalError — вид события журнала о сбое внутри программы: паника при обработке одного
// ребёнка или одного дела обслуживания. Текст для родителя без техники (NFR-11).
const evInternalError = "internal_error"

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
		s.guard("tick", cs, func() { s.tickChild(cs, now) })
	}
}

// tickChild — тик одного ребёнка: сохранение по DueSave до тика (эталон G10), тик ядра, разбор
// решения. Вызывается под s.mu и только из guard.
func (s *Service) tickChild(cs *childState, now time.Time) {
	if cs.admin {
		return
	}
	if cs.eng.DueSave(now) {
		s.saveDay(cs)
	}
	d := cs.eng.Tick(s.sampleFor(cs))
	s.afterDecision(cs, d, nil)
}

// guard выполняет единицу работы под s.mu так, чтобы паника в ней (журнал, датчик, вызов
// Windows) не роняла процесс службы и не мешала остальным детям и каналу связи: сбой
// записывается, цикл идёт дальше (R18). Так же канал связи защищён в Handle.
func (s *Service) guard(where string, cs *childState, f func()) {
	defer func() {
		if p := recover(); p != nil {
			s.internalError(where, cs, p)
		}
	}()
	f()
}

// internalError записывает сбой, пойманный guard: в журнал работы со стеком и событием в журнал
// родителя (R22). Не чаще раза в errLogEvery на место и ребёнка: сбой, повторяющийся каждую
// секунду, не должен засыпать журналы. Сам не паникует: причиной сбоя мог быть как раз журнал.
func (s *Service) internalError(where string, cs *childState, p any) {
	key := "panic|" + where
	text := textInternalFailure
	if cs != nil {
		key += fmt.Sprint("|", cs.child.ID)
		text = fmt.Sprintf(textInternalFailureOf, cs.child.Name)
	}
	if last, ok := s.errAt[key]; ok && s.real.Sub(last) < errLogEvery {
		return
	}
	s.errAt[key] = s.real
	func() {
		defer func() { _ = recover() }()
		s.cfg.Logf("Внутренняя ошибка (%s): %v\n%s", where, p, debug.Stack())
	}()
	func() {
		defer func() { _ = recover() }()
		s.journal.Write(0, evInternalError, map[string]any{"text": text})
	}()
}

// runGuarded выполняет шаг цикла вне s.mu: паника в нём (например, в часах) не убивает
// горутину RunLoop. Блокировку Tick и Housekeep к этому моменту уже отпустили.
func (s *Service) runGuarded(where string, f func()) {
	defer func() {
		if p := recover(); p != nil {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.internalError(where, nil, p)
		}
	}()
	f()
}

// sampleFor возвращает снимок датчиков для тика: свежий снимок стража, иначе «неактивен».
// Служба ничего не додумывает за стража: нет вестей дольше sampleMaxAge, секунда не идёт.
// Но и стражу она не верит там, где знает больше него (FB-3, R2): страж в отключённом сеансе
// не может открыть рабочий стол ввода и считает сеанс незаблокированным, а после выхода из
// учётки его последний снимок ещё живёт до sampleMaxAge. Сеанса нет, в нём другой пользователь,
// он отключён или заблокирован: секунда не идёт.
func (s *Service) sampleFor(cs *childState) core.SessionSample {
	if !cs.haveSample || s.real.Sub(cs.sampleAt) > sampleMaxAge {
		return core.SessionSample{}
	}
	if ss := s.sessions[cs.sampleSess]; ss == nil || ss.childID != cs.child.ID || ss.offScreen() {
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
	s.guard("load", nil, func() {
		if !s.loaded {
			s.loadChildren(real)
		}
	})
	for _, id := range s.childOrder() {
		cs := s.children[id]
		s.guard("housekeep", cs, func() { s.housekeepChild(cs, real) })
	}
	s.guard("parents", nil, func() { s.expireParents(real) })
	s.guard("binds", nil, func() { s.expireBinds(real) })
	s.guard("sessions", nil, func() { s.syncSessions(real) })
}

// housekeepChild — обслуживание одного ребёнка: проверка R1, повторное чтение правил,
// устаревшие предупреждения. Вызывается под s.mu и только из guard.
func (s *Service) housekeepChild(cs *childState, real time.Time) {
	if real.Sub(cs.adminAt) >= adminCheckEvery {
		s.checkAdmin(cs, real)
	}
	if cs.rules.Broken && real.Sub(cs.rulesAt) >= rulesRetryEvery {
		s.retryRules(cs, real)
	}
	cs.expireWarnings(real)
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
			s.runGuarded("tick", func() { s.Tick(s.cfg.Clock.Now()) })
		case <-house.C:
			s.runGuarded("housekeep", func() { s.Housekeep(s.cfg.RealNow()) })
		}
	}
}
