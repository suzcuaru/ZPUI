package isp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNormalizeISP(t *testing.T) {
	cases := map[string]string{
		"  Rostelecom ":  "rostelecom",
		"MTS":            "mts",
		"  ":             "",
		"":               "",
		"Билайн":         "билайн",
	}
	for in, want := range cases {
		if got := NormalizeISP(in); got != want {
			t.Errorf("NormalizeISP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMakeOperatorKey(t *testing.T) {
	cases := map[string]string{
		"  Rostelecom ": "rostelecom",
		"MTS OOO":       "mts ooo",
		"":              "",
	}
	for in, want := range cases {
		if got := MakeOperatorKey(in); got != want {
			t.Errorf("MakeOperatorKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitOrg(t *testing.T) {
	cases := []struct {
		in      string
		wantIsp string
		wantAsn string
	}{
		{"AS43038 МТС ООО \"Мобайл\"", "МТС ООО \"Мобайл\"", "AS43038"},
		{"as1234 Some ISP", "Some ISP", "as1234"},
		{"JustAName", "JustAName", ""},
		{"", "", ""},
		{"   ", "", ""},
		{"AS1", "AS1", ""}, // single token starting with AS, len(parts)==1
	}
	for _, tc := range cases {
		isp, asn := splitOrg(tc.in)
		if isp != tc.wantIsp || asn != tc.wantAsn {
			t.Errorf("splitOrg(%q) = (%q,%q), want (%q,%q)", tc.in, isp, asn, tc.wantIsp, tc.wantAsn)
		}
	}
}

func TestCleanOperatorName(t *testing.T) {
	cases := map[string]string{
		"МТС ООО \"Мобайл\"":                   "МТС ООО \"Мобайл\"",
		"ООО Ростелеком":                        "Ростелеком",
		"ПАО «Билайн»":                          "«Билайн»",
		"Ростелеком филиал":                     "Ростелеком",
		"SomeNet branch":                        "SomeNet",
		"ROSTELECOM OAO":                        "ROSTELECOM",
		"\"Quoted\"":                            "Quoted",
		"":                                      "",
		"   ":                                   "",
		"Acme LLC":                              "Acme",
		"Foo Inc":                               "Foo",
		"\"\"":                                  "\"\"",
	}
	for in, want := range cases {
		got := cleanOperatorName(in)
		if got != want {
			t.Errorf("cleanOperatorName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractASN(t *testing.T) {
	cases := map[string]string{
		"AS43038 MTS":  "AS43038",
		"AS1234":       "AS1234",
		"":             "",
		"AS9  AS2":     "AS9",
		"nodash":       "nodash",
	}
	for in, want := range cases {
		if got := extractASN(in); got != want {
			t.Errorf("extractASN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseIPInfo(t *testing.T) {
	t.Run("valid with AS field", func(t *testing.T) {
		data := []byte(`{"ip":"1.2.3.4","org":"AS43038 МТС","city":"Moscow","country":"RU","as":"AS43038 MTS OJSC"}`)
		op := parseIPInfo(data)
		if op == nil {
			t.Fatal("expected operator, got nil")
		}
		if op.IP != "1.2.3.4" {
			t.Errorf("IP = %q", op.IP)
		}
		if op.City != "Moscow" {
			t.Errorf("City = %q", op.City)
		}
		if op.ASN != "AS43038" {
			t.Errorf("ASN = %q, want AS43038 (from AS field)", op.ASN)
		}
	})

	t.Run("valid without AS field uses splitOrg", func(t *testing.T) {
		data := []byte(`{"ip":"9.9.9.9","org":"AS999 Name","city":"X","country":"RU"}`)
		op := parseIPInfo(data)
		if op.ASN != "AS999" {
			t.Errorf("ASN = %q, want AS999", op.ASN)
		}
		if op.ISP != "Name" {
			t.Errorf("ISP = %q, want Name", op.ISP)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if op := parseIPInfo([]byte("not json")); op != nil {
			t.Errorf("expected nil for invalid json, got %+v", op)
		}
	})
}

func TestParseIPAPI(t *testing.T) {
	t.Run("uses ISP field", func(t *testing.T) {
		data := []byte(`{"query":"8.8.8.8","isp":"Google","org":"AS15169 Google","city":"MTV","country":"US","as":"AS15169 Google LLC"}`)
		op := parseIPAPI(data)
		if op == nil {
			t.Fatal("nil")
		}
		if op.ISP != "Google" {
			t.Errorf("ISP = %q", op.ISP)
		}
		if op.IP != "8.8.8.8" {
			t.Errorf("IP = %q", op.IP)
		}
		if op.ASN != "AS15169" {
			t.Errorf("ASN = %q", op.ASN)
		}
	})

	t.Run("empty ISP falls back to org", func(t *testing.T) {
		data := []byte(`{"query":"1.1.1.1","isp":"","org":"AS13335 Cloudflare","city":"","country":"","as":"AS13335 Cloudflare"}`)
		op := parseIPAPI(data)
		if op.ISP != "Cloudflare" {
			t.Errorf("ISP = %q, want Cloudflare", op.ISP)
		}
		if op.ASN != "AS13335" {
			t.Errorf("ASN = %q", op.ASN)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if op := parseIPAPI([]byte("{{{")); op != nil {
			t.Errorf("expected nil, got %+v", op)
		}
	})
}

func TestParseMyIP(t *testing.T) {
	t.Run("valid with ASN override", func(t *testing.T) {
		data := []byte(`{"ip":"5.5.5.5","org":"AS10 Foo","city":"C","country":"RU","asn":"AS9999"}`)
		op := parseMyIP(data)
		if op == nil {
			t.Fatal("nil")
		}
		if op.ASN != "AS9999" {
			t.Errorf("ASN = %q, want AS9999", op.ASN)
		}
		if op.ISP != "Foo" {
			t.Errorf("ISP = %q", op.ISP)
		}
	})

	t.Run("valid without ASN field uses splitOrg", func(t *testing.T) {
		data := []byte(`{"ip":"5.5.5.5","org":"AS10 Foo","city":"C","country":"RU"}`)
		op := parseMyIP(data)
		if op.ASN != "AS10" {
			t.Errorf("ASN = %q, want AS10", op.ASN)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if op := parseMyIP([]byte("bad")); op != nil {
			t.Errorf("expected nil, got %+v", op)
		}
	})
}

func TestDetectOperator_NoNetworkReturnsUnknown(t *testing.T) {
	orig := client
	client = newFailingClient()
	defer func() { client = orig }()

	op, err := DetectOperator()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op == nil {
		t.Fatal("expected non-nil operator")
	}
	if op.Key != "unknown" {
		t.Errorf("Key = %q, want unknown", op.Key)
	}
	if op.Name != "Неизвестно" {
		t.Errorf("Name = %q, want Неизвестно", op.Name)
	}
}

func newFailingClient() *http.Client {
	return &http.Client{
		Timeout: 1 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return nil, errors.New("test: network disabled")
			},
		},
	}
}
