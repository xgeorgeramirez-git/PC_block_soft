package systray

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func timeView(h, m, s string, st Status) CountdownView {
	title := textCountdownTitle
	if st == StatusOut {
		title = textCountdownTimeUp
	}
	return CountdownView{Kind: CountdownTime, Title: title, Hours: h, Minutes: m, Seconds: s, Status: st}
}

func px(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }

func near(a, b uint8, tol int) bool {
	d := int(a) - int(b)
	return d >= -tol && d <= tol
}

// Ловит: фон карточки не по макету: переход от тёмного сине-серого (#3F5675) слева к светлому
// (#7E98B4) справа вверху потерян, цвета концов не те.
func TestCardGradientEnds(t *testing.T) {
	if got := cardGradient(0); got != (color.RGBA{0x3F, 0x56, 0x75, 0xFF}) {
		t.Errorf("начало перехода %v, ждали #3F5675", got)
	}
	if got := cardGradient(1); got != (color.RGBA{0x7E, 0x98, 0xB4, 0xFF}) {
		t.Errorf("конец перехода %v, ждали #7E98B4", got)
	}
	l := NewCardLayout(1)
	img := RenderCardBase(l, timeView("01", "23", "45", StatusNormal), CardState{})
	left := px(img, l.Card.Left+4, l.Card.Top+l.Card.Dy()/2)
	right := px(img, l.Card.Right-40, l.Card.Top+36)
	if !(int(right.R)+int(right.G)+int(right.B) > int(left.R)+int(left.G)+int(left.B)+60) {
		t.Errorf("справа вверху %v должно быть заметно светлее, чем слева %v", right, left)
	}
	if left.A != 255 || right.A != 255 {
		t.Errorf("карточка прозрачная внутри: %v %v", left, right)
	}
}

// Ловит: углы не скруглены или не сглажены (угол окна непрозрачный), тень пропала и карточка
// выглядит наклейкой, а не карточкой макета.
func TestCardCornersAndShadow(t *testing.T) {
	l := NewCardLayout(1)
	img := RenderCardBase(l, timeView("01", "23", "45", StatusNormal), CardState{})
	if c := px(img, 0, 0); c.A != 0 {
		t.Errorf("угол окна непрозрачен: %v", c)
	}
	if c := px(img, l.Card.Left+1, l.Card.Top+1); c.A > 40 {
		t.Errorf("угол карточки не скруглён: %v", c)
	}
	// На самой дуге скругления есть частично прозрачные пиксели: сглаживание.
	partial := 0
	for x := l.Card.Left; x < l.Card.Left+l.Radius; x++ {
		for y := l.Card.Top; y < l.Card.Top+l.Radius; y++ {
			if a := px(img, x, y).A; a > 60 && a < 250 {
				partial++
			}
		}
	}
	if partial < l.Radius {
		t.Errorf("на дуге угла только %d сглаженных пикселей, углы рублены", partial)
	}
	below := px(img, l.W/2, l.Card.Bottom+4)
	if below.A == 0 || below.A > 200 {
		t.Errorf("под карточкой тень должна быть полупрозрачной, а там %v", below)
	}
	if far := px(img, l.W/2, l.H-1); far.A > below.A {
		t.Errorf("тень не затухает: у края окна %v, у карточки %v", far, below)
	}
}

// Ловит: пиксели с цветом выше прозрачности ломают UpdateLayeredWindow (ему нужна
// предумноженная прозрачность): в углах получились бы светлые полосы.
func TestCardPremultiplied(t *testing.T) {
	l := NewCardLayout(1.5)
	for _, v := range []CountdownView{timeView("01", "23", "45", StatusLow), {Kind: CountdownError, Button: "Обновить"}, {Kind: CountdownMessage, Big: "x"}} {
		img := RenderCardBase(l, v, CardState{Hover: TargetArrow, Focus: TargetArrow, FocusVisible: true})
		if img.Bounds().Dx() != l.W || img.Bounds().Dy() != l.H {
			t.Fatalf("размер %v, ждали %dx%d", img.Bounds(), l.W, l.H)
		}
		for i := 0; i < len(img.Pix); i += 4 {
			a := img.Pix[i+3]
			if img.Pix[i] > a || img.Pix[i+1] > a || img.Pix[i+2] > a {
				t.Fatalf("пиксель %d: цвет %v выше прозрачности", i/4, img.Pix[i:i+4])
			}
		}
	}
}

// Ловит: плитки не светлые с лёгким голубым и не светлее сверху (макет), либо нарисованы
// там, где должна быть одна крупная строка.
func TestCardTiles(t *testing.T) {
	l := NewCardLayout(1)
	img := RenderCardBase(l, timeView("01", "23", "45", StatusNormal), CardState{})
	for i, tl := range l.Tiles {
		top := px(img, (tl.Left+tl.Right)/2, tl.Top+4)
		bot := px(img, (tl.Left+tl.Right)/2, tl.Bottom-4)
		if top.R < 0xC8 || top.G < 0xD8 || top.B < 0xE8 || top.B < top.R {
			t.Errorf("плитка %d сверху %v: нужно почти белое с лёгким голубым", i, top)
		}
		if int(top.R)+int(top.G)+int(top.B) <= int(bot.R)+int(bot.G)+int(bot.B) {
			t.Errorf("плитка %d: сверху %v не светлее, чем снизу %v", i, top, bot)
		}
	}
	msg := RenderCardBase(l, CountdownView{Kind: CountdownMessage, Big: "Пауза"}, CardState{})
	if c := px(msg, (l.Tiles[0].Left+l.Tiles[0].Right)/2, l.Tiles[0].Top+4); c.R > 0xB0 {
		t.Errorf("у особого состояния плиток нет, а там светло: %v", c)
	}
}

// Ловит: между парами плиток нет белых двоеточий из двух точек.
func TestCardColons(t *testing.T) {
	l := NewCardLayout(1)
	img := RenderCardBase(l, timeView("01", "23", "45", StatusNormal), CardState{})
	for g, c := range l.Colons {
		cx := (c.Left + c.Right) / 2
		midY := (c.Top + c.Bottom) / 2
		up := px(img, cx, midY-int(cardColonDot*l.Scale))
		down := px(img, cx, midY+int(cardColonDot*l.Scale))
		between := px(img, cx, midY)
		for name, p := range map[string]color.RGBA{"верхняя": up, "нижняя": down} {
			if p.R < 0xF0 || p.G < 0xF0 || p.B < 0xF0 {
				t.Errorf("двоеточие %d, %s точка %v не белая", g, name, p)
			}
		}
		if between.R > 0xC0 {
			t.Errorf("двоеточие %d: между точками светло %v, а должен быть фон", g, between)
		}
	}
}

// Ловит: стрелка «>» в круглой кнопке не видна, у наведённой кнопки нет отклика, а фокус с
// клавиатуры не виден (ТЗ 8.2.1 п.3: Tab, Enter/Space, видимый фокус).
func TestCardArrowStates(t *testing.T) {
	l := NewCardLayout(1)
	v := timeView("01", "23", "45", StatusNormal)
	plain := RenderCardBase(l, v, CardState{})
	cx, cy := (l.Arrow.Left+l.Arrow.Right)/2, (l.Arrow.Top+l.Arrow.Bottom)/2
	// Чуть правее центра лежит острие стрелки.
	tip := px(plain, cx+int(0.07*float64(l.Arrow.Dx())), cy)
	if tip.R < 0xF0 {
		t.Errorf("острие стрелки %v не белое", tip)
	}
	ring := px(plain, l.Arrow.Left+1, cy)
	if ring.R < 0xB0 {
		t.Errorf("светлая окружность кнопки не видна: %v", ring)
	}
	hover := RenderCardBase(l, v, CardState{Hover: TargetArrow})
	if a, b := px(plain, cx-12, cy-14), px(hover, cx-12, cy-14); !(int(b.R)+int(b.G)+int(b.B) > int(a.R)+int(a.G)+int(a.B)) {
		t.Errorf("наведение не светлит кнопку: было %v, стало %v", a, b)
	}
	// Фокус рисуется только при управлении с клавиатуры и вокруг кнопки.
	focusHidden := RenderCardBase(l, v, CardState{Focus: TargetArrow})
	focus := RenderCardBase(l, v, CardState{Focus: TargetArrow, FocusVisible: true})
	if diffPixels(plain, focusHidden) != 0 {
		t.Error("фокус виден, хотя клавиатурой ещё не пользовались")
	}
	if n := diffPixels(plain, focus); n < 100 {
		t.Errorf("видимый фокус изменил только %d пикселей: кольца нет", n)
	}
	ringX := l.Arrow.Right + int(cardFocusGap*l.Scale)
	if c := px(focus, ringX, cy); c.R < 0xF0 {
		t.Errorf("кольцо фокуса %v не белое", c)
	}
}

func diffPixels(a, b *image.RGBA) int {
	n := 0
	for i := 0; i < len(a.Pix); i += 4 {
		if a.Pix[i] != b.Pix[i] || a.Pix[i+1] != b.Pix[i+1] || a.Pix[i+2] != b.Pix[i+2] || a.Pix[i+3] != b.Pix[i+3] {
			n++
		}
	}
	return n
}

// Ловит: на экране ошибки нет светлой кнопки «Обновить» (урок L-114: повторить запрос).
func TestCardRefreshButton(t *testing.T) {
	l := NewCardLayout(1)
	img := RenderCardBase(l, CountdownView{Kind: CountdownError, Big: "x", Button: "Обновить"}, CardState{})
	c := px(img, l.Button.Left+8, (l.Button.Top+l.Button.Bottom)/2)
	if c.R < 0xE0 || c.A != 255 {
		t.Errorf("кнопка «Обновить» не светлая: %v", c)
	}
	none := RenderCardBase(l, timeView("01", "23", "45", StatusNormal), CardState{})
	if n := diffPixels(img, none); n < 1000 {
		t.Errorf("экран ошибки почти не отличается от табло (%d пикселей)", n)
	}
}

// Ловит: тексты табло не те, что в макете: заголовок белый и жирный слева, цифры тёмно-синие
// (янтарные при остатке до 15 минут, красные при «ВРЕМЯ ВЫШЛО»), подписи светлые под парами.
func TestCardTexts(t *testing.T) {
	l := NewCardLayout(1)
	runs := CardTexts(l, timeView("01", "23", "45", StatusNormal))
	byText := map[string][]TextRun{}
	for _, r := range runs {
		byText[r.Text] = append(byText[r.Text], r)
	}
	title := byText[textCountdownTitle]
	if len(title) != 1 || title[0].Color != (color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) || !title[0].Bold || title[0].Align != AlignLeft {
		t.Fatalf("заголовок: %+v", title)
	}
	digits := ""
	for _, r := range runs {
		if r.Role == RoleDigit {
			digits += r.Text
			if r.Color != (color.RGBA{0x1E, 0x2B, 0x40, 0xFF}) || !r.Bold || r.Align != AlignCenter {
				t.Errorf("цифра %q: %+v", r.Text, r)
			}
		}
	}
	if digits != "012345" {
		t.Errorf("цифры плиток %q, ждали 012345", digits)
	}
	for i, name := range []string{"часы", "минуты", "секунды"} {
		r := byText[name]
		if len(r) != 1 || r[0].Color != (color.RGBA{0xD6, 0xDF, 0xEA, 0xFF}) || r[0].Rect != l.Labels[i] {
			t.Errorf("подпись %q: %+v", name, r)
		}
	}
	low := CardTexts(l, timeView("00", "14", "59", StatusLow))
	out := CardTexts(l, timeView("00", "00", "00", StatusOut))
	for _, r := range low {
		if r.Role == RoleDigit && r.Color != colorAmber {
			t.Errorf("цифра при остатке до 15 мин не янтарная: %+v", r)
		}
	}
	for _, r := range out {
		if r.Role == RoleDigit && r.Color != colorRed {
			t.Errorf("цифра при «ВРЕМЯ ВЫШЛО» не красная: %+v", r)
		}
	}
}

// Ловит: у особых состояний и ошибки вместо плиток нет крупной строки, а у ошибки нет
// подписи кнопки.
func TestCardTextsMessageAndError(t *testing.T) {
	l := NewCardLayout(1)
	msg := CardTexts(l, CountdownView{Kind: CountdownMessage, Title: textCountdownForbidden, Big: "Можно снова в 07:00"})
	var haveBig, haveTitle bool
	for _, r := range msg {
		if r.Role == RoleDigit || r.Role == RoleLabel {
			t.Errorf("у особого состояния нет плиток и подписей, а есть %+v", r)
		}
		if r.Text == "Можно снова в 07:00" && r.Wrap && r.Align == AlignCenter {
			haveBig = true
		}
		if r.Text == textCountdownForbidden {
			haveTitle = true
		}
	}
	if !haveBig || !haveTitle {
		t.Errorf("нет крупной строки или заголовка: %+v", msg)
	}
	er := CardTexts(l, CountdownView{Kind: CountdownError, Title: textCountdownTitle, Big: textCountdownError, Button: "Обновить"})
	var btn, text bool
	for _, r := range er {
		if r.Text == "Обновить" && r.Rect == l.Button && r.Role == RoleButton {
			btn = true
		}
		if r.Text == textCountdownError && r.Rect == l.ErrorText() {
			text = true
		}
	}
	if !btn || !text {
		t.Errorf("на ошибке нет текста или подписи кнопки: %+v", er)
	}
}

func luminance(c color.RGBA) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

func contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Ловит: на плитке виден горизонтальный шов, как у флип-табло (макет: плитка цельная, сверху
// чуть светлее плавным переходом): соседние строки колонки плитки не должны отличаться
// больше чем на 2 уровня по сумме каналов, а полный перепад сверху вниз не больше 40.
func TestCardTileHasNoSeam(t *testing.T) {
	for _, sc := range []float64{1, 1.5} {
		l := NewCardLayout(sc)
		img := RenderCardBase(l, timeView("01", "23", "45", StatusNormal), CardState{})
		sum := func(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }
		for i, tl := range l.Tiles {
			x := tl.Left + tl.Dx()/2
			from, to := tl.Top+int(8*sc), tl.Bottom-int(8*sc)
			prev := sum(px(img, x, from))
			first := prev
			for y := from + 1; y <= to; y++ {
				cur := sum(px(img, x, y))
				if d := prev - cur; d > 2 || d < -2 {
					t.Fatalf("масштаб %.1f, плитка %d: шов на строке %d (перепад %d за один пиксель)", sc, i, y-tl.Top, d)
				}
				prev = cur
			}
			if drop := first - prev; drop < 0 || drop > 40 {
				t.Errorf("масштаб %.1f, плитка %d: перепад сверху вниз %d, ждали от 0 до 40", sc, i, drop)
			}
		}
	}
}

// Ловит: подписи под парами бледные: цвет #D6DFEA на фоне карточки в месте подписи должен
// давать контраст не ниже 3:1 (полужирный текст от 18,7 px считается крупным по WCAG 2.1 AA);
// на светлом правом нижнем углу градиента было ниже.
func TestCardLabelContrast(t *testing.T) {
	for _, sc := range []float64{1, 1.5, 2} {
		l := NewCardLayout(sc)
		img := RenderCardBase(l, timeView("01", "23", "45", StatusNormal), CardState{})
		for g, lb := range l.Labels {
			for _, x := range []int{lb.Left, (lb.Left + lb.Right) / 2, lb.Right} {
				bg := px(img, x, (lb.Top+lb.Bottom)/2)
				if c := contrast(colorLabel, bg); c < 3.0 {
					t.Errorf("масштаб %.1f, подпись %d, x=%d: контраст %.2f ниже 3:1 (фон %v)", sc, g, x, c, bg)
				}
			}
		}
	}
}
