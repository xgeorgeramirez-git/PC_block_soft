package systray

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Размеры значка в пикселях: масштаб Windows 100, 150 и 200 % (NFR-14, FR-7).
const (
	size100 = 16
	size150 = 24
	size200 = 32
)

// iconMargin — поле от края значка до надписи, px (L-10, L-11: надпись не обрезается и не прижата).
const iconMargin = 1

// glyph — знак растрового шрифта: w столбцов, строки сверху вниз, старший бит слева.
// Строка знака ниже строки набора (lineH) прижимается к низу, как «ч» к цифрам;
// mid=true выравнивает знак по центру строки (для «∞»).
type glyph struct {
	w    int
	mid  bool
	rows []uint32
}

// face — набор знаков одного размера значка. Растр рисуется целыми пикселями, без
// сглаживания; scale — целое увеличение (32 px это удвоенный набор 16 px).
type face struct {
	glyphs map[rune]glyph
	lineH  int // высота цифр в пикселях растра
	gap    int // промежуток между знаками в пикселях растра
	scale  int
}

// Набор 16 px: цифры 4×10, «ч» 4×8, «∞» 12×8, штрих 1 px. Три знака («24ч») занимают
// ровно 14 px при поле 1 px с каждой стороны.
var glyphs16 = map[rune]glyph{
	'0': {w: 4, rows: []uint32{
		0b0110, // .XX.
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b0110, // .XX.
	}},
	'1': {w: 4, rows: []uint32{
		0b0010, // ..X.
		0b0110, // .XX.
		0b1010, // X.X.
		0b0010, // ..X.
		0b0010, // ..X.
		0b0010, // ..X.
		0b0010, // ..X.
		0b0010, // ..X.
		0b0010, // ..X.
		0b1111, // XXXX
	}},
	'2': {w: 4, rows: []uint32{
		0b0110, // .XX.
		0b1001, // X..X
		0b0001, // ...X
		0b0001, // ...X
		0b0010, // ..X.
		0b0100, // .X..
		0b1000, // X...
		0b1000, // X...
		0b1000, // X...
		0b1111, // XXXX
	}},
	'3': {w: 4, rows: []uint32{
		0b0110, // .XX.
		0b1001, // X..X
		0b0001, // ...X
		0b0001, // ...X
		0b0110, // .XX.
		0b0001, // ...X
		0b0001, // ...X
		0b0001, // ...X
		0b1001, // X..X
		0b0110, // .XX.
	}},
	'4': {w: 4, rows: []uint32{
		0b0001, // ...X
		0b0011, // ..XX
		0b0101, // .X.X
		0b1001, // X..X
		0b1001, // X..X
		0b1111, // XXXX
		0b0001, // ...X
		0b0001, // ...X
		0b0001, // ...X
		0b0001, // ...X
	}},
	'5': {w: 4, rows: []uint32{
		0b1111, // XXXX
		0b1000, // X...
		0b1000, // X...
		0b1000, // X...
		0b1110, // XXX.
		0b0001, // ...X
		0b0001, // ...X
		0b0001, // ...X
		0b1001, // X..X
		0b0110, // .XX.
	}},
	'6': {w: 4, rows: []uint32{
		0b0110, // .XX.
		0b1001, // X..X
		0b1000, // X...
		0b1000, // X...
		0b1110, // XXX.
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b0110, // .XX.
	}},
	'7': {w: 4, rows: []uint32{
		0b1111, // XXXX
		0b0001, // ...X
		0b0001, // ...X
		0b0010, // ..X.
		0b0010, // ..X.
		0b0100, // .X..
		0b0100, // .X..
		0b0100, // .X..
		0b0100, // .X..
		0b0100, // .X..
	}},
	'8': {w: 4, rows: []uint32{
		0b0110, // .XX.
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b0110, // .XX.
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b0110, // .XX.
	}},
	'9': {w: 4, rows: []uint32{
		0b0110, // .XX.
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b0111, // .XXX
		0b0001, // ...X
		0b0001, // ...X
		0b0001, // ...X
		0b1001, // X..X
		0b0110, // .XX.
	}},
	'ч': {w: 4, rows: []uint32{
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b1001, // X..X
		0b0111, // .XXX
		0b0001, // ...X
		0b0001, // ...X
		0b0001, // ...X
	}},
	'∞': {w: 12, mid: true, rows: []uint32{
		0b011000000110, // .XX......XX.
		0b100100001001, // X..X....X..X
		0b100010010001, // X...X..X...X
		0b100001100001, // X....XX....X
		0b100001100001, // X....XX....X
		0b100010010001, // X...X..X...X
		0b100100001001, // X..X....X..X
		0b011000000110, // .XX......XX.
	}},
}

// Набор 24 px: цифры 6×14, «ч» 6×12, «∞» 18×12, штрих 2 px. Три знака занимают 22 px.
var glyphs24 = map[rune]glyph{
	'0': {w: 6, rows: []uint32{
		0b011110, // .XXXX.
		0b111111, // XXXXXX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011110, // .XXXX.
	}},
	'1': {w: 6, rows: []uint32{
		0b001100, // ..XX..
		0b011100, // .XXX..
		0b111100, // XXXX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b001100, // ..XX..
		0b111111, // XXXXXX
		0b111111, // XXXXXX
	}},
	'2': {w: 6, rows: []uint32{
		0b011110, // .XXXX.
		0b111111, // XXXXXX
		0b110011, // XX..XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000111, // ...XXX
		0b001110, // ..XXX.
		0b011100, // .XXX..
		0b111000, // XXX...
		0b110000, // XX....
		0b110000, // XX....
		0b110000, // XX....
		0b111111, // XXXXXX
		0b111111, // XXXXXX
	}},
	'3': {w: 6, rows: []uint32{
		0b011110, // .XXXX.
		0b111111, // XXXXXX
		0b110011, // XX..XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b001110, // ..XXX.
		0b001110, // ..XXX.
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011110, // .XXXX.
	}},
	'4': {w: 6, rows: []uint32{
		0b000011, // ....XX
		0b000111, // ...XXX
		0b001111, // ..XXXX
		0b011011, // .XX.XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b111111, // XXXXXX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
	}},
	'5': {w: 6, rows: []uint32{
		0b111111, // XXXXXX
		0b111111, // XXXXXX
		0b110000, // XX....
		0b110000, // XX....
		0b110000, // XX....
		0b111110, // XXXXX.
		0b111111, // XXXXXX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011110, // .XXXX.
	}},
	'6': {w: 6, rows: []uint32{
		0b011110, // .XXXX.
		0b111111, // XXXXXX
		0b110011, // XX..XX
		0b110000, // XX....
		0b110000, // XX....
		0b111110, // XXXXX.
		0b111111, // XXXXXX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011110, // .XXXX.
	}},
	'7': {w: 6, rows: []uint32{
		0b111111, // XXXXXX
		0b111111, // XXXXXX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000111, // ...XXX
		0b000110, // ...XX.
		0b001110, // ..XXX.
		0b001100, // ..XX..
		0b011100, // .XXX..
		0b011000, // .XX...
		0b011000, // .XX...
		0b011000, // .XX...
		0b011000, // .XX...
		0b011000, // .XX...
	}},
	'8': {w: 6, rows: []uint32{
		0b011110, // .XXXX.
		0b111111, // XXXXXX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b011110, // .XXXX.
		0b011110, // .XXXX.
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011110, // .XXXX.
	}},
	'9': {w: 6, rows: []uint32{
		0b011110, // .XXXX.
		0b111111, // XXXXXX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011111, // .XXXXX
		0b000011, // ....XX
		0b000011, // ....XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011110, // .XXXX.
	}},
	'ч': {w: 6, rows: []uint32{
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b110011, // XX..XX
		0b111111, // XXXXXX
		0b011111, // .XXXXX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
		0b000011, // ....XX
	}},
	'∞': {w: 18, mid: true, rows: []uint32{
		0b001110000000011100, // ..XXX........XXX..
		0b011111000000111110, // .XXXXX......XXXXX.
		0b111011100001110111, // XXX.XXX....XXX.XXX
		0b110001110011100011, // XX...XXX..XXX...XX
		0b110000111111000011, // XX....XXXXXX....XX
		0b110000011110000011, // XX.....XXXX.....XX
		0b110000011110000011, // XX.....XXXX.....XX
		0b110000111111000011, // XX....XXXXXX....XX
		0b110001110011100011, // XX...XXX..XXX...XX
		0b111011100001110111, // XXX.XXX....XXX.XXX
		0b011111000000111110, // .XXXXX......XXXXX.
		0b001110000000011100, // ..XXX........XXX..
	}},
}

var (
	face16 = face{glyphs: glyphs16, lineH: 10, gap: 1, scale: 1}
	face24 = face{glyphs: glyphs24, lineH: 14, gap: 2, scale: 1}
	face32 = face{glyphs: glyphs16, lineH: 10, gap: 1, scale: 2}
)

// snapSize приводит любой запрошенный размер к ближайшему из 16, 24, 32; при равном
// расстоянии берётся больший (20 даёт 24, 28 даёт 32). Нуль и отрицательное дают 16.
func snapSize(px int) int {
	switch {
	case px < (size100+size150)/2:
		return size100
	case px < (size150+size200)/2:
		return size150
	default:
		return size200
	}
}

func faceFor(px int) face {
	switch snapSize(px) {
	case size100:
		return face16
	case size150:
		return face24
	default:
		return face32
	}
}

// RenderIcon рисует значок у часов: квадрат цвета статуса со скруглёнными углами
// (радиус px/8, углы прозрачные) и белая надпись по центру. Допустимы размеры 16, 24 и 32;
// другой размер заменяется ближайшим допустимым, и картинка возвращается уже этого
// размера. Знаки, которых нет в шрифте (цифры, «ч», «∞»), пропускаются; надпись, не
// помещающаяся с полем 1 px, укорачивается справа. У не детской учётки (StatusNotChild)
// надписи нет: графитовый квадрат с белой рамкой, чтобы значок не казался таймером.
func RenderIcon(label string, st Status, px int) *image.RGBA {
	px = snapSize(px)
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	fillRounded(img, st.Color(), px/8)
	if st == StatusNotChild {
		drawFrame(img)
		return img
	}
	drawLabel(img, label)
	return img
}

// fillRounded закрашивает квадрат цветом c; в каждом углу пиксели вне круга радиуса r
// остаются прозрачными. Края резкие: пиксель или закрашен полностью, или прозрачный.
func fillRounded(img *image.RGBA, c color.RGBA, r int) {
	n := img.Bounds().Dx()
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if insideRounded(x, y, n, r) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// insideRounded — лежит ли пиксель (x, y) квадрата n×n внутри скруглённого квадрата
// с радиусом угла r. Углы сводятся к левому верхнему; центр пикселя против центра дуги.
func insideRounded(x, y, n, r int) bool {
	if x >= n-r {
		x = n - 1 - x
	}
	if y >= n-r {
		y = n - 1 - y
	}
	if x >= r || y >= r {
		return true
	}
	dx, dy := 2*(r-x)-1, 2*(r-y)-1 // удвоенные расстояния от центров, целые числа
	return dx*dx+dy*dy <= 4*r*r
}

// drawFrame рисует белую рамку не детской учётки: отступ 3/16 стороны, толщина 1, 2, 2 px.
func drawFrame(img *image.RGBA) {
	n := img.Bounds().Dx()
	inset := n * 3 / 16
	thick := (n + 8) / 16
	for y := inset; y < n-inset; y++ {
		for x := inset; x < n-inset; x++ {
			if x < inset+thick || x >= n-inset-thick || y < inset+thick || y >= n-inset-thick {
				img.SetRGBA(x, y, colorText)
			}
		}
	}
}

// labelWidth — ширина надписи из знаков gs в пикселях.
func (f face) labelWidth(gs []glyph) int {
	w := 0
	for i, g := range gs {
		if i > 0 {
			w += f.gap * f.scale
		}
		w += g.w * f.scale
	}
	return w
}

// drawLabel рисует белую надпись по центру значка.
func drawLabel(img *image.RGBA, label string) {
	n := img.Bounds().Dx()
	f := faceFor(n)
	var gs []glyph
	for _, r := range label {
		if g, ok := f.glyphs[r]; ok {
			gs = append(gs, g)
		}
	}
	for len(gs) > 1 && f.labelWidth(gs) > n-2*iconMargin {
		gs = gs[:len(gs)-1]
	}
	if len(gs) == 0 || f.labelWidth(gs) > n-2*iconMargin {
		return
	}
	x := (n - f.labelWidth(gs)) / 2
	lineTop := (n - f.lineH*f.scale) / 2
	for _, g := range gs {
		free := (f.lineH - len(g.rows)) * f.scale // свободное место в строке набора
		y := lineTop + free
		if g.mid {
			y = lineTop + free/2
		}
		drawGlyph(img, g, x, y, f.scale)
		x += (g.w + f.gap) * f.scale
	}
}

// drawGlyph рисует знак с левым верхним углом (x0, y0), каждый бит растра блоком scale×scale.
func drawGlyph(img *image.RGBA, g glyph, x0, y0, scale int) {
	for row, bits := range g.rows {
		for col := 0; col < g.w; col++ {
			if bits>>uint(g.w-1-col)&1 == 0 {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetRGBA(x0+col*scale+dx, y0+row*scale+dy, colorText)
				}
			}
		}
	}
}

// icoMaxSide — наибольшая сторона картинки в каталоге ICO; в байте размера 256 пишется нулём.
const icoMaxSide = 256

// EncodeICO упаковывает картинки в файл ICO с PNG внутри (Windows Vista и новее): заголовок
// (reserved 0, type 1, число записей), каталог по 16 байт на картинку (ширина и высота,
// 0 означает 256; 32 бита на пиксель; размер и смещение данных), затем PNG подряд.
// Ошибка, если картинок нет, картинка пустая (nil) или любая сторона вне 1…256.
func EncodeICO(imgs ...image.Image) ([]byte, error) {
	if len(imgs) == 0 {
		return nil, errors.New("systray: для ICO нужна хотя бы одна картинка")
	}
	if len(imgs) > math.MaxUint16 {
		return nil, fmt.Errorf("systray: в ICO не больше %d картинок, передано %d", math.MaxUint16, len(imgs))
	}
	pngs := make([][]byte, len(imgs))
	for i, img := range imgs {
		if img == nil {
			return nil, fmt.Errorf("systray: картинка %d для ICO пустая", i)
		}
		b := img.Bounds()
		if b.Dx() < 1 || b.Dy() < 1 || b.Dx() > icoMaxSide || b.Dy() > icoMaxSide {
			return nil, fmt.Errorf("systray: картинка %d для ICO размером %d×%d, допустимо от 1×1 до %d×%d",
				i, b.Dx(), b.Dy(), icoMaxSide, icoMaxSide)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("systray: картинка %d для ICO не упаковалась в PNG: %w", i, err)
		}
		pngs[i] = buf.Bytes()
	}

	le := binary.LittleEndian
	head := make([]byte, 6+16*len(imgs))
	le.PutUint16(head[2:], 1) // type: значок
	le.PutUint16(head[4:], uint16(len(imgs)))
	offset := len(head)
	for i, img := range imgs {
		e := head[6+16*i : 6+16*(i+1)]
		e[0] = sideByte(img.Bounds().Dx())
		e[1] = sideByte(img.Bounds().Dy())
		le.PutUint16(e[4:], 1)  // planes
		le.PutUint16(e[6:], 32) // bpp
		le.PutUint32(e[8:], uint32(len(pngs[i])))
		le.PutUint32(e[12:], uint32(offset))
		offset += len(pngs[i])
	}
	out := bytes.NewBuffer(make([]byte, 0, offset))
	out.Write(head)
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes(), nil
}

// sideByte — байт стороны в каталоге ICO: 256 пишется нулём.
func sideByte(side int) byte {
	if side >= icoMaxSide {
		return 0
	}
	return byte(side)
}
