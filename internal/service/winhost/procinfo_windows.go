//go:build windows

package winhost

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Общие вызовы Windows для процессов и путей: путь exe по ручке процесса, момент создания,
// развёртывание 8.3-имён и junction. Ими пользуются наблюдатель (procwatch_windows.go), удаление
// службы (killLeftovers), запуск в сеансах (launcher.Stamp) и корни компьютера.

const (
	maxImagePathChars = 32768 // предел пути Windows в символах (с префиксом «\\?\»)

	// Флаги GetFinalPathNameByHandle: нормализованный путь с буквой диска.
	fileNameNormalized = 0x0 // FILE_NAME_NORMALIZED
	volumeNameDOS      = 0x0 // VOLUME_NAME_DOS
)

// queryImagePath — полный путь exe процесса по ручке (нужен доступ
// PROCESS_QUERY_LIMITED_INFORMATION). Буфер растёт до maxImagePathChars символов: пути длиннее
// MAX_PATH читаются целиком.
func queryImagePath(h windows.Handle) (string, error) {
	for n := uint32(512); ; n *= 2 {
		buf := make([]uint16, n)
		size := n
		err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
		if err == nil {
			return windows.UTF16ToString(buf[:size]), nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || n >= maxImagePathChars {
			return "", err
		}
	}
}

// processCreated — момент создания процесса по ручке (FILETIME одним числом, как
// launcher.Stamp) и признак, что процесс уже завершился.
func processCreated(h windows.Handle) (stamp uint64, exited bool, err error) {
	var created, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exit, &kernel, &user); err != nil {
		return 0, false, err
	}
	exited = exit.HighDateTime != 0 || exit.LowDateTime != 0
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), exited, nil
}

// imagePath возвращает полный путь к файлу программы процесса.
func imagePath(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	p, err := queryImagePath(h)
	return p, err == nil
}

// canonicalPath разворачивает путь файла или папки к виду, по которому классифицирует служба
// (RM3-1): junction и символические ссылки — через GetFinalPathNameByHandle, иначе 8.3-имена —
// через GetLongPathName. Сетевой путь (в том числе подключённый диск) не разворачивается: по сети
// наблюдатель не ходит. ok = false — путь непригоден: в нём осталось 8.3-имя, которое не удалось
// развернуть («c:\progra~1\…» выглядел бы путём вне Program Files).
func canonicalPath(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if isUNC(raw) || remoteDrive(raw) {
		return raw, !hasShortName(raw)
	}
	if p, ok := finalPath(raw); ok {
		return p, true
	}
	if p, ok := longPath(raw); ok {
		return p, !hasShortName(p)
	}
	return raw, !hasShortName(raw)
}

// remoteDrive: путь на подключённом сетевом диске («Z:\…»).
func remoteDrive(p string) bool {
	if len(p) < 3 || p[1] != ':' || p[2] != '\\' {
		return false
	}
	root, err := windows.UTF16PtrFromString(p[:3])
	if err != nil {
		return false
	}
	return windows.GetDriveType(root) == windows.DRIVE_REMOTE
}

// finalPath — путь после всех junction и ссылок с длинными именами (GetFinalPathNameByHandle).
// Файл открывается без доступа к содержимому, только чтобы узнать путь.
func finalPath(raw string) (string, bool) {
	name, err := windows.UTF16PtrFromString(extendedPath(raw))
	if err != nil {
		return "", false
	}
	h, err := windows.CreateFile(name, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	n := uint32(512)
	for range 2 {
		buf := make([]uint16, n)
		got, err := windows.GetFinalPathNameByHandle(h, &buf[0], n, fileNameNormalized|volumeNameDOS)
		if err != nil || got == 0 {
			return "", false
		}
		if got < n {
			return stripExtended(windows.UTF16ToString(buf[:got])), true
		}
		if got > maxImagePathChars+1 {
			return "", false
		}
		n = got + 1
	}
	return "", false
}

// longPath — путь с длинными именами вместо 8.3 (GetLongPathName); файл должен существовать.
func longPath(raw string) (string, bool) {
	name, err := windows.UTF16PtrFromString(extendedPath(raw))
	if err != nil {
		return "", false
	}
	n := uint32(512)
	for range 2 {
		buf := make([]uint16, n)
		got, err := windows.GetLongPathName(name, &buf[0], n)
		if err != nil || got == 0 {
			return "", false
		}
		if got < n {
			return stripExtended(windows.UTF16ToString(buf[:got])), true
		}
		if got > maxImagePathChars+1 {
			return "", false
		}
		n = got + 1
	}
	return "", false
}
