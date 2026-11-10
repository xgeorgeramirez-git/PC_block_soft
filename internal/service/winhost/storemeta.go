package winhost

import (
	"encoding/xml"
	"strconv"
	"strings"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

// Свойства приложений магазина (RM4-11, T6-10): издатель — PublisherDisplayName из
// AppxManifest.xml пакета; ключ программы без версии (programs.Roots.StableKey) разворачивается в
// папку новейшей версии пакета. Без вызовов Windows: файлы читает scanFS, свойства exe — file.

// microsoftPublisherID — код издателя пакетов Microsoft: без манифеста и CompanyName издатель
// «Microsoft Corporation» (RM4-11).
const (
	microsoftPublisherID   = "8wekyb3d8bbwe"
	microsoftPublisherName = "Microsoft Corporation"
)

// metaReader — service.FileMeta с папкой пакетов магазина store (programs.Roots.StoreDir,
// нормализованная; "" — магазина нет): путь вне неё — свойства файла как есть.
type metaReader struct {
	store string
	fs    scanFS
	file  service.FileMeta // свойства exe (fileMeta на Windows)
}

// read — издатель, описание и отпечаток exe path (настоящий путь или ключ магазина). Путь в
// папке пакета: издатель из манифеста этой папки, иначе CompanyName exe, иначе для пакетов
// Microsoft — «Microsoft Corporation»; описание и отпечаток — exe этой папки. Ключ «имя__издатель»:
// папка новейшей версии пакета (pickStoreDir); ни одной — только издатель Microsoft.
func (m metaReader) read(path string) (publisher, description, fingerprint string) {
	p := programs.Normalize(path)
	store := strings.TrimSuffix(m.store, `\`)
	if store == "" || !strings.HasPrefix(p, store+`\`) {
		return m.file(path)
	}
	dir, rest, ok := strings.Cut(p[len(store)+1:], `\`)
	if !ok || rest == "" {
		return m.file(path)
	}
	var pkgDir, pubID string
	switch f := strings.Split(dir, "_"); {
	case len(f) == 5 && f[0] != "" && f[4] != "":
		pkgDir, pubID = dir, f[4]
	case len(f) == 3 && f[0] != "" && f[1] == "" && f[2] != "":
		pubID = f[2]
		if ents, err := m.fs.ReadDir(store, false); err == nil {
			names := make([]string, 0, len(ents))
			for _, e := range ents {
				if e.dir {
					names = append(names, e.name)
				}
			}
			pkgDir = pickStoreDir(names, f[0], f[2])
		}
	default:
		return m.file(path)
	}
	if pkgDir != "" {
		base := store + `\` + pkgDir
		publisher, description, fingerprint = m.file(base + `\` + rest)
		if data, err := m.fs.ReadFile(base+`\`+storeManifest, manifestMaxBytes); err == nil {
			if mp := manifestPublisher(data); mp != "" {
				publisher = mp
			}
		}
	}
	if strings.TrimSpace(publisher) == "" && pubID == microsoftPublisherID {
		publisher = microsoftPublisherName
	}
	return publisher, description, fingerprint
}

// manifestPublisher — PublisherDisplayName из AppxManifest.xml; «» — нет, манифест испорчен или
// значение — ссылка на ресурс «ms-resource:…» (не разворачивается, README «Отклонения от ТЗ»).
func manifestPublisher(data []byte) string {
	var m appxManifest
	if xml.Unmarshal(data, &m) != nil {
		return ""
	}
	pub := strings.TrimSpace(m.Properties.PublisherDisplayName)
	if strings.HasPrefix(strings.ToLower(pub), "ms-resource:") {
		return ""
	}
	return pub
}

// pickStoreDir — папка новейшей версии пакета name издателя pub среди имён папок names
// («<имя>_<версия>_<арх>__<издатель>», регистр не важен): ресурсные пакеты (непустой ресурс)
// не берутся, версии сравниваются по числам «a.b.c.d». Нет подходящей — «».
func pickStoreDir(names []string, name, pub string) string {
	best, bestVer := "", []int(nil)
	for _, n := range names {
		f := strings.Split(strings.ToLower(n), "_")
		if len(f) != 5 || f[0] != name || f[3] != "" || f[4] != pub {
			continue
		}
		ver, ok := parseStoreVersion(f[1])
		if !ok {
			continue
		}
		if best == "" || versionLess(bestVer, ver) {
			best, bestVer = n, ver
		}
	}
	return best
}

// parseStoreVersion — версия пакета «a.b.c.d» числами; ok = false — не числа.
func parseStoreVersion(s string) ([]int, bool) {
	parts := strings.Split(s, ".")
	out := make([]int, len(parts))
	for i, x := range parts {
		n, err := strconv.Atoi(x)
		if err != nil || n < 0 {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

// versionLess — версия a старше b (по числам слева направо; короче — меньше при равном начале).
func versionLess(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
