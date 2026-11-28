// Команда remote-phone — тестовый телефон приёмки S55…S59 (M7, FR-40, docs/07_Протокол_связи.md).
// Делает то же, что Android-приложение, но из командной строки: привязка кодом со сверкой числа,
// команды доп. времени, проверенный отчёт компьютера. Криптография — только функции
// internal/remote (подпись, обязательство, число сверки), своей здесь нет. В установщик не входит.
//
//	remote-phone pair    -state f.json -url http://127.0.0.1:18080 -code 123456 [-name Pixel]
//	remote-phone confirm -state f.json
//	remote-phone cmd     -state f.json -child 3 -delta 15
//	remote-phone status  -state f.json [-human]          (отчёт; -human — для человека, со статистикой за сегодня)
//	remote-phone today   -state f.json -child 3 [-human] (статистика за сегодня одного ребёнка, FR-43)
//	remote-phone forge-cmd -state f.json -child 3 -delta 15   (подпись чужим ключом)
//	remote-phone replay  -state f.json                        (повтор последней команды, тот же seq)
//	remote-phone note    -state f.json -child 3 -text "Иди ужинать" -show 0 [-sticker 033] [-sound 2]
//	                                                          (сообщение, FR-41; без -sticker и -sound
//	                                                          полей нет, как у телефона 0.8.2)
//	remote-phone forge-note -state f.json -child 3 -text "…" -show 0       (подпись чужим ключом)
//	remote-phone notes   -state f.json                        (состояния сообщений из отчёта)
//	remote-phone forbid-off -state f.json -child 3            (снять запрет на сегодня, FR-42)
//	remote-phone forge-forbid-off -state f.json -child 3      (подпись чужим ключом)
//	remote-phone bank    -state f.json -child 3 -min 30       (положить в копилку, FR-44; предел не
//	                                                          проверяется: -min 601 уходит числом 601)
//	remote-phone bank-clear -state f.json -child 3            (обнулить копилку)
//	remote-phone forge-bank -state f.json -child 3 -min 30    (подпись чужим ключом)
//	remote-phone promos  -state f.json                        (список промокодов, FR-45)
//	remote-phone promo-create -state f.json -code SUMMER -min 30 [-raw-min '"30"']
//	                                                          (-raw-min уходит в JSON как есть)
//	remote-phone promo-gen -state f.json -count 5 -len 6 -charset mixed -min 15
//	remote-phone promo-delete -state f.json -code SUMMER
//	remote-phone decide  -state f.json -key <id из pending> -cat games   (решение о программе, FR-47)
//
// Вывод — одна строка JSON (с -human у status и today — строки для человека). Код выхода: 0 —
// удалось, 2 — компьютер ответил отказом (ответ проверен подписью), 1 — любая другая ошибка
// ({"error":"<код>"}).
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
	"strconv"
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
	errNoChild      = "no_child" // ребёнка нет в отчёте (today)
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
		return a.emitErr(fail(errUsage, "нужна подкоманда: pair, confirm, cmd, status, today, forge-cmd, replay, note, forge-note, notes, forbid-off, forge-forbid-off, "+
			"bank, bank-clear, forge-bank, promos, promo-create, promo-gen, promo-delete, decide"))
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet(sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	statePath := fs.String("state", "remote-phone.json", "файл состояния телефона")
	rawURL := fs.String("url", "", "адрес сервера связи (pair)")
	code := fs.String("code", "", "код привязки (pair) или промокод (promo-create, promo-delete)")
	name := fs.String("name", "Тестовый телефон", "имя телефона (pair)")
	child := fs.Int64("child", 0, "id ребёнка (cmd, forge-cmd)")
	delta := fs.Int("delta", 0, "минуты доп. времени, со знаком (cmd, forge-cmd)")
	text := fs.String("text", "", "текст сообщения (note, forge-note)")
	show := fs.Int("show", 0, "сколько показывать, с: 0 (до «Понял»), 10, 30, 60 (note, forge-note)")
	sticker := fs.String("sticker", "", "номер стикера из 3 цифр, уходит строкой как есть; пусто: поля нет (note, forge-note)")
	sound := fs.String("sound", "", "номер звука, целое, уходит числом как есть (0: без звука); пусто: поля нет (note, forge-note)")
	human := fs.Bool("human", false, "вывод для человека вместо JSON (status, today)")
	var op opArgs
	fs.IntVar(&op.min, "min", 0, "минуты (bank, forge-bank, promo-create, promo-gen)")
	fs.StringVar(&op.rawMin, "raw-min", "", "поле min как есть, JSON (promo-create; проверка range)")
	fs.IntVar(&op.count, "count", 0, "сколько кодов (promo-gen)")
	fs.IntVar(&op.length, "len", 0, "длина кода (promo-gen)")
	fs.StringVar(&op.charset, "charset", "", "состав: digits, letters, mixed (promo-gen)")
	fs.StringVar(&op.key, "key", "", "id строки pending (decide)")
	fs.StringVar(&op.cat, "cat", "", "категория: games, study, other, blocked (decide)")
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
	case "cmd", "forge-cmd", "replay", "forbid-off", "forge-forbid-off",
		"bank", "bank-clear", "forge-bank", "promos", "promo-create", "promo-gen", "promo-delete", "decide":
		var r cmdOut
		op.child, op.delta, op.code = *child, *delta, *code
		r, err = a.command(*statePath, sub, op)
		res, rejected = r, err == nil && !r.OK
	case "status":
		var r statusOut
		r, err = a.status(*statePath)
		if err == nil && *human {
			a.printText(r, 0)
			return 0
		}
		res = r
	case "today":
		var r statusOut
		r, err = a.status(*statePath)
		if err == nil && *child == 0 {
			err = fail(errUsage, "нужен -child")
		}
		if err == nil {
			var c *remote.ChildReport
			for i := range r.Children {
				if r.Children[i].ID == *child {
					c = &r.Children[i]
				}
			}
			switch {
			case c == nil:
				err = fail(errNoChild, "ребёнка %d нет в отчёте", *child)
			case *human:
				a.printText(r, *child)
				return 0
			default:
				res = map[string]any{"reportTs": r.ReportTS, "child": c.ID, "name": c.Name, "today": c.Today}
			}
		}
	case "note", "forge-note":
		var r noteOut
		r, err = a.note(*statePath, sub, *child, *text, *show, *sticker, *sound)
		res, rejected = r, err == nil && !r.OK
	case "notes":
		var r statusOut
		r, err = a.status(*statePath)
		res = map[string]any{"notes": r.Notes, "reportTs": r.ReportTS, "report": r.Report}
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
	// Forbid, ForbidUntilMs — запретные часы ребёнка из отчёта в ответе (раздел 6).
	Forbid        string `json:"forbid,omitempty"`
	ForbidUntilMs int64  `json:"forbidUntilMs,omitempty"`
	// BankMin — копилка ребёнка команды из отчёта в ответе (раздел 5в).
	BankMin *int `json:"bankMin,omitempty"`
	// Promos, Codes — список промокодов и новые коды из ответа promo* (раздел 5г); Report — отчёт
	// в ответе целиком (pending, bankMin всех детей).
	Promos *remote.PromoList `json:"promos,omitempty"`
	Codes  []string          `json:"codes,omitempty"`
	Report *remote.Report    `json:"report,omitempty"`
}

// opArgs — поля команды из флагов: child и delta (cmd, forbid-off) и поля разделов 5в–5д.
type opArgs struct {
	child             int64
	delta, min        int
	rawMin, code      string
	count, length     int
	charset, key, cat string
}

// opCommand — данные команды подкоманды sub (без подписи): seq и ts — следующего номера и сейчас.
// Поля 5г, 5д пишутся сырыми в порядке структуры remote.Command (как у Android и векторов).
// Пределы не проверяются: неверные значения уходят как есть (приёмка проверяет range).
func opCommand(sub, pc, phone string, seq, ts int64, op opArgs) (remote.Command, error) {
	c := remote.Command{V: remote.ProtoVersion, T: remote.TypeCmd, PC: pc, Phone: phone, Seq: seq, TS: ts, Child: op.child}
	str := func(v string) json.RawMessage { raw, _ := json.Marshal(v); return raw }
	num := func(n int) json.RawMessage { return json.RawMessage(strconv.Itoa(n)) }
	switch sub {
	case "cmd", "forge-cmd":
		if op.child <= 0 || op.delta == 0 {
			return c, fail(errUsage, "%s: нужны -child и -delta", sub)
		}
		c.Delta = op.delta
	case "forbid-off", "forge-forbid-off":
		if op.child <= 0 {
			return c, fail(errUsage, "%s: нужен -child", sub)
		}
		c.Op = remote.OpNoForbidden
	case "bank", "forge-bank":
		if op.child <= 0 {
			return c, fail(errUsage, "%s: нужен -child", sub)
		}
		c.Op, c.Delta = remote.OpBankAdd, op.min
	case "bank-clear":
		if op.child <= 0 {
			return c, fail(errUsage, "%s: нужен -child", sub)
		}
		c.Op = remote.OpBankClear
	case "promos":
		c.Child, c.Op = 0, remote.OpPromoList
	case "promo-create":
		c.Child, c.Op, c.CodeRaw, c.MinRaw = 0, remote.OpPromoCreate, str(op.code), num(op.min)
		if op.rawMin != "" {
			c.MinRaw = json.RawMessage(op.rawMin)
		}
	case "promo-gen":
		c.Child, c.Op = 0, remote.OpPromoGen
		c.MinRaw, c.CountRaw, c.LenRaw, c.CharsetRaw = num(op.min), num(op.count), num(op.length), str(op.charset)
	case "promo-delete":
		c.Child, c.Op, c.CodeRaw = 0, remote.OpPromoDelete, str(op.code)
	case "decide":
		if op.key == "" || op.cat == "" {
			return c, fail(errUsage, "decide: нужны -key и -cat")
		}
		c.Child, c.Op, c.KeyRaw, c.CatRaw = 0, remote.OpDecide, str(op.key), str(op.cat)
	default:
		return c, fail(errUsage, "неизвестная подкоманда %q", sub)
	}
	return c, nil
}

// command — cmd: новая команда со следующим seq, подписанная ключом телефона; forge-cmd — та же
// команда, но подписанная только что созданным чужим ключом (seq не тратится); replay — последняя
// отправленная команда байт в байт (тот же seq); forbid-off — «снять запрет на сегодня» (op
// noForbidden, без delta, протокол 5б), forge-forbid-off — она же чужим ключом; bank, bank-clear,
// forge-bank — копилка (5в); promos, promo-create, promo-gen, promo-delete — промокоды (5г);
// decide — решение о программе (5д). Ответ cmdr принимается, только если подпись компьютера верна
// и cmd — хэш нашей команды.
func (a *app) command(statePath, sub string, op opArgs) (cmdOut, error) {
	st, err := loadState(statePath)
	if err != nil {
		return cmdOut{}, err
	}
	if !st.Confirmed {
		return cmdOut{}, fail(errNotConfirmed, "сначала confirm")
	}
	forged := strings.HasPrefix(sub, "forge-")
	var signed remote.Signed
	switch sub {
	case "replay":
		if st.LastCmd == nil {
			return cmdOut{}, fail(errNoCmd, "команд ещё не было")
		}
		signed = *st.LastCmd
	default:
		c, err := opCommand(sub, st.PCID, st.PhoneID, st.Seq+1, a.now().UnixMilli(), op)
		if err != nil {
			return cmdOut{}, err
		}
		data, err := json.Marshal(c)
		if err != nil {
			return cmdOut{}, err
		}
		key, err := st.key()
		if err != nil {
			return cmdOut{}, err
		}
		if forged {
			if key, err = remote.NewKey(); err != nil {
				return cmdOut{}, err
			}
		}
		if signed, err = remote.Sign(key, data); err != nil {
			return cmdOut{}, err
		}
		if !forged {
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
	out := cmdOut{OK: res.OK, Message: res.Message, Error: res.Error, Ms: ms, Seq: sent.Seq, LastSeq: res.LastSeq,
		Promos: res.Promos, Codes: res.Codes, Report: res.Report}
	if res.Report != nil {
		for _, ch := range res.Report.Children {
			if ch.ID == sent.Child {
				left, bank := ch.LeftSec, ch.BankMin
				out.LeftSec, out.BankMin = &left, &bank
				out.Forbid, out.ForbidUntilMs = ch.Forbid, ch.ForbidUntilMs
			}
		}
	}
	if !res.OK && res.LastSeq != nil && *res.LastSeq > st.Seq && sub != "replay" && !forged {
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
	Notes        []remote.NoteReport  `json:"notes"`
	// Pending, PendingCount — ждущие решения программы (раздел 6); у детей — bankMin.
	Pending      []remote.PendingReport `json:"pending"`
	PendingCount int                    `json:"pendingCount"`
}

// status — GET /v1/phone/status: доступность по pcLastSeenMs и отчёт, проверенный подписью
// компьютера (протокол, раздел 6). Отчёт без верной подписи — ошибка, не показывается.
func (a *app) status(statePath string) (statusOut, error) {
	st, err := loadState(statePath)
	if err != nil {
		return statusOut{}, err
	}
	if !st.Confirmed {
		return statusOut{}, fail(errNotConfirmed, "сначала confirm")
	}
	var resp struct {
		PCLastSeenMs int64           `json:"pcLastSeenMs"`
		ServerNowMs  int64           `json:"serverNowMs"`
		Report       json.RawMessage `json:"report"`
	}
	if err := a.call(http.MethodGet, st.URL, "/v1/phone/status", st.PhoneToken, nil, &resp); err != nil {
		return statusOut{}, err
	}
	out := statusOut{
		Online:       resp.PCLastSeenMs > 0 && resp.ServerNowMs-resp.PCLastSeenMs <= 60_000,
		PCLastSeenMs: resp.PCLastSeenMs, ServerNowMs: resp.ServerNowMs, Children: []remote.ChildReport{}, Notes: []remote.NoteReport{},
		Pending: []remote.PendingReport{},
	}
	if len(resp.Report) == 0 || string(resp.Report) == "null" {
		return out, nil
	}
	var s remote.Signed
	if err := json.Unmarshal(resp.Report, &s); err != nil {
		return statusOut{}, fail(errBadAnswer, "report: %v", err)
	}
	pcKey, err := st.pcKey()
	if err != nil {
		return statusOut{}, err
	}
	data, err := remote.Verify(pcKey, s)
	if err != nil {
		return statusOut{}, fail(errBadSig, "report: %v", err)
	}
	var rep remote.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return statusOut{}, fail(errBadAnswer, "report: %v", err)
	}
	if rep.T != remote.TypeReport || rep.PC != st.PCID || rep.Phone != st.PhoneID {
		return statusOut{}, fail(errBadSig, "отчёт не для этого телефона (t=%q)", rep.T)
	}
	out.Report, out.ReportTS = true, rep.TS
	if rep.Children != nil {
		out.Children = rep.Children
	}
	if rep.Notes != nil {
		out.Notes = rep.Notes
	}
	if rep.Pending != nil {
		out.Pending = rep.Pending
	}
	out.PendingCount = rep.PendingCount
	return out, nil
}

// printText — отчёт для человека: строка ребёнка и «Статистика за сегодня» (FR-43), время — по
// местным часам этого компьютера. only > 0 — только этот ребёнок.
func (a *app) printText(r statusOut, only int64) {
	hm := func(ms int64) string { return time.UnixMilli(ms).Format("15:04") }
	online := "Компьютер не на связи"
	if r.Online {
		online = "На связи"
	}
	if !r.Report {
		fmt.Fprintf(a.out, "%s, отчёта нет\n", online)
		return
	}
	fmt.Fprintf(a.out, "%s, данные на %s\n", online, hm(r.ReportTS))
	for _, c := range r.Children {
		if only > 0 && c.ID != only {
			continue
		}
		left := "без лимита"
		if c.LeftSec >= 0 {
			left = "осталось " + dur(c.LeftSec)
		}
		fmt.Fprintf(a.out, "%s (id %d): использовано %s, %s, доп. время %d мин\n", c.Name, c.ID, dur(c.UsedSec), left, c.ExtraMin)
		t := c.Today
		if t == nil {
			fmt.Fprintln(a.out, "  статистики за сегодня нет (компьютер старой версии)")
			continue
		}
		at := "Не за компьютером с " + hm(t.SinceMs)
		if t.NowAt {
			at = "За компьютером с " + hm(t.SinceMs)
		}
		first := "сегодня не садился"
		if t.FirstMs > 0 {
			first = "первый вход " + hm(t.FirstMs)
		}
		fmt.Fprintf(a.out, "  %s, %s\n", at, first)
		fmt.Fprintf(a.out, "  всего %s, игры %s, учёба %s, видео и соцсети %s\n", dur(t.UsedSec), dur(t.GamesSec), dur(t.StudySec), dur(t.VideoSec))
		for _, l := range []struct {
			title string
			items []remote.TimeItem
			more  int
		}{{"программы", t.Programs, t.More.Programs}, {"сайты", t.Sites, t.More.Sites}} {
			parts := []string{}
			for _, it := range l.items {
				parts = append(parts, fmt.Sprintf("%s (%s) %s", it.Name, it.Cat, dur(it.Sec)))
			}
			if l.more > 0 {
				parts = append(parts, fmt.Sprintf("и ещё %d", l.more))
			}
			if len(parts) == 0 {
				parts = append(parts, "нет")
			}
			fmt.Fprintf(a.out, "  %s: %s\n", l.title, strings.Join(parts, ", "))
		}
		fmt.Fprintf(a.out, "  нарушений %d, из них попыток обхода %d\n", t.Violations, t.Bypass)
	}
}

// dur — «1 ч 05 мин», «7 мин», «0 мин» (минуты вниз).
func dur(sec int64) string {
	m := sec / 60
	if m >= 60 {
		return fmt.Sprintf("%d ч %02d мин", m/60, m%60)
	}
	return fmt.Sprintf("%d мин", m)
}

// ---------- note, forge-note ----------

type noteOut struct {
	OK      bool   `json:"ok"`
	ID      string `json:"id"`
	Error   string `json:"error,omitempty"`
	Ms      int64  `json:"ms"`
	Seq     int64  `json:"seq"`
	LastSeq *int64 `json:"lastSeq,omitempty"`
}

// note — сообщение ребёнку (протокол, раздел 5а): следующий seq (общий с командами), случайный id
// из 16 байтов, подпись ключом телефона; forge-note — подпись чужим ключом (seq не тратится). Ответ
// noter принимается, только если подпись компьютера верна и cmd — хэш нашего сообщения.
// sticker и sound уходят как есть (неверные — для проверки отказа range): sticker — JSON-строкой,
// sound — JSON-числом (не целое — ошибка вызова); пустые — поля нет.
func (a *app) note(statePath, sub string, child int64, text string, show int, sticker, sound string) (noteOut, error) {
	st, err := loadState(statePath)
	if err != nil {
		return noteOut{}, err
	}
	if !st.Confirmed {
		return noteOut{}, fail(errNotConfirmed, "сначала confirm")
	}
	if child <= 0 || text == "" {
		return noteOut{}, fail(errUsage, "%s: нужны -child и -text", sub)
	}
	idRaw := make([]byte, 16)
	if _, err := rand.Read(idRaw); err != nil {
		return noteOut{}, err
	}
	n := remote.Note{V: remote.ProtoVersion, T: remote.TypeNote, PC: st.PCID, Phone: st.PhoneID, Seq: st.Seq + 1,
		TS: a.now().UnixMilli(), Child: child, ID: b64(idRaw), Text: text, ShowSec: show}
	if sticker != "" {
		if n.StickerRaw, err = json.Marshal(sticker); err != nil {
			return noteOut{}, err
		}
	}
	if sound != "" {
		v, err := strconv.Atoi(sound)
		if err != nil {
			return noteOut{}, fail(errUsage, "%s: -sound должен быть целым числом", sub)
		}
		n.SoundRaw = json.RawMessage(strconv.Itoa(v))
	}
	data, err := json.Marshal(n)
	if err != nil {
		return noteOut{}, err
	}
	key, err := st.key()
	if err != nil {
		return noteOut{}, err
	}
	if sub == "forge-note" {
		if key, err = remote.NewKey(); err != nil {
			return noteOut{}, err
		}
	} else {
		st.Seq = n.Seq // номер записан до отправки, как у команды
		if err := saveState(statePath, st); err != nil {
			return noteOut{}, err
		}
	}
	signed, err := remote.Sign(key, data)
	if err != nil {
		return noteOut{}, err
	}
	var resp struct {
		Reply remote.Msg `json:"reply"`
	}
	start := time.Now()
	err = a.call(http.MethodPost, st.URL, "/v1/phone/send", st.PhoneToken, map[string]any{"kind": remote.KindNote, "body": signed}, &resp)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return noteOut{}, err
	}
	if resp.Reply.Kind != remote.KindNoteR {
		return noteOut{}, fail(errBadAnswer, "ответ не noter (%q)", resp.Reply.Kind)
	}
	var rs remote.Signed
	if err := json.Unmarshal(resp.Reply.Body, &rs); err != nil {
		return noteOut{}, fail(errBadAnswer, "noter: %v", err)
	}
	pcKey, err := st.pcKey()
	if err != nil {
		return noteOut{}, err
	}
	rdata, err := remote.Verify(pcKey, rs)
	if err != nil {
		return noteOut{}, fail(errBadSig, "noter: %v", err)
	}
	var res remote.CmdResult
	if err := json.Unmarshal(rdata, &res); err != nil {
		return noteOut{}, fail(errBadAnswer, "noter: %v", err)
	}
	if res.T != remote.TypeNoteR || res.PC != st.PCID || res.Phone != st.PhoneID || res.Cmd != remote.HashData(data) {
		return noteOut{}, fail(errBadSig, "noter не на наше сообщение (t=%q)", res.T)
	}
	out := noteOut{OK: res.OK, ID: n.ID, Error: res.Error, Ms: ms, Seq: n.Seq, LastSeq: res.LastSeq}
	if !res.OK && res.LastSeq != nil && *res.LastSeq > st.Seq && sub == "note" {
		st.Seq = *res.LastSeq
		_ = saveState(statePath, st)
	}
	return out, nil
}
