package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// rawFrame собирает кадр вручную: длина (little-endian) и тело как есть.
func rawFrame(length uint32, body string) []byte {
	b := make([]byte, 4, 4+len(body))
	binary.LittleEndian.PutUint32(b, length)
	return append(b, body...)
}

// goodFrame собирает кадр с верной длиной тела.
func goodFrame(body string) []byte { return rawFrame(uint32(len(body)), body) }

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := Msg{Cmd: CmdLogin, Token: "tok-1", Payload: json.RawMessage(`{"password":"пароль & <тест>"}`)}
	frame := Encode(in)
	if got := binary.LittleEndian.Uint32(frame[:4]); int(got) != len(frame)-4 {
		t.Fatalf("длина в заголовке %d, тело %d", got, len(frame)-4)
	}
	if !bytes.Contains(frame, []byte("<тест>")) || bytes.Contains(frame, []byte("\\u003c")) {
		t.Fatalf("HTML не должен экранироваться: %s", frame[4:])
	}
	out, err := Decode(frame)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.Cmd != in.Cmd || out.Token != in.Token || string(out.Payload) != string(in.Payload) {
		t.Fatalf("получили %+v, ждали %+v", out, in)
	}
}

func TestMsgJSONKeysAreLowerCamel(t *testing.T) {
	body := string(Encode(Msg{Cmd: "X", Token: "t", Payload: json.RawMessage(`{}`)})[4:])
	for _, key := range []string{`"cmd":"X"`, `"token":"t"`, `"payload":{}`} {
		if !strings.Contains(body, key) {
			t.Errorf("в %s нет %s", body, key)
		}
	}
	rep := string(EncodeReply(Reply{OK: false, Error: "Ошибка", Field: "password", Payload: json.RawMessage(`{"a":1}`)})[4:])
	for _, key := range []string{`"ok":false`, `"error":"Ошибка"`, `"field":"password"`, `"payload":{"a":1}`} {
		if !strings.Contains(rep, key) {
			t.Errorf("в %s нет %s", rep, key)
		}
	}
}

func TestEncodeOmitsEmptyOptionals(t *testing.T) {
	body := string(Encode(Msg{Cmd: "Hello"})[4:])
	if body != `{"cmd":"Hello"}` {
		t.Fatalf("тело %s", body)
	}
	rep := string(EncodeReply(Reply{OK: true})[4:])
	if rep != `{"ok":true}` {
		t.Fatalf("ответ %s", rep)
	}
}

func TestEncodeDropsInvalidPayload(t *testing.T) {
	// Encode не возвращает ошибку: испорченная полезная нагрузка не должна ломать кадр.
	frame := Encode(Msg{Cmd: "X", Payload: json.RawMessage(`{не json`)})
	m, err := Decode(frame)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Cmd != "X" || len(m.Payload) != 0 {
		t.Fatalf("получили %+v", m)
	}
	r, err := DecodeReply(EncodeReply(Reply{OK: true, Payload: json.RawMessage(`[`)}))
	if err != nil || !r.OK || len(r.Payload) != 0 {
		t.Fatalf("ответ %+v, ошибка %v", r, err)
	}
}

func TestDecodeErrors(t *testing.T) {
	big := rawFrame(MaxFrame+1, "")
	cases := []struct {
		name  string
		frame []byte
		want  error
	}{
		{"пустой", nil, ErrTruncated},
		{"короче заголовка", []byte{1, 0}, ErrTruncated},
		{"нулевая длина", rawFrame(0, ""), ErrEmptyFrame},
		{"огромная длина", rawFrame(0xFFFFFFFF, "{}"), ErrFrameTooLarge},
		{"чуть больше предела", big, ErrFrameTooLarge},
		{"усечённое тело", rawFrame(20, `{"cmd":"A"}`), ErrTruncated},
		{"лишние байты", rawFrame(11, `{"cmd":"A"} `+"x"), ErrTrailing},
		{"не JSON", goodFrame("hello"), ErrBadMessage},
		{"JSON не объект", goodFrame("[]"), ErrBadMessage},
		{"пустая команда", goodFrame(`{"token":"t"}`), ErrBadMessage},
		{"команда не строка", goodFrame(`{"cmd":42}`), ErrBadMessage},
		{"два объекта", goodFrame(`{"cmd":"A"}{}`), ErrBadMessage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Decode(c.frame)
			if !errors.Is(err, c.want) {
				t.Fatalf("ошибка %v, ждали %v", err, c.want)
			}
		})
	}
}

func TestDecodeFrameAtLimit(t *testing.T) {
	// Кадр ровно в 1 МБ принимается, на байт больше нет.
	prefix := `{"cmd":"A","payload":"`
	suffix := `"}`
	fill := MaxFrame - len(prefix) - len(suffix)
	body := prefix + strings.Repeat("a", fill) + suffix
	if len(body) != MaxFrame {
		t.Fatalf("тело %d, ждали %d", len(body), MaxFrame)
	}
	if _, err := Decode(rawFrame(uint32(len(body)), body)); err != nil {
		t.Fatalf("кадр в предел отвергнут: %v", err)
	}
	over := body + " "
	if _, err := Decode(rawFrame(uint32(len(over)), over)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("кадр сверх предела: %v", err)
	}
}

func TestDecodeNullPayloadIsEmpty(t *testing.T) {
	m, err := Decode(goodFrame(`{"cmd":"A","payload":null}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(m.Payload) != 0 {
		t.Fatalf("payload %q, ждали пусто", m.Payload)
	}
}

func TestDecodeIgnoresUnknownFields(t *testing.T) {
	body := `{"cmd":"A","future":{"x":1}}`
	if m, err := Decode(rawFrame(uint32(len(body)), body)); err != nil || m.Cmd != "A" {
		t.Fatalf("m=%+v err=%v", m, err)
	}
}

func TestDecodeReply(t *testing.T) {
	r, err := DecodeReply(EncodeReply(Reply{OK: false, Error: "Неверный пароль. Осталось попыток: 3", Field: "password"}))
	if err != nil {
		t.Fatalf("DecodeReply: %v", err)
	}
	if r.OK || r.Error != "Неверный пароль. Осталось попыток: 3" || r.Field != "password" {
		t.Fatalf("ответ %+v", r)
	}
	if _, err := DecodeReply(goodFrame("42")); !errors.Is(err, ErrBadMessage) {
		t.Fatalf("число вместо ответа: %v", err)
	}
	if _, err := DecodeReply(rawFrame(0, "")); !errors.Is(err, ErrEmptyFrame) {
		t.Fatalf("пустой кадр: %v", err)
	}
}

func TestReadWriteMsgOverStream(t *testing.T) {
	var buf bytes.Buffer
	for _, cmd := range []string{"A", "B", "C"} {
		if err := WriteMsg(&buf, Msg{Cmd: cmd}); err != nil {
			t.Fatalf("WriteMsg: %v", err)
		}
	}
	for _, want := range []string{"A", "B", "C"} {
		m, err := ReadMsg(&buf)
		if err != nil || m.Cmd != want {
			t.Fatalf("ждали %s, получили %+v, %v", want, m, err)
		}
	}
	if _, err := ReadMsg(&buf); !errors.Is(err, io.EOF) {
		t.Fatalf("конец потока: %v", err)
	}
}

func TestReadReplyOverStream(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReply(&buf, OK(HelloReply{Protocol: ProtocolVersion, Version: "0.1.0"})); err != nil {
		t.Fatalf("WriteReply: %v", err)
	}
	r, err := ReadReply(&buf)
	if err != nil || !r.OK {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	var h HelloReply
	if err := r.Into(&h); err != nil || h.Version != "0.1.0" {
		t.Fatalf("h=%+v err=%v", h, err)
	}
}

func TestReadFrameErrors(t *testing.T) {
	if _, err := ReadFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Errorf("пустой поток: %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{5, 0})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("обрыв в заголовке: %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader(rawFrame(10, "abc"))); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("обрыв в теле: %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader(rawFrame(0, ""))); !errors.Is(err, ErrEmptyFrame) {
		t.Errorf("нулевая длина: %v", err)
	}
	// Огромную длину отвергаем по заголовку, не пытаясь выделить память под тело.
	if _, err := ReadFrame(bytes.NewReader(rawFrame(0xFFFFFFFF, ""))); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("огромная длина: %v", err)
	}
}

func TestReadHeaderAndBodySeparately(t *testing.T) {
	r := bytes.NewReader(rawFrame(7, `{"a":1}`))
	n, err := ReadHeader(r)
	if err != nil || n != 7 {
		t.Fatalf("ReadHeader: %d, %v", n, err)
	}
	body, err := ReadBody(r, n)
	if err != nil || string(body) != `{"a":1}` {
		t.Fatalf("ReadBody: %q, %v", body, err)
	}
}

// oneByteReader отдаёт по одному байту: кадр собирается из кусков.
type oneByteReader struct{ r io.Reader }

func (o oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

func TestReadMsgFromTrickle(t *testing.T) {
	frame := Encode(Msg{Cmd: CmdHello, Payload: json.RawMessage(`{"a":"б"}`)})
	m, err := ReadMsg(oneByteReader{bytes.NewReader(frame)})
	if err != nil || m.Cmd != CmdHello {
		t.Fatalf("m=%+v err=%v", m, err)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWriteFrameErrors(t *testing.T) {
	if err := WriteFrame(io.Discard, nil); !errors.Is(err, ErrEmptyFrame) {
		t.Errorf("пустое тело: %v", err)
	}
	if err := WriteFrame(io.Discard, make([]byte, MaxFrame+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("тело больше предела: %v", err)
	}
	if err := WriteMsg(failWriter{}, Msg{Cmd: "A"}); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("ошибка записи потеряна: %v", err)
	}
	huge := Msg{Cmd: "A", Payload: json.RawMessage(`"` + strings.Repeat("a", MaxFrame) + `"`)}
	if err := WriteMsg(io.Discard, huge); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("кадр сверх предела: %v", err)
	}
	if err := WriteReply(io.Discard, Reply{OK: true, Payload: huge.Payload}); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("ответ сверх предела: %v", err)
	}
}

func TestErrorHidesTechnicalDetails(t *testing.T) {
	cause := errors.New("CreateFile: The system cannot find the file specified")
	err := newError(ErrUnavailable, cause)
	if err.Error() != "Программа не отвечает" {
		t.Fatalf("текст для человека: %q", err.Error())
	}
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("Is: вид и причина должны находиться")
	}
	var e *Error
	if !errors.As(err, &e) || !strings.Contains(e.Detail(), "CreateFile") {
		t.Fatalf("подробность для журнала потеряна: %v", err)
	}
	if ErrNotService.Error() != "Программа не отвечает" {
		t.Fatalf("ErrNotService: %q", ErrNotService)
	}
	if newError(ErrTimeout, nil).Detail() != ErrTimeout.Error() {
		t.Fatalf("без причины подробность равна виду")
	}
}

func TestPeerString(t *testing.T) {
	p := Peer{PID: 42, SessionID: 3, SID: "S-1-5-18", System: true, Admin: true, Elevated: true}
	s := p.String()
	for _, want := range []string{"pid=42", "session=3", "S-1-5-18", "system"} {
		if !strings.Contains(s, want) {
			t.Errorf("в %q нет %q", s, want)
		}
	}
	if strings.Contains(Peer{SID: "S-1-5-21-1"}.String(), "system") {
		t.Errorf("обычный пользователь помечен как система")
	}
}
