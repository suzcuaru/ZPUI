package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/logger"
	"zpui/internal/reports"
	"zpui/internal/winprogress"
)

var version = "1.0.0"

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	logFile := filepath.Join(exeDir, "logs", "report.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), exeDir)
	defer logMgr.Close()

	logMgr.Info("report", fmt.Sprintf("Report module started (v%s)", version))

	pw := winprogress.New(fmt.Sprintf("ZPUI Report v%s", version))
	defer func() {
		pw.Close()
		pw.WaitClosed()
	}()

	pw.SetStatus("Чтение конфигурации...")
	pw.SetProgress(10)
	time.Sleep(300 * time.Millisecond)

	configPath := filepath.Join(exeDir, "config.json")
	cfg := config.Load(configPath, exeDir)

	dbPath := filepath.Join(exeDir, "zpui.db")
	if err := database.Init(dbPath); err != nil {
		logMgr.Error("report", "Database init failed: "+err.Error())
		pw.SetStatus("✗ Ошибка: база данных не найдена")
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}
	defer database.Close()

	pw.SetStatus("Сбор данных за 14 дней...")
	pw.SetProgress(30)
	time.Sleep(300 * time.Millisecond)

	gen := reports.NewGenerator(
		cfg.ModVersion,
		cfg.GetZapretPath(),
		cfg.CurrentStrategy,
	)

	pw.SetStatus("Генерация отчёта...")
	pw.SetProgress(60)

	periodDays := 14
	content, err := gen.Generate(periodDays)
	if err != nil {
		logMgr.Error("report", "Generate failed: "+err.Error())
		pw.SetStatus("✗ Ошибка генерации: " + err.Error())
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}

	pw.SetStatus("Сохранение файла...")
	pw.SetProgress(85)

	filename := reports.ReportFilename()
	savedPath, err := reports.SaveToFile(content, filename)
	if err != nil {
		logMgr.Error("report", "Save failed: "+err.Error())
		pw.SetStatus("✗ Ошибка сохранения: " + err.Error())
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}

	logMgr.Info("report", "Report saved to: "+savedPath)

	pw.SetProgress(100)
	pw.SetStatus(fmt.Sprintf("✓ Отчёт сохранён: %s", filename))

	time.Sleep(8 * time.Second)
}
