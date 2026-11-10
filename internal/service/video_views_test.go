package service

import (
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Ловит (R6, R21, 8.3.2 п.3, 8.2.1 п.2): видео не доходит до карточки «Сегодня» или берёт цифры
// игр, строка видео есть без лимита видео, «Моя статистика» не знает секунд видео (прочее их
// включает), «Мои правила» молчат о видео или ставят его раньше игр.
func TestVideoInCardStatusAndMyToday(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withVideo(childSID, 20), withChild(otherSID, "Даша", 120))
	misha := h.childID(childSID)
	h.store.days[dayKey(misha, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 1200, GamesS: 0, VideoS: 600}
	h.start()
	tok := h.login(adminPeer())
	var today ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	if len(today.Children) != 2 {
		t.Fatalf("карточек %d", len(today.Children))
	}
	if m, d := today.Children[0], today.Children[1]; m.VideoText != "Видео 10 мин из 20 мин" || d.VideoText != "" {
		t.Errorf("карточки: Миша %q, Даша %q; want «Видео 10 мин из 20 мин» и пусто", m.VideoText, d.VideoText)
	}
	st := h.status()
	if st.VideoRemainingS != 600 || st.VideoLimitS != 1200 || st.VideoUnlimited || st.Tooltip != "Осталось 1 ч 40 мин\nВидео: 10 мин" {
		t.Errorf("статус значка: видео %d/%d (без лимита %v), подсказка %q", st.VideoRemainingS, st.VideoLimitS, st.VideoUnlimited, st.Tooltip)
	}
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if my.VideoS != 600 || my.OtherS != 600 {
		t.Errorf("«Моя статистика»: видео %d, прочее %d; want 600 и 600", my.VideoS, my.OtherS)
	}
	want := []ipc.MyRule{{Text: "Видео и соцсети: до 20 мин в день"}, {Text: "Когда время вышло: экран блокировки"}}
	if jsonOf(t, my.Rules[7:]) != jsonOf(t, want) {
		t.Errorf("общие правила %s, want %s", jsonOf(t, my.Rules[7:]), jsonOf(t, want))
	}
}

// Ловит (R21): строка видео «Моих правил» стоит раньше игр.
func TestMyRulesVideoAfterGames(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withVideo(childSID, 20), withGames(childSID, 60))
	h.start()
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	want := []ipc.MyRule{{Text: "Игры: до 1 ч в день"}, {Text: "Видео и соцсети: до 20 мин в день"}, {Text: "Когда время вышло: экран блокировки"}}
	if jsonOf(t, my.Rules[7:]) != jsonOf(t, want) {
		t.Errorf("общие правила %s, want %s", jsonOf(t, my.Rules[7:]), jsonOf(t, want))
	}
}

// Ловит (R6, R21): строка видео путает «нельзя» и «до N», пропускает дни, говорит «в день», когда
// видео ограничено не каждый день, или есть без лимита видео.
func TestVideoRuleText(t *testing.T) {
	week := func(vs ...*int) core.Rules {
		var r core.Rules
		for i, v := range vs {
			r.Week[i].VideoMin = v
		}
		return r
	}
	z, h1, h2 := 0, 60, 120
	cases := []struct {
		r    core.Rules
		want string
	}{
		{core.Rules{}, ""},
		{week(&z, &z, &z, &z, &z, &z, &z), "Видео и соцсети: нельзя"},
		{week(&h1, &h1, &h1, &h1, &h1, &h1, &h1), "Видео и соцсети: до 1 ч в день"},
		{week(nil, nil, nil, nil, nil, &z, &h2), "Видео и соцсети: сб нельзя, вс до 2 ч"},
	}
	for _, c := range cases {
		if got := videoRuleText(c.r); got != c.want {
			t.Errorf("videoRuleText = %q, want %q", got, c.want)
		}
	}
}

// Ловит (FR-33): «Скопировать правила с…» теряет лимит видео или ставит его дню без лимита
// (копия — черновик приёмника, в хранилище её пишет «Сохранить»).
func TestCopyRulesCarriesVideo(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(otherSID, "Даша", 90))
	src := h.childID(childSID)
	v := 20
	week := h.store.limits[src]
	week[0].VideoMin = &v
	h.store.SetDayLimits(src, week)
	h.start()
	tok := h.login(adminPeer())
	var rep ipc.CopyRulesReply
	h.ok(adminPeer(), tok, ipc.CmdCopyRules, ipc.CopyRulesRequest{FromChildID: src, ToChildID: h.childID(otherSID)}, &rep)
	if l := rep.Limits[0]; l.VideoMin != 20 || l.VideoUnlimited {
		t.Errorf("понедельник %+v, want видео 20", l)
	}
	if l := rep.Limits[1]; !l.VideoUnlimited {
		t.Errorf("вторник %+v, want видео без лимита", l)
	}
}
