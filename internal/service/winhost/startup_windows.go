//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"pcblock/internal/service"
	"pcblock/internal/startup"
)

// Автозагрузка учёток детей (service.StartupSource, FR-39, RM5b-1, RM5b-2, RM5b-7). Чтение и
// запись — только штатные места Windows, как у Диспетчера задач: значения Run/RunOnce, папки
// «Автозагрузка», задания Планировщика с триггером «при входе» и приложения магазина.

const (
	runPath      = `Software\Microsoft\Windows\CurrentVersion\Run`
	runOncePath  = `Software\Microsoft\Windows\CurrentVersion\RunOnce`
	run32Path    = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`
	approvedPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\`
	// appDataPath — ключи «при входе» приложений магазина в ветке классов пользователя
	// (HKU\<SID>_Classes, уточнение 6): <семейство пакета>\<TaskId>, значение State.
	appDataPath = `Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\SystemAppData`
	stateValue  = "State"

	approvedRun    = "Run"
	approvedRun32  = "Run32"
	approvedFolder = "StartupFolder"

	// Значения State приложения магазина: 2 — включено, 4 — включено политикой; 1 — выключено
	// пользователем (как пишет Диспетчер задач), 0 — выключено по умолчанию, 3 — политикой.
	storeOff      = 1
	storeOn       = 2
	storeOnPolicy = 4
)

// userStartupDir и commonStartupDir — папки «Автозагрузка» в профиле и в ProgramData.
var (
	userStartupDir   = filepath.Join("AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	commonStartupDir = filepath.Join("Microsoft", "Windows", "Start Menu", "Programs", "StartUp")
)

// startupSource — service.StartupSource этого компьютера.
type startupSource struct {
	meta        service.FileMeta // издатель exe; nil — не читается
	windir      string           // «C:\Windows»: под ней — компоненты Windows (RM5b-7), System32\Tasks
	pcblockDir  string           // папка установки PC Block (RM5b-7)
	programData string           // «C:\ProgramData»: общая папка «Автозагрузка»
	now         func() time.Time // момент выключения в StartupApproved; nil — time.Now
}

var _ service.StartupSource = (*startupSource)(nil)

func newStartupSource(f machineFolders, meta service.FileMeta) *startupSource {
	return &startupSource{meta: meta, windir: f.windows, pcblockDir: f.installDir, programData: f.programData}
}

// List — записи автозагрузки ребёнка sid: свои и общие, без скрываемого (RM5b-7). Любая ошибка
// чтения — ошибка всего списка (PL-46: сбой датчика не равен «записи нет»). Профиля ещё нет —
// своих записей нет.
func (s *startupSource) List(sid string) ([]startup.Entry, error) {
	out := []startup.Entry{}
	own, err := s.ownEntries(sid)
	if err != nil {
		return nil, err
	}
	out = append(out, own...)
	shared, err := s.sharedEntries()
	if err != nil {
		return nil, err
	}
	out = append(out, shared...)
	tasks, err := s.taskEntries(sid)
	if err != nil {
		return nil, err
	}
	out = append(out, tasks...)

	visible := out[:0]
	for _, e := range out {
		if e.Exe != "" && s.meta != nil {
			e.Publisher, _, _ = s.meta(e.Exe)
		}
		if !startup.Hidden(e, s.windir, s.pcblockDir) {
			visible = append(visible, e)
		}
	}
	return visible, nil
}

// --- свои записи ---------------------------------------------------------------------------

func (s *startupSource) ownEntries(sid string) ([]startup.Entry, error) {
	dir, hasProfile, err := profileDir(sid)
	if err != nil {
		return nil, err
	}
	env := userEnv(dir, hasProfile)
	out := []startup.Entry{}
	var folderState map[string][]byte

	hiveMu.Lock()
	defer hiveMu.Unlock()
	readUser := func(hive string) error {
		for _, src := range []struct{ path, source, approved string }{
			{runPath, startup.SrcRun, approvedRun},
			{runOncePath, startup.SrcRunOnce, ""},
		} {
			es, err := runEntries(registry.USERS, hive+`\`+src.path, hive+`\`+approvedPath+src.approved, src.approved != "", startup.ScopeOwn, src.source, env)
			if err != nil {
				return err
			}
			out = append(out, es...)
		}
		st, err := approvedValues(registry.USERS, hive+`\`+approvedPath+approvedFolder)
		folderState = st
		return err
	}
	if err := onUserHive(sid, readUser); err != nil {
		return nil, fmt.Errorf("автозагрузка %s: %w", sid, err)
	}
	if hasProfile {
		es, err := s.folderEntries(filepath.Join(dir, userStartupDir), startup.ScopeOwn, folderState, env)
		if err != nil {
			return nil, err
		}
		out = append(out, es...)
	}
	store := []startup.Entry{}
	readClasses := func(hive string) error {
		es, err := storeEntries(hive)
		store = es
		return err
	}
	if err := onClassesHive(sid, readClasses); err != nil {
		return nil, fmt.Errorf("приложения магазина %s: %w", sid, err)
	}
	return append(out, store...), nil
}

// onUserHive выполняет fn над веткой пользователя: загружена (вошёл) — HKU\<sid>, иначе — через
// профиль (withHive; профиля нет — fn не вызывается). Под hiveMu.
func onUserHive(sid string, fn func(hive string) error) error {
	loaded, err := hiveLoaded(sid)
	if err != nil {
		return err
	}
	if loaded {
		return fn(sid)
	}
	return withHive(sid, fn)
}

// onClassesHive — то же для ветки классов HKU\<sid>_Classes (UsrClass.dat). Под hiveMu.
func onClassesHive(sid string, fn func(hive string) error) error {
	loaded, err := hiveLoaded(sid + classesSuffix)
	if err != nil {
		return err
	}
	if loaded {
		return fn(sid + classesSuffix)
	}
	return withClassesHive(sid, fn)
}

// userEnv — переменные окружения ребёнка для раскрытия %VAR% в командах его записей: папки
// профиля — свои (служба работает от системы), остальное — переменные компьютера.
func userEnv(profile string, ok bool) func(string) string {
	return func(name string) string {
		if ok {
			local := filepath.Join(profile, "AppData", "Local")
			switch strings.ToUpper(name) {
			case "USERPROFILE":
				return profile
			case "APPDATA":
				return filepath.Join(profile, "AppData", "Roaming")
			case "LOCALAPPDATA":
				return local
			case "TEMP", "TMP":
				return filepath.Join(local, "Temp")
			}
		}
		return os.Getenv(name)
	}
}

// --- общие записи --------------------------------------------------------------------------

func (s *startupSource) sharedEntries() ([]startup.Entry, error) {
	out := []startup.Entry{}
	approved := `SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\`
	for _, src := range []struct{ path, source, approved string }{
		{`SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, startup.SrcRun, approvedRun},
		{`SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`, startup.SrcRunOnce, ""},
		{run32Path, startup.SrcRun32, approvedRun32},
	} {
		es, err := runEntries(registry.LOCAL_MACHINE, src.path, approved+src.approved, src.approved != "", startup.ScopeShared, src.source, os.Getenv)
		if err != nil {
			return nil, fmt.Errorf("общая автозагрузка: %w", err)
		}
		out = append(out, es...)
	}
	if s.programData == "" {
		return out, nil
	}
	st, err := approvedValues(registry.LOCAL_MACHINE, approved+approvedFolder)
	if err != nil {
		return nil, fmt.Errorf("общая автозагрузка: %w", err)
	}
	es, err := s.folderEntries(filepath.Join(s.programData, commonStartupDir), startup.ScopeShared, st, os.Getenv)
	if err != nil {
		return nil, err
	}
	return append(out, es...), nil
}

// --- реестр ---------------------------------------------------------------------------------

// runEntries читает строковые значения ключа path (Run или RunOnce); состояние — из значений
// StartupApproved по пути approved (withState; нет значения — включено). Ключа нет — записей нет.
func runEntries(root registry.Key, path, approved string, withState bool, scope, source string, env func(string) string) ([]startup.Entry, error) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return []startup.Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ключ %s: %w", path, err)
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, fmt.Errorf("ключ %s: %w", path, err)
	}
	state := map[string][]byte{}
	if withState {
		if state, err = approvedValues(root, approved); err != nil {
			return nil, err
		}
	}
	out := make([]startup.Entry, 0, len(names))
	for _, name := range names {
		cmd, _, err := k.GetStringValue(name) // REG_EXPAND_SZ раскрывает ExeOf
		if errors.Is(err, registry.ErrUnexpectedType) {
			continue // не строка: Windows такое значение не запускает
		}
		if err != nil {
			return nil, fmt.Errorf("значение %s\\%s: %w", path, name, err)
		}
		out = append(out, startup.Entry{
			Scope: scope, Source: source, Name: name, Command: cmd,
			Exe: startup.ExeOf(cmd, env), Enabled: approvedEnabled(state, name),
		})
	}
	return out, nil
}

// approvedValues — значения StartupApproved ключа path по именам; ключа нет — пусто.
func approvedValues(root registry.Key, path string) (map[string][]byte, error) {
	out := map[string][]byte{}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ключ %s: %w", path, err)
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, fmt.Errorf("ключ %s: %w", path, err)
	}
	for _, name := range names {
		b, _, err := k.GetBinaryValue(name)
		if errors.Is(err, registry.ErrUnexpectedType) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("значение %s\\%s: %w", path, name, err)
		}
		out[strings.ToLower(name)] = b
	}
	return out, nil
}

// approvedEnabled — запись включена: значения нет или оно не разобрано — включено (так считает
// Windows), иначе по первому байту.
func approvedEnabled(state map[string][]byte, name string) bool {
	b, ok := state[strings.ToLower(name)]
	if !ok {
		return true
	}
	on, known := startup.DecodeApproved(b)
	return on || !known
}

// --- папки «Автозагрузка» -------------------------------------------------------------------

// folderEntries — файлы папки dir (без вложенных папок и desktop.ini). Ярлык .lnk — цель и
// аргументы ярлыка (startup.LnkTarget); не разобран — команда и exe = путь ярлыка. Папки нет —
// записей нет.
func (s *startupSource) folderEntries(dir, scope string, state map[string][]byte, env func(string) string) ([]startup.Entry, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []startup.Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("папка %s: %w", dir, err)
	}
	out := make([]startup.Entry, 0, len(ents))
	for _, de := range ents {
		name := de.Name()
		if de.IsDir() || strings.EqualFold(name, "desktop.ini") {
			continue
		}
		path := filepath.Join(dir, name)
		cmd, exe := path, path
		if strings.EqualFold(filepath.Ext(name), ".lnk") {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("ярлык %s: %w", path, err)
			}
			if target, args, ok := startup.LnkTarget(data, dir); ok {
				exe = target
				if t, ok := expandOnly(target, env); ok {
					exe = t
				}
				cmd = quoteCmd(target, args)
			}
		}
		out = append(out, startup.Entry{
			Scope: scope, Source: startup.SrcFolder, Name: name, Command: cmd, Exe: exe,
			Enabled: approvedEnabled(state, name),
		})
	}
	return out, nil
}

// expandOnly раскрывает %VAR% в пути (цель ярлыка бывает с переменными).
func expandOnly(p string, env func(string) string) (string, bool) {
	if !strings.Contains(p, "%") {
		return p, true
	}
	e := startup.ExeOf(`"`+p+`"`, env)
	return e, e != ""
}

func quoteCmd(exe, args string) string {
	if strings.ContainsAny(exe, " \t") {
		exe = `"` + exe + `"`
	}
	if args != "" {
		return exe + " " + args
	}
	return exe
}

// --- задания Планировщика -------------------------------------------------------------------

// taskEntries — задания «при входе» ребёнка и общие (startup.TaskScope) из файлов
// %WINDIR%\System32\Tasks. Папка \Microsoft пропускается целиком: её задания скрываются
// (RM5b-7). Файл не читается — ошибка; не разбирается — пропускается (это не задание).
func (s *startupSource) taskEntries(sid string) ([]startup.Entry, error) {
	out := []startup.Entry{}
	if s.windir == "" {
		return out, nil
	}
	root := filepath.Join(s.windir, "System32", "Tasks")
	account := accountOf(sid)
	dir, hasProfile, err := profileDir(sid)
	if err != nil {
		return nil, err
	}
	ownEnv := userEnv(dir, hasProfile)
	err = filepath.WalkDir(root, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			if path == root && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		if de.IsDir() {
			if strings.EqualFold(rel, "Microsoft") {
				return fs.SkipDir
			}
			return nil
		}
		if !de.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		t, perr := startup.ParseTaskXML(data)
		if perr != nil {
			return nil
		}
		scope, ok := startup.TaskScope(t, sid, account)
		if !ok {
			return nil
		}
		env := os.Getenv
		if scope == startup.ScopeOwn {
			env = ownEnv
		}
		tp := `\` + rel // путь задания — по месту файла: его понимает schtasks /TN
		out = append(out, startup.Entry{
			Scope: scope, Source: startup.SrcTask, Name: tp, Task: tp, Command: t.Command,
			Exe: startup.ExeOf(t.Command, env), Enabled: t.Enabled,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("задания Планировщика: %w", err)
	}
	return out, nil
}

// accountOf — «ДОМЕН\имя» учётки sid; не нашлась — "" (задания тогда сверяются только по SID).
func accountOf(sid string) string {
	s, err := windows.StringToSid(sid)
	if err != nil {
		return ""
	}
	name, domain, _, err := s.LookupAccount("")
	if err != nil || name == "" {
		return ""
	}
	if domain == "" {
		return name
	}
	return domain + `\` + name
}

// --- приложения магазина --------------------------------------------------------------------

// storeEntries — ключи «при входе» приложений магазина в ветке классов hive: <семейство>\<TaskId>
// со значением State (нет State — не показывается, уточнение 6). Ключа нет — записей нет.
func storeEntries(hive string) ([]startup.Entry, error) {
	out := []startup.Entry{}
	base := hive + `\` + appDataPath
	k, err := registry.OpenKey(registry.USERS, base, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ключ %s: %w", base, err)
	}
	pkgs, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return nil, fmt.Errorf("ключ %s: %w", base, err)
	}
	for _, pkg := range pkgs {
		pk, err := registry.OpenKey(registry.USERS, base+`\`+pkg, registry.ENUMERATE_SUB_KEYS)
		if errors.Is(err, registry.ErrNotExist) {
			continue // пакет удалили между чтениями
		}
		if err != nil {
			return nil, fmt.Errorf("пакет %s: %w", pkg, err)
		}
		tasks, err := pk.ReadSubKeyNames(-1)
		pk.Close()
		if err != nil {
			return nil, fmt.Errorf("пакет %s: %w", pkg, err)
		}
		for _, task := range tasks {
			tk, err := registry.OpenKey(registry.USERS, base+`\`+pkg+`\`+task, registry.QUERY_VALUE)
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("пакет %s\\%s: %w", pkg, task, err)
			}
			st, _, err := tk.GetIntegerValue(stateValue)
			tk.Close()
			if errors.Is(err, registry.ErrNotExist) || errors.Is(err, registry.ErrUnexpectedType) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("пакет %s\\%s: %w", pkg, task, err)
			}
			out = append(out, startup.Entry{
				Scope: startup.ScopeOwn, Source: startup.SrcStore, Name: pkg + `!` + task,
				Command: pkg + `!` + task, Package: pkg, Task: task, Enabled: st == storeOn || st == storeOnPolicy,
			})
		}
	}
	return out, nil
}

// --- включение и выключение -----------------------------------------------------------------

// SetEnabled штатно включает или выключает свою запись (RM5b-2): Run и папка — StartupApproved,
// задание — schtasks /Change, приложение магазина — State. Общая — service.ErrSharedNotWritable,
// RunOnce — service.ErrNotSwitchable. Профиля нет — ошибка: записать некуда.
func (s *startupSource) SetEnabled(sid string, e startup.Entry, on bool) error {
	if e.Scope != startup.ScopeOwn {
		return service.ErrSharedNotWritable
	}
	switch e.Source {
	case startup.SrcRunOnce:
		return service.ErrNotSwitchable
	case startup.SrcRun, startup.SrcFolder:
		sub := approvedRun
		if e.Source == startup.SrcFolder {
			sub = approvedFolder
		}
		now := time.Now
		if s.now != nil {
			now = s.now
		}
		val := startup.EncodeApproved(on, now())
		return s.onHive(sid, onUserHive, func(hive string) error {
			k, _, err := registry.CreateKey(registry.USERS, hive+`\`+approvedPath+sub, registry.SET_VALUE)
			if err != nil {
				return err
			}
			defer k.Close()
			return k.SetBinaryValue(e.Name, val)
		})
	case startup.SrcTask:
		if e.Task == "" {
			return errors.New("автозагрузка: у задания нет пути")
		}
		return s.schtasks(e.Task, on)
	case startup.SrcStore:
		if e.Package == "" || e.Task == "" {
			return errors.New("автозагрузка: у приложения магазина нет пакета")
		}
		st := uint32(storeOff)
		if on {
			st = storeOn
		}
		return s.onHive(sid, onClassesHive, func(hive string) error {
			k, err := registry.OpenKey(registry.USERS, hive+`\`+appDataPath+`\`+e.Package+`\`+e.Task, registry.SET_VALUE)
			if err != nil {
				return err
			}
			defer k.Close()
			return k.SetDWordValue(stateValue, st)
		})
	}
	return fmt.Errorf("автозагрузка: неизвестный источник %q", e.Source)
}

// onHive выполняет запись fn над веткой ребёнка через on (onUserHive или onClassesHive) под
// hiveMu; профиля нет (fn не вызвана) — ошибка.
func (s *startupSource) onHive(sid string, on func(string, func(string) error) error, fn func(string) error) error {
	hiveMu.Lock()
	defer hiveMu.Unlock()
	called := false
	err := on(sid, func(hive string) error {
		called = true
		return fn(hive)
	})
	if err != nil {
		return fmt.Errorf("автозагрузка %s не записана: %w", sid, err)
	}
	if !called {
		return fmt.Errorf("автозагрузка %s не записана: профиля нет", sid)
	}
	return nil
}

// schtasks включает или выключает задание штатно: schtasks.exe /Change /TN <путь> /ENABLE|/DISABLE,
// без окна; код выхода не 0 — ошибка с выводом.
func (s *startupSource) schtasks(task string, on bool) error {
	flag := "/DISABLE"
	if on {
		flag = "/ENABLE"
	}
	exe := "schtasks.exe"
	if s.windir != "" {
		exe = filepath.Join(s.windir, "System32", "schtasks.exe")
	}
	cmd := exec.Command(exe, "/Change", "/TN", task, flag)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s %s: %w: %s", flag, task, err, strings.TrimSpace(string(out)))
	}
	return nil
}
