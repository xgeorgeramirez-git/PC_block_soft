package ipc

import (
	"strings"
	"testing"

	"pcblock/internal/core"
)

// Ловит: предупреждения о видео (R6, R9) идут текстами общего лимита или игр («Сохрани игру»), с
// видом события «limit» (страж считал бы плитки по общему остатку) или без звука заказчика (D-55).
func TestGuardWarningVideoTexts(t *testing.T) {
	want := map[int]string{
		30: "Через 30 минут время на видео и соцсети закончится",
		15: "Через 15 минут время на видео и соцсети закончится",
		5:  "Через 5 минут время на видео и соцсети закончится",
		1:  "Осталась 1 минута на видео и соцсети",
	}
	for th, text := range want {
		w := NewGuardWarningFor(th, core.WarnVideo, th*60-7)
		if w.Text != text || w.Event != "video" || !w.Sound || w.Threshold != th || w.EventS != th*60-7 {
			t.Errorf("порог %d: %+v, want текст %q, событие video", th, w, text)
		}
		if strings.ContainsAny(w.Text, "—–") {
			t.Errorf("длинное тире: %q", w.Text)
		}
	}
	if WarnEventVideo != "video" {
		t.Errorf("WarnEventVideo = %q", WarnEventVideo)
	}
}

// Ловит: поля видео не доходят из решения ядра до значка и карточки (одно решение — одни цифры);
// подсказка значка без строки «Видео: 20 мин» (8.2.1 п.2) или с ней, когда отдельного лимита видео
// нет; администратор или не ребёнок получают «видео нельзя».
func TestChildStatusVideoFields(t *testing.T) {
	d := core.Decision{
		Status: core.StatusOK, RemainingS: 85 * 60, LimitS: 7200,
		GamesRemainingS: 19*60 + 5, GamesLimitS: 3600, VideoRemainingS: 19*60 + 5, VideoLimitS: 1200,
	}
	s := ChildStatusFrom("Миша", d)
	if s.VideoRemainingS != 19*60+5 || s.VideoLimitS != 1200 || s.VideoUnlimited || s.CloseVideo {
		t.Fatalf("поля видео: %+v", s)
	}
	if s.Tooltip != "Осталось 1 ч 25 мин\nИгры: 20 мин\nВидео: 20 мин" {
		t.Errorf("подсказка %q", s.Tooltip)
	}

	d.GamesUnlimited, d.GamesRemainingS, d.GamesLimitS = true, 0, 0
	if s := ChildStatusFrom("Миша", d); s.Tooltip != "Осталось 1 ч 25 мин\nВидео: 20 мин" {
		t.Errorf("без лимита игр: подсказка %q", s.Tooltip)
	}

	// Видео сегодня нельзя (лимит 0): ядро отдаёт лимит видео 0 и закрытие видео.
	zero := core.Decision{Status: core.StatusOK, RemainingS: 600, LimitS: 3600, GamesUnlimited: true, CloseVideo: true}
	if s := ChildStatusFrom("Миша", zero); s.Tooltip != "Осталось 10 мин\nВидео: 0 мин" || !s.CloseVideo {
		t.Errorf("видео нельзя: подсказка %q, CloseVideo %v", s.Tooltip, s.CloseVideo)
	}

	d.VideoUnlimited, d.VideoRemainingS, d.VideoLimitS = true, 0, 0
	if s := ChildStatusFrom("Миша", d); s.Tooltip != "Осталось 1 ч 25 мин" || !s.VideoUnlimited {
		t.Errorf("без лимита видео: подсказка %q, VideoUnlimited %v", s.Tooltip, s.VideoUnlimited)
	}

	if !ChildStatusAdmin("Оля").VideoUnlimited {
		t.Error("администратор: VideoUnlimited false")
	}
	if !ChildStatusNotChild().VideoUnlimited {
		t.Error("не ребёнок: VideoUnlimited false")
	}
}

// Ловит: строка видео на карточке «Сегодня» собрана не по образцу игр или есть без отдельного
// лимита видео.
func TestVideoCardText(t *testing.T) {
	cases := []struct {
		used, limit int
		unl         bool
		want        string
	}{
		{600, 1200, false, "Видео 10 мин из 20 мин"},
		{3900, 7200, false, "Видео 1 ч 05 мин из 2 ч"},
		{0, 0, true, ""},
	}
	for _, c := range cases {
		if got := VideoCardText(c.used, c.limit, c.unl); got != c.want {
			t.Errorf("VideoCardText(%d, %d, %v) = %q, want %q", c.used, c.limit, c.unl, got, c.want)
		}
	}
}
