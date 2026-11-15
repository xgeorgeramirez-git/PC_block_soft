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
// исчезает, когда ребёнок начал печатать новый код (D-85: остаётся до верного кода), или стоит
// после первого круга (D-84: движется по кругу).
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
	if v.Promo.Frame != 0 || v.Promo.Frames != 79 || !v.Promo.Started.Equal(t0) || v.Promo.Still {
		t.Fatalf("кадры неудачи: %+v", v.Promo)
	}
	if f := m.View(at(time.Second)).Promo.Frame; f != 30 {
		t.Fatalf("через 1 с кадр %d, want 30", f)
	}
	// Ребёнок печатает новый код: стикер и надпись остаются (D-85), показ не начинается заново.
	m.OnFieldEdit(at(2 * time.Second))
	v = m.View(at(2 * time.Second))
	if v.Promo == nil || v.Promo.Sticker != "023" || v.Promo.Text != "Такого кода нет. Проверь буквы и цифры" || !v.Promo.Started.Equal(t0) || v.Panel != PanelPromo {
		t.Fatalf("после ввода в поле: %+v, Panel=%v", v.Promo, v.Panel)
	}
	// 79 кадров при 30 кадрах/с — круг 2,633 с: в 3 с идёт второй круг (кадр 90-79 = 11), в 8 с — четвёртый.
	if f := m.View(at(3 * time.Second)).Promo.Frame; f != 11 {
		t.Fatalf("в 3 с кадр %d, want 11 (второй круг)", f)
	}
	if f := m.View(at(8 * time.Second)).Promo.Frame; f != 3 {
		t.Fatalf("в 8 с кадр %d, want 3 (240 по модулю 79)", f)
	}
	// «Применить» старый ответ не убирает: он стоит, пока служба не ответила.
	m.Press(at(9*time.Second), ButtonSubmitPromo, "nope2")
	if v := m.View(at(9 * time.Second)); v.Promo == nil || v.Promo.Sticker != "023" {
		t.Fatalf("после «Применить» до ответа: %+v", v.Promo)
	}
	m.OnPromoResult(at(9*time.Second), PromoResult{Error: "Код не подходит", BadCode: true})
	v = m.View(at(9 * time.Second))
	if v.Promo == nil || v.Promo.Sticker != "028" || v.Promo.Text != "Хм, код не подходит. Посмотри внимательнее" || !v.Promo.Started.Equal(at(9*time.Second)) || v.Promo.Frame != 0 {
		t.Fatalf("второй неверный код: %+v", v.Promo)
	}
	// «Отмена» убирает ответ вместе с панелью.
	m.Press(at(10*time.Second), ButtonCancel, "")
	if v := m.View(at(10 * time.Second)); v.Promo != nil || v.Panel != PanelNone {
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
	if v.Promo == nil || !v.Promo.OK || v.Promo.Sticker != "017" || v.Promo.Text != "Ура! +30 минут" || v.Promo.Frames != 81 {
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
	// 81 кадр при 30 кадрах/с — круг 2,7 с: в 2,69 с экран ещё есть, на последнем кадре.
	if v := m.View(at(2690 * time.Millisecond)); !v.LockShown || v.Promo == nil || v.Promo.Frame != 80 {
		t.Fatalf("в 2,69 с: shown=%v promo=%+v", v.LockShown, v.Promo)
	}
	// Ровно один круг: в 2,7 с экрана уже нет (второго круга удачи нет).
	m.Tick(at(2700 * time.Millisecond))
	v = m.View(at(2700 * time.Millisecond))
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
	for _, d := range []time.Duration{0, time.Second, 7 * time.Second} {
		v := m.View(at(d))
		if p := v.Promo; p == nil || p.Frame != 78 || !p.Still || StickerFrame(p, at(d)) != 78 {
			t.Fatalf("без анимаций в %v кадр %+v, ждали последний (78) без движения", d, p)
		}
		if StickerAnimating(v) {
			t.Fatal("без анимаций окну назначен таймер кадров")
		}
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
	// После первого круга экран остаётся, стикер удачи идёт по кругу (81 кадр: в 5 с кадр 150-81 = 69),
	// кнопки обычные.
	m.Tick(at(5 * time.Second))
	m.OnDecision(at(5*time.Second), d, false, nil)
	v := m.View(at(5 * time.Second))
	if !v.LockShown || v.Promo == nil || v.Promo.Frame != 69 || !StickerAnimating(v) || buttonOf(v.Buttons, ButtonPromo) == nil {
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
	// Ребёнок печатает: ответ остаётся (D-85).
	r.ctl.FieldEdited()
	if v := r.ctl.View(); v.Promo == nil || v.Promo.Sticker != first.Sticker || v.Promo.Text != first.Text {
		t.Fatalf("после ввода ответ пропал или сменился: %+v", v.Promo)
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
	if a.Promo.Frame == b.Promo.Frame {
		t.Fatal("за секунду кадр не сменился")
	}
	// Стикер идёт по кругу: таймер кадров нужен и после первого круга; без ответа — не нужен.
	if !StickerAnimating(m.View(at(5*time.Second))) || !StickerAnimating(a) || StickerAnimating(View{}) {
		t.Fatal("признак анимации: не идёт после первого круга, не идёт на первом кадре или идёт без стикера")
	}
	if StickerAnimating(View{Promo: &PromoStickerView{Sticker: "017", Frames: 1}}) {
		t.Fatal("стикеру из одного кадра назначен шаг кадров")
	}
	m.OnFieldEdit(at(2 * time.Second))
	if LayoutKey(b, "") != LayoutKey(m.View(at(2*time.Second)), "") {
		t.Fatal("ввод в поле перестраивает окно, хотя ответ тот же")
	}
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

// Ловит: опрос стража получил «без блокировки» раньше ответа на промокод — экран спрятан, а ответ
// возвращает его со старым текстом и перехватом клавиш на время стикера (мигание, ревью T9).
func TestPromoSuccessAfterLockLifted(t *testing.T) {
	m := promoModel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPromo, "ab12")
	m.OnDecision(at(200*time.Millisecond), runningDecision(1800), false, nil)
	if v := m.View(at(200 * time.Millisecond)); v.LockShown {
		t.Fatal("экран не снят решением без блокировки")
	}
	m.OnPromoResult(at(300*time.Millisecond), PromoResult{OK: true, Minutes: 30})
	if v := m.View(at(400 * time.Millisecond)); v.LockShown || v.SuppressKeys || v.Promo != nil || v.Warning != nil {
		t.Fatalf("ответ вернул экран: shown=%v keys=%v promo=%+v plate=%+v", v.LockShown, v.SuppressKeys, v.Promo, v.Warning)
	}
}

// Ловит: открытая панель промокода висит без нажатий дольше 60 с или закрывается раньше (D-85); ввод
// в поле и кнопки панели отсчёт не начинают заново; закрытие оставляет стикер, надпись, ошибку поля
// или набранный код; без Tick вид этого не показывает.
func TestPromoPanelClosesAfterIdle(t *testing.T) {
	sec := func(n int) time.Time { return at(time.Duration(n) * time.Second) }
	d := timeUpDecision()
	var m *Model
	// view — вид в секунду n после свежего ответа службы (как при опросе раз в секунду; иначе обрыв связи).
	// Ответ приходит за миллисекунду до вида: сам вид должен показать закрытие без Tick.
	last := 0
	view := func(n int) View {
		for s := last + 5; s < n; s += 5 {
			m.OnDecision(sec(s), d, false, nil)
		}
		last = n
		m.OnDecision(sec(n).Add(-time.Millisecond), d, false, nil)
		return m.View(sec(n))
	}
	if promoIdleClose != 60*time.Second {
		t.Fatalf("promoIdleClose = %v, want 60 с", promoIdleClose)
	}
	// Панель просто открыта, стикера нет: 59 с открыта, в 60 с закрыта.
	m = promoModel(t, d)
	seq := m.View(t0).PasswordClearSeq
	if v := view(59); v.Panel != PanelPromo || !v.PasswordEnabled {
		t.Fatalf("в 59 с панель закрыта: %v", v.Panel)
	}
	v := view(60) // вид без Tick: чистая функция времени
	if v.Panel != PanelNone || v.PasswordEnabled || v.Promo != nil || !v.LockShown || buttonOf(v.Buttons, ButtonPromo) == nil || len(v.Choices) != 0 {
		t.Fatalf("в 60 с: Panel=%v field=%v promo=%+v buttons=%+v", v.Panel, v.PasswordEnabled, v.Promo, v.Buttons)
	}
	m.Tick(sec(60))
	if v := view(60); v.Panel != PanelNone || v.PasswordClearSeq != seq+1 {
		t.Fatalf("после закрытия: Panel=%v clear %d→%d (поле не очищено)", v.Panel, seq, v.PasswordClearSeq)
	}
	// Закрытая панель «Применить» не принимает; открыть снова — отсчёт с нуля.
	if a := m.Press(sec(61), ButtonSubmitPromo, "ab12"); a.Kind != ActNone {
		t.Fatalf("закрытая панель отправила код: %+v", a)
	}
	view(99)
	m.Press(sec(100), ButtonPromo, "")
	if v := view(159); v.Panel != PanelPromo {
		t.Fatal("после нового открытия панель закрылась раньше 60 с")
	}
	if v := view(160); v.Panel != PanelNone {
		t.Fatal("после нового открытия панель не закрылась через 60 с")
	}

	// Стикер неудачи: ввод в поле в 50 с начинает отсчёт заново — в 109 с открыта, в 110 с закрыта.
	m, last = promoModel(t, d), 0
	m.Press(t0, ButtonSubmitPromo, "nope")
	m.OnPromoResult(t0, PromoResult{Error: "Код не подходит", BadCode: true})
	view(49)
	m.OnFieldEdit(sec(50))
	if v := view(109); v.Panel != PanelPromo || v.Promo == nil || v.Promo.Sticker != "023" {
		t.Fatalf("в 109 с (ввод был в 50 с): Panel=%v promo=%+v", v.Panel, v.Promo)
	}
	m.Tick(sec(110))
	if v := view(110); v.Panel != PanelNone || v.Promo != nil || v.PasswordError != "" || StickerAnimating(v) {
		t.Fatalf("в 110 с: Panel=%v promo=%+v err=%q", v.Panel, v.Promo, v.PasswordError)
	}
	// Открыть снова: прошлого ответа нет.
	m.Press(sec(111), ButtonPromo, "")
	if v := view(111); v.Panel != PanelPromo || v.Promo != nil || v.PasswordError != "" {
		t.Fatalf("после нового открытия: promo=%+v err=%q", v.Promo, v.PasswordError)
	}

	// «Применить» (и пустое) — тоже нажатие: отсчёт заново. Ответ службы — не нажатие.
	m, last = promoModel(t, d), 0
	view(39)
	m.Press(sec(40), ButtonSubmitPromo, "  ")
	if v := view(99); v.Panel != PanelPromo || v.PasswordError != "Введи код" {
		t.Fatalf("в 99 с (нажатие было в 40 с): Panel=%v err=%q", v.Panel, v.PasswordError)
	}
	m.Press(sec(99), ButtonSubmitPromo, "nope")
	view(119)
	m.OnPromoResult(sec(120), PromoResult{Error: "Код не подходит", BadCode: true})
	if v := view(158); v.Panel != PanelPromo || v.Promo == nil {
		t.Fatalf("в 158 с (нажатие было в 99 с): Panel=%v promo=%+v", v.Panel, v.Promo)
	}
	if v := view(159); v.Panel != PanelNone || v.Promo != nil {
		t.Fatalf("в 159 с: Panel=%v promo=%+v", v.Panel, v.Promo)
	}
	// Ответ службы на закрывшуюся панель её не возвращает.
	m.Tick(sec(159))
	m.OnPromoResult(sec(160), PromoResult{Error: "Код не подходит", BadCode: true})
	if v := view(160); v.Panel != PanelNone || v.Promo != nil {
		t.Fatalf("опоздавший отказ вернул панель: Panel=%v promo=%+v", v.Panel, v.Promo)
	}

	// Панель пароля сама не закрывается (срок только у промокода).
	m, last = locked(d), 0
	m.Press(t0, ButtonParentPassword, "")
	if v := view(120); v.Panel != PanelPassword {
		t.Fatalf("панель пароля закрылась: %v", v.Panel)
	}
	// Ввод в поле пароля отсчёт промокода не заводит.
	m.OnFieldEdit(sec(121))
	m.Press(sec(122), ButtonCancel, "")
	m.Press(sec(123), ButtonPromo, "")
	if v := view(182); v.Panel != PanelPromo {
		t.Fatal("панель промокода закрылась по старой отметке")
	}
}

// Ловит: стикер удачи в запретный интервал крутится часами (таймер кадров и до 32 МБ кадров не
// освобождаются): без нажатий 60 с ответ убирается, экран остаётся (Ruling 0.6.1).
func TestPromoSuccessInForbiddenIdle(t *testing.T) {
	d := timeUpDecision()
	d.Lock, d.Status = core.LockForbidden, core.StatusForbidden
	m := promoModel(t, d)
	m.Press(t0, ButtonSubmitPromo, "ab12")
	m.OnPromoResult(at(time.Second), PromoResult{OK: true, Minutes: 20})
	for s := 5; s < 60; s += 5 {
		m.OnDecision(at(time.Duration(s)*time.Second), d, false, nil)
		if v := m.View(at(time.Duration(s) * time.Second)); v.Promo == nil || !StickerAnimating(v) {
			t.Fatalf("в %d с стикер удачи не идёт: %+v", s, v.Promo)
		}
	}
	if v := m.View(at(60 * time.Second)); !v.LockShown || v.Promo == nil || !v.Promo.OK {
		t.Fatalf("в 60 с (ответ был в 1 с): %+v", v.Promo)
	}
	v := m.View(at(61 * time.Second))
	if !v.LockShown || v.Promo != nil || v.Panel != PanelNone || buttonOf(v.Buttons, ButtonPromo) == nil {
		t.Fatalf("в 61 с: shown=%v promo=%+v", v.LockShown, v.Promo)
	}
	m.Tick(at(61 * time.Second))
	m.OnDecision(at(62*time.Second), d, false, nil)
	if v := m.View(at(62 * time.Second)); !v.LockShown || v.Promo != nil {
		t.Fatalf("после закрытия: shown=%v promo=%+v", v.LockShown, v.Promo)
	}
}

// Ловит: показ стикера считается от ответа службы, а кадры разбираются ещё долю секунды — первые
// кадры пропадают, а круг удачи обрывается раньше конца; повторная готовность сдвигает показ снова.
func TestPromoReadyRestartsShow(t *testing.T) {
	m := promoModel(t, timeUpDecision())
	m.PromoReady(t0) // ответа нет: ничего
	m.Press(t0, ButtonSubmitPromo, "ab12")
	m.OnPromoResult(t0, PromoResult{OK: true, Minutes: 30})
	m.PromoReady(at(400 * time.Millisecond))
	v := m.View(at(400 * time.Millisecond))
	if v.Promo == nil || !v.Promo.Started.Equal(at(400*time.Millisecond)) || v.Promo.Frame != 0 {
		t.Fatalf("после готовности кадров: %+v", v.Promo)
	}
	m.PromoReady(at(900 * time.Millisecond)) // второй раз не сдвигает
	// Круг 2,7 с от готовности: в 3,09 с экран ещё есть, в 3,1 с снят.
	m.OnDecision(at(time.Second), runningDecision(1800), false, nil)
	if v := m.View(at(3090 * time.Millisecond)); !v.LockShown || v.Promo == nil || v.Promo.Frame != 80 {
		t.Fatalf("в 3,09 с: shown=%v promo=%+v", v.LockShown, v.Promo)
	}
	if v := m.View(at(3100 * time.Millisecond)); v.LockShown {
		t.Fatal("в 3,1 с экран остался")
	}
}

// Ловит: окно считает кадр не по кругу или не от начала показа; конец круга удачи не замечен (экран
// исчезает с опозданием до тика 250 мс, стикер успевает начать второй круг).
func TestStickerFrameAndRoundEnd(t *testing.T) {
	p := &PromoStickerView{Sticker: "017", OK: true, Started: t0, Frames: 81}
	for ms, want := range map[int]int{0: 0, 33: 0, 34: 1, 1000: 30, 2699: 80, 2700: 0, 2734: 1, 5400: 0, 6000: 18} {
		if got := StickerFrame(p, at(msDur(ms))); got != want {
			t.Fatalf("StickerFrame(%d мс) = %d, want %d", ms, got, want)
		}
	}
	if StickerFrame(nil, t0) != 0 {
		t.Fatal("StickerFrame(nil) != 0")
	}
	if StickerRoundEnded(p, at(2699*time.Millisecond)) || !StickerRoundEnded(p, at(2700*time.Millisecond)) {
		t.Fatal("конец круга удачи: не в 2,7 с")
	}
	fail := &PromoStickerView{Sticker: "023", Started: t0, Frames: 79}
	if StickerRoundEnded(fail, at(time.Hour)) || StickerRoundEnded(nil, t0) {
		t.Fatal("у неудачи конца круга нет: она идёт, пока не введён верный код")
	}
}

// Ловит: кадры стикера разбираются уже после начала показа (первые кадры и конец круга удачи
// пропадают): показ начинается, когда кадры готовы; нажатие «Применить», пока служба отвечает на
// прошлое, не считается нажатием для срока без нажатий (панель закрывается под рукой).
func TestControllerPromoReadyAndBusyTouch(t *testing.T) {
	r := newCtRig()
	r.ctl.SetStickerWarmer(func(string) { r.clk.Advance(400 * time.Millisecond) }) // разбор кадров
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.ctl.Press(ButtonPromo, "")
	r.svc.reply[ipc.CmdRedeemPromo] = func(m ipc.Msg) ipc.Reply { return ipc.FailField("Код не подходит", ipc.FieldCode) }
	r.ctl.Run(r.ctl.Press(ButtonSubmitPromo, "nope"))
	v := r.ctl.View()
	if v.Promo == nil || !v.Promo.Started.Equal(t0.Add(400*time.Millisecond)) || v.Promo.Frame != 0 {
		t.Fatalf("показ начат не с готовности кадров: %+v", v.Promo)
	}
	// wait — n секунд с опросом раз в секунду (связь есть).
	wait := func(n int) View {
		for i := 0; i < n; i++ {
			r.clk.Advance(time.Second)
			r.poll()
		}
		return r.ctl.View()
	}
	act := r.ctl.Press(ButtonSubmitPromo, "nope") // запрос ушёл, ответа пока нет
	wait(50)
	if a := r.ctl.Press(ButtonSubmitPromo, "nope"); a.Kind != ActNone {
		t.Fatalf("второй «Применить» во время запроса: %+v", a)
	}
	if v := wait(59); v.Panel != PanelPromo {
		t.Fatal("нажатие во время запроса не начало срок заново: панель закрылась")
	}
	if v := wait(1); v.Panel != PanelNone || v.Promo != nil {
		t.Fatalf("через 60 с после нажатия: Panel=%v promo=%+v", v.Panel, v.Promo)
	}
	r.ctl.Run(act)
	if v := r.ctl.View(); v.Panel != PanelNone || v.Promo != nil {
		t.Fatalf("опоздавший ответ вернул панель: Panel=%v promo=%+v", v.Panel, v.Promo)
	}
}
