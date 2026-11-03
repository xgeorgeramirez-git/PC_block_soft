package systray

import (
	"slices"
	"testing"
)

// Плитки табло без карточки (экран перерыва стража, 8.2.3 п.12): тот же расчёт и тот же рисунок,
// что у табло значка, только без фона карточки. Ожидания — литералы при масштабе 1 (×0,8 табло).

// Ловит: плитки экрана перерыва посчитаны своим расчётом (другие размеры и зазоры, чем у табло
// значка), не сдвинуты к началу рисунка (поле под тень потеряно) или размер рисунка не по ним.
func TestTilesLayoutSameAsCard(t *testing.T) {
	l := NewTilesLayout(1)
	want := [6]Rect{{11, 11, 54, 81}, {60, 11, 103, 81}, {135, 11, 178, 81}, {184, 11, 227, 81}, {259, 11, 302, 81}, {308, 11, 351, 81}}
	if l.Tiles != want {
		t.Fatalf("плитки %v, ждали %v", l.Tiles, want)
	}
	if l.Colons != [2]Rect{{103, 11, 135, 81}, {227, 11, 259, 81}} {
		t.Errorf("двоеточия %v", l.Colons)
	}
	if l.Labels != [3]Rect{{11, 92, 103, 118}, {135, 92, 227, 118}, {259, 92, 351, 118}} {
		t.Errorf("подписи %v", l.Labels)
	}
	if l.W != 362 || l.H != 129 || l.Margin != 11 {
		t.Errorf("рисунок %d×%d, поле %d; ждали 362×129, поле 11", l.W, l.H, l.Margin)
	}
	card := NewCardLayout(1)
	for i := range l.Tiles {
		if l.Tiles[i].Dx() != card.Tiles[i].Dx() || l.Tiles[i].Dy() != card.Tiles[i].Dy() {
			t.Errorf("плитка %d размером %d×%d, у табло значка %d×%d", i, l.Tiles[i].Dx(), l.Tiles[i].Dy(), card.Tiles[i].Dx(), card.Tiles[i].Dy())
		}
	}
}

// Ловит: на рисунке плиток остался фон карточки (экран перерыва получил бы синий прямоугольник),
// нет самих плиток или белых двоеточий.
func TestRenderTilesWithoutCard(t *testing.T) {
	l := NewTilesLayout(1)
	img := RenderTiles(l)
	if img.Bounds().Dx() != 362 || img.Bounds().Dy() != 129 {
		t.Fatalf("размер рисунка %v", img.Bounds())
	}
	if c := px(img, 0, 0); c.A != 0 {
		t.Errorf("угол рисунка %v: фон должен быть прозрачным", c)
	}
	if c := px(img, 120, 100); c.A != 0 {
		t.Errorf("под двоеточием между подписями %v: фона карточки быть не должно", c)
	}
	for i, tl := range l.Tiles {
		if c := px(img, (tl.Left+tl.Right)/2, tl.Top+4); c.A != 0xFF || c.R < 0xF0 || c.B < 0xF8 {
			t.Errorf("плитка %d сверху %v: нужна непрозрачная почти белая", i, c)
		}
	}
	for g, c := range l.Colons {
		cx, cy := (c.Left+c.Right)/2, (c.Top+c.Bottom)/2
		if p := px(img, cx, cy-int(cardColonDot*l.Scale)); p.A != 0xFF || p.R != 0xFF {
			t.Errorf("двоеточие %d: верхняя точка %v не белая", g, p)
		}
	}
}

// Ловит: цифры и подписи плиток экрана перерыва не те, что у табло значка (другой размер шрифта,
// цвет, не на своих плитках), или в них попал заголовок карточки.
func TestTileTexts(t *testing.T) {
	l := NewTilesLayout(1)
	v := TimeCountdown(14*60 + 59)
	v.Status = StatusNormal
	runs := TileTexts(l, v)
	texts := []string{}
	for _, r := range runs {
		texts = append(texts, r.Text)
	}
	if !slices.Equal(texts, []string{"0", "0", "1", "4", "5", "9", "часы", "минуты", "секунды"}) {
		t.Fatalf("надписи %v", texts)
	}
	for i := 0; i < 6; i++ {
		r := runs[i]
		if r.Role != RoleDigit || r.Rect != l.Tiles[i] || r.Px != 54 || !r.Bold || r.Color != colorNavy || r.Align != AlignCenter {
			t.Errorf("цифра %d: %+v", i, r)
		}
	}
	for i := 0; i < 3; i++ {
		r := runs[6+i]
		if r.Role != RoleLabel || r.Rect != l.Labels[i] || r.Px != 20 || r.Color != colorLabel {
			t.Errorf("подпись %d: %+v", i, r)
		}
	}
}
