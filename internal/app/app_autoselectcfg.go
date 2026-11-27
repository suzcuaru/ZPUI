package app

// Алгоритм подбора: серверный версионированный набор (CRM → вкладка «Подбор»).
// Блок autoselect {version, unchanged, data} приходит в sync.pull/state.sync
// (как скип-лист и DNS); хранение локально — флаги БД:
//   autoselect_cfg_version — версия набора (epoch updated_at на сервере);
//   autoselect_cfg_json    — применённый набор (JSON).
// Применение — zapret.ApplyAutoSelectCfg (пороги/веса/тайминги подбора).
//
// Обновление «при запуске программы, на стартовом меню до начала подбора
// или проверки стратегии»: RefreshAutoSelectCfg зовётся из RunStartupChecks
// (до шага auto-select), перед каждым подбором (RunAutoSelectStream); перед
// одиночным тестом стратегии — параллельно проверке (старт не ждёт сеть).
// Фоновый часовой sync.pull применяет блок через ApplyPull.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"zpui/internal/database"
	"zpui/internal/zapret"
)

// autoselectLocalVersion — версия набора, применённая локально (0 = дефолт).
func autoselectLocalVersion() int64 {
	return parseFlagInt64("autoselect_cfg_version")
}

// autoselectLoadApplied — применить сохранённый набор при старте (до любых
// подборов): рестарт не должен сбрасывать серверные настройки на дефолт.
func autoselectLoadApplied() {
	raw, err := database.GetFlag("autoselect_cfg_json")
	if err != nil || raw == "" {
		return
	}
	var data map[string]any
	if json.Unmarshal([]byte(raw), &data) != nil {
		return
	}
	zapret.ApplyAutoSelectCfg(data)
}

// applyAutoselectBlock — применить блок из sync.pull/state.sync по правилам
// курсора: unchanged/совпавшая версия — ничего не делаем.
func (a *App) applyAutoselectBlock(version int64, unchanged bool, raw json.RawMessage) {
	if version <= 0 || unchanged || version == autoselectLocalVersion() {
		return
	}
	var data map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			a.log.Warn("autoselect", "bad config block: "+err.Error())
			return
		}
	}
	zapret.ApplyAutoSelectCfg(data)
	blob, _ := json.Marshal(data)
	_ = database.SetFlag("autoselect_cfg_json", string(blob))
	_ = database.SetFlag("autoselect_cfg_version", strconv.FormatInt(version, 10))
	_ = database.SetFlag("autoselect_cfg_synced_at", time.Now().Format(time.RFC3339))
	custom := len(data) > 0
	if custom {
		a.log.Info("autoselect", fmt.Sprintf("selection config applied from server (v%d): %d keys", version, len(data)))
	} else {
		a.log.Info("autoselect", fmt.Sprintf("selection config reset to defaults (v%d)", version))
	}
}

// RefreshAutoSelectCfg — лёгкий sync.pull с курсором только по autoselect:
// сервер отдаёт блок, если версия изменилась. Быстро (дельта), анонимно
// (набор публичный) — перед подбором синхронно, перед одиночным тестом
// параллельно (не задерживает старт проверки).
func (a *App) RefreshAutoSelectCfg() {
	if ServerURL == "" {
		return
	}
	data, e := a.syncCallMap("sync.pull", "", map[string]any{
		"have": map[string]int64{"autoselect": autoselectLocalVersion()},
	})
	if e != nil {
		return // тихо: подбор пойдёт на текущем наборе
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	var pd struct {
		Autoselect *struct {
			Version   int64           `json:"version"`
			Unchanged bool            `json:"unchanged"`
			Data      json.RawMessage `json:"data"`
		} `json:"autoselect"`
	}
	if json.Unmarshal(raw, &pd) != nil || pd.Autoselect == nil {
		return
	}
	a.applyAutoselectBlock(pd.Autoselect.Version, pd.Autoselect.Unchanged, pd.Autoselect.Data)
}

// autoselectInfoMap — версия набора для UI (бейдж в модалке подбора,
// пороги грейдов S/A/B/C/D для шкалы баллов).
func (a *App) autoselectInfoMap() map[string]interface{} {
	c := zapret.AutoSel()
	return map[string]interface{}{
		"version":    autoselectLocalVersion(),
		"custom":     zapret.AutoSelIsCustom(),
		"grade_s":    c.GradeS,
		"grade_a":    c.GradeA,
		"grade_b":    c.GradeB,
		"grade_c":    c.GradeC,
		"key_pct":    c.KeyFilterPct,
		"acceptable": c.SmartAcceptableScore,
	}
}

// AutoselectCfgInfo — GET /api/autoselect/cfg: версия/пороги набора (бейдж
// «v{версия}» в углу модалки подбора и пороги шкалы баллов).
func (a *App) AutoselectCfgInfo() map[string]interface{} {
	return okRespWithData(a.autoselectInfoMap())
}
