package ui

import (
	"testing"

	"pcblock/internal/launch"
)

// Ловит (R-9, 8.2.6 п. 1): окно «Моя статистика» открывается прежним 800×640, и «Игровое лобби»
// (макет 1040×680) сжимается — плитки уходят в столбец, главная кнопка ниже края; минимум окна
// не должен меняться (640×520, раскладка проверена e2e на этом размере).
func TestSpecForMyStats(t *testing.T) {
	sp := specFor(launch.ModeMyStats)
	if sp.width != 1000 || sp.height != 700 {
		t.Errorf("размер окна «Моя статистика» %d×%d, ждали 1000×700", sp.width, sp.height)
	}
	if sp.minWidth != 640 || sp.minHeight != 520 {
		t.Errorf("минимум окна «Моя статистика» %d×%d, ждали 640×520", sp.minWidth, sp.minHeight)
	}
	if sp.fixed {
		t.Error("окно «Моя статистика» должно менять размер")
	}
	if sp.title != "Моя статистика" {
		t.Errorf("заголовок %q, ждали «Моя статистика»", sp.title)
	}
}
