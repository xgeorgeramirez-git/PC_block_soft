package main

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	l := newLimiter(3, time.Minute)
	now := time.Unix(0, 0)
	for i := 0; i < 3; i++ {
		if l.blocked("a", now) {
			t.Fatalf("рано заблокирован на %d", i)
		}
		l.hit("a", now.Add(time.Duration(i)*time.Second))
	}
	if !l.blocked("a", now.Add(59*time.Second)) {
		t.Fatal("не заблокирован внутри окна")
	}
	if l.blocked("b", now) {
		t.Fatal("ключи не раздельны")
	}
	if l.blocked("a", now.Add(time.Minute)) {
		t.Fatal("не снят в конце окна")
	}
}

func TestLimiterBoundedMemory(t *testing.T) {
	l := newLimiter(1, time.Minute)
	now := time.Unix(0, 0)
	for i := 0; i < 5000; i++ {
		l.hit(fmt.Sprint(i), now)
	}
	if len(l.m) > limiterMaxKeys {
		t.Fatalf("ключей %d больше %d", len(l.m), limiterMaxKeys)
	}
	now = now.Add(2 * time.Minute)
	l.hit("new", now)
	for i := 0; i < limiterMaxKeys; i++ {
		l.hit(fmt.Sprint("z", i), now)
	}
	if len(l.m) > limiterMaxKeys {
		t.Fatalf("ключей %d", len(l.m))
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		remote, xff string
		trust       bool
		want        string
	}{
		{"1.2.3.4:5", "", false, "1.2.3.4"},
		{"1.2.3.4:5", "9.9.9.9", false, "1.2.3.4"},
		{"1.2.3.4:5", "9.9.9.9", true, "9.9.9.9"},
		{"1.2.3.4:5", "8.8.8.8, 9.9.9.9 ", true, "9.9.9.9"},
		{"1.2.3.4:5", "", true, "1.2.3.4"},
		{"[::1]:5", "", false, "::1"},
		{"weird", "", false, "weird"},
	}
	for _, c := range cases {
		if got := clientIP(c.remote, c.xff, c.trust); got != c.want {
			t.Errorf("clientIP(%q,%q,%v)=%q, ждали %q", c.remote, c.xff, c.trust, got, c.want)
		}
	}
}
