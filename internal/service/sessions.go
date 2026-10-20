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
	expected  bool      // закрытие сообщено интерфейсом (TrayClosing, WindowClosing): это не обход
	nextTry   time.Time // не запускать заново раньше этого момента
	failures  int
}

// sessState — интерактивный сеанс Windows и запущенные в нём части программы.
type sessState struct {
	id      uint32
	user    string
	sid     string
	childID int64 // 0: в сеансе работает не ребёнок
	guard   *supProc
	tray    *supProc
	windows map[string]*supProc
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
	s.guard("session", nil, func() { s.syncSessions(real) })
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
// обход (R17в): блокировка «Попытка обойти защиту», процесс запускается заново сразу, то есть
// не позже следующего обслуживания (FR-27: не позже 3 с). В сеансе взрослого перезапуск без обхода.
func (s *Service) procDied(ss *sessState, sp *supProc, real time.Time) {
	sp.proc.Close()
	sp.proc = nil
	if sp.expected {
		sp.expected = false
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
	s.bypassIn(ss, detail, subtype)
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
	sp.proc, sp.pid, sp.startedAt, sp.expected = p, p.PID(), real, false
	sp.stamp, _ = s.cfg.Launcher.Stamp(sp.pid)
	sp.nextTry = time.Time{}
	return true
}

// bypassIn ставит блокировку за обход ребёнку сеанса. В сеансе, где работает не ребёнок, и у
// ребёнка-администратора (R1) обхода нет.
func (s *Service) bypassIn(ss *sessState, detail, subtype string) {
	if ss.childID == 0 {
		return
	}
	cs := s.children[ss.childID]
	if cs == nil || cs.admin {
		return
	}
	s.markBypass(cs, detail, subtype)
}

// markBypass ставит блокировку за обход и запоминает её в базе (Ruling 18). Уже стоящая
// блокировка не меняется и события не плодит: страж повторяет сообщение каждую секунду.
func (s *Service) markBypass(cs *childState, detail, subtype string) bool {
	if _, cur := cs.eng.State(); cur != "" {
		return false
	}
	d := cs.eng.MarkBypass(detail)
	_, eff := cs.eng.State()
	if err := s.cfg.Store.SetBypass(cs.child.ID, eff); err != nil {
		s.logRate("bypass", "не записана блокировка за обход для %s: %v", cs.child.Name, err)
	}
	s.afterDecision(cs, d, map[string]any{"subtype": subtype})
	return true
}

// superviseWindow следит за окном. Закрытое штатно забывается; завершённое иначе в сеансе
// ребёнка открывается заново и считается обходом (Ruling 19); у взрослого просто забывается.
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
	s.bypassIn(ss, detailWindowClosed, "window_closed")
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
