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

// Ловит: доп. время не расширяет лимит видео или убавление уводит его в минус; лимит видео 0
// не закрывает видео с первой секунды, закрывает и общее, засчитывает секунду видео или
// трогает игры; «без лимита видео» что-то закрывает; Broken закрывает видео.
func TestEngineVideoRemaining(t *testing.T) {
	t.Run("20 + 30, 5 мин видео", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(120), nil, intp(20), EndBlock), DayState{Day: monday}, "")
		d, err := e.AddExtra(30)
		if err != nil {
			t.Fatal(err)
		}
		if d.VideoLimitS != 3000 || d.LimitS != 9000 || d.VideoUnlimited {
			t.Fatalf("после +30: лимит видео %d, общий %d, без лимита видео %v; want 3000, 9000, false", d.VideoLimitS, d.LimitS, d.VideoUnlimited)
		}
		run(e, c, at(15, 0, 0), at(15, 5, 0), videoSample())
		cur := e.Current()
		if cur.VideoRemainingS != 2700 || cur.RemainingS != 8700 || cur.CloseVideo {
			t.Fatalf("после 5 мин видео: видео %d, общее %d, закрыто %v; want 2700, 8700, false", cur.VideoRemainingS, cur.RemainingS, cur.CloseVideo)
		}
		d, removed, err := e.ReduceExtra(60)
		if err != nil || removed != 60 {
			t.Fatalf("−60: убрано %d, err %v", removed, err)
		}
		if d.VideoLimitS != 0 || d.VideoRemainingS != 0 || !d.CloseVideo {
			t.Fatalf("после −60: лимит видео %d, остаток %d, закрыто %v; want 0, 0, true", d.VideoLimitS, d.VideoRemainingS, d.CloseVideo)
		}
	})
	t.Run("лимит видео 0", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(120), nil, intp(0), EndBlock), DayState{Day: monday}, "")
		d := e.Tick(videoSample())
		if !d.CloseVideo || d.Lock != LockNone || d.RemainingS != 7200 || d.Counted || d.Status != StatusOK {
			t.Fatalf("первая секунда видео: закрыто %v, экран %v, общее %d, засчитано %v, статус %v", d.CloseVideo, d.Lock, d.RemainingS, d.Counted, d.Status)
		}
		if d.VideoCloseReason != "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 2 ч 00 мин" || d.CloseReason != "" || d.CloseGames {
			t.Fatalf("тексты: видео %q, общий %q, игры закрыты %v", d.VideoCloseReason, d.CloseReason, d.CloseGames)
		}
		c.Set(at(15, 0, 1))
		if d := e.Tick(activeSample()); !d.Counted || d.CountedCat != CatOther {
			t.Fatalf("«Прочее» при закрытом видео: засчитано %v (%v)", d.Counted, d.CountedCat)
		}
		c.Set(at(15, 0, 2))
		if d := e.Tick(gameSample()); !d.Counted || d.CountedCat != CatGames {
			t.Fatalf("игра при закрытом видео: засчитано %v (%v)", d.Counted, d.CountedCat)
		}
		if st, _ := e.State(); st.TotalS != 2 || st.VideoS != 0 {
			t.Fatalf("итог %+v, want общее 2, видео 0", st)
		}
		d, _ = e.AddExtra(15)
		if d.VideoLimitS != 900 || d.CloseVideo || d.VideoCloseReason != "" {
			t.Fatalf("после +15: лимит видео %d, закрыто %v, текст %q; want 900, false, «»", d.VideoLimitS, d.CloseVideo, d.VideoCloseReason)
		}
	})
	t.Run("без лимита видео", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(120), nil, nil, EndBlock), DayState{Day: monday, VideoS: 7000, TotalS: 7000}, "")
		d := e.Tick(videoSample())
		if !d.VideoUnlimited || d.VideoLimitS != 0 || d.CloseVideo || !d.Counted {
			t.Fatalf("решение %+v, want без лимита видео, ничего не закрыто, секунда засчитана", d)
		}
	})
	t.Run("правила не прочитаны", func(t *testing.T) {
		r := videoMon(intp(0), intp(0), intp(0), EndStudyOnly)
		r.Broken = true
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, r, DayState{Day: monday}, "")
		d := e.Tick(videoSample())
		if !d.VideoUnlimited || d.CloseVideo || !d.Allows(CatVideoSocial) || !d.Counted {
			t.Fatalf("Broken: %+v, want ничего не закрыто (R18)", d)
		}
	})
}

// Ловит (RM3-7, PL-37): секунда видео при закрытом видео засчитывается; закрытие видео
// закрывает и «Прочее»; текст журнала не тот.
func TestEngineVideoClosedSecondNotCounted(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, videoMon(intp(120), nil, intp(20), EndBlock), DayState{Day: monday, VideoS: 1199, TotalS: 1199}, "")
	if d := e.Tick(videoSample()); !d.Counted || d.CountedCat != CatVideoSocial {
		t.Fatalf("последняя секунда видео: засчитано %v (%v)", d.Counted, d.CountedCat)
	}
	c.Set(at(15, 0, 1))
	d := e.Tick(videoSample())
	if !d.CloseVideo || d.Counted {
		t.Fatalf("после исчерпания: закрыто %v, засчитано %v; want true, false", d.CloseVideo, d.Counted)
	}
	c.Set(at(15, 0, 2))
	if d := e.Tick(activeSample()); !d.Counted {
		t.Fatal("«Прочее» при закрытом видео не засчитано")
	}
	if st, _ := e.State(); st.VideoS != 1200 || st.TotalS != 1201 {
		t.Fatalf("итог %+v, want видео 1200, общее 1201", st)
	}
	if d.Allows(CatVideoSocial) || !d.Allows(CatOther) {
		t.Fatalf("Allows: видео %v, прочее %v; want false, true", d.Allows(CatVideoSocial), d.Allows(CatOther))
	}
	if got := d.DeniedText(CatVideoSocial); got != "Закрыто: время на видео и соцсети на сегодня закончилось" {
		t.Fatalf("DeniedText(видео) = %q", got)
	}
}

// Ловит (8.2.5 п. 3, B-5, PL-35): минуты общего остатка вниз вместо вверх, «1 ч 05 мин» не
// по ТЗ 4, без общего лимита — лишняя фраза.
func TestVideoOverPlateText(t *testing.T) {
	for _, tc := range []struct {
		remS int
		unl  bool
		want string
	}{
		{2400, false, "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин"},
		{2399, false, "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин"},
		{3899, false, "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 1 ч 05 мин"},
		{0, true, "Время на видео и соцсети на сегодня закончилось"},
	} {
		if got := VideoOverPlateText(tc.remS, tc.unl); got != tc.want {
			t.Errorf("VideoOverPlateText(%d, %v) = %q, want %q", tc.remS, tc.unl, got, tc.want)
		}
	}
}

// Ловит: решение кладёт в текст плашки видео не общий остаток или фразу без общего лимита.
func TestEngineVideoCloseText(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, videoMon(intp(120), nil, intp(20), EndBlock), DayState{Day: monday, VideoS: 1200, TotalS: 4800}, "")
	if got := e.Tick(videoSample()).VideoCloseReason; got != "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин" {
		t.Fatalf("с общим лимитом: %q", got)
	}
	c = &fakeClock{t: at(15, 0, 0)}
	e = NewEngine(c, msk, videoMon(nil, nil, intp(20), EndBlock), DayState{Day: monday, VideoS: 1200, TotalS: 4800}, "")
	if got := e.Tick(videoSample()).VideoCloseReason; got != "Время на видео и соцсети на сегодня закончилось" {
		t.Fatalf("без общего лимита: %q", got)
	}
}

// Ловит (V-4, R8): видео закрывается под экраном блокировки (R8 закрывает только игры);
// «только учёба» пропускает видео; пауза оставляет видео закрытым.
func TestEngineVideoEndModes(t *testing.T) {
	t.Run("экран блокировки", func(t *testing.T) {
		c := &fakeClock{t: at(16, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(60), nil, intp(20), EndBlock), DayState{Day: monday, TotalS: 3600, VideoS: 1200}, "")
		d := e.Tick(videoSample())
		if d.Lock != LockTimeUp || !d.CloseGames || d.CloseVideo || d.VideoCloseReason != "" {
			t.Fatalf("время вышло: экран %v, игры %v, видео %v, текст %q; want «время вышло», true, false, «»", d.Lock, d.CloseGames, d.CloseVideo, d.VideoCloseReason)
		}
	})
	t.Run("только учёба", func(t *testing.T) {
		c := &fakeClock{t: at(16, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(60), nil, nil, EndStudyOnly), DayState{Day: monday, TotalS: 3600}, "")
		d := e.Tick(videoSample())
		if !d.StudyOnly || d.Allows(CatVideoSocial) || d.Counted {
			t.Fatalf("видео: только учёба %v, видео можно %v, засчитано %v; want true, false, false", d.StudyOnly, d.Allows(CatVideoSocial), d.Counted)
		}
		c.Set(at(16, 0, 1))
		if d := e.Tick(studySample()); !d.Counted {
			t.Fatal("учёба не засчитана")
		}
	})
	t.Run("пауза при исчерпанном видео", func(t *testing.T) {
		c := &fakeClock{t: at(16, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(120), nil, intp(20), EndBlock), DayState{Day: monday, TotalS: 1200, VideoS: 1200}, "")
		if _, err := e.Pause(at(17, 0, 0)); err != nil {
			t.Fatal(err)
		}
		if d := e.Tick(videoSample()); d.CloseVideo || !d.Counted {
			t.Fatalf("пауза: видео закрыто %v, засчитано %v; want false, true", d.CloseVideo, d.Counted)
		}
	})
}

// Ловит (R26): промокод не расширяет лимит видео или не открывает закрытое видео.
func TestRedeemPromoMinutesExtendsVideo(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, videoMon(intp(120), nil, intp(30), EndBlock), DayState{Day: monday}, "")
	run(e, c, at(15, 0, 0), at(15, 30, 0), videoSample())
	if cur := e.Current(); !cur.CloseVideo || cur.VideoRemainingS != 0 {
		t.Fatalf("до промокода: видео закрыто %v, остаток %d; want true, 0", cur.CloseVideo, cur.VideoRemainingS)
	}
	d, err := e.RedeemPromoMinutes(20)
	if err != nil {
		t.Fatal(err)
	}
	if d.CloseVideo || d.VideoRemainingS != 1200 || d.VideoLimitS != 3000 || d.LimitS != 8400 {
		t.Fatalf("после промокода 20: закрыто %v, остаток %d, лимит видео %d, общий %d; want false, 1200, 3000, 8400", d.CloseVideo, d.VideoRemainingS, d.VideoLimitS, d.LimitS)
	}
}

// Ловит (FR-19): SetRules теряет лимит видео или применяет его не сразу.
func TestEngineSetRulesVideo(t *testing.T) {
	c := &fakeClock{t: at(15, 0, 0)}
	e := NewEngine(c, msk, videoMon(intp(120), nil, nil, EndBlock), DayState{Day: monday, VideoS: 1800, TotalS: 1800}, "")
	d := e.SetRules(videoMon(intp(120), nil, intp(30), EndBlock))
	if !d.CloseVideo || d.VideoLimitS != 1800 {
		t.Fatalf("лимит видео 30: закрыто %v, лимит %d; want true, 1800", d.CloseVideo, d.VideoLimitS)
	}
	d = e.SetRules(videoMon(intp(120), nil, intp(60), EndBlock))
	if d.CloseVideo || d.VideoRemainingS != 1800 {
		t.Fatalf("лимит видео 60: закрыто %v, остаток %d; want false, 1800", d.CloseVideo, d.VideoRemainingS)
	}
}
