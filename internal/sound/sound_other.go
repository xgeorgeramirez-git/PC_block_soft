//go:build !windows

package sound

const isWindows = false

// PlayWarning на других ОС ничего не играет и сообщает, что звук не принят.
func PlayWarning() bool { return false }
