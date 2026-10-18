package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Ловит: номер версии в коде разошёлся с версией в wails.json, из которой
// собирается установщик (exe ушёл бы с одной версией в окне и другой в свойствах файла).
func TestVersionMatchesWailsJSON(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "wails.json"))
	if err != nil {
		t.Fatalf("не прочитан wails.json: %v", err)
	}
	var cfg struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("wails.json не разобран: %v", err)
	}
	if cfg.Info.ProductVersion != Version {
		t.Fatalf("версия в wails.json %q, в коде %q", cfg.Info.ProductVersion, Version)
	}
}

// Ловит: версия не в формате semver (X.Y.Z) — ломает теги выпусков vX.Y.Z.
func TestVersionIsSemver(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("версия %q не в формате X.Y.Z", Version)
	}
}
