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
		out = append(out, service.Child{ID: c.ID, SID: c.SID, Name: c.Name, BypassDetail: c.BypassDetail})
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
