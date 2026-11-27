package syncclient

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapSmallPayloadUntouched(t *testing.T) {
	p := map[string]any{"rows": []map[string]any{{"a": 1}}, "v": 1}
	out := capSectionPayload("data.status", p, nil)
	raw, _ := json.Marshal(out)
	if len(raw) > maxSectionPayload {
		t.Fatalf("small payload must pass through")
	}
	if len(out) != 2 {
		t.Fatalf("fields lost: %v", out)
	}
}

func TestCapTruncatesListKeepingTail(t *testing.T) {
	rows := make([]map[string]any, 5000)
	for i := range rows {
		rows[i] = map[string]any{"i": i, "msg": strings.Repeat("x", 300)}
	}
	p := map[string]any{"rows": rows, "cursor": "2026-11-01T00:00:00Z"}
	log := &warnLog{}
	out := capSectionPayload("data.errors", p, log)
	raw, _ := json.Marshal(out)
	if len(raw) > maxSectionPayload {
		t.Fatalf("capped = %d B, want <= %d", len(raw), maxSectionPayload)
	}
	got := out["rows"].([]map[string]any)
	if len(got) == 0 || len(got) >= len(rows) {
		t.Fatalf("rows = %d, want truncated but non-empty", len(got))
	}
	// Хвост сохранён: последний элемент — исходный последний (курсор диффа).
	if got[len(got)-1]["i"] != 4999 {
		t.Fatalf("tail row lost: last i = %v", got[len(got)-1]["i"])
	}
	if log.count() != 1 {
		t.Fatalf("warn = %d, want 1", log.count())
	}
}

func TestCapTruncatesStringKeepingTail(t *testing.T) {
	long := strings.Repeat("domain.example\n", 40000) // ~600 КБ
	p := map[string]any{"skip_user": long, "skip_sys_version": 7}
	out := capSectionPayload("data.exclusions", p, nil)
	raw, _ := json.Marshal(out)
	if len(raw) > maxSectionPayload {
		t.Fatalf("capped = %d B, want <= %d", len(raw), maxSectionPayload)
	}
	got := out["skip_user"].(string)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated string must stay valid UTF-8")
	}
	if !strings.HasSuffix(long, got) || len(got) >= len(long) {
		t.Fatalf("string tail not preserved (len %d of %d)", len(got), len(long))
	}
}

func TestCapNestedMapField(t *testing.T) {
	inner := make([]map[string]any, 3000)
	for i := range inner {
		inner[i] = map[string]any{"i": i, "msg": strings.Repeat("y", 400)}
	}
	p := map[string]any{"settings": map[string]any{"usage": inner, "theme": "dark"}}
	out := capSectionPayload("data.system", p, nil)
	raw, _ := json.Marshal(out)
	if len(raw) > maxSectionPayload {
		t.Fatalf("capped = %d B, want <= %d", len(raw), maxSectionPayload)
	}
	if out["settings"].(map[string]any)["theme"] != "dark" {
		t.Fatalf("small sibling fields must survive")
	}
}

func TestTailStringRuneBoundary(t *testing.T) {
	s := strings.Repeat("привет", 1000) // многобайтовые руны
	got := tailString(s)
	if !utf8.ValidString(got) {
		t.Fatalf("cut mid-rune: invalid UTF-8")
	}
	if !strings.HasSuffix(s, got) {
		t.Fatalf("tail is not a suffix")
	}
}
