package ipc

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Фаззинг разбора кадров (ТЗ 10.4, L-37): любой мусор от клиента даёт ошибку, а не панику,
// не зависание и не выделение памяти под заявленную длину. Запуск: go test -run '^$'
// -fuzz FuzzDecode -fuzztime 30s ./internal/ipc/
func FuzzDecode(f *testing.F) {
	valid := Encode(Msg{Cmd: CmdLogin, Token: "t", Payload: json.RawMessage(`{"password":"пароль"}`)})
	validReply := EncodeReply(Reply{OK: false, Error: "Неверный пароль", Field: "password", Payload: json.RawMessage(`{"attemptsLeft":3}`)})
	seeds := [][]byte{
		nil,                         // пустой
		{0},                         // короче заголовка
		{5, 0, 0, 0},                // длина без тела
		valid[:len(valid)-3],        // усечённый кадр
		append([]byte{}, valid...),  // валидный запрос
		validReply,                  // валидный ответ
		{0xFF, 0xFF, 0xFF, 0xFF, 1}, // огромная длина
		{0, 0, 0, 0},                // нулевая длина
		rawFrame(5, "hello"),        // не JSON
		goodFrame(`{"cmd":"A"} x`),  // лишнее после JSON
		goodFrame(`{"cmd":"A","payload":` + string(bytes.Repeat([]byte("["), 5000)) + `}`), // глубокая вложенность
		goodFrame(`{"cmd":"` + string([]byte{0xff, 0xfe}) + `"}`),                          // не UTF-8
		goodFrame(`{"cmd":"A","payload":null}`),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Decode(data)
		if err == nil {
			if m.Cmd == "" {
				t.Fatalf("принято сообщение без команды: %q", data)
			}
			// Принятое сообщение записывается и читается снова с теми же команды и токеном.
			again := Encode(m)
			if len(again)-headerLen <= MaxFrame {
				m2, err := Decode(again)
				if err != nil {
					t.Fatalf("принятое сообщение не читается после записи: %v\nвход: %q", err, data)
				}
				if m2.Cmd != m.Cmd || m2.Token != m.Token {
					t.Fatalf("команда или токен изменились: %+v -> %+v", m, m2)
				}
			}
		}
		if r, err := DecodeReply(data); err == nil {
			if len(EncodeReply(r)) < headerLen+2 {
				t.Fatalf("ответ записался пустым: %+v", r)
			}
		}
		// Потоковое чтение тех же байт: ошибка или кадр, но не паника и не зависание.
		_, _ = ReadMsg(bytes.NewReader(data))
		_, _ = ReadReply(bytes.NewReader(data))
	})
}
