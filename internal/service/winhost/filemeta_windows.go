//go:build windows

package winhost

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"

	"pcblock/internal/service"
)

// maxVersionInfo — ресурс версии длиннее не читается (настоящие — единицы килобайт).
const maxVersionInfo = 1 << 20

// fileMeta — service.FileMeta: издатель (CompanyName) и описание (FileDescription) из ресурса
// версии файла (RM3-3) и отпечаток «размер:время изменения unix» (RM3-12). Что не прочиталось —
// пустая строка; пустой отпечаток ничем не подменяется (служба тогда снова спрашивает родителя,
// T4a Д-1). Подпись Authenticode не проверяется (README «Отклонения от ТЗ»).
func fileMeta(path string) (publisher, description, fingerprint string) {
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		fingerprint = fingerprintOf(fi.Size(), fi.ModTime())
	}
	publisher, description = versionStrings(path)
	return publisher, description, fingerprint
}

var _ service.FileMeta = fileMeta

// versionStrings читает CompanyName и FileDescription из ресурса версии (version.dll): таблица по
// \VarFileInfo\Translation, затем запасные (translationKeys).
func versionStrings(path string) (company, descr string) {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 || size > maxVersionInfo {
		return "", ""
	}
	buf := make([]byte, size)
	block := unsafe.Pointer(&buf[0])
	if err := windows.GetFileVersionInfo(path, 0, size, block); err != nil {
		return "", ""
	}
	// within: указатель, который вернул VerQueryValue, со своей длиной лежит внутри buf.
	within := func(p unsafe.Pointer, bytes uint32) bool {
		off := uintptr(p) - uintptr(block)
		return p != nil && uintptr(p) >= uintptr(block) && off+uintptr(bytes) <= uintptr(len(buf))
	}
	var raw []byte
	var tp unsafe.Pointer
	var tl uint32
	if windows.VerQueryValue(block, `\VarFileInfo\Translation`, unsafe.Pointer(&tp), &tl) == nil && tl > 0 && within(tp, tl) {
		raw = unsafe.Slice((*byte)(tp), tl)
	}
	get := func(key, name string) string {
		var p *uint16
		var n uint32 // символов, с завершающим нулём
		if windows.VerQueryValue(block, `\StringFileInfo\`+key+`\`+name, unsafe.Pointer(&p), &n) != nil || n == 0 {
			return ""
		}
		if !within(unsafe.Pointer(p), n*2) {
			return ""
		}
		return windows.UTF16ToString(unsafe.Slice(p, n))
	}
	return pickVersionStrings(translationKeys(raw), get)
}
