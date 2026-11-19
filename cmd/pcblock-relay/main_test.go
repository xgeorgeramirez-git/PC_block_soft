package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Ловит: у сервера не было ReadTimeout, медленное тело запроса держало соединение сколько
// угодно (R7). ReadTimeout считает чтение запроса, а не ответ: длинный опрос дольше
// ReadTimeout возвращается полностью (проверено на уменьшенных сроках).
func TestHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer(":0", http.NotFoundHandler(), context.Background())
	if srv.ReadTimeout <= 0 || srv.ReadTimeout > 15*time.Second || srv.ReadHeaderTimeout <= 0 || srv.WriteTimeout != 0 {
		t.Fatalf("сроки: read %v, header %v, write %v", srv.ReadTimeout, srv.ReadHeaderTimeout, srv.WriteTimeout)
	}

	e := newEnv(t, tcfg{PollMax: 600 * time.Millisecond, ReplyWait: 2 * time.Second})
	_, tok := e.register()
	hs := newHTTPServer("127.0.0.1:0", e.s.Handler(), context.Background())
	hs.ReadTimeout = 200 * time.Millisecond
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go hs.Serve(ln)
	t.Cleanup(func() { hs.Close() })
	base := "http://" + ln.Addr().String()

	q, _ := http.NewRequest("GET", base+"/v1/pc/poll?wait=25", nil)
	q.Header.Set("Authorization", "Bearer "+tok)
	start := time.Now()
	resp, err := http.DefaultClient.Do(q)
	if err != nil {
		t.Fatalf("опрос оборван: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if el := time.Since(start); resp.StatusCode != 200 || el < 500*time.Millisecond || !strings.Contains(string(raw), `"messages":[]`) {
		t.Fatalf("опрос: %d за %v: %s", resp.StatusCode, el, raw)
	}

	// Медленное тело обрывается по ReadTimeout.
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	io.WriteString(c, "POST /v1/pc/register HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{")
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	start = time.Now()
	buf := make([]byte, 512)
	n, _ := c.Read(buf)
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("медленное тело держало соединение %v (%q)", el, buf[:n])
	}
}
