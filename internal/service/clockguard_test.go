package service

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// Тесты перевода часов (веха M6, R17а, FR-28, D-87). Служба сверяет настоящие часы компьютера
// (RealNow) со своим внутренним счётчиком (Mono): скачок больше 120 с, когда на экране ребёнок, —
// запись обхода у ребёнка; при включении — сверка с последней записью службы, назад больше 120 с.
// В тестах счётчик — своя переменная: «перевод часов» двигает только стену.

// clockRig — служба с отдельным счётчиком: mono двигается вместе со стеной в hk и отдельно
// в jump/sleep.
type clockRig struct {
	*harness
	mono time.Duration
}

func newClockRig(t *testing.T, opts ...hopt) *clockRig {
	r := &clockRig{}
	opts = append([]hopt{withChild(childSID, "Миша", 120), configured(), func(h *harness) {
		h.cfg.Mono = func() time.Duration { return r.mono }
	}}, opts...)
	r.harness = newHarness(t, opts...)
	return r
}

// tick — секунды работы: стена и счётчик идут вместе.
func (r *clockRig) tick(n int) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		r.mono += time.Second
		r.hk(1)
	}
}

// jump — перевод часов: стена сдвигается на d, счётчик нет; затем секунда работы.
func (r *clockRig) jump(d time.Duration) {
	r.t.Helper()
	r.real.Advance(d)
	r.tick(1)
}

// sleep — сон или гибернация на d: стена и счётчик сдвигаются вместе (Go на Windows считает
// счётчик по времени прерываний, сон в нём есть).
func (r *clockRig) sleep(d time.Duration) {
	r.t.Helper()
	r.real.Advance(d)
	r.mono += d
	r.tick(1)
}

func (r *clockRig) clockEvents() []StoredEvent {
	out := []StoredEvent{}
	for _, e := range r.store.eventsOf("bypass") {
		if st, _ := e.Details["subtype"].(string); strings.HasPrefix(st, "clock_") {
			out = append(out, e)
		}
	}
	return out
}

// Ловит: поправка часов меньше порога (синхронизация Windows) или сон записываются обходом.
func TestClockSmallSyncAndSleepAreNotBypass(t *testing.T) {
	r := newClockRig(t)
	r.start()
	r.childLogsIn()
	r.tick(3)
	r.jump(30 * time.Second)
	r.jump(-30 * time.Second)
	r.jump(120 * time.Second) // ровно порог — не обход (строго больше)
	r.sleep(8 * time.Hour)
	r.tick(3)
	if n := len(r.clockEvents()); n != 0 {
		t.Fatalf("записей о часах: %d, ждали 0: %v", n, r.clockEvents())
	}
}

// Ловит: перевод часов вперёд и назад при ребёнке на экране не записывается, записывается не
// ребёнку или без прежнего и нового времени.
func TestClockJumpWithChildOnScreenIsBypass(t *testing.T) {
	r := newClockRig(t)
	r.start()
	r.childLogsIn()
	r.tick(3)
	// realT0 = 12:00 UTC = 15:00 МСК; три секунды работы до перевода.
	r.jump(10 * time.Minute)
	r.tick(2)
	r.jump(-10 * time.Minute)
	ev := r.clockEvents()
	if len(ev) != 2 {
		t.Fatalf("записей о часах: %d, ждали 2: %v", len(ev), ev)
	}
	id := r.childID(childSID)
	for _, e := range ev {
		if e.ChildID != id {
			t.Fatalf("запись о часах у ребёнка %d, ждали %d", e.ChildID, id)
		}
		if st := e.Details["subtype"]; st != "clock_changed" {
			t.Fatalf("подтип %v, ждали clock_changed", st)
		}
	}
	want0 := "Часы компьютера переведены: было 19.10 15:00, стало 19.10 15:10"
	if d := ev[0].Details["detail"]; d != want0 {
		t.Fatalf("подробность «%v», ждали «%s»", d, want0)
	}
	want1 := "Часы компьютера переведены: было 19.10 15:10, стало 19.10 15:00"
	if d := ev[1].Details["detail"]; d != want1 {
		t.Fatalf("подробность «%v», ждали «%s»", d, want1)
	}
	if r.status().Lock == "bypass" {
		t.Fatal("после перевода часов экран заблокирован, а не должен (D-41)")
	}
}

// Ловит: перевод часов, когда на экране не ребёнок (сеанс заблокирован, родитель у себя), или
// ребёнком-администратором записывается обходом.
func TestClockJumpNotChildOnScreenIsNotBypass(t *testing.T) {
	t.Run("сеанс ребёнка заблокирован", func(t *testing.T) {
		r := newClockRig(t)
		r.start()
		r.childLogsIn()
		r.tick(2)
		r.svc.OnSession(SessionLock, childSession, r.real.Now())
		r.jump(time.Hour)
		if n := len(r.clockEvents()); n != 0 {
			t.Fatalf("записей о часах: %d, ждали 0", n)
		}
		if !strings.Contains(r.logText(), "на экране не ребёнок") {
			t.Fatalf("в журнале работы нет строки о переводе часов:\n%s", r.logText())
		}
	})
	t.Run("никто не вошёл, часы вперёд", func(t *testing.T) {
		r := newClockRig(t)
		r.start()
		r.tick(2)
		r.jump(time.Hour)
		if n := len(r.clockEvents()); n != 0 {
			t.Fatalf("записей о часах: %d, ждали 0", n)
		}
	})
	t.Run("ребёнок-администратор", func(t *testing.T) {
		r := newClockRig(t)
		r.admins.set(childSID, true)
		r.start()
		r.childLogsIn()
		r.tick(2)
		r.jump(time.Hour)
		if n := len(r.clockEvents()); n != 0 {
			t.Fatalf("записей о часах: %d, ждали 0 (R1)", n)
		}
	})
}

// Ловит: при включении часы назад (перевод в BIOS) не записываются; включение после ночи
// (часы вперёд) или небольшая разница записываются (D-87).
func TestClockBootCheck(t *testing.T) {
	cases := []struct {
		name string
		last time.Time // последняя запись службы
		want int
	}{
		{"часы ушли назад на час", realT0.Add(time.Hour), 1},
		{"назад на 60 с: не обход", realT0.Add(60 * time.Second), 0},
		{"компьютер был выключен ночь", realT0.Add(-9 * time.Hour), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newClockRig(t)
			r.store.settings[settingClockLastSeen] = strconv.FormatInt(c.last.UnixMilli(), 10)
			r.start()
			ev := r.clockEvents()
			if len(ev) != c.want {
				t.Fatalf("записей о часах: %d, ждали %d: %v", len(ev), c.want, ev)
			}
			if c.want == 1 {
				e := ev[0]
				if e.ChildID != 0 || e.Details["subtype"] != "clock_back_boot" {
					t.Fatalf("запись %+v, ждали без ребёнка с подтипом clock_back_boot", e)
				}
				want := "Часы компьютера ушли назад: служба последний раз работала 19.10 в 16:00, после включения на часах 19.10 в 15:00"
				if e.Details["detail"] != want {
					t.Fatalf("подробность «%v», ждали «%s»", e.Details["detail"], want)
				}
			}
			if v := r.store.settings[settingClockLastSeen]; v != strconv.FormatInt(realT0.UnixMilli(), 10) {
				t.Fatalf("граница после включения %q, ждали момент включения", v)
			}
		})
	}
}

// Ловит: граница «служба последний раз работала» пишется каждую секунду (износ базы) или не
// пишется совсем (проверка при включении не сработает); при остановке не сохраняется.
func TestClockLastSeenSavedOncePerMinute(t *testing.T) {
	r := newClockRig(t)
	r.start()
	writes := 0
	prev := r.store.settings[settingClockLastSeen]
	for i := 0; i < 125; i++ {
		r.tick(1)
		if v := r.store.settings[settingClockLastSeen]; v != prev {
			writes++
			prev = v
		}
	}
	if writes != 2 {
		t.Fatalf("граница записана %d раз за 125 с, ждали 2", writes)
	}
	r.tick(5)
	r.svc.Shutdown(r.real.Now())
	if v := r.store.settings[settingClockLastSeen]; v != strconv.FormatInt(r.real.Now().UnixMilli(), 10) {
		t.Fatalf("граница при остановке %q, ждали момент остановки", v)
	}
}

// Ловит: часы назад при выходе из гибернации или быстрого запуска (служба не перезапускалась,
// на экране никого) уходят только в журнал работы; а часы назад при родителе на экране пишутся.
func TestClockBackAfterResumeWithNobodyOnScreen(t *testing.T) {
	r := newClockRig(t)
	r.start()
	r.tick(2)
	r.sleep(8 * time.Hour)
	r.jump(-time.Hour) // часы перевели назад, пока компьютер спал; экран входа
	ev := r.clockEvents()
	if len(ev) != 1 || ev[0].ChildID != 0 || ev[0].Details["subtype"] != "clock_back_boot" {
		t.Fatalf("записи о часах %v, ждали одну без ребёнка clock_back_boot", ev)
	}
	want := "Часы компьютера ушли назад, пока компьютер спал или был выключен: было 19.10 23:00, стало 19.10 22:00"
	if ev[0].Details["detail"] != want {
		t.Fatalf("подробность «%v», ждали «%s»", ev[0].Details["detail"], want)
	}

	p := newClockRig(t)
	p.start()
	p.launcher.setSessions(SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID})
	p.svc.OnSession(SessionLogon, 1, p.real.Now())
	p.tick(2)
	p.jump(-time.Hour)
	if n := len(p.clockEvents()); n != 0 {
		t.Fatalf("часы назад при родителе на экране: записей %d, ждали 0", n)
	}
}

// Ловит: память повторов обхода одна на ребёнка — попытки двух видов вперемешку записываются
// каждую секунду, а не раз в 60 с на каждую.
func TestBypassDedupPerDetail(t *testing.T) {
	r := newClockRig(t)
	r.start()
	cs := r.svc.children[r.childID(childSID)]
	at := r.real.Now()
	for i := 0; i < 30; i++ {
		r.svc.mu.Lock()
		r.svc.markBypass(cs, "А", "x", at)
		r.svc.markBypass(cs, "Б", "y", at)
		r.svc.mu.Unlock()
		at = at.Add(time.Second)
	}
	if n := len(r.store.eventsOf("bypass")); n != 2 {
		t.Fatalf("записей обхода: %d, ждали 2 (по одной на подробность)", n)
	}
	r.svc.mu.Lock()
	r.svc.markBypass(cs, "А", "x", at.Add(bypassLogWindow))
	r.svc.mu.Unlock()
	if n := len(r.store.eventsOf("bypass")); n != 3 {
		t.Fatalf("после 60 с тишины записей: %d, ждали 3", n)
	}
}

// Ловит: перевод часов приписан ребёнку, хотя за его экраном родитель вошёл в PC Block (окно
// настроек с паролем в сеансе ребёнка).
func TestClockJumpParentInChildSession(t *testing.T) {
	r := newClockRig(t)
	r.start()
	r.childLogsIn()
	r.tick(2)
	r.svc.mu.Lock()
	r.svc.parents["tok-p"] = &parentSession{token: "tok-p", sessionID: childSession, sid: parentSID, last: r.real.Now()}
	r.svc.mu.Unlock()
	r.jump(10 * time.Minute)
	if n := len(r.clockEvents()); n != 0 {
		t.Fatalf("записей о часах при родителе в сеансе ребёнка: %d, ждали 0", n)
	}
}
