package app

import (
	"encoding/json"
	"fmt"


	"zpui/internal/database"
)


// ============================================================
// ACTION LOGS
// ============================================================

// FrontendLogs — batch логов от фронтенда (замена POST /api/logs/frontend).
func (a *App) FrontendLogs(events []interface{}) map[string]interface{} {
	if len(events) > 100 {
		events = events[:100]
	}

	var logs []database.ActionLog
	for _, ev := range events {
		event, ok := ev.(map[string]interface{})
		if !ok {
			continue
		}

		details, _ := json.Marshal(map[string]interface{}{
			"category": event["category"],
			"action":   event["action"],
			"details":  event["details"],
		})

		logs = append(logs, database.ActionLog{
			Category: "frontend",
			Action:   fmt.Sprintf("%v", event["action"]),
			Details:  string(details),
		})
	}

	if err := database.BatchInsertActionLogs(logs); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	return map[string]interface{}{
		"status": "ok",
		"count":  len(logs),
	}
}
