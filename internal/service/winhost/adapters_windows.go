//go:build windows

package winhost

import (
	"errors"
	"fmt"

	"pcblock/internal/auth"
	"pcblock/internal/auth/pwhash"
	"pcblock/internal/core"
	"pcblock/internal/service"
	"pcblock/internal/storage"
)

// storeOpener открывает базу программы (service.StoreOpener над internal/storage).
type storeOpener struct{}

// Open открывает базу; порча файла приходит как service.ErrStoreCorrupt (R18).
func (storeOpener) Open(path string) (service.Store, error) {
	st, err := storage.Open(path)
	if err != nil {
		if errors.Is(err, storage.ErrCorrupt) {
			return nil, fmt.Errorf("%w (%w)", service.ErrStoreCorrupt, err)
		}
		return nil, err
	}
	return storeAdapter{st}, nil
}

// Quarantine сохраняет копию испорченной базы рядом с ней.
func (storeOpener) Quarantine(path string) (string, error) { return storage.Quarantine(path) }

// storeAdapter приводит *storage.Store к service.Store: типы службы не зависят от SQLite.
type storeAdapter struct{ s *storage.Store }

func (a storeAdapter) UpsertChild(sid, name string) (int64, error) {
	return a.s.UpsertChild(storage.Child{SID: sid, Name: name})
}

func (a storeAdapter) ListChildren() ([]service.Child, error) {
	list, err := a.s.ListChildren()
	if err != nil {
		return nil, err
	}
	out := make([]service.Child, 0, len(list))
	for _, c := range list {
		out = append(out, service.Child{ID: c.ID, SID: c.SID, Name: c.Name, BypassDetail: c.BypassDetail, EndMode: c.EndMode})
	}
	return out, nil
}

func (a storeAdapter) RemoveChild(id int64) error { return a.s.RemoveChild(id) }

func (a storeAdapter) SetBypass(childID int64, detail string) error {
	return a.s.SetBypass(childID, detail)
}

func (a storeAdapter) SetDayLimits(childID int64, week [7]core.DayLimit) error {
	return a.s.SetDayLimits(childID, week)
}

func (a storeAdapter) GetDayLimits(childID int64) ([7]core.DayLimit, error) {
	return a.s.GetDayLimits(childID)
}

func (a storeAdapter) SetEndMode(childID int64, mode string) error {
	return a.s.SetEndMode(childID, mode)
}

func (a storeAdapter) GetEndMode(childID int64) (string, error) { return a.s.GetEndMode(childID) }

// --- программы, «Ждут решения» и учёт (M3) ---------------------------------------------
// service.Program и storage.Program совпадают по полям: преобразование типа без копирования по полю.

func (a storeAdapter) UpsertProgram(p service.Program) error {
	return a.s.UpsertProgram(storage.Program(p))
}

func (a storeAdapter) SeedPrograms(ps []service.Program) (int, error) {
	in := make([]storage.Program, 0, len(ps))
	for _, p := range ps {
		in = append(in, storage.Program(p))
	}
	return a.s.SeedPrograms(in)
}

func (a storeAdapter) GetProgram(path string) (service.Program, bool, error) {
	p, ok, err := a.s.GetProgram(path)
	return service.Program(p), ok, err
}

func programsOf(list []storage.Program) []service.Program {
	out := make([]service.Program, 0, len(list))
	for _, p := range list {
		out = append(out, service.Program(p))
	}
	return out
}

func (a storeAdapter) ListPrograms() ([]service.Program, error) {
	list, err := a.s.ListPrograms()
	if err != nil {
		return nil, err
	}
	return programsOf(list), nil
}

func (a storeAdapter) SetProgramsCategory(paths []string, category string, at int64) ([]service.Program, error) {
	list, err := a.s.SetProgramsCategory(paths, category, at)
	if err != nil {
		return nil, err
	}
	return programsOf(list), nil
}

func (a storeAdapter) SetProgramsHidden(paths []string, hidden bool) ([]service.Program, error) {
	list, err := a.s.SetProgramsHidden(paths, hidden)
	if err != nil {
		return nil, err
	}
	return programsOf(list), nil
}

func (a storeAdapter) RestoreProgram(p service.Program) error {
	return a.s.RestoreProgram(storage.Program(p))
}

func (a storeAdapter) DeleteProgram(path string) error { return a.s.DeleteProgram(path) }

func pendingOf(p storage.PendingProgram) service.PendingProgram {
	out := service.PendingProgram{
		Path: p.Path, Name: p.Name, Publisher: p.Publisher, Description: p.Description, WasCategory: p.WasCategory,
		FirstChildID: p.FirstChildID, FirstAttemptAt: p.FirstAttemptAt, LastAttemptAt: p.LastAttemptAt, Attempts: p.Attempts,
		Children: make([]service.PendingChild, 0, len(p.Children)),
	}
	for _, c := range p.Children {
		out.Children = append(out.Children, service.PendingChild(c))
	}
	return out
}

func pendingToStorage(p service.PendingProgram) storage.PendingProgram {
	out := storage.PendingProgram{
		Path: p.Path, Name: p.Name, Publisher: p.Publisher, Description: p.Description, WasCategory: p.WasCategory,
		FirstChildID: p.FirstChildID, FirstAttemptAt: p.FirstAttemptAt, LastAttemptAt: p.LastAttemptAt, Attempts: p.Attempts,
		Children: make([]storage.PendingChild, 0, len(p.Children)),
	}
	for _, c := range p.Children {
		out.Children = append(out.Children, storage.PendingChild(c))
	}
	return out
}

func (a storeAdapter) AddPendingAttempt(p service.PendingProgram, childID, at int64) (int, error) {
	return a.s.AddPendingAttempt(pendingToStorage(p), childID, at)
}

func (a storeAdapter) ListPending() ([]service.PendingProgram, error) {
	list, err := a.s.ListPending()
	if err != nil {
		return nil, err
	}
	out := make([]service.PendingProgram, 0, len(list))
	for _, p := range list {
		out = append(out, pendingOf(p))
	}
	return out, nil
}

func (a storeAdapter) ResolvePending(path string) (service.PendingProgram, bool, error) {
	p, ok, err := a.s.ResolvePending(path)
	if err != nil || !ok {
		return service.PendingProgram{}, ok, err
	}
	return pendingOf(p), true, nil
}

func (a storeAdapter) RestorePending(p service.PendingProgram) error {
	return a.s.RestorePending(pendingToStorage(p))
}

func (a storeAdapter) AddUsage(childID int64, day, category, kind, subject string, seconds int) error {
	return a.s.AddUsage(childID, day, category, kind, subject, seconds)
}

func (a storeAdapter) SaveDayAndUsage(childID int64, st core.DayState, usage []service.UsageRow) error {
	rows := make([]storage.UsageRow, 0, len(usage))
	for _, u := range usage {
		rows = append(rows, storage.UsageRow(u))
	}
	return a.s.SaveDayAndUsage(childID, st, rows)
}

func (a storeAdapter) ProgramSeconds(fromDay string) (map[string]int, error) {
	return a.s.ProgramSeconds(fromDay)
}

func (a storeAdapter) GetSchedule(childID int64) ([]core.Interval, error) {
	return a.s.GetSchedule(childID)
}

func (a storeAdapter) SetSchedule(childID int64, intervals []core.Interval) error {
	return a.s.SetSchedule(childID, intervals)
}

func (a storeAdapter) AppendBonus(childID int64, deltaMin int, reason string) (int, error) {
	return a.s.AppendBonus(childID, deltaMin, reason)
}

func (a storeAdapter) GetBonusBalance(childID int64) (int, error) {
	return a.s.GetBonusBalance(childID)
}

func (a storeAdapter) ListBonus(childID int64) ([]service.BonusRecord, error) {
	list, err := a.s.ListBonus(childID)
	if err != nil {
		return nil, err
	}
	out := make([]service.BonusRecord, 0, len(list))
	for _, e := range list {
		out = append(out, service.BonusRecord{AtMs: e.AtMs, DeltaMin: e.DeltaMin, Reason: e.Reason, Balance: e.BalanceAfter})
	}
	return out, nil
}

func (a storeAdapter) CreatePromo(codeDisplay string, minutes int) (service.PromoRecord, error) {
	p, err := a.s.CreatePromo(codeDisplay, minutes)
	if err != nil {
		return service.PromoRecord{}, err
	}
	return promoRecordOf(p), nil
}

func (a storeAdapter) RedeemPromo(childID int64, code string, atMs int64) (int, string, bool, error) {
	return a.s.RedeemPromo(childID, code, atMs)
}

func (a storeAdapter) ListPromos() ([]service.PromoRecord, error) {
	list, err := a.s.ListPromos()
	if err != nil {
		return nil, err
	}
	out := make([]service.PromoRecord, 0, len(list))
	for _, p := range list {
		out = append(out, promoRecordOf(p))
	}
	return out, nil
}

// promoRecordOf приводит storage.Promo к service.PromoRecord: указатели used_at/used_by
// становятся 0 для «ещё не использован».
func promoRecordOf(p storage.Promo) service.PromoRecord {
	r := service.PromoRecord{CodeDisplay: p.CodeDisplay, Minutes: p.Minutes, CreatedAtMs: p.CreatedAtMs}
	if p.UsedAtMs != nil {
		r.UsedAtMs = *p.UsedAtMs
	}
	if p.UsedByChild != nil {
		r.UsedByChild = *p.UsedByChild
	}
	return r
}

func (a storeAdapter) LoadDayState(childID int64, day string) (core.DayState, error) {
	return a.s.LoadDayState(childID, day)
}

func (a storeAdapter) SaveDayState(childID int64, st core.DayState) error {
	return a.s.SaveDayState(childID, st)
}

func (a storeAdapter) AppendEvent(e service.StoredEvent) error {
	ev := storage.Event{AtMs: e.AtMs, TzOffsetMin: e.TzOffsetMin, Kind: e.Kind, Details: e.Details}
	if e.ChildID != 0 {
		id := e.ChildID
		ev.ChildID = &id
	}
	return a.s.AppendEvent(ev)
}

func (a storeAdapter) ListEvents(q service.EventQuery) ([]service.StoredEvent, error) {
	f := storage.EventFilter{Limit: q.Limit}
	if q.ChildID != 0 {
		id := q.ChildID
		f.ChildID = &id
	}
	list, err := a.s.ListEvents(f)
	if err != nil {
		return nil, err
	}
	out := make([]service.StoredEvent, 0, len(list))
	for _, e := range list {
		se := service.StoredEvent{ID: e.ID, AtMs: e.AtMs, TzOffsetMin: e.TzOffsetMin, Kind: e.Kind, Details: e.Details}
		if e.ChildID != nil {
			se.ChildID = *e.ChildID
		}
		out = append(out, se)
	}
	return out, nil
}

func (a storeAdapter) GetAuth() (service.Auth, bool, error) {
	au, ok, err := a.s.GetAuth()
	if err != nil || !ok {
		return service.Auth{}, ok, err
	}
	return service.Auth{
		Hash:        au.Hash,
		Attempts:    auth.Attempts{Failed: au.Failed, Series: au.Series, BlockedUntilMs: au.BlockedUntilMs},
		ChangedAtMs: au.ChangedAtMs,
	}, true, nil
}

func (a storeAdapter) SetAuth(au service.Auth) error {
	return a.s.SetAuth(storage.Auth{
		Hash:           au.Hash,
		Failed:         au.Attempts.Failed,
		Series:         au.Attempts.Series,
		BlockedUntilMs: au.Attempts.BlockedUntilMs,
		ChangedAtMs:    au.ChangedAtMs,
	})
}

func (a storeAdapter) GetSetting(key string) (string, bool, error) { return a.s.GetSetting(key) }
func (a storeAdapter) SetSetting(key, value string) error          { return a.s.SetSetting(key, value) }
func (a storeAdapter) Close() error                                { return a.s.Close() }

// hasher — хеш пароля родителя Argon2id (service.Hasher над auth/pwhash).
type hasher struct{}

func (hasher) Hash(pw string) (string, error)       { return pwhash.Hash(pw) }
func (hasher) Verify(hash, pw string) (bool, error) { return pwhash.Verify(hash, pw) }
