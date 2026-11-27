package remote

import (
	"crypto/ecdsa"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParsePoll(t *testing.T) {
	msgs, now, err := ParsePoll([]byte(`{"messages":[{"id":"m1","from":"p1","kind":"pair2","body":{"nPhone":"x"}}],"serverNowMs":42,"extra":1}`))
	if err != nil || now != 42 || len(msgs) != 1 || msgs[0].ID != "m1" || msgs[0].From != "p1" || msgs[0].Kind != "pair2" {
		t.Fatalf("разбор опроса: %+v %d %v", msgs, now, err)
	}
	if msgs, _, err := ParsePoll([]byte(`{"messages":null}`)); err != nil || len(msgs) != 0 {
		t.Fatalf("пустой опрос: %v %v", msgs, err)
	}
	for _, bad := range []string{``, `[]`, `{"messages":[1]}`, `{"messages":{}}`, `{`, `null`} {
		if _, _, err := ParsePoll([]byte(bad)); err == nil {
			t.Errorf("%q принят", bad)
		}
	}
}

func TestParseMsg(t *testing.T) {
	k := mustKey(t)
	pub := MarshalPub(&k.PublicKey)
	n16 := b64(make([]byte, 16))
	ok := []Msg{
		{ID: "1", From: "p", Kind: KindPair1, Body: json.RawMessage(`{"phonePub":"` + pub + `","name":"Pixel 7"}`)},
		{ID: "2", From: "p", Kind: KindPair2, Body: json.RawMessage(`{"nPhone":"` + n16 + `"}`)},
		{ID: "3", From: "p", Kind: KindCmd, Body: json.RawMessage(`{"data":"e30","sig":"AA"}`)},
	}
	for _, m := range ok {
		v, err := ParseMsg(m)
		if err != nil {
			t.Fatalf("%s: %v", m.Kind, err)
		}
		switch m.Kind {
		case KindPair1:
			p := v.(*Pair1)
			if p.Name != "Pixel 7" || p.key == nil {
				t.Fatalf("pair1: %+v", p)
			}
		case KindPair2:
			if len(v.(*Pair2).nonce) != 16 {
				t.Fatal("pair2 без nPhone")
			}
		case KindCmd:
			if v.(*Signed).Data != "e30" {
				t.Fatal("cmd")
			}
		}
	}
	bad := []Msg{
		{ID: "", From: "p", Kind: KindCmd, Body: ok[2].Body},
		{ID: "1", From: "", Kind: KindCmd, Body: ok[2].Body},
		{ID: "1", From: "p", Kind: "zzz", Body: json.RawMessage(`{}`)},
		{ID: "1", From: "p", Kind: KindPair1, Body: json.RawMessage(`{"phonePub":"AAAA","name":"x"}`)},
		{ID: "1", From: "p", Kind: KindPair1, Body: json.RawMessage(`[]`)},
		{ID: "1", From: "p", Kind: KindPair2, Body: json.RawMessage(`{"nPhone":"` + b64(make([]byte, 15)) + `"}`)},
		{ID: "1", From: "p", Kind: KindPair2, Body: json.RawMessage(`{"nPhone":"` + n16 + `="}`)},
		{ID: "1", From: "p", Kind: KindCmd, Body: json.RawMessage(`{"data":"","sig":"AA"}`)},
		{ID: "1", From: "p", Kind: KindCmd, Body: json.RawMessage(`"x"`)},
	}
	for i, m := range bad {
		if _, err := ParseMsg(m); err == nil {
			t.Errorf("плохое сообщение %d (%s) принято", i, m.Kind)
		}
	}
}

func TestCleanName(t *testing.T) {
	if got := cleanName("  Pixel\x00 7\n "); got != "Pixel 7" {
		t.Fatalf("%q", got)
	}
	long := strings.Repeat("я", 100)
	if got := cleanName(long); len([]rune(got)) != maxNameRunes {
		t.Fatalf("длина %d", len([]rune(got)))
	}
}

// Таблица отказов команды (протокол, раздел 5) на уровне проверки.
func TestCheckCommandTable(t *testing.T) {
	phone := mustKey(t)
	stranger := mustKey(t)
	now := time.UnixMilli(1763490000000)
	st := State{PCID: "pc1", PCToken: "tok", Phone: &Phone{ID: "ph1", Name: "Pixel", Pub: MarshalPub(&phone.PublicKey)}, LastSeq: 16}
	base := Command{V: 1, T: TypeCmd, PC: "pc1", Phone: "ph1", Seq: 17, TS: now.UnixMilli(), Child: 3, Delta: 15}
	sign := func(k *ecdsa.PrivateKey, c Command) Signed {
		raw, _ := json.Marshal(c)
		s, err := Sign(k, raw)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	with := func(f func(*Command)) Command { c := base; f(&c); return c }
	corrupt := func(s Signed) Signed {
		sig, _ := unb64(s.Sig)
		sig[len(sig)-1] ^= 1
		s.Sig = b64(sig)
		return s
	}
	unpaired := st
	unpaired.Phone = nil
	cases := []struct {
		name string
		st   State
		s    Signed
		from string
		code string
	}{
		{"верная", st, sign(phone, base), "ph1", ""},
		{"чужой ключ", st, sign(stranger, base), "ph1", ErrCodeBadSig},
		{"испорченная подпись", st, corrupt(sign(phone, base)), "ph1", ErrCodeBadSig},
		{"чужой тип", st, sign(phone, with(func(c *Command) { c.T = TypeReport })), "ph1", ErrCodeBadSig},
		{"версия 2", st, sign(phone, with(func(c *Command) { c.V = 2 })), "ph1", ErrCodeBadSig},
		{"старый seq", st, sign(phone, with(func(c *Command) { c.Seq = 15 })), "ph1", ErrCodeReplay},
		{"равный seq", st, sign(phone, with(func(c *Command) { c.Seq = 16 })), "ph1", ErrCodeReplay},
		{"ts -6 мин", st, sign(phone, with(func(c *Command) { c.TS -= 6 * 60000 })), "ph1", ErrCodeStale},
		{"ts +6 мин", st, sign(phone, with(func(c *Command) { c.TS += 6 * 60000 })), "ph1", ErrCodeStale},
		{"ts -5 мин ровно", st, sign(phone, with(func(c *Command) { c.TS -= 5 * 60000 })), "ph1", ""},
		{"чужой pc", st, sign(phone, with(func(c *Command) { c.PC = "pc2" })), "ph1", ErrCodeNotPaired},
		{"чужой phone", st, sign(phone, with(func(c *Command) { c.Phone = "ph2" })), "ph1", ErrCodeNotPaired},
		{"чужой отправитель", st, sign(phone, base), "ph2", ErrCodeNotPaired},
		{"нет привязки", unpaired, sign(phone, base), "ph1", ErrCodeNotPaired},
		{"мусор в data", st, func() Signed { s, _ := Sign(phone, []byte("{")); return s }(), "ph1", ErrCodeBadSig},
	}
	for _, c := range cases {
		cmd, data, code := CheckCommand(c.st, c.s, c.from, now)
		if code != c.code {
			t.Errorf("%s: код %q, нужно %q", c.name, code, c.code)
			continue
		}
		if code == "" {
			if cmd.Seq < 17 || cmd.Child != 3 || cmd.Delta != 15 || len(data) == 0 {
				t.Errorf("%s: команда %+v", c.name, cmd)
			}
		}
	}
}

func FuzzParseMsg(f *testing.F) {
	k, _ := NewKey()
	f.Add(KindPair1, []byte(`{"phonePub":"`+MarshalPub(&k.PublicKey)+`","name":"Pixel"}`))
	f.Add(KindPair2, []byte(`{"nPhone":"AAAAAAAAAAAAAAAAAAAAAA"}`))
	f.Add(KindCmd, []byte(`{"data":"e30","sig":"AA"}`))
	f.Add("poll", []byte(`{"messages":[{"id":"1","from":"p","kind":"cmd","body":{}}]}`))
	f.Fuzz(func(t *testing.T, kind string, body []byte) {
		if kind == "poll" {
			msgs, _, err := ParsePoll(body)
			if err != nil {
				return
			}
			for _, m := range msgs {
				_, _ = ParseMsg(m)
			}
			return
		}
		v, err := ParseMsg(Msg{ID: "1", From: "p", Kind: kind, Body: json.RawMessage(body)})
		if err == nil && v == nil {
			t.Fatal("нет ошибки и нет значения")
		}
		if s, ok := v.(*Signed); ok && err == nil {
			st := State{PCID: "pc", Phone: &Phone{ID: "p", Pub: MarshalPub(&k.PublicKey)}}
			_, _, _ = CheckCommand(st, *s, "p", time.Now())
		}
	})
}

func TestNoteText(t *testing.T) {
	cases := []struct {
		in, out string
		ok      bool
	}{
		{"  Иди ужинать  ", "Иди ужинать", true},
		{"a\r\nb\rc", "a\nb\nc", true},
		{"a\tb\x00c​d", "a bcd", true},
		{"👨‍👩‍👧 ужин", "👨‍👩‍👧 ужин", true},
		{" \n\t ", "", false},
		{"", "", false},
		{strings.Repeat("я", 300), strings.Repeat("я", 300), true},
		{strings.Repeat("я", 301), strings.Repeat("я", 301), false},
		{"1\n2\n3\n4\n5\n6", "1\n2\n3\n4\n5\n6", true},
		{"1\n2\n3\n4\n5\n6\n7", "1\n2\n3\n4\n5\n6\n7", false},
		{"\n\n\n\n\n\nтекст\n", "текст", true}, // переводы по краям обрезаны
		// Только невидимые знаки — пусто (тот же набор, что NoteTest.invisibleCharsRemovedLikeGo телефона).
		{"\u200b\u200e\u200f\ufeff\u2060\u00ad\u202e\U000E0001\ufffd \n", "", false},
		{"\u200bИди\u2060 ужинать\ufeff", "Иди ужинать", true},
		{"\u200e👨\u200d👩", "👨\u200d👩", true},
	}
	for _, c := range cases {
		got, ok := NoteText(c.in)
		if got != c.out || ok != c.ok {
			t.Errorf("NoteText(%q) = %q, %v; нужно %q, %v", c.in, got, ok, c.out, c.ok)
		}
	}
}

func TestCheckNote(t *testing.T) {
	phone := mustKey(t)
	st := State{PCID: "pc", Phone: &Phone{ID: "ph", Pub: MarshalPub(&phone.PublicKey)}, LastSeq: 3}
	now := time.UnixMilli(1763490000000)
	base := Note{V: 1, T: TypeNote, PC: "pc", Phone: "ph", Seq: 4, TS: now.UnixMilli(), Child: 3, ID: "AbC-_9", Text: " Иди\r\nужинать ", ShowSec: 10}
	sign := func(n Note) Signed {
		raw, _ := json.Marshal(n)
		s, _ := Sign(phone, raw)
		return s
	}
	n, data, code := CheckNote(st, sign(base), "ph", now)
	if code != "" || n.Text != "Иди\nужинать" || n.ShowSec != 10 || n.ID != "AbC-_9" || len(data) == 0 {
		t.Fatalf("верное сообщение: %+v %q", n, code)
	}
	for _, sec := range []int{0, 30, 60} {
		b := base
		b.ShowSec = sec
		if _, _, code := CheckNote(st, sign(b), "ph", now); code != "" {
			t.Errorf("showSec %d: %s", sec, code)
		}
	}
	bad := map[string]func(*Note){
		"showSec -1":  func(n *Note) { n.ShowSec = -1 },
		"showSec 20":  func(n *Note) { n.ShowSec = 20 },
		"id с точкой": func(n *Note) { n.ID = "a.b" },
		"id 65":       func(n *Note) { n.ID = strings.Repeat("a", 65) },
		"v 2":         func(n *Note) { n.V = 2 },
	}
	for name, f := range bad {
		b := base
		f(&b)
		_, _, code := CheckNote(st, sign(b), "ph", now)
		want := ErrCodeRange
		if name == "v 2" {
			want = ErrCodeBadSig
		}
		if code != want {
			t.Errorf("%s: %q, нужно %q", name, code, want)
		}
	}
	// Неверный текст отклоняется раньше seq: номер не тратится, ответ range, а не replay.
	b := base
	b.Seq, b.Text = 2, ""
	if _, _, code := CheckNote(st, sign(b), "ph", now); code != ErrCodeRange {
		t.Fatalf("пустой текст со старым seq: %q", code)
	}
	b = base
	b.Seq = 3
	if _, _, code := CheckNote(st, sign(b), "ph", now); code != ErrCodeReplay {
		t.Fatalf("равный seq: %q", code)
	}
}

// Ловит (протокол 5а, FB-28): нет поля sound даёт 0 вместо 1 (у старого телефона пропадает звук);
// неверный вид (строка вместо числа, дробное, null, не 3 цифры) проходит или валит разбор в bad_sig
// (телефон увидел бы «чужая подпись»); неизвестный номер (999, 9) отклоняется (подмена — дело
// стража); range по стикеру тратит seq.
func TestCheckNoteStickerSound(t *testing.T) {
	phone := mustKey(t)
	st := State{PCID: "pc", Phone: &Phone{ID: "ph", Pub: MarshalPub(&phone.PublicKey)}, LastSeq: 3}
	now := time.UnixMilli(1763490000000)
	base := Note{V: 1, T: TypeNote, PC: "pc", Phone: "ph", Seq: 4, TS: now.UnixMilli(), Child: 3, ID: "AbC", Text: "Иди ужинать", ShowSec: 0}
	// sign — подписанные данные base с дописанными сырыми полями extra (`"sticker":"033"` и т. п.).
	sign := func(n Note, extra string) Signed {
		raw, _ := json.Marshal(n)
		if extra != "" {
			raw = append(raw[:len(raw)-1], []byte(","+extra+"}")...)
		}
		s, err := Sign(phone, raw)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	good := []struct {
		extra   string
		sticker string
		sound   int
	}{
		{"", "", 1},
		{`"sticker":"033","sound":2`, "033", 2},
		{`"sound":0`, "", 0},
		{`"sound":1`, "", 1},
		{`"sound":99`, "", 99},
		{`"sticker":"000"`, "000", 1},
		{`"sticker":"999","sound":9`, "999", 9},
		{`"sticker" : "074" , "sound" : 5`, "074", 5},
		{`"sticker":"033"`, "033", 1},
	}
	for _, c := range good {
		n, data, code := CheckNote(st, sign(base, c.extra), "ph", now)
		if code != "" || n.Sticker != c.sticker || n.Sound != c.sound || n.Text != "Иди ужинать" || len(data) == 0 {
			t.Errorf("%s: %q стикер %q звук %d; нужно %q, %d", c.extra, code, n.Sticker, n.Sound, c.sticker, c.sound)
		}
	}
	bad := []string{
		`"sticker":"33"`,
		`"sticker":"abc"`,
		`"sticker":"0333"`,
		`"sticker":""`,
		`"sticker":" 33"`,
		`"sticker":"٠٣٣"`,
		`"sticker":33`,
		`"sticker":null`,
		`"sticker":["033"]`,
		`"sound":-1`,
		`"sound":100`,
		`"sound":2.0`,
		`"sound":2.5`,
		`"sound":"2"`,
		`"sound":null`,
		`"sound":true`,
		`"sound":1e0`,
		`"sound":-0`,
		`"sound":{}`,
	}
	for _, extra := range bad {
		if _, _, code := CheckNote(st, sign(base, extra), "ph", now); code != ErrCodeRange {
			t.Errorf("%s: %q, нужно range", extra, code)
		}
	}
	// Неверный вид отклоняется раньше seq: номер не тратится, ответ range, а не replay.
	old := base
	old.Seq = 2
	if _, _, code := CheckNote(st, sign(old, `"sound":"2"`), "ph", now); code != ErrCodeRange {
		t.Fatalf("неверный звук со старым seq: %q", code)
	}
	if _, _, code := CheckNote(st, sign(old, `"sticker":"033"`), "ph", now); code != ErrCodeReplay {
		t.Fatalf("старый seq со стикером: %q", code)
	}
	// Разобранные поля в data не попадают: собранное без сырых — как сообщение 0.8.2.
	m := base
	m.Sticker, m.Sound = "033", 2
	raw, _ := json.Marshal(m)
	if string(raw) != `{"v":1,"t":"note","pc":"pc","phone":"ph","seq":4,"ts":1763490000000,"child":3,"id":"AbC","text":"Иди ужинать","showSec":0}` {
		t.Fatalf("data: %s", raw)
	}
}

// Ловит (протокол 5б): неизвестный op применяется как доп. время; «снять запрет» с delta проходит;
// старая команда без op перестала проходить; поле op попадает в данные старой команды (байты
// векторов Android меняются) или delta 0 уходит у «снять запрет».
func TestCheckCommandOp(t *testing.T) {
	phone := mustKey(t)
	now := time.UnixMilli(1763490000000)
	st := State{PCID: "pc1", Phone: &Phone{ID: "ph1", Pub: MarshalPub(&phone.PublicKey)}, LastSeq: 16}
	base := Command{V: 1, T: TypeCmd, PC: "pc1", Phone: "ph1", Seq: 17, TS: now.UnixMilli(), Child: 3}
	sign := func(c Command) Signed {
		raw, _ := json.Marshal(c)
		s, err := Sign(phone, raw)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	with := func(f func(*Command)) Command { c := base; f(&c); return c }
	cases := []struct {
		name string
		cmd  Command
		code string
		op   string
	}{
		{"без op (старый телефон)", with(func(c *Command) { c.Delta = 15 }), "", ""},
		{"op extra", with(func(c *Command) { c.Op = OpExtra; c.Delta = -30 }), "", OpExtra},
		{"снять запрет", with(func(c *Command) { c.Op = OpNoForbidden }), "", OpNoForbidden},
		{"снять запрет с delta", with(func(c *Command) { c.Op = OpNoForbidden; c.Delta = 15 }), ErrCodeRange, ""},
		{"неизвестный op", with(func(c *Command) { c.Op = "pause"; c.Delta = 15 }), ErrCodeRange, ""},
		{"неизвестный op, старый seq", with(func(c *Command) { c.Op = "pause"; c.Seq = 3 }), ErrCodeRange, ""},
		{"снять запрет, старый seq", with(func(c *Command) { c.Op = OpNoForbidden; c.Seq = 16 }), ErrCodeReplay, ""},
		{"снять запрет, чужой pc", with(func(c *Command) { c.Op = OpNoForbidden; c.PC = "pc2" }), ErrCodeNotPaired, ""},
	}
	for _, c := range cases {
		cmd, _, code := CheckCommand(st, sign(c.cmd), "ph1", now)
		if code != c.code {
			t.Errorf("%s: код %q, нужно %q", c.name, code, c.code)
			continue
		}
		if code == "" && cmd.Op != c.op {
			t.Errorf("%s: op %q, нужно %q", c.name, cmd.Op, c.op)
		}
	}
	old, _ := json.Marshal(with(func(c *Command) { c.Delta = 15 }))
	if strings.Contains(string(old), `"op"`) || !strings.Contains(string(old), `"delta":15`) {
		t.Fatalf("команда без op: %s", old)
	}
	nf, _ := json.Marshal(with(func(c *Command) { c.Op = OpNoForbidden }))
	if !strings.HasSuffix(string(nf), `"child":3,"op":"noForbidden"}`) {
		t.Fatalf("команда «снять запрет»: %s", nf)
	}
}

// Ловит: поля запрета в отчёте ломают отчёт старого вида (пустой статус уходит строкой "").
func TestChildReportForbidJSON(t *testing.T) {
	old, _ := json.Marshal(ChildReport{ID: 3, Name: "Миша"})
	if strings.Contains(string(old), "forbid") {
		t.Fatalf("пустой статус запрета в отчёте: %s", old)
	}
	act, _ := json.Marshal(ChildReport{ID: 3, Name: "Миша", Forbid: ForbidActive, ForbidUntilMs: 1763496000000})
	if !strings.Contains(string(act), `"blocked":false,"forbid":"active","forbidUntilMs":1763496000000,"bankMin":0,`) {
		t.Fatalf("отчёт с запретом: %s", act)
	}
}
