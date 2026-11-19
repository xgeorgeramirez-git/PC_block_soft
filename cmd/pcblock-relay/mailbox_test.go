package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"strings"
	"testing"
	"time"
)

func TestQueueBasics(t *testing.T) {
	var q queue
	now := time.Unix(1000, 0)
	ttl := 30 * time.Second
	for i := 0; i < queueMax; i++ {
		if !q.put(Msg{ID: fmt.Sprint(i), Kind: "cmd"}, now, ttl) {
			t.Fatalf("put %d отказан", i)
		}
	}
	if q.put(Msg{ID: "x"}, now, ttl) {
		t.Fatal("33-е сообщение принято")
	}
	// Истёкшие освобождают место.
	if !q.put(Msg{ID: "late"}, now.Add(ttl), ttl) {
		t.Fatal("после истечения место не освободилось")
	}
	got := q.take(now.Add(ttl), ttl, 1<<20)
	if len(got) != 1 || got[0].ID != "late" {
		t.Fatalf("take: %+v", got)
	}
	if q.len() != 0 {
		t.Fatalf("len %d", q.len())
	}
}

// Свойство очереди: любая последовательность «положить / выдать / убрать / время идёт»
// не выдаёт сообщение дважды, не выдаёт истёкшее и не теряет живое (выдаётся по порядку,
// пока хватает бюджета байтов; хотя бы одно за раз).
func TestQueueProperty(t *testing.T) {
	r := mrand.New(mrand.NewSource(20261118))
	ttl := 30 * time.Second
	for run := 0; run < 1000; run++ {
		var q queue
		type item struct {
			id   string
			at   time.Time
			size int
		}
		var model []item
		delivered := map[string]bool{}
		now := time.Unix(1_700_000_000, 0)
		next := 0
		steps := 20 + r.Intn(80)
		for st := 0; st < steps; st++ {
			switch op := r.Intn(10); {
			case op < 4: // положить
				id := fmt.Sprintf("m%d", next)
				next++
				size := r.Intn(4000)
				live := model[:0:0]
				for _, it := range model {
					if now.Sub(it.at) < ttl {
						live = append(live, it)
					}
				}
				model = live
				ok := q.put(Msg{ID: id, Kind: "cmd", Body: json.RawMessage(`"` + strings.Repeat("a", size) + `"`)}, now, ttl)
				if want := len(model) < queueMax; ok != want {
					t.Fatalf("прогон %d: put=%v, ждали %v", run, ok, want)
				}
				if ok {
					model = append(model, item{id, now, size})
				}
			case op < 7: // выдать
				budget := 1 + r.Intn(20000)
				got := q.take(now, ttl, budget)
				live := model[:0:0]
				for _, it := range model {
					if now.Sub(it.at) < ttl {
						live = append(live, it)
					}
				}
				// Ожидаемое: префикс живых в пределах бюджета, но не меньше одного.
				var want []string
				used := 0
				for i, it := range live {
					sz := msgSize(Msg{ID: it.id, Kind: "cmd", Body: json.RawMessage(`"` + strings.Repeat("a", it.size) + `"`)})
					if i > 0 && used+sz > budget {
						break
					}
					used += sz
					want = append(want, it.id)
				}
				if len(got) != len(want) {
					t.Fatalf("прогон %d: выдано %d, ждали %d", run, len(got), len(want))
				}
				for i, m := range got {
					if m.ID != want[i] {
						t.Fatalf("прогон %d: порядок %s != %s", run, m.ID, want[i])
					}
					if delivered[m.ID] {
						t.Fatalf("прогон %d: %s выдано дважды", run, m.ID)
					}
					delivered[m.ID] = true
				}
				model = live[len(want):]
			case op < 8: // убрать (телефон не дождался)
				if len(model) > 0 {
					i := r.Intn(len(model))
					if !q.remove(model[i].id) && now.Sub(model[i].at) < ttl {
						t.Fatalf("прогон %d: remove живого не сработал", run)
					}
					model = append(model[:i:i], model[i+1:]...)
				} else if q.remove("nope") {
					t.Fatal("remove несуществующего")
				}
			default: // время идёт
				now = now.Add(time.Duration(r.Intn(15000)) * time.Millisecond)
			}
		}
		// В конце всё живое выдаётся, ничего не потеряно.
		got := q.take(now, ttl, 1<<30)
		live := 0
		for _, it := range model {
			if now.Sub(it.at) < ttl {
				live++
			}
		}
		if len(got) != live {
			t.Fatalf("прогон %d: в конце выдано %d, живых %d", run, len(got), live)
		}
		for _, m := range got {
			if delivered[m.ID] {
				t.Fatalf("прогон %d: %s выдано дважды", run, m.ID)
			}
		}
	}
}

func TestDecodeBody(t *testing.T) {
	var s sendReq
	if err := decodeJSON(strings.NewReader(`{"kind":"cmd","body":{"a":1},"zzz":true}`), &s, false); err != nil {
		t.Fatal(err)
	}
	if s.Kind != "cmd" || string(s.Body) != `{"a":1}` {
		t.Fatalf("%+v", s)
	}
	if err := decodeJSON(strings.NewReader(``), &s, false); err == nil {
		t.Fatal("пустое тело там, где оно нужно")
	}
	var e struct{}
	if err := decodeJSON(strings.NewReader(``), &e, true); err != nil {
		t.Fatal(err)
	}
	if err := decodeJSON(strings.NewReader(`{} {}`), &e, true); err == nil {
		t.Fatal("хвост после объекта")
	}
}

func FuzzDecodeBody(f *testing.F) {
	for _, s := range []string{
		`{}`, ``, `{"code":"123456","kind":"pair1","body":{"phonePub":"x"}}`,
		`{"kind":"cmd","body":{"data":"a","sig":"b"}}`, `{"to":"a","replyTo":"b","kind":"c","body":null}`,
		`{"body":[1,2,3]}`, `{"phoneId":"x"}`, `[`, `{"kind":"\u0000"}`, `null`, `{} x`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		targets := []any{&pairReq{}, &sendReq{}, &replyReq{}, &reportReq{}, &unpairReq{}, &struct{}{}}
		for _, v := range targets {
			for _, allowEmpty := range []bool{false, true} {
				err := decodeJSON(bytes.NewReader(data), v, allowEmpty)
				if err != nil {
					continue
				}
				// Разобранное тело — корректный JSON, который можно переслать как есть.
				var raw json.RawMessage
				switch x := v.(type) {
				case *pairReq:
					raw = x.Body
				case *sendReq:
					raw = x.Body
				case *replyReq:
					raw = x.Body
				case *reportReq:
					raw = x.Body
				}
				if len(raw) > 0 && !json.Valid(raw) {
					t.Fatalf("невалидное тело %q", raw)
				}
				if _, err := json.Marshal(Msg{ID: "i", From: "f", Kind: "k", Body: raw}); err != nil {
					t.Fatalf("не пересылается: %v", err)
				}
			}
		}
	})
}
