// Package programs — классификация программ по пути exe (веха M3: R12, R13, RM3-1, RM3-12):
// нормализация пути, доверенные каталоги и корни приложений, класс «система», встроенный каталог
// игр и браузеров и Classifier. Только стандартная библиотека: файловую систему и Windows пакет
// не трогает (8.3-имена и junction разворачивает адаптер Windows до вызова).
package programs

import (
	"strings"
	"unicode"
)

// MaxNameLen — наибольшая длина имени программы в символах для плашек и журнала (R22).
const MaxNameLen = 120

// Normalize приводит путь exe к виду ключа таблицы programs (RM3-1): нижний регистр, «/» → «\»,
// без префикса «\\?\» («\\?\UNC\» → «\\»), без «.», «..», двойных и хвостового разделителей.
// «..» не поднимается выше диска или сетевой папки. Путь должен быть абсолютным: «c:\…» или
// «\\сервер\папка\…»; относительный, пустой, «c:файл» или с нулевым символом — «» (непригоден).
// Пробелы и точки в именах папок не срезаются: «program files.» — другая папка, не Program Files.
func Normalize(path string) string {
	if path == "" || strings.ContainsRune(path, 0) {
		return ""
	}
	p := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	switch {
	case strings.HasPrefix(p, `\\?\unc\`):
		p = `\\` + p[len(`\\?\unc\`):]
	case strings.HasPrefix(p, `\\?\`), strings.HasPrefix(p, `\\.\`), strings.HasPrefix(p, `\??\`):
		p = p[4:]
	}
	var head, rest string
	switch {
	case len(p) >= 3 && p[0] >= 'a' && p[0] <= 'z' && p[1] == ':' && p[2] == '\\':
		head, rest = p[:2], p[3:]
	case strings.HasPrefix(p, `\\`):
		parts := strings.SplitN(p[2:], `\`, 3)
		if len(parts) < 2 || !plainSegment(parts[0]) || !plainSegment(parts[1]) {
			return ""
		}
		head = `\\` + parts[0] + `\` + parts[1]
		if len(parts) == 3 {
			rest = parts[2]
		}
	default:
		return ""
	}
	segs := make([]string, 0, 8)
	for _, s := range strings.Split(rest, `\`) {
		switch s {
		case "", ".":
			continue
		case "..":
			if len(segs) > 0 {
				segs = segs[:len(segs)-1]
			}
			continue
		}
		segs = append(segs, s)
	}
	if len(segs) == 0 {
		if strings.HasPrefix(head, `\\`) {
			return head
		}
		return head + `\`
	}
	return head + `\` + strings.Join(segs, `\`)
}

// plainSegment: имя сервера или сетевой папки — непустое, не «.» и не «..».
func plainSegment(s string) bool { return s != "" && s != "." && s != ".." }

// Roots — корни, от которых зависит класс программы (RM3-1). Значения уже нормализованы
// (Normalize); пустые не участвуют. В службе их даёт адаптер Windows (KnownFolders и каталог
// установки из os.Executable).
type Roots struct {
	SystemRoot      string // %SystemRoot%: «c:\windows»
	ProgramFiles    string // %ProgramFiles%: «c:\program files»
	ProgramFilesX86 string // %ProgramFiles(x86)%: «c:\program files (x86)»
	InstallDir      string // каталог установки PC Block: служба, страж, значок, окна
	ProgramData     string // %ProgramData%: «c:\programdata» (компоненты Защитника Windows, RM3-13)
	UserProfiles    string // папка профилей (KnownFolder UserProfiles): «c:\users» (RM3-13)
}

// under сообщает, что нормализованный путь p лежит внутри root (строго глубже, по границе
// разделителя): «c:\program files games\x.exe» не внутри «c:\program files».
func under(p, root string) bool {
	if root == "" || p == "" {
		return false
	}
	pre := root
	if !strings.HasSuffix(pre, `\`) {
		pre += `\`
	}
	return len(p) > len(pre) && strings.HasPrefix(p, pre)
}

// IsTrusted сообщает, что путь лежит в доверенном каталоге (RM3-1): Program Files, Program Files
// (x86) или папка Windows. Ребёнок туда писать не может, поэтому каталог по имени файла и
// снимок без отпечатка действуют только здесь.
func (r Roots) IsTrusted(path string) bool {
	p := Normalize(path)
	return under(p, r.SystemRoot) || under(p, r.ProgramFiles) || under(p, r.ProgramFilesX86)
}

// IsSystem сообщает, что программа из класса «система» (RM3-1 п.1): папка Windows (в том числе
// SystemApps оболочки), каталог установки PC Block, WebView2 внутри Program Files и пакеты
// компонентов Windows в WindowsApps (издатель windowsPackagePublisher). Остальные приложения
// Microsoft Store — не система (Д-3): ребёнок ставит их сам. Такие процессы наблюдатель не
// трогает никогда, их секунды идут как «Прочее».
func (r Roots) IsSystem(path string) bool {
	p := Normalize(path)
	if under(p, r.SystemRoot) || under(p, r.InstallDir) {
		return true
	}
	for _, pf := range []string{r.ProgramFiles, r.ProgramFilesX86} {
		if pf == "" {
			continue
		}
		if under(p, pf+`\microsoft\edgewebview`) {
			return true
		}
	}
	if pkg, ok := r.storePackage(p); ok && pkg.publisher == windowsPackagePublisher {
		return true
	}
	if r.ProgramData != "" {
		pd := strings.TrimSuffix(r.ProgramData, `\`)
		for _, d := range defenderDirs {
			if under(p, pd+`\`+d) {
				return true
			}
		}
	}
	return false
}

// defenderDirs — папки Защитника Windows внутри %ProgramData% (RM3-13): его платформа обновляется
// в папки версий, ребёнок туда писать не может.
var defenderDirs = []string{`microsoft\windows defender`, `microsoft\windows defender advanced threat protection`}

// windowsPackagePublisher — код издателя пакетов компонентов Windows (MicrosoftWindows.*): их
// ставит и обновляет сама Windows, ребёнок такой пакет поставить не может.
const windowsPackagePublisher = "cw5n1h2txyewy"

// storeDir — папка пакетов Microsoft Store внутри %ProgramFiles%.
const storeDir = `windowsapps`

// storePkg — пакет Microsoft Store по имени папки «имя_версия_архитектура_ресурс_издатель».
type storePkg struct {
	dir       string // имя папки пакета как есть
	name      string // имя пакета без версии («microsoft.minecraftuwp»); «» — папка не по образцу
	publisher string // код издателя («8wekyb3d8bbwe»)
}

// family — семейство пакета «имя__издатель»: не зависит от версии и архитектуры (Д-3). Папка
// не по образцу — её имя как есть.
func (k storePkg) family() string {
	if k.name == "" {
		return k.dir
	}
	return k.name + "__" + k.publisher
}

// storePackage разбирает нормализованный путь внутри <ProgramFiles>\WindowsApps\<пакет>\…;
// ok = false — путь не в пакете Store (или прямо в WindowsApps).
func (r Roots) storePackage(p string) (storePkg, bool) {
	if r.ProgramFiles == "" {
		return storePkg{}, false
	}
	base := strings.TrimSuffix(r.ProgramFiles, `\`) + `\` + storeDir
	if !under(p, base) {
		return storePkg{}, false
	}
	parts := strings.Split(p[len(base)+1:], `\`)
	if len(parts) < 2 {
		return storePkg{}, false
	}
	k := storePkg{dir: parts[0]}
	if f := strings.Split(parts[0], "_"); len(f) == 5 && f[0] != "" && f[4] != "" {
		k.name, k.publisher = f[0], f[4]
	}
	return k, true
}

// containerDirs — папки Program Files, в которых лежат разные приложения одного издателя или
// общие компоненты: корнем приложения у них считается следующая папка («…\microsoft\edge»),
// иначе решение по одной программе Microsoft распространилось бы на все.
var containerDirs = map[string]bool{"microsoft": true, "common files": true}

// AppRoot возвращает корень приложения (RM3-1 п.3): первую папку внутри Program Files или Program
// Files (x86) («c:\program files\brave software»; в общих папках Microsoft и Common Files —
// вторую; в WindowsApps — семейство пакета без версии «…\windowsapps\имя__издатель», Д-3). Файл
// прямо в Program Files, сама папка и путь вне них — «».
func (r Roots) AppRoot(path string) string {
	p := Normalize(path)
	if r.ProgramFiles != "" {
		store := strings.TrimSuffix(r.ProgramFiles, `\`) + `\` + storeDir
		if pkg, ok := r.storePackage(p); ok {
			return store + `\` + pkg.family()
		}
		if under(p, store) {
			return "" // файл прямо в WindowsApps: не приложение
		}
	}
	for _, pf := range []string{r.ProgramFiles, r.ProgramFilesX86} {
		if !under(p, pf) {
			continue
		}
		base := strings.TrimSuffix(pf, `\`)
		parts := strings.Split(p[len(base)+1:], `\`)
		if containerDirs[parts[0]] {
			if len(parts) < 3 {
				return ""
			}
			return base + `\` + parts[0] + `\` + parts[1]
		}
		if len(parts) < 2 {
			return ""
		}
		return base + `\` + parts[0]
	}
	return r.ProfileRoot(p)
}

// ProfileRoot возвращает корень приложения в профиле пользователя (RM3-13) по форме пути
// «<UserProfiles>\<имя>\appdata\local|roaming\…»: первая папка под Local и Roaming, вторая под
// Local\Programs и под Microsoft («…\local\microsoft\onedrive»). Local\Packages (данные Store),
// Local\Temp, LocalLow, файл прямо в этих папках и путь вне AppData — «».
func (r Roots) ProfileRoot(path string) string {
	rel := r.profileRel(Normalize(path))
	if rel == "" {
		return ""
	}
	return strings.TrimSuffix(r.UserProfiles, `\`) + `\` + rel
}

// profileRel — корень приложения в профиле относительно папки профилей, с именем профиля
// («миша\appdata\local\discord»); «» — не в корне приложения профиля. p нормализован.
func (r Roots) profileRel(p string) string {
	if !under(p, r.UserProfiles) {
		return ""
	}
	base := strings.TrimSuffix(r.UserProfiles, `\`)
	parts := strings.Split(p[len(base)+1:], `\`) // имя, appdata, local|roaming, папка, …, файл
	if len(parts) < 5 || parts[1] != "appdata" || (parts[2] != "local" && parts[2] != "roaming") {
		return ""
	}
	n := 4 // частей пути в корне
	switch first := parts[3]; {
	case parts[2] == "local" && (first == "packages" || first == "temp"):
		return ""
	case (parts[2] == "local" && first == "programs") || first == "microsoft":
		n = 5
	}
	if len(parts) <= n {
		return ""
	}
	return strings.Join(parts[:n], `\`)
}

// NameFromPath — имя программы для плашки и журнала из пути: имя файла без «.exe», обрезанное до
// MaxNameLen символов (регистр как в пути). Имя берётся из пути, не из слов стража.
func NameFromPath(path string) string {
	p := strings.ReplaceAll(path, "/", `\`)
	base := p[strings.LastIndex(p, `\`)+1:]
	if len(base) > 4 && strings.EqualFold(base[len(base)-4:], ".exe") {
		base = base[:len(base)-4]
	}
	return ClipName(base)
}

// ClipName готовит имя программы к показу: управляющие символы — пробелы, края без пробелов,
// не длиннее MaxNameLen символов.
func ClipName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > MaxNameLen {
		s = strings.TrimSpace(string(r[:MaxNameLen]))
	}
	return s
}

// MaxCustomNameLen — наибольшая длина своего названия программы в символах (D-59, FR-20).
const MaxCustomNameLen = 60

// DisplayName — единственное место, которое решает, как программа называется для людей (D-59):
// своё название родителя, иначе название из файла, иначе имя файла из пути. Его используют все
// показы: список программ, журнал, плашки ребёнку, статистика, классификатор. Не длиннее MaxNameLen.
func DisplayName(custom, file, path string) string {
	if n := ClipName(custom); n != "" {
		return n
	}
	if n := ClipName(file); n != "" {
		return n
	}
	return NameFromPath(path)
}

// CustomName проверяет своё название, которое ввёл родитель (D-59): пробелы по краям обрезаются,
// пустое — сброс к названию из файла (ok = true, ""); длиннее MaxCustomNameLen символов или с
// управляющими символами — ok = false.
func CustomName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	if len([]rune(s)) > MaxCustomNameLen {
		return "", false
	}
	return s, true
}
