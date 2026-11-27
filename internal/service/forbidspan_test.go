package service

import (
	"reflect"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// Запретные часы сегодня — одна истина окна и телефона (B-79, PL-83), присутствие и ждущие
// программы на «Сегодня» (8.3.2), нарушения за сегодня в окне ребёнка.

// forbidAt — запретные часы Миши в момент at часов ядра (пояс +3): forbidSpan, строка мини-панели и
// поля статуса для окна.
func forbidAt(t *testing.T, h *harness, at time.Time) (forbidInfo, string, ipc.ChildStatus) {
	t.Helper()
	h.clock.Set(at)
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	cs := h.svc.children[h.childID(childSID)]
	return h.svc.forbidSpan(cs), h.svc.forbiddenNote(cs), h.svc.statusOf(cs)
}

func clockOf(tm time.Time) string {
	if tm.IsZero() {
		return ""
	}
	return tm.In(tzMSK).Format("2006-01-02 15:04")
}

// Ловит (B-79): карточка окна и отчёт телефону расходятся в состоянии запрета; «active» без конца
// завтра; «ahead» без интервала; снятые на сегодня часы или администратор показывают интервал.
func TestForbidSpanStates(t *testing.T) {
	night := core.Interval{Weekday: core.Monday, StartMin: 21 * 60, EndMin: 7 * 60}
	cases := []struct {
		name     string
		ivs      []core.Interval
		at       time.Time
		clear    bool
		admin    bool
		state    string
		from, to string // местные «дата час:мин»
		note     string
		ff, ft   string // ForbidFrom, ForbidTo статуса
	}{
		{"впереди", []core.Interval{night}, time.Date(2026, 10, 19, 20, 0, 0, 0, tzMSK), false, false,
			"ahead", "2026-10-19 21:00", "2026-10-20 07:00", "Сегодня нельзя с 21:00 до 07:00", "21:00", "07:00"},
		{"идёт", []core.Interval{night}, time.Date(2026, 10, 19, 22, 0, 0, 0, tzMSK), false, false,
			"active", "2026-10-19 21:00", "2026-10-20 07:00", "Сейчас нельзя до 07:00", "21:00", "07:00"},
		{"снят на сегодня", []core.Interval{night}, time.Date(2026, 10, 19, 20, 0, 0, 0, tzMSK), true, false,
			"cleared", "", "", "Сегодня запретные часы не действуют", "", ""},
		{"нет интервалов", nil, time.Date(2026, 10, 19, 20, 0, 0, 0, tzMSK), false, false,
			"none", "", "", "", "", ""},
		{"администратор", []core.Interval{night}, time.Date(2026, 10, 19, 22, 0, 0, 0, tzMSK), false, true,
			"none", "", "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := []hopt{configured(), withChild(childSID, "Миша", 600)}
			if len(c.ivs) > 0 {
				opts = append(opts, withForbidden(childSID, c.ivs...))
			}
			h := newHarness(t, opts...)
			if c.admin {
				h.admins.set(childSID, true)
			}
			h.start()
			h.svc.Housekeep(h.real.Now())
			if c.clear {
				h.svc.mu.Lock()
				h.svc.setNoForbidden(h.svc.children[h.childID(childSID)], true, extraFromWindow)
				h.svc.mu.Unlock()
			}
			fi, note, st := forbidAt(t, h, c.at)
			if fi.state != c.state || clockOf(fi.from) != c.from || clockOf(fi.to) != c.to {
				t.Errorf("forbidSpan = %s %q–%q; want %s %q–%q", fi.state, clockOf(fi.from), clockOf(fi.to), c.state, c.from, c.to)
			}
			if !c.admin && note != c.note {
				t.Errorf("forbiddenNote = %q; want %q", note, c.note)
			}
			if st.ForbidState != c.state || st.ForbidFrom != c.ff || st.ForbidTo != c.ft {
				t.Errorf("статус: %q %q–%q; want %q %q–%q", st.ForbidState, st.ForbidFrom, st.ForbidTo, c.state, c.ff, c.ft)
			}
		})
	}
}

// Ловит (B-79): строки состояния окна (ipc.ForbidState*) разошлись со строками отчёта телефону (remote.Forbid*).
func TestForbidStateStringsMatchRemote(t *testing.T) {
	got := []string{ipc.ForbidStateNone, ipc.ForbidStateAhead, ipc.ForbidStateActive, ipc.ForbidStateCleared}
	want := []string{remote.ForbidNone, remote.ForbidAhead, remote.ForbidActive, remote.ForbidCleared}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ipc %q, remote %q", got, want)
	}
}

// Ловит (B-79): в 23:00 интервал, начинающийся завтра в 00:30, показан как «Сегодня нельзя с 00:30».
func TestForbidSpanAfterMidnightIsTomorrow(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600),
		withForbidden(childSID, core.Interval{Weekday: core.Tuesday, StartMin: 30, EndMin: 6 * 60}))
	h.start()
	h.svc.Housekeep(h.real.Now())
	fi, note, st := forbidAt(t, h, time.Date(2026, 10, 19, 23, 0, 0, 0, tzMSK))
	if fi.state != "none" || !fi.from.IsZero() || !fi.to.IsZero() || note != "" || st.ForbidState != "none" || st.ForbiddenNote != "" {
		t.Errorf("23:00 перед интервалом вт 00:30: %+v, строка %q, статус %q %q", fi, note, st.ForbidState, st.ForbiddenNote)
	}
}

// Ловит (8.3.2): карточка не говорит, за компьютером ли ребёнок и с какого времени; ждущие программы
// не новые первыми, больше трёх, nil вместо [] или без общего числа.
func TestTodayCardPresenceAndPending(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h.clock.Set(time.Date(2026, 10, 19, 18, 19, 59, 0, tzMSK))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)

	var empty ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, ipc.Empty{}, &empty)
	if empty.PendingNames == nil || len(empty.PendingNames) != 0 || empty.PendingTotal != 0 {
		t.Errorf("без ждущих: %#v, %d; want [] и 0", empty.PendingNames, empty.PendingTotal)
	}
	if c := empty.Children[0]; c.PresenceNow || c.PresenceText != "Не за компьютером с 18:19" {
		t.Errorf("до входа: %v %q; want «Не за компьютером с 18:19»", c.PresenceNow, c.PresenceText)
	}

	h.childLogsIn()
	h.run(1, activeSample)
	for i, p := range []struct{ path, name string }{
		{`c:\games\old.exe`, "Old Game"}, {`c:\games\steam.exe`, "Steam"}, {`c:\dl\supergame.exe`, ""}, {`c:\games\studio.exe`, "Roblox Studio"},
	} {
		if _, err := h.store.AddPendingAttempt(PendingProgram{Path: p.path, Name: p.name}, id, int64(1_000+i)); err != nil && p.name != "" {
			t.Fatal(err)
		}
	}
	// Программа без названия из файла: имя из пути (programName). Фейк не берёт пустое имя — кладём сами.
	h.store.mu.Lock()
	h.store.pending[`c:\dl\supergame.exe`] = &PendingProgram{Path: `c:\dl\supergame.exe`, LastAttemptAt: 1_002, Attempts: 1, Children: []PendingChild{}}
	h.store.mu.Unlock()

	var rep ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, ipc.Empty{}, &rep)
	if want := []string{"Roblox Studio", "supergame", "Steam"}; !reflect.DeepEqual(rep.PendingNames, want) || rep.PendingTotal != 4 {
		t.Errorf("ждущие %q, всего %d; want %q и 4", rep.PendingNames, rep.PendingTotal, want)
	}
	if c := rep.Children[0]; !c.PresenceNow || c.PresenceText != "За компьютером с 18:20" {
		t.Errorf("за компьютером: %v %q; want «За компьютером с 18:20»", c.PresenceNow, c.PresenceText)
	}
}

// Ловит: блок «Сегодня» окна ребёнка берёт нарушения выбранного периода, а не сегодняшние.
func TestMyTodayViolationsTodayIndependentOfPeriod(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 600))
	h.clock.Set(time.Date(2026, 10, 20, 15, 0, 0, 0, tzMSK)) // вторник: вчера — в той же неделе
	h.start()
	h.svc.Housekeep(h.real.Now())
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	today := h.clock.Now()
	for _, at := range []time.Time{today.Add(-time.Hour), today.AddDate(0, 0, -1), today.AddDate(0, 0, -1).Add(time.Hour)} {
		if err := h.store.AppendEvent(StoredEvent{ChildID: id, AtMs: at.UnixMilli(), TzOffsetMin: 180, Kind: core.EvBypass, Details: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	week := h.parentPeriod(tok, id, ipc.PeriodWeek)
	if len(week.Violations) != 3 || week.ViolationsToday != 1 {
		t.Errorf("неделя: нарушений %d, сегодня %d; want 3 и 1", len(week.Violations), week.ViolationsToday)
	}
	day := h.parentPeriod(tok, id, ipc.PeriodToday)
	if len(day.Violations) != 1 || day.ViolationsToday != 1 {
		t.Errorf("сегодня: нарушений %d, сегодня %d; want 1 и 1", len(day.Violations), day.ViolationsToday)
	}
	h.childLogsIn()
	var mine ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, ipc.MyTodayRequest{Period: ipc.PeriodWeek}, &mine)
	if mine.ViolationsToday != 1 {
		t.Errorf("окно ребёнка, неделя: сегодня %d; want 1", mine.ViolationsToday)
	}
}
