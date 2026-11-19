package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSkillFileInSync — embedded-копия не должна расходиться с каноническим
// скиллом проекта: правка скилла без пересинка копии ломает поставку.
func TestSkillFileInSync(t *testing.T) {
	want, err := os.ReadFile(filepath.FromSlash("../../.agents/skills/zpui-mcp/SKILL.md"))
	if err != nil {
		t.Fatalf("canonical skill file: %v", err)
	}
	if string(want) != SkillEmbedded() {
		t.Fatalf("skilldata/SKILL.md расходится с каноническим скиллом — скопируй канонический файл из .agents/skills/zpui-mcp/SKILL.md в internal/mcpserver/skilldata/SKILL.md")
	}
}

func TestMaterializeSkill(t *testing.T) {
	dir := t.TempDir()
	wantPath := filepath.Join(dir, "skills", "zpui-mcp", "SKILL.md")

	path, err := MaterializeSkill(dir)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != SkillEmbedded() {
		t.Fatal("written content differs from embedded")
	}

	if err := os.WriteFile(path, []byte("stale content"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	if _, err := MaterializeSkill(dir); err != nil {
		t.Fatalf("re-materialize: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after heal: %v", err)
	}
	if string(data) != SkillEmbedded() {
		t.Fatal("stale content was not replaced with embedded")
	}

	if _, err := MaterializeSkill(dir); err != nil {
		t.Fatalf("idempotent materialize: %v", err)
	}
}
