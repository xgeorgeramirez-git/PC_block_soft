package guard

import (
	"fmt"

	"pcblock/internal/systray"
)

// Экран «Перерыв на отдых» (8.2.3 п.12): картинка, заголовок, крупный таймер плитками табло
// остатка (общий код systray, D-53), «Можно снова в …», сообщение родителя. Решения о размерах и
// перерисовке здесь, окно Windows (lockscreen_windows.go) только кладёт пиксели и текст (PL-23).

const (
	restCatMinPx  = 64 // картинка меньше этого не показывается: на низком экране важнее кнопки
	restCatGapPx  = 12 // от картинки до заголовка
	restMarginPx  = 24 // поле столбца сверху и снизу (как FlowTop экрана блокировки)
	restTilesGap  = 8  // от заголовка до плиток (у плиток своё поле под тень)
	restUntilGap  = 4  // от плиток до «Можно снова в …»
	restMsgGap    = 12 // от «Можно снова» до сообщения родителя
	restMsgLineGp = 2  // между строками сообщения
)

// RestTiles — места плиток таймера при dpi: тот же расчёт, что у табло значка.
func RestTiles(dpi int) systray.CardLayout { return systray.NewTilesLayout(systray.CardScale(dpi)) }

// RestTimerView — табло для плиток таймера: цифры из RestView, спокойный цвет (перерыв — не
// конец времени).
func RestTimerView(r RestView) systray.CountdownView {
	return systray.CountdownView{Kind: systray.CountdownTime, Hours: r.Tiles[0], Minutes: r.Tiles[1], Seconds: r.Tiles[2], Status: systray.StatusNormal}
}

// RestCatSize — сторона картинки в пикселях: 160 px при 100 %. Если столбец экрана без картинки
// (columnH) вместе с ней не помещается в высоту areaH с полями, картинка уменьшается; меньше
// 64 px при 100 % — не показывается (0).
func RestCatSize(dpi int, areaH, columnH int32) int32 {
	free := areaH - 2*Scaled(restMarginPx, dpi) - columnH - Scaled(restCatGapPx, dpi)
	size := min(Scaled(RestCatPx, dpi), free)
	if size < Scaled(restCatMinPx, dpi) {
		return 0
	}
	return size
}

// OverBackground кладёт пиксели BGRA с предумноженной прозрачностью на сплошной фон (r, g, b):
// получаются непрозрачные пиксели для вывода на экран. Исходный срез не меняется.
func OverBackground(bgra []byte, r, g, b byte) []byte {
	out := make([]byte, len(bgra))
	for i := 0; i+3 < len(bgra); i += 4 {
		rest := 255 - uint32(bgra[i+3])
		out[i] = byte(uint32(bgra[i]) + uint32(b)*rest/255)
		out[i+1] = byte(uint32(bgra[i+1]) + uint32(g)*rest/255)
		out[i+2] = byte(uint32(bgra[i+2]) + uint32(r)*rest/255)
		out[i+3] = 255
	}
	return out
}

// RestTimerPixels — рисунок плиток таймера (без цифр) на фоне экрана: непрозрачные BGRA l.W×l.H.
func RestTimerPixels(l systray.CardLayout, r, g, b byte) []byte {
	img := systray.RenderTiles(l)
	bgra := make([]byte, len(img.Pix))
	for i := 0; i+3 < len(img.Pix); i += 4 {
		bgra[i], bgra[i+1], bgra[i+2], bgra[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
	return OverBackground(bgra, r, g, b)
}

// LayoutKey — подпись вида экрана блокировки: окно перестраивается (раскладка, кнопки, фокус),
// только когда она меняется. Плашка — отдельное окно, секунды таймера перерыва перерисовывают
// только плитки (TimerChanged), поэтому в подпись они не входят.
func LayoutKey(v View, notice string) string {
	shown := v
	shown.Warning, shown.Rest = nil, nil
	key := fmt.Sprintf("%+v|%s", shown, notice)
	if v.Rest != nil {
		key += "|rest:" + v.Rest.Message
	}
	return key
}

// TimerChanged: у экрана перерыва сменились цифры таймера.
func TimerChanged(old, now View) bool {
	return old.Rest != nil && now.Rest != nil && old.Rest.Tiles != now.Rest.Tiles
}

// timerText — таймер перерыва словами для отчёта стража: «00:14:59».
func timerText(r *RestView) string {
	if r == nil {
		return ""
	}
	return r.Tiles[0] + ":" + r.Tiles[1] + ":" + r.Tiles[2]
}
