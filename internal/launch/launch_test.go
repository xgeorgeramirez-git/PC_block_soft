package launch

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Все режимы с ключом, именем для интерфейса (App.Mode()) и признаком «открывает окно Wails».
// Ожидания записаны литералами из пакета поручения.
var allModes = []struct {
	mode   Mode
	flag   string
	name   string
	window bool
}{
	{ModeWindow, "", "window", true},
	{ModeService, "--service", "service", false},
	{ModeInstallService, "--install-service", "install-service", false},
	{ModeRemoveService, "--remove-service", "remove-service", false},
	{ModeGuard, "--guard", "guard", false},
	{ModeTray, "--tray", "tray", false},
	{ModeParent, "--parent", "parent", true},
	{ModeMyStats, "--mystats", "mystats", true},
	{ModeFirstRun, "--firstrun", "firstrun", true},
	{ModeWizard, "--wizard", "wizard", true},
	{ModeResetPassword, "--reset-password", "reset-password", true},
	{ModeUninstallGate, "--uninstall-gate", "uninstall-gate", true},
	{ModeTestSetup, "--test-setup", "test-setup", false},
	{ModeTestIPC, "--test-ipc", "test-ipc", false},
	{ModeTestRunAs, "--test-runas", "test-runas", false},
}

// Параметры, у которых значение необязательно: без значения равны «1».
var flagParams = []string{"silent", "delete-data", "elevated"}

// Параметры со значением вида --key=value.
var valueParams = []string{"out", "session", "handover", "password", "new-password", "payload", "user", "exe"}

// Какому режиму какой параметр положен (расшифровка плана M1: Ruling 2 и 23, S25, T9, T10, T16).
var paramModes = map[string][]string{
	"out":          {"--install-service", "--remove-service", "--reset-password", "--test-setup", "--test-ipc", "--test-runas"},
	"session":      {"--guard", "--tray"},
	"handover":     {"--parent"},
	"password":     {"--test-setup", "--uninstall-gate", "--test-runas"},
	"new-password": {"--reset-password"},
	"silent":       {"--install-service", "--remove-service", "--uninstall-gate"},
	"delete-data":  {"--remove-service", "--uninstall-gate"},
	"elevated":     {"--reset-password"},
	"payload":      {"--test-ipc"},
	"user":         {"--test-setup", "--test-runas"},
	"exe":          {"--test-runas"},
}

// Ловит (Ruling 22): запуск без ключей, каким программу запускает генератор биндингов Wails,
// считается ошибкой или не окном; пустая карта параметров не должна быть nil.
func TestParseNoArgsIsWindow(t *testing.T) {
	for name, args := range map[string][]string{"nil": nil, "пустой срез": {}} {
		mode, opts, err := Parse(args)
		if err != nil {
			t.Fatalf("%s: Parse вернул ошибку %v", name, err)
		}
		if mode != ModeWindow {
			t.Errorf("%s: режим %v, ожидалось окно", name, mode)
		}
		if opts == nil || len(opts) != 0 {
			t.Errorf("%s: параметры %#v, ожидалась пустая карта, не nil", name, opts)
		}
	}
}

// Ловит: режим не распознан по ключу или у режима не то имя для интерфейса (App.Mode()),
// не тот признак окна, не тот ключ для командной строки службы.
func TestParseEveryMode(t *testing.T) {
	if len(allModes) != 15 {
		t.Fatalf("в таблице теста режимов %d, в пакете поручения их 15", len(allModes))
	}
	names := map[string]bool{}
	for _, tc := range allModes {
		if names[tc.name] {
			t.Errorf("имя режима %q повторяется", tc.name)
		}
		names[tc.name] = true
		if tc.flag != "" {
			mode, opts, err := Parse([]string{tc.flag})
			if err != nil || mode != tc.mode || len(opts) != 0 {
				t.Errorf("Parse(%q) = %v, %v, %v; ожидалось %v без параметров и ошибки", tc.flag, mode, opts, err, tc.mode)
			}
		}
		if got := tc.mode.String(); got != tc.name {
			t.Errorf("%d.String() = %q, ожидалось %q", int(tc.mode), got, tc.name)
		}
		if got := tc.mode.Flag(); got != tc.flag {
			t.Errorf("%q.Flag() = %q, ожидалось %q", tc.name, got, tc.flag)
		}
		if got := tc.mode.IsWindow(); got != tc.window {
			t.Errorf("%q.IsWindow() = %v, ожидалось %v", tc.name, got, tc.window)
		}
	}
}

// Ловит: неизвестное число режима выглядит как настоящий режим или как пустая строка.
func TestUnknownModeValue(t *testing.T) {
	for _, m := range []Mode{Mode(15), Mode(99), Mode(-1)} {
		if m.String() != "unknown" || m.Flag() != "" || m.IsWindow() {
			t.Errorf("Mode(%d): String=%q Flag=%q IsWindow=%v, ожидалось unknown, пусто, false", int(m), m.String(), m.Flag(), m.IsWindow())
		}
	}
}

// Ловит: параметр не разобран или потерял значение; каждый разрешённый параметр
// принимается вместе с подходящим режимом.
func TestParseEveryParameter(t *testing.T) {
	cases := []struct {
		args []string
		mode Mode
		want Options
	}{
		{[]string{"--install-service", "--out=C:\\Temp\\r.json"}, ModeInstallService, Options{"out": "C:\\Temp\\r.json"}},
		{[]string{"--guard", "--session=2"}, ModeGuard, Options{"session": "2"}},
		{[]string{"--tray", "--session=3"}, ModeTray, Options{"session": "3"}},
		{[]string{"--parent", "--handover=7f3a"}, ModeParent, Options{"handover": "7f3a"}},
		{[]string{"--test-setup", "--password=Secret1", "--user=runner", "--out=o.json"}, ModeTestSetup,
			Options{"password": "Secret1", "user": "runner", "out": "o.json"}},
		{[]string{"--reset-password", "--new-password=Новый-Пароль-1"}, ModeResetPassword, Options{"new-password": "Новый-Пароль-1"}},
		{[]string{"--reset-password", "--elevated"}, ModeResetPassword, Options{"elevated": "1"}},
		{[]string{"--remove-service", "--silent", "--delete-data"}, ModeRemoveService, Options{"silent": "1", "delete-data": "1"}},
		{[]string{"--uninstall-gate", "--silent", "--password=p"}, ModeUninstallGate, Options{"silent": "1", "password": "p"}},
		{[]string{"--test-ipc", `--payload={"cmd":"Hello"}`, "--out=r.json"}, ModeTestIPC,
			Options{"payload": `{"cmd":"Hello"}`, "out": "r.json"}},
		{[]string{"--test-runas", "--user=pcbtest", "--password=P@ss=1", "--exe=C:\\Program Files\\PCBlock\\PCBlock.exe --test-ipc"}, ModeTestRunAs,
			Options{"user": "pcbtest", "password": "P@ss=1", "exe": "C:\\Program Files\\PCBlock\\PCBlock.exe --test-ipc"}},
	}
	covered := map[string]bool{}
	for _, tc := range cases {
		mode, opts, err := Parse(tc.args)
		if err != nil {
			t.Errorf("Parse(%q): ошибка %v", tc.args, err)
			continue
		}
		if mode != tc.mode || !reflect.DeepEqual(opts, tc.want) {
			t.Errorf("Parse(%q) = %v, %#v; ожидалось %v, %#v", tc.args, mode, opts, tc.mode, tc.want)
		}
		for k := range tc.want {
			covered[k] = true
		}
	}
	for _, k := range append(append([]string{}, flagParams...), valueParams...) {
		if !covered[k] {
			t.Errorf("параметр --%s не проверен ни одним примером", k)
		}
	}
}

// Ловит (L-37): значение режется не по первому «=», теряет пробелы и скобки; JSON для
// --test-ipc должен дойти целиком, пустое значение допустимо.
func TestParseValuesAreKeptWhole(t *testing.T) {
	cases := []struct{ name, arg, key, want string }{
		{"JSON из плана", `--payload={"cmd":"Hello"}`, "payload", `{"cmd":"Hello"}`},
		{"JSON со вложенным знаком равно", `--payload={"cmd":"SaveLimits","args":{"q":"a=b","days":[1,2,3]}}`, "payload",
			`{"cmd":"SaveLimits","args":{"q":"a=b","days":[1,2,3]}}`},
		{"JSON с пробелами", `--payload={ "cmd": "Hello", "n": 1 }`, "payload", `{ "cmd": "Hello", "n": 1 }`},
		{"пустое значение", "--payload=", "payload", ""},
		{"значение из знаков равно", "--payload===", "payload", "=="},
		{"пробелы по краям сохраняются", "--payload=  a b  ", "payload", "  a b  "},
		{"кириллица", "--payload=Привет, мир", "payload", "Привет, мир"},
		{"значение, похожее на ключ", "--payload=--service", "payload", "--service"},
	}
	for _, tc := range cases {
		mode, opts, err := Parse([]string{"--test-ipc", tc.arg})
		if err != nil {
			t.Errorf("%s: ошибка %v", tc.name, err)
			continue
		}
		if mode != ModeTestIPC {
			t.Errorf("%s: режим %v", tc.name, mode)
		}
		got, present := opts[tc.key]
		if !present || got != tc.want {
			t.Errorf("%s: %s = %q (есть=%v), ожидалось %q", tc.name, tc.key, got, present, tc.want)
		}
	}
}

// Ловит: ключи --silent и --delete-data без значения не становятся «1», значение «0» теряется,
// а любое другое значение (yes, пусто, 2) молча принимается за «включено».
func TestParseFlagParameters(t *testing.T) {
	cases := []struct {
		args []string
		want Options
	}{
		{[]string{"--remove-service", "--silent"}, Options{"silent": "1"}},
		{[]string{"--remove-service", "--silent=1"}, Options{"silent": "1"}},
		{[]string{"--remove-service", "--silent=0"}, Options{"silent": "0"}},
		{[]string{"--remove-service", "--delete-data"}, Options{"delete-data": "1"}},
		{[]string{"--remove-service", "--delete-data=0", "--silent"}, Options{"delete-data": "0", "silent": "1"}},
	}
	for _, tc := range cases {
		mode, opts, err := Parse(tc.args)
		if err != nil || mode != ModeRemoveService || !reflect.DeepEqual(opts, tc.want) {
			t.Errorf("Parse(%q) = %v, %#v, %v; ожидалось параметры %#v", tc.args, mode, opts, err, tc.want)
		}
	}
	for _, bad := range []string{"--silent=yes", "--silent=", "--silent=2", "--silent=true", "--delete-data=да"} {
		_, _, err := Parse([]string{"--remove-service", bad})
		if err == nil || !strings.Contains(err.Error(), "допустимы только значения 0 и 1") {
			t.Errorf("Parse(%q): ошибка %v, ожидалось «допустимы только значения 0 и 1»", bad, err)
		}
	}
}

// Ловит: разбор зависит от порядка ключей: параметр до режима отвергается или режим
// после параметров теряется.
func TestParseOrderDoesNotMatter(t *testing.T) {
	a, optsA, errA := Parse([]string{"--out=r.json", "--payload={}", "--test-ipc"})
	b, optsB, errB := Parse([]string{"--test-ipc", "--payload={}", "--out=r.json"})
	if errA != nil || errB != nil || a != ModeTestIPC || b != ModeTestIPC || !reflect.DeepEqual(optsA, optsB) {
		t.Errorf("порядок изменил результат: %v %#v %v / %v %#v %v", a, optsA, errA, b, optsB, errB)
	}
}

// Ловит: параметр принят там, где он не положен, или отвергнут там, где положен.
// Проверяются все десять параметров на всех пятнадцати режимах.
func TestParseParameterMatrix(t *testing.T) {
	for _, param := range append(append([]string{}, valueParams...), flagParams...) {
		allowed := map[string]bool{}
		for _, f := range paramModes[param] {
			allowed[f] = true
		}
		arg := "--" + param + "=x"
		for _, flag := range flagParams {
			if param == flag {
				arg = "--" + param
			}
		}
		for _, m := range allModes {
			args := []string{arg}
			if m.flag != "" {
				args = []string{m.flag, arg}
			}
			_, _, err := Parse(args)
			if allowed[m.flag] && err != nil {
				t.Errorf("Parse(%q): параметр должен быть допустим, ошибка %v", args, err)
			}
			if !allowed[m.flag] && err == nil {
				t.Errorf("Parse(%q): параметр не положен этому режиму, а ошибки нет", args)
			}
		}
	}
}

// Ловит: ошибки разбора теряют точный человеческий текст или первую по порядку ошибку,
// возвращают режим и параметры вместе с ошибкой, пишут технический текст или длинное тире (NFR-11).
func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"два режима", []string{"--service", "--guard"}, "Нельзя указать два режима запуска сразу: --service и --guard"},
		{"два режима через параметр", []string{"--tray", "--session=1", "--wizard"}, "Нельзя указать два режима запуска сразу: --tray и --wizard"},
		{"режим дважды", []string{"--service", "--service"}, "Ключ --service указан дважды"},
		{"неизвестный ключ", []string{"--x"}, "Неизвестный ключ запуска: --x"},
		{"неизвестный ключ со значением", []string{"--x=5"}, "Неизвестный ключ запуска: --x"},
		{"верхний регистр", []string{"--Service"}, "Неизвестный ключ запуска: --Service"},
		{"верхний регистр параметра", []string{"--test-ipc", "--Out=a"}, "Неизвестный ключ запуска: --Out"},
		{"у окна нет ключа", []string{"--window"}, "Неизвестный ключ запуска: --window"},
		{"один дефис", []string{"-service"}, "Неизвестный ключ запуска: -service"},
		{"голые дефисы", []string{"--"}, "Неизвестный ключ запуска: --"},
		{"пустое имя ключа", []string{"--=1"}, "Неизвестный ключ запуска: --"},
		{"ключ с пробелом", []string{"--service "}, "Неизвестный ключ запуска: --service "},
		{"позиционный аргумент", []string{"foo"}, "Лишний аргумент запуска: foo"},
		{"позиционный после режима", []string{"--service", "foo"}, "Лишний аргумент запуска: foo"},
		{"путь вместо ключа", []string{"C:\\Temp\\r.json"}, "Лишний аргумент запуска: C:\\Temp\\r.json"},
		{"значение отдельным аргументом", []string{"--test-ipc", "--out", "r.json"}, "У ключа --out нет значения: напишите --out=значение"},
		{"слеш вместо дефисов", []string{"/service"}, "Лишний аргумент запуска: /service"},
		{"пустой аргумент", []string{""}, "Лишний аргумент запуска: (пустой)"},
		{"значение там, где оно нужно", []string{"--test-ipc", "--payload"}, "У ключа --payload нет значения: напишите --payload=значение"},
		{"значение у параметра пароля", []string{"--test-setup", "--password"}, "У ключа --password нет значения: напишите --password=значение"},
		{"значение у режима", []string{"--service=1"}, "У ключа --service не бывает значения"},
		{"параметр дважды", []string{"--test-setup", "--out=a", "--out=b"}, "Ключ --out указан дважды"},
		{"параметр без режима", []string{"--out=x"}, "Параметр --out нельзя указать без ключа режима запуска"},
		{"флаг без режима", []string{"--silent"}, "Параметр --silent нельзя указать без ключа режима запуска"},
		{"параметр чужому режиму", []string{"--guard", "--out=x"}, "Параметр --out не подходит к режиму --guard"},
		{"пароль службе", []string{"--service", "--password=x"}, "Параметр --password не подходит к режиму --service"},
		{"JSON не тому режиму", []string{"--test-setup", "--payload={}"}, "Параметр --payload не подходит к режиму --test-setup"},
		{"первая ошибка по порядку", []string{"--bogus", "--service", "--guard"}, "Неизвестный ключ запуска: --bogus"},
		{"неверное значение флага", []string{"--install-service", "--silent=maybe"}, "У ключа --silent допустимы только значения 0 и 1"},
	}
	for _, tc := range cases {
		mode, opts, err := Parse(tc.args)
		if err == nil {
			t.Errorf("%s: ожидалась ошибка %q, получено %v %#v", tc.name, tc.want, mode, opts)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("%s: текст ошибки %q, ожидалось %q", tc.name, err.Error(), tc.want)
		}
		if mode != ModeWindow || opts != nil {
			t.Errorf("%s: вместе с ошибкой возвращено %v и %#v, ожидалось окно и nil", tc.name, mode, opts)
		}
		if strings.ContainsAny(err.Error(), "—–") {
			t.Errorf("%s: в тексте ошибки длинное тире: %q", tc.name, err.Error())
		}
	}
}

// Ловит: запрет «два режима» не срабатывает, когда после первого режима идут параметры,
// а ошибка про параметр не того режима перекрывает ошибку про два режима.
func TestParseTwoModesBeatBadParameter(t *testing.T) {
	_, _, err := Parse([]string{"--out=x", "--service", "--guard"})
	if err == nil || err.Error() != "Нельзя указать два режима запуска сразу: --service и --guard" {
		t.Errorf("ошибка %v, ожидалось про два режима", err)
	}
}

// Ловит: параметр Options.Flag читает значение не так: только «1» считается включённым;
// nil-карта не должна падать.
func TestOptionsFlag(t *testing.T) {
	o := Options{"silent": "1", "delete-data": "0", "out": "1x"}
	cases := []struct {
		key  string
		want bool
	}{{"silent", true}, {"delete-data", false}, {"out", false}, {"нет такого", false}}
	for _, tc := range cases {
		if got := o.Flag(tc.key); got != tc.want {
			t.Errorf("Flag(%q) = %v, ожидалось %v", tc.key, got, tc.want)
		}
	}
	var nilOpts Options
	if nilOpts.Flag("silent") {
		t.Error("Flag на nil-карте вернул true")
	}
}

// Ловит: разбор зависит от случая или не повторяется (L-37): один и тот же ввод обязан
// давать тот же результат, а результат не должен делить карту с прошлым вызовом.
func TestParseIsDeterministicAndIndependent(t *testing.T) {
	args := []string{"--test-ipc", `--payload={"cmd":"Hello"}`, "--out=r.json"}
	_, first, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	first["payload"] = "испорчено"
	_, second, err := Parse(args)
	if err != nil || second["payload"] != `{"cmd":"Hello"}` {
		t.Errorf("второй вызов вернул %#v, %v: карта общая или результат зависит от прошлого вызова", second, err)
	}
	args[1] = "--payload=другое"
	if second["payload"] != `{"cmd":"Hello"}` {
		t.Error("карта параметров держит ссылку на исходный срез аргументов")
	}
}

// FuzzParse: произвольный набор аргументов (разделитель NUL) не роняет разбор; при успехе
// режим и ключи известны, при ошибке возвращается окно и nil; повторный вызов даёт то же.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"", "--service", "--guard\x00--session=1", "--test-ipc\x00--payload={\"cmd\":\"Hello\"}\x00--out=r.json",
		"--x", "foo", "--", "--=", "-service", "--Service", "--silent=0", "--remove-service\x00--silent\x00--delete-data",
		"--out", "--out=a\x00--out=b", "\x00\x00", "--parent\x00--handover=" + strings.Repeat("a", 300),
		"--service\x00--guard", "--password===", "\xff\xfe", "--test-runas\x00--user=u\x00--password=p\x00--exe=x y",
	} {
		f.Add(seed)
	}
	known := map[string]bool{}
	for _, k := range append(append([]string{}, flagParams...), valueParams...) {
		known[k] = true
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var args []string
		if raw != "" {
			args = strings.Split(raw, "\x00")
		}
		mode, opts, err := Parse(args)
		mode2, opts2, err2 := Parse(args)
		if mode != mode2 || !reflect.DeepEqual(opts, opts2) || (err == nil) != (err2 == nil) {
			t.Fatalf("Parse(%q) не повторяется", args)
		}
		if err != nil {
			if mode != ModeWindow || opts != nil || err.Error() == "" {
				t.Fatalf("Parse(%q): при ошибке %v, %#v, %q", args, mode, opts, err)
			}
			return
		}
		if mode < ModeWindow || mode > ModeTestRunAs || mode.String() == "unknown" {
			t.Fatalf("Parse(%q): режим %d вне списка", args, int(mode))
		}
		var keys []string
		for k := range opts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !known[k] {
				t.Fatalf("Parse(%q): неизвестный ключ %q в результате", args, k)
			}
			ok := false
			for _, fl := range paramModes[k] {
				if fl == mode.Flag() {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("Parse(%q): параметр %s принят для режима %s", args, k, mode)
			}
		}
	})
}
