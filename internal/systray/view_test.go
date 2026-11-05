package systray

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

func okStatus(remainingS, limitS int) ipc.ChildStatus {
	d := core.Decision{Status: core.StatusOK, RemainingS: remainingS, LimitS: limitS}
	if remainingS <= core.LowRemainingS {
		d.Status = core.StatusLow
	}
	return ipc.ChildStatusFrom("Миша", d)
}

// Ловит: значок показывает не то, что сказала служба: цвет не по статусу, надпись не по
// FR-7 (минуты до 99, от 100 целые часы), подсказка без остатка.
func TestDisplayFor(t *testing.T) {
	timeUp := ipc.ChildStatusFrom("Миша", core.Decision{
		Status: core.StatusTimeUp, Lock: core.LockTimeUp, Reason: core.ReasonTimeUp, LimitS: 7200,
	})
	bypass := ipc.ChildStatusFrom("Миша", core.Decision{
		Status: core.StatusBypass, Lock: core.LockBypass, Reason: core.ReasonBypass, RemainingS: 3000, LimitS: 7200,
	})
	cases := []struct {
		name    string
		st      ipc.ChildStatus
		label   string
		status  Status
		tooltip string
	}{
		{"много времени", okStatus(85*60, 7200), "85", StatusNormal, "Осталось 1 ч 25 мин из 2 ч"},
		{"больше ста минут", okStatus(125*60, 10800), "2ч", StatusNormal, "Осталось 2 ч 05 мин из 3 ч"},
		{"ровно порог", okStatus(15*60, 7200), "15", StatusLow, "Осталось 15 мин из 2 ч"},
		{"чуть меньше порога", okStatus(14*60+59, 7200), "15", StatusLow, "Осталось 15 мин из 2 ч"},
		{"чуть больше порога", okStatus(15*60+1, 7200), "16", StatusNormal, "Осталось 16 мин из 2 ч"},
		{"остались секунды", okStatus(5, 7200), "1", StatusLow, "Осталось 1 мин из 2 ч"},
		{"время вышло", timeUp, "0", StatusOut, core.ReasonTimeUp},
		{"блокировка за обход", bypass, "0", StatusOut, core.ReasonBypass},
		{"без лимита", ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusUnlimited, Unlimited: true}), "∞", StatusNormal, "Сегодня без ограничений по времени"},
		{"правила не прочитаны", ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusSafe, Unlimited: true}), "∞", StatusPaused, "Ограничения сейчас не действуют"},
		{"учётка стала администратором", ipc.ChildStatusAdmin("Миша"), "∞", StatusPaused, "Администратор: ограничения не действуют"},
		{"не детская учётка", ipc.ChildStatusNotChild(), "", StatusNotChild, "PC Block: настройки для родителя"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := DisplayFor(c.st)
			if d.Label != c.label || d.Status != c.status || d.Tooltip != c.tooltip {
				t.Fatalf("получили {%q %v %q}, ждали {%q %v %q}", d.Label, d.Status, d.Tooltip, c.label, c.status, c.tooltip)
			}
		})
	}
}

// Ловит: значок и служба расходятся в границах (15 мин, ноль) или в цифрах, потому что
// считают остаток по-разному: на каждой секунде остатка цвет значка должен совпасть со
// статусом службы, а подсказка начинаться с её подсказки.
func TestDisplayForAgreesWithServiceEverySecond(t *testing.T) {
	const limit = 3 * 3600
	for rem := 0; rem <= limit; rem++ {
		d := core.Decision{Status: core.StatusOK, RemainingS: rem, LimitS: limit}
		switch {
		case rem == 0:
			d.Status, d.Lock, d.Reason = core.StatusTimeUp, core.LockTimeUp, core.ReasonTimeUp
		case rem <= core.LowRemainingS:
			d.Status = core.StatusLow
		}
		st := ipc.ChildStatusFrom("Миша", d)
		got := DisplayFor(st)
		want := map[string]Status{ipc.StatusOK: StatusNormal, ipc.StatusLow: StatusLow, ipc.StatusTimeUp: StatusOut}[st.Status]
		if got.Status != want {
			t.Fatalf("остаток %d с: служба %q, значок %v, ждали %v", rem, st.Status, got.Status, want)
		}
		if !strings.HasPrefix(got.Tooltip, st.Tooltip) {
			t.Fatalf("остаток %d с: подсказка значка %q не начинается с подсказки службы %q", rem, got.Tooltip, st.Tooltip)
		}
		if ceilMin(rem) < 100 && got.Label != st.Label {
			t.Fatalf("остаток %d с: надпись значка %q, службы %q", rem, got.Label, st.Label)
		}
	}
}

// Ловит: пустое или неизвестное состояние службы роняет значок или рисует цифры из воздуха.
func TestDisplayForUnknownStatusDoesNotLie(t *testing.T) {
	for _, st := range []ipc.ChildStatus{{}, {Status: "futureStatus", RemainingS: 600}} {
		d := DisplayFor(st)
		if d.Label != "" || d.Status != StatusNormal || d.Tooltip != tooltipWaiting {
			t.Fatalf("незнакомое состояние %q: %+v", st.Status, d)
		}
	}
}

// Ловит: значок до первого ответа службы показывает чужие цифры; после обрыва связи
// вечно держит старый остаток; после возвращения связи не обновляется.
func TestTrackerDisplay(t *testing.T) {
	var tr Tracker
	t0 := time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)

	if d := tr.Display(t0); d.Label != "" || d.Tooltip != tooltipWaiting {
		t.Fatalf("до ответа службы: %+v", d)
	}
	tr.OnError()
	if d := tr.Display(t0); d.Label != "" || d.Tooltip != tooltipNoData {
		t.Fatalf("первый опрос не удался: %+v", d)
	}

	tr.OnStatus(t0, okStatus(85*60, 7200))
	if d := tr.Display(t0); d.Label != "85" {
		t.Fatalf("после ответа: %+v", d)
	}
	tr.OnError()
	if d := tr.Display(t0.Add(StaleAfter)); d.Label != "85" {
		t.Fatalf("один неудачный опрос не должен стирать цифры: %+v", d)
	}
	if d := tr.Display(t0.Add(StaleAfter + time.Second)); d.Label != "" || d.Tooltip != tooltipNoData {
		t.Fatalf("через %v без ответа цифры устарели: %+v", StaleAfter, d)
	}
	tr.OnStatus(t0.Add(time.Minute), okStatus(80*60, 7200))
	if d := tr.Display(t0.Add(time.Minute)); d.Label != "80" {
		t.Fatalf("связь вернулась: %+v", d)
	}
}

func TestSnapshotFreshness(t *testing.T) {
	var tr Tracker
	t0 := time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	if s := tr.Snapshot(t0); s.Has || s.Fresh || s.Failed {
		t.Fatalf("пустой: %+v", s)
	}
	tr.OnStatus(t0, ipc.ChildStatusNotChild())
	if s := tr.Snapshot(t0.Add(StaleAfter)); !s.Has || !s.Fresh || s.Failed {
		t.Fatalf("свежий: %+v", s)
	}
	if s := tr.Snapshot(t0.Add(StaleAfter + time.Nanosecond)); s.Fresh {
		t.Fatalf("устаревший: %+v", s)
	}
}

func TestPollIntervalWithinFirstIconDeadline(t *testing.T) {
	// ТЗ 8.2.1 п.1: значок появляется не позже 10 с; цифры не должны ждать дольше двух опросов.
	if 2*PollEvery > 10*time.Second || StaleAfter < 2*PollEvery {
		t.Fatalf("PollEvery=%v StaleAfter=%v", PollEvery, StaleAfter)
	}
}

// Ловит (ТЗ 8.2.1 п. 2, R6, T7-11): подсказка значка не показывает остаток игр и видео (служба
// строки дописывает, а значок строит подсказку сам), показывает их без лимита или при блокировке.
// Ожидания — литералы.
func TestDisplayForLimitLines(t *testing.T) {
	base := core.Decision{Status: core.StatusOK, RemainingS: 85 * 60, LimitS: 7200,
		GamesRemainingS: 1200, GamesLimitS: 3600, VideoRemainingS: 600, VideoLimitS: 1200}
	onlyVideo := base
	onlyVideo.GamesUnlimited, onlyVideo.GamesRemainingS, onlyVideo.GamesLimitS = true, 0, 0
	videoOver := onlyVideo
	videoOver.VideoRemainingS, videoOver.CloseVideo = 0, true
	videoZero := onlyVideo
	videoZero.VideoRemainingS, videoZero.VideoLimitS, videoZero.CloseVideo = 0, 0, true
	gamesOver := base
	gamesOver.GamesRemainingS, gamesOver.GamesLimitS, gamesOver.CloseGames = 0, 0, true
	gamesOver.VideoUnlimited, gamesOver.VideoRemainingS, gamesOver.VideoLimitS = true, 0, 0
	noLimits := base
	noLimits.GamesUnlimited, noLimits.VideoUnlimited = true, true
	low := base
	low.Status, low.RemainingS = core.StatusLow, 14*60+1
	timeUp := base
	timeUp.Status, timeUp.Lock, timeUp.Reason, timeUp.RemainingS = core.StatusTimeUp, core.LockTimeUp, core.ReasonTimeUp, 0
	unlimited := base
	unlimited.Status, unlimited.Unlimited = core.StatusUnlimited, true
	cases := []struct {
		name string
		d    core.Decision
		want string
	}{
		{"игры и видео", base, "Осталось 1 ч 25 мин из 2 ч\nИгры: 20 мин\nВидео: 10 мин"},
		{"только видео", onlyVideo, "Осталось 1 ч 25 мин из 2 ч\nВидео: 10 мин"},
		{"видео исчерпано", videoOver, "Осталось 1 ч 25 мин из 2 ч\nВидео: 0 мин"},
		{"видео запрещено совсем", videoZero, "Осталось 1 ч 25 мин из 2 ч\nВидео: 0 мин"},
		{"игры исчерпаны", gamesOver, "Осталось 1 ч 25 мин из 2 ч\nИгры: 0 мин"},
		{"без лимитов игр и видео", noLimits, "Осталось 1 ч 25 мин из 2 ч"},
		{"мало времени", low, "Осталось 15 мин из 2 ч\nИгры: 20 мин\nВидео: 10 мин"},
		{"время вышло", timeUp, core.ReasonTimeUp},
		{"без ограничений", unlimited, "Сегодня без ограничений по времени"},
	}
	for _, c := range cases {
		got := DisplayFor(ipc.ChildStatusFrom("Миша", c.d)).Tooltip
		if got != c.want {
			t.Errorf("%s: подсказка %q, ждали %q", c.name, got, c.want)
		}
		if n := len(utf16.Encode([]rune(got))); n > 127 {
			t.Errorf("%s: подсказка занимает %d знаков UTF-16, поле вмещает 127", c.name, n)
		}
	}
	// Пауза (подсказку объясняет служба) и перерыв на отдых (экран при идущем времени): строк
	// лимитов нет.
	paused := base
	paused.Status, paused.Unlimited = core.StatusSafe, true
	rest := base
	rest.Lock, rest.Reason = core.LockRest, "Перерыв на отдых"
	for name, d := range map[string]core.Decision{"пауза": paused, "перерыв": rest} {
		if got := DisplayFor(ipc.ChildStatusFrom("Миша", d)).Tooltip; strings.Contains(got, "Видео") || strings.Contains(got, "Игры") {
			t.Errorf("%s: подсказка %q со строками лимитов", name, got)
		}
	}
}
