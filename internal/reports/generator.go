package reports

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"zpui/internal/database"
	"zpui/internal/logdb"
	"zpui/internal/sysinfo"
)

type SystemInfo struct {
	OS           string
	CPUModel     string
	NumCores     int
	SystemRAMMB  float64
	AvailableRAM float64
}

type ReportData struct {
	ReportDate        string
	ZPUIVersion       string
	ZapretVersion     string
	Strategy          string
	PeriodDays        int
	PeriodStart       time.Time
	PeriodEnd         time.Time
	Availability      []database.AvailabilityRecord
	Devices           []database.SessionDevice
	Snapshots         []database.TrafficSnapshot
	ActionLogs        []database.ActionLog
	SystemInfo        SystemInfo
	Operator          *database.CurrentOperator
	OperatorInfo      *database.OperatorInfo
	ComponentVersions []database.ComponentVersion
	Strategies        []database.OperatorStrategy
	ResourceDaily     []database.ResourceDaily
	LogErrors         []logdb.LogEntry
	TotalErrors       int64
	ServiceCrashCount int
	ServiceLastCrash  int64
}

type Generator struct {
	version           string
	zapretDir         string
	strategy          string
	serviceCrashCount int
	serviceLastCrash  int64
}

func NewGenerator(version, zapretDir, strategy string) *Generator {
	return &Generator{version: version, zapretDir: zapretDir, strategy: strategy}
}

func NewGeneratorEx(version, zapretDir, strategy string, crashCount int, lastCrash int64) *Generator {
	return &Generator{
		version:           version,
		zapretDir:         zapretDir,
		strategy:          strategy,
		serviceCrashCount: crashCount,
		serviceLastCrash:  lastCrash,
	}
}

func (g *Generator) Generate(periodDays int) (string, error) {
	if periodDays <= 0 {
		periodDays = 7
	}

	now := time.Now()
	since := now.AddDate(0, 0, -periodDays)

	data := ReportData{
		ReportDate:  now.Format("2006-01-02 15:04:05"),
		ZPUIVersion: g.version,
		PeriodDays:  periodDays,
		PeriodStart: since,
		PeriodEnd:   now,
	}

	zapretVerPath := filepath.Join(g.zapretDir, "version.txt")
	if v, err := os.ReadFile(zapretVerPath); err == nil {
		data.ZapretVersion = strings.TrimSpace(string(v))
	}
	if data.ZapretVersion == "" {
		binVerPath := filepath.Join(g.zapretDir, "bin", "version.txt")
		if v, err := os.ReadFile(binVerPath); err == nil {
			data.ZapretVersion = strings.TrimSpace(string(v))
		}
	}
	data.Strategy = g.strategy
	data.ServiceCrashCount = g.serviceCrashCount
	data.ServiceLastCrash = g.serviceLastCrash

	if avail, err := database.GetAvailabilityHistory(since); err == nil {
		data.Availability = avail
	}
	if devs, err := database.GetAllDevices(); err == nil {
		data.Devices = devs
	}
	if snaps, err := database.GetSnapshots(since); err == nil {
		data.Snapshots = snaps
	}
	if logs, err := database.GetActionLogs("", 100, 0); err == nil {
		data.ActionLogs = logs
	}

	if op, err := database.GetCurrentOperator(); err == nil && op != nil {
		data.Operator = op
		if info, err := database.GetOperatorInfo(op.OperatorKey); err == nil {
			data.OperatorInfo = info
		}
		if strategies, err := database.GetOperatorStrategies(op.OperatorKey); err == nil {
			data.Strategies = strategies
		}
	}

	if cvs, err := database.GetAllComponentVersions(); err == nil {
		data.ComponentVersions = cvs
	}

	if daily, err := database.GetResourceDailyHistory("", 30); err == nil {
		data.ResourceDaily = daily
	}

	data.SystemInfo = collectSystemInfo()

	if errs, err := logdb.QueryRecentErrors(50); err == nil {
		data.LogErrors = errs
	}
	data.TotalErrors = logdb.CountErrors()

	return renderMD(data), nil
}

func collectSystemInfo() SystemInfo {
	si := SystemInfo{}
	sr := sysinfo.GetSystemResources()
	si.NumCores = sr.NumCores
	si.SystemRAMMB = sr.SystemRAMMB
	si.AvailableRAM = sr.AvailableRAM

	if out, err := exec.Command("cmd", "/c", "ver").Output(); err == nil {
		si.OS = strings.TrimSpace(strings.ReplaceAll(string(out), "\r\n", ""))
	}
	if si.OS == "" {
		si.OS = "Windows (version unknown)"
	}

	if out, err := exec.Command("reg", "query",
		`HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0`,
		"/v", "ProcessorNameString").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "ProcessorNameString") {
				if idx := strings.Index(line, "REG_SZ"); idx >= 0 {
					si.CPUModel = strings.TrimSpace(line[idx+6:])
				}
			}
		}
	}
	if si.CPUModel == "" {
		si.CPUModel = "Unknown"
	}

	return si
}

func renderMD(d ReportData) string {
	var b strings.Builder

	b.WriteString("# ZPUI Diagnostic Report\n\n")
	b.WriteString(fmt.Sprintf("- **Date:** %s\n", d.ReportDate))
	b.WriteString(fmt.Sprintf("- **ZPUI Version:** %s\n", d.ZPUIVersion))
	if d.ZapretVersion == "" {
		b.WriteString("- **Zapret Version:** _not found_\n")
	} else {
		b.WriteString(fmt.Sprintf("- **Zapret Version:** %s\n", d.ZapretVersion))
	}
	b.WriteString(fmt.Sprintf("- **Strategy:** %s\n", d.Strategy))
	b.WriteString(fmt.Sprintf("- **Period:** %s — %s (%d days)\n\n",
		d.PeriodStart.Format("2006-01-02"), d.PeriodEnd.Format("2006-01-02"), d.PeriodDays))

	b.WriteString("## System Information\n\n")
	b.WriteString(fmt.Sprintf("- **OS:** %s\n", d.SystemInfo.OS))
	b.WriteString(fmt.Sprintf("- **CPU:** %s (%d cores)\n", d.SystemInfo.CPUModel, d.SystemInfo.NumCores))
	b.WriteString(fmt.Sprintf("- **RAM:** %.0f MB total, %.0f MB available\n",
		d.SystemInfo.SystemRAMMB, d.SystemInfo.AvailableRAM))
	if d.ServiceCrashCount > 0 {
		lastCrashStr := "—"
		if d.ServiceLastCrash > 0 {
			lastCrashStr = time.Unix(d.ServiceLastCrash, 0).Format("2006-01-02 15:04")
		}
		b.WriteString(fmt.Sprintf("- **Service crashes:** %d (last: %s)\n", d.ServiceCrashCount, lastCrashStr))
	}
	b.WriteString("\n")

	if d.Operator != nil {
		b.WriteString("## Operator\n\n")
		b.WriteString(fmt.Sprintf("- **Name:** %s\n", d.Operator.OperatorName))
		b.WriteString(fmt.Sprintf("- **Key:** %s\n", d.Operator.OperatorKey))
		if d.OperatorInfo != nil {
			b.WriteString(fmt.Sprintf("- **ISP:** %s\n", d.OperatorInfo.ISP))
			b.WriteString(fmt.Sprintf("- **ASN:** %s\n", d.OperatorInfo.ASN))
			b.WriteString(fmt.Sprintf("- **City:** %s\n", d.OperatorInfo.City))
			b.WriteString(fmt.Sprintf("- **Organization:** %s\n", d.OperatorInfo.Org))
		}
		b.WriteString("\n")
	}

	if len(d.ComponentVersions) > 0 {
		b.WriteString("## Component Versions\n\n")
		b.WriteString("| Component | Installed | Remote | Source |\n|-----------|-----------|--------|--------|\n")
		for _, cv := range d.ComponentVersions {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				cv.ID, cv.InstalledVersion, cv.RemoteVersion, cv.RemoteSource))
		}
		b.WriteString("\n")
	}

	if len(d.Strategies) > 0 {
		b.WriteString(fmt.Sprintf("## Strategies (%d)\n\n", len(d.Strategies)))
		b.WriteString("| Strategy | Active | Availability | Use Count | Source | Last Applied |\n")
		b.WriteString("|----------|--------|-------------|-----------|--------|-------------|\n")
		for _, s := range d.Strategies {
			active := ""
			if s.IsActive {
				active = "✓"
			}
			avail := "—"
			if s.AvailabilityPct >= 0 {
				avail = fmt.Sprintf("%.0f%%", s.AvailabilityPct)
			}
			applied := "—"
			if s.AppliedAt != nil {
				applied = s.AppliedAt.Format("2006-01-02 15:04")
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s | %s |\n",
				s.Strategy, active, avail, s.UseCount, s.LastSource, applied))
		}
		b.WriteString("\n")
	}

	if len(d.Availability) > 0 {
		var sum float64
		for _, a := range d.Availability {
			sum += a.Pct
		}
		avg := sum / float64(len(d.Availability))

		b.WriteString(fmt.Sprintf("## Resource Availability (avg: %.1f%%, %d samples)\n\n", avg, len(d.Availability)))

		byType := map[string][]database.AvailabilityRecord{}
		for _, a := range d.Availability {
			byType[a.Type] = append(byType[a.Type], a)
		}

		for typ, records := range byType {
			var typeSum float64
			for _, r := range records {
				typeSum += r.Pct
			}
			typeAvg := typeSum / float64(len(records))
			b.WriteString(fmt.Sprintf("### %s (avg: %.1f%%, %d samples)\n\n", typ, typeAvg, len(records)))

			step := 1
			if len(records) > 30 {
				step = len(records) / 30
			}
			b.WriteString("| Time | OK/Total | % |\n|------|----------|----|\n")
			for i := 0; i < len(records); i += step {
				a := records[i]
				b.WriteString(fmt.Sprintf("| %s | %d/%d | %.0f%% |\n",
					a.Timestamp.Format("01-02 15:04"), a.OKResources, a.TotalResources, a.Pct))
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("## Resource Availability\n\nNo data for this period.\n\n")
	}

	if len(d.ResourceDaily) > 0 {
		b.WriteString("## Per-Resource Availability (30-day daily)\n\n")
		byHost := map[string][]database.ResourceDaily{}
		for _, r := range d.ResourceDaily {
			byHost[r.Host] = append(byHost[r.Host], r)
		}

		hosts := make([]string, 0, len(byHost))
		for h := range byHost {
			hosts = append(hosts, h)
		}
		sort.Strings(hosts)

		b.WriteString("| Resource | Days Checked | Avg % | Sparkline |\n|----------|-------------|-------|-----------|\n")
		for _, host := range hosts {
			days := byHost[host]
			var sumPct float64
			for _, d := range days {
				sumPct += d.Pct
			}
			avgPct := sumPct / float64(len(days))
			b.WriteString(fmt.Sprintf("| %s | %d | %.0f%% | %s |\n", host, len(days), avgPct, sparkline(days)))
		}
		b.WriteString("\n")
	}

	if len(d.Snapshots) > 0 {
		first := d.Snapshots[0]
		last := d.Snapshots[len(d.Snapshots)-1]
		totalDL := int64(0)
		totalUL := int64(0)
		if last.TotalDL > first.TotalDL {
			totalDL = last.TotalDL - first.TotalDL
		}
		if last.TotalUL > first.TotalUL {
			totalUL = last.TotalUL - first.TotalUL
		}

		const maxReasonableSpeed = float64(10 * 1024 * 1024 * 1024)
		var maxDL, maxUL float64
		for _, s := range d.Snapshots {
			if s.DLSpeed > 0 && s.DLSpeed < maxReasonableSpeed && s.DLSpeed > maxDL {
				maxDL = s.DLSpeed
			}
			if s.ULSpeed > 0 && s.ULSpeed < maxReasonableSpeed && s.ULSpeed > maxUL {
				maxUL = s.ULSpeed
			}
		}

		b.WriteString("## Traffic Summary\n\n")
		b.WriteString(fmt.Sprintf("- Peak download: %s/s\n", formatSpeed(maxDL)))
		b.WriteString(fmt.Sprintf("- Peak upload: %s/s\n", formatSpeed(maxUL)))
		b.WriteString(fmt.Sprintf("- Total download: %s (system traffic)\n", formatBytes(uint64(totalDL))))
		b.WriteString(fmt.Sprintf("- Total upload: %s (system traffic)\n", formatBytes(uint64(totalUL))))
		b.WriteString(fmt.Sprintf("- Samples: %d\n\n", len(d.Snapshots)))
	}

	if len(d.Devices) > 0 {
		b.WriteString("## Connected Devices\n\n")
		b.WriteString("| Hostname | IP | MAC | Download | Upload | Status |\n|----------|-----|-----|----------|--------|--------|\n")
		for _, dev := range d.Devices {
			status := "offline"
			if dev.IsOnline {
				status = "online"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				dev.Hostname, dev.IP, dev.MAC,
				formatBytes(uint64(dev.TotalDL)),
				formatBytes(uint64(dev.TotalUL)),
				status))
		}
		b.WriteString("\n")
	}

	if len(d.ActionLogs) > 0 {
		b.WriteString(fmt.Sprintf("## Recent Actions (%d)\n\n", len(d.ActionLogs)))
		b.WriteString("| Time | Category | Action | Details |\n|------|----------|--------|---------|\n")
		n := len(d.ActionLogs)
		if n > 50 {
			n = 50
		}
		for _, l := range d.ActionLogs[:n] {
			action := l.Action
			if len(action) > 40 {
				action = action[:37] + "..."
			}
			details := l.Details
			if len(details) > 40 {
				details = details[:37] + "..."
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				l.Timestamp.Format("01-02 15:04"), l.Category, action, details))
		}
		b.WriteString("\n")
	}

	if len(d.LogErrors) > 0 || d.TotalErrors > 0 {
		errCount := 0
		warnCount := 0
		for _, e := range d.LogErrors {
			if e.Level == "ERROR" {
				errCount++
			} else {
				warnCount++
			}
		}
		b.WriteString(fmt.Sprintf("## Logs & Errors (total errors in DB: %d)\n\n", d.TotalErrors))
		if len(d.LogErrors) > 0 {
			b.WriteString(fmt.Sprintf("Recent %d entries (showing last %d):\n\n", len(d.LogErrors), len(d.LogErrors)))
			b.WriteString("| Time | Level | Source | Category | Message |\n")
			b.WriteString("|------|-------|--------|----------|--------|\n")
			for _, e := range d.LogErrors {
				msg := e.Message
				if len(msg) > 80 {
					msg = msg[:77] + "..."
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					e.Timestamp, e.Level, e.Table, e.Category, msg))
			}
		} else {
			b.WriteString("No recent errors or warnings.\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("---\nGenerated by ZPUI Report Module\n")
	return b.String()
}

var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func sparkline(days []database.ResourceDaily) string {
	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	var b strings.Builder
	for _, d := range days {
		idx := int(d.Pct / 100.0 * float64(len(sparkBlocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		b.WriteRune(sparkBlocks[idx])
	}
	return b.String()
}

func SaveToFile(content, filename string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	downloadsDir := filepath.Join(home, "Downloads")
	os.MkdirAll(downloadsDir, 0755)
	p := filepath.Join(downloadsDir, filename)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		return "", err
	}
	return p, nil
}

func ReportFilename() string {
	return fmt.Sprintf("ZPUI_Report_%s.md", time.Now().Format("2006-01-02_150405"))
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatSpeed(bps float64) string {
	const (
		KB = 1024.0
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bps >= GB:
		return fmt.Sprintf("%.1f GiB", bps/GB)
	case bps >= MB:
		return fmt.Sprintf("%.1f MiB", bps/MB)
	case bps >= KB:
		return fmt.Sprintf("%.1f KiB", bps/KB)
	default:
		return fmt.Sprintf("%.0f B", bps)
	}
}
