//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/sys/windows/registry"

	"pcblock/internal/service"
)

// readTestMode читает HKLM\SOFTWARE\PCBlock\TestMode (Ruling 4). Ключа нет: обычный режим.
// Писать в ключ может только администратор, поэтому ребёнок включить его не может.
// Значения могут быть строками или числами (ClockSpeed удобно писать числом).
func readTestMode() (service.TestMode, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, service.TestModeKeyPath, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return service.ParseTestMode(nil, time.Local)
	}
	if err != nil {
		return service.TestMode{}, fmt.Errorf("ключ тестового режима не открыт: %w", err)
	}
	defer k.Close()
	values := map[string]string{}
	for _, name := range service.TestModeValueNames {
		_, typ, err := k.GetValue(name, nil)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil && !errors.Is(err, registry.ErrShortBuffer) {
			return service.TestMode{}, fmt.Errorf("Тестовый режим: значение %s не прочитано: %w", name, err)
		}
		switch typ {
		case registry.DWORD, registry.QWORD:
			n, _, err := k.GetIntegerValue(name)
			if err != nil {
				return service.TestMode{}, fmt.Errorf("Тестовый режим: значение %s не прочитано: %w", name, err)
			}
			values[name] = strconv.FormatUint(n, 10)
		default:
			s, _, err := k.GetStringValue(name)
			if err != nil {
				return service.TestMode{}, fmt.Errorf("Тестовый режим: значение %s не прочитано: %w", name, err)
			}
			values[name] = s
		}
	}
	return service.ParseTestMode(values, time.Local)
}

// testModeOn: ключ тестового режима есть. Служебные ключи приёмки без него не работают.
func testModeOn() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, service.TestModeKeyPath, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return false
	}
	k.Close()
	return true
}
