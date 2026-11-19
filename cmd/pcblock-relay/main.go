// Package main — сервер связи pcblock-relay компьютера и телефона PC Block (протокол
// docs/07_Протокол_связи.md, раздел 3). Почтовый ящик: хранит учётные записи (только хэши
// токенов) и пересылает сообщения, не понимая их содержимого. Только стандартная библиотека.
//
//	pcblock-relay -addr :8080 -data /data -trust-proxy
//	pcblock-relay -health http://127.0.0.1:8080/healthz   (проверка здоровья: выход 0 или 1)
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "адрес, на котором слушать")
	data := flag.String("data", "/data", "папка данных (relay.json)")
	trust := flag.Bool("trust-proxy", false, "брать адрес клиента из X-Forwarded-For (за Traefik)")
	health := flag.String("health", "", "проверить здоровье по адресу и выйти (для HEALTHCHECK)")
	flag.Parse()

	if *health != "" {
		os.Exit(healthCheck(*health))
	}

	logger := log.New(os.Stderr, "", log.LstdFlags|log.LUTC)
	s, err := New(Config{DataDir: *data, TrustProxy: *trust, Log: logger})
	if err != nil {
		logger.Fatalf("запуск: %v", err)
	}

	// Сигнал остановки отменяет базовый контекст: длинные опросы возвращаются сразу,
	// и Shutdown укладывается в 5 с.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	srv := newHTTPServer(*addr, s.Handler(), ctx)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Printf("pcblock-relay слушает %s, данные %s, trust-proxy=%v", *addr, *data, *trust)

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("сервер: %v", err)
		}
	case <-ctx.Done():
		logger.Printf("остановка")
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			logger.Printf("остановка: %v", err)
		}
	}
}

// newHTTPServer — HTTP-сервер со сроками. ReadTimeout считает чтение запроса (заголовки и тело
// ≤ 16 КБ), а не ответ: длинный опрос 25 с им не обрывается. WriteTimeout нет: опрос сам
// ограничен 25 с, ожидание ответа компьютера — 10 с.
func newHTTPServer(addr string, h http.Handler, ctx context.Context) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
}

// healthCheck — GET по адресу: 0, если ответ 200, иначе 1.
func healthCheck(url string) int {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
