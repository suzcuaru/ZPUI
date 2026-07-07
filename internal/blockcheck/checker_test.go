package blockcheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTargets(t *testing.T) {
	content := `# list-general.txt sample
discord.com
youtube.com
google.com

# comment
PING:8.8.8.8
`
	targets := ParseTargets(content)

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d: %+v", len(targets), targets)
	}

	expect := map[string]string{
		"discord.com":  "https://discord.com",
		"youtube.com":  "https://youtube.com",
		"google.com":   "https://google.com",
	}
	for _, tg := range targets {
		if expect[tg.Name] != tg.URL {
			t.Errorf("target %s: expected URL %s, got %s", tg.Name, expect[tg.Name], tg.URL)
		}
	}
}

func TestReadTargets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list-general.txt")
	content := `discord.com
youtube.com
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	targets, err := ReadTargets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Name != "discord.com" {
		t.Errorf("expected discord.com, got %s", targets[0].Name)
	}
	if targets[0].URL != "https://discord.com" {
		t.Errorf("expected https://discord.com, got %s", targets[0].URL)
	}
}

func TestCheckLive(t *testing.T) {
	checker := NewChecker(10, "")
	result := checker.Check("https://www.google.com")

	t.Logf("Google: verdict=%s confidence=%s", result.Verdict, result.Confidence)
	t.Logf("  HTTP: ok=%v status=%d stub=%v err=%s", result.HTTP.Ok, result.HTTP.Status, result.HTTP.StubPage, result.HTTP.Error)
	t.Logf("  Notes: %v", result.Notes)

	if result.Verdict != VerdictOK {
		t.Skipf("no network or blocked: google.com verdict=%s", result.Verdict)
	}
}

func TestBulkCheck(t *testing.T) {
	targets := []BulkTarget{
		{Name: "GOOGLE", URL: "https://www.google.com"},
		{Name: "CLOUDFLARE", URL: "https://www.cloudflare.com"},
		{Name: "GITHUB", URL: "https://github.com"},
	}

	checker := NewChecker(10, "")
	report := checker.BulkCheck(targets, nil)

	if len(report.Default) != 3 {
		t.Fatalf("expected 3 results, got %d", len(report.Default))
	}

	okCount := 0
	for _, r := range report.Default {
		t.Logf("%-12s ok=%-5v blocked=%-5v verdict=%-12s %dms",
			r.Name, r.OK, r.Blocked, r.Verdict, r.LatencyMs)
		if r.OK {
			okCount++
		}
	}

	if okCount == 0 {
		t.Skip("no network: all targets failed")
	}
}

func TestCheckBlockedResource(t *testing.T) {
	checker := NewChecker(5, "")

	result := checker.Check("https://192.0.2.1")
	t.Logf("Fake IP: verdict=%s http_err=%s", result.Verdict, result.HTTP.Error)

	if result.Verdict == VerdictOK {
		t.Error("192.0.2.1 (TEST-NET) should not be OK")
	}
}

func TestClassifyLogic(t *testing.T) {
	cases := []struct {
		name     string
		httpOK   bool
		stubPage bool
		httpErr  string
		status   int
		want     string
	}{
		{"all_ok", true, false, "", 200, VerdictOK},
		{"redirect_ok", true, false, "", 301, VerdictOK},
		{"http_stub", false, true, "", 451, VerdictHTTPStub},
		{"timeout", false, false, "timeout", 0, VerdictTimeout},
		{"connection_reset", false, false, "connection_reset", 0, VerdictTCPReset},
		{"server_error", false, false, "", 503, VerdictDown},
		{"not_found", false, false, "", 404, VerdictDown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &CheckResult{
				HTTP: LayerResult{Ok: tc.httpOK, StubPage: tc.stubPage, Error: tc.httpErr, Status: tc.status},
			}
			c := &Checker{}
			c.classify(r)
			if r.Verdict != tc.want {
				t.Errorf("classify(%s): expected %s, got %s (notes: %v)", tc.name, tc.want, r.Verdict, r.Notes)
			}
		})
	}
}
