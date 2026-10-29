//go:build windows

package sound

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const isWindows = true

// Флаги PlaySoundW.
const (
	sndAsync     = 0x0001 // не ждать конца звука: поток окон не блокируется
	sndNoDefault = 0x0002 // нет звука — тишина, а не системный «динь»
	sndMemory    = 0x0004 // первый параметр — wav в памяти
)

var procPlaySound = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")

// PlayWarning один раз играет звук предупреждения из памяти, не дожидаясь его конца. Возвращает,
// принял ли Windows звук (результат PlaySoundW): без звуковой карты (раннер CI) — false, но
// показ плашки от этого не зависит.
func PlayWarning() bool {
	if len(warningWAV) == 0 || procPlaySound.Find() != nil {
		return false
	}
	r, _, _ := procPlaySound.Call(uintptr(unsafe.Pointer(&warningWAV[0])), 0, sndMemory|sndAsync|sndNoDefault)
	return r != 0
}
