package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
)

// Роли процессов в памяти о запущенном (procs.json) и ключи окон.
const (
	roleGuard = "guard"
	roleTray  = "tray"
)

// supProc — процесс, запущенный службой и находящийся под её наблюдением (FR-27, R17в).
type supProc struct {
	kind      ProcKind
	role      string // roleGuard, roleTray или вид окна (ipc.Window*)
	proc      Proc   // nil: процесса сейчас нет (ждёт перезапуска)
	pid       uint32
	stamp     uint64
	startedAt time.Time
	expected  bool      // закрытие сообщено интерфейсом (TrayClosing, WindowClosing) или службой (конец сеанса): это не обход
	closingAt time.Time // когда страж сообщил GuardClosing; действует closingLife, обход не стирает, а откладывает
	nextTry   time.Time // не запускать заново раньше этого момента
	failures  int
}

// pendingBypass — гибель стража, значка или окна в сеансе ребёнка, о которой служба ещё не решила,
// обход это или конец сеанса (R17, FR-27). Перезапуск идёт сразу, а запись в журнал ждёт
// bypassGrace: за это время приходит событие конца сеанса или остановка службы, и записи нет.
type pendingBypass struct {
	sessionID uint32
	sid       string        // пользователь сеанса в момент гибели
	detail    string        // подробность для журнала
	subtype   string        // подтип события: guard_closed, tray_closed, window_closed
	at        time.Time     // настоящий момент, когда служба заметила гибель
	grace     time.Duration // срок решения; 0: bypassGrace. Гибель стража с GuardClosing ждёт closingLife
}

// wait возвращает срок решения по этому ожиданию.
func (p pendingBypass) wait() time.Duration {
	if p.grace > 0 {
		return p.grace
	}
	return bypassGrace
}

// sessState — интерактивный сеанс Windows и запущенные в нём части программы.
type sessState struct {
	id       uint32
	user     string
	sid      string
	childID  int64       // 0: в сеансе работает не ребёнок
	screen   ScreenState // на экране ли сеанс: по списку Windows и событиям подключения (FB-3)
	listWTS  uint32      // последнее состояние WTS из списка (для журнала)
	listSeen ScreenState // и вывод из него
	offHold  time.Time   // до этого момента список Windows не возвращает «на экране» поверх события отключения
	locked   bool        // сеанс заблокирован: по событиям блокировки и разблокировки; сбрасывается снимком стража (FB-3)
	guard    *supProc
	tray     *supProc
	windows  map[string]*supProc
}

// OnSession принимает событие сеанса от диспетчера служб: служба сверяет сеансы и запускает
// недостающие части. Вход ребёнка дополнительно запускает проверку R1 и ставит политику
// Диспетчера задач (в syncSessions, при появлении сеанса ребёнка).
func (s *Service) OnSession(ev SessionEvent, id uint32, real time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.real = real
	if s.stopping || !s.started {
		return
	}
	s.vlogf("событие сеанса %d: %v", id, ev)
	switch ev {
	case SessionLogoff:
		// Штатный выход из учётки завершает процессы сеанса: это не обход (R17, D-41). Их
		// помечаем как ожидаемое завершение, а то, что успело погибнуть раньше события, не
		// записываем (FB-1). Номер сеанса неизвестен: вслепую ничего не снимаем, исчезнувший
		// сеанс сверка уберёт сама (dropSession снимает его ожидания).
		if id != SessionUnknown {
			if ss := s.sessions[id]; ss != nil {
				s.expectSessionProcs(ss)
			}
			s.cancelBypass(id, "конец сеанса")
		}
	case SessionDisconnect:
		// Смена пользователя сеанс не завершает: страж, значок и окна ребёнка в отключённом
		// сеансе живут дальше, поэтому они не помечаются, а ожидания не снимаются: гибель
		// стража, значка или окна прямо перед отключением остаётся обходом (иначе это дыра:
		// убить и сменить пользователя). Признак «не на экране» ставит noteSessionState.
	}
	s.guard("session", nil, func() { s.syncSessions(real) })
	s.noteSessionState(ev, id)
}

// noteSessionState запоминает, на экране ли сеанс и заблокирован ли он, по событию Windows
// (FB-3, R2). Вызывается после сверки сеансов: событие может прийти раньше, чем служба узнала
// о новом сеансе. Событие точнее списка Windows, который в этот момент мог ещё не обновиться;
// следующая сверка снова берёт состояние из списка.
func (s *Service) noteSessionState(ev SessionEvent, id uint32) {
	if id == SessionUnknown {
		// Номер сеанса прочитать не удалось: экран уже перечитан сверкой со списком Windows,
		// блокировку по такому событию не ставим (её видит страж), а разблокировку, вход и
		// подключение применяем ко всем сеансам: лишний сброс безвреден, страж поправит.
		if ev == SessionUnlock || ev == SessionLogon || ev == SessionConnect {
			for _, ss := range s.sessions {
				ss.locked = false
			}
		}
		return
	}
	ss := s.sessions[id]
	if ss == nil {
		return
	}
	switch ev {
	case SessionLock:
		ss.locked = true
	case SessionUnlock, SessionLogon:
		ss.locked = false
	case SessionConnect:
		// Подключение сбрасывает признак блокировки: пропущенная разблокировка не должна
		// навсегда остановить время, а экран входа после подключения видит сам страж.
		ss.screen, ss.locked, ss.offHold = ScreenOn, false, time.Time{}
		s.vlogf("сеанс %d: подключение, на экране", id)
	case SessionDisconnect:
		// Список Windows может отстать от события: пока держится offHold, он не отменяет отключение.
		ss.screen, ss.offHold = ScreenOff, s.real.Add(offHoldFor)
		s.vlogf("сеанс %d: отключение, удержание отключения до %s", id, ss.offHold.Format("15:04:05"))
	}
}

// offHoldFor — сколько событие отключения сеанса сильнее списка сеансов Windows (FB-3, S33).
const offHoldFor = 5 * time.Second

// lockResetInputMs — ввод не старше этого значит: за сеансом сидят. Снимок стража с таким вводом
// и SessionActive=true противоречит блокировке (в заблокированном сеансе страж видит рабочий
// стол Winlogon и присылает SessionActive=false) и сбрасывает признак, если событие
// разблокировки потеряно.
const lockResetInputMs = 5000

// resetLockBySample сбрасывает признак блокировки по свежему снимку стража, пока сеанс на экране.
func (ss *sessState) resetLockBySample(sm ipc.SensorSample) {
	if ss.locked && ss.screen == ScreenOn && sm.SessionActive && sm.LastInputAgoMs < lockResetInputMs {
		ss.locked = false
	}
}

// offScreen: ребёнка за компьютером нет, что бы ни прислал страж (отключённый сеанс, блокировка).
func (ss *sessState) offScreen() bool { return ss.screen == ScreenOff || ss.locked }

// expectSessionProcs помечает значок, страж и окна сеанса как штатно завершаемые, чтобы их
// гибель при выходе из учётки или смене пользователя не считалась обходом (R17, D-41).
func (s *Service) expectSessionProcs(ss *sessState) {
	if ss.guard != nil {
		ss.guard.expected = true
	}
	if ss.tray != nil {
		ss.tray.expected = true
	}
	for _, sp := range ss.windows {
		sp.expected = true
	}
}

// syncSessions сверяет список сеансов Windows с тем, что запущено: заводит новые сеансы,
// убирает исчезнувшие (не обход), запускает недостающие значки и стражей и заново запускает
// упавшие (обход, если упал процесс в сеансе ребёнка).
func (s *Service) syncSessions(real time.Time) {
	if !s.started || s.stopping {
		return
	}
	list, err := s.cfg.Launcher.Sessions()
	if err != nil {
		s.logRate("sessions", "не удалось получить список сеансов Windows: %v", err)
		return
	}
	seen := map[uint32]bool{}
	for _, si := range list {
		seen[si.ID] = true
		ss := s.sessions[si.ID]
		if ss != nil && ss.sid != si.SID {
			// В том же номере сеанса теперь другой пользователь.
			s.dropSession(ss)
			ss = nil
		}
		if ss == nil {
			ss = &sessState{id: si.ID, user: si.User, sid: si.SID, windows: map[string]*supProc{}}
			s.sessions[si.ID] = ss
		}
		if si.Screen != ScreenUnknown {
			if si.Screen == ScreenOn && real.Before(ss.offHold) {
				s.vlogf("сеанс %d: список Windows говорит «на экране» (WTS %d), удержание отключения до %s сильнее", ss.id, si.WTSState, ss.offHold.Format("15:04:05"))
			} else {
				ss.screen = si.Screen
			}
		}
		if ss.listWTS != si.WTSState || ss.listSeen != si.Screen {
			ss.listWTS, ss.listSeen = si.WTSState, si.Screen
			s.vlogf("сеанс %d: состояние WTS %d (экран: %v), в службе: %v", ss.id, si.WTSState, si.Screen, ss.screen)
		}
		prev := ss.childID
		ss.childID = 0
		if cs := s.childBySID(si.SID); cs != nil {
			ss.childID = cs.child.ID
			if prev != ss.childID {
				s.checkAdmin(cs, real)
				s.applyPolicy(cs.child.SID, true)
			}
		}
	}
	for id, ss := range s.sessions {
		if !seen[id] {
			s.dropSession(ss)
		}
	}
	for _, id := range s.sessionOrder() {
		ss := s.sessions[id]
		s.guard("session", nil, func() { s.superviseSession(ss, real) })
	}
	s.guard("bypass", nil, func() { s.settleBypass(real) })
	s.saveProcs()
}

func (s *Service) sessionOrder() []uint32 {
	ids := make([]uint32, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// dropSession завершает то, что осталось в исчезнувшем сеансе, и забывает его.
func (s *Service) dropSession(ss *sessState) {
	s.cancelBypass(ss.id, "сеанс исчез или в нём другой пользователь")
	s.stopSession(ss)
	for _, ps := range s.parents {
		if ps.sessionID == ss.id {
			s.endParent(ps)
		}
	}
	delete(s.binds, ss.id)
	delete(s.guardCmd, ss.id)
	delete(s.guardUI, ss.id)
	delete(s.sessions, ss.id)
}

func (s *Service) stopSession(ss *sessState) {
	s.stopProc(ss.guard)
	s.stopProc(ss.tray)
	for _, sp := range ss.windows {
		s.stopProc(sp)
	}
	ss.guard, ss.tray, ss.windows = nil, nil, map[string]*supProc{}
}

func (s *Service) stopProc(sp *supProc) {
	if sp == nil || sp.proc == nil {
		return
	}
	if err := sp.proc.Terminate(); err != nil {
		s.logRate(fmt.Sprint("term", sp.pid), "не удалось завершить процесс %d (%s): %v", sp.pid, sp.role, err)
	}
	sp.proc.Close()
	sp.proc = nil
}

// stopAllProcs завершает значки, стражей и окна во всех сеансах (остановка службы, Ruling 15).
func (s *Service) stopAllProcs() {
	for _, id := range s.sessionOrder() {
		s.stopSession(s.sessions[id])
	}
	s.sessions = map[uint32]*sessState{}
	s.parents = map[string]*parentSession{}
	s.binds = map[uint32]*guardBind{}
	s.saveProcs()
}

// superviseSession держит в сеансе значок, у ребёнка ещё и стража, и следит за окнами.
func (s *Service) superviseSession(ss *sessState, real time.Time) {
	sess := fmt.Sprintf("--session=%d", ss.id)
	if ss.guard == nil {
		ss.guard = &supProc{kind: ProcGuard, role: roleGuard}
	}
	if ss.tray == nil {
		ss.tray = &supProc{kind: ProcTray, role: roleTray}
	}
	s.ensureProc(ss, ss.tray, []string{launch.ModeTray.Flag(), sess}, real)
	if ss.childID != 0 {
		s.ensureProc(ss, ss.guard, []string{launch.ModeGuard.Flag(), sess}, real)
	} else {
		s.stopProc(ss.guard)
	}
	kinds := make([]string, 0, len(ss.windows))
	for k := range ss.windows {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		s.superviseWindow(ss, k, ss.windows[k], real)
	}
}

// ensureProc запускает процесс, если его нет, и разбирает его гибель.
func (s *Service) ensureProc(ss *sessState, sp *supProc, args []string, real time.Time) {
	if sp.proc != nil {
		if sp.proc.Alive() {
			return
		}
		s.procDied(ss, sp, real)
	}
	if real.Before(sp.nextTry) {
		return
	}
	s.launchProc(ss, sp, args, real)
}

// procDied разбирает гибель значка или стража. Не по сообщению интерфейса в сеансе ребёнка это
// возможный обход (R17в): процесс запускается заново сразу, то есть не позже следующего
// обслуживания (FR-27: не позже 3 с), а запись в журнал ждёт bypassGrace, не закончился ли
// сеанс (deferBypass). В сеансе взрослого перезапуск без обхода.
func (s *Service) procDied(ss *sessState, sp *supProc, real time.Time) {
	sp.proc.Close()
	sp.proc = nil
	closing := !sp.closingAt.IsZero() && real.Sub(sp.closingAt) <= closingLife
	if sp.expected {
		sp.expected, sp.closingAt = false, time.Time{}
		sp.nextTry = real.Add(10 * time.Second)
		return
	}
	if real.Sub(sp.startedAt) < time.Minute {
		sp.failures++
	} else {
		sp.failures = 1
	}
	sp.nextTry = real
	if sp.failures > 3 {
		sp.nextTry = real.Add(backoff(sp.failures - 3))
	}
	detail, subtype := detailTrayClosed, "tray_closed"
	if sp.kind == ProcGuard {
		detail, subtype = detailGuardClosed, "guard_closed"
	}
	s.cfg.Logf("Процесс %s в сеансе %d завершился без сообщения интерфейса", sp.role, ss.id)
	grace := bypassGrace
	if closing {
		// Страж сообщил GuardClosing: выключение или выход могли быть отменены, поэтому гибель
		// не стирается, а ждёт решения дольше; конец сеанса и остановка службы всё равно снимут
		// её без записи. Перезапуск от этого не откладывается (FR-27).
		grace = closingLife
	}
	sp.closingAt = time.Time{}
	s.deferBypass(ss, detail, subtype, real, grace)
}

// backoff возвращает паузу перед следующей попыткой запуска: 1, 2, 4 ... не больше 30 с.
func backoff(n int) time.Duration {
	d := time.Second
	for i := 1; i < n && d < 30*time.Second; i++ {
		d *= 2
	}
	return min(d, 30*time.Second)
}

func (s *Service) launchProc(ss *sessState, sp *supProc, args []string, real time.Time) bool {
	p, err := s.cfg.Launcher.Launch(sp.kind, ss.id, args)
	if err != nil {
		sp.failures++
		sp.nextTry = real.Add(backoff(sp.failures))
		s.logRate(fmt.Sprint("launch", ss.id, sp.role), "не удалось запустить %s в сеансе %d: %v", sp.role, ss.id, err)
		return false
	}
	sp.proc, sp.pid, sp.startedAt, sp.expected, sp.closingAt = p, p.PID(), real, false, time.Time{}
	sp.stamp, _ = s.cfg.Launcher.Stamp(sp.pid)
	sp.nextTry = time.Time{}
	return true
}

// deferBypass ставит возможный обход в ожидание на grace (обычно bypassGrace) вместо немедленной записи (FB-1,
// R17, FR-27): при выключении, перезагрузке и выходе из учётки Windows закрывает стража, значок и
// окна раньше, чем служба узнаёт о конце сеанса. Повторная гибель того же вида в пределах
// ожидания второго ожидания не создаёт и срок не сдвигает. В сеансе без ребёнка ожидать нечего.
func (s *Service) deferBypass(ss *sessState, detail, subtype string, real time.Time, grace time.Duration) {
	if ss.childID == 0 {
		return
	}
	for i, p := range s.bypassWait {
		if p.sessionID == ss.id && p.subtype == subtype {
			if grace > p.wait() {
				s.bypassWait[i].grace = grace // страж сообщил GuardClosing: срок продлевается, начало прежнее
			}
			return
		}
	}
	s.bypassWait = append(s.bypassWait, pendingBypass{sessionID: ss.id, sid: ss.sid, detail: detail, subtype: subtype, at: real, grace: grace})
	s.vlogf("Обход в сеансе %d поставлен в ожидание на %d с: %s", ss.id, int(grace/time.Second), detail)
}

// cancelBypass снимает без записи все ожидания сеанса: пришёл конец сеанса или сеанс исчез.
func (s *Service) cancelBypass(sessionID uint32, why string) {
	keep := s.bypassWait[:0]
	for _, p := range s.bypassWait {
		if p.sessionID != sessionID {
			keep = append(keep, p)
			continue
		}
		s.vlogf("Обход не записан (%s): %s, сеанс %d", why, p.detail, p.sessionID)
	}
	s.bypassWait = keep
}

// cancelAllBypass снимает без записи все ожидания: служба останавливается (выключение компьютера).
func (s *Service) cancelAllBypass(why string) {
	for _, p := range s.bypassWait {
		s.vlogf("Обход не записан (%s): %s, сеанс %d", why, p.detail, p.sessionID)
	}
	s.bypassWait = nil
}

// settleBypass превращает ожидания старше bypassGrace в записи, если сеанс жив и в нём тот же
// пользователь. Зовётся из обычного обслуживания (syncSessions) после проверки списка сеансов.
func (s *Service) settleBypass(real time.Time) {
	var due []pendingBypass
	keep := s.bypassWait[:0]
	for _, p := range s.bypassWait {
		if real.Sub(p.at) < p.wait() {
			keep = append(keep, p)
		} else {
			due = append(due, p)
		}
	}
	s.bypassWait = keep
	for _, p := range due {
		ss := s.sessions[p.sessionID]
		if ss == nil || ss.sid != p.sid {
			s.vlogf("Обход не записан (сеанс исчез): %s, сеанс %d", p.detail, p.sessionID)
			continue
		}
		s.bypassIn(ss, p.detail, p.subtype, real)
	}
}

// bypassIn записывает попытку обхода ребёнку сеанса (R17, D-41): только журнал, без
// блокировки. В сеансе, где работает не ребёнок, и у ребёнка-администратора (R1) обхода нет.
func (s *Service) bypassIn(ss *sessState, detail, subtype string, real time.Time) {
	if ss.childID == 0 {
		return
	}
	cs := s.children[ss.childID]
	if cs == nil || cs.admin {
		return
	}
	s.markBypass(cs, detail, subtype, real)
}

// markBypass пишет попытку обхода подробным событием в журнал (R17в, D-41): экран не
// блокируется, время не отключается. Страж повторяет сообщение каждую секунду, поэтому
// одинаковая подробность в пределах bypassLogWindow записывается один раз.
func (s *Service) markBypass(cs *childState, detail, subtype string, real time.Time) bool {
	if cs.lastBypass == detail && !real.IsZero() && real.Sub(cs.lastBypassAt) < bypassLogWindow {
		cs.lastBypassAt = real
		return false
	}
	cs.lastBypass, cs.lastBypassAt = detail, real
	d := cs.eng.MarkBypass(detail)
	s.afterDecision(cs, d, map[string]any{"subtype": subtype})
	return true
}

// superviseWindow следит за окном. Закрытое штатно забывается; завершённое иначе в сеансе
// ребёнка открывается заново и считается возможным обходом (Ruling 19; запись через bypassGrace,
// deferBypass); у взрослого просто забывается.
func (s *Service) superviseWindow(ss *sessState, kind string, sp *supProc, real time.Time) {
	if sp.proc == nil || sp.proc.Alive() {
		return
	}
	sp.proc.Close()
	sp.proc = nil
	s.endParentsOf(sp.pid)
	delete(ss.windows, kind)
	if sp.expected {
		return
	}
	s.cfg.Logf("Окно %s в сеансе %d завершилось без сообщения интерфейса", kind, ss.id)
	if ss.childID == 0 {
		return
	}
	s.deferBypass(ss, detailWindowClosed, "window_closed", real, bypassGrace)
	if _, err := s.openWindow(ss, kind, false, real); err != nil {
		s.logRate(fmt.Sprint("reopen", ss.id), "окно %s не открыто заново: %v", kind, err)
	}
}

// windowFlag возвращает ключ запуска окна.
func windowFlag(kind string) (string, bool) {
	switch kind {
	case ipc.WindowParent:
		return launch.ModeParent.Flag(), true
	case ipc.WindowMyStats:
		return launch.ModeMyStats.Flag(), true
	case ipc.WindowFirstRun:
		return launch.ModeFirstRun.Flag(), true
	}
	return "", false
}

// openWindow открывает окно в сеансе от имени его пользователя. Окно уже открыто: ничего не
// делает. handover (только для окна родителя): окно получает родительскую сессию, токен
// передаётся ключом запуска и привязывается к процессу окна здесь же, под той же блокировкой,
// поэтому окно не успеет прийти со своим запросом раньше привязки.
func (s *Service) openWindow(ss *sessState, kind string, handover bool, real time.Time) (bool, error) {
	flag, ok := windowFlag(kind)
	if !ok {
		return false, fmt.Errorf("неизвестный вид окна %q", kind)
	}
	if old := ss.windows[kind]; old != nil && old.proc != nil && old.proc.Alive() {
		if !handover {
			return true, nil
		}
		// Открытое окно не принимает сессию на ходу: закрываем его штатно и открываем новое.
		old.expected = true
		s.endParentsOf(old.pid)
		s.stopProc(old)
		delete(ss.windows, kind)
	}
	args := []string{flag}
	var tok string
	if handover {
		if tok = s.cfg.Rand(); tok == "" {
			return false, fmt.Errorf("нет случайных чисел для токена сессии")
		}
		args = append(args, "--handover="+tok)
	}
	p, err := s.cfg.Launcher.Launch(ProcWindow, ss.id, args)
	if err != nil {
		return false, err
	}
	sp := &supProc{kind: ProcWindow, role: kind, proc: p, pid: p.PID(), startedAt: real}
	var ok2 bool
	sp.stamp, ok2 = s.cfg.Launcher.Stamp(sp.pid)
	ss.windows[kind] = sp
	if handover {
		s.parents[tok] = &parentSession{
			token: tok, pid: sp.pid, stamp: sp.stamp, stampOK: ok2,
			sid: ss.sid, sessionID: ss.id, last: real,
		}
	}
	s.saveProcs()
	return true, nil
}

// trayClosing принимает от значка сообщение «закрываюсь штатно» (конец сеанса): это не обход.
// Принимается только от процесса, который служба сама запустила: иначе ребёнок сообщил бы
// «закрываюсь» и закрыл значок без последствий.
func (s *Service) trayClosing(peer ipc.Peer) {
	ss := s.sessions[peer.SessionID]
	if ss != nil && ss.tray != nil && ss.tray.proc != nil && ss.tray.pid == peer.PID {
		ss.tray.expected = true
	}
}

// guardClosing принимает от стража сообщение «закрываюсь» (конец сеанса: выход из учётной
// записи, выключение, смена пользователя): R17, FR-27, FB-1. Принимается только от процесса,
// который служба сама запустила, как и у значка. Запись об обходе сообщение НЕ стирает: ребёнок
// может запустить shutdown и отменить его отказом в WM_QUERYENDSESSION, и тогда страж пропал,
// а сеанс остался. Поэтому сообщение лишь продлевает ожидание решения до closingLife (procDied);
// конец сеанса, исчезновение сеанса и остановка службы снимают его без записи, перезапуск стража
// идёт сразу, как при любой гибели.
func (s *Service) guardClosing(peer ipc.Peer) {
	ss := s.sessions[peer.SessionID]
	if ss != nil && ss.guard != nil && ss.guard.proc != nil && ss.guard.pid == peer.PID {
		ss.guard.closingAt = s.real
		s.vlogf("страж сеанса %d сообщил о закрытии: решение об обходе отложено на %d с", ss.id, int(closingLife/time.Second))
	}
}

// windowClosing принимает от окна сообщение о штатном закрытии (код 0, FR-27). Закрытие окна
// родителя заканчивает и его родительскую сессию (R19).
func (s *Service) windowClosing(peer ipc.Peer, kind string) {
	ss := s.sessions[peer.SessionID]
	if ss == nil {
		return
	}
	if sp := ss.windows[kind]; sp != nil && sp.pid == peer.PID {
		sp.expected = true
	}
	s.endParentsOf(peer.PID)
}

// --- память о запущенном ---------------------------------------------------------------

func (s *Service) procsPath() string {
	if s.cfg.DataDir == "" {
		return ""
	}
	return filepath.Join(s.cfg.DataDir, procStateFile)
}

// saveProcs записывает, что запущено: после падения службы новая служба найдёт и завершит
// эти процессы (Ruling 15).
func (s *Service) saveProcs() {
	path := s.procsPath()
	if path == "" {
		return
	}
	recs := []procRecord{}
	add := func(ss *sessState, sp *supProc) {
		if sp != nil && sp.proc != nil {
			recs = append(recs, procRecord{Role: sp.role, Session: ss.id, PID: sp.pid, Stamp: sp.stamp})
		}
	}
	for _, id := range s.sessionOrder() {
		ss := s.sessions[id]
		add(ss, ss.guard)
		add(ss, ss.tray)
		kinds := make([]string, 0, len(ss.windows))
		for k := range ss.windows {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			add(ss, ss.windows[k])
		}
	}
	if err := writeProcRecords(path, recs); err != nil {
		s.logRate("procs", "не записана память о запущенных процессах: %v", err)
	}
}

// killStale завершает процессы, оставшиеся от прошлого запуска службы (Ruling 15: в сеансе
// ровно один страж и один значок). Процесс узнаётся по PID вместе с моментом создания, а не
// по пути программы: установщик с тем же именем файла не должен пострадать.
func (s *Service) killStale() {
	path := s.procsPath()
	if path == "" {
		return
	}
	for _, r := range readProcRecords(path) {
		if s.cfg.Launcher.KillStale(r.PID, r.Stamp) {
			s.cfg.Logf("Завершён процесс %s (PID %d) от прошлого запуска службы", r.Role, r.PID)
		}
	}
	if err := writeProcRecords(path, nil); err != nil {
		s.logRate("procs", "не очищена память о запущенных процессах: %v", err)
	}
}
