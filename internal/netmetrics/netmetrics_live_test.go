// Живые замеры netmetrics против реальной сети. По умолчанию пропускаются,
// запускаются только с ZPUI_NETMETRICS_LIVE=1:
//
//	ZPUI_NETMETRICS_LIVE=1 go test ./internal/netmetrics/ -v -run TestLive
package netmetrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func liveEnabled(t *testing.T) {
	if os.Getenv("ZPUI_NETMETRICS_LIVE") == "" {
		t.Skip("live network test: set ZPUI_NETMETRICS_LIVE=1")
	}
}

// По-атомарно: холодный (с DNS+TLS) и тёплые TTFB ya.ru, затем минимум Ping().
func TestLivePingBreakdown(t *testing.T) {
	liveEnabled(t)
	for i := 0; i < 5; i++ {
		ms, err := pingOnce()
		if err != nil {
			t.Logf("attempt %d: error %v", i+1, err)
			continue
		}
		t.Logf("attempt %d: %d ms", i+1, ms)
	}
	best, err := Ping(pingAttempts)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	t.Logf("Ping(min of %d): %d ms", pingAttempts, best)
}

// Эталон RTT: системный ICMP ping ya.ru (то, что показывает Windows и
// примерно то, что показывает yandex.ru/internet).
func TestLiveICMPReference(t *testing.T) {
	liveEnabled(t)
	out, err := exec.Command("ping", "-n", "4", "-w", "2000", "ya.ru").CombinedOutput()
	if err != nil {
		t.Fatalf("ping: %v\n%s", err, out)
	}
	if decoded, derr := charmap.CodePage866.NewDecoder().Bytes(out); derr == nil {
		out = decoded
	}
	t.Logf("raw output:\n%s", strings.TrimSpace(string(out)))
	re := regexp.MustCompile(`(?:time|время)[=<]\s*(\d+)\s*(?:ms|мс)`)
	var times []string
	for _, m := range re.FindAllStringSubmatch(string(out), -1) {
		times = append(times, m[1]+"ms")
	}
	t.Logf("ICMP RTT: %s", strings.Join(times, ", "))
}

// Скорость при разных бюджетах: 2с — старый quick (разгон в знаменателе),
// 6с/8с — новые quick/полный бюджеты с отсечкой разгона, 12с — эталон.
func TestLiveSpeedBudgets(t *testing.T) {
	liveEnabled(t)
	for _, budget := range []time.Duration{2 * time.Second, quickSpeedMax, speedBudget, 12 * time.Second} {
		mbps, err := Speed(budget)
		if err != nil {
			t.Logf("budget %v: error %v", budget, err)
			continue
		}
		t.Logf("budget %v: %.1f Mbps", budget, mbps)
	}
}

// Оверхед перед полезными байтами: DNS+TCP+TLS+TTFB до repo.yandex.ru —
// этот отрезок Speed() кладёт в знаменатель, занижая результат.
func TestLiveHandshakeOverhead(t *testing.T) {
	liveEnabled(t)
	url := testFileURL(t)
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Range", "bytes=0-1048575")
		start := time.Now()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			t.Fatalf("do: %v", err)
		}
		buf := make([]byte, 1)
		_, rerr := io.ReadFull(resp.Body, buf)
		ttfb := time.Since(start)
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		cancel()
		if rerr != nil {
			t.Fatalf("read first byte: %v", rerr)
		}
		t.Logf("attempt %d: status %d, TTFB(range request) %v", i+1, resp.StatusCode, ttfb.Round(time.Millisecond))
	}
}

// Эталон пропускной способности: 4 параллельных ranged-потока, 8 секунд —
// грубая оценка того, что показал бы многопоточный спидтест.
func TestLiveMultiStreamReference(t *testing.T) {
	liveEnabled(t)
	url := testFileURL(t)
	const streams = 4
	const runFor = 8 * time.Second
	var wg sync.WaitGroup
	var totals [streams]int64
	start := time.Now()
	for s := 0; s < streams; s++ {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), runFor+5*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			offset := int64(s) * 32 << 20
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			buf := make([]byte, 64<<10)
			deadline := time.Now().Add(runFor)
			for time.Now().Before(deadline) {
				n, err := resp.Body.Read(buf)
				totals[s] += int64(n)
				if err != nil {
					return
				}
			}
		}(s)
	}
	wg.Wait()
	elapsed := time.Since(start)
	var total int64
	for s := 0; s < streams; s++ {
		t.Logf("stream %d: %.1f MB", s, float64(totals[s])/(1<<20))
		total += totals[s]
	}
	t.Logf("multi-stream %dx%v: %.1f Mbps total", streams, runFor, float64(total)*8/elapsed.Seconds()/1e6)
}

func testFileURL(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, RepoListing, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	m := debRe.FindStringSubmatch(string(body))
	if len(m) < 2 {
		t.Fatalf("no .deb in listing")
	}
	return RepoListing + m[1]
}
