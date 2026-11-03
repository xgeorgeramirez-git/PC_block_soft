package guard

import "testing"

// Ловит: картинка перерыва не встроена или не разбирается; размер не тот; фон не прозрачный
// (на тёмном экране был бы квадрат); прозрачность не предумножена (AlphaBlend даст светлый ореол).
func TestRestCatImage(t *testing.T) {
	img, err := restCat()
	if err != nil {
		t.Fatalf("PNG не разобран: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 320 {
		t.Fatalf("размер %v, want 320×320", b)
	}
	px, err := RestCatBGRA(RestCatPx)
	if err != nil || len(px) != 160*160*4 {
		t.Fatalf("BGRA %d байт, %v; want %d", len(px), err, 160*160*4)
	}
	if px[3] != 0 || px[0] != 0 {
		t.Fatalf("угол не прозрачный: %v", px[:4])
	}
	opaque := 0
	for i := 0; i < len(px); i += 4 {
		if px[i] > px[i+3] || px[i+1] > px[i+3] || px[i+2] > px[i+3] {
			t.Fatalf("пиксель %d не предумножен: %v", i/4, px[i:i+4])
		}
		if px[i+3] == 255 {
			opaque++
		}
	}
	if opaque < 160*160/5 {
		t.Fatalf("непрозрачных пикселей %d: картинки почти нет", opaque)
	}
	if px, _ := RestCatBGRA(0); len(px) != 0 {
		t.Fatal("размер 0 не пустой")
	}
}
