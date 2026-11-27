//go:build !windows

package sound

const isWindows = false

// PlayWarning на других ОС ничего не играет и сообщает, что звук не принят.
func PlayWarning() bool { return false }

// PlayMessage на других ОС ничего не играет и сообщает, что звук не принят.
func PlayMessage(n int) bool { return false }

// PlaySystem на других ОС ничего не играет и сообщает, что звук не принят.
func PlaySystem(alias string) bool { return false }
