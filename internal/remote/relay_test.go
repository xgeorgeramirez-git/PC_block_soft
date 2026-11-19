package remote

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// miniRelay — свой маленький «почтовый ящик» на httptest по разделу 3 протокола (модуль
// pcblock-relay импортировать нельзя). Телефон в тестах действует прямо через методы send/inject.
type miniRelay struct {
	t   testing.TB
	srv *httptest.Server

	mu        sync.Mutex
	nextID    int
	pcs       map[string]string // токен → pcId
	queue     []Msg
	signal    chan struct{}
	waiters   map[string]chan Msg
	code      string
	reports   []Signed
	unpairs   []string
	registers int
	pairCodes int
	polls     int
	pollMax   time.Duration

	// Подмены для тестов (под mu).
	pollHook  func(n int) (status int, body []byte, ok bool) // ok — ответ подменён
	hangPoll  bool                                           // опрос висит до отмены запроса
	codeFail  bool                                           // pair-code отвечает 503
	toPC      func(*Msg)                                     // посредник: сообщение телефона → компьютеру
	toPhone   func(*Msg)                                     // посредник: ответ компьютера → телефону
	reqHeader []http.Header
}

func newMiniRelay(t testing.TB) *miniRelay {
	r := &miniRelay{t: t, pcs: map[string]string{}, waiters: map[string]chan Msg{}, signal: make(chan struct{}), pollMax: 30 * time.Millisecond}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pc/register", r.hRegister)
	mux.HandleFunc("POST /v1/pc/pair-code", r.auth(r.hPairCode))
	mux.HandleFunc("GET /v1/pc/poll", r.auth(r.hPoll))
	mux.HandleFunc("POST /v1/pc/reply", r.auth(r.hReply))
	mux.HandleFunc("POST /v1/pc/report", r.auth(r.hReport))
	mux.HandleFunc("POST /v1/pc/unpair", r.auth(r.hUnpair))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { r.fail(w, 404, "not_found") })
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.mu.Lock()
		r.reqHeader = append(r.reqHeader, q.Header.Clone())
		r.mu.Unlock()
		mux.ServeHTTP(w, q)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *miniRelay) id(prefix string) string {
	r.nextID++
	return fmt.Sprintf("%s%d", prefix, r.nextID)
}

func (r *miniRelay) send(w http.ResponseWriter, code int, v map[string]any) {
	if v == nil {
		v = map[string]any{}
	}
	v["serverNowMs"] = time.Now().UnixMilli()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (r *miniRelay) fail(w http.ResponseWriter, code int, e string) {
	r.send(w, code, map[string]any{"error": e})
}

type pcHandler func(w http.ResponseWriter, q *http.Request, pcID string)

func (r *miniRelay) auth(h pcHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, q *http.Request) {
		tok := strings.TrimPrefix(q.Header.Get("Authorization"), "Bearer ")
		r.mu.Lock()
		pc, ok := r.pcs[tok]
		r.mu.Unlock()
		if !ok {
			r.fail(w, 401, "unauthorized")
			return
		}
		h(w, q, pc)
	}
}

func (r *miniRelay) hRegister(w http.ResponseWriter, q *http.Request) {
	r.mu.Lock()
	r.registers++
	pc, tok := r.id("pc"), r.id("tok")
	r.pcs[tok] = pc
	r.mu.Unlock()
	r.send(w, 200, map[string]any{"pcId": pc, "pcToken": tok})
}

func (r *miniRelay) hPairCode(w http.ResponseWriter, q *http.Request, _ string) {
	r.mu.Lock()
	if r.codeFail {
		r.mu.Unlock()
		r.fail(w, 503, "busy")
		return
	}
	r.pairCodes++
	r.code = fmt.Sprintf("%06d", 123450+r.pairCodes)
	code := r.code
	r.mu.Unlock()
	r.send(w, 200, map[string]any{"code": code, "expiresMs": time.Now().Add(10 * time.Minute).UnixMilli()})
}

func (r *miniRelay) hPoll(w http.ResponseWriter, q *http.Request, _ string) {
	r.mu.Lock()
	r.polls++
	n := r.polls
	hook, hang := r.pollHook, r.hangPoll
	r.mu.Unlock()
	if hook != nil {
		if code, body, ok := hook(n); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			w.Write(body)
			return
		}
	}
	if hang {
		<-q.Context().Done()
		return
	}
	wait, _ := strconv.Atoi(q.URL.Query().Get("wait"))
	if wait != 25 {
		r.fail(w, 400, "bad_request")
		return
	}
	r.mu.Lock()
	msgs, sig := r.queue, r.signal
	r.queue = nil
	pollMax := r.pollMax
	r.mu.Unlock()
	if len(msgs) == 0 {
		t := time.NewTimer(pollMax)
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
		msgs = []Msg{}
	}
	r.send(w, 200, map[string]any{"messages": msgs})
}

func (r *miniRelay) hReply(w http.ResponseWriter, q *http.Request, pc string) {
	var in struct {
		To, ReplyTo, Kind string
		Body              json.RawMessage
	}
	if json.NewDecoder(q.Body).Decode(&in) != nil || in.To == "" || in.Kind == "" {
		r.fail(w, 400, "bad_request")
		return
	}
	r.mu.Lock()
	ch := r.waiters[in.ReplyTo]
	delete(r.waiters, in.ReplyTo)
	id := r.id("r")
	r.mu.Unlock()
	if ch == nil {
		r.fail(w, 404, "not_found")
		return
	}
	ch <- Msg{ID: id, From: pc, Kind: in.Kind, Body: in.Body}
	r.send(w, 200, nil)
}

func (r *miniRelay) hReport(w http.ResponseWriter, q *http.Request, _ string) {
	var in struct{ Body Signed }
	if json.NewDecoder(q.Body).Decode(&in) != nil {
		r.fail(w, 400, "bad_request")
		return
	}
	r.mu.Lock()
	r.reports = append(r.reports, in.Body)
	r.mu.Unlock()
	r.send(w, 200, nil)
}

func (r *miniRelay) hUnpair(w http.ResponseWriter, q *http.Request, _ string) {
	var in struct{ PhoneID string }
	if json.NewDecoder(q.Body).Decode(&in) != nil || in.PhoneID == "" {
		r.fail(w, 400, "bad_request")
		return
	}
	r.mu.Lock()
	r.unpairs = append(r.unpairs, in.PhoneID)
	r.mu.Unlock()
	r.send(w, 200, nil)
}

// sendFrom — телефон from кладёт сообщение компьютеру и ждёт ответа до wait.
func (r *miniRelay) sendFrom(from, kind string, body any, wait time.Duration) (Msg, bool) {
	raw, err := json.Marshal(body)
	if err != nil {
		r.t.Fatal(err)
	}
	r.mu.Lock()
	m := Msg{ID: r.id("m"), From: from, Kind: kind, Body: raw}
	if r.toPC != nil {
		r.toPC(&m)
	}
	ch := make(chan Msg, 1)
	r.waiters[m.ID] = ch
	r.queue = append(r.queue, m)
	close(r.signal)
	r.signal = make(chan struct{})
	toPhone := r.toPhone
	r.mu.Unlock()
	select {
	case rep := <-ch:
		if toPhone != nil {
			toPhone(&rep)
		}
		return rep, true
	case <-time.After(wait):
		r.mu.Lock()
		delete(r.waiters, m.ID)
		r.mu.Unlock()
		return Msg{}, false
	}
}

func (r *miniRelay) revokeAll() {
	r.mu.Lock()
	r.pcs = map[string]string{}
	r.mu.Unlock()
}

func (r *miniRelay) snapshot() (registers, pairCodes int, unpairs []string, reports []Signed) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registers, r.pairCodes, append([]string(nil), r.unpairs...), append([]Signed(nil), r.reports...)
}

func (r *miniRelay) currentCode() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.code
}
