package service

import (
	"fmt"
	"sort"
	"time"

	"pcblock/internal/ipc"
)

// Окно при входе в Windows (FR-46, D-108, S63): при входе в учётку служба сама открывает окно
// программы — ребёнку «Моя статистика» (первый вход после установки — сначала окно FR-32, после
// «Понятно» статистика), другой учётке — окно родителя без сессии (пароль спросит окно).
// Только по событию входа, пришедшему после старта службы (R-13): перезапуск службы и
// обновление программы окон в уже открытых сеансах не открывают. Разблокировка и возврат через
// «Смену пользователя» — не вход.

// logonWindowDeadline — окно при входе появляется не позже 10 с (FR-46, NFR-6); не успели — не открываем.
const logonWindowDeadline = 10 * time.Second

// noteLogon запоминает вход в сеанс id (под s.mu, только после s.started: onSession событий до
// старта не пропускает). Сеанс, где вход уже разобран (окно открыто или главнее экран
// блокировки), второго окна не получает: дубль события, тестовое событие поверх настоящего.
// Повтор события до открытия окна срок не сдвигает.
func (s *Service) noteLogon(id uint32, real time.Time) {
	if id == SessionUnknown {
		return
	}
	if ss := s.sessions[id]; ss != nil && ss.logonDone {
		return
	}
	if _, ok := s.logonDue[id]; ok {
		return
	}
	if s.logonDue == nil {
		s.logonDue = map[uint32]time.Time{}
	}
	s.logonDue[id] = real
	s.vlogf("вход в сеанс %d: ждёт окно при входе", id)
}

// forgetLogon сбрасывает вход сеанса: выход из учётки, сеанс исчез или в нём другой пользователь.
func (s *Service) forgetLogon(id uint32) {
	delete(s.logonDue, id)
	if ss := s.sessions[id]; ss != nil {
		ss.logonDone, ss.logonStats = false, false
	}
}

// openLogonWindows открывает окна при входе в сеансах из logonDue: каждый тик обслуживания и
// сразу после события входа. Сеанс, ещё не известный службе (событие пришло раньше списка
// Windows), ждёт до logonWindowDeadline; не появился или окно так и не запустилось — строка в
// журнал работы, окна нет.
func (s *Service) openLogonWindows(real time.Time) {
	if !s.started || s.stopping || len(s.logonDue) == 0 {
		return
	}
	ids := make([]uint32, 0, len(s.logonDue))
	for id := range s.logonDue {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		ss := s.sessions[id]
		if real.Sub(s.logonDue[id]) > logonWindowDeadline {
			// Позже 10 с окно уже не «при входе»: ребёнок занят своим, окно было бы помехой.
			delete(s.logonDue, id)
			if ss != nil {
				ss.logonDone = true
			}
			s.cfg.Logf("Окно при входе в сеанс %d не открыто: за %d с сеанс не найден или окно не запустилось", id, int(logonWindowDeadline/time.Second))
			continue
		}
		if ss == nil || !s.loaded {
			continue // ждём сеанс в списке Windows или список детей
		}
		done, err := s.openLogonWindow(ss, real)
		if err != nil {
			s.logRate(fmt.Sprint("logonwin", id), "окно при входе в сеанс %d не запустилось: %v", id, err)
		}
		if done {
			delete(s.logonDue, id)
			ss.logonDone = true
		}
	}
}

// openLogonWindow решает и открывает окно при входе в сеанс ss. done — вход разобран (окно
// открыто или уже было открыто, или окна не будет); false с ошибкой — повторить на следующем тике.
func (s *Service) openLogonWindow(ss *sessState, real time.Time) (done bool, err error) {
	if ss.childID == 0 {
		if !s.configured() {
			// Пароля нет: окно родителя открыло бы первую настройку в любой учётке (в том числе
			// детской, ещё не отмеченной). Её открывает установщик, не вход.
			s.vlogf("сеанс %d: программа не настроена, окно при входе не открывается", ss.id)
			return true, nil
		}
		if _, err := s.openWindow(ss, ipc.WindowParent, "", false, real); err != nil {
			return false, err
		}
		s.vlogf("сеанс %d: при входе открыто окно родителя", ss.id)
		return true, nil
	}
	cs := s.children[ss.childID]
	if cs == nil {
		return false, nil
	}
	if lock := s.statusOf(cs).Lock; lock != ipc.LockNone {
		// Экран блокировки главнее окна (FR-46): время вышло, запретные часы, перерыв.
		s.vlogf("сеанс %d: при входе экран блокировки (%s), окно не открывается", ss.id, lock)
		return true, nil
	}
	kind := ipc.WindowMyStats
	if !s.firstRunSeen(cs) {
		kind = ipc.WindowFirstRun // FR-32: после «Понятно» откроется статистика (hFirstRunSeen)
	}
	if _, err := s.openWindow(ss, kind, "", false, real); err != nil {
		return false, err
	}
	ss.logonStats = kind == ipc.WindowFirstRun
	s.vlogf("сеанс %d: при входе открыто окно %s", ss.id, kind)
	return true, nil
}

// logonStatsAfterFirstRun открывает «Моя статистика» после «Понятно» в окне первого входа,
// открытом при входе (FR-46, FR-32). Один раз: флаг сбрасывается.
func (s *Service) logonStatsAfterFirstRun(peer ipc.Peer, real time.Time) {
	ss := s.sessions[peer.SessionID]
	if ss == nil || !ss.logonStats || ss.sid != peer.SID {
		return
	}
	ss.logonStats = false
	if _, err := s.openWindow(ss, ipc.WindowMyStats, "", false, real); err != nil {
		s.logRate(fmt.Sprint("logonstats", ss.id), "окно «Моя статистика» после первого входа не открыто: %v", err)
	}
}
