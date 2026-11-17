//go:build windows

package winhost

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"

	"pcblock/internal/service"
)

// Часовой пояс и безопасный режим компьютера для службы (веха M6, R17б, R17е).

const (
	tzInfoKey  = `SYSTEM\CurrentControlSet\Control\TimeZoneInformation`
	tzListKey  = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Time Zones`
	safeOptKey = `SYSTEM\CurrentControlSet\Control\SafeBoot\Option` // есть только в безопасном режиме
)

// readTimeZone — пояс из реестра (service.Config.TimeZone): ключ TimeZoneKeyName, стандартное
// смещение Bias и DynamicDaylightTimeDisabled; название — MUI_Display (на языке Windows), иначе
// Display. Название читается каждый раз: вызов раз в секунду, это доли миллисекунды.
func readTimeZone() (service.TimeZoneInfo, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, tzInfoKey, registry.QUERY_VALUE)
	if err != nil {
		return service.TimeZoneInfo{}, fmt.Errorf("ветка часового пояса: %w", err)
	}
	defer k.Close()
	key, _, err := k.GetStringValue("TimeZoneKeyName")
	if err != nil {
		return service.TimeZoneInfo{}, fmt.Errorf("TimeZoneKeyName: %w", err)
	}
	bias, _, err := k.GetIntegerValue("Bias")
	if err != nil {
		return service.TimeZoneInfo{}, fmt.Errorf("Bias: %w", err)
	}
	noDST, _, err := k.GetIntegerValue("DynamicDaylightTimeDisabled")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return service.TimeZoneInfo{}, fmt.Errorf("DynamicDaylightTimeDisabled: %w", err)
	}
	z := service.TimeZoneInfo{Key: key, BiasMin: int(int32(uint32(bias))), NoDST: noDST != 0}
	if key != "" {
		if lk, err := registry.OpenKey(registry.LOCAL_MACHINE, tzListKey+`\`+key, registry.QUERY_VALUE); err == nil {
			if d, err := lk.GetMUIStringValue("MUI_Display"); err == nil && d != "" {
				z.Display = d
			} else if d, _, err := lk.GetStringValue("Display"); err == nil {
				z.Display = d
			}
			lk.Close()
		}
	}
	return z, nil
}

// safeModeOn — Windows запущена в безопасном режиме (service.Config.SafeMode): ключ
// SafeBoot\Option создаётся только при таком запуске.
func safeModeOn() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, safeOptKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}
