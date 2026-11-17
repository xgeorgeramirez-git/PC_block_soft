package service

import "testing"

// Тесты безопасного режима (веха M6, R17е, FR-30, S24): вход ребёнка в безопасном режиме Windows
// записывается обходом один раз на сеанс, экран не блокируется; взрослый и обычный режим — нет.

func safeModeEvents(h *harness) []StoredEvent {
	out := []StoredEvent{}
	for _, e := range h.store.eventsOf("bypass") {
		if e.Details["subtype"] == "safe_mode" {
			out = append(out, e)
		}
	}
	return out
}

// Ловит: вход ребёнка в безопасном режиме не записывается, записывается каждую сверку сеансов
// (раз в секунду), записывается взрослому или в обычном режиме; экран блокируется.
func TestSafeModeChildLogon(t *testing.T) {
	for _, safe := range []bool{true, false} {
		h := newHarness(t, withChild(childSID, "Миша", 120), configured(), func(h *harness) {
			h.cfg.SafeMode = func() bool { return safe }
		})
		h.start()
		h.launcher.setSessions(SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID})
		h.svc.OnSession(SessionLogon, 1, h.real.Now())
		h.hk(3)
		if n := len(safeModeEvents(h)); n != 0 {
			t.Fatalf("безопасный режим %v: у взрослого записей %d, ждали 0", safe, n)
		}
		h.childLogsIn()
		h.hk(130)
		want := 0
		if safe {
			want = 1
		}
		ev := safeModeEvents(h)
		if len(ev) != want {
			t.Fatalf("безопасный режим %v: записей %d, ждали %d", safe, len(ev), want)
		}
		if safe {
			if ev[0].ChildID != h.childID(childSID) || ev[0].Details["detail"] != detailSafeMode {
				t.Fatalf("запись %+v", ev[0])
			}
			if h.status().Lock == "bypass" {
				t.Fatal("экран заблокирован за безопасный режим, а не должен (D-41)")
			}
		}
	}
}
