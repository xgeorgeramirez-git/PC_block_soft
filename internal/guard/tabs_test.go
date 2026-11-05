package guard

import (
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Ловит (RM4-2, PL-41): лестница закрытия вкладки не доходит до кнопки вкладки и WM_CLOSE, второй
// Ctrl+W раньше 2 с закрыл бы соседнюю вкладку, новое окно продолжало бы чужую лестницу. Ожидания —
// литералы, шаги одной лестницы по порядку.
func TestNextClose(t *testing.T) {
	look := TabLook{Window: 7, Brave: true, Domain: "example.com"}
	other := look
	other.Window = 9
	steps := []struct {
		look     TabLook
		at       time.Time
		wantWay  string
		wantStep int
	}{
		{look, t0, "ctrlW", 1},
		{look, at(time.Second), "", 1},
		{look, at(2 * time.Second), "invoke", 2},
		{look, at(3 * time.Second), "", 2},
		{look, at(4 * time.Second), "wmClose", 3},
		{look, at(6 * time.Second), "wmClose", 3},
		{other, at(6500 * time.Millisecond), "ctrlW", 1},
	}
	var l tabLadder
	for i, s := range steps {
		way, next := nextClose(l, "example.com", false, s.look, s.at)
		if way != s.wantWay || next.step != s.wantStep {
			t.Fatalf("шаг %d: way %q, step %d; ждали %q, %d", i+1, way, next.step, s.wantWay, s.wantStep)
		}
		l = next
	}
	if l.window != 9 || l.domain != "example.com" || !l.at.Equal(at(6500*time.Millisecond)) {
		t.Fatalf("лестница после смены окна: %+v", l)
	}
}

// Ловит (RM4-2, PL-9, Review Focus 2): команда исполнилась бы в окне, которое уже не Brave, во
// вкладке с другим доменом или при непрочитанном адресе.
func TestNextCloseNeedsSameBraveTab(t *testing.T) {
	base := TabLook{Window: 7, Brave: true, Domain: "example.com"}
	cases := []struct {
		name   string
		look   TabLook
		domain string
	}{
		{"другая вкладка", TabLook{Window: 7, Brave: true, Domain: "school.example"}, "example.com"},
		{"не Brave", TabLook{Window: 7, Brave: false, Domain: "example.com"}, "example.com"},
		{"адрес не прочитан", TabLook{Window: 7, Brave: true, Domain: "example.com", Unread: true}, "example.com"},
		{"нет окна", TabLook{Window: 0, Brave: true, Domain: "example.com"}, "example.com"},
		{"команда без домена", base, ""},
	}
	for _, c := range cases {
		if way, next := nextClose(tabLadder{}, c.domain, false, c.look, t0); way != "" || next.step != 0 {
			t.Errorf("%s: way %q, step %d; ждали ничего", c.name, way, next.step)
		}
	}
}

// Уточнение оркестратора к T7-3: если после действия вкладок стало меньше, действие удалось —
// следующая вкладка того же сайта закрывается снова Ctrl+W, окно Brave целиком не закрывается.
// Вкладки не посчитаны (0) — считается, что не уменьшились.
func TestNextCloseResetsWhenTabsDecreased(t *testing.T) {
	tabs := func(n int) TabLook { return TabLook{Window: 7, Brave: true, Domain: "example.com", Tabs: n} }
	steps := []struct {
		look     TabLook
		at       time.Time
		wantWay  string
		wantStep int
	}{
		{tabs(3), t0, "ctrlW", 1},
		{tabs(2), at(time.Second), "ctrlW", 1},
		{tabs(2), at(2 * time.Second), "", 1},
		{tabs(2), at(3 * time.Second), "invoke", 2},
		{tabs(1), at(4 * time.Second), "ctrlW", 1},
		{tabs(1), at(6 * time.Second), "invoke", 2},
		{tabs(0), at(7 * time.Second), "", 2},
		{tabs(0), at(8 * time.Second), "wmClose", 3},
		{tabs(1), at(10 * time.Second), "wmClose", 3},
		{tabs(3), at(12 * time.Second), "wmClose", 3},
		{tabs(2), at(12500 * time.Millisecond), "ctrlW", 1},
	}
	var l tabLadder
	for i, s := range steps {
		way, next := nextClose(l, "example.com", false, s.look, s.at)
		if way != s.wantWay || next.step != s.wantStep {
			t.Fatalf("шаг %d (вкладок %d): way %q, step %d; ждали %q, %d", i+1, s.look.Tabs, way, next.step, s.wantWay, s.wantStep)
		}
		l = next
	}
}

// Ловит (RM4-1, T7-6): окно Brave без адреса закрывалось бы, когда адрес уже прочитан, чаще раза в
// 2 с или не в Brave.
func TestNextCloseBraveWindow(t *testing.T) {
	look := TabLook{Window: 7, Brave: true, Unread: true}
	steps := []struct {
		look    TabLook
		at      time.Time
		wantWay string
	}{
		{look, t0, "wmClose"},
		{look, at(time.Second), ""},
		{look, at(2 * time.Second), "wmClose"},
		{TabLook{Window: 7, Brave: true, Unread: false}, at(5 * time.Second), ""},
		{TabLook{Window: 7, Brave: false, Unread: true}, at(5 * time.Second), ""},
		{TabLook{Window: 0, Brave: true, Unread: true}, at(5 * time.Second), ""},
		{TabLook{Window: 9, Brave: true, Unread: true}, at(2500 * time.Millisecond), "wmClose"},
	}
	var l tabLadder
	for i, s := range steps {
		way, next := nextClose(l, "", true, s.look, s.at)
		if way != s.wantWay {
			t.Fatalf("шаг %d: way %q; ждали %q", i+1, way, s.wantWay)
		}
		l = next
	}
}

// Плашки сайтов (RM4-9) уступают плашке 1 мин; плашки о программах — нет (M3 без изменений).
func TestIsSitePlate(t *testing.T) {
	for kind, want := range map[string]bool{
		ipc.BlockedSite: true, ipc.BlockedAdult: true, ipc.BlockedVideoOver: true, ipc.BlockedBrowser: true,
		ipc.BlockedForbidden: false, ipc.BlockedUnknown: false, ipc.BlockedStudyOnly: false,
		ipc.BlockedGamesOver: false, ipc.BlockedDecided: false, "": false,
	} {
		if got := isSitePlate(kind); got != want {
			t.Errorf("isSitePlate(%q) = %v, ждали %v", kind, got, want)
		}
	}
}
