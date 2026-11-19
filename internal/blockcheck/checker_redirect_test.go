package blockcheck

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func redirectChainServer(n int) *httptest.Server {
	mux := http.NewServeMux()
	for i := 1; i <= n; i++ {
		if i == n {
			mux.HandleFunc(fmt.Sprintf("/hop%d", i), func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})
			continue
		}
		mux.HandleFunc(fmt.Sprintf("/hop%d", i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/hop%d", i+1), http.StatusMovedPermanently)
		})
	}
	return httptest.NewServer(mux)
}

// Реальная цепочка discord.dev занимает 5 редиректов — раньше лимит
// обрывал её на финальном запросе («too many redirects» → DOWN).
func TestCheckFollowsFiveRedirectChain(t *testing.T) {
	srv := redirectChainServer(6)
	defer srv.Close()

	c := NewChecker(false, false, true, 5)
	res := c.Check(srv.URL + "/hop1")
	if res.Verdict != VerdictOK {
		t.Fatalf("expected OK, got %s (%v)", res.Verdict, res.Notes)
	}
	if !res.HTTP.Ok || res.HTTP.Status != http.StatusOK {
		t.Fatalf("expected HTTP 200, got status=%d ok=%v", res.HTTP.Status, res.HTTP.Ok)
	}
}

// Бесконечный редирект должен упираться в лимит, а не крутиться вечно.
func TestCheckRedirectLoopBounded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusMovedPermanently)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewChecker(false, false, true, 5)
	res := c.Check(srv.URL + "/loop")
	if res.Verdict == VerdictOK {
		t.Fatalf("redirect loop must not end OK, got %s", res.Verdict)
	}
}
