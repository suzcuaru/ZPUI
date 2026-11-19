package app

import (
	"strings"

	"zpui/internal/database"
)


// ============================================================
// AUTOSWITCH
// ============================================================

func (a *App) GetAutoSwitchStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled":   a.cfg.GetAutoSwitchEnabled(),
		"threshold": a.cfg.GetAutoSwitchThresholdPct(),
		"interval":  a.cfg.GetAutoSwitchIntervalMin(),
	}
}

func (a *App) SetAutoSwitchEnabled(enabled bool) map[string]interface{} {
	if err := a.cfg.SetAutoSwitchEnabled(enabled); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}

func (a *App) SetAutoSwitchConfig(threshold, interval int) map[string]interface{} {
	if threshold >= 10 && threshold <= 100 {
		if err := a.cfg.SetAutoSwitchThresholdPct(threshold); err != nil {
			return errResp(err.Error())
		}
	}
	if interval >= 1 && interval <= 60 {
		if err := a.cfg.SetAutoSwitchIntervalMin(interval); err != nil {
			return errResp(err.Error())
		}
	}
	return okResp()
}

func (a *App) GetStrategyChangeHistory() map[string]interface{} {
	logs, err := database.GetActionLogs("zapret", 100, 0)
	if err != nil {
		return errResp(err.Error())
	}
	var changes []map[string]interface{}
	for _, l := range logs {
		if l.Action == "strategy_applied" {
			parts := strings.SplitN(l.Details, "|", 2)
			strategy := ""
			source := ""
			if len(parts) > 0 {
				strategy = parts[0]
			}
			if len(parts) > 1 {
				source = parts[1]
			}
			changes = append(changes, map[string]interface{}{
				"timestamp": l.Timestamp,
				"strategy":  strategy,
				"source":    source,
			})
		}
	}
	return map[string]interface{}{"changes": changes}
}
