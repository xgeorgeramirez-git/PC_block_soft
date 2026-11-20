package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// «Статистика за сегодня» на телефоне (FR-43, D-99, S59): те же числа, что «Итоги» за сегодня окна
// родителя (одна истина, класс A), и «за компьютером» из учёта сеанса.

// todayOf — today ребёнка id из отчёта телефону (Children(true)).
func (h *harness) todayOf(id int64) remote.ChildReport {
	h.t.Helper()
	for _, c := range (remoteHost{h.svc}).Children(true) {
		if c.ID == id {
			if c.Today == nil {
				h.t.Fatalf("у ребёнка %d нет today", id)
			}
			return c
		}
	}
	h.t.Fatalf("ребёнка %d нет в отчёте", id)
	return remote.ChildReport{}
}

// windowItems — строки вкладок окна kind: «название/категория» → секунды.
func windowItems(rows []ipc.ProgramTime, kind string) map[string]int64 {
	out := map[string]int64{}
	for _, r := range rows {
		if r.Path != "" && r.Kind == kind {
			out[r.Name+"/"+todayCat(r.Category)] += int64(r.Seconds)
		}
	}
	return out
}

func itemsMap(items []remote.TimeItem) map[string]int64 {
	out := map[string]int64{}
	for _, it := range items {
		out[it.Name+"/"+it.Cat] += it.Sec
	}
	return out
}

// Ловит: телефон считает «Итоги» за сегодня своим кодом и расходится с окном родителя (игры, учёба,
// видео, список программ и сайтов, скрытые и запрещённые видны телефону по имени, нарушения и
// обходы); «осталось» не то, что в строке ребёнка; пустые списки null; статистика в ответе на
// команду (Children(false)).
func TestRemoteTodayEqualsWindow(t *testing.T) {
	h := siteHarness(t, withStalker())
	h.setCategory(pathGeoGebra, "study")
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.hide(tok, true, pathLauncher)
	h.rename(tok, pathShipping, "Сталкер")
	start := h.clock.Now()
	h.run(50, fgSample(pathShipping, ipc.CatNameOther))
	h.run(15, fgSample(pathLauncher, ipc.CatNameOther)) // скрытая: не засчитывается и не видна
	h.run(15, fgSample(pathGeoGebra, ipc.CatNameOther))
	h.run(20, braveSample("youtube.com"))
	h.run(12, braveSample("wiki.example"))
	h.run(5, braveSample("example.com")) // запрещённый сайт: в «Итоги» не попадает
	h.run(7, fgSample("", ipc.CatNameOther))
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "Закрыт значок программы"})
	h.run(3, activeSample)

	var ps ipc.MyTodayReply
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id, Period: ipc.PeriodToday}, &ps)
	row := h.todayOf(id)
	td := row.Today
	if td.UsedSec != int64(ps.UsedS) || td.GamesSec != int64(ps.GamesS) || td.StudySec != int64(ps.StudyS) || td.VideoSec != int64(ps.VideoS) {
		t.Fatalf("итоги: телефон %+v, окно used %d games %d study %d video %d", td, ps.UsedS, ps.GamesS, ps.StudyS, ps.VideoS)
	}
	if ps.GamesS != 50 || ps.StudyS != 15 || ps.VideoS != 20 {
		t.Fatalf("окно: игры %d, учёба %d, видео %d; want 50, 15, 20 (данные теста не те)", ps.GamesS, ps.StudyS, ps.VideoS)
	}
	if td.LeftSec != row.LeftSec || td.LeftSec != int64(ps.Status.RemainingS) {
		t.Fatalf("осталось: today %d, строка %d, окно %d", td.LeftSec, row.LeftSec, ps.Status.RemainingS)
	}
	for kind, got := range map[string][]remote.TimeItem{usageKindProgram: td.Programs, usageKindSite: td.Sites} {
		want := windowItems(ps.Programs, kind)
		if g := itemsMap(got); fmt.Sprint(g) != fmt.Sprint(want) || len(got) != len(want) {
			t.Errorf("%s: телефон %v, окно %v", kind, g, want)
		}
	}
	all := fmt.Sprint(td.Programs, td.Sites)
	for _, bad := range []string{"{Stalker2 ", "{example.com", textOtherTime} {
		if strings.Contains(all, bad) {
			t.Errorf("в списках телефона лишнее %q: %s", bad, all)
		}
	}
	if len(td.Programs) == 0 || td.Programs[0].Name != "Сталкер" || td.Programs[0].Cat != remote.ItemGame || td.Programs[0].Sec != 50 || len(td.Sites) != 2 || td.Sites[0].Name != "youtube.com" {
		t.Fatalf("списки: %+v / %+v", td.Programs, td.Sites)
	}
	if td.Violations != len(ps.Violations) || td.Violations == 0 || td.Bypass != 1 {
		t.Fatalf("нарушения %d (окно %d), обходов %d", td.Violations, len(ps.Violations), td.Bypass)
	}
	if want := start.Add(time.Second).UnixMilli(); td.FirstMs != want || !td.NowAt || td.SinceMs != want {
		t.Fatalf("за компьютером: first %d, nowAt %v, since %d; want %d", td.FirstMs, td.NowAt, td.SinceMs, want)
	}
	// Ответ на команду — без статистики.
	for _, c := range (remoteHost{h.svc}).Children(false) {
		if c.Today != nil {
			t.Fatal("today в строке без статистики")
		}
	}
}

// Ловит: ребёнок без единой программы — списки null; первый вход есть, хотя сегодня не садился.
func TestRemoteTodayEmpty(t *testing.T) {
	h, _ := remoteHarness(t)
	td := h.todayOf(h.childID(childSID)).Today
	raw, _ := json.Marshal(td)
	if strings.Contains(string(raw), "null") || !strings.Contains(string(raw), `"programs":[],"sites":[]`) {
		t.Fatalf("пустой today: %s", raw)
	}
	if td.FirstMs != 0 || td.NowAt || td.SinceMs == 0 {
		t.Fatalf("не садился: %+v", td)
	}
}

// Ловит: «за компьютером» не меняется при отключении и блокировке сеанса; первый вход сдвигается
// при каждом возвращении; после перезапуска службы первый вход за день теряется; новый день
// показывает вчерашний первый вход.
func TestRemotePresence(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	t0 := h.clock.Now()
	h.run(10, activeSample)
	first := t0.Add(time.Second).UnixMilli()
	if td := h.todayOf(id).Today; td.FirstMs != first || !td.NowAt || td.SinceMs != first {
		t.Fatalf("сел: %+v", td)
	}
	// Блокировка экрана (Win+L): не за компьютером с этого тика.
	h.svc.OnSession(SessionLock, childSession, h.real.Now())
	h.run(5, activeSample)
	off := t0.Add(11 * time.Second).UnixMilli()
	if td := h.todayOf(id).Today; td.FirstMs != first || td.NowAt || td.SinceMs != off {
		t.Fatalf("заблокирован: %+v, since want %d", td, off)
	}
	h.svc.OnSession(SessionUnlock, childSession, h.real.Now())
	h.run(3, activeSample)
	back := t0.Add(16 * time.Second).UnixMilli()
	if td := h.todayOf(id).Today; td.FirstMs != first || !td.NowAt || td.SinceMs != back {
		t.Fatalf("вернулся: %+v, since want %d", td, back)
	}
	// Отключение сеанса (смена пользователя) — тоже «не за компьютером».
	h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
	h.run(2, activeSample)
	if td := h.todayOf(id).Today; td.NowAt {
		t.Fatalf("отключён: %+v", td)
	}
	// Перезапуск службы: первый вход за день сохранён, «с» — от запуска.
	h.svc.Shutdown(h.real.Now())
	var err error
	if h.svc, err = New(h.cfg); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.childLogsIn()
	t1 := h.clock.Now()
	h.run(2, activeSample)
	if td := h.todayOf(id).Today; td.FirstMs != first || !td.NowAt || td.SinceMs != t1.Add(time.Second).UnixMilli() {
		t.Fatalf("после перезапуска: %+v (first want %d)", td, first)
	}
	if v, ok, _ := h.store.GetSetting(settingPresenceFirst + fmt.Sprint(id)); !ok || v != fmt.Sprint(first) {
		t.Fatalf("настройка первого входа: %q %v", v, ok)
	}
	// Новый день: вчерашний первый вход не показывается, пока ребёнок не сел.
	h.svc.OnSession(SessionLock, childSession, h.real.Now())
	h.clock.Advance(10 * time.Hour)
	h.real.Advance(10 * time.Hour)
	h.svc.Tick(h.clock.Now())
	if td := h.todayOf(id).Today; td.FirstMs != 0 || td.NowAt {
		t.Fatalf("новый день, не садился: %+v", td)
	}
	h.svc.OnSession(SessionUnlock, childSession, h.real.Now())
	t2 := h.clock.Now()
	h.run(1, activeSample)
	if td := h.todayOf(id).Today; td.FirstMs != t2.Add(time.Second).UnixMilli() || core.DayOf(time.UnixMilli(td.FirstMs).In(tzMSK)) == core.DayOf(t0.In(tzMSK)) {
		t.Fatalf("новый день, сел: %+v", td)
	}
}
