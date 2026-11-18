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
