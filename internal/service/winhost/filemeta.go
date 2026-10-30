package winhost

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// Свойства файла exe (M3: RM3-3, RM3-12): разбор без вызовов Windows. Чтение version.dll —
// в filemeta_windows.go.

// fallbackVersionKeys — таблицы строк, которые пробуются после списка переводов файла:
// английская и русская Юникод, английская 1252.
var fallbackVersionKeys = []string{"040904b0", "041904b0", "040904e4"}

// translationKeys переводит значение \VarFileInfo\Translation (пары «язык, кодовая страница» по
// 2 байта, младший байт первым) в имена таблиц \StringFileInfo («041904b0») и добавляет запасные.
// Повторы убираются, порядок сохраняется: сначала переводы файла.
func translationKeys(raw []byte) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for i := 0; i+4 <= len(raw); i += 4 {
		lang := binary.LittleEndian.Uint16(raw[i:])
		cp := binary.LittleEndian.Uint16(raw[i+2:])
		add(fmt.Sprintf("%04x%04x", lang, cp))
	}
	for _, k := range fallbackVersionKeys {
		add(k)
	}
	return out
}

// pickVersionStrings берёт CompanyName и FileDescription из первой таблицы, где значение не
// пустое (каждое отдельно): у части файлов издатель есть только в английской таблице, а описание
// в русской. get возвращает строку таблицы key или "".
func pickVersionStrings(keys []string, get func(key, name string) string) (company, descr string) {
	for _, k := range keys {
		if company == "" {
			company = strings.TrimSpace(get(k, "CompanyName"))
		}
		if descr == "" {
			descr = strings.TrimSpace(get(k, "FileDescription"))
		}
		if company != "" && descr != "" {
			break
		}
	}
	return company, descr
}

// fingerprintOf — отпечаток файла для решения родителя (RM3-12): «размер:время изменения unix».
func fingerprintOf(size int64, mod time.Time) string {
	return fmt.Sprintf("%d:%d", size, mod.Unix())
}
