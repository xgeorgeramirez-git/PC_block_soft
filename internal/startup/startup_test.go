package startup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func testEnv(name string) string {
	switch strings.ToUpper(name) {
	case "PROGRAMFILES":
		return `C:\Program Files`
	case "LOCALAPPDATA":
		return `C:\Users\Миша\AppData\Local`
	case "WINDIR", "SYSTEMROOT":
		return `C:\Windows`
	}
	return ""
}

// Ловит: кавычки, аргументы, пробелы в пути без кавычек, переменные окружения и обёртки
// rundll32/cmd разбираются не так — тогда наблюдатель не узнает exe записи (RM5b-3), а Hidden
// не спрячет компоненты Windows (RM5b-7).
func TestExeOf(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{`"C:\Program Files\App\app.exe" /background`, `C:\Program Files\App\app.exe`},
		{`"C:\Program Files\App\app.exe"`, `C:\Program Files\App\app.exe`},
		{`  "C:\a b\x.exe"  -y`, `C:\a b\x.exe`},
		{`C:\Tools\tool.exe -minimized`, `C:\Tools\tool.exe`},
		{`C:\Program Files\Vendor App\vendor.exe /tray`, `C:\Program Files\Vendor App\vendor.exe`},
		{`C:\PROGRAM FILES\X\X.EXE`, `C:\PROGRAM FILES\X\X.EXE`},
		{`%ProgramFiles%\App\app.exe --hidden`, `C:\Program Files\App\app.exe`},
		{`"%LOCALAPPDATA%\Discord\Update.exe" --processStart Discord.exe`, `C:\Users\Миша\AppData\Local\Discord\Update.exe`},
		{`%windir%\system32\SecurityHealthSystray.exe`, `C:\Windows\system32\SecurityHealthSystray.exe`},
		{`C:\Tools\run.bat`, `C:\Tools\run.bat`},
		{`C:\Tools\run.bat /x C:\other.exe`, `C:\Tools\run.bat`},
		{`cmd /c cmd /c cmd /c cmd /c cmd /c C:\deep.exe`, ``},
		{`cmd /c cmd /c C:\two.exe`, `C:\two.exe`},
		{`notepad`, `notepad`},
		{`rundll32.exe C:\Users\Миша\AppData\Roaming\x\helper.dll,Start`, `C:\Users\Миша\AppData\Roaming\x\helper.dll`},
		{`C:\Windows\System32\rundll32.exe "C:\Program Files\Y\y.dll",#1 arg`, `C:\Program Files\Y\y.dll`},
		{`RUNDLL32 shell32.dll,Control_RunDLL`, `shell32.dll`},
		{`cmd /c "C:\Games\launcher.exe" -silent`, `C:\Games\launcher.exe`},
		{`C:\Windows\system32\cmd.exe /C start "" "C:\Program Files\Z\z.exe" /min`, `C:\Program Files\Z\z.exe`},
		{`cmd.exe /k start C:\Games\g.exe`, `C:\Games\g.exe`},
		{`cmd /c start "Окно" C:\Games\g.exe`, `C:\Games\g.exe`},
		{`cmd /c start /b C:\Games\g.exe`, `C:\Games\g.exe`},
		{`cmd.exe /q`, `cmd.exe`},
		{`cmd /c`, ``},
		{`rundll32.exe`, ``},
		{``, ``},
		{`   `, ``},
		{`"C:\no end quote.exe`, ``},
		{`%NOSUCHVAR%\x.exe`, ``},
		{`%unclosed\x.exe`, ``},
		{`%%\x.exe`, ``},
	} {
		if got := ExeOf(tc.cmd, testEnv); got != tc.want {
			t.Errorf("ExeOf(%q) = %q, want %q", tc.cmd, got, tc.want)
		}
	}
	if got := ExeOf(`%ProgramFiles%\a.exe`, nil); got != "" {
		t.Errorf("ExeOf без окружения с переменной = %q, want пусто", got)
	}
	if got := ExeOf(`C:\a.exe /x`, nil); got != `C:\a.exe` {
		t.Errorf("ExeOf без окружения = %q, want C:\\a.exe", got)
	}
}

// Ловит: включённость StartupApproved читается не по первому байту, принимает значение не
// из 12 байт или незнакомый байт — запись ошибочно «включена» или «выключена» (RM5b-2).
func TestDecodeApproved(t *testing.T) {
	tail := make([]byte, 11)
	for _, tc := range []struct {
		b           []byte
		enabled, ok bool
	}{
		{append([]byte{0x02}, tail...), true, true},
		{append([]byte{0x06}, tail...), true, true},
		{append([]byte{0x03}, tail...), false, true},
		{append([]byte{0x07}, tail...), false, true},
		{[]byte{0x03, 0, 0, 0, 0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x01}, false, true},
		{append([]byte{0x00}, tail...), false, false},
		{append([]byte{0x01}, tail...), false, false},
		{append([]byte{0x04}, tail...), false, false},
		{[]byte{0x02}, false, false},
		{append([]byte{0x02}, make([]byte, 12)...), false, false},
		{nil, false, false},
	} {
		en, ok := DecodeApproved(tc.b)
		if en != tc.enabled || ok != tc.ok {
			t.Errorf("DecodeApproved(% x) = %v, %v; want %v, %v", tc.b, en, ok, tc.enabled, tc.ok)
		}
	}
}

// Ловит: выключение пишет не 12 байт, не 0x03 или неверное время FILETIME (Диспетчер задач
// показывает «Выключено» с датой), включение — не 0x02 с нулями.
func TestEncodeApproved(t *testing.T) {
	now := time.Date(2026, 11, 14, 18, 40, 0, 0, time.UTC)
	off := EncodeApproved(false, now)
	if len(off) != 12 || off[0] != 0x03 || off[1] != 0 || off[2] != 0 || off[3] != 0 {
		t.Fatalf("выключено = % x, want 03 00 00 00 + FILETIME", off)
	}
	const wantFT = uint64(134391552000000000) // 2026-11-14 18:40:00 UTC в интервалах по 100 нс с 1601 года
	if ft := binary.LittleEndian.Uint64(off[4:]); ft != wantFT {
		t.Errorf("FILETIME = %d, want %d", ft, wantFT)
	}
	if en, ok := DecodeApproved(off); en || !ok {
		t.Errorf("DecodeApproved(выключено) = %v, %v; want false, true", en, ok)
	}
	on := EncodeApproved(true, now)
	if !reflect.DeepEqual(on, []byte{0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("включено = % x, want 02 и 11 нулей", on)
	}
	local := EncodeApproved(false, now.In(time.FixedZone("MSK", 3*3600)))
	if !reflect.DeepEqual(local, off) {
		t.Errorf("FILETIME зависит от пояса: % x против % x", local, off)
	}
}

func readTask(t *testing.T, name string) Task {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	task, err := ParseTaskXML(data)
	if err != nil {
		t.Fatalf("ParseTaskXML(%s): %v", name, err)
	}
	return task
}

// Ловит: задания из папки System32\Tasks (UTF-16 LE с BOM, объявлено encoding="UTF-16") не
// читаются; теряется путь, команда с аргументами, триггер «при входе», его пользователь,
// выключенность или учётка, от которой запускается (RM5b-1, уточнение 7). Образцы в testdata
// составлены по схеме Планировщика, не сняты с реальной машины.
func TestParseTaskXMLSamples(t *testing.T) {
	const sid = "S-1-5-21-3623811015-3361044348-30300820-1013"
	for _, tc := range []struct {
		file string
		want Task
	}{
		{"onedrive_logon.xml", Task{Path: `\OneDrive Reporting Task-` + sid, Command: `%localappdata%\Microsoft\OneDrive\OneDriveStandaloneUpdater.exe /reporting`,
			Logon: true, UserID: sid, Enabled: true, RunsAsUser: true, RunAs: sid}},
		{"onedrive_logon_utf8.xml", Task{Path: `\OneDrive Reporting Task-` + sid, Command: `%localappdata%\Microsoft\OneDrive\OneDriveStandaloneUpdater.exe /reporting`,
			Logon: true, UserID: sid, Enabled: true, RunsAsUser: true, RunAs: sid}},
		{"teams_logon.xml", Task{Path: `\Chat Helper Update`, Command: `"C:\Users\Миша\AppData\Local\Chat Helper\Update.exe" --processStart "ChatHelper.exe" --process-start-args "--system-initiated"`,
			Logon: true, UserID: `DESKTOP-KIDS\Миша`, Enabled: false, RunsAsUser: true, RunAs: sid}},
		{"system_logon.xml", Task{Path: `\Vendor\Vendor Service Watchdog`, Command: `"C:\Program Files\Vendor\watchdog.exe"`,
			Logon: true, UserID: "", Enabled: true, RunsAsUser: false, RunAs: "S-1-5-18"}},
		{"users_group_logon.xml", Task{Path: `\Tray Notifier`, Command: `"C:\Program Files (x86)\Tray Notifier\notifier.exe" /tray`,
			Logon: true, UserID: "", Enabled: true, RunsAsUser: true, Group: "S-1-5-32-545"}},
	} {
		if got := readTask(t, tc.file); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", tc.file, got, tc.want)
		}
	}
}

func utf16LE(s string, bom bool) []byte {
	var out []byte
	if bom {
		out = []byte{0xff, 0xfe}
	}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func utf16BE(s string, bom bool) []byte {
	var out []byte
	if bom {
		out = []byte{0xfe, 0xff}
	}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u>>8), byte(u))
	}
	return out
}

const taskNS = `xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"`

func taskXML(body string) string {
	return `<?xml version="1.0" encoding="UTF-16"?><Task version="1.4" ` + taskNS + `>` + body + `</Task>`
}

// Ловит: каждое условие разбора заданий (кодировки, триггеры, главный, действия) — мутация
// любого условия меняет итог хотя бы одной строки таблицы.
func TestParseTaskXMLRules(t *testing.T) {
	const reg = `<RegistrationInfo><URI>\T</URI></RegistrationInfo>`
	const act = `<Actions Context="Author"><Exec><Command>C:\a.exe</Command></Exec></Actions>`
	const interactive = `<Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType></Principal></Principals>`
	base := Task{Path: `\T`, Command: `C:\a.exe`, Logon: true, Enabled: true, RunsAsUser: true}
	with := func(f func(*Task)) Task { t := base; f(&t); return t }
	for _, tc := range []struct {
		name string
		xml  string
		want Task
	}{
		{"при входе любого", reg + `<Triggers><LogonTrigger/></Triggers>` + interactive + act, base},
		{"триггер выключен", reg + `<Triggers><LogonTrigger><Enabled>false</Enabled><UserId>S-1-5-21-1</UserId></LogonTrigger></Triggers>` + interactive + act,
			with(func(t *Task) { t.Logon = false })},
		{"нет триггера входа", reg + `<Triggers><BootTrigger/><TimeTrigger><StartBoundary>2026-01-01T00:00:00</StartBoundary></TimeTrigger></Triggers>` + interactive + act,
			with(func(t *Task) { t.Logon = false })},
		{"нет триггеров", reg + interactive + act, with(func(t *Task) { t.Logon = false })},
		{"вход любого сильнее входа одного", reg + `<Triggers><LogonTrigger><UserId>S-1-5-21-1</UserId></LogonTrigger><LogonTrigger/></Triggers>` + interactive + act, base},
		{"два входа разных пользователей — первый", reg + `<Triggers><LogonTrigger><UserId>S-1-5-21-1</UserId></LogonTrigger><LogonTrigger><UserId>S-1-5-21-2</UserId></LogonTrigger></Triggers>` + interactive + act,
			with(func(t *Task) { t.UserID = "S-1-5-21-1" })},
		{"без Context — первый главный", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal id="A"><LogonType>InteractiveToken</LogonType></Principal><Principal><UserId>SYSTEM</UserId></Principal></Principals><Actions><Exec><Command>C:\a.exe</Command></Exec></Actions>`, base},
		{"выключенный триггер не учитывается", reg + `<Triggers><LogonTrigger><Enabled>false</Enabled></LogonTrigger><LogonTrigger><UserId>S-1-5-21-1</UserId></LogonTrigger></Triggers>` + interactive + act,
			with(func(t *Task) { t.UserID = "S-1-5-21-1" })},
		{"задание выключено", reg + `<Triggers><LogonTrigger/></Triggers><Settings><Enabled>false</Enabled></Settings>` + interactive + act,
			with(func(t *Task) { t.Enabled = false })},
		{"задание выключено нулём", reg + `<Triggers><LogonTrigger/></Triggers><Settings><Enabled>0</Enabled></Settings>` + interactive + act,
			with(func(t *Task) { t.Enabled = false })},
		{"Enabled с пробелами и заглавными", reg + `<Triggers><LogonTrigger><Enabled> FALSE </Enabled></LogonTrigger></Triggers><Settings><Enabled>True</Enabled></Settings>` + interactive + act,
			with(func(t *Task) { t.Logon = false })},
		{"главный по Context", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal id="Sys"><UserId>SYSTEM</UserId></Principal><Principal id="Author"><LogonType>InteractiveToken</LogonType></Principal></Principals>` + act, base},
		{"Context на систему", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType></Principal><Principal id="Sys"><UserId>NT AUTHORITY\SYSTEM</UserId></Principal></Principals><Actions Context="Sys"><Exec><Command>C:\a.exe</Command></Exec></Actions>`,
			with(func(t *Task) { t.RunsAsUser = false; t.RunAs = `NT AUTHORITY\SYSTEM` })},
		{"Context без пары — первый", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal id="X"><UserId>S-1-5-19</UserId></Principal></Principals><Actions Context="Nope"><Exec><Command>C:\a.exe</Command></Exec></Actions>`,
			with(func(t *Task) { t.RunsAsUser = false; t.RunAs = "S-1-5-19" })},
		{"LOCAL SERVICE", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal><UserId>local service</UserId><LogonType>InteractiveToken</LogonType></Principal></Principals>` + act,
			with(func(t *Task) { t.RunsAsUser = false; t.RunAs = "local service" })},
		{"NETWORK SERVICE по SID", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal><UserId>S-1-5-20</UserId></Principal></Principals>` + act,
			with(func(t *Task) { t.RunsAsUser = false; t.RunAs = "S-1-5-20" })},
		{"пользователь с паролем", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal><UserId>PC\Папа</UserId><LogonType>Password</LogonType></Principal></Principals>` + act,
			with(func(t *Task) { t.RunAs = `PC\Папа` })},
		{"группа Users по имени", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal><GroupId>BUILTIN\Users</GroupId></Principal></Principals>` + act,
			with(func(t *Task) { t.Group = `BUILTIN\Users` })},
		{"группа Users коротко", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal><GroupId>users</GroupId></Principal></Principals>` + act,
			with(func(t *Task) { t.Group = `users` })},
		{"группа администраторов", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal><GroupId>S-1-5-32-544</GroupId></Principal></Principals>` + act,
			with(func(t *Task) { t.RunsAsUser = false; t.Group = "S-1-5-32-544" })},
		{"главный без учётки и не интерактивный", reg + `<Triggers><LogonTrigger/></Triggers><Principals><Principal><LogonType>S4U</LogonType></Principal></Principals>` + act,
			with(func(t *Task) { t.RunsAsUser = false })},
		{"без главного", reg + `<Triggers><LogonTrigger/></Triggers>` + act, with(func(t *Task) { t.RunsAsUser = false })},
		{"без URI", `<Triggers><LogonTrigger/></Triggers>` + interactive + act, with(func(t *Task) { t.Path = "" })},
		{"без Exec (COM)", reg + `<Triggers><LogonTrigger/></Triggers>` + interactive + `<Actions><ComHandler><ClassId>{0}</ClassId></ComHandler></Actions>`,
			with(func(t *Task) { t.Command = "" })},
		{"первое Exec, аргументы, пробел в пути", reg + `<Triggers><LogonTrigger/></Triggers>` + interactive + `<Actions><Exec><Command> C:\A B\a.exe </Command><Arguments> -x  </Arguments></Exec><Exec><Command>C:\b.exe</Command></Exec></Actions>`,
			with(func(t *Task) { t.Command = `"C:\A B\a.exe" -x` })},
		{"путь в кавычках не берётся в кавычки дважды", reg + `<Triggers><LogonTrigger/></Triggers>` + interactive + `<Actions><Exec><Command>"C:\A B\a.exe"</Command></Exec></Actions>`,
			with(func(t *Task) { t.Command = `"C:\A B\a.exe"` })},
	} {
		for _, enc := range []struct {
			name string
			data []byte
		}{
			{"UTF-16 LE", utf16LE(taskXML(tc.xml), true)},
			{"UTF-16 BE", utf16BE(taskXML(tc.xml), true)},
			{"UTF-8", []byte(strings.Replace(taskXML(tc.xml), "UTF-16", "UTF-8", 1))},
			{"UTF-8 с BOM", append([]byte{0xef, 0xbb, 0xbf}, strings.Replace(taskXML(tc.xml), `encoding="UTF-16"`, `encoding="utf-8"`, 1)...)},
		} {
			got, err := ParseTaskXML(enc.data)
			if err != nil {
				t.Errorf("%s, %s: %v", tc.name, enc.name, err)
				continue
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s, %s:\n got %+v\nwant %+v", tc.name, enc.name, got, tc.want)
			}
		}
	}
}

// Ловит: мусор, чужой корень, нечётная длина UTF-16 или незнакомая кодировка принимаются за
// задание (пустое задание «при входе» стало бы записью без команды).
func TestParseTaskXMLErrors(t *testing.T) {
	for name, data := range map[string][]byte{
		"пусто":                nil,
		"не XML":               []byte("hello"),
		"чужой корень":         []byte(`<?xml version="1.0"?><Job ` + taskNS + `></Job>`),
		"оборванный":           []byte(`<?xml version="1.0"?><Task ` + taskNS + `><Triggers>`),
		"нечётный UTF-16":      append(utf16LE(taskXML(""), true), 0x41),
		"незнакомая кодировка": []byte(`<?xml version="1.0" encoding="KOI8-R"?><Task ` + taskNS + `></Task>`),
	} {
		if _, err := ParseTaskXML(data); err == nil {
			t.Errorf("%s: без ошибки", name)
		}
	}
	// UTF-16 без BOM (так файл не пишет Планировщик): распознаётся по нулевым байтам.
	got, err := ParseTaskXML(utf16LE(taskXML(`<RegistrationInfo><URI>\T</URI></RegistrationInfo>`), false))
	if err != nil || got.Path != `\T` {
		t.Errorf("UTF-16 LE без BOM = %+v, %v; want путь \\T", got, err)
	}
	got, err = ParseTaskXML(utf16BE(taskXML(`<RegistrationInfo><URI>\T</URI></RegistrationInfo>`), false))
	if err != nil || got.Path != `\T` {
		t.Errorf("UTF-16 BE без BOM = %+v, %v; want путь \\T", got, err)
	}
}

// Ловит: свои и общие задания путаются с чужими и системными (уточнение 7): своё — триггер на
// SID или имя ребёнка (с доменом и без, без регистра); общее — триггер без пользователя.
func TestTaskScope(t *testing.T) {
	const sid = "S-1-5-21-1-2-3-1001"
	own := Task{Logon: true, UserID: sid, RunsAsUser: true, RunAs: sid}
	for _, tc := range []struct {
		name  string
		t     Task
		scope string
		ok    bool
	}{
		{"своё по SID", own, ScopeOwn, true},
		{"своё по имени с доменом", Task{Logon: true, UserID: `DESKTOP-KIDS\миша`, RunsAsUser: true}, ScopeOwn, true},
		{"своё по имени", Task{Logon: true, UserID: `Миша`, RunsAsUser: true}, ScopeOwn, true},
		{"своё без главного пользователя", Task{Logon: true, UserID: sid, RunsAsUser: true, Group: "S-1-5-32-545"}, ScopeOwn, true},
		{"чужой триггер", Task{Logon: true, UserID: "S-1-5-21-1-2-3-1002", RunsAsUser: true}, "", false},
		{"чужое имя", Task{Logon: true, UserID: `DESKTOP-KIDS\Аня`, RunsAsUser: true}, "", false},
		{"триггер ребёнка, запуск от папы", Task{Logon: true, UserID: sid, RunsAsUser: true, RunAs: `PC\Папа`}, "", false},
		{"от системы", Task{Logon: true, UserID: sid, RunsAsUser: false, RunAs: "S-1-5-18"}, "", false},
		{"общее для группы", Task{Logon: true, RunsAsUser: true, Group: "S-1-5-32-545"}, ScopeShared, true},
		{"общее интерактивное без учётки", Task{Logon: true, RunsAsUser: true}, ScopeShared, true},
		{"любой вход, запуск от ребёнка — своё", Task{Logon: true, RunsAsUser: true, RunAs: sid}, ScopeOwn, true},
		{"общее, запуск от папы", Task{Logon: true, RunsAsUser: true, RunAs: `PC\Папа`}, "", false},
		{"общее от системы", Task{Logon: true, RunsAsUser: false, RunAs: "S-1-5-18"}, "", false},
		{"не при входе", Task{Logon: false, UserID: sid, RunsAsUser: true}, "", false},
		{"триггер ребёнка, главный не пользователь", Task{Logon: true, UserID: sid, RunsAsUser: false, Group: "S-1-5-32-544"}, "", false},
		{"любой вход, главный не пользователь", Task{Logon: true, RunsAsUser: false}, "", false},
		{"триггер Ани, запуск от ребёнка", Task{Logon: true, UserID: "S-1-5-21-1-2-3-1002", RunsAsUser: true, RunAs: sid}, "", false},
		{"триггер и запуск от ребёнка по имени", Task{Logon: true, UserID: `Миша`, RunsAsUser: true, RunAs: `DESKTOP-KIDS\Миша`}, ScopeOwn, true},
	} {
		scope, ok := TaskScope(tc.t, sid, `DESKTOP-KIDS\Миша`)
		if scope != tc.scope || ok != tc.ok {
			t.Errorf("%s: TaskScope = %q, %v; want %q, %v", tc.name, scope, ok, tc.scope, tc.ok)
		}
	}
	if _, ok := TaskScope(Task{Logon: true, UserID: `Миша`, RunsAsUser: true}, sid, ""); ok {
		t.Error("пустое имя ребёнка совпало с именем в триггере")
	}
	if _, ok := TaskScope(Task{Logon: true, UserID: `X\`, RunsAsUser: true}, sid, `X\`); ok {
		t.Error("имя из одного домена совпало")
	}
}

// Ловит: в список попадают компоненты Windows, задания Microsoft, сам PC Block или пустые
// команды — либо прячутся обычные программы из профиля, Program Files и Temp (RM5b-7).
func TestHidden(t *testing.T) {
	const windir = `C:\Windows`
	const pcb = `C:\Program Files\PCBlock`
	for _, tc := range []struct {
		name string
		e    Entry
		want bool
	}{
		{"системный трей", Entry{Source: SrcRun, Command: `%windir%\system32\SecurityHealthSystray.exe`, Exe: `C:\Windows\system32\SecurityHealthSystray.exe`}, true},
		{"регистр и косая", Entry{Source: SrcRun, Command: "x", Exe: `c:/WINDOWS/SysWOW64/x.exe`}, true},
		{"rundll32 с dll Windows", Entry{Source: SrcRun, Command: "x", Exe: `C:\Windows\System32\shell32.dll`}, true},
		{"Temp Windows не прячется", Entry{Source: SrcRun, Command: "x", Exe: `C:\Windows\Temp\setup.exe`}, false},
		{"похожая папка", Entry{Source: SrcRun, Command: "x", Exe: `C:\WindowsApps2\x.exe`}, false},
		{"сам Windows-каталог как exe", Entry{Source: SrcRun, Command: "x", Exe: `C:\Windows`}, false},
		{"PC Block", Entry{Source: SrcRun, Command: "x", Exe: `C:\Program Files\PCBlock\PCBlock.exe`}, true},
		{"рядом с PC Block", Entry{Source: SrcRun, Command: "x", Exe: `C:\Program Files\PCBlockHelper\h.exe`}, false},
		{"обычная программа", Entry{Source: SrcRun, Command: "x", Exe: `C:\Program Files\App\app.exe`}, false},
		{"профиль", Entry{Source: SrcFolder, Command: "x", Exe: `C:\Users\Миша\AppData\Roaming\x.exe`}, false},
		{"пустая команда", Entry{Source: SrcRun, Command: "  ", Exe: `C:\App\a.exe`}, true},
		{"задание Microsoft", Entry{Source: SrcTask, Task: `\Microsoft\Windows\Defrag\ScheduledDefrag`, Command: "x", Exe: `C:\App\a.exe`}, true},
		{"задание microsoft строчными", Entry{Source: SrcTask, Task: `\microsoft\Office\x`, Command: "x", Exe: `C:\App\a.exe`}, true},
		{"задание с похожим именем", Entry{Source: SrcTask, Task: `\MicrosoftEdgeUpdateTaskMachineUA`, Command: "x", Exe: `C:\App\a.exe`}, false},
		{"путь Microsoft не у задания", Entry{Source: SrcRun, Task: `\Microsoft\x`, Command: "x", Exe: `C:\App\a.exe`}, false},
		{"exe не разобран", Entry{Source: SrcRun, Command: "x"}, false},
		{"приложение магазина", Entry{Source: SrcStore, Command: "x", Package: "SpotifyAB.SpotifyMusic_zpdnekdrzrea0"}, false},
	} {
		if got := Hidden(tc.e, windir, pcb); got != tc.want {
			t.Errorf("%s: Hidden = %v, want %v", tc.name, got, tc.want)
		}
	}
	if Hidden(Entry{Source: SrcRun, Command: "x", Exe: `C:\Windows\x.exe`}, "", "") {
		t.Error("пустой windir спрятал запись")
	}
	if Hidden(Entry{Source: SrcRun, Command: "x", Exe: `C:\x.exe`}, `C:\Windows\`, `C:\`+"\\") {
		t.Error("корень диска как папка PC Block спрятал всё")
	}
	if !Hidden(Entry{Source: SrcRun, Command: "x", Exe: `C:\Windows\x.exe`}, `C:\Windows\`, "") {
		t.Error("windir с косой в конце не спрятал запись Windows")
	}
}

// Ловит: подпись «откуда» не дословно по ТЗ 8.3.11.
func TestFromText(t *testing.T) {
	for src, want := range map[string]string{
		SrcRun:     "Реестр, при входе",
		SrcRunOnce: "Реестр, один раз при входе",
		SrcFolder:  "Папка «Автозагрузка»",
		SrcTask:    "Планировщик заданий, при входе",
		SrcStore:   "Приложение из магазина",
		"other":    "",
	} {
		if got := FromText(Entry{Source: src}); got != want {
			t.Errorf("FromText(%s) = %q, want %q", src, got, want)
		}
	}
}

// Ловит: сверка «было/найдено» не по ключу Scope+Source+Name (одноимённые свои и общие
// сливаются), изменение команды или издателя не замечено, решение пропавшей теряется
// молча, nil вместо [] (RM5b-4).
func TestCompare(t *testing.T) {
	a := Entry{Scope: ScopeOwn, Source: SrcRun, Name: "A", Command: "a.exe"}
	aShared := Entry{Scope: ScopeShared, Source: SrcRun, Name: "A", Command: "a.exe"}
	aOnce := Entry{Scope: ScopeOwn, Source: SrcRunOnce, Name: "A", Command: "a.exe"}
	b := Entry{Scope: ScopeOwn, Source: SrcTask, Name: `\B`, Command: "b.exe", Publisher: "P"}
	bCmd := b
	bCmd.Command = "b2.exe"
	bPub := b
	bPub.Publisher = "Q"
	bState := b
	bState.Enabled, bState.Exe = true, `C:\b.exe` // включённость и exe — не изменение записи
	c := Entry{Scope: ScopeOwn, Source: SrcFolder, Name: "C.lnk", Command: "c.exe"}

	d := Compare([]Entry{a, b, c}, []Entry{aShared, bCmd, aOnce, a})
	want := Diff{New: []Entry{aShared, aOnce}, Changed: []Entry{bCmd}, Gone: []Entry{c}}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("Compare:\n got %+v\nwant %+v", d, want)
	}
	if d := Compare([]Entry{b}, []Entry{bPub}); !reflect.DeepEqual(d.Changed, []Entry{bPub}) {
		t.Errorf("смена издателя: Changed = %+v", d.Changed)
	}
	empty := Compare(nil, []Entry{bState})
	if empty.New == nil || empty.Changed == nil || empty.Gone == nil || len(empty.New) != 1 {
		t.Errorf("пустое было: %#v; want New=[b], остальное []", empty)
	}
	same := Compare([]Entry{b}, []Entry{bState})
	if len(same.New)+len(same.Changed)+len(same.Gone) != 0 || same.New == nil || same.Changed == nil || same.Gone == nil {
		t.Errorf("та же запись: %#v; want пустые []", same)
	}
}
