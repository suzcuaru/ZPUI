package reports

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/database"
	"zpui/internal/monitor"
)

// ReportData contains all data for generating a diagnostic report.
type ReportData struct {
	DeviceName    string
	ReportDate    string
	ZPUIVersion   string
	ZapretVersion string
	Strategy      string
	OS            string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	Frequency     string
	ErrorStats    []map[string]interface{}
	ErrorLogs     []database.ErrorLog
	Availability  []database.AvailabilityRecord
	TrafficTotal  uint64
	TrafficPeakDL float64
	Devices       []database.SessionDevice
}

// Generator creates diagnostic reports in Markdown format.
type Generator struct {
	version   string
	zapretDir string
}

// NewGenerator creates a new report generator.
func NewGenerator(version, zapretDir string) *Generator {
	return &Generator{version: version, zapretDir: zapretDir}
}

// Generate creates an MD report for the given period (in days).
func (g *Generator) Generate(periodDays int) (string, error) {
	now := time.Now()
	since := now.AddDate(0, 0, -periodDays)

	data := ReportData{
		ReportDate:  now.Format("2006-01-02 15:04:05"),
		ZPUIVersion: g.version,
		OS:          getOSVersion(),
		PeriodStart: since,
		PeriodEnd:   now,
	}

	// Zapret version
	if v, err := os.ReadFile(filepath.Join(g.zapretDir, "version.txt")); err == nil {
		data.ZapretVersion = strings.TrimSpace(string(v))
	}

	// Error stats
	if stats, err := database.GetErrorStats(since); err == nil {
		data.ErrorStats = stats
	}

	// Error logs (last 30)
	if logs, err := database.GetErrorLogs(since, 30, 0); err == nil {
		data.ErrorLogs = logs
	}

	// Availability
	if avail, err := database.GetAvailabilityHistory(since); err == nil {
		data.Availability = avail
	}

	// Devices
	if devs, err := database.GetAllDevices(); err == nil {
		data.Devices = devs
	}

	return renderMD(data), nil
}

func renderMD(d ReportData) string {
	var b strings.Builder

	b.WriteString("# ZPUI Diagnostic Report\n\n")
	b.WriteString(fmt.Sprintf("- **Date:** %s\n", d.ReportDate))
	b.WriteString(fmt.Sprintf("- **ZPUI Version:** %s\n", d.ZPUIVersion))
	b.WriteString(fmt.Sprintf("- **Zapret Version:** %s\n", d.ZapretVersion))
	b.WriteString(fmt.Sprintf("- **Strategy:** %s\n", d.Strategy))
	b.WriteString(fmt.Sprintf("- **OS:** %s\n", d.OS))
	b.WriteString(fmt.Sprintf("- **Period:** %s — %s (%d days)\n\n",
		d.PeriodStart.Format("2006-01-02"), d.PeriodEnd.Format("2006-01-02"),
		int(d.PeriodEnd.Sub(d.PeriodStart).Hours()/24)+1))

	// Errors
	totalErrors := 0
	for _, s := range d.ErrorStats {
		if c, ok := s["count"].(int); ok { totalErrors += c }
	}
	b.WriteString(fmt.Sprintf("## Errors (%d total)\n\n", totalErrors))
	if len(d.ErrorStats) > 0 {
		b.WriteString("| Category | Level | Count |\n|----------|-------|-------|\n")
		for _, s := range d.ErrorStats {
			b.WriteString(fmt.Sprintf("| %s | %s | %v |\n", s["category"], s["level"], s["count"]))
		}
	}
	if len(d.ErrorLogs) > 0 {
		b.WriteString("\n### Recent Errors\n\n")
		b.WriteString("| Time | Category | Message |\n|------|----------|---------|\n")
		n := len(d.ErrorLogs)
		if n > 20 { n = 20 }
		for _, e := range d.ErrorLogs[:n] {
			msg := e.Message
			if len(msg) > 80 { msg = msg[:77] + "..." }
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", e.Timestamp.Format("15:04 01-02"), e.Category, msg))
		}
	}
	b.WriteString("\n")

	// Availability
	if len(d.Availability) > 0 {
		var sum float64
		for _, a := range d.Availability { sum += a.Pct }
		avg := 0.0
		if len(d.Availability) > 0 { avg = sum / float64(len(d.Availability)) }
		b.WriteString(fmt.Sprintf("## Resource Availability (avg: %.1f%%)\n\n", avg))
		b.WriteString("| Time | Type | OK/Total | %% |\n|------|------|----------|----|\n")
		for _, a := range d.Availability {
			b.WriteString(fmt.Sprintf("| %s | %s | %d/%d | %.0f%% |\n",
				a.Timestamp.Format("01-02 15:04"), a.Type, a.OKResources, a.TotalResources, a.Pct))
		}
		b.WriteString("\n")
	}

	// Devices
	if len(d.Devices) > 0 {
		b.WriteString("## Connected Devices\n\n")
		b.WriteString("| Hostname | IP | MAC | DL | UL |\n|----------|-----|-----|----|----|\n")
		for _, dev := range d.Devices {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				dev.Hostname, dev.IP, dev.MAC,
				monitor.FormatBytes(uint64(dev.TotalDL)),
				monitor.FormatBytes(uint64(dev.TotalUL))))
		}
	}

	return b.String()
}

// SaveToFile saves report content to the user's Downloads folder.
func SaveToFile(content, filename string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil { return "", err }
	p := filepath.Join(home, "Downloads", filename)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil { return "", err }
	return p, nil
}

// ReportFilename generates a timestamped report filename.
func ReportFilename() string {
	return fmt.Sprintf("ZPUI_Report_%s.md", time.Now().Format("2006-01-02_150405"))
}

func getOSVersion() string {
	// Placeholder — in real code use golang.org/x/sys/windows
	return "Windows"
}