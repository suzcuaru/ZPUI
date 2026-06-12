package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var logFile *os.File

func initLog(exeDir string) bool {
	logPath := filepath.Join(exeDir, "zapretctl.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return false
	}
	logFile = f
	logMsg("=== Zapret Service Manager started ===")
	return true
}

func closeLog() {
	if logFile != nil {
		logMsg("=== Zapret Service Manager stopped ===")
		logFile.Close()
	}
}

func logMsg(format string, args ...interface{}) {
	if logFile == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	t := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(logFile, "[%s] %s\n", t, msg)
}

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	initLog(exeDir)
	defer closeLog()

	if len(os.Args) < 2 {
		logMsg("ERROR: no subcommand")
		fmt.Println("Usage: zapretctl <install|remove|strategy> [args]")
		os.Exit(1)
	}

	zapretDir := findZapretDir(exeDir)
	logMsg("exeDir: %s, zapretDir: %s", exeDir, zapretDir)

	switch os.Args[1] {
	case "install":
		cmdInstall(zapretDir)
	case "remove":
		cmdRemove()
	case "strategy":
		cmdStrategy(zapretDir)
	default:
		logMsg("ERROR: unknown subcommand: %s", os.Args[1])
		fmt.Printf("Unknown subcommand: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func checkAdmin() {
	if exec.Command("net", "session").Run() != nil {
		logMsg("ERROR: not running as admin")
		fmt.Println("This program must be run as Administrator")
		os.Exit(1)
	}
}

func cmdInstall(zapretDir string) {
	checkAdmin()

	if len(os.Args) < 3 {
		logMsg("ERROR: install requires strategy filename")
		fmt.Println("Usage: zapretctl install <strategy.bat>")
		os.Exit(1)
	}
	strategyFile := os.Args[2]
	batPath := filepath.Join(zapretDir, strategyFile)

	logMsg("Strategy file: %s", batPath)
	if _, err := os.Stat(batPath); err != nil {
		logMsg("ERROR: strategy file not found: %s", batPath)
		fmt.Printf("Strategy file not found: %s\n", strategyFile)
		os.Exit(1)
	}

	args := extractArgs(batPath, zapretDir)
	if args == "" {
		logMsg("ERROR: failed to parse args from %s", strategyFile)
		fmt.Println("Failed to parse arguments from", strategyFile)
		os.Exit(1)
	}

	logMsg("Parsed args: %s", args)

	logMsg("Stopping/removing existing services...")
	stopService("zapret")
	deleteService("zapret")
	stopService("WinDivert")
	deleteService("WinDivert")
	stopService("WinDivert14")
	deleteService("WinDivert14")

	winws := filepath.Join(zapretDir, "bin", "winws.exe")
	fullCmd := fmt.Sprintf(`"%s" %s`, winws, args)
	logMsg("Full binPath: %s", fullCmd)

	if err := runCmdErr("sc", "create", "zapret",
		"binPath=", fullCmd,
		"DisplayName=", "zapret",
		"start=", "auto"); err != nil {
		logMsg("ERROR: sc create failed: %v", err)
		fmt.Println("Service creation FAILED")
		os.Exit(1)
	}
	fmt.Println("[SC] CreateService SUCCESS")
	logMsg("sc create SUCCESS")

	runCmd("sc", "description", "zapret", "Zapret DPI bypass software")

	name := strings.TrimSuffix(strategyFile, ".bat")
	runCmd("reg", "add",
		"HKLM\\System\\CurrentControlSet\\Services\\zapret",
		"/v", "zapret-discord-youtube",
		"/t", "REG_SZ",
		"/d", name,
		"/f")

	if err := runCmdErr("sc", "start", "zapret"); err != nil {
		logMsg("ERROR: sc start failed: %v", err)
		fmt.Println("Service start FAILED")
		os.Exit(2)
	}

	fmt.Println("Service started successfully")
	os.Exit(0)
}

func cmdRemove() {
	checkAdmin()

	logMsg("--- Removing services ---")

	scQuery := execCmd("sc", "query", "zapret")
	logMsg("Before removal - sc query zapret: %s", strings.TrimSpace(scQuery))

	stopService("zapret")
	deleteService("zapret")

	out := runCmd("tasklist", "/FI", "IMAGENAME eq winws.exe")
	logMsg("tasklist winws.exe: %s", strings.TrimSpace(out))
	if strings.Contains(out, "winws.exe") {
		fmt.Println("winws.exe is still running, killing...")
		logMsg("Killing winws.exe")
		runCmd("taskkill", "/IM", "winws.exe", "/F")
	}

	scQuery = execCmd("sc", "query", "zapret")
	logMsg("After removal - sc query zapret: %s", strings.TrimSpace(scQuery))

	if serviceExists("WinDivert") {
		logMsg("WinDivert service exists, removing...")
		stopService("WinDivert")
		deleteService("WinDivert")
	}
	if serviceExists("WinDivert14") {
		logMsg("WinDivert14 service exists, removing...")
		stopService("WinDivert14")
		deleteService("WinDivert14")
	}

	logMsg("Remove completed")
	fmt.Println("Service removed successfully")
	os.Exit(0)
}

func cmdStrategy(zapretDir string) {
	checkAdmin()

	if len(os.Args) < 3 {
		logMsg("ERROR: strategy requires strategy filename")
		fmt.Println("Usage: zapretctl strategy <strategy.bat>")
		os.Exit(1)
	}
	strategyFile := os.Args[2]
	batPath := filepath.Join(zapretDir, strategyFile)

	logMsg("Strategy file: %s", batPath)
	if _, err := os.Stat(batPath); err != nil {
		logMsg("ERROR: strategy file not found: %s", batPath)
		fmt.Printf("Strategy file not found: %s\n", strategyFile)
		os.Exit(1)
	}

	logMsg("--- Changing strategy ---")
	if serviceExists("zapret") {
		logMsg("zapret service exists, removing first")
		fmt.Println("Removing existing service...")
		stopService("zapret")
		deleteService("zapret")
		stopService("WinDivert")
		deleteService("WinDivert")
		stopService("WinDivert14")
		deleteService("WinDivert14")
	} else {
		logMsg("zapret service not installed, installing fresh")
	}

	args := extractArgs(batPath, zapretDir)
	if args == "" {
		logMsg("ERROR: failed to parse args from %s", strategyFile)
		fmt.Println("Failed to parse arguments from", strategyFile)
		os.Exit(1)
	}

	logMsg("Parsed args: %s", args)

	winws := filepath.Join(zapretDir, "bin", "winws.exe")
	fullCmd := fmt.Sprintf(`"%s" %s`, winws, args)
	logMsg("Full binPath: %s", fullCmd)

	if err := runCmdErr("sc", "create", "zapret",
		"binPath=", fullCmd,
		"DisplayName=", "zapret",
		"start=", "auto"); err != nil {
		logMsg("ERROR: sc create failed: %v", err)
		fmt.Println("Service creation FAILED")
		os.Exit(1)
	}
	fmt.Println("[SC] CreateService SUCCESS")
	logMsg("sc create SUCCESS")

	runCmd("sc", "description", "zapret", "Zapret DPI bypass software")

	name := strings.TrimSuffix(strategyFile, ".bat")
	runCmd("reg", "add",
		"HKLM\\System\\CurrentControlSet\\Services\\zapret",
		"/v", "zapret-discord-youtube",
		"/t", "REG_SZ",
		"/d", name,
		"/f")

	if err := runCmdErr("sc", "start", "zapret"); err != nil {
		logMsg("ERROR: sc start failed: %v", err)
		fmt.Println("Service start FAILED")
		os.Exit(2)
	}

	fmt.Println("Service started successfully")
	os.Exit(0)
}

func extractArgs(batPath, zapretDir string) string {
	data, err := os.ReadFile(batPath)
	if err != nil {
		logMsg("ERROR reading bat file %s: %v", batPath, err)
		return ""
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var argLines []string
	inArgs := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "winws.exe") {
			idx := strings.Index(trimmed, "winws.exe")
			rest := trimmed[idx+len("winws.exe"):]
			rest = strings.TrimLeft(rest, ` "`)
			rest = strings.TrimSpace(rest)
			if rest != "" {
				argLines = append(argLines, rest)
				inArgs = true
				logMsg("  found winws.exe line, rest: %s", rest)
			}
			continue
		}

		if inArgs {
			if trimmed == "" ||
				strings.HasPrefix(trimmed, "@echo") ||
				strings.HasPrefix(trimmed, "set ") ||
				strings.HasPrefix(trimmed, "cd ") ||
				strings.HasPrefix(trimmed, "call ") ||
				strings.HasPrefix(trimmed, "start ") ||
				strings.HasPrefix(trimmed, "chcp ") ||
				strings.HasPrefix(trimmed, "::") ||
				strings.HasPrefix(trimmed, "rem ") {
				logMsg("  args end at line: %s", trimmed)
				break
			}
			argLines = append(argLines, trimmed)
			logMsg("  continuation line: %s", trimmed)
		}
	}

	if len(argLines) == 0 {
		logMsg("ERROR: no argument lines found")
		return ""
	}

	raw := strings.Join(argLines, " ")
	raw = strings.ReplaceAll(raw, "^", "")
	raw = strings.ReplaceAll(raw, "\t", " ")

	binPath := filepath.Join(zapretDir, "bin") + "\\"
	listsPath := filepath.Join(zapretDir, "lists") + "\\"
	raw = strings.ReplaceAll(raw, "%BIN%", binPath)
	raw = strings.ReplaceAll(raw, "%LISTS%", listsPath)
	raw = strings.ReplaceAll(raw, "%GameFilterTCP%", "12")
	raw = strings.ReplaceAll(raw, "%GameFilterUDP%", "12")
	raw = strings.ReplaceAll(raw, "%GameFilter%", "12")

	tokens := tokenize(raw)
	var result []string
	for _, t := range tokens {
		if strings.HasPrefix(t, "--") && strings.Contains(t, "=") {
			parts := strings.SplitN(t, "=", 2)
			result = append(result, parts[0])
			if len(parts) > 1 {
				result = append(result, parts[1])
			}
		} else {
			result = append(result, t)
		}
	}

	final := strings.Join(result, " ")
	logMsg("  final args: %s", final)
	return final
}

func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuote = !inQuote
			cur.WriteByte(ch)
			continue
		}
		if (ch == ' ' || ch == '\t') && !inQuote {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func findZapretDir(exeDir string) string {
	candidates := []string{
		exeDir,
		filepath.Dir(exeDir),
		filepath.Join(exeDir, ".."),
	}
	for _, dir := range candidates {
		winws := filepath.Join(dir, "bin", "winws.exe")
		if _, err := os.Stat(winws); err == nil {
			logMsg("findZapretDir: found winws.exe in %s", dir)
			return dir
		}
	}
	logMsg("findZapretDir: fallback to %s", exeDir)
	return exeDir
}

func stopService(name string) {
	logMsg("  stop service %s", name)
	runCmd("net", "stop", name)
}

func deleteService(name string) {
	logMsg("  delete service %s", name)
	runCmd("sc", "delete", name)
}

func serviceExists(name string) bool {
	err := runCmdErr("sc", "query", name)
	exists := err == nil
	logMsg("  service %s exists: %v", name, exists)
	return exists
}

func runCmd(name string, args ...string) string {
	fullArgs := strings.Join(args, " ")
	logMsg("> %s %s", name, fullArgs)
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if err != nil {
		logMsg("  exit: %v", err)
	} else {
		logMsg("  ok")
	}
	logMsg("  stdout/stderr:\n%s", strings.TrimSpace(outStr))
	return outStr
}

func runCmdErr(name string, args ...string) error {
	fullArgs := strings.Join(args, " ")
	logMsg("> %s %s", name, fullArgs)
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logMsg("  exit: %v", err)
	} else {
		logMsg("  ok")
	}
	logMsg("  stdout/stderr:\n%s", strings.TrimSpace(string(out)))
	return err
}

func execCmd(name string, args ...string) string {
	fullArgs := strings.Join(args, " ")
	logMsg("> %s %s (capture)", name, fullArgs)
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		logMsg("  exit: %v", err)
		return ""
	}
	logMsg("  ok")
	return string(out)
}
