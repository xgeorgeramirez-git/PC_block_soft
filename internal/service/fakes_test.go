package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/auth"
	"pcblock/internal/core"
)

// --- хранилище ------------------------------------------------------------------------

// fakeStore — хранилище в памяти. Поля fail* включают ошибки, чтобы проверять безопасное поведение (R18).
type fakeStore struct {
	mu        sync.Mutex
	children  map[int64]*Child
	removed   map[int64]bool
	nextID    int64
	limits    map[int64][7]core.DayLimit
	days      map[string]core.DayState // "id|день"
	saves     []savedDay               // история SaveDayState
	events    []StoredEvent
	lastEvID  int64        // номер последнего события: после DeleteChildEvents номера не повторяются (как id в базе)
	evQueries []EventQuery // запросы ListEvents по порядку (фильтр уходит в хранилище, RM5-5)
	auth      *Auth
	settings  map[string]string
	closed    bool

	schedule         map[int64][]core.Interval // интервалы по ребёнку (M2)
	bonus            map[int64][]BonusRecord   // история копилки по ребёнку (M2)
	promos           []*fakePromo              // промокоды (R26, D-50)
	failCreatePromos bool                      // CreatePromos возвращает ошибку, ничего не создав (RM5-12)
	failListPromos   bool                      // ListPromos возвращает ошибку

	failLimits   bool // GetDayLimits возвращает ошибку
	failSave     bool // SaveDayState возвращает ошибку
	failEvents   bool // AppendEvent возвращает ошибку
	failAuth     bool // GetAuth возвращает ошибку
	failSetAuth  bool // SetAuth возвращает ошибку (оборванная настройка, M4)
	failSchedule bool // GetSchedule возвращает ошибку
	failEndMode  bool // GetEndMode возвращает ошибку
	failPrograms bool // ListPrograms возвращает ошибку
	failMerge    bool // MergePrograms возвращает ошибку, ничего не меняя (RM4-11)
	mergeCalls   int  // сколько раз вызван MergePrograms

	endModes    map[int64]string           // режим «Когда время вышло» (M3); нет записи — "lock"
	programs    map[string]Program         // программы по пути (M3)
	nextProg    int64                      // следующий номер программы
	pending     map[string]*PendingProgram // «Ждут решения» по пути (M3)
	usage       map[string]int             // учёт: "id|день|категория|вид|предмет" → секунды (M3)
	failUsage   bool                       // DayUsage и PeriodUsage отвечают ошибкой (FB-10)
	failStats   bool                       // DaysSeries отвечает ошибкой (M5, RM5-4)
	failReset   bool                       // GetStatsReset отвечает ошибкой (M5, RM5-8)
	failUngrant bool                       // UngrantedDays отвечает ошибкой (FB-21)
	failBalance bool                       // GetBonusBalance отвечает ошибкой (FB-21)
	seriesCalls int                        // сколько раз прочитан ряд дней (одно чтение на ответ, RM5-4)
	resets      map[int64]StatsReset       // снимок очистки статистики по ребёнку (RM5-8)
	now         func() int64               // момент записи копилки, UTC мс; nil — 0
	usageLog    []savedUsage               // история SaveDayAndUsage

	breakRules  map[int64]core.BreakRules // «Отдых» по ребёнку (FB-14); нет записи — по умолчанию
	breakStates map[int64]core.BreakState // счёт к перерыву по ребёнку
	failBreak   bool                      // GetBreakRules возвращает ошибку

	sites     map[string]string // списки сайтов (M4): домен → blocked|study|video_social
	adultOff  map[int64]bool    // ребёнок с выключенным списком 18+ (M4); нет записи — включён
	failSites bool              // ListSiteRules возвращает ошибку (M4)
}

// fakeStarterVideo — стартовый набор «Видео и соцсети», как в миграции 009 (R14).
var fakeStarterVideo = []string{"youtube.com", "vk.com", "tiktok.com", "rutube.ru", "ok.ru", "telegram.org", "web.telegram.org", "discord.com", "twitch.tv"}

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
		sites: fakeSites(), adultOff: map[int64]bool{}, resets: map[int64]StatsReset{},
	}
}

func fakeSites() map[string]string {
	out := map[string]string{}
	for _, d := range fakeStarterVideo {
		out[d] = "video_social"
	}
	return out
}

var fakeSiteKinds = map[string]bool{"blocked": true, "study": true, "video_social": true}

// ListSiteRules — как storage.ListSiteRules: по списку, затем по домену.
func (f *fakeStore) ListSiteRules() ([]SiteRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSites {
		return nil, errors.New("диск недоступен")
	}
	out := []SiteRule{}
	for d, k := range f.sites {
		out = append(out, SiteRule{Domain: d, Kind: k})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Domain < out[j].Domain
	})
	return out, nil
}

// AddSiteRule — как storage.AddSiteRule: вставка, повтор или перенос той же записи.
func (f *fakeStore) AddSiteRule(domain, kind string) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if domain == "" || !fakeSiteKinds[kind] {
		return false, "", errors.New("неверный домен или список")
	}
	prev, ok := f.sites[domain]
	switch {
	case !ok:
		f.sites[domain] = kind
		return false, "", nil
	case prev == kind:
		return false, kind, nil
	default:
		f.sites[domain] = kind
		return true, prev, nil
	}
}

// RemoveSiteRule — как storage.RemoveSiteRule: прежний список для «Отменить».
func (f *fakeStore) RemoveSiteRule(domain string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if domain == "" {
		return "", false, errors.New("пустой домен")
	}
	prev, ok := f.sites[domain]
	if !ok {
		return "", false, nil
	}
	delete(f.sites, domain)
	return prev, true, nil
}

func (f *fakeStore) CountSiteRules(kind string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !fakeSiteKinds[kind] {
		return 0, errors.New("неверный список")
	}
	n := 0
	for _, k := range f.sites {
		if k == kind {
			n++
		}
	}
	return n, nil
}

// ReplaceSiteRules — как storage.ReplaceSiteRules: список целиком, ошибка — ничего не изменилось.
func (f *fakeStore) ReplaceSiteRules(kind string, domains []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !fakeSiteKinds[kind] {
		return errors.New("неверный список")
	}
	for _, d := range domains {
		if d == "" {
			return errors.New("пустой домен")
		}
	}
	for d, k := range f.sites {
		if k == kind {
			delete(f.sites, d)
		}
	}
	for _, d := range domains {
		f.sites[d] = kind
	}
	return nil
}

func (f *fakeStore) SetAdultFilter(id int64, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.children[id]; !ok {
		return errors.New("нет такого ребёнка")
	}
	if on {
		delete(f.adultOff, id)
	} else {
		f.adultOff[id] = true
	}
	return nil
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

// SetProgramsHidden — как storage.SetProgramsHidden: только флаг; нет в списке, системные и уже
// такие не трогаются; прежние значения изменённых.
func (f *fakeStore) SetProgramsHidden(paths []string, hidden bool) ([]Program, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	changed := []Program{}
	for _, path := range paths {
		p, ok := f.programs[path]
		if !ok || p.Hidden == hidden || p.Category == "system" {
			continue
		}
		changed = append(changed, p)
		p.Hidden = hidden
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

// MergePrograms — как storage: весь набор или ничего (проверка до записи).
func (f *fakeStore) MergePrograms(ms []ProgramMerge) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mergeCalls++
	if f.failMerge {
		return errFake
	}
	for _, m := range ms {
		if m.Key == "" || (m.Into != nil && (m.Into.Path != m.Key || m.Into.Name == "" || !fakeCategories[m.Into.Category])) ||
			(m.Pending != nil && (m.Pending.Path != m.Key || m.Pending.Name == "")) {
			return errFake
		}
	}
	for _, m := range ms {
		if m.Into != nil {
			for _, p := range m.From {
				if p != m.Key {
					delete(f.programs, p)
				}
			}
			f.putProgram(*m.Into)
		}
		for _, p := range append(append([]string{}, m.From...), m.PendingFrom...) {
			if p == m.Key {
				continue
			}
			for k, sec := range f.usage {
				parts := strings.SplitN(k, "|", 5)
				if len(parts) == 5 && parts[3] == usageKindProgram && parts[4] == p {
					delete(f.usage, k)
					f.usage[strings.Join(parts[:4], "|")+"|"+m.Key] += sec
				}
			}
		}
		for _, p := range m.PendingFrom {
			delete(f.pending, p)
		}
		if m.Pending != nil {
			cp := pendingCopy(m.Pending)
			f.pending[m.Key] = &cp
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

// DayUsage — как storage.DayUsage: учёт по программам ребёнка за день, по категории и пути;
// скрытые тоже (их прячет служба).
func (f *fakeStore) DayUsage(id int64, day string) ([]UsageRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failUsage {
		return nil, errFake
	}
	out := []UsageRow{}
	for k, sec := range f.usage {
		parts := strings.SplitN(k, "|", 5)
		if parts[0] == fmt.Sprint(id) && parts[1] == day {
			out = append(out, UsageRow{Day: day, Category: parts[2], Kind: parts[3], Subject: parts[4], Seconds: sec})
		}
	}
	sortUsage(out)
	return out, nil
}

// fakeDayOf разбирает ключ дня "id|день": ребёнок совпал — день и true.
func fakeDayOf(k string, id int64) (string, bool) {
	prefix := fmt.Sprint(id) + "|"
	if !strings.HasPrefix(k, prefix) {
		return "", false
	}
	return strings.TrimPrefix(k, prefix), true
}

// DaysSeries — как storage.DaysSeries: строки from ≤ день < before по возрастанию дня; нет — [].
func (f *fakeStore) DaysSeries(id int64, from, before string) ([]DayRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seriesCalls++
	if f.failStats {
		return nil, errFake
	}
	out := []DayRow{}
	for k, st := range f.days {
		day, ok := fakeDayOf(k, id)
		if !ok || day < from || day >= before {
			continue
		}
		out = append(out, DayRow{Day: day, TotalS: st.TotalS, GamesS: st.GamesS, VideoS: st.VideoS, StudyS: st.StudyS, ExtraMin: st.ExtraMin})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}

// FirstStatsDay — как storage.FirstStatsDay: самый ранний день ребёнка; нет строк — false.
func (f *fakeStore) FirstStatsDay(id int64) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	first, found := "", false
	for k := range f.days {
		if day, ok := fakeDayOf(k, id); ok && (!found || day < first) {
			first, found = day, true
		}
	}
	return first, found, nil
}

// GetStatsReset — как storage.GetStatsReset.
func (f *fakeStore) GetStatsReset(id int64) (StatsReset, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failReset {
		return StatsReset{}, false, errFake
	}
	r, ok := f.resets[id]
	return r, ok, nil
}

// ClearStats — как storage.ClearStats: весь учёт ребёнка, его дни до r.Day, строка дня ядра и снимок;
// день строки не день снимка или сбой записи (failSave) — ничего не изменилось.
func (f *fakeStore) ClearStats(id int64, day core.DayState, r StatsReset) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if day.Day != r.Day || r.Day == "" || f.failSave {
		return errFake
	}
	for k := range f.usage {
		if strings.HasPrefix(k, fmt.Sprint(id)+"|") {
			delete(f.usage, k)
		}
	}
	for k := range f.days {
		if d, ok := fakeDayOf(k, id); ok && d < r.Day {
			delete(f.days, k)
		}
	}
	f.days[dayKey(id, day.Day)] = day
	f.resets[id] = r
	return nil
}

// UngrantedDays — как storage.UngrantedDays: дни без автобонуса строго до before, по возрастанию.
func (f *fakeStore) UngrantedDays(id int64, before string) ([]core.DayState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failUngrant {
		return nil, errFake
	}
	out := []core.DayState{}
	for k, st := range f.days {
		if day, ok := fakeDayOf(k, id); ok && day < before && !st.BonusGranted {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}

// PeriodUsage — как storage.PeriodUsage: учёт по программам за from ≤ день < before, сложенный по
// категории и пути.
func (f *fakeStore) PeriodUsage(id int64, from, before string) ([]UsageRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failUsage {
		return nil, errFake
	}
	type key struct{ cat, kind, subject string }
	sum := map[key]int{}
	for k, sec := range f.usage {
		parts := strings.SplitN(k, "|", 5)
		if parts[0] == fmt.Sprint(id) && parts[1] >= from && parts[1] < before {
			sum[key{parts[2], parts[3], parts[4]}] += sec
		}
	}
	out := []UsageRow{}
	for k, sec := range sum {
		out = append(out, UsageRow{Category: k.cat, Kind: k.kind, Subject: k.subject, Seconds: sec})
	}
	sortUsage(out)
	return out, nil
}

// sortUsage — порядок storage.DayUsage и PeriodUsage: категория, вид (program, site), предмет.
func sortUsage(out []UsageRow) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Subject < out[j].Subject
	})
}

func (f *fakeStore) ProgramSeconds(fromDay string) (map[string]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, sec := range f.usage {
		parts := strings.SplitN(k, "|", 5)
		if parts[3] == "program" && parts[1] >= fromDay && !f.programs[parts[4]].Hidden {
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
			cp.AdultFilter = !f.adultOff[id]
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
	at := timeNowService()
	if f.now != nil {
		at = f.now()
	}
	f.bonus[id] = append(f.bonus[id], BonusRecord{AtMs: at, DeltaMin: deltaMin, Reason: reason, Balance: after})
	return after, nil
}

func (f *fakeStore) GetBonusBalance(id int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failBalance {
		return 0, errFake
	}
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

// ListPromos — как storage.ListPromos: действующие новые сверху (created_at, затем номер), затем
// использованные по моменту использования, новые сверху.
func (f *fakeStore) ListPromos() ([]PromoRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failListPromos {
		return nil, errFake
	}
	idx := make([]int, len(f.promos))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		pa, pb := f.promos[idx[a]].rec, f.promos[idx[b]].rec
		if (pa.UsedAtMs != 0) != (pb.UsedAtMs != 0) {
			return pa.UsedAtMs == 0
		}
		ka, kb := pa.CreatedAtMs, pb.CreatedAtMs
		if pa.UsedAtMs != 0 {
			ka, kb = pa.UsedAtMs, pb.UsedAtMs
		}
		if ka != kb {
			return ka > kb
		}
		return idx[a] > idx[b] // id DESC: позже добавленный — выше
	})
	out := make([]PromoRecord, 0, len(f.promos))
	for _, i := range idx {
		out = append(out, f.promos[i].rec)
	}
	return out, nil
}

// CreatePromos — как storage.CreatePromos: вся пачка или ничего (повтор с базой, в том числе с
// использованным, или внутри пачки; пустой код; минуты вне 1..600).
func (f *fakeStore) CreatePromos(codesDisplay []string, minutes int) ([]PromoRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCreatePromos {
		return nil, errFake
	}
	if !core.PromoMinutesValid(minutes) {
		return nil, errFake
	}
	seen := map[string]bool{}
	for _, p := range f.promos {
		seen[p.norm] = true
	}
	batch := make([]*fakePromo, 0, len(codesDisplay))
	for _, c := range codesDisplay {
		norm := core.NormalizePromoCode(c)
		if norm == "" || seen[norm] {
			return nil, errFake
		}
		seen[norm] = true
		batch = append(batch, &fakePromo{norm: norm, rec: PromoRecord{CodeDisplay: c, Minutes: minutes, CreatedAtMs: timeNowService()}})
	}
	out := make([]PromoRecord, 0, len(batch))
	for _, p := range batch {
		f.promos = append(f.promos, p)
		out = append(out, p.rec)
	}
	return out, nil
}

// ClearUsedPromos — как storage.ClearUsedPromos: удаляет использованные, число удалённых.
func (f *fakeStore) ClearUsedPromos() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.promos[:0]
	n := 0
	for _, p := range f.promos {
		if p.rec.UsedAtMs != 0 {
			n++
			continue
		}
		kept = append(kept, p)
	}
	f.promos = kept
	return n, nil
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
	f.lastEvID++
	e.ID = f.lastEvID
	f.events = append(f.events, e)
	return nil
}

// ListEvents — как storage.ListEvents: ребёнок, виды, FromMs <= at < ToMs, курсор строго раньше
// (BeforeAtMs, BeforeID); порядок (at, id) сверху вниз; предел как storage (0 — 200, не больше 1000).
func (f *fakeStore) ListEvents(q EventQuery) ([]StoredEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evQueries = append(f.evQueries, q)
	kinds := map[string]bool{}
	for _, k := range q.Kinds {
		kinds[k] = true
	}
	matched := []StoredEvent{}
	for _, e := range f.events {
		switch {
		case q.ChildID != 0 && e.ChildID != q.ChildID,
			q.FromMs != 0 && e.AtMs < q.FromMs,
			q.ToMs != 0 && e.AtMs >= q.ToMs,
			len(kinds) > 0 && !kinds[e.Kind],
			q.BeforeAtMs != 0 && !(e.AtMs < q.BeforeAtMs || (e.AtMs == q.BeforeAtMs && e.ID < q.BeforeID)):
			continue
		}
		matched = append(matched, e)
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].AtMs != matched[j].AtMs {
			return matched[i].AtMs > matched[j].AtMs
		}
		return matched[i].ID > matched[j].ID
	})
	limit := q.Limit
	switch {
	case limit <= 0:
		limit = 200
	case limit > 1000:
		limit = 1000
	}
	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

// DeleteChildEvents — как storage.DeleteChildEvents: события ребёнка; без ребёнка (0) остаются.
func (f *fakeStore) DeleteChildEvents(id int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.events[:0]
	n := 0
	for _, e := range f.events {
		if e.ChildID == id && id != 0 {
			n++
			continue
		}
		kept = append(kept, e)
	}
	f.events = kept
	return n, nil
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
	if a.Hash == "" || f.failSetAuth {
		return errors.New("пустой хеш или диск недоступен")
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

	// Правила Brave (T5): что лежит в ветке учётки, ошибка адаптера, учётки без загруженной ветки
	// (BraveRulesOK их не читает, T5-4) и история вызовов: "set <sid> N", "ok <sid>", "bad <sid>", "rm <sid>".
	brave    map[string][]BraveValue
	braveErr error
	unloaded map[string]bool
	braveLog []string
}

func newPolicy() *fakePolicy {
	return &fakePolicy{state: map[string]bool{}, brave: map[string][]BraveValue{}, unloaded: map[string]bool{}}
}

func (p *fakePolicy) SetBraveRules(sid string, r BraveRules) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.braveErr != nil {
		return p.braveErr
	}
	p.brave[sid] = r.Values()
	p.braveLog = append(p.braveLog, fmt.Sprintf("set %s %d", sid, len(p.brave[sid])))
	return nil
}

func (p *fakePolicy) BraveRulesOK(sid string, r BraveRules) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.braveErr != nil {
		return false, p.braveErr
	}
	if p.unloaded[sid] {
		return true, nil
	}
	set, del, delKeys := BravePlan(p.brave[sid], r.Values())
	if len(set)+len(del)+len(delKeys) == 0 {
		p.braveLog = append(p.braveLog, "ok "+sid)
		return true, nil
	}
	p.braveLog = append(p.braveLog, "bad "+sid)
	return false, nil
}

func (p *fakePolicy) RemoveBraveRules(sid string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.braveErr != nil {
		return p.braveErr
	}
	delete(p.brave, sid)
	p.braveLog = append(p.braveLog, "rm "+sid)
	return nil
}

// tamper убирает из ветки учётки запрет окна Tor (правка ветки чужими руками).
func (p *fakePolicy) tamper(sid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	keep := []BraveValue{}
	for _, v := range p.brave[sid] {
		if v.Name != "TorDisabled" {
			keep = append(keep, v)
		}
	}
	p.brave[sid] = keep
}

// braveOf — значения ветки учётки и есть ли ветка.
func (p *fakePolicy) braveOf(sid string) ([]BraveValue, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.brave[sid]
	return append([]BraveValue(nil), v...), ok
}

// braveCalls — история вызовов правил Brave с номера from.
func (p *fakePolicy) braveCalls(from int) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.braveLog[from:]...)
}

func (p *fakePolicy) braveCallN() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.braveLog)
}

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
	mu      sync.Mutex
	files   map[string][3]string
	asked   []string
	gate    chan struct{} // не nil: чтение ждёт, пока его закроют (медленный сетевой путь)
	in      chan struct{} // сигнал о входе в чтение
	entered int           // начатые чтения (и ждущие у gate)
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
	m.entered++
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

// enteredCount — сколько чтений свойств начато, включая ждущие у ворот hold.
func (m *fakeMeta) enteredCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entered
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
	linger    map[uint32]bool // PID, который после успешного Kill ещё один List живёт (завершается)
	dying     map[uint32]bool // Kill удался, процесс исчезнет после следующего List
}

func newWatcher() *fakeWatcher {
	return &fakeWatcher{listAll: map[uint32]bool{}, listErr: map[uint32]error{}, protected: map[uint32]bool{},
		lists: map[uint32]int{}, killAt: map[uint32]time.Time{}, nextPID: 5000,
		linger: map[uint32]bool{}, dying: map[uint32]bool{}}
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
	live := w.procs[:0:0]
	for _, p := range w.procs {
		if p.SessionID == session || w.listAll[session] {
			out = append(out, p)
		}
		if w.dying[p.PID] {
			delete(w.dying, p.PID) // показан последний раз: завершился
			continue
		}
		live = append(live, p)
	}
	w.procs = live
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
	if w.dying[p.PID] {
		return false // уже завершается: как настоящий адаптер, «не завершён этим вызовом»
	}
	for i, q := range w.procs {
		if q.PID == p.PID && q.Stamp == p.Stamp && w.linger[p.PID] {
			delete(w.linger, p.PID)
			w.dying[p.PID] = true
			w.killed = append(w.killed, q)
			w.killAt[q.PID] = time.Now()
			return true
		}
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

// startFrom запускает процесс с путём path от имени ребёнка; его родитель — процесс parent (R13, D-65).
func (w *fakeWatcher) startFrom(session uint32, path string, parent ProcInfo) ProcInfo {
	p := w.start(session, path)
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := range w.procs {
		if w.procs[i].PID == p.PID {
			w.procs[i].ParentPID = parent.PID
		}
	}
	p.ParentPID = parent.PID
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

// killTries — сколько вызовов Kill начато, включая ждущие у ворот holdKill.
func (w *fakeWatcher) killTries() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tries
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

// SetProgramName — как storage.SetProgramName: только своё название; прежнее значение.
func (f *fakeStore) SetProgramName(path, name string) (Program, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.programs[path]
	if !ok {
		return Program{}, false, nil
	}
	if len([]rune(name)) > 60 {
		return Program{}, false, errFake
	}
	prev := p
	p.CustomName = name
	f.programs[path] = p
	return prev, true, nil
}

// --- перерыв на отдых (FB-14) ------------------------------------------------------------

func (f *fakeStore) GetBreakRules(id int64) (core.BreakRules, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failBreak {
		return core.BreakRules{}, errFake
	}
	if b, ok := f.breakRules[id]; ok {
		return b, nil
	}
	return core.DefaultBreakRules(), nil
}

func (f *fakeStore) SetBreakRules(id int64, b core.BreakRules) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := b.Validate(); err != nil {
		return err
	}
	if f.breakRules == nil {
		f.breakRules = map[int64]core.BreakRules{}
	}
	f.breakRules[id] = b
	return nil
}

func (f *fakeStore) LoadBreakState(id int64) (core.BreakState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.breakStates[id], nil
}

func (f *fakeStore) SaveBreakState(id int64, b core.BreakState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.breakStates == nil {
		f.breakStates = map[int64]core.BreakState{}
	}
	f.breakStates[id] = b
	return nil
}

// Ловит: фейк хранилища расходится с storage в фильтрах журнала, курсоре, очистке статистики и
// журнала, пачке промокодов и порядке списка — тогда тесты службы зелёные на неправде (M5).
func TestFakeStoreMatchesStorageM5(t *testing.T) {
	f := newFakeStore()
	kinds := []string{"a", "b", "c"}
	for i := 0; i < 450; i++ {
		_ = f.AppendEvent(StoredEvent{ChildID: 1, AtMs: int64(1000 + i/3), Kind: kinds[i%3]})
	}
	_ = f.AppendEvent(StoredEvent{ChildID: 2, AtMs: 1100, Kind: "a"})
	_ = f.AppendEvent(StoredEvent{AtMs: 1100, Kind: "a"})

	q := EventQuery{ChildID: 1, Limit: 200}
	seen := map[int64]bool{}
	var sizes []int
	for page := 0; page < 5; page++ {
		evs, _ := f.ListEvents(q)
		if len(evs) == 0 {
			break
		}
		sizes = append(sizes, len(evs))
		for _, e := range evs {
			if seen[e.ID] {
				t.Fatalf("событие %d повторилось", e.ID)
			}
			seen[e.ID] = true
		}
		q.BeforeAtMs, q.BeforeID = evs[len(evs)-1].AtMs, evs[len(evs)-1].ID
	}
	if fmt.Sprint(sizes) != "[200 200 50]" || len(seen) != 450 {
		t.Errorf("страницы %v, всего %d; want [200 200 50] и 450", sizes, len(seen))
	}
	list := func(q EventQuery) string {
		evs, _ := f.ListEvents(q)
		var out []string
		for _, e := range evs {
			out = append(out, fmt.Sprintf("%d:%s", e.AtMs, e.Kind))
		}
		return strings.Join(out, " ")
	}
	if got, want := list(EventQuery{ChildID: 1, FromMs: 1001, ToMs: 1003, Kinds: []string{"a", "c"}}), "1002:c 1002:a 1001:c 1001:a"; got != want {
		t.Errorf("виды и время: %q, want %q", got, want)
	}
	if got := list(EventQuery{FromMs: 1100, ToMs: 1101, Kinds: []string{"a"}}); got != "1100:a 1100:a 1100:a" {
		t.Errorf("все дети в 1100: %q", got)
	}
	if n, _ := f.DeleteChildEvents(1); n != 450 {
		t.Errorf("DeleteChildEvents = %d, want 450", n)
	}
	_ = f.AppendEvent(StoredEvent{ChildID: 1, AtMs: 5, Kind: "journal_cleared"})
	if evs, _ := f.ListEvents(EventQuery{}); len(evs) != 3 || evs[2].ID != 453 {
		t.Errorf("после очистки = %+v; want 3 события, новое с номером 453 (номера не повторяются)", evs)
	}

	// Очистка статистики.
	_ = f.SaveDayState(1, core.DayState{Day: "2026-11-09", TotalS: 10})
	_ = f.SaveDayState(1, core.DayState{Day: "2026-11-10", TotalS: 20})
	_ = f.SaveDayState(2, core.DayState{Day: "2026-11-09", TotalS: 30})
	_ = f.AddUsage(1, "2026-11-10", "games", "program", `c:\g.exe`, 20)
	_ = f.AddUsage(2, "2026-11-09", "games", "program", `c:\g.exe`, 30)
	if err := f.ClearStats(1, core.DayState{Day: "2026-11-09"}, StatsReset{Day: "2026-11-10"}); err == nil {
		t.Error("ClearStats с разными днями прошёл")
	}
	day := core.DayState{Day: "2026-11-10", TotalS: 25, BonusGranted: true}
	r := StatsReset{ClearedAtMs: 77, Day: "2026-11-10", TotalS: 25}
	if err := f.ClearStats(1, day, r); err != nil {
		t.Fatal(err)
	}
	if rows, _ := f.DaysSeries(1, "2026-01-01", "2027-01-01"); fmt.Sprint(rows) != "[{2026-11-10 25 0 0 0 0}]" {
		t.Errorf("дни после очистки = %v", rows)
	}
	if u, _ := f.PeriodUsage(1, "2026-01-01", "2027-01-01"); len(u) != 0 {
		t.Errorf("учёт после очистки = %v, want пусто", u)
	}
	if got, ok, _ := f.GetStatsReset(1); !ok || got != r {
		t.Errorf("снимок = %+v %v", got, ok)
	}
	if d, ok, _ := f.FirstStatsDay(2); !ok || d != "2026-11-09" {
		t.Errorf("FirstStatsDay(2) = %q %v; другой ребёнок задет", d, ok)
	}
	if u, _ := f.PeriodUsage(2, "2026-01-01", "2027-01-01"); len(u) != 1 {
		t.Errorf("учёт ребёнка 2 = %v, want на месте", u)
	}
	if empty, _ := f.DaysSeries(3, "2026-01-01", "2027-01-01"); empty == nil || len(empty) != 0 {
		t.Errorf("DaysSeries без дней = %#v, want []", empty)
	}
	_ = f.SaveDayState(2, core.DayState{Day: "2026-11-08", TotalS: 1, BonusGranted: true})
	_ = f.SaveDayState(2, core.DayState{Day: "2026-11-10", TotalS: 1})
	if ds, _ := f.UngrantedDays(2, "2026-11-10"); len(ds) != 1 || ds[0].Day != "2026-11-09" {
		t.Errorf("UngrantedDays = %+v, want только 2026-11-09", ds)
	}

	// Промокоды: пачка целиком или ничего, порядок списка, очистка истории.
	if _, err := f.CreatePromo("USED", 10); err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = f.RedeemPromo(1, "used", 500)
	if _, err := f.CreatePromos([]string{"N1", "used"}, 10); err == nil {
		t.Error("пачка с использованным кодом прошла")
	}
	if _, err := f.CreatePromos([]string{"N1", "n1"}, 10); err == nil {
		t.Error("пачка с повтором внутри прошла")
	}
	if got, err := f.CreatePromos([]string{"N1", "N2"}, 10); err != nil || len(got) != 2 {
		t.Fatalf("CreatePromos = %v, %v", got, err)
	}
	codes := func() string {
		ps, _ := f.ListPromos()
		var out []string
		for _, p := range ps {
			out = append(out, p.CodeDisplay)
		}
		return strings.Join(out, " ")
	}
	if got := codes(); got != "N2 N1 USED" {
		t.Errorf("порядок = %q, want действующие новые сверху, затем использованные", got)
	}
	if n, _ := f.ClearUsedPromos(); n != 1 || codes() != "N2 N1" {
		t.Errorf("ClearUsedPromos = %d, список %q", n, codes())
	}
}
