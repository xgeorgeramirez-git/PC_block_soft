//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"pcblock/internal/service"
)

const (
	policyPath        = `Software\Microsoft\Windows\CurrentVersion\Policies\System`
	policyValue       = "DisableTaskMgr"
	profileListPath   = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`
	profileImageValue = "ProfileImagePath"
	tempHivePrefix    = "PCBlockTmp-"
	// bravePolicyPath — ветка правил Brave в разделе пользователя (R15, FR-31, RM4-10): Brave
	// применяет пользовательские политики и на компьютере без домена (проба T0, RM4-10-итог).
	bravePolicyPath = `Software\Policies\BraveSoftware\Brave`
	policiesPath    = `Software\Policies`
)

// errHiveNotLoaded — ветки пользователя нет в HKEY_USERS: он не вошёл.
var errHiveNotLoaded = errors.New("ветка пользователя не загружена")

var (
	advapi32          = windows.NewLazySystemDLL("advapi32.dll")
	procRegLoadKeyW   = advapi32.NewProc("RegLoadKeyW")
	procRegUnLoadKeyW = advapi32.NewProc("RegUnLoadKeyW")
)

// policy ставит и снимает запрет Диспетчера задач в учётке ребёнка (service.Policy, FR-29,
// Ruling 20): стандартная политика Windows DisableTaskMgr в ветке реестра пользователя; и правила
// Brave (RM4-10) в ветке bravePolicyPath того же раздела.
// Пользователь вошёл: ветка уже загружена в HKEY_USERS. Не вошёл: ветка профиля загружается
// на время записи (RegLoadKey). Профиля ещё нет: запись откладывается до первого входа
// (служба повторяет её при появлении сеанса ребёнка) и ошибкой не считается.
type policy struct {
	mu   sync.Mutex           // загрузка ветки профиля идёт по одной
	base string               // ветка правил Brave в разделе пользователя; "" — bravePolicyPath (тест подставляет свою)
	logf func(string, ...any) // журнал работы; nil — не писать
}

func (p *policy) braveBase() string {
	if p.base == "" {
		return bravePolicyPath
	}
	return p.base
}

func (p *policy) log(format string, args ...any) {
	if p.logf != nil {
		p.logf(format, args...)
	}
}

// SetTaskMgrDisabled включает или снимает запрет Диспетчера задач.
func (p *policy) SetTaskMgrDisabled(sid string, disabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := writePolicy(sid, disabled); err == nil {
		return nil
	} else if !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("политика Диспетчера задач для %s: %w", sid, err)
	}
	// Ветка пользователя не загружена.
	return p.withHive(sid, func(hive string) error {
		if err := writePolicy(hive, disabled); err != nil {
			return fmt.Errorf("политика Диспетчера задач для %s: %w", sid, err)
		}
		return nil
	})
}

// writePolicy пишет значение в загруженную ветку HKEY_USERS\<SID>. Ветки нет: registry.ErrNotExist.
func writePolicy(hiveName string, disabled bool) error {
	root, err := registry.OpenKey(registry.USERS, hiveName, registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	root.Close()
	if !disabled {
		k, err := registry.OpenKey(registry.USERS, hiveName+`\`+policyPath, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil // ключа политики нет: снимать нечего
		}
		if err != nil {
			return err
		}
		defer k.Close()
		if err := k.DeleteValue(policyValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	k, _, err := registry.CreateKey(registry.USERS, hiveName+`\`+policyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue(policyValue, 1)
}

// withHive подгружает ветку профиля пользователя (NTUSER.DAT) под временным именем, выполняет fn с
// этим именем и выгружает ветку. Профиля ещё нет — nil: запись при первом входе. Под p.mu.
func (p *policy) withHive(sid string, fn func(hive string) error) error {
	pk, err := registry.OpenKey(registry.LOCAL_MACHINE, profileListPath+sid, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return nil // профиля ещё нет: запись при первом входе
	}
	if err != nil {
		return fmt.Errorf("профиль %s не найден: %w", sid, err)
	}
	raw, _, err := pk.GetStringValue(profileImageValue)
	pk.Close()
	if err != nil {
		return fmt.Errorf("папка профиля %s не прочитана: %w", sid, err)
	}
	dir, err := registry.ExpandString(raw)
	if err != nil {
		return fmt.Errorf("папка профиля %s: %w", sid, err)
	}
	dat := filepath.Join(dir, "NTUSER.DAT")
	if _, err := os.Stat(dat); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("файл профиля %s: %w", dat, err)
	}

	// Права на загрузку и выгрузку веток: у системы они есть, но выключены.
	_ = enablePrivilege("SeBackupPrivilege")
	_ = enablePrivilege("SeRestorePrivilege")

	temp := tempHivePrefix + sid
	if err := loadHive(temp, dat); err != nil {
		return fmt.Errorf("ветка профиля %s не загружена: %w", sid, err)
	}
	werr := fn(temp)
	uerr := unloadHive(temp)
	if werr != nil {
		return werr
	}
	if uerr != nil {
		return fmt.Errorf("ветка профиля %s не выгружена: %w", sid, uerr)
	}
	return nil
}

func loadHive(name, file string) error {
	np, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	fp, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	r, _, _ := procRegLoadKeyW.Call(uintptr(registry.USERS), uintptr(unsafe.Pointer(np)), uintptr(unsafe.Pointer(fp)))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func unloadHive(name string) error {
	np, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, _ := procRegUnLoadKeyW.Call(uintptr(registry.USERS), uintptr(unsafe.Pointer(np)))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// --- правила Brave (RM4-10, T5) -------------------------------------------------------------

// SetBraveRules приводит ветку правил Brave учётки к r планом service.BravePlan: недостающее
// дописывается, лишнее удаляется, запреты не снимаются ни на миг (T5-3). Ветка не загружена —
// через профиль; профиля нет — nil.
func (p *policy) SetBraveRules(sid string, r service.BraveRules) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	want := r.Values()
	err := p.setBrave(sid, want)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errHiveNotLoaded) {
		return fmt.Errorf("правила Brave для %s: %w", sid, err)
	}
	return p.withHive(sid, func(hive string) error {
		if err := p.setBrave(hive, want); err != nil {
			return fmt.Errorf("правила Brave для %s: %w", sid, err)
		}
		return nil
	})
}

// BraveRulesOK — ветка правил Brave учётки совпадает с r. Ветка пользователя не загружена (не
// вошёл) — (true, nil): профиль ради сверки не подгружается, иначе вход ребёнка в эту секунду
// сломался бы (T5-4).
func (p *policy) BraveRulesOK(sid string, r service.BraveRules) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	loaded, err := hiveLoaded(sid)
	if err != nil {
		return false, fmt.Errorf("правила Brave для %s: %w", sid, err)
	}
	if !loaded {
		return true, nil
	}
	have, _, err := readBrave(registry.USERS, sid+`\`+p.braveBase())
	if err != nil {
		return false, fmt.Errorf("правила Brave для %s не прочитаны: %w", sid, err)
	}
	set, del, delKeys := service.BravePlan(have, r.Values())
	return len(set)+len(del)+len(delKeys) == 0, nil
}

// RemoveBraveRules снимает ветку правил Brave учётки целиком и пустую ветку над ней. Ветки нет —
// nil; ветка пользователя не загружена — через профиль.
func (p *policy) RemoveBraveRules(sid string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.removeBrave(sid)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errHiveNotLoaded) {
		return fmt.Errorf("правила Brave для %s не сняты: %w", sid, err)
	}
	return p.withHive(sid, func(hive string) error {
		if err := p.removeBrave(hive); err != nil {
			return fmt.Errorf("правила Brave для %s не сняты: %w", sid, err)
		}
		return nil
	})
}

// hiveLoaded — ветка hive есть в HKEY_USERS.
func hiveLoaded(hive string) (bool, error) {
	k, err := registry.OpenKey(registry.USERS, hive, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	k.Close()
	return true, nil
}

// setBrave приводит ветку правил Brave в загруженной ветке hive к want.
func (p *policy) setBrave(hive string, want []service.BraveValue) error {
	if loaded, err := hiveLoaded(hive); err != nil {
		return err
	} else if !loaded {
		return errHiveNotLoaded
	}
	base := p.braveBase()
	path := hive + `\` + base
	have, _, err := readBrave(registry.USERS, path)
	if err != nil {
		return err
	}
	set, del, delKeys := service.BravePlan(have, want)
	if len(set)+len(del)+len(delKeys) == 0 {
		return nil
	}
	// Своих прав ветке не ставим: она наследует от Software\Policies «пользователь только читает»
	// (ТЗ 3.4, T5-9). Нет Software\Policies — CreateKey создаст его с правами Software.
	if strings.HasPrefix(strings.ToLower(base), strings.ToLower(policiesPath)+`\`) {
		if k, err := registry.OpenKey(registry.USERS, hive+`\`+policiesPath, registry.QUERY_VALUE); errors.Is(err, registry.ErrNotExist) {
			p.log("в ветке %s не было Software\\Policies", hive)
		} else if err == nil {
			k.Close()
		}
	}
	return applyBravePlan(registry.USERS, path, set, del, delKeys)
}

// removeBrave удаляет ветку правил Brave в загруженной ветке hive и пустую ветку над ней (её
// создала программа; с другими подключами или значениями она остаётся).
func (p *policy) removeBrave(hive string) error {
	if loaded, err := hiveLoaded(hive); err != nil {
		return err
	} else if !loaded {
		return errHiveNotLoaded
	}
	base := p.braveBase()
	if err := deleteTree(registry.USERS, hive+`\`+base); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	i := strings.LastIndex(base, `\`)
	if i <= 0 {
		return nil
	}
	parent := hive + `\` + base[:i]
	k, err := registry.OpenKey(registry.USERS, parent, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	st, err := k.Stat()
	k.Close()
	if err != nil {
		return err
	}
	if st.SubKeyCount == 0 && st.ValueCount == 0 {
		if err := registry.DeleteKey(registry.USERS, parent); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	return nil
}

// readBrave читает ветку path в значения плана: свои значения и значения подключей первого уровня;
// подключ без значений и подключ второго уровня — значением с пустым Name (план удалит лишние).
// Ветки нет — (nil, false, nil).
func readBrave(root registry.Key, path string) ([]service.BraveValue, bool, error) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer k.Close()
	out, err := readValues(k, "")
	if err != nil {
		return nil, true, err
	}
	subs, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, true, err
	}
	for _, sub := range subs {
		sk, err := registry.OpenKey(root, path+`\`+sub, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			return nil, true, fmt.Errorf("подключ %s: %w", sub, err)
		}
		vals, verr := readValues(sk, sub)
		deep, derr := sk.ReadSubKeyNames(-1)
		sk.Close()
		if verr != nil {
			return nil, true, verr
		}
		if derr != nil {
			return nil, true, derr
		}
		if len(vals) == 0 {
			vals = []service.BraveValue{{Key: sub}}
		}
		out = append(out, vals...)
		for _, d := range deep {
			out = append(out, service.BraveValue{Key: sub + `\` + d})
		}
	}
	return out, true, nil
}

// readValues читает значения ключа k: REG_DWORD — числом, REG_SZ и REG_EXPAND_SZ — строкой, иной
// тип — строкой с пустым Str (план заменит или удалит его).
func readValues(k registry.Key, key string) ([]service.BraveValue, error) {
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	out := make([]service.BraveValue, 0, len(names))
	for _, name := range names {
		_, typ, err := k.GetValue(name, nil)
		if err != nil && !errors.Is(err, registry.ErrShortBuffer) {
			return nil, fmt.Errorf("значение %s: %w", name, err)
		}
		v := service.BraveValue{Key: key, Name: name}
		switch typ {
		case registry.DWORD:
			n, _, err := k.GetIntegerValue(name)
			if err != nil {
				return nil, fmt.Errorf("значение %s: %w", name, err)
			}
			v.DWord, v.Num = true, uint32(n)
		case registry.SZ, registry.EXPAND_SZ:
			str, _, err := k.GetStringValue(name)
			if err != nil {
				return nil, fmt.Errorf("значение %s: %w", name, err)
			}
			v.Str = str
		}
		out = append(out, v)
	}
	return out, nil
}

// applyBravePlan исполняет план: сначала дописывает нужное, затем удаляет лишние значения и подключи.
func applyBravePlan(root registry.Key, path string, set, del []service.BraveValue, delKeys []string) error {
	for _, v := range set {
		k, _, err := registry.CreateKey(root, joinKey(path, v.Key), registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("ключ %s: %w", v.Key, err)
		}
		if v.DWord {
			err = k.SetDWordValue(v.Name, v.Num)
		} else {
			err = k.SetStringValue(v.Name, v.Str)
		}
		k.Close()
		if err != nil {
			return fmt.Errorf("значение %s: %w", v.Name, err)
		}
	}
	for _, v := range del {
		k, err := registry.OpenKey(root, joinKey(path, v.Key), registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("ключ %s: %w", v.Key, err)
		}
		err = k.DeleteValue(v.Name)
		k.Close()
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("значение %s не удалено: %w", v.Name, err)
		}
	}
	for _, sub := range delKeys {
		if err := deleteTree(root, path+`\`+sub); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("подключ %s не удалён: %w", sub, err)
		}
	}
	return nil
}

func joinKey(path, key string) string {
	if key == "" {
		return path
	}
	return path + `\` + key
}

// deleteTree удаляет ключ path со всеми подключами (registry.DeleteKey удаляет только лист).
func deleteTree(root registry.Key, path string) error {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return err
	}
	for _, sub := range subs {
		if err := deleteTree(root, path+`\`+sub); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	return registry.DeleteKey(root, path)
}
