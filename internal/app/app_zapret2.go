package app

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Экспертные биндинги эпохи zapret2 (флаги nfqws2, lua-функции, dry-run).
// Движок снова zapret1 (winws.exe, Flowseal) — этих возможностей у него нет.
// Маршруты сохранены, чтобы фронт не ловил «undefined binding»; эксперт-панель
// в UI скрывается (react-зона), биндинги отвечают честной ошибкой.

const errExpertNotSupported = "экспертные настройки доступны только для движка zapret2"

// GetZapret2Settings — маршрут сохранён, движок zapret1 экспертных флагов не имеет.
func (a *App) GetZapret2Settings() map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// SetZapret2Settings — маршрут сохранён, флаги не применяются.
func (a *App) SetZapret2Settings(opts map[string]interface{}) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// validStrategyFilename — имя пресета только в корне папки zapret:
// general*.bat, без путей и обхода каталогов.
func validStrategyFilename(name string) bool {
	if !strings.HasSuffix(strings.ToLower(name), ".bat") {
		return false
	}
	if !strings.HasPrefix(name, "general") {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	return true
}

// GetStrategyContent возвращает текст пресета для просмотра.
func (a *App) GetStrategyContent(filename string) map[string]interface{} {
	if !validStrategyFilename(filename) {
		return errResp("invalid strategy filename")
	}
	data, err := os.ReadFile(a.cfg.StrategyPath(filename))
	if err != nil {
		return errResp("не удалось прочитать стратегию: " + err.Error())
	}
	return okRespWithData(map[string]interface{}{
		"filename":  filename,
		"content":   string(data),
		"functions": []string{},
	})
}

// SaveStrategyContent сохраняет текст пресета. Валидатор флагов zapret2 выведен
// вместе с движком — сохранение без валидации (испорченный пресет движок
// не запустит, восстановление стратегий доступно из каталога дистрибутива).
func (a *App) SaveStrategyContent(filename, content string) map[string]interface{} {
	if !validStrategyFilename(filename) {
		return errResp("invalid strategy filename")
	}
	if len(content) > 64*1024 {
		return errResp("стратегия слишком большая (лимит 64 КБ)")
	}
	if err := os.WriteFile(a.cfg.StrategyPath(filename), []byte(content), 0644); err != nil {
		return errResp("не удалось сохранить стратегию: " + err.Error())
	}
	a.log.Info("zapret", "Стратегия сохранена: "+filename)
	return okRespWithData(map[string]interface{}{"saved": true, "filename": filename, "functions": []string{}})
}

// CopyStrategy создаёт редактируемую копию пресета general (<имя>).bat;
// имя уникализируется суффиксом, если файл уже есть.
func (a *App) CopyStrategy(filename, name string) map[string]interface{} {
	if !validStrategyFilename(filename) {
		return errResp("invalid strategy filename")
	}
	name = sanitizeStrategyName(name)
	if name == "" {
		name = "Моя"
	}
	data, err := os.ReadFile(a.cfg.StrategyPath(filename))
	if err != nil {
		return errResp("не удалось прочитать исходную стратегию: " + err.Error())
	}
	target := fmt.Sprintf("general (%s).bat", name)
	for i := 2; ; i++ {
		if _, err := os.Stat(a.cfg.StrategyPath(target)); os.IsNotExist(err) {
			break
		}
		target = fmt.Sprintf("general (%s %d).bat", name, i)
	}
	if err := os.WriteFile(a.cfg.StrategyPath(target), data, 0644); err != nil {
		return errResp("не удалось создать копию: " + err.Error())
	}
	a.log.Info("zapret", "Создана копия стратегии: "+target)
	return okRespWithData(map[string]interface{}{"filename": target})
}

// ValidateStrategy — валидатор флагов zapret2 выведен вместе с движком.
func (a *App) ValidateStrategy(content string) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// GetStrategyLuaToggles — lua-функции есть только у zapret2.
func (a *App) GetStrategyLuaToggles(filename string) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// SetStrategyLuaToggle — lua-функции есть только у zapret2.
func (a *App) SetStrategyLuaToggle(filename, fn string, enabled bool) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// ValidateStrategyLive — --dry-run есть только у zapret2.
func (a *App) ValidateStrategyLive(filename string) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

var strategyNameSanitizer = regexp.MustCompile(`[\\/:*?"<>|]+`)

func sanitizeStrategyName(name string) string {
	name = strategyNameSanitizer.ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

// ============================================================
// Lua-скрипты — механизм zapret2, у zapret1 скриптов нет
// ============================================================

// GetLuaScripts — маршрут сохранён, движок zapret1 lua не поддерживает.
func (a *App) GetLuaScripts() map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// GetLuaScript — маршрут сохранён, движок zapret1 lua не поддерживает.
func (a *App) GetLuaScript(scope, name string) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// SaveLuaScript — маршрут сохранён, движок zapret1 lua не поддерживает.
func (a *App) SaveLuaScript(scope, name, content string) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// CreateLuaScript — маршрут сохранён, движок zapret1 lua не поддерживает.
func (a *App) CreateLuaScript(name string) map[string]interface{} {
	return errResp(errExpertNotSupported)
}

// DeleteLuaScript — маршрут сохранён, движок zapret1 lua не поддерживает.
func (a *App) DeleteLuaScript(name string) map[string]interface{} {
	return errResp(errExpertNotSupported)
}
