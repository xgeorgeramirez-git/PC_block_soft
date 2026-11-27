//go:build windows

package sound

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const isWindows = true

// Флаги PlaySoundW.
const (
	sndAsync     = 0x0001     // не ждать конца звука: поток окон не блокируется
	sndNoDefault = 0x0002     // нет звука — тишина, а не системный «динь»
	sndMemory    = 0x0004     // первый параметр — wav в памяти
	sndSync      = 0x0000     // ждать конца звука (системный звук: длина неизвестна, R-7)
	sndAlias     = 0x00010000 // первый параметр — имя события звуковой схемы
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

// PlayMessage один раз играет встроенный звук сообщения n из памяти, не дожидаясь его конца (флаги
// как у PlayWarning). Нет такого звука или Windows его не принял — false. Буфер MessageWAV живёт
// всё время работы: SND_ASYNC читает его после возврата.
func PlayMessage(n int) bool {
	b := MessageWAV(n)
	if len(b) == 0 || procPlaySound.Find() != nil {
		return false
	}
	r, _, _ := procPlaySound.Call(uintptr(unsafe.Pointer(&b[0])), 0, sndMemory|sndAsync|sndNoDefault)
	return r != 0
}

// PlaySystem один раз играет звук события схемы Windows alias (AliasExclamation, AliasNotify) и
// ждёт его конца (SND_SYNC, R-7): длина звука схемы неизвестна, а исполнитель звуков стража зовёт
// его в своей горутине - синхронный вызов сам держит очередь до конца звука и не обрывает звук
// предупреждения или сообщения (PL-87). Звук выключен в схеме или Windows его не принял - false
// (SND_NODEFAULT: без системного «динь» взамен).
func PlaySystem(alias string) bool {
	if alias == "" || procPlaySound.Find() != nil {
		return false
	}
	p, err := windows.UTF16PtrFromString(alias)
	if err != nil {
		return false
	}
	r, _, _ := procPlaySound.Call(uintptr(unsafe.Pointer(p)), 0, sndAlias|sndSync|sndNoDefault)
	return r != 0
}
