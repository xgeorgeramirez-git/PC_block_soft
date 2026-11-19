package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	e := newEnv(t, tcfg{})
	r := e.get("/healthz", "")
	expect(t, r, 200, "")
	if r.M["ok"] != true {
		t.Fatalf("ждали ok:true: %s", r.Raw)
	}
	if !strings.HasPrefix(r.Raw, `{"ok":true`) {
		t.Fatalf("ответ должен начинаться с {\"ok\":true: %s", r.Raw)
	}
	expect(t, e.get("/v1/nope", ""), 404, "not_found")
}

func TestHealthCheckMode(t *testing.T) {
	e := newEnv(t, tcfg{})
	ts := httptest.NewServer(e.s.Handler())
	defer ts.Close()
	if c := healthCheck(ts.URL + "/healthz"); c != 0 {
		t.Fatalf("живой сервер: код %d", c)
	}
	if c := healthCheck(ts.URL + "/v1/nope"); c != 1 {
		t.Fatalf("404: код %d", c)
	}
	ts.Close()
	if c := healthCheck(ts.URL + "/healthz"); c != 1 {
		t.Fatalf("выключенный сервер: код %d", c)
	}
}

// Весь круг: регистрация → код → phone/pair доставлен опросом → pc/reply вернулся телефону;
// затем pair2 через phone/send, отчёт и phone/status, отвязка.
func TestFullCircle(t *testing.T) {
	e := newEnv(t, tcfg{PollMax: 2 * time.Second, ReplyWait: 2 * time.Second})
	pcID, pcTok := e.register()
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`).MatchString(pcID) {
		t.Fatalf("pcId не 16 байт base64url: %q", pcID)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(pcTok) {
		t.Fatalf("pcToken не 32 байта base64url: %q", pcTok)
	}
	pc := e.post("/v1/pc/pair-code", pcTok, `{}`)
	expect(t, pc, 200, "")
	code := pc.str("code")
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(code) {
		t.Fatalf("код не 6 цифр: %q", code)
	}
	if got, want := pc.num("expiresMs"), e.clock.Now().Add(10*time.Minute).UnixMilli(); got != want {
		t.Fatalf("expiresMs %d, ждали %d", got, want)
	}

	// Неизвестный ключ JSON не ошибка.
	ph := e.async(req{method: "POST", path: "/v1/phone/pair",
		body: `{"code":"` + code + `","kind":"pair1","body":{"phonePub":"AAA","name":"Pixel 7"},"extra":1}`})
	p := e.get("/v1/pc/poll?wait=25", pcTok)
	expect(t, p, 200, "")
	m := msgs(t, p)
	if len(m) != 1 || m[0]["kind"] != "pair1" {
		t.Fatalf("ждали pair1: %s", p.Raw)
	}
	body := m[0]["body"].(map[string]any)
	if body["phonePub"] != "AAA" || body["name"] != "Pixel 7" {
		t.Fatalf("тело не передано как есть: %s", p.Raw)
	}
	phoneID := m[0]["from"].(string)
	rep := e.post("/v1/pc/reply", pcTok, `{"to":"`+phoneID+`","replyTo":"`+m[0]["id"].(string)+`","kind":"pair1r","body":{"pcPub":"BBB","commit":"CCC"},"x":2}`)
	expect(t, rep, 200, "")
	r := wait(t, ph, 3*time.Second)
	expect(t, r, 200, "")
	if r.str("pcId") != pcID || r.str("phoneId") != phoneID || len(r.str("phoneToken")) != 43 {
		t.Fatalf("ответ привязки: %s", r.Raw)
	}
	reply := r.M["reply"].(map[string]any)
	if reply["kind"] != "pair1r" || reply["from"] != pcID || reply["body"].(map[string]any)["pcPub"] != "BBB" {
		t.Fatalf("reply: %s", r.Raw)
	}
	phTok := r.str("phoneToken")

	// Код одноразовый.
	expect(t, e.post("/v1/phone/pair", "", `{"code":"`+code+`","kind":"pair1","body":{}}`), 403, "bad_code")

	// pair2 через phone/send.
	snd := e.async(req{method: "POST", path: "/v1/phone/send", token: phTok, body: `{"kind":"pair2","body":{"nPhone":"NNN"}}`})
	p = e.get("/v1/pc/poll?wait=25", pcTok)
	m = msgs(t, p)
	if len(m) != 1 || m[0]["kind"] != "pair2" || m[0]["from"] != phoneID {
		t.Fatalf("ждали pair2: %s", p.Raw)
	}
	expect(t, e.post("/v1/pc/reply", pcTok, `{"to":"`+phoneID+`","replyTo":"`+m[0]["id"].(string)+`","kind":"pair2r","body":{"nPc":"PPP"}}`), 200, "")
	r = wait(t, snd, 3*time.Second)
	expect(t, r, 200, "")
	if rr := r.M["reply"].(map[string]any); rr["kind"] != "pair2r" || rr["body"].(map[string]any)["nPc"] != "PPP" {
		t.Fatalf("reply pair2: %s", r.Raw)
	}

	// Отчёт и статус.
	e.clock.Add(3 * time.Second)
	expect(t, e.post("/v1/pc/report", pcTok, `{"body":{"data":"REP","sig":"SIG"}}`), 200, "")
	st := e.get("/v1/phone/status", phTok)
	expect(t, st, 200, "")
	if st.num("pcLastSeenMs") != e.clock.Now().UnixMilli() {
		t.Fatalf("pcLastSeenMs: %s", st.Raw)
	}
	if rp := st.M["report"].(map[string]any); rp["data"] != "REP" || rp["sig"] != "SIG" {
		t.Fatalf("report: %s", st.Raw)
	}

	// Отвязка гасит токен телефона.
	expect(t, e.post("/v1/pc/unpair", pcTok, `{"phoneId":"`+phoneID+`"}`), 200, "")
	expect(t, e.get("/v1/phone/status", phTok), 401, "unauthorized")
	expect(t, e.post("/v1/pc/unpair", pcTok, `{"phoneId":"`+phoneID+`"}`), 404, "not_found")
}

func TestStatusBeforeReport(t *testing.T) {
	e := newEnv(t, tcfg{})
	p := e.pair()
	st := e.get("/v1/phone/status", p.phoneToken)
	expect(t, st, 200, "")
	if !strings.Contains(st.Raw, `"report":null`) {
		t.Fatalf("ждали report:null: %s", st.Raw)
	}
	seen := st.num("pcLastSeenMs")
	e.clock.Add(90 * time.Second)
	expect(t, e.get("/v1/pc/poll?wait=0", p.pcToken), 200, "")
	st = e.get("/v1/phone/status", p.phoneToken)
	if st.num("pcLastSeenMs") != e.clock.Now().UnixMilli() || st.num("pcLastSeenMs") == seen {
		t.Fatalf("опрос не отметил «на связи»: %s", st.Raw)
	}
	if st.num("serverNowMs") != e.clock.Now().UnixMilli() {
		t.Fatalf("serverNowMs: %s", st.Raw)
	}
}

func otherCode(code string) string {
	n, _ := strconv.Atoi(code)
	return fmt.Sprintf("%06d", (n+1)%1000000)
}

func TestPairCodeExpiredAndWrong(t *testing.T) {
	e := newEnv(t, tcfg{})
	_, pcTok := e.register()
	code := e.pairCode(pcTok)
	e.clock.Add(10*time.Minute + time.Second)
	pairBody := func(c string) string { return `{"code":"` + c + `","kind":"pair1","body":{}}` }
	expect(t, e.post("/v1/phone/pair", "", pairBody(code)), 403, "bad_code") // 1: истёк
	code = e.pairCode(pcTok)
	// Новый код гасит прежний (прежний уже истёк; проверим замену на свежем).
	code2 := e.pairCode(pcTok)
	expect(t, e.post("/v1/phone/pair", "", pairBody(code)), 403, "bad_code") // 2: погашен
	for i := 0; i < 3; i++ {                                                 // 3..5
		expect(t, e.post("/v1/phone/pair", "", pairBody(otherCode(code2))), 403, "bad_code")
	}
	// 6-я попытка за 10 мин — 429 даже с верным кодом.
	expect(t, e.post("/v1/phone/pair", "", pairBody(code2)), 429, "too_many")
	// Окно прошло — снова можно (код к тому времени тоже истёк, новый).
	e.clock.Add(10 * time.Minute)
	code3 := e.pairCode(pcTok)
	r := e.do(req{method: "POST", path: "/v1/phone/pair", body: pairBody(code3)})
	expect(t, r, 504, "pc_offline") // компьютер не ответил, но ограничения нет
	// Неверный формат кода и пустой запрос.
	expect(t, e.post("/v1/phone/pair", "", `{"kind":"pair1","body":{}}`), 400, "bad_request")
	expect(t, e.post("/v1/phone/pair", "", `{"code":"`+code3+`","kind":"cmd","body":{}}`), 400, "bad_request")
}

// Код компьютера защищён и от перебора с разных адресов.
func TestPairLimitPerPC(t *testing.T) {
	e := newEnv(t, tcfg{})
	_, pcTok := e.register()
	code := e.pairCode(pcTok)
	for i := 0; i < 5; i++ {
		r := e.do(req{method: "POST", path: "/v1/phone/pair", remote: fmt.Sprintf("10.0.0.%d:5000", i+1),
			body: `{"code":"` + otherCode(code) + `","kind":"pair1","body":{}}`})
		expect(t, r, 403, "bad_code")
	}
	r := e.do(req{method: "POST", path: "/v1/phone/pair", remote: "10.0.1.1:5000",
		body: `{"code":"` + code + `","kind":"pair1","body":{}}`})
	expect(t, r, 429, "too_many")
}

func TestXForwardedFor(t *testing.T) {
	wrong := func(e *env, i int) resp {
		return e.do(req{method: "POST", path: "/v1/phone/pair", remote: "172.18.0.2:4000",
			xff: fmt.Sprintf("6.6.6.6, 1.2.3.%d", i), body: `{"code":"123456","kind":"pair1","body":{}}`})
	}
	// Без -trust-proxy заголовок не учитывается: все запросы с одного адреса.
	e := newEnv(t, tcfg{})
	for i := 0; i < 5; i++ {
		expect(t, wrong(e, i), 403, "bad_code")
	}
	expect(t, wrong(e, 99), 429, "too_many")

	// С -trust-proxy адрес — последний элемент: разные адреса, ограничения нет.
	e = newEnv(t, tcfg{TrustProxy: true})
	for i := 0; i < 7; i++ {
		expect(t, wrong(e, i), 403, "bad_code")
	}
	// Тот же последний элемент — ограничение срабатывает, хотя первый элемент меняется.
	for i := 0; i < 4; i++ {
		r := e.do(req{method: "POST", path: "/v1/phone/pair", remote: "172.18.0.2:4000",
			xff: fmt.Sprintf("9.9.9.%d, 1.2.3.0", i), body: `{"code":"123456","kind":"pair1","body":{}}`})
		expect(t, r, 403, "bad_code")
	}
	r := e.do(req{method: "POST", path: "/v1/phone/pair", remote: "172.18.0.2:4000",
		xff: "8.8.8.8, 1.2.3.0", body: `{"code":"123456","kind":"pair1","body":{}}`})
	expect(t, r, 429, "too_many")
}

func TestRegisterLimits(t *testing.T) {
	e := newEnv(t, tcfg{})
	for i := 0; i < 10; i++ {
		expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "1.1.1.1:1", body: `{}`}), 200, "")
	}
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "1.1.1.1:1", body: `{}`}), 429, "too_many")
	e.clock.Add(time.Hour)
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "1.1.1.1:1"}), 200, "") // пустое тело можно
	for i := 0; i < 9; i++ {
		expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: fmt.Sprintf("2.2.2.%d:1", i), body: `{}`}), 200, "")
	}
	// 20 компьютеров на сервере — больше нельзя.
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "3.3.3.3:1", body: `{}`}), 429, "too_many")
}

func TestPollWaitsAndWakes(t *testing.T) {
	e := newEnv(t, tcfg{PollMax: 100 * time.Millisecond})
	_, pcTok := e.register()
	start := time.Now()
	r := e.get("/v1/pc/poll?wait=25", pcTok)
	el := time.Since(start)
	expect(t, r, 200, "")
	if !strings.Contains(r.Raw, `"messages":[]`) {
		t.Fatalf("пустой опрос — массив []: %s", r.Raw)
	}
	if el < 80*time.Millisecond || el > time.Second {
		t.Fatalf("опрос ждал %v, ждали ~100 мс (PollMax)", el)
	}
	start = time.Now()
	expect(t, e.get("/v1/pc/poll?wait=0", pcTok), 200, "")
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("wait=0 должен вернуться сразу")
	}
	expect(t, e.get("/v1/pc/poll?wait=abc", pcTok), 400, "bad_request")
	expect(t, e.get("/v1/pc/poll?wait=-1", pcTok), 400, "bad_request")

	// Просыпается на сообщение.
	e = newEnv(t, tcfg{PollMax: 3 * time.Second, ReplyWait: 3 * time.Second})
	p := e.pair()
	start = time.Now()
	poll := e.async(req{method: "GET", path: "/v1/pc/poll?wait=25", token: p.pcToken})
	time.Sleep(20 * time.Millisecond)
	snd := e.async(req{method: "POST", path: "/v1/phone/send", token: p.phoneToken, body: `{"kind":"cmd","body":{"data":"D","sig":"S"}}`})
	r = wait(t, poll, 2*time.Second)
	if el := time.Since(start); el > time.Second {
		t.Fatalf("опрос не проснулся на сообщение: %v", el)
	}
	m := msgs(t, r)
	if len(m) != 1 || m[0]["kind"] != "cmd" {
		t.Fatalf("ждали cmd: %s", r.Raw)
	}
	expect(t, e.post("/v1/pc/reply", p.pcToken, `{"to":"`+p.phoneID+`","replyTo":"`+m[0]["id"].(string)+`","kind":"cmdr","body":{"data":"R","sig":"S"}}`), 200, "")
	expect(t, wait(t, snd, 2*time.Second), 200, "")
	// Сообщение выдаётся один раз.
	r = e.get("/v1/pc/poll?wait=0", p.pcToken)
	if len(msgs(t, r)) != 0 {
		t.Fatalf("сообщение выдано дважды: %s", r.Raw)
	}
}

// Срок сообщения в очереди — 10 с (как ожидание ответа): старше не выдаётся.
func TestMessageOlderThanTTLNotDelivered(t *testing.T) {
	e := newEnv(t, tcfg{ReplyWait: 300 * time.Millisecond})
	p := e.pair()
	snd := e.async(req{method: "POST", path: "/v1/phone/send", token: p.phoneToken, body: `{"kind":"cmd","body":{}}`})
	waitFor(t, time.Second, func() bool { return e.s.queueLen(p.pcID) == 1 })
	e.clock.Add(10 * time.Second)
	r := e.get("/v1/pc/poll?wait=0", p.pcToken)
	if len(msgs(t, r)) != 0 {
		t.Fatalf("сообщение старше 10 с выдано: %s", r.Raw)
	}
	expect(t, wait(t, snd, 2*time.Second), 504, "pc_offline")

	// Моложе срока — выдаётся.
	snd = e.async(req{method: "POST", path: "/v1/phone/send", token: p.phoneToken, body: `{"kind":"cmd","body":{}}`})
	waitFor(t, time.Second, func() bool { return e.s.queueLen(p.pcID) == 1 })
	e.clock.Add(9 * time.Second)
	m := msgs(t, e.get("/v1/pc/poll?wait=0", p.pcToken))
	if len(m) != 1 {
		t.Fatal("живое сообщение не выдано")
	}
	expect(t, e.post("/v1/pc/reply", p.pcToken, `{"to":"`+p.phoneID+`","replyTo":"`+m[0]["id"].(string)+`","kind":"cmdr","body":{}}`), 200, "")
	expect(t, wait(t, snd, 2*time.Second), 200, "")
}

// После 504 на phone/pair сообщение тоже убрано: компьютер не увидит pair1, о котором
// телефону сказали «нет связи».
func TestPair504RemovesMessage(t *testing.T) {
	e := newEnv(t, tcfg{ReplyWait: 100 * time.Millisecond})
	pcID, pcTok := e.register()
	code := e.pairCode(pcTok)
	expect(t, e.post("/v1/phone/pair", "", `{"code":"`+code+`","kind":"pair1","body":{}}`), 504, "pc_offline")
	if n := e.s.queueLen(pcID); n != 0 {
		t.Fatalf("в очереди осталось %d", n)
	}
	if len(msgs(t, e.get("/v1/pc/poll?wait=0", pcTok))) != 0 {
		t.Fatal("сообщение выдано после 504")
	}
}

func TestSendWithoutReply504(t *testing.T) {
	e := newEnv(t, tcfg{ReplyWait: 100 * time.Millisecond})
	p := e.pair()
	start := time.Now()
	r := e.post("/v1/phone/send", p.phoneToken, `{"kind":"cmd","body":{}}`)
	el := time.Since(start)
	expect(t, r, 504, "pc_offline")
	if el < 90*time.Millisecond || el > time.Second {
		t.Fatalf("504 через %v, ждали ReplyWait ~100 мс", el)
	}
	// Неотвеченное сообщение убрано из очереди: компьютер его уже не выполнит.
	if n := e.s.queueLen(p.pcID); n != 0 {
		t.Fatalf("в очереди осталось %d", n)
	}
	// Опоздавший ответ — 404.
	expect(t, e.post("/v1/pc/reply", p.pcToken, `{"to":"`+p.phoneID+`","replyTo":"nope","kind":"cmdr","body":{}}`), 404, "not_found")
}

func TestQueueFull503(t *testing.T) {
	e := newEnv(t, tcfg{ReplyWait: 5 * time.Second})
	p := e.pair()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.do(req{method: "POST", path: "/v1/phone/send", token: p.phoneToken, body: `{"kind":"cmd","body":{}}`, ctx: ctx})
		}()
	}
	waitFor(t, 2*time.Second, func() bool { return e.s.queueLen(p.pcID) == 32 })
	expect(t, e.post("/v1/phone/send", p.phoneToken, `{"kind":"cmd","body":{}}`), 503, "busy")
	cancel()
	wg.Wait()
	if n := e.s.queueLen(p.pcID); n != 0 {
		t.Fatalf("после отмены в очереди %d", n)
	}
}

func TestUnauthorized(t *testing.T) {
	e := newEnv(t, tcfg{})
	p := e.pair()
	bad := strings.Repeat("A", 43)
	expect(t, e.get("/v1/pc/poll?wait=0", ""), 401, "unauthorized")
	expect(t, e.get("/v1/pc/poll?wait=0", bad), 401, "unauthorized")
	expect(t, e.do(req{method: "GET", path: "/v1/pc/poll?wait=0", token: ""}), 401, "unauthorized")
	// Токены компьютера и телефона не взаимозаменяемы.
	expect(t, e.get("/v1/pc/poll?wait=0", p.phoneToken), 401, "unauthorized")
	expect(t, e.get("/v1/phone/status", p.pcToken), 401, "unauthorized")
	expect(t, e.post("/v1/pc/pair-code", bad, `{}`), 401, "unauthorized")
	expect(t, e.post("/v1/pc/report", bad, `{"body":{}}`), 401, "unauthorized")
	expect(t, e.post("/v1/phone/send", bad, `{"kind":"cmd","body":{}}`), 401, "unauthorized")
	// Чужой компьютер не может отвязать чужой телефон.
	_, other := e.register()
	expect(t, e.post("/v1/pc/unpair", other, `{"phoneId":"`+p.phoneID+`"}`), 404, "not_found")
	// После отвязки токен телефона погашен.
	expect(t, e.post("/v1/pc/unpair", p.pcToken, `{"phoneId":"`+p.phoneID+`"}`), 200, "")
	expect(t, e.post("/v1/phone/send", p.phoneToken, `{"kind":"cmd","body":{}}`), 401, "unauthorized")
	expect(t, e.get("/v1/phone/status", p.phoneToken), 401, "unauthorized")
	// Компьютер жив.
	expect(t, e.get("/v1/pc/poll?wait=0", p.pcToken), 200, "")
}

func TestReplyOnlyToOwnWaiter(t *testing.T) {
	e := newEnv(t, tcfg{PollMax: 2 * time.Second, ReplyWait: 500 * time.Millisecond})
	p := e.pair()
	_, otherTok := e.register()
	snd := e.async(req{method: "POST", path: "/v1/phone/send", token: p.phoneToken, body: `{"kind":"cmd","body":{}}`})
	m := msgs(t, e.get("/v1/pc/poll?wait=25", p.pcToken))
	if len(m) != 1 {
		t.Fatal("нет сообщения")
	}
	id := m[0]["id"].(string)
	expect(t, e.post("/v1/pc/reply", otherTok, `{"to":"`+p.phoneID+`","replyTo":"`+id+`","kind":"cmdr","body":{}}`), 404, "not_found")
	expect(t, e.post("/v1/pc/reply", p.pcToken, `{"to":"someone","replyTo":"`+id+`","kind":"cmdr","body":{}}`), 404, "not_found")
	expect(t, e.post("/v1/pc/reply", p.pcToken, `{"to":"`+p.phoneID+`","replyTo":"`+id+`","kind":"","body":{}}`), 400, "bad_request")
	expect(t, e.post("/v1/pc/reply", p.pcToken, `{"to":"`+p.phoneID+`","replyTo":"`+id+`","kind":"cmdr","body":{"ok":1}}`), 200, "")
	expect(t, wait(t, snd, 2*time.Second), 200, "")
}

func TestRestartKeepsAccounts(t *testing.T) {
	dir := t.TempDir()
	e := newEnv(t, tcfg{Dir: dir, ReplyWait: 50 * time.Millisecond})
	p := e.pair()
	expect(t, e.post("/v1/pc/report", p.pcToken, `{"body":{"data":"x","sig":"y"}}`), 200, "")
	expect(t, e.post("/v1/phone/send", p.phoneToken, `{"kind":"cmd","body":{}}`), 504, "pc_offline")
	code := e.pairCode(p.pcToken)

	fi, err := os.Stat(filepath.Join(dir, "relay.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("права relay.json %v, ждали 0600", fi.Mode().Perm())
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "relay.json"))
	for _, secret := range []string{p.pcToken, p.phoneToken, code} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("в relay.json секрет в открытом виде: %s", raw)
		}
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(left) != 0 {
		t.Fatalf("остались временные файлы: %v", left)
	}

	e2 := newEnv(t, tcfg{Dir: dir})
	st := e2.get("/v1/phone/status", p.phoneToken)
	expect(t, st, 200, "")
	if !strings.Contains(st.Raw, `"report":null`) || st.num("pcLastSeenMs") != 0 {
		t.Fatalf("после перезапуска отчёт и «на связи» должны быть пусты: %s", st.Raw)
	}
	r := e2.get("/v1/pc/poll?wait=0", p.pcToken)
	expect(t, r, 200, "")
	if len(msgs(t, r)) != 0 {
		t.Fatalf("очередь после перезапуска не пуста: %s", r.Raw)
	}
	// Код в памяти: после перезапуска не действует.
	expect(t, e2.post("/v1/phone/pair", "", `{"code":"`+code+`","kind":"pair1","body":{}}`), 403, "bad_code")
	expect(t, e2.post("/v1/pc/unpair", p.pcToken, `{"phoneId":"`+p.phoneID+`"}`), 200, "")
}

func TestCorruptStoreRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "relay.json"), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{DataDir: dir}); err == nil {
		t.Fatal("испорченный relay.json должен давать ошибку запуска, а не тихо обнуляться")
	}
}

func TestBodyTooLarge413(t *testing.T) {
	e := newEnv(t, tcfg{})
	p := e.pair()
	big := `{"kind":"cmd","body":"` + strings.Repeat("x", 17*1024) + `"}`
	expect(t, e.post("/v1/phone/send", p.phoneToken, big), 413, "too_large")
	expect(t, e.post("/v1/pc/register", "", big), 413, "too_large")
	expect(t, e.post("/v1/pc/report", p.pcToken, `{"body":"`+strings.Repeat("x", 17*1024)+`"}`), 413, "too_large")
	// Ровно 16 КБ тела — можно.
	pad := 16*1024 - len(`{"body":""}`)
	expect(t, e.post("/v1/pc/report", p.pcToken, `{"body":"`+strings.Repeat("x", pad)+`"}`), 200, "")
}

func TestBadJSON400(t *testing.T) {
	e := newEnv(t, tcfg{})
	p := e.pair()
	for _, b := range []string{`{`, `[]`, `{"kind":1}`, `{"kind":"cmd"} {}`, `"x"`} {
		expect(t, e.post("/v1/phone/send", p.phoneToken, b), 400, "bad_request")
	}
	expect(t, e.post("/v1/phone/send", p.phoneToken, `{"kind":"CMD!","body":{}}`), 400, "bad_request")
	expect(t, e.post("/v1/pc/unpair", p.pcToken, `{}`), 400, "bad_request")
}

func TestPhonesPerPCBounded(t *testing.T) {
	e := newEnv(t, tcfg{PollMax: 2 * time.Second, ReplyWait: 2 * time.Second})
	pcID, pcTok := e.register()
	var tokens []string
	for i := 0; i < maxPhonesPerPC+1; i++ {
		code := e.pairCode(pcTok)
		ph := e.async(req{method: "POST", path: "/v1/phone/pair", remote: fmt.Sprintf("5.5.5.%d:1", i),
			body: `{"code":"` + code + `","kind":"pair1","body":{}}`})
		m := msgs(t, e.get("/v1/pc/poll?wait=25", pcTok))
		expect(t, e.post("/v1/pc/reply", pcTok, `{"to":"`+m[0]["from"].(string)+`","replyTo":"`+m[0]["id"].(string)+`","kind":"pair1r","body":{}}`), 200, "")
		r := wait(t, ph, 3*time.Second)
		expect(t, r, 200, "")
		tokens = append(tokens, r.str("phoneToken"))
		e.clock.Add(time.Second)
	}
	if n := e.s.phoneCount(pcID); n != maxPhonesPerPC {
		t.Fatalf("телефонов %d, ждали не больше %d", n, maxPhonesPerPC)
	}
	expect(t, e.get("/v1/phone/status", tokens[0]), 401, "unauthorized")
	expect(t, e.get("/v1/phone/status", tokens[len(tokens)-1]), 200, "")
}

func TestPollBudgetFitsClientLimit(t *testing.T) {
	e := newEnv(t, tcfg{ReplyWait: 2 * time.Second})
	p := e.pair()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := `{"kind":"cmd","body":"` + strings.Repeat("y", 15*1024) + `"}`
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.do(req{method: "POST", path: "/v1/phone/send", token: p.phoneToken, body: body, ctx: ctx})
		}()
	}
	waitFor(t, 2*time.Second, func() bool { return e.s.queueLen(p.pcID) == 8 })
	total := 0
	for i := 0; i < 8 && total < 8; i++ {
		r := e.get("/v1/pc/poll?wait=0", p.pcToken)
		if len(r.Raw) > 64*1024 {
			t.Fatalf("ответ опроса %d байт больше 64 КБ (предел клиента)", len(r.Raw))
		}
		total += len(msgs(t, r))
	}
	if total != 8 {
		t.Fatalf("выдано %d из 8", total)
	}
	cancel()
	wg.Wait()
}

func TestLogWithoutSecrets(t *testing.T) {
	e := newEnv(t, tcfg{})
	p := e.pair()
	code := e.pairCode(p.pcToken)
	e.post("/v1/pc/report", p.pcToken, `{"body":{"data":"SECRETBODY","sig":"s"}}`)
	e.get("/v1/phone/status", p.phoneToken)
	logs := e.logText()
	for _, s := range []string{p.pcToken, p.phoneToken, code, "SECRETBODY"} {
		if strings.Contains(logs, s) {
			t.Fatalf("в журнале секрет %q:\n%s", s, logs)
		}
	}
	if !strings.Contains(logs, "POST /v1/pc/register 200") || !strings.Contains(logs, "GET /v1/phone/status 200") {
		t.Fatalf("журнал без метода, пути и кода:\n%s", logs)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	e := newEnv(t, tcfg{})
	r := e.get("/v1/pc/register", "")
	if r.Code != http.StatusMethodNotAllowed && r.Code != http.StatusNotFound {
		t.Fatalf("GET register: %d", r.Code)
	}
}

// Компьютер, которого не было 30 дней, удаляется с телефонами: при регистрации, раз в час и при старте.
func TestStalePCPurged(t *testing.T) {
	dir := t.TempDir()
	e := newEnv(t, tcfg{Dir: dir})
	a := e.pair()
	b := e.pair()
	e.clock.Add(29 * 24 * time.Hour)
	expect(t, e.get("/v1/pc/poll?wait=0", b.pcToken), 200, "") // b на связи
	e.clock.Add(2 * 24 * time.Hour)
	// Раз в час: любой запрос после часа запускает чистку.
	expect(t, e.get("/v1/phone/status", b.phoneToken), 200, "")
	expect(t, e.get("/v1/phone/status", a.phoneToken), 401, "unauthorized")
	expect(t, e.get("/v1/pc/poll?wait=0", a.pcToken), 401, "unauthorized")
	if e.s.phoneCount(a.pcID) != 0 {
		t.Fatal("телефоны удалённого компьютера остались")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "relay.json"))
	if strings.Contains(string(raw), a.pcID) {
		t.Fatalf("удалённый компьютер остался в файле: %s", raw)
	}
	// При старте: через 31 день после последнего опроса b удаляется сразу.
	later := newClock()
	later.t = e.clock.Now().Add(31 * 24 * time.Hour)
	e2 := newEnv(t, tcfg{Dir: dir, Clock: later})
	if n := e2.s.pcCount(); n != 0 {
		t.Fatalf("при старте осталось компьютеров %d", n)
	}
}

func TestPurgeOnRegisterFreesSlots(t *testing.T) {
	e := newEnv(t, tcfg{})
	for i := 0; i < maxPCs; i++ {
		expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: fmt.Sprintf("7.7.%d.1:1", i), body: `{}`}), 200, "")
	}
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "8.8.8.8:1", body: `{}`}), 429, "too_many")
	e.clock.Add(30*24*time.Hour + time.Minute)
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "8.8.8.8:1", body: `{}`}), 200, "")
	if n := e.s.pcCount(); n != 1 {
		t.Fatalf("компьютеров %d, ждали 1", n)
	}
}

// «На связи» пишется в файл не чаще раза в час.
func TestLastSeenWrittenHourly(t *testing.T) {
	dir := t.TempDir()
	e := newEnv(t, tcfg{Dir: dir})
	_, tok := e.register()
	read := func() string { b, _ := os.ReadFile(filepath.Join(dir, "relay.json")); return string(b) }
	before := read()
	for i := 0; i < 59; i++ {
		e.clock.Add(time.Minute)
		expect(t, e.get("/v1/pc/poll?wait=0", tok), 200, "")
	}
	if read() != before {
		t.Fatal("файл переписан раньше чем через час")
	}
	e.clock.Add(time.Minute)
	expect(t, e.get("/v1/pc/poll?wait=0", tok), 200, "")
	if read() == before {
		t.Fatal("через час «на связи» не записано в файл")
	}
}

// pc/unpair гасит любой телефон этого компьютера, в том числе «брошенный» после прерванной сверки.
func TestUnpairAbandonedPhone(t *testing.T) {
	e := newEnv(t, tcfg{PollMax: 2 * time.Second, ReplyWait: 2 * time.Second})
	_, pcTok := e.register()
	var ids, toks []string
	for i := 0; i < 2; i++ {
		code := e.pairCode(pcTok)
		ph := e.async(req{method: "POST", path: "/v1/phone/pair", body: `{"code":"` + code + `","kind":"pair1","body":{}}`})
		m := msgs(t, e.get("/v1/pc/poll?wait=25", pcTok))
		expect(t, e.post("/v1/pc/reply", pcTok, `{"to":"`+m[0]["from"].(string)+`","replyTo":"`+m[0]["id"].(string)+`","kind":"pair1r","body":{}}`), 200, "")
		r := wait(t, ph, 3*time.Second)
		ids, toks = append(ids, r.str("phoneId")), append(toks, r.str("phoneToken"))
	}
	expect(t, e.post("/v1/pc/unpair", pcTok, `{"phoneId":"`+ids[0]+`"}`), 200, "")
	expect(t, e.get("/v1/phone/status", toks[0]), 401, "unauthorized")
	expect(t, e.get("/v1/phone/status", toks[1]), 200, "")
}

// Ловит: json.Marshal экранировал < > & шестью байтами, и опрос с телом из «<» выходил за
// 64 КБ клиента, хотя сообщения уже сняты с очереди (R2). Бюджет — по закодированным байтам.
func TestPollBudgetCountsEncodedBytes(t *testing.T) {
	for _, fill := range []string{"<", "&", " "} {
		e := newEnv(t, tcfg{ReplyWait: 2 * time.Second})
		p := e.pair()
		ctx, cancel := context.WithCancel(context.Background())
		n := 15 * 1024 / len(fill)
		body := `{"kind":"cmd","body":"` + strings.Repeat(fill, n) + `"}`
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e.do(req{method: "POST", path: "/v1/phone/send", token: p.phoneToken, body: body, ctx: ctx})
			}()
		}
		waitFor(t, 2*time.Second, func() bool { return e.s.queueLen(p.pcID) == 4 })
		total := 0
		for i := 0; i < 8 && total < 4; i++ {
			r := e.get("/v1/pc/poll?wait=0", p.pcToken)
			if len(r.Raw) > 64*1024 {
				t.Fatalf("%q: ответ опроса %d байт больше 64 КБ", fill, len(r.Raw))
			}
			for _, m := range msgs(t, r) {
				if b, _ := m["body"].(string); b != strings.Repeat(fill, n) {
					t.Fatalf("%q: тело искажено", fill)
				}
				total++
			}
		}
		if total != 4 {
			t.Fatalf("%q: выдано %d из 4", fill, total)
		}
		if strings.Contains(e.get("/v1/phone/status", p.phoneToken).Raw, `<`) {
			t.Fatal("ответ экранирует HTML")
		}
		cancel()
		wg.Wait()
	}
}

// Ловит: 20 мест на сервере занимались регистрациями без единого запроса компьютера на 30 дней
// (R3). Зарегистрированный и ни разу не вышедший на связь компьютер удаляется через 24 ч; тот,
// что выходил на связь, остаётся.
func TestUnseenPCPurgedAfterDay(t *testing.T) {
	e := newEnv(t, tcfg{})
	_, aliveTok := e.register()
	expect(t, e.get("/v1/pc/poll?wait=0", aliveTok), 200, "")
	for i := 1; i < maxPCs; i++ {
		expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: fmt.Sprintf("7.7.%d.1:1", i), body: `{}`}), 200, "")
	}
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "8.8.8.8:1", body: `{}`}), 429, "too_many")
	e.clock.Add(23 * time.Hour)
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "8.8.8.8:1", body: `{}`}), 429, "too_many")
	e.clock.Add(time.Hour + time.Minute)
	expect(t, e.do(req{method: "POST", path: "/v1/pc/register", remote: "8.8.8.8:1", body: `{}`}), 200, "")
	if n := e.s.pcCount(); n != 2 {
		t.Fatalf("компьютеров %d, ждали 2 (вышедший на связь и новый)", n)
	}
	expect(t, e.get("/v1/pc/poll?wait=0", aliveTok), 200, "")
}

// Ловит: pair1 через phone/send обходил код и пределы phone/pair (R6).
func TestSendRejectsPair1(t *testing.T) {
	e := newEnv(t, tcfg{})
	p := e.pair()
	expect(t, e.post("/v1/phone/send", p.phoneToken, `{"kind":"pair1","body":{}}`), 400, "bad_request")
	if n := e.s.queueLen(p.pcID); n != 0 {
		t.Fatalf("в очереди %d", n)
	}
}

// Ловит: 504 на phone/pair возвращал код, хотя опрос уже выдал pair1 компьютеру (R8): второй
// телефон мог бы ввести тот же код, а компьютер уже на этапе wait_nonce.
func TestPair504AfterDeliveryKeepsCodeSpent(t *testing.T) {
	e := newEnv(t, tcfg{PollMax: 2 * time.Second, ReplyWait: 200 * time.Millisecond})
	_, pcTok := e.register()
	code := e.pairCode(pcTok)
	ph := e.async(req{method: "POST", path: "/v1/phone/pair", body: `{"code":"` + code + `","kind":"pair1","body":{}}`})
	if m := msgs(t, e.get("/v1/pc/poll?wait=25", pcTok)); len(m) != 1 {
		t.Fatalf("pair1 не выдан: %v", m)
	}
	expect(t, wait(t, ph, 2*time.Second), 504, "pc_offline")
	expect(t, e.post("/v1/phone/pair", "", `{"code":"`+code+`","kind":"pair1","body":{}}`), 403, "bad_code")

	// Компьютер pair1 не получил — код возвращается: телефон может повторить тот же код.
	code = e.pairCode(pcTok)
	expect(t, e.post("/v1/phone/pair", "", `{"code":"`+code+`","kind":"pair1","body":{}}`), 504, "pc_offline")
	expect(t, e.post("/v1/phone/pair", "", `{"code":"`+code+`","kind":"pair1","body":{}}`), 504, "pc_offline")
}
