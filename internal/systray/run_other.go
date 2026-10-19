//go:build !windows

package systray

import (
	"errors"

	"pcblock/internal/launch"
)

// Run на других системах не работает: значок у часов есть только в Windows.
func Run(launch.Options) error {
	return errors.New("значок работает только в Windows")
}
