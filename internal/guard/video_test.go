package guard

import (
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Тесты стража вехи M4: предупреждения о видео и соцсетях (R6, R9, D-54) и старшинство плашек
// сайтов (RM4-9, PL-34).

func videoDecision(remainingS, videoS int) core.Decision {
	d := runningDecision(remainingS)
	d.VideoRemainingS, d.VideoLimitS = videoS, 1200
	return d
}

func videoWarn(thr, eventS int) Incoming {
	w := ipc.NewGuardWarningFor(thr, core.WarnVideo, eventS)
	return Incoming{Threshold: w.Threshold, Text: w.Text, Event: w.Event, EventS: w.EventS}
}

// Ловит (T7-10): табло видео считает общий остаток вместо остатка видео, заголовок общий, звука нет;
// плашка 1 мин видео подменяет текст службы общим и не снимается, когда видео закрыто; плашка
// «время на видео закончилось» звучит или без крестика.
func TestVideoBoardCountsVideo(t *testing.T) {
	m := NewModel()
	m.OnWarnings(t0, videoDecision(5000, 300), false, []Incoming{videoWarn(5, 300)})
	w := m.View(t0).Warning
	if w == nil || w.Kind != ipc.PlateBoard || hms(w) != "00 : 05 : 00" || !w.Sound ||
		w.Board.Title != "ОСТАЛОСЬ НА ВИДЕО:" || w.Text != "Через 5 минут время на видео и соцсети закончится" {
		t.Fatalf("табло видео: %+v %+v", w, w.Board)
	}
	m.OnWarnings(at(2*time.Second), videoDecision(4998, 298), false, nil)
	if got := hms(m.View(at(2 * time.Second)).Warning); got != "00 : 04 : 58" {
		t.Fatalf("табло видео через 2 с: %q", got)
	}

	one := NewModel()
	one.OnWarnings(t0, videoDecision(5000, 60), false, []Incoming{videoWarn(1, 60)})
	one.OnWarnings(at(time.Second), videoDecision(4999, 59), false, nil)
	w = one.View(at(time.Second)).Warning
	if w == nil || w.Kind != ipc.PlateOneMinute || w.Text != "Осталась 1 минута на видео и соцсети: 0:59" ||
		buttonNames(w) != TargetName(ButtonPlateAskMore) {
		t.Fatalf("плашка 1 мин видео: %+v (кнопки %s)", w, buttonNames(w))
	}
	for s := 2; s <= 60; s++ {
		one.OnWarnings(at(time.Duration(s)*time.Second), videoDecision(5000-s, max(0, 60-s)), false, nil)
	}
	if w := one.View(at(60 * time.Second)).Warning; w == nil || w.Countdown != "0:00" {
		t.Fatalf("на нуле видео до закрытия плашка 1 мин должна быть: %+v", w)
	}
	over := videoDecision(4940, 0)
	over.CloseVideo = true
	one.OnWarnings(at(61*time.Second), over, false, nil)
	if w := one.View(at(61 * time.Second)).Warning; w != nil {
		t.Fatalf("видео закрыто, а плашка 1 мин висит: %+v", *w)
	}
	one.OnBlocked(at(62*time.Second), ipc.BlockedPlate{Kind: ipc.BlockedVideoOver,
		Text: "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин", ShowS: 10})
	w = one.View(at(62 * time.Second)).Warning
	if w == nil || w.Kind != "videoOver" || w.Sound || buttonNames(w) != TargetName(ButtonPlateClose) ||
		w.Text != "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин" {
		t.Fatalf("плашка «время на видео закончилось»: %+v (кнопки %s)", w, buttonNames(w))
	}

	for _, c := range []struct {
		thr  int
		want string
	}{
		{30, "Через 30 минут время на видео и соцсети закончится"},
		{15, "Через 15 минут время на видео и соцсети закончится"},
		{5, "Через 5 минут время на видео и соцсети закончится"},
		{1, "Осталась 1 минута на видео и соцсети: 1:00"},
	} {
		f := NewModel()
		f.OnWarnings(t0, videoDecision(5000, c.thr*60), false, []Incoming{{Threshold: c.thr, Event: ipc.WarnEventVideo, EventS: c.thr * 60}})
		if w := f.View(t0).Warning; w == nil || w.Text != c.want {
			t.Errorf("запасной текст видео %d: %+v", c.thr, w)
		}
	}
}

// Ловит (PL-14): повтор порога видео при следующем опросе даёт второй показ и звук; сдвиг остатка
// видео (доп. время) не взводит порог снова.
func TestVideoWarningRearmsAndSounds(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	r.ctl.SetSoundPlayer(func() bool { return true })
	r.svc.status = ipc.ChildStatusFrom("Миша", videoDecision(5000, 300))
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarningFor(5, core.WarnVideo, 300)}
	r.poll()
	r.clk.Advance(time.Second)
	r.svc.status = ipc.ChildStatusFrom("Миша", videoDecision(4999, 299))
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarningFor(5, core.WarnVideo, 299)}
	r.poll()
	st := r.lastState(t)
	if len(st.Warnings) != 1 || st.Warnings[0].Threshold != 5 || len(st.Sounds) != 1 {
		t.Fatalf("повтор порога видео: показано %+v, звуки %+v", st.Warnings, st.Sounds)
	}
	r.clk.Advance(time.Second)
	r.svc.status = ipc.ChildStatusFrom("Миша", videoDecision(6800, 2100))
	r.poll()
	r.clk.Advance(time.Second)
	r.svc.status = ipc.ChildStatusFrom("Миша", videoDecision(5000, 300))
	r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarningFor(5, core.WarnVideo, 300)}
	r.poll()
	if st := r.lastState(t); len(st.Warnings) != 2 || len(st.Sounds) != 2 {
		t.Fatalf("после сдвига остатка видео порог не показан снова: %+v, звуки %+v", st.Warnings, st.Sounds)
	}
}

// Ловит (T7-10): при равных порогах и секундах видео вытесняет общее предупреждение или игры;
// при меньшем остатке видео не выигрывает.
func TestMostUrgentRank(t *testing.T) {
	limit5 := Incoming{Threshold: 5, Text: "Через 5 минут время закончится. Сохрани игру", Event: ipc.WarnEventLimit, EventS: 300}
	cases := []struct {
		name string
		in   []Incoming
		want string
	}{
		{"игры и видео поровну: игры", []Incoming{gamesWarn(5, 300), videoWarn(5, 300)}, ipc.WarnEventGames},
		{"видео и игры поровну: игры", []Incoming{videoWarn(5, 300), gamesWarn(5, 300)}, ipc.WarnEventGames},
		{"видео и общий поровну: общий", []Incoming{videoWarn(5, 300), limit5}, ipc.WarnEventLimit},
		{"общий и видео поровну: общий", []Incoming{limit5, videoWarn(5, 300)}, ipc.WarnEventLimit},
		{"видео меньше секунд", []Incoming{videoWarn(5, 290), gamesWarn(5, 300)}, ipc.WarnEventVideo},
		{"видео меньший порог", []Incoming{limit5, videoWarn(1, 60)}, ipc.WarnEventVideo},
	}
	for _, c := range cases {
		if got, ok := mostUrgent(c.in); !ok || got.Event != c.want {
			t.Errorf("%s: %q, %v; ждали %q", c.name, got.Event, ok, c.want)
		}
	}
}
