// Command adultlist строит встроенный список «18+» internal/sites/adult/adult.bin из файлов domains* базы UT1 (RM4-3).
//
// Запускается только в CI (шаг windows.yml «Полный список 18+»): загрузку зеркала UT1 делает git
// в шаге, сам инструмент в сеть не ходит (NFR-1). Пустой файл формата для репозитория:
//
//	go run ./tools/adultlist -empty -out internal/sites/adult/adult.bin
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"pcblock/internal/sites"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "::error title=Список 18+::%v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("adultlist", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	src := fs.String("src", "", "папка с файлами domains* (UT1 blacklists/adult)")
	out := fs.String("out", "", "файл adult.bin")
	sample := fs.String("sample", "", "файл для трёх доменов входа: первый, средний, последний")
	empty := fs.Bool("empty", false, "записать пустой файл формата")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("не задан -out")
	}
	if *empty {
		return os.WriteFile(*out, sites.EncodeAdult(nil), 0o644)
	}
	if *src == "" {
		return errors.New("не задан -src")
	}
	files, err := domainFiles(*src)
	if err != nil {
		return err
	}
	st, err := readDomains(files)
	if err != nil {
		return err
	}
	if len(st.domains) == 0 {
		return fmt.Errorf("в %s нет ни одного домена (строк %d, пропущено %d)", *src, st.lines, st.skipped)
	}
	data := sites.EncodeAdult(st.domains)
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return err
	}
	if *sample != "" {
		d := st.domains
		s := d[0] + "\n" + d[len(d)/2] + "\n" + d[len(d)-1] + "\n"
		if err := os.WriteFile(*sample, []byte(s), 0o644); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(stdout, "::notice title=Список 18+::строк %d, пропущено %d, записей %d, размер %d байт\n",
		st.lines, st.skipped, binary.LittleEndian.Uint32(data[12:]), len(data))
	return err
}

// domainFiles — файлы domains* папки по порядку частей: «domains», затем «domains.0», «domains.1»…
// по номеру (зеркало режет файл на части, строка может быть разрезана на границе), остальные — по имени.
func domainFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type part struct {
		name string
		num  int
	}
	var parts []part
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "domains") {
			continue
		}
		rest := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "domains"), ".gz")
		num := -1
		if rest != "" {
			n, err := strconv.Atoi(strings.TrimPrefix(rest, "."))
			if err != nil || !strings.HasPrefix(rest, ".") {
				n = 1 << 30
			}
			num = n
		}
		parts = append(parts, part{e.Name(), num})
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("в %s нет файлов domains*", dir)
	}
	slices.SortFunc(parts, func(a, b part) int {
		if a.num != b.num {
			return a.num - b.num
		}
		return strings.Compare(a.name, b.name)
	})
	files := make([]string, len(parts))
	for i, p := range parts {
		files[i] = filepath.Join(dir, p.name)
	}
	return files, nil
}

type stats struct {
	lines, skipped int
	domains        []string
}

// readDomains читает части подряд как один поток: непустые строки без «#» — кандидаты, домен —
// sites.NormalizeInput; не домен — строка пропущена и посчитана.
func readDomains(files []string) (stats, error) {
	var st stats
	readers := make([]io.Reader, 0, len(files))
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return st, err
		}
		defer fh.Close()
		var r io.Reader = fh
		if strings.HasSuffix(f, ".gz") {
			z, err := gzip.NewReader(fh)
			if err != nil {
				return st, fmt.Errorf("%s: %w", filepath.Base(f), err)
			}
			defer z.Close()
			r = z
		}
		readers = append(readers, r)
	}
	sc := bufio.NewScanner(io.MultiReader(readers...))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		st.lines++
		d, err := sites.NormalizeInput(line)
		if err != nil {
			st.skipped++
			continue
		}
		st.domains = append(st.domains, d)
	}
	return st, sc.Err()
}
