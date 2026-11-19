package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/database"
	"zpui/internal/netcheck"
	"zpui/internal/notify"
	"zpui/internal/zapret"
)

// Автосмена по доступности: такт проверки ниже порога НЕ переключает сразу —
// запускается серия подтверждения: 3 фоновые проверки с шагом 30 секунд,
// НЕ попадающие в график и историю. Все три ниже порога — смена на лучшую
// по баллу и одна штатная проверка под новой стратегией (эта уже в графике).
// «Лёгкий старт»: серия возможна только после полной готовности интерфейса
// и минуты тишины. Cooldown между сменами — против пинг-понга.
const (
	autoSwitchConfirmChecks = 3
	autoSwitchConfirmGap    = 30 * time.Second
	autoSwitchCooldown      = 10 * time.Minute
	autoSwitchArmDelay      = time.Minute
)

// autoSwitchTick — хук после такта проверки ресурсов (doResourceCheckAndSave):
// ok/total — доступные ресурсы такта. Ниже порога — запуск серии подтверждения.
func (a *App) autoSwitchTick(ok, total int) {
	if total <= 0 {
		return
	}
	// Тумблер автосмены выключен — никакой автосмены, включая серию.
	if !a.cfg.GetAutoSwitchEnabled() {
		return
	}
	avail := 100 * float64(ok) / float64(total)

	a.autoSwitchMu.Lock()
	// Лёгкий старт / серия уже идёт — мимо.
	if !a.autoSwitchArmed || a.autoSwitchConfirming {
		a.autoSwitchMu.Unlock()
		return
	}
	// Идёт подбор (prep/свип/финал/хвост) или проверка стратегий — движок
	// сам перебирает варианты; движок не работает — сменить нечего и не на чем.
	if a.selectionRunning.Load() || a.zapret.IsAutoTestRunning() || a.zapret.GetStatus() != zapret.StatusRunning {
		a.autoSwitchMu.Unlock()
		return
	}
	threshold := float64(a.cfg.GetAutoSwitchThresholdPct())
	if avail >= threshold {
		a.autoSwitchMu.Unlock()
		return
	}
	// Низкий такт при отсутствующем интернете — не вина стратегии
	// (пробуждение ноутбука, отвал сети): смену не провоцируем.
	if !netcheck.Online() {
		a.autoSwitchMu.Unlock()
		a.log.Info("autoswitch", fmt.Sprintf("availability %.0f%%, но сеть недоступна — такт не учитывается", avail))
		return
	}
	if time.Since(a.autoSwitchLastAt) < autoSwitchCooldown {
		a.autoSwitchMu.Unlock()
		a.log.Info("autoswitch", fmt.Sprintf("availability %.0f%% < %d%% — cooldown until %s",
			avail, int(threshold), a.autoSwitchLastAt.Add(autoSwitchCooldown).Format("15:04:05")))
		return
	}
	best, score := a.autoSwitchBest()
	if best == "" {
		a.autoSwitchMu.Unlock()
		a.log.Info("autoswitch", fmt.Sprintf("availability %.0f%% below threshold — no better strategy to switch to", avail))
		return
	}
	from := a.cfg.GetCurrentStrategy()
	a.autoSwitchConfirming = true
	a.autoSwitchMu.Unlock()

	go a.autoSwitchConfirmSeries(from, best, score, avail, threshold)
}

// autoSwitchConfirmSeries — защита от случайного низкого такта: N фоновых
// проверок с шагом 30 секунд, без записи в график/историю. Любой хороший
// результат отменяет смену; все плохие — переключение.
func (a *App) autoSwitchConfirmSeries(from, best string, score, avail, threshold float64) {
	defer func() {
		a.autoSwitchMu.Lock()
		a.autoSwitchConfirming = false
		a.autoSwitchMu.Unlock()
	}()
	for i := 1; i <= autoSwitchConfirmChecks; i++ {
		select {
		case <-a.stopCh:
			return
		case <-time.After(autoSwitchConfirmGap):
		}
		// Пользователь запустил подбор/проверку — вмешиваться нельзя.
		if a.selectionRunning.Load() || a.zapret.IsAutoTestRunning() || a.zapret.GetStatus() != zapret.StatusRunning {
			a.log.Info("autoswitch", "confirm: идёт подбор/движок не работает — серия остановлена")
			return
		}
		// Сеть пропала — низкий результат был бы не по вине стратегии.
		if !netcheck.Online() {
			a.log.Info("autoswitch", "confirm: сеть недоступна — серия остановлена")
			return
		}
		pct := a.autoSwitchProbe()
		a.log.Info("autoswitch", fmt.Sprintf("confirm %d/%d: %.0f%% (порог %.0f%%)", i, autoSwitchConfirmChecks, pct, threshold))
		if pct >= threshold {
			a.log.Info("autoswitch", fmt.Sprintf("подтверждение не сошлось (%.0f%%) — остаётся %s", pct, from))
			return
		}
	}
	a.autoSwitchApply(from, best, score, avail)
}

// autoSwitchProbe — доступность всех целей одним замером БЕЗ записи
// в БД/график/историю (подтверждающие проверки график не трогают).
func (a *App) autoSwitchProbe() float64 {
	report := a.getResourceStatusForced()
	if report == nil {
		return 100
	}
	all := append(report.Default, report.User...)
	if len(all) == 0 {
		return 100
	}
	okCnt := 0
	for _, r := range all {
		if r.OK {
			okCnt++
		}
	}
	return 100 * float64(okCnt) / float64(len(all))
}

// autoSwitchApply — сама смена: лучшая стратегия, журнал/история/тост и одна
// штатная проверка под новой стратегией (эта — уже в графике).
func (a *App) autoSwitchApply(from, best string, score, avail float64) {
	// Страховка: серия стартовала до подбора, а применяется уже во время него.
	if a.selectionRunning.Load() {
		a.log.Warn("autoswitch", "apply suppressed: strategy selection is running")
		return
	}
	if err := a.zapret.SetStrategy(best); err != nil {
		a.log.Warn("autoswitch", "switch failed: "+err.Error())
		return
	}
	a.autoSwitchMu.Lock()
	a.autoSwitchLastAt = time.Now()
	a.autoSwitchMu.Unlock()
	threshold := float64(a.cfg.GetAutoSwitchThresholdPct())
	a.log.Info("autoswitch", fmt.Sprintf("availability %.0f%% < %.0f%% (3 подтверждения) — switched %s → %s (score %.0f)",
		avail, threshold, from, best, score))

	a.zapret.LogStrategyChange(best, "autoswitch")
	if op, _ := database.GetCurrentOperator(); op != nil && op.OperatorKey != "" {
		database.EnsureOperatorStrategy(op.OperatorKey, best, strings.TrimSuffix(best, ".bat"))
		if err := database.MarkActiveStrategy(op.OperatorKey, best, "autoswitch"); err != nil {
			a.log.Warn("autoswitch", "mark active "+best+": "+err.Error())
		}
		database.SetCurrentOperator(op.OperatorKey, op.OperatorName, best)
	}
	a.appendAutoSwitchHistory(from, best, int(avail))

	// Проверка доступности под новой стратегией + синхронизация интерфейса.
	a.scheduleResourceCheckAfterStrategyChange()
	runtime.EventsEmit(a.ctx, "strategy:changed", map[string]interface{}{
		"strategy": best,
		"source":   "autoswitch",
	})
	notify.Show("ZPUI · "+a.trCur("auto_switch_title"), a.trCur("auto_switch_applied", int(avail), strings.TrimSuffix(best, ".bat")))
}

// startAutoSwitchArm — «лёгкий старт» автосмены: активируется после полной
// готовности интерфейса (закрыт гейт стартового экрана) и минуты тишины.
func (a *App) startAutoSwitchArm() {
	if a.startupReadyCh != nil {
		select {
		case <-a.startupReadyCh:
		case <-a.stopCh:
			return
		}
	}
	select {
	case <-time.After(autoSwitchArmDelay):
	case <-a.stopCh:
		return
	}
	a.autoSwitchMu.Lock()
	a.autoSwitchArmed = true
	a.autoSwitchMu.Unlock()
	a.log.Info("autoswitch", "armed: ui ready + 1 min")
}

// autoSwitchBest — лучшая по сохранённому баллу стратегия на диске, не
// равная текущей. Нет данных или кандидатов — "".
func (a *App) autoSwitchBest() (string, float64) {
	current := a.cfg.GetCurrentStrategy()
	scores := a.zapret.StrategyScores()
	files := make(map[string]bool)
	for _, s := range a.zapret.ListStrategies() {
		files[s.Filename] = true
	}
	best := ""
	var bestScore float64
	for name, info := range scores {
		if !files[name] || name == current || info.Score <= bestScore {
			continue
		}
		best, bestScore = name, info.Score
	}
	return best, bestScore
}

// appendAutoSwitchHistory — запись в logs/auto_switch_history.json.
// Ошибка записи не критична.
func (a *App) appendAutoSwitchHistory(from, to string, avail int) {
	path := filepath.Join(a.cfg.LogsDir(), "auto_switch_history.json")
	var records []map[string]interface{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &records)
	}
	records = append(records, map[string]interface{}{
		"time":         time.Now().Format(time.RFC3339),
		"from":         from,
		"to":           to,
		"availability": avail,
	})
	if len(records) > 200 {
		records = records[len(records)-200:]
	}
	if data, err := json.MarshalIndent(records, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
