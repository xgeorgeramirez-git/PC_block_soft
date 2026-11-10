package service

import (
	"encoding/json"
	"testing"

	"pcblock/internal/ipc"
)

// Ловит (S-3, PL-21): лимит «Видео и соцсети» из «Лимитов» не доходит до базы и обратно в ответ
// (окно показывало бы «без лимита» после сохранения), день без полей видео читается «видео нельзя»
// (старое окно и PowerShell-приёмка закрыли бы видео первым же «Сохранить»), лимит вне 0…1440
// уходит в базу или отказ не у поля video.N (L-121).
func TestSaveLimitsVideo(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)

	limits := sevenDays(120)
	for i := range limits {
		limits[i].VideoUnlimited = true
	}
	limits[0].VideoUnlimited, limits[0].VideoMin = false, 30 // пн: видео 30 мин
	limits[2].VideoUnlimited, limits[2].VideoMin = false, 0  // ср: видео нельзя
	var rep ipc.SaveLimitsReply
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: limits}, &rep)
	var rules ipc.ChildRules
	h.ok(adminPeer(), tok, ipc.CmdGetChildRules, ipc.ChildRequest{ChildID: id}, &rules)
	for _, got := range [][]ipc.WeekdayLimit{rep.Rules.Limits, rules.Limits} {
		if l := got[0]; l.VideoMin != 30 || l.VideoUnlimited {
			t.Errorf("пн: видео %d, без лимита %v; want 30, false", l.VideoMin, l.VideoUnlimited)
		}
		if l := got[1]; l.VideoMin != 0 || !l.VideoUnlimited {
			t.Errorf("вт: видео %d, без лимита %v; want 0, true", l.VideoMin, l.VideoUnlimited)
		}
		if l := got[2]; l.VideoMin != 0 || l.VideoUnlimited {
			t.Errorf("ср: видео %d, без лимита %v; want 0, false (нельзя)", l.VideoMin, l.VideoUnlimited)
		}
	}
	if v := h.store.limits[id][0].VideoMin; v == nil || *v != 30 {
		t.Errorf("видео пн в базе %v, want 30", v)
	}
	if v := h.store.limits[id][1].VideoMin; v != nil {
		t.Errorf("видео вт в базе %v, want nil (без лимита)", *v)
	}
	if v := h.store.limits[id][2].VideoMin; v == nil || *v != 0 {
		t.Errorf("видео ср в базе %v, want 0", v)
	}

	// Запрос без ключей видео (окно до экрана видео, мастер, PowerShell): все дни без лимита видео.
	raw := json.RawMessage(`{"childId":` + jsonOf(t, id) + `,"limits":[` +
		`{"weekday":1,"totalMin":60,"gamesUnlimited":true},{"weekday":2,"totalMin":60,"gamesUnlimited":true},` +
		`{"weekday":3,"totalMin":60,"gamesUnlimited":true},{"weekday":4,"totalMin":60,"gamesUnlimited":true},` +
		`{"weekday":5,"totalMin":60,"gamesUnlimited":true},{"weekday":6,"totalMin":60,"gamesUnlimited":true},` +
		`{"weekday":7,"totalMin":60,"gamesUnlimited":true}]}`)
	if r := h.do(adminPeer(), tok, ipc.CmdSaveLimits, raw); !r.OK {
		t.Fatalf("сохранение без полей видео: отказ %q у %q", r.Error, r.Field)
	}
	h.ok(adminPeer(), tok, ipc.CmdGetChildRules, ipc.ChildRequest{ChildID: id}, &rules)
	for _, l := range rules.Limits {
		if !l.VideoUnlimited || l.VideoMin != 0 || l.TotalMin != 60 {
			t.Errorf("день %d без полей видео: %+v, want видео без лимита", l.Weekday, l)
		}
	}
	for d, v := range h.store.limits[id] {
		if v.VideoMin != nil {
			t.Errorf("день %d в базе: видео %d, want nil", d+1, *v.VideoMin)
		}
	}

	// Вне 0…1440 — отказ у поля video.N, база не меняется.
	bad := func(edit func([]ipc.WeekdayLimit), text, field string) {
		t.Helper()
		l := sevenDays(90)
		for i := range l {
			l[i].VideoUnlimited = true
		}
		edit(l)
		r := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: l})
		if r.Error != text || r.Field != field {
			t.Errorf("отказ %q у %q, want %q у %q", r.Error, r.Field, text, field)
		}
	}
	bad(func(l []ipc.WeekdayLimit) { l[0].VideoUnlimited, l[0].VideoMin = false, 1441 }, "Не больше 24 ч", "video.1")
	bad(func(l []ipc.WeekdayLimit) { l[4].VideoUnlimited, l[4].VideoMin = false, -1 }, "Не меньше 0 мин", "video.5")
	if l := h.store.limits[id][0]; *l.TotalMin != 60 || l.VideoMin != nil {
		t.Errorf("после отказов лимиты изменены: общий %d, видео %v", *l.TotalMin, l.VideoMin)
	}
}
