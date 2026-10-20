//go:build !windows

package ui

import (
	"errors"

	"pcblock/internal/ipc"
)

// dialService вне Windows канала связи не имеет: окно Wails собирается только под Windows,
// заглушка нужна, чтобы go vet на других системах не падал.
func dialService() (ipc.Caller, error) {
	return nil, errors.New("канал связи со службой есть только в Windows")
}
