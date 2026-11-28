package guard

import (
	"fmt"
	"strings"
	"testing"
)

func TestScaled(t *testing.T) {
	cases := []struct{ px, dpi, want int }{
		{48, 96, 48}, {48, 144, 72}, {48, 192, 96}, {17, 144, 26}, {0, 144, 0}, {48, 0, 48}, {48, -5, 48},
	}
	for _, c := range cases {
		if got := Scaled(c.px, c.dpi); got != int32(c.want) {
			t.Errorf("Scaled(%d, %d) = %d, ждали %d", c.px, c.dpi, got, c.want)
		}
	}
}

func TestFlowStacksCenteredWithoutOverlap(t *testing.T) {
	area := Rect{0, 0, 1920, 1080}
	items := []FlowItem{{W: 900, H: 60}, {W: 900, H: 30, Gap: 10}, {W: 360, H: 52, Gap: 40}, {W: 360, H: 52, Gap: 14}}
	total := FlowHeight(items)
	if total != 60+30+10+52+40+52+14 {
		t.Fatalf("высота столбца %d", total)
	}
	top := FlowTop(area, total, 24)
	rects := Flow(area, top, items)
	for i, r := range rects {
		if r.Right-r.Left != items[i].W || r.Bottom-r.Top != items[i].H {
			t.Errorf("блок %d изменил размер: %+v", i, r)
		}
		if (r.Left+r.Right)/2 != 960 {
			t.Errorf("блок %d не по центру: %+v", i, r)
		}
		if r.Top < area.Top || r.Bottom > area.Bottom {
			t.Errorf("блок %d вышел из области: %+v", i, r)
		}
		if i > 0 && r.Top != rects[i-1].Bottom+items[i].Gap {
			t.Errorf("блок %d: отступ %d, ждали %d", i, r.Top-rects[i-1].Bottom, items[i].Gap)
		}
	}
	if rects[0].Top != top {
		t.Errorf("столбец начинается с %d, ждали %d", rects[0].Top, top)
	}
	if rects[0].Top >= (area.Bottom-total)/2 {
		t.Errorf("столбец не выше середины: верх %d", rects[0].Top)
	}
}

func TestFlowOnSecondMonitorOffset(t *testing.T) {
	// Основной монитор не обязан начинаться с нуля: область со смещением сохраняет центр.
	area := Rect{-1920, 0, 0, 1080}
	rects := Flow(area, 100, []FlowItem{{W: 400, H: 50}})
	if rects[0].Left != -1160 || rects[0].Right != -760 {
		t.Fatalf("блок в области со смещением: %+v", rects[0])
	}
}

func TestFlowClampsWideBlocksAndHandlesEmpty(t *testing.T) {
	area := Rect{0, 0, 500, 400}
	r := Flow(area, 0, []FlowItem{{W: 900, H: 20}})
	if r[0].Left != 0 || r[0].Right != 500 {
		t.Fatalf("широкий блок не сжат: %+v", r[0])
	}
	if got := Flow(area, 0, nil); len(got) != 0 {
		t.Fatalf("пустой столбец дал %v", got)
	}
	if FlowHeight(nil) != 0 {
		t.Fatal("высота пустого столбца не нулевая")
	}
}

func TestFlowTopKeepsMarginWhenColumnIsTall(t *testing.T) {
	area := Rect{0, 0, 1280, 720}
	if got := FlowTop(area, 900, 24); got != 24 {
		t.Fatalf("высокий столбец начинается с %d, ждали поле 24", got)
	}
	if got := FlowTop(area, 320, 24); got != 160 {
		t.Fatalf("столбец 320 в экране 720: верх %d, ждали 160", got)
	}
}

// Ловит: на плашке с кнопками (плашка 1 мин, «ждёт разрешения») крестик, текст и кнопки налезают
// друг на друга или выходят за окно при масштабах 100…200 %; попадание мимо кнопки нажимает её.
func TestPlateLayoutNoOverlap(t *testing.T) {
	overlap := func(a, b Rect) bool {
		return a.Left < b.Right && b.Left < a.Right && a.Top < b.Bottom && b.Top < a.Bottom
	}
	inside := func(r Rect, w, h int32) bool { return r.Left >= 0 && r.Top >= 0 && r.Right <= w && r.Bottom <= h }
	for _, dpi := range []int{96, 120, 144, 192} {
		for _, c := range []struct {
			n     int
			close bool
		}{{0, false}, {0, true}, {1, false}, {2, false}, {1, true}} {
			for _, textH := range []int32{Scaled(16, dpi), Scaled(60, dpi)} {
				l := NewPlateLayout(dpi, textH, c.n, c.close)
				if l.W != Scaled(400, dpi) || len(l.Buttons) != c.n {
					t.Fatalf("dpi %d %+v: %+v", dpi, c, l)
				}
				all := append([]Rect{l.Text}, l.Buttons...)
				if c.close {
					all = append(all, l.Close)
				}
				for i, a := range all {
					if !inside(a, l.W, l.H) || a.Right <= a.Left || a.Bottom <= a.Top {
						t.Errorf("dpi %d %+v: блок %d вне окна %dx%d: %+v", dpi, c, i, l.W, l.H, a)
					}
					for j := i + 1; j < len(all); j++ {
						if overlap(a, all[j]) {
							t.Errorf("dpi %d %+v: блоки %d и %d налезают: %+v %+v", dpi, c, i, j, a, all[j])
						}
					}
				}
				if l.Text.Right-l.Text.Left != PlateTextWidth(dpi, c.close) {
					t.Errorf("dpi %d: ширина текста %d, окно мерило %d", dpi, l.Text.Right-l.Text.Left, PlateTextWidth(dpi, c.close))
				}
				for i, b := range l.Buttons {
					if got := l.Hit((b.Left+b.Right)/2, (b.Top+b.Bottom)/2); got != i {
						t.Errorf("центр кнопки %d: %d", i, got)
					}
				}
				if c.close {
					if got := l.Hit((l.Close.Left+l.Close.Right)/2, (l.Close.Top+l.Close.Bottom)/2); got != -1 {
						t.Errorf("центр крестика: %d", got)
					}
				}
				if got := l.Hit(1, 1); got != -2 {
					t.Errorf("угол окна: %d", got)
				}
			}
		}
	}
}

// promoColumn — столбец экрана блокировки с ответом на промокод при dpi (высоты текстов —
// как у шрифтов экрана: заголовок 40 px, текст 22, мелкий 17): индекс места стикера и кнопок.
func promoColumn(dpi int, fail bool) (items []FlowItem, sticker int, buttons []int) {
	s := func(px int) int32 { return Scaled(px, dpi) }
	items = []FlowItem{
		{W: s(900), H: s(54)},             // «Время на сегодня закончилось»
		{W: s(900), H: s(30), Gap: s(16)}, // «Можно снова: завтра»
		{W: s(900), H: s(30), Gap: s(6)},  // «Сегодня: …»
		{W: s(900), H: s(30), Gap: s(16)}, // «Попроси родителя…»
	}
	sticker = len(items)
	items = append(items, FlowItem{}, FlowItem{W: s(900), H: s(30), Gap: s(StickerTextGapPx)}) // стикер, надпись
	if fail {
		items = append(items,
			FlowItem{W: s(900), H: s(23), Gap: s(24)}, // «Промокод»
			FlowItem{W: s(380), H: s(52), Gap: s(8)},  // поле
		)
		buttons = []int{len(items), len(items) + 1}
		items = append(items, FlowItem{W: s(380), H: s(54), Gap: s(32)}, FlowItem{W: s(380), H: s(54), Gap: s(14)})
		return
	}
	for i := 0; i < 4; i++ { // «Пароль родителя» и три выхода
		gap := s(14)
		switch i {
		case 0:
			gap = s(32)
		case 1:
			gap = s(28)
		}
		buttons = append(buttons, len(items))
		items = append(items, FlowItem{W: s(380), H: s(54), Gap: gap})
	}
	return
}

// placePromo раскладывает столбец с ответом на промокод так же, как экран блокировки: стикер в
// столбце, если помещается; иначе стикер и надпись слева от поля (неудача) или первой кнопки (удача).
func placePromo(dpi int, area Rect, fail bool) (rects []Rect, anchors []int, sticker, text Rect, side bool, size int32) {
	s := func(px int) int32 { return Scaled(px, dpi) }
	items, si, buttons := promoColumn(dpi, fail)
	margin := s(24)
	size = PromoStickerSize(dpi, area.Bottom-area.Top, FlowHeight(items))
	anchors = buttons
	if fail {
		anchors = append([]int{buttons[0] - 1}, buttons...) // поле и кнопки
	}
	if size > 0 {
		items[si] = FlowItem{W: size, H: size, Gap: s(StickerGapPx)}
		rects = Flow(area, FlowTop(area, FlowHeight(items), margin), items)
		return rects, anchors, rects[si], rects[si+1], false, size
	}
	items[si], items[si+1] = FlowItem{}, FlowItem{}
	rects = Flow(area, FlowTop(area, FlowHeight(items), margin), items)
	textH := 3 * s(30) // надпись в узком месте сбоку: до трёх строк
	size, sticker, text = PromoSide(dpi, area, s(380), rects[anchors[0]].Top, textH)
	return rects, anchors, sticker, text, true, size
}

// Ловит: стикер ответа на промокод при 100/125/150 % наезжает на кнопки и поле, выталкивает кнопки
// за низ экрана или пропадает на частом экране ноутбука 1366×768 при 125 % и на 1280×720 при 150 %
// (Review Focus 5, PL-26, PL-61); на большом экране стикер не своего размера.
func TestPromoStickerLayoutFits(t *testing.T) {
	for _, c := range []struct {
		dpi  int
		w, h int32
		full bool // места хватает: стикер полного размера в столбце
	}{
		{96, 1920, 1080, true}, {96, 1366, 768, false}, {120, 1920, 1080, true}, {120, 1366, 768, false},
		{144, 1920, 1080, false}, {144, 2560, 1440, true}, {144, 1600, 900, false}, {144, 1280, 720, false},
	} {
		for _, fail := range []bool{true, false} {
			name := fmt.Sprintf("dpi %d %dx%d fail=%v", c.dpi, c.w, c.h, fail)
			area := Rect{0, 0, c.w, c.h}
			rects, anchors, st, tx, side, size := placePromo(c.dpi, area, fail)
			if c.full && (side || size != StickerPx(c.dpi)) {
				t.Errorf("%s: стикер %d (сбоку %v), ждали %d в столбце", name, size, side, StickerPx(c.dpi))
			}
			if size < Scaled(96, c.dpi) || size > StickerPx(c.dpi) {
				t.Errorf("%s: стикер %d вне %d…%d (сбоку %v)", name, size, Scaled(96, c.dpi), StickerPx(c.dpi), side)
			}
			in := func(r Rect) bool {
				return r.Left >= area.Left && r.Right <= area.Right && r.Top >= area.Top && r.Bottom <= area.Bottom
			}
			if !in(st) || !in(tx) {
				t.Errorf("%s: стикер %+v или надпись %+v вне экрана", name, st, tx)
			}
			if !side {
				for i, r := range rects {
					if !in(r) {
						t.Errorf("%s: блок %d вне экрана: %+v", name, i, r)
					}
				}
			}
			ring := Scaled(4, c.dpi) // кольцо фокуса вокруг кнопки и поля
			hit := func(a, b Rect) bool {
				return a.Left < b.Right+ring && b.Left < a.Right+ring && a.Top < b.Bottom+ring && b.Top < a.Bottom+ring
			}
			for _, i := range anchors {
				if hit(st, rects[i]) || side && hit(tx, rects[i]) {
					t.Errorf("%s: стикер %+v или надпись %+v наезжает на %+v", name, st, tx, rects[i])
				}
			}
			if st.Bottom > tx.Top {
				t.Errorf("%s: надпись %+v не под стикером %+v", name, tx, st)
			}
		}
	}
}

// Ловит: сбоку стикер больше места (налезает на столбец) или выходит за низ экрана; при нехватке
// места стикер рисуется меньше 96 px при 100 % вместо того, чтобы остаться только надписью.
func TestPromoSide(t *testing.T) {
	area := Rect{0, 0, 1366, 768}
	size, st, tx := PromoSide(120, area, 475, 381, 113)
	if size != 234 || st != (Rect{30 + (385-234)/2, 381, 30 + (385-234)/2 + 234, 615}) || tx != (Rect{30, 625, 415, 738}) {
		t.Fatalf("сбоку: %d %+v %+v", size, st, tx)
	}
	if size, st, tx := PromoSide(120, area, 475, 600, 113); size != 0 || st != (Rect{}) || tx.Top != 600 {
		t.Fatalf("без места: %d %+v %+v", size, st, tx)
	}
	if size, st, _ := PromoSide(120, Rect{0, 0, 900, 1080}, 475, 100, 38); size != 152 || st.Right-st.Left != 152 {
		t.Fatalf("узкое место сбоку: стикер %d %+v, want 152 по ширине", size, st)
	}
	// В столбце: свободно 80 px при 100 % — меньше 96, стикер в столбец не ставится.
	if got := PromoStickerSize(96, 768, 768-48-24-80); got != 0 {
		t.Fatalf("в столбце при 80 px свободных стикер %d, want 0", got)
	}
	if got := PromoStickerSize(96, 768, 768-48-24-96); got != 96 {
		t.Fatalf("в столбце при 96 px свободных стикер %d, want 96", got)
	}
	if w := PromoSideWidth(96, Rect{0, 0, 600, 768}, 380); w != 62 {
		t.Fatalf("ширина места сбоку %d, want 62", w)
	}
	if w := PromoSideWidth(96, Rect{0, 0, 400, 768}, 380); w != 0 {
		t.Fatalf("ширина места сбоку на узком экране %d, want 0", w)
	}
}

// Окно сообщения (FR-41, вид 8.2): блоки не налезают и лежат внутри карточки, кнопка «Понял» во всю
// ширину внутреннего поля высотой 56 при 100 %, карточка 520; на узком экране окно не шире экрана.
func TestNoteLayout(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		for _, areaW := range []int32{1280, 800, 400} {
			for _, st := range []int32{0, NoteStickerPx(dpi)} {
				w := NoteWidth(dpi, areaW)
				l := NewNoteLayout(dpi, NoteNormal, w, st, Scaled(28, dpi), Scaled(150, dpi))
				name := fmt.Sprintf("dpi %d, экран %d, стикер %d", dpi, areaW, st)
				if l.W > areaW {
					t.Fatalf("%s: окно %d шире экрана", name, l.W)
				}
				if areaW == 1280 && dpi <= 144 && l.Card.Right-l.Card.Left != Scaled(520, dpi) {
					t.Fatalf("%s: карточка %+v, ждали ширину %d", name, l.Card, Scaled(520, dpi))
				}
				checkNoteBlocks(t, name, dpi, NoteNormal, l)
				if l.Button.Bottom-l.Button.Top != Scaled(56, dpi) {
					t.Fatalf("%s: кнопка высотой %d, ждали %d", name, l.Button.Bottom-l.Button.Top, Scaled(56, dpi))
				}
			}
		}
	}
	// Рабочий стол 1920×1080 при 100 %: окно 552×394 (карточка 520 и тень 16 по бокам), стикер 170
	// над карточкой, его нижние 56 px на карточке; заголовок через 8 px под стикером.
	f := FitNote(96, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, nil, true, textNoteTitle, "Иди ужинать", fakeNoteMeasure(96))
	l := f.Layout
	want := NoteLayout{W: 552, H: 394, Card: Rect{16, 114, 536, 372}, Shadow: Rect{0, 104, 552, 394},
		Sticker: Rect{191, 0, 361, 170}, StickerPx: 170, Overlap: 56,
		Title: Rect{48, 178, 504, 200}, Text: Rect{48, 212, 504, 264}, Button: Rect{48, 288, 504, 344},
		Radius: 20, Border: 2, BtnRadius: 6, ShadowDY: 6, ShadowBlur: 16}
	if l != want {
		t.Fatalf("раскладка со стикером:\n%+v\nждали\n%+v", l, want)
	}
	if p := f.Place; p != (Rect{684, 24, 1236, 418}) || f.Density != NoteNormal || f.FontPx != NoteFontPx || f.Text != "Иди ужинать" {
		t.Fatalf("место %+v, плотность %+v, шрифт %d", p, f.Density, f.FontPx)
	}
	// Без стикера: та же карточка без верхнего нахлёста, сверху только тень.
	f = FitNote(96, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, nil, false, textNoteTitle, "Иди ужинать", fakeNoteMeasure(96))
	want = NoteLayout{W: 552, H: 254, Card: Rect{16, 10, 536, 232}, Shadow: Rect{0, 0, 552, 254},
		Title: Rect{48, 38, 504, 60}, Text: Rect{48, 72, 504, 124}, Button: Rect{48, 148, 504, 204},
		Radius: 20, Border: 2, BtnRadius: 6, ShadowDY: 6, ShadowBlur: 16}
	if f.Layout != want || f.Place != (Rect{684, 24, 1236, 278}) {
		t.Fatalf("без стикера:\n%+v %+v\nждали\n%+v", f.Layout, f.Place, want)
	}
}

// checkNoteBlocks — блоки окна сообщения на своих местах: карточка и тень в окне, заголовок, текст и
// кнопка в карточке сверху вниз; кнопка во всю ширину внутреннего поля (как текст); стикер по центру
// над карточкой, нижняя часть на карточке (нахлёст 56/170), не больше кадра 300 px и не меньше 96 при
// 100 %; заголовок не под стикером. Без стикера — нахлёста нет, верх карточки — край тени.
func checkNoteBlocks(t *testing.T, name string, dpi int, d NoteDensity, l NoteLayout) {
	t.Helper()
	inWin := func(r Rect) bool { return r.Left >= 0 && r.Top >= 0 && r.Right <= l.W && r.Bottom <= l.H }
	inCard := func(r Rect) bool {
		return r.Left >= l.Card.Left && r.Top >= l.Card.Top && r.Right <= l.Card.Right && r.Bottom <= l.Card.Bottom
	}
	if !inWin(l.Card) || !inWin(l.Shadow) || !inCard(l.Title) || !inCard(l.Text) || !inCard(l.Button) {
		t.Fatalf("%s: блок вне окна или карточки: %+v", name, l)
	}
	if l.Shadow.Left > l.Card.Left || l.Shadow.Right < l.Card.Right || l.Shadow.Bottom < l.Card.Bottom || l.Shadow.Bottom != l.H {
		t.Fatalf("%s: тень %+v не вокруг карточки %+v или окно не до низа тени (%d)", name, l.Shadow, l.Card, l.H)
	}
	if l.Title.Bottom > l.Text.Top || l.Text.Bottom > l.Button.Top || l.Title.Top >= l.Title.Bottom || l.Button.Top >= l.Button.Bottom {
		t.Fatalf("%s: блоки налезают: %+v", name, l)
	}
	inner := l.Card.Right - l.Card.Left - 2*Scaled(d.PadSide, dpi)
	if l.Button.Left != l.Text.Left || l.Button.Right != l.Text.Right || l.Button.Right-l.Button.Left != inner ||
		l.Text.Right-l.Text.Left != NoteTextWidth(dpi, l.Card.Right-l.Card.Left, d) || l.Title.Left != l.Text.Left || l.Title.Right != l.Text.Right {
		t.Fatalf("%s: кнопка %+v не во всю ширину внутреннего поля %d (текст %+v)", name, l.Button, inner, l.Text)
	}
	if l.BtnRadius != Scaled(6, dpi) || l.Radius != Scaled(20, dpi) || l.Border != Scaled(2, dpi) {
		t.Fatalf("%s: скругления %d/%d, рамка %d", name, l.Radius, l.BtnRadius, l.Border)
	}
	if l.StickerPx == 0 {
		if l.Sticker != (Rect{}) || l.Overlap != 0 || l.Card.Top != l.Shadow.Top+l.ShadowBlur-l.ShadowDY || l.Shadow.Top != 0 {
			t.Fatalf("%s: без стикера есть стикер или верхний отступ: %+v", name, l)
		}
		return
	}
	px := l.StickerPx
	if px > 300 || px > min(Scaled(170, dpi), 300) || px < min(Scaled(96, dpi), 300) {
		t.Fatalf("%s: стикер %d вне %d…%d", name, px, min(Scaled(96, dpi), 300), min(Scaled(170, dpi), 300))
	}
	if l.Sticker.Right-l.Sticker.Left != px || l.Sticker.Bottom-l.Sticker.Top != px || !inWin(l.Sticker) || l.Sticker.Top != 0 {
		t.Fatalf("%s: стикер %+v не %d×%d у верха окна", name, l.Sticker, px, px)
	}
	if c, cc := l.Sticker.Left+l.Sticker.Right, l.Card.Left+l.Card.Right; c < cc-1 || c > cc+1 {
		t.Fatalf("%s: стикер %+v не по центру карточки %+v", name, l.Sticker, l.Card)
	}
	if want := (px*56 + 85) / 170; l.Overlap != want || l.Sticker.Bottom != l.Card.Top+want || l.Overlap <= 0 {
		t.Fatalf("%s: нахлёст %d (стикер до %d, карточка с %d), ждали %d", name, l.Overlap, l.Sticker.Bottom, l.Card.Top, want)
	}
	if l.Title.Top < l.Sticker.Bottom {
		t.Fatalf("%s: заголовок %+v под стикером %+v", name, l.Title, l.Sticker)
	}
}

// fakeNoteMeasure — мерка текста как у Windows с переносом по словам и буквам (DT_EDITCONTROL):
// буква 0,55 размера шрифта, строка 1,25 размера; каждый перевод строки — новая строка.
func fakeNoteMeasure(dpi int) NoteMeasure {
	return func(font int, width int32, s string) int32 {
		px := Scaled(font, dpi)
		cpl := max(1, width/max(1, px*55/100))
		var lines int32
		for _, p := range strings.Split(s, "\n") {
			n := int32(len([]rune(p)))
			lines += max(1, (n+cpl-1)/cpl)
		}
		return lines * (px * 5 / 4)
	}
}

// longNote — 300 знаков с 5 переводами строк (предел FR-41).
func longNote() string {
	line := []rune(strings.Repeat("Иди ужинать скорее, ", 3))[:49] // 6 строк по 49 знаков и 5 переводов = 299
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = string(line)
	}
	return strings.Join(parts, "\n") + "!"
}

// Ловит: длинное сообщение (300 знаков, 5 переводов строк) при 150 % уводит кнопку «Понял» за экран,
// окно выше 55 % рабочей области или текст обрезан; на рабочем столе окно не сверху по центру.
func TestNoteLayoutLongText(t *testing.T) {
	text := longNote()
	if n := len([]rune(text)); n != 300 || strings.Count(text, "\n") != 5 {
		t.Fatalf("текст %d знаков, %d переводов", n, strings.Count(text, "\n"))
	}
	for _, c := range []struct {
		dpi  int
		area Rect
	}{
		{144, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1032}},
		{120, Rect{Left: 0, Top: 0, Right: 1366, Bottom: 728}},
		{96, Rect{Left: 0, Top: 0, Right: 1024, Bottom: 728}},
		{144, Rect{Left: -1920, Top: 40, Right: 0, Bottom: 1072}}, // второй монитор слева, панель задач сверху
	} {
		for _, st := range []bool{false, true} {
			f := FitNote(c.dpi, c.area, nil, st, textNoteTitle, text, fakeNoteMeasure(c.dpi))
			btn := Rect{Left: f.Place.Left + f.Layout.Button.Left, Top: f.Place.Top + f.Layout.Button.Top,
				Right: f.Place.Left + f.Layout.Button.Right, Bottom: f.Place.Top + f.Layout.Button.Bottom}
			if btn.Top < c.area.Top || btn.Bottom > c.area.Bottom || btn.Left < c.area.Left || btn.Right > c.area.Right {
				t.Fatalf("dpi %d, %+v: «Понял» %+v вне области", c.dpi, c.area, btn)
			}
			if h := f.Place.Bottom - f.Place.Top; h > (c.area.Bottom-c.area.Top)*55/100 {
				t.Fatalf("dpi %d: окно %d px выше 55 %% области %d", c.dpi, h, c.area.Bottom-c.area.Top)
			}
			if f.Place.Top != c.area.Top+Scaled(24, c.dpi) {
				t.Fatalf("dpi %d: окно не сверху: %+v", c.dpi, f.Place)
			}
			if st && f.Layout.StickerPx == 0 {
				t.Fatalf("dpi %d, %+v: на рабочем столе пропал стикер", c.dpi, c.area)
			}
			checkNoteWhole(t, fmt.Sprintf("рабочий стол, dpi %d, стикер %v", c.dpi, st), c.dpi, c.area, f, text)
		}
	}
	// Сначала шире плашка, потом мельче шрифт: текст средней длины влезает целиком шрифтом 42.
	mid := strings.Repeat("Иди ужинать. ", 12)
	f := FitNote(144, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1032}, nil, false, textNoteTitle, mid, fakeNoteMeasure(144))
	if f.Text != mid || f.FontPx != NoteFontPx || f.Layout.Card.Right-f.Layout.Card.Left <= NoteWidth(144, 1920) {
		t.Fatalf("средний текст: шрифт %d, текст %q", f.FontPx, f.Text)
	}
}

// plain300 — 300 знаков без переводов строк (снимок s57-note-long-lock.png).
func plain300() string {
	return string([]rune(strings.Repeat("Иди ужинать, суп остывает, потом доделаешь уроки. ", 7))[:300])
}

// lockCase — экран блокировки под плашкой: геометрия столбца как у lockNoteGeom (координаты экрана).
type lockCase struct {
	name string
	dpi  int
	area Rect
	lock NoteLock
}

// lockCases — экраны, на которых длинное сообщение обязано встать целиком, не закрыв заголовок и
// кнопки. 1024×768 при 100 % — как на снимке CI: заголовок 65…115, кнопки 272…680.
func lockCases() []lockCase {
	return []lockCase{
		{"1024×768", 96, Rect{Left: 0, Top: 0, Right: 1024, Bottom: 768},
			NoteLock{Title: Rect{Left: 62, Top: 65, Right: 962, Bottom: 115}, ControlsTop: 272, MaxUp: 65 - 16, MaxDown: 768 - 16 - 680}},
		{"1366×768", 96, Rect{Left: 0, Top: 0, Right: 1366, Bottom: 768},
			NoteLock{Title: Rect{Left: 233, Top: 65, Right: 1133, Bottom: 115}, ControlsTop: 272, MaxUp: 65 - 16, MaxDown: 768 - 16 - 680}},
		{"1920×1080", 96, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080},
			NoteLock{Title: Rect{Left: 510, Top: 186, Right: 1410, Bottom: 236}, ControlsTop: 393, MaxUp: 186 - 16, MaxDown: 1080 - 16 - 801}},
		{"1920×1080 150 %", 144, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080},
			NoteLock{Title: Rect{Left: 285, Top: 140, Right: 1635, Bottom: 215}, ControlsTop: 450, MaxUp: 140 - 24, MaxDown: 1080 - 24 - 1000}},
	}
}

// checkNoteWhole — текст показан целиком (без «…», все строки в раскладке), шрифт не мельче 14 px,
// окно с тенью в области, не шире 80 % и не выше 55 % её (Ruling FB28-1), блоки на местах.
func checkNoteWhole(t *testing.T, name string, dpi int, area Rect, f NoteFit, text string) {
	t.Helper()
	if f.Text != text || strings.Contains(f.Text, "…") {
		t.Fatalf("%s: текст обрезан: %q", name, f.Text)
	}
	if f.FontPx < 14 {
		t.Fatalf("%s: шрифт %d мельче 14", name, f.FontPx)
	}
	l, p := f.Layout, f.Place
	tw := NoteTextWidth(dpi, l.Card.Right-l.Card.Left, f.Density)
	if got := fakeNoteMeasure(dpi)(f.FontPx, tw, text); got != l.Text.Bottom-l.Text.Top || l.Text.Right-l.Text.Left != tw {
		t.Fatalf("%s: не все строки видны: текст %d px, место %+v", name, got, l.Text)
	}
	if got := fakeNoteMeasure(dpi)(f.Density.TitlePx, tw, textNoteTitle); got != l.Title.Bottom-l.Title.Top {
		t.Fatalf("%s: заголовок %d px, место %+v", name, got, l.Title)
	}
	checkNoteBlocks(t, name, dpi, f.Density, l)
	if p.Bottom-p.Top != l.H || p.Right-p.Left != l.W {
		t.Fatalf("%s: место %+v не окно с тенью %d×%d", name, p, l.W, l.H)
	}
	if p.Top < area.Top || p.Bottom > area.Bottom || p.Left < area.Left || p.Right > area.Right {
		t.Fatalf("%s: окно %+v вне области %+v", name, p, area)
	}
	if h := p.Bottom - p.Top; h > (area.Bottom-area.Top)*55/100 {
		t.Fatalf("%s: окно %d px выше 55 %% области", name, h)
	}
	if w := p.Right - p.Left; w > (area.Right-area.Left)*80/100 {
		t.Fatalf("%s: окно %d px шире 80 %% области", name, w)
	}
}

// Ловит (S57, снимок s57-note-long-lock.png): длинное сообщение над экраном блокировки показано одной
// строкой мелким шрифтом и обрезано «…»; окно (с тенью: её пиксели ловят клики) закрывает заголовок
// или кнопки экрана блокировки (NFR-10); кнопки, сдвинутые вниз ради окна, уходят за экран; шрифт
// длинного мельче короткого; экран двигается не ради крупного текста (D-107: 42 px важнее
// неподвижного экрана, но тот же размер без раздвижки - без раздвижки).
func TestNoteLayoutOverLockScreen(t *testing.T) {
	short := "Скоро ужин, собирайся"
	for _, c := range lockCases() {
		for _, st := range []bool{false, true} {
			fs := FitNote(c.dpi, c.area, &c.lock, st, textNoteTitle, short, fakeNoteMeasure(c.dpi))
			fixed := c.lock
			fixed.MaxUp, fixed.MaxDown = 0, 0
			if still := FitNote(c.dpi, c.area, &fixed, st, textNoteTitle, short, fakeNoteMeasure(c.dpi)); !st &&
				(fs.ShiftUp != 0 || fs.ShiftDown != 0) && still.FontPx >= fs.FontPx {
				t.Fatalf("%s: короткое сообщение двигает экран блокировки без выгоды для шрифта: %+v", c.name, fs)
			}
			// Карточка шире NoteWidth - только если на NoteWidth тот же текст занял бы больше строк.
			tw0 := NoteTextWidth(c.dpi, NoteWidth(c.dpi, c.area.Right-c.area.Left), fs.Density)
			tw := NoteTextWidth(c.dpi, fs.Layout.Card.Right-fs.Layout.Card.Left, fs.Density)
			if m := fakeNoteMeasure(c.dpi); tw > tw0 && m(fs.FontPx, tw0, short) == m(fs.FontPx, tw, short) {
				t.Fatalf("%s, стикер %v: короткое сообщение в карточке шире нужного: %d", c.name, st, fs.Layout.Card.Right-fs.Layout.Card.Left)
			}
			for _, text := range []string{short, plain300(), longNote()} {
				name := fmt.Sprintf("%s, стикер %v, %d знаков", c.name, st, len([]rune(text)))
				f := FitNote(c.dpi, c.area, &c.lock, st, textNoteTitle, text, fakeNoteMeasure(c.dpi))
				checkNoteWhole(t, name, c.dpi, c.area, f, text)
				checkNoteOverLock(t, name, c.dpi, c.lock, f)
			}
			// Длинное не крупнее короткого того же вида (D-107: короткое 42, длинное мельче, где не
			// помещается): со стикером или без (стикер мог уйти ради текста).
			fl := FitNote(c.dpi, c.area, &c.lock, st, textNoteTitle, plain300(), fakeNoteMeasure(c.dpi))
			if fl.Layout.StickerPx == 0 {
				fs = FitNote(c.dpi, c.area, &c.lock, false, textNoteTitle, short, fakeNoteMeasure(c.dpi))
			}
			if fl.FontPx > fs.FontPx {
				t.Fatalf("%s, стикер %v: шрифт длинного сообщения %d крупнее короткого %d", c.name, st, fl.FontPx, fs.FontPx)
			}
		}
	}
}

// checkNoteOverLock — окно с тенью не заходит на заголовок и столбец кнопок экрана блокировки (с учётом
// сдвига), сдвиг в пределах, кнопка «Понял» не меньше 40 px при 100 %.
func checkNoteOverLock(t *testing.T, name string, dpi int, lock NoteLock, f NoteFit) {
	t.Helper()
	if f.ShiftUp < 0 || f.ShiftUp > lock.MaxUp || f.ShiftDown < 0 || f.ShiftDown > lock.MaxDown {
		t.Fatalf("%s: сдвиг экрана блокировки вверх %d, вниз %d вне пределов %+v", name, f.ShiftUp, f.ShiftDown, lock)
	}
	title := lock.Title
	title.Top, title.Bottom = title.Top-f.ShiftUp, title.Bottom-f.ShiftUp
	controls := lock.ControlsTop + f.ShiftDown
	p := f.Place
	if p.Bottom > controls {
		t.Fatalf("%s: окно %+v заходит на столбец кнопок (с %d)", name, p, controls)
	}
	if p.Top < title.Bottom && p.Bottom > title.Top {
		t.Fatalf("%s: окно %+v закрывает заголовок %+v", name, p, title)
	}
	if l := f.Layout; l.Button.Bottom-l.Button.Top < Scaled(40, dpi) {
		t.Fatalf("%s: кнопка «Понял» мала: %+v", name, l.Button)
	}
}

// Ловит: на низком экране (1366×768 при 125 %, места между заголовком и кнопками почти нет)
// сообщение обрезано или мельче 14 px, кнопка «Понял» или кнопки экрана блокировки за экраном.
// Здесь допустимо закрыть заголовок: кнопки и поле пароля важнее (NFR-10).
func TestNoteLayoutOverLockLowScreen(t *testing.T) {
	area := Rect{Left: 0, Top: 0, Right: 1366, Bottom: 728}
	lock := NoteLock{Title: Rect{Left: 120, Top: 40, Right: 1246, Bottom: 103}, ControlsTop: 300, MaxUp: 10, MaxDown: 20}
	for _, st := range []bool{false, true} {
		for _, text := range []string{plain300(), longNote()} {
			f := FitNote(120, area, &lock, st, textNoteTitle, text, fakeNoteMeasure(120))
			checkNoteWhole(t, fmt.Sprintf("1366×768 125 %%, стикер %v", st), 120, area, f, text)
			if f.Place.Bottom > lock.ControlsTop+f.ShiftDown || f.ShiftDown > lock.MaxDown || f.ShiftUp > lock.MaxUp {
				t.Fatalf("окно %+v заходит на кнопки или сдвиг вне пределов: %+v", f.Place, f)
			}
		}
	}
}

// fakeLock — экран блокировки «Время вышло» без стикера промокода на экране area при dpi: столбец
// как у promoColumn (заголовок, три строки, «Пароль родителя» и три выхода) по FlowTop; пределы
// раздвижки — как lockNoteGeom (поле 16 px у края). На 1280×720 при 150 % столбец выше экрана:
// раздвигать вниз некуда.
func fakeLock(dpi int, area Rect) NoteLock {
	items, si, buttons := promoColumn(dpi, false)
	items = append(items[:si:si], items[si+2:]...)
	split := buttons[0] - 2
	rects := Flow(area, FlowTop(area, FlowHeight(items), Scaled(24, dpi)), items)
	edge := Scaled(16, dpi)
	return NoteLock{Title: rects[0], ControlsTop: rects[split].Top,
		MaxUp: max(0, rects[0].Top-(area.Top+edge)), MaxDown: max(0, area.Bottom-edge-rects[len(rects)-1].Bottom)}
}

// Ловит (S60, FB28-1): на экранах 1024×768, 1280×720, 1366×768, 1920×1080 при 100 % и 150 % (и
// 1920×1080 при 200 % ради предела кадра) окно сообщения со стикером или без выходит за рабочую область, выше 55 %
// её, тень или стикер заходят на заголовок и кнопки экрана блокировки, кнопка «Понял» не во всю
// ширину, стикер не по центру, больше кадра 300 px или меньше 96 px при 100 %, текст обрезан; на
// свободном экране 1920×1080 стикер не полного размера.
func TestNoteLayoutScreens(t *testing.T) {
	short, two := "Молодец!", "Иди ужинать, суп остывает. Потом доделаешь уроки и ляжешь спать"
	for _, sz := range [][2]int32{{1024, 768}, {1280, 720}, {1366, 768}, {1920, 1080}} {
		for _, dpi := range []int{96, 144, 192} {
			if dpi == 192 && sz[0] < 1920 {
				continue // 200 % на малом экране не бывает: 1024×768 при 200 % - 512×384
			}
			area := Rect{0, 0, sz[0], sz[1]}
			lock := fakeLock(dpi, area)
			for _, st := range []bool{false, true} {
				for _, over := range []bool{false, true} {
					for _, text := range []string{short, two, longNote()} {
						name := fmt.Sprintf("%dx%d dpi %d стикер %v блокировка %v, %d знаков", sz[0], sz[1], dpi, st, over, len([]rune(text)))
						var lk *NoteLock
						if over {
							lk = &lock
						}
						f := FitNote(dpi, area, lk, st, textNoteTitle, text, fakeNoteMeasure(dpi))
						checkNoteWhole(t, name, dpi, area, f, text)
						switch {
						case over && text != longNote():
							checkNoteOverLock(t, name, dpi, lock, f)
						case over:
							// 300 знаков на крошечном экране (1024×768 при 150 %) не встают между заголовком и
							// кнопками и мельчайшим шрифтом: окно закрывает заголовок, но не кнопки (NFR-10).
							if f.Place.Bottom > lock.ControlsTop+f.ShiftDown || f.ShiftDown > lock.MaxDown || f.ShiftUp > lock.MaxUp {
								t.Fatalf("%s: окно %+v заходит на кнопки или сдвиг вне пределов: %+v", name, f.Place, f)
							}
						}
						if plain := FitNote(dpi, area, lk, false, textNoteTitle, text, fakeNoteMeasure(dpi)); over && f.FontPx < plain.FontPx {
							t.Fatalf("%s: над экраном блокировки стикер сделал текст мельче: %d, без стикера %d", name, f.FontPx, plain.FontPx)
						}
						if !st && f.Layout.StickerPx != 0 {
							t.Fatalf("%s: стикер без стикера: %+v", name, f.Layout)
						}
						// На свободном экране стикер есть всегда, кроме 300 знаков на экране ниже 600 px при 100 %
						// (1024×768 при 150 %, 1920×1080 при 200 %).
						if st && !over && f.Layout.StickerPx == 0 && (text != longNote() || int(sz[1])*96/dpi >= 600) {
							t.Fatalf("%s: на свободном экране пропал стикер", name)
						}
						if st && !over && text == short && sz == [2]int32{1920, 1080} && dpi <= 144 &&
							(f.Layout.StickerPx != Scaled(170, dpi) || f.Density != NoteNormal || f.FontPx != NoteFontPx) {
							t.Fatalf("%s: стикер %d, шрифт %d, плотность %+v, ждали полный вид", name, f.Layout.StickerPx, f.FontPx, f.Density)
						}
					}
				}
			}
		}
	}
	// 200 %: стикер не крупнее кадра 300 px (иначе размыт), нахлёст 99.
	l := NewNoteLayout(192, NoteNormal, NoteWidth(192, 3840), NoteStickerPx(192), 40, 60)
	if l.StickerPx != 300 || l.Overlap != 99 || NoteStickerMinPx(192) != 192 || NoteStickerPx(96) != 170 || NoteStickerMinPx(96) != 96 {
		t.Fatalf("200 %%: стикер %d, нахлёст %d, минимум %d", l.StickerPx, l.Overlap, NoteStickerMinPx(192))
	}
}

// Ловит (FB-35, D-107, R-8): текст сообщения не в 1,5 раза крупнее (42 px), лестница размеров не
// проходит через ориентир длинного текста 21 px или опускается ниже 14.
func TestNoteFontLadder(t *testing.T) {
	if NoteFontPx != 42 || NoteFontLongPx != 21 || NoteFontMinPx != 14 {
		t.Fatalf("шрифт %d, длинный %d, мельчайший %d; ждали 42, 21, 14", NoteFontPx, NoteFontLongPx, NoteFontMinPx)
	}
	if noteFontSteps[0] != NoteFontPx || noteFontSteps[len(noteFontSteps)-1] != NoteFontMinPx {
		t.Fatalf("лестница %v не от %d до %d", noteFontSteps, NoteFontPx, NoteFontMinPx)
	}
	has21 := false
	for i, px := range noteFontSteps {
		if i > 0 && px >= noteFontSteps[i-1] {
			t.Fatalf("лестница %v не убывает", noteFontSteps)
		}
		has21 = has21 || px == NoteFontLongPx
	}
	if !has21 {
		t.Fatalf("лестница %v без ориентира 21", noteFontSteps)
	}
}

// normalScreens — обычные экраны (≥ 1366×768) при 100 %: рабочая область с панелью задач 40 px.
var normalScreens = []Rect{{0, 0, 1366, 728}, {0, 0, 1920, 1040}}

// Ловит (FB-35, D-107): короткое сообщение показано не 42 px на обычных экранах при 100 % и 150 %,
// на рабочем столе и поверх экрана блокировки. Исключение - 1366×768 при 150 % поверх экрана
// блокировки: между заголовком и кнопками (раздвинутыми до предела) 42 px не встаёт, а окно не
// закрывает ни то, ни другое (8.2) - там крупнейший, что встаёт, и не мельче 21.
func TestFitNoteShortIs42(t *testing.T) {
	for _, sz := range [][2]int32{{1366, 768}, {1920, 1080}} {
		for _, dpi := range []int{96, 144} {
			area := Rect{0, 0, sz[0], sz[1]}
			lock := fakeLock(dpi, area)
			for _, lk := range []*NoteLock{nil, &lock} {
				for _, st := range []bool{false, true} {
					f := FitNote(dpi, area, lk, st, textNoteTitle, "Иди ужинать", fakeNoteMeasure(dpi))
					if low := lk != nil && dpi == 144 && sz[0] == 1366; low && f.FontPx >= NoteFontLongPx {
						checkNoteOverLock(t, "1366×768 150 %", dpi, lock, f)
						continue
					}
					if f.FontPx != 42 {
						t.Fatalf("%dx%d dpi %d блокировка %v стикер %v: шрифт %d, ждали 42", sz[0], sz[1], dpi, lk != nil, st, f.FontPx)
					}
				}
			}
		}
	}
}

// Ловит (8.2, R-8): 300 знаков с 5 переводами строк на обычном экране при 100 % мельче 21 px, обрезаны
// или окно выше 55 % области (Ruling FB28-1).
func TestFitNoteLongNotBelow21OnNormalScreens(t *testing.T) {
	for _, area := range normalScreens {
		for _, text := range []string{longNote(), plain300()} {
			for _, st := range []bool{false, true} {
				name := fmt.Sprintf("%+v стикер %v, %d знаков", area, st, len([]rune(text)))
				f := FitNote(96, area, nil, st, textNoteTitle, text, fakeNoteMeasure(96))
				if f.FontPx < NoteFontLongPx {
					t.Fatalf("%s: шрифт %d мельче 21", name, f.FontPx)
				}
				checkNoteWhole(t, name, 96, area, f, text)
			}
		}
	}
}

// Ловит (8.2): 300 знаков на очень низком экране (1024×768 при 150 %) поверх экрана блокировки мельче
// 14 px, обрезаны или окно заходит на столбец кнопок экрана блокировки (NFR-10).
func TestFitNoteLongTinyScreen(t *testing.T) {
	area := Rect{0, 0, 1024, 768}
	lock := fakeLock(144, area)
	for _, st := range []bool{false, true} {
		for _, text := range []string{longNote(), plain300()} {
			name := fmt.Sprintf("1024×768 150 %% стикер %v, %d знаков", st, len([]rune(text)))
			f := FitNote(144, area, &lock, st, textNoteTitle, text, fakeNoteMeasure(144))
			checkNoteWhole(t, name, 144, area, f, text)
			if f.Place.Bottom > lock.ControlsTop+f.ShiftDown || f.ShiftDown > lock.MaxDown || f.ShiftUp > lock.MaxUp {
				t.Fatalf("%s: окно %+v заходит на кнопки или сдвиг вне пределов: %+v", name, f.Place, f)
			}
		}
	}
}

// Ловит (S57, D-107, ТЗ 8.2): признак «текст сообщения поместился целиком» (GuardState.NoteTextFits)
// врёт: истинен, когда прямоугольник текста ниже высоты, намеренной тем же шрифтом в ту же ширину,
// когда окно меряет крупнее, чем разложено, или когда запасное тесное широкое окно на крошечном
// экране выше области и текст уходит за её край; или ложен у обычной раскладки короткого и длинного
// сообщения поверх экрана блокировки 1024×768 (как у раннера).
func TestNoteTextFits(t *testing.T) {
	m := fakeNoteMeasure(96)
	lc := lockCases()[0]
	short := FitNote(96, lc.area, &lc.lock, true, textNoteTitle, "Иди ужинать", m)
	long := FitNote(96, lc.area, &lc.lock, true, textNoteTitle, longNote(), m)
	if short.FontPx != 42 || long.FontPx >= short.FontPx || long.FontPx < NoteFontMinPx {
		t.Fatalf("шрифт короткого %d, длинного %d: ждали 42 и мельче, но не мельче 14", short.FontPx, long.FontPx)
	}
	if !NoteTextFits(short, lc.area, m) || !NoteTextFits(long, lc.area, m) {
		t.Fatalf("1024×768: признак ложен у короткого (%v) или длинного (%v)", NoteTextFits(short, lc.area, m), NoteTextFits(long, lc.area, m))
	}
	cut := long
	cut.Layout.Text.Bottom--
	if NoteTextFits(cut, lc.area, m) {
		t.Fatal("прямоугольник текста на 1 px ниже меры, а признак истинен")
	}
	bigger := func(font int, w int32, s string) int32 { return m(font, w, s) + 1 }
	if NoteTextFits(long, lc.area, bigger) {
		t.Fatal("окно меряет текст выше раскладки, а признак истинен")
	}
	left := short
	left.Place.Left, left.Place.Right = lc.area.Left-short.Layout.Text.Right, lc.area.Left-short.Layout.Text.Right+short.Layout.W
	if NoteTextFits(left, lc.area, m) {
		t.Fatalf("текст левее области (окно %+v), а признак истинен", left.Place)
	}
	tiny := Rect{Left: 0, Top: 0, Right: 320, Bottom: 200}
	f := FitNote(96, tiny, nil, false, textNoteTitle, longNote(), m)
	tw := f.Layout.Text.Right - f.Layout.Text.Left
	if f.FontPx != NoteFontMinPx || f.Density != NoteCompact || f.Layout.Text.Bottom-f.Layout.Text.Top != m(NoteFontMinPx, tw, f.Text) {
		t.Fatalf("крошечный экран: ждали запасное тесное окно 14 px с текстом по мере, а %+v", f)
	}
	if f.Place.Top+f.Layout.Text.Bottom <= tiny.Bottom {
		t.Fatalf("крошечный экран: текст %d…%d не ушёл за край %d, случай не проверяет край", f.Place.Top+f.Layout.Text.Top, f.Place.Top+f.Layout.Text.Bottom, tiny.Bottom)
	}
	if NoteTextFits(f, tiny, m) {
		t.Fatal("крошечный экран: текст за краем области, а признак истинен")
	}
	if NoteTextFits(NoteFit{}, lc.area, m) {
		t.Fatal("окна нет, а признак истинен")
	}
}
