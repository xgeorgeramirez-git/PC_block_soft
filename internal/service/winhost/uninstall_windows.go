//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
)

// Коды выхода --uninstall-gate: их читает установщик (build/windows/installer/project.nsi, ТЗ FR-4).
const (
	gateDelete     = 0 // удалять программу, статистику оставить
	gateCancel     = 1 // отмена, программа остаётся
	gateDeleteData = 2 // удалять программу вместе со статистикой
)

// gateService — в каком состоянии служба на момент вопроса об удалении.
type gateService int

const (
	gateServiceUnknown gateService = iota // диспетчер служб не ответил: пробуем канал как есть
	gateServiceNone                       // службы нет: защищать нечего
	gateServiceBroken                     // служба есть, но не запускается или не отвечает
	gateServiceReady                      // служба работает
)

// UninstallGate решает за окно удаления то, что окну решать нечем (FR-4, Ruling 21), и возвращает
// код выхода и признак «окно не нужно». Службы нет или она не запускается: пароль проверять некому,
// удалять можно (удаление и так под силу только администратору, а при мёртвой службе защита не
// действует). Тихий режим (--silent) обходится без окна: проверка по --password, галочка
// «Удалить статистику» приходит как --delete-data. Окно нужно только в обычном режиме при живой
// службе: тогда возвращается (0, false), и окно само спросит пароль и галочку.
func UninstallGate(opts launch.Options) (code int, done bool) {
	allow := gateDelete
	if opts.Flag("delete-data") {
		allow = gateDeleteData
	}
	state, why := probeService()
	switch state {
	case gateServiceNone:
		gateLog("службы нет, пароль не нужен")
		return allow, true
	case gateServiceBroken:
		gateLog("служба не отвечает, пароль не проверяется: " + why)
		return allow, true
	}
	if !opts.Flag("silent") {
		return 0, false
	}
	return silentGate(opts, allow), true
}

// probeService смотрит службу в диспетчере. Остановленную службу запускает и ждёт, пока она
// заработает: пароль знает только она.
func probeService() (gateService, string) {
	m, err := mgr.Connect()
	if err != nil {
		return gateServiceUnknown, err.Error()
	}
	defer m.Disconnect()
	s, err := m.OpenService(ipc.ServiceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return gateServiceNone, ""
	}
	if err != nil {
		return gateServiceUnknown, err.Error()
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return gateServiceUnknown, err.Error()
	}
	if st.State != svc.Running && st.State != svc.StartPending {
		if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			return gateServiceBroken, fmt.Sprintf("служба не запущена: %v", err)
		}
	}
	if err := waitState(s, svc.Running, serviceStartWait); err != nil {
		return gateServiceBroken, err.Error()
	}
	return gateServiceReady, ""
}

// silentGate проверяет разрешение на удаление без окна. Программа не настроена: пароль не нужен
// (Ruling 21). Настроена: нужен верный --password, иначе отказ. Нет ответа от службы: как «службы нет».
func silentGate(opts launch.Options, allow int) int {
	c, err := dial()
	if err != nil {
		gateLog("канал службы не открылся: " + err.Error())
		return allow
	}
	defer c.Close()

	var hello ipc.HelloReply
	if err := ipc.Call(c, ipc.CmdHello, "", nil, &hello); err != nil {
		gateLog("служба не ответила на Hello: " + err.Error())
		return allow
	}
	if !hello.Configured {
		return allow
	}
	password := opts["password"]
	if password == "" {
		gateLog("удаление отклонено: программа настроена, а пароль родителя (/PASSWORD=) не указан")
		return gateCancel
	}
	var reply ipc.UninstallCheckReply
	err = ipc.Call(c, ipc.CmdUninstallCheck, "", ipc.UninstallCheckRequest{Password: password}, &reply)
	var denied *ipc.ReplyError
	switch {
	case errors.As(err, &denied):
		gateLog("удаление отклонено: " + denied.Text)
		return gateCancel
	case err != nil:
		gateLog("служба оборвала разговор о пароле: " + err.Error())
		return allow
	case !reply.Allowed:
		gateLog("удаление отклонено службой")
		return gateCancel
	}
	return allow
}

// gateLog дописывает строку в журнал запуска папки данных, если папка есть. Пароль в журнал не
// попадает. Папку, которой нет, не создаёт: после удаления её не должно быть.
func gateLog(text string) {
	dir := defaultDataDir()
	if tm, err := readTestMode(); err == nil && tm.DataDir != "" {
		dir = tm.DataDir
	}
	if _, err := os.Stat(dir); err != nil {
		return
	}
	appendBootLog(dir, "Удаление: "+text)
}
