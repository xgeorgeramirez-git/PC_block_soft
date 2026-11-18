package remote

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	cases := []struct {
		raw      string
		loopback bool
		ok       bool
		want     string
	}{
		{"https://pcblock.atlas404.ru", false, true, "https://pcblock.atlas404.ru"},
		{"https://pcblock.atlas404.ru/", false, true, "https://pcblock.atlas404.ru"},
		{"  HTTPS://Pcblock.atlas404.ru:8443/relay/ ", false, true, "https://Pcblock.atlas404.ru:8443/relay"},
		{"http://pcblock.atlas404.ru", false, false, ""},
		{"http://pcblock.atlas404.ru", true, false, ""},
		{"http://127.0.0.1:18080", false, false, ""},
		{"http://127.0.0.1:18080", true, true, "http://127.0.0.1:18080"},
		{"http://localhost:18080/", true, true, "http://localhost:18080"},
		{"http://127.0.0.2:18080", true, false, ""},
		{"http://localhost.evil.ru", true, false, ""},
		{"https://", false, false, ""},
		{"https:///path", false, false, ""},
		{"pcblock.atlas404.ru", false, false, ""},
		{"ftp://pcblock.atlas404.ru", false, false, ""},
		{"https://user:pass@pcblock.atlas404.ru", false, false, ""},
		{"https://pcblock.atlas404.ru/?a=1", false, false, ""},
		{"https://pcblock.atlas404.ru/#x", false, false, ""},
		{"", false, false, ""},
		{"https://pc block.ru", false, false, ""},
	}
	for _, c := range cases {
		u, err := ValidateURL(c.raw, c.loopback)
		if c.ok != (err == nil) {
			t.Errorf("%q (loopback=%v): ошибка %v", c.raw, c.loopback, err)
			continue
		}
		if c.ok && u.String() != c.want {
			t.Errorf("%q: %q, нужно %q", c.raw, u.String(), c.want)
		}
	}
}

func testClient(t *testing.T, base string) *Client {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	return &Client{Base: u, HTTP: &http.Client{Transport: newTransport(), CheckRedirect: noRedirect}}
}

func TestClientRoundTrip(t *testing.T) {
	r := newMiniRelay(t)
	c := testClient(t, r.srv.URL)
	ctx := context.Background()
	if _, err := c.PairCode(ctx); errCode(err) != StatusUnauthorized {
		t.Fatalf("без токена: %v", err)
	}
	pc, tok, err := c.Register(ctx)
	if err != nil || pc == "" || tok == "" {
		t.Fatalf("регистрация: %v", err)
	}
	c.Token = tok
	code, err := c.PairCode(ctx)
	if err != nil || len(code) != 6 {
		t.Fatalf("код: %q %v", code, err)
	}
	msgs, err := c.Poll(ctx, 25*time.Second)
	if err != nil || len(msgs) != 0 {
		t.Fatalf("пустой опрос: %v %v", msgs, err)
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		r.sendFrom("ph1", KindPair2, map[string]string{"nPhone": "x"}, time.Second)
	}()
	r.mu.Lock()
	r.pollMax = time.Second
	r.mu.Unlock()
	msgs, err = c.Poll(ctx, 25*time.Second)
	if err != nil || len(msgs) != 1 || msgs[0].From != "ph1" {
		t.Fatalf("опрос: %v %v", msgs, err)
	}
	if err := c.Reply(ctx, "ph1", msgs[0].ID, KindPair2R, Pair2R{NPc: "y"}); err != nil {
		t.Fatalf("ответ: %v", err)
	}
	if err := c.Reply(ctx, "ph1", "нет-такого", KindPair2R, Pair2R{}); errCode(err) != StatusBadAddress {
		t.Fatalf("ответ без ожидателя: %v", err)
	}
	if err := c.SendReport(ctx, Signed{Data: "e30", Sig: "AA"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Unpair(ctx, "ph1"); err != nil {
		t.Fatal(err)
	}
	_, _, unpairs, reports := r.snapshot()
	if len(unpairs) != 1 || unpairs[0] != "ph1" || len(reports) != 1 {
		t.Fatalf("на сервере: %v %v", unpairs, reports)
	}
	r.mu.Lock()
	h := r.reqHeader[len(r.reqHeader)-1]
	r.mu.Unlock()
	if h.Get("Authorization") != "Bearer "+tok || h.Get("Content-Type") != "application/json" {
		t.Fatalf("заголовки: %v", h)
	}
}

func TestClientBadAnswers(t *testing.T) {
	var body []byte
	status := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.URL.Path == "/v1/pc/poll" && q.URL.Query().Get("wait") == "" {
			w.WriteHeader(400)
			return
		}
		if status == 302 {
			http.Redirect(w, q, "https://example.invalid/", 302)
			return
		}
		w.WriteHeader(status)
		w.Write(body)
	}))
	defer srv.Close()
	c := testClient(t, srv.URL)
	c.Token = "t"
	ctx := context.Background()
	cases := []struct {
		name   string
		status int
		body   []byte
		code   string
	}{
		{"1 МБ", 200, append([]byte(`{"messages":[],"x":"`), bytes.Repeat([]byte("a"), 1<<20)...), StatusBadAnswer},
		{"ровно за пределом", 200, append([]byte(`{"messages":[],"x":"`), bytes.Repeat([]byte("a"), MaxResponse)...), StatusBadAnswer},
		{"мусор", 200, []byte(`<html>`), StatusBadAnswer},
		{"массив", 200, []byte(`[]`), StatusBadAnswer},
		{"503", 503, []byte(`{"error":"busy"}`), StatusNoServer},
		{"504 без JSON", 504, []byte(`Gateway Timeout`), StatusNoServer},
		{"429", 429, []byte(`{"error":"too_many"}`), StatusTooMany},
		{"404", 404, []byte(`not found`), StatusBadAddress},
		{"401", 401, []byte(`{"error":"unauthorized"}`), StatusUnauthorized},
		{"перенаправление", 302, nil, StatusBadAnswer},
	}
	for _, cs := range cases {
		status, body = cs.status, cs.body
		_, err := c.Poll(ctx, 25*time.Second)
		if err == nil || errCode(err) != cs.code {
			t.Errorf("%s: %v (код %q, нужно %q)", cs.name, err, errCode(err), cs.code)
		}
	}
	status, body = 200, []byte(`{"messages":[],"pad":"`+strings.Repeat("a", MaxResponse-40)+`"}`)
	if _, err := c.Poll(ctx, 25*time.Second); err != nil {
		t.Fatalf("ответ чуть меньше 64 КБ отклонён: %v", err)
	}
	var se *StatusError
	status, body = 503, []byte(`{"error":"busy"}`)
	if _, err := c.Poll(ctx, time.Second); !errors.As(err, &se) || se.Code != "busy" || se.Status != 503 {
		t.Fatalf("StatusError: %v", err)
	}
}

func TestErrCodeNetwork(t *testing.T) {
	c := testClient(t, "http://127.0.0.1:1")
	_, _, err := c.Register(context.Background())
	if errCode(err) != StatusNoServer {
		t.Fatalf("нет сервера: %v → %q", err, errCode(err))
	}
	if errCode(nil) != "" {
		t.Fatal("nil")
	}
}

// Транспорт по умолчанию — без прокси из окружения (S56, NFR-1).
func TestTransportNoProxy(t *testing.T) {
	proxied := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxied++
		w.WriteHeader(502)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	tr := newTransport()
	if tr.Proxy != nil {
		t.Fatal("у транспорта задан Proxy")
	}
	r := newMiniRelay(t)
	// Go сам не шлёт запросы к 127.0.0.1 через прокси, поэтому главная проверка — Proxy == nil.
	c := testClient(t, r.srv.URL)
	if _, _, err := c.Register(context.Background()); err != nil {
		t.Fatalf("регистрация напрямую: %v", err)
	}
	if proxied != 0 {
		t.Fatal("запрос ушёл через прокси")
	}
	l := NewLink(Config{URL: c.Base, Host: newFakeHost()})
	if l.tr.(*http.Transport).Proxy != nil {
		t.Fatal("Link по умолчанию с прокси")
	}
}
