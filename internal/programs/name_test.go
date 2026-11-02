package programs

import (
	"strings"
	"testing"
)

// Своё название программы (FB-11, D-59, FR-20).

// Ловит: своё название не главнее названия из файла; пустое или из пробелов своё не возвращает
// название из файла; нет названия из файла — не берётся имя из пути; своё не обрезается до
// MaxNameLen и не чистится от управляющих символов.
func TestDisplayName(t *testing.T) {
	path := `c:\program files\epic games\stalker2\binaries\win64\stalker2-win64-shipping.exe`
	cases := []struct{ custom, file, want string }{
		{"Сталкер 2", "Stalker2-Win64-Shipping", "Сталкер 2"},
		{"  Сталкер 2  ", "Stalker2-Win64-Shipping", "Сталкер 2"},
		{"", "Stalker2-Win64-Shipping", "Stalker2-Win64-Shipping"},
		{"   ", "Stalker2-Win64-Shipping", "Stalker2-Win64-Shipping"},
		{"", "", "stalker2-win64-shipping"},
		{"", "  ", "stalker2-win64-shipping"},
		{"Моя\tигра", "X", "Моя игра"},
		{strings.Repeat("я", 130), "X", strings.Repeat("я", MaxNameLen)},
	}
	for _, c := range cases {
		if got := DisplayName(c.custom, c.file, path); got != c.want {
			t.Errorf("DisplayName(%q, %q) = %q, want %q", c.custom, c.file, got, c.want)
		}
	}
}

// Ловит: проверка своего названия пропускает 61 символ, управляющие символы, считает байты
// вместо символов (кириллица: 60 букв — 120 байт), не обрезает пробелы по краям; пустое не
// принимается как сброс.
func TestCustomName(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"Сталкер 2", "Сталкер 2", true},
		{"  Моя игра  ", "Моя игра", true},
		{"", "", true},
		{"   ", "", true},
		{strings.Repeat("я", 60), strings.Repeat("я", 60), true},
		{" " + strings.Repeat("я", 60) + " ", strings.Repeat("я", 60), true},
		{strings.Repeat("я", 61), "", false},
		{"Моя\nигра", "", false},
		{"Моя\x00игра", "", false},
		{"Игра\u0007", "", false},
	}
	for _, c := range cases {
		got, ok := CustomName(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("CustomName(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
	if MaxCustomNameLen != 60 {
		t.Errorf("MaxCustomNameLen = %d, want 60 (D-59)", MaxCustomNameLen)
	}
}

// Ловит: классификатор отдаёт для плашек и журнала название из файла, а не своё (S40: «закрыта
// «Моя игра»»); своё название теряется у скрытой и у подменённой (RM3-12) программы.
func TestClassifyCustomName(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	game := `c:\program files\mygame\game.exe`
	hidden := `c:\program files\mygame\launcher.exe`
	sub := `c:\users\миша\downloads\supergame.exe`
	c.Reload([]Entry{
		{Path: game, Name: "Game Deluxe", CustomName: "Моя игра", Category: "games"},
		{Path: hidden, Name: "Launcher", CustomName: "Запускалка", Category: "games", Hidden: true},
		{Path: sub, Name: "SuperGame", CustomName: "Супер", Category: "games", Fingerprint: "1:1"},
		{Path: `c:\program files\other\x.exe`, Name: "X", CustomName: "  ", Category: "other"},
	})
	cases := []struct{ path, fp, want string }{
		{game, "", "Моя игра"},
		{hidden, "", "Запускалка"},
		{sub, "2:2", "Супер"},
		{`c:\program files\other\x.exe`, "", "X"},
	}
	for _, k := range cases {
		if got := c.Classify(k.path, k.fp, "").Name; got != k.want {
			t.Errorf("Classify(%s).Name = %q, want %q", k.path, got, k.want)
		}
	}
}
