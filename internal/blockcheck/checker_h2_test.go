package blockcheck

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Транспорт чекера должен уметь HTTP/2 (ALPN): иначе h2-only серверы
// (dns.quad9.net) отвечают 505 на HTTP/1.1 и классифицируются как DOWN.
func newH2TestServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	svr := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	svr.EnableHTTP2 = true
	svr.StartTLS()
	t.Cleanup(svr.Close)
	return svr
}

func TestCheckerHTTPClientUsesHTTP2(t *testing.T) {
	svr := newH2TestServer(t, http.StatusNotFound)
	c := NewChecker(true, true, true, 5)

	resp, err := c.httpClient.Get(svr.URL)
	if err != nil {
		t.Fatalf("запрос к тестовому h2-серверу: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.Proto != "HTTP/2.0" {
		t.Errorf("протокол = %q, ожидался HTTP/2.0 (ForceAttemptHTTP2 не работает)", resp.Proto)
	}
}
