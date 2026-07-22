package autoselect

import (
	"context"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

// Result — итог автоподбора стратегии.
type Result struct {
	Applied  bool   `json:"applied"`
	Strategy string `json:"strategy,omitempty"`
	Error    string `json:"error,omitempty"`
}

// RunWithManager запускает автоподбор на готовом менеджере zapret.
// Единственный движок подбора — zapret.Manager.AutoSelectAndApply:
// тестирует стратегии, сортирует по скору и применяет лучшую с проверкой.
func RunWithManager(ctx context.Context, m *zapret.Manager, onResult func(zapret.AutoTestResult)) Result {
	if m.GetStatus() == zapret.StatusRunning {
		m.Stop()
		time.Sleep(2 * time.Second)
	}

	results := make(chan zapret.AutoTestResult, 50)
	done := make(chan struct{})
	go m.AutoSelectAndApply(ctx, results, done)

	res := Result{}
	for {
		select {
		case r, ok := <-results:
			if !ok {
				return res
			}
			if onResult != nil {
				onResult(r)
			}
			if r.Type == "done" {
				if r.Error != "" {
					res.Error = r.Error
				} else {
					res.Applied = true
				}
			}
			if r.Type == "result" && r.Strategy != "" {
				res.Strategy = r.Strategy
			}
		case <-done:
			return res
		case <-ctx.Done():
			m.CancelAutoTest()
			res.Error = "отменено"
			return res
		}
	}
}

// Run — автономный запуск (для CLI-обёртки): создаёт менеджер из cfg/log.
func Run(ctx context.Context, cfg *config.Config, log *logger.Logger, onResult func(zapret.AutoTestResult)) Result {
	return RunWithManager(ctx, zapret.NewManager(cfg, log), onResult)
}
