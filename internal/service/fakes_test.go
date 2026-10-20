package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"pcblock/internal/auth"
	"pcblock/internal/core"
)

// --- хранилище ------------------------------------------------------------------------

// fakeStore — хранилище в памяти. Поля fail* включают ошибки, чтобы проверять безопасное поведение (R18).
type fakeStore struct {
	mu       sync.Mutex
	children map[int64]*Child
	removed  map[int64]bool
	nextID   int64
	limits   map[int64][7]core.DayLimit
	days     map[string]core.DayState // "id|день"
	saves    []savedDay               // история SaveDayState
	events   []StoredEvent
	auth     *Auth
	settings map[string]string
	closed   bool

	failLimits bool // GetDayLimits возвращает ошибку
	failSave   bool // SaveDayState возвращает ошибку
	failEvents bool // AppendEvent возвращает ошибку
	failAuth   bool // GetAuth возвращает ошибку
}

type savedDay struct {
	childID int64
	st      core.DayState
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		children: map[int64]*Child{}, removed: map[int64]bool{}, nextID: 1,
		limits: map[int64][7]core.DayLimit{}, days: map[string]core.DayState{}, settings: map[string]string{},
	}
}

var errFake = errors.New("поддельная ошибка хранилища")

func (f *fakeStore) UpsertChild(sid, name string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if sid == "" || name == "" {
		return 0, errors.New("нужны SID и имя")
	}
	for id, c := range f.children {
		if c.SID == sid {
			c.Name = name
			delete(f.removed, id)
			return id, nil
		}
	}
	id := f.nextID
	f.nextID++
	f.children[id] = &Child{ID: id, SID: sid, Name: name}
	return id, nil
}

func (f *fakeStore) ListChildren() ([]Child, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []Child{}
	for id, c := range f.children {
		if !f.removed[id] {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeStore) RemoveChild(id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.children[id]; !ok {
		return errors.New("нет такого ребёнка")
	}
	f.removed[id] = true
	return nil
}

func (f *fakeStore) SetBypass(id int64, detail string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.children[id]
	if !ok {
		return errors.New("нет такого ребёнка")
	}
	c.BypassDetail = detail
	return nil
}

func (f *fakeStore) SetDayLimits(id int64, week [7]core.DayLimit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limits[id] = week
	return nil
}

func (f *fakeStore) GetDayLimits(id int64) ([7]core.DayLimit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failLimits {
		return [7]core.DayLimit{}, errFake
	}
	return f.limits[id], nil
}

func dayKey(id int64, day string) string { return fmt.Sprintf("%d|%s", id, day) }

func (f *fakeStore) LoadDayState(id int64, day string) (core.DayState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.days[dayKey(id, day)]; ok {
		return st, nil
	}
	return core.DayState{Day: day}, nil
}

func (f *fakeStore) SaveDayState(id int64, st core.DayState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSave {
		return errFake
	}
	f.days[dayKey(id, st.Day)] = st
	f.saves = append(f.saves, savedDay{id, st})
	return nil
}

func (f *fakeStore) AppendEvent(e StoredEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failEvents {
		return errFake
	}
	e.ID = int64(len(f.events) + 1)
	f.events = append(f.events, e)
	return nil
}

func (f *fakeStore) ListEvents(q EventQuery) ([]StoredEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []StoredEvent{}
	for i := len(f.events) - 1; i >= 0; i-- { // новые сверху
		e := f.events[i]
		if q.ChildID != 0 && e.ChildID != q.ChildID {
			continue
		}
		out = append(out, e)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeStore) GetAuth() (Auth, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAuth {
		return Auth{}, false, errFake
	}
	if f.auth == nil {
		return Auth{}, false, nil
	}
	return *f.auth, true, nil
}

func (f *fakeStore) SetAuth(a Auth) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a.Hash == "" {
		return errors.New("пустой хеш")
	}
	f.auth = &a
	return nil
}

func (f *fakeStore) GetSetting(key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.settings[key]
	return v, ok, nil
}

func (f *fakeStore) SetSetting(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings[key] = value
	return nil
}

func (f *fakeStore) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// eventKinds возвращает виды событий журнала по порядку записи.
func (f *fakeStore) eventKinds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, e := range f.events {
		out = append(out, e.Kind)
	}
	return out
}

// eventsOf возвращает события вида kind.
func (f *fakeStore) eventsOf(kind string) []StoredEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []StoredEvent{}
	for _, e := range f.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func (f *fakeStore) lastSavedTotal(childID int64) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.saves) - 1; i >= 0; i-- {
		if f.saves[i].childID == childID {
			return f.saves[i].st.TotalS, true
		}
	}
	return 0, false
}

// --- хеш пароля -----------------------------------------------------------------------

// fakeHasher: «хеш» — строка "H:"+пароль. Считает вызовы Verify.
type fakeHasher struct {
	mu      sync.Mutex
	verifys int
	broken  bool // Verify возвращает ошибку (испорченный хеш)
}

func (h *fakeHasher) Hash(pw string) (string, error) {
	if err := auth.CheckLength(pw); err != nil {
		return "", err
	}
	return "H:" + pw, nil
}

func (h *fakeHasher) Verify(hash, pw string) (bool, error) {
	h.mu.Lock()
	h.verifys++
	broken := h.broken
	h.mu.Unlock()
	if broken {
		return false, errors.New("хеш испорчен")
	}
	return hash == "H:"+pw, nil
}

// --- часы -----------------------------------------------------------------------------

// fakeClock — часы, которыми управляет тест. Ещё и «останавливаемые» (Pause/Resume), как ScaledClock.
type fakeClock struct {
	mu     sync.Mutex
	t      time.Time
	paused bool
}

func newClock(t time.Time) *fakeClock { return &fakeClock{t: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func (c *fakeClock) Pause() {
	c.mu.Lock()
	c.paused = true
	c.mu.Unlock()
}

func (c *fakeClock) Resume() {
	c.mu.Lock()
	c.paused = false
	c.mu.Unlock()
}

func (c *fakeClock) isPaused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused
}

// --- запуск процессов ------------------------------------------------------------------

// fakeProc — поддельный процесс.
type fakeProc struct {
	pid        uint32
	alive      bool
	terminated bool
	closed     bool
	l          *fakeLauncher
}

func (p *fakeProc) PID() uint32 { return p.pid }
func (p *fakeProc) Alive() bool {
	p.l.mu.Lock()
	defer p.l.mu.Unlock()
	return p.alive
}
func (p *fakeProc) Terminate() error {
	p.l.mu.Lock()
	defer p.l.mu.Unlock()
	p.alive, p.terminated = false, true
	return nil
}
func (p *fakeProc) Close() { p.l.mu.Lock(); p.closed = true; p.l.mu.Unlock() }

// launchRec — запись об одном запуске.
type launchRec struct {
	kind    ProcKind
	session uint32
	args    []string
	proc    *fakeProc
}

// fakeLauncher — поддельный SessionLauncher.
type fakeLauncher struct {
	mu        sync.Mutex
	sessions  []SessionInfo
	listErr   error
	launches  []launchRec
	nextPID   uint32
	launchErr error
	failNext  int // следующие N запусков не удаются
	logoffs   []uint32
	shutdowns int               // сколько раз просили выключить компьютер
	switches  []uint32          // сеансы, которым просили сменить пользователя
	stamps    map[uint32]uint64 // PID -> момент создания; нет записи: процесса нет
	killed    []uint32          // PID, завершённые через KillStale
	stale     map[uint32]uint64 // PID -> stamp, которые KillStale считает «теми же»
}

func newLauncher() *fakeLauncher {
	return &fakeLauncher{nextPID: 100, stamps: map[uint32]uint64{}, stale: map[uint32]uint64{}}
}

func (l *fakeLauncher) Sessions() ([]SessionInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listErr != nil {
		return nil, l.listErr
	}
	return append([]SessionInfo(nil), l.sessions...), nil
}

func (l *fakeLauncher) Launch(kind ProcKind, session uint32, args []string) (Proc, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failNext > 0 {
		l.failNext--
		return nil, errors.New("не запустилось")
	}
	if l.launchErr != nil {
		return nil, l.launchErr
	}
	l.nextPID++
	p := &fakeProc{pid: l.nextPID, alive: true, l: l}
	l.stamps[p.pid] = uint64(p.pid) * 1000
	l.launches = append(l.launches, launchRec{kind, session, append([]string(nil), args...), p})
	return p, nil
}

func (l *fakeLauncher) Logoff(session uint32) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logoffs = append(l.logoffs, session)
	return nil
}

func (l *fakeLauncher) Shutdown() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.shutdowns++
	return nil
}

func (l *fakeLauncher) SwitchUser(session uint32) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.switches = append(l.switches, session)
	return nil
}

func (l *fakeLauncher) Stamp(pid uint32) (uint64, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.stamps[pid]
	return s, ok
}

func (l *fakeLauncher) KillStale(pid uint32, stamp uint64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := l.stale[pid]; ok && s == stamp {
		l.killed = append(l.killed, pid)
		delete(l.stale, pid)
		return true
	}
	return false
}

// setSessions задаёт список сеансов.
func (l *fakeLauncher) setSessions(s ...SessionInfo) {
	l.mu.Lock()
	l.sessions = s
	l.mu.Unlock()
}

// launched возвращает запуски вида kind в сеансе (session 0: любой).
func (l *fakeLauncher) launched(kind ProcKind, session uint32) []launchRec {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []launchRec{}
	for _, r := range l.launches {
		if r.kind == kind && (session == 0 || r.session == session) {
			out = append(out, r)
		}
	}
	return out
}

// aliveOf возвращает живые процессы вида kind в сеансе.
func (l *fakeLauncher) aliveOf(kind ProcKind, session uint32) []*fakeProc {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []*fakeProc{}
	for _, r := range l.launches {
		if r.kind == kind && r.session == session && r.proc.alive {
			out = append(out, r.proc)
		}
	}
	return out
}

// kill имитирует принудительное завершение процесса (taskkill /F).
func (l *fakeLauncher) kill(p *fakeProc) {
	l.mu.Lock()
	p.alive = false
	l.mu.Unlock()
}

// --- прочие порты ---------------------------------------------------------------------

type fakeAdmins struct {
	mu     sync.Mutex
	admins map[string]bool
	errs   map[string]error
	calls  map[string]int
}

func newAdmins() *fakeAdmins {
	return &fakeAdmins{admins: map[string]bool{}, errs: map[string]error{}, calls: map[string]int{}}
}

func (a *fakeAdmins) IsAdmin(sid string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls[sid]++
	if err := a.errs[sid]; err != nil {
		return false, err
	}
	return a.admins[sid], nil
}

func (a *fakeAdmins) set(sid string, admin bool) {
	a.mu.Lock()
	a.admins[sid] = admin
	a.mu.Unlock()
}

type fakeAccounts struct{ list []AccountInfo }

func (a *fakeAccounts) List() ([]AccountInfo, error) {
	return append([]AccountInfo(nil), a.list...), nil
}

type fakePolicy struct {
	mu    sync.Mutex
	state map[string]bool
	log   []string
	err   error
}

func newPolicy() *fakePolicy { return &fakePolicy{state: map[string]bool{}} }

func (p *fakePolicy) SetTaskMgrDisabled(sid string, disabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.state[sid] = disabled
	p.log = append(p.log, fmt.Sprintf("%s=%v", sid, disabled))
	return nil
}

func (p *fakePolicy) disabled(sid string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state[sid]
}

// containsAll сообщает, что в s есть все подстроки.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
