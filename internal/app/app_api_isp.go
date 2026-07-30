package app

import (
	"zpui/internal/database"
)

// GetCurrentOperator возвращает текущего оператора из БД.
func (a *App) GetCurrentOperator() map[string]interface{} {
	op, err := database.GetCurrentOperator()
	if err != nil {
		return errResp("failed to get current operator: " + err.Error())
	}
	if op == nil {
		return okRespWithData(map[string]interface{}{
			"key":  "unknown",
			"name": "",
		})
	}
	return okRespWithData(map[string]interface{}{
		"key":      op.OperatorKey,
		"name":     op.OperatorName,
		"strategy": op.Strategy,
	})
}

// GetOperatorHistory возвращает список всех известных операторов.
func (a *App) GetOperatorHistory() map[string]interface{} {
	operators, err := database.GetAllOperatorInfo()
	if err != nil {
		return errResp("failed to get operator history: " + err.Error())
	}
	if operators == nil {
		operators = []database.OperatorInfo{}
	}
	return okRespWithData(map[string]interface{}{
		"operators": operators,
	})
}
