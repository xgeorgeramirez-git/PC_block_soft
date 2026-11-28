package guard

import "math"

// Рисунок плашки «Игрового лобби» (plaque.go) без текста: сплошная тень, панель с обводкой и бликом,
// золотая кнопка, крестик с белым «✕». Чистый Go тем же способом, что окно сообщения (noterender.go):
// сглаженные края по расстоянию до контура, BGRA с предумноженной прозрачностью, строки сверху вниз;
// вне панели и тени альфа 0 — там клики проходят к окнам под плашкой. Текст рисует окно шрифтом
// системы поверх основы (plaque_windows.go), кадр стикера кладёт PlaqueStickerOver.

// plaqueInset — прямоугольник r, ужатый на d со всех сторон.
func plaqueInset(r Rect, d int32) Rect {
	return Rect{Left: r.Left + d, Top: r.Top + d, Right: r.Right - d, Bottom: r.Bottom - d}
}

// plaqueShift — прямоугольник r, сдвинутый на dy вниз.
func plaqueShift(r Rect, dy int32) Rect {
	r.Top, r.Bottom = r.Top+dy, r.Bottom+dy
	return r
}

// plaqueInsetBand кладёт цвет c с прозрачностью a полосой высотой band внутри скруглённого r: сверху
// (band > 0, «inset 0 3px») или снизу (band < 0, «inset 0 -6px») — внутри r и вне r, сдвинутого на band.
func plaqueInsetBand(buf []byte, w, h int32, r Rect, rad, band int32, c uint32, a float64) {
	moved := plaqueShift(r, band)
	notePaint(buf, w, h, r, c, a, func(x, y float64) float64 {
		return noteCover(noteSDF(x, y, r, float64(rad))) * (1 - noteCover(noteSDF(x, y, moved, float64(rad))))
	})
}

// plaqueLerp — цвет между a и b в доле t (0…1).
func plaqueLerp(a, b uint32, t float64) uint32 {
	t = math.Max(0, math.Min(1, t))
	ch := func(s uint) uint32 {
		x, y := float64(a>>s&0xFF), float64(b>>s&0xFF)
		return uint32(math.Round(x+(y-x)*t)) << s
	}
	return ch(16) | ch(8) | ch(0)
}

// plaqueSegDist — расстояние от точки до отрезка (x0, y0)–(x1, y1).
func plaqueSegDist(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	t := math.Max(0, math.Min(1, ((px-x0)*dx+(py-y0)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

// RenderPlaqueBase — основа плашки l в новом буфере (RenderPlaqueInto).
func RenderPlaqueBase(l PlaqueLayout, hotBtn, hotClose bool) []byte {
	buf := make([]byte, int(max(l.W, 0))*int(max(l.H, 0))*4)
	RenderPlaqueInto(buf, l, hotBtn, hotClose)
	return buf
}

// RenderPlaqueInto рисует основу плашки l прямо в буфер окна dst (DIB): тень (панель, сдвинутая вниз, #0B1D5C), панель (рамка
// #0B1D5C, фон #132C8A, блик сверху #3A5DD6), кнопку «Попросить сейчас», если есть (тень, рамка,
// градиент #FFE066→#FFB800, нижняя полоса #D98200, белый блик; hotBtn — светлее), и крестик (рамка,
// #22388F, под мышью hotClose — светлее, белый «✕»). Буфер l.W×l.H, BGRA с предумноженной прозрачностью;
// прежнее содержимое стирается. Отдельной копии основы окно не держит (NFR-5): прозрачность после
// текста возвращает PlaqueFixTextAlpha, под стикером — заплатка PlaquePatch. false — буфер не размера окна.
func RenderPlaqueInto(buf []byte, l PlaqueLayout, hotBtn, hotClose bool) bool {
	w, h := l.W, l.H
	if w <= 0 || h <= 0 || len(buf) != int(w)*int(h)*4 {
		return false
	}
	clear(buf)
	noteFillRound(buf, w, h, l.Shadow, l.Radius, PlaqueInkRGB)
	noteFillRound(buf, w, h, l.Panel, l.Radius, PlaqueInkRGB)
	inner := plaqueInset(l.Panel, l.Border)
	innerRad := max(0, l.Radius-l.Border)
	noteFillRound(buf, w, h, inner, innerRad, PlaqueBgRGB)
	plaqueInsetBand(buf, w, h, inner, innerRad, l.Hi, PlaqueHiRGB, 1)

	if b := l.Btn; b != (Rect{}) {
		noteFillRound(buf, w, h, plaqueShift(b, l.BtnShadow), l.BtnRadius, PlaqueInkRGB)
		noteFillRound(buf, w, h, b, l.BtnRadius, PlaqueInkRGB)
		bi := plaqueInset(b, l.Border)
		rad := max(0, l.BtnRadius-l.Border)
		top, mid := uint32(plaqueBtnTop), uint32(plaqueBtnMid)
		if hotBtn {
			top, mid = plaqueBtnHotTop, plaqueBtnHotMid
		}
		cover := func(x, y float64) float64 { return noteCover(noteSDF(x, y, bi, float64(rad))) }
		span := math.Max(1, 0.55*float64(bi.Bottom-bi.Top))
		for y := bi.Top - 1; y <= bi.Bottom; y++ {
			c := plaqueLerp(top, mid, (float64(y-bi.Top)+0.5)/span)
			notePaint(buf, w, h, Rect{Left: bi.Left - 1, Top: y, Right: bi.Right + 1, Bottom: y + 1}, c, 1, cover)
		}
		plaqueInsetBand(buf, w, h, bi, rad, -l.BtnStrip, plaqueBtnLow, 1)
		plaqueInsetBand(buf, w, h, bi, rad, l.Hi, 0xFFFFFF, 0.55)
	}

	c := l.Close
	noteFillRound(buf, w, h, c, l.CloseRadius, PlaqueInkRGB)
	fill := uint32(PlaqueCloseRGB)
	if hotClose {
		fill = plaqueCloseHot
	}
	noteFillRound(buf, w, h, plaqueInset(c, l.Border), max(0, l.CloseRadius-l.Border), fill)
	// «✕»: две линии через центр, плечо 0,15 стороны, толщина 0,08 стороны (шрифт 16, жирность 900).
	side := float64(c.Right - c.Left)
	cx, cy := float64(c.Left+c.Right)/2, float64(c.Top+c.Bottom)/2
	arm, half := 0.15*side, math.Max(0.75, 0.04*side)
	notePaint(buf, w, h, c, 0xFFFFFF, 1, func(x, y float64) float64 {
		d := math.Min(plaqueSegDist(x, y, cx-arm, cy-arm, cx+arm, cy+arm), plaqueSegDist(x, y, cx-arm, cy+arm, cx+arm, cy-arm))
		return noteCover(d - half)
	})
	return true
}

// plaqueTextPad — на сколько текст шрифта системы может выйти за свой прямоугольник: обводка
// заголовка путём и его тень 2 px вниз (96 dpi).
const plaqueTextPad = 4

// PlaqueFixTextAlpha возвращает непрозрачность пикселям текста в буфере окна dst: шрифт системы в DIB
// пишет альфу 0, а под текстом всегда непрозрачная панель. Правятся только места текста (заголовок,
// текст, кнопка) с запасом plaqueTextPad и только внутри панели: прозрачные углы окна не трогаются.
func PlaqueFixTextAlpha(dst []byte, l PlaqueLayout, dpi int) {
	if len(dst) != int(l.W)*int(l.H)*4 {
		return
	}
	pad := Scaled(plaqueTextPad, dpi)
	for _, r := range []Rect{l.Title, l.Text, l.Btn} {
		if r == (Rect{}) {
			continue
		}
		x0, y0 := max(r.Left-pad, 0), max(r.Top-pad, 0)
		x1, y1 := min(r.Right+pad, l.W), min(r.Bottom+pad, l.H)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if noteSDF(float64(x)+0.5, float64(y)+0.5, l.Panel, float64(l.Radius)) <= -0.5 {
					dst[int((y*l.W+x)*4)+3] = 255
				}
			}
		}
	}
}

// PlaquePatch — копия пикселей основы под стикером (сторона места стикера): её кладёт PlaqueStickerOver
// под каждый кадр. nil — буфер не размера окна.
func PlaquePatch(base []byte, l PlaqueLayout) []byte {
	s := l.Sticker
	side := int(s.Right - s.Left)
	if side <= 0 || len(base) != int(l.W)*int(l.H)*4 || s.Left < 0 || s.Top < 0 || s.Right > l.W || s.Bottom > l.H {
		return nil
	}
	out := make([]byte, side*side*4)
	for y := 0; y < side; y++ {
		i := int(((s.Top+int32(y))*l.W + s.Left) * 4)
		copy(out[y*side*4:(y+1)*side*4], base[i:i+side*4])
	}
	return out
}

// PlaqueStickerOver обновляет в буфере окна dst прямоугольник стикера l.Sticker: кладёт заплатку
// основы patch (PlaquePatch: панель под стикером) и поверх кадр frame стороной framePx (BGRA с
// предумноженной прозрачностью). Кадр крупнее места (300 px) уменьшается усреднением блока, альфа
// сохраняется. Вне прямоугольника стикера dst не меняется: окну хватает обновить только его (prcDirty).
// false — буфер не размера окна, заплатка или кадр не той длины, кадр меньше места.
func PlaqueStickerOver(dst, patch []byte, l PlaqueLayout, frame []byte, framePx int) bool {
	s := l.Sticker
	side := int(s.Right - s.Left)
	if side <= 0 || len(dst) != int(l.W)*int(l.H)*4 || len(patch) != side*side*4 || framePx < side || len(frame) != framePx*framePx*4 ||
		s.Left < 0 || s.Top < 0 || s.Right > l.W || s.Bottom > l.H {
		return false
	}
	x0, y0, x1, y1 := s.Left, s.Top, s.Right, s.Bottom
	for y := y0; y < y1; y++ {
		i := int((y*l.W + x0) * 4)
		copy(dst[i:i+side*4], patch[int(y-y0)*side*4:int(y-y0+1)*side*4])
	}
	for y := y0; y < y1; y++ {
		fy := int(y - s.Top)
		sy0, sy1 := fy*framePx/side, max((fy+1)*framePx/side, fy*framePx/side+1)
		for x := x0; x < x1; x++ {
			fx := int(x - s.Left)
			sx0, sx1 := fx*framePx/side, max((fx+1)*framePx/side, fx*framePx/side+1)
			var px [4]uint32
			n := uint32(0)
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					f := (yy*framePx + xx) * 4
					for k := 0; k < 4; k++ {
						px[k] += uint32(frame[f+k])
					}
					n++
				}
			}
			sa := px[3] / n
			if sa == 0 {
				continue
			}
			d := int((y*l.W + x) * 4)
			rest := 255 - sa
			for k := 0; k < 4; k++ {
				dst[d+k] = byte(min(255, px[k]/n+(uint32(dst[d+k])*rest+127)/255))
			}
		}
	}
	return true
}
