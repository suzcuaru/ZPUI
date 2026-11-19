package blockcheck

import (
	"errors"
	"strings"
	"testing"
)

// --- parseURL ---

func TestParseURL(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantFull string
		wantNil  bool
	}{
		{"", "", "", true},
		{"   ", "", "", true},
		{"https://", "", "", true},
		{"discord.com", "discord.com", "https://discord.com", false},
		{"http://example.com", "example.com", "http://example.com", false},
		{"https://youtube.com/watch?v=1", "youtube.com", "https://youtube.com/watch?v=1", false},
	}
	for _, tc := range cases {
		_, host, full := parseURL(tc.in)
		if tc.wantNil {
			if host != "" || full != "" {
				t.Errorf("parseURL(%q): expected empty, got host=%q full=%q", tc.in, host, full)
			}
			continue
		}
		if host != tc.wantHost || full != tc.wantFull {
			t.Errorf("parseURL(%q): host=%q (want %q), full=%q (want %q)", tc.in, host, tc.wantHost, full, tc.wantFull)
		}
	}
}

// --- NewChecker timeout normalization ---

func TestNewCheckerZeroTimeout(t *testing.T) {
	if c := NewChecker(false, false, false, 0); c == nil {
		t.Fatal("expected non-nil checker for timeout=0")
	}
	if c := NewChecker(false, false, false, -5); c == nil {
		t.Fatal("expected non-nil checker for negative timeout")
	}
}

// --- Check invalid-URL path ---

func TestCheckInvalidURL(t *testing.T) {
	c := NewChecker(false, false, false, 2)
	r := c.Check("https://")
	if r.Verdict != VerdictDown {
		t.Errorf("expected DOWN for invalid URL, got %s", r.Verdict)
	}
	if r.Confidence != ConfHigh {
		t.Errorf("expected HIGH confidence, got %s", r.Confidence)
	}
	if len(r.Notes) == 0 || !strings.Contains(r.Notes[0], "URL") {
		t.Errorf("expected invalid-URL note, got %v", r.Notes)
	}
}

// --- classify TCP layer ---

func TestClassifyTCP(t *testing.T) {
	cases := []struct {
		name   string
		tcpErr string
		want   string
	}{
		{"dns_failure", "dns_failure", VerdictDNSBlock},
		{"timeout", "timeout", VerdictTimeout},
		{"reset", "connection_reset", VerdictTCPBlock},
		{"other", "connection_refused", VerdictTCPBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &CheckResult{TCP: LayerResult{Ok: false, Error: tc.tcpErr}}
			c := &Checker{checkTCP: true}
			c.classify(r)
			if r.Verdict != tc.want {
				t.Errorf("classify TCP %s: got %s, want %s (notes %v)", tc.name, r.Verdict, tc.want, r.Notes)
			}
		})
	}
}

// --- classify TLS layer extra branches ---

func TestClassifyTLSDNSBlock(t *testing.T) {
	r := &CheckResult{TLS: LayerResult{Ok: false, Error: "dns_failure"}}
	c := &Checker{checkTLS: true}
	c.classify(r)
	if r.Verdict != VerdictDNSBlock {
		t.Errorf("expected DNSBlock for TLS dns_failure, got %s", r.Verdict)
	}
}

func TestClassifyTLSResetVsGeneric(t *testing.T) {
	c := &Checker{checkTLS: true}

	// connection_reset → specific note mentioning "сброшено".
	r1 := &CheckResult{TLS: LayerResult{Ok: false, Error: "connection_reset"}}
	c.classify(r1)
	if r1.Verdict != VerdictTLSBlock {
		t.Errorf("expected TLSBlock, got %s", r1.Verdict)
	}
	if !noteContains(r1.Notes, "сброшено") {
		t.Errorf("expected reset-specific note, got %v", r1.Notes)
	}

	// Generic TLS error → generic note still TLSBlock.
	r2 := &CheckResult{TLS: LayerResult{Ok: false, Error: "tls_error"}}
	c.classify(r2)
	if r2.Verdict != VerdictTLSBlock {
		t.Errorf("expected TLSBlock for generic err, got %s", r2.Verdict)
	}
	if !noteContains(r2.Notes, "tls_error") {
		t.Errorf("expected generic note referencing tls_error, got %v", r2.Notes)
	}
}

// --- classify HTTP extras ---

func TestClassifyHTTPExtras(t *testing.T) {
	c := &Checker{checkTLS: true, checkHTTP: true}

	// HTTP 451 stub → HTTPStub with note referencing 451.
	r := &CheckResult{TLS: LayerResult{Ok: true}, HTTP: LayerResult{StubPage: true, Status: 451}}
	c.classify(r)
	if r.Verdict != VerdictHTTPStub {
		t.Fatalf("expected HTTPStub for 451, got %s", r.Verdict)
	}
	if !noteContains(r.Notes, "451") {
		t.Errorf("expected 451 note, got %v", r.Notes)
	}

	// Generic stub (non-451).
	r2 := &CheckResult{TLS: LayerResult{Ok: true}, HTTP: LayerResult{StubPage: true, Status: 200}}
	c.classify(r2)
	if r2.Verdict != VerdictHTTPStub {
		t.Errorf("expected HTTPStub, got %s", r2.Verdict)
	}

	// HTTP connection_reset → TLSBlock (DPI cutting during HTTP).
	r3 := &CheckResult{TLS: LayerResult{Ok: true}, HTTP: LayerResult{Error: "connection_reset"}}
	c.classify(r3)
	if r3.Verdict != VerdictTLSBlock {
		t.Errorf("expected TLSBlock for HTTP reset, got %s", r3.Verdict)
	}
}

// --- classify fallback DOWN ---

func TestClassifyFallbackDown(t *testing.T) {
	c := &Checker{checkHTTP: true}

	// All HTTP fields empty → Down with "недоступен".
	r := &CheckResult{}
	c.classify(r)
	if r.Verdict != VerdictDown {
		t.Errorf("expected Down, got %s", r.Verdict)
	}
	if !noteContains(r.Notes, "недоступен") {
		t.Errorf("expected 'недоступен' note, got %v", r.Notes)
	}

	// With HTTP error string → note is the error itself.
	r2 := &CheckResult{HTTP: LayerResult{Error: "eof"}}
	c.classify(r2)
	if r2.Verdict != VerdictDown {
		t.Errorf("expected Down, got %s", r2.Verdict)
	}
	if !noteContains(r2.Notes, "eof") {
		t.Errorf("expected 'eof' note, got %v", r2.Notes)
	}
}

// --- checkOne aggregation (instant: invalid URL → DOWN) ---

func TestCheckOneAggregation(t *testing.T) {
	c := NewChecker(false, true, true, 2)
	res := c.checkOne(BulkTarget{Name: "bad", URL: "https://"})

	if res.Name != "bad" {
		t.Errorf("name = %q, want bad", res.Name)
	}
	if res.URL != "https://" {
		t.Errorf("url = %q", res.URL)
	}
	if res.OK {
		t.Errorf("invalid URL should not be OK")
	}
	if res.Reason == "" {
		t.Errorf("reason should be populated for non-OK result")
	}
	if res.Verdict != VerdictDown {
		t.Errorf("expected Down verdict, got %s", res.Verdict)
	}
}

func TestBulkCheckStructure(t *testing.T) {
	c := NewChecker(false, true, true, 1)
	report := c.BulkCheck(
		[]BulkTarget{{Name: "a", URL: "https://"}},
		[]BulkTarget{{Name: "u", URL: "https://"}},
	)
	if len(report.Default) != 1 {
		t.Fatalf("expected 1 default result, got %d", len(report.Default))
	}
	if len(report.User) != 1 {
		t.Fatalf("expected 1 user result, got %d", len(report.User))
	}
	for _, r := range append(append([]BulkResult{}, report.Default...), report.User...) {
		if r.OK {
			t.Errorf("invalid URL should not be OK: %+v", r)
		}
		if r.Reason == "" {
			t.Errorf("reason empty for %+v", r)
		}
	}
}

func TestBulkCheckEmptyTargets(t *testing.T) {
	c := NewChecker(false, true, true, 1)
	report := c.BulkCheck(nil, nil)
	if len(report.Default) != 0 || len(report.User) != 0 {
		t.Errorf("expected empty report, got %+v", report)
	}
}

// --- isBlockingVerdict extra coverage ---

func TestIsBlockingVerdictComplete(t *testing.T) {
	blocking := map[string]bool{
		VerdictTCPBlock: true,
		VerdictTLSBlock: true,
		VerdictDNSBlock: true,
		VerdictHTTPStub: true,
	}
	for v := range blocking {
		if !isBlockingVerdict(v) {
			t.Errorf("expected %s to be blocking", v)
		}
	}
	nonBlocking := []string{VerdictOK, VerdictTimeout, VerdictDown, VerdictUnknown, "", "weird"}
	for _, v := range nonBlocking {
		if isBlockingVerdict(v) {
			t.Errorf("expected %q to NOT be blocking", v)
		}
	}
}

// --- Provider parsers ---

func TestParseIPInfo(t *testing.T) {
	body := []byte(`{"ip":"1.2.3.4","city":"Moscow","country":"RU","org":"AS123 Some ISP"}`)
	var info ProviderInfo
	if !parseIPInfo(body, &info) {
		t.Fatal("expected success")
	}
	if info.IP != "1.2.3.4" || info.City != "Moscow" || info.Country != "RU" {
		t.Errorf("wrong parse: %+v", info)
	}
	if info.ASN != "AS123" || info.ISP != "Some ISP" {
		t.Errorf("ASN/ISP split wrong: ASN=%q ISP=%q", info.ASN, info.ISP)
	}

	// Org without space → ISP = org, ASN empty.
	var info2 ProviderInfo
	parseIPInfo([]byte(`{"ip":"9.9.9.9","org":"SoloOrg"}`), &info2)
	if info2.ISP != "SoloOrg" {
		t.Errorf("solo org ISP = %q", info2.ISP)
	}

	// Empty IP → false.
	if parseIPInfo([]byte(`{"ip":""}`), &info) {
		t.Error("expected false for empty IP")
	}
	// Bad JSON → false.
	if parseIPInfo([]byte("not json"), &info) {
		t.Error("expected false for bad json")
	}
}

func TestParseIPAPI(t *testing.T) {
	body := []byte(`{"query":"5.6.7.8","city":"Berlin","country":"DE","org":"OrgName","isp":"MyISP","as":"AS456"}`)
	var info ProviderInfo
	if !parseIPAPI(body, &info) {
		t.Fatal("expected success")
	}
	if info.IP != "5.6.7.8" || info.City != "Berlin" || info.Country != "DE" || info.ISP != "MyISP" || info.ASN != "AS456" {
		t.Errorf("wrong parse: %+v", info)
	}
	if parseIPAPI([]byte(`{"query":""}`), &info) {
		t.Error("expected false for empty query")
	}
	if parseIPAPI([]byte("{bad"), &info) {
		t.Error("expected false for bad json")
	}
}

func TestParseMyIP(t *testing.T) {
	body := []byte(`{"ip":"7.7.7.7","city":"Paris","country":"FR","asn":"AS789 MyOrg"}`)
	var info ProviderInfo
	if !parseMyIP(body, &info) {
		t.Fatal("expected success")
	}
	if info.IP != "7.7.7.7" || info.City != "Paris" || info.Country != "FR" {
		t.Errorf("wrong parse: %+v", info)
	}
	if info.ASN != "AS789" || info.ISP != "MyOrg" {
		t.Errorf("ASN/ISP wrong: ASN=%q ISP=%q", info.ASN, info.ISP)
	}
	// Solo org.
	var info2 ProviderInfo
	parseMyIP([]byte(`{"ip":"7.7.7.7","asn":"SoloASN"}`), &info2)
	if info2.ISP != "SoloASN" {
		t.Errorf("solo asn ISP = %q", info2.ISP)
	}
	if parseMyIP([]byte(`{"ip":""}`), &info) {
		t.Error("expected false for empty ip")
	}
	if parseMyIP([]byte("bad"), &info) {
		t.Error("expected false for bad json")
	}
}

// --- classifyErr default branch ---

func TestClassifyErrDefault(t *testing.T) {
	// Unknown error string → returned as-is (lowercased).
	got := classifyErr(errors.New("SomeWeirdError"))
	if got != "someweirderror" {
		t.Errorf("default classifyErr = %q, want someweirderror", got)
	}
	if classifyErr(nil) != "" {
		t.Error("nil err should return empty")
	}
}

// --- detectStubPage boundary ---

func TestDetectStubPageBoundary(t *testing.T) {
	// Exactly at/over 4096 bytes → not a stub even with marker.
	big := []byte("доступ ограничен")
	for len(big) <= 4096 {
		big = append(big, big...)
	}
	if detectStubPage(big) {
		t.Error("body > 4096 should not be detected as stub")
	}
	// Small body without any marker → false.
	if detectStubPage([]byte("<html>hello world</html>")) {
		t.Error("body without marker should not be detected")
	}
}

// noteContains is a small helper to assert a substring appears in any note.
func noteContains(notes []string, sub string) bool {
	for _, n := range notes {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}
