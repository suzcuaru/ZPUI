package app

import (
	"os"
	"path/filepath"
	"testing"

	"zpui/internal/config"
	"zpui/internal/logger"
)

// Anti-клоббер: SetConfig не должен затирать server.url/account_token
// пустыми строками из фронтенд-снимков конфига; легитимная очистка token —
// только AccountLogout (и 401-обработка).
func TestSetConfigServerAntiClobber(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	// logdb держит файл открытым дольше жизни теста — отдельный каталог вне
	// t.TempDir(), чтобы автоочистка не падала на занятом дескрипторе.
	logDir, err := os.MkdirTemp("", "zpui-test-logdb")
	if err != nil {
		t.Fatalf("logdb dir: %v", err)
	}
	logMgr, err := logger.New(dir, logDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	defer logMgr.Close()
	a := &App{cfg: cfg, log: logMgr}

	seed := config.ServerConfig{
		URL:          "https://srv.example",
		AccountToken: "tok-abc",
		AccountEmail: "user@example.com",
	}
	if err := cfg.UpdateServerConfig(func(s *config.ServerConfig) { *s = seed }); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if res := a.SetConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"url":           "",
			"account_token": "",
		},
	}); isErrorResp(res) {
		t.Fatalf("SetConfig: %v", res["error"])
	}

	sc := cfg.GetServerConfig()
	if sc.URL != "https://srv.example" {
		t.Errorf("url clobbered: %q", sc.URL)
	}
	if sc.AccountToken != "tok-abc" {
		t.Errorf("account_token clobbered: %q", sc.AccountToken)
	}
	if sc.AccountEmail != "user@example.com" {
		t.Errorf("account_email clobbered: %q", sc.AccountEmail)
	}

	// Непустой url (эхо фронта) сохраняется.
	if res := a.SetConfig(map[string]interface{}{
		"server": map[string]interface{}{"url": "https://other.example"},
	}); isErrorResp(res) {
		t.Fatalf("SetConfig non-empty url: %v", res["error"])
	}
	if got := cfg.GetServerConfig().URL; got != "https://other.example" {
		t.Errorf("non-empty url not saved: %q", got)
	}

	// Легитимная очистка token — V4Logout (v4).
	if res := a.V4Logout(); isErrorResp(res) {
		t.Fatalf("AccountLogout: %v", res["error"])
	}
	sc = cfg.GetServerConfig()
	if sc.AccountToken != "" || sc.AccountEmail != "" {
		t.Errorf("logout must clear token/email, got %q/%q", sc.AccountToken, sc.AccountEmail)
	}
	if sc.URL != "https://other.example" {
		t.Errorf("logout must not touch url: %q", sc.URL)
	}
}
