//go:build windows

package guard

import (
	"fmt"
	"unsafe"
)

// Закрытие активной вкладки Brave сочетанием Ctrl+W (RM4-2). Нажатия уходят окну переднего плана;
// вызывающий перед этим перечитывает передний план и домен (то же окно, тот же домен).

var (
	procSendInput        = user32.NewProc("SendInput")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

const (
	vkShift        = 0x10
	vkControl      = 0x11
	vkMenu         = 0x12 // Alt; vkLWin и vkRWin — в lockwindow_windows.go
	vkW            = 0x57
	inputKeyboard  = 1
	keyeventfKeyUp = 0x0002
)

// keybdInput — INPUT с KEYBDINPUT для x64: тип (4 байта и 4 выравнивания), затем объединение
// размером с MOUSEINPUT (32 байта); всего 40 байт.
type keybdInput struct {
	typ       uint32
	_         uint32
	wVk       uint16
	wScan     uint16
	dwFlags   uint32
	time      uint32
	_         uint32
	extraInfo uintptr
	_         [8]byte
}

func keyHeld(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

// closeActiveTab нажимает Ctrl+W в окне переднего плана. Зажат Shift, Alt или Win — errModifierHeld
// (tabs.go) без нажатий.
func closeActiveTab() error {
	for _, vk := range []uintptr{vkShift, vkMenu, vkLWin, vkRWin} {
		if keyHeld(vk) {
			return errModifierHeld
		}
	}
	in := []keybdInput{
		{typ: inputKeyboard, wVk: vkControl},
		{typ: inputKeyboard, wVk: vkW},
		{typ: inputKeyboard, wVk: vkW, dwFlags: keyeventfKeyUp},
		{typ: inputKeyboard, wVk: vkControl, dwFlags: keyeventfKeyUp},
	}
	n, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(n) != len(in) {
		return fmt.Errorf("SendInput: отправлено %d из %d: %v", n, len(in), err)
	}
	return nil
}
