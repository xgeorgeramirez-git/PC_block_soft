package winhost

import "strings"

// Пути процессов для наблюдателя (M3: RM3-1, Review Focus 3): разбор строк без вызовов Windows,
// чтобы проверять его тестами на любой системе. Сами вызовы — в procinfo_windows.go.

// maxPathChars — MAX_PATH: длиннее без префикса «\\?\» функции файлов Windows путь не принимают.
const maxPathChars = 260

// extendedPath добавляет к длинному (не короче MAX_PATH) абсолютному пути префикс «\\?\» (сетевой
// путь — «\\?\UNC\»), чтобы GetLongPathName и CreateFile его приняли. Короткий, относительный и
// уже расширенный путь, а также путь устройства («\\.\») возвращаются как есть.
func extendedPath(p string) string {
	if len(p) < maxPathChars || strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\\.\`) {
		return p
	}
	if strings.HasPrefix(p, `\\`) {
		return `\\?\UNC\` + p[2:]
	}
	if len(p) >= 3 && p[1] == ':' && p[2] == '\\' {
		return `\\?\` + p
	}
	return p
}

// stripExtended снимает префикс «\\?\» (и «\\?\UNC\» → «\\»), который возвращают
// GetFinalPathNameByHandle и GetLongPathName для расширенного пути. Путь тома без буквы диска
// («\\?\Volume{…}\…») остаётся как есть: без префикса он стал бы относительным, а programs.Normalize
// такой путь отвергнет (нечитаемый путь — «не трогать»).
func stripExtended(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`):
		rest := p[4:]
		if len(rest) >= 3 && rest[1] == ':' && rest[2] == '\\' {
			return rest
		}
	}
	return p
}

// hasShortName сообщает, что в пути есть похожее на 8.3 имя («RUNNER~1», «PROGRA~1»): тильда и
// за ней цифра. Если такой путь развернуть не удалось, он непригоден для классификации:
// «c:\progra~1\…» не внутри Program Files.
func hasShortName(p string) bool {
	for i := 0; i+1 < len(p); i++ {
		if p[i] == '~' && p[i+1] >= '0' && p[i+1] <= '9' {
			return true
		}
	}
	return false
}

// isUNC сообщает, что путь сетевой («\\сервер\папка\…» или «\\?\UNC\…»): по нему наблюдатель не
// ходит в сеть (мёртвая сетевая папка держала бы опрос десятки секунд).
func isUNC(p string) bool {
	p = strings.ReplaceAll(p, "/", `\`)
	if strings.HasPrefix(p, `\\?\UNC\`) {
		return true
	}
	return strings.HasPrefix(p, `\\`) && !strings.HasPrefix(p, `\\?\`) && !strings.HasPrefix(p, `\\.\`)
}
