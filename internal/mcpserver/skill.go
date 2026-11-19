package mcpserver

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed skilldata/SKILL.md
var skillMD string

// SkillEmbedded — содержимое скилла zpui-mcp, зашитое в exe.
func SkillEmbedded() string {
	return skillMD
}

// MaterializeSkill поддерживает файл <exeDir>\skills\zpui-mcp\SKILL.md:
// создаёт или перезаписывает его, если на диске нет файла или текст устарел
// (обновление exe приносит новую embedded-копию). Путь возвращает даже при
// ошибке записи — он вычислен детерминированно.
func MaterializeSkill(exeDir string) (string, error) {
	path := filepath.Join(exeDir, "skills", "zpui-mcp", "SKILL.md")
	if data, err := os.ReadFile(path); err == nil && string(data) == skillMD {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, err
	}
	if err := os.WriteFile(path, []byte(skillMD), 0o644); err != nil {
		return path, err
	}
	return path, nil
}
