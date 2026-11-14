package startup

import "strings"

// ExeOf — путь программы, которую запускает команда записи автозагрузки (RM5b-3, RM5b-7):
// переменные %VAR% раскрываются через env (нет переменной или env == nil — ""), путь в кавычках
// берётся до закрывающей, без кавычек — до первого «.exe» перед пробелом или концом (путь
// с пробелами, как его понимает Проводник), иначе до первого пробела. Обёртки разворачиваются:
// rundll32 — библиотека (до запятой), cmd /c и /k — команда после ключа, в том числе после
// start с заголовком. Не разобрать — "".
func ExeOf(command string, env func(string) string) string {
	s, ok := expandEnv(strings.TrimSpace(command), env)
	if !ok {
		return ""
	}
	return exeOf(s, 0)
}

// expandEnv раскрывает %VAR%; незакрытый знак процента или неизвестная переменная — false.
func expandEnv(s string, env func(string) string) (string, bool) {
	if !strings.Contains(s, "%") {
		return s, true
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			b.WriteString(s)
			return b.String(), true
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j <= 0 || env == nil {
			return "", false
		}
		v := env(s[i+1 : i+1+j])
		if v == "" {
			return "", false
		}
		b.WriteString(s[:i])
		b.WriteString(v)
		s = s[i+1+j+1:]
	}
}

// maxWrap — предел вложенных обёрток (cmd /c cmd /c …): глубже — не разбираем.
const maxWrap = 4

func exeOf(s string, depth int) string {
	s = strings.TrimSpace(s)
	if s == "" || depth > maxWrap {
		return ""
	}
	exe, rest, ok := firstPath(s)
	if !ok {
		return ""
	}
	switch baseName(exe) {
	case "rundll32", "rundll32.exe":
		return dllOf(rest)
	case "cmd", "cmd.exe":
		inner, ok := afterCmdSwitch(rest)
		if !ok {
			return exe // cmd без /c и /k запускает сам себя
		}
		return exeOf(skipStart(inner), depth+1)
	}
	return exe
}

// firstPath отделяет путь программы от аргументов.
func firstPath(s string) (exe, rest string, ok bool) {
	if s[0] == '"' {
		end := strings.IndexByte(s[1:], '"')
		if end < 0 {
			return "", "", false
		}
		return s[1 : 1+end], s[2+end:], true
	}
	// Первое слово — уже программа (есть расширение или это обёртка): «C:\Tools\a.exe -x»,
	// «cmd /c …». Иначе путь с пробелами без кавычек — до первого «.exe».
	if tok, rest := cutSpace(s); strings.Contains(baseName(tok), ".") || wrapper(baseName(tok)) {
		return tok, rest, true
	}
	for i := 0; i+4 <= len(s); i++ {
		if strings.EqualFold(s[i:i+4], ".exe") && (i+4 == len(s) || s[i+4] == ' ' || s[i+4] == '\t') {
			return s[:i+4], s[i+4:], true
		}
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], s[i:], true
	}
	return s, "", true
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		p = p[i+1:]
	}
	return strings.ToLower(p)
}

// dllOf — библиотека из аргументов rundll32: «путь,Точка» или «"путь",Точка».
func dllOf(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	if args[0] == '"' {
		end := strings.IndexByte(args[1:], '"')
		if end < 0 {
			return ""
		}
		return args[1 : 1+end]
	}
	if i := strings.IndexAny(args, ", \t"); i >= 0 {
		return args[:i]
	}
	return args
}

// afterCmdSwitch — текст после ключа /c или /k у cmd (прочие ключи пропускаются).
func afterCmdSwitch(args string) (string, bool) {
	for {
		args = strings.TrimSpace(args)
		if !strings.HasPrefix(args, "/") {
			return "", false
		}
		tok, rest := cutSpace(args)
		if strings.EqualFold(tok, "/c") || strings.EqualFold(tok, "/k") {
			return rest, true
		}
		args = rest
	}
}

// skipStart снимает «start [/ключи] ["заголовок"]» перед командой: у start первый аргумент
// в кавычках — всегда заголовок окна.
func skipStart(s string) string {
	s = strings.TrimSpace(s)
	tok, rest := cutSpace(s)
	if !strings.EqualFold(tok, "start") {
		return s
	}
	for {
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "/") {
			_, rest = cutSpace(rest)
			continue
		}
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return ""
			}
			return rest[2+end:]
		}
		return rest
	}
}

func cutSpace(s string) (string, string) {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

func wrapper(base string) bool {
	switch base {
	case "rundll32", "rundll32.exe", "cmd", "cmd.exe":
		return true
	}
	return false
}
