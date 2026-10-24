//go:build windows

package winhost

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/service"
	"pcblock/internal/version"
)

// host — запущенная служба: логика на портах, цикл учёта, сервер канала связи и журнал работы.
type host struct {
	svc    *service.Service
	store  service.Store
	rlog   *service.RotatingLog
	logf   func(string, ...any)
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// openHost собирает службу: читает тестовый режим, открывает базу и журнал работы, строит
// порты, запускает цикл и сервер канала. Ошибка здесь значит, что служба не стартует; текст
// пишется в журнал запуска (boot.log), пока журнал работы может быть ещё недоступен.
func openHost() (*host, error) {
	tm, tmErr := readTestMode()
	dataDir := defaultDataDir()
	if tmErr == nil && tm.DataDir != "" {
		dataDir = tm.DataDir
	}
	if err := ensureDataDir(dataDir); err != nil {
		appendBootLog(defaultDataDir(), err.Error())
		return nil, err
	}
	rl, err := service.OpenRotatingLog(logDir(dataDir), logBaseName, logMaxBytes, logFiles)
	if err != nil {
		appendBootLog(dataDir, fmt.Sprintf("журнал работы не открыт: %v", err))
		return nil, err
	}
	lg := log.New(rl, "", log.LstdFlags)
	logf := func(format string, args ...any) { lg.Printf(format, args...) }
	fail := func(err error) (*host, error) {
		logf("Служба не запустилась: %v", err)
		appendBootLog(dataDir, fmt.Sprintf("служба не запустилась: %v", err))
		_ = rl.Close()
		return nil, err
	}
	if tmErr != nil {
		return fail(tmErr)
	}

	exe, err := exePath()
	if err != nil {
		return fail(err)
	}
	clock := tm.NewClock(time.Now)
	res, err := service.OpenStore(storeOpener{}, dbPath(dataDir), time.Now, logf)
	if err != nil {
		return fail(fmt.Errorf("база данных не открыта: %w", err))
	}
	admins := adminChecker{}
	s, err := service.New(service.Config{
		Store:      res.Store,
		Hasher:     hasher{},
		Launcher:   launcher{exe: exe},
		OpenFolder: openFolder,
		Admins:     admins,
		Accounts:   accountDir{admins: admins},
		Policy:     &policy{},
		Clock:      clock,
		Loc:        time.Local,
		Version:    version.Version,
		DataDir:    dataDir,
		Test:       tm,
		Recovered:  res.Recovered,
		BrokenCopy: res.BrokenCopy,
		Logf:       logf,
	})
	if err != nil {
		_ = res.Store.Close()
		return fail(err)
	}
	logf("Служба PC Block %s запускается, папка данных %s, программа %s", version.Version, dataDir, exe)
	if err := s.Start(time.Now()); err != nil {
		_ = res.Store.Close()
		return fail(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	h := &host{svc: s, store: res.Store, rlog: rl, logf: logf, cancel: cancel}
	h.wg.Add(2)
	go func() {
		defer h.wg.Done()
		s.RunLoop(ctx)
	}()
	go func() {
		defer h.wg.Done()
		h.serve(ctx)
	}()
	return h, nil
}

// serve держит сервер канала связи; если он остановился не по просьбе, поднимает заново
// (пока канала нет, страж и значок живут без связи со службой, Ruling 15).
func (h *host) serve(ctx context.Context) {
	handler := func(p ipc.Peer, m ipc.Msg) ipc.Reply { return h.svc.Handle(p, m, time.Now()) }
	var lastMsg string
	var lastAt time.Time
	for ctx.Err() == nil {
		err := ipc.Serve(ctx, ipc.PipeName, handler)
		if err == nil || ctx.Err() != nil {
			return
		}
		if msg := err.Error(); msg != lastMsg || time.Since(lastAt) > time.Minute {
			h.logf("Канал связи остановился: %v. Попробую открыть заново", err)
			lastMsg, lastAt = msg, time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// onSession сообщает службе о событии сеанса (вход, выход, подключение).
func (h *host) onSession(ev service.SessionEvent) {
	go h.svc.OnSession(ev, 0, time.Now())
}

// stop останавливает цикл и канал, сохраняет состояние, завершает значки, стражей и окна во
// всех сеансах (Ruling 15) и закрывает базу.
func (h *host) stop() {
	h.cancel()
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		h.logf("Цикл или канал не остановились за 5 с, продолжаю остановку")
	}
	h.svc.Shutdown(time.Now())
	if err := h.store.Close(); err != nil {
		h.logf("База не закрыта: %v", err)
	}
	h.logf("Служба остановлена")
	_ = h.rlog.Close()
}
