//go:build !windows

package guard

import (
	"errors"

	"pcblock/internal/launch"
)

// Run вне Windows не работает: страж это окно Windows.
func Run(opts launch.Options) error {
	return errors.New("страж работает только в Windows")
}
