package service

import (
	"context"
	"fmt"
	"sort"
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
	// launchSettle — сколько после закрытия запрещённой, запустившейся сама, наблюдатель ждёт
	// уточнения (R13, D-65): следующего снимка стража с вводом (раз в секунду) и ручного запуска
	// того же exe; потом пишет «запускалась сама».
	launchSettle = 2 * time.Second
	// killWorkers — сколько Kill одного прохода идут одновременно (B-37, NFR-6): Windows-адаптер
	// ждёт исчезновения процесса до 300 мс, и по очереди пять закрываемых держали бы проход 1,5 с.
	killWorkers = 8
	// metaBudget — сколько проход ждёт свойства файлов (B-32, NFR-6, PL-17): чтение с сетевого
	// пути может висеть. До решения (отпечаток, издатель; fetchMeta) не успело — процесс в этот
	// проход не трогается, результат берёт следующий. После закрытия (издатель, описание закрытых
	// незнакомых) не успело — в «Ждут решения» «Без подписи издателя»; решение родителя всё равно
	// перечитывает свойства (fileMetaUnlocked).
	metaBudget = 300 * time.Millisecond
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
	norm    string // programs.Normalize(path): настоящий путь — свойства файла, отпечаток, родитель процесса
	key     string // ключ программы (Roots.StableKey, RM4-11): «Ждут решения», журнал, плашка, повторы
	fp      string
	fpRead  bool
	pub     string // издатель файла (RM3-13): читается, только если решение в профиле его ждёт
	pubRead bool
	closed  bool // Kill уже удался: записи сделаны, повторный Kill без записей
	seen    uint64
	// manual — запуск ручной (R13, D-65): решено по фактам facts в момент, когда процесс увиден
	// впервые (родитель, ввод ребёнка, передний план, вход в учётку); why — почему (подробный
	// журнал), parent — путь родителя («» — нет в сеансе). Ввод может уточнить следующий снимок
	// стража (settleLaunches).
	manual bool
	why    string
	parent string
	facts  launchFacts
	// quiet — дочерний процесс того же exe, что и родитель (браузер, Discord): решение и записи —
	// у главного процесса, у дочернего только закрытие (ревью FB-18).
	quiet bool
	// forbiddenAtStart — при первой классификации программа уже была запрещена: только тогда
	// запуск может быть «сам»; запрещённая после запуска закрывается с плашкой, как раньше.
	classified       bool
	forbiddenAtStart bool
}

// launchPending — запрещённая, закрытая как «запустилась сама», ждёт уточнения до until: снимка
// стража новее решения с вводом (тогда это ручной запуск) или ручной попытки того же exe (тогда
// записи «сама» нет). Под s.mu.
type launchPending struct {
	a     watchAct
	until time.Time
}

// maxLaunchDepth — сколько поколений процессов того же exe наблюдатель проходит к главному.
const maxLaunchDepth = 8

// watchTarget — снимок сеанса ребёнка для одного прохода (под s.mu).
type watchTarget struct {
	session uint32
	childID int64
	sid     string // SID учётки ребёнка: процессы другого пользователя в его сеансе не трогаются (R19)
	d       core.Decision
	own     map[uint32]bool // процессы PC Block этого сеанса (страж, значок, окна): не трогать никогда
	// Факты для «ручной или сам» (R13, D-65, launchKind): момент прохода, вход ребёнка в сеанс,
	// последний ввод и передний план по свежему снимку стража (нулевые и «» — данных нет),
	// процессы сеанса по номерам (родитель запуска).
	at       time.Time
	logonAt  time.Time
	inputAt  time.Time
	sampleAt time.Time // момент снимка стража, по которому взяты inputAt и fg
	fg       string
	byPID    map[uint32]ProcInfo
	// hold — пауза ребёнка (R11): процессы только читаются ради категории секунды учёта (B-30),
	// ничего не закрывается и не пишется (watchHeld).
	hold bool
}

// watchAct — процесс, который наблюдатель закрывает в этом проходе.
type watchAct struct {
	t         *watchTarget
	p         ProcInfo
	e         *watchEntry
	res       programs.Result
	cat       core.Category // для закрытия по правилу (игры, только учёба)
	plateKind string        // ipc.Blocked*
	auto      bool          // запрещённая запустилась сама (R13, D-65): без плашки и нарушения
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
	// held — процессы сеансов на паузе (watchHeld): только отпечаток и издатель для учёта (B-30).
	// Отдельно от procs: решения о запуске (ручной или сам) и «уже закрывали» пауза не трогает.
	held map[procKey]*watchEntry
	// metaBusy — пути, свойства которых сейчас читаются (B-32): зависший сетевой путь, запускаемый
	// снова и снова, не плодит горутины. Под своим metaMu: горутина чтения снимает отметку сама,
	// когда проход, начавший её, уже закончен.
	metaMu   sync.Mutex
	metaBusy map[string]bool
	// metaDone — готовые чтения свойств до решения, которые проход не дождался (fetchMeta): их
	// берёт следующий проход. Под metaMu.
	metaDone map[string]fileMeta
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
	targets, ok := s.watchSnapshot(real)
	if !ok {
		return
	}
	if s.watch.procs == nil {
		s.watch.procs = map[procKey]*watchEntry{}
	}
	s.watch.gen++
	gen := s.watch.gen
	var acts []watchAct
	type item struct {
		t *watchTarget
		p ProcInfo
		e *watchEntry
	}
	var items []item
	var need []*watchEntry // свойства файла нужны до решения (B-32): читаются вместе, с бюджетом
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
		if t.hold {
			need = append(need, s.watchHeld(t, list, gen)...)
			continue
		}
		t.byPID = make(map[uint32]ProcInfo, len(list))
		for _, p := range list {
			t.byPID[p.PID] = p
		}
		for _, p := range list {
			e, skip := s.watchEntryOf(t, p, gen)
			switch skip {
			case skipUnreadable:
				unreadable[t.session]++
			case skipOtherUser:
				others[t.session]++
			}
			if e != nil {
				items = append(items, item{t, p, e})
				if s.needsMeta(e) {
					need = append(need, e)
				}
			}
		}
	}
	for _, path := range s.fetchMeta(need) {
		notes = append(notes, fmt.Sprintf("watch-meta|%s\x00наблюдатель: свойства %s читаются дольше %v, решение отложено", path, path, metaBudget))
	}
	for _, it := range items {
		if s.needsMeta(it.e) {
			continue // отпечаток или издатель ещё читаются: «не знаю» — не трогать (PL-9)
		}
		if a := s.watchDecide(it.t, it.p, it.e); a != nil {
			acts = append(acts, *a)
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
	for k, e := range s.watch.held {
		if e.seen != gen {
			delete(s.watch.held, k)
			continue
		}
		if _, ok := fps[e.norm]; !ok && e.fpRead {
			fps[e.norm] = e.fp
		}
		if _, ok := pubs[e.norm]; !ok && e.pubRead {
			pubs[e.norm] = e.pub
		}
	}
	s.killAll(acts)
	s.readClosedMeta(acts)
	for i := range acts {
		if acts[i].killed {
			acts[i].e.closed = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.real = real
	s.fpSeen = fps   // категория секунды переднего плана сверяет тот же отпечаток (RM3-12)
	s.pubSeen = pubs // и того же издателя (RM3-13)
	s.applyWatch(acts, notes, unreadable, others, real)
}

// killAll закрывает процессы прохода пулом из killWorkers горутин (B-37) и ждёт все. Каждая
// горутина пишет только killed своей записи; порядок записей (applyWatch) остаётся порядком acts.
// Паника Kill доходит до Watch после ожидания (runGuarded пишет её, как раньше).
func (s *Service) killAll(acts []watchAct) {
	if len(acts) == 0 {
		return
	}
	var (
		wg       sync.WaitGroup
		pmu      sync.Mutex
		panicked any
	)
	next := make(chan int)
	for range min(killWorkers, len(acts)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				func() {
					defer func() {
						if p := recover(); p != nil {
							pmu.Lock()
							if panicked == nil {
								panicked = p
							}
							pmu.Unlock()
						}
					}()
					acts[i].killed = s.cfg.Watcher.Kill(acts[i].p)
				}()
			}
		}()
	}
	for i := range acts {
		next <- i
	}
	close(next)
	wg.Wait()
	if panicked != nil {
		panic(panicked)
	}
}

// readClosedMeta читает издателя и описание закрытых незнакомых (для «Ждут решения») после всех
// Kill (B-32): медленный путь одной программы не держит закрытие других. Одно чтение на путь в
// своей горутине; проход ждёт их не дольше metaBudget, опоздавшие отбрасываются (запись без
// издателя). Путь, чтение которого ещё висит с прошлого прохода, не читается снова.
func (s *Service) readClosedMeta(acts []watchAct) {
	byPath := map[string][]int{}
	for i := range acts {
		a := &acts[i]
		if a.killed && !a.again && a.res.Verdict == programs.VerdictUnknown {
			byPath[a.e.norm] = append(byPath[a.e.norm], i)
		}
	}
	type meta struct{ path, pub, descr string }
	out := make(chan meta, len(byPath)) // горутина не ждёт опоздавшего читателя
	started := 0
	for path := range byPath {
		if !s.watch.takeMeta(path) {
			continue
		}
		started++
		go func() {
			var pub, descr string
			defer func() {
				if p := recover(); p != nil {
					s.cfg.Logf("наблюдатель: сбой чтения свойств %s: %v", path, p)
				}
				s.watch.doneMeta(path)
				out <- meta{path, pub, descr}
			}()
			pub, descr, _ = s.cfg.FileMeta(path)
		}()
	}
	if started == 0 {
		return
	}
	budget := time.NewTimer(metaBudget)
	defer budget.Stop()
	for ; started > 0; started-- {
		select {
		case m := <-out:
			for _, i := range byPath[m.path] {
				acts[i].publisher, acts[i].descr = programs.ClipName(m.pub), programs.ClipName(m.descr)
			}
		case <-budget.C:
			return
		}
	}
}

// takeMeta отмечает, что свойства path читаются; false — уже читаются.
func (w *watchState) takeMeta(path string) bool {
	w.metaMu.Lock()
	defer w.metaMu.Unlock()
	if w.metaBusy[path] {
		return false
	}
	if w.metaBusy == nil {
		w.metaBusy = map[string]bool{}
	}
	w.metaBusy[path] = true
	return true
}

func (w *watchState) doneMeta(path string) {
	w.metaMu.Lock()
	defer w.metaMu.Unlock()
	delete(w.metaBusy, path)
}

// finishMeta снимает отметку чтения path и кладёт результат до решения в metaDone.
func (w *watchState) finishMeta(path string, m fileMeta) {
	w.metaMu.Lock()
	defer w.metaMu.Unlock()
	delete(w.metaBusy, path)
	if w.metaDone == nil || len(w.metaDone) >= maxMetaDone {
		w.metaDone = map[string]fileMeta{}
	}
	w.metaDone[path] = m
}

// takeDone забирает готовое чтение свойств path до решения.
func (w *watchState) takeDone(path string) (fileMeta, bool) {
	w.metaMu.Lock()
	defer w.metaMu.Unlock()
	m, ok := w.metaDone[path]
	if ok {
		delete(w.metaDone, path)
	}
	return m, ok
}

// keepSession помечает виденными процессы сеанса, список которого не прочитался: память о них
// (и о том, что их уже закрывали) не теряется из-за одного сбоя.
func (s *Service) keepSession(session uint32, gen uint64) {
	for _, m := range []map[procKey]*watchEntry{s.watch.procs, s.watch.held} {
		for _, e := range m {
			if e.session == session {
				e.seen = gen
			}
		}
	}
}

// watchHeld — проход по сеансу ребёнка на паузе (R11, B-30): учёт в паузе продолжается, и
// категория секунды переднего плана сверяет отпечаток и издателя (fpSeen, pubSeen). Здесь они
// только читаются (need → fetchMeta) для процессов ребёнка с прочитанным путём; ничего не
// закрывается, «Ждут решения», журнал и плашки не трогаются. Вне s.mu, под watch.mu.
func (s *Service) watchHeld(t *watchTarget, list []ProcInfo, gen uint64) (need []*watchEntry) {
	if s.watch.held == nil {
		s.watch.held = map[procKey]*watchEntry{}
	}
	for _, p := range list {
		if p.SessionID != t.session || t.own[p.PID] || !p.UserOK || !strings.EqualFold(p.UserSID, t.sid) || !p.PathOK || p.Path == "" {
			continue
		}
		key := procKey{pid: p.PID, stamp: p.Stamp}
		e := s.watch.held[key]
		if e == nil {
			norm := programs.Normalize(p.Path)
			if norm == "" {
				continue
			}
			e = &watchEntry{session: t.session, path: p.Path, norm: norm}
			s.watch.held[key] = e
		}
		e.seen = gen
		if s.needsMeta(e) {
			need = append(need, e)
		}
	}
	return need
}

// needsMeta — решению о процессе e нужны свойства его файла, а они ещё не прочитаны: отпечаток
// (RM3-12) или издатель (RM3-13). Читаются и в тестовом режиме вне WatchDir (T6-5): чтение ничего
// не закрывает, а без издателя Brave из профиля раннера был бы «незнакомой», и сайты в нём не
// различались бы.
func (s *Service) needsMeta(e *watchEntry) bool {
	c := s.classifier
	return (!e.fpRead && !c.Roots().IsTrusted(e.norm) && c.NeedsFingerprint(e.norm)) ||
		(!e.pubRead && c.NeedsPublisher(e.norm))
}

// maxMetaDone — сколько готовых, но ещё не взятых чтений свойств хранится (процесс мог завершиться,
// пока читался его файл): больше — память сбрасывается, чтения пойдут заново.
const maxMetaDone = 1000

// fetchMeta читает свойства файлов процессов need до решения (B-32): одно чтение на путь (отпечаток
// и издатель вместе) в своей горутине, проход ждёт их не дольше metaBudget. Готовое заполняет
// записи; опоздавшее дочитывается в фоне и берётся следующим проходом (watchState.metaDone), путь,
// чтение которого ещё висит, не читается снова. late — пути, чьи свойства в этот проход не
// успели: их процессы проход не трогает.
func (s *Service) fetchMeta(need []*watchEntry) (late []string) {
	byPath := map[string][]*watchEntry{}
	for _, e := range need {
		byPath[e.norm] = append(byPath[e.norm], e)
	}
	apply := func(path string, m fileMeta) {
		for _, e := range byPath[path] {
			e.fp, e.pub = m.fingerprint, programs.ClipName(m.publisher)
			e.fpRead, e.pubRead = true, true
		}
		delete(byPath, path)
	}
	out := make(chan string, len(byPath)) // горутина не ждёт опоздавшего читателя
	started := 0
	for path := range byPath {
		if m, ok := s.watch.takeDone(path); ok {
			apply(path, m)
			continue
		}
		if !s.watch.takeMeta(path) {
			continue // читается с прошлого прохода
		}
		started++
		go func() {
			var m fileMeta
			defer func() {
				if p := recover(); p != nil {
					s.cfg.Logf("наблюдатель: сбой чтения свойств %s: %v", path, p)
					s.watch.doneMeta(path) // свойства не известны: процесс не трогается, чтение — снова
				} else {
					s.watch.finishMeta(path, m)
				}
				out <- path
			}()
			m.publisher, m.description, m.fingerprint = s.cfg.FileMeta(path)
		}()
	}
	if started > 0 {
		budget := time.NewTimer(metaBudget)
		defer budget.Stop()
	wait:
		for ; started > 0; started-- {
			select {
			case path := <-out:
				if m, ok := s.watch.takeDone(path); ok {
					apply(path, m)
				}
			case <-budget.C:
				break wait
			}
		}
	}
	for path := range byPath {
		late = append(late, path)
	}
	sort.Strings(late)
	return late
}

// watchSnapshot снимает под s.mu то, что нужно проходу: сеансы детей (кроме администраторов, R1),
// их решения ядра и свои процессы. ok = false — проход ничего не делает: служба не запущена или
// останавливается, список программ не готов (RM3-10, R18), не определены папка Windows или
// каталог установки (класс «система» был бы пуст), тестовый режим без WatchDir (RM3-8).
// Безопасный режим (R18): у ребёнка ничего не закрывается, его сеанс не опрашивается. Пауза (R11):
// ничего не закрывается, сеанс опрашивается только ради свойств файлов для учёта (hold, B-30).
// Сеансы взрослых (R19) и сеансы без пользователя не опрашиваются вовсе. Заодно снимаются факты
// для «ручной или сам» (launchFacts): вход в сеанс, ввод и передний план по снимку стража этого
// сеанса не старше sampleMaxAge; снимок с неактивным сеансом — данных о вводе нет.
func (s *Service) watchSnapshot(real time.Time) ([]watchTarget, bool) {
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
		if d.Status == core.StatusSafe {
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
		t := watchTarget{session: id, childID: ss.childID, sid: cs.child.SID, d: d, own: own, at: real, logonAt: ss.since, hold: d.Status == core.StatusPaused}
		if sm := cs.sample; cs.haveSample && cs.sampleSess == id && real.Sub(cs.sampleAt) <= sampleMaxAge && sm.SessionActive && sm.LastInputAgoMs >= 0 {
			t.inputAt, t.sampleAt = s.inputMoment(cs.sampleAt, sm.LastInputAgoMs), cs.sampleAt
			t.fg = programs.Normalize(sm.ForegroundPath)
		}
		out = append(out, t)
	}
	return out, true
}

// Почему процесс пропущен (для журнала работы).
const (
	skipNone = iota
	skipUnreadable
	skipOtherUser
)

// watchEntryOf — запись наблюдателя о процессе p сеанса t вне s.mu (заводится при первой встрече);
// nil — не трогать: чужой сеанс, свой процесс PC Block, процесс другого пользователя или с
// непрочитанным пользователем (программа администратора через UAC в сеансе ребёнка, R19;
// проверяется пользователь, а не повышение прав), нечитаемый путь.
func (s *Service) watchEntryOf(t *watchTarget, p ProcInfo, gen uint64) (*watchEntry, int) {
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
		norm := programs.Normalize(p.Path) // настоящий путь, не ключ
		if norm == "" {
			return nil, skipUnreadable
		}
		e = s.newWatchEntry(t, p, norm, gen, 0)
		s.watch.procs[key] = e
	}
	e.seen = gen
	return e, skipNone
}

// watchDecide решает судьбу процесса p сеанса t с записью e (watchEntryOf; свойства файла уже
// прочитаны, если решение их ждёт) вне s.mu: nil — не трогать. Вне WatchDir в тестовом режиме,
// класс «система» и скрытая — не трогать; «незнакомая» и «запрещённая» — закрыть; разрешённая
// категория — закрыть, только если её сейчас нельзя и правило закрывает её (игры при CloseGames,
// всё, кроме учёбы, при StudyOnly; R5, R8). Под экраном «Блокировать экран» закрываются только игры.
func (s *Service) watchDecide(t *watchTarget, p ProcInfo, e *watchEntry) *watchAct {
	c := s.classifier
	if dir := s.cfg.Test.WatchDir; s.cfg.Test.Enabled && !strings.HasPrefix(e.norm, strings.TrimSuffix(dir, `\`)+`\`) {
		return nil // тестовый режим: закрывается только папка тестовых программ (RM3-8)
	}
	res := c.Classify(e.path, e.fp, e.pub)
	if !e.classified {
		e.classified, e.forbiddenAtStart = true, res.Verdict == programs.VerdictForbidden
	}
	if res.Verdict == programs.VerdictSystem || res.Verdict == programs.VerdictHidden {
		return nil // скрытую родителем не закрывает ни одно правило (D-57, RF9-2)
	}
	a := &watchAct{t: t, p: p, e: e, res: res, again: e.closed}
	switch res.Verdict {
	case programs.VerdictUnknown:
		a.plateKind = ipc.BlockedUnknown
	case programs.VerdictForbidden:
		a.plateKind, a.auto = ipc.BlockedForbidden, e.forbiddenAtStart && !e.manual
	default:
		a.cat = coreCategory(res.Category)
		if t.d.Allows(a.cat) {
			return nil
		}
		switch {
		case t.d.StudyOnly && a.cat != core.CatStudy && !isBrave(e.norm, res):
			// Brave в «только учёбе» не закрывается: вкладки не из «Учёбы» закрывает siteGuard (S-14).
			a.plateKind = ipc.BlockedStudyOnly
		case t.d.CloseGames && a.cat == core.CatGames:
			a.plateKind = ipc.BlockedGamesOver
		default:
			return nil // под экраном блокировки остальное остаётся под экраном (R8)
		}
	}
	return a
}

// newWatchEntry — память о впервые увиденном процессе p и решение «ручной или сам» (R13, D-65).
// Родитель того же exe (многопроцессная программа): решение главного процесса (его запись
// заводится здесь же, если он в списке дальше), записи только у него (quiet).
func (s *Service) newWatchEntry(t *watchTarget, p ProcInfo, norm string, gen uint64, depth int) *watchEntry {
	roots := s.classifier.Roots()
	e := &watchEntry{session: t.session, path: p.Path, norm: norm, key: roots.StableKey(norm), seen: gen}
	e.parent = launchParentOf(p, t.byPID, t.sid)
	e.facts = launchFacts{
		At: t.at, LogonAt: t.logonAt, InputAt: t.inputAt, Self: norm, Parent: e.parent,
		Shell: launchShell(roots.SystemRoot), Foreground: t.fg,
		Store: launchStoreApp(norm, roots.StoreDir()), Broker: launchBroker(roots.SystemRoot),
	}
	if e.parent != "" && e.parent == norm && depth < maxLaunchDepth {
		q := t.byPID[p.ParentPID]
		pk := procKey{pid: q.PID, stamp: q.Stamp}
		pe := s.watch.procs[pk]
		if pe == nil {
			pe = s.newWatchEntry(t, q, norm, gen, depth+1)
			s.watch.procs[pk] = pe
		}
		e.manual, e.why, e.quiet = pe.manual, "дочерний процесс того же exe: "+pe.why, true
		return e
	}
	e.manual, e.why = launchKind(e.facts)
	return e
}

// inputMoment — момент последнего ввода по снимку стража от sampleAt. В тестовом режиме страж
// присылает простой в часах ядра (guard.ScaleIdle, умножен на ускорение): служба делит его обратно,
// иначе ввод 3 с назад при ускорении 60 выглядел бы как 3 мин назад.
func (s *Service) inputMoment(sampleAt time.Time, agoMs int64) time.Time {
	if s.cfg.Test.Enabled && s.cfg.Test.ClockSpeed > 1 {
		agoMs /= int64(s.cfg.Test.ClockSpeed)
	}
	return sampleAt.Add(-time.Duration(agoMs) * time.Millisecond)
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
		// Не чаще раза в час и только при новом числе: иначе строка каждую минуту забивает журнал
		// (у заказчика 1550 строк из 1843 за две недели).
		s.logCountRate(fmt.Sprint("watch-otheruser|", session), n, real, otherUserLogEvery,
			"наблюдатель: %d процессов в сеансе %d идут от имени другого пользователя или пользователь не прочитан, они не трогаются", n, session)
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
		s.vlogf("наблюдатель закрыл %s (PID %d, родитель %d %q) в сеансе %d: %s; запуск ручной: %v (%s)", a.e.norm, a.p.PID, a.p.ParentPID, a.e.parent, a.t.session, a.res.Verdict, a.e.manual, a.e.why)
		s.recordClosed(cs, a, real)
	}
	s.settleLaunches(real) // после ручных записей этого прохода: «сама» не пишется рядом с ручной
}

// settleLaunches решает отложенные «запустилась сама» (launchPending): снимок стража новее
// решения показал ввод не позже чем через watchEvery после того, как процесс увиден, и запуск по
// остальным признакам ручной — плашка и нарушение; срок вышел — «запускалась сама», если в
// последние watchRepeatEvery не было ручной попытки того же exe (её дочерние процессы), раз в
// день на программу. Ввод позже не мог вызвать запуск и не считается.
func (s *Service) settleLaunches(real time.Time) {
	keep := s.launchWait[:0]
	for _, p := range s.launchWait {
		a := &p.a
		cs := s.children[a.t.childID]
		if cs == nil {
			continue
		}
		if sm := cs.sample; cs.haveSample && cs.sampleSess == a.t.session && cs.sampleAt.After(a.t.sampleAt) && sm.SessionActive && sm.LastInputAgoMs >= 0 {
			f := a.e.facts
			f.InputAt = s.inputMoment(cs.sampleAt, sm.LastInputAgoMs)
			if !f.InputAt.After(f.At.Add(watchEvery)) {
				if manual, why := launchKind(f); manual {
					a.e.manual, a.e.why, a.auto = true, why+" (по следующему снимку стража)", false
					s.vlogf("наблюдатель: запуск %s (PID %d) ручной по следующему снимку: %s", a.e.norm, a.p.PID, why)
					s.recordClosed(cs, a, real)
					continue
				}
			}
		}
		if real.Before(p.until) {
			keep = append(keep, p)
			continue
		}
		if at, ok := s.watchAt[fmt.Sprintf("ev|%d|%s|%s", cs.child.ID, evProgramForbiddenTry, a.e.key)]; ok && real.Sub(at) < watchRepeatEvery {
			s.vlogf("наблюдатель: %s (PID %d) без записи «сама»: рядом ручной запуск того же exe", a.e.norm, a.p.PID)
			continue
		}
		if s.autoClosedOnce(cs.child.ID, a.t.d.Day, a.e.key, real) {
			_, details := closedName(a)
			s.journal.Write(cs.child.ID, evProgramAutoClosed, details)
		}
	}
	s.launchWait = keep
}

// closedName — название закрытой программы и подробности записи журнала о ней.
func closedName(a *watchAct) (string, map[string]any) {
	name := a.res.Name
	if name == "" {
		name = programs.NameFromPath(a.e.path)
	}
	return name, map[string]any{"name": name, "path": a.e.key}
}

// recordClosed — записи о закрытой программе: журнал, «Ждут решения» и плашка.
func (s *Service) recordClosed(cs *childState, a *watchAct, real time.Time) {
	name, details := closedName(a)
	var kind, text string
	plate := ipc.BlockedPlate{Kind: a.plateKind, ShowS: blockedPlateShowS, Path: a.e.key}
	switch a.plateKind {
	case ipc.BlockedUnknown:
		kind, text = evProgramPending, ""
		plate.Text, plate.CanAsk, plate.ShowS = fmt.Sprintf(textPlateUnknown, name), true, unknownPlateShowS
		// В «Ждут решения» своих названий нет (D-59): название из файла, иначе решение родителя
		// записало бы своё название как название из файла.
		pendName := name
		if p, ok, err := s.cfg.Store.GetProgram(a.e.key); err == nil && ok {
			pendName = programName(p.Name, p.Path)
		}
		pend := PendingProgram{Path: a.e.key, Name: pendName, Publisher: a.publisher, Description: a.descr, WasCategory: a.res.WasCategory}
		if _, err := s.cfg.Store.AddPendingAttempt(pend, cs.child.ID, real.UnixMilli()); err != nil {
			s.logRate("pending-add", "попытка запуска %s не записана в «Ждут решения»: %v", a.e.norm, err)
		}
	case ipc.BlockedForbidden:
		if a.e.quiet {
			return // дочерний процесс того же exe: записи у главного процесса
		}
		if a.auto {
			// Запустилась сама (R13, D-65): ребёнку ничего, нарушения нет; родителю раз в день
			// после уточнения (settleLaunches).
			s.launchWait = append(s.launchWait, launchPending{a: *a, until: real.Add(launchSettle)})
			return
		}
		kind = evProgramForbiddenTry
		plate.Text = fmt.Sprintf(textPlateForbidden, name)
		if _, ok := s.classifier.Catalog().ForbiddenBrowser(a.e.norm); ok {
			// Браузер, кроме Brave (RM4-8, 8.2.5 п. 4): при любом источнике запрета.
			plate.Kind, plate.Text = ipc.BlockedBrowser, textPlateBrowser
		}
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
	if s.watchOnce(fmt.Sprintf("ev|%d|%s|%s", cs.child.ID, kind, a.e.key), watchRepeatEvery, real) {
		s.journal.Write(cs.child.ID, kind, details)
	}
	s.blockedPlate(a.t.session, cs.child.ID, plate, real)
}

// autoClosedDay — запрещённые программы, уже записанные как «запускалась сама» в день day.
type autoClosedDay struct {
	day   string
	paths map[string]bool
}

// autoClosedOnce — true, если о запрещённой программе path, запустившейся сама, сегодня (день
// учёта day ребёнка childID; пустой — местная дата real) ещё не писали (R13, D-65: одна запись в
// день на программу и ребёнка). Память только в службе: после её перезапуска запись может
// повториться в тот же день. Вызывать под s.mu.
func (s *Service) autoClosedOnce(childID int64, day, path string, real time.Time) bool {
	if day == "" {
		day = real.In(s.cfg.Loc).Format(time.DateOnly)
	}
	if s.autoClosed == nil {
		s.autoClosed = map[int64]*autoClosedDay{}
	}
	d := s.autoClosed[childID]
	if d == nil || d.day != day {
		d = &autoClosedDay{day: day, paths: map[string]bool{}}
		s.autoClosed[childID] = d
	}
	if d.paths[path] {
		return false
	}
	d.paths[path] = true
	return true
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
