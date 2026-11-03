package systray

import (
	"image"
	"image/color"
	"math"
)

// Плитки табло без карточки: шесть плиток «часы : минуты : секунды», двоеточия и подписи тем же
// расчётом и тем же рисунком, что у табло значка (D-53). Ими страж рисует крупный таймер экрана
// перерыва на отдых (8.2.3 п.12) прямо на тёмном фоне экрана блокировки.

// NewTilesLayout — места плиток, двоеточий и подписей по расчёту NewCardLayoutFor, сдвинутые так,
// что первая плитка начинается в поле Margin (поле под тень). W×H — размер рисунка плиток.
// Остальные места карточки (заголовок, кнопки, строка) пусты.
func NewTilesLayout(scale float64) CardLayout {
	c := NewCardLayout(scale)
	m := c.Margin
	dx, dy := m-c.Tiles[0].Left, m-c.Tiles[0].Top
	move := func(r Rect) Rect { return Rect{r.Left + dx, r.Top + dy, r.Right + dx, r.Bottom + dy} }
	l := CardLayout{Scale: c.Scale, Margin: m}
	for i, t := range c.Tiles {
		l.Tiles[i] = move(t)
	}
	for i, r := range c.Colons {
		l.Colons[i] = move(r)
	}
	for i, r := range c.Labels {
		l.Labels[i] = move(r)
	}
	l.Body = Rect{l.Tiles[0].Left, l.Tiles[0].Top, l.Tiles[5].Right, l.Labels[2].Bottom}
	l.W, l.H = l.Body.Right+m, l.Body.Bottom+m
	return l
}

// RenderTiles рисует плитки с тенями и двоеточия без карточки: за ними прозрачно, прозрачность
// предумножена (как у RenderCardBase).
func RenderTiles(l CardLayout) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, l.W, l.H))
	drawTiles(img, l)
	return img
}

// drawTiles — плитки, их тени и двоеточия: один код у табло значка и экрана перерыва.
func drawTiles(img *image.RGBA, l CardLayout) {
	s := l.Scale
	tileR := 18 * s
	for _, t := range l.Tiles {
		shadowRound(img, t, tileR, 4*s, 9*s, 0.34)
	}
	for _, t := range l.Tiles {
		top, h := float64(t.Top), float64(t.Dy())
		fillRound(img, t, tileR, func(x, y float64) (color.RGBA, float64) {
			return mix(cardTileTop, cardTileBot, (y-top)/h), 1
		})
	}
	for _, c := range l.Colons {
		cx := float64(c.Left+c.Right) / 2
		cy := float64(c.Top+c.Bottom) / 2
		disc(img, cx, cy-cardColonDot*s, cardDotRadius*s, colorWhite, 1)
		disc(img, cx, cy+cardColonDot*s, cardDotRadius*s, colorWhite, 1)
	}
}

// TileTexts — цифры на плитках и подписи «часы», «минуты», «секунды»: те же размеры и цвета,
// что у табло значка.
func TileTexts(l CardLayout, v CountdownView) []TextRun {
	px := func(base int) int { return int(math.Round(float64(base) * l.Scale)) }
	runs := []TextRun{}
	digits := []rune(v.Hours + v.Minutes + v.Seconds)
	for i, d := range digits {
		if i >= len(l.Tiles) {
			break
		}
		runs = append(runs, TextRun{Role: RoleDigit, Text: string(d), Rect: l.Tiles[i], Px: px(68), Bold: true, Color: digitColor(v.Status), Align: AlignCenter})
	}
	for i, name := range []string{textLabelHours, textLabelMinutes, textLabelSeconds} {
		runs = append(runs, TextRun{Role: RoleLabel, Text: name, Rect: l.Labels[i], Px: px(25), Bold: true, Color: colorLabel, Align: AlignCenter})
	}
	return runs
}
