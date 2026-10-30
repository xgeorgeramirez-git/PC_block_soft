package winhost

import (
	"slices"
	"testing"
	"time"
)

// Ловит: кодовая страница таблицы строк берётся не из \VarFileInfo\Translation (тогда у файлов с
// русской или немецкой таблицей издатель пустой), порядок байтов перепутан, нет запасных
// английской и русской таблиц для файлов без списка переводов.
func TestTranslationKeys(t *testing.T) {
	// Два перевода: 0x0419/0x04B0 (русский, Юникод) и 0x0407/0x04E4 (немецкий, 1252).
	raw := []byte{0x19, 0x04, 0xb0, 0x04, 0x07, 0x04, 0xe4, 0x04}
	if got, want := translationKeys(raw), []string{"041904b0", "040704e4", "040904b0", "040904e4"}; !slices.Equal(got, want) {
		t.Errorf("translationKeys = %q; want %q", got, want)
	}
	if got, want := translationKeys(nil), []string{"040904b0", "041904b0", "040904e4"}; !slices.Equal(got, want) {
		t.Errorf("без переводов: %q; want %q", got, want)
	}
	if got, want := translationKeys([]byte{0x09, 0x04, 0xb0}), []string{"040904b0", "041904b0", "040904e4"}; !slices.Equal(got, want) {
		t.Errorf("обрезанный список переводов: %q; want %q", got, want)
	}
}

// Ловит: издатель берётся из одной таблицы, а описание из другой не ищется (или наоборот);
// пробелы по краям попадают в список программ.
func TestPickVersionStrings(t *testing.T) {
	data := map[string]string{
		"041904b0\x00CompanyName":     "",
		"041904b0\x00FileDescription": "  Игра  ",
		"040904b0\x00CompanyName":     " ООО Ромашка ",
		"040904b0\x00FileDescription": "Game",
	}
	get := func(key, name string) string { return data[key+"\x00"+name] }
	c, d := pickVersionStrings([]string{"041904b0", "040904b0"}, get)
	if c != "ООО Ромашка" || d != "Игра" {
		t.Errorf("pickVersionStrings = %q, %q; want «ООО Ромашка», «Игра»", c, d)
	}
	data["040904e4\x00CompanyName"] = "Чужая таблица"
	c, d = pickVersionStrings([]string{"040904b0", "040904e4"}, get)
	if c != "ООО Ромашка" || d != "Game" {
		t.Errorf("первая таблица: %q, %q; want «ООО Ромашка», «Game»", c, d)
	}
	data["040904e4\x00FileDescription"] = "Описание"
	data["040904b0\x00FileDescription"] = ""
	c, d = pickVersionStrings([]string{"040904b0", "040904e4"}, get)
	if c != "ООО Ромашка" || d != "Описание" {
		t.Errorf("издатель из первой таблицы, описание из второй: %q, %q; want «ООО Ромашка», «Описание»", c, d)
	}
	c, d = pickVersionStrings([]string{"000004b0"}, get)
	if c != "" || d != "" {
		t.Errorf("нет таблицы: %q, %q; want пустые", c, d)
	}
}

// Ловит: отпечаток не в формате «размер:время изменения unix», которого ждёт служба (RM3-12):
// тогда каждое решение родителя по программе вне доверенных каталогов «разошлось» бы.
func TestFingerprintOf(t *testing.T) {
	mod := time.Date(2023, 11, 14, 22, 13, 20, 999, time.UTC)
	if got := fingerprintOf(1024, mod); got != "1024:1700000000" {
		t.Errorf("fingerprintOf = %q; want 1024:1700000000", got)
	}
}
