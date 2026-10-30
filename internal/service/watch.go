package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Наблюдатель процессов (веха M3: R5, R8, R12, R13, R19, NFR-6, RM3-1, RM3-8, RM3-10, RM3-11).
// Опрос — собственный тикер по настоящим часам (watchEvery), не тик учёта. Под s.mu берётся только
// снимок (сеансы детей, решение ядра, свои процессы PC Block); List, свойства файлов и Kill — вне
// s.mu, итог записывается под s.mu. «Не знаю» = «не трогать» (PL-9): нечитаемый путь, чужой или
// неизвестный сеанс, процесс другого пользователя в сеансе ребёнка, класс «система», список программ не готов, пауза и безопасный режим.

const (
	watchEvery        = 250 * time.Millisecond // период опроса процессов (NFR-6: не позже 1 с)
	watchRepeatEvery  = 60 * time.Second       // одно и то же событие о программе в журнал не чаще
	plateEvery        = 5 * time.Second        // плашка о программе в сеансе не чаще
	blockedPlateShowS = 10                     // секунд показа плашки о закрытой программе
	unknownPlateShowS = 15                     // плашка «ждёт разрешения» с кнопкой «Попросить сейчас»
)

// procKey — процесс: PID повторяется, пара (PID, момент создания) — нет (RM3-11).
type procKey struct {
	pid   uint32
	stamp uint64
}

// watchEntry — то, что наблюдатель помнит о живом процессе до его смерти: путь, отпечаток файла
// (прочитан один раз, RM3-12), закрыт ли уже (запись в журнал и «Ждут решения» одна на процесс).
type watchEntry struct {
	session uint32
	path    string // путь как прислал адаптер (регистр для имени)
	norm    string // programs.Normalize(path)
	fp      string
	fpRead  bool
	pub     string // издатель файла (RM3-13): читается, только если решение в профиле его ждёт
	pubRead bool
	closed  bool // Kill уже удался: записи сделаны, повторный Kill без записей
	seen    uint64
}

// watchTarget — снимок сеанса ребёнка для одного прохода (под s.mu).
type watchTarget struct {
	session uint32
	childID int64
	sid     string // SID учётки ребёнка: процессы другого пользователя в его сеансе не трогаются (R19)
	d       core.Decision
	own     map[uint32]bool // процессы PC Block этого сеанса (страж, значок, окна): не трогать никогда
}

// watchAct — процесс, который наблюдатель закрывает в этом проходе.
type watchAct struct {
	t         *watchTarget
	p         ProcInfo
	e         *watchEntry
	res       programs.Result
	cat       core.Category // для закрытия по правилу (игры, только учёба)
	plateKind string        // ipc.Blocked*
	killed    bool
	again     bool // процесс уже закрывали: без записей
	publisher string
	descr     string
}

// watchState — память наблюдателя; её трогает только Watch (под watchMu, не под s.mu).
type watchState struct {
	mu    sync.Mutex
	gen   uint64
	procs map[procKey]*watchEntry
}

// RunWatch крутит наблюдатель на собственном тикере по настоящим часам, пока не отменён ctx.
// Нет наблюдателя (Watcher nil) — сразу возвращается. Паника прохода не останавливает цикл.
func (s *Service) RunWatch(ctx context.Context) {
	if s.cfg.Watcher == nil {
		return
	}
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runGuarded("watch", func() { s.Watch(s.cfg.RealNow()) })
		}
	}
}

// Watch — один проход наблюдателя в настоящий момент real.
func (s *Service) Watch(real time.Time) {
	if s.cfg.Watcher == nil {
		return
	}
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	targets, ok := s.watchSnapshot()
	if !ok {
		return
	}
	if s.watch.procs == nil {
		s.watch.procs = map[procKey]*watchEntry{}
	}
	s.watch.gen++
	gen := s.watch.gen
	var acts []watchAct
	notes := []string{}
	unreadable, others := map[uint32]int{}, map[uint32]int{}
	for i := range targets {
		t := &targets[i]
		list, err := s.cfg.Watcher.List(t.session)
		if err != nil {
			notes = append(notes, fmt.Sprintf("watch-list|%d\x00наблюдатель: процессы сеанса %d не прочитаны, ничего не закрывается: %v", t.session, t.session, err))
			s.keepSession(t.session, gen)
			continue
		}
		for _, p := range list {
			a, skip := s.watchDecide(t, p, gen)
			switch skip {
			case skipUnreadable:
				unreadable[t.session]++
			case skipOtherUser:
				others[t.session]++
			}
			if a != nil {
				acts = append(acts, *a)
			}
		}
	}
	fps, pubs := map[string]string{}, map[string]string{}
	for k, e := range s.watch.procs {
		if e.seen != gen {
			delete(s.watch.procs, k)
			continue
		}
		if e.fpRead {
			fps[e.norm] = e.fp
		}
		if e.pubRead {
			pubs[e.norm] = e.pub
		}
	}
	for i := range acts {
		a := &acts[i]
		a.killed = s.cfg.Watcher.Kill(a.p)
		if a.killed && !a.again && a.res.Verdict == programs.VerdictUnknown {
			pub, descr, _ := s.cfg.FileMeta(a.e.norm)
			a.publisher, a.descr = programs.ClipName(pub), programs.ClipName(descr)
		}
		if a.killed {
			a.e.closed = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.real = real
	s.fpSeen = fps   // категория секунды переднего плана сверяет тот же отпечаток (RM3-12)
	s.pubSeen = pubs // и того же издателя (RM3-13)
	s.applyWatch(acts, notes, unreadable, others, real)
}

// keepSession помечает виденными процессы сеанса, список которого не прочитался: память о них
// (и о том, что их уже закрывали) не теряется из-за одного сбоя.
func (s *Service) keepSession(session uint32, gen uint64) {
	for _, e := range s.watch.procs {
		if e.session == session {
			e.seen = gen
		}
	}
}

// watchSnapshot снимает под s.mu то, что нужно проходу: сеансы детей (кроме администраторов, R1),
// их решения ядра и свои процессы. ok = false — проход ничего не делает: служба не запущена или
// останавливается, список программ не готов (RM3-10, R18), не определены папка Windows или
// каталог установки (класс «система» был бы пуст), тестовый режим без WatchDir (RM3-8).
// Пауза (R11) и безопасный режим (R18): у ребёнка ничего не закрывается, его сеанс не опрашивается.
// Сеансы взрослых (R19) и сеансы без пользователя не опрашиваются вовсе.
func (s *Service) watchSnapshot() ([]watchTarget, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.stopping || !s.programsReady() {
		return nil, false
	}
	if r := s.classifier.Roots(); r.SystemRoot == "" || r.InstallDir == "" {
		// Без этих корней класс «система» пуст: всё в папке Windows и окна PC Block стали бы
		// «незнакомыми» (PL-13). «Не знаю» = «не трогать».
		s.logRate("watch-roots", "наблюдатель: папка Windows или каталог установки PC Block не определены (%q, %q), ничего не закрывается", r.SystemRoot, r.InstallDir)
		return nil, false
	}
	if s.cfg.Test.Enabled && s.cfg.Test.WatchDir == "" {
		s.logRate("watch-testdir", "тестовый режим без WatchDir: наблюдатель ничего не закрывает")
		return nil, false
	}
	out := []watchTarget{}
	for _, id := range s.sessionOrder() {
		ss := s.sessions[id]
		if ss.childID == 0 {
			continue
		}
		cs := s.children[ss.childID]
		if cs == nil || cs.admin || cs.rules.Broken {
			continue
		}
		if cs.child.SID == "" {
			// PL-9: без SID ребёнка не отличить его процессы от программ администратора (UAC).
			s.logRate(fmt.Sprint("watch-nosid|", ss.childID), "наблюдатель: SID ребёнка неизвестен (id %d), в сеансе %d ничего не закрывается", ss.childID, id)
			continue
		}
		d := cs.eng.Current()
		if d.Status == core.StatusPaused || d.Status == core.StatusSafe {
			continue
		}
		own := map[uint32]bool{}
		for _, sp := range []*supProc{ss.guard, ss.tray} {
			if sp != nil && sp.pid != 0 {
				own[sp.pid] = true
			}
		}
		for _, sp := range ss.windows {
			if sp != nil && sp.pid != 0 {
				own[sp.pid] = true
			}
		}
		out = append(out, watchTarget{session: id, childID: ss.childID, sid: cs.child.SID, d: d, own: own})
	}
	return out, true
}

// Почему процесс пропущен (для журнала работы).
const (
	skipNone = iota
	skipUnreadable
	skipOtherUser
)

// watchDecide решает судьбу одного процесса сеанса t вне s.mu: nil — не трогать. Порядок:
// чужой сеанс, свой процесс PC Block, процесс другого пользователя или с непрочитанным
// пользователем (программа администратора через UAC в сеансе ребёнка, R19; проверяется
// пользователь, а не повышение прав), нечитаемый путь, вне WatchDir в тестовом режиме, класс
// «система» — не трогать; «незнакомая» и «запрещённая» — закрыть; разрешённая категория —
// закрыть, только если её сейчас нельзя и правило закрывает её (игры при CloseGames, всё,
// кроме учёбы, при StudyOnly; R5, R8). Под экраном «Блокировать экран» закрываются только игры.
func (s *Service) watchDecide(t *watchTarget, p ProcInfo, gen uint64) (*watchAct, int) {
	if p.SessionID != t.session || t.own[p.PID] {
		return nil, skipNone
	}
	if !p.UserOK || !strings.EqualFold(p.UserSID, t.sid) {
		return nil, skipOtherUser
	}
	if !p.PathOK || p.Path == "" {
		return nil, skipUnreadable
	}
	key := procKey{pid: p.PID, stamp: p.Stamp}
	e := s.watch.procs[key]
	if e == nil {
		norm := programs.Normalize(p.Path)
		if norm == "" {
			return nil, skipUnreadable
		}
		e = &watchEntry{session: t.session, path: p.Path, norm: norm}
		s.watch.procs[key] = e
	}
	e.seen = gen
	if dir := s.cfg.Test.WatchDir; s.cfg.Test.Enabled && !strings.HasPrefix(e.norm, strings.TrimSuffix(dir, `\`)+`\`) {
		return nil, skipNone // тестовый режим: только папка тестовых программ (RM3-8)
	}
	c := s.classifier
	needFP := !e.fpRead && !c.Roots().IsTrusted(e.norm) && c.NeedsFingerprint(e.norm)
	needPub := !e.pubRead && c.NeedsPublisher(e.norm)
	if needFP || needPub {
		// Одно чтение свойств на процесс: отпечаток (RM3-12) и издатель (RM3-13) вместе.
		var pub string
		pub, _, e.fp = s.cfg.FileMeta(e.norm)
		e.pub = programs.ClipName(pub)
		e.fpRead, e.pubRead = true, true
	}
	res := c.Classify(e.path, e.fp, e.pub)
	if res.Verdict == programs.VerdictSystem {
		return nil, skipNone
	}
	a := &watchAct{t: t, p: p, e: e, res: res, again: e.closed}
	switch res.Verdict {
	case programs.VerdictUnknown:
		a.plateKind = ipc.BlockedUnknown
	case programs.VerdictForbidden:
		a.plateKind = ipc.BlockedForbidden
	default:
		a.cat = coreCategory(res.Category)
		if t.d.Allows(a.cat) {
			return nil, skipNone
		}
		switch {
		case t.d.StudyOnly && a.cat != core.CatStudy:
			a.plateKind = ipc.BlockedStudyOnly
		case t.d.CloseGames && a.cat == core.CatGames:
			a.plateKind = ipc.BlockedGamesOver
		default:
			return nil, skipNone // под экраном блокировки остальное остаётся под экраном (R8)
		}
	}
	return a, skipNone
}

// coreCategory — категория программы для ядра: games, study; остальное — «Прочее».
func coreCategory(cat string) core.Category {
	switch cat {
	case programs.CatGames:
		return core.CatGames
	case programs.CatStudy:
		return core.CatStudy
	}
	return core.CatOther
}

// applyWatch записывает итог прохода под s.mu: «Ждут решения», журнал родителя (с ограничителем
// повторов), плашки стражу сеанса на экране, журнал работы о незакрытом и нечитаемом.
func (s *Service) applyWatch(acts []watchAct, notes []string, unreadable, others map[uint32]int, real time.Time) {
	for _, n := range notes {
		key, text, _ := strings.Cut(n, "\x00")
		s.logRate(key, "%s", text)
	}
	for session, n := range unreadable {
		s.logRate(fmt.Sprint("watch-unreadable|", session), "наблюдатель: у %d процессов сеанса %d путь не прочитан, они не трогаются", n, session)
	}
	for session, n := range others {
		s.logRate(fmt.Sprint("watch-otheruser|", session), "наблюдатель: %d процессов в сеансе %d идут от имени другого пользователя или пользователь не прочитан, они не трогаются", n, session)
	}
	s.pruneWatchLimits(real)
	for i := range acts {
		a := &acts[i]
		if !a.killed && a.again {
			continue // уже закрыт этим наблюдателем и ещё завершается (PL-16): не сбой
		}
		if !a.killed {
			s.logRate("watch-kill|"+a.e.norm, "наблюдатель: не удалось закрыть %s (PID %d, %s)", a.e.norm, a.p.PID, a.res.Verdict)
			continue
		}
		if a.again {
			continue
		}
		cs := s.children[a.t.childID]
		if cs == nil {
			continue
		}
		s.vlogf("наблюдатель закрыл %s (PID %d) в сеансе %d: %s", a.e.norm, a.p.PID, a.t.session, a.res.Verdict)
		s.recordClosed(cs, a, real)
	}
}

// recordClosed — записи о закрытой программе: журнал, «Ждут решения» и плашка.
func (s *Service) recordClosed(cs *childState, a *watchAct, real time.Time) {
	name := a.res.Name
	if name == "" {
		name = programs.NameFromPath(a.e.path)
	}
	details := map[string]any{"name": name, "path": a.e.norm}
	var kind, text string
	plate := ipc.BlockedPlate{Kind: a.plateKind, ShowS: blockedPlateShowS, Path: a.e.norm}
	switch a.plateKind {
	case ipc.BlockedUnknown:
		kind, text = evProgramPending, ""
		plate.Text, plate.CanAsk, plate.ShowS = fmt.Sprintf(textPlateUnknown, name), true, unknownPlateShowS
		pend := PendingProgram{Path: a.e.norm, Name: name, Publisher: a.publisher, Description: a.descr, WasCategory: a.res.WasCategory}
		if _, err := s.cfg.Store.AddPendingAttempt(pend, cs.child.ID, real.UnixMilli()); err != nil {
			s.logRate("pending-add", "попытка запуска %s не записана в «Ждут решения»: %v", a.e.norm, err)
		}
	case ipc.BlockedForbidden:
		kind = evProgramForbiddenTry
		plate.Text = fmt.Sprintf(textPlateForbidden, name)
	case ipc.BlockedStudyOnly:
		kind, text = evProgramClosedStudyOnly, a.t.d.DeniedText(a.cat)
		plate.Text = a.t.d.CloseReason
		if plate.Text == "" {
			plate.Text = core.StudyOnlyPlateText
		}
	default:
		kind, text = evProgramClosedGames, a.t.d.DeniedText(a.cat)
		plate.Text = a.t.d.CloseReason
		if plate.Text == "" {
			plate.Text = core.GamesOverPlateText
		}
	}
	if text != "" {
		details["text"] = text
	}
	if s.watchOnce(fmt.Sprintf("ev|%d|%s|%s", cs.child.ID, kind, a.e.norm), watchRepeatEvery, real) {
		s.journal.Write(cs.child.ID, kind, details)
	}
	s.blockedPlate(a.t.session, cs.child.ID, plate, real)
}

// blockedPlate ставит плашку о закрытой программе в очередь стража сеанса (как noticeGuards):
// только ребёнку, чей сеанс сейчас на экране, и не чаще plateEvery на программу.
func (s *Service) blockedPlate(session uint32, childID int64, plate ipc.BlockedPlate, real time.Time) {
	ss := s.sessions[session]
	if ss == nil || ss.childID != childID || ss.offScreen() {
		return
	}
	if !s.watchOnce(fmt.Sprintf("plate|%d|%s", session, plate.Path), plateEvery, real) {
		return
	}
	s.guardCmd[session] = append(s.guardCmd[session], ipc.GuardCommand{Kind: ipc.GuardCmdBlocked, Plate: &plate})
}

// watchOnce — ограничитель повторов: true, если по ключу ничего не было последние every
// (первое событие проходит всегда, PL-16). Вызывать под s.mu.
func (s *Service) watchOnce(key string, every time.Duration, real time.Time) bool {
	if s.watchAt == nil {
		s.watchAt = map[string]time.Time{}
	}
	if last, ok := s.watchAt[key]; ok && real.Sub(last) < every && !real.Before(last) {
		return false
	}
	s.watchAt[key] = real
	return true
}

// pruneWatchLimits забывает ключи ограничителя старше watchRepeatEvery: память не растёт.
func (s *Service) pruneWatchLimits(real time.Time) {
	for k, at := range s.watchAt {
		if real.Sub(at) >= watchRepeatEvery {
			delete(s.watchAt, k)
		}
	}
}
