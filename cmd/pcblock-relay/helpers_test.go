package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	mrand "math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock — поддельные часы: время двигает только тест.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// lockedRand — детерминированный источник случайности для тестов (io.Reader).
type lockedRand struct {
	mu sync.Mutex
	r  *mrand.Rand
}

func (l *lockedRand) Read(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.Read(p)
}

type env struct {
	t     *testing.T
	s     *Server
	clock *fakeClock
	dir   string
	logs  *bytes.Buffer
	logMu *sync.Mutex
}

type tcfg struct {
	PollMax, ReplyWait time.Duration
	TrustProxy         bool
	Dir                string
	Clock              *fakeClock
}

type syncWriter struct {
	mu *sync.Mutex
	b  *bytes.Buffer
}

func (w syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func newEnv(t *testing.T, c tcfg) *env {
	t.Helper()
	if c.PollMax == 0 {
		c.PollMax = 100 * time.Millisecond
	}
	if c.ReplyWait == 0 {
		c.ReplyWait = 100 * time.Millisecond
	}
	if c.Dir == "" {
		c.Dir = t.TempDir()
	}
	clock := c.Clock
	if clock == nil {
		clock = newClock()
	}
	logs := &bytes.Buffer{}
	mu := &sync.Mutex{}
	s, err := New(Config{
		DataDir:    c.Dir,
		TrustProxy: c.TrustProxy,
		Now:        clock.Now,
		Rand:       &lockedRand{r: mrand.New(mrand.NewSource(7))},
		PollMax:    c.PollMax,
		ReplyWait:  c.ReplyWait,
		Log:        log.New(syncWriter{mu, logs}, "", 0),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &env{t: t, s: s, clock: clock, dir: c.Dir, logs: logs, logMu: mu}
}

type resp struct {
	Code int
	Raw  string
	M    map[string]any
}

func (r resp) str(k string) string {
	v, _ := r.M[k].(string)
	return v
}

func (r resp) num(k string) int64 {
	v, _ := r.M[k].(float64)
	return int64(v)
}

type req struct {
	method, path, token, body, remote, xff string
	ctx                                    context.Context
}

func (e *env) do(q req) resp {
	e.t.Helper()
	var body io.Reader
	if q.body != "" {
		body = strings.NewReader(q.body)
	}
	r := httptest.NewRequest(q.method, q.path, body)
	if q.ctx != nil {
		r = r.WithContext(q.ctx)
	}
	if q.token != "" {
		r.Header.Set("Authorization", "Bearer "+q.token)
	}
	if q.remote != "" {
		r.RemoteAddr = q.remote
	}
	if q.xff != "" {
		r.Header.Set("X-Forwarded-For", q.xff)
	}
	if q.body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.s.Handler().ServeHTTP(w, r)
	out := resp{Code: w.Code, Raw: w.Body.String()}
	_ = json.Unmarshal(w.Body.Bytes(), &out.M)
	return out
}

func (e *env) post(path, token, body string) resp {
	e.t.Helper()
	return e.do(req{method: http.MethodPost, path: path, token: token, body: body})
}

func (e *env) get(path, token string) resp {
	e.t.Helper()
	return e.do(req{method: http.MethodGet, path: path, token: token})
}

func expect(t *testing.T, r resp, code int, errCode string) {
	t.Helper()
	if r.Code != code {
		t.Fatalf("код %d, ждали %d: %s", r.Code, code, r.Raw)
	}
	if errCode != "" && r.str("error") != errCode {
		t.Fatalf("ошибка %q, ждали %q: %s", r.str("error"), errCode, r.Raw)
	}
	if _, ok := r.M["serverNowMs"]; !ok {
		t.Fatalf("нет serverNowMs: %s", r.Raw)
	}
}

func (e *env) register() (pcID, pcToken string) {
	e.t.Helper()
	r := e.post("/v1/pc/register", "", `{}`)
	expect(e.t, r, 200, "")
	return r.str("pcId"), r.str("pcToken")
}

func (e *env) pairCode(pcToken string) string {
	e.t.Helper()
	r := e.post("/v1/pc/pair-code", pcToken, `{}`)
	expect(e.t, r, 200, "")
	return r.str("code")
}

// msgs достаёт сообщения из ответа опроса.
func msgs(t *testing.T, r resp) []map[string]any {
	t.Helper()
	arr, ok := r.M["messages"].([]any)
	if !ok {
		t.Fatalf("messages не массив: %s", r.Raw)
	}
	out := make([]map[string]any, 0, len(arr))
	for _, a := range arr {
		out = append(out, a.(map[string]any))
	}
	return out
}

// async выполняет запрос в горутине.
func (e *env) async(q req) <-chan resp {
	ch := make(chan resp, 1)
	go func() { ch <- e.do(q) }()
	return ch
}

func wait(t *testing.T, ch <-chan resp, d time.Duration) resp {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(d):
		t.Fatalf("запрос не вернулся за %v", d)
	}
	return resp{}
}

// paired — компьютер и привязанный телефон (полный круг pair1 → pair1r).
type paired struct {
	pcID, pcToken, phoneID, phoneToken string
}

func (e *env) pair() paired {
	e.t.Helper()
	pcID, pcTok := e.register()
	code := e.pairCode(pcTok)
	ph := e.async(req{method: "POST", path: "/v1/phone/pair",
		body: `{"code":"` + code + `","kind":"pair1","body":{"phonePub":"AAA","name":"Pixel 7"}}`})
	p := e.get("/v1/pc/poll?wait=25", pcTok)
	expect(e.t, p, 200, "")
	m := msgs(e.t, p)
	if len(m) != 1 {
		e.t.Fatalf("ждали 1 сообщение: %s", p.Raw)
	}
	rep := e.post("/v1/pc/reply", pcTok, `{"to":"`+m[0]["from"].(string)+`","replyTo":"`+m[0]["id"].(string)+`","kind":"pair1r","body":{"pcPub":"BBB"}}`)
	expect(e.t, rep, 200, "")
	r := wait(e.t, ph, 2*time.Second)
	expect(e.t, r, 200, "")
	return paired{pcID: pcID, pcToken: pcTok, phoneID: r.str("phoneId"), phoneToken: r.str("phoneToken")}
}

func (e *env) logText() string {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	return e.logs.String()
}

// waitFor ждёт условия до d (реальное время, шаг 2 мс).
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("условие не наступило за %v", d)
}
