//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
	"pcblock/internal/storage"
)

const (
	serviceStartWait = 35 * time.Second // Ruling 27: служба поднимается до 35 с
	serviceStopWait  = 30 * time.Second
)

// Install ставит службу и запускает её (--install-service). Повторный запуск безопасен
// (Ruling 21): служба уже есть, её настройки обновляются, остановленная запускается.
// Результат пишется в --out; ошибка возвращается для кода выхода.
func Install(opts launch.Options) error {
	info := map[string]any{}
	return finish(opts["out"], info, install(info))
}

func install(info map[string]any) error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	info["exe"] = exe
	dataDir := defaultDataDir()
	if err := ensureDataDir(dataDir); err != nil {
		return err
	}
	info["dataDir"] = dataDir

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("Нужны права администратора, чтобы установить службу: %w", err)
	}
	defer m.Disconnect()

	cfg := mgr.Config{
		ServiceType:    windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:      mgr.StartAutomatic,
		ErrorControl:   mgr.ErrorNormal,
		DisplayName:    serviceDisplayName,
		Description:    serviceDescription,
		BinaryPathName: syscall.EscapeArg(exe) + " " + launch.ModeService.Flag(),
	}
	s, err := m.OpenService(ipc.ServiceName)
	switch {
	case err == nil:
		info["service"] = "обновлена"
		if err := s.UpdateConfig(cfg); err != nil {
			s.Close()
			return fmt.Errorf("служба не обновлена: %w", err)
		}
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		info["service"] = "создана"
		s, err = m.CreateService(ipc.ServiceName, exe, cfg, launch.ModeService.Flag())
		if err != nil {
			return fmt.Errorf("служба не создана: %w", err)
		}
	default:
		return fmt.Errorf("служба не открыта: %w", err)
	}
	defer s.Close()

	actions := make([]mgr.RecoveryAction, 0, len(recoveryDelays))
	for _, d := range recoveryDelays {
		actions = append(actions, mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: d})
	}
	if err := s.SetRecoveryActions(actions, recoveryResetS); err != nil {
		return fmt.Errorf("перезапуск службы при сбое не настроен: %w", err)
	}
	// Выход службы с ошибкой при неудачном запуске тоже сбой: пусть перезапускается.
	if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("перезапуск службы при ошибке запуска не настроен: %w", err)
	}
	if err := registerSafeBoot(); err != nil {
		return err
	}

	st, err := s.Query()
	if err != nil {
		return fmt.Errorf("состояние службы не прочитано: %w", err)
	}
	if st.State != svc.Running && st.State != svc.StartPending {
		if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			return fmt.Errorf("служба не запущена: %w", err)
		}
	}
	if err := waitState(s, svc.Running, serviceStartWait); err != nil {
		return err
	}
	info["state"] = stateName(svc.Running)
	return nil
}

// registerSafeBoot записывает службу в списки безопасного режима (FR-30): без этого она не
// стартует в безопасном режиме, и ребёнок мог бы войти туда без учёта.
func registerSafeBoot() error {
	for _, path := range []string{safeBootMinimalKey, safeBootNetworkKey} {
		k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, path, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("служба не записана для безопасного режима: %w", err)
		}
		err = k.SetStringValue("", "Service")
		k.Close()
		if err != nil {
			return fmt.Errorf("служба не записана для безопасного режима: %w", err)
		}
	}
	return nil
}

func unregisterSafeBoot() error {
	var firstErr error
	for _, path := range []string{safeBootMinimalKey, safeBootNetworkKey} {
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, path); err != nil && !errors.Is(err, registry.ErrNotExist) && firstErr == nil {
			firstErr = fmt.Errorf("запись безопасного режима не удалена: %w", err)
		}
	}
	return firstErr
}

// Remove останавливает и удаляет службу (--remove-service): завершает оставшиеся стражи,
// значки и окна, снимает политику Диспетчера задач у детей, убирает запись безопасного режима.
// Статистика остаётся, если не указан --delete-data (FR-4).
func Remove(opts launch.Options) error {
	info := map[string]any{}
	return finish(opts["out"], info, remove(opts.Flag("delete-data"), info))
}

func remove(deleteData bool, info map[string]any) error {
	// Завершать стражей (процессы системы) администратору позволяет только отладочная привилегия.
	_ = enablePrivilege("SeDebugPrivilege")

	var problems []string
	note := func(err error) {
		if err != nil {
			problems = append(problems, err.Error())
		}
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("Нужны права администратора, чтобы удалить службу: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ipc.ServiceName)
	switch {
	case err == nil:
		note(stopAndDelete(s, info))
		s.Close()
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		info["service"] = "не было"
	default:
		return fmt.Errorf("служба не открыта: %w", err)
	}

	dataDir := defaultDataDir()
	if tm, err := readTestMode(); err == nil && tm.DataDir != "" {
		dataDir = tm.DataDir
	}
	note(clearChildPolicies(dataDir, info))

	if exe, err := exePath(); err == nil {
		killed, kerr := killLeftovers(exe)
		info["killed"] = killed
		note(kerr)
	}
	note(unregisterSafeBoot())

	if deleteData {
		if err := os.RemoveAll(dataDir); err != nil {
			note(fmt.Errorf("папка данных %s не удалена: %w", dataDir, err))
		} else {
			info["data"] = "удалены"
		}
	} else {
		info["data"] = "оставлены"
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// stopAndDelete останавливает службу (ожидая остановки) и помечает её на удаление.
func stopAndDelete(s *mgr.Service, info map[string]any) error {
	st, err := s.Query()
	if err != nil {
		return fmt.Errorf("состояние службы не прочитано: %w", err)
	}
	if st.State != svc.Stopped {
		if st.State != svc.StopPending {
			if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
				return fmt.Errorf("служба не остановлена: %w", err)
			}
		}
		if err := waitState(s, svc.Stopped, serviceStopWait); err != nil {
			// Служба зависла: завершаем её процесс, иначе удаление не завершится.
			if st, qerr := s.Query(); qerr == nil && st.ProcessId != 0 {
				if h, oerr := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, st.ProcessId); oerr == nil {
					_ = windows.TerminateProcess(h, 1)
					_, _ = windows.WaitForSingleObject(h, 5000)
					_ = windows.CloseHandle(h)
					info["service"] = "завершена принудительно"
				}
			}
		}
	}
	if err := s.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		return fmt.Errorf("служба не удалена: %w", err)
	}
	if _, ok := info["service"]; !ok {
		info["service"] = "удалена"
	}
	return nil
}

// clearChildPolicies снимает запрет Диспетчера задач у отмеченных детей (Ruling 20). Базы нет
// или она не открылась: снимать нечего или нечем, это не мешает удалению.
func clearChildPolicies(dataDir string, info map[string]any) error {
	path := dbPath(dataDir)
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	st, err := storage.Open(path)
	if err != nil {
		info["policy"] = "база не открылась, запрет Диспетчера задач не снят"
		return nil
	}
	defer st.Close()
	children, err := st.ListChildren()
	if err != nil {
		return fmt.Errorf("список детей не прочитан: %w", err)
	}
	var p policy
	var failed []string
	for _, c := range children {
		if err := p.SetTaskMgrDisabled(c.SID, false); err != nil {
			failed = append(failed, err.Error())
		}
	}
	info["policyCleared"] = len(children) - len(failed)
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

// killLeftovers завершает процессы этой же программы (стражи, значки, окна), оставшиеся
// после остановки службы. Свой процесс и чужие программы с тем же именем не трогаются:
// сравнивается полный путь к файлу.
func killLeftovers(exe string) (int, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, fmt.Errorf("список процессов не получен: %w", err)
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	self := uint32(os.Getpid())
	base := filepath.Base(exe)
	killed := 0
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if pe.ProcessID == self || !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), base) {
			continue
		}
		if path, ok := imagePath(pe.ProcessID); !ok || !strings.EqualFold(path, exe) {
			continue
		}
		h, oerr := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pe.ProcessID)
		if oerr != nil {
			continue
		}
		if windows.TerminateProcess(h, 1) == nil {
			killed++
			_, _ = windows.WaitForSingleObject(h, 3000)
		}
		_ = windows.CloseHandle(h)
	}
	return killed, nil
}

// imagePath возвращает полный путь к файлу программы процесса.
func imagePath(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:size]), true
}

// waitState ждёт, пока служба придёт в состояние want.
func waitState(s *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last svc.Status
	for {
		st, err := s.Query()
		if err != nil {
			return fmt.Errorf("состояние службы не прочитано: %w", err)
		}
		last = st
		if st.State == want {
			return nil
		}
		if want == svc.Running && st.State == svc.Stopped {
			// Служба запустилась и сразу остановилась: ждать нечего.
			return fmt.Errorf("служба остановилась сразу после запуска (код %d, код службы %d)", st.Win32ExitCode, st.ServiceSpecificExitCode)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("служба не пришла в состояние «%s» за %d с, сейчас «%s»", stateName(want), int(timeout.Seconds()), stateName(last.State))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func stateName(s svc.State) string {
	switch s {
	case svc.Stopped:
		return "остановлена"
	case svc.StartPending:
		return "запускается"
	case svc.StopPending:
		return "останавливается"
	case svc.Running:
		return "работает"
	case svc.ContinuePending, svc.PausePending, svc.Paused:
		return "приостановлена"
	}
	return fmt.Sprintf("состояние %d", uint32(s))
}
