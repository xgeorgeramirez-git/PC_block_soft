package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pcblock/internal/sites"
)

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGz(t *testing.T, path, s string) {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	if _, err := z.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, b.String())
}

func openOut(t *testing.T, path string) sites.AdultList {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := sites.OpenAdult(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRunSmallInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "domains"), "# комментарий\n\nExample.COM\r\nwww.example.com\nporn.test\nкотики видео\nпример.рф\n")
	writeGz(t, filepath.Join(dir, "domains.2.gz"), "gz.test\n")
	writeFile(t, filepath.Join(dir, "urls"), "must.not.read\n")
	out := filepath.Join(t.TempDir(), "adult.bin")
	sample := filepath.Join(t.TempDir(), "sample.txt")
	var stdout bytes.Buffer
	if err := run([]string{"-src", dir, "-out", out, "-sample", sample}, &stdout); err != nil {
		t.Fatal(err)
	}
	a := openOut(t, out)
	if a.Count() != 4 {
		t.Errorf("Count = %d, want 4", a.Count())
	}
	for _, d := range []string{"example.com", "porn.test", "xn--e1afmkfd.xn--p1ai", "gz.test"} {
		if !a.Contains(d) {
			t.Errorf("%s не в списке", d)
		}
	}
	if a.Contains("must.not.read") {
		t.Error("прочитан файл urls")
	}
	if !strings.Contains(stdout.String(), "строк 6, пропущено 1, записей 4") {
		t.Errorf("итог: %q", stdout.String())
	}
	if !strings.HasPrefix(stdout.String(), "::notice title=Список 18+::") {
		t.Errorf("итог не пометка: %q", stdout.String())
	}
	s, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(s)), "\n"); len(got) != 3 || got[0] != "example.com" || got[2] != "gz.test" {
		t.Errorf("образец %q", got)
	}
}

// Зеркало UT1 режет файл на части по 50 МБ: domains.0, domains.1, … склеиваются по порядку
// номеров, строка может быть разрезана на границе.
func TestRunSplitParts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "domains.10"), "ten.test\n")
	writeFile(t, filepath.Join(dir, "domains.1"), "it.test\nnine-")
	writeFile(t, filepath.Join(dir, "domains.0"), "a.test\nspl")
	writeFile(t, filepath.Join(dir, "domains.2"), "x.test")
	writeFile(t, filepath.Join(dir, "domains.3"), "\nb.test\n")
	out := filepath.Join(t.TempDir(), "adult.bin")
	var stdout bytes.Buffer
	if err := run([]string{"-src", dir, "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	a := openOut(t, out)
	for _, d := range []string{"a.test", "split.test", "nine-x.test", "b.test", "ten.test"} {
		if !a.Contains(d) {
			t.Errorf("%s не в списке", d)
		}
	}
	if a.Count() != 5 {
		t.Errorf("Count = %d, want 5", a.Count())
	}
}

// Ruling RM4-3а: голые зоны верхнего уровня не теряются, а идут в список зон.
func TestRunZones(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "domains"), "porn.test\nxxx\nPORN\nsex.\nadult\nxxx\n123\n")
	out := filepath.Join(t.TempDir(), "adult.bin")
	var stdout bytes.Buffer
	if err := run([]string{"-src", dir, "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "строк 7, пропущено 1, записей 1, зон 4, размер") {
		t.Errorf("итог: %q", stdout.String())
	}
	a := openOut(t, out)
	if a.ZoneCount() != 4 {
		t.Errorf("ZoneCount = %d, want 4", a.ZoneCount())
	}
	for _, c := range []struct {
		d    string
		want bool
	}{{"a.xxx", true}, {"b.porn", true}, {"c.sex", true}, {"d.adult", true}, {"xxx.com", false}, {"porn.test", true}} {
		if got := a.Contains(c.d); got != c.want {
			t.Errorf("Contains(%q) = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestRunNoDomains(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "urls"), "a.test\n")
	out := filepath.Join(t.TempDir(), "adult.bin")
	if err := run([]string{"-src", dir, "-out", out}, &bytes.Buffer{}); err == nil {
		t.Error("папка без domains*: нет ошибки")
	}
	writeFile(t, filepath.Join(dir, "domains"), "# только комментарий\nкотики\n")
	if err := run([]string{"-src", dir, "-out", out}, &bytes.Buffer{}); err == nil {
		t.Error("ноль доменов: нет ошибки")
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("при ошибке файл записан")
	}
}

func TestRunEmpty(t *testing.T) {
	out := filepath.Join(t.TempDir(), "adult.bin")
	var stdout bytes.Buffer
	if err := run([]string{"-empty", "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 20 || string(data) != string(sites.EncodeAdult(nil)) {
		t.Errorf("пустой файл %q", data)
	}
}

func TestRunBadArgs(t *testing.T) {
	cases := [][]string{
		{"-out", filepath.Join(t.TempDir(), "x")}, // нет -src
		{"-src", t.TempDir()},                     // нет -out
		{"-src", filepath.Join(t.TempDir(), "нет"), "-out", filepath.Join(t.TempDir(), "x")},
		{"-неизвестный"},
	}
	for _, args := range cases {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Errorf("run(%q): нет ошибки", args)
		}
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "domains.gz"), "не gzip")
	if err := run([]string{"-src", dir, "-out", filepath.Join(t.TempDir(), "x")}, &bytes.Buffer{}); err == nil {
		t.Error("испорченный gzip: нет ошибки")
	}
}
