package app

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed list-google-default.txt
var googleListDefault string

const googleListFile = "list-google.txt"

func (a *App) ensureGoogleList() {
	dir := a.cfg.ListsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.log.Warn("lists", "mkdir: "+err.Error())
		return
	}
	path := filepath.Join(dir, googleListFile)
	if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
		return
	}
	if err := os.WriteFile(path, []byte(googleListDefault), 0o644); err != nil {
		a.log.Warn("lists", "create "+googleListFile+": "+err.Error())
		return
	}
	a.log.Info("lists", "created "+googleListFile)
}
