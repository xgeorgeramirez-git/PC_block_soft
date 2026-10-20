package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"pcblock/internal/core"
)

// Тестовый режим (ТЗ 10.8, Ruling 4): ключ реестра HKLM\SOFTWARE\PCBlock\TestMode, писать в
// который может только администратор; нет ключа: обычный режим. Чтение реестра лежит в
// подпакете winhost, здесь значения разбираются как чистые данные.
const (
	// TestModeKeyPath — ключ реестра тестового режима (под HKLM).
	TestModeKeyPath = `SOFTWARE\PCBlock\TestMode`

	valDataDir        = "DataDir"
	valClockSpeed     = "ClockSpeed"
	valClockStart     = "ClockStart"
	valClockRealStart = "ClockRealStart"
	valTestChildSID   = "TestChildSID"
	valLockAutoSwitch = "LockAutoSwitchS"

	// MaxClockSpeed — наибольшее ускорение: период тика не короче миллисекунды.
	MaxClockSpeed = 1000

	// MaxLockAutoSwitchS — наибольший срок автосмены пользователя в тестовом режиме, секунд:
	// те же 15 минут, что в обычном режиме. Тестовый режим срок только сокращает.
	MaxLockAutoSwitchS = 900
)

// TestModeValueNames — значения ключа, которые читает служба.
var TestModeValueNames = []string{valDataDir, valClockSpeed, valClockStart, valClockRealStart, valTestChildSID, valLockAutoSwitch}

// TestMode — значения тестового режима.
type TestMode struct {
	Enabled        bool      // ключ реестра есть
	DataDir        string    // временная папка данных вместо C:\ProgramData\PCBlock
	ClockSpeed     int       // ускорение часов ядра, целое от 1; в выключенном режиме 1
	ClockStart     time.Time // момент часов ядра в начале теста (пн 19.10.2026 15:00); нулевой: настоящее время
	ClockRealStart time.Time // настоящий момент начала теста; нулевой: момент создания часов
	TestChildSID   string    // учётка раннера: для неё сняты запрет A2 и правило R1
	// LockAutoSwitchS — срок автоматической смены пользователя на экране блокировки в секундах
	// (1 до 900); 0: обычные 15 минут. Приёмка не ждёт 15 минут настоящих часов.
	LockAutoSwitchS int
}

// momentLayouts — допустимые записи момента: с поясом или местное время без пояса.
var momentLayouts = []struct {
	layout string
	local  bool
}{
	{time.RFC3339, false},
	{"2006-01-02T15:04:05", true},
	{"2006-01-02 15:04:05", true},
}

func parseMoment(s string, loc *time.Location) (time.Time, error) {
	for _, l := range momentLayouts {
		var t time.Time
		var err error
		if l.local {
			t, err = time.ParseInLocation(l.layout, s, loc)
		} else {
			t, err = time.Parse(l.layout, s)
		}
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q не похоже на дату и время", s)
}

// ParseTestMode разбирает значения ключа реестра (строки). values == nil: ключа нет, режим
// выключен; пустая карта: ключ есть, значения по умолчанию. Время без пояса читается в поясе
// loc (nil: time.Local). Неразобранное значение даёт ошибку с именем значения: тихо принятый
// мусор запустил бы приёмку в обычном темпе. Лишние значения игнорируются.
func ParseTestMode(values map[string]string, loc *time.Location) (TestMode, error) {
	if values == nil {
		return TestMode{ClockSpeed: 1}, nil
	}
	if loc == nil {
		loc = time.Local
	}
	tm := TestMode{Enabled: true, ClockSpeed: 1}
	tm.DataDir = strings.TrimSpace(values[valDataDir])
	if s := strings.TrimSpace(values[valClockSpeed]); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > MaxClockSpeed {
			return TestMode{}, fmt.Errorf("Тестовый режим: %s должно быть целым числом от 1 до %d, а не %q", valClockSpeed, MaxClockSpeed, s)
		}
		tm.ClockSpeed = n
	}
	var err error
	if s := strings.TrimSpace(values[valClockStart]); s != "" {
		if tm.ClockStart, err = parseMoment(s, loc); err != nil {
			return TestMode{}, fmt.Errorf("Тестовый режим: %s: %w", valClockStart, err)
		}
	}
	if s := strings.TrimSpace(values[valClockRealStart]); s != "" {
		if tm.ClockRealStart, err = parseMoment(s, loc); err != nil {
			return TestMode{}, fmt.Errorf("Тестовый режим: %s: %w", valClockRealStart, err)
		}
	}
	if s := strings.TrimSpace(values[valTestChildSID]); s != "" {
		if !strings.HasPrefix(s, "S-1-") {
			return TestMode{}, fmt.Errorf("Тестовый режим: %s должен быть идентификатором вида S-1-5-21-..., а не %q", valTestChildSID, s)
		}
		tm.TestChildSID = s
	}
	if s := strings.TrimSpace(values[valLockAutoSwitch]); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > MaxLockAutoSwitchS {
			return TestMode{}, fmt.Errorf("Тестовый режим: %s должно быть целым числом от 1 до %d, а не %q", valLockAutoSwitch, MaxLockAutoSwitchS, s)
		}
		tm.LockAutoSwitchS = n
	}
	return tm, nil
}

// TickInterval — настоящий период тика службы: одна секунда часов ядра, то есть 1 с / ClockSpeed
// (Ruling 4: каждый тик это секунда учёта, правило разрыва Ruling 13 работает как обычно).
func (t TestMode) TickInterval() time.Duration {
	if t.ClockSpeed < 1 {
		return time.Second
	}
	return time.Second / time.Duration(t.ClockSpeed)
}

// NewClock строит часы ядра: в обычном режиме настоящие, в тестовом с ускорением или с
// ClockStart это ScaledClock. Пустой ClockStart берёт настоящее время, пустой ClockRealStart
// момент создания часов; чтобы время продолжалось после перезапуска службы, ClockRealStart
// записывается при включении режима. realNow задаёт «сейчас» для этих значений по умолчанию.
func (t TestMode) NewClock(realNow func() time.Time) core.Clock {
	if !t.Enabled || (t.ClockSpeed <= 1 && t.ClockStart.IsZero()) {
		return core.SystemClock{}
	}
	now := realNow()
	virtual, realStart := t.ClockStart, t.ClockRealStart
	if virtual.IsZero() {
		virtual = now
	}
	if realStart.IsZero() {
		realStart = now
	}
	return core.NewScaledClock(virtual, realStart, max(t.ClockSpeed, 1))
}
