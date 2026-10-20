//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	policyPath        = `Software\Microsoft\Windows\CurrentVersion\Policies\System`
	policyValue       = "DisableTaskMgr"
	profileListPath   = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`
	profileImageValue = "ProfileImagePath"
	tempHivePrefix    = "PCBlockTmp-"
)

var (
	advapi32          = windows.NewLazySystemDLL("advapi32.dll")
	procRegLoadKeyW   = advapi32.NewProc("RegLoadKeyW")
	procRegUnLoadKeyW = advapi32.NewProc("RegUnLoadKeyW")
)

// policy ставит и снимает запрет Диспетчера задач в учётке ребёнка (service.Policy, FR-29,
// Ruling 20): стандартная политика Windows DisableTaskMgr в ветке реестра пользователя.
// Пользователь вошёл: ветка уже загружена в HKEY_USERS. Не вошёл: ветка профиля загружается
// на время записи (RegLoadKey). Профиля ещё нет: запись откладывается до первого входа
// (служба повторяет её при появлении сеанса ребёнка) и ошибкой не считается.
type policy struct {
	mu sync.Mutex // загрузка ветки профиля идёт по одной
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
	return p.viaProfile(sid, disabled)
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

// viaProfile подгружает ветку профиля пользователя (NTUSER.DAT), пишет значение и выгружает.
func (p *policy) viaProfile(sid string, disabled bool) error {
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
	werr := writePolicy(temp, disabled)
	uerr := unloadHive(temp)
	if werr != nil {
		return fmt.Errorf("политика Диспетчера задач для %s: %w", sid, werr)
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
