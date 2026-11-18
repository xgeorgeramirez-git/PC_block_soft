package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const storeFile = "relay.json"

// pcRec — учётная запись компьютера. Токен хранится только хэшем SHA-256. LastSeenMs —
// последний запрос компьютера с точностью до часа (по нему удаляются брошенные записи).
type pcRec struct {
	ID         string `json:"id"`
	TokenHash  string `json:"tokenHash"`
	CreatedMs  int64  `json:"createdMs"`
	LastSeenMs int64  `json:"lastSeenMs"`
}

// phoneRec — учётная запись телефона, привязанного к компьютеру.
type phoneRec struct {
	ID        string `json:"id"`
	PCID      string `json:"pcId"`
	TokenHash string `json:"tokenHash"`
	CreatedMs int64  `json:"createdMs"`
}

type storeData struct {
	V      int        `json:"v"`
	PCs    []pcRec    `json:"pcs"`
	Phones []phoneRec `json:"phones"`
}

// Store — учётные записи на диске (/data/relay.json). Не потокобезопасен: под мьютексом сервера.
type Store struct {
	dir  string
	data storeData
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	st := &Store{dir: dir, data: storeData{V: 1, PCs: []pcRec{}, Phones: []phoneRec{}}}
	raw, err := os.ReadFile(filepath.Join(dir, storeFile))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &st.data); err != nil {
		// Тихо обнулять нельзя: компьютеры и телефоны потеряли бы привязку.
		return nil, fmt.Errorf("%s испорчен: %w", storeFile, err)
	}
	if st.data.PCs == nil {
		st.data.PCs = []pcRec{}
	}
	if st.data.Phones == nil {
		st.data.Phones = []phoneRec{}
	}
	return st, nil
}

// save пишет файл атомарно: временный файл в той же папке, Sync, Rename.
func (st *Store) save() error {
	raw, err := json.MarshalIndent(st.data, "", " ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(st.dir, "relay-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := f.Write(raw); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(st.dir, storeFile)); err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(st.dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// tokenMatches сравнивает хэши за постоянное время.
func tokenMatches(hash, tok string) bool {
	return subtle.ConstantTimeCompare([]byte(hash), []byte(hashToken(tok))) == 1
}

// pcByToken — индекс компьютера по токену или -1 (обходит все записи: время не зависит от места).
func (st *Store) pcByToken(tok string) int {
	found := -1
	for i := range st.data.PCs {
		if tokenMatches(st.data.PCs[i].TokenHash, tok) {
			found = i
		}
	}
	return found
}

// dropStale удаляет компьютеры, которых не было дольше maxAge, вместе с их телефонами.
// Возвращает id удалённых.
func (st *Store) dropStale(nowMs, maxAgeMs int64) []string {
	var gone []string
	keep := []pcRec{}
	for _, pc := range st.data.PCs {
		seen := pc.LastSeenMs
		if seen == 0 {
			seen = pc.CreatedMs
		}
		if nowMs-seen > maxAgeMs {
			gone = append(gone, pc.ID)
			continue
		}
		keep = append(keep, pc)
	}
	if len(gone) == 0 {
		return nil
	}
	isGone := map[string]bool{}
	for _, id := range gone {
		isGone[id] = true
	}
	phones := []phoneRec{}
	for _, ph := range st.data.Phones {
		if !isGone[ph.PCID] {
			phones = append(phones, ph)
		}
	}
	st.data.PCs, st.data.Phones = keep, phones
	return gone
}

func (st *Store) phoneByToken(tok string) *phoneRec {
	var found *phoneRec
	for i := range st.data.Phones {
		if tokenMatches(st.data.Phones[i].TokenHash, tok) {
			found = &st.data.Phones[i]
		}
	}
	return found
}
