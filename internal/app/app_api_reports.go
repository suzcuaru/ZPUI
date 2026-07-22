package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"zpui/internal/database"
	"zpui/internal/reports"
)

// GenerateAndDownloadReport generates a diagnostic report and opens the Downloads folder.
func (a *App) GenerateAndDownloadReport() map[string]interface{} {
	a.log.Info("reports", "User requested diagnostic report")

	periodDays := 7

	gen := reports.NewGenerator(a.version, a.cfg.GetZapretPath())
	content, err := gen.Generate(periodDays)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("Generate failed: %v", err)}
	}

	now := time.Now()
	since := now.AddDate(0, 0, -periodDays)
	_ = database.SaveDiagnosticReport(&database.DiagnosticReport{
		GeneratedAt: now,
		PeriodStart: &since,
		PeriodEnd:   &now,
		Frequency:   "manual",
		Content:     content,
	})

	filename := reports.ReportFilename()
	path, err := reports.SaveToFile(content, filename)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("Save failed: %v", err)}
	}

	// Open Downloads folder
	home, _ := os.UserHomeDir()
	go exec.Command("explorer", filepath.Join(home, "Downloads")).Start()

	return map[string]interface{}{
		"status":  "ok",
		"path":   path,
		"message": fmt.Sprintf("Saved: %s", filepath.Base(path)),
	}
}

// GetReportHistory returns the list of generated reports.
func (a *App) GetReportHistory(limit int) map[string]interface{} {
	if limit <= 0 { limit = 20 }
	reps, err := database.GetDiagnosticReports(limit)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"reports": reps}
}

// GetReportContent returns the content of a specific report.
func (a *App) GetReportContent(id string) map[string]interface{} {
	reps, err := database.GetDiagnosticReports(100)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	for _, r := range reps {
		if r.ID == id {
			return map[string]interface{}{"content": r.Content, "report": r}
		}
	}
	return map[string]interface{}{"error": "report not found"}
}
