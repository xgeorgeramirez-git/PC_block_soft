package ipc

import (
	"encoding/json"
	"strings"
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/sites"
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

// Ловит (S-3): день без ключа videoUnlimited (старый фронт, мастер, PowerShell-приёмка) читается
// как «видео нельзя» — после первого «Сохранить» в «Лимитах» видео закрыто каждый день.
func TestWeekdayLimitVideoDefault(t *testing.T) {
	cases := []struct {
		in      string
		wantMin int
		wantUnl bool
	}{
		{`{"weekday":1,"totalMin":60,"unlimited":false}`, 0, true},
		{`{"weekday":1,"totalMin":60,"unlimited":false,"videoMin":30,"videoUnlimited":false}`, 30, false},
		{`{"weekday":1,"videoMin":30}`, 30, true},
	}
	for _, c := range cases {
		var w WeekdayLimit
		if err := json.Unmarshal([]byte(c.in), &w); err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if w.VideoMin != c.wantMin || w.VideoUnlimited != c.wantUnl || w.Weekday != 1 {
			t.Errorf("%s: %+v, want videoMin %d, videoUnlimited %v", c.in, w, c.wantMin, c.wantUnl)
		}
	}
	var week []WeekdayLimit
	if err := json.Unmarshal([]byte(`[{"weekday":2,"totalMin":90},{"weekday":3}]`), &week); err != nil || len(week) != 2 ||
		!week[0].VideoUnlimited || !week[1].VideoUnlimited || week[0].TotalMin != 90 {
		t.Errorf("массив дней: %+v, %v", week, err)
	}
	if err := json.Unmarshal([]byte(`{"weekday":"пн"}`), new(WeekdayLimit)); err == nil {
		t.Error("неверный тип поля разобран без ошибки")
	}
	for _, w := range []WeekdayLimit{
		{Weekday: 1, TotalMin: 120, GamesMin: 60, VideoMin: 30},
		{Weekday: 7, Unlimited: true, GamesUnlimited: true, VideoUnlimited: true},
		{Weekday: 3, TotalMin: 90, GamesUnlimited: true, VideoMin: 0},
	} {
		var back WeekdayLimit
		if err := json.Unmarshal(Payload(w), &back); err != nil || back != w {
			t.Errorf("круг JSON: %+v -> %+v (%v)", w, back, err)
		}
	}
}

// Ловит: предел длины домена канала разошёлся с пакетом сайтов (служба отбрасывала бы домены,
// которые пакет считает верными, или наоборот); стартовый набор «Видео и соцсетей» не тот, что R14
// и миграция 009.
func TestMaxDomainLenMatchesSites(t *testing.T) {
	if MaxDomainLen != sites.MaxDomainLen {
		t.Errorf("ipc.MaxDomainLen %d, sites.MaxDomainLen %d", MaxDomainLen, sites.MaxDomainLen)
	}
	const r14 = "youtube.com vk.com tiktok.com rutube.ru ok.ru telegram.org web.telegram.org discord.com twitch.tv"
	if got := strings.Join(DefaultVideoSites, " "); got != r14 {
		t.Errorf("DefaultVideoSites %q, want %q", got, r14)
	}
	if MaxBlockedSites != 1000 {
		t.Errorf("MaxBlockedSites %d, want 1000", MaxBlockedSites)
	}
}
