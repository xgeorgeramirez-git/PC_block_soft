// Package launch разбирает ключи запуска программы: в каком режиме работает PCBlock.exe.
//
// Программа одна (Ruling 2 плана M1): служба, страж, значок, окна и служебные команды для
// приёмки отличаются только ключом запуска. Пакет на стандартной библиотеке: разбор
// детерминированный, мусор в аргументах даёт человеческую ошибку, а не панику (L-37).
// Значения параметров пишутся одним аргументом, --key=value; через пробел не принимаются.
package launch

import (
	"fmt"
	"strings"
)

// Mode — режим запуска программы. Нулевое значение (ModeWindow) самое безопасное:
// обычное окно, ничего не блокирует и не меняет.
type Mode int

const (
	// ModeWindow — без ключа: окно (мастер, если программа не настроена, иначе вход родителя).
	ModeWindow Mode = iota
	// ModeService — служба, так её запускает диспетчер служб.
	ModeService
	// ModeInstallService — установить и запустить службу.
	ModeInstallService
	// ModeRemoveService — остановить и удалить службу.
	ModeRemoveService
	// ModeGuard — страж сеанса ребёнка.
	ModeGuard
	// ModeTray — значок у часов.
	ModeTray
	// ModeParent — окно «Настройки для родителя».
	ModeParent
	// ModeMyStats — окно «Моя статистика».
	ModeMyStats
	// ModeFirstRun — окно первого входа ребёнка.
	ModeFirstRun
	// ModeWizard — мастер первой настройки.
	ModeWizard
	// ModeResetPassword — сброс пароля родителя от администратора.
	ModeResetPassword
	// ModeUninstallGate — пароль родителя перед удалением.
	ModeUninstallGate
	// ModeTestSetup — служебная первоначальная настройка для приёмки в CI (тестовый режим).
	ModeTestSetup
	// ModeTestIPC — служебный вызов канала связи для приёмки в CI (тестовый режим).
	ModeTestIPC
	// ModeTestRunAs — запуск программы от имени другой учётки для приёмки в CI (тестовый режим).
	ModeTestRunAs
)

// modeInfos — имя режима (оно же ключ запуска с двумя дефисами и значение App.Mode()
// во фронте) и признак «открывает окно Wails»; индекс — номер режима.
var modeInfos = [...]struct {
	name   string
	window bool
}{
	ModeWindow:         {"window", true},
	ModeService:        {"service", false},
	ModeInstallService: {"install-service", false},
	ModeRemoveService:  {"remove-service", false},
	ModeGuard:          {"guard", false},
	ModeTray:           {"tray", false},
	ModeParent:         {"parent", true},
	ModeMyStats:        {"mystats", true},
	ModeFirstRun:       {"firstrun", true},
	ModeWizard:         {"wizard", true},
	ModeResetPassword:  {"reset-password", true},
	ModeUninstallGate:  {"uninstall-gate", true},
	ModeTestSetup:      {"test-setup", false},
	ModeTestIPC:        {"test-ipc", false},
	ModeTestRunAs:      {"test-runas", false},
}

func (m Mode) valid() bool { return m >= 0 && int(m) < len(modeInfos) }

// String — имя режима для App.Mode() во фронте: window, service, install-service и так далее;
// неизвестное число даёт «unknown».
func (m Mode) String() string {
	if !m.valid() {
		return "unknown"
	}
	return modeInfos[m].name
}

// Flag — ключ запуска режима для командной строки («--service»); у окна без ключа
// и у неизвестного режима пусто.
func (m Mode) Flag() string {
	if !m.valid() || m == ModeWindow {
		return ""
	}
	return "--" + modeInfos[m].name
}

// IsWindow — режим открывает окно Wails: window, parent, mystats, firstrun, wizard,
// reset-password, uninstall-gate.
func (m Mode) IsWindow() bool { return m.valid() && modeInfos[m].window }

// Options — параметры запуска: ключ без дефисов и значение. Ключ без значения из
// списка флагов (silent, delete-data, elevated) хранится как «1».
type Options map[string]string

// Flag — параметр-флаг включён: значение «1». Нет параметра, «0» и nil-карта дают false.
func (o Options) Flag(key string) bool { return o[key] == "1" }

// paramInfo — описание параметра: флаг без значения или с ним (--key=value), и режимы,
// которым он положен. Список режимов выведен из плана M1 (Ruling 2, 23, S25, T9, T10, T16);
// новому ключу достаточно добавить строку здесь и в таблицу теста.
type paramInfo struct {
	flag  bool
	modes []Mode
}

var params = map[string]paramInfo{
	// Файл для результата служебного ключа (Ruling 23: у программы нет стандартного вывода).
	"out": {modes: []Mode{ModeInstallService, ModeRemoveService, ModeResetPassword, ModeTestSetup, ModeTestIPC, ModeTestRunAs}},
	// Номер сеанса Windows, в котором работает страж или значок (мьютекс на сеанс).
	"session": {modes: []Mode{ModeGuard, ModeTray}},
	// Токен родительской сессии, переданный окну настроек после ввода пароля (R19).
	"handover": {modes: []Mode{ModeParent}},
	// Пароль родителя без окна: только тестовый режим и приёмка в CI.
	"password": {modes: []Mode{ModeTestSetup, ModeUninstallGate, ModeTestRunAs}},
	// Новый пароль без окна: только тестовый режим (S25).
	"new-password": {modes: []Mode{ModeResetPassword}},
	// Тихо, без окон и вопросов (установщик).
	"silent": {flag: true, modes: []Mode{ModeInstallService, ModeRemoveService, ModeUninstallGate}},
	// Удалить данные программы вместе с удалением.
	"delete-data": {flag: true, modes: []Mode{ModeRemoveService, ModeUninstallGate}},
	// Программа уже перезапущена с повышением прав (окно UAC показано): второй раз не просить.
	// Без метки неудавшаяся проверка повышения вызывала бы окно UAC снова и снова.
	"elevated": {flag: true, modes: []Mode{ModeResetPassword}},
	// Запрос канала связи в JSON для --test-ipc.
	"payload": {modes: []Mode{ModeTestIPC}},
	// Учётка Windows, к которой относится служебная команда.
	"user": {modes: []Mode{ModeTestSetup, ModeTestRunAs}},
	// Программа и её ключи, которые --test-runas запускает от имени другой учётки.
	"exe": {modes: []Mode{ModeTestRunAs}},
	// Экран, на котором открывается окно родителя (RM3-4: «Попросить сейчас» → «Программы»);
	// значение только из Screens.
	"screen": {modes: []Mode{ModeParent}},
}

// Screens — экраны окна родителя для --screen (те же имена, что ipc.Screen*). Другое значение
// ключа — ошибка разбора: окно не открывает экран, которого не знает.
var Screens = []string{"programs"}

// validScreen: значение --screen из списка Screens.
func validScreen(v string) bool {
	for _, s := range Screens {
		if s == v {
			return true
		}
	}
	return false
}

// ScreenArg — ключ запуска окна родителя на экране screen («--screen=programs»); пусто для
// пустого и неизвестного экрана (окно откроется на главном).
func ScreenArg(screen string) string {
	if !validScreen(screen) {
		return ""
	}
	return "--screen=" + screen
}

func (p paramInfo) allowedFor(m Mode) bool {
	for _, allowed := range p.modes {
		if allowed == m {
			return true
		}
	}
	return false
}

// modeByKey ищет режим по ключу без дефисов; «window» ключа не имеет.
func modeByKey(key string) (Mode, bool) {
	for m := ModeService; int(m) < len(modeInfos); m++ {
		if modeInfos[m].name == key {
			return m, true
		}
	}
	return ModeWindow, false
}

// Parse разбирает аргументы командной строки (без имени программы, os.Args[1:]).
// Ровно один ключ режима или ни одного; параметры пишутся --key=value, значение может
// содержать «=» и пробелы, пустое допустимо; --silent и --delete-data без значения дают «1».
// Регистр ключей строгий (нижний). Без аргументов результат ModeWindow без ошибки: так
// программу запускает генератор биндингов Wails (Ruling 22).
//
// Ошибка возвращается с человеческим русским текстом, вместе с ней режим ModeWindow и
// nil-параметры; первая по порядку аргументов ошибка перекрывает остальные, а проверка
// «параметр подходит режиму» идёт после просмотра всех аргументов (параметр может стоять
// до режима).
func Parse(args []string) (Mode, Options, error) {
	mode := ModeWindow
	modeFlag := ""
	opts := Options{}
	var order []string // параметры в порядке появления: ошибки не зависят от порядка обхода карты

	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			if arg == "" {
				arg = "(пустой)"
			}
			return fail("Лишний аргумент запуска: %s", arg)
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if !strings.HasPrefix(name, "--") {
			return fail("Неизвестный ключ запуска: %s", name)
		}
		key := name[2:]

		if m, ok := modeByKey(key); ok {
			switch {
			case hasValue:
				return fail("У ключа %s не бывает значения", name)
			case modeFlag == name:
				return fail("Ключ %s указан дважды", name)
			case modeFlag != "":
				return fail("Нельзя указать два режима запуска сразу: %s и %s", modeFlag, name)
			}
			mode, modeFlag = m, name
			continue
		}

		p, ok := params[key]
		if !ok {
			return fail("Неизвестный ключ запуска: %s", name)
		}
		switch {
		case p.flag && !hasValue:
			value = "1"
		case p.flag && value != "0" && value != "1":
			return fail("У ключа %s допустимы только значения 0 и 1", name)
		case !p.flag && !hasValue:
			return fail("У ключа %s нет значения: напишите %s=значение", name, name)
		}
		if _, dup := opts[key]; dup {
			return fail("Ключ %s указан дважды", name)
		}
		if key == "screen" && !validScreen(value) {
			return fail("Неизвестный ключ запуска: %s", arg)
		}
		opts[key] = value
		order = append(order, key)
	}

	for _, key := range order {
		if params[key].allowedFor(mode) {
			continue
		}
		if mode == ModeWindow {
			return fail("Параметр --%s нельзя указать без ключа режима запуска", key)
		}
		return fail("Параметр --%s не подходит к режиму %s", key, modeFlag)
	}
	return mode, opts, nil
}

// fail собирает ошибку разбора: режим окна и nil-параметры, чтобы случайно проигнорированная
// ошибка не запустила ничего, кроме обычного окна.
func fail(format string, a ...any) (Mode, Options, error) {
	return ModeWindow, nil, fmt.Errorf(format, a...)
}
