package version

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// whatsNewPath — каталог «Что нового» во фронте (FR-36) относительно пакета.
var whatsNewPath = filepath.Join("..", "..", "frontend", "src", "whatsnew", "entries.ts")

// whatsNewVersions — версии всех записей каталога «Что нового».
func whatsNewVersions(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(whatsNewPath)
	if err != nil {
		t.Fatalf("не прочитан каталог «Что нового»: %v", err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`version:\s*"([^"]+)"`).FindAllStringSubmatch(string(raw), -1) {
		out = append(out, m[1])
	}
	return out
}

func hasVersion(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Ловит: вышла новая версия, а в «Что нового» нет её записи (ТЗ 8.3.9 п.2: каждая версия добавляет запись).
func TestWhatsNewHasCurrentVersion(t *testing.T) {
	versions := whatsNewVersions(t)
	if len(versions) == 0 {
		t.Fatalf("в %s не найдено ни одной записи", whatsNewPath)
	}
	if !hasVersion(versions, Version) {
		t.Fatalf("в «Что нового» нет записи версии %s (есть %v): добавьте её сверху в %s", Version, versions, whatsNewPath)
	}
}
