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
		"discord.com": "https://discord.com",
		"youtube.com": "https://youtube.com",
		"google.com":  "https://google.com",
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

func TestCheckLayers(t *testing.T) {
	checker := NewChecker(10, "")
	result := checker.Check("https://www.google.com")

	t.Logf("Google: verdict=%s confidence=%s", result.Verdict, result.Confidence)
	t.Logf("  DNS: ok=%v ips=%v %.1fms", result.DNS.Ok, result.DNS.IPs, result.DNS.TimeMs)
	t.Logf("  DoH: ok=%v ips=%v", result.DNSDoH.Ok, result.DNSDoH.IPs)
	t.Logf("  TCP: ok=%v err=%s %.1fms", result.TCP.Ok, result.TCP.Error, result.TCP.TimeMs)
	t.Logf("  TLS: ok=%v cn=%s err=%s", result.TLS.Ok, result.TLS.CertCN, result.TLS.Error)
	t.Logf("  HTTP: ok=%v status=%d stub=%v", result.HTTP.Ok, result.HTTP.Status, result.HTTP.StubPage)
	t.Logf("  Notes: %v", result.Notes)

	if !result.DNS.Ok && !result.DNSDoH.Ok {
		t.Skip("no network: DNS resolution failed for google.com")
	}

	if result.Verdict != VerdictOK {
		t.Errorf("google.com should be OK, got %s: %v", result.Verdict, result.Notes)
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
	checker := NewChecker(8, "")

	result := checker.Check("https://192.0.2.1")
	t.Logf("Fake IP: verdict=%s dns_ok=%v tcp_ok=%v", result.Verdict, result.DNS.Ok, result.TCP.Ok)

	if result.Verdict == VerdictOK {
		t.Error("192.0.2.1 (TEST-NET) should not be OK")
	}
}

func TestClassifyLogic(t *testing.T) {
	cases := []struct {
		name     string
		dnsOK    bool
		dohOK    bool
		tcpOK    bool
		tlsOK    bool
		httpOK   bool
		stubPage bool
		tcpErr   string
		mismatch bool
		want     string
	}{
		{"all_ok", true, true, true, true, true, false, "", false, VerdictOK},
		{"dns_block", false, true, false, false, false, false, "", false, VerdictDNSBlock},
		{"dns_mismatch", true, true, true, true, true, false, "", true, VerdictDNSBlock},
		{"tcp_reset", true, true, false, false, false, false, "RST", false, VerdictTCPReset},
		{"tls_block", true, true, true, false, false, false, "", false, VerdictTLSBlock},
		{"http_stub", true, true, true, true, false, true, "", false, VerdictHTTPStub},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &CheckResult{
				DNS:        LayerResult{Ok: tc.dnsOK},
				DNSDoH:     LayerResult{Ok: tc.dohOK},
				TCP:        LayerResult{Ok: tc.tcpOK, Error: tc.tcpErr},
				TLS:        LayerResult{Ok: tc.tlsOK},
				HTTP:       LayerResult{Ok: tc.httpOK, StubPage: tc.stubPage},
				DNSMismatch: tc.mismatch,
			}
			if tc.dnsOK {
				r.DNS.IPs = []string{"1.2.3.4"}
			}
			if tc.dohOK {
				r.DNSDoH.IPs = []string{"1.2.3.4"}
			}
			c := &Checker{}
			c.classify(r)
			if r.Verdict != tc.want {
				t.Errorf("classify(%s): expected %s, got %s (notes: %v)", tc.name, tc.want, r.Verdict, r.Notes)
			}
		})
	}
}
