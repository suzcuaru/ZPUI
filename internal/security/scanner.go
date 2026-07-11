package security

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

type Severity int

const (
	SeverityLow Severity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "LOW"
	case SeverityMedium:
		return "MEDIUM"
	case SeverityHigh:
		return "HIGH"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

type Threat struct {
	File     string   `json:"file"`
	Category string   `json:"category"`
	Detail   string   `json:"detail"`
	Severity Severity `json:"severity"`
	IOC      string   `json:"ioc"`
}

type ScanResult struct {
	Clean       bool      `json:"clean"`
	Threats     []Threat  `json:"threats,omitempty"`
	ScannedFiles int      `json:"scanned_files"`
	TotalFiles   int      `json:"total_files"`
}

func (r *ScanResult) AddThreat(t Threat) {
	r.Threats = append(r.Threats, t)
	r.Clean = false
}

func ScanZip(zipPath string, expectedDirs []string) (*ScanResult, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	result := &ScanResult{Clean: true}
	result.TotalFiles = len(r.File)

	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)

		if f.FileInfo().IsDir() {
			continue
		}

		if !isScannableFile(name) {
			continue
		}

		result.ScannedFiles++

		rc, err := f.Open()
		if err != nil {
			continue
		}

		data, err := io.ReadAll(io.LimitReader(rc, 50*1024*1024))
		rc.Close()
		if err != nil {
			continue
		}

		scanFile(name, data, expectedDirs, result)
	}

	return result, nil
}

func ScanFile(path string) (*ScanResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	result := &ScanResult{Clean: true, TotalFiles: 1, ScannedFiles: 1}
	scanFile(filepath.Base(path), data, nil, result)
	return result, nil
}

func ScanBytes(data []byte, filename string) *ScanResult {
	result := &ScanResult{Clean: true, TotalFiles: 1, ScannedFiles: 1}
	scanFile(filename, data, nil, result)
	return result
}

func isScannableFile(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{
		".exe", ".dll", ".sys", ".bat", ".cmd", ".ps1",
		".vbs", ".js", ".wsf", ".scr", ".com", ".pif",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func scanFile(name string, data []byte, expectedDirs []string, result *ScanResult) {
	lower := strings.ToLower(name)
	ext := filepath.Ext(lower)

	switch {
	case ext == ".exe" || ext == ".dll" || ext == ".sys" || ext == ".scr" || ext == ".com":
		scanPE(name, data, result)
	case ext == ".bat" || ext == ".cmd":
		scanBatch(name, data, result)
	case ext == ".ps1":
		scanPowerShell(name, data, result)
	case ext == ".vbs" || ext == ".js" || ext == ".wsf":
		scanScript(name, data, result)
	}

	scanUnexpectedExecutables(name, expectedDirs, result)
}

func scanPE(name string, data []byte, result *ScanResult) {
	extracted := extractStrings(data)
	joined := stringsToText(extracted)
	lower := strings.ToLower(joined)

	checkIOCGroups(name, lower, result)

	if isPE(data) {
		checkSuspiciousImports(data, result)
	}
}

func isPE(data []byte) bool {
	if len(data) < 64 {
		return false
	}
	if data[0] != 'M' || data[1] != 'Z' {
		return false
	}
	offset := binary.LittleEndian.Uint32(data[60:64])
	if int(offset)+4 > len(data) {
		return false
	}
	return data[offset] == 'P' && data[offset+1] == 'E'
}

func extractStrings(data []byte) []string {
	var result []string

	result = append(result, extractASCIIStrings(data, 6)...)
	result = append(result, extractUTF16Strings(data, 6)...)

	return result
}

func extractASCIIStrings(data []byte, minLen int) []string {
	var strings []string
	var current bytes.Buffer

	for _, b := range data {
		if b >= 0x20 && b < 0x7F {
			current.WriteByte(b)
		} else {
			if current.Len() >= minLen {
				strings = append(strings, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() >= minLen {
		strings = append(strings, current.String())
	}

	return strings
}

func extractUTF16Strings(data []byte, minLen int) []string {
	var result []string
	if len(data) < 2 {
		return result
	}

	for i := 0; i < len(data)-1; i += 2 {
		r := utf16.Decode([]uint16{binary.LittleEndian.Uint16(data[i : i+2])})
		if len(r) == 0 {
			continue
		}
		if r[0] < 0x20 || r[0] >= 0x7F {
			continue
		}

		current := i
		var buf bytes.Buffer
		for current < len(data)-1 {
			ch := utf16.Decode([]uint16{binary.LittleEndian.Uint16(data[current : current+2])})
			if len(ch) != 1 || ch[0] < 0x20 || ch[0] >= 0x7F {
				break
			}
			buf.WriteRune(ch[0])
			current += 2
		}

		if buf.Len() >= minLen {
			result = append(result, buf.String())
		}
	}

	return result
}

func stringsToText(strs []string) string {
	var buf bytes.Buffer
	for _, s := range strs {
		buf.WriteString(s)
		buf.WriteByte('\n')
	}
	return buf.String()
}

type iocPattern struct {
	Category string
	Pattern  string
	Severity Severity
	Label    string
}

var stillerIOCs = []iocPattern{
	{
		Category: "Browser Stealer",
		Pattern:  "\\local state\\",
		Severity: SeverityCritical,
		Label:    "Chromium Local State (master key)",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "\\login data\\",
		Severity: SeverityCritical,
		Label:    "Chromium Login Data (saved passwords)",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "cookies.sqlite",
		Severity: SeverityCritical,
		Label:    "Firefox cookies",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "signons.sqlite",
		Severity: SeverityCritical,
		Label:    "Firefox saved logins",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "formhistory.sqlite",
		Severity: SeverityCritical,
		Label:    "Firefox form history",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "web data",
		Severity: SeverityHigh,
		Label:    "Chromium Web Data (autofill)",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "bookmarks.bak",
		Severity: SeverityMedium,
		Label:    "Chrome bookmarks backup",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "cookies\\cookies",
		Severity: SeverityCritical,
		Label:    "Chromium Cookies",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "opera\\gx\\cookies",
		Severity: SeverityCritical,
		Label:    "Opera GX cookies",
	},
	{
		Category: "Browser Stealer",
		Pattern:  "brave\\brave-software",
		Severity: SeverityCritical,
		Label:    "Brave browser data",
	},
	{
		Category: "Discord Stealer",
		Pattern:  "discord.com/api",
		Severity: SeverityCritical,
		Label:    "Discord API endpoint",
	},
	{
		Category: "Discord Stealer",
		Pattern:  "leveldb\\",
		Severity: SeverityHigh,
		Label:    "Discord LevelDB path",
	},
	{
		Category: "Discord Stealer",
		Pattern:  "local storage\\leveldb",
		Severity: SeverityHigh,
		Label:    "Discord Local Storage LevelDB",
	},
	{
		Category: "Discord Stealer",
		Pattern:  "token",
		Severity: SeverityMedium,
		Label:    "Token reference",
	},
	{
		Category: "Telegram Stealer",
		Pattern:  "tdesktop\\tdata",
		Severity: SeverityCritical,
		Label:    "Telegram Desktop tdata",
	},
	{
		Category: "Telegram Stealer",
		Pattern:  "d877f783d5d3ef8c",
		Severity: SeverityCritical,
		Label:    "Telegram session file pattern",
	},
	{
		Category: "Telegram Stealer",
		Pattern:  "map1.json",
		Severity: SeverityCritical,
		Label:    "Telegram map1.json",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "metamask",
		Severity: SeverityCritical,
		Label:    "MetaMask wallet",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "phantom",
		Severity: SeverityCritical,
		Label:    "Phantom wallet",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "solflare",
		Severity: SeverityCritical,
		Label:    "Solflare wallet",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "exodus",
		Severity: SeverityCritical,
		Label:    "Exodus wallet",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "electrum\\wallets",
		Severity: SeverityCritical,
		Label:    "Electrum Bitcoin wallet",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "wallet.dat",
		Severity: SeverityCritical,
		Label:    "Bitcoin Core wallet.dat",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "keystore.json",
		Severity: SeverityHigh,
		Label:    "JSON keystore file",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "seed",
		Severity: SeverityMedium,
		Label:    "Seed phrase reference",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "mnemonic",
		Severity: SeverityHigh,
		Label:    "Mnemonic phrase reference",
	},
	{
		Category: "Crypto Wallet Stealer",
		Pattern:  "erc-20",
		Severity: SeverityMedium,
		Label:    "ERC-20 token reference",
	},
	{
		Category: "Credential Stealer",
		Pattern:  "credential",
		Severity: SeverityHigh,
		Label:    "Credential store reference",
	},
	{
		Category: "Credential Stealer",
		Pattern:  "vaultcli",
		Severity: SeverityCritical,
		Label:    "Windows Vault API",
	},
	{
		Category: "Credential Stealer",
		Pattern:  "cryptunprotectdata",
		Severity: SeverityCritical,
		Label:    "DPAPI decryption call",
	},
	{
		Category: "Credential Stealer",
		Pattern:  "cryptunprotect",
		Severity: SeverityCritical,
		Label:    "DPAPI decryption",
	},
	{
		Category: "Credential Stealer",
		Pattern:  "cerezamanager",
		Severity: SeverityHigh,
		Label:    "Password manager",
	},
	{
		Category: "Credential Stealer",
		Pattern:  "bitwarden",
		Severity: SeverityHigh,
		Label:    "Bitwarden vault",
	},
	{
		Category: "Credential Stealer",
		Pattern:  "1password",
		Severity: SeverityHigh,
		Label:    "1Password vault",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "webhook",
		Severity: SeverityHigh,
		Label:    "Discord webhook (common exfil channel)",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "file.io",
		Severity: SeverityCritical,
		Label:    "file.io upload endpoint",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "transfer.sh",
		Severity: SeverityCritical,
		Label:    "transfer.sh upload endpoint",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "pastebin.com/api",
		Severity: SeverityCritical,
		Label:    "Pastebin API upload",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "api.telegram.org/bot",
		Severity: SeverityHigh,
		Label:    "Telegram Bot API (exfil channel)",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "ngrok.io",
		Severity: SeverityHigh,
		Label:    "ngrok tunnel (exfil endpoint)",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "serveo.net",
		Severity: SeverityHigh,
		Label:    "Serveo tunnel (exfil endpoint)",
	},
	{
		Category: "Data Exfiltration",
		Pattern:  "localtunnel",
		Severity: SeverityHigh,
		Label:    "Localtunnel (exfil endpoint)",
	},
	{
		Category: "Keylogger",
		Pattern:  "getasynckeystate",
		Severity: SeverityCritical,
		Label:    "Async key state API (keylogger)",
	},
	{
		Category: "Keylogger",
		Pattern:  "getkeyboardstate",
		Severity: SeverityCritical,
		Label:    "Keyboard state API (keylogger)",
	},
	{
		Category: "Keylogger",
		Pattern:  "setwindowshookex",
		Severity: SeverityCritical,
		Label:    "Windows hook (keylogger)",
	},
	{
		Category: "Screen Capture",
		Pattern:  "getdesktopwindow",
		Severity: SeverityMedium,
		Label:    "Desktop window capture",
	},
	{
		Category: "Screen Capture",
		Pattern:  "bitblt",
		Severity: SeverityHigh,
		Label:    "BitBlt screen capture",
	},
	{
		Category: "Screen Capture",
		Pattern:  "gdiplusstartup",
		Severity: SeverityMedium,
		Label:    "GDI+ image capture",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "isdebuggerpresent",
		Severity: SeverityHigh,
		Label:    "Debugger detection",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "checkremotedebugger",
		Severity: SeverityHigh,
		Label:    "Remote debugger detection",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "outputdebugstringw",
		Severity: SeverityMedium,
		Label:    "Debug output detection",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "processhollowing",
		Severity: SeverityCritical,
		Label:    "Process hollowing technique",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "createremotethread",
		Severity: SeverityHigh,
		Label:    "Remote thread injection",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "ntwritevirtualmemory",
		Severity: SeverityCritical,
		Label:    "Process injection",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "setprocessdeppolicy",
		Severity: SeverityMedium,
		Label:    "DEP policy manipulation",
	},
	{
		Category: "Anti-Analysis",
		Pattern:  "setwindowtextw",
		Severity: SeverityLow,
		Label:    "Window text manipulation",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "redline",
		Severity: SeverityCritical,
		Label:    "RedLine stealer reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "raccoon",
		Severity: SeverityCritical,
		Label:    "Raccoon stealer reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "vidar",
		Severity: SeverityCritical,
		Label:    "Vidar stealer reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "lumma",
		Severity: SeverityCritical,
		Label:    "Lumma stealer reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "stealc",
		Severity: SeverityCritical,
		Label:    "Stealc stealer reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "amadey",
		Severity: SeverityCritical,
		Label:    "Amadey stealer reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "formbook",
		Severity: SeverityCritical,
		Label:    "Formbook stealer reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "remcos",
		Severity: SeverityCritical,
		Label:    "Remcos RAT reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "nanocore",
		Severity: SeverityCritical,
		Label:    "NanoCore RAT reference",
	},
	{
		Category: "Stealer Framework",
		Pattern:  "njrat",
		Severity: SeverityCritical,
		Label:    "NjRAT reference",
	},
}

func checkIOCGroups(name, lower string, result *ScanResult) {
	for _, ioc := range stillerIOCs {
		if strings.Contains(lower, ioc.Pattern) {
			result.AddThreat(Threat{
				File:     name,
				Category: ioc.Category,
				Detail:   ioc.Label,
				Severity: ioc.Severity,
				IOC:      ioc.Pattern,
			})
		}
	}
}

func checkSuspiciousImports(data []byte, result *ScanResult) {
	if !isPE(data) {
		return
	}

	offset := binary.LittleEndian.Uint32(data[60:64])
	peSig := data[offset : offset+4]
	if string(peSig) != "PE\x00\x00" {
		return
	}
}

var batchSuspiciousPatterns = []struct {
	Pattern  string
	Severity Severity
	Label    string
}{
	{
		Pattern:  "powershell",
		Severity: SeverityMedium,
		Label:    "PowerShell invocation from batch",
	},
	{
		Pattern:  "powershell.exe -enc",
		Severity: SeverityCritical,
		Label:    "Obfuscated PowerShell (base64 encoded)",
	},
	{
		Pattern:  "powershell -enc",
		Severity: SeverityCritical,
		Label:    "Obfuscated PowerShell (base64 encoded)",
	},
	{
		Pattern:  "powershell -windowstyle hidden",
		Severity: SeverityCritical,
		Label:    "Hidden PowerShell window",
	},
	{
		Pattern:  "powershell -w hidden",
		Severity: SeverityCritical,
		Label:    "Hidden PowerShell window",
	},
	{
		Pattern:  "powershell -nop",
		Severity: SeverityHigh,
		Label:    "PowerShell no-profile mode",
	},
	{
		Pattern:  "bitsadmin",
		Severity: SeverityHigh,
		Label:    "BITSAdmin download tool",
	},
	{
		Pattern:  "certutil -urlcache",
		Severity: SeverityCritical,
		Label:    "Certutil file download",
	},
	{
		Pattern:  "certutil -decode",
		Severity: SeverityCritical,
		Label:    "Certutil decode (base64 payload)",
	},
	{
		Pattern:  "mshta",
		Severity: SeverityCritical,
		Label:    "MSHTA script execution",
	},
	{
		Pattern:  "wscript",
		Severity: SeverityHigh,
		Label:    "Windows Script Host",
	},
	{
		Pattern:  "cscript",
		Severity: SeverityHigh,
		Label:    "Windows Script Host (cscript)",
	},
	{
		Pattern:  "regsvr32",
		Severity: SeverityCritical,
		Label:    "RegSvr32 DLL registration (LOLBins)",
	},
	{
		Pattern:  "rundll32",
		Severity: SeverityHigh,
		Label:    "Rundll32 execution",
	},
	{
		Pattern:  "schtasks",
		Severity: SeverityHigh,
		Label:    "Scheduled task creation",
	},
	{
		Pattern:  "reg add",
		Severity: SeverityMedium,
		Label:    "Registry modification",
	},
	{
		Pattern:  "start /b",
		Severity: SeverityMedium,
		Label:    "Background process start",
	},
	{
		Pattern:  "curl",
		Severity: SeverityMedium,
		Label:    "curl download",
	},
	{
		Pattern:  "wget",
		Severity: SeverityMedium,
		Label:    "wget download",
	},
	{
		Pattern:  "invoke-webrequest",
		Severity: SeverityHigh,
		Label:    "PowerShell web request",
	},
	{
		Pattern:  "iwr",
		Severity: SeverityMedium,
		Label:    "PowerShell Invoke-WebRequest alias",
	},
	{
		Pattern:  "iex",
		Severity: SeverityCritical,
		Label:    "PowerShell Invoke-Expression (code execution)",
	},
	{
		Pattern:  "invoke-expression",
		Severity: SeverityCritical,
		Label:    "PowerShell Invoke-Expression",
	},
	{
		Pattern:  "downloadstring",
		Severity: SeverityCritical,
		Label:    "PowerShell download and string",
	},
	{
		Pattern:  "downloadfile",
		Severity: SeverityCritical,
		Label:    "PowerShell download file",
	},
	{
		Pattern:  "start-process",
		Severity: SeverityHigh,
		Label:    "PowerShell process start",
	},
	{
		Pattern:  "new-object net.webclient",
		Severity: SeverityCritical,
		Label:    "PowerShell WebClient creation",
	},
	{
		Pattern:  "new-object system.net.webclient",
		Severity: SeverityCritical,
		Label:    "PowerShell WebClient creation",
	},
	{
		Pattern:  "start-bitstransfer",
		Severity: SeverityHigh,
		Label:    "PowerShell BITS transfer",
	},
	{
		Pattern:  "%temp%\\",
		Severity: SeverityMedium,
		Label:    "Temp directory usage",
	},
	{
		Pattern:  "%appdata%\\",
		Severity: SeverityMedium,
		Label:    "AppData Roaming usage",
	},
	{
		Pattern:  "appdata\\local\\temp",
		Severity: SeverityMedium,
		Label:    "Temp directory usage",
	},
	{
		Pattern:  "hidden",
		Severity: SeverityMedium,
		Label:    "Hidden window attribute",
	},
	{
		Pattern:  "minimized",
		Severity: SeverityLow,
		Label:    "Minimized window",
	},
	{
		Pattern:  "attrib +s +h",
		Severity: SeverityHigh,
		Label:    "File hidden attribute",
	},
	{
		Pattern:  "del %0",
		Severity: SeverityHigh,
		Label:    "Self-deletion (anti-forensics)",
	},
	{
		Pattern:  "taskkill",
		Severity: SeverityMedium,
		Label:    "Process termination",
	},
	{
		Pattern:  "cipher /w",
		Severity: SeverityCritical,
		Label:    "Disk wiping (cipher /w)",
	},
	{
		Pattern:  "wevtutil cl",
		Severity: SeverityCritical,
		Label:    "Event log clearing",
	},
	{
		Pattern:  "vssadmin delete shadows",
		Severity: SeverityCritical,
		Label:    "Shadow copy deletion (ransomware)",
	},
	{
		Pattern:  "bcdedit",
		Severity: SeverityCritical,
		Label:    "Boot config modification",
	},
	{
		Pattern:  "chkdsk",
		Severity: SeverityLow,
		Label:    "Disk check",
	},
	{
		Pattern:  "format",
		Severity: SeverityCritical,
		Label:    "Disk format command",
	},
}

func scanBatch(name string, data []byte, result *ScanResult) {
	lower := strings.ToLower(string(data))

	for _, p := range batchSuspiciousPatterns {
		if strings.Contains(lower, p.Pattern) {
			result.AddThreat(Threat{
				File:     name,
				Category: "Suspicious Script",
				Detail:   p.Label,
				Severity: p.Severity,
				IOC:      p.Pattern,
			})
		}
	}
}

var ps1SuspiciousPatterns = []struct {
	Pattern  string
	Severity Severity
	Label    string
}{
	{
		Pattern:  "[convert]::frombase64string",
		Severity: SeverityCritical,
		Label:    "Base64 decode (obfuscated payload)",
	},
	{
		Pattern:  "[system.text.encoding]::utf8.getstring",
		Severity: SeverityHigh,
		Label:    "String decode",
	},
	{
		Pattern:  "-join",
		Severity: SeverityMedium,
		Label:    "String join (obfuscation)",
	},
	{
		Pattern:  "char]",
		Severity: SeverityMedium,
		Label:    "Character array construction",
	},
	{
		Pattern:  "invoke-restmethod",
		Severity: SeverityHigh,
		Label:    "PowerShell REST API call",
	},
	{
		Pattern:  "start-bitstransfer",
		Severity: SeverityHigh,
		Label:    "PowerShell BITS transfer",
	},
	{
		Pattern:  "register-scheduledtask",
		Severity: SeverityCritical,
		Label:    "Scheduled task registration",
	},
	{
		Pattern:  "new-scheduledtask",
		Severity: SeverityCritical,
		Label:    "New scheduled task creation",
	},
	{
		Pattern:  "set-mppreference",
		Severity: SeverityCritical,
		Label:    "Defender preference modification",
	},
	{
		Pattern:  "add-mppreference",
		Severity: SeverityCritical,
		Label:    "Defender exclusion addition",
	},
	{
		Pattern:  "disable-realtimeMonitoring",
		Severity: SeverityCritical,
		Label:    "Defender realtime monitoring disable",
	},
	{
		Pattern:  "add-exclusion",
		Severity: SeverityCritical,
		Label:    "Defender exclusion addition",
	},
	{
		Pattern:  "get-ciminstance win32_product",
		Severity: SeverityMedium,
		Label:    "Installed software enumeration",
	},
	{
		Pattern:  "get-wmiobject",
		Severity: SeverityMedium,
		Label:    "WMI query",
	},
	{
		Pattern:  "get-process",
		Severity: SeverityLow,
		Label:    "Process enumeration",
	},
	{
		Pattern:  "stop-process -name",
		Severity: SeverityHigh,
		Label:    "Process termination",
	},
	{
		Pattern:  "disable-computerrestore",
		Severity: SeverityCritical,
		Label:    "System Restore disable",
	},
	{
		Pattern:  "set-itemproperty 'hklm:\\software\\microsoft\\windows\\currentversion\\run",
		Severity: SeverityCritical,
		Label:    "Registry autostart modification",
	},
	{
		Pattern:  "currentversion\\run",
		Severity: SeverityHigh,
		Label:    "Registry Run key modification",
	},
	{
		Pattern:  "bypass",
		Severity: SeverityHigh,
		Label:    "Execution policy bypass",
	},
	{
		Pattern:  "unrestricted",
		Severity: SeverityHigh,
		Label:    "Unrestricted execution policy",
	},
	{
		Pattern:  "getcontent -encoding byte",
		Severity: SeverityHigh,
		Label:    "Byte-encoded file read",
	},
	{
		Pattern:  "[io.file]::writeallbytes",
		Severity: SeverityCritical,
		Label:    "Binary file write",
	},
	{
		Pattern:  "out-file -encoding",
		Severity: SeverityMedium,
		Label:    "Encoded file write",
	},
}

func scanPowerShell(name string, data []byte, result *ScanResult) {
	lower := strings.ToLower(string(data))

	for _, p := range ps1SuspiciousPatterns {
		if strings.Contains(lower, p.Pattern) {
			result.AddThreat(Threat{
				File:     name,
				Category: "Suspicious PowerShell",
				Detail:   p.Label,
				Severity: p.Severity,
				IOC:      p.Pattern,
			})
		}
	}

	scanBatch(name, data, result)
}

var scriptSuspiciousPatterns = []struct {
	Pattern  string
	Severity Severity
	Label    string
}{
	{
		Pattern:  "createobject(\"wscript.shell\")",
		Severity: SeverityHigh,
		Label:    "WScript.Shell creation",
	},
	{
		Pattern:  "createobject(\"scripting.filesystemobject\")",
		Severity: SeverityMedium,
		Label:    "FileSystemObject creation",
	},
	{
		Pattern:  "xmlhttp",
		Severity: SeverityHigh,
		Label:    "XMLHTTP request (download)",
	},
	{
		Pattern:  "msxml2.xmlhttp",
		Severity: SeverityHigh,
		Label:    "MSXML2 HTTP request",
	},
	{
		Pattern:  ".send",
		Severity: SeverityMedium,
		Label:    "HTTP send call",
	},
	{
		Pattern:  "savetofile",
		Severity: SeverityHigh,
		Label:    "Save to file from HTTP",
	},
	{
		Pattern:  "run",
		Severity: SeverityMedium,
		Label:    "WScript.Shell Run method",
	},
	{
		Pattern:  "eval",
		Severity: SeverityHigh,
		Label:    "JavaScript eval (code execution)",
	},
	{
		Pattern:  "executewcript",
		Severity: SeverityCritical,
		Label:    "WSF script execution",
	},
}

func scanScript(name string, data []byte, result *ScanResult) {
	lower := strings.ToLower(string(data))

	for _, p := range scriptSuspiciousPatterns {
		if strings.Contains(lower, p.Pattern) {
			result.AddThreat(Threat{
				File:     name,
				Category: "Suspicious Script",
				Detail:   p.Label,
				Severity: p.Severity,
				IOC:      p.Pattern,
			})
		}
	}
}

func scanUnexpectedExecutables(name string, expectedDirs []string, result *ScanResult) {
	lower := strings.ToLower(name)
	ext := filepath.Ext(lower)

	if ext != ".exe" && ext != ".dll" && ext != ".sys" {
		return
	}

	if len(expectedDirs) > 0 {
		dir := strings.ToLower(filepath.Dir(name))
		allowed := false
		for _, d := range expectedDirs {
			if strings.Contains(dir, strings.ToLower(d)) {
				allowed = true
				break
			}
		}
		if !allowed {
			result.AddThreat(Threat{
				File:     name,
				Category: "Unexpected Executable",
				Detail:   fmt.Sprintf("Executable found outside expected directories: %s", name),
				Severity: SeverityMedium,
				IOC:      name,
			})
		}
	}

	suspiciousNames := []string{
		"svchost", "csrss", "smss", "lsass", "services",
		"wininit", "winlogon", "dwm", "taskhostw",
		"runtimebroker", "searchindexer", "dllhost",
		"conhost", "sihost", "shellexperiencehost",
		"startmenuexperiencehost", "textinputhost",
		"securityhealthservice", "securityhealthsystray",
		"ctfmon", "msdtc", "spoolsv", "svchost32",
	}

	for _, s := range suspiciousNames {
		if lower == s+".exe" {
			result.AddThreat(Threat{
				File:     name,
				Category: "Suspicious Filename",
				Detail:   fmt.Sprintf("File masquerades as system process: %s", name),
				Severity: SeverityCritical,
				IOC:      name,
			})
			break
		}
	}
}

func FormatScanResult(r *ScanResult) string {
	if r.Clean {
		return fmt.Sprintf("Security scan: CLEAN (scanned %d files)", r.ScannedFiles)
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("SECURITY THREAT DETECTED! (found %d threats in %d files)\n", len(r.Threats), r.ScannedFiles))

	for i, t := range r.Threats {
		buf.WriteString(fmt.Sprintf("  [%d] %s | %s\n", i+1, t.Severity, t.Category))
		buf.WriteString(fmt.Sprintf("      File: %s\n", t.File))
		buf.WriteString(fmt.Sprintf("      Detail: %s\n", t.Detail))
		buf.WriteString(fmt.Sprintf("      IOC: %s\n", t.IOC))
	}

	return buf.String()
}
