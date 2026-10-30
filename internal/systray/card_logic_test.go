package systray

import (
	"errors"
	"image"
	"image/color"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Ловит: клавиши табло делают не то: Esc не закрывает, Tab и стрелки не ходят по кругу или в
// другую сторону, Shift спрашивается не для Tab, Enter нажимает не ту кнопку, чужая клавиша
// перехватывается (тогда она не дойдёт до окна).
func TestCardKey(t *testing.T) {
	noShift := func() bool { return false }
	shiftAsked := 0
	withShift := func() bool { shiftAsked++; return true }
	errFocus := CardState{Focus: TargetRefresh}

	cases := []struct {
		name  string
		vk    uintptr
		kind  CountdownKind
		st    CardState
		shift func() bool
		focus CardTarget
		act   cardKeyAct
	}{
		{"Esc закрывает", 0x1B, CountdownError, errFocus, noShift, TargetRefresh, keyHide},
		{"Tab вперёд", 0x09, CountdownError, errFocus, noShift, TargetArrow, keyMove},
		// У табло две кнопки: с кнопки вперёд и назад ведут в одно место, поэтому направление
		// проверяется без фокуса (вперёд: первая, назад: последняя).
		{"Tab без фокуса вперёд", 0x09, CountdownError, CardState{}, noShift, TargetRefresh, keyMove},
		{"Shift+Tab назад", 0x09, CountdownError, CardState{}, withShift, TargetArrow, keyMove},
		{"Shift+Tab по кругу", 0x09, CountdownError, CardState{Focus: TargetArrow}, withShift, TargetRefresh, keyMove},
		{"вправо вперёд", 0x27, CountdownError, CardState{}, noShift, TargetRefresh, keyMove},
		{"вниз вперёд", 0x28, CountdownError, CardState{}, noShift, TargetRefresh, keyMove},
		{"влево назад", 0x25, CountdownError, CardState{}, noShift, TargetArrow, keyMove},
		{"вверх назад", 0x26, CountdownError, CardState{}, noShift, TargetArrow, keyMove},
		{"вверх с кнопки", 0x26, CountdownError, errFocus, noShift, TargetArrow, keyMove},
		{"Enter жмёт фокус", 0x0D, CountdownError, errFocus, noShift, TargetRefresh, keyPress},
		{"пробел жмёт фокус", 0x20, CountdownError, CardState{Focus: TargetArrow}, noShift, TargetArrow, keyPress},
		{"Enter без фокуса жмёт первую", 0x0D, CountdownError, CardState{}, noShift, TargetRefresh, keyPress},
		{"Enter на цифрах: фокус «Обновить» не из списка", 0x0D, CountdownTime, errFocus, noShift, TargetArrow, keyPress},
		{"буква не табло", 0x41, CountdownError, errFocus, noShift, TargetRefresh, keyIgnored},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, act := cardKey(c.vk, c.kind, c.st, c.shift)
			if act != c.act || st.Focus != c.focus {
				t.Fatalf("действие %v, фокус %v; ждали %v, %v", act, st.Focus, c.act, c.focus)
			}
			if !st.FocusVisible {
				t.Fatal("после клавиши кольцо фокуса видно")
			}
		})
	}
	before := shiftAsked
	cardKey(0x25, CountdownError, errFocus, withShift)
	cardKey(0x0D, CountdownError, errFocus, withShift)
	if shiftAsked != before {
		t.Fatal("Shift спрашивается только для Tab")
	}
}

// Ловит: щелчок срабатывает без нажатия над кнопкой, при отпускании мимо неё или над другой
// кнопкой; нажатие не снимается после отпускания; нажатие мимо кнопок захватывает мышь.
func TestCardMouseDownUp(t *testing.T) {
	st, capture := cardDown(CardState{Hover: TargetArrow}, TargetNone)
	if capture || st.Pressed != TargetNone {
		t.Fatalf("мимо кнопок: захват %v, нажата %v", capture, st.Pressed)
	}
	st, capture = cardDown(CardState{Hover: TargetArrow}, TargetArrow)
	if !capture || st.Pressed != TargetArrow || st.Hover != TargetArrow {
		t.Fatalf("над стрелкой: захват %v, %+v", capture, st)
	}

	up, fire := cardUp(st, TargetArrow)
	if fire != TargetArrow || up.Pressed != TargetNone {
		t.Fatalf("отпущена над той же: %v, %+v", fire, up)
	}
	up, fire = cardUp(st, TargetNone)
	if fire != TargetNone || up.Pressed != TargetNone {
		t.Fatalf("отпущена мимо: %v, %+v", fire, up)
	}
	up, fire = cardUp(CardState{Pressed: TargetRefresh}, TargetArrow)
	if fire != TargetNone || up.Pressed != TargetNone {
		t.Fatalf("отпущена над другой: %v, %+v", fire, up)
	}
	if _, fire = cardUp(CardState{}, TargetNone); fire != TargetNone {
		t.Fatal("без нажатия щелчка нет")
	}
}

// Ловит: координаты мыши теряют знак (за левым и верхним краем окна) или путают x и y;
// тестовый щелчок по крестику упаковывает точку не так, как её читает окно.
func TestLParamPoint(t *testing.T) {
	if x, y := pointFromLParam(0x0028_0014); x != 20 || y != 40 {
		t.Fatalf("точка %d,%d, ждали 20,40", x, y)
	}
	if x, y := pointFromLParam(0xFFFE_FFFF); x != -1 || y != -2 {
		t.Fatalf("за краем %d,%d, ждали -1,-2", x, y)
	}
	if lp := lParamFromPoint(20, 40); lp != 0x0028_0014 {
		t.Fatalf("упаковка %#x, ждали 0x280014", lp)
	}
	if lp := lParamFromPoint(-1, 3); lp != 0x0003_FFFF {
		t.Fatalf("упаковка отрицательной %#x, ждали 0x3ffff", lp)
	}
}

// Ловит: смена вида оставляет фокус на кнопке, которой больше нет (Enter нажал бы пустоту),
// или сбрасывает фокус, который остался на месте.
func TestFocusFor(t *testing.T) {
	cases := []struct {
		kind CountdownKind
		cur  CardTarget
		want CardTarget
	}{
		{CountdownTime, TargetRefresh, TargetArrow},
		{CountdownTime, TargetNone, TargetArrow},
		{CountdownError, TargetArrow, TargetArrow},
		{CountdownError, TargetNone, TargetRefresh},
		{CountdownMessage, TargetArrow, TargetArrow},
	}
	for _, c := range cases {
		if got := focusFor(c.kind, c.cur); got != c.want {
			t.Errorf("вид %v, фокус %v: %v, ждали %v", c.kind, c.cur, got, c.want)
		}
	}
	if contains(nil, TargetNone) || !contains([]CardTarget{TargetRefresh, TargetArrow}, TargetArrow) {
		t.Fatal("contains")
	}
}

// Ловит: рисунок без текста не пересчитывается при смене вида, цвета или подсветки (на табло
// остаётся старое) или ключ зависит от цифр (тогда пересчёт каждую секунду).
func TestCardBaseKey(t *testing.T) {
	v := TimeCountdown(3600)
	k := cardBaseKey(v, CardState{})
	if k != "0|{0 0 0 false}|0" {
		t.Fatalf("ключ %q", k)
	}
	if cardBaseKey(TimeCountdown(3599), CardState{}) != k {
		t.Fatal("цифры не меняют рисунок без текста")
	}
	for _, other := range []string{
		cardBaseKey(CountdownView{Kind: CountdownError}, CardState{}),
		cardBaseKey(v, CardState{Hover: TargetArrow}),
		cardBaseKey(v, CardState{FocusVisible: true}),
		cardBaseKey(CountdownView{Status: StatusLow}, CardState{}),
	} {
		if other == k {
			t.Fatalf("ключ %q не отличает вид", other)
		}
	}
}

// Ловит: DIB Windows получает красный и синий не на своих местах (табло синее вместо
// красного) или прозрачность теряется.
func TestToBGRA(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 40})
	img.SetRGBA(1, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	want := []byte{30, 20, 10, 40, 50, 100, 200, 255}
	got := toBGRA(img, nil)
	if string(got) != string(want) {
		t.Fatalf("получили %v, ждали %v", got, want)
	}
	buf := make([]byte, 8)
	if again := toBGRA(img, buf); &again[0] != &buf[0] || string(again) != string(want) {
		t.Fatal("буфер нужного размера переиспользуется")
	}
	for _, n := range []int{4, 12} {
		if got := toBGRA(img, make([]byte, n)); len(got) != 8 {
			t.Fatalf("буфер длиной %d заменяется буфером картинки: длина %d", n, len(got))
		}
	}
}

// Ловит: окно табло ставится по углу карточки, а не окна с полями под тень (карточка
// съезжает от значка или угла на величину поля).
func TestCardOrigins(t *testing.T) {
	l := NewCardLayout(1)
	work := Rect{0, 0, 1920, 1040}
	anchor := Rect{1800, 1048, 1816, 1072}
	px, py := PlacePanel(anchor, work, l.Card.Dx(), l.Card.Dy())
	if x, y := cardOriginAt(l, anchor, work); x != px-l.Margin || y != py-l.Margin || l.Margin == 0 {
		t.Fatalf("у значка %d,%d, ждали %d,%d", x, y, px-l.Margin, py-l.Margin)
	}
	cx, cy := PlaceCorner(work, l.Card.Dx(), l.Card.Dy(), 16)
	if x, y := cardOriginCorner(l, work, 16); x != cx-l.Margin || y != cy-l.Margin {
		t.Fatalf("в углу %d,%d, ждали %d,%d", x, y, cx-l.Margin, cy-l.Margin)
	}
}

// Ловит: текст табло рисуется не по своему выравниванию, с «&» как подчёркиванием или
// многострочный текст стоит не по центру высоты места.
func TestTextRunFlagsAndCentering(t *testing.T) {
	if f := textRunFlags(TextRun{}); f != 0x0800|0x0020|0x0004 {
		t.Fatalf("одна строка слева: %#x", f)
	}
	if f := textRunFlags(TextRun{Align: AlignCenter}); f != 0x0800|0x0001|0x0020|0x0004 {
		t.Fatalf("одна строка по центру: %#x", f)
	}
	if f := textRunFlags(TextRun{Wrap: true, Align: AlignCenter}); f != 0x0800|0x0001|0x0010 {
		t.Fatalf("несколько строк по центру: %#x", f)
	}
	if top, bottom := centerRows(100, 200, 40); top != 130 || bottom != 170 {
		t.Fatalf("сдвиг %d..%d, ждали 130..170", top, bottom)
	}
	if top, bottom := centerRows(100, 200, 100); top != 100 || bottom != 200 {
		t.Fatalf("ровно по месту %d..%d", top, bottom)
	}
	if top, bottom := centerRows(100, 200, 140); top != 100 || bottom != 200 {
		t.Fatalf("выше места не сдвигается: %d..%d", top, bottom)
	}
}

// Ловит: рука вместо стрелки, пока команда в пути или указатель не над кнопкой; забытое табло
// не закрывается через предельный срок; табло значка не прячется по потере фокуса, а плашка
// стража прячется (игра забрала бы её у ребёнка).
func TestCardSmallDecisions(t *testing.T) {
	if !cardHandCursor(CardState{Hover: TargetArrow}, false) || cardHandCursor(CardState{Hover: TargetArrow}, true) || cardHandCursor(CardState{}, false) {
		t.Fatal("рука над кнопкой и только без команды в пути")
	}
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if shownTooLong(true, at, at.Add(5*time.Minute), 5*time.Minute) || !shownTooLong(true, at, at.Add(5*time.Minute+time.Second), 5*time.Minute) || shownTooLong(false, at, at.Add(time.Hour), 5*time.Minute) {
		t.Fatal("срок показа строго больше предела и только у открытого")
	}
	if tooSoon(at, at.Add(400*time.Millisecond), 400*time.Millisecond) || !tooSoon(at, at.Add(399*time.Millisecond), 400*time.Millisecond) {
		t.Fatal("повторное открытие запрещено только внутри защитного срока")
	}
	if !hidesOnDeactivate(false, 0x0000_0000) || hidesOnDeactivate(false, 1) || hidesOnDeactivate(false, 2) || hidesOnDeactivate(true, 0) {
		t.Fatal("по потере активации прячется только табло значка")
	}
	if !hidesOnDeactivate(false, 0x0005_0000) {
		t.Fatal("старшее слово wParam (свёрнуто ли окно) не влияет")
	}
	if !canPress(false, TargetArrow, true) || canPress(true, TargetArrow, true) || canPress(false, TargetNone, true) || canPress(false, TargetArrow, false) {
		t.Fatal("нажатие доходит только без команды в пути, по кнопке и при подписчике")
	}
}

// Ловит: табло с отказом открыть окно показывает цифры без объяснения или ломает экран
// ошибки службы своим текстом.
func TestCardViewWith(t *testing.T) {
	s := freshSnap(okStatus(5025, 7200))
	if v := cardViewWith(s, ""); v.Kind != CountdownTime || v.Hours != "01" {
		t.Fatalf("без отказа: %+v", v)
	}
	v := cardViewWith(s, "Не получилось открыть окно. Попробуй ещё раз")
	if v.Kind != CountdownError || v.Big != "Не получилось открыть окно. Попробуй ещё раз" || v.Button != "Обновить" || v.Title != "ОСТАЛОСЬ ВРЕМЕНИ:" {
		t.Fatalf("с отказом: %+v", v)
	}
	if v := cardViewWith(Snapshot{}, "отказ"); v.Big != "Не удалось получить данные. Попробуй через минуту" {
		t.Fatalf("ошибка службы важнее отказа: %+v", v)
	}
}

// Ловит: кнопка мини-панели делает не своё: недоступная что-то выполняет вместо пояснения,
// «Попросить ещё» не открывает «Мою статистику», кнопка без окна шлёт службе пустую команду.
func TestPanelAction(t *testing.T) {
	cases := []struct {
		name string
		b    PanelButton
		act  panelAct
		arg  string
	}{
		{"недоступная объясняет", PanelButton{Kind: PanelSpendBonus, Disabled: true, DisabledNote: "позже"}, actExplain, "позже"},
		{"обновить", PanelButton{Kind: PanelRefresh}, actRefresh, ""},
		{"копилка", PanelButton{Kind: PanelSpendBonus}, actSpend, ""},
		{"попросить ещё", PanelButton{Kind: PanelAskMore}, actOpen, ipc.WindowMyStats},
		{"моя статистика", PanelButton{Kind: PanelMyStats}, actOpen, ipc.WindowMyStats},
		{"родителю", PanelButton{Kind: PanelParent}, actOpen, ipc.WindowParent},
		{"неизвестная", PanelButton{Kind: PanelKind(99)}, actNone, ""},
	}
	for _, c := range cases {
		if act, arg := panelAction(c.b); act != c.act || arg != c.arg {
			t.Errorf("%s: %v %q, ждали %v %q", c.name, act, arg, c.act, c.arg)
		}
	}
}

// Ловит: стрелка табло не открывает «Мою статистику», «Обновить» не повторяет запрос, крестик
// или пустота что-то нажимают в панели.
func TestCardPressButton(t *testing.T) {
	if b, ok := cardPressButton(TargetArrow); !ok || b.Kind != PanelMyStats {
		t.Fatalf("стрелка: %v %v", b, ok)
	}
	if b, ok := cardPressButton(TargetRefresh); !ok || b.Kind != PanelRefresh {
		t.Fatalf("обновить: %v %v", b, ok)
	}
	for _, tg := range []CardTarget{TargetNone, TargetClose} {
		if _, ok := cardPressButton(tg); ok {
			t.Fatalf("%v ничего не нажимает", tg)
		}
	}
}

// Ловит: из копилки без пароля берётся больше 30 минут, больше остатка или при пустой копилке.
func TestSpendMinutes(t *testing.T) {
	for balance, want := range map[int]int{0: 30, -5: 30, 10: 10, 29: 29, 30: 30, 45: 30} {
		if got := spendMinutes(balance); got != want {
			t.Errorf("копилка %d: %d, ждали %d", balance, got, want)
		}
	}
}

// Ловит: отказ службы теряет человеческий текст или обрыв канала показывает техническую
// ошибку; удачная трата не показывает итог службы.
func TestServiceTexts(t *testing.T) {
	refuse := error(&ipc.ReplyError{Text: "Копилка пуста"})
	wrapped := errors.Join(errors.New("канал"), refuse)
	broken := errors.New("pipe: broken")
	if serviceText(refuse) != "Копилка пуста" || serviceText(wrapped) != "Копилка пуста" || serviceText(broken) != "" {
		t.Fatal("serviceText")
	}
	if got := spendResultText(refuse, ""); got != "Копилка пуста" {
		t.Fatalf("отказ: %q", got)
	}
	if got := spendResultText(broken, ""); got != "Не удалось получить данные. Попробуй через минуту" {
		t.Fatalf("обрыв: %q", got)
	}
	if got := spendResultText(nil, "Добавлено 30 минут"); got != "Добавлено 30 минут" {
		t.Fatalf("удача: %q", got)
	}
}

// Ловит: номер сеанса из ключа --session не читается или мусор принимается за сеанс 0.
func TestParseSession(t *testing.T) {
	cases := []struct {
		in string
		n  uint32
		ok bool
	}{
		{"7", 7, true},
		{"0", 0, true},
		{"4294967295", 4294967295, true},
		{"4294967296", 0, false},
		{"", 0, false},
		{"x", 0, false},
		{"-1", 0, false},
	}
	for _, c := range cases {
		if n, ok := parseSession(c.in); n != c.n || ok != c.ok {
			t.Errorf("%q: %d %v, ждали %d %v", c.in, n, ok, c.n, c.ok)
		}
	}
}
