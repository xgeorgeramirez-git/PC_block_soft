//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

// processWatcher — service.ProcessWatcher над Toolhelp32 (M3: R12, R13, NFR-6, RM3-11, PL-9).
// Адаптер решений не принимает: он сообщает факты (процесс, сеанс, путь, момент создания) и
// завершает процесс по просьбе службы, только если совпали номер, момент создания и путь.
// Содержимое процессов не читается и не меняется.
type processWatcher struct {
	self uint32

	mu    sync.Mutex
	cache map[procStamp]procPath // путь по (номер, момент создания): разворачивается один раз
}

// procStamp — процесс: номер повторяется, пара (номер, момент создания) — нет.
type procStamp struct {
	pid   uint32
	stamp uint64
}

// procPath — развёрнутый путь процесса и его пользователь; ok = false — путь не прочитан,
// userOK = false — пользователь (SID маркера доступа) не прочитан.
type procPath struct {
	session uint32
	path    string
	ok      bool
	user    string
	userOK  bool
}

// Номера процессов, которых наблюдатель не касается: бездействие системы и System.
const (
	pidIdle   = 0
	pidSystem = 4

	killWaitMs = 300 // сколько Kill ждёт исчезновения процесса (Ruling T4b: не дольше 300 мс)
)

func newProcessWatcher() *processWatcher {
	return &processWatcher{self: uint32(os.Getpid()), cache: map[procStamp]procPath{}}
}

// List возвращает процессы сеанса sessionID, кроме System, бездействия и самой службы. Процесс,
// который не открылся (защищённый, отказ в доступе) или чей путь не развернулся, приходит с
// PathOK = false: служба его не трогает. Ошибка — только если не получен список процессов.
func (w *processWatcher) List(sessionID uint32) ([]service.ProcInfo, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("список процессов не получен: %w", err)
	}
	defer windows.CloseHandle(snap)

	w.mu.Lock()
	defer w.mu.Unlock()
	seen := map[procStamp]bool{}
	out := []service.ProcInfo{}
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		pid := pe.ProcessID
		if pid == pidIdle || pid == pidSystem || pid == w.self {
			continue
		}
		var sid uint32
		if windows.ProcessIdToSessionId(pid, &sid) != nil || sid != sessionID {
			continue // сеанс не узнать или чужой: не наш процесс
		}
		info, key, ok := w.inspect(pid, sid)
		if !ok {
			continue // процесс уже завершился
		}
		if key.stamp != 0 {
			seen[key] = true
		}
		out = append(out, info)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("список процессов прочитан не до конца: %w", err)
	}
	for k, v := range w.cache {
		if v.session == sessionID && !seen[k] {
			delete(w.cache, k)
		}
	}
	return out, nil
}

// inspect открывает процесс на чтение сведений и возвращает его описание (путь, пользователь);
// ok = false — процесс уже завершился. Ручка закрывается здесь же.
func (w *processWatcher) inspect(pid, session uint32) (service.ProcInfo, procStamp, bool) {
	info := service.ProcInfo{PID: pid, SessionID: session}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return info, procStamp{}, true // защищённый процесс или отказ в доступе: путь не прочитан
	}
	defer windows.CloseHandle(h)
	stamp, exited, err := processCreated(h)
	if err != nil {
		return info, procStamp{}, true
	}
	if exited {
		return info, procStamp{}, false
	}
	info.Stamp = stamp
	key := procStamp{pid: pid, stamp: stamp}
	c, ok := w.cache[key]
	if !ok {
		c = procPath{session: session}
		c.path, c.ok = resolveImage(h)
		c.user, c.userOK = processUser(h)
		w.cache[key] = c
	}
	info.Path, info.PathOK = c.path, c.ok
	info.UserSID, info.UserOK = c.user, c.userOK
	return info, key, true
}

// processUser — SID пользователя, от имени которого идёт процесс, по уже открытой ручке
// (OpenProcessToken с TOKEN_QUERY, GetTokenUser). Не прочитался — ok = false: служба такой
// процесс не трогает (R19, PL-9).
func processUser(h windows.Handle) (string, bool) {
	var tok windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok); err != nil {
		return "", false
	}
	defer tok.Close()
	tu, err := tok.GetTokenUser()
	if err != nil || tu.User.Sid == nil {
		return "", false
	}
	sid := tu.User.Sid.String()
	return sid, sid != ""
}

// resolveImage — развёрнутый путь exe процесса по ручке.
func resolveImage(h windows.Handle) (string, bool) {
	raw, err := queryImagePath(h)
	if err != nil {
		return "", false
	}
	return canonicalPath(raw)
}

// Kill завершает процесс p, если это всё ещё он: ручка открывается один раз, по ней сверяются
// момент создания (PID мог достаться другому процессу, PL-9) и путь, ею же процесс завершается.
// Ждёт исчезновения до killWaitMs, но ответ от ожидания не зависит: true — TerminateProcess этого
// вызова удался (процесс мог ещё не исчезнуть, следующий List его покажет, а повторный Kill
// ответит false «уже завершается»; иначе служба не записала бы закрытие вовсе, PL-16); уже
// завершённый, другой, защищённый (отказ в доступе) процесс и непрочитанный путь — false.
func (w *processWatcher) Kill(p service.ProcInfo) bool {
	if !p.PathOK || p.Path == "" || p.PID == pidIdle || p.PID == pidSystem || p.PID == w.self {
		return false
	}
	want := programs.Normalize(p.Path)
	if want == "" {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, p.PID)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	stamp, exited, err := processCreated(h)
	if err != nil || exited || stamp != p.Stamp {
		return false
	}
	if ev, err := windows.WaitForSingleObject(h, 0); err != nil || ev != uint32(windows.WAIT_TIMEOUT) {
		return false // уже завершается
	}
	// Путь того же процесса (номер и момент совпали) уже развёрнут при List: берётся из памяти,
	// иначе разворачивается по этой же ручке.
	w.mu.Lock()
	c, cached := w.cache[procStamp{pid: p.PID, stamp: stamp}]
	w.mu.Unlock()
	if !cached {
		c.path, c.ok = resolveImage(h)
	}
	if !c.ok || programs.Normalize(c.path) != want {
		return false
	}
	if err := windows.TerminateProcess(h, 1); err != nil {
		return false
	}
	_, _ = windows.WaitForSingleObject(h, killWaitMs) // даём процессу исчезнуть до следующего List
	return true
}

var _ service.ProcessWatcher = (*processWatcher)(nil)
