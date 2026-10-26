package winhost

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pcblock/internal/service"
	"pcblock/internal/winsession"
)

// Имена и значения, общие для всех систем.
const (
	// dataDirSDDL — доступ к папке данных только у SYSTEM и Администраторов, без наследования
	// от родителя (NFR-9): D:P защищает список, OICI передаёт права файлам и папкам внутри.
	dataDirSDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

	dataDirName = "PCBlock"
	dbFileName  = "pcblock.db"
	logDirName  = "logs"
	logBaseName = "service" // OpenRotatingLog сам добавляет ".log"
	bootLogName = "boot.log"

	logMaxBytes = 10 << 20 // журнал работы по кругу: 5 файлов по 10 МБ
	logFiles    = 5

	serviceDisplayName = "PC Block"
	serviceDescription = "Считает время за компьютером в детских учётках и применяет правила родительского контроля"

	// Ключи безопасного режима: служба стартует и там (FR-30); значение по умолчанию «Service».
	safeBootMinimalKey = `SYSTEM\CurrentControlSet\Control\SafeBoot\Minimal\PCBlock`
	safeBootNetworkKey = `SYSTEM\CurrentControlSet\Control\SafeBoot\Network\PCBlock`

	// recoveryResetS — через сколько секунд без сбоев счётчик сбоев службы обнуляется (сутки).
	recoveryResetS = 24 * 60 * 60
)

// recoveryDelays — перезапуск службы после 1-го, 2-го и следующих сбоев (ТЗ 3.2, Ruling 27).
var recoveryDelays = [...]time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second}

// sessionIDOf читает номер сеанса из данных события SessionChange: структура
// WTSSESSION_NOTIFICATION {cbSize uint32, dwSessionId uint32}. false: данных нет, они короче
// структуры или cbSize не сходится с ними; тогда служба получает SessionUnknown и сверяет весь
// список сеансов, а не угадывает сеанс.
func sessionIDOf(raw []byte) (uint32, bool) {
	const size = 8
	if len(raw) < size {
		return 0, false
	}
	cb := binary.LittleEndian.Uint32(raw)
	if cb < size || int64(cb) > int64(len(raw)) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(raw[4:]), true
}

// sessionEventOf переводит код WTS_SESSION_* из уведомления диспетчера служб в событие службы.
// Номер сеанса приходит отдельно (sessionIDOf); на любое событие служба ещё и сверяет список сеансов.
func sessionEventOf(eventType uint32) (service.SessionEvent, bool) {
	switch eventType {
	case 1, 3, 0xa: // WTS_CONSOLE_CONNECT, WTS_REMOTE_CONNECT, WTS_SESSION_CREATE
		return service.SessionConnect, true
	case 2, 4: // WTS_CONSOLE_DISCONNECT, WTS_REMOTE_DISCONNECT
		return service.SessionDisconnect, true
	case 5: // WTS_SESSION_LOGON
		return service.SessionLogon, true
	case 6, 0xb: // WTS_SESSION_LOGOFF, WTS_SESSION_TERMINATE
		return service.SessionLogoff, true
	case 7: // WTS_SESSION_LOCK
		return service.SessionLock, true
	case 8: // WTS_SESSION_UNLOCK
		return service.SessionUnlock, true
	}
	return 0, false
}

// sessionInfos переводит сеансы Windows в список службы (FB-3, R2): в него входят все сеансы
// с вошедшим пользователем, в том числе отключённые при смене пользователя, с признаком,
// на экране ли сеанс. Служебный сеанс 0 и экран входа без пользователя не в счёт. Отключённый
// сеанс нельзя выбрасывать из списка: служба сочла бы его закрытым и завершила бы стража, а
// программы ребёнка в нём продолжают работать и должны оставаться под присмотром.
func sessionInfos(list []winsession.Session) []service.SessionInfo {
	out := make([]service.SessionInfo, 0, len(list))
	for _, s := range list {
		if s.SID == "" {
			continue
		}
		screen := service.ScreenOff
		if s.Active {
			screen = service.ScreenOn
		}
		out = append(out, service.SessionInfo{ID: s.ID, User: s.User, SID: s.SID, Screen: screen})
	}
	return out
}

// dbPath — путь к базе в папке данных.
func dbPath(dataDir string) string { return filepath.Join(dataDir, dbFileName) }

// logDir — папка журнала работы в папке данных.
func logDir(dataDir string) string { return filepath.Join(dataDir, logDirName) }

// appendBootLog дописывает строку в журнал запуска: туда попадает то, что не дало службе
// даже открыть свой журнал. Ошибки записи молча теряются: больше писать некуда.
func appendBootLog(dataDir, text string) {
	dir := logDir(dataDir)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, bootLogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), text)
}

// result — итог служебного ключа: его пишет файл --out (Ruling 23). Info — произвольные
// подробности для проверяющего (состояние службы, найденные учётки и так далее).
type result struct {
	OK    bool           `json:"ok"`
	Error string         `json:"error,omitempty"`
	Info  map[string]any `json:"info,omitempty"`
}

// writeJSON пишет значение в файл path одним JSON без экранирования кириллицы и угловых скобок.
// Пустой path: результат не нужен.
func writeJSON(path string, v any) error {
	if path == "" {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("результат не собран: %w", err)
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("папка результата не создана: %w", err)
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("результат не записан: %w", err)
	}
	return nil
}

// finish записывает итог служебного ключа в --out и возвращает ошибку действия (или ошибку
// записи, если само действие удалось): код выхода программы определяется по ней.
func finish(out string, info map[string]any, actionErr error) error {
	r := result{OK: actionErr == nil, Info: info}
	if actionErr != nil {
		r.Error = actionErr.Error()
	}
	if err := writeJSON(out, r); err != nil && actionErr == nil {
		return err
	}
	return actionErr
}
