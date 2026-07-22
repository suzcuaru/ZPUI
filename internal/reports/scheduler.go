package reports

import (
	"fmt"
	"time"

	"zpui/internal/database"
	"zpui/internal/logger"
)

// Scheduler manages periodic report generation.
type Scheduler struct {
	log  *logger.Logger
	gen  *Generator
	stop chan struct{}
}

// NewScheduler creates a new scheduler.
func NewScheduler(log *logger.Logger, gen *Generator) *Scheduler {
	return &Scheduler{log: log, gen: gen, stop: make(chan struct{})}
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
	select {
	case <-s.stop:
		return
	case <-time.After(1 * time.Minute):
	}

	for {
		s.generate(7)

		select {
		case <-s.stop:
			return
		case <-time.After(7 * 24 * time.Hour):
		}
	}
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

	_ = database.SaveDiagnosticReport(&database.DiagnosticReport{
		GeneratedAt: now,
		PeriodStart: &since,
		PeriodEnd:   &now,
		Frequency:   "weekly",
		Content:     content,
	})

	if path, err := SaveToFile(content, ReportFilename()); err == nil {
		s.log.Info("reports", "Saved to "+path)
	}

	s.log.Info("reports", "Report generated successfully")
}