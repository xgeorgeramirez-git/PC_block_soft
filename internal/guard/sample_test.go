package guard

import (
	"math"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

func TestScaleIdle(t *testing.T) {
	cases := []struct {
		ms    int64
		speed int
		want  int64
	}{
		{1500, 1, 1500},
		{1500, 0, 1500},
		{1500, -3, 1500},
		{1500, 10, 15000},
		{0, 1000, 0},
		{-5, 10, 0},
		{math.MaxInt64, 2, math.MaxInt64},
		{math.MaxInt64 / 1000, 1000, (math.MaxInt64 / 1000) * 1000},
	}
	for _, c := range cases {
		if got := ScaleIdle(c.ms, c.speed); got != c.want {
			t.Errorf("ScaleIdle(%d, %d) = %d, ждали %d", c.ms, c.speed, got, c.want)
		}
	}
}

func TestBuildSampleFillsFieldsAndKeepsNothingElse(t *testing.T) {
	loc := time.FixedZone("MSK", 3*3600)
	now := time.Date(2026, 10, 19, 15, 0, 7, 0, loc)
	game := `C:\Users\Миша\Downloads\SuperGame.exe`
	s := BuildSample(now, RawSensors{IdleMs: 4200, Fullscreen: true, ForegroundPath: game}, 1)
	// Категорию по пути определяет служба (RM3-1): страж шлёт путь как есть и «Прочее».
	want := ipc.SensorSample{
		AtMs: now.UnixMilli(), TzOffsetMin: 180, SessionActive: true,
		LastInputAgoMs: 4200, Fullscreen: true, Foreground: ipc.CatNameOther, ForegroundPath: game,
	}
	if s != want {
		t.Fatalf("снимок:\n%+v\nждали\n%+v", s, want)
	}
	if got := BuildSample(now, RawSensors{Locked: true}, 1); got.SessionActive {
		t.Fatal("заблокированный сеанс назван активным")
	}
	west := time.FixedZone("EST", -5*3600)
	if got := BuildSample(now.In(west), RawSensors{}, 1).TzOffsetMin; got != -300 {
		t.Fatalf("пояс к западу: %d, ждали -300", got)
	}
	if got := BuildSample(now, RawSensors{IdleMs: 3000}, 20).LastInputAgoMs; got != 60000 {
		t.Fatalf("простой в тестовом режиме: %d, ждали 60000", got)
	}
}

// Ловит (R23, RM3-1, PL-9): путь длиннее предела канала уходит службе (служба его отбросит,
// но страж не должен слать лишнее), окна PC Block и самого стража считаются программой ребёнка.
func TestForegroundPathFor(t *testing.T) {
	const own, dir = 4242, `C:\Program Files\PC Block`
	long := `C:\` + strings.Repeat("я", 517) // 520 символов
	cases := []struct {
		name string
		path string
		pid  uint32
		want string
	}{
		{"программа ребёнка", `C:\Users\Миша\Downloads\SuperGame.exe`, 100, `C:\Users\Миша\Downloads\SuperGame.exe`},
		{"окно самого стража", `C:\Windows\Temp\guard.exe`, own, ""},
		{"значок PC Block", `C:\Program Files\PC Block\PCBlock.exe`, 200, ""},
		{"регистр каталога установки", `c:\program files\pc block\PCBlock.exe`, 200, ""},
		{"соседний каталог с тем же началом", `C:\Program Files\PC Block Games\x.exe`, 200, `C:\Program Files\PC Block Games\x.exe`},
		{"путь не прочитан", "", 200, ""},
		{"нет процесса", `C:\x.exe`, 0, ""},
		{"ровно 520 символов", long, 200, long},
		{"521 символ", long + "z", 200, ""},
	}
	for _, c := range cases {
		if got := ForegroundPathFor(c.path, c.pid, own, dir); got != c.want {
			t.Errorf("%s: %q, ждали %q", c.name, got, c.want)
		}
	}
	if got := ForegroundPathFor(`C:\Program Files\PC Block\PCBlock.exe`, 200, own, dir+`\`); got != "" {
		t.Errorf("каталог установки с разделителем в конце: %q", got)
	}
	if got := ForegroundPathFor(`C:\a.exe`, 200, own, ""); got != `C:\a.exe` {
		t.Errorf("каталог установки не известен: %q", got)
	}
}

func TestSampleToCoreRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 19, 15, 0, 7, 0, time.UTC)
	core := BuildSample(now, RawSensors{IdleMs: 10, Fullscreen: true}, 1).ToCore()
	if core.LastInputAgoMs != 10 || !core.Fullscreen || !core.SessionActive {
		t.Fatalf("снимок для ядра: %+v", core)
	}
}

func TestCoversMonitor(t *testing.T) {
	mon := Rect{0, 0, 1920, 1080}
	cases := []struct {
		name string
		win  Rect
		want bool
	}{
		{"ровно на весь монитор", Rect{0, 0, 1920, 1080}, true},
		{"с запасом за краями", Rect{-8, -8, 1928, 1088}, true},
		{"не хватает снизу панели задач", Rect{0, 0, 1920, 1040}, false},
		{"окно поменьше", Rect{100, 100, 800, 600}, false},
		{"на другом мониторе", Rect{1920, 0, 3840, 1080}, false},
		{"пустое окно", Rect{}, false},
	}
	for _, c := range cases {
		if got := CoversMonitor(c.win, mon); got != c.want {
			t.Errorf("%s: %v, ждали %v", c.name, got, c.want)
		}
	}
	if CoversMonitor(Rect{0, 0, 100, 100}, Rect{}) {
		t.Error("пустой монитор нельзя закрыть")
	}
}

func TestFullscreenNeedsNoCaption(t *testing.T) {
	mon := Rect{0, 0, 1920, 1080}
	full := Rect{0, 0, 1920, 1080}
	if !IsFullscreenWindow(full, mon, 0x90000000) { // WS_POPUP|WS_VISIBLE: плеер без заголовка
		t.Error("окно без заголовка на весь монитор не названо полноэкранным")
	}
	if IsFullscreenWindow(full, mon, 0x16CF0000) { // обычное окно с заголовком, развёрнутое при скрытой панели задач
		t.Error("развёрнутое окно с заголовком названо полноэкранным")
	}
	if IsFullscreenWindow(Rect{0, 0, 800, 600}, mon, 0x90000000) {
		t.Error("маленькое окно названо полноэкранным")
	}
}

func TestDesktopIsNotFullscreen(t *testing.T) {
	for _, class := range []string{"Progman", "WorkerW", "progman", "WORKERW"} {
		if !IsDesktopClass(class) {
			t.Errorf("%q не узнан как рабочий стол (Ruling 24)", class)
		}
	}
	for _, class := range []string{"Chrome_WidgetWin_1", "Notepad", "", "Shell_TrayWnd"} {
		if IsDesktopClass(class) {
			t.Errorf("%q принят за рабочий стол", class)
		}
	}
}

func TestClampSpeed(t *testing.T) {
	cases := []struct {
		in   int64
		want int
	}{{-4, 1}, {0, 1}, {1, 1}, {20, 20}, {1000, 1000}, {1001, 1000}, {math.MaxInt64, 1000}}
	for _, c := range cases {
		if got := ClampSpeed(c.in); got != c.want {
			t.Errorf("ClampSpeed(%d) = %d, ждали %d", c.in, got, c.want)
		}
	}
}
