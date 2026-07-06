package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

var version = "1.0.0"

const (
	zapretRepo = "Flowseal/zapret-discord-youtube"
	timeout    = 5 * time.Minute
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	selfDir := getSelfDir()
	moduleDir := filepath.Dir(selfDir)
	zapretDir := filepath.Join(moduleDir, "zapret")

	switch os.Args[1] {
	case "service":
		runService(zapretDir, moduleDir)
	case "install":
		cmdInstall(zapretDir, moduleDir)
	case "uninstall":
		cmdUninstall(zapretDir)
	case "start":
		cmdStart(zapretDir)
	case "stop":
		cmdStop(zapretDir)
	case "restart":
		cmdStop(zapretDir)
		time.Sleep(500 * time.Millisecond)
		cmdStart(zapretDir)
	case "status":
		cmdStatus(zapretDir)
	case "update":
		cmdUpdate(zapretDir)
	case "strategies":
		cmdStrategies(zapretDir)
	case "config":
		cmdConfig(zapretDir, os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "Unknown command:", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Zapret Module v` + version + `
Usage: zapret <command> [args]

Commands:
  service         Run as background daemon (writes status.json)
  install         Download and install zapret-discord-youtube
  uninstall       Remove zapret files
  start           Start zapret (winws.exe)
  stop            Stop zapret
  restart         Restart zapret
  status          Show zapret status
  update          Update zapret to latest version
  strategies      List available strategies
  config get <key>
  config set <key> <value>
  config list`)
}

func getSelfDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func writeStatus(moduleDir string, s map[string]interface{}) {
	s["updated"] = time.Now().Unix()
	data, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(filepath.Join(moduleDir, "status.json"), data, 0644)
}

// ─── service ────────────────────────────────────────────────

func runService(zapretDir, moduleDir string) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()

	for range tick.C {
		st := getZapretState(zapretDir)
		writeStatus(moduleDir, st)
	}
}

func getZapretState(zapretDir string) map[string]interface{} {
	s := map[string]interface{}{
		"zapret_state":   "stopped",
		"zapret_version": "",
		"zapret_error":   "",
	}

	// check installed
	info, err := os.Stat(filepath.Join(zapretDir, "winws.exe"))
	if err != nil || info == nil {
		s["zapret_state"] = "not_installed"
		label := "Zapret: не установлен"
		s["ui"] = map[string]interface{}{
			"statusbar": map[string]interface{}{
				"zapret-status": map[string]interface{}{
					"label": label,
					"color": "#888",
				},
			},
		}
		return s
	}

	s["zapret_version"] = readVersion(zapretDir)

	// check running
	running := isWinwsRunning()
	if running {
		s["zapret_state"] = "running"
		label := fmt.Sprintf("Zapret: ↵ %s", s["zapret_version"])
		s["ui"] = map[string]interface{}{
			"statusbar": map[string]interface{}{
				"zapret-status": map[string]interface{}{
					"label": label,
					"color": "#34d058",
				},
			},
		}
	} else {
		s["zapret_state"] = "stopped"
		label := fmt.Sprintf("Zapret: ⏸ %s", s["zapret_version"])
		s["ui"] = map[string]interface{}{
			"statusbar": map[string]interface{}{
				"zapret-status": map[string]interface{}{
					"label": label,
					"color": "#888",
				},
			},
		}
	}

	return s
}

// ─── install ────────────────────────────────────────────────

func cmdInstall(zapretDir, moduleDir string) {
	writeStatus(moduleDir, map[string]interface{}{"zapret_state": "installing", "zapret_error": ""})
	fmt.Println("Installing zapret-discord-youtube...")

	version, err := fetchLatestVersion()
	if err != nil {
		fail("fetch version: %v", err)
	}
	fmt.Println("Latest version:", version)

	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/zapret-discord-youtube-%s.zip", zapretRepo, version, version)
	zipPath := filepath.Join(zapretDir, "zapret.zip")
	if err := downloadFile(downloadURL, zipPath); err != nil {
		// try alt URL pattern
		downloadURL = fmt.Sprintf("https://github.com/%s/archive/refs/tags/%s.zip", zapretRepo, version)
		if err2 := downloadFile(downloadURL, zipPath); err2 != nil {
			os.RemoveAll(zapretDir)
			fail("download failed: %v (alt: %v)", err, err2)
		}
	}

	if err := extractZip(zipPath, zapretDir); err != nil {
		os.RemoveAll(zapretDir)
		fail("extract: %v", err)
	}
	os.Remove(zipPath)
	fmt.Println("Installed successfully")
	writeStatus(moduleDir, getZapretState(zapretDir))
}

func fetchLatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", zapretRepo)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ZPUI-Zapret-Module/"+version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}

func downloadFile(url, dest string) error {
	fmt.Println("Downloading:", url)
	_ = os.MkdirAll(filepath.Dir(dest), 0755)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "ZPUI-Zapret-Module/"+version)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest + ".tmp")
	if err != nil {
		return err
	}
	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(dest + ".tmp")
		return err
	}
	if written == 0 {
		os.Remove(dest + ".tmp")
		return fmt.Errorf("empty file")
	}
	return os.Rename(dest+".tmp", dest)
}

func extractZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// detect common root prefix
	prefix := ""
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			parts := strings.SplitN(f.Name, "/", 2)
			if len(parts) == 2 {
				prefix = parts[0]
			}
			break
		}
	}

	for _, f := range r.File {
		fpath := f.Name
		if prefix != "" && strings.HasPrefix(fpath, prefix+"/") {
			fpath = fpath[len(prefix)+1:]
		}
		if fpath == "" {
			continue
		}

		fdest := filepath.Join(dest, filepath.FromSlash(fpath))

		if f.FileInfo().IsDir() {
			os.MkdirAll(fdest, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(fdest), 0755)

		rc, err := f.Open()
		if err != nil {
			return err
		}

		w, err := os.Create(fdest)
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(w, rc)
		rc.Close()
		w.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// ─── uninstall ──────────────────────────────────────────────

func cmdUninstall(zapretDir string) {
	cmdStop(zapretDir)
	time.Sleep(300 * time.Millisecond)
	if err := os.RemoveAll(zapretDir); err != nil {
		fmt.Fprintln(os.Stderr, "Remove error:", err)
		os.Exit(1)
	}
	fmt.Println("zapret uninstalled")
}

// ─── start / stop ──────────────────────────────────────────

func cmdStart(zapretDir string) {
	if !isInstalled(zapretDir) {
		fmt.Fprintln(os.Stderr, "zapret not installed, run 'install' first")
		os.Exit(1)
	}

	if isWinwsRunning() {
		fmt.Println("already running")
		return
	}

	strat := findStrategy(zapretDir)
	if strat == "" {
		fmt.Fprintln(os.Stderr, "no strategy found")
		os.Exit(1)
	}

	args := parseStrategy(zapretDir, strat)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "failed to parse strategy:", strat)
		os.Exit(1)
	}

	winws := filepath.Join(zapretDir, "bin", "winws.exe")
	if _, err := os.Stat(winws); err != nil {
		winws = filepath.Join(zapretDir, "winws.exe")
	}

	cmd := exec.Command(winws, args...)
	cmd.Dir = zapretDir
	cmd.SysProcAttr = hideWindowAttr()

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start error:", err)
		os.Exit(1)
	}
	fmt.Println("zapret started (pid", cmd.Process.Pid, ")")
}

func cmdStop(zapretDir string) {
	_ = exec.Command("taskkill", "/f", "/im", "winws.exe").Run()
	fmt.Println("zapret stopped")
}

func isWinwsRunning() bool {
	err := exec.Command("tasklist", "/fi", "imagename eq winws.exe", "/nh").Run()
	return err == nil
}

// ─── status ─────────────────────────────────────────────────

func cmdStatus(zapretDir string) {
	if !isInstalled(zapretDir) {
		fmt.Println(`{"zapret_state":"not_installed","zapret_version":"","zapret_error":""}`)
		return
	}
	st := getZapretState(zapretDir)
	data, _ := json.Marshal(st)
	fmt.Println(string(data))
}

// ─── update ─────────────────────────────────────────────────

func cmdUpdate(zapretDir string) {
	if !isInstalled(zapretDir) {
		fmt.Fprintln(os.Stderr, "zapret not installed")
		os.Exit(1)
	}

	latest, err := fetchLatestVersion()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch version:", err)
		os.Exit(1)
	}

	current := readVersion(zapretDir)
	if current == latest || strings.TrimPrefix(current, "v") == strings.TrimPrefix(latest, "v") {
		fmt.Println("already up to date:", current)
		return
	}

	fmt.Printf("updating %s → %s\n", current, latest)
	cmdStop(zapretDir)
	time.Sleep(500 * time.Millisecond)

	backupDir := zapretDir + ".bak"
	os.RemoveAll(backupDir)
	os.Rename(zapretDir, backupDir)

	// install fresh
	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/zapret-discord-youtube-%s.zip", zapretRepo, latest, latest)
	zipPath := filepath.Join(zapretDir, "zapret.zip")
	if err := downloadFile(downloadURL, zipPath); err != nil {
		os.Rename(backupDir, zapretDir)
		fail("download: %v", err)
	}

	if err := extractZip(zipPath, zapretDir); err != nil {
		os.Rename(backupDir, zapretDir)
		fail("extract: %v", err)
	}
	os.Remove(zipPath)
	os.RemoveAll(backupDir)

	fmt.Println("updated to", latest)
}

// ─── strategies ─────────────────────────────────────────────

func cmdStrategies(zapretDir string) {
	strats := listStrategies(zapretDir)
	if len(strats) == 0 {
		fmt.Println("no strategies found")
		return
	}
	sort.Strings(strats)
	for _, s := range strats {
		fmt.Println(s)
	}
}

// ─── config ─────────────────────────────────────────────────

func cmdConfig(zapretDir string, args []string) {
	if len(args) == 0 || args[0] == "list" {
		cfg := loadConfig(zapretDir)
		for k, v := range cfg {
			fmt.Printf("%s = %v\n", k, v)
		}
		return
	}

	if args[0] == "get" && len(args) >= 2 {
		cfg := loadConfig(zapretDir)
		if v, ok := cfg[args[1]]; ok {
			fmt.Println(v)
		} else {
			fmt.Fprintln(os.Stderr, "key not found:", args[1])
			os.Exit(1)
		}
		return
	}

	if args[0] == "set" && len(args) >= 3 {
		cfg := loadConfig(zapretDir)
		cfg[args[1]] = args[2]
		saveConfig(zapretDir, cfg)
		fmt.Printf("%s = %s\n", args[1], args[2])
		return
	}

	fmt.Fprintln(os.Stderr, "usage: config get <key> | config set <key> <value> | config list")
	os.Exit(1)
}

// ─── helpers ────────────────────────────────────────────────

func isInstalled(zapretDir string) bool {
	_, err := os.Stat(filepath.Join(zapretDir, "winws.exe"))
	return err == nil
}

func readVersion(zapretDir string) string {
	data, err := os.ReadFile(filepath.Join(zapretDir, "service.bat"))
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "LOCAL_VERSION=") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "LOCAL_VERSION="))
			return v
		}
	}
	return "unknown"
}

func listStrategies(zapretDir string) []string {
	entries, err := os.ReadDir(zapretDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "general") && strings.HasSuffix(e.Name(), ".bat") {
			out = append(out, e.Name())
		}
	}
	return out
}

func findStrategy(zapretDir string) string {
	strats := listStrategies(zapretDir)
	if len(strats) == 0 {
		return ""
	}
	for _, s := range strats {
		if strings.Contains(strings.ToLower(s), "alt") {
			return s
		}
	}
	return strats[0]
}

func parseStrategy(zapretDir, name string) []string {
	data, err := os.ReadFile(filepath.Join(zapretDir, name))
	if err != nil {
		return nil
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "winws.exe") {
			continue
		}
		idx := strings.Index(line, "winws.exe")
		if idx < 0 {
			continue
		}
		argsPart := line[idx+len("winws.exe"):]
		argsPart = strings.TrimSpace(argsPart)
		argsPart = strings.ReplaceAll(argsPart, "%BIN%", filepath.Join(zapretDir, "bin")+"\\")
		argsPart = strings.ReplaceAll(argsPart, "%LISTS%", filepath.Join(zapretDir, "lists")+"\\")
		argsPart = strings.ReplaceAll(argsPart, `"%~dp0bin\`, filepath.Join(zapretDir, "bin")+`\`)
		argsPart = strings.ReplaceAll(argsPart, `"%~dp0lists\`, filepath.Join(zapretDir, "lists")+`\`)

		return splitArgs(argsPart)
	}
	return nil
}

func splitArgs(s string) []string {
	var args []string
	cur := ""
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
			continue
		}
		if c == ' ' && !inQuote {
			if cur != "" {
				args = append(args, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		args = append(args, cur)
	}
	return args
}

func loadConfig(zapretDir string) map[string]string {
	cfg := map[string]string{
		"strategy":      findStrategy(zapretDir),
		"auto_start":    "false",
		"game_mode":     "disabled",
	}
	data, err := os.ReadFile(filepath.Join(zapretDir, "module.json"))
	if err != nil {
		return cfg
	}
	var extra map[string]string
	if json.Unmarshal(data, &extra) == nil {
		for k, v := range extra {
			cfg[k] = v
		}
	}
	return cfg
}

func saveConfig(zapretDir string, cfg map[string]string) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(filepath.Join(zapretDir, "module.json"), data, 0644)
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// windows-specific
func hideWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
}
