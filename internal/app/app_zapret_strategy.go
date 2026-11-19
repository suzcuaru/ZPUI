package app

import (
	"encoding/json"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/database"
	"zpui/internal/zapret"
)


// ============================================================
// STRATEGIES
// ============================================================

func (a *App) GetStrategies() map[string]interface{} {
	strategies := a.zapret.ListStrategies()
	scores := a.zapret.StrategyScores()
	out := make([]map[string]interface{}, 0, len(strategies))
	for _, s := range strategies {
		row := map[string]interface{}{
			"name":     s.Name,
			"filename": s.Filename,
			"current":  s.Current,
		}
		if info, ok := scores[s.Filename]; ok {
			row["score"] = info.Score
			row["grade"] = info.Grade
			row["availability"] = info.Availability
			row["key_resources"] = info.KeyResources
			row["speed_mbps"] = info.SpeedMbps
			row["ping_ms"] = info.PingMs
			row["weakest"] = info.Weakest
			row["game_score"] = info.GameScore
			row["game_badge"] = info.GameBadge
			row["game_ping_ms"] = info.GamePingMs
			row["game_ok"] = info.GameOk
			row["game_total"] = info.GameTotal
			row["game_gf_tcp"] = info.GameGfTCP
			row["game_gf_udp"] = info.GameGfUDP
		}
		out = append(out, row)
	}
	return map[string]interface{}{"strategies": out}
}

// emitStrategiesChanged — пуш во фронт: данные вкладки стратегий изменились
// (смена стратегии, игровой фильтр, ipset, завершение подбора).
func (a *App) emitStrategiesChanged() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "zapret:strategies-changed", nil)
	}
}

func (a *App) SetStrategy(filename string) map[string]interface{} {
	if filename == "" {
		return errResp("filename required")
	}
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	if err := a.zapret.SetStrategy(filename); err != nil {
		return errResp(err.Error())
	}
	// Проверка доступности после смены запускается хуком OnStrategyApplied
	// (внутри менеджера) — здесь дублировать не нужно.
	a.zapret.LogStrategyChange(filename, "manual")
	a.emitStrategiesChanged()

	op, _ := database.GetCurrentOperator()
	if op != nil && op.OperatorKey != "" {
		displayName := strings.TrimSuffix(filename, ".bat")
		database.EnsureOperatorStrategy(op.OperatorKey, filename, displayName)
		// MarkActiveStrategy инкрементирует use_count, выставляет is_active,
		// last_source и applied_at — именно он делает переключение видимым в отчёте.
		if err := database.MarkActiveStrategy(op.OperatorKey, filename, "manual"); err != nil {
			a.log.Error("strategies", "mark active "+filename+": "+err.Error())
		}
		database.SetCurrentOperator(op.OperatorKey, op.OperatorName, filename)
	}

	return map[string]interface{}{"status": "ok", "strategy": filename}
}

// GetAutoTestResults — агрегированные результаты проверок для текущего
// оператора: по каждой стратегии из operator_strategies берётся её
// auto_test_results (последний тест) и availability_pct. Работает и после
// одиночной проверки (ПКМ), и после частичного автоподбора.
func (a *App) GetAutoTestResults() map[string]interface{} {
	op, _ := database.GetCurrentOperator()
	if op == nil || op.OperatorKey == "" {
		return map[string]interface{}{"results": []interface{}{}}
	}
	rows, err := database.GetOperatorStrategies(op.OperatorKey)
	if err != nil {
		return map[string]interface{}{"results": []interface{}{}}
	}
	// Детект оператора может временно падать: часть автотестов остаётся под
	// fallback-оператором "unknown". Дополняем текущего оператора его
	// неучтёнными результатами, чтобы они не «пропадали» при переключениях.
	if unk, uerr := database.GetOperatorStrategies("unknown"); uerr == nil {
		has := map[string]bool{}
		for _, r := range rows {
			has[r.Strategy] = r.AutoTestResults != "" || r.AvailabilityPct > 0
		}
		for _, u := range unk {
			if !has[u.Strategy] && (u.AutoTestResults != "" || u.AvailabilityPct > 0) {
				rows = append(rows, u)
			}
		}
	}
	results := []map[string]interface{}{}
	for _, r := range rows {
		if r.AutoTestResults == "" && r.AvailabilityPct <= 0 {
			continue
		}
		var detail []map[string]interface{}
		if r.AutoTestResults != "" {
			_ = json.Unmarshal([]byte(r.AutoTestResults), &detail)
		}
		entry := map[string]interface{}{
			"strategy": r.Strategy,
		}
		// Из детального JSON берём ok/total; иначе считаем из pct.
		if len(detail) > 0 {
			d := detail[0]
			if v, ok := d["ok"].(float64); ok {
				entry["ok_count"] = int(v)
			}
			if v, ok := d["total"].(float64); ok {
				entry["total"] = int(v)
			}
			if v, ok := d["avg_ms"].(float64); ok {
				entry["avg_ms"] = v
			}
			// Отрезана ключевым фильтром (этап 1 подбора) — явный маркер
			// для UI, балл такой записи не считается.
			if s, ok := d["status"].(string); ok && s == "filtered" {
				entry["filtered"] = true
			}
		}
		if r.AvailabilityPct > 0 {
			entry["pct"] = int(r.AvailabilityPct)
		}
		if _, ok := entry["pct"]; !ok {
			okC, _ := entry["ok_count"].(int)
			totC, _ := entry["total"].(int)
			if totC > 0 {
				entry["pct"] = okC * 100 / totC
			}
		}
		// Единый балл (доступность + пинг) — та же формула, что ранжирует
		// автоподбор и серверные топы (скорость в балл не входит).
		if pct, ok := entry["pct"].(int); ok {
			ping, _ := parseAutoTestNet(r.AutoTestResults)
			entry["score"] = int(zapret.ScoreStrategy(float64(pct), float64(ping)))
		}
		results = append(results, entry)
	}
	return map[string]interface{}{"results": results}
}

// ============================================================
// SINGLE STRATEGY TEST
// ============================================================

// TestSingleStrategy проверяет одну стратегию из контекстного меню карточки:
// game=false — доступность ресурсов, game=true — игровой режим (игровой балл,
// пинг до платформ; результат пишется в strategy_games). Асинхронная: результат
// приходит через GET /api/strategy/test-single-result (polling) и событие.
func (a *App) TestSingleStrategy(filename string, game bool) map[string]interface{} {
	if filename == "" {
		return errResp("filename required")
	}
	if a.zapret.IsAutoTestRunning() {
		return errResp("автоподбор уже запущен")
	}
	// Свежий набор алгоритма подбора до проверки стратегии (дельта по версии).
	a.RefreshAutoSelectCfg()

	// Состояние проверки — для polling из фронтенда.
	a.singleTestMu.Lock()
	a.singleTestRunning = true
	a.singleTestResult = nil
	a.singleTestMu.Unlock()

	a.emitSafe("strategy:single_test", map[string]interface{}{
		"strategy": filename, "phase": "running", "game": game,
	})

	go func() {
		if game {
			gs, err := a.zapret.TestSingleStrategyGame(filename)
			a.singleTestMu.Lock()
			a.singleTestRunning = false
			if err != nil {
				a.singleTestResult = map[string]interface{}{
					"strategy": filename, "phase": "error", "error": err.Error(), "game": true,
				}
			} else {
				a.singleTestResult = map[string]interface{}{
					"strategy": filename, "phase": "done", "game": true,
					"game_score":   gs.Score,
					"game_ping_ms": gs.PingMs,
					"game_ok":      gs.Ok,
					"game_total":   gs.Total,
				}
			}
			result := a.singleTestResult
			a.singleTestMu.Unlock()
			a.emitSafe("strategy:single_test", result)
			return
		}
		res, err := a.zapret.TestSingleStrategy(filename)
		a.singleTestMu.Lock()
		a.singleTestRunning = false
		if err != nil {
			a.singleTestResult = map[string]interface{}{
				"strategy": filename, "phase": "error", "error": err.Error(),
			}
		} else {
			payload := map[string]interface{}{
				"strategy":   res.Strategy,
				"phase":      "done",
				"passed":     res.Passed,
				"pct":        res.Pct,
				"ok_count":   res.OkCount,
				"total":      res.Total,
				"ping_ms":    res.PingMs,
				"speed_mbps": res.SpeedMbps,
				"score":      res.Score,
				"grade":      res.Grade,
			}
			if res.Error != "" {
				payload["error"] = res.Error
			}
			a.singleTestResult = payload
		}
		result := a.singleTestResult
		a.singleTestMu.Unlock()

		a.emitSafe("strategy:single_test", result)
	}()

	return okResp()
}

// GetSingleTestResult — polling статуса проверки одной стратегии.
func (a *App) GetSingleTestResult() map[string]interface{} {
	a.singleTestMu.Lock()
	defer a.singleTestMu.Unlock()
	return map[string]interface{}{
		"running": a.singleTestRunning,
		"result":  a.singleTestResult,
	}
}

// ============================================================
// AUTO TEST (STRATEGY)
// ============================================================

func (a *App) CancelAutoTest() map[string]interface{} {
	a.cancelSweep()
	a.cancelSelectionRun()
	a.zapret.CancelAutoTest()
	return map[string]interface{}{"status": "cancelled"}
}
