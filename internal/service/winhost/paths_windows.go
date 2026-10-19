//go:build windows

package winhost

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// defaultDataDir — папка данных программы: C:\ProgramData\PCBlock.
func defaultDataDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0); err == nil && p != "" {
		return filepath.Join(p, dataDirName)
	}
	if p := os.Getenv("ProgramData"); p != "" {
		return filepath.Join(p, dataDirName)
	}
	return filepath.Join(`C:\ProgramData`, dataDirName)
}

// ensureDataDir создаёт папку данных и закрывает её от всех, кроме SYSTEM и Администраторов
// (NFR-9). Повторный вызов безопасен: права выставляются заново.
func ensureDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("папка данных %s не создана: %w", dir, err)
	}
	sd, err := windows.SecurityDescriptorFromString(dataDirSDDL)
	if err != nil {
		return fmt.Errorf("права на папку данных: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("права на папку данных: %w", err)
	}
	err = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
	if err != nil {
		return fmt.Errorf("права на папку данных %s не выставлены: %w", dir, err)
	}
	return nil
}

// exePath — путь к запущенной программе (его службе нужно, чтобы запускать себя в сеансах).
func exePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("не найден путь к программе: %w", err)
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return exe, nil
}
