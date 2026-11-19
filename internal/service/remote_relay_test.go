package service

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/remote"
)

// testRelay — маленький «почтовый ящик» сервера связи на httptest (протокол, раздел 3) для
// тестов службы: подход internal/remote/relay_test.go. Телефон действует методами phone*.
type testRelay struct {
	t   testing.TB
	srv *httptest.Server

	mu       sync.Mutex
	nextID   int
	pcs      map[string]string // токен → pcId
	queue    []remote.Msg
	signal   chan struct{}
	waiters  map[string]chan remote.Msg
	code     string
	reports  []remote.Signed
	unpairs  []string
	requests int
}

func newTestRelay(t testing.TB) *testRelay {
	r := &testRelay{t: t, pcs: map[string]string{}, waiters: map[string]chan remote.Msg{}, signal: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pc/register", r.hRegister)
	mux.HandleFunc("POST /v1/pc/pair-code", r.auth(r.hPairCode))
	mux.HandleFunc("GET /v1/pc/poll", r.auth(r.hPoll))
	mux.HandleFunc("POST /v1/pc/reply", r.auth(r.hReply))
	mux.HandleFunc("POST /v1/pc/report", r.auth(r.hReport))
	mux.HandleFunc("POST /v1/pc/unpair", r.auth(r.hUnpair))
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.mu.Lock()
		r.requests++
		r.mu.Unlock()
		mux.ServeHTTP(w, q)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *testRelay) url() string { return r.srv.URL }

func (r *testRelay) id(prefix string) string {
	r.nextID++
	return fmt.Sprintf("%s%d", prefix, r.nextID)
}

func (r *testRelay) send(w http.ResponseWriter, code int, v map[string]any) {
	if v == nil {
		v = map[string]any{}
	}
	v["serverNowMs"] = time.Now().UnixMilli()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (r *testRelay) auth(h func(w http.ResponseWriter, q *http.Request, pc string)) http.HandlerFunc {
	return func(w http.ResponseWriter, q *http.Request) {
		tok := strings.TrimPrefix(q.Header.Get("Authorization"), "Bearer ")
		r.mu.Lock()
		pc, ok := r.pcs[tok]
		r.mu.Unlock()
		if !ok {
			r.send(w, 401, map[string]any{"error": "unauthorized"})
			return
		}
		h(w, q, pc)
	}
}

func (r *testRelay) hRegister(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	pc, tok := r.id("pc"), r.id("tok")
	r.pcs[tok] = pc
	r.mu.Unlock()
	r.send(w, 200, map[string]any{"pcId": pc, "pcToken": tok})
}

func (r *testRelay) hPairCode(w http.ResponseWriter, _ *http.Request, _ string) {
	r.mu.Lock()
	r.code = fmt.Sprintf("%06d", 482000+r.nextID)
	r.nextID++
	code := r.code
	r.mu.Unlock()
	r.send(w, 200, map[string]any{"code": code, "expiresMs": time.Now().Add(10 * time.Minute).UnixMilli()})
}

func (r *testRelay) hPoll(w http.ResponseWriter, q *http.Request, _ string) {
	r.mu.Lock()
	msgs, sig := r.queue, r.signal
	r.queue = nil
	r.mu.Unlock()
	if len(msgs) == 0 {
		t := time.NewTimer(30 * time.Millisecond)
		select {
		case <-sig:
		case <-t.C:
		case <-q.Context().Done():
		}
		t.Stop()
		r.mu.Lock()
		msgs = r.queue
		r.queue = nil
		r.mu.Unlock()
	}
	if msgs == nil {
		msgs = []remote.Msg{}
	}
	r.send(w, 200, map[string]any{"messages": msgs})
}

func (r *testRelay) hReply(w http.ResponseWriter, q *http.Request, pc string) {
	var in struct {
		To, ReplyTo, Kind string
		Body              json.RawMessage
	}
	if json.NewDecoder(q.Body).Decode(&in) != nil || in.To == "" || in.Kind == "" {
		r.send(w, 400, map[string]any{"error": "bad_request"})
		return
	}
	r.mu.Lock()
	ch := r.waiters[in.ReplyTo]
	delete(r.waiters, in.ReplyTo)
	id := r.id("r")
	r.mu.Unlock()
	if ch == nil {
		r.send(w, 404, map[string]any{"error": "not_found"})
		return
	}
	ch <- remote.Msg{ID: id, From: pc, Kind: in.Kind, Body: in.Body}
	r.send(w, 200, nil)
}

func (r *testRelay) hReport(w http.ResponseWriter, q *http.Request, _ string) {
	var in struct{ Body remote.Signed }
	if json.NewDecoder(q.Body).Decode(&in) != nil {
		r.send(w, 400, map[string]any{"error": "bad_request"})
		return
	}
	r.mu.Lock()
	r.reports = append(r.reports, in.Body)
	r.mu.Unlock()
	r.send(w, 200, nil)
}

func (r *testRelay) hUnpair(w http.ResponseWriter, q *http.Request, _ string) {
	var in struct{ PhoneID string }
	if json.NewDecoder(q.Body).Decode(&in) != nil || in.PhoneID == "" {
		r.send(w, 400, map[string]any{"error": "bad_request"})
		return
	}
	r.mu.Lock()
	r.unpairs = append(r.unpairs, in.PhoneID)
	r.mu.Unlock()
	r.send(w, 200, nil)
}

// sendFrom — телефон from кладёт сообщение компьютеру и ждёт ответа до wait.
func (r *testRelay) sendFrom(from, kind string, body any, wait time.Duration) (remote.Msg, bool) {
	raw, err := json.Marshal(body)
	if err != nil {
		r.t.Fatal(err)
	}
	r.mu.Lock()
	m := remote.Msg{ID: r.id("m"), From: from, Kind: kind, Body: raw}
	ch := make(chan remote.Msg, 1)
	r.waiters[m.ID] = ch
	r.queue = append(r.queue, m)
	close(r.signal)
	r.signal = make(chan struct{})
	r.mu.Unlock()
	select {
	case rep := <-ch:
		return rep, true
	case <-time.After(wait):
		r.mu.Lock()
		delete(r.waiters, m.ID)
		r.mu.Unlock()
		return remote.Msg{}, false
	}
}

func (r *testRelay) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests
}

func (r *testRelay) unpaired() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.unpairs...)
}

func (r *testRelay) reportCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reports)
}

// --- телефон ---------------------------------------------------------------------------------

var b64u = base64.RawURLEncoding

// testPhone — телефон теста: ключ, id на сервере, ключ компьютера после привязки, номер команды.
type testPhone struct {
	t     testing.TB
	r     *testRelay
	id    string
	name  string
	key   *ecdsa.PrivateKey
	pcPub *ecdsa.PublicKey
	pcID  string
	seq   int64
}

func newTestPhone(t testing.TB, r *testRelay, id, name string) *testPhone {
	k, err := remote.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return &testPhone{t: t, r: r, id: id, name: name, key: k}
}

func mustDER(t testing.TB, pub string) []byte {
	b, err := b64u.DecodeString(pub)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// pair проходит pair1 и pair2 (протокол, раздел 4) и возвращает число сверки телефона.
func (p *testPhone) pair() string {
	t := p.t
	t.Helper()
	phonePub := remote.MarshalPub(&p.key.PublicKey)
	rep, ok := p.r.sendFrom(p.id, remote.KindPair1, remote.Pair1{PhonePub: phonePub, Name: p.name}, 5*time.Second)
	if !ok || rep.Kind != remote.KindPair1R {
		t.Fatalf("pair1: нет ответа pair1r (%v, %q)", ok, rep.Kind)
	}
	var r1 remote.Pair1R
	if err := json.Unmarshal(rep.Body, &r1); err != nil {
		t.Fatal(err)
	}
	nPhone := make([]byte, remote.NonceLen)
	for i := range nPhone {
		nPhone[i] = byte(i + 7)
	}
	rep, ok = p.r.sendFrom(p.id, remote.KindPair2, remote.Pair2{NPhone: b64u.EncodeToString(nPhone)}, 5*time.Second)
	if !ok || rep.Kind != remote.KindPair2R {
		t.Fatalf("pair2: нет ответа pair2r (%v, %q)", ok, rep.Kind)
	}
	var r2 remote.Pair2R
	if err := json.Unmarshal(rep.Body, &r2); err != nil {
		t.Fatal(err)
	}
	pcDER, phoneDER, nPc := mustDER(t, r1.PCPub), mustDER(t, phonePub), mustDER(t, r2.NPc)
	if b64u.EncodeToString(remote.Commit(pcDER, phoneDER, nPc)) != r1.Commit {
		t.Fatal("обязательство компьютера не сошлось")
	}
	pub, err := remote.ParsePub(r1.PCPub)
	if err != nil {
		t.Fatal(err)
	}
	p.pcPub = pub
	return remote.SAS(pcDER, phoneDER, nPc, nPhone)
}

// command шлёт подписанную команду и разбирает подписанный ответ компьютера.
func (p *testPhone) command(seq, child int64, delta int, ts time.Time) remote.CmdResult {
	t := p.t
	t.Helper()
	raw, _ := json.Marshal(remote.Command{V: remote.ProtoVersion, T: remote.TypeCmd, PC: p.pcID, Phone: p.id, Seq: seq, TS: ts.UnixMilli(), Child: child, Delta: delta})
	s, err := remote.Sign(p.key, raw)
	if err != nil {
		t.Fatal(err)
	}
	rep, ok := p.r.sendFrom(p.id, remote.KindCmd, s, 5*time.Second)
	if !ok || rep.Kind != remote.KindCmdR {
		t.Fatalf("cmd: нет ответа cmdr (%v, %q)", ok, rep.Kind)
	}
	var signed remote.Signed
	if err := json.Unmarshal(rep.Body, &signed); err != nil {
		t.Fatal(err)
	}
	data, err := remote.Verify(p.pcPub, signed)
	if err != nil {
		t.Fatalf("ответ без подписи компьютера: %v", err)
	}
	var res remote.CmdResult
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	if res.Cmd != remote.HashData(raw) {
		t.Fatal("ответ не на эту команду")
	}
	return res
}
