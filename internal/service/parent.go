package service

import (
	"time"

	"pcblock/internal/auth"
	"pcblock/internal/ipc"
)

// parentSession — родительская сессия (R19): вход по паролю, привязан к процессу окна.
type parentSession struct {
	token      string
	pid        uint32
	stamp      uint64 // момент создания процесса: PID сам по себе может повториться
	stampOK    bool
	sid        string
	sessionID  uint32
	last       time.Time // последнее действие (чтение сессию не продлевает)
	sawNotices bool      // окну показали важные сообщения (после конца сессии они гаснут)
}

// guardBind — «страж только что провёл верную проверку пароля»: окно в 30 с и одно действие.
// Страж пароль не хранит и токена не получает (модель стража, эскалация T10a), поэтому
// «+15», «Снять блокировку» и «Открыть настройки» с экрана блокировки служба принимает
// от него только внутри этого окна.
type guardBind struct {
	until time.Time
}

// checkPassword проверяет пароль родителя с паузами R19 (по настоящим часам). ok = true:
// пароль верный. Иначе возвращается готовый отказ. who и childID идут в журнал при неверном
// вводе (R22); childID 0: ввод не из сеанса ребёнка.
func (s *Service) checkPassword(pw string, real time.Time, childID int64) (bool, ipc.Reply) {
	a, ok, err := s.cfg.Store.GetAuth()
	if err != nil {
		s.logRate("auth", "не удалось прочитать пароль родителя: %v", err)
		return false, ipc.Fail(textRetry)
	}
	if !ok {
		return false, ipc.FailField(textNotConfigured, ipc.FieldPassword)
	}
	nowMs := real.UnixMilli()
	if blocked, until := a.Attempts.Blocked(nowMs); blocked {
		sec := secondsLeft(until, nowMs)
		return false, ipc.FailWith(pauseText(sec), ipc.FieldPassword, ipc.LoginDenied{PauseS: sec})
	}
	if pw == "" {
		return false, ipc.FailField(textEnterPassword, ipc.FieldPassword)
	}
	match, err := s.cfg.Hasher.Verify(a.Hash, pw)
	if err != nil {
		s.logRate("verify", "пароль родителя не проверен, хеш не читается: %v", err)
		return false, ipc.FailField(textHashBroken, ipc.FieldPassword)
	}
	if match {
		if a.Attempts != (auth.Attempts{}) {
			a.Attempts.Reset()
			if err := s.cfg.Store.SetAuth(a); err != nil {
				s.logRate("setauth", "не сброшен счётчик неверных паролей: %v", err)
			}
		}
		return true, ipc.Reply{}
	}
	until := a.Attempts.Fail(nowMs)
	if err := s.cfg.Store.SetAuth(a); err != nil {
		s.logRate("setauth", "не записан неверный ввод пароля: %v", err)
	}
	s.journal.Write(childID, "password_failed", map[string]any{"text": "Неверный пароль родителя"})
	if until > 0 {
		sec := secondsLeft(until, nowMs)
		return false, ipc.FailWith(pauseText(sec), ipc.FieldPassword, ipc.LoginDenied{PauseS: sec})
	}
	left := auth.FailsPerSeries - a.Attempts.Failed
	return false, ipc.FailWith(wrongPasswordText(left), ipc.FieldPassword, ipc.LoginDenied{AttemptsLeft: left})
}

// secondsLeft возвращает секунды до момента untilMs, округляя вверх.
func secondsLeft(untilMs, nowMs int64) int {
	d := untilMs - nowMs
	if d <= 0 {
		return 0
	}
	return int((d + 999) / 1000)
}

// newParent открывает родительскую сессию для процесса peer и возвращает её.
func (s *Service) newParent(peer ipc.Peer, real time.Time) *parentSession {
	tok := s.cfg.Rand()
	if tok == "" {
		return nil
	}
	stamp, ok := s.cfg.Launcher.Stamp(peer.PID)
	// У одного окна одна сессия: повторный вход заменяет прежнюю.
	for t, p := range s.parents {
		if p.pid == peer.PID && p.stamp == stamp && p.stampOK == ok {
			delete(s.parents, t)
		}
	}
	ps := &parentSession{
		token: tok, pid: peer.PID, stamp: stamp, stampOK: ok,
		sid: peer.SID, sessionID: peer.SessionID, last: real,
	}
	s.parents[tok] = ps
	return ps
}

// parentFor возвращает действующую сессию для токена и процесса или nil: токен чужого
// процесса, неизвестный и просроченный одинаково не подходят. touch = true: действие
// продлевает сессию (правки и ExtendSession), чтение её не продлевает.
func (s *Service) parentFor(peer ipc.Peer, token string, real time.Time, touch bool) *parentSession {
	if token == "" {
		return nil
	}
	ps := s.parents[token]
	if ps == nil {
		return nil
	}
	if real.Sub(ps.last) >= parentIdle {
		s.endParent(ps)
		return nil
	}
	if ps.pid != peer.PID || ps.sessionID != peer.SessionID || ps.sid != peer.SID {
		return nil
	}
	if stamp, ok := s.cfg.Launcher.Stamp(peer.PID); ok != ps.stampOK || stamp != ps.stamp {
		return nil
	}
	if touch {
		ps.last = real
	}
	return ps
}

// endParent заканчивает сессию. Важные сообщения, которые окно успело показать, гаснут.
func (s *Service) endParent(ps *parentSession) {
	delete(s.parents, ps.token)
	if ps.sawNotices {
		for _, other := range s.parents {
			if other.sawNotices {
				return
			}
		}
		if err := s.cfg.Store.SetSetting(settingDBNotice, ""); err != nil {
			s.logRate("dbnotice", "не снято предупреждение о базе: %v", err)
		}
	}
}

// endParentsOf заканчивает сессии, привязанные к процессу pid (окно закрыто или завершилось).
func (s *Service) endParentsOf(pid uint32) {
	for _, ps := range s.parents {
		if ps.pid == pid {
			s.endParent(ps)
		}
	}
}

// expireParents заканчивает сессии, у которых вышел срок бездействия или исчез процесс.
func (s *Service) expireParents(real time.Time) {
	for _, ps := range s.parents {
		switch {
		case real.Sub(ps.last) >= parentIdle:
			s.endParent(ps)
		case ps.stampOK:
			if stamp, ok := s.cfg.Launcher.Stamp(ps.pid); !ok || stamp != ps.stamp {
				s.endParent(ps)
			}
		}
	}
}

// parentRemaining возвращает состояние сессии для ответов ExtendSession и SessionState.
func (s *Service) parentState(ps *parentSession, real time.Time) ipc.SessionStateReply {
	if ps == nil {
		return ipc.SessionStateReply{}
	}
	expires := ps.last.Add(parentIdle)
	rem := int(expires.Sub(real) / time.Second)
	if rem < 0 {
		rem = 0
	}
	return ipc.SessionStateReply{Active: true, ExpiresMs: expires.UnixMilli(), RemainingS: rem}
}

// parentOpenIn сообщает, что в сеансе Windows открыта родительская сессия: пока она идёт,
// страж этого сеанса прячет экран блокировки (Ruling 19).
func (s *Service) parentOpenIn(session uint32, real time.Time) bool {
	for _, ps := range s.parents {
		if ps.sessionID == session && real.Sub(ps.last) < parentIdle {
			return true
		}
	}
	return false
}

// --- привязка стража -----------------------------------------------------------------

// bindGuard запоминает, что страж сеанса только что провёл верную проверку пароля.
func (s *Service) bindGuard(session uint32, real time.Time) time.Time {
	until := real.Add(guardBindLife)
	s.binds[session] = &guardBind{until: until}
	return until
}

// guardBound сообщает, что у стража сеанса есть непотраченная привязка.
func (s *Service) guardBound(session uint32, real time.Time) bool {
	b := s.binds[session]
	return b != nil && real.Before(b.until)
}

// spendBind тратит привязку: одно действие на один ввод пароля.
func (s *Service) spendBind(session uint32) { delete(s.binds, session) }

func (s *Service) expireBinds(real time.Time) {
	for id, b := range s.binds {
		if !real.Before(b.until) {
			delete(s.binds, id)
		}
	}
}
