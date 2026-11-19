// Команда remote-phone — тестовый телефон приёмки S55, S56 (M7, FR-40, docs/07_Протокол_связи.md).
// Делает то же, что Android-приложение, но из командной строки: привязка кодом со сверкой числа,
// команды доп. времени, проверенный отчёт компьютера. Криптография — только функции
// internal/remote (подпись, обязательство, число сверки), своей здесь нет. В установщик не входит.
//
//	remote-phone pair    -state f.json -url http://127.0.0.1:18080 -code 123456 [-name Pixel]
//	remote-phone confirm -state f.json
//	remote-phone cmd     -state f.json -child 3 -delta 15
//	remote-phone status  -state f.json
//	remote-phone forge-cmd -state f.json -child 3 -delta 15   (подпись чужим ключом)
//	remote-phone replay  -state f.json                        (повтор последней команды, тот же seq)
//
// Вывод — одна строка JSON. Код выхода: 0 — удалось, 2 — компьютер ответил отказом (ответ
// проверен подписью), 1 — любая другая ошибка ({"error":"<код>"}).
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"pcblock/internal/remote"
)

// Коды ошибок вывода, кроме кодов сервера (bad_code, pc_offline, too_many…).
const (
	errUsage        = "usage"
	errNoState      = "no_state"
	errNetwork      = "network"
	errBadAnswer    = "bad_answer"
	errBadSig       = "bad_sig_answer" // ответ компьютера не прошёл проверку подписи
	errCommit       = "commit"         // обязательство компьютера не сошлось с раскрытым nPc
	errNotConfirmed = "not_confirmed"
	errNoCmd        = "no_cmd"
)

// phoneState — файл -state: всё, что телефон помнит между вызовами.
type phoneState struct {
	URL        string         `json:"url"`
	PCID       string         `json:"pcId"`
	PhoneID    string         `json:"phoneId"`
	PhoneToken string         `json:"phoneToken"`
	PhoneKey   string         `json:"phoneKey"` // PKCS#8 base64url
	PCPub      string         `json:"pcPub"`
	SAS        string         `json:"sas"`
	Confirmed  bool           `json:"confirmed"`
	Seq        int64          `json:"seq"`
	LastCmd    *remote.Signed `json:"lastCmd,omitempty"`
}

// failure — ошибка с кодом для вывода {"error":…}.
type failure struct {
	code   string
	detail string
}

func (f *failure) Error() string { return f.code + ": " + f.detail }

func fail(code, format string, args ...any) error {
	return &failure{code: code, detail: fmt.Sprintf(format, args...)}
}

// app — зависимости команд (подменяются в тестах).
type app struct {
	out  io.Writer
	http *http.Client
	now  func() time.Time
}

func main() {
	a := &app{
		out:  os.Stdout,
		http: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}},
		now:  time.Now,
	}
	os.Exit(a.run(os.Args[1:]))
}

func (a *app) run(args []string) int {
	if len(args) == 0 {
		return a.emitErr(fail(errUsage, "нужна подкоманда: pair, confirm, cmd, status, forge-cmd, replay"))
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet(sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	statePath := fs.String("state", "remote-phone.json", "файл состояния телефона")
	rawURL := fs.String("url", "", "адрес сервера связи (pair)")
	code := fs.String("code", "", "код привязки (pair)")
	name := fs.String("name", "Тестовый телефон", "имя телефона (pair)")
	child := fs.Int64("child", 0, "id ребёнка (cmd, forge-cmd)")
	delta := fs.Int("delta", 0, "минуты доп. времени, со знаком (cmd, forge-cmd)")
	if err := fs.Parse(rest); err != nil {
		return a.emitErr(fail(errUsage, "%v", err))
	}
	var res any
	var err error
	rejected := false
	switch sub {
	case "pair":
		res, err = a.pair(*statePath, *rawURL, *code, *name)
	case "confirm":
		res, err = a.confirm(*statePath)
	case "cmd", "forge-cmd", "replay":
		var r cmdOut
		r, err = a.command(*statePath, sub, *child, *delta)
		res, rejected = r, err == nil && !r.OK
	case "status":
		res, err = a.status(*statePath)
	default:
		err = fail(errUsage, "неизвестная подкоманда %q", sub)
	}
	if err != nil {
		return a.emitErr(err)
	}
	a.emit(res)
	if rejected {
		return 2
	}
	return 0
}

func (a *app) emit(v any) {
	raw, _ := json.Marshal(v)
	fmt.Fprintln(a.out, string(raw))
}

func (a *app) emitErr(err error) int {
	var f *failure
	if !errors.As(err, &f) {
		f = &failure{code: errNetwork, detail: err.Error()}
	}
	a.emit(map[string]string{"error": f.code, "detail": f.detail})
	return 1
}

// ---------- состояние ----------

func loadState(path string) (*phoneState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fail(errNoState, "%v", err)
	}
	var st phoneState
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fail(errNoState, "%v", err)
	}
	return &st, nil
}

func saveState(path string, st *phoneState) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fail(errNoState, "%v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fail(errNoState, "%v", err)
	}
	return nil
}

func (st *phoneState) key() (*ecdsa.PrivateKey, error) {
	k, err := remote.ParsePriv(st.PhoneKey)
	if err != nil {
		return nil, fail(errNoState, "ключ телефона: %v", err)
	}
	return k, nil
}

func (st *phoneState) pcKey() (*ecdsa.PublicKey, error) {
	k, err := remote.ParsePub(st.PCPub)
	if err != nil {
		return nil, fail(errNoState, "ключ компьютера: %v", err)
	}
	return k, nil
}

// ---------- HTTP ----------

// call — запрос к серверу связи (протокол, раздел 3). Ответ не 200 — failure с кодом сервера.
func (a *app) call(method, base, path, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return fail(errUsage, "%v", err)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fail(errNetwork, "%v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, remote.MaxResponse+1))
	if err != nil {
		return fail(errNetwork, "%v", err)
	}
	if len(raw) > remote.MaxResponse {
		return fail(errBadAnswer, "ответ больше %d байт", remote.MaxResponse)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		return fail(e.Error, "сервер ответил %d", resp.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fail(errBadAnswer, "%v", err)
		}
	}
	return nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func unb64(s string) ([]byte, error) { return base64.RawURLEncoding.Strict().DecodeString(s) }

// ---------- pair, confirm ----------

type pairOut struct {
	SAS     string `json:"sas"`
	PCID    string `json:"pcId"`
	PhoneID string `json:"phoneId"`
}

// pair — привязка (протокол, раздел 4): pair1 → pair1r (ключ компьютера и обязательство) →
// pair2 (nPhone) → pair2r (nPc); проверка обязательства и число сверки. Ключ компьютера
// записан, но не подтверждён до confirm («Совпадает»).
func (a *app) pair(statePath, rawURL, code, name string) (any, error) {
	if rawURL == "" || code == "" {
		return nil, fail(errUsage, "pair: нужны -url и -code")
	}
	key, err := remote.NewKey()
	if err != nil {
		return nil, err
	}
	phonePub := remote.MarshalPub(&key.PublicKey)
	var r1 struct {
		PCID       string     `json:"pcId"`
		PhoneID    string     `json:"phoneId"`
		PhoneToken string     `json:"phoneToken"`
		Reply      remote.Msg `json:"reply"`
	}
	in1 := map[string]any{"code": code, "kind": remote.KindPair1, "body": remote.Pair1{PhonePub: phonePub, Name: name}}
	if err := a.call(http.MethodPost, rawURL, "/v1/phone/pair", "", in1, &r1); err != nil {
		return nil, err
	}
	if r1.PCID == "" || r1.PhoneID == "" || r1.PhoneToken == "" || r1.Reply.Kind != remote.KindPair1R {
		return nil, fail(errBadAnswer, "pair1: ответ без pcId/phoneId/токена или не pair1r (%q)", r1.Reply.Kind)
	}
	var p1r remote.Pair1R
	if err := json.Unmarshal(r1.Reply.Body, &p1r); err != nil {
		return nil, fail(errBadAnswer, "pair1r: %v", err)
	}
	pcPubDER, err1 := unb64(p1r.PCPub)
	commit, err2 := unb64(p1r.Commit)
	if err1 != nil || err2 != nil {
		return nil, fail(errBadAnswer, "pair1r: неверный base64")
	}
	if _, err := remote.ParsePub(p1r.PCPub); err != nil {
		return nil, fail(errBadAnswer, "pair1r: ключ компьютера: %v", err)
	}
	nPhone := make([]byte, remote.NonceLen)
	if _, err := rand.Read(nPhone); err != nil {
		return nil, err
	}
	var r2 struct {
		Reply remote.Msg `json:"reply"`
	}
	in2 := map[string]any{"kind": remote.KindPair2, "body": map[string]string{"nPhone": b64(nPhone)}}
	if err := a.call(http.MethodPost, rawURL, "/v1/phone/send", r1.PhoneToken, in2, &r2); err != nil {
		return nil, err
	}
	if r2.Reply.Kind != remote.KindPair2R {
		return nil, fail(errBadAnswer, "pair2: ответ не pair2r (%q)", r2.Reply.Kind)
	}
	var p2r remote.Pair2R
	if err := json.Unmarshal(r2.Reply.Body, &p2r); err != nil {
		return nil, fail(errBadAnswer, "pair2r: %v", err)
	}
	nPc, err := unb64(p2r.NPc)
	if err != nil || len(nPc) != remote.NonceLen {
		return nil, fail(errBadAnswer, "pair2r: неверный nPc")
	}
	phonePubDER, _ := unb64(phonePub)
	if !bytes.Equal(remote.Commit(pcPubDER, phonePubDER, nPc), commit) {
		return nil, fail(errCommit, "обязательство компьютера не сошлось")
	}
	sas := remote.SAS(pcPubDER, phonePubDER, nPc, nPhone)
	priv, err := remote.MarshalPriv(key)
	if err != nil {
		return nil, err
	}
	st := &phoneState{URL: strings.TrimRight(rawURL, "/"), PCID: r1.PCID, PhoneID: r1.PhoneID, PhoneToken: r1.PhoneToken,
		PhoneKey: priv, PCPub: p1r.PCPub, SAS: sas}
	if err := saveState(statePath, st); err != nil {
		return nil, err
	}
	return pairOut{SAS: sas, PCID: r1.PCID, PhoneID: r1.PhoneID}, nil
}

// confirm — «Совпадает» на телефоне: ключ компьютера принят.
func (a *app) confirm(statePath string) (any, error) {
	st, err := loadState(statePath)
	if err != nil {
		return nil, err
	}
	if st.PCPub == "" {
		return nil, fail(errNoState, "привязки нет")
	}
	st.Confirmed = true
	if err := saveState(statePath, st); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "sas": st.SAS}, nil
}

// ---------- cmd, forge-cmd, replay ----------

type cmdOut struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	LeftSec *int64 `json:"leftSec,omitempty"`
	Ms      int64  `json:"ms"`
	Seq     int64  `json:"seq"`
	LastSeq *int64 `json:"lastSeq,omitempty"`
}

// command — cmd: новая команда со следующим seq, подписанная ключом телефона; forge-cmd — та же
// команда, но подписанная только что созданным чужим ключом (seq не тратится); replay — последняя
// отправленная команда байт в байт (тот же seq). Ответ cmdr принимается, только если подпись
// компьютера верна и cmd — хэш нашей команды.
func (a *app) command(statePath, sub string, child int64, delta int) (cmdOut, error) {
	st, err := loadState(statePath)
	if err != nil {
		return cmdOut{}, err
	}
	if !st.Confirmed {
		return cmdOut{}, fail(errNotConfirmed, "сначала confirm")
	}
	var signed remote.Signed
	switch sub {
	case "replay":
		if st.LastCmd == nil {
			return cmdOut{}, fail(errNoCmd, "команд ещё не было")
		}
		signed = *st.LastCmd
	default:
		if child <= 0 || delta == 0 {
			return cmdOut{}, fail(errUsage, "%s: нужны -child и -delta", sub)
		}
		c := remote.Command{V: remote.ProtoVersion, T: remote.TypeCmd, PC: st.PCID, Phone: st.PhoneID,
			Seq: st.Seq + 1, TS: a.now().UnixMilli(), Child: child, Delta: delta}
		data, err := json.Marshal(c)
		if err != nil {
			return cmdOut{}, err
		}
		key, err := st.key()
		if err != nil {
			return cmdOut{}, err
		}
		if sub == "forge-cmd" {
			if key, err = remote.NewKey(); err != nil {
				return cmdOut{}, err
			}
		}
		if signed, err = remote.Sign(key, data); err != nil {
			return cmdOut{}, err
		}
		if sub == "cmd" {
			// seq записан до отправки: повтор (replay) берёт ровно эту команду.
			st.Seq, st.LastCmd = c.Seq, &signed
			if err := saveState(statePath, st); err != nil {
				return cmdOut{}, err
			}
		}
	}
	data, err := unb64(signed.Data)
	if err != nil {
		return cmdOut{}, fail(errNoState, "команда в состоянии: %v", err)
	}
	var sent remote.Command
	_ = json.Unmarshal(data, &sent)

	var resp struct {
		Reply remote.Msg `json:"reply"`
	}
	start := time.Now()
	err = a.call(http.MethodPost, st.URL, "/v1/phone/send", st.PhoneToken, map[string]any{"kind": remote.KindCmd, "body": signed}, &resp)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return cmdOut{}, err
	}
	if resp.Reply.Kind != remote.KindCmdR {
		return cmdOut{}, fail(errBadAnswer, "ответ не cmdr (%q)", resp.Reply.Kind)
	}
	var rs remote.Signed
	if err := json.Unmarshal(resp.Reply.Body, &rs); err != nil {
		return cmdOut{}, fail(errBadAnswer, "cmdr: %v", err)
	}
	pcKey, err := st.pcKey()
	if err != nil {
		return cmdOut{}, err
	}
	rdata, err := remote.Verify(pcKey, rs)
	if err != nil {
		return cmdOut{}, fail(errBadSig, "cmdr: %v", err)
	}
	var res remote.CmdResult
	if err := json.Unmarshal(rdata, &res); err != nil {
		return cmdOut{}, fail(errBadAnswer, "cmdr: %v", err)
	}
	if res.T != remote.TypeCmdR || res.PC != st.PCID || res.Phone != st.PhoneID || res.Cmd != remote.HashData(data) {
		return cmdOut{}, fail(errBadSig, "cmdr не на нашу команду (t=%q, cmd совпал: %v)", res.T, res.Cmd == remote.HashData(data))
	}
	out := cmdOut{OK: res.OK, Message: res.Message, Error: res.Error, Ms: ms, Seq: sent.Seq, LastSeq: res.LastSeq}
	if res.Report != nil {
		for _, ch := range res.Report.Children {
			if ch.ID == sent.Child {
				left := ch.LeftSec
				out.LeftSec = &left
			}
		}
	}
	if !res.OK && res.LastSeq != nil && *res.LastSeq > st.Seq && sub == "cmd" {
		// Компьютер помнит больший номер (например, после перезапуска телефона): догоняем.
		st.Seq = *res.LastSeq
		_ = saveState(statePath, st)
	}
	return out, nil
}

// ---------- status ----------

type statusOut struct {
	Online       bool                 `json:"online"`
	PCLastSeenMs int64                `json:"pcLastSeenMs"`
	ServerNowMs  int64                `json:"serverNowMs"`
	Report       bool                 `json:"report"`
	ReportTS     int64                `json:"reportTs"`
	Children     []remote.ChildReport `json:"children"`
}

// status — GET /v1/phone/status: доступность по pcLastSeenMs и отчёт, проверенный подписью
// компьютера (протокол, раздел 6). Отчёт без верной подписи — ошибка, не показывается.
func (a *app) status(statePath string) (any, error) {
	st, err := loadState(statePath)
	if err != nil {
		return nil, err
	}
	if !st.Confirmed {
		return nil, fail(errNotConfirmed, "сначала confirm")
	}
	var resp struct {
		PCLastSeenMs int64           `json:"pcLastSeenMs"`
		ServerNowMs  int64           `json:"serverNowMs"`
		Report       json.RawMessage `json:"report"`
	}
	if err := a.call(http.MethodGet, st.URL, "/v1/phone/status", st.PhoneToken, nil, &resp); err != nil {
		return nil, err
	}
	out := statusOut{
		Online:       resp.PCLastSeenMs > 0 && resp.ServerNowMs-resp.PCLastSeenMs <= 60_000,
		PCLastSeenMs: resp.PCLastSeenMs, ServerNowMs: resp.ServerNowMs, Children: []remote.ChildReport{},
	}
	if len(resp.Report) == 0 || string(resp.Report) == "null" {
		return out, nil
	}
	var s remote.Signed
	if err := json.Unmarshal(resp.Report, &s); err != nil {
		return nil, fail(errBadAnswer, "report: %v", err)
	}
	pcKey, err := st.pcKey()
	if err != nil {
		return nil, err
	}
	data, err := remote.Verify(pcKey, s)
	if err != nil {
		return nil, fail(errBadSig, "report: %v", err)
	}
	var rep remote.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fail(errBadAnswer, "report: %v", err)
	}
	if rep.T != remote.TypeReport || rep.PC != st.PCID || rep.Phone != st.PhoneID {
		return nil, fail(errBadSig, "отчёт не для этого телефона (t=%q)", rep.T)
	}
	out.Report, out.ReportTS = true, rep.TS
	if rep.Children != nil {
		out.Children = rep.Children
	}
	return out, nil
}
