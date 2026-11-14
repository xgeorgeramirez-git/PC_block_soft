package guard

import (
	"bytes"
	"image"
	"image/png"
	"sort"
	"strings"
	"testing"
)

// Стикеры промокода R26 (D-83): 9 удачных и 8 неудачных.
var promoStickerNumbers = []string{
	"017", "023", "028", "033", "044", "048", "049", "050", "069",
	"077", "082", "084", "093", "095", "096", "110", "112",
}

func TestStickerFramesIndexListsPromoStickers(t *testing.T) {
	counts, err := stickerFrameCounts()
	if err != nil {
		t.Fatalf("frames.json: %v", err)
	}
	got := make([]string, 0, len(counts))
	for n := range counts {
		got = append(got, n)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(promoStickerNumbers, ",") {
		t.Fatalf("в frames.json %v, ждали %v", got, promoStickerNumbers)
	}
}

func TestStickerSpritesMatchIndex(t *testing.T) {
	counts, err := stickerFrameCounts()
	if err != nil {
		t.Fatalf("frames.json: %v", err)
	}
	entries, err := stickerFS.ReadDir("assets/stickers")
	if err != nil {
		t.Fatal(err)
	}
	pngs := 0
	total := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			pngs++
		}
	}
	if pngs != 17 {
		t.Fatalf("PNG стикеров: %d, ждали 17", pngs)
	}
	for _, n := range promoStickerNumbers {
		data, err := stickerSprite(n)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		total += len(data)
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s.png не разбирается: %v", n, err)
		}
		frames := counts[n]
		// Одно проигрывание удачи держит экран не дольше 3 с (8.2.3 п.3).
		if frames < 1 || frames > 3*StickerFPS {
			t.Fatalf("%s: кадров %d, ждали 1…%d (3 с при %d кадрах/с)", n, frames, 3*StickerFPS, StickerFPS)
		}
		b := img.Bounds()
		if b.Dx() != frames*StickerFramePx || b.Dy() != StickerFramePx {
			t.Fatalf("%s: %dx%d, ждали %dx300 (%d кадров)", n, b.Dx(), b.Dy(), frames*300, frames)
		}
		checkStickerAlpha(t, n, img, frames)
	}
	if total > 4<<20 {
		t.Fatalf("стикеры вместе %d байт, бюджет 4 МБ (%d)", total, 4<<20)
	}
	t.Logf("стикеры вместе %d байт", total)
}

// checkStickerAlpha: углы первого кадра прозрачные (фон не залит), ни один кадр не пустой.
func checkStickerAlpha(t *testing.T, n string, img image.Image, frames int) {
	t.Helper()
	const s = StickerFramePx
	for _, p := range []image.Point{{0, 0}, {s - 1, 0}, {0, s - 1}, {s - 1, s - 1}} {
		if _, _, _, a := img.At(p.X, p.Y).RGBA(); a != 0 {
			t.Fatalf("%s: угол %v первого кадра не прозрачный (alpha %d)", n, p, a>>8)
		}
	}
	for f := 0; f < frames; f++ {
		if stickerFrameEmpty(img, f*s, s) {
			t.Fatalf("%s: кадр %d целиком прозрачный", n, f)
		}
	}
}

func stickerFrameEmpty(img image.Image, x0, s int) bool {
	for y := 0; y < s; y++ {
		for x := x0; x < x0+s; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				return false
			}
		}
	}
	return true
}

func TestStickerFrameEmptyDetectsBlankFrame(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 6, 3))
	img.Pix[4*4+3] = 255 // пиксель (4,0) — во втором кадре
	if !stickerFrameEmpty(img, 0, 3) {
		t.Fatal("первый кадр пустой, а функция говорит, что нет")
	}
	if stickerFrameEmpty(img, 3, 3) {
		t.Fatal("во втором кадре есть пиксель, а функция говорит, что пусто")
	}
}

func TestStickerSpriteUnknown(t *testing.T) {
	if _, err := stickerSprite("999"); err == nil {
		t.Fatal("нет ошибки для неизвестного стикера")
	}
	if _, err := stickerSprite("../rest-cat"); err == nil {
		t.Fatal("нет ошибки для имени с путём")
	}
}

func TestParseStickerFrames(t *testing.T) {
	got, err := parseStickerFrames([]byte(`{"017": 41, "023": 40}`))
	if err != nil || len(got) != 2 || got["017"] != 41 || got["023"] != 40 {
		t.Fatalf("разбор: %v %v", got, err)
	}
	for _, bad := range []string{`{"017": 0}`, `{"017": -1}`, `[1]`, `{"17": 4}`, `{"01a": 4}`} {
		if _, err := parseStickerFrames([]byte(bad)); err == nil {
			t.Fatalf("%s: нет ошибки", bad)
		}
	}
}
