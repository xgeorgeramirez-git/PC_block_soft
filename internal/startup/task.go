package startup

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

// Task — то, что из задания Планировщика нужно автозагрузке (RM5b-1, уточнение 7 плана).
type Task struct {
	Path    string // RegistrationInfo/URI, например \OneDrive Reporting Task-S-1-5-21-…
	Command string // первое действие Exec: путь (в кавычках, если с пробелом) и аргументы; нет Exec — ""
	Logon   bool   // есть включённый триггер «при входе»
	UserID  string // пользователь триггера «при входе» (SID или ДОМЕН\имя); "" — вход любого пользователя
	Enabled bool   // Settings/Enabled (нет — включено)
	// RunsAsUser — задание запускается от учётки пользователя, а не службы: главный (Principal
	// действий) — пользователь не из NT AUTHORITY, группа Users или InteractiveToken без учётки.
	RunsAsUser bool
	RunAs      string // UserId главного; "" — не задан
	Group      string // GroupId главного; "" — не задан
}

type taskDoc struct {
	XMLName  xml.Name `xml:"Task"`
	URI      string   `xml:"RegistrationInfo>URI"`
	Triggers []struct {
		Enabled *string `xml:"Enabled"`
		UserID  string  `xml:"UserId"`
	} `xml:"Triggers>LogonTrigger"`
	Principals []struct {
		ID        string `xml:"id,attr"`
		UserID    string `xml:"UserId"`
		GroupID   string `xml:"GroupId"`
		LogonType string `xml:"LogonType"`
	} `xml:"Principals>Principal"`
	Enabled *string `xml:"Settings>Enabled"`
	Actions struct {
		Context string `xml:"Context,attr"`
		Exec    []struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Exec"`
	} `xml:"Actions"`
}

// ParseTaskXML разбирает файл задания из %WINDIR%\System32\Tasks: UTF-16 (LE или BE, с BOM
// или без) либо UTF-8. Объявление encoding="UTF-16" после перевода в UTF-8 принимается
// (CharsetReader), другие кодировки, кроме UTF-8, — ошибка; корень не <Task> — ошибка.
func ParseTaskXML(data []byte) (Task, error) {
	text, err := taskText(data)
	if err != nil {
		return Task{}, err
	}
	d := xml.NewDecoder(strings.NewReader(text))
	d.CharsetReader = func(label string, r io.Reader) (io.Reader, error) {
		switch strings.ToLower(label) {
		case "utf-16", "utf-16le", "utf-16be", "unicode":
			return r, nil // текст уже переведён в UTF-8
		}
		return nil, fmt.Errorf("startup: кодировка задания %q не поддерживается", label)
	}
	var doc taskDoc
	if err := d.Decode(&doc); err != nil {
		return Task{}, fmt.Errorf("startup: задание: %w", err)
	}
	t := Task{Path: strings.TrimSpace(doc.URI), Enabled: !xmlFalse(doc.Enabled)}
	anyUser := false
	for _, tr := range doc.Triggers {
		if xmlFalse(tr.Enabled) {
			continue
		}
		u := strings.TrimSpace(tr.UserID)
		if !t.Logon {
			t.UserID = u
		}
		t.Logon = true
		anyUser = anyUser || u == ""
	}
	if anyUser {
		t.UserID = ""
	}
	if len(doc.Actions.Exec) > 0 {
		t.Command = execCommand(doc.Actions.Exec[0].Command, doc.Actions.Exec[0].Arguments)
	}
	if len(doc.Principals) > 0 {
		p := doc.Principals[0]
		for _, q := range doc.Principals {
			if doc.Actions.Context != "" && q.ID == doc.Actions.Context {
				p = q
				break
			}
		}
		t.RunAs = strings.TrimSpace(p.UserID)
		t.Group = strings.TrimSpace(p.GroupID)
		switch {
		case t.RunAs != "":
			t.RunsAsUser = !serviceAccount(t.RunAs)
		case t.Group != "":
			t.RunsAsUser = usersGroup(t.Group)
		default:
			t.RunsAsUser = strings.EqualFold(strings.TrimSpace(p.LogonType), "InteractiveToken")
		}
	}
	return t, nil
}

// taskText переводит байты файла задания в строку UTF-8.
func taskText(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		return fromUTF16(data[2:], false)
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		return fromUTF16(data[2:], true)
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return string(data[3:]), nil
	case len(data) >= 2 && data[0] != 0 && data[1] == 0:
		return fromUTF16(data, false)
	case len(data) >= 2 && data[0] == 0 && data[1] != 0:
		return fromUTF16(data, true)
	}
	return string(data), nil
}

func fromUTF16(b []byte, bigEndian bool) (string, error) {
	if len(b)%2 != 0 {
		return "", errors.New("startup: задание в UTF-16 нечётной длины")
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		if bigEndian {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			u[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	return string(utf16.Decode(u)), nil
}

// xmlFalse — элемент xs:boolean есть и равен false (0). Нет элемента или другое значение —
// true по умолчанию схемы Планировщика (Enabled).
func xmlFalse(v *string) bool {
	if v == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(*v)) {
	case "false", "0":
		return true
	}
	return false
}

func execCommand(cmd, args string) string {
	cmd, args = strings.TrimSpace(cmd), strings.TrimSpace(args)
	if cmd == "" {
		return ""
	}
	if strings.ContainsAny(cmd, " \t") && !strings.HasPrefix(cmd, `"`) {
		cmd = `"` + cmd + `"`
	}
	if args != "" {
		cmd += " " + args
	}
	return cmd
}

// serviceAccount — учётки служб: SYSTEM, LOCAL SERVICE, NETWORK SERVICE (по имени, с NT AUTHORITY\ или по SID).
func serviceAccount(u string) bool {
	if hasPrefixFold(u, `NT AUTHORITY\`) {
		u = u[len(`NT AUTHORITY\`):]
	}
	for _, s := range []string{"SYSTEM", "LOCAL SERVICE", "LOCALSERVICE", "NETWORK SERVICE", "NETWORKSERVICE", "S-1-5-18", "S-1-5-19", "S-1-5-20"} {
		if strings.EqualFold(u, s) {
			return true
		}
	}
	return false
}

// usersGroup — встроенная группа «Пользователи» (S-1-5-32-545).
func usersGroup(g string) bool {
	return strings.EqualFold(g, "S-1-5-32-545") || strings.EqualFold(g, "Users") || strings.EqualFold(g, `BUILTIN\Users`)
}

// TaskScope — относится ли задание «при входе» к ребёнку с SID sid и учёткой account (ДОМЕН\имя
// или имя), и как (уточнение 7): своё — триггер на ребёнка или любого пользователя, а запуск от
// учётки ребёнка; либо триггер на ребёнка, а главный — группа или интерактивный без учётки.
// Общее — триггер любого пользователя и запуск от входящего (группа Users или интерактивный без
// учётки). Задания служб и чужих пользователей — ok = false.
func TaskScope(t Task, sid, account string) (scope string, ok bool) {
	if !t.Logon || !t.RunsAsUser {
		return "", false
	}
	isChild := func(u string) bool {
		return strings.EqualFold(u, sid) || sameAccount(u, account) // u не пусто: проверено выше
	}
	if t.RunAs != "" {
		if isChild(t.RunAs) && (t.UserID == "" || isChild(t.UserID)) {
			return ScopeOwn, true
		}
		return "", false
	}
	switch {
	case t.UserID == "":
		return ScopeShared, true
	case isChild(t.UserID):
		return ScopeOwn, true
	}
	return "", false
}

// sameAccount сравнивает учётки «ДОМЕН\имя» и «имя» без регистра; домены сверяются, только
// если заданы у обеих.
func sameAccount(a, b string) bool {
	da, na := splitAccount(a)
	db, nb := splitAccount(b)
	if na == "" || nb == "" || !strings.EqualFold(na, nb) {
		return false
	}
	return da == "" || db == "" || strings.EqualFold(da, db)
}

func splitAccount(s string) (domain, name string) {
	if i := strings.LastIndexByte(s, '\\'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}
