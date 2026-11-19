package tgproxy

import (
	"strings"
	"testing"

	"zpui/internal/config"
)

func hasPair(args []string, pair ...string) bool {
	if len(pair) == 1 {
		for _, a := range args {
			if a == pair[0] {
				return true
			}
		}
		return false
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] == pair[0] && args[i+1] == pair[1] {
			return true
		}
	}
	return false
}

func TestBuildArgsMinimal(t *testing.T) {
	pc := config.TelegramProxyConfig{
		BindHost: "0.0.0.0", Port: 1443, FallbackCF: true,
		Secret: "abcdef0123456789abcdef0123456789",
	}
	args := buildArgs(pc, `C:\logs\tgproxy.log`)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--host 0.0.0.0", "--port 1443", "--secret abcdef0123456789abcdef0123456789",
		`--log-file C:\logs\tgproxy.log`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
	if hasPair(args, "--no-cfproxy") {
		t.Fatalf("default config must not disable cfproxy: %v", args)
	}
	if hasPair(args, "--no-h2") {
		t.Fatalf("H2 enabled by default: %v", args)
	}
}

func TestBuildArgsFull(t *testing.T) {
	pc := config.TelegramProxyConfig{
		BindHost: "127.0.0.1", Port: 9050,
		Secret:          "00",
		FakeTLSDomain:   "Example.COM",
		ProxyProtocol:   true,
		FallbackCF:      false,
		NoH2:            true,
		NoSecure:        true,
		ForceTestDC:     true,
		Verbose:         true,
		PoolSize:        8,
		BufKB:           512,
		DCIP:            []string{"2:1.2.3.4", "4:5.6.7.8"},
		CFUserDomains:   []string{"a.com"},
		CFWorkerDomains: []string{"w.workers.dev"},
	}
	args := buildArgs(pc, `L`)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--port 9050", "--fake-tls-domain Example.COM", "--proxy-protocol",
		"--no-cfproxy", "--no-h2", "--no-secure", "--force-test-dc", "-v",
		"--pool-size 8", "--buf-kb 512",
		"--dc-ip 2:1.2.3.4", "--dc-ip 4:5.6.7.8",
		"--cfproxy-domain a.com", "--cfproxy-worker-domain w.workers.dev",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
}
