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

// Плашка сообщения (FR-41): блоки не налезают, кнопка «Понял» большая (не меньше 200×56 при 100 %),
// всё внутри окна; на узком экране плашка не шире экрана; стоит сверху по центру.
func TestNoteLayout(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		for _, areaW := range []int32{1280, 800, 400} {
			w := NoteWidth(dpi, areaW)
			if w > areaW {
				t.Fatalf("dpi %d, экран %d: плашка %d шире экрана", dpi, areaW, w)
			}
			l := NewNoteLayout(dpi, NoteNormal, w, Scaled(28, dpi), Scaled(150, dpi))
			in := func(r Rect) bool { return r.Left >= 0 && r.Top >= 0 && r.Right <= l.W && r.Bottom <= l.H }
			if !in(l.Title) || !in(l.Text) || !in(l.Button) {
				t.Fatalf("dpi %d: блок вне окна: %+v", dpi, l)
			}
			if l.Title.Bottom > l.Text.Top || l.Text.Bottom > l.Button.Top {
				t.Fatalf("dpi %d: блоки налезают: %+v", dpi, l)
			}
			if l.Button.Bottom-l.Button.Top < Scaled(56, dpi) || areaW >= 800 && l.Button.Right-l.Button.Left < Scaled(200, dpi) {
				t.Fatalf("dpi %d: кнопка мала: %+v", dpi, l.Button)
			}
			if c := (l.Button.Left + l.Button.Right) / 2; c < (l.Text.Left+l.Text.Right)/2-1 || c > (l.Text.Left+l.Text.Right)/2+1 {
				t.Fatalf("dpi %d: кнопка не по центру: %+v", dpi, l)
			}
		}
	}
	f := FitNote(96, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, nil, textNoteTitle, "Иди ужинать", fakeNoteMeasure(96))
	if p := f.Place; p.Left != 680 || p.Top != 24 || p.Right != 1240 || p.Bottom != 24+f.Layout.H {
		t.Fatalf("место: %+v", p)
	}
	if f.Density != NoteNormal || f.FontPx != NoteFontPx || f.Text != "Иди ужинать" {
		t.Fatalf("короткий текст на рабочем столе: %+v", f)
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
// плашка выше 40 % рабочей области или текст обрезан; на рабочем столе плашка не сверху по центру.
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
		f := FitNote(c.dpi, c.area, nil, textNoteTitle, text, fakeNoteMeasure(c.dpi))
		btn := Rect{Left: f.Place.Left + f.Layout.Button.Left, Top: f.Place.Top + f.Layout.Button.Top,
			Right: f.Place.Left + f.Layout.Button.Right, Bottom: f.Place.Top + f.Layout.Button.Bottom}
		if btn.Top < c.area.Top || btn.Bottom > c.area.Bottom || btn.Left < c.area.Left || btn.Right > c.area.Right {
			t.Fatalf("dpi %d, %+v: «Понял» %+v вне области", c.dpi, c.area, btn)
		}
		if h := f.Place.Bottom - f.Place.Top; h > (c.area.Bottom-c.area.Top)*40/100 {
			t.Fatalf("dpi %d: плашка %d px выше 40 %% области %d", c.dpi, h, c.area.Bottom-c.area.Top)
		}
		if f.Place.Top != c.area.Top+Scaled(24, c.dpi) {
			t.Fatalf("dpi %d: плашка не сверху: %+v", c.dpi, f.Place)
		}
		tw := NoteTextWidth(c.dpi, f.Layout.W, f.Density)
		if got := fakeNoteMeasure(c.dpi)(f.FontPx, tw, f.Text); got != f.Layout.Text.Bottom-f.Layout.Text.Top {
			t.Fatalf("dpi %d: высота текста %d, в раскладке %d", c.dpi, got, f.Layout.Text.Bottom-f.Layout.Text.Top)
		}
		checkNoteWhole(t, fmt.Sprintf("рабочий стол, dpi %d", c.dpi), c.dpi, c.area, f, text)
	}
	// Сначала шире плашка, потом мельче шрифт: текст средней длины влезает целиком мельче 28.
	mid := strings.Repeat("Иди ужинать. ", 12)
	f := FitNote(144, Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1032}, nil, textNoteTitle, mid, fakeNoteMeasure(144))
	if f.Text != mid || f.FontPx != NoteFontPx || f.Layout.W <= NoteWidth(144, 1920) {
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
// плашка в области, не шире 80 % и не выше 40 % её, блоки не налезают.
func checkNoteWhole(t *testing.T, name string, dpi int, area Rect, f NoteFit, text string) {
	t.Helper()
	if f.Text != text || strings.Contains(f.Text, "…") {
		t.Fatalf("%s: текст обрезан: %q", name, f.Text)
	}
	if f.FontPx < 14 {
		t.Fatalf("%s: шрифт %d мельче 14", name, f.FontPx)
	}
	l, p := f.Layout, f.Place
	tw := NoteTextWidth(dpi, l.W, f.Density)
	if got := fakeNoteMeasure(dpi)(f.FontPx, tw, text); got != l.Text.Bottom-l.Text.Top || l.Text.Right-l.Text.Left != tw {
		t.Fatalf("%s: не все строки видны: текст %d px, место %+v", name, got, l.Text)
	}
	if l.Title.Bottom > l.Text.Top || l.Text.Bottom > l.Button.Top || l.Button.Bottom > l.H || p.Bottom-p.Top != l.H || p.Right-p.Left != l.W {
		t.Fatalf("%s: блоки налезают: %+v %+v", name, l, p)
	}
	if p.Top < area.Top || p.Bottom > area.Bottom || p.Left < area.Left || p.Right > area.Right {
		t.Fatalf("%s: плашка %+v вне области %+v", name, p, area)
	}
	if h := p.Bottom - p.Top; h > (area.Bottom-area.Top)*40/100 {
		t.Fatalf("%s: плашка %d px выше 40 %% области", name, h)
	}
	if w := p.Right - p.Left; w > (area.Right-area.Left)*80/100 {
		t.Fatalf("%s: плашка %d px шире 80 %% области", name, w)
	}
}

// Ловит (S57, снимок s57-note-long-lock.png): длинное сообщение над экраном блокировки показано одной
// строкой мелким шрифтом и обрезано «…»; плашка закрывает заголовок или кнопки экрана блокировки
// (NFR-10); кнопки, сдвинутые вниз ради плашки, уходят за экран; шрифт длинного мельче короткого.
func TestNoteLayoutOverLockScreen(t *testing.T) {
	short := "Скоро ужин, собирайся"
	for _, c := range lockCases() {
		fs := FitNote(c.dpi, c.area, &c.lock, textNoteTitle, short, fakeNoteMeasure(c.dpi))
		if fs.ShiftUp != 0 || fs.ShiftDown != 0 {
			t.Fatalf("%s: короткое сообщение двигает экран блокировки: %+v", c.name, fs)
		}
		for _, text := range []string{short, plain300(), longNote()} {
			name := fmt.Sprintf("%s, %d знаков", c.name, len([]rune(text)))
			f := FitNote(c.dpi, c.area, &c.lock, textNoteTitle, text, fakeNoteMeasure(c.dpi))
			checkNoteWhole(t, name, c.dpi, c.area, f, text)
			if f.ShiftUp < 0 || f.ShiftUp > c.lock.MaxUp || f.ShiftDown < 0 || f.ShiftDown > c.lock.MaxDown {
				t.Fatalf("%s: сдвиг экрана блокировки вверх %d, вниз %d вне пределов %+v", name, f.ShiftUp, f.ShiftDown, c.lock)
			}
			title := c.lock.Title
			title.Top, title.Bottom = title.Top-f.ShiftUp, title.Bottom-f.ShiftUp
			controls := c.lock.ControlsTop + f.ShiftDown
			p := f.Place
			if p.Bottom > controls {
				t.Fatalf("%s: плашка %+v заходит на столбец кнопок (с %d)", name, p, controls)
			}
			if p.Top < title.Bottom && p.Bottom > title.Top {
				t.Fatalf("%s: плашка %+v закрывает заголовок %+v", name, p, title)
			}
			if l := f.Layout; l.Button.Bottom-l.Button.Top < Scaled(40, c.dpi) {
				t.Fatalf("%s: кнопка «Понял» мала: %+v", name, l.Button)
			}
		}
		if fl := FitNote(c.dpi, c.area, &c.lock, textNoteTitle, plain300(), fakeNoteMeasure(c.dpi)); fl.FontPx < fs.FontPx {
			t.Fatalf("%s: шрифт длинного сообщения %d мельче короткого %d", c.name, fl.FontPx, fs.FontPx)
		}
	}
}

// Ловит: на низком экране (1366×768 при 125 %, места между заголовком и кнопками почти нет)
// сообщение обрезано или мельче 14 px, кнопка «Понял» или кнопки экрана блокировки за экраном.
// Здесь допустимо закрыть заголовок: кнопки и поле пароля важнее (NFR-10).
func TestNoteLayoutOverLockLowScreen(t *testing.T) {
	area := Rect{Left: 0, Top: 0, Right: 1366, Bottom: 728}
	lock := NoteLock{Title: Rect{Left: 120, Top: 40, Right: 1246, Bottom: 103}, ControlsTop: 300, MaxUp: 10, MaxDown: 20}
	for _, text := range []string{plain300(), longNote()} {
		f := FitNote(120, area, &lock, textNoteTitle, text, fakeNoteMeasure(120))
		checkNoteWhole(t, "1366×768 125 %", 120, area, f, text)
		if f.Place.Bottom > lock.ControlsTop+f.ShiftDown || f.ShiftDown > lock.MaxDown || f.ShiftUp > lock.MaxUp {
			t.Fatalf("плашка %+v заходит на кнопки или сдвиг вне пределов: %+v", f.Place, f)
		}
	}
}
