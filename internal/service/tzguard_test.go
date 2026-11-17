package service

import (
	"errors"
	"testing"
)

// Тесты смены часового пояса (веха M6, R17б, FR-28, S23): смена при ребёнке на экране — обход у
// него с прежним и новым поясом; при родителе, без ребёнка на экране и переход на летнее время —
// нет; ошибка чтения не мешает обслуживанию.

var (
	tzMoscow = TimeZoneInfo{Key: "Russian Standard Time", Display: "(UTC+03:00) Москва, Санкт-Петербург", BiasMin: -180, NoDST: false}
	tzEkb    = TimeZoneInfo{Key: "Ekaterinburg Standard Time", Display: "(UTC+05:00) Екатеринбург", BiasMin: -300}
)

type tzRig struct {
	*harness
	zone TimeZoneInfo
	err  error
}

func newTZRig(t *testing.T) *tzRig {
	r := &tzRig{zone: tzMoscow}
	r.harness = newHarness(t, withChild(childSID, "Миша", 120), configured(), func(h *harness) {
		h.cfg.TimeZone = func() (TimeZoneInfo, error) { return r.zone, r.err }
	})
	return r
}

func (r *tzRig) tzEvents() []StoredEvent {
	out := []StoredEvent{}
	for _, e := range r.store.eventsOf("bypass") {
		if e.Details["subtype"] == "tz_changed" {
			out = append(out, e)
		}
	}
	return out
}

// Ловит: смена пояса при ребёнке на экране не записывается, записывается позже 2 с или без
// прежнего и нового пояса; повторное чтение того же пояса пишет ещё раз.
func TestTZChangeWithChildOnScreen(t *testing.T) {
	r := newTZRig(t)
	r.start()
	r.childLogsIn()
	r.hk(3)
	r.zone = tzEkb
	r.hk(1)
	ev := r.tzEvents()
	if len(ev) != 1 {
		t.Fatalf("записей о поясе через 1 с: %d, ждали 1", len(ev))
	}
	if ev[0].ChildID != r.childID(childSID) {
		t.Fatalf("запись у ребёнка %d", ev[0].ChildID)
	}
	want := "Часовой пояс изменён: был «(UTC+03:00) Москва, Санкт-Петербург», стал «(UTC+05:00) Екатеринбург»"
	if ev[0].Details["detail"] != want {
		t.Fatalf("подробность «%v», ждали «%s»", ev[0].Details["detail"], want)
	}
	r.hk(5)
	if n := len(r.tzEvents()); n != 1 {
		t.Fatalf("без новой смены записей стало %d, ждали 1", n)
	}
	r.zone = TimeZoneInfo{Key: "UTC", BiasMin: 0} // без названия — ключ
	r.hk(1)
	ev = r.tzEvents()
	if len(ev) != 2 || ev[1].Details["detail"] != "Часовой пояс изменён: был «(UTC+05:00) Екатеринбург», стал «UTC»" {
		t.Fatalf("вторая смена: %v", ev)
	}
}

// Ловит: смена пояса без ребёнка на экране, переход на летнее время или ошибка чтения пишут обход.
func TestTZNotBypass(t *testing.T) {
	t.Run("на экране родитель", func(t *testing.T) {
		r := newTZRig(t)
		r.start()
		r.launcher.setSessions(SessionInfo{ID: 1, User: `PC\Анна`, SID: parentSID})
		r.svc.OnSession(SessionLogon, 1, r.real.Now())
		r.hk(2)
		r.zone = tzEkb
		r.hk(2)
		if n := len(r.tzEvents()); n != 0 {
			t.Fatalf("записей: %d, ждали 0", n)
		}
	})
	t.Run("сеанс ребёнка заблокирован", func(t *testing.T) {
		r := newTZRig(t)
		r.start()
		r.childLogsIn()
		r.hk(2)
		r.svc.OnSession(SessionLock, childSession, r.real.Now())
		r.zone = tzEkb
		r.hk(2)
		if n := len(r.tzEvents()); n != 0 {
			t.Fatalf("записей: %d, ждали 0", n)
		}
	})
	t.Run("ошибка чтения, потом тот же пояс", func(t *testing.T) {
		r := newTZRig(t)
		r.start()
		r.childLogsIn()
		r.hk(2)
		r.err = errors.New("реестр недоступен")
		r.hk(3)
		r.err = nil
		r.hk(2)
		if n := len(r.tzEvents()); n != 0 {
			t.Fatalf("записей: %d, ждали 0", n)
		}
	})
}
