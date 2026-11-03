package guard

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	"image/png"
	"sync"
)

// Картинка экрана «Перерыв на отдых» (8.2.3 п.12): спящий котик, нарисованный для программы
// (assets/rest-cat.svg, PNG делает tools/rest-cat-png.sh). PNG встроен в сборку; окно Windows
// берёт готовые пиксели нужного размера (RestCatBGRA) и только кладёт их на экран (PL-23).

//go:embed assets/rest-cat.png
var restCatPNG []byte

// RestCatPx — размер картинки на экране при 100 % (96 dpi).
const RestCatPx = 160

var (
	restCatOnce sync.Once
	restCatImg  *image.NRGBA
	restCatErr  error
)

// restCat разбирает встроенный PNG один раз.
func restCat() (*image.NRGBA, error) {
	restCatOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(restCatPNG))
		if err != nil {
			restCatErr = err
			return
		}
		n := image.NewNRGBA(img.Bounds())
		draw.Draw(n, n.Bounds(), img, img.Bounds().Min, draw.Src)
		restCatImg = n
	})
	return restCatImg, restCatErr
}

// RestCatBGRA возвращает картинку size×size в пикселях BGRA с предумноженной прозрачностью
// (как ждёт AlphaBlend), строки сверху вниз. Уменьшение — усреднением по площади: линии не
// рвутся. size < 1 — пустой срез.
func RestCatBGRA(size int) ([]byte, error) {
	src, err := restCat()
	if err != nil || size < 1 {
		return []byte{}, err
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	out := make([]byte, size*size*4)
	for y := 0; y < size; y++ {
		y0, y1 := y*sh/size, max((y+1)*sh/size, y*sh/size+1)
		for x := 0; x < size; x++ {
			x0, x1 := x*sw/size, max((x+1)*sw/size, x*sw/size+1)
			var r, g, b, a, n uint32
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					p := src.PixOffset(sx, sy)
					pa := uint32(src.Pix[p+3])
					r += uint32(src.Pix[p]) * pa / 255
					g += uint32(src.Pix[p+1]) * pa / 255
					b += uint32(src.Pix[p+2]) * pa / 255
					a += pa
					n++
				}
			}
			o := (y*size + x) * 4
			out[o], out[o+1], out[o+2], out[o+3] = byte(b/n), byte(g/n), byte(r/n), byte(a/n)
		}
	}
	return out, nil
}
