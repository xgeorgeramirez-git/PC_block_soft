package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/remote"
)

// fakePC — сервер связи и компьютер в одном: отвечает телефону так, как ответил бы компьютер
// через сервер (протокол, разделы 3…6), проверяя команды remote.CheckCommand.
type fakePC struct {
	t   *testing.T
	key *ecdsa.PrivateKey

	mu         sync.Mutex
	phonePub   string
	phoneDER   []byte
	nPc        []byte
	sas        string
	lastSeq    int64
	extra      int
	badCommit  bool // подменить обязательство (взломанный сервер)
	wrongHash  bool // ответ cmdr не на эту команду
	srv        *httptest.Server
	cmdApplied int
}

func newFakePC(t *testing.T) *fakePC {
	k, err := remote.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakePC{t: t, key: k, nPc: make([]byte, remote.NonceLen)}
	rand.Read(f.nPc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/phone/pair", f.hPair)
	mux.HandleFunc("POST /v1/phone/send", f.hSend)
	mux.HandleFunc("GET /v1/phone/status", f.hStatus)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePC) reply(w http.ResponseWriter, code int, v map[string]any) {
	v["serverNowMs"] = time.Now().UnixMilli()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (f *fakePC) pcPubDER() []byte {
	der, _ := unb64(remote.MarshalPub(&f.key.PublicKey))
	return der
}

func (f *fakePC) hPair(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string       `json:"code"`
		Kind string       `json:"kind"`
		Body remote.Pair1 `json:"body"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	if in.Code != "123456" || in.Kind != remote.KindPair1 {
		f.reply(w, 403, map[string]any{"error": "bad_code"})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.phonePub = in.Body.PhonePub
	f.phoneDER, _ = unb64(in.Body.PhonePub)
	commit := remote.Commit(f.pcPubDER(), f.phoneDER, f.nPc)
	if f.badCommit {
		commit = remote.Commit(f.pcPubDER(), f.phoneDER, make([]byte, remote.NonceLen))
	}
	body, _ := json.Marshal(remote.Pair1R{PCPub: remote.MarshalPub(&f.key.PublicKey), Commit: b64(commit)})
	f.reply(w, 200, map[string]any{"pcId": "pc1", "phoneId": "ph1", "phoneToken": "tok",
		"reply": remote.Msg{ID: "m1", From: "ph1", Kind: remote.KindPair1R, Body: body}})
}

func (f *fakePC) signed(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	s, err := remote.Sign(f.key, raw)
	if err != nil {
		f.t.Fatal(err)
	}
	out, _ := json.Marshal(s)
	return out
}

func (f *fakePC) report() remote.Report {
	return remote.Report{V: 1, T: remote.TypeReport, PC: "pc1", Phone: "ph1", TS: time.Now().UnixMilli(),
		Children: []remote.ChildReport{{ID: 3, Name: "Миша", UsedSec: 60, LeftSec: int64(1800 + f.extra*60), ExtraMin: f.extra}}}
}

func (f *fakePC) hSend(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer tok" {
		f.reply(w, 401, map[string]any{"error": "unauthorized"})
		return
	}
	var in struct {
		Kind string          `json:"kind"`
		Body json.RawMessage `json:"body"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	switch in.Kind {
	case remote.KindPair2:
		var p remote.Pair2
		json.Unmarshal(in.Body, &p)
		nPhone, _ := unb64(p.NPhone)
		f.sas = remote.SAS(f.pcPubDER(), f.phoneDER, f.nPc, nPhone)
		body, _ := json.Marshal(remote.Pair2R{NPc: b64(f.nPc)})
		f.reply(w, 200, map[string]any{"reply": remote.Msg{ID: "m2", From: "ph1", Kind: remote.KindPair2R, Body: body}})
	case remote.KindCmd:
		var s remote.Signed
		json.Unmarshal(in.Body, &s)
		st := remote.State{PCID: "pc1", Phone: &remote.Phone{ID: "ph1", Pub: f.phonePub}, LastSeq: f.lastSeq}
		c, _, code := remote.CheckCommand(st, s, "ph1", time.Now())
		data, _ := unb64(s.Data) // как служба: хэш присланных байтов, даже при неверной подписи
		res := remote.CmdResult{V: 1, T: remote.TypeCmdR, PC: "pc1", Phone: "ph1", Seq: c.Seq, Cmd: remote.HashData(data), TS: time.Now().UnixMilli()}
		if f.wrongHash {
			res.Cmd = remote.HashData([]byte("другая"))
		}
		if code == "" {
			f.lastSeq, f.extra, f.cmdApplied = c.Seq, f.extra+c.Delta, f.cmdApplied+1
			rep := f.report()
			res.OK, res.Message, res.Report = true, "Добавлено", &rep
		} else {
			last := f.lastSeq
			res.Error, res.LastSeq = code, &last
		}
		f.reply(w, 200, map[string]any{"reply": remote.Msg{ID: "m3", From: "ph1", Kind: remote.KindCmdR, Body: f.signed(res)}})
	default:
		f.reply(w, 400, map[string]any{"error": "bad_request"})
	}
}

func (f *fakePC) hStatus(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply(w, 200, map[string]any{"pcLastSeenMs": time.Now().UnixMilli() - 1000, "report": f.signed(f.report())})
}

// runJSON — запуск подкоманды: код выхода и разобранная строка вывода.
func runJSON(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	a := &app{out: &buf, http: &http.Client{Timeout: 10 * time.Second}, now: time.Now}
	code := a.run(args)
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("%v: вывод не JSON: %q", args, buf.String())
	}
	return code, out
}

// Ловит: тестовый телефон неверно ведёт привязку, принимает чужие ответы или не умеет
// подделку и повтор, на которых S55 проверяет отказ службы.
func TestPhoneFlow(t *testing.T) {
	f := newFakePC(t)
	state := filepath.Join(t.TempDir(), "phone.json")

	if code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL, "-code", "000000"); code == 0 || out["error"] != "bad_code" {
		t.Fatalf("неверный код: код выхода %d, вывод %v", code, out)
	}
	code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL+"/", "-code", "123456", "-name", "Pixel")
	if code != 0 {
		t.Fatalf("pair: %d %v", code, out)
	}
	if out["sas"] != f.sas || len(f.sas) != 6 || out["pcId"] != "pc1" || out["phoneId"] != "ph1" {
		t.Fatalf("pair: вывод %v, у компьютера число %q", out, f.sas)
	}
	if code, out := runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "15"); code != 1 || out["error"] != errNotConfirmed {
		t.Fatalf("команда до confirm: %d %v", code, out)
	}
	if code, _ := runJSON(t, "confirm", "-state", state); code != 0 {
		t.Fatal("confirm")
	}

	code, out = runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "15")
	if code != 0 || out["ok"] != true || out["leftSec"] != float64(2700) || out["seq"] != float64(1) {
		t.Fatalf("cmd +15: %d %v", code, out)
	}
	if _, ok := out["ms"].(float64); !ok {
		t.Fatalf("cmd: нет ms: %v", out)
	}
	code, out = runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "-15")
	if code != 0 || out["ok"] != true || out["leftSec"] != float64(1800) || out["seq"] != float64(2) {
		t.Fatalf("cmd -15: %d %v", code, out)
	}

	code, out = runJSON(t, "forge-cmd", "-state", state, "-child", "3", "-delta", "15")
	if code != 2 || out["ok"] != false || out["error"] != remote.ErrCodeBadSig {
		t.Fatalf("forge-cmd: %d %v", code, out)
	}
	code, out = runJSON(t, "replay", "-state", state)
	if code != 2 || out["error"] != remote.ErrCodeReplay || out["seq"] != float64(2) || out["lastSeq"] != float64(2) {
		t.Fatalf("replay: %d %v", code, out)
	}
	if f.cmdApplied != 2 || f.extra != 0 {
		t.Fatalf("применено %d команд, доп. время %d; ждали 2 и 0", f.cmdApplied, f.extra)
	}
	// forge-cmd не тратит seq: следующая настоящая команда — 3.
	if code, out := runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "1"); code != 0 || out["seq"] != float64(3) {
		t.Fatalf("cmd после forge: %d %v", code, out)
	}

	code, out = runJSON(t, "status", "-state", state)
	if code != 0 || out["online"] != true || out["report"] != true {
		t.Fatalf("status: %d %v", code, out)
	}
	ch, _ := out["children"].([]any)
	if len(ch) != 1 || ch[0].(map[string]any)["leftSec"] != float64(1860) {
		t.Fatalf("status: дети %v", out["children"])
	}

	f.mu.Lock()
	f.wrongHash = true
	f.mu.Unlock()
	if code, out := runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "1"); code != 1 || out["error"] != errBadSig {
		t.Fatalf("ответ не на нашу команду принят: %d %v", code, out)
	}
}

// Ловит: телефон принимает обязательство, не совпавшее с раскрытым nPc (сервер подставил ключ).
func TestPhoneBadCommit(t *testing.T) {
	f := newFakePC(t)
	f.badCommit = true
	state := filepath.Join(t.TempDir(), "phone.json")
	code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL, "-code", "123456")
	if code != 1 || out["error"] != errCommit {
		t.Fatalf("неверное обязательство: %d %v", code, out)
	}
}

// Ловит: неизвестная подкоманда или нет файла состояния — не JSON или код 0.
func TestPhoneUsage(t *testing.T) {
	if code, out := runJSON(t, "nope"); code != 1 || out["error"] != errUsage {
		t.Fatalf("%d %v", code, out)
	}
	missing := filepath.Join(t.TempDir(), "нет.json")
	if code, out := runJSON(t, "status", "-state", missing); code != 1 || !strings.Contains(out["error"].(string), "no_state") {
		t.Fatalf("%d %v", code, out)
	}
}
