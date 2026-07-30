package app

import (
	"zpui/internal/executil"
	"zpui/internal/reports"
)

// ============================================================
// REPORTS
// ============================================================

// GetReport генерирует отчёт в процессе (без запуска report.exe)
// и возвращает Markdown-контент для отображения в UI.
func (a *App) GetReport() map[string]interface{} {
	gen := reports.NewGeneratorEx(
		a.cfg.ModVersion,
		a.cfg.GetZapretPath(),
		a.cfg.GetCurrentStrategy(),
		a.cfg.GetServiceCrashCount(),
		a.cfg.GetServiceLastCrash(),
	)
	content, err := gen.Generate(14)
	if err != nil {
		a.log.Error("report", "Generate failed: "+err.Error())
		return errResp("report: " + err.Error())
	}
	return okRespWithData(map[string]interface{}{
		"content": content,
	})
}

// SaveReport сохраняет Markdown-контент в файл в папке Downloads.
func (a *App) SaveReport(body map[string]interface{}) map[string]interface{} {
	content, _ := body["content"].(string)
	if content == "" {
		return errResp("empty content")
	}
	filename := reports.ReportFilename()
	path, err := reports.SaveToFile(content, filename)
	if err != nil {
		a.log.Error("report", "Save failed: "+err.Error())
		return errResp("save: " + err.Error())
	}
	a.log.Info("report", "Report saved: "+path)
	return okRespWithData(map[string]interface{}{
		"path": path,
	})
}

// GenerateReport запускает автономный report.exe (устаревший путь,
// оставлен для совместимости).
func (a *App) GenerateReport() map[string]interface{} {
	reportExe := a.findModulePath("report")
	if reportExe == "" {
		return errResp("report.exe не найден")
	}

	if err := executil.DetachedCmd(reportExe).Start(); err != nil {
		return errResp("не удалось запустить report.exe: " + err.Error())
	}

	return okRespWithData(map[string]interface{}{
		"status": "report_launched",
	})
}
