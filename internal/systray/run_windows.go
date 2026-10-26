//go:build windows

package systray

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
)

const (
	wmTrayCallback = wmApp + 1 // действие пользователя со значком
	wmStatus       = wmApp + 2 // опрос службы получил ответ или ошибку
	wmOpenDone     = wmApp + 3 // служба ответила на просьбу открыть окно
	wmTestOpenCard = wmApp + 4 // тестовый режим: открыть табло без мыши (testhook_windows.go)

	hostTimerID    = 5
	hostTimerEvery = 2000 // мс: повторить добавление значка, пока оболочка не готова

	dialTimeout    = 2 * time.Second // сколько ждать канал службы за один заход
	closingTimeout = 2 * time.Second // сколько ждать службу при выходе из сеанса
)

var (
	hostProcPtr uintptr
	theApp      *app
)

func init() { hostProcPtr = windows.NewCallback(hostProc) }

// app — значок сеанса: опрос службы в своей горутине, окна в потоке окон
// (runtime.LockOSThread в Run).
type app struct {
	host, hInst    uintptr
	taskbarCreated uint32

	tracker Tracker
	caller  *lazyCaller
	kick    chan struct{}

	icon  *trayIcon
	panel *panel // мини-панель взрослого сеанса (ТЗ 8.2.1 п.7)
	card  *card  // табло остатка в детском сеансе (D-53)

	cardOpen atomic.Bool // пока табло открыто, служба опрашивается раз в секунду

	mu       sync.Mutex
	note     string // итог последней просьбы открыть окно, если не получилось
	notChild bool
}

// lazyCaller открывает канал службы при первом запросе и после обрыва: значок обязан пережить
// запуск раньше службы и её перезапуск.
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

// singleton занимает именованный мьютекс. already = true: такой значок в сеансе уже есть.
func singleton(name string) (release func(), already bool, err error) {
	h, err := windows.CreateMutex(nil, false, wstr(name))
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

// safely выполняет f и не даёт панике убить значок: гибель значка без сообщения службе
// считается обходом (R17в), а ошибка одного кадра обходом не является.
func safely(f func()) {
	defer func() { _ = recover() }()
	f()
}

// Run — значок у часов в сеансе пользователя (FR-7, FR-8). Занимается мьютекс
// Global\PCBlockTray-<сеанс> (повторный запуск молча завершается), раз в PollEvery идёт опрос
// службы, поток окон держит значок и мини-панель. Возвращается, когда сеанс завершается.
func Run(opts launch.Options) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	session := sessionOf(opts)
	release, already, err := singleton(`Global\PCBlockTray-` + strconv.FormatUint(uint64(session), 10))
	if err != nil {
		return err
	}
	if already {
		return nil
	}
	defer release()

	procSetProcessDPIAware.Call()
	hInst, _, _ := procGetModuleHandle.Call(0)
	a := &app{hInst: hInst, caller: &lazyCaller{}, kick: make(chan struct{}, 1)}
	theApp = a
	defer func() { theApp = nil }()
	defer a.caller.close()

	cls, err := registerPlainClass("PCBlockTrayHost", hostProcPtr, hInst, 0)
	if err != nil {
		return fmt.Errorf("RegisterClassEx(значок): %w", err)
	}
	// Обычное скрытое окно, а не окно-сообщение: окна-сообщения не получают рассылок
	// TaskbarCreated и WM_ENDSESSION.
	host, _, err := procCreateWindowEx.Call(
		wsExToolWin,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(wstr("PC Block Tray"))),
		wsPopup,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if host == 0 {
		return fmt.Errorf("CreateWindowEx(значок): %w", err)
	}
	a.host = host
	tc, _, _ := procRegisterWindowMessage.Call(uintptr(unsafe.Pointer(wstr("TaskbarCreated"))))
	a.taskbarCreated = uint32(tc)
	a.icon = newTrayIcon(host, wmTrayCallback)
	if p, err := newPanel(a.onPanelButton); err == nil {
		a.panel = p
	}
	if c, err := newCard(a.onCardPress); err == nil {
		c.onHide = func() { a.cardOpen.Store(false) }
		a.card = c
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.pollLoop(stop)
	}()
	procSetTimer.Call(host, hostTimerID, hostTimerEvery, 0)
	stopHook := a.startTestHook(stop)
	defer stopHook()

	a.loop()

	close(stop)
	a.caller.close() // прерывает ожидающий запрос
	wg.Wait()
	a.shutdown()
	return nil
}

// pollLoop опрашивает службу раз в PollEvery (раз в секунду, пока открыто табло) и сразу по
// просьбе (Kick).
func (a *app) pollLoop(stop <-chan struct{}) {
	for {
		safely(a.pollOnce)
		t := time.NewTimer(PollInterval(a.cardOpen.Load()))
		select {
		case <-stop:
			t.Stop()
			return
		case <-t.C:
		case <-a.kick:
			t.Stop()
		}
	}
}

func (a *app) pollOnce() {
	var st ipc.ChildStatus
	if err := ipc.Call(a.caller, ipc.CmdGetChildStatus, "", nil, &st); err != nil {
		a.tracker.OnError()
	} else {
		a.tracker.OnStatus(time.Now(), st)
		a.mu.Lock()
		a.notChild = st.Status == ipc.StatusNotChild
		a.mu.Unlock()
	}
	procPostMessage.Call(a.host, wmStatus, 0, 0)
}

// kickPoll просит опрос немедленно.
func (a *app) kickPoll() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// loop крутит цикл сообщений потока окон.
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

// filter обрабатывает клавиши открытой панели: Esc закрывает, Enter нажимает кнопку, Tab и
// стрелки переходят между кнопками. true: сообщение разобрано.
func (a *app) filter(m *tagMSG) bool {
	p := a.panel
	if p == nil || !p.visible {
		return false
	}
	if m.hwnd != p.hwnd {
		if in, _, _ := procIsChild.Call(p.hwnd, m.hwnd); in == 0 {
			return false
		}
	}
	if m.message == wmKeyDown {
		switch m.wParam {
		case vkEscape:
			p.hide()
			return true
		case vkReturn:
			safely(p.activateFocused)
			return true
		}
	}
	r, _, _ := procIsDialogMessage.Call(p.hwnd, uintptr(unsafe.Pointer(m)))
	return r != 0
}

// refresh приводит значок и открытые панель и табло к последним данным службы.
func (a *app) refresh() {
	now := time.Now()
	a.icon.apply(a.tracker.Display(now))
	if a.panel != nil && a.panel.visible {
		a.panel.setView(BuildPanel(a.tracker.Snapshot(now), a.currentNote()))
	}
	if a.card != nil && a.card.visible {
		a.card.setView(a.cardView(now))
	}
}

// cardView — что показать на табло: по данным службы; если не вышло открыть окно, текст
// отказа с кнопкой «Обновить» (она снимает его).
func (a *app) cardView(now time.Time) CountdownView {
	v := CountdownFor(a.tracker.Snapshot(now))
	if note := a.currentNote(); note != "" && v.Kind != CountdownError {
		return CountdownView{Kind: CountdownError, Title: v.Title, Big: note, Button: textButtonRefresh}
	}
	return v
}

// setBusy запрещает нажатия, пока команда службе в пути.
func (a *app) setBusy(b bool) {
	if a.panel != nil {
		a.panel.setBusy(b)
	}
	if a.card != nil {
		a.card.setBusy(b)
	}
}

// onCardPress выполняет нажатие кнопки табло: стрелка открывает «Мою статистику», «Обновить»
// повторяет запрос к службе.
func (a *app) onCardPress(t CardTarget) {
	switch t {
	case TargetArrow:
		a.onPanelButton(PanelButton{Kind: PanelMyStats})
	case TargetRefresh:
		a.onPanelButton(PanelButton{Kind: PanelRefresh})
	}
}

func (a *app) currentNote() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.note
}

func (a *app) setNote(s string) {
	a.mu.Lock()
	a.note = s
	a.mu.Unlock()
}

func (a *app) isNotChild() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.notChild
}

// togglePanel по щелчку на значке: в детском сеансе открывает табло, во взрослом мини-панель
// (ТЗ 8.2.1 п.7); открытую закрывает.
func (a *app) togglePanel() {
	if a.card != nil && UsesCountdown(a.tracker.Snapshot(time.Now())) {
		a.toggleCard()
		return
	}
	a.togglePanelMini()
}

// toggleCard открывает табло у значка или закрывает открытое.
func (a *app) toggleCard() {
	c := a.card
	if c.visible {
		c.hide()
		return
	}
	a.setNote("")
	a.cardOpen.Store(true) // секундный опрос с этого момента
	a.kickPoll()           // свежие цифры к открытию
	c.setBusy(false)
	c.setView(a.cardView(time.Now()))
	anchor, ok := a.icon.rect()
	c.show(anchor, ok)
	if !c.visible {
		a.cardOpen.Store(false)
	}
}

// togglePanelMini открывает мини-панель у значка или закрывает открытую.
func (a *app) togglePanelMini() {
	p := a.panel
	if p == nil {
		return
	}
	if p.visible {
		p.hide()
		return
	}
	a.setNote("")
	a.kickPoll() // свежие цифры к открытию
	p.setBusy(false)
	p.sig = ""
	p.setView(BuildPanel(a.tracker.Snapshot(time.Now()), ""))
	anchor, ok := a.icon.rect()
	p.show(anchor, ok)
}

// onPanelButton выполняет нажатие кнопки мини-панели.
func (a *app) onPanelButton(b PanelButton) {
	if b.Disabled {
		// Недоступная кнопка объясняет, почему (L-17): показываем пояснение в note.
		a.setNote(b.DisabledNote)
		return
	}
	if b.Kind == PanelRefresh {
		a.setNote("")
		a.kickPoll()
		return
	}
	if b.Kind == PanelSpendBonus {
		a.onSpendBonus()
		return
	}
	if b.Kind == PanelAskMore {
		// «Попросить ещё» открывает окно «Моя статистика», где ребёнок зовёт родителя
		// вводом пароля; значок сам пароль не принимает (мини-панель — нативное окно).
		b.Kind = PanelMyStats
	}
	kind, ok := b.Kind.OpenKind()
	if !ok {
		return
	}
	a.setNote("")
	a.setBusy(true)
	go safely(func() {
		var rep ipc.OpenWindowReply
		err := ipc.Call(a.caller, ipc.CmdOpenWindow, "", ipc.OpenWindowRequest{Kind: kind}, &rep)
		if err != nil {
			text := ""
			var re *ipc.ReplyError
			if errors.As(err, &re) {
				text = re.Text
			}
			a.setNote(OpenFailedText(text, a.isNotChild()))
		}
		ok := uintptr(1)
		if err != nil {
			ok = 0
		}
		procPostMessage.Call(a.host, wmOpenDone, ok, 0)
	})
}

// onSpendBonus тратит из копилки 30 минут (или весь остаток, если его меньше) без пароля
// (FR-11); результат показывает в note. Полный выбор суммы с подтверждением — на экране
// блокировки и в «Моей статистике» (WebView).
func (a *app) onSpendBonus() {
	st := a.tracker.Snapshot(time.Now()).Status
	minutes := 30
	if st.BonusBalance > 0 && st.BonusBalance < minutes {
		minutes = st.BonusBalance
	}
	if minutes <= 0 {
		return
	}
	a.setNote("")
	a.setBusy(true)
	go safely(func() {
		var rep ipc.SpendBonusReply
		// ChildID 0: служба берёт ребёнка этого сеанса (childSelf).
		err := ipc.Call(a.caller, ipc.CmdSpendBonus, "", ipc.SpendBonusRequest{Minutes: minutes}, &rep)
		if err != nil {
			text := ""
			var re *ipc.ReplyError
			if errors.As(err, &re) {
				text = re.Text
			}
			if text == "" {
				text = textPanelError
			}
			a.setNote(text)
		} else {
			a.setNote(rep.Message)
		}
		a.kickPoll()
		procPostMessage.Call(a.host, wmOpenDone, 1, 0)
	})
}

// shutdown убирает значок и окна.
func (a *app) shutdown() {
	if a.icon != nil {
		a.icon.remove()
	}
	if a.panel != nil {
		a.panel.close()
	}
	if a.card != nil {
		a.card.close()
	}
	if a.host != 0 {
		procDestroyWindow.Call(a.host)
	}
}

// notifyClosing сообщает службе, что сеанс завершается штатно: это не обход (FR-27). Отдельное
// соединение и срок в две секунды: сеанс не должен ждать службу.
func notifyClosing() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		c, err := ipc.Dial(ipc.PipeName, time.Second)
		if err != nil {
			return
		}
		defer c.Close()
		_ = ipc.Call(c, ipc.CmdTrayClosing, "", nil, nil)
	}()
	select {
	case <-done:
	case <-time.After(closingTimeout):
	}
}

// hostProc — оконная процедура скрытого окна: сообщения значка, таймер, конец сеанса.
func hostProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := theApp
	if a != nil && a.taskbarCreated != 0 && uint32(msg) == a.taskbarCreated {
		a.icon.forget() // оболочка перезапустилась и забыла значок
		safely(a.refresh)
		return 0
	}
	switch uint32(msg) {
	case wmTrayCallback:
		if a != nil && a.icon.clicked(lParam) {
			safely(a.togglePanel)
		}
		return 0
	case wmStatus, wmTimer:
		if a != nil {
			safely(a.refresh)
		}
		return 0
	case wmOpenDone:
		if a != nil {
			safely(func() {
				a.setBusy(false)
				if wParam != 0 {
					// Окно открывается, панель и табло своё дело сделали.
					if a.panel != nil {
						a.panel.hide()
					}
					if a.card != nil {
						a.card.hide()
					}
					return
				}
				a.refresh()
			})
		}
		return 0
	case wmTestOpenCard:
		if a != nil && a.card != nil && !a.card.visible {
			safely(a.togglePanel)
		}
		return 0
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if wParam != 0 {
			safely(notifyClosing)
			if a != nil {
				a.icon.remove()
			}
			procPostQuitMessage.Call(0)
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
