package blockcheck

import (
        "errors"
        "os"
        "path/filepath"
        "strings"
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

func TestCheckLive(t *testing.T) {
        checker := NewChecker(false, true, true, 10)
        result := checker.Check("https://www.google.com")

        t.Logf("Google: verdict=%s confidence=%s", result.Verdict, result.Confidence)
        t.Logf("  TCP: ok=%v err=%s", result.TCP.Ok, result.TCP.Error)
        t.Logf("  TLS: ok=%v err=%s", result.TLS.Ok, result.TLS.Error)
        t.Logf("  HTTP: ok=%v status=%d stub=%v err=%s server=%s",
                result.HTTP.Ok, result.HTTP.Status, result.HTTP.StubPage, result.HTTP.Error, result.HTTP.Header)
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

        checker := NewChecker(false, true, true, 10)
        report := checker.BulkCheck(targets, nil)

        if len(report.Default) != 3 {
                t.Fatalf("expected 3 results, got %d", len(report.Default))
        }

        okCount := 0
        for _, r := range report.Default {
                t.Logf("%-12s ok=%-5v blocked=%-5v verdict=%-12s %dms reason=%s",
                        r.Name, r.OK, r.Blocked, r.Verdict, r.LatencyMs, r.Reason)
                if r.OK {
                        okCount++
                }
        }

        if okCount == 0 {
                t.Skip("no network: all targets failed")
        }
}

func TestCheckBlockedResource(t *testing.T) {
        checker := NewChecker(false, true, true, 5)

        result := checker.Check("https://192.0.2.1")
        t.Logf("Fake IP: verdict=%s http_err=%s", result.Verdict, result.HTTP.Error)

        if result.Verdict == VerdictOK {
                t.Error("192.0.2.1 (TEST-NET) should not be OK")
        }
}

// TestClassifyLogic — проверка классификации для типовых ситуаций.
func TestClassifyLogic(t *testing.T) {
        cases := []struct {
                name     string
                tlsOk    bool
                tlsErr   string
                httpOK   bool
                stubPage bool
                httpErr  string
                status   int
                want     string
        }{
                {"all_ok", true, "", true, false, "", 200, VerdictOK},
                {"redirect_ok", true, "", true, false, "", 301, VerdictOK},
                {"tls_block", false, "connection_reset", false, false, "", 0, VerdictTLSBlock},
                {"tls_timeout", false, "timeout", false, false, "", 0, VerdictTimeout},
                {"http_stub", true, "", false, true, "", 451, VerdictHTTPStub},
                {"http_timeout", true, "", false, false, "timeout", 0, VerdictTimeout},
                {"server_error_5xx", true, "", false, false, "", 503, VerdictDown},
                // 4xx без CDN-заголовка — тоже не блокировка, сервер жив.
                {"not_found_4xx", true, "", false, false, "", 404, VerdictOK},
                {"forbidden_4xx", true, "", false, false, "", 403, VerdictOK},
        }

        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        r := &CheckResult{
                                TLS:  LayerResult{Ok: tc.tlsOk, Error: tc.tlsErr},
                                HTTP: LayerResult{Ok: tc.httpOK, StubPage: tc.stubPage, Error: tc.httpErr, Status: tc.status},
                        }
                        c := &Checker{checkTLS: true, checkHTTP: true}
                        c.classify(r)
                        if r.Verdict != tc.want {
                                t.Errorf("classify(%s): expected %s, got %s (notes: %v)", tc.name, tc.want, r.Verdict, r.Notes)
                        }
                })
        }
}

// TestClassifyCDN4xx — 4xx от CDN (с Server: cloudflare/nginx) должен
// давать VerdictOK с пояснением про CDN.
func TestClassifyCDN4xx(t *testing.T) {
        r := &CheckResult{
                TLS:  LayerResult{Ok: true},
                HTTP: LayerResult{Ok: false, Status: 403, Header: "cloudflare"},
        }
        c := &Checker{checkTLS: true, checkHTTP: true}
        c.classify(r)

        if r.Verdict != VerdictOK {
                t.Errorf("CDN 4xx: expected OK, got %s (notes: %v)", r.Verdict, r.Notes)
        }
        if !r.HTTP.Ok {
                t.Error("HTTP.Ok should be flipped to true for CDN 4xx")
        }
        found := false
        for _, n := range r.Notes {
                if strings.Contains(strings.ToLower(n), "cdn") {
                        found = true
                        break
                }
        }
        if !found {
                t.Errorf("expected CDN note in %v", r.Notes)
        }
}

// TestClassifyErr — проверка распознавания ошибок, особенно Windows
// "wsarecv: An existing connection was forcibly closed".
func TestClassifyErr(t *testing.T) {
        cases := []struct {
                name   string
                errStr string
                want   string
        }{
                {"dns_no_host", "dial tcp: lookup badhost.example: no such host", "dns_failure"},
                {"forcibly_closed", "read tcp 192.168.3.2:56268->188.253.19.77:443: wsarecv: An existing connection was forcibly closed by the remote host.", "connection_reset"},
                {"connection_reset", "read tcp: connection reset by peer", "connection_reset"},
                {"broken_pipe", "write tcp: broken pipe", "connection_reset"},
                {"eof", "tls: handshake failure: EOF", "eof"},
                {"tls_handshake", "tls: handshake failure", "tls_error"},
                {"timeout", "dial tcp: i/o timeout", "timeout"},
                {"deadline", "context deadline exceeded", "timeout"},
                {"connection_refused", "dial tcp: connection refused", "connection_refused"},
        }
        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        got := classifyErr(errors.New(tc.errStr))
                        if got != tc.want {
                                t.Errorf("classifyErr(%q): got %q, want %q", tc.errStr, got, tc.want)
                        }
                })
        }
}

// TestIsBlockingVerdict — только реальные блокировки должны давать Blocked=true.
func TestIsBlockingVerdict(t *testing.T) {
        blocking := []string{VerdictTCPBlock, VerdictTLSBlock, VerdictDNSBlock, VerdictHTTPStub}
        for _, v := range blocking {
                if !isBlockingVerdict(v) {
                        t.Errorf("expected %s to be blocking", v)
                }
        }
        nonBlocking := []string{VerdictOK, VerdictTimeout, VerdictDown, VerdictUnknown}
        for _, v := range nonBlocking {
                if isBlockingVerdict(v) {
                        t.Errorf("expected %s to NOT be blocking", v)
                }
        }
}

// TestDetectStubPage — проверка маркеров заглушек.
func TestDetectStubPage(t *testing.T) {
        stubBody := []byte(`<html><body><h1>Доступ к информационному ресурсу ограничен</h1>
<p>Меры по ограничению доступа</p></body></html>`)
        if !detectStubPage(stubBody) {
                t.Error("stub body should be detected")
        }

        // Реальный сайт с упоминанием "роскомнадзор" в новостях — НЕ должен
        // детектиться как заглушка (мы убрали "роскомнадзор" из маркеров,
        // потому что он встречается на нормальных новостных сайтах).
        realBodyWithNews := []byte(`<html><body><h1>Новости</h1>
<p>Роскомнадзор сообщил о новых правилах</p></body></html>`)
        if detectStubPage(realBodyWithNews) {
                t.Error("real news site with 'роскомнадзор' should NOT be detected as stub")
        }

        // Большой реальный сайт — не должен детектиться как заглушка.
        realBody := []byte(`<html><head><title>AnimeGo — смотреть аниме онлайн</title></head>
<body><h1>AnimeGo</h1><p>Смотрите аниме онлайн в HD качестве</p></body></html>`)
        for len(realBody) < 10000 {
                realBody = append(realBody, []byte("<p>контент</p>")...)
        }
        if detectStubPage(realBody) {
                t.Error("real site body should not be detected as stub")
        }
}

// TestCDNServerMarkers — Cloudflare/nginx/Amazon должны распознаваться.
func TestCDNServerMarkers(t *testing.T) {
        for _, s := range []string{"cloudflare", "nginx/1.20.1", "amazon cloudfront", "akamai"} {
                if !containsAny(s, cdnServerMarkers) {
                        t.Errorf("CDN marker %q not detected", s)
                }
        }
        if containsAny("microsoft-iis/10.0", []string{"cloudflare"}) {
                t.Error("false positive")
        }
}
