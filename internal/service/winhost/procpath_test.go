package winhost

import (
	"strings"
	"testing"
)

// Ловит: длинный путь (больше MAX_PATH) уходит в GetLongPathName и CreateFile без префикса «\\?\»
// и не разворачивается; сетевой путь получает префикс диска вместо «\\?\UNC\»; уже расширенный
// путь получает второй префикс.
func TestExtendedPath(t *testing.T) {
	long := `C:\` + strings.Repeat(`папка\`, 50) + `игра.exe`
	cases := map[string]string{
		`C:\Windows\notepad.exe`:     `C:\Windows\notepad.exe`,
		long:                         `\\?\` + long,
		`\\srv\share\` + long[3:]:    `\\?\UNC\srv\share\` + long[3:],
		`\\?\` + long:                `\\?\` + long,
		`\\?\UNC\srv\share\a.exe`:    `\\?\UNC\srv\share\a.exe`,
		``:                           ``,
		`relative\` + long[3:]:       `relative\` + long[3:],
		`\\.\pipe\` + long[3:]:       `\\.\pipe\` + long[3:],
		`C:\Program Files\Brave.exe`: `C:\Program Files\Brave.exe`,
	}
	for in, want := range cases {
		if got := extendedPath(in); got != want {
			t.Errorf("extendedPath(%.40q…) = %.60q…; want %.60q…", in, got, want)
		}
	}
}

// Ловит: GetFinalPathNameByHandle возвращает «\\?\C:\…» и «\\?\UNC\srv\…»: без снятия префикса
// путь не совпал бы с ключом таблицы программ; путь тома без буквы («\\?\Volume{…}») снимать
// нельзя: получился бы относительный путь, похожий на настоящий.
func TestStripExtended(t *testing.T) {
	cases := map[string]string{
		`\\?\C:\Program Files\Brave\brave.exe`: `C:\Program Files\Brave\brave.exe`,
		`\\?\UNC\srv\share\game.exe`:           `\\srv\share\game.exe`,
		`\\?\Volume{1234}\game.exe`:            `\\?\Volume{1234}\game.exe`,
		`C:\Windows\explorer.exe`:              `C:\Windows\explorer.exe`,
		`\\?\c:\x.exe`:                         `c:\x.exe`,
		`\\?\`:                                 `\\?\`,
		``:                                     ``,
	}
	for in, want := range cases {
		if got := stripExtended(in); got != want {
			t.Errorf("stripExtended(%q) = %q; want %q", in, got, want)
		}
	}
}

// Ловит: 8.3-имя (RUNNER~1, PROGRA~1) принимается за настоящий путь, когда развернуть его не
// удалось: «c:\progra~1\…» не внутри Program Files, и наблюдатель закрыл бы программу оттуда
// (RM3-1, Review Focus 3). Тильда без цифры после неё — обычное имя.
func TestHasShortName(t *testing.T) {
	yes := []string{
		`C:\Users\RUNNER~1\AppData\Local\Temp\x.exe`,
		`C:\PROGRA~1\Brave\brave.exe`,
		`C:\Games\GAME~12.EXE`,
		`c:\a\b~1`,
	}
	no := []string{
		`C:\Program Files\Brave\brave.exe`,
		`C:\Games\~tmp\x.exe`,
		`C:\Games\a~b\x.exe`,
		`C:\Games\x~.exe`,
		``,
	}
	for _, p := range yes {
		if !hasShortName(p) {
			t.Errorf("hasShortName(%q) = false; want true", p)
		}
	}
	for _, p := range no {
		if hasShortName(p) {
			t.Errorf("hasShortName(%q) = true; want false", p)
		}
	}
}

// Ловит: сетевой путь не распознан, и наблюдатель на каждом новом процессе ходит по сети
// (GetFinalPathNameByHandle по мёртвой сетевой папке висит десятки секунд, а опрос раз в 250 мс).
func TestIsUNC(t *testing.T) {
	cases := map[string]bool{
		`\\srv\share\game.exe`:       true,
		`\\?\UNC\srv\share\game.exe`: true,
		`\\?\C:\game.exe`:            false,
		`\\.\pipe\x`:                 false,
		`C:\game.exe`:                false,
		`//srv/share/game.exe`:       true,
		``:                           false,
	}
	for in, want := range cases {
		if got := isUNC(in); got != want {
			t.Errorf("isUNC(%q) = %v; want %v", in, got, want)
		}
	}
}
