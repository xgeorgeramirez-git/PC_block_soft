package guard

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/systray"
)

// Экран перерыва на отдых (8.2.3 п.12): решения окна вне Windows (PL-23). Ожидания — литералы.

// Ловит: таймер экрана перерыва нарисован не плитками табло (свой расчёт мест) или не крупнее
// при масштабе 150 %; цифры янтарные или красные, как у кончающегося времени (перерыв — не конец).
func TestRestTimerTiles(t *testing.T) {
	l := RestTiles(96)
	if l.W != 362 || l.H != 129 || l.Tiles[0] != (systray.Rect{Left: 11, Top: 11, Right: 54, Bottom: 81}) {
		t.Fatalf("плитки при 100 %%: %d×%d, первая %v", l.W, l.H, l.Tiles[0])
	}
	if big := RestTiles(144); big.Tiles[0].Dx() != 65 || big.Tiles[0].Dy() != 106 {
		t.Errorf("плитка при 150 %%: %d×%d, ждали 65×106", big.Tiles[0].Dx(), big.Tiles[0].Dy())
	}
	v := RestTimerView(RestView{Tiles: [3]string{"00", "14", "59"}})
	if v.Kind != systray.CountdownTime || v.Hours != "00" || v.Minutes != "14" || v.Seconds != "59" || v.Status != systray.StatusNormal {
		t.Errorf("табло таймера %+v", v)
	}
}

// Ловит: картинка не уменьшается на низком экране (столбец уходит за нижний край, кнопки не
// видны), уменьшается на высоком, или остаётся крошкой вместо того, чтобы не показываться.
func TestRestCatSize(t *testing.T) {
	cases := []struct {
		name           string
		dpi            int
		areaH, columnH int32
		want           int32
	}{
		{"1080 строк, места хватает", 96, 1080, 600, 160},
		{"150 %, места хватает", 144, 1440, 900, 240},
		{"768 строк: картинка меньше", 96, 768, 600, 108},
		{"768 строк: ровно 64", 96, 768, 644, 64},
		{"768 строк с паролем: без картинки", 96, 768, 660, 0},
		{"столбец выше экрана", 96, 600, 900, 0},
	}
	for _, c := range cases {
		if got := RestCatSize(c.dpi, c.areaH, c.columnH); got != c.want {
			t.Errorf("%s: картинка %d, ждали %d", c.name, got, c.want)
		}
	}
}

// Ловит: прозрачные места картинки и плиток на тёмном фоне экрана становятся чёрными или
// светлыми (фон не подложен), непрозрачные меняют цвет, порядок байт BGRA перепутан.
func TestOverBackground(t *testing.T) {
	px := []byte{
		0, 0, 0, 0, // прозрачный
		10, 20, 200, 255, // непрозрачный: B=10, G=20, R=200
		50, 50, 50, 128, // половина белого 100 предумноженного
	}
	got := OverBackground(px, 0x15, 0x18, 0x1B)
	want := []byte{
		0x1B, 0x18, 0x15, 255,
		10, 20, 200, 255,
		50 + 0x1B*127/255, 50 + 0x18*127/255, 50 + 0x15*127/255, 255,
	}
	if string(got) != string(want) {
		t.Fatalf("пиксели %v, ждали %v", got, want)
	}
	if px[3] != 0 {
		t.Error("исходные пиксели изменены")
	}
}

// Ловит: рисунок плиток таймера не подложен фоном экрана (углы не цвета фона) или плитки
// потерялись при переводе в BGRA.
func TestRestTimerPixels(t *testing.T) {
	l := RestTiles(96)
	px := RestTimerPixels(l, 0x15, 0x18, 0x1B)
	if len(px) != 362*129*4 {
		t.Fatalf("размер %d", len(px))
	}
	if string(px[:4]) != string([]byte{0x1B, 0x18, 0x15, 255}) {
		t.Errorf("угол %v: ждали фон экрана", px[:4])
	}
	tl := l.Tiles[0]
	o := ((tl.Top+4)*l.W + (tl.Left+tl.Right)/2) * 4
	if b, g, r := px[o], px[o+1], px[o+2]; r < 0xF0 || b < 0xF8 || g < 0xF0 {
		t.Errorf("плитка сверху B=%d G=%d R=%d: нужна почти белая", b, g, r)
	}
}

// Ловит: окно экрана перерыва перестраивается каждую четверть секунды (кнопки двигаются,
// фокус прыгает), потому что подпись вида меняется со сменой секунд таймера или адресом
// RestView; или наоборот не перестраивается при смене сообщения родителя.
func TestLayoutKeyIgnoresTimer(t *testing.T) {
	m := NewModel()
	m.OnWarnings(t0, restDecision(15*60), false, nil)
	a, b := m.View(at(0)), m.View(at(2*time.Second))
	if a.Rest == nil || b.Rest == nil || a.Rest.Tiles == b.Rest.Tiles {
		t.Fatalf("таймер не идёт: %v → %v", a.Rest, b.Rest)
	}
	if LayoutKey(a, "") != LayoutKey(b, "") {
		t.Error("смена секунд таймера перестраивает окно")
	}
	if !TimerChanged(a, b) || TimerChanged(a, a) {
		t.Error("смена секунд не замечена или замечена без смены")
	}
	c := b
	c.Rest = &RestView{Tiles: b.Rest.Tiles, Message: "Пойди погуляй"}
	if LayoutKey(b, "") == LayoutKey(c, "") {
		t.Error("смена сообщения родителя не перестраивает окно")
	}
	if LayoutKey(b, "") == LayoutKey(b, "Не получилось") {
		t.Error("сообщение стража не перестраивает окно")
	}
	if !strings.Contains(LayoutKey(b, ""), "Перерыв на отдых") {
		t.Error("в подписи нет заголовка экрана")
	}
	w := b
	w.Warning = &WarningView{Text: "плашка"}
	if LayoutKey(b, "") != LayoutKey(w, "") {
		t.Error("плашка (отдельное окно) перестраивает экран блокировки")
	}
}

// Ловит: отчёт стража в тестовом режиме не даёт приёмке S43 проверить экран перерыва: нет вида
// экрана, таймера (уменьшается вживую) или сообщения родителя; у обычного экрана они не пусты.
func TestStateReportsRestScreen(t *testing.T) {
	m := NewModel()
	m.OnWarnings(t0, restDecision(15*60), false, nil)
	st := StateFromView(m.View(at(time.Second)), ipc.LockRest, nil, nil, map[string]string{})
	if st.LockKind != "rest" || st.RestTimer != "00:14:59" || st.RestMessage != "Пойди погуляй с собакой" {
		t.Fatalf("отчёт %+v", st)
	}
	if strings.Join(st.Buttons, ",") != "parentPassword,shutdown,logoff,switchUser" {
		t.Errorf("кнопки %v", st.Buttons)
	}
	m2 := NewModel()
	m2.OnWarnings(t0, timeUpDecision(), false, nil)
	if st := StateFromView(m2.View(at(0)), ipc.LockTimeUp, nil, nil, map[string]string{}); st.RestTimer != "" || st.RestMessage != "" {
		t.Errorf("у экрана «время вышло» таймер перерыва %q и сообщение %q", st.RestTimer, st.RestMessage)
	}
}
