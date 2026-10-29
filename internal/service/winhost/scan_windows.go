//go:build windows

package winhost

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"pcblock/internal/programs"
)

// machineFolders — папки компьютера из KnownFolders и каталог установки PC Block, развёрнутые
// так же, как пути процессов (canonicalPath): иначе снимок и наблюдатель увидели бы один файл
// под разными путями.
type machineFolders struct {
	windows         string // «C:\Windows»
	programFiles    string // «C:\Program Files»
	programFilesX86 string // «C:\Program Files (x86)»
	installDir      string // папка PCBlock.exe
}

// readMachineFolders читает папки (SHGetKnownFolderPath; не прочиталось — переменная окружения).
func readMachineFolders() machineFolders {
	known := func(id *windows.KNOWNFOLDERID, env string) string {
		p, err := windows.KnownFolderPath(id, 0)
		if err != nil || p == "" {
			p = os.Getenv(env)
		}
		return canonicalDir(p)
	}
	f := machineFolders{
		windows:         known(windows.FOLDERID_Windows, "SystemRoot"),
		programFiles:    known(windows.FOLDERID_ProgramFiles, "ProgramFiles"),
		programFilesX86: known(windows.FOLDERID_ProgramFilesX86, "ProgramFiles(x86)"),
	}
	if exe, err := exePath(); err == nil {
		f.installDir = canonicalDir(filepath.Dir(exe))
	}
	return f
}

// canonicalDir — развёрнутый путь папки без хвостового «\»; не развернулся — как есть.
func canonicalDir(p string) string {
	if p == "" {
		return ""
	}
	if c, ok := canonicalPath(p); ok {
		p = c
	}
	if len(p) > 3 {
		p = strings.TrimSuffix(p, `\`)
	}
	return p
}

// roots — корни классификатора (programs.Normalize) для службы и наблюдателя (RM3-1).
func (f machineFolders) roots() programs.Roots {
	return programs.Roots{
		SystemRoot:      programs.Normalize(f.windows),
		ProgramFiles:    programs.Normalize(f.programFiles),
		ProgramFilesX86: programs.Normalize(f.programFilesX86),
		InstallDir:      programs.Normalize(f.installDir),
	}
}

// newWinScanner — сканер программ этого компьютера (service.Scanner).
func newWinScanner(f machineFolders) *programScanner {
	pfs := []string{f.programFiles}
	if f.programFilesX86 != "" && !strings.EqualFold(f.programFilesX86, f.programFiles) {
		pfs = append(pfs, f.programFilesX86)
	}
	store := ""
	if f.programFiles != "" {
		store = f.programFiles + `\WindowsApps`
	}
	return &programScanner{
		programFiles: pfs,
		store:        store,
		installDir:   programs.Normalize(f.installDir),
		cat:          programs.DefaultCatalog,
		fs:           osScanFS{},
		meta:         fileMeta,
		timeout:      scanTimeout,
		grace:        scanGrace,
		maxEntries:   scanMaxEntries,
		maxFiles:     scanMaxFiles,
	}
}

// osScanFS — scanFS над os.ReadDir. Ссылки и junction (ModeSymlink, ModeIrregular) не отдаются:
// сканер по ним не ходит (петли, сетевые папки).
type osScanFS struct{}

func (osScanFS) ReadDir(dir string) ([]scanEntry, error) {
	ents, err := os.ReadDir(dir)
	if err != nil && len(ents) == 0 {
		return nil, err
	}
	out := make([]scanEntry, 0, len(ents))
	for _, e := range ents {
		t := e.Type()
		switch {
		case t&(fs.ModeSymlink|fs.ModeIrregular) != 0:
		case e.IsDir():
			out = append(out, scanEntry{name: e.Name(), dir: true})
		case t.IsRegular():
			var size int64
			if isExe(e.Name()) {
				if fi, err := e.Info(); err == nil {
					size = fi.Size()
				}
			}
			out = append(out, scanEntry{name: e.Name(), size: size})
		}
	}
	return out, nil
}

func (osScanFS) ReadFile(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("файл слишком большой")
	}
	return data, nil
}
