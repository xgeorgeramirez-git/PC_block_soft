package core

import (
	"testing"
)

// Тесты M4 ядра: лимит «Видео и соцсети» (R6) по образцу лимита игр (R5): остаток, доп. время,
// закрытие видео, незасчитанная секунда (RM3-7), тексты плашки (8.2.5 п. 3), пороги R9 только
// при видео на переднем плане (PL-14). Ожидания — литералы, посчитанные вручную.

// videoMon — понедельник: общий лимит total мин (nil — без лимита), лимит игр games и лимит
// видео video (nil — без отдельного лимита), режим окончания mode; остальные дни без лимитов.
func videoMon(total, games, video *int, mode EndMode) Rules {
	var r Rules
	r.Week[0] = DayLimit{TotalMin: total, GamesMin: games, VideoMin: video}
	r.EndMode = mode
	return r
}

func videoSample() SessionSample {
	return SessionSample{SessionActive: true, LastInputAgoMs: 1000, Foreground: CatVideoSocial}
}

// Ловит: остаток видео без доп. времени, отрицательный остаток, «без лимита видео» даёт лимит,
// лимит 0 не даёт нулевой остаток.
func TestRemainingVideoS(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lim           DayLimit
		extra, videoS int
		rem           int
		unl           bool
	}{
		{"нет лимита видео", DayLimit{TotalMin: intp(120)}, 30, 100, 0, true},
		{"20 мин, 5 мин видео", DayLimit{VideoMin: intp(20)}, 0, 300, 900, false},
		{"20 + 30, 5 мин видео", DayLimit{VideoMin: intp(20)}, 30, 300, 2700, false},
		{"0 мин", DayLimit{VideoMin: intp(0)}, 0, 0, 0, false},
		{"0 + 30", DayLimit{VideoMin: intp(0)}, 30, 0, 1800, false},
		{"перебор", DayLimit{VideoMin: intp(10)}, -5, 900, 0, false},
	} {
		rem, unl := RemainingVideoS(tc.lim, tc.extra, tc.videoS)
		if rem != tc.rem || unl != tc.unl {
			t.Errorf("%s: (%d, %v), want (%d, %v)", tc.name, rem, unl, tc.rem, tc.unl)
		}
	}
}

// Ловит: ключ вида события видео расходится с каналом (T3) или сдвигает ключ игр.
func TestWarnVideoString(t *testing.T) {
	if WarnVideo.String() != "video" || WarnGames.String() != "games" {
		t.Fatalf("ключи: %q %q", WarnVideo, WarnGames)
	}
}
