package themes

// Предустановленная тема-пример: ZIP в том же формате, что импорт галереи
// (theme.json + bg/<файл>). Разворачивается один раз — пока themes.json
// не существует; удалённую пользователем тему не восстанавливаем.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "embed"

	"zpui/internal/logger"
)

//go:embed assets/zpui-theme-ZPUI_2_0.zip
var presetZip []byte

const (
	presetID   = "preset-zpui-2-0"
	presetName = "ZPUI 2.0"
)

func EnsurePreset(s *Store, dataDir string, log *logger.Logger) {
	if _, err := os.Stat(s.path); err == nil {
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(presetZip), int64(len(presetZip)))
	if err != nil {
		log.Error("themes", "preset zip: "+err.Error())
		return
	}
	var theme map[string]any
	for _, f := range zr.File {
		if f.Name != "theme.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			log.Error("themes", "preset theme.json: "+err.Error())
			return
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Error("themes", "preset theme.json: "+err.Error())
			return
		}
		if err := json.Unmarshal(body, &theme); err != nil {
			log.Error("themes", "preset theme.json: "+err.Error())
			return
		}
		break
	}
	if theme == nil {
		log.Error("themes", "preset zip: theme.json not found")
		return
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasPrefix(f.Name, "bg/") {
			continue
		}
		name := filepath.Base(f.Name)
		if !strings.HasSuffix(name, ".jpg") && !strings.HasSuffix(name, ".jpeg") &&
			!strings.HasSuffix(name, ".png") && !strings.HasSuffix(name, ".webp") &&
			!strings.HasSuffix(name, ".gif") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || len(body) == 0 {
			continue
		}
		bgDir := filepath.Join(dataDir, "bg")
		if err := os.MkdirAll(bgDir, 0o755); err != nil {
			log.Error("themes", "preset bg dir: "+err.Error())
			continue
		}
		if err := os.WriteFile(filepath.Join(bgDir, name), body, 0o644); err != nil {
			log.Error("themes", "preset bg file: "+err.Error())
		}
	}
	s.Upsert(presetID, presetName, theme)
	log.Info("themes", "preset theme seeded: "+presetName)
}
