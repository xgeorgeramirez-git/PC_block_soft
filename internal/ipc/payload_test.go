package ipc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type nested struct {
	Items []string `json:"items"`
}

type withSlices struct {
	Name   string            `json:"name"`
	List   []int             `json:"list"`
	Nested nested            `json:"nested"`
	Ptr    *nested           `json:"ptr,omitempty"`
	Many   []nested          `json:"many"`
	Map    map[string]nested `json:"map"`
	Raw    json.RawMessage   `json:"raw,omitempty"`
	Bytes  []byte            `json:"bytes,omitempty"`
}

func TestOKTurnsNilSlicesIntoEmptyArrays(t *testing.T) {
	v := withSlices{
		Many: []nested{{}, {Items: []string{"a"}}},
		Map:  map[string]nested{"k": {}},
		Ptr:  &nested{},
	}
	r := OK(v)
	if !r.OK {
		t.Fatalf("ответ не OK: %+v", r)
	}
	s := string(r.Payload)
	if strings.Contains(s, "null") {
		t.Fatalf("в ответе null (L-22): %s", s)
	}
	for _, want := range []string{`"list":[]`, `"nested":{"items":[]}`, `"ptr":{"items":[]}`, `"many":[{"items":[]},{"items":["a"]}]`, `"map":{"k":{"items":[]}}`} {
		if !strings.Contains(s, want) {
			t.Errorf("в %s нет %s", s, want)
		}
	}
}

func TestOKLeavesOriginalValueUntouched(t *testing.T) {
	v := &withSlices{}
	_ = OK(v)
	if v.List != nil || v.Nested.Items != nil {
		t.Fatalf("OK изменил переданное значение: %+v", v)
	}
}

func TestOKKeepsRawMessageAndBytes(t *testing.T) {
	v := withSlices{Raw: json.RawMessage(`{"x":1}`), Bytes: []byte("ab")}
	s := string(OK(v).Payload)
	if !strings.Contains(s, `"raw":{"x":1}`) || !strings.Contains(s, `"bytes":"YWI="`) {
		t.Fatalf("RawMessage или байты испорчены: %s", s)
	}
}

func TestOKNilAndScalars(t *testing.T) {
	if r := OK(nil); !r.OK || len(r.Payload) != 0 {
		t.Errorf("OK(nil): %+v", r)
	}
	if r := OK(Empty{}); string(r.Payload) != `{}` {
		t.Errorf("OK(Empty): %s", r.Payload)
	}
	if r := OK(7); string(r.Payload) != `7` {
		t.Errorf("OK(7): %s", r.Payload)
	}
	var nilPtr *nested
	if r := OK(nilPtr); !r.OK {
		t.Errorf("OK(nil pointer): %+v", r)
	}
	if r := OK(make(chan int)); r.OK || r.Error != TextInternal {
		t.Errorf("значение, которое нельзя записать, должно дать человеческую ошибку: %+v", r)
	}
}

func TestFailHelpers(t *testing.T) {
	if r := Fail("Нельзя"); r.OK || r.Error != "Нельзя" || r.Field != "" {
		t.Errorf("Fail: %+v", r)
	}
	if r := FailField("Не меньше 6 символов", FieldPassword); r.OK || r.Field != "password" {
		t.Errorf("FailField: %+v", r)
	}
	r := FailWith("Неверный пароль. Осталось попыток: 3", FieldPassword, LoginDenied{AttemptsLeft: 3})
	if r.OK || r.Field != "password" || string(r.Payload) != `{"attemptsLeft":3,"pauseS":0}` {
		t.Errorf("FailWith: %+v (%s)", r, r.Payload)
	}
}

func TestMsgInto(t *testing.T) {
	m := Msg{Cmd: CmdLogin, Payload: json.RawMessage(`{"password":"abc","extra":1}`)}
	var req LoginRequest
	if err := m.Into(&req); err != nil || req.Password != "abc" {
		t.Fatalf("req=%+v err=%v", req, err)
	}
	// Пустая нагрузка — нулевое значение, не ошибка.
	var empty LoginRequest
	if err := (Msg{Cmd: CmdLogin}).Into(&empty); err != nil || empty.Password != "" {
		t.Fatalf("пустая нагрузка: %+v %v", empty, err)
	}
	// Нагрузка не того вида — ошибка, а не паника.
	if err := (Msg{Cmd: CmdLogin, Payload: json.RawMessage(`[1,2]`)}).Into(&req); err == nil {
		t.Fatalf("массив вместо объекта принят")
	}
}

func TestReplyInto(t *testing.T) {
	var dst HelloReply
	if err := OK(HelloReply{Version: "1"}).Into(&dst); err != nil || dst.Version != "1" {
		t.Fatalf("dst=%+v err=%v", dst, err)
	}
	if err := OK(HelloReply{}).Into(nil); err != nil {
		t.Fatalf("Into(nil) должен игнорировать нагрузку: %v", err)
	}
	err := FailField("Неверный пароль", FieldPassword).Into(&dst)
	var re *ReplyError
	if !errors.As(err, &re) || re.Text != "Неверный пароль" || re.Field != "password" || err.Error() != "Неверный пароль" {
		t.Fatalf("ошибка ответа: %v", err)
	}
	bad := Reply{OK: true, Payload: json.RawMessage(`"строка"`)}
	if err := bad.Into(&dst); err == nil {
		t.Fatalf("нагрузка не того вида принята")
	}
}

type fakeCaller struct {
	got   Msg
	reply Reply
	err   error
}

func (f *fakeCaller) Do(m Msg) (Reply, error) { f.got = m; return f.reply, f.err }

func TestCall(t *testing.T) {
	f := &fakeCaller{reply: OK(SessionStateReply{Active: true, RemainingS: 540})}
	var out SessionStateReply
	if err := Call(f, CmdSessionState, "tok", nil, &out); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if f.got.Cmd != CmdSessionState || f.got.Token != "tok" || len(f.got.Payload) != 0 || !out.Active || out.RemainingS != 540 {
		t.Fatalf("got=%+v out=%+v", f.got, out)
	}
	if err := Call(f, CmdLogin, "", LoginRequest{Password: "p"}, nil); err != nil || string(f.got.Payload) != `{"password":"p"}` {
		t.Fatalf("запрос %s, ошибка %v", f.got.Payload, err)
	}
	f.err = ErrNotService
	if err := Call(f, CmdHello, "", nil, nil); !errors.Is(err, ErrNotService) {
		t.Fatalf("ошибка транспорта потеряна: %v", err)
	}
	f.err = nil
	f.reply = Fail("Нельзя")
	if err := Call(f, CmdHello, "", nil, nil); err == nil || err.Error() != "Нельзя" {
		t.Fatalf("отказ службы: %v", err)
	}
	if err := Call(f, CmdHello, "", make(chan int), nil); err == nil {
		t.Fatalf("запрос, который нельзя записать, принят")
	}
}
