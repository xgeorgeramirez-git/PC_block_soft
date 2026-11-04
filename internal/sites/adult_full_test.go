package sites

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	st2Total = 4_600_000
	st2Real  = 10_000
)

// syntheticAdult — список ST2: st2Real хешей доменов d0000000.test… и случайные хеши до st2Total.
func syntheticAdult() string {
	hs := make([]uint64, 0, st2Total)
	for i := 0; i < st2Real; i++ {
		hs = append(hs, HashDomain(fmt.Sprintf("d%07d.test", i)))
	}
	r := rand.New(rand.NewPCG(1, 2))
	for len(hs) < st2Total {
		hs = append(hs, r.Uint64())
	}
	return string(encodeHashes(hs))
}

// p99 — 99-й процентиль времени проверки доменов без кэша.
func p99(a AdultList, domains []string) (time.Duration, int) {
	ds := make([]time.Duration, len(domains))
	found := 0
	for i, d := range domains {
		t0 := time.Now()
		if a.containsNoCache(d) {
			found++
		}
		ds[i] = time.Since(t0)
	}
	slices.Sort(ds)
	return ds[len(ds)*99/100], found
}

// ST2: загрузка списка ≤ 2 с, проверка домена ≤ 1 мс (99-й процентиль) на 4,6 млн записей.
func TestAdultST2Synthetic(t *testing.T) {
	t0 := time.Now()
	data := syntheticAdult()
	build := time.Since(t0)
	t0 = time.Now()
	a, err := OpenAdult(data)
	open := time.Since(t0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("синтетический список: %d записей, %d байт, сборка %v, загрузка %v", a.Count(), len(data), build, open)
	if open > 2*time.Second {
		t.Errorf("загрузка списка %v, ST2 ≤ 2 с", open)
	}
	domains := make([]string, st2Real)
	for i := range domains {
		domains[i] = fmt.Sprintf("d%07d.test", i)
	}
	p, found := p99(a, domains)
	t.Logf("проверка домена: p99 %v", p)
	if p > time.Millisecond {
		t.Errorf("проверка домена p99 %v, ST2 ≤ 1 мс", p)
	}
	if found != st2Real {
		t.Errorf("найдено %d из %d настоящих доменов", found, st2Real)
	}
	for _, d := range []string{"example.com", "google.com"} {
		if a.Contains(d) {
			t.Errorf("Contains(%q) = true: ложное совпадение", d)
		}
	}
}

// Полный встроенный список (windows.yml строит его перед тестами, PCB_ADULT_FULL=1, RM4-3).
func TestAdultListFull(t *testing.T) {
	if os.Getenv("PCB_ADULT_FULL") != "1" {
		t.Skip("PCB_ADULT_FULL не 1: встроен пустой список")
	}
	var facts []string
	defer func() {
		notice := "::notice title=Список 18+::" + strings.Join(facts, ", ")
		if t.Failed() {
			notice = "::error title=Список 18+::" + strings.Join(facts, ", ")
		}
		t.Log(notice)
		if dir := os.Getenv("RUNNER_TEMP"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "pcb-adult.txt"), []byte(notice+"\n"), 0o644); err != nil {
				t.Errorf("итог списка не записан: %v", err)
			}
		}
	}()
	t0 := time.Now()
	a, err := OpenAdult(adultBin)
	open := time.Since(t0)
	facts = append(facts, fmt.Sprintf("размер %d байт", len(adultBin)), fmt.Sprintf("загрузка %v", open))
	if err != nil {
		facts = append(facts, "ошибка "+err.Error())
		t.Fatal(err)
	}
	facts = append(facts, fmt.Sprintf("записей %d", a.Count()))
	if a.Count() < 4_000_000 {
		t.Errorf("записей %d, нужно ≥ 4 000 000", a.Count())
	}
	if open > 2*time.Second {
		t.Errorf("загрузка %v, ST2 ≤ 2 с", open)
	}
	if e, err := Embedded(); err != nil || e.Count() != a.Count() {
		t.Errorf("Embedded() = %d записей, %v", e.Count(), err)
	}
	sample := strings.Split(os.Getenv("PCB_ADULT_SAMPLE"), ",")
	missing := 0
	for _, d := range sample {
		if d = strings.TrimSpace(d); d != "" && !a.Contains(d) {
			t.Errorf("домен образца %q не найден", d)
			missing++
		}
	}
	if len(strings.TrimSpace(os.Getenv("PCB_ADULT_SAMPLE"))) == 0 {
		t.Error("PCB_ADULT_SAMPLE пуст: образец не передан")
	}
	facts = append(facts, fmt.Sprintf("образец %d, не найдено %d", len(sample), missing))
	if a.Contains("example.com") {
		t.Error("example.com в списке 18+")
	}
	domains := make([]string, st2Real)
	for i := range domains {
		domains[i] = fmt.Sprintf("pcb-check-%d.example", i)
	}
	p, _ := p99(a, domains)
	facts = append(facts, fmt.Sprintf("проверка p99 %v", p))
	if p > time.Millisecond {
		t.Errorf("проверка домена p99 %v, ST2 ≤ 1 мс", p)
	}
}

func BenchmarkAdultContains(b *testing.B) {
	a, err := OpenAdult(syntheticAdult())
	if err != nil {
		b.Fatal(err)
	}
	domains := []string{"d0000001.test", "www.d0005000.test", "example.com", "a.b.c.google.com"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		a.containsNoCache(domains[i%len(domains)])
	}
}
