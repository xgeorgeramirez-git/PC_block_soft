package systray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

var (
	white     = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	iconSizes = []int{16, 24, 32}
	// Ожидаемые цвета фона записаны литералами из ТЗ 8.0 п.9, а не берутся из кода.
	wantBackground = map[Status]color.RGBA{
		StatusNormal:   {0x2B, 0x31, 0x38, 0xFF},
		StatusLow:      {0xA1, 0x5C, 0x00, 0xFF},
		StatusOut:      {0xB3, 0x26, 0x1E, 0xFF},
		StatusPaused:   {0x2C, 0x7A, 0x4B, 0xFF},
		StatusNotChild: {0x2B, 0x31, 0x38, 0xFF},
	}
	allStatuses = []Status{StatusNormal, StatusLow, StatusOut, StatusPaused, StatusNotChild}
	// Надписи, которые значок реально показывает (FR-7) и их крайние случаи.
	sampleLabels = []string{"0", "7", "15", "85", "99", "1ч", "2ч", "10ч", "24ч", "99ч", "∞"}
)

// inkBox — рамка белых пикселей картинки; ok=false, если белых пикселей нет.
func inkBox(img *image.RGBA) (minX, minY, maxX, maxY int, ok bool) {
	minX, minY = img.Bounds().Dx(), img.Bounds().Dy()
	maxX, maxY = -1, -1
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.RGBAAt(x, y) == white {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
				ok = true
			}
		}
	}
	return
}

// inkColumnGroups — сколько отдельных знаков видно: группы столбцов с белыми пикселями,
// между которыми есть хотя бы один пустой столбец.
func inkColumnGroups(img *image.RGBA) int {
	groups, inside := 0, false
	for x := 0; x < img.Bounds().Dx(); x++ {
		has := false
		for y := 0; y < img.Bounds().Dy(); y++ {
			if img.RGBAAt(x, y) == white {
				has = true
				break
			}
		}
		if has && !inside {
			groups++
		}
		inside = has
	}
	return groups
}

func inkMask(img *image.RGBA) string {
	var b strings.Builder
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.RGBAAt(x, y) == white {
				b.WriteByte('X')
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Ловит: размер картинки не 16, 24, 32 (масштаб Windows 100, 150, 200 %) или не квадрат.
func TestRenderIconSize(t *testing.T) {
	for _, px := range iconSizes {
		for _, st := range allStatuses {
			img := RenderIcon("85", st, px)
			if img == nil {
				t.Fatalf("RenderIcon(\"85\", %v, %d) = nil", st, px)
			}
			if b := img.Bounds(); b.Min != (image.Point{}) || b.Dx() != px || b.Dy() != px {
				t.Errorf("RenderIcon(\"85\", %v, %d): границы %v, нужен квадрат %d×%d от нуля", st, px, b, px, px)
			}
		}
	}
}

// Ловит: полупрозрачные и размытые края (масштаб без целых пикселей), чужие цвета на
// значке (в картинке должны быть только цвет статуса, белая надпись и прозрачные углы),
// надпись не белая или фон не цвета статуса (L-14, NFR-14).
func TestRenderIconOnlyStatusColorAndWhite(t *testing.T) {
	labels := append([]string{""}, sampleLabels...)
	for _, px := range iconSizes {
		r := px / 8
		for _, st := range allStatuses {
			for _, label := range labels {
				img := RenderIcon(label, st, px)
				ink := 0
				for y := 0; y < px; y++ {
					for x := 0; x < px; x++ {
						c := img.RGBAAt(x, y)
						inCorner := (x < r || x >= px-r) && (y < r || y >= px-r)
						switch {
						case c == (color.RGBA{}):
							if !inCorner {
								t.Errorf("%q %v %d px: прозрачный пиксель (%d,%d) вне угловых квадратов %d×%d", label, st, px, x, y, r, r)
							}
						case c == white:
							ink++
						case c == wantBackground[st]:
						default:
							t.Errorf("%q %v %d px: пиксель (%d,%d) цвета %v, ожидались фон %v или белый", label, st, px, x, y, c, wantBackground[st])
						}
					}
				}
				if label == "" && st != StatusNotChild && ink != 0 {
					t.Errorf("пустая надпись %v %d px: белых пикселей %d, ожидалось 0", st, px, ink)
				}
				if label != "" && st != StatusNotChild && ink == 0 {
					t.Errorf("надпись %q %v %d px: не нарисована ни одного белого пикселя", label, st, px)
				}
			}
		}
	}
}

// Ловит: углы не скруглены (квадрат) или срезаны сильнее радиуса px/8: на каждый угол
// 16 и 24 px приходится один прозрачный пиксель, на 32 px три; середины сторон закрашены.
func TestRenderIconRoundedCorners(t *testing.T) {
	wantTransparent := map[int]int{16: 4, 24: 4, 32: 12}
	for _, px := range iconSizes {
		img := RenderIcon("", StatusNormal, px)
		n := 0
		for y := 0; y < px; y++ {
			for x := 0; x < px; x++ {
				if img.RGBAAt(x, y).A == 0 {
					n++
				}
			}
		}
		if n != wantTransparent[px] {
			t.Errorf("%d px: прозрачных пикселей %d, ожидалось %d", px, n, wantTransparent[px])
		}
		for _, p := range [][2]int{{0, 0}, {px - 1, 0}, {0, px - 1}, {px - 1, px - 1}} {
			if img.RGBAAt(p[0], p[1]).A != 0 {
				t.Errorf("%d px: угол (%d,%d) не скруглён", px, p[0], p[1])
			}
		}
		for _, p := range [][2]int{{px / 2, 0}, {px / 2, px - 1}, {0, px / 2}, {px - 1, px / 2}} {
			if img.RGBAAt(p[0], p[1]) != wantBackground[StatusNormal] {
				t.Errorf("%d px: середина стороны (%d,%d) не закрашена", px, p[0], p[1])
			}
		}
	}
}

// Ловит: надпись мелкая (высота знака меньше половины значка, NFR-14), обрезана краем или
// прижата к нему (нужно поле не меньше 1 px, L-10, L-11), «ч» отдельно слишком низкая.
func TestRenderIconLabelReadable(t *testing.T) {
	labels := append([]string{"ч"}, sampleLabels...)
	for _, px := range iconSizes {
		for _, label := range labels {
			img := RenderIcon(label, StatusNormal, px)
			x0, y0, x1, y1, ok := inkBox(img)
			if !ok {
				t.Errorf("%q %d px: надписи нет", label, px)
				continue
			}
			if h := y1 - y0 + 1; 2*h < px {
				t.Errorf("%q %d px: высота надписи %d меньше половины значка", label, px, h)
			}
			if x0 < 1 || y0 < 1 || x1 > px-2 || y1 > px-2 {
				t.Errorf("%q %d px: надпись занимает x %d..%d, y %d..%d, поле меньше 1 px", label, px, x0, x1, y0, y1)
			}
		}
	}
}

// Ловит: знаки надписи слипаются в пятно (нет пустого столбца между ними) или знак
// выпал при нехватке места: «85», «99», «1ч», «24ч» должны читаться как 2 и 3 знака.
func TestRenderIconGlyphsStaySeparate(t *testing.T) {
	cases := []struct {
		label string
		want  int
	}{
		{"7", 1}, {"∞", 1}, {"ч", 1},
		{"85", 2}, {"99", 2}, {"11", 2}, {"1ч", 2}, {"2ч", 2}, {"15", 2},
		{"10ч", 3}, {"24ч", 3}, {"99ч", 3}, {"888", 3},
	}
	for _, px := range iconSizes {
		for _, tc := range cases {
			if got := inkColumnGroups(RenderIcon(tc.label, StatusNormal, px)); got != tc.want {
				t.Errorf("%q %d px: видно знаков %d, ожидалось %d\n%s", tc.label, px, got, tc.want, inkMask(RenderIcon(tc.label, StatusNormal, px)))
			}
		}
	}
}

// Ловит: разные надписи рисуются одинаково (значок не говорит, сколько осталось).
func TestRenderIconDifferentLabelsDiffer(t *testing.T) {
	labels := append([]string{"", "ч", "8", "88", "4", "14", "1ч"}, sampleLabels...)
	for _, px := range iconSizes {
		seen := map[string]string{}
		for _, label := range labels {
			key := string(RenderIcon(label, StatusNormal, px).Pix)
			if other, dup := seen[key]; dup && other != label {
				t.Errorf("%d px: надписи %q и %q нарисованы одинаково", px, other, label)
			}
			seen[key] = label
		}
	}
}

// Ловит: надпись смещается или меняется при смене статуса; один и тот же текст на всех
// цветах стоит в одном месте, меняется только фон.
func TestRenderIconStatusChangesOnlyBackground(t *testing.T) {
	for _, px := range iconSizes {
		for _, label := range sampleLabels {
			base := inkMask(RenderIcon(label, StatusNormal, px))
			for _, st := range []Status{StatusLow, StatusOut, StatusPaused} {
				img := RenderIcon(label, st, px)
				if got := inkMask(img); got != base {
					t.Errorf("%q %d px: надпись при статусе %v отличается от обычной", label, px, st)
				}
				if got := img.RGBAAt(px/2, 1); got != wantBackground[st] {
					t.Errorf("%q %d px %v: фон у верхнего края %v, ожидался %v", label, px, st, got, wantBackground[st])
				}
			}
		}
	}
}

// Ловит: у не детской учётки на значке остаток (таймер) или значок неотличим от пустого
// графитового квадрата; рамка должна стоять на своём месте при каждом масштабе.
func TestRenderIconNotChildHasFrameAndNoLabel(t *testing.T) {
	// отступ рамки от края и толщина рамки, px: 16 -> 3 и 1, 24 -> 4 и 2, 32 -> 6 и 2.
	frame := map[int][2]int{16: {3, 1}, 24: {4, 2}, 32: {6, 2}}
	for _, px := range iconSizes {
		img := RenderIcon("85", StatusNotChild, px)
		m, th := frame[px][0], frame[px][1]
		x0, y0, x1, y1, ok := inkBox(img)
		if !ok {
			t.Fatalf("%d px: рамки нет", px)
		}
		if x0 != m || y0 != m || x1 != px-1-m || y1 != px-1-m {
			t.Errorf("%d px: рамка занимает x %d..%d, y %d..%d, ожидалось %d..%d по обеим осям", px, x0, x1, y0, y1, m, px-1-m)
		}
		if img.RGBAAt(m, px/2) != white || img.RGBAAt(m+th-1, px/2) != white {
			t.Errorf("%d px: левая сторона рамки тоньше %d px", px, th)
		}
		if img.RGBAAt(m+th, px/2) == white {
			t.Errorf("%d px: левая сторона рамки толще %d px", px, th)
		}
		if img.RGBAAt(px/2, px/2) != wantBackground[StatusNotChild] {
			t.Errorf("%d px: середина рамки не цвета статуса", px)
		}
		for _, label := range []string{"", "∞", "1ч", "x"} {
			if !bytes.Equal(RenderIcon(label, StatusNotChild, px).Pix, img.Pix) {
				t.Errorf("%d px: надпись %q изменила значок не детской учётки", px, label)
			}
		}
		if bytes.Equal(img.Pix, RenderIcon("", StatusNormal, px).Pix) {
			t.Errorf("%d px: значок не детской учётки не отличается от пустого графитового", px)
		}
	}
}

// Ловит: мусор в надписи роняет значок или рисует за краем (L-37): неизвестные знаки
// пропускаются, слишком длинная надпись укорачивается так, чтобы осталось поле 1 px.
func TestRenderIconHostileLabels(t *testing.T) {
	blank := map[int]string{}
	for _, px := range iconSizes {
		blank[px] = string(RenderIcon("", StatusNormal, px).Pix)
	}
	onlyUnknown := []string{"x", "abc", "ё", "\x00", " ", "💥", "\xff\xfe", "?!", "-"}
	long := []string{"12345", strings.Repeat("9", 100), "∞∞∞", "1ч1ч1ч", "99ччч"}
	for _, px := range iconSizes {
		for _, label := range onlyUnknown {
			img := RenderIcon(label, StatusNormal, px)
			if string(img.Pix) != blank[px] {
				t.Errorf("%d px: неизвестная надпись %q должна давать пустой значок", px, label)
			}
		}
		for _, label := range long {
			img := RenderIcon(label, StatusNormal, px)
			x0, y0, x1, y1, ok := inkBox(img)
			if !ok {
				t.Errorf("%d px: длинная надпись %q не нарисована", px, label)
				continue
			}
			if x0 < 1 || y0 < 1 || x1 > px-2 || y1 > px-2 {
				t.Errorf("%d px: длинная надпись %q вышла за поле: x %d..%d, y %d..%d", px, label, x0, x1, y0, y1)
			}
			if n := inkColumnGroups(img); n > 3 {
				t.Errorf("%d px: у длинной надписи %q видно %d знаков, предел 3", px, label, n)
			}
		}
		mixed := RenderIcon("8x5", StatusNormal, px)
		if string(mixed.Pix) != string(RenderIcon("85", StatusNormal, px).Pix) {
			t.Errorf("%d px: неизвестный знак внутри надписи должен пропускаться без сдвига остальных", px)
		}
	}
}

// Ловит: размер вне 16, 24, 32 (Windows при 125 и 175 % просит 20 и 28 px) даёт nil,
// нулевую картинку или панику; берётся ближайший поддерживаемый размер, при равенстве больший.
func TestRenderIconUnsupportedSizeSnaps(t *testing.T) {
	cases := []struct{ ask, want int }{
		{-5, 16}, {0, 16}, {1, 16}, {16, 16}, {19, 16}, {20, 24}, {24, 24}, {27, 24},
		{28, 32}, {32, 32}, {40, 32}, {1000, 32},
	}
	for _, tc := range cases {
		img := RenderIcon("85", StatusNormal, tc.ask)
		if img == nil {
			t.Errorf("RenderIcon(px=%d) = nil", tc.ask)
			continue
		}
		if img.Bounds().Dx() != tc.want || img.Bounds().Dy() != tc.want {
			t.Errorf("RenderIcon(px=%d): размер %v, ожидалось %d×%d", tc.ask, img.Bounds(), tc.want, tc.want)
		}
	}
}

// Ловит: статус вне списка рисуется прозрачным значком; он должен выглядеть как обычный.
func TestRenderIconUnknownStatusLooksNormal(t *testing.T) {
	for _, px := range iconSizes {
		for _, st := range []Status{Status(77), Status(-1)} {
			if !bytes.Equal(RenderIcon("85", st, px).Pix, RenderIcon("85", StatusNormal, px).Pix) {
				t.Errorf("%d px: статус %d нарисован не как обычный", px, int(st))
			}
		}
	}
}

// Ловит: опечатка в растре шрифта: лишний бит правее ширины знака, не та высота строк,
// не та ширина цифры, знак шрифта, которого нет для одного из масштабов.
func TestFontTablesAreConsistent(t *testing.T) {
	want := map[int]struct{ w, h, gap, scale int }{
		16: {4, 10, 1, 1},
		24: {6, 14, 2, 1},
		32: {4, 10, 1, 2}, // 32 px это целое удвоение набора 16 px
	}
	for _, px := range iconSizes {
		f := faceFor(px)
		w := want[px]
		if f.lineH != w.h || f.gap != w.gap || f.scale != w.scale {
			t.Errorf("%d px: lineH=%d gap=%d scale=%d, ожидалось %d %d %d", px, f.lineH, f.gap, f.scale, w.h, w.gap, w.scale)
		}
		for _, r := range "0123456789ч∞" {
			g, ok := f.glyphs[r]
			if !ok {
				t.Errorf("%d px: в шрифте нет знака %q", px, r)
				continue
			}
			if len(g.rows) == 0 || len(g.rows) > f.lineH {
				t.Errorf("%d px: у знака %q строк %d, строка набора %d", px, r, len(g.rows), f.lineH)
			}
			if r >= '0' && r <= '9' && (len(g.rows) != f.lineH || g.w != w.w) {
				t.Errorf("%d px: цифра %q размером %d×%d, ожидалось %d×%d", px, r, g.w, len(g.rows), w.w, f.lineH)
			}
			for i, row := range g.rows {
				if row>>uint(g.w) != 0 {
					t.Errorf("%d px: у знака %q в строке %d биты правее ширины %d", px, r, i, g.w)
				}
				if row == 0 {
					t.Errorf("%d px: у знака %q пустая строка %d (лишнее место сверху или снизу)", px, r, i)
				}
			}
		}
	}
}

// Ловит: надпись, которую выдаёт TrayLabel, содержит знак, которого нет в шрифте значка,
// и значок молча рисуется без него; каждый остаток от 0 до суток и «∞» рисуется целиком.
func TestEveryTrayLabelIsDrawnCompletely(t *testing.T) {
	check := func(label string) {
		for _, px := range iconSizes {
			img := RenderIcon(label, StatusNormal, px)
			if got, want := inkColumnGroups(img), len([]rune(label)); got != want {
				t.Fatalf("надпись %q при %d px: нарисовано знаков %d из %d", label, px, got, want)
			}
		}
	}
	for m := -5; m <= 1500; m++ {
		check(TrayLabel(m, false))
	}
	check(TrayLabel(0, true))
	check(TrayLabel(1<<40, false))
}

// icoEntry — запись каталога ICO, прочитанная из байтов.
type icoEntry struct {
	w, h, colors, reserved byte
	planes, bpp            uint16
	size, offset           uint32
}

func parseICO(t *testing.T, data []byte) []icoEntry {
	t.Helper()
	if len(data) < 6 {
		t.Fatalf("ICO короче заголовка: %d байт", len(data))
	}
	le := binary.LittleEndian
	if le.Uint16(data[0:2]) != 0 {
		t.Errorf("reserved = %d, ожидался 0", le.Uint16(data[0:2]))
	}
	if le.Uint16(data[2:4]) != 1 {
		t.Errorf("type = %d, ожидалась 1 (значок)", le.Uint16(data[2:4]))
	}
	count := int(le.Uint16(data[4:6]))
	if len(data) < 6+16*count {
		t.Fatalf("каталог на %d записей не помещается в %d байт", count, len(data))
	}
	entries := make([]icoEntry, count)
	for i := range entries {
		d := data[6+16*i : 6+16*(i+1)]
		entries[i] = icoEntry{
			w: d[0], h: d[1], colors: d[2], reserved: d[3],
			planes: le.Uint16(d[4:6]), bpp: le.Uint16(d[6:8]),
			size: le.Uint32(d[8:12]), offset: le.Uint32(d[12:16]),
		}
	}
	return entries
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

func sameNRGBA(a, b image.Image) bool {
	if a.Bounds().Dx() != b.Bounds().Dx() || a.Bounds().Dy() != b.Bounds().Dy() {
		return false
	}
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			ca := color.NRGBAModel.Convert(a.At(a.Bounds().Min.X+x, a.Bounds().Min.Y+y))
			cb := color.NRGBAModel.Convert(b.At(b.Bounds().Min.X+x, b.Bounds().Min.Y+y))
			if ca != cb {
				return false
			}
		}
	}
	return true
}

// Ловит: контейнер ICO собран неверно: заголовок, число записей, ширина и высота в
// каталоге, 32 бита на пиксель, размеры и смещения данных (Windows не покажет такой значок),
// внутри не PNG или PNG не тот размер и не те пиксели.
func TestEncodeICOStructure(t *testing.T) {
	var imgs []image.Image
	for _, px := range iconSizes {
		imgs = append(imgs, RenderIcon("85", StatusLow, px))
	}
	data, err := EncodeICO(imgs...)
	if err != nil {
		t.Fatalf("EncodeICO: %v", err)
	}
	entries := parseICO(t, data)
	if len(entries) != 3 {
		t.Fatalf("записей %d, ожидалось 3", len(entries))
	}
	nextOffset := uint32(6 + 16*3)
	for i, e := range entries {
		px := iconSizes[i]
		if int(e.w) != px || int(e.h) != px {
			t.Errorf("запись %d: размер %d×%d, ожидалось %d×%d", i, e.w, e.h, px, px)
		}
		if e.colors != 0 || e.reserved != 0 || e.planes != 1 || e.bpp != 32 {
			t.Errorf("запись %d: colors=%d reserved=%d planes=%d bpp=%d, ожидалось 0 0 1 32", i, e.colors, e.reserved, e.planes, e.bpp)
		}
		if e.offset != nextOffset {
			t.Errorf("запись %d: смещение %d, ожидалось %d (данные подряд после каталога)", i, e.offset, nextOffset)
		}
		end := uint64(e.offset) + uint64(e.size)
		if e.size == 0 || end > uint64(len(data)) {
			t.Fatalf("запись %d: данные [%d, %d) выходят за %d байт", i, e.offset, end, len(data))
		}
		chunk := data[e.offset:end]
		if !bytes.HasPrefix(chunk, pngSignature) {
			t.Errorf("запись %d: по смещению %d нет сигнатуры PNG", i, e.offset)
			continue
		}
		dec, err := png.Decode(bytes.NewReader(chunk))
		if err != nil {
			t.Errorf("запись %d: png.Decode: %v", i, err)
			continue
		}
		if dec.Bounds().Dx() != px || dec.Bounds().Dy() != px {
			t.Errorf("запись %d: PNG размером %v, ожидалось %d×%d", i, dec.Bounds(), px, px)
		}
		if !sameNRGBA(dec, imgs[i]) {
			t.Errorf("запись %d: пиксели PNG не совпали с исходной картинкой", i)
		}
		nextOffset = uint32(end)
	}
	if int(nextOffset) != len(data) {
		t.Errorf("после последней записи осталось %d лишних байт", len(data)-int(nextOffset))
	}
}

// Ловит: одиночная картинка кладётся не сразу за каталогом (смещение 22) и размер 256
// пишется числом 256 вместо нуля (в ICO ноль означает 256).
func TestEncodeICOSingleAndLarge(t *testing.T) {
	data, err := EncodeICO(RenderIcon("∞", StatusPaused, 16))
	if err != nil {
		t.Fatalf("EncodeICO: %v", err)
	}
	if e := parseICO(t, data); len(e) != 1 || e[0].offset != 22 {
		t.Errorf("одна запись: %+v, смещение должно быть 22", e)
	}

	big := image.NewRGBA(image.Rect(0, 0, 256, 256))
	data, err = EncodeICO(big)
	if err != nil {
		t.Fatalf("EncodeICO 256: %v", err)
	}
	e := parseICO(t, data)
	if e[0].w != 0 || e[0].h != 0 {
		t.Errorf("256 px записано как %d×%d, в ICO это 0×0", e[0].w, e[0].h)
	}
	dec, err := png.Decode(bytes.NewReader(data[e[0].offset : e[0].offset+e[0].size]))
	if err != nil || dec.Bounds().Dx() != 256 || dec.Bounds().Dy() != 256 {
		t.Errorf("PNG записи 256: %v, ошибка %v", dec, err)
	}

	wide := image.NewRGBA(image.Rect(0, 0, 32, 16))
	data, err = EncodeICO(wide)
	if err != nil {
		t.Fatalf("EncodeICO 32×16: %v", err)
	}
	if e := parseICO(t, data); e[0].w != 32 || e[0].h != 16 {
		t.Errorf("запись 32×16 в каталоге %d×%d", e[0].w, e[0].h)
	}
}

// Ловит: пустой список, пустая картинка, картинка больше 256 px или нулевого размера
// молча дают битый файл вместо ошибки.
func TestEncodeICOErrors(t *testing.T) {
	cases := []struct {
		name string
		imgs []image.Image
	}{
		{"нет картинок", nil},
		{"пустой список", []image.Image{}},
		{"nil вместо картинки", []image.Image{nil}},
		{"хорошая и nil", []image.Image{RenderIcon("1", StatusNormal, 16), nil}},
		{"шире 256", []image.Image{image.NewRGBA(image.Rect(0, 0, 257, 16))}},
		{"выше 256", []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 257))}},
		{"нулевой размер", []image.Image{image.NewRGBA(image.Rect(0, 0, 0, 0))}},
		{"нулевая высота", []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 0))}},
	}
	for _, tc := range cases {
		data, err := EncodeICO(tc.imgs...)
		if err == nil {
			t.Errorf("%s: ожидалась ошибка, получено %d байт", tc.name, len(data))
		}
		if data != nil {
			t.Errorf("%s: при ошибке должен возвращаться nil, получено %d байт", tc.name, len(data))
		}
	}
}
