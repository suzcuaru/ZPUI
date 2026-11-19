package app

import (


	"zpui/internal/database"
	"zpui/internal/zapret"
)


// ============================================================
// GAME FILTER
// ============================================================

func (a *App) GetGameFilter() map[string]interface{} {
	mode, gfTCP, gfUDP := a.zapret.LoadGameFilter()
	res := map[string]interface{}{"mode": mode, "gf_tcp": gfTCP, "gf_udp": gfUDP}
	cur := a.cfg.GetCurrentStrategy()
	if g, ok, _ := database.GetStrategyGame(cur); ok {
		res["game_badge"] = g.Score >= zapret.GameBadgeThreshold()
		res["game_score"] = g.Score
		if g.GfTCP != "" || g.GfUDP != "" {
			res["strategy_gf_tcp"] = g.GfTCP
			res["strategy_gf_udp"] = g.GfUDP
		}
	}
	return res
}

// GetGameFilterPorts — пользовательские диапазоны портов игрового фильтра
// (пустая строка = дефолт 1024-65535).
func (a *App) GetGameFilterPorts() map[string]interface{} {
	var tcp, udp string
	a.cfg.RView(func() {
		tcp = a.cfg.GameFilter.TCPPorts
		udp = a.cfg.GameFilter.UDPPorts
	})
	return map[string]interface{}{"tcp_ports": tcp, "udp_ports": udp}
}

// applyGameFilterLive — живое применение смены игрового фильтра запущенному
// движку (тот же контракт, что ToggleIpset для ipset-режима): в автотесте и
// на выключенном движке не трогаем — применится при следующем старте.
func (a *App) applyGameFilterLive() {
	if st := a.zapret.GetStatus(); st == zapret.StatusRunning && !a.zapret.IsAutoTestRunning() {
		if err := a.zapret.ApplyGameFilter(); err != nil {
			a.log.Warn("strategy", "game filter apply: "+err.Error())
		}
	}
}

// SetGameFilterPorts — сохраняет диапазоны портов игрового фильтра и
// переустанавливает текущий режим движка, чтобы порты применились сразу
// (запущенный движок перезапускается с новыми макросами %GameFilter%).
func (a *App) SetGameFilterPorts(tcp, udp string) map[string]interface{} {
	if err := a.cfg.SetGameFilterPorts(tcp, udp); err != nil {
		return errResp("invalid port spec")
	}
	mode, _, _ := a.zapret.LoadGameFilter()
	if mode != "disabled" {
		if err := a.zapret.SetGameFilter(mode); err != nil {
			return errResp(err.Error())
		}
		a.applyGameFilterLive()
	}
	a.emitStrategiesChanged()
	return map[string]interface{}{"status": "ok"}
}

func (a *App) SetGameFilter(mode string, force bool) map[string]interface{} {
	// Игровой режим разрешён на любой стратегии: на стратегии без игрового
	// значка (балл ≥ порога) фронт сначала показывает предупреждение и
	// повторяет вызов с force=true.
	if mode != "" && mode != "off" {
		cur := a.cfg.GetCurrentStrategy()
		badged := false
		if g, ok, _ := database.GetStrategyGame(cur); ok {
			badged = g.Score >= zapret.GameBadgeThreshold()
		}
		if !badged {
			games, _ := database.GetStrategyGames()
			list := make([]map[string]interface{}, 0, len(games))
			for _, g := range games {
				if g.Score < zapret.GameBadgeThreshold() {
					continue
				}
				list = append(list, map[string]interface{}{
					"strategy": g.Strategy,
					"score":    g.Score,
				})
			}
			if !force {
				a.log.Info("strategy", "game filter needs confirmation: "+cur+" has no game badge")
				return map[string]interface{}{
					"warning":         "game_no_badge",
					"current":         cur,
					"game_strategies": list,
				}
			}
			a.log.Warn("strategy", "game filter forced on strategy without game badge: "+cur)
		}
	}
	if err := a.zapret.SetGameFilter(mode); err != nil {
		a.log.Error("strategy", "Game filter set failed: "+err.Error())
		return map[string]interface{}{"error": err.Error()}
	}
	a.log.Info("strategy", "Game filter set to: "+mode)
	a.applyGameFilterLive()
	a.emitStrategiesChanged()
	return map[string]interface{}{"status": "ok"}
}
