package systray

import (
	"bytes"
	"image/png"
	"testing"
)

func TestPickSide(t *testing.T) {
	cases := map[int]int{0: 16, 1: 16, 16: 16, 17: 24, 20: 24, 24: 24, 25: 32, 32: 32, 48: 32, 256: 32}
	for px, want := range cases {
		if got := PickSide(px); got != want {
			t.Errorf("PickSide(%d) = %d, ждали %d", px, got, want)
		}
	}
}

// Ловит: из ICO достаётся не та картинка (для CreateIconFromResourceEx нужна ровно одна,
// нужной стороны, целым PNG).
func TestICOImageReturnsRequestedSide(t *testing.T) {
	ico, err := BuildICO("85", StatusNormal)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range IconSides {
		data, err := ICOImage(ico, side)
		if err != nil {
			t.Fatalf("сторона %d: %v", side, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("сторона %d: это не PNG: %v", side, err)
		}
		if b := img.Bounds(); b.Dx() != side || b.Dy() != side {
			t.Fatalf("сторона %d: картинка %dx%d", side, b.Dx(), b.Dy())
		}
		want := RenderIcon("85", StatusNormal, side)
		if !sameNRGBA(img, want) {
			t.Fatalf("сторона %d: пиксели не совпали с RenderIcon", side)
		}
	}
	if _, err := ICOImage(ico, 48); err == nil {
		t.Fatal("картинки 48 в наборе нет, ждали ошибку")
	}
}

func TestICOImageRejectsBrokenFiles(t *testing.T) {
	good, err := BuildICO("1ч", StatusLow)
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string][]byte{
		"пустой":           nil,
		"короче заголовка": {0, 0, 1},
		"не ICO":           append([]byte{0, 0, 2, 0, 1, 0}, make([]byte, 16)...),
		"каталог обрезан":  good[:10],
		"данные обрезаны":  good[:len(good)-5],
	}
	for name, data := range bad {
		if _, err := ICOImage(data, 16); err == nil && name != "данные обрезаны" {
			t.Errorf("%s: ждали ошибку", name)
		}
	}
	// Обрезанный хвост задевает последнюю (32) картинку.
	if _, err := ICOImage(good[:len(good)-5], 32); err == nil {
		t.Error("обрезанная картинка 32: ждали ошибку")
	}
}

// Ловит: значок любого статуса не упаковывается или теряет размеры.
func TestBuildICOAllStatuses(t *testing.T) {
	for _, st := range []Status{StatusNormal, StatusLow, StatusOut, StatusPaused, StatusNotChild} {
		ico, err := BuildICO("0", st)
		if err != nil {
			t.Fatalf("%v: %v", st, err)
		}
		for _, side := range IconSides {
			if _, err := ICOImage(ico, side); err != nil {
				t.Errorf("%v: %v", st, err)
			}
		}
	}
}
