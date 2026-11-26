package guard

import (
	"math"
	"testing"
)

// noteTestLayout - окно «Иди ужинать» на 1920×1080 при 100 %: стикер 170, карточка 520 (TestNoteLayout).
func noteTestLayout(t *testing.T, sticker bool) NoteLayout {
	t.Helper()
	f := FitNote(96, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, nil, sticker, textNoteTitle, "Иди ужинать", fakeNoteMeasure(96))
	return f.Layout
}

// px - пиксель (x, y) буфера BGRA шириной w: синий, зелёный, красный, альфа.
func px(buf []byte, w int32, x, y int32) [4]byte {
	i := (y*w + x) * 4
	return [4]byte{buf[i], buf[i+1], buf[i+2], buf[i+3]}
}

func bgra(rgb uint32) [4]byte { return [4]byte{byte(rgb), byte(rgb >> 8), byte(rgb >> 16), 255} }

// Ловит (ТЗ 8.2, вид Б): окно сообщения рисуется не прозрачным вокруг тени (углы окна ловили бы
// клики и закрывали экран блокировки), фон карточки не #EEF4FC, рамка или кнопка не #2F6DB5, тени нет
// под карточкой; прозрачность не предумножена (UpdateLayeredWindow рисует такие пиксели светлыми).
func TestRenderNoteBase(t *testing.T) {
	for _, sticker := range []bool{true, false} {
		l := noteTestLayout(t, sticker)
		base := RenderNoteBase(l, false)
		if int32(len(base)) != l.W*l.H*4 {
			t.Fatalf("буфер %d байт, ждали %d", len(base), l.W*l.H*4)
		}
		for _, p := range [][2]int32{{0, 0}, {l.W - 1, 0}, {l.W - 1, l.H - 1}, {0, l.H - 1}} {
			if a := px(base, l.W, p[0], p[1])[3]; a != 0 {
				t.Fatalf("стикер %v: угол окна %v непрозрачен: альфа %d", sticker, p, a)
			}
		}
		cx, cy := (l.Card.Left+l.Card.Right)/2, (l.Card.Top+l.Card.Bottom)/2
		if got := px(base, l.W, cx, cy); got != bgra(NoteTintRGB) {
			t.Fatalf("стикер %v: центр карточки %v, ждали #EEF4FC непрозрачный", sticker, got)
		}
		if got := px(base, l.W, l.Card.Left+l.Border/2, cy); got != bgra(NoteBlueRGB) {
			t.Fatalf("стикер %v: рамка слева %v, ждали #2F6DB5", sticker, got)
		}
		if got := px(base, l.W, cx, l.Card.Bottom-1); got != bgra(NoteBlueRGB) {
			t.Fatalf("стикер %v: рамка снизу %v, ждали #2F6DB5", sticker, got)
		}
		if got := px(base, l.W, l.Card.Left+l.Border+2, cy); got != bgra(NoteTintRGB) {
			t.Fatalf("стикер %v: за рамкой 2 px %v, ждали фон", sticker, got)
		}
		bx, by := (l.Button.Left+l.Button.Right)/2, (l.Button.Top+l.Button.Bottom)/2
		if got := px(base, l.W, bx, by); got != bgra(NoteBlueRGB) {
			t.Fatalf("стикер %v: кнопка %v, ждали #2F6DB5", sticker, got)
		}
		if got := px(base, l.W, l.Button.Left+1, by); got != bgra(NoteBlueRGB) {
			t.Fatalf("стикер %v: левый край кнопки %v: кнопка не во всю ширину", sticker, got)
		}
		if got := px(base, l.W, l.Button.Left-2, by); got != bgra(NoteTintRGB) {
			t.Fatalf("стикер %v: слева от кнопки %v, ждали фон", sticker, got)
		}
		if got := px(base, l.W, l.Button.Left, l.Button.Top); got[3] != 255 || got == bgra(NoteBlueRGB) {
			t.Fatalf("стикер %v: угол кнопки %v не скруглён", sticker, got)
		}
		if hot := RenderNoteBase(l, true); px(hot, l.W, bx, by) != bgra(NoteBlueHotRGB) {
			t.Fatalf("стикер %v: кнопка под мышью %v", sticker, px(hot, l.W, bx, by))
		}
		// Тень под карточкой: полупрозрачная, тёмная; к низу окна сходит на нет.
		sh := px(base, l.W, cx, l.Card.Bottom+l.ShadowDY)
		if sh[3] == 0 || sh[3] > 110 || sh[0] > sh[3] {
			t.Fatalf("стикер %v: тень под карточкой %v", sticker, sh)
		}
		if a := px(base, l.W, cx, l.H-1)[3]; a > 4 {
			t.Fatalf("стикер %v: тень у низа окна альфа %d, ждали почти 0", sticker, a)
		}
		// Угол карточки скруглён на 20 px: в углу карточки (не тени) фона нет.
		if got := px(base, l.W, l.Card.Left+1, l.Card.Top+1); got == bgra(NoteBlueRGB) || got == bgra(NoteTintRGB) {
			t.Fatalf("стикер %v: угол карточки не скруглён: %v", sticker, got)
		}
		for i := 0; i < len(base); i += 4 {
			if base[i] > base[i+3] || base[i+1] > base[i+3] || base[i+2] > base[i+3] {
				t.Fatalf("стикер %v: пиксель %d не предумножен: %v", sticker, i/4, base[i:i+4])
			}
		}
	}
}

// Ловит (ТЗ 8.2): кадр стикера кладётся без своей прозрачности (квадрат вокруг стикера), стикер не
// заходит на карточку, следующий кадр ложится на прежний (полупрозрачные края копятся), наложение
// задевает пиксели вне стикера (T4b обновляет только прямоугольник стикера) или кадр не своего размера
// портит окно.
func TestNoteStickerOver(t *testing.T) {
	l := noteTestLayout(t, true)
	base := RenderNoteBase(l, false)
	side := l.StickerPx
	// Кадр: левая половина прозрачна, правая - красный с альфой 128 (предумноженный: 0, 0, 128, 128).
	frame := make([]byte, side*side*4)
	for y := int32(0); y < side; y++ {
		for x := side / 2; x < side; x++ {
			i := (y*side + x) * 4
			frame[i+2], frame[i+3] = 128, 128
		}
	}
	dst := append([]byte(nil), base...)
	if !NoteStickerOver(dst, base, l, frame) {
		t.Fatal("кадр своего размера не наложен")
	}
	s := l.Sticker
	right, left := s.Left+side*3/4, s.Left+side/4
	if got := px(dst, l.W, right, s.Top+2); got != [4]byte{0, 0, 128, 128} {
		t.Fatalf("стикер над карточкой %v, ждали альфу кадра 128", got)
	}
	if got := px(dst, l.W, left, s.Top+2); got != [4]byte{} {
		t.Fatalf("прозрачная часть кадра над карточкой %v, ждали прозрачность", got)
	}
	// Нижняя часть стикера лежит на карточке: под кадром фон #EEF4FC, окно там непрозрачно.
	y := s.Bottom - 4
	if y <= l.Card.Top+l.Border || y >= l.Title.Top {
		t.Fatalf("строка %d не на карточке под стикером: %+v", y, l)
	}
	tint := bgra(NoteTintRGB)
	want := [4]byte{byte(int(tint[0]) * 127 / 255), byte(int(tint[1]) * 127 / 255), 128 + byte(int(tint[2])*127/255), 255}
	if got := px(dst, l.W, right, y); diff4(got, want) > 1 {
		t.Fatalf("стикер на карточке %v, ждали %v", got, want)
	}
	if got := px(dst, l.W, left, y); got != tint {
		t.Fatalf("прозрачная часть кадра на карточке %v, ждали фон", got)
	}
	// Следующий кадр (весь прозрачный) возвращает основу: прежний кадр не копится.
	if !NoteStickerOver(dst, base, l, make([]byte, side*side*4)) || string(dst) != string(base) {
		t.Fatal("прозрачный кадр не вернул основу: прежний кадр остался")
	}
	// Вне стикера ничего не меняется; кадр не своего размера - только основа.
	NoteStickerOver(dst, base, l, frame)
	for yy := int32(0); yy < l.H; yy++ {
		for xx := int32(0); xx < l.W; xx++ {
			in := xx >= s.Left && xx < s.Right && yy >= s.Top && yy < s.Bottom
			if !in && px(dst, l.W, xx, yy) != px(base, l.W, xx, yy) {
				t.Fatalf("пиксель (%d, %d) вне стикера изменён", xx, yy)
			}
		}
	}
	if NoteStickerOver(dst, base, l, frame[:len(frame)-4]) || string(dst) != string(base) {
		t.Fatal("кадр не своего размера наложен или основа не восстановлена")
	}
	if NoteStickerOver(dst[:8], base, l, frame) {
		t.Fatal("буфер окна не своего размера принят")
	}
	if nl := noteTestLayout(t, false); NoteStickerOver(make([]byte, nl.W*nl.H*4), RenderNoteBase(nl, false), nl, frame) {
		t.Fatal("кадр наложен в окно без стикера")
	}
}

func diff4(a, b [4]byte) int {
	d := 0
	for i := range a {
		d = max(d, int(math.Abs(float64(int(a[i])-int(b[i])))))
	}
	return d
}

// Ловит (ТЗ 8.0 п. 4): синий на карточке, белый на кнопке и текст на карточке ниже 4,5:1.
func TestNoteContrast(t *testing.T) {
	lum := func(rgb uint32) float64 {
		ch := func(v uint32) float64 {
			x := float64(v&0xFF) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(rgb>>16) + 0.7152*ch(rgb>>8) + 0.0722*ch(rgb)
	}
	ratio := func(a, b uint32) float64 {
		la, lb := lum(a), lum(b)
		return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
	}
	for _, c := range []struct {
		name    string
		a, b    uint32
		atLeast float64
	}{
		{"заголовок на карточке", NoteBlueRGB, NoteTintRGB, 4.77},
		{"«Понял» на кнопке", 0xFFFFFF, NoteBlueRGB, 5.28},
		{"«Понял» на кнопке под мышью", 0xFFFFFF, NoteBlueHotRGB, 5.28},
		{"текст на карточке", NoteInkRGB, NoteTintRGB, 4.5},
	} {
		if r := ratio(c.a, c.b); r < c.atLeast-0.005 {
			t.Errorf("%s: контраст %.2f, ждали не ниже %.2f", c.name, r, c.atLeast)
		}
	}
}
