package service

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
)

var tzMSK = time.FixedZone("MSK", 3*3600)

// Ловит: ключа реестра нет, а тестовый режим включается (ТЗ 10.8: в обычной установке его нет),
// или пустой ключ не включает режим со значениями по умолчанию.
func TestParseTestModeEnabledOnlyWithKey(t *testing.T) {
	off, err := ParseTestMode(nil, tzMSK)
	if err != nil || off.Enabled {
		t.Fatalf("нет ключа: Enabled=%v err=%v, ждали выключено", off.Enabled, err)
	}
	on, err := ParseTestMode(map[string]string{}, tzMSK)
	if err != nil || !on.Enabled {
		t.Fatalf("пустой ключ: Enabled=%v err=%v, ждали включено", on.Enabled, err)
	}
	if on.ClockSpeed != 1 || on.DataDir != "" || on.TestChildSID != "" || !on.ClockStart.IsZero() || !on.ClockRealStart.IsZero() {
		t.Fatalf("значения по умолчанию: %+v", on)
	}
}

// Ловит: значения реестра разбираются не в те поля, пробелы по краям ломают разбор,
// местное время без пояса читается в UTC, а не в поясе компьютера.
func TestParseTestModeValues(t *testing.T) {
	tm, err := ParseTestMode(map[string]string{
		"DataDir":        ` C:\Temp\pcb `,
		"ClockSpeed":     " 60 ",
		"ClockStart":     "2026-10-19T15:00:00",
		"ClockRealStart": "2026-10-19T12:05:09Z",
		"TestChildSID":   " S-1-5-21-1-2-3-1001 ",
		"Чужое":          "игнорируется",
	}, tzMSK)
	if err != nil {
		t.Fatal(err)
	}
	if tm.DataDir != `C:\Temp\pcb` || tm.ClockSpeed != 60 || tm.TestChildSID != "S-1-5-21-1-2-3-1001" {
		t.Fatalf("поля: %+v", tm)
	}
	if want := time.Date(2026, 10, 19, 15, 0, 0, 0, tzMSK); !tm.ClockStart.Equal(want) {
		t.Fatalf("ClockStart = %v, ждали %v (местное время в поясе компьютера)", tm.ClockStart, want)
	}
	if want := time.Date(2026, 10, 19, 12, 5, 9, 0, time.UTC); !tm.ClockRealStart.Equal(want) {
		t.Fatalf("ClockRealStart = %v, ждали %v", tm.ClockRealStart, want)
	}
}

// Ловит: форматы момента: с поясом (RFC 3339) принимается как есть, пробел вместо «T» тоже.
func TestParseTestModeMomentFormats(t *testing.T) {
	for _, s := range []string{"2026-10-19T15:00:00+03:00", "2026-10-19 15:00:00", "2026-10-19T12:00:00Z"} {
		tm, err := ParseTestMode(map[string]string{"ClockStart": s}, tzMSK)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if want := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC); !tm.ClockStart.Equal(want) {
			t.Fatalf("%q: %v, ждали %v", s, tm.ClockStart, want)
		}
	}
}

// Ловит: мусор в значениях тестового режима молча принимается: служба тогда пошла бы с часами
// «по умолчанию», и тест S2 прошёл бы в реальном времени; ошибка должна быть человеческой.
func TestParseTestModeRejectsGarbage(t *testing.T) {
	bad := []map[string]string{
		{"ClockSpeed": "0"},
		{"ClockSpeed": "-5"},
		{"ClockSpeed": "быстро"},
		{"ClockSpeed": "1001"},
		{"ClockStart": "вчера"},
		{"ClockRealStart": "2026-13-45T00:00:00Z"},
		{"TestChildSID": "pcbtest"},
		{"LockAutoSwitchS": "0"},
		{"LockAutoSwitchS": "-1"},
		{"LockAutoSwitchS": "901"},
		{"LockAutoSwitchS": "скоро"},
	}
	for _, v := range bad {
		_, err := ParseTestMode(v, tzMSK)
		if err == nil {
			t.Errorf("%v: ждали ошибку", v)
			continue
		}
		for k := range v {
			if !strings.Contains(err.Error(), k) {
				t.Errorf("%v: в тексте ошибки нет имени значения %s: %q", v, k, err)
			}
		}
	}
}

// Ловит: период тика не равен 1 с / ClockSpeed (Ruling 4: каждый тик это одна секунда часов ядра).
func TestTestModeTickInterval(t *testing.T) {
	for speed, want := range map[int]time.Duration{1: time.Second, 60: time.Second / 60, 1000: time.Millisecond} {
		if got := (TestMode{Enabled: true, ClockSpeed: speed}).TickInterval(); got != want {
			t.Errorf("скорость %d: период %v, ждали %v", speed, got, want)
		}
	}
	if got := (TestMode{}).TickInterval(); got != time.Second {
		t.Errorf("режим выключен: период %v, ждали 1 с", got)
	}
}

// Ловит: в обычном режиме часы ядра не настоящие (учёт пошёл бы по чужому времени), а в
// тестовом без ускорения и без ClockStart остаются настоящими.
func TestTestModeClockKinds(t *testing.T) {
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	realNow := func() time.Time { return now }
	if _, ok := (TestMode{}).NewClock(realNow).(core.SystemClock); !ok {
		t.Error("обычный режим: ждали core.SystemClock")
	}
	if _, ok := (TestMode{Enabled: true, ClockSpeed: 1}).NewClock(realNow).(core.SystemClock); !ok {
		t.Error("тестовый режим без ускорения и ClockStart: ждали core.SystemClock")
	}
	if _, ok := (TestMode{Enabled: true, ClockSpeed: 60}).NewClock(realNow).(*core.ScaledClock); !ok {
		t.Error("ускорение 60: ждали *core.ScaledClock")
	}
	if _, ok := (TestMode{Enabled: true, ClockSpeed: 1, ClockStart: now}).NewClock(realNow).(*core.ScaledClock); !ok {
		t.Error("задан ClockStart: ждали *core.ScaledClock")
	}
}

// Ловит: после перезапуска службы время ядра идёт заново от ClockStart, а не продолжается
// (Ruling 4, ClockRealStart: «чтобы после перезапуска время ядра продолжалось, а не шло назад»).
func TestTestModeClockContinuesFromRealStart(t *testing.T) {
	start := time.Date(2026, 10, 19, 15, 0, 0, 0, tzMSK)
	realStart := time.Now().Add(-10 * time.Minute)
	c := TestMode{Enabled: true, ClockSpeed: 60, ClockStart: start, ClockRealStart: realStart}.NewClock(time.Now)
	got := c.Now().Sub(start)
	want := 10 * time.Minute * 60 // десять настоящих минут при ускорении 60
	if d := got - want; d < -2*time.Second*60 || d > 2*time.Second*60 {
		t.Fatalf("прошло %v виртуального времени, ждали около %v", got, want)
	}
}

// Ловит: без ClockRealStart время начинается с ClockStart в момент создания часов (а не с
// «сейчас» и не с нуля).
func TestTestModeClockWithoutRealStartBeginsAtClockStart(t *testing.T) {
	start := time.Date(2026, 10, 19, 15, 0, 0, 0, tzMSK)
	c := TestMode{Enabled: true, ClockSpeed: 60, ClockStart: start}.NewClock(time.Now)
	if d := c.Now().Sub(start); d < 0 || d > 5*time.Second*60 {
		t.Fatalf("сдвиг от ClockStart %v, ждали почти 0", d)
	}
}

// Ловит: срок автосмены пользователя из реестра читается не в то поле, принимается больше
// 15 минут (тестовый режим только сокращает срок) или по умолчанию не нулевой.
func TestParseTestModeLockAutoSwitch(t *testing.T) {
	def, err := ParseTestMode(map[string]string{}, tzMSK)
	if err != nil || def.LockAutoSwitchS != 0 {
		t.Fatalf("по умолчанию: %+v, %v", def, err)
	}
	for in, want := range map[string]int{" 20 ": 20, "1": 1, "900": 900} {
		tm, err := ParseTestMode(map[string]string{"LockAutoSwitchS": in}, tzMSK)
		if err != nil || tm.LockAutoSwitchS != want {
			t.Fatalf("%q: %+v, %v; ждали %d", in, tm, err, want)
		}
	}
}
