package programs

import "strings"

// Системные инструменты (веха M6, R17г, FR-29, D-86): командная строка, PowerShell, Терминал,
// редактор реестра, консоль управления, taskkill, sc. Запущенный ребёнком закрывается и пишется
// обходом; запущенный другой программой не трогается (решает служба, watch.go). Инструмент
// узнаётся по имени файла и папке: копия под другим именем или в папке ребёнка — обычная
// программа (незнакомая ждёт решения родителя, R12).

// toolNames — имя файла → название для ребёнка и родителя; папку проверяет SystemTool.
var toolNames = map[string]string{
	"cmd.exe":             "Командная строка",
	"powershell.exe":      "Windows PowerShell",
	"powershell_ise.exe":  "Windows PowerShell",
	"pwsh.exe":            "PowerShell",
	"regedit.exe":         "Редактор реестра",
	"regedt32.exe":        "Редактор реестра",
	"mmc.exe":             "Консоль управления",
	"taskkill.exe":        "taskkill",
	"sc.exe":              "sc",
	"windowsterminal.exe": "Терминал",
	"wt.exe":              "Терминал",
}

// IsTerminal — инструмент path — Терминал Windows (у него своё правило «ручного» запуска в службе:
// Windows 11 открывает его сама для любой консоли).
func IsTerminal(path string) bool {
	return toolNames[baseName(Normalize(path))] == "Терминал"
}

// SystemTool — путь path (любой, нормализуется здесь) — системный инструмент; name — его название.
// Папки: cmd, Windows PowerShell, редактор реестра, mmc, taskkill, sc — внутри папки Windows;
// pwsh — внутри Program Files\PowerShell или пакета Магазина Microsoft.PowerShell; Терминал — внутри
// пакета Магазина Microsoft.WindowsTerminal (и WindowsTerminalPreview). testDir (тестовый режим,
// RM3-8; «» — нет): файл с именем инструмента прямо в этой папке тоже инструмент.
func (r Roots) SystemTool(path, testDir string) (string, bool) {
	p := Normalize(path)
	base := baseName(p)
	name, ok := toolNames[base]
	if !ok {
		return "", false
	}
	if td := strings.TrimSuffix(Normalize(testDir), `\`); td != "" && p == td+`\`+base {
		return name, true
	}
	switch base {
	case "pwsh.exe":
		for _, pf := range []string{r.ProgramFiles, r.ProgramFilesX86} {
			if pf != "" && under(p, strings.TrimSuffix(pf, `\`)+`\powershell`) {
				return name, true
			}
		}
		if pkg, ok := r.storePackage(p); ok && strings.HasPrefix(pkg.dir, "microsoft.powershell") {
			return name, true
		}
		return "", false
	case "windowsterminal.exe", "wt.exe":
		if pkg, ok := r.storePackage(p); ok && strings.HasPrefix(pkg.dir, "microsoft.windowsterminal") {
			return name, true
		}
		return "", false
	}
	if under(p, r.SystemRoot) {
		return name, true
	}
	return "", false
}
