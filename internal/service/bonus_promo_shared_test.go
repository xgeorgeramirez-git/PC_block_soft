package service

import (
	mrand "math/rand"
	"reflect"
	"testing"

	"pcblock/internal/ipc"
)

// Общие функции копилки и промокодов окна и телефона (FR-22, FR-44, FR-45, R26, D-113, PL-83).

// sharedHarness — служба с Мишей и вошедшим родителем; cs — состояние Миши.
func sharedHarness(t *testing.T) (*harness, string, *childState) {
	h, tok, id := promoHarness(t)
	return h, tok, h.svc.children[id]
}

// bonusReasons — причины записей копилки ребёнка по порядку записи.
func (h *harness) bonusReasons(id int64) []string {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	out := []string{}
	for _, r := range h.store.bonus[id] {
		out = append(out, r.Reason)
	}
	return out
}

// Ловит (D-113): начисление упирается в старый предел баланса 600; одно начисление вне 1…600
// проходит; отказ пишет копилку или журнал.
func TestGrantBonusNoCeiling(t *testing.T) {
	h, tok, cs := sharedHarness(t)
	id := cs.child.ID
	if _, err := h.store.AppendBonus(id, 590, "за лето"); err != nil {
		t.Fatal(err)
	}
	var rep ipc.GrantBonusReply
	h.ok(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 600, Reason: "за уборку"}, &rep)
	if rep.BalanceAfter != 1190 || rep.Message != "Начислено 600 минут. Баланс 1190 мин" {
		t.Fatalf("окно +600 при 590: %+v; want баланс 1190", rep)
	}
	evs := h.store.eventsOf(evBonusGrant)
	if len(evs) != 1 || detStr(evs[0].Details, "text") != "Начислено в копилку +600 мин: за уборку" || detStr(evs[0].Details, "source") != "" {
		t.Fatalf("журнал окна: %+v", evs)
	}
	for _, m := range []int{601, 0, -5} {
		f := h.fail(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: m, Reason: "много"})
		if f.Error != "Можно начислить от 1 до 600 минут" || f.Field != ipc.FieldMinutes {
			t.Errorf("%d мин: отказ %q у %q; want «Можно начислить от 1 до 600 минут» у minutes", m, f.Error, f.Field)
		}
	}
	h.svc.mu.Lock()
	bal, fail := h.svc.grantBonus(cs, 601, "много", extraFromPhone)
	h.svc.mu.Unlock()
	if fail != "Можно начислить от 1 до 600 минут" || bal != 0 {
		t.Errorf("телефон 601: баланс %d, отказ %q", bal, fail)
	}
	if got, _ := h.store.GetBonusBalance(id); got != 1190 {
		t.Errorf("после отказов баланс %d, want 1190", got)
	}
	if n := len(h.store.eventsOf(evBonusGrant)); n != 1 {
		t.Errorf("после отказов записей журнала %d, want 1", n)
	}
	// Причина обязательна только у окна и только при верных минутах (порядок проверок прежний).
	if f := h.fail(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 30}); f.Field != ipc.FieldReason {
		t.Errorf("без причины: %+v; want отказ у reason", f)
	}
	// Сбой записи — «Не удалось сохранить» без поля (прежний расчёт «сколько ещё можно» убран, D-113).
	h.store.mu.Lock()
	h.store.failAppendBonus = true
	h.store.mu.Unlock()
	if f := h.fail(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 1, Reason: "за уборку"}); f.Error != textSaveFailed || f.Field != "" {
		t.Errorf("сбой записи: %q у %q, want %q без поля", f.Error, f.Field, textSaveFailed)
	}
	if n := len(h.store.eventsOf(evBonusGrant)); n != 1 {
		t.Errorf("после сбоя записей журнала %d, want 1", n)
	}
}

// Ловит (FR-44): «Положить» с телефона идёт не тем путём, что окно: без причины «С телефона», без
// подробности source, с текстом журнала окна; ставит «Отменить» окна.
func TestGrantBonusFromPhone(t *testing.T) {
	h, _, cs := sharedHarness(t)
	h.svc.mu.Lock()
	bal, fail := h.svc.grantBonus(cs, 30, "", extraFromPhone)
	undo := cs.bonusUndo
	h.svc.mu.Unlock()
	if bal != 30 || fail != "" {
		t.Fatalf("телефон +30: баланс %d, отказ %q", bal, fail)
	}
	if undo != nil {
		t.Errorf("с телефона поставлено «Отменить» окна: %+v", undo)
	}
	if got := h.bonusReasons(cs.child.ID); !reflect.DeepEqual(got, []string{"С телефона"}) {
		t.Errorf("причины копилки %v, want [С телефона]", got)
	}
	if got := h.journalTexts(evBonusGrant); !reflect.DeepEqual(got, []string{"Миша: +30 мин в копилку с телефона"}) {
		t.Errorf("журнал %q", got)
	}
}

// Ловит (FR-22, FR-44, D-46): обнуление с телефона подписано как родительское; пустая копилка
// пишет журнал или строку копилки.
func TestClearBonusSources(t *testing.T) {
	h, tok, cs := sharedHarness(t)
	id := cs.child.ID
	var empty ipc.BonusChangeReply
	h.ok(adminPeer(), tok, ipc.CmdClearBonus, ipc.ClearBonusRequest{ChildID: id}, &empty)
	if empty.Message != "Копилка уже пуста" || empty.BalanceAfter != 0 {
		t.Errorf("окно, пустая: %+v", empty)
	}
	h.svc.mu.Lock()
	was, bal, fail := h.svc.clearBonus(cs, extraFromPhone)
	h.svc.mu.Unlock()
	if was != 0 || bal != 0 || fail != "" {
		t.Errorf("телефон, пустая: было %d, стало %d, отказ %q", was, bal, fail)
	}
	if n := len(h.store.eventsOf(evBonusClear)); n != 0 || len(h.bonusReasons(id)) != 0 {
		t.Fatalf("пустая копилка: журнал %d, строки %v", n, h.bonusReasons(id))
	}

	if _, err := h.store.AppendBonus(id, 45, "за уборку"); err != nil {
		t.Fatal(err)
	}
	var rep ipc.BonusChangeReply
	h.ok(adminPeer(), tok, ipc.CmdClearBonus, ipc.ClearBonusRequest{ChildID: id}, &rep)
	if rep.Message != "Копилка обнулена" || rep.BalanceAfter != 0 {
		t.Errorf("окно: %+v", rep)
	}
	if _, err := h.store.AppendBonus(id, 75, "за уроки"); err != nil {
		t.Fatal(err)
	}
	h.svc.mu.Lock()
	was, bal, fail = h.svc.clearBonus(cs, extraFromPhone)
	h.svc.mu.Unlock()
	if was != 75 || bal != 0 || fail != "" {
		t.Errorf("телефон: было %d, стало %d, отказ %q; want 75 → 0", was, bal, fail)
	}
	if got := h.bonusReasons(id); !reflect.DeepEqual(got, []string{"за уборку", "Обнулено родителем", "за уроки", "Обнулено с телефона"}) {
		t.Errorf("причины копилки %v", got)
	}
	want := []string{"Миша: Копилка обнулена родителем (было 45 мин)", "Миша: копилка обнулена с телефона"}
	if got := h.journalTexts(evBonusClear); !reflect.DeepEqual(got, want) {
		t.Errorf("журнал %q, want %q", got, want)
	}
}

// Ловит (FR-45, R26): промокоды с телефона пишут журнал как окно (без «с телефона»), повтор кода
// не отличим от сбоя, удаление трогает использованный код или не освобождает код.
func TestPromoFromPhoneJournal(t *testing.T) {
	h, _, _ := sharedHarness(t)
	withPromoRand(t, mrand.New(mrand.NewSource(1)))
	lock := func() func() { h.svc.mu.Lock(); return h.svc.mu.Unlock }

	unlock := lock()
	p, field, fail, exists := h.svc.createPromo(" summer ", 30, extraFromPhone)
	_, field2, fail2, exists2 := h.svc.createPromo("SUMMER", 15, extraFromPhone)
	created, gfield, gfail, noCodes := h.svc.generatePromos(5, 6, ipc.CharsetMixed, 15, extraFromPhone)
	del, notFound, dfail := h.svc.deletePromo("summer", extraFromPhone)
	_, notFound2, dfail2 := h.svc.deletePromo("SUMMER", extraFromPhone)
	_, _, fail3, exists3 := h.svc.createPromo("SUMMER", 30, extraFromPhone)
	active, used, err := h.svc.promoLists()
	unlock()

	if p.CodeDisplay != "SUMMER" || p.Minutes != 30 || field != "" || fail != "" || exists {
		t.Fatalf("создание: %+v %q %q %v", p, field, fail, exists)
	}
	if !exists2 || field2 != ipc.FieldCode || fail2 != "Такой код уже есть, придумайте другой" {
		t.Errorf("повтор: поле %q, отказ %q, exists %v", field2, fail2, exists2)
	}
	if len(created) != 5 || gfield != "" || gfail != "" || noCodes {
		t.Errorf("придумать: %d кодов, %q %q %v", len(created), gfield, gfail, noCodes)
	}
	if del.CodeDisplay != "SUMMER" || del.Minutes != 30 || notFound || dfail != "" {
		t.Errorf("удаление: %+v %v %q", del, notFound, dfail)
	}
	if !notFound2 || dfail2 != "" {
		t.Errorf("повторное удаление: notFound %v, отказ %q", notFound2, dfail2)
	}
	if fail3 != "" || exists3 {
		t.Errorf("SUMMER после удаления: %q %v", fail3, exists3)
	}
	want := []string{"Промокод SUMMER на 30 мин создан с телефона", "Промокод SUMMER на 30 мин создан с телефона"}
	if got := h.journalTexts(evPromoCreate); !reflect.DeepEqual(got, want) {
		t.Errorf("журнал создания %q, want %q", got, want)
	}
	if got := h.journalTexts(evPromoDelete); !reflect.DeepEqual(got, []string{"Промокод SUMMER на 30 мин удалён с телефона"}) {
		t.Errorf("журнал удаления %q", got)
	}
	if got := h.journalTexts(evPromoGenerate); !reflect.DeepEqual(got, []string{"Придумано 5 промокодов на 15 мин с телефона"}) {
		t.Errorf("журнал «Придумать коды» %q", got)
	}
	if evs := h.store.eventsOf(evPromoDelete); len(evs) != 1 || detStr(evs[0].Details, "source") != extraSourcePhone || detStr(evs[0].Details, "code") != "SUMMER" {
		t.Errorf("подробности удаления %+v", evs)
	}
	if err != nil || used == nil || len(used) != 0 || len(active) != 6 || active[0].CodeDisplay != "SUMMER" {
		t.Errorf("списки: %v %v %v", codesOf(active), used, err)
	}
	// Сбой записи удаления — отказ словами окна, а не «кода нет».
	h.store.mu.Lock()
	h.store.failDeletePromo = true
	h.store.mu.Unlock()
	unlock = lock()
	_, notFound, dfail = h.svc.deletePromo("SUMMER", extraFromPhone)
	unlock()
	if notFound || dfail != textSaveFailed {
		t.Errorf("сбой удаления: notFound %v, отказ %q", notFound, dfail)
	}
}

// Ловит (PL-83): после переноса на общие функции окно отвечает другими текстами или полями.
func TestWindowPromoHandlersUnchanged(t *testing.T) {
	h, tok, _ := sharedHarness(t)
	var cr ipc.CreatePromoReply
	h.ok(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "summer", Minutes: 30}, &cr)
	if cr.Message != "Промокод «SUMMER» на 30 минут готов" || cr.Promo.CodeDisplay != "SUMMER" || cr.Promo.Minutes != 30 {
		t.Errorf("создан: %+v", cr)
	}
	if got := h.journalTexts(evPromoCreate); !reflect.DeepEqual(got, []string{"Создан промокод «SUMMER» на 30 мин"}) {
		t.Errorf("журнал окна %q", got)
	}
	fails := []struct {
		name, cmd   string
		req         any
		text, field string
	}{
		{"повтор", ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "SUMMER", Minutes: 15}, "Такой код уже есть, придумайте другой", ipc.FieldCode},
		{"пусто", ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: " ", Minutes: 15}, "Введите код", ipc.FieldCode},
		{"кириллица", ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "лето", Minutes: 15}, "Только латинские буквы A-Z и цифры", ipc.FieldCode},
		{"коротко", ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "AB", Minutes: 15}, "Код от 3 до 16 знаков", ipc.FieldCode},
		{"601 мин", ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "WINTER", Minutes: 601}, "Можно от 1 до 600 минут", ipc.FieldMinutes},
		{"51 код", ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 51, Length: 6, Charset: ipc.CharsetMixed, Minutes: 15}, "Можно от 1 до 50 кодов", ipc.FieldCount},
		{"длина 3", ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 5, Length: 3, Charset: ipc.CharsetMixed, Minutes: 15}, "Длина от 4 до 12 знаков", ipc.FieldLength},
		{"состав", ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 5, Length: 6, Charset: "emoji", Minutes: 15}, ipc.TextBadRequest, ""},
		{"0 мин", ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 5, Length: 6, Charset: ipc.CharsetMixed, Minutes: 0}, "Можно от 1 до 600 минут", ipc.FieldMinutes},
	}
	for _, c := range fails {
		if f := h.fail(adminPeer(), tok, c.cmd, c.req); f.Error != c.text || f.Field != c.field {
			t.Errorf("%s: %q у %q; want %q у %q", c.name, f.Error, f.Field, c.text, c.field)
		}
	}
	var gr ipc.GeneratePromosReply
	h.ok(adminPeer(), tok, ipc.CmdGeneratePromos, ipc.GeneratePromosRequest{Count: 2, Length: 6, Charset: ipc.CharsetMixed, Minutes: 15}, &gr)
	if gr.Message != "Готово: 2 кода на 15 мин" || len(gr.Promos) != 2 {
		t.Errorf("придуманы: %+v", gr)
	}
	if got := h.journalTexts(evPromoGenerate); !reflect.DeepEqual(got, []string{"Придумано 2 промокода на 15 мин"}) {
		t.Errorf("журнал окна %q", got)
	}
	list := h.listPromos(tok)
	if len(list.Active) != 3 || list.Used == nil || len(list.Used) != 0 {
		t.Errorf("список: %+v", list)
	}
	h.store.mu.Lock()
	h.store.failListPromos = true
	h.store.mu.Unlock()
	if f := h.fail(adminPeer(), tok, ipc.CmdListPromos, ipc.Empty{}); f.Error != textRetry {
		t.Errorf("сбой списка: %q", f.Error)
	}
}
