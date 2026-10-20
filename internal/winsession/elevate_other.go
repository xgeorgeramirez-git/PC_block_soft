//go:build !windows

package winsession

import "errors"

// ErrElevationDeclined — человек отказался в окне UAC. Вне Windows окна UAC нет.
var ErrElevationDeclined = errors.New("Повышение прав отклонено: окно сброса пароля не открыто")

// IsElevatedAdmin вне Windows всегда false.
func IsElevatedAdmin() bool { return false }

// RelaunchElevated запускает программу с повышением прав. Вне Windows возвращает ошибку.
func RelaunchElevated(args []string) error { return errOnlyWindows }
