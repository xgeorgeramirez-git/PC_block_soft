//go:build windows

package guard

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
	"pcblock/internal/sound"
	"pcblock/internal/systray"
)

const (
	wmAppRefresh = wmApp + 1 // опрос получил ответ или закончилась команда: перерисовать

	tickTimerID = 7
	tickEveryMs = 250 // как часто окна сверяются с Model (автозакрытие плашек, отсчёт паузы, обрыв связи)

	dialTimeout = 2 * time.Second // сколько ждать канал службы за один заход

	closingTimeout = 2 * time.Second // сколько ждать службу при конце сеанса: сеанс не должен ждать дольше (FB-1)

	boardRetry = time.Minute // окно табло не создалось: следующая попытка не раньше (не каждый кадр 250 мс)
)

var (
	hostProcPtr uintptr
	theApp      *app
)

func init() { hostProcPtr = windows.NewCallback(hostProc) }

// app — страж сеанса: контроллер, скрытое окно сообщений, экран блокировки и плашка. Всё,
// что трогает окна, выполняется в одном потоке (runtime.LockOSThread в Run); другие
// горутины только опрашивают службу и будят этот поток сообщением wmAppRefresh.
type app struct {
	ctl   *Controller
	host  uintptr
	hInst uintptr
	speed int // ускорение часов в тестовом режиме, иначе 1

	lock      *LockWindow
	ui        *lockUI
	plate     *plateWindow
	board     *systray.WarningCard
	boardFail time.Time // когда не удалось создать окно табло; повтор не чаще boardRetry
	hookTried bool
}

// lazyCaller открывает канал службы при первом запросе и после обрыва: страж обязан
// пережить перезапуск службы и запуск раньше неё (клиент канала сам переоткрывает соединение
// после ошибки, но не умеет начать без соединения).
type lazyCaller struct {
	mu sync.Mutex
	c  *ipc.Client
}

func (l *lazyCaller) Do(m ipc.Msg) (ipc.Reply, error) {
	l.mu.Lock()
	c := l.c
	if c == nil {
		nc, err := ipc.Dial(ipc.PipeName, dialTimeout)
		if err != nil {
			l.mu.Unlock()
			return ipc.Reply{}, err
		}
		l.c, c = nc, nc
	}
	l.mu.Unlock()
	return c.Do(m)
}

func (l *lazyCaller) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.c != nil {
		_ = l.c.Close()
		l.c = nil
	}
}

// singleton занимает именованный мьютекс. already = true: такой страж в сеансе уже есть.
func singleton(name string) (release func(), already bool, err error) {
	h, err := windows.CreateMutex(nil, false, utf16(name))
	if h == 0 {
		return nil, false, fmt.Errorf("мьютекс %s не создан: %w", name, err)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(h)
		return nil, true, nil
	}
	return func() { _ = windows.CloseHandle(h) }, false, nil
}

// sessionOf возвращает номер сеанса: из ключа --session (его передаёт служба) или свой.
func sessionOf(opts launch.Options) uint32 {
	if s := opts["session"]; s != "" {
		if n, err := strconv.ParseUint(s, 10, 32); err == nil {
			return uint32(n)
		}
	}
	var id uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &id) == nil {
		return id
	}
	return 0
}

// Run — страж сеанса ребёнка (FR-6, FR-9). Поток блокируется за ОС, занимается мьютекс
// Global\PCBlockGuard-<сеанс> (повторный запуск молча завершается), раз в настоящую секунду
// идёт опрос службы, а поток окон показывает предупреждения и экран блокировки по
// решению службы. Нет ответа службы 10 с: экран и плашки снимаются сами (Ruling 15, NFR-10).
// Возвращается, когда сеанс завершается; службу останавливает завершением процесса.
func Run(opts launch.Options) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	session := sessionOf(opts)
	release, already, err := singleton(`Global\PCBlockGuard-` + strconv.FormatUint(uint64(session), 10))
	if err != nil {
		return err
	}
	if already {
		return nil
	}
	defer release()

	procSetProcessDPIAware.Call()
	hInst, _, _ := procGetModuleHandle.Call(0)
	a := &app{hInst: hInst, speed: testClockSpeed()}
	theApp = a
	defer func() { theApp = nil }()

	cls, err := registerPlainClass("PCBlockGuardHost", hostProcPtr, hInst, 0)
	if err != nil {
		return fmt.Errorf("RegisterClassEx(страж): %w", err)
	}
	// Обычное скрытое окно, а не окно-сообщение: окна-сообщения не получают рассылок
	// WM_QUERYENDSESSION и WM_ENDSESSION.
	host, _, err := procCreateWindowEx.Call(
		wsExToolWin,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(utf16("PC Block Guard"))),
		wsPopup,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if host == 0 {
		return fmt.Errorf("CreateWindowEx(страж): %w", err)
	}
	a.host = host

	caller := &lazyCaller{}
	defer caller.close()
	a.ctl = NewController(NewModel(), caller, time.Now, a.readSensors, func() {
		procPostMessage.Call(a.host, wmAppRefresh, 0, 0)
	})
	a.ctl.SetSoundPlayer(sound.PlayWarning)
	a.ctl.SetClockSpeed(a.speed)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.pollLoop(stop)
	}()

	procSetTimer.Call(host, tickTimerID, tickEveryMs, 0)
	a.loop()

	close(stop)
	caller.close() // прерывает ожидающий запрос
	wg.Wait()
	a.shutdown()
	return nil
}

func (a *app) readSensors(now time.Time) ipc.SensorSample {
	speed := 1
	if a.ctl != nil && a.ctl.TestMode() {
		speed = a.speed
	}
	return ReadSensors(now, speed)
}

// pollLoop опрашивает службу раз в секунду и сразу после команды, меняющей решение; после
// каждого опроса проверяет срок автоматической смены пользователя.
func (a *app) pollLoop(stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		safely(func() {
			a.ctl.PollOnce()
			// Экран показан 15 минут, пароль родителя не введён: смена пользователя (D-40).
			if act := a.ctl.AutoSwitch(); act.Kind != ActNone {
				go safely(func() { a.ctl.Run(act) })
			}
			a.ctl.PushState()
		})
		select {
		case <-stop:
			return
		case <-t.C:
		case <-a.ctl.Kicked():
		}
	}
}

// loop крутит цикл сообщений потока окон. Enter на экране блокировки нажимает главную
// кнопку или ту, где фокус; Tab и стрелки обрабатывает IsDialogMessage.
func (a *app) loop() {
	var m tagMSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return // 0 = WM_QUIT, -1 = ошибка
		}
		if a.filter(&m) {
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// filter обрабатывает клавиши экрана блокировки; true: сообщение разобрано.
func (a *app) filter(m *tagMSG) bool {
	if a.lock == nil || a.ui == nil || !a.lock.Visible() {
		return false
	}
	if m.hwnd != a.lock.hwnd {
		if in, _, _ := procIsChild.Call(a.lock.hwnd, m.hwnd); in == 0 {
			return false
		}
	}
	if m.message == wmKeyDown && m.wParam == vkReturn {
		safely(a.enter)
		return true
	}
	r, _, _ := procIsDialogMessage.Call(a.lock.hwnd, uintptr(unsafe.Pointer(m)))
	return r != 0
}

// enter нажимает кнопку, у которой фокус, а если фокус в поле пароля, главную кнопку (ТЗ 8.0 п.8).
func (a *app) enter() {
	b := a.ui.focusedButton()
	if b == ButtonNone {
		b = a.ui.mainButton()
	}
	if b != ButtonNone {
		a.press(b)
	}
}

// press передаёт нажатие модели; команда службе выполняется в своей горутине, чтобы поток
// окон не ждал ответа (иначе перехват клавиш не успевал бы отвечать системе).
func (a *app) press(b Button) {
	pw := ""
	if (b == ButtonSubmitPassword || b == ButtonSubmitPromo) && a.ui != nil {
		pw = a.ui.passwordText() // пароль или промокод: поле одно, режим задаёт панель
	}
	act := a.ctl.Press(b, pw)
	if act.Kind != ActNone {
		go safely(func() { a.ctl.Run(act) })
	}
	a.tick()
}

// onEdit запоминает текст поля для GuardState (только тестовый режим) под именем поля открытой
// панели: пароль или промокод.
func (a *app) onEdit(text string) {
	id := FieldPasswordID
	if a.ui != nil && a.ui.view.Panel == PanelPromo {
		id = FieldPromoID
	}
	a.ctl.SetField(id, text)
}

// tick приводит окна к виду из Model.
func (a *app) tick() {
	a.ctl.NoteUITick(time.Now())
	v := a.ctl.View()
	a.applyLock(v)
	a.applyPlate(v)
	for _, t := range a.ctl.QueuedTests() {
		a.runTestAction(t)
	}
}

// applyLock показывает, обновляет или прячет экран блокировки; перехват клавиш держится
// ровно пока экран показан (View.SuppressKeys).
func (a *app) applyLock(v View) {
	if !v.LockShown {
		if a.lock != nil && a.lock.Visible() {
			a.lock.RemoveKeyboardGuard()
			a.lock.Hide()
		}
		a.hookTried = false
		return
	}
	if a.lock == nil {
		w, err := NewLockWindow(v.Lock)
		if err != nil {
			return // следующий кадр попробует снова
		}
		a.lock = w
		a.ui = newLockUI(w, a.press, a.onEdit)
		forceForeground(w.hwnd)
	} else if !a.lock.Visible() {
		a.lock.Show()
		forceForeground(a.lock.hwnd)
	}
	a.ui.apply(v, a.ctl.Notice())

	installed := a.lock.KeyboardGuardInstalled()
	switch {
	case v.SuppressKeys && !installed && !a.hookTried:
		a.hookTried = true
		_ = a.lock.InstallKeyboardGuard()
	case !v.SuppressKeys && installed:
		a.lock.RemoveKeyboardGuard()
	}
}

// applyPlate показывает плашку предупреждения или прячет её: табло за 30/15/5 мин — общей
// карточкой (systray.WarningCard, R-2), плашку 1 мин, «Добавлено N минут» и плашки о закрытых
// программах — текстовым окном с кнопками.
func (a *app) applyPlate(v View) {
	board, text := SplitPlate(v.Warning)
	if board == nil {
		if a.board != nil {
			a.board.Hide()
		}
	} else {
		if a.board == nil && (a.boardFail.IsZero() || time.Since(a.boardFail) >= boardRetry) {
			c, err := systray.NewWarningCard(func() { a.press(ButtonWarningClose) })
			if err == nil {
				a.board, a.boardFail = c, time.Time{}
			} else {
				if a.boardFail.IsZero() {
					// Один раз за серию отказов: у стража нет своего журнала, строка идёт в stderr
					// процесса (виден в отладке и в тестовом режиме); показ плашек это не останавливает.
					log.Printf("PC Block, страж: окно табло предупреждения не создано: %v", err)
				}
				a.boardFail = time.Now()
			}
		}
		if a.board != nil {
			a.board.Show(*board.Board)
		}
	}
	if text == nil {
		if a.plate != nil {
			a.plate.hide()
		}
		return
	}
	if a.plate == nil {
		p, err := newPlateWindow(a.press)
		if err != nil {
			return
		}
		a.plate = p
	}
	a.plate.apply(text)
}

// shutdown убирает окна и перехват клавиш.
func (a *app) shutdown() {
	if a.lock != nil {
		a.lock.RemoveKeyboardGuard()
		procDestroyWindow.Call(a.lock.hwnd)
		a.lock.Close()
	}
	if a.ui != nil {
		a.ui.close()
	}
	if a.plate != nil {
		a.plate.close()
	}
	if a.board != nil {
		a.board.Close()
	}
	if a.host != 0 {
		procDestroyWindow.Call(a.host)
	}
}

// notifyClosing сообщает службе, что страж закрывается штатно: это не обход (FB-1, R17, FR-27).
// Ошибки и опоздание службы не важны, конец сеанса от них не задерживается.
func (a *app) notifyClosing() {
	if a == nil || a.ctl == nil {
		return
	}
	safely(func() { a.ctl.NotifyClosing(closingTimeout) })
}

// endSession: сеанс заканчивается (WM_ENDSESSION с wParam != 0) или это изображает тестовая
// команда endSession: сообщить службе о штатном закрытии и завершиться. Вызывается в потоке окон.
func (a *app) endSession() {
	a.notifyClosing()
	procPostQuitMessage.Call(0)
}

// hostProc — оконная процедура скрытого окна: таймер, сигнал опроса, конец сеанса.
func hostProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := theApp
	switch uint32(msg) {
	case wmTimer, wmAppRefresh:
		if a != nil && a.ctl != nil {
			safely(a.tick)
		}
		return 0
	case wmQueryEndSession:
		// Сеанс спрашивает, можно ли заканчиваться: возражать нельзя (NFR-10), но службе
		// сообщаем сразу, потом процесс может не успеть. Если конец сеанса отменят, служба
		// не пишет обход ещё полминуты, а страж возвращается сразу.
		a.notifyClosing()
		return 1
	case wmEndSession:
		if wParam != 0 {
			if a != nil {
				a.endSession()
			} else {
				procPostQuitMessage.Call(0)
			}
		} else if a != nil && a.ctl != nil {
			a.ctl.ResetClosing() // конец сеанса отменён: при следующем уведомить службу снова
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
