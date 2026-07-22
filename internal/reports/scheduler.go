package reports

import (
	"fmt"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/logger"
)

// Scheduler manages periodic report generation.
type Scheduler struct {
	cfg  *config.Config
	log  *logger.Logger
	gen  *Generator
	stop chan struct{}
}

// NewScheduler creates a new scheduler.
func NewScheduler(cfg *config.Config, log *logger.Logger, gen *Generator) *Scheduler {
	return &Scheduler{cfg: cfg, log: log, gen: gen, stop: make(chan struct{})}
}

// Start launches the report generation loop in a goroutine.
func (s *Scheduler) Start() {
	go s.loop()
}

// Stop terminates the scheduler.
func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) loop() {
	// Wait 1 minute after start
	select {
	case <-s.stop: return
	case <-time.After(1 * time.Minute):
	}

	for {
		freq, period := s.getFrequency()
		interval := freqToDuration(req)
		if interval == 0 {
			select {
			case <-s.stop: return
			case <-time.After(5 * time.Minute):
			}
			continue
		}

		s.generate(period)

		select {
		case <-s.stop: return
		case <-time.After(interval):
		}
	}
}

func (s *Scheduler) getFrequency() (string, int) {
	dr := s.cfg.GetDiagnosticReports()
	if !dr.Enabled { return "", 0 }
	pd := dr.PeriodDays
	if pd <= 0 { pd = 7 }
	return dr.Frequency, pd
}

func (s *Scheduler) generate(periodDays int) {
	s.log.Info("reports", fmt.Sprintf("Generating report (period: %d days)", periodDays))

	content, err := s.gen.Generate(periodDays)
	if err != nil {
		s.log.Error("reports", "Generate failed: "+err.Error())
		return
	}

	now := time.Now()
	since := now.AddDate(0, 0, -periodDays)
	freq, _ := s.getFrequency()

	_ = database.SaveDiagnosticReport(&database.DiagnosticReport{
		GeneratedAt: now,
		PeriodStart: &since,
		PeriodEnd:   &now,
		Frequency:   req,
		Content:     content,
	})

	dr := s.cfg.GetDiagnosticReports()
	if dr.AutoSaveMD {
		if path, err := SaveToFile(content, ReportFilename()); err == nil {
			s.log.Info("reports", "Saved to "+path)
		}
	}

	s.log.Info("reports", "Report generated successfully")
}

func freqToDuration(freq string) time.Duration {
	switch freq {
	case "hourly": return 1 * time.Hour
	case "daily": return 24 * time.Hour
	case "weekly": return 7 * 24 * time.Hour
	case "biweekly": return 14 * 24 * time.Hour
	case "monthly": return 30 * 24 * time.Hour
	default: return 7 * 24 * time.Hour
	}
}