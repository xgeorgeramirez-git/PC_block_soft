package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
	notes      []remote.NoteReport
	noteData   string      // data последнего сообщения (FB-28: есть ли поля sticker и sound)
	note       remote.Note // последнее принятое сообщение после CheckNote
	forbid     string      // запретные часы в отчёте (раздел 6)
	forbidEnd  int64
	today      *remote.TodayReport    // статистика за сегодня в отчёте (FR-43)
	bank       int                    // копилка Миши (раздел 5в)
	promos     []remote.PromoActive   // действующие промокоды (раздел 5г)
	pending    []remote.PendingReport // ждущие решения (раздел 5д)
	ops        []string               // применённые команды 1.5: op и разобранные поля
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
		Children: []remote.ChildReport{{ID: 3, Name: "Миша", UsedSec: 60, LeftSec: int64(1800 + f.extra*60), ExtraMin: f.extra, Forbid: f.forbid, ForbidUntilMs: f.forbidEnd, Today: f.today, BankMin: f.bank}},
		Notes:    append([]remote.NoteReport(nil), f.notes...),
		Pending:  append([]remote.PendingReport{}, f.pending...), PendingCount: len(f.pending)}
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
			f.lastSeq, f.cmdApplied = c.Seq, f.cmdApplied+1
			msg := "Добавлено"
			switch c.Op {
			case "", remote.OpExtra:
				f.extra += c.Delta
			case remote.OpNoForbidden:
				f.forbid, f.forbidEnd, msg = remote.ForbidCleared, 0, "Запрет на сегодня снят"
			default:
				msg = f.applyOp(c, &res)
			}
			if res.Error == "" && res.Promos == nil {
				rep := f.report()
				res.Report = &rep
			}
			res.OK, res.Message = res.Error == "", msg
		} else {
			last := f.lastSeq
			res.Error, res.LastSeq = code, &last
		}
		f.reply(w, 200, map[string]any{"reply": remote.Msg{ID: "m3", From: "ph1", Kind: remote.KindCmdR, Body: f.signed(res)}})
	case remote.KindNote:
		var s remote.Signed
		json.Unmarshal(in.Body, &s)
		st := remote.State{PCID: "pc1", Phone: &remote.Phone{ID: "ph1", Pub: f.phonePub}, LastSeq: f.lastSeq}
		n, _, code := remote.CheckNote(st, s, "ph1", time.Now())
		data, _ := unb64(s.Data)
		f.noteData = string(data)
		res := remote.CmdResult{V: 1, T: remote.TypeNoteR, PC: "pc1", Phone: "ph1", Seq: n.Seq, Cmd: remote.HashData(data), TS: time.Now().UnixMilli()}
		if code == "" {
			f.lastSeq, f.note = n.Seq, n
			f.notes = append(f.notes, remote.NoteReport{ID: n.ID, Child: n.Child, State: remote.NoteQueued, AtMs: 1})
			rep := f.report()
			res.OK, res.Report = true, &rep
		} else {
			last := f.lastSeq
			res.Error, res.LastSeq = code, &last
		}
		f.reply(w, 200, map[string]any{"reply": remote.Msg{ID: "m4", From: "ph1", Kind: remote.KindNoteR, Body: f.signed(res)}})
	default:
		f.reply(w, 400, map[string]any{"error": "bad_request"})
	}
}

// applyOp — команды 1.5 фейка компьютера (под f.mu): копилка, промокоды (ответ со списком), решение.
func (f *fakePC) applyOp(c remote.Command, res *remote.CmdResult) string {
	f.ops = append(f.ops, fmt.Sprintf("%s:%d:%s:%d:%d:%d:%s:%s:%s", c.Op, c.Delta, c.Code, c.Min, c.Count, c.Len, c.Charset, c.Key, c.Cat))
	list := func() *remote.PromoList {
		return &remote.PromoList{Active: append([]remote.PromoActive{}, f.promos...), Used: []remote.PromoUsed{}}
	}
	switch c.Op {
	case remote.OpBankAdd:
		f.bank += c.Delta
		return fmt.Sprintf("Положено %d минут. В копилке %d мин", c.Delta, f.bank)
	case remote.OpBankClear:
		f.bank = 0
		return "Копилка обнулена"
	case remote.OpPromoList:
		res.Promos = list()
		return ""
	case remote.OpPromoCreate:
		f.promos = append(f.promos, remote.PromoActive{Code: c.Code, Min: c.Min, CreatedMs: 1})
		res.Promos = list()
		return "Промокод " + c.Code + " готов"
	case remote.OpPromoGen:
		res.Codes = []string{"K7PX", "M3QA"}
		res.Promos = list()
		return "Готово"
	case remote.OpPromoDelete:
		res.Promos = list()
		res.Error = remote.ErrCodeNotFound
		return ""
	case remote.OpDecide:
		for i, p := range f.pending {
			if p.ID == c.Key {
				f.pending = append(f.pending[:i], f.pending[i+1:]...)
				return p.Name + ": игра"
			}
		}
		res.Error = remote.ErrCodeNotFound
	}
	return ""
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

// Ловит: тестовый телефон шлёт сообщение не в общей нумерации с командами, тратит номер на подделку,
// не видит состояния сообщений в проверенном отчёте (S57).
func TestPhoneNotes(t *testing.T) {
	f := newFakePC(t)
	state := filepath.Join(t.TempDir(), "phone.json")
	if code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL, "-code", "123456"); code != 0 {
		t.Fatalf("pair: %v", out)
	}
	runJSON(t, "confirm", "-state", state)
	runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "1")
	code, out := runJSON(t, "note", "-state", state, "-child", "3", "-text", "Иди ужинать", "-show", "0")
	id, _ := out["id"].(string)
	if code != 0 || out["ok"] != true || out["seq"] != float64(2) || len(id) != 22 {
		t.Fatalf("note: %d %v", code, out)
	}
	if _, ok := out["ms"].(float64); !ok {
		t.Fatalf("note: нет ms: %v", out)
	}
	if code, out := runJSON(t, "forge-note", "-state", state, "-child", "3", "-text", "Чужое", "-show", "0"); code != 2 || out["error"] != remote.ErrCodeBadSig {
		t.Fatalf("forge-note: %d %v", code, out)
	}
	if code, out := runJSON(t, "note", "-state", state, "-child", "3", "-text", "x", "-show", "15"); code != 2 || out["error"] != remote.ErrCodeRange || out["seq"] != float64(3) {
		t.Fatalf("show 15: %d %v", code, out)
	}
	if code, out := runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "1"); code != 0 || out["seq"] != float64(4) {
		t.Fatalf("команда после сообщений: %d %v", code, out)
	}
	code, out = runJSON(t, "notes", "-state", state)
	notes, _ := out["notes"].([]any)
	if code != 0 || len(notes) != 1 || notes[0].(map[string]any)["id"] != id || notes[0].(map[string]any)["state"] != remote.NoteQueued {
		t.Fatalf("notes: %d %v", code, out)
	}
	if code, out := runJSON(t, "note", "-state", state, "-child", "3"); code != 1 || out["error"] != errUsage {
		t.Fatalf("без текста: %d %v", code, out)
	}
}

// Ловит (FB-28): без -sticker и -sound поля всё же уходят (телефон 0.8.2 их не шлёт); звук 0
// пропадает (omitempty), стикер или звук уходят не тем JSON-типом; неверный -sound (не целое)
// уходит на компьютер вместо ошибки вызова.
func TestPhoneNoteStickerSound(t *testing.T) {
	f := newFakePC(t)
	state := filepath.Join(t.TempDir(), "phone.json")
	if code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL, "-code", "123456"); code != 0 {
		t.Fatalf("pair: %v", out)
	}
	runJSON(t, "confirm", "-state", state)
	last := func() (string, remote.Note) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.noteData, f.note
	}
	if code, out := runJSON(t, "note", "-state", state, "-child", "3", "-text", "Иди ужинать"); code != 0 {
		t.Fatalf("без стикера: %d %v", code, out)
	}
	if data, n := last(); strings.Contains(data, "sticker") || strings.Contains(data, "sound") || n.Sticker != "" || n.Sound != 1 {
		t.Fatalf("без стикера и звука: %s %+v", data, n)
	}
	if code, out := runJSON(t, "note", "-state", state, "-child", "3", "-text", "Иди", "-sticker", "033", "-sound", "2"); code != 0 {
		t.Fatalf("стикер и звук: %d %v", code, out)
	}
	if data, n := last(); !strings.HasSuffix(data, `"showSec":0,"sticker":"033","sound":2}`) || n.Sticker != "033" || n.Sound != 2 {
		t.Fatalf("стикер и звук: %s %+v", data, n)
	}
	if code, out := runJSON(t, "note", "-state", state, "-child", "3", "-text", "Иди", "-sound", "0"); code != 0 {
		t.Fatalf("звук 0: %d %v", code, out)
	}
	if data, n := last(); !strings.HasSuffix(data, `"showSec":0,"sound":0}`) || n.Sound != 0 {
		t.Fatalf("звук 0: %s %+v", data, n)
	}
	// Неверные значения уходят как есть: компьютер отвечает range.
	for _, bad := range [][]string{{"-sticker", "33"}, {"-sticker", "abc"}, {"-sound", "100"}, {"-sound", "-1"}} {
		args := append([]string{"note", "-state", state, "-child", "3", "-text", "Иди"}, bad...)
		if code, out := runJSON(t, args...); code != 2 || out["error"] != remote.ErrCodeRange {
			t.Fatalf("%v: %d %v", bad, code, out)
		}
		if data, _ := last(); !strings.Contains(data, `"`+strings.TrimPrefix(bad[0], "-")+`":`) {
			t.Fatalf("%v: поле не ушло: %s", bad, data)
		}
	}
	if code, out := runJSON(t, "note", "-state", state, "-child", "3", "-text", "Иди", "-sound", "два"); code != 1 || out["error"] != errUsage {
		t.Fatalf("-sound не число: %d %v", code, out)
	}
}

// Ловит: «снять запрет» (протокол 5б) уходит без op или с delta, не в общей нумерации seq, подделка
// тратит номер; статус запретных часов из отчёта не печатается (S58).
func TestPhoneForbidOff(t *testing.T) {
	f := newFakePC(t)
	f.forbid, f.forbidEnd = remote.ForbidActive, 1763496000000
	state := filepath.Join(t.TempDir(), "phone.json")
	if code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL, "-code", "123456"); code != 0 {
		t.Fatalf("pair: %v", out)
	}
	runJSON(t, "confirm", "-state", state)
	code, out := runJSON(t, "status", "-state", state)
	ch, _ := out["children"].([]any)
	if code != 0 || len(ch) != 1 || ch[0].(map[string]any)["forbid"] != remote.ForbidActive || ch[0].(map[string]any)["forbidUntilMs"] != float64(1763496000000) {
		t.Fatalf("status до снятия: %d %v", code, out)
	}
	runJSON(t, "cmd", "-state", state, "-child", "3", "-delta", "30")
	if code, out := runJSON(t, "forge-forbid-off", "-state", state, "-child", "3"); code != 2 || out["error"] != remote.ErrCodeBadSig {
		t.Fatalf("forge-forbid-off: %d %v", code, out)
	}
	if code, out := runJSON(t, "forbid-off", "-state", state); code != 1 || out["error"] != errUsage {
		t.Fatalf("forbid-off без -child: %d %v", code, out)
	}
	code, out = runJSON(t, "forbid-off", "-state", state, "-child", "3")
	if code != 0 || out["ok"] != true || out["seq"] != float64(2) || out["message"] != "Запрет на сегодня снят" || out["forbid"] != remote.ForbidCleared {
		t.Fatalf("forbid-off: %d %v", code, out)
	}
	if _, ok := out["forbidUntilMs"]; ok {
		t.Fatalf("forbid-off: конец интервала у снятого запрета: %v", out)
	}
	if code, out := runJSON(t, "replay", "-state", state); code != 2 || out["error"] != remote.ErrCodeReplay || out["seq"] != float64(2) {
		t.Fatalf("replay «снять запрет»: %d %v", code, out)
	}
	f.mu.Lock()
	applied, extra := f.cmdApplied, f.extra
	f.mu.Unlock()
	if applied != 2 || extra != 30 {
		t.Fatalf("применено %d, доп. время %d; ждали 2 и 30", applied, extra)
	}
	code, out = runJSON(t, "status", "-state", state)
	ch, _ = out["children"].([]any)
	if code != 0 || ch[0].(map[string]any)["forbid"] != remote.ForbidCleared {
		t.Fatalf("status после снятия: %d %v", code, out)
	}
}

// Ловит: тестовый телефон не печатает «Статистику за сегодня» (S59 сверяет её с окном), today
// без -child, неизвестный ребёнок; вывод для человека теряет игры и «за компьютером».
func TestPhoneToday(t *testing.T) {
	f := newFakePC(t)
	f.today = &remote.TodayReport{FirstMs: 1763460000000, NowAt: true, SinceMs: 1763461000000, UsedSec: 3900, LeftSec: 1800,
		GamesSec: 1200, StudySec: 600, Programs: []remote.TimeItem{{Name: "Minecraft", Cat: remote.ItemGame, Sec: 1200}},
		Sites: []remote.TimeItem{}, More: remote.TodayMore{Sites: 2}, Violations: 2, Bypass: 1}
	state := filepath.Join(t.TempDir(), "phone.json")
	if code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL, "-code", "123456"); code != 0 {
		t.Fatalf("pair: %v", out)
	}
	runJSON(t, "confirm", "-state", state)
	code, out := runJSON(t, "today", "-state", state, "-child", "3")
	td, _ := out["today"].(map[string]any)
	progs, _ := td["programs"].([]any)
	if code != 0 || out["child"] != float64(3) || td["firstMs"] != float64(1763460000000) || td["gamesSec"] != float64(1200) || len(progs) != 1 ||
		progs[0].(map[string]any)["cat"] != remote.ItemGame || td["nowAt"] != true || out["reportTs"] == nil {
		t.Fatalf("today: %d %v", code, out)
	}
	if code, out := runJSON(t, "status", "-state", state); code != 0 || out["children"].([]any)[0].(map[string]any)["today"] == nil {
		t.Fatalf("status без today: %v", out)
	}
	if code, out := runJSON(t, "today", "-state", state); code != 1 || out["error"] != errUsage {
		t.Fatalf("без -child: %d %v", code, out)
	}
	if code, out := runJSON(t, "today", "-state", state, "-child", "9"); code != 1 || out["error"] != errNoChild {
		t.Fatalf("нет ребёнка: %d %v", code, out)
	}
	var buf bytes.Buffer
	a := &app{out: &buf, http: &http.Client{Timeout: 10 * time.Second}, now: time.Now}
	if a.run([]string{"status", "-state", state, "-human"}) != 0 {
		t.Fatalf("status -human: %s", buf.String())
	}
	text := buf.String()
	for _, want := range []string{"На связи", "Миша (id 3)", "За компьютером с ", "первый вход ", "игры 20 мин", "учёба 10 мин", "Minecraft (game) 20 мин", "сайты: и ещё 2", "из них попыток обхода 1"} {
		if !strings.Contains(text, want) {
			t.Errorf("status -human без %q:\n%s", want, text)
		}
	}
}

// Ловит: тестовый телефон собирает команды 1.5 не теми байтами, что векторы Go↔Kotlin (порядок
// полей, числа строкой, лишние поля), и приёмка проверяет не то, что пошлёт Android.
func TestOpCommandVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "remote", "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	key, _ := v["pendingKey"].(string)
	cases := []struct {
		sub, vec string
		seq, ts  int64
		op       opArgs
	}{
		{"bank", "cmdBankAddJson", 20, 1763490005000, opArgs{child: 3, min: 30}},
		{"bank-clear", "cmdBankClearJson", 21, 1763490006000, opArgs{child: 3}},
		{"promos", "cmdPromoListJson", 22, 1763490007000, opArgs{}},
		{"promo-create", "cmdPromoCreateJson", 23, 1763490008000, opArgs{code: "SUMMER", min: 30}},
		{"promo-gen", "cmdPromoGenJson", 24, 1763490009000, opArgs{count: 5, length: 6, charset: "mixed", min: 15}},
		{"promo-delete", "cmdPromoDeleteJson", 25, 1763490010000, opArgs{code: "SUMMER"}},
		{"decide", "cmdDecideJson", 26, 1763490011000, opArgs{key: key, cat: "games"}},
		{"forbid-off", "cmdNoForbiddenJson", 19, 1763490004000, opArgs{child: 3}},
		{"cmd", "cmdJson", 17, 1763490000000, opArgs{child: 3, delta: 15}},
	}
	for _, c := range cases {
		cmd, err := opCommand(c.sub, "pc-vector", "phone-vector", c.seq, c.ts, c.op)
		if err != nil {
			t.Fatalf("%s: %v", c.sub, err)
		}
		got, _ := json.Marshal(cmd)
		if want, _ := v[c.vec].(string); want == "" || string(got) != want {
			t.Errorf("%s:\n got %s\nwant %s", c.sub, got, want)
		}
	}
	// -raw-min уходит как есть; -min 601 у bank — числом (предел не проверяется, S61).
	cmd, _ := opCommand("promo-create", "pc", "ph", 1, 1, opArgs{code: "X1Y", min: 30, rawMin: `"30"`})
	if got, _ := json.Marshal(cmd); !strings.HasSuffix(string(got), `"op":"promoCreate","code":"X1Y","min":"30"}`) {
		t.Fatalf("raw-min: %s", got)
	}
	cmd, _ = opCommand("bank", "pc", "ph", 1, 1, opArgs{child: 3, min: 601})
	if got, _ := json.Marshal(cmd); !strings.HasSuffix(string(got), `"child":3,"delta":601,"op":"bankAdd"}`) {
		t.Fatalf("bank 601: %s", got)
	}
	for _, sub := range []string{"bank", "bank-clear", "forge-bank"} {
		if _, err := opCommand(sub, "pc", "ph", 1, 1, opArgs{min: 5}); err == nil {
			t.Errorf("%s без -child принят", sub)
		}
	}
	if _, err := opCommand("decide", "pc", "ph", 1, 1, opArgs{cat: "games"}); err == nil {
		t.Error("decide без -key принят")
	}
}

// Ловит: подкоманды 1.5 не доходят до компьютера, вывод без promos, codes, bankMin и отчёта;
// отказ range тратит номер телефона так, что следующая команда — replay; status без pending.
func TestPhoneBankPromoDecide(t *testing.T) {
	f := newFakePC(t)
	f.bank = 45
	f.pending = []remote.PendingReport{{ID: remote.PendingKey(`C:\g.exe`), Name: "SuperGame", Path: `C:\g.exe`, Kids: []remote.PendingKid{{ID: 3, Tries: 3}}, Tries: 3, LastMs: 5}}
	state := filepath.Join(t.TempDir(), "phone.json")
	if code, out := runJSON(t, "pair", "-state", state, "-url", f.srv.URL, "-code", "123456"); code != 0 {
		t.Fatalf("pair: %v", out)
	}
	runJSON(t, "confirm", "-state", state)

	code, out := runJSON(t, "status", "-state", state)
	pend, _ := out["pending"].([]any)
	ch, _ := out["children"].([]any)
	if code != 0 || len(pend) != 1 || out["pendingCount"] != float64(1) || ch[0].(map[string]any)["bankMin"] != float64(45) {
		t.Fatalf("status: %d %v", code, out)
	}
	key := pend[0].(map[string]any)["id"].(string)

	code, out = runJSON(t, "bank", "-state", state, "-child", "3", "-min", "30")
	if code != 0 || out["message"] != "Положено 30 минут. В копилке 75 мин" || out["bankMin"] != float64(75) || out["report"] == nil {
		t.Fatalf("bank: %d %v", code, out)
	}
	if code, out := runJSON(t, "bank", "-state", state, "-child", "3", "-min", "601"); code != 2 || out["error"] != remote.ErrCodeRange {
		t.Fatalf("bank 601: %d %v", code, out)
	}
	if code, out := runJSON(t, "forge-bank", "-state", state, "-child", "3", "-min", "30"); code != 2 || out["error"] != remote.ErrCodeBadSig {
		t.Fatalf("forge-bank: %d %v", code, out)
	}
	if code, out := runJSON(t, "bank-clear", "-state", state, "-child", "3"); code != 0 || out["message"] != "Копилка обнулена" {
		t.Fatalf("bank-clear: %d %v", code, out)
	}
	if code, out := runJSON(t, "promo-create", "-state", state, "-code", "SUMMER", "-raw-min", `"30"`); code != 2 || out["error"] != remote.ErrCodeRange {
		t.Fatalf("promo-create raw-min: %d %v", code, out)
	}
	code, out = runJSON(t, "promo-create", "-state", state, "-code", " summer ", "-min", "30")
	if p, _ := out["promos"].(map[string]any); code != 0 || p == nil || out["report"] != nil || len(p["active"].([]any)) != 1 {
		t.Fatalf("promo-create: %d %v", code, out)
	}
	code, out = runJSON(t, "promo-gen", "-state", state, "-count", "5", "-len", "6", "-charset", "mixed", "-min", "15")
	if c, _ := out["codes"].([]any); code != 0 || len(c) != 2 {
		t.Fatalf("promo-gen: %d %v", code, out)
	}
	if code, out := runJSON(t, "promos", "-state", state); code != 0 || out["promos"] == nil {
		t.Fatalf("promos: %d %v", code, out)
	}
	if code, out := runJSON(t, "promo-delete", "-state", state, "-code", "NOSUCH"); code != 2 || out["error"] != remote.ErrCodeNotFound || out["promos"] == nil {
		t.Fatalf("promo-delete: %d %v", code, out)
	}
	if code, out := runJSON(t, "decide", "-state", state, "-key", key, "-cat", "games"); code != 0 || out["message"] != "SuperGame: игра" {
		t.Fatalf("decide: %d %v", code, out)
	}
	if code, out := runJSON(t, "decide", "-state", state, "-key", key, "-cat", "games"); code != 2 || out["error"] != remote.ErrCodeNotFound {
		t.Fatalf("decide второй раз: %d %v", code, out)
	}
	f.mu.Lock()
	ops := strings.Join(f.ops, " ")
	f.mu.Unlock()
	want := "bankAdd:30::0:0:0::: bankClear:0::0:0:0::: promoCreate:0:SUMMER:30:0:0::: promoGen:0::15:5:6:mixed:: promoList:0::0:0:0::: " +
		"promoDelete:0:NOSUCH:0:0:0::: decide:0::0:0:0::" + key + ":games decide:0::0:0:0::" + key + ":games"
	if ops != want {
		t.Fatalf("применены:\n%s\nждали\n%s", ops, want)
	}
}
