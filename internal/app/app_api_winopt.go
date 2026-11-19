package app

// Игровая оптимизация Windows: твики реестра против отложенных ACK/Nagle
// и сетевого троттлинга. Состояние живёт в реестре (как автозапуск),
// config.json не касается.

import (
	"zpui/internal/winopt"
)

func (a *App) gameOptimizeResp(st winopt.Status) map[string]interface{} {
	return okRespWithData(map[string]interface{}{
		"applied":        st.Applied,
		"interfaces":     st.Interfaces,
		"with_tweaks":    st.WithTweaks,
		"throttling_off": st.Throttling,
	})
}

// GetGameOptimize — фактическое состояние игровых твиков.
func (a *App) GetGameOptimize() map[string]interface{} {
	st, err := winopt.Read()
	if err != nil {
		return errResp(err.Error())
	}
	return a.gameOptimizeResp(st)
}

// SetGameOptimize включает (true) или снимает (false) игровые твики.
func (a *App) SetGameOptimize(enabled bool) map[string]interface{} {
	st, err := winopt.Apply()
	state := "enabled"
	if !enabled {
		st, err = winopt.Remove()
		state = "disabled"
	}
	if err != nil {
		a.log.Warn("winopt", err.Error())
		return errResp(err.Error())
	}
	a.log.Info("winopt", state)
	return a.gameOptimizeResp(st)
}
