package main

import (
	"net"
	"strings"
	"time"
)

// limiterMaxKeys — сколько ключей (адресов, компьютеров) помнит один ограничитель.
// При переполнении сначала выбрасываются истёкшие окна, потом самые старые.
const limiterMaxKeys = 4096

// window — фиксированное окно: начало и число событий в нём.
type window struct {
	start time.Time
	n     int
}

// limiter — «не больше max событий за per» по ключу (протокол, раздел 3). Окно начинается
// с первого события и действует до конца: после max событий ключ заблокирован до конца окна.
// Часы передаются снаружи, поэтому тесты идут на поддельном времени. Не потокобезопасен:
// вызывается под мьютексом сервера.
type limiter struct {
	max int
	per time.Duration
	m   map[string]*window
}

func newLimiter(max int, per time.Duration) *limiter {
	return &limiter{max: max, per: per, m: map[string]*window{}}
}

func (l *limiter) get(key string, now time.Time) *window {
	w := l.m[key]
	if w != nil && now.Sub(w.start) >= l.per {
		delete(l.m, key)
		return nil
	}
	return w
}

// blocked — исчерпан ли предел ключа в текущем окне.
func (l *limiter) blocked(key string, now time.Time) bool {
	w := l.get(key, now)
	return w != nil && w.n >= l.max
}

// hit отмечает событие ключа.
func (l *limiter) hit(key string, now time.Time) {
	w := l.get(key, now)
	if w == nil {
		if len(l.m) >= limiterMaxKeys {
			l.sweep(now)
		}
		w = &window{start: now}
		l.m[key] = w
	}
	w.n++
}

// sweep выбрасывает истёкшие окна; если места всё равно нет, освобождает четверть
// (случайные ключи: порядок обхода map случаен), чтобы не обходить карту на каждом событии.
func (l *limiter) sweep(now time.Time) {
	for k, w := range l.m {
		if now.Sub(w.start) >= l.per {
			delete(l.m, k)
		}
	}
	for k := range l.m {
		if len(l.m) < limiterMaxKeys*3/4 {
			break
		}
		delete(l.m, k)
	}
}

// clientIP — адрес клиента для ограничений. За Traefik (trust) — последний элемент
// X-Forwarded-For: его дописал сам Traefik, подделать его клиент не может.
func clientIP(remoteAddr, xff string, trust bool) string {
	if trust && strings.TrimSpace(xff) != "" {
		parts := strings.Split(xff, ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
			return last
		}
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}
