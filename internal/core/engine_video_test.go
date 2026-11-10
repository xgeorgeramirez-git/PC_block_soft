package core

import (
	"slices"
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

// Ловит (R9, PL-13, PL-14, PL-34): пороги видео без видео на переднем плане, взвод нестрого
// («+30» при нуле, повтор после SetRules), два уведомления в один тик или не то, порог видео
// показан, когда общий лимит кончится раньше, пауза, копилка и сутки не перевзводят пороги
// видео, порог 1 мин видео ставит последнюю минуту общего лимита.
func TestEngineVideoWarnings(t *testing.T) {
	t.Run("пороги при видео на переднем плане", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(300), nil, intp(60), EndBlock), DayState{Day: monday}, "")
		got := warnEvLog(e, c, at(15, 0, 0), at(16, 0, 5), videoSample())
		want := []string{"15:30:00 30 video", "15:45:00 15 video", "15:55:00 5 video", "15:59:00 1 video"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
		// Порог 1 мин видео — не последняя минута общего лимита (D-60).
		if st, _ := e.State(); st.LastMinuteEndMs != nil {
			t.Fatalf("последняя минута поставлена порогом видео: %d", *st.LastMinuteEndMs)
		}
	})
	t.Run("только при видео на переднем плане", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(120), nil, intp(60), EndBlock), DayState{Day: monday, TotalS: 1790, VideoS: 1790}, "")
		if got := warnEvLog(e, c, at(15, 0, 0), at(15, 0, 10), videoSample()); len(got) != 0 {
			t.Fatalf("до 30:00: %v", got)
		}
		if got := warnEvLog(e, c, at(15, 0, 10), at(15, 5, 0), gameSample()); len(got) != 0 {
			t.Fatalf("без видео на переднем плане: %v", got)
		}
		e.Current() // Current порогов не гасит
		got := warnEvLog(e, c, at(15, 5, 0), at(15, 5, 2), videoSample())
		if want := []string{"15:05:00 30 video"}; !slices.Equal(got, want) {
			t.Fatalf("вернулся к видео: %v, want %v", got, want)
		}
	})
	t.Run("игры и видео с равным остатком", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(300), intp(60), intp(60), EndBlock), DayState{Day: monday, GamesS: 1799, VideoS: 1799, TotalS: 3598}, "")
		if got, want := warnEvLog(e, c, at(15, 0, 0), at(15, 0, 5), videoSample()), []string{"15:00:01 30 video"}; !slices.Equal(got, want) {
			t.Fatalf("видео: %v, want %v", got, want)
		}
		if got, want := warnEvLog(e, c, at(15, 0, 5), at(15, 0, 8), gameSample()), []string{"15:00:06 30 games"}; !slices.Equal(got, want) {
			t.Fatalf("игра: %v, want %v", got, want)
		}
	})
	t.Run("+30 при исчерпанном видео", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(300), nil, intp(0), EndBlock), DayState{Day: monday}, "")
		if _, err := e.AddExtra(30); err != nil {
			t.Fatal(err)
		}
		got := warnEvLog(e, c, at(15, 0, 0), at(15, 30, 5), videoSample())
		want := []string{"15:15:00 15 video", "15:25:00 5 video", "15:29:00 1 video"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("SetRules на границе той же секунды", func(t *testing.T) {
		r := videoMon(intp(120), nil, intp(60), EndBlock)
		c := &fakeClock{t: at(15, 29, 59)}
		e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 1799, VideoS: 1799}, "")
		// 15:29:59 засчитана: остаток видео ровно 30:00; дальше видео на переднем плане, сеанс спит.
		stalled := SessionSample{Foreground: CatVideoSocial}
		if got := warnEvLog(e, c, at(15, 29, 59), at(15, 30, 0), videoSample()); len(got) != 0 {
			t.Fatalf("при остатке 30:01: %v", got)
		}
		got := warnEvLog(e, c, at(15, 30, 0), at(15, 30, 1), stalled)
		if want := []string{"15:30:00 30 video"}; !slices.Equal(got, want) {
			t.Fatalf("до сохранения правил: %v, want %v", got, want)
		}
		e.SetRules(r)
		if got := warnEvLog(e, c, at(15, 30, 1), at(15, 31, 0), stalled); len(got) != 0 {
			t.Fatalf("после сохранения тех же правил повтор: %v", got)
		}
	})
	t.Run("равные события в один тик", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(60), nil, intp(60), EndBlock), DayState{Day: monday}, "")
		got := warnEvLog(e, c, at(15, 0, 0), at(16, 0, 5), videoSample())
		want := []string{"15:30:00 30 limit", "15:45:00 15 limit", "15:55:00 5 limit", "15:59:00 1 limit"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("два порога в один тик, меньший остаток", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(70), nil, intp(60), EndBlock), DayState{Day: monday}, "")
		got := warnEvLog(e, c, at(15, 0, 0), at(16, 0, 5), videoSample())
		// 15:55: общий «15» (остаток 15:00) и видео «5» (5:00) в один тик → только видео.
		want := []string{"15:30:00 30 video", "15:40:00 30 limit", "15:45:00 15 video", "15:55:00 5 video", "15:59:00 1 video"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("общий лимит кончится раньше", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		// Общий остаток 10:00, видео 16:00.
		e := NewEngine(c, msk, videoMon(intp(120), nil, intp(60), EndBlock), DayState{Day: monday, TotalS: 6600, VideoS: 2640}, "")
		got := warnEvLog(e, c, at(15, 0, 0), at(15, 10, 5), videoSample())
		want := []string{"15:05:00 5 limit", "15:09:00 1 limit"}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("пауза гасит, конец паузы взводит", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		e := NewEngine(c, msk, videoMon(intp(300), nil, intp(60), EndBlock), DayState{Day: monday}, "")
		if _, err := e.Pause(at(15, 40, 0)); err != nil {
			t.Fatal(err)
		}
		got := warnEvLog(e, c, at(15, 0, 0), at(16, 0, 5), videoSample())
		want := []string{
			"15:10:00 30 pause_end", "15:25:00 15 pause_end", "15:35:00 5 pause_end", "15:39:00 1 pause_end",
			"15:45:00 15 video", "15:55:00 5 video", "15:59:00 1 video",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("копилка взводит 30 заново", func(t *testing.T) {
		c := &fakeClock{t: at(15, 0, 0)}
		// Остаток видео 28 мин: «30» не взведён; копилка +5 → 33 мин, «30» взводится.
		e := NewEngine(c, msk, videoMon(intp(300), nil, intp(60), EndBlock), DayState{Day: monday, VideoS: 1920, TotalS: 1920}, "")
		if _, err := e.SpendBonus(5); err != nil {
			t.Fatal(err)
		}
		got := warnEvLog(e, c, at(15, 0, 0), at(15, 3, 2), videoSample())
		if want := []string{"15:03:00 30 video"}; !slices.Equal(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("запретные часы гасят дошедший порог", func(t *testing.T) {
		r := videoMon(intp(300), nil, intp(60), EndBlock)
		r.Schedule = []Interval{iv(Monday, 16*60, 17*60)}
		c := &fakeClock{t: at(16, 30, 0)}
		// Остаток видео 40 мин: «30» взведён; −10 мин доп. времени → ровно 30:00, «30» остаётся
		// взведённым (граница не потеряна), но под экраном запретных часов гасится без показа.
		e := NewEngine(c, msk, r, DayState{Day: monday, VideoS: 1200, TotalS: 1200}, "")
		if _, removed, err := e.ReduceExtra(10); err != nil || removed != 10 {
			t.Fatalf("−10: убрано %d, err %v", removed, err)
		}
		if got := warnEvLog(e, c, at(16, 30, 0), at(17, 0, 0), videoSample()); len(got) != 0 {
			t.Fatalf("под экраном: %v", got)
		}
		if got := warnEvLog(e, c, at(17, 0, 0), at(17, 0, 2), videoSample()); len(got) != 0 {
			t.Fatalf("после запретных часов порог всплыл задним числом: %v", got)
		}
	})
	t.Run("смена суток обнуляет видео", func(t *testing.T) {
		r := videoMon(intp(120), nil, intp(60), EndBlock)
		r.Week[1] = DayLimit{TotalMin: intp(120), VideoMin: intp(60)}
		c := &fakeClock{t: at(23, 59, 59)}
		e := NewEngine(c, msk, r, DayState{Day: monday, VideoS: 3600, TotalS: 3600}, "")
		if d := e.Tick(videoSample()); !d.CloseVideo {
			t.Fatal("пн 23:59:59: видео не закрыто")
		}
		c.Set(tue(0, 0, 0))
		if cur := e.Current(); cur.Day != tuesday || cur.CloseVideo || cur.VideoRemainingS != 3600 || cur.VideoCloseReason != "" {
			t.Fatalf("Current во вторник без тика: %+v", cur)
		}
		got := warnEvLog(e, c, tue(0, 0, 0), tue(0, 30, 1), videoSample())
		if want := []string{"00:30:00 30 video"}; !slices.Equal(got, want) {
			t.Fatalf("вторник: %v, want %v", got, want)
		}
	})
}
