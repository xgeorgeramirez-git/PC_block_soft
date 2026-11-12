package service

import (
	"bytes"
	"fmt"
	"io"
	mrand "math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Промокоды M5 (R26, FR-35, D-75, D-75a; RM5-11, RM5-12, RM5-14; задача T3b).

// promosIn — коды в фейке хранилища: действующие и использованные, как их хранит база.
func (h *harness) promosIn() (active, used []string) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	active, used = []string{}, []string{}
	for _, p := range h.store.promos {
		if p.rec.UsedAtMs == 0 {
			active = append(active, p.rec.CodeDisplay)
		} else {
			used = append(used, p.rec.CodeDisplay)
		}
	}
	return active, used
}

// listPromos — ListPromos родителем.
func (h *harness) listPromos(tok string) ipc.ListPromosReply {
	h.t.Helper()
	var rep ipc.ListPromosReply
	h.ok(adminPeer(), tok, ipc.CmdListPromos, ipc.Empty{}, &rep)
	return rep
}

func codesOf(list []ipc.PromoView) []string {
	out := []string{}
	for _, p := range list {
		out = append(out, p.CodeDisplay)
	}
	return out
}

// withPromoRand подменяет источник случайности «Придумать коды» на время теста.
func withPromoRand(t *testing.T, r io.Reader) {
	t.Helper()
	old := promoRand
	promoRand = r
	t.Cleanup(func() { promoRand = old })
}

// promoHarness — служба с одним ребёнком и вошедшим родителем.
func promoHarness(t *testing.T) (*harness, string, int64) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h.start()
	h.svc.Housekeep(h.real.Now())
	return h, h.login(adminPeer()), h.childID(childSID)
}

// Ловит (R26, D-75, RM5-11, Review Focus 5): новый код с кириллицей или знаком принят; код
// хранится и показывается не заглавными; старый код (до 0.5.0, в том числе с кириллицей)
// перестал показываться заглавными или приниматься при вводе ребёнком.
func TestCreatePromoLatinOnlyAndUpper(t *testing.T) {
	h, tok, id := promoHarness(t)

	for _, code := range []string{"код!", "ab 12", "ＡＢ１２", "ab"} {
		f := h.fail(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: code, Minutes: 30})
		if f.Error != "Только латинские буквы A-Z и цифры" || f.Field != ipc.FieldCode {
			t.Errorf("код %q: отказ %q у поля %q; want «Только латинские буквы A-Z и цифры» у code", code, f.Error, f.Field)
		}
	}
	if f := h.fail(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "  ", Minutes: 30}); f.Error != "Введите код" {
		t.Errorf("пустой код: %q, want «Введите код»", f.Error)
	}
	if a, u := h.promosIn(); len(a)+len(u) != 0 {
		t.Fatalf("после отказов в базе коды %v %v", a, u)
	}

	var cr ipc.CreatePromoReply
	h.ok(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: " ab12 ", Minutes: 30}, &cr)
	if cr.Promo.CodeDisplay != "AB12" || cr.Message != "Промокод «AB12» на 30 минут готов" {
		t.Errorf("создан %+v, %q; want AB12", cr.Promo, cr.Message)
	}
	if a, _ := h.promosIn(); !reflect.DeepEqual(a, []string{"AB12"}) {
		t.Errorf("в базе %v, want [AB12] (хранится заглавными)", a)
	}
	if got := h.journalTexts(evPromoCreate); !reflect.DeepEqual(got, []string{"Создан промокод «AB12» на 30 мин"}) {
		t.Errorf("журнал %v", got)
	}

	// Коды, созданные до 0.5.0: строчными и с кириллицей; база их не переписывает.
	for _, old := range []string{"zz9", "привет"} {
		if _, err := h.store.CreatePromo(old, 15); err != nil {
			t.Fatal(err)
		}
	}
	list := h.listPromos(tok)
	want := map[string]bool{"AB12": true, "ZZ9": true, "ПРИВЕТ": true}
	if got := codesOf(list.Active); len(got) != 3 || !want[got[0]] || !want[got[1]] || !want[got[2]] {
		t.Errorf("действующие %v, want AB12, ZZ9, ПРИВЕТ (заглавными)", got)
	}
	for _, in := range []string{"zz9", "Привет", "ab12"} {
		h.real.Advance(time.Minute) // момент использования у каждого свой
		var rr ipc.RedeemPromoReply
		h.ok(childPeer(), "", ipc.CmdRedeemPromo, ipc.RedeemPromoRequest{ChildID: id, Code: in}, &rr)
		if rr.AddedMin == 0 {
			t.Errorf("ввод %q: время не добавлено", in)
		}
	}
	if used := codesOf(h.listPromos(tok).Used); !reflect.DeepEqual(used, []string{"AB12", "ПРИВЕТ", "ZZ9"}) {
		t.Errorf("использованные %v, want [AB12 ПРИВЕТ ZZ9] (последний введённый сверху)", used)
	}
}

// Ловит (RM5-11): служба перекладывает «Использованные» не по моменту использования (новые
// сверху), а, например, по моменту создания.
func TestListPromosUsedNewestFirst(t *testing.T) {
	h, tok, _ := promoHarness(t)
	h.store.mu.Lock()
	h.store.promos = append(h.store.promos,
		&fakePromo{norm: "old", rec: PromoRecord{CodeDisplay: "OLD", Minutes: 5, CreatedAtMs: 100, UsedAtMs: 900, UsedByChild: 1}},
		&fakePromo{norm: "mid", rec: PromoRecord{CodeDisplay: "MID", Minutes: 5, CreatedAtMs: 300, UsedAtMs: 500, UsedByChild: 1}},
		&fakePromo{norm: "new", rec: PromoRecord{CodeDisplay: "NEW", Minutes: 5, CreatedAtMs: 200, UsedAtMs: 700, UsedByChild: 1}},
	)
	h.store.mu.Unlock()
	if got := codesOf(h.listPromos(tok).Used); !reflect.DeepEqual(got, []string{"OLD", "NEW", "MID"}) {
		t.Errorf("использованные %v, want [OLD NEW MID] (по used_at, новые сверху)", got)
	}
}

// Ловит (RM5-12): коды не той длины или не из своего алфавита; повторы внутри пачки; совпадение
// с кодами базы; генератор не сдаётся, когда разных кодов не хватает.
func TestGenPromoCodes(t *testing.T) {
	alphabets := map[string]string{
		ipc.CharsetDigits:  "0123456789",
		ipc.CharsetLetters: "ABCDEFGHJKLMNPQRSTUVWXYZ",
		ipc.CharsetMixed:   "ABCDEFGHJKLMNPQRSTUVWXYZ23456789",
	}
	taken := map[string]bool{"aaaa": true, "0000": true}
	for cs, alpha := range alphabets {
		codes, ok := genPromoCodes(mrand.New(mrand.NewSource(7)), 50, 4, cs, taken)
		if !ok || len(codes) != 50 {
			t.Fatalf("%s: ok=%v, кодов %d; want 50", cs, ok, len(codes))
		}
		seen := map[string]bool{}
		letters, digits := false, false
		for _, c := range codes {
			if len(c) != 4 {
				t.Errorf("%s: код %q не из 4 знаков", cs, c)
			}
			for _, r := range c {
				if !strings.ContainsRune(alpha, r) {
					t.Errorf("%s: код %q: знак %q не из алфавита", cs, c, r)
				}
				letters = letters || r >= 'A' && r <= 'Z'
				digits = digits || r >= '0' && r <= '9'
			}
			if seen[c] {
				t.Errorf("%s: код %q повторился", cs, c)
			}
			seen[c] = true
			if taken[core.NormalizePromoCode(c)] {
				t.Errorf("%s: код %q уже есть в базе", cs, c)
			}
		}
		if cs == ipc.CharsetMixed && !(letters && digits) {
			t.Errorf("mixed: в 50 кодах нет и букв, и цифр: %v", codes)
		}
	}

	// Все коды из 4 цифр, кроме одного, уже есть — два разных не придумать.
	full := map[string]bool{}
	for i := 0; i < 10000; i++ {
		if i != 4242 {
			full[fmt.Sprintf("%04d", i)] = true
		}
	}
	if len(full) != 9999 {
		t.Fatalf("taken %d", len(full))
	}
	if codes, ok := genPromoCodes(mrand.New(mrand.NewSource(1)), 2, 4, ipc.CharsetDigits, full); ok {
		t.Errorf("9 999 из 10 000 заняты, а придуманы %v", codes)
	}
	// Источник всегда отдаёт ноль: один код «0000» есть, второго разного нет.
	zero := bytes.NewReader(make([]byte, 1<<16))
	if codes, ok := genPromoCodes(zero, 1, 4, ipc.CharsetDigits, nil); !ok || !reflect.DeepEqual(codes, []string{"0000"}) {
		t.Errorf("один код из нулей: %v, %v; want [0000]", codes, ok)
	}
	if codes, ok := genPromoCodes(bytes.NewReader(make([]byte, 1<<16)), 2, 4, ipc.CharsetDigits, nil); ok {
		t.Errorf("из одних нулей два разных кода: %v", codes)
	}
	// Источник кончился или всегда даёт байт вне честного диапазона — отказ, а не зависание.
	if _, ok := genPromoCodes(bytes.NewReader(nil), 1, 4, ipc.CharsetDigits, nil); ok {
		t.Error("пустой источник: ok = true")
	}
	if _, ok := genPromoCodes(bytes.NewReader(bytes.Repeat([]byte{255}, 1<<20)), 1, 4, ipc.CharsetDigits, nil); ok {
		t.Error("источник одних 255 (вне 0…249 для 10 цифр): ok = true")
	}
	if _, ok := genPromoCodes(mrand.New(mrand.NewSource(1)), 1, 4, "emoji", nil); ok {
		t.Error("неизвестный состав: ok = true")
	}
}

// Ловит (ТЗ 8.3.10, FR-35): в придуманных кодах с буквами попадаются похожие знаки I, O, 0, 1,
// и ребёнок путает их при вводе; или из «Только цифры» пропала какая-то цифра.
func TestGenPromoCodesNoLookalikes(t *testing.T) {
	for _, cs := range []string{ipc.CharsetLetters, ipc.CharsetMixed} {
		codes, ok := genPromoCodes(mrand.New(mrand.NewSource(11)), 1000, 12, cs, nil)
		if !ok || len(codes) != 1000 {
			t.Fatalf("%s: ok=%v, кодов %d; want 1000", cs, ok, len(codes))
		}
		for _, c := range codes {
			if strings.ContainsAny(c, "IO01") {
				t.Fatalf("%s: код %q с похожим знаком I, O, 0 или 1", cs, c)
			}
		}
	}
	codes, ok := genPromoCodes(mrand.New(mrand.NewSource(11)), 1000, 12, ipc.CharsetDigits, nil)
	if !ok || len(codes) != 1000 {
		t.Fatalf("digits: ok=%v, кодов %d; want 1000", ok, len(codes))
	}
	all := strings.Join(codes, "")
	for _, d := range "0123456789" {
		if !strings.ContainsRune(all, d) {
			t.Errorf("digits: в 1000 кодах нет цифры %q", d)
		}
	}
}

// Ловит (RM5-12, Review Focus 5): придуманный код совпал со старым кодом базы, записанным
// строчными, или с использованным кодом (UNIQUE общая), потому что сравнение без нормализации.
func TestGeneratePromosSkipsTakenNormalized(t *testing.T) {
	h, tok, id := promoHarness(t)
	if _, err := h.store.CreatePromo("zz99", 15); err != nil { // создан до 0.5.0 строчными
		t.Fatal(err)
	}
	if _, err := h.store.CreatePromo("aAaA", 15); err != nil { // и такой мог остаться
		t.Fatal(err)
	}
	var rr ipc.RedeemPromoReply
	h.ok(childPeer(), "", ipc.CmdRedeemPromo, ipc.RedeemPromoRequest{ChildID: id, Code: "aaaa"}, &rr)
	// В составе mixed (без I, O, 0, 1) байт i — знак i: Z=23, 9=31, A=0, B=1. Первая попытка «ZZ99»,
	// вторая «AAAA» (использованный), третья «BBBB».
	withPromoRand(t, bytes.NewReader([]byte{23, 23, 31, 31, 0, 0, 0, 0, 1, 1, 1, 1}))
	var rep ipc.GeneratePromosReply
	h.ok(adminPeer(), tok, ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 1, Length: 4, Charset: ipc.CharsetMixed, Minutes: 10}, &rep)
	if got := codesOf(rep.Promos); !reflect.DeepEqual(got, []string{"BBBB"}) {
		t.Errorf("придуманы %v, want [BBBB]", got)
	}
}

// Ловит (FR-35, RM5-12): пачка создана не целиком, не на те минуты, не заглавными, журнал пишет
// строку на каждый код или коды в тексте, сообщение без склонения.
func TestGeneratePromos(t *testing.T) {
	h, tok, _ := promoHarness(t)
	var rep ipc.GeneratePromosReply
	h.ok(adminPeer(), tok, ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 20, Length: 6, Charset: ipc.CharsetMixed, Minutes: 15}, &rep)
	if rep.Message != "Готово: 20 кодов на 15 мин" {
		t.Errorf("сообщение %q", rep.Message)
	}
	if len(rep.Promos) != 20 {
		t.Fatalf("в ответе %d кодов, want 20", len(rep.Promos))
	}
	active, used := h.promosIn()
	if len(active) != 20 || len(used) != 0 {
		t.Fatalf("в базе действующих %d, использованных %d; want 20 и 0", len(active), len(used))
	}
	for _, p := range rep.Promos {
		if p.Minutes != 15 || len(p.CodeDisplay) != 6 || strings.ToUpper(p.CodeDisplay) != p.CodeDisplay {
			t.Errorf("код %+v: want 6 знаков заглавными на 15 мин", p)
		}
	}
	if got := codesOf(h.listPromos(tok).Active); len(got) != 20 {
		t.Errorf("в списке действующих %d", len(got))
	}
	evs := h.store.eventsOf(evPromoGenerate)
	if len(evs) != 1 {
		t.Fatalf("записей журнала promo_generate %d, want 1", len(evs))
	}
	if got := h.journalTexts(evPromoGenerate); !reflect.DeepEqual(got, []string{"Придумано 20 промокодов на 15 мин"}) {
		t.Errorf("журнал %v", got)
	}
	if codes, _ := evs[0].Details["codes"].([]string); !reflect.DeepEqual(codes, codesOf(rep.Promos)) {
		t.Errorf("коды в подробностях журнала %v, want %v", evs[0].Details["codes"], codesOf(rep.Promos))
	}
	if txt := h.journalTexts(evPromoGenerate)[0]; strings.Contains(txt, rep.Promos[0].CodeDisplay) {
		t.Errorf("код попал в текст журнала: %q", txt)
	}
}

// Ловит (RM5-12): пределы не проверены или ошибка не у своего поля; при отказе часть кодов всё
// же создана или записана в журнал; сбой записи пачки показан как успех.
func TestGeneratePromosLimitsAndFailure(t *testing.T) {
	h, tok, _ := promoHarness(t)
	base := ipc.GeneratePromosRequest{Count: 5, Length: 6, Charset: ipc.CharsetDigits, Minutes: 15}
	cases := []struct {
		name  string
		edit  func(*ipc.GeneratePromosRequest)
		text  string
		field string
	}{
		{"0 кодов", func(r *ipc.GeneratePromosRequest) { r.Count = 0 }, "Можно от 1 до 50 кодов", ipc.FieldCount},
		{"51 код", func(r *ipc.GeneratePromosRequest) { r.Count = 51 }, "Можно от 1 до 50 кодов", ipc.FieldCount},
		{"длина 3", func(r *ipc.GeneratePromosRequest) { r.Length = 3 }, "Длина от 4 до 12 знаков", ipc.FieldLength},
		{"длина 13", func(r *ipc.GeneratePromosRequest) { r.Length = 13 }, "Длина от 4 до 12 знаков", ipc.FieldLength},
		{"0 минут", func(r *ipc.GeneratePromosRequest) { r.Minutes = 0 }, "Можно от 1 до 600 минут", ipc.FieldMinutes},
		{"601 минута", func(r *ipc.GeneratePromosRequest) { r.Minutes = 601 }, "Можно от 1 до 600 минут", ipc.FieldMinutes},
		{"состав", func(r *ipc.GeneratePromosRequest) { r.Charset = "emoji" }, ipc.TextBadRequest, ""},
	}
	for _, c := range cases {
		req := base
		c.edit(&req)
		f := h.fail(adminPeer(), tok, ipc.CmdGeneratePromos, req)
		if f.Error != c.text || f.Field != c.field {
			t.Errorf("%s: отказ %q у поля %q; want %q у %q", c.name, f.Error, f.Field, c.text, c.field)
		}
	}
	// Разных кодов не хватает: 50 кодов из 4 цифр при источнике одних нулей.
	withPromoRand(t, bytes.NewReader(make([]byte, 1<<20)))
	f := h.fail(adminPeer(), tok, ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 50, Length: 4, Charset: ipc.CharsetDigits, Minutes: 15})
	if f.Error != "Не получилось придумать столько разных кодов. Увеличьте длину" {
		t.Errorf("исчерпание: %q", f.Error)
	}
	withPromoRand(t, mrand.New(mrand.NewSource(3)))
	h.store.mu.Lock()
	h.store.failCreatePromos = true
	h.store.mu.Unlock()
	if f := h.fail(adminPeer(), tok, ipc.CmdGeneratePromos, base); f.Error != textRetry {
		t.Errorf("сбой записи пачки: %q, want %q", f.Error, textRetry)
	}
	// Сбой чтения списка кодов: ничего не сохранялось — «Не удалось выполнить», а не «сохранить».
	h.store.mu.Lock()
	h.store.failCreatePromos, h.store.failListPromos = false, true
	h.store.mu.Unlock()
	if f := h.fail(adminPeer(), tok, ipc.CmdGeneratePromos, base); f.Error != textRetry {
		t.Errorf("сбой чтения кодов: %q, want %q", f.Error, textRetry)
	}
	if a, u := h.promosIn(); len(a)+len(u) != 0 {
		t.Errorf("после отказов в базе коды %v %v", a, u)
	}
	if n := len(h.store.eventsOf(evPromoGenerate)); n != 0 {
		t.Errorf("после отказов записей журнала %d", n)
	}
}

// Ловит (RM5-14): «Очистить историю» удаляет действующие коды, не пишет журнал с числом, или
// удалённый код нельзя создать заново.
func TestClearPromoHistory(t *testing.T) {
	h, tok, id := promoHarness(t)
	for _, c := range []string{"KEEP1", "KEEP2", "USED1", "USED2"} {
		h.ok(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: c, Minutes: 10}, nil)
	}
	for _, c := range []string{"used1", "used2"} {
		h.ok(childPeer(), "", ipc.CmdRedeemPromo, ipc.RedeemPromoRequest{ChildID: id, Code: c}, nil)
	}
	var rep ipc.ClearPromoHistoryReply
	h.ok(adminPeer(), tok, ipc.CmdClearPromoHistory, ipc.Empty{}, &rep)
	if rep.Message != "История промокодов очищена" {
		t.Errorf("сообщение %q", rep.Message)
	}
	list := h.listPromos(tok)
	if len(list.Used) != 0 || len(list.Active) != 2 {
		t.Errorf("после очистки действующие %v, использованные %v; want 2 и 0", codesOf(list.Active), codesOf(list.Used))
	}
	if got := h.journalTexts(evPromoHistoryCleared); !reflect.DeepEqual(got, []string{"История промокодов очищена (2)"}) {
		t.Errorf("журнал %v", got)
	}
	h.ok(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "used1", Minutes: 10}, nil)
}

// Ловит (RM5-12): неверное склонение числа кодов в сообщении и журнале (11–14 — «кодов»).
func TestCodesText(t *testing.T) {
	cases := []struct {
		n             int
		reply, events string
	}{
		{1, "1 код", "Придуман 1 промокод на 15 мин"},
		{2, "2 кода", "Придумано 2 промокода на 15 мин"},
		{4, "4 кода", "Придумано 4 промокода на 15 мин"},
		{5, "5 кодов", "Придумано 5 промокодов на 15 мин"},
		{11, "11 кодов", "Придумано 11 промокодов на 15 мин"},
		{12, "12 кодов", "Придумано 12 промокодов на 15 мин"},
		{14, "14 кодов", "Придумано 14 промокодов на 15 мин"},
		{21, "21 код", "Придуман 21 промокод на 15 мин"},
		{22, "22 кода", "Придумано 22 промокода на 15 мин"},
		{25, "25 кодов", "Придумано 25 промокодов на 15 мин"},
		{50, "50 кодов", "Придумано 50 промокодов на 15 мин"},
	}
	for _, c := range cases {
		if got := codesText(c.n); got != c.reply {
			t.Errorf("codesText(%d) = %q, want %q", c.n, got, c.reply)
		}
		// Из базы число приходит float64.
		if got := eventText(evPromoGenerate, "", map[string]any{"count": float64(c.n), "minutes": float64(15)}); got != c.events {
			t.Errorf("журнал %d: %q, want %q", c.n, got, c.events)
		}
	}
}
