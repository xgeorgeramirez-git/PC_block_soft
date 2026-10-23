package service

import (
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Ловит: «снять запрет на сегодня» (R24, D-51) не снимает блокировку запретного интервала,
// не ставит флаг в карточке, или не возвращается обратно.
func TestSetNoForbidden(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	id := h.childID(childSID)
	// Запретный интервал пн 14:00–16:00 — часы ядра стоят на пн 15:00 (внутри интервала).
	h.store.SetSchedule(id, []core.Interval{{Weekday: core.Monday, StartMin: 14 * 60, EndMin: 16 * 60}})
	h.start()
	tok := h.login(adminPeer())

	var rep ipc.SetNoForbiddenReply
	h.ok(adminPeer(), tok, ipc.CmdSetNoForbidden, ipc.SetNoForbiddenRequest{ChildID: id, On: true}, &rep)
	if !rep.Card.NoForbiddenToday {
		t.Fatalf("после снятия карточка NoForbiddenToday=false, want true")
	}
	if rep.Card.Status.Lock == "forbidden" {
		t.Fatalf("после снятия всё ещё блокировка запрета: %+v", rep.Card.Status)
	}
	// Вернуть запрет.
	h.ok(adminPeer(), tok, ipc.CmdSetNoForbidden, ipc.SetNoForbiddenRequest{ChildID: id, On: false}, &rep)
	if rep.Card.NoForbiddenToday {
		t.Fatalf("после возврата NoForbiddenToday=true, want false")
	}
}

// Ловит: обнуление/убавление копилки не ограничено нулём снизу, не пишет в журнал, или
// убавляет не ту величину (FR-22, D-46).
func TestClearAndReduceBonus(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	// Начислить 60 для почвы.
	var gr ipc.GrantBonusReply
	h.ok(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 60, Reason: "за уборку"}, &gr)

	var red ipc.BonusChangeReply
	h.ok(adminPeer(), tok, ipc.CmdReduceBonus, ipc.ReduceBonusRequest{ChildID: id, Minutes: 30}, &red)
	if red.BalanceAfter != 30 {
		t.Fatalf("после убавления 30 баланс = %d, want 30", red.BalanceAfter)
	}
	// Убавить больше, чем есть — до нуля, не ниже.
	h.ok(adminPeer(), tok, ipc.CmdReduceBonus, ipc.ReduceBonusRequest{ChildID: id, Minutes: 100}, &red)
	if red.BalanceAfter != 0 {
		t.Fatalf("после убавления 100 из 30 баланс = %d, want 0", red.BalanceAfter)
	}
	// Снова начислить и обнулить.
	h.ok(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 45, Reason: "ещё"}, &gr)
	var cl ipc.BonusChangeReply
	h.ok(adminPeer(), tok, ipc.CmdClearBonus, ipc.ClearBonusRequest{ChildID: id}, &cl)
	if cl.BalanceAfter != 0 {
		t.Fatalf("после обнуления баланс = %d, want 0", cl.BalanceAfter)
	}
}

// Ловит: промокод не одноразовый, не добавляет время, или ввод доступен не из сеанса ребёнка
// (R26, D-50).
func TestPromoCreateRedeemOnce(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h.start()
	h.svc.Housekeep(h.real.Now()) // загрузить детей в память службы
	tok := h.login(adminPeer())
	id := h.childID(childSID)

	var cr ipc.CreatePromoReply
	h.ok(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "A1234", Minutes: 30}, &cr)
	if cr.Promo.CodeDisplay != "A1234" || cr.Promo.Minutes != 30 {
		t.Fatalf("создан промокод %+v", cr.Promo)
	}
	// Повтор того же кода (иной регистр) — отказ.
	dup := h.fail(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: " a1234 ", Minutes: 15})
	if dup.Field != ipc.FieldCode {
		t.Fatalf("повтор кода: ошибка не у поля code: %+v", dup)
	}
	// Ребёнок своего сеанса вводит код (в другом регистре) — принято, время добавлено.
	var rr ipc.RedeemPromoReply
	h.ok(childPeer(), "", ipc.CmdRedeemPromo, ipc.RedeemPromoRequest{ChildID: id, Code: " a1234 "}, &rr)
	if rr.AddedMin != 30 || rr.Card.ExtraMin != 30 {
		t.Fatalf("после промокода AddedMin=%d, ExtraMin=%d, want 30 и 30", rr.AddedMin, rr.Card.ExtraMin)
	}
	// Повторный ввод — «Код не подходит».
	bad := h.fail(childPeer(), "", ipc.CmdRedeemPromo, ipc.RedeemPromoRequest{ChildID: id, Code: "a1234"})
	if bad.Field != ipc.FieldCode {
		t.Fatalf("повторный ввод: ошибка не у поля code: %+v", bad)
	}
	// Список: код в использованных, с именем ребёнка.
	var list ipc.ListPromosReply
	h.ok(adminPeer(), tok, ipc.CmdListPromos, ipc.Empty{}, &list)
	if len(list.Used) != 1 || list.Used[0].UsedByName != "Миша" || len(list.Active) != 0 {
		t.Fatalf("списки: active=%+v used=%+v", list.Active, list.Used)
	}
}

// Ловит: автобонус за прошедший день не начисляется на смене суток, считается не по формуле,
// или начисляется повторно (R25, D-49).
func TestAutoBonusAtMidnight(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600)) // лимит 600 мин
	id := h.childID(childSID)
	// Предзаполнить прошедший день: использовано 500 мин → остаток 100 мин.
	// Часы ядра поставим на конец дня, чтобы сегодня = 2026-10-19.
	h.clock.Set(time.Date(2026, 10, 19, 23, 59, 59, 0, tzMSK))
	if err := h.store.SaveDayState(id, core.DayState{Day: "2026-10-19", TotalS: 500 * 60}); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.svc.Housekeep(h.real.Now()) // загрузить детей; ядро берёт день 2026-10-19 с TotalS

	// Перешагнуть полночь: день закрывается, служба начисляет автобонус за 2026-10-19.
	h.clock.Set(time.Date(2026, 10, 20, 0, 0, 1, 0, tzMSK))
	h.svc.Tick(h.clock.Now())

	bal, err := h.store.GetBonusBalance(id)
	if err != nil {
		t.Fatal(err)
	}
	// Остаток 100 мин, N=5 → 5×(100/10)=50.
	if bal != 50 {
		t.Fatalf("автобонус: баланс копилки = %d, want 50", bal)
	}
	// День помечен — повторный тик в новом дне бонус не удваивает.
	h.svc.Tick(h.clock.Now())
	if bal2, _ := h.store.GetBonusBalance(id); bal2 != 50 {
		t.Fatalf("после повторного тика баланс = %d, want 50 (без повторного начисления)", bal2)
	}
}

// Ловит: настройка N автобонуса не читается/не хранится или меняет диапазон (R25, D-49).
func TestBonusRateSetting(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h.start()
	tok := h.login(adminPeer())
	// По умолчанию 5.
	var rate ipc.BonusRateReply
	h.ok(adminPeer(), tok, ipc.CmdGetBonusRate, ipc.Empty{}, &rate)
	if rate.Minutes != 5 {
		t.Fatalf("деф. N = %d, want 5", rate.Minutes)
	}
	h.ok(adminPeer(), tok, ipc.CmdSetBonusRate, ipc.SetBonusRateRequest{Minutes: 10}, &rate)
	h.ok(adminPeer(), tok, ipc.CmdGetBonusRate, ipc.Empty{}, &rate)
	if rate.Minutes != 10 {
		t.Fatalf("после установки N = %d, want 10", rate.Minutes)
	}
}

// Ловит: «открыть папку логов» не возвращает путь (FR-25, D-48).
func TestOpenLogsPath(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h.start()
	tok := h.login(adminPeer())
	var rep ipc.OpenLogsReply
	h.ok(adminPeer(), tok, ipc.CmdOpenLogs, ipc.Empty{}, &rep)
	if rep.Path == "" {
		t.Fatalf("пустой путь к логам")
	}
}
