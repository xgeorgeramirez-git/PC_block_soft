package systray

import (
	"strings"
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

func freshSnapshot(st ipc.ChildStatus) Snapshot {
	return Snapshot{Status: st, Has: true, Fresh: true}
}

func labels(v PanelView) []string {
	var out []string
	for _, b := range v.Buttons {
		out = append(out, b.Label)
	}
	return out
}

// Ловит: в панели ребёнка нет «Моей статистики» или «Настроек для родителя», полоса
// показывает не остаток, строка не повторяет сводку службы.
func TestBuildPanelChild(t *testing.T) {
	st := okStatus(85*60, 7200)
	v := BuildPanel(freshSnapshot(st), "")
	if v.Title != "Сегодня осталось 1 ч 25 мин из 2 ч" || v.Error {
		t.Fatalf("заголовок: %+v", v)
	}
	if v.Bar == nil || v.Bar.Status != StatusNormal || v.Bar.Fraction < 0.70 || v.Bar.Fraction > 0.71 {
		t.Fatalf("полоса: %+v", v.Bar)
	}
	if got := strings.Join(labels(v), "|"); got != "Моя статистика|Настройки для родителя" {
		t.Fatalf("кнопки: %s", got)
	}
	if !v.Buttons[0].Main || v.Buttons[0].Small || !v.Buttons[1].Small || v.Buttons[1].Main {
		t.Fatalf("«Моя статистика» главная, «Настройки для родителя» мелкая: %+v", v.Buttons)
	}
	if v.Buttons[0].Kind != PanelMyStats || v.Buttons[1].Kind != PanelParent {
		t.Fatalf("виды кнопок: %+v", v.Buttons)
	}
}

func TestBuildPanelBarColorFollowsStatus(t *testing.T) {
	low := BuildPanel(freshSnapshot(okStatus(10*60, 7200)), "")
	if low.Bar == nil || low.Bar.Status != StatusLow {
		t.Fatalf("янтарная полоса: %+v", low.Bar)
	}
	out := ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusTimeUp, Lock: core.LockTimeUp, Reason: core.ReasonTimeUp, LimitS: 7200})
	v := BuildPanel(freshSnapshot(out), "")
	if v.Bar == nil || v.Bar.Status != StatusOut || v.Bar.Fraction != 0 {
		t.Fatalf("красная пустая полоса: %+v", v.Bar)
	}
	if v.Title != core.ReasonTimeUp {
		t.Fatalf("заголовок: %q", v.Title)
	}
}

// Ловит: без лимита или в безопасном режиме рисуется полоса «0 из 0».
func TestBuildPanelNoBarWithoutLimit(t *testing.T) {
	for name, st := range map[string]ipc.ChildStatus{
		"без лимита": ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusUnlimited, Unlimited: true}),
		"безопасный": ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusSafe, Unlimited: true}),
		"админ":      ipc.ChildStatusAdmin("Миша"),
	} {
		v := BuildPanel(freshSnapshot(st), "")
		if v.Bar != nil {
			t.Errorf("%s: полосы быть не должно: %+v", name, v.Bar)
		}
		if v.Title == "" {
			t.Errorf("%s: пустой заголовок", name)
		}
	}
	v := BuildPanel(freshSnapshot(ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusUnlimited, Unlimited: true})), "")
	if v.Title != "Сегодня без ограничений по времени" {
		t.Fatalf("без лимита: %q", v.Title)
	}
}

// Ловит: у учётки не ребёнка в панели появляется таймер или «Моя статистика»
// (ТЗ 8.2.1: значок без таймера и только «Настройки для родителя»).
func TestBuildPanelNotChild(t *testing.T) {
	v := BuildPanel(freshSnapshot(ipc.ChildStatusNotChild()), "")
	if v.Bar != nil || v.Error {
		t.Fatalf("не ребёнок: %+v", v)
	}
	if len(v.Buttons) != 1 || v.Buttons[0].Kind != PanelParent || !v.Buttons[0].Main {
		t.Fatalf("только «Настройки для родителя»: %+v", v.Buttons)
	}
	if v.Title != "В этой учётной записи время не ограничено" {
		t.Fatalf("заголовок: %q", v.Title)
	}
}

// Ловит: при ошибке связи в панели старые цифры вместо «Не удалось получить данные»
// и нет кнопки «Обновить» (ТЗ 8.2.1 п.5, урок L-114).
func TestBuildPanelError(t *testing.T) {
	for name, s := range map[string]Snapshot{
		"ещё не отвечала": {},
		"опрос не удался": {Status: okStatus(85*60, 7200), Has: true, Failed: true, Fresh: true},
		"ответ устарел":   {Status: okStatus(85*60, 7200), Has: true, Fresh: false},
	} {
		v := BuildPanel(s, "")
		if !v.Error || v.Title != "Не удалось получить данные. Попробуй через минуту" || v.Bar != nil {
			t.Errorf("%s: %+v", name, v)
		}
		if len(v.Buttons) != 1 || v.Buttons[0].Kind != PanelRefresh || v.Buttons[0].Label != "Обновить" {
			t.Errorf("%s: кнопки %+v", name, v.Buttons)
		}
	}
}

// Ловит: родителю в панели обращение на «ты».
func TestBuildPanelErrorForParentUsesVy(t *testing.T) {
	v := BuildPanel(Snapshot{Status: ipc.ChildStatusNotChild(), Has: true, Failed: true}, "")
	if v.Title != "Не удалось получить данные. Попробуйте через минуту" {
		t.Fatalf("родителю на «вы»: %q", v.Title)
	}
}

func TestBuildPanelKeepsNote(t *testing.T) {
	v := BuildPanel(freshSnapshot(okStatus(85*60, 7200)), "Не получилось открыть окно. Попробуй ещё раз")
	if v.Note == "" {
		t.Fatal("итог последнего нажатия потерян")
	}
}

func TestOpenKind(t *testing.T) {
	if k, ok := PanelMyStats.OpenKind(); !ok || k != ipc.WindowMyStats {
		t.Fatalf("моя статистика: %q %v", k, ok)
	}
	if k, ok := PanelParent.OpenKind(); !ok || k != ipc.WindowParent {
		t.Fatalf("родитель: %q %v", k, ok)
	}
	if _, ok := PanelRefresh.OpenKind(); ok {
		t.Fatal("«Обновить» окон не открывает")
	}
}

func TestOpenFailedText(t *testing.T) {
	if got := OpenFailedText("Служба занята. Попробуйте позже", false); got != "Служба занята. Попробуйте позже" {
		t.Fatalf("текст службы теряется: %q", got)
	}
	if got := OpenFailedText("", false); !strings.Contains(got, "Попробуй ещё раз") {
		t.Fatalf("ребёнку на «ты»: %q", got)
	}
	if got := OpenFailedText("", true); !strings.Contains(got, "Попробуйте ещё раз") {
		t.Fatalf("родителю на «вы»: %q", got)
	}
}

func TestRemainingFraction(t *testing.T) {
	cases := []struct {
		rem, limit int
		want       float64
	}{{0, 100, 0}, {-5, 100, 0}, {100, 100, 1}, {150, 100, 1}, {25, 100, 0.25}, {5, 0, 0}}
	for _, c := range cases {
		if got := remainingFraction(c.rem, c.limit); got != c.want {
			t.Errorf("remainingFraction(%d, %d) = %v, ждали %v", c.rem, c.limit, got, c.want)
		}
	}
}

// Ловит: панель выходит за экран или закрывает значок: при панели задач внизу она над ним,
// при панели сверху под ним, у краёв прижимается, на крошечной области не уходит в минус.
func TestPlacePanel(t *testing.T) {
	work := Rect{0, 0, 1920, 1040} // панель задач внизу: 1040...1080
	cases := []struct {
		name         string
		anchor, work Rect
		w, h         int
		wantX, wantY int
	}{
		{"значок посреди панели задач", Rect{900, 1048, 916, 1072}, work, 320, 200, 908 - 160, 832},
		{"значок справа: прижимается к краю", Rect{1800, 1048, 1816, 1072}, work, 320, 200, 1920 - 320 - 8, 832},
		{"значок слева: прижимается к краю", Rect{4, 1048, 20, 1072}, work, 320, 200, 8, 832},
		{"панель задач сверху: под значком", Rect{900, 4, 916, 28}, Rect{0, 40, 1920, 1080}, 320, 200, 908 - 160, 48},
		{"панель задач справа: слева от края", Rect{1904, 500, 1920, 520}, Rect{0, 0, 1880, 1080}, 320, 200, 1880 - 320 - 8, 528},
		{"выше рабочей области не выходит", Rect{900, 1048, 916, 1072}, work, 320, 1100, 908 - 160, 8},
	}
	for _, c := range cases {
		x, y := PlacePanel(c.anchor, c.work, c.w, c.h)
		if x != c.wantX || y != c.wantY {
			t.Errorf("%s: получили (%d, %d), ждали (%d, %d)", c.name, x, y, c.wantX, c.wantY)
		}
	}
}

func TestPlacePanelTinyWorkAreaDoesNotGoNegative(t *testing.T) {
	x, y := PlacePanel(Rect{10, 10, 26, 34}, Rect{0, 0, 100, 100}, 320, 400)
	if x != 8 || y != 8 {
		t.Fatalf("(%d, %d), ждали прижатие к левому верхнему углу (8, 8)", x, y)
	}
}

func TestPanelTextsHaveNoDashes(t *testing.T) {
	for _, s := range []string{
		textPanelError, textPanelErrorParent, textPanelNotChild, textButtonMyStats, textButtonParent,
		textButtonRefresh, textOpenFailed, textOpenFailedParent, tooltipWaiting, tooltipNoData,
	} {
		if strings.ContainsAny(s, "‒–—―") {
			t.Errorf("в тексте %q длинное тире", s)
		}
	}
}
