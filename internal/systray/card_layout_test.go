package systray

import "testing"

// Ловит: карточка не по макету заказчика (пропорции примерно 1,6 : 1) или вылезает из окна.
func TestCardLayoutProportions(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		l := NewCardLayout(CardScale(dpi))
		ratio := float64(l.Card.Dx()) / float64(l.Card.Dy())
		if ratio < 1.58 || ratio > 1.62 {
			t.Errorf("dpi %d: пропорции %.3f, ждали около 1,6", dpi, ratio)
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
	if l100.Tiles[0].Dx() != 50 || l100.Tiles[0].Dy() != 104 {
		t.Errorf("плитка при 100 %%: %dx%d, ждали 50x104", l100.Tiles[0].Dx(), l100.Tiles[0].Dy())
	}
	if l150.Tiles[0].Dx() != 75 || l150.Tiles[0].Dy() != 156 {
		t.Errorf("плитка при 150 %%: %dx%d, ждали 75x156", l150.Tiles[0].Dx(), l150.Tiles[0].Dy())
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
