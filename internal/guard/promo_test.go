package guard

import (
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// FR-35, R26, ТЗ 8.2.3 п.3: «Ввести промокод» на экране блокировки. Ребёнок вводит код без
// пароля родителя; верный код — плашка «Добавлено N минут», неверный — «Код не подходит».

func buttonOf(list []ButtonView, b Button) *ButtonView {
	for i := range list {
		if list[i].Button == b {
			return &list[i]
		}
	}
	return nil
}

// Ловит: кнопки промокода нет на экране, она стоит не на месте или недоступна в запретные часы
// (ТЗ 8.2.3 п.3: промокод доступен и в запретный интервал).
func TestLockPromoButton(t *testing.T) {
	v := locked(timeUpDecision()).View(t0)
	b := buttonOf(v.Buttons, ButtonPromo)
	if b == nil || !b.Enabled || b.Label != "Ввести промокод" || b.Main {
		t.Fatalf("нет доступной второстепенной «Ввести промокод»: %+v", v.Buttons)
	}
	// Порядок: «Попросить ещё», («Взять из копилки»), «Ввести промокод», «Пароль родителя», выходы.
	got := []Button{}
	for _, x := range v.Buttons {
		got = append(got, x.Button)
	}
	want := []Button{ButtonAskMore, ButtonPromo, ButtonParentPassword, ButtonShutdown, ButtonLogoff, ButtonSwitchUser}
	if len(got) != len(want) {
		t.Fatalf("кнопки %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("кнопки %v, want %v", got, want)
		}
	}

	df := timeUpDecision()
	df.Lock = core.LockForbidden
	if b := buttonOf(locked(df).View(t0).Buttons, ButtonPromo); b == nil || !b.Enabled {
		t.Fatalf("в запретный интервал «Ввести промокод» доступна: %+v", b)
	}
	// Блокировка за обход: только пароль родителя и выходы.
	if b := buttonOf(locked(bypassDecision()).View(t0).Buttons, ButtonPromo); b != nil {
		t.Fatalf("при обходе промокода быть не должно")
	}
}

// Ловит: панель промокода прячет текст точками, без подписи, или «Применить» отправляет пустой код.
func TestPromoPanelFlow(t *testing.T) {
	m := locked(timeUpDecision())
	if a := m.Press(t0, ButtonPromo, ""); a.Kind != ActNone {
		t.Fatalf("открытие панели не должно звать службу: %+v", a)
	}
	v := m.View(t0)
	if v.Panel != PanelPromo || !v.PasswordEnabled || !v.FieldPlain || v.FieldLabel != "Промокод" {
		t.Fatalf("панель промокода: Panel=%v enabled=%v plain=%v label=%q", v.Panel, v.PasswordEnabled, v.FieldPlain, v.FieldLabel)
	}
	if len(v.Choices) != 2 || v.Choices[0].Button != ButtonSubmitPromo || !v.Choices[0].Main || v.Choices[1].Button != ButtonCancel {
		t.Fatalf("кнопки панели: %+v", v.Choices)
	}
	if v.Choices[0].Label != "Применить" {
		t.Fatalf("главная кнопка панели %q, want «Применить»", v.Choices[0].Label)
	}
	// Пустой код — подсказка у поля, службе ничего.
	if a := m.Press(t0, ButtonSubmitPromo, "  "); a.Kind != ActNone {
		t.Fatalf("пустой код ушёл службе: %+v", a)
	}
	if v := m.View(t0); v.PasswordError != "Введи код" {
		t.Fatalf("ошибка пустого кода %q", v.PasswordError)
	}
	// Код уходит службе как есть (нормализует служба).
	a := m.Press(t0, ButtonSubmitPromo, " A1234 ")
	if a.Kind != ActRedeemPromo || a.Code != " A1234 " {
		t.Fatalf("действие %+v, want ActRedeemPromo с кодом", a)
	}
	// Поле панели пароля — с точками: признак не протекает.
	m2 := withPasswordPanel(t, timeUpDecision())
	if v := m2.View(t0); v.FieldPlain || v.FieldLabel != "Пароль родителя" {
		t.Fatalf("панель пароля: plain=%v label=%q", v.FieldPlain, v.FieldLabel)
	}
}

// Ловит: «Код не подходит» не показан, или после верного кода панель висит и плашки нет.
func TestPromoResult(t *testing.T) {
	m := locked(timeUpDecision())
	m.Press(t0, ButtonPromo, "")
	m.Press(t0, ButtonSubmitPromo, "nope")
	m.OnPromoResult(t0, PromoResult{Error: "Код не подходит"})
	v := m.View(t0)
	if v.Panel != PanelPromo || v.PasswordError != "Код не подходит" {
		t.Fatalf("после неверного кода Panel=%v err=%q", v.Panel, v.PasswordError)
	}
	m.Press(t0, ButtonSubmitPromo, "a1234")
	m.OnPromoResult(at(1), PromoResult{OK: true, Minutes: 30})
	if v := m.View(at(1)); v.Panel != PanelNone || v.PasswordError != "" {
		t.Fatalf("после верного кода панель не закрыта: %+v", v.Panel)
	}
	// Служба сняла экран: видна плашка «Добавлено 30 минут».
	m.OnDecision(at(2), runningDecision(1800), false, nil)
	v = m.View(at(2))
	if v.Warning == nil || v.Warning.Text != "Добавлено 30 минут" {
		t.Fatalf("плашка после промокода: %+v", v.Warning)
	}
}

// Ловит: «Отмена» оставляет набранный код в поле до следующего открытия.
func TestPromoCancelClearsField(t *testing.T) {
	m := locked(timeUpDecision())
	m.Press(t0, ButtonPromo, "")
	seq := m.View(t0).PasswordClearSeq
	m.Press(t0, ButtonCancel, "")
	v := m.View(t0)
	if v.Panel != PanelNone || v.PasswordClearSeq == seq {
		t.Fatalf("после «Отмена» Panel=%v clearSeq %d→%d", v.Panel, seq, v.PasswordClearSeq)
	}
}

// Ловит: имя кнопки для приёмки (TestGuard) не задано — сценарий не нажмёт «Ввести промокод».
func TestPromoTargets(t *testing.T) {
	for b, want := range map[Button]string{ButtonPromo: TargetPromo, ButtonSubmitPromo: TargetSubmitPromo} {
		if TargetName(b) != want {
			t.Fatalf("TargetName(%v) = %q, want %q", b, TargetName(b), want)
		}
		if got, ok := ButtonByTarget(want); !ok || got != b {
			t.Fatalf("ButtonByTarget(%q) = %v", want, got)
		}
	}
	if TargetPromo != "promo" || TargetSubmitPromo != "submitPromo" || FieldPromoID != "promo" {
		t.Fatalf("имена: %q %q %q", TargetPromo, TargetSubmitPromo, FieldPromoID)
	}
}

// Ловит: промокод с экрана блокировки не доходит до службы, отказ «Код не подходит» не виден
// у поля, обрыв связи молчит, верный код не даёт плашку (FR-35, R26).
func TestControllerRedeemPromo(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonPromo, "")
	if v := r.ctl.View(); v.Panel != PanelPromo {
		t.Fatalf("панель %v, ждали промокод", v.Panel)
	}
	// Отказ службы: текст у поля, панель на месте.
	r.svc.reply[ipc.CmdRedeemPromo] = func(m ipc.Msg) ipc.Reply { return ipc.FailField("Код не подходит", ipc.FieldCode) }
	act := r.ctl.Press(ButtonSubmitPromo, "nope")
	if act.Kind != ActRedeemPromo {
		t.Fatalf("действие %v, ждали ActRedeemPromo", act.Kind)
	}
	r.ctl.Run(act)
	var req ipc.RedeemPromoRequest
	calls := r.svc.called(ipc.CmdRedeemPromo)
	if len(calls) != 1 || calls[0].Into(&req) != nil || req.Code != "nope" || req.ChildID != 0 {
		t.Fatalf("вызов службы: %+v %+v", calls, req)
	}
	if v := r.ctl.View(); v.Panel != PanelPromo || v.PasswordError != "Код не подходит" {
		t.Fatalf("после отказа Panel=%v err=%q", v.Panel, v.PasswordError)
	}
	// Обрыв связи: сообщение под заголовком, панель на месте.
	r.svc.setDown(true)
	r.ctl.Run(r.ctl.Press(ButtonSubmitPromo, "a1234"))
	if got := r.ctl.Notice(); got == "" {
		t.Fatalf("обрыв связи без сообщения")
	}
	r.svc.setDown(false)
	// Верный код: панель закрыта, после снятия экрана — «Добавлено 30 минут».
	r.svc.reply[ipc.CmdRedeemPromo] = func(m ipc.Msg) ipc.Reply {
		return ipc.OK(ipc.RedeemPromoReply{AddedMin: 30, Message: "Добавлено 30 минут"})
	}
	r.ctl.Run(r.ctl.Press(ButtonSubmitPromo, "a1234"))
	if v := r.ctl.View(); v.Panel != PanelNone {
		t.Fatalf("после верного кода панель %v", v.Panel)
	}
	r.svc.status = ctRunningStatus(1800)
	if v := r.poll(); v.Warning == nil || v.Warning.Text != "Добавлено 30 минут" {
		t.Fatalf("плашка после промокода: %+v", v.Warning)
	}
}

// Ловит: во время запроса повторный «Применить» шлёт код второй раз; набранный код попадает в
// GuardState вне тестового режима или под чужим именем поля.
func TestControllerPromoBusyAndField(t *testing.T) {
	if !actsOnService(ButtonSubmitPromo) {
		t.Fatalf("«Применить» должна ждать ответа службы (без двойной отправки)")
	}
	st := StateFromView(View{LockShown: true, Panel: PanelPromo, Buttons: []ButtonView{}, Choices: []ButtonView{}},
		ipc.LockTimeUp, nil, nil, map[string]string{FieldPromoID: "a12", FieldPasswordID: "секрет"})
	if len(st.Fields) != 1 || st.Fields[0].ID != FieldPromoID || st.Fields[0].Text != "a12" {
		t.Fatalf("поля GuardState на панели промокода: %+v", st.Fields)
	}
}

// Ловит: в запретные часы верный промокод принят, но экран остаётся без единого слова — ребёнок
// думает, что код не сработал (ТЗ 8.2.3 п.3: из запретного интервала промокод не выводит).
func TestControllerPromoInForbiddenSaysWhenUsable(t *testing.T) {
	r := newCtRig()
	d := timeUpDecision()
	d.Lock, d.Status, d.Reason = core.LockForbidden, core.StatusForbidden, "Сейчас нельзя пользоваться компьютером"
	r.svc.status = ipc.ChildStatusFrom("Миша", d)
	r.poll()
	r.ctl.Press(ButtonPromo, "")
	r.svc.reply[ipc.CmdRedeemPromo] = func(m ipc.Msg) ipc.Reply {
		return ipc.OK(ipc.RedeemPromoReply{AddedMin: 20, Message: "Добавлено 20 минут"})
	}
	r.ctl.Run(r.ctl.Press(ButtonSubmitPromo, "a1"))
	if got := r.ctl.Notice(); got != "Добавлено 20 минут. Их можно будет потратить, когда закончатся запретные часы" {
		t.Fatalf("сообщение в запретные часы %q", got)
	}
}
