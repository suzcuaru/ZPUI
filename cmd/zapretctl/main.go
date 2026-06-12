package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	logFile *os.File
	stdin   = bufio.NewReader(os.Stdin)
)

func initLog(exeDir string) {
	logPath := filepath.Join(exeDir, "zapretctl.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Warning: cannot create log file %s: %v\n", logPath, err)
		return
	}
	logFile = f
	logMsg("=== Zapret Service Manager started ===")
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
	if !isAdmin() {
		relaunchAsAdmin()
		return
	}

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	initLog(exeDir)
	defer closeLog()

	zapretDir := findZapretDir(exeDir)
	logMsg("exeDir: %s", exeDir)
	logMsg("zapretDir: %s", zapretDir)

	winws := filepath.Join(zapretDir, "bin", "winws.exe")
	if _, err := os.Stat(winws); err != nil {
		logMsg("ERROR: winws.exe not found at %s", winws)
		fmt.Println("Error: winws.exe not found at", winws)
		fmt.Println("Make sure the program is in the zapret directory")
		pause()
		return
	}
	logMsg("winws.exe found at %s", winws)

	for {
		cls()
		showCurrentStrategy()
		fmt.Println()
		fmt.Println("=== Zapret Service Manager ===")
		fmt.Println(" 1. Install Service")
		fmt.Println(" 2. Remove Service")
		fmt.Println(" 3. Change Strategy")
		fmt.Println(" 0. Exit")
		fmt.Println()

		switch readLine("Select option: ") {
		case "1":
			logMsg("User selected: Install Service")
			installService(zapretDir)
		case "2":
			logMsg("User selected: Remove Service")
			removeService()
		case "3":
			logMsg("User selected: Change Strategy")
			changeStrategy(zapretDir)
		case "0":
			fmt.Println("Exiting...")
			logMsg("User selected: Exit")
			return
		default:
			fmt.Println("Invalid option")
		}
		pause()
	}
}

func installService(zapretDir string) {
	files := listStrategies(zapretDir)
	logMsg("Found %d strategy files", len(files))
	if len(files) == 0 {
		fmt.Println("No strategy files found (*.bat)")
		return
	}

	fmt.Println()
	fmt.Println("Select strategy:")
	for i, f := range files {
		fmt.Printf(" %2d. %s\n", i+1, f)
	}
	fmt.Println()

	idx := readInt("Input file index: ")
	if idx < 1 || idx > len(files) {
		logMsg("Invalid strategy index: %d", idx)
		fmt.Println("Invalid index")
		return
	}
	logMsg("Selected strategy: %s (index %d)", files[idx-1], idx)

	batPath := filepath.Join(zapretDir, files[idx-1])
	logMsg("Reading bat file: %s", batPath)
	batData, _ := os.ReadFile(batPath)
	logMsg("Bat file raw content:\n%s", string(batData))

	args := extractArgs(batPath, zapretDir)
	if args == "" {
		logMsg("ERROR: failed to parse arguments from %s", files[idx-1])
		fmt.Println("Failed to parse arguments from", files[idx-1])
		fmt.Println("Make sure the bat file contains a winws.exe launch line")
		return
	}

	logMsg("Parsed final args: %s", args)
	fmt.Println("Final args:", args)

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
		fmt.Println("Failed to create service (see zapretctl.log)")
		return
	}
	fmt.Println("[SC] CreateService SUCCESS")
	logMsg("sc create SUCCESS")

	runCmd("sc", "description", "zapret", "Zapret DPI bypass software")

	name := strings.TrimSuffix(files[idx-1], ".bat")
	runCmd("reg", "add",
		"HKLM\\System\\CurrentControlSet\\Services\\zapret",
		"/v", "zapret-discord-youtube",
		"/t", "REG_SZ",
		"/d", name,
		"/f")

	if err := runCmdErr("sc", "start", "zapret"); err != nil {
		logMsg("ERROR: sc start failed: %v", err)
		fmt.Println("Service start FAILED (see zapretctl.log)")
	}

	showServiceStatus()

	scQuery := execCmd("sc", "query", "zapret")
	logMsg("Final sc query zapret:\n%s", scQuery)
}

func removeService() {
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
}

func changeStrategy(zapretDir string) {
	logMsg("--- Changing strategy ---")
	if serviceExists("zapret") {
		logMsg("zapret service exists, removing first")
		fmt.Println("Removing existing service...")
		removeService()
	} else {
		logMsg("zapret service not installed, installing fresh")
	}
	installService(zapretDir)
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
				logMsg("  found winws.exe line, rest: %s", rest[:min(len(rest), 120)])
				logMsg("  full rest: %s", rest)
			}
			continue
		}

		if inArgs {
			if trimmed == "" || strings.HasPrefix(trimmed, "@echo") || strings.HasPrefix(trimmed, "set ") || strings.HasPrefix(trimmed, "cd ") || strings.HasPrefix(trimmed, "call ") || strings.HasPrefix(trimmed, "start ") || strings.HasPrefix(trimmed, "chcp ") || strings.HasPrefix(trimmed, "::") || strings.HasPrefix(trimmed, "rem ") {
				logMsg("  args end at line: %s", trimmed)
				break
			}
			argLines = append(argLines, trimmed)
			logMsg("  continuation line: %s", trimmed[:min(len(trimmed), 120)])
		}
	}

	if len(argLines) == 0 {
		logMsg("ERROR: no argument lines found in %s", batPath)
		return ""
	}
	logMsg("  total arg lines: %d", len(argLines))

	raw := strings.Join(argLines, " ")
	raw = strings.ReplaceAll(raw, "^", "")
	raw = strings.ReplaceAll(raw, "\t", " ")

	binPath := filepath.Join(zapretDir, "bin") + "\\"
	listsPath := filepath.Join(zapretDir, "lists") + "\\"
	logMsg("  substituting: %%BIN%% -> %s, %%LISTS%% -> %s", binPath, listsPath)
	raw = strings.ReplaceAll(raw, "%BIN%", binPath)
	raw = strings.ReplaceAll(raw, "%LISTS%", listsPath)
	raw = strings.ReplaceAll(raw, "%GameFilterTCP%", "12")
	raw = strings.ReplaceAll(raw, "%GameFilterUDP%", "12")
	raw = strings.ReplaceAll(raw, "%GameFilter%", "12")

	tokens := tokenize(raw)
	logMsg("  tokenized into %d tokens", len(tokens))

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

func listStrategies(zapretDir string) []string {
	entries, err := os.ReadDir(zapretDir)
	if err != nil {
		logMsg("ERROR reading %s: %v", zapretDir, err)
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bat") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(e.Name()), "service") {
			continue
		}
		files = append(files, e.Name())
	}
	logMsg("listStrategies from %s: %v", zapretDir, files)
	return files
}

func isAdmin() bool {
	return exec.Command("net", "session").Run() == nil
}

func relaunchAsAdmin() {
	fmt.Println("Requesting admin rights...")
	exe, _ := os.Executable()
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Start-Process '\"%s\"' -Verb RunAs", exe))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
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

func showCurrentStrategy() {
	out := execCmd("reg", "query", "HKLM\\System\\CurrentControlSet\\Services\\zapret", "/v", "zapret-discord-youtube")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "zapret-discord-youtube") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				strat := parts[len(parts)-1]
				fmt.Println("Current strategy:", strat)
				logMsg("Current strategy from reg: %s", strat)
				return
			}
		}
	}
}

func showServiceStatus() {
	if serviceExists("zapret") {
		out := runCmd("sc", "query", "zapret")
		if strings.Contains(out, "RUNNING") || strings.Contains(out, "РАБОТАЕТ") {
			fmt.Println("Service status: RUNNING")
			logMsg("Service status: RUNNING")
		} else {
			fmt.Println("Service status: STOPPED")
			logMsg("Service status: STOPPED")
		}
	} else {
		fmt.Println("Service is NOT installed")
		logMsg("Service status: NOT installed")
	}
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

func cls() {
	cmd := exec.Command("cmd", "/c", "cls")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func readLine(prompt string) string {
	fmt.Print(prompt)
	text, _ := stdin.ReadString('\n')
	return strings.TrimSpace(text)
}

func readInt(prompt string) int {
	fmt.Print(prompt)
	line, _ := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	var n int
	fmt.Sscanf(line, "%d", &n)
	return n
}

func pause() {
	fmt.Println("\nPress Enter to continue...")
	stdin.ReadBytes('\n')
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
		logMsg("findZapretDir: %s\\bin\\winws.exe not found", dir)
	}
	fallback := filepath.Dir(exeDir)
	logMsg("findZapretDir: fallback to %s", fallback)
	return fallback
}
