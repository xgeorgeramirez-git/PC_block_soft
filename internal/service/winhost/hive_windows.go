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

// Ветки реестра профилей пользователей (HKEY_USERS): общий помощник запрета Диспетчера задач,
// правил Brave (policy) и автозагрузки (startupSource, уточнение 5 плана M5b).

const (
	profileListPath   = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`
	profileImageValue = "ProfileImagePath"
	tempHivePrefix    = "PCBlockTmp-"
	// classesSuffix — ветка классов пользователя в HKEY_USERS (<SID>_Classes, файл UsrClass.dat).
	classesSuffix = "_Classes"
)

// hiveMu — загрузка и выгрузка веток профилей идут по одной на всю службу: имена временных
// веток общие (PCBlockTmp-<SID>), а policy и startupSource работают из разных горутин. Кто
// читает или пишет ветку пользователя, держит hiveMu всё время работы с ней: иначе соседняя
// операция выгрузит ветку посреди чтения.
var hiveMu sync.Mutex

var (
	advapi32          = windows.NewLazySystemDLL("advapi32.dll")
	procRegLoadKeyW   = advapi32.NewProc("RegLoadKeyW")
	procRegUnLoadKeyW = advapi32.NewProc("RegUnLoadKeyW")
)

// profileDir — папка профиля пользователя sid из ProfileList. Профиля ещё нет — ok = false без
// ошибки (пользователь ни разу не входил).
func profileDir(sid string) (dir string, ok bool, err error) {
	pk, err := registry.OpenKey(registry.LOCAL_MACHINE, profileListPath+sid, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("профиль %s не найден: %w", sid, err)
	}
	raw, _, err := pk.GetStringValue(profileImageValue)
	pk.Close()
	if err != nil {
		return "", false, fmt.Errorf("папка профиля %s не прочитана: %w", sid, err)
	}
	dir, err = registry.ExpandString(raw)
	if err != nil {
		return "", false, fmt.Errorf("папка профиля %s: %w", sid, err)
	}
	return dir, true, nil
}

// withHive подгружает ветку профиля пользователя (NTUSER.DAT) под временным именем, выполняет fn с
// этим именем и выгружает ветку. Профиля ещё нет — nil без вызова fn: запись при первом входе.
// Под hiveMu.
func withHive(sid string, fn func(hive string) error) error {
	return withProfileHive(sid, "NTUSER.DAT", "", fn)
}

// withClassesHive — то же для ветки классов (AppData\Local\Microsoft\Windows\UsrClass.dat), которую
// Windows держит в HKEY_USERS\<SID>_Classes. Под hiveMu.
func withClassesHive(sid string, fn func(hive string) error) error {
	return withProfileHive(sid, filepath.Join("AppData", "Local", "Microsoft", "Windows", "UsrClass.dat"), classesSuffix, fn)
}

func withProfileHive(sid, rel, suffix string, fn func(hive string) error) error {
	dir, ok, err := profileDir(sid)
	if err != nil || !ok {
		return err
	}
	dat := filepath.Join(dir, rel)
	if _, err := os.Stat(dat); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("файл профиля %s: %w", dat, err)
	}

	// Права на загрузку и выгрузку веток: у системы они есть, но выключены.
	_ = enablePrivilege("SeBackupPrivilege")
	_ = enablePrivilege("SeRestorePrivilege")

	temp := tempHivePrefix + sid + suffix
	if err := loadHive(temp, dat); err != nil {
		return fmt.Errorf("ветка профиля %s не загружена: %w", sid+suffix, err)
	}
	werr := fn(temp)
	uerr := unloadHive(temp)
	if werr != nil {
		return werr
	}
	if uerr != nil {
		return fmt.Errorf("ветка профиля %s не выгружена: %w", sid+suffix, uerr)
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
