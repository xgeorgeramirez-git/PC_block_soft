package guard

import "testing"

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
