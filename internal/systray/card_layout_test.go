package systray

import "testing"

// Ловит: карточка не по макету заказчика: окно целиком (с полями под тень, как оно видно на
// кадре) вышло около 1,5 : 1 вместо 1,6 : 1, либо карточка вылезает из окна.
func TestCardLayoutProportions(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		l := NewCardLayout(CardScale(dpi))
		win := float64(l.W) / float64(l.H)
		if win < 1.58 || win > 1.63 {
			t.Errorf("dpi %d: пропорции окна %.3f, ждали около 1,6", dpi, win)
		}
		if ratio := float64(l.Card.Dx()) / float64(l.Card.Dy()); ratio < 1.6 || ratio > 1.7 {
			t.Errorf("dpi %d: пропорции самой карточки %.3f, ждали 1,6 до 1,7", dpi, ratio)
		}
		if min := l.Card.Dx() * 105 / 1000; l.Radius < min {
			t.Errorf("dpi %d: радиус углов %d, у макета заметно круглее (не меньше 10,5 %% ширины карточки, то есть %d)", dpi, l.Radius, min)
		}
		if l.Card.Left != l.Margin || l.Card.Top != l.Margin || l.W != l.Card.Dx()+2*l.Margin || l.H != l.Card.Dy()+2*l.Margin {
			t.Errorf("dpi %d: окно %dx%d не вмещает карточку %+v с полями %d", dpi, l.W, l.H, l.Card, l.Margin)
		}
	}
}

// Ловит: при 100 % и 150 % (FR-7) размеры разъезжаются: плитки не масштабируются, шесть
// плиток вылезают за карточку или заходят друг на друга, подписи не под парами.
func TestCardLayoutTiles(t *testing.T) {
	l100, l150 := NewCardLayout(CardScale(96)), NewCardLayout(CardScale(144))
	if l100.Tiles[0].Dx() != 54 || l100.Tiles[0].Dy() != 88 {
		t.Errorf("плитка при 100 %%: %dx%d, ждали 54x88 (ближе к квадрату, чем 50x104)", l100.Tiles[0].Dx(), l100.Tiles[0].Dy())
	}
	if l150.Tiles[0].Dx() != 81 || l150.Tiles[0].Dy() != 132 {
		t.Errorf("плитка при 150 %%: %dx%d, ждали 81x132", l150.Tiles[0].Dx(), l150.Tiles[0].Dy())
	}
	for _, l := range []CardLayout{l100, l150} {
		for i, tl := range l.Tiles {
			if tl.Left < l.Card.Left || tl.Right > l.Card.Right || tl.Dx() != l.Tiles[0].Dx() {
				t.Errorf("плитка %d вне карточки или другой ширины: %+v", i, tl)
			}
			if i > 0 && tl.Left < l.Tiles[i-1].Right {
				t.Errorf("плитка %d заходит на плитку %d", i, i-1)
			}
		}
		for g := 0; g < 3; g++ {
			if l.Labels[g].Left != l.Tiles[g*2].Left || l.Labels[g].Right != l.Tiles[g*2+1].Right || l.Labels[g].Top < l.Tiles[g*2].Bottom {
				t.Errorf("подпись %d не под парой плиток: %+v", g, l.Labels[g])
			}
		}
		if l.Labels[2].Bottom > l.Card.Bottom {
			t.Errorf("подписи вылезли за низ карточки")
		}
		// Правый отступ равен левому с точностью до пикселя округления.
		left, right := l.Tiles[0].Left-l.Card.Left, l.Card.Right-l.Tiles[5].Right
		if d := left - right; d < -2 || d > 2 {
			t.Errorf("отступы слева %d и справа %d не равны", left, right)
		}
		if !(l.Title.Right <= l.Arrow.Left) {
			t.Errorf("заголовок заходит под кнопку со стрелкой")
		}
		if l.Arrow.Right > l.Card.Right-int(24*l.Scale)+1 || l.Arrow.Top < l.Card.Top {
			t.Errorf("кнопка со стрелкой не в правом верхнем углу: %+v", l.Arrow)
		}
	}
}

// Ловит: нажатие в углу квадрата вокруг круглой кнопки считается нажатием (кнопка круглая),
// «Обновить» откликается вне экрана ошибки.
func TestCardHitTest(t *testing.T) {
	l := NewCardLayout(1)
	cx, cy := (l.Arrow.Left+l.Arrow.Right)/2, (l.Arrow.Top+l.Arrow.Bottom)/2
	if got := l.HitTest(cx, cy, CountdownTime); got != TargetArrow {
		t.Errorf("центр кнопки: %v", got)
	}
	if got := l.HitTest(l.Arrow.Left, l.Arrow.Top, CountdownTime); got != TargetNone {
		t.Errorf("угол квадрата вокруг круга: %v, ждали TargetNone", got)
	}
	bx, by := (l.Button.Left+l.Button.Right)/2, (l.Button.Top+l.Button.Bottom)/2
	if got := l.HitTest(bx, by, CountdownError); got != TargetRefresh {
		t.Errorf("«Обновить» на экране ошибки: %v", got)
	}
	if got := l.HitTest(bx, by, CountdownTime); got != TargetNone {
		t.Errorf("«Обновить» вне ошибки: %v, ждали TargetNone", got)
	}
	if got := l.HitTest(l.Tiles[0].Left+5, l.Tiles[0].Top+5, CountdownTime); got != TargetNone {
		t.Errorf("плитка не кнопка: %v", got)
	}
}

// Ловит: Tab не обходит кнопки по кругу, Shift+Tab идёт не назад, а на экране ошибки фокус
// первой не на «Обновить» (ТЗ 8.2.1 п.3: кнопка со стрелкой доступна с клавиатуры).
func TestCardFocusOrder(t *testing.T) {
	timeOrder := FocusOrder(CountdownTime)
	if len(timeOrder) != 1 || timeOrder[0] != TargetArrow {
		t.Fatalf("на табло фокус только на стрелке: %v", timeOrder)
	}
	if got := NextFocus(timeOrder, TargetArrow, false); got != TargetArrow {
		t.Errorf("Tab с единственной кнопки: %v", got)
	}
	errOrder := FocusOrder(CountdownError)
	if errOrder[0] != TargetRefresh || errOrder[1] != TargetArrow {
		t.Fatalf("на ошибке порядок «Обновить», стрелка: %v", errOrder)
	}
	if got := NextFocus(errOrder, TargetRefresh, false); got != TargetArrow {
		t.Errorf("Tab с «Обновить»: %v", got)
	}
	if got := NextFocus(errOrder, TargetArrow, false); got != TargetRefresh {
		t.Errorf("Tab со стрелки по кругу: %v", got)
	}
	if got := NextFocus(errOrder, TargetRefresh, true); got != TargetArrow {
		t.Errorf("Shift+Tab с первой кнопки по кругу назад: %v", got)
	}
	if got := NextFocus(errOrder, TargetNone, true); got != TargetArrow {
		t.Errorf("Shift+Tab без фокуса: последняя кнопка, получили %v", got)
	}
	if got := NextFocus(nil, TargetNone, false); got != TargetNone {
		t.Errorf("пустой список: %v", got)
	}
}

// Ловит: двоеточия зажаты между плитками и почти касаются их (на кадре воздуха нет): от точки
// до соседней плитки должно быть не меньше 12 px при 100 %.
func TestCardColonAir(t *testing.T) {
	for _, sc := range []float64{1, 1.5, 2} {
		l := NewCardLayout(sc)
		for g, c := range l.Colons {
			air := float64(c.Dx())/2 - cardDotRadius*sc
			if air < 12*sc-1 {
				t.Errorf("масштаб %.1f, двоеточие %d: воздуха до плитки %.1f px, ждали не меньше %.0f", sc, g, air, 12*sc)
			}
		}
	}
}

// Ловит: подписи «часы / минуты / секунды» мелкие (было 16 px при цифрах 80 px): подпись
// должна быть около трети высоты цифры (от 0,25 до 0,36 размера цифр).
func TestCardLabelSize(t *testing.T) {
	for _, sc := range []float64{1, 1.5, 2} {
		l := NewCardLayout(sc)
		var digit, label int
		for _, r := range CardTexts(l, timeView("01", "23", "45", StatusNormal)) {
			switch r.Role {
			case RoleDigit:
				digit = r.Px
			case RoleLabel:
				label = r.Px
				if !r.Bold {
					t.Errorf("подпись %q не полужирная: на тёмном фоне тонкий шрифт бледнеет", r.Text)
				}
			}
		}
		if f := float64(label) / float64(digit); f < 0.25 || f > 0.40 {
			t.Errorf("масштаб %.1f: подпись %d px при цифрах %d px (доля %.2f), ждали 0,25 до 0,40", sc, label, digit, f)
		}
		if label < int(24*sc) {
			t.Errorf("масштаб %.1f: подпись %d px, ждали не меньше %d", sc, label, int(24*sc))
		}
	}
}

// Ловит: заголовок «ОСТАЛОСЬ ВРЕМЕНИ:» мельче макета (было 25 px, нужно не меньше 28 при 100 %).
func TestCardTitleSize(t *testing.T) {
	for _, sc := range []float64{1, 1.5} {
		for _, r := range CardTexts(NewCardLayout(sc), timeView("01", "23", "45", StatusNormal)) {
			if r.Role == RoleTitle && r.Px < int(28*sc) {
				t.Errorf("масштаб %.1f: заголовок %d px, ждали не меньше %d", sc, r.Px, int(28*sc))
			}
		}
	}
}
