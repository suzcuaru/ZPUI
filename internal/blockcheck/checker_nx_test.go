package blockcheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestIsNXDomainErr(t *testing.T) {
	nx := &net.DNSError{Err: "no such host", Name: "dead.example", IsNotFound: true}
	if !isNXDomainErr(nx) {
		t.Error("DNSError with IsNotFound=true must be detected as NXDOMAIN")
	}
	if !isNXDomainErr(fmt.Errorf("dial tcp: lookup dead.example: %w", nx)) {
		t.Error("wrapped NXDOMAIN must be detected")
	}

	if isNXDomainErr(&net.DNSError{Err: "server misbehaving", IsTemporary: true}) {
		t.Error("DNS server failure must not be treated as NXDOMAIN")
	}
	if isNXDomainErr(errors.New("dial tcp: connection refused")) {
		t.Error("connection refused must not be treated as NXDOMAIN")
	}
	if isNXDomainErr(errors.New("read tcp: i/o timeout")) {
		t.Error("timeout must not be treated as NXDOMAIN")
	}
	if isNXDomainErr(nil) {
		t.Error("nil error must not be treated as NXDOMAIN")
	}
}

func TestCheckFlagsNXDomainOnTCP(t *testing.T) {
	c := NewChecker(true, true, true, 1)
	c.dialCtx = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, &net.DNSError{Err: "no such host", Name: "dead.example", IsNotFound: true}
	}

	res := c.Check("https://dead.example/")
	if !res.DNSNotFound {
		t.Fatal("DNSNotFound flag not set on NXDOMAIN at TCP layer")
	}
	if res.Verdict != VerdictDNSBlock {
		t.Fatalf("verdict = %s, want %s", res.Verdict, VerdictDNSBlock)
	}
	if res.TCP.Ok || res.TLS.Ok || res.HTTP.Ok {
		t.Error("no layer may be ok for NXDOMAIN")
	}
	if res.TCP.Error != "dns_failure" {
		t.Errorf("tcp error = %q, want dns_failure (classification unchanged)", res.TCP.Error)
	}
}

func TestCheckFlagsNXDomainOnTLS(t *testing.T) {
	c := NewChecker(false, true, true, 1)
	c.dialCtx = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, &net.DNSError{Err: "no such host", Name: "dead.example", IsNotFound: true}
	}

	res := c.Check("https://dead.example/")
	if !res.DNSNotFound {
		t.Fatal("DNSNotFound flag not set on NXDOMAIN at TLS layer")
	}
	if res.Verdict != VerdictDNSBlock {
		t.Fatalf("verdict = %s, want %s", res.Verdict, VerdictDNSBlock)
	}
	if res.TLS.Error != "dns_failure" {
		t.Errorf("tls error = %q, want dns_failure (classification unchanged)", res.TLS.Error)
	}
}

func TestCheckNoNXDomainFlagForRefused(t *testing.T) {
	c := NewChecker(true, true, true, 1)
	c.dialCtx = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, errors.New("dial tcp 192.0.2.1:443: connection refused")
	}

	res := c.Check("https://blocked.example/")
	if res.DNSNotFound {
		t.Fatal("DNSNotFound must not be set for connection refused")
	}
	if res.Verdict != VerdictTCPBlock {
		t.Fatalf("verdict = %s, want %s", res.Verdict, VerdictTCPBlock)
	}
}
