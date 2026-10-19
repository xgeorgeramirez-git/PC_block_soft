//go:build windows

package winhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
)

// Служебные ключи приёмки (Ruling 23): --test-setup, --test-ipc, --test-runas. Работают только
// при включённом тестовом режиме (ключ реестра, писать в который может лишь администратор);
// результат пишут в файл --out. Права на команды проверяет служба, как у любого клиента.

var errTestModeOff = errors.New("Служебный ключ работает только при включённом тестовом режиме")

const dialTimeout = 10 * time.Second

func dial() (*ipc.Client, error) {
	c, err := ipc.Dial(ipc.PipeName, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("служба не отвечает: %w", err)
	}
	return c, nil
}

// TestSetup делает первоначальную настройку без окна (--test-setup): пароль из --password,
// ребёнок из --user (по умолчанию учётка, от имени которой запущена программа), 2 часа в
// каждый день недели.
func TestSetup(opts launch.Options) error {
	info := map[string]any{}
	return finish(opts["out"], info, testSetup(opts, info))
}

func testSetup(opts launch.Options, info map[string]any) error {
	if !testModeOn() {
		return errTestModeOff
	}
	password := opts["password"]
	if password == "" {
		return errors.New("Не указан --password")
	}
	sid, name, err := resolveUser(opts["user"])
	if err != nil {
		return err
	}
	info["sid"], info["name"] = sid, name

	limits := make([]ipc.WeekdayLimit, 0, 7)
	for day := 1; day <= 7; day++ {
		limits = append(limits, ipc.WeekdayLimit{Weekday: day, TotalMin: 120})
	}
	c, err := dial()
	if err != nil {
		return err
	}
	defer c.Close()
	req := ipc.SetupRequest{Password: password, Children: []ipc.SetupChild{{SID: sid, Name: name, Limits: limits}}}
	if err := ipc.Call(c, ipc.CmdSetup, "", req, nil); err != nil {
		return replyText(err)
	}
	return nil
}

// resolveUser возвращает SID и имя учётки: по имени из --user, а без него для учётки
// процесса.
func resolveUser(user string) (sid, name string, err error) {
	user = strings.TrimSpace(user)
	if user == "" {
		tu, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			return "", "", fmt.Errorf("учётка процесса не определена: %w", err)
		}
		account, _, _, err := tu.User.Sid.LookupAccount("")
		if err != nil {
			return "", "", fmt.Errorf("имя учётки процесса не определено: %w", err)
		}
		return tu.User.Sid.String(), account, nil
	}
	s, _, _, err := windows.LookupSID("", user)
	if err != nil {
		return "", "", fmt.Errorf("учётка %q не найдена: %w", user, err)
	}
	if i := strings.LastIndex(user, `\`); i >= 0 {
		user = user[i+1:]
	}
	return s.String(), user, nil
}

// TestIPC отправляет один запрос в канал связи (--test-ipc): --payload — запрос целиком в
// JSON, например {"cmd":"GetChildStatus"}; ответ службы пишется в --out как есть. Код выхода
// 0, если служба ответила успехом.
func TestIPC(opts launch.Options) error {
	out := opts["out"]
	if !testModeOn() {
		return finish(out, nil, errTestModeOff)
	}
	var msg ipc.Msg
	if err := json.Unmarshal([]byte(opts["payload"]), &msg); err != nil || msg.Cmd == "" {
		return finish(out, nil, errors.New("--payload должен быть запросом в JSON с полем cmd"))
	}
	c, err := dial()
	if err != nil {
		return finish(out, nil, err)
	}
	defer c.Close()
	rep, err := c.Do(msg)
	if err != nil {
		return finish(out, nil, err)
	}
	if werr := writeJSON(out, rep); werr != nil {
		return werr
	}
	if !rep.OK {
		return errors.New(rep.Error)
	}
	return nil
}

// replyText достаёт из отказа службы готовый текст для человека.
func replyText(err error) error {
	var re *ipc.ReplyError
	if errors.As(err, &re) && re.Text != "" {
		return errors.New(re.Text)
	}
	return err
}

var procCreateProcessWithLogonW = advapi32.NewProc("CreateProcessWithLogonW")

const (
	logonWithProfile         = 0x1
	createUnicodeEnvironment = 0x400
	runAsWait                = 120 * time.Second
)

// TestRunAs запускает программу от имени другой учётки (--test-runas): --user, --password и
// --exe (команда целиком: путь и ключи), ждёт завершения и пишет код выхода в --out. Если
// вторичный вход на раннере недоступен, ошибка попадёт в --out.
func TestRunAs(opts launch.Options) error {
	info := map[string]any{}
	return finish(opts["out"], info, testRunAs(opts, info))
}

func testRunAs(opts launch.Options, info map[string]any) error {
	if !testModeOn() {
		return errTestModeOff
	}
	user, password, cmd := opts["user"], opts["password"], opts["exe"]
	if user == "" || cmd == "" {
		return errors.New("Нужны --user и --exe")
	}
	domain := "."
	if i := strings.LastIndex(user, `\`); i >= 0 {
		domain, user = user[:i], user[i+1:]
	}
	userp, err := windows.UTF16PtrFromString(user)
	if err != nil {
		return err
	}
	domainp, err := windows.UTF16PtrFromString(domain)
	if err != nil {
		return err
	}
	passp, err := windows.UTF16PtrFromString(password)
	if err != nil {
		return err
	}
	cmdp, err := windows.UTF16PtrFromString(cmd)
	if err != nil {
		return err
	}
	si := windows.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	r, _, callErr := procCreateProcessWithLogonW.Call(
		uintptr(unsafe.Pointer(userp)), uintptr(unsafe.Pointer(domainp)), uintptr(unsafe.Pointer(passp)),
		logonWithProfile, 0, uintptr(unsafe.Pointer(cmdp)),
		createUnicodeEnvironment, 0, 0,
		uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		return fmt.Errorf("Процесс от имени %q не запущен: %w", user, callErr)
	}
	defer windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)
	info["pid"] = pi.ProcessId
	ev, _ := windows.WaitForSingleObject(pi.Process, uint32(runAsWait/time.Millisecond))
	if ev != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("Процесс %d не завершился за %d с", pi.ProcessId, int(runAsWait.Seconds()))
	}
	var code uint32
	if err := windows.GetExitCodeProcess(pi.Process, &code); err != nil {
		return fmt.Errorf("Код выхода процесса %d не получен: %w", pi.ProcessId, err)
	}
	info["exitCode"] = code
	if code != 0 {
		return fmt.Errorf("Процесс %d завершился с кодом %d", pi.ProcessId, code)
	}
	return nil
}
