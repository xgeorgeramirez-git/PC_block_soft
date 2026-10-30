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

	schedule map[int64][]core.Interval // интервалы по ребёнку (M2)
	bonus    map[int64][]BonusRecord   // история копилки по ребёнку (M2)
	promos   []*fakePromo              // промокоды (R26, D-50)

	failLimits   bool // GetDayLimits возвращает ошибку
	failSave     bool // SaveDayState возвращает ошибку
	failEvents   bool // AppendEvent возвращает ошибку
	failAuth     bool // GetAuth возвращает ошибку
	failSchedule bool // GetSchedule возвращает ошибку
	failEndMode  bool // GetEndMode возвращает ошибку
	failPrograms bool // ListPrograms возвращает ошибку

	endModes map[int64]string           // режим «Когда время вышло» (M3); нет записи — "lock"
	programs map[string]Program         // программы по пути (M3)
	nextProg int64                      // следующий номер программы
	pending  map[string]*PendingProgram // «Ждут решения» по пути (M3)
	usage    map[string]int             // учёт: "id|день|категория|вид|предмет" → секунды (M3)
	usageLog []savedUsage               // история SaveDayAndUsage
}

type savedUsage struct {
	childID int64
	st      core.DayState
	rows    []UsageRow
}

type savedDay struct {
	childID int64
	st      core.DayState
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		children: map[int64]*Child{}, removed: map[int64]bool{}, nextID: 1,
		limits: map[int64][7]core.DayLimit{}, days: map[string]core.DayState{}, settings: map[string]string{},
		schedule: map[int64][]core.Interval{}, bonus: map[int64][]BonusRecord{},
		endModes: map[int64]string{}, programs: map[string]Program{}, nextProg: 1,
		pending: map[string]*PendingProgram{}, usage: map[string]int{},
	}
}

func (f *fakeStore) SetEndMode(id int64, mode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if mode != "lock" && mode != "study_only" {
		return errors.New("неверный режим")
	}
	if _, ok := f.children[id]; !ok {
		return errors.New("нет такого ребёнка")
	}
	f.endModes[id] = mode
	return nil
}

func (f *fakeStore) GetEndMode(id int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failEndMode {
		return "", errFake
	}
	if m, ok := f.endModes[id]; ok {
		return m, nil
	}
	return "lock", nil
}

// --- программы и «Ждут решения» (M3) --------------------------------------------------

var fakeCategories = map[string]bool{"games": true, "study": true, "other": true, "blocked": true, "system": true}

func (f *fakeStore) putProgram(p Program) {
	if old, ok := f.programs[p.Path]; ok {
		p.ID = old.ID
	} else {
		p.ID = f.nextProg
		f.nextProg++
	}
	f.programs[p.Path] = p
}

func (f *fakeStore) UpsertProgram(p Program) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p.Path == "" || p.Name == "" || !fakeCategories[p.Category] {
		return errFake
	}
	f.putProgram(p)
	return nil
}

func (f *fakeStore) RestoreProgram(p Program) error { return f.UpsertProgram(p) }

func (f *fakeStore) SeedPrograms(ps []Program) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range ps {
		if p.Path == "" || p.Name == "" || !fakeCategories[p.Category] {
			return 0, errFake
		}
	}
	added := 0
	for _, p := range ps {
		if _, ok := f.programs[p.Path]; !ok {
			f.putProgram(p)
			added++
		}
	}
	return added, nil
}

func (f *fakeStore) GetProgram(path string) (Program, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.programs[path]
	return p, ok, nil
}

func (f *fakeStore) ListPrograms() ([]Program, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPrograms {
		return nil, errFake
	}
	out := make([]Program, 0, len(f.programs))
	for _, p := range f.programs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func (f *fakeStore) SetProgramsCategory(paths []string, category string, at int64) ([]Program, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !fakeCategories[category] {
		return nil, errFake
	}
	changed := []Program{}
	for _, path := range paths {
		p, ok := f.programs[path]
		if !ok || (p.Category == category && p.Source == "parent") {
			continue
		}
		changed = append(changed, p)
		p.Category, p.Source, p.DecidedAt = category, "parent", at
		f.programs[path] = p
	}
	return changed, nil
}

func (f *fakeStore) DeleteProgram(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.programs[path]; !ok {
		return errors.New("нет такой программы")
	}
	delete(f.programs, path)
	return nil
}

func (f *fakeStore) AddPendingAttempt(p PendingProgram, childID, at int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p.Path == "" || p.Name == "" {
		return 0, errFake
	}
	if childID != 0 {
		if _, ok := f.children[childID]; !ok {
			return 0, errFake
		}
	}
	cur, ok := f.pending[p.Path]
	if !ok {
		cur = &PendingProgram{Path: p.Path, Name: p.Name, Publisher: p.Publisher, Description: p.Description, WasCategory: p.WasCategory,
			FirstChildID: childID, FirstAttemptAt: at, LastAttemptAt: at, Children: []PendingChild{}}
		f.pending[p.Path] = cur
	} else {
		cur.Name = p.Name
		if p.Publisher != "" {
			cur.Publisher = p.Publisher
		}
		if p.Description != "" {
			cur.Description = p.Description
		}
		if p.WasCategory != "" {
			cur.WasCategory = p.WasCategory
		}
		cur.LastAttemptAt = max(cur.LastAttemptAt, at)
	}
	cur.Attempts++
	if childID != 0 {
		found := false
		for i := range cur.Children {
			if cur.Children[i].ChildID == childID {
				cur.Children[i].Attempts++
				cur.Children[i].LastAttemptAt = max(cur.Children[i].LastAttemptAt, at)
				found = true
			}
		}
		if !found {
			cur.Children = append(cur.Children, PendingChild{ChildID: childID, Attempts: 1, LastAttemptAt: at})
		}
	}
	return cur.Attempts, nil
}

// pendingCopy копирует ждущую программу с детьми, новые сверху.
func pendingCopy(p *PendingProgram) PendingProgram {
	out := *p
	out.Children = append([]PendingChild{}, p.Children...)
	sort.SliceStable(out.Children, func(i, j int) bool { return out.Children[i].LastAttemptAt > out.Children[j].LastAttemptAt })
	return out
}

func (f *fakeStore) ListPending() ([]PendingProgram, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PendingProgram, 0, len(f.pending))
	for _, p := range f.pending {
		out = append(out, pendingCopy(p))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastAttemptAt != out[j].LastAttemptAt {
			return out[i].LastAttemptAt > out[j].LastAttemptAt
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func (f *fakeStore) ResolvePending(path string) (PendingProgram, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pending[path]
	if !ok {
		return PendingProgram{}, false, nil
	}
	delete(f.pending, path)
	return pendingCopy(p), true, nil
}

func (f *fakeStore) RestorePending(p PendingProgram) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p.Path == "" || p.Name == "" {
		return errFake
	}
	cur, ok := f.pending[p.Path]
	if !ok {
		cp := pendingCopy(&p)
		f.pending[p.Path] = &cp
		return nil
	}
	cur.Attempts += p.Attempts
	if cur.WasCategory == "" { // как storage: прежняя категория не теряется
		cur.WasCategory = p.WasCategory
	}
	if p.FirstAttemptAt < cur.FirstAttemptAt {
		cur.FirstAttemptAt, cur.FirstChildID = p.FirstAttemptAt, p.FirstChildID
	}
	cur.LastAttemptAt = max(cur.LastAttemptAt, p.LastAttemptAt)
	for _, c := range p.Children {
		found := false
		for i := range cur.Children {
			if cur.Children[i].ChildID == c.ChildID {
				cur.Children[i].Attempts += c.Attempts
				cur.Children[i].LastAttemptAt = max(cur.Children[i].LastAttemptAt, c.LastAttemptAt)
				found = true
			}
		}
		if !found {
			cur.Children = append(cur.Children, c)
		}
	}
	return nil
}

// --- учёт по программам (M3) ----------------------------------------------------------

var fakeUsageCats = map[string]bool{"games": true, "video_social": true, "study": true, "other": true}

func usageKey(id int64, u UsageRow) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s", id, u.Day, u.Category, u.Kind, u.Subject)
}

func (f *fakeStore) AddUsage(id int64, day, category, kind, subject string, seconds int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := UsageRow{Day: day, Category: category, Kind: kind, Subject: subject, Seconds: seconds}
	if !fakeUsageCats[category] || subject == "" || seconds < 0 || day == "" {
		return errFake
	}
	f.usage[usageKey(id, u)] += seconds
	return nil
}

func (f *fakeStore) SaveDayAndUsage(id int64, st core.DayState, rows []UsageRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSave {
		return errFake
	}
	cp := make([]UsageRow, 0, len(rows))
	for _, u := range rows {
		if u.Day == "" {
			u.Day = st.Day
		}
		if !fakeUsageCats[u.Category] || u.Subject == "" || u.Seconds < 0 {
			return errFake
		}
		cp = append(cp, u)
	}
	f.days[dayKey(id, st.Day)] = st
	f.saves = append(f.saves, savedDay{id, st})
	for _, u := range cp {
		f.usage[usageKey(id, u)] += u.Seconds
	}
	f.usageLog = append(f.usageLog, savedUsage{id, st, cp})
	return nil
}

func (f *fakeStore) ProgramSeconds(fromDay string) (map[string]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, sec := range f.usage {
		parts := strings.SplitN(k, "|", 5)
		if parts[3] == "program" && parts[1] >= fromDay {
			out[parts[4]] += sec
		}
	}
	return out, nil
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
			cp := *c
			cp.EndMode = "lock"
			if m, ok := f.endModes[id]; ok {
				cp.EndMode = m
			}
			out = append(out, cp)
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

func (f *fakeStore) GetSchedule(id int64) ([]core.Interval, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSchedule {
		return nil, errFake
	}
	out := make([]core.Interval, len(f.schedule[id]))
	copy(out, f.schedule[id])
	if out == nil {
		out = []core.Interval{}
	}
	return out, nil
}

func (f *fakeStore) SetSchedule(id int64, intervals []core.Interval) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]core.Interval, len(intervals))
	copy(cp, intervals)
	f.schedule[id] = cp
	return nil
}

func (f *fakeStore) AppendBonus(id int64, deltaMin int, reason string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if reason == "" || deltaMin == 0 {
		return 0, errFake
	}
	cur := 0
	if h := f.bonus[id]; len(h) > 0 {
		cur = h[len(h)-1].Balance
	}
	after := cur + deltaMin
	if after < 0 || after > 600 {
		return 0, errFake
	}
	f.bonus[id] = append(f.bonus[id], BonusRecord{AtMs: timeNowService(), DeltaMin: deltaMin, Reason: reason, Balance: after})
	return after, nil
}

func (f *fakeStore) GetBonusBalance(id int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if h := f.bonus[id]; len(h) > 0 {
		return h[len(h)-1].Balance, nil
	}
	return 0, nil
}

// fakePromo — промокод в памяти фейка.
type fakePromo struct {
	norm string
	rec  PromoRecord
}

func (f *fakeStore) CreatePromo(codeDisplay string, minutes int) (PromoRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	norm := core.NormalizePromoCode(codeDisplay)
	if norm == "" || !core.PromoMinutesValid(minutes) {
		return PromoRecord{}, errFake
	}
	for _, p := range f.promos {
		if p.norm == norm {
			return PromoRecord{}, errFake // уже есть такой код (UNIQUE)
		}
	}
	rec := PromoRecord{CodeDisplay: codeDisplay, Minutes: minutes, CreatedAtMs: timeNowService()}
	f.promos = append(f.promos, &fakePromo{norm: norm, rec: rec})
	return rec, nil
}

func (f *fakeStore) RedeemPromo(childID int64, code string, atMs int64) (int, string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	norm := core.NormalizePromoCode(code)
	if norm == "" {
		return 0, "", false, nil
	}
	for _, p := range f.promos {
		if p.norm == norm && p.rec.UsedAtMs == 0 {
			p.rec.UsedAtMs = atMs
			p.rec.UsedByChild = childID
			return p.rec.Minutes, p.rec.CodeDisplay, true, nil
		}
	}
	return 0, "", false, nil
}

func (f *fakeStore) ListPromos() ([]PromoRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PromoRecord, 0, len(f.promos))
	for i := len(f.promos) - 1; i >= 0; i-- {
		out = append(out, f.promos[i].rec)
	}
	return out, nil
}

func (f *fakeStore) ListBonus(id int64) ([]BonusRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.bonus[id]
	out := make([]BonusRecord, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- { // новые первыми
		out = append(out, src[i])
	}
	return out, nil
}

// timeNowService — момент для фейка копилки (тесты не проверяют конкретный момент).
func timeNowService() int64 { return 0 }

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

// --- сканер программ и свойства файлов (M3) ---------------------------------------------

// fakeScanner — снимок Program Files: список, ошибка и число вызовов. gate (не nil) держит Scan,
// пока его не закроют, entered получает сигнал о входе в Scan (сканер с задержкой, Д-2).
type fakeScanner struct {
	mu      sync.Mutex
	list    []FoundProgram
	err     error
	calls   int
	gate    chan struct{}
	entered chan struct{}
}

func (sc *fakeScanner) Scan() ([]FoundProgram, error) {
	sc.mu.Lock()
	sc.calls++
	gate, entered, err := sc.gate, sc.entered, sc.err
	list := append([]FoundProgram{}, sc.list...)
	sc.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	return list, nil
}

// hold включает задержку: Scan ждёт release.
func (sc *fakeScanner) hold() {
	sc.mu.Lock()
	sc.gate, sc.entered = make(chan struct{}), make(chan struct{}, 8)
	sc.mu.Unlock()
}

// release отпускает задержанные Scan.
func (sc *fakeScanner) release() {
	sc.mu.Lock()
	g := sc.gate
	sc.gate = nil
	sc.mu.Unlock()
	if g != nil {
		close(g)
	}
}

// waitEntered ждёт входа в Scan не дольше d.
func (sc *fakeScanner) waitEntered(d time.Duration) bool {
	sc.mu.Lock()
	e := sc.entered
	sc.mu.Unlock()
	select {
	case <-e:
		return true
	case <-time.After(d):
		return false
	}
}

func (sc *fakeScanner) callCount() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.calls
}

// fakeMeta — свойства файлов по пути (издатель, описание, отпечаток) и журнал запросов.
type fakeMeta struct {
	mu    sync.Mutex
	files map[string][3]string
	asked []string
	gate  chan struct{} // не nil: чтение ждёт, пока его закроют (медленный сетевой путь)
	in    chan struct{} // сигнал о входе в чтение
}

// hold — следующие чтения свойств ждут release (проверка, что их ждёт не блокировка службы).
func (m *fakeMeta) hold() {
	m.mu.Lock()
	m.gate, m.in = make(chan struct{}), make(chan struct{}, 1)
	m.mu.Unlock()
}

func (m *fakeMeta) release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gate != nil {
		close(m.gate)
		m.gate = nil
	}
}

func newMeta() *fakeMeta { return &fakeMeta{files: map[string][3]string{}} }

func (m *fakeMeta) set(path, publisher, description, fingerprint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = [3]string{publisher, description, fingerprint}
}

func (m *fakeMeta) meta(path string) (string, string, string) {
	m.mu.Lock()
	gate, in := m.gate, m.in
	m.mu.Unlock()
	if in != nil {
		select {
		case in <- struct{}{}:
		default:
		}
	}
	if gate != nil {
		<-gate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.asked = append(m.asked, path)
	f := m.files[path]
	return f[0], f[1], f[2]
}

func (m *fakeMeta) askedPaths() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.asked...)
}

// --- процессы сеансов для наблюдателя (M3) ----------------------------------------------

// fakeWatcher — процессы по сеансам, журнал завершений, ошибки списка, «защищённые» процессы,
// которые Kill не берёт, и задержка Kill (проверка, что канал не ждёт завершения).
type fakeWatcher struct {
	mu        sync.Mutex
	procs     []ProcInfo       // все живые процессы; List отдаёт по сеансу (или всё из listAll)
	listAll   map[uint32]bool  // List этого сеанса возвращает процессы всех сеансов (адаптер ошибся)
	listErr   map[uint32]error // ошибка List по сеансу
	protected map[uint32]bool  // PID, которые Kill не завершает
	killGate  chan struct{}    // не nil: Kill ждёт, пока его закроют
	killIn    chan struct{}    // сигнал о входе в Kill
	lists     map[uint32]int   // вызовы List по сеансу
	killed    []ProcInfo       // завершённые
	tries     int              // вызовы Kill
	listAt    []time.Time      // моменты вызовов List (настоящие часы)
	killAt    map[uint32]time.Time
	nextPID   uint32
}

func newWatcher() *fakeWatcher {
	return &fakeWatcher{listAll: map[uint32]bool{}, listErr: map[uint32]error{}, protected: map[uint32]bool{},
		lists: map[uint32]int{}, killAt: map[uint32]time.Time{}, nextPID: 5000}
}

func (w *fakeWatcher) List(session uint32) ([]ProcInfo, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lists[session]++
	w.listAt = append(w.listAt, time.Now())
	if err := w.listErr[session]; err != nil {
		return nil, err
	}
	out := []ProcInfo{}
	for _, p := range w.procs {
		if p.SessionID == session || w.listAll[session] {
			out = append(out, p)
		}
	}
	return out, nil
}

func (w *fakeWatcher) Kill(p ProcInfo) bool {
	w.mu.Lock()
	gate, in := w.killGate, w.killIn
	w.tries++
	w.mu.Unlock()
	if in != nil {
		select {
		case in <- struct{}{}:
		default:
		}
	}
	if gate != nil {
		<-gate
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.protected[p.PID] {
		return false
	}
	for i, q := range w.procs {
		if q.PID == p.PID && q.Stamp == p.Stamp {
			w.procs = append(w.procs[:i], w.procs[i+1:]...)
			w.killed = append(w.killed, q)
			w.killAt[q.PID] = time.Now()
			return true
		}
	}
	return false
}

// start запускает процесс с путём path в сеансе session от имени ребёнка (childSID) и возвращает его.
func (w *fakeWatcher) start(session uint32, path string) ProcInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextPID++
	p := ProcInfo{PID: w.nextPID, Stamp: uint64(w.nextPID) * 7, SessionID: session, Path: path, PathOK: path != "",
		UserSID: childSID, UserOK: true}
	w.procs = append(w.procs, p)
	return p
}

// startAs запускает процесс от имени пользователя sid (ok = false: пользователь не прочитан).
func (w *fakeWatcher) startAs(session uint32, path, sid string, ok bool) ProcInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextPID++
	p := ProcInfo{PID: w.nextPID, Stamp: uint64(w.nextPID) * 7, SessionID: session, Path: path, PathOK: true,
		UserSID: sid, UserOK: ok}
	w.procs = append(w.procs, p)
	return p
}

// add добавляет процесс как есть (свой PID, Stamp, нечитаемый путь).
func (w *fakeWatcher) add(p ProcInfo) {
	w.mu.Lock()
	w.procs = append(w.procs, p)
	w.mu.Unlock()
}

// exit — процесс завершился сам.
func (w *fakeWatcher) exit(p ProcInfo) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, q := range w.procs {
		if q.PID == p.PID && q.Stamp == p.Stamp {
			w.procs = append(w.procs[:i], w.procs[i+1:]...)
			return
		}
	}
}

// alive сообщает, что процесс ещё работает.
func (w *fakeWatcher) alive(p ProcInfo) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, q := range w.procs {
		if q.PID == p.PID && q.Stamp == p.Stamp {
			return true
		}
	}
	return false
}

// killedPaths — пути завершённых процессов по порядку.
func (w *fakeWatcher) killedPaths() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []string{}
	for _, p := range w.killed {
		out = append(out, p.Path)
	}
	return out
}

// listCalls — вызовы List по сеансам.
func (w *fakeWatcher) listCalls() map[uint32]int {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[uint32]int{}
	for k, v := range w.lists {
		out[k] = v
	}
	return out
}

// listCount — все вызовы List.
func (w *fakeWatcher) listCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.listAt)
}

// holdKill включает задержку Kill до releaseKill.
func (w *fakeWatcher) holdKill() {
	w.mu.Lock()
	w.killGate, w.killIn = make(chan struct{}), make(chan struct{}, 8)
	w.mu.Unlock()
}

func (w *fakeWatcher) releaseKill() {
	w.mu.Lock()
	g := w.killGate
	w.killGate = nil
	w.mu.Unlock()
	if g != nil {
		close(g)
	}
}
