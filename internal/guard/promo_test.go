package guard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// FR-35, R26, ТЗ 8.2.3 п.3: «Ввести промокод» на экране блокировки. Ребёнок вводит код без
// пароля родителя; верный код — стикер и надпись удачи на одно проигрывание, затем экран
// снимается; неверный — стикер и надпись неудачи над полем (D-83).

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

// promoModel — экран «время вышло» с открытой панелью промокода и предсказуемым выбором стикеров
// (всегда первый из оставшихся).
func promoModel(t *testing.T, d core.Decision) *Model {
	t.Helper()
	m := locked(d)
	m.picker = newStickerPicker(seqRand(0))
	m.Press(t0, ButtonPromo, "")
	if v := m.View(t0); v.Panel != PanelPromo {
		t.Fatalf("панель промокода не открылась: %v", v.Panel)
	}
	return m
}

// Ловит: неверный код показан старым текстом службы у поля, а не стикером и надписью неудачи
// (R26, D-83, PL-49); код в поле стирается; второй неверный повторяет первый ответ; ответ не
// исчезает, когда ребёнок начал печатать новый код.
func TestPromoBadCodeSticker(t *testing.T) {
	m := promoModel(t, timeUpDecision())
	seq := m.View(t0).PasswordClearSeq
	if a := m.Press(t0, ButtonSubmitPromo, "nope"); a.Kind != ActRedeemPromo {
		t.Fatalf("действие %+v", a)
	}
	m.OnPromoResult(t0, PromoResult{Error: "Код не подходит", BadCode: true})
	v := m.View(t0)
	if v.Panel != PanelPromo || !v.PasswordEnabled || v.PasswordError != "" || v.PasswordClearSeq != seq {
		t.Fatalf("после неверного кода Panel=%v enabled=%v err=%q clear %d→%d", v.Panel, v.PasswordEnabled, v.PasswordError, seq, v.PasswordClearSeq)
	}
	if v.Promo == nil || v.Promo.OK || v.Promo.Sticker != "023" || v.Promo.Text != "Такого кода нет. Проверь буквы и цифры" {
		t.Fatalf("ответ на неверный код: %+v", v.Promo)
	}
	if v.Promo.Frame != 0 || v.Promo.Frames != 32 || !v.Promo.Started.Equal(t0) {
		t.Fatalf("кадры неудачи: %+v", v.Promo)
	}
	if f := m.View(at(time.Second)).Promo.Frame; f != 12 {
		t.Fatalf("через 1 с кадр %d, want 12", f)
	}
	// Ребёнок начал печатать новый код: ответ исчезает, панель остаётся.
	m.OnFieldEdit(at(2 * time.Second))
	if v := m.View(at(2 * time.Second)); v.Promo != nil || v.Panel != PanelPromo {
		t.Fatalf("после ввода ответ остался: %+v, Panel=%v", v.Promo, v.Panel)
	}
	m.Press(at(3*time.Second), ButtonSubmitPromo, "nope2")
	m.OnPromoResult(at(3*time.Second), PromoResult{Error: "Код не подходит", BadCode: true})
	v = m.View(at(3 * time.Second))
	if v.Promo == nil || v.Promo.Sticker != "028" || v.Promo.Text != "Хм, код не подходит. Посмотри внимательнее" || !v.Promo.Started.Equal(at(3*time.Second)) {
		t.Fatalf("второй неверный код: %+v", v.Promo)
	}
	// «Отмена» убирает ответ вместе с панелью.
	m.Press(at(4*time.Second), ButtonCancel, "")
	if v := m.View(at(4 * time.Second)); v.Promo != nil || v.Panel != PanelNone {
		t.Fatalf("после «Отмена» ответ %+v", v.Promo)
	}
}

// Ловит: пустой код даёт стикер вместо подсказки «Введи код» (уточнение 9); отказ службы не по
// полю кода (нет связи с базой) показан стикером неудачи, будто код неверный.
func TestPromoEmptyAndOtherErrors(t *testing.T) {
	m := promoModel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPromo, "nope")
	m.OnPromoResult(t0, PromoResult{Error: "Код не подходит", BadCode: true})
	m.Press(t0, ButtonSubmitPromo, "a1")
	m.OnPromoResult(t0, PromoResult{Error: "Не удалось сохранить. Попробуйте ещё раз"})
	if v := m.View(t0); v.Promo != nil || v.PasswordError != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Fatalf("отказ не по коду после неверного: promo=%+v err=%q", v.Promo, v.PasswordError)
	}
	m.Press(t0, ButtonSubmitPromo, "nope")
	m.OnPromoResult(t0, PromoResult{Error: "Код не подходит", BadCode: true})
	if a := m.Press(at(time.Second), ButtonSubmitPromo, "  "); a.Kind != ActNone {
		t.Fatalf("пустой код ушёл службе: %+v", a)
	}
	if v := m.View(at(time.Second)); v.Promo != nil || v.PasswordError != "Введи код" {
		t.Fatalf("пустой код: promo=%+v err=%q", v.Promo, v.PasswordError)
	}
	m.Press(at(2*time.Second), ButtonSubmitPromo, "a1")
	m.OnPromoResult(at(2*time.Second), PromoResult{Error: "Не удалось сохранить. Попробуйте ещё раз"})
	if v := m.View(at(2 * time.Second)); v.Promo != nil || v.PasswordError != "Не удалось сохранить. Попробуйте ещё раз" {
		t.Fatalf("отказ не по коду: promo=%+v err=%q", v.Promo, v.PasswordError)
	}
}

// Ловит: верный код снимает экран сразу (стикер удачи не виден) или держит его дольше одного
// проигрывания; после снятия всплывает плашка «Добавлено N минут» (надпись уже сказала, PL-49);
// панель с полем остаётся рядом со стикером.
func TestPromoSuccessHoldsScreen(t *testing.T) {
	d := timeUpDecision()
	d.Reason = "Время вышло"
	m := promoModel(t, d)
	m.Press(t0, ButtonSubmitPromo, "ab12")
	m.OnPromoResult(t0, PromoResult{OK: true, Minutes: 30})
	// Служба уже сняла блокировку (лимит вырос), а экран держится на время стикера.
	m.OnDecision(at(500*time.Millisecond), runningDecision(1800), false, nil)
	v := m.View(at(500 * time.Millisecond))
	if !v.LockShown || !v.SuppressKeys || v.Panel != PanelNone || v.PasswordEnabled {
		t.Fatalf("показ удачи: shown=%v keys=%v Panel=%v field=%v", v.LockShown, v.SuppressKeys, v.Panel, v.PasswordEnabled)
	}
	if v.Promo == nil || !v.Promo.OK || v.Promo.Sticker != "017" || v.Promo.Text != "Ура! +30 минут" || v.Promo.Frames != 33 {
		t.Fatalf("ответ на верный код: %+v", v.Promo)
	}
	if v.Lock.Reason != "Время вышло" || v.Lock.Until != "Можно снова: завтра" {
		t.Fatalf("тексты экрана во время показа: %+v", v.Lock)
	}
	// Во время показа — только «Пароль родителя» и выходы: время уже добавлено.
	got := []Button{}
	for _, b := range v.Buttons {
		got = append(got, b.Button)
	}
	if fmt.Sprint(got) != fmt.Sprint([]Button{ButtonParentPassword, ButtonShutdown, ButtonLogoff, ButtonSwitchUser}) {
		t.Fatalf("кнопки показа удачи %v", got)
	}
	// 33 кадра при 12 кадрах/с — 2,75 с: в 2,7 с экран ещё есть, на последнем кадре.
	if v := m.View(at(2700 * time.Millisecond)); !v.LockShown || v.Promo == nil || v.Promo.Frame != 32 {
		t.Fatalf("в 2,7 с: shown=%v promo=%+v", v.LockShown, v.Promo)
	}
	m.Tick(at(2750 * time.Millisecond))
	v = m.View(at(2750 * time.Millisecond))
	if v.LockShown || v.SuppressKeys || v.Promo != nil {
		t.Fatalf("после проигрывания экран остался: shown=%v promo=%+v", v.LockShown, v.Promo)
	}
	if v.Warning != nil {
		t.Fatalf("после промокода плашка %+v (надпись уже сказала)", v.Warning)
	}
	m.OnDecision(at(3*time.Second), runningDecision(1799), false, nil)
	if v := m.View(at(3 * time.Second)); v.LockShown || v.Warning != nil {
		t.Fatalf("после опроса: shown=%v plate=%+v", v.LockShown, v.Warning)
	}
}

// Ловит: опрос службы раньше конца показа обрывает его; показ, закончившийся раньше опроса,
// оставляет экран, а опрос со снятой блокировкой его не снимает.
func TestPromoSuccessHoldAndPollOrder(t *testing.T) {
	m := promoModel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPromo, "ab12")
	m.OnPromoResult(t0, PromoResult{OK: true, Minutes: 30})
	m.OnDecision(at(time.Second), runningDecision(1800), false, nil)
	if v := m.View(at(2 * time.Second)); !v.LockShown || v.Promo == nil {
		t.Fatalf("опрос оборвал показ: shown=%v promo=%+v", v.LockShown, v.Promo)
	}

	m = promoModel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPromo, "ab12")
	m.OnPromoResult(t0, PromoResult{OK: true, Minutes: 30})
	m.Tick(at(3 * time.Second)) // показ кончился, а служба ещё не ответила: решение «время вышло»
	if v := m.View(at(3 * time.Second)); !v.LockShown {
		t.Fatal("экран снят до ответа службы")
	}
	m.OnDecision(at(3500*time.Millisecond), runningDecision(1800), false, nil)
	if v := m.View(at(3500 * time.Millisecond)); v.LockShown || v.Promo != nil || v.Warning != nil {
		t.Fatalf("после опроса: shown=%v promo=%+v plate=%+v", v.LockShown, v.Promo, v.Warning)
	}
}

// Ловит: при выключенных анимациях Windows стикер движется (R26: показывается без движения).
func TestPromoStillWhenAnimationsOff(t *testing.T) {
	m := promoModel(t, timeUpDecision())
	m.SetAnimations(false)
	m.Press(t0, ButtonSubmitPromo, "nope")
	m.OnPromoResult(t0, PromoResult{Error: "Код не подходит", BadCode: true})
	if p := m.View(t0).Promo; p == nil || p.Frame != p.Frames-1 {
		t.Fatalf("без анимаций кадр %+v, ждали последний", p)
	}
}

// Ловит: во время показа удачи не работают пароль родителя, выход, выключение и смена
// пользователя (NFR-10, S3, S7, LE1, LE2); кнопки, которых нет, что-то делают.
func TestPromoSuccessHoldKeepsExits(t *testing.T) {
	hold := func(t *testing.T) *Model {
		m := promoModel(t, timeUpDecision())
		m.Press(t0, ButtonSubmitPromo, "ab12")
		m.OnPromoResult(t0, PromoResult{OK: true, Minutes: 30})
		m.OnDecision(at(100*time.Millisecond), runningDecision(1800), false, nil)
		return m
	}
	now := at(time.Second)
	m := hold(t)
	if a := m.Press(now, ButtonSwitchUser, ""); a.Kind != ActSwitchUser {
		t.Fatalf("смена пользователя: %+v", a)
	}
	m = hold(t)
	m.Press(now, ButtonShutdown, "")
	if a := m.Press(now, ButtonConfirmShutdown, ""); a.Kind != ActShutdown {
		t.Fatalf("выключение: %+v", a)
	}
	m = hold(t)
	m.Press(now, ButtonLogoff, "")
	if a := m.Press(now, ButtonConfirmLogoff, ""); a.Kind != ActLogoff {
		t.Fatalf("выход: %+v", a)
	}
	m = hold(t)
	m.Press(now, ButtonParentPassword, "")
	if v := m.View(now); v.Panel != PanelPassword || !v.PasswordEnabled || v.Promo != nil {
		t.Fatalf("пароль родителя во время показа: Panel=%v promo=%+v", v.Panel, v.Promo)
	}
	if a := m.Press(now, ButtonSubmitPassword, "секрет"); a.Kind != ActCheckPassword {
		t.Fatalf("пароль не ушёл: %+v", a)
	}
	m.OnPasswordResult(now, PasswordResult{OK: true})
	if v := m.View(now); v.Panel != PanelParentChoices {
		t.Fatalf("после верного пароля Panel=%v", v.Panel)
	}
	m = hold(t)
	for _, b := range []Button{ButtonAskMore, ButtonSpendBonus, ButtonPromo} {
		if a := m.Press(now, b, ""); a.Kind != ActNone || m.View(now).Panel != PanelNone {
			t.Fatalf("кнопка %v во время показа: %+v", b, a)
		}
	}
	// Связи нет 10 с: экран снимается и во время показа (аварийный выход, Ruling 15).
	m = hold(t)
	if v := m.View(t0.Add(offlineAfter + 200*time.Millisecond)); v.LockShown {
		t.Fatal("без связи экран показа удачи держится")
	}
	// Родительская сессия: экран снимается сразу.
	m = hold(t)
	m.OnDecision(at(time.Second), runningDecision(1800), true, nil)
	if v := m.View(at(time.Second)); v.LockShown {
		t.Fatal("в сессии родителя экран показа удачи держится")
	}
}

// Ловит: в запретный интервал верный код снимает экран или не показывает стикер удачи (8.2.3 п.3:
// время добавлено, экран остаётся до конца интервала).
func TestPromoSuccessInForbidden(t *testing.T) {
	d := timeUpDecision()
	d.Lock, d.Status = core.LockForbidden, core.StatusForbidden
	m := promoModel(t, d)
	m.Press(t0, ButtonSubmitPromo, "ab12")
	m.OnPromoResult(t0, PromoResult{OK: true, Minutes: 20})
	m.OnDecision(at(time.Second), d, false, nil)
	if v := m.View(at(time.Second)); !v.LockShown || v.Promo == nil || !v.Promo.OK || v.Promo.Text != "Ура! +20 минут" {
		t.Fatalf("показ удачи в запретный интервал: %+v", v.Promo)
	}
	// После проигрывания экран остаётся, стикер стоит на последнем кадре, кнопки обычные.
	m.Tick(at(5 * time.Second))
	m.OnDecision(at(5*time.Second), d, false, nil)
	v := m.View(at(5 * time.Second))
	if !v.LockShown || v.Promo == nil || v.Promo.Frame != v.Promo.Frames-1 || buttonOf(v.Buttons, ButtonPromo) == nil {
		t.Fatalf("после показа: shown=%v promo=%+v buttons=%+v", v.LockShown, v.Promo, v.Buttons)
	}
	// Новое нажатие убирает ответ: после «Пароль родителя» и «Отмена» стикера нет.
	m.Press(at(6*time.Second), ButtonParentPassword, "")
	m.Press(at(6*time.Second), ButtonCancel, "")
	if v := m.View(at(6 * time.Second)); v.Promo != nil || v.Panel != PanelNone {
		t.Fatalf("после «Отмена» ответ %+v", v.Promo)
	}
	m.Press(at(6*time.Second), ButtonPromo, "")
	if v := m.View(at(6 * time.Second)); v.Promo != nil || v.Panel != PanelPromo {
		t.Fatalf("после нового нажатия ответ %+v", v.Promo)
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

// Ловит: промокод с экрана блокировки не доходит до службы; отказ службы по полю кода не даёт
// стикер неудачи (RM5b-10); обрыв связи молчит; верный код не держит экран на время стикера или
// после снятия показывает плашку «Добавлено» (FR-35, R26, D-83).
func TestControllerRedeemPromo(t *testing.T) {
	r := newCtRig()
	warmed := []string{}
	r.ctl.SetStickerWarmer(func(n string) { warmed = append(warmed, n) })
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonPromo, "")
	if v := r.ctl.View(); v.Panel != PanelPromo {
		t.Fatalf("панель %v, ждали промокод", v.Panel)
	}
	// Отказ службы по полю кода: стикер неудачи над полем, панель на месте.
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
	v := r.ctl.View()
	if v.Panel != PanelPromo || v.PasswordError != "" || v.Promo == nil || v.Promo.OK || !contains(stickerFail, v.Promo.Sticker) || !contains(stickerFailTexts, v.Promo.Text) {
		t.Fatalf("после отказа Panel=%v err=%q promo=%+v", v.Panel, v.PasswordError, v.Promo)
	}
	first := *v.Promo
	if len(warmed) != 1 || warmed[0] != first.Sticker {
		t.Fatalf("кадры стикера не подготовлены заранее: %v", warmed)
	}
	// Ребёнок печатает: ответ исчезает.
	r.ctl.FieldEdited()
	if v := r.ctl.View(); v.Promo != nil {
		t.Fatalf("после ввода ответ остался: %+v", v.Promo)
	}
	r.ctl.Run(r.ctl.Press(ButtonSubmitPromo, "nope"))
	if v := r.ctl.View(); v.Promo == nil || v.Promo.Sticker == first.Sticker || v.Promo.Text == first.Text {
		t.Fatalf("второй неверный повторил первый: %+v после %+v", v.Promo, first)
	}
	// Обрыв связи: сообщение под заголовком, панель на месте.
	r.svc.setDown(true)
	r.ctl.Run(r.ctl.Press(ButtonSubmitPromo, "a1234"))
	if got := r.ctl.Notice(); got == "" {
		t.Fatalf("обрыв связи без сообщения")
	}
	r.svc.setDown(false)
	// Верный код: стикер удачи, экран держится одно проигрывание, плашки «Добавлено» нет.
	r.svc.reply[ipc.CmdRedeemPromo] = func(m ipc.Msg) ipc.Reply {
		return ipc.OK(ipc.RedeemPromoReply{AddedMin: 30, Message: "Добавлено 30 минут"})
	}
	r.ctl.Run(r.ctl.Press(ButtonSubmitPromo, "a1234"))
	r.svc.status = ctRunningStatus(1800)
	v = r.poll()
	if !v.LockShown || v.Panel != PanelNone || v.Promo == nil || !v.Promo.OK || !strings.Contains(v.Promo.Text, "+30 минут") {
		t.Fatalf("после верного кода: shown=%v Panel=%v promo=%+v", v.LockShown, v.Panel, v.Promo)
	}
	st := StateFromView(v, ipc.LockNone, nil, nil, nil)
	if !st.LockShown || st.PromoSticker != v.Promo.Sticker || st.PromoText != v.Promo.Text {
		t.Fatalf("GuardState показа: %+v", st)
	}
	r.clk.Advance(3 * time.Second)
	if v := r.poll(); v.LockShown || v.Warning != nil {
		t.Fatalf("после показа: shown=%v plate=%+v", v.LockShown, v.Warning)
	}
	if st := StateFromView(r.ctl.View(), ipc.LockNone, nil, nil, nil); st.PromoSticker != "" || st.PromoText != "" {
		t.Fatalf("GuardState без показа: %+v", st)
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

// Ловит: каждый новый кадр стикера перестраивает экран (кнопки, фокус, мерцание) вместо
// перерисовки одного стикера; новый ответ на промокод экран не перестраивает.
func TestLayoutKeyIgnoresStickerFrame(t *testing.T) {
	m := promoModel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPromo, "nope")
	m.OnPromoResult(t0, PromoResult{Error: "Код не подходит", BadCode: true})
	a, b := m.View(t0), m.View(at(time.Second))
	if LayoutKey(a, "") != LayoutKey(b, "") {
		t.Fatal("смена кадра стикера перестраивает окно")
	}
	if !StickerChanged(a, b) || StickerChanged(b, b) {
		t.Fatal("смена кадра не замечена или замечена без смены")
	}
	if StickerAnimating(m.View(at(5*time.Second))) || !StickerAnimating(a) || StickerAnimating(View{}) {
		t.Fatal("признак анимации: идёт на последнем кадре или не идёт на первом")
	}
	m.OnFieldEdit(at(2 * time.Second))
	m.Press(at(2*time.Second), ButtonSubmitPromo, "nope")
	m.OnPromoResult(at(2*time.Second), PromoResult{Error: "Код не подходит", BadCode: true})
	c := m.View(at(2 * time.Second))
	if LayoutKey(b, "") == LayoutKey(c, "") {
		t.Fatal("новый ответ не перестраивает окно")
	}
	if !strings.Contains(LayoutKey(c, ""), c.Promo.Text) {
		t.Fatal("в подписи нет надписи ответа")
	}
}

// Ловит: ответ «код принят» без показанного экрана блокировки (опоздавший ответ после снятия
// экрана и перезапуска стража) рисует пустой экран блокировки на время стикера.
func TestPromoSuccessWithoutLockScreen(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(1800), false, nil)
	m.OnPromoResult(t0, PromoResult{OK: true, Minutes: 30})
	if v := m.View(at(time.Second)); v.LockShown || v.Promo != nil || v.Warning != nil {
		t.Fatalf("без экрана: shown=%v promo=%+v plate=%+v", v.LockShown, v.Promo, v.Warning)
	}
}
