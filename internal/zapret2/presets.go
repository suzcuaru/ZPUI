package zapret2

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed presets/*.cmd
var presetFS embed.FS

func (m *Manager) writePresets(destDir string) error {
	entries, err := presetFS.ReadDir("presets")
	if err != nil {
		return fmt.Errorf("read embedded presets: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := presetFS.ReadFile("presets/" + e.Name())
		if err != nil {
			continue
		}
		dest := filepath.Join(destDir, e.Name())
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			m.log.Warn("presets", "Failed to write "+e.Name()+": "+err.Error())
		}
	}

	return nil
}