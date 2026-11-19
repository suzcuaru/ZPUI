package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/database"
	"zpui/internal/zapret"
)


// ============================================================
// STREAMING (SSE → Wails Events)
// ============================================================

// autoSelectEvent — событие автоподбора: результат теста + режим запуска.
// Phase — фаза конвейера (operator/server/local/analyze/select) у info-событий
// до перебора; на progress/result перекачивается фаза прогона из
// AutoTestResult (start/test/apply), иначе поле с тегом phase перекрыло бы её.
type autoSelectEvent struct {
	zapret.AutoTestResult
	Mode  string `json:"mode"`
	Phase string `json:"phase,omitempty"`
	// Total — размер этапа, когда известен: UI показывает «проверено X из N».
	Total int `json:"total,omitempty"`
}

// emitAutoSelectPhase — info-событие фазы конвейера подбора + строка в лог.
func (a *App) emitAutoSelectPhase(mode, phase, message string, total ...int) {
	a.log.Info("autoselect", fmt.Sprintf("phase=%s mode=%s: %s", phase, mode, message))
	ev := autoSelectEvent{
		AutoTestResult: zapret.AutoTestResult{Type: "info", Message: message},
		Mode:           mode,
		Phase:          phase,
	}
	if len(total) > 0 {
		ev.Total = total[0]
	}
	runtime.EventsEmit(a.ctx, "autoselect:event", ev)
}

// RunAutoSelectStream — автоподбор лучшей стратегии с применением.
// mode: "smart" (умный подбор: пул сервер ≥70 ∪ ПК ≥70 + 2 случайные,
// без лимитов; сильных/данных нет — все general*.bat; применяется лучшая,
// набравшая ≥70), "game" (игровой подбор: strategies — CSV выбранных в UI
// стратегий, пусто — топ-5 по баллам; только игровые цели, применяется
// лучшая по игровому баллу) или "full"/"" (классический полный перебор).
// Эмитит события: autoselect:event — фазы конвейера type=info с phase
// (smart/game: operator → [server] → local → analyze → select, затем
// существующие progress/result; + mode), autoselect:done (mode/cancelled;
// reason — источник кандидатов умного подбора: server_plan | server_top |
// local_data | local_empty_all | no_operator).
// onlyStrategy — если задан, тестируется только эта стратегия (ПКМ).
// missingOnly — только стратегии без данных в БД (вне UI, оставлен для API).
// withGame — игровой прогон финалистов (галочка в модалке, по умолчанию выкл).
// strategies — CSV имён *.bat для mode=game.
func (a *App) RunAutoSelectStream(mode string, onlyStrategy string, missingOnly bool, withGame bool, strategies string) {
	if mode == "" {
		mode = "full"
	}
	// Свежий набор алгоритма подбора ДО старта (дельта по версии; сети нет —
	// идём на текущем). См. app_autoselectcfg.go.
	a.RefreshAutoSelectCfg()
	// Новый прогон не стартует, пока старый в хвосте (5с-такт, restore):
	// иначе новый прогон получает чужой restore и чужие замеры.
	if a.zapret.IsAutoTestRunning() || a.selectionRunning.Load() {
		a.log.Warn("autoselect", "already running, aborting")
		runtime.EventsEmit(a.ctx, "autoselect:done", map[string]interface{}{"error": "already_running", "mode": mode})
		return
	}
	// Пути с реальным прогоном движка; стартовый quick-путь (стратегия уже
	// работает, замер без касания движка) и ПКМ флаг selectionRunning не ставят.
	runPath := mode == "smart" || mode == "game" || ((mode == "full" || mode == "") && onlyStrategy == "")

	go func() {
		start := time.Now()
		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.setSelectionCancel(cancel)
		if runPath {
			a.selectionRunning.Store(true)
		}
		// Мёртвые домены — в пользовательские исключения до подбора: они не
		// должны занижать балл стратегий (стартовый экран идёт тем же путём).
		if onlyStrategy == "" && (mode == "full" || mode == "smart" || mode == "") {
			a.collectDeadDomainsBeforeSelect()
			if runCtx.Err() != nil {
				a.selectionRunning.Store(false)
				runtime.EventsEmit(a.ctx, "autoselect:done", map[string]interface{}{"mode": mode, "cancelled": true})
				return
			}
		}
		if mode == "smart" {
			a.runSmartSelect(runCtx, withGame, start)
			return
		}
		if mode == "startup" {
			// Стартовый экран: сначала прошлая стратегия оператора, при
			// слабом балле — добор по пулу ≥70 (+2 случайные).
			// Заявка — синхронно: первый такт проверки ресурсов ждёт завершения.
			a.startupSelectRequested.Store(true)
			a.runStartupSelect(runCtx, withGame, start)
			return
		}
		if mode == "game" {
			a.runGameSelect(runCtx, onlyStrategy, strategies, start)
			return
		}
		// План полного прогона — весь список с диска (модалка показывает строки).
		var participants []string
		if onlyStrategy == "" {
			all := a.zapret.ListStrategies()
			participants = make([]string, 0, len(all))
			allItems := make([]smartPlanItem, 0, len(all))
			for _, f := range all {
				participants = append(participants, f.Filename)
				allItems = append(allItems, smartPlanItem{Name: f.Filename, Source: "all"})
			}
			runtime.EventsEmit(a.ctx, "autoselect:event", map[string]interface{}{
				"type": "plan", "mode": mode, "names": participants,
				"items": allItems, "scenario": 1,
			})
		}
		engineDown := false
		if onlyStrategy == "" {
			engineDown = a.measureRunBase(runCtx, mode)
			if runCtx.Err() != nil {
				a.selectionRunning.Store(false)
				runtime.EventsEmit(a.ctx, "autoselect:done", map[string]interface{}{"mode": mode, "cancelled": true})
				a.emitStrategiesChanged()
				return
			}
		}
		valid, cancelled, stopped := a.pumpAutoSelect(runCtx, mode, onlyStrategy, missingOnly, nil, withGame, false)
		if engineDown && a.zapret.GetStatus() != zapret.StatusRunning {
			a.restoreEngineAfterBase(a.cfg.GetCurrentStrategy())
		}
		if stopped {
			a.selectionRunning.Store(false)
			a.emitStrategiesChanged()
			return
		}
		a.emitAutoSelectDone(mode, cancelled, nil)
		a.logAutoSelectFinish(mode, a.cfg.GetCurrentStrategy(), cancelled, start)
		// Полный прогон освежает метрики для умного подбора — запоминаем.
		if mode == "full" && !missingOnly && onlyStrategy == "" {
			if valid > 0 {
				_ = database.SetFlag("full_sweep_last_at", strconv.FormatInt(time.Now().Unix(), 10))
			}
			// Эра данных оператора = этот прогон: участники сохраняют свежие
			// записи, остальные стратегии оператора становятся «непроверенными».
			a.wipeOperatorEra(participants)
		}
		a.emitStrategiesChanged()
		if cancelled {
			a.selectionRunning.Store(false)
			return
		}
		// Лучшая применялась внутри прогона (хук приложений подавлен) —
		// первый такт доступности делает хвост прогона.
		if runPath {
			a.finishSelectionRun(runCtx, mode, a.cfg.GetCurrentStrategy())
		} else {
			a.scheduleResourceCheckAfterStrategyChange()
		}
	}()
}

// runSmartSelect — умный подбор: пул кандидатов smartPool (сервер ≥70 ∪
// ПК ≥70 + 2 случайные, без лимитов; сильных нет — полный перебор),
// двухэтапный прогон (быстрый фильтр по ключевым → полный тест финалистов)
// и применение лучшей (≥70).
func (a *App) runSmartSelect(ctx context.Context, withGame bool, start time.Time) {
	plan := a.smartPool("smart")
	if len(plan.names) == 0 {
		a.log.Error("autoselect", "smart: no strategies on disk")
		a.selectionRunning.Store(false)
		a.emitAutoSelectDone("smart", false, map[string]interface{}{"error": "no strategies", "reason": plan.reason})
		a.emitStrategiesChanged()
		return
	}
	a.log.Info("autoselect", fmt.Sprintf("smart: scenario=%d source=%s candidates=%d", plan.scenario, plan.source, len(plan.names)))
	a.log.Info("autoselect", "smart list: "+strings.Join(plan.names, ", "))
	a.emitAutoSelectPhase("smart", "select", a.trCur("asm_sweep", len(plan.names)))
	// План целиком — фронту сразу: модалка показывает все строки с бейджами
	// источников и приоритетами (визуализация алгоритма).
	runtime.EventsEmit(a.ctx, "autoselect:event", map[string]interface{}{
		"type": "plan", "mode": "smart", "names": plan.names,
		"items": plan.items, "scenario": plan.scenario, "reason": plan.reason,
	})

	best, pct, finalists, cancelled, stopped := a.runTwoStage(ctx, "smart", plan.names, withGame, true)
	if stopped {
		a.selectionRunning.Store(false)
		return
	}
	if cancelled {
		a.selectionRunning.Store(false)
		a.emitAutoSelectDone("smart", true, map[string]interface{}{"reason": plan.reason})
		a.emitStrategiesChanged()
		return
	}
	// Прогон завершён, записи участников (фильтр + финал) уже в базе:
	// данные оператора приводятся к эре этого прогона.
	if finalists > 0 {
		_ = database.SetFlag("full_sweep_last_at", strconv.FormatInt(time.Now().Unix(), 10))
	}
	a.wipeOperatorEra(plan.names)
	if finalists == 0 {
		a.emitAutoSelectPhase("smart", "analyze", a.trCur("asm_filter_none"))
		if cur := a.cfg.GetCurrentStrategy(); cur != "" {
			a.emitAutoSelectDone("smart", false, map[string]interface{}{"strategy": cur, "applied": false, "reason": plan.reason})
		} else {
			a.emitAutoSelectDone("smart", false, map[string]interface{}{"error": "no working strategy", "reason": plan.reason})
		}
		a.selectionRunning.Store(false)
		a.scheduleResourceCheckAfterStrategyChange()
		a.emitStrategiesChanged()
		return
	}

	done := map[string]interface{}{"mode": "smart", "cancelled": false, "reason": plan.reason}
	prev := a.cfg.GetCurrentStrategy()
	applied := false
	if pct >= smartAcceptableScoreOf() && best != "" {
		if best == prev {
			applied = true
		} else if err := a.zapret.SetStrategy(best); err != nil {
			a.log.Warn("autoselect", "smart apply best failed: "+err.Error())
		} else {
			applied = true
		}
	}
	if applied && best != prev {
		a.zapret.LogStrategyChange(best, "autoselect")
		if op, _ := database.GetCurrentOperator(); op != nil && op.OperatorKey != "" {
			database.EnsureOperatorStrategy(op.OperatorKey, best, strings.TrimSuffix(best, ".bat"))
			if err := database.MarkActiveStrategy(op.OperatorKey, best, "autoselect"); err != nil {
				a.log.Warn("autoselect", "mark active "+best+": "+err.Error())
			}
			database.SetCurrentOperator(op.OperatorKey, op.OperatorName, best)
		}
	}
	// Игровой прогон — один раз, по финальной стратегии.
	if applied && withGame {
		a.pumpAutoSelectCollect(ctx, "game", "", false, []string{best}, false, true, true)
	}
	if pct >= smartAcceptableScoreOf() {
		a.log.Info("autoselect", fmt.Sprintf("smart finished: best=%s score=%.0f applied=%v scenario=%d finalists=%d duration=%s",
			best, pct, applied, plan.scenario, finalists, time.Since(start).Round(time.Second)))
	} else {
		a.log.Warn("autoselect", fmt.Sprintf("smart: nothing reached %.0f (%.0f best) — keeping %s, duration=%s",
			smartAcceptableScoreOf(), pct, prev, time.Since(start).Round(time.Second)))
	}
	if applied {
		done["strategy"] = best
		done["applied"] = true
	} else if cur := a.cfg.GetCurrentStrategy(); cur != "" {
		done["strategy"] = cur
		done["applied"] = false
	}
	done["finalists"] = finalists
	done["probed"] = len(plan.names)
	done["scenario"] = plan.scenario
	runtime.EventsEmit(a.ctx, "autoselect:done", done)
	a.emitStrategiesChanged()
	if applied {
		a.finishSelectionRun(ctx, "smart", best)
	} else {
		a.selectionRunning.Store(false)
		a.scheduleResourceCheckAfterStrategyChange()
	}
}

// runStartupSelect — стартовый подбор. База оператора пуста (первый
// запуск / данных нет вообще) — полный перебор всех стратегий.
// Иначе проверяется ровно одна стратегия — последняя включавшаяся
// на этом операторе (нет такой — лучшая по сохранённому баллу);
// она остаётся активной независимо от балла замера.
func (a *App) runStartupSelect(ctx context.Context, withGame bool, start time.Time) {
	// Все исходы (проверена/отменена/пропущена) снимают ожидание первого
	// такта проверки ресурсов — startResourceMonitor ждёт этого канала.
	defer a.markStartupSelectDone()
	opKey, _ := a.zapret.ResolveAutoOperatorKey()
	first := a.startupFirst(opKey)
	if first == "" {
		// Пустая база оператора: полный перебор всех стратегий.
		var all []string
		for _, f := range a.zapret.ListStrategies() {
			all = append(all, f.Filename)
		}
		a.log.Info("autoselect", "startup: empty operator data \u2014 full sweep of all strategies")
		a.selectionRunning.Store(true)
		a.runStartupBatch(ctx, all, withGame, start)
		return
	}
	a.log.Info("autoselect", "startup: testing current strategy "+first)
	a.emitAutoSelectPhase("startup", "startup-last", a.trCur("asm_startup_last"))
	runtime.EventsEmit(a.ctx, "autoselect:event", map[string]interface{}{
		"type": "plan", "mode": "startup", "names": []string{first}, "scenario": 0,
	})
	// Кандидат уже стоит в движке — переключать и перезапускать службу
	// нечего: одна проверка доступности без единого касания движка.
	// Это и смысл (проверяем то, что работает), и главная экономия старта:
	// без переустановки службы замер занимает секунды, а не полминуты.
	if first == a.cfg.GetCurrentStrategy() && a.zapret.GetStatus() == zapret.StatusRunning && a.zapret.VerifyStrategyApplied(first) {
		a.log.Info("autoselect", "startup: "+first+" already running — availability check without re-apply")
		a.doResourceCheckAndSave()
		a.logAutoSelectFinish("startup", first, false, start)
		a.emitAutoSelectDone("startup", false, map[string]interface{}{"strategy": first, "applied": true})
		a.emitStrategiesChanged()
		return
	}
	a.selectionRunning.Store(true)
	_, cancelled, stopped, _ := a.pumpAutoSelectCollect(ctx, "startup", "", false, []string{first}, withGame, false, true)
	if stopped {
		a.selectionRunning.Store(false)
		return
	}
	if cancelled {
		a.selectionRunning.Store(false)
		a.emitAutoSelectDone("startup", true, nil)
		a.emitStrategiesChanged()
		return
	}
	a.logAutoSelectFinish("startup", a.cfg.GetCurrentStrategy(), false, start)
	a.emitAutoSelectDone("startup", false, map[string]interface{}{"strategy": a.cfg.GetCurrentStrategy(), "applied": true})
	a.emitStrategiesChanged()
	a.finishSelectionRun(ctx, "startup", a.cfg.GetCurrentStrategy())
}

// runStartupBatch — полный перебор списка (данных для пула нет):
// двухэтапный конвейер — фильтр по ключевым ресурсам → полный тест
// финалистов с применением лучшей.
func (a *App) runStartupBatch(ctx context.Context, top []string, withGame bool, start time.Time) {
	a.emitAutoSelectPhase("startup", "select", a.trCur("asm_startup_sweep", len(top)), len(top))
	runtime.EventsEmit(a.ctx, "autoselect:event", map[string]interface{}{
		"type": "plan", "mode": "startup", "names": top, "scenario": 0,
	})
	_, _, finalists, cancelled, stopped := a.runTwoStage(ctx, "startup", top, withGame, true)
	if stopped {
		a.selectionRunning.Store(false)
		return
	}
	if cancelled {
		a.selectionRunning.Store(false)
		a.emitAutoSelectDone("startup", true, nil)
		a.emitStrategiesChanged()
		return
	}
	if finalists == 0 {
		if cur := a.cfg.GetCurrentStrategy(); cur != "" {
			a.emitAutoSelectDone("startup", false, map[string]interface{}{"strategy": cur, "applied": false})
		} else {
			a.emitAutoSelectDone("startup", false, map[string]interface{}{"error": "no working strategy"})
		}
		a.selectionRunning.Store(false)
		a.emitStrategiesChanged()
		return
	}
	a.logAutoSelectFinish("startup", a.cfg.GetCurrentStrategy(), false, start)
	a.emitAutoSelectDone("startup", false, map[string]interface{}{"strategy": a.cfg.GetCurrentStrategy(), "applied": true})
	a.emitStrategiesChanged()
	a.finishSelectionRun(ctx, "startup", a.cfg.GetCurrentStrategy())
}

// startupFirst — первая кандидат стартового подбора: стратегия,
// последней включавшаяся на этом операторе (is_active); нет такой — лучшая
// по сохранённому баллу из базы этого оператора (без результатов — по
// доступности). Учитываются только файлы, которые есть на диске;
// данных нет вообще — "" (стартовый подбор делает полный перебор).
func (a *App) startupFirst(opKey string) string {
	stored, err := database.GetOperatorStrategies(opKey)
	if err != nil || len(stored) == 0 {
		return ""
	}
	// Строки, созданные стартовым синком файлов (нет замеров и применений),
	// «базой оператора» не считаются: иначе полный перебор на чистой системе
	// подменялся одиночной проверкой дефолтной стратегии.
	hasData := false
	for _, s := range stored {
		if s.TestedAt != nil || s.AppliedAt != nil || s.UseCount > 0 {
			hasData = true
			break
		}
	}
	if !hasData {
		return ""
	}
	files := make(map[string]bool)
	for _, s := range a.zapret.ListStrategies() {
		files[s.Filename] = true
	}

	type cand struct {
		name  string
		score float64
		avail float64
	}
	var tested, untested []cand
	lastActive := ""
	for _, s := range stored {
		if !files[s.Strategy] {
			continue
		}
		if s.IsActive && lastActive == "" {
			lastActive = s.Strategy
		}
		if sc, ok := storedBestScore(s.AutoTestResults); ok {
			tested = append(tested, cand{name: s.Strategy, score: sc, avail: s.AvailabilityPct})
		} else {
			untested = append(untested, cand{name: s.Strategy, avail: s.AvailabilityPct})
		}
	}
	sort.SliceStable(tested, func(i, j int) bool { return tested[i].score > tested[j].score })
	sort.SliceStable(untested, func(i, j int) bool { return untested[i].avail > untested[j].avail })

	if lastActive != "" {
		return lastActive
	}
	if len(tested) > 0 {
		return tested[0].name
	}
	if len(untested) > 0 {
		return untested[0].name
	}
	return ""
}

// storedBestScore — лучший сохранённый балл из результатов полного теста
// стратегии (записи со status=failed балла не имеют/не считаются).
func storedBestScore(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}
	var rows []struct {
		Score  float64 `json:"score"`
		Status string  `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return 0, false
	}
	best, ok := 0.0, false
	for _, r := range rows {
		if r.Status == "failed" || r.Status == "filtered" {
			continue
		}
		if !ok || r.Score > best {
			best, ok = r.Score, true
		}
	}
	return best, ok
}

// runGameSelect — игровой подбор: фазы operator/local/analyze/select, затем
// прогон списка только по игровым целям, применяется лучшая по игровому
// баллу. strategies — CSV имён из UI; пусто (или ничего не валидировалось) —
// топ-5 по баллам системы.
func (a *App) runGameSelect(ctx context.Context, onlyStrategy string, strategies string, start time.Time) {
	opMsg := a.trCur("asm_provider_unknown")
	if op, _ := database.GetCurrentOperator(); op != nil && op.OperatorKey != "" && op.OperatorKey != "unknown" && op.OperatorName != "" {
		opMsg = a.trCur("asm_provider", op.OperatorName)
	}
	a.emitAutoSelectPhase("game", "operator", opMsg)

	list := a.gameListFromCSV(onlyStrategy, strategies)
	if len(list) > 0 {
		a.emitAutoSelectPhase("game", "local", a.trCur("asm_game_selected", len(list)))
	} else {
		if strings.TrimSpace(strategies) != "" {
			a.log.Warn("autoselect", "game: no valid strategies in selected list — using top list")
		}
		list = a.gameTopList()
		a.emitAutoSelectPhase("game", "local", a.trCur("asm_game_top", len(list)))
	}
	a.log.Info("autoselect", "game list: "+strings.Join(list, ", "))
	a.emitAutoSelectPhase("game", "analyze", a.trCur("asm_game_run", len(list)))
	a.emitAutoSelectPhase("game", "select", a.trCur("asm_game_sweep", len(list)))
	runtime.EventsEmit(a.ctx, "autoselect:event", map[string]interface{}{
		"type": "plan", "mode": "game", "names": list,
	})

	_, cancelled, stopped := a.pumpAutoSelect(ctx, "game", "", false, list, true, true)
	if stopped {
		a.selectionRunning.Store(false)
		a.emitStrategiesChanged()
		return
	}
	a.emitAutoSelectDone("game", cancelled, nil)
	a.logAutoSelectFinish("game", a.cfg.GetCurrentStrategy(), cancelled, start)
	a.emitStrategiesChanged()
	// Лучшая применялась внутри прогона — первый такт доступности в хвосте.
	if cancelled {
		a.selectionRunning.Store(false)
	} else {
		a.finishSelectionRun(ctx, "game", a.cfg.GetCurrentStrategy())
	}
}

// gameListFromCSV — валидация списка стратегий из UI: только существующие
// general*.bat, дубликаты убраны, ПКМ-стратегия (onlyStrategy) исключена.
func (a *App) gameListFromCSV(onlyStrategy, strategies string) []string {
	if strings.TrimSpace(strategies) == "" {
		return nil
	}
	known := make(map[string]string)
	for _, s := range a.zapret.ListStrategies() {
		known[strings.ToLower(s.Filename)] = s.Filename
	}
	return zapret.ParseStrategyCSV(strategies, known, onlyStrategy)
}

// logAutoSelectFinish — итоговая строка прогона: применённая стратегия и время.
func (a *App) logAutoSelectFinish(mode, applied string, cancelled bool, start time.Time) {
	if cancelled {
		a.log.Info("autoselect", fmt.Sprintf("%s: cancelled after %s", mode, time.Since(start).Round(time.Second)))
		return
	}
	a.log.Info("autoselect", fmt.Sprintf("%s: finished, applied=%s, duration=%s", mode, applied, time.Since(start).Round(time.Second)))
}

// Умный подбор: кандидаты отбирает сервер (план по оператору), но итоговый
// выбор — по баллу клиентского прогона (score.go: доступность + ключевые +
// пинг + скорость; замер без данных веса не теряет). Порог 70 — проходной
// балл лучшей: ниже — остаётся прежняя стратегия. Агрегат сервер строит по
// личным сэмплам этого ПК, а когда их ещё мало — по общим сэмплам оператора
// (source в ответе); кандидаты идут по убыванию серверного балла.
// smartAcceptableScoreOf — проходной балл (серверный набор подбора;
// дефолт 70 — прежняя константа).
func smartAcceptableScoreOf() float64 { return zapret.AutoSel().SmartAcceptableScore }

// smartPlan — кандидаты подбора и их источник. source: "server" (топ
// strategies.top), "local" (локальные данные оператора), "mixed", "all" (все
// general*.bat). reason — для диагностики в done-событии: server_top |
// local_data | aggregate | no_strong | no_operator.
type smartPlan struct {
	names    []string
	items    []smartPlanItem
	source   string
	reason   string
	scenario int // 1 полный перебор; 2 сервер; 3 сервер+ПК; 4 ПК; 0 стартовый
}

// smartPlanItem — кандидат плана с видимой причиной попадания (для UI).
type smartPlanItem struct {
	Name    string  `json:"name"`
	Prior   float64 `json:"prior"`   // ожидаемый балл (0 — данных нет)
	Samples int64   `json:"samples"` // сколько данных за этим ожиданием
	Source  string  `json:"source"`  // all | server | device | explore
}

// normBatName — нормализация имени стратегии: без пробелов, с суффиксом .bat.
func normBatName(name string) string {
	name = strings.TrimSpace(name)
	if name != "" && !strings.HasSuffix(strings.ToLower(name), ".bat") {
		name += ".bat"
	}
	return name
}

// smartPool — пул кандидатов умного и стартового подбора по правилу:
// стратегии оператора с баллом ≥70 с сервера ∪ с этого ПК, + 2 случайные
// вне списка (исследование). Лимитов на размер пула нет. Сильных нет вообще
// (данных нет или всё ниже порога) — полный перебор всех стратегий с диска.
// Фазы конвейера эмитятся с mode.
func (a *App) smartPool(mode string) smartPlan {
	opKey, opName := a.zapret.ResolveAutoOperatorKey()
	files := a.zapret.ListStrategies()
	inFiles := make(map[string]bool, len(files))
	fileNames := make([]string, 0, len(files))
	for _, f := range files {
		fileNames = append(fileNames, f.Filename)
		inFiles[f.Filename] = true
	}
	fullSweep := func(reason string) smartPlan {
		items := make([]smartPlanItem, 0, len(fileNames))
		for _, n := range fileNames {
			items = append(items, smartPlanItem{Name: n, Source: "all"})
		}
		a.emitAutoSelectPhase(mode, "local", a.trCur("asm_local_all", len(items)))
		a.emitAutoSelectPhase(mode, "analyze", a.trCur("asm_ready", len(items), smartAcceptableScoreOf()))
		return smartPlan{names: fileNames, items: items, source: "all", reason: reason, scenario: 1}
	}

	// Неизвестный оператор: серверный агрегат по оператору бессмысленен —
	// полный перебор с быстрым фильтром.
	if opKey == "" || opKey == "unknown" {
		a.emitAutoSelectPhase(mode, "operator", a.trCur("asm_provider_unknown"))
		return fullSweep("no_operator")
	}
	a.emitAutoSelectPhase(mode, "operator", a.trCur("asm_provider", opName))

	out, err := a.strategiesTop(opKey)
	hasServer := err == nil && len(out.Items) > 0
	if hasServer {
		// Игровые баллы с сервера (агрегат по оператору) показывают бейдж
		// «игровая» сразу, без тяжёлого локального прогона.
		a.mergeServerGames(out.Games)
	}
	stored, storedErr := database.GetOperatorStrategies(opKey)
	hasDevice := storedErr == nil && len(stored) > 0

	type poolCand struct {
		name    string
		prior   float64
		samples int64
		src     string
	}
	var pool []poolCand
	inPool := make(map[string]bool)

	// Серверные ≥70 — основа пула, по убыванию серверного балла.
	if hasServer {
		a.emitAutoSelectPhase(mode, "server", a.trCur("asm_server_ok"))
		srv := make([]poolCand, 0, len(out.Items))
		for _, it := range out.Items {
			if it.Score < smartAcceptableScoreOf() {
				continue
			}
			name := normBatName(it.Name)
			if name == "" || !inFiles[name] {
				continue
			}
			srv = append(srv, poolCand{name: name, prior: it.Score, samples: it.Samples, src: "server"})
		}
		sort.SliceStable(srv, func(i, j int) bool {
			if srv[i].prior != srv[j].prior {
				return srv[i].prior > srv[j].prior
			}
			return srv[i].samples > srv[j].samples
		})
		for _, c := range srv {
			pool = append(pool, c)
			inPool[c.name] = true
		}
	} else {
		a.emitAutoSelectPhase(mode, "server", a.trCur("asm_server_local"))
	}

	// Локальные (ПК) ≥70: балл устройства (с относительной скоростью канала)
	// ценнее серверного среднего; стратегия из обоих источников получает
	// локальный балл и суммарные сэмплы.
	if hasDevice {
		localScores := a.zapret.StrategyScores()
		var loc []poolCand
		for _, es := range stored {
			name := normBatName(es.Strategy)
			if name == "" || !inFiles[name] {
				continue
			}
			score := 0.0
			if info, ok := localScores[es.Strategy]; ok && info.Score > 0 {
				score = info.Score
			} else if es.AvailabilityPct >= 0 {
				score = es.AvailabilityPct
			}
			if score < smartAcceptableScoreOf() {
				continue
			}
			loc = append(loc, poolCand{name: name, prior: score, samples: int64(es.UseCount), src: "device"})
		}
		sort.SliceStable(loc, func(i, j int) bool {
			if loc[i].prior != loc[j].prior {
				return loc[i].prior > loc[j].prior
			}
			return loc[i].name < loc[j].name
		})
		for _, c := range loc {
			if inPool[c.name] {
				for i := range pool {
					if pool[i].name == c.name {
						pool[i].prior = c.prior
						pool[i].samples += c.samples
						break
					}
				}
				continue
			}
			pool = append(pool, c)
			inPool[c.name] = true
		}
	}
	a.emitAutoSelectPhase(mode, "local", a.trCur("asm_local_data", len(pool)))

	// Сильных нет вообще — полный перебор всех стратегий.
	if len(pool) == 0 {
		return fullSweep("no_strong")
	}

	// 2 случайные вне списка — исследование.
	inPlan := make(map[string]bool, len(pool))
	for _, c := range pool {
		inPlan[c.name] = true
	}
	rest := make([]string, 0)
	for _, f := range fileNames {
		if !inPlan[f] {
			rest = append(rest, f)
		}
	}
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	extra := zapret.AutoSel().PoolExtraRandom
	for k, f := range rest {
		if k >= extra {
			break
		}
		pool = append(pool, poolCand{name: f, src: "explore"})
	}

	names := make([]string, 0, len(pool))
	items := make([]smartPlanItem, 0, len(pool))
	fromServer, fromDevice := 0, 0
	for _, c := range pool {
		names = append(names, c.name)
		items = append(items, smartPlanItem{Name: c.name, Prior: c.prior, Samples: c.samples, Source: c.src})
		switch c.src {
		case "server":
			fromServer++
		case "device":
			fromDevice++
		}
	}
	scenario, reason := 3, "aggregate"
	if fromServer == 0 {
		scenario, reason = 4, "local_data"
	} else if fromDevice == 0 {
		scenario, reason = 2, "server_top"
	}
	src := "server"
	if fromDevice > 0 {
		src = "local"
		if fromServer > 0 {
			src = "mixed"
		}
	}
	a.emitAutoSelectPhase(mode, "analyze", a.trCur("asm_ready", len(names), smartAcceptableScoreOf()))
	return smartPlan{names: names, items: items, source: src, reason: reason, scenario: scenario}
}

// runTwoStage — двухэтапный прогон списка (быстрый фильтр по ключевым
// ресурсам → полный тест финалистов). Свип отменяется общим каналом отмены
// («Пропустить»/CancelAutoTest); отмена свипа возвращает исходную стратегию.
// applyBest — применяет ли движок лучшую группу сам (умный/полный перебор)
// или решение остаётся за вызывающим (добор стартового подбора). Возвращает
// лучшего финалиста с баллом, число финалистов (0 — никто не прошёл фильтр)
// и флаги отмены/остановки приложения.
func (a *App) runTwoStage(ctx context.Context, mode string, names []string, withGame bool, applyBest bool) (best string, pct float64, finalists int, cancelled, stopped bool) {
	original := a.cfg.GetCurrentStrategy()
	engineDown := false
	if mode == "smart" {
		engineDown = a.measureRunBase(ctx, mode)
		if ctx.Err() != nil {
			return "", 0, 0, true, false
		}
	}
	a.emitAutoSelectPhase(mode, "filter", a.trCur("asm_stage_filter", len(names)), len(names))
	sweepCtx, cancelSweep := context.WithTimeout(ctx, 15*time.Minute)
	defer cancelSweep()
	a.setSweepCancel(cancelSweep)
	list := a.zapret.KeySweepNames(sweepCtx, names, func(r zapret.AutoTestResult) {
		runtime.EventsEmit(a.ctx, "autoselect:event", autoSelectEvent{AutoTestResult: r, Mode: mode, Phase: "filter"})
	})
	a.setSweepCancel(nil)
	if sweepCtx.Err() != nil {
		// Отмена в фазе свипа: свип не восстанавливает стратегию сам.
		if original != "" {
			_ = a.zapret.SetStrategy(original)
		}
		return "", 0, 0, true, false
	}
	if len(list) == 0 {
		if engineDown {
			a.restoreEngineAfterBase(original)
		}
		return "", 0, 0, false, false
	}
	a.emitAutoSelectPhase(mode, "final", a.trCur("asm_stage_final", len(list)), len(list))
	_, cancelled, stopped, results := a.pumpAutoSelectCollect(ctx, mode, "", false, list, withGame, false, applyBest)
	if stopped || cancelled {
		return "", 0, len(list), cancelled, stopped
	}
	best, pct = bestSmartResult(list, results)
	return best, pct, len(list), false, false
}

// bestSmartResult — лучшая стратегия батча по свежим результатам прогона.
func bestSmartResult(batch []string, results []zapret.AutoTestResult) (best string, pct float64) {
	inBatch := make(map[string]bool, len(batch))
	for _, n := range batch {
		inBatch[n] = true
	}
	for _, r := range results {
		if r.Type != "result" || r.Strategy == "" || !inBatch[r.Strategy] {
			continue
		}
		p := r.Score
		if p <= 0 && r.ResourcesN > 0 {
			p = 100.0 * float64(r.ResourcesOK) / float64(r.ResourcesN)
		}
		if p > pct {
			pct, best = p, r.Strategy
		}
	}
	return best, pct
}

// measureRunBase — фаза базы ручного прогона (smart/full): замер прямого
// канала без стратегий, относительно которого считается балл скорости и
// пинга. Работающий движок останавливается на время замера; возвращается
// признак «движок остался остановлен» — прогон поднимает его, если ни один
// исход не применил стратегию. Startup и ПКМ этот путь не вызывают:
// стартовый подбор не останавливает работающий движок.
func (a *App) measureRunBase(ctx context.Context, mode string) bool {
	wasRunning := a.zapret.GetStatus() == zapret.StatusRunning
	a.emitAutoSelectPhase(mode, "base", a.trCur("asm_base"))
	a.zapret.MeasureChannelBase(ctx)
	return wasRunning && a.zapret.GetStatus() != zapret.StatusRunning
}

// restoreEngineAfterBase — подъём движка, остановленного фазой базы.
func (a *App) restoreEngineAfterBase(strategy string) {
	if strategy == "" || a.zapret.GetStatus() == zapret.StatusRunning {
		return
	}
	if err := a.zapret.SetStrategy(strategy); err != nil {
		a.log.Warn("autoselect", "base engine restore: "+err.Error())
	}
}

// mergeServerGames — серверные игровые баллы заполняют пробелы локальной
// таблицы strategy_games: бейдж «игровая» виден сразу после умного подбора.
// Локальный прогон ценнее усреднённого — существующие записи не трогаем.
func (a *App) mergeServerGames(games []syncStrategyGame) {
	for _, g := range games {
		name := strings.TrimSpace(g.Name)
		if name == "" || g.Score <= 0 {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".bat") {
			name += ".bat"
		}
		if _, ok, _ := database.GetStrategyGame(name); ok {
			continue
		}
		if err := database.SaveStrategyGame(database.StrategyGame{
			Strategy: name,
			Score:    g.Score,
			PingMs:   int64(g.PingMs),
			TestedAt: time.Now(),
		}); err == nil {
			a.log.Info("autoselect", "game score from server: "+name)
		}
	}
}

// gameTopList — топ-5 стратегий по текущим сохранённым баллам системы
// (для игрового подбора). Без баллов — первые пять стратегий в системе.
func (a *App) gameTopList() []string {
	scores := a.zapret.StrategyScores()
	names := make([]string, 0, len(scores))
	for name := range scores {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if scores[names[i]].Score != scores[names[j]].Score {
			return scores[names[i]].Score > scores[names[j]].Score
		}
		return names[i] < names[j]
	})
	if len(names) > 5 {
		names = names[:5]
	}
	if len(names) == 0 {
		for i, s := range a.zapret.ListStrategies() {
			if i >= 5 {
				break
			}
			names = append(names, s.Filename)
		}
	}
	return names
}

// setSweepCancel/cancelSweep — отмена фазы быстрого свипа подбора: до входа
// в AutoSelectAndApply движковый флаг отмены не поднят, и «Пропустить»
// (/api/strategy/cancel) гасит свип через эту пару.
func (a *App) setSweepCancel(c context.CancelFunc) {
	a.sweepMu.Lock()
	a.sweepCancel = c
	a.sweepMu.Unlock()
}

func (a *App) cancelSweep() {
	a.sweepMu.Lock()
	c := a.sweepCancel
	a.sweepCancel = nil
	a.sweepMu.Unlock()
	if c != nil {
		c()
	}
}

// setSelectionCancel/cancelSelectionRun — отмена всего прогона подбора
// (prep/свип/финал/хвост): CancelAutoTest гасит runCtx горутины
// RunAutoSelectStream на любой фазе, включая паузы между фазами, где
// sweepCancel и движковый флаг оба пусты.
func (a *App) setSelectionCancel(c context.CancelFunc) {
	a.selectionMu.Lock()
	a.selectionCancel = c
	a.selectionMu.Unlock()
}

func (a *App) cancelSelectionRun() {
	a.selectionMu.Lock()
	c := a.selectionCancel
	a.selectionCancel = nil
	a.selectionMu.Unlock()
	if c != nil {
		c()
	}
}

// waitCtx — спящий await: true по истечении d, false при отмене прогона
// или закрытии приложения.
func (a *App) waitCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	case <-a.stopCh:
		return false
	}
}

// finishSelectionRun — хвост ручного прогона: победитель уже применён
// движком. Верификация применения, пауза 5с и первый такт доступности под
// новой стратегией (дальше штатный график). Стадия «check» видна в модалке
// подбора — такт не должен проходить незаметно. Отмена/закрытие приложения —
// такт не выполняется. Флаг selectionRunning снимается последним, чтобы
// автосмена/хуки/recovery молчали до полного завершения прогона.
func (a *App) finishSelectionRun(ctx context.Context, mode, winner string) {
	if winner != "" {
		if !a.zapret.VerifyStrategyApplied(winner) {
			a.log.Warn("autoselect", "winner not applied after run: "+winner)
		} else if st := a.zapret.GetStatus(); st != zapret.StatusRunning {
			a.log.Warn("autoselect", "winner not running after run: "+winner)
		}
	}
	if !a.waitCtx(ctx, 5*time.Second) {
		a.selectionRunning.Store(false)
		return
	}
	a.emitAutoSelectPhase(mode, "check", a.trCur("asm_check"))
	a.doResourceCheckAndSave()
	a.selectionRunning.Store(false)
}

// wipeOperatorEra — данные оператора приводятся к эре завершённого ручного
// прогона: стратегии вне списка участников теряют результаты прошлых
// прогонов; история применений и данные провайдера не трогаются.
func (a *App) wipeOperatorEra(keep []string) {
	opKey, _ := a.zapret.ResolveAutoOperatorKey()
	if opKey == "" {
		return
	}
	if err := database.ClearOperatorStrategyEra(opKey, keep); err != nil {
		a.log.Warn("autoselect", "era wipe: "+err.Error())
	}
}

// markStartupSelectDone — стартовый подбор завершён (в любом исходе):
// первый такт проверки ресурсов больше не ждёт.
func (a *App) markStartupSelectDone() {
	a.startupSelectOnce.Do(func() { close(a.startupSelectDone) })
}

// pumpAutoSelect — прогон AutoSelectAndApply с перекачкой событий в
// autoselect:event. Возвращает число валидных результатов (критерий
// достаточности умного подбора), флаг отмены и stopped (приложение
// закрывается — done уже эмитнут изнутри). Обычное done-событие эмитит
// вызывающий через emitAutoSelectDone: умному подбору нужно дополнить его
// reason (источник кандидатов).
func (a *App) pumpAutoSelect(runCtx context.Context, mode string, onlyStrategy string, missingOnly bool, listOnly []string, withGame bool, gameOnly bool) (valid int, cancelled bool, stopped bool) {
	v, c, s, _ := a.pumpAutoSelectCollect(runCtx, mode, onlyStrategy, missingOnly, listOnly, withGame, gameOnly, true)
	return v, c, s
}

// pumpAutoSelectCollect — pumpAutoSelect, дополнительно возвращающий
// результаты тестов (умному подбору нужен лучший балл группы).
// applyBest=false — прогон «только тест»: лучшая группа не применяется,
// движок возвращает исходную стратегию (промежуточные группы умного подбора).
func (a *App) pumpAutoSelectCollect(runCtx context.Context, mode string, onlyStrategy string, missingOnly bool, listOnly []string, withGame bool, gameOnly bool, applyBest bool) (valid int, cancelled bool, stopped bool, results []zapret.AutoTestResult) {
	ctx, cancel := context.WithTimeout(runCtx, 15*time.Minute)
	defer cancel()

	chanResults := make(chan zapret.AutoTestResult, 50)
	done := make(chan struct{})
	go a.zapret.AutoSelectAndApply(ctx, chanResults, done, onlyStrategy, missingOnly, listOnly, withGame, gameOnly, applyBest)

	for {
		select {
		case result, ok := <-chanResults:
			if !ok {
				return valid, cancelled, false, results
			}
			if result.Type == "result" && result.Error == "" && !result.Cancelled {
				valid++
			}
			if result.Type == "result" && result.Strategy != "" {
				results = append(results, result)
			}
			if result.Type == "done" && result.Cancelled {
				cancelled = true
			}
			// Внутренний done движка наружу не идёт: он не несёт применённую
			// стратегию (у смарта финального result лучшей нет вовсе), фронт
			// на нём закрывал стрим и показывал «последнюю проверенную»
			// вместо применённой. Финальный done всегда шлёт обёртка:
			// full/game — emitAutoSelectDone, smart — свой с лучшей.
			if result.Type != "done" {
				runtime.EventsEmit(a.ctx, "autoselect:event", autoSelectEvent{AutoTestResult: result, Mode: mode, Phase: result.Phase})
			}
		case <-done:
			return valid, cancelled, false, results
		case <-a.stopCh:
			cancel()
			runtime.EventsEmit(a.ctx, "autoselect:done", map[string]interface{}{"error": "Приложение закрывается", "mode": mode, "cancelled": true})
			return valid, true, true, results
		}
	}
}

// emitAutoSelectDone — единое done-событие автоподбора; extra добавляет поля
// (reason — источник кандидатов умного подбора). strategy — ФАКТИЧЕСКИ
// применённая стратегия (по конфигу после прогона): фронт показывает её в
// баннере результата, а не «последнюю проверенную» из потока результатов.
func (a *App) emitAutoSelectDone(mode string, cancelled bool, extra map[string]interface{}) {
	m := map[string]interface{}{"mode": mode, "cancelled": cancelled}
	if !cancelled {
		if cur := a.cfg.GetCurrentStrategy(); cur != "" {
			m["strategy"] = cur
		}
	}
	for k, v := range extra {
		m[k] = v
	}
	runtime.EventsEmit(a.ctx, "autoselect:done", m)
}
