// Package netmetrics — замер качества сети по методике Яндекс.Интернетометра
// (yandex.ru/internet): пинг — время ответа тёплого (keep-alive) запроса к
// ping-эндпоинту CDN Яндекса, скорость — параллельное ranged-скачивание
// probes/50mb с того же CDN без учёта рукопожатия. Запасной источник
// скорости — ranged-скачивание из репозитория Яндекс.Браузера. Показатели
// используются как дополнительный фактор при проверках, в автоподборе
// стратегий, в графике монитора и в отчёте.
package netmetrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// speedtestHost — cdnFallback-хост из конфига Яндекс.Интернетометра:
	// отдаёт /ping и /probes/50mb напрямую (основной хост редиректит на
	// гео-ноду, для повторяемости проще фиксированный).
	speedtestHost = "cdnrrtcrwqtgkafu6ulm.svc.cdn.yandex.net"
	pingURL       = "https://" + speedtestHost + "/ping"
	probeURL      = "https://" + speedtestHost + "/probes/50mb"
	// RepoListing — запасной источник: каталог стабильной сборки
	// Яндекс.Браузера (.deb ~150 МБ, отдаёт 206 Partial Content).
	RepoListing = "https://repo.yandex.ru/yandex-browser/deb/pool/main/y/yandex-browser-stable/"

	pingWarmups   = 2
	pingAttempts  = 5
	pingTimeout   = 2 * time.Second
	speedBudget   = 8 * time.Second // сколько качаем
	speedStreams  = 4               // параллельные ranged-потока
	speedMaxBytes = 64 << 20        // предохранитель по суммарному объёму
	quickSpeedMax = 6 * time.Second
)

var debRe = regexp.MustCompile(`href="([^"]+\.deb)"`)

// Result — снимок качества сети.
type Result struct {
	PingMs    int64     `json:"ping_ms"`
	SpeedMbps float64   `json:"speed_mbps"`
	At        time.Time `json:"at"`
}

// sharedTransport — общий транспорт для всех замеров: keep-alive соединения
// переживают отдельные вызовы, тёплый TLS переиспользуется и пингом, и
// потоками спидтеста.
var sharedTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = 16
	return t
}()

func client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: sharedTransport}
}

// Ping — минимальное время ответа ping-эндпоинта CDN Яндекса среди успешных
// попыток, мс. Прогревы (DNS+TLS) в замер не идут: тёплый TTFB соответствует
// «Времени отклика» интернетометра, минимум отсекает хвосты очередей.
// Ошибка — если все попытки не удались.
func Ping(attempts int) (int64, error) {
	if attempts <= 0 {
		attempts = pingAttempts
	}
	for i := 0; i < pingWarmups; i++ {
		_, _ = pingOnce()
	}
	var best int64
	for i := 0; i < attempts; i++ {
		if ms, err := pingOnce(); err == nil && (best == 0 || ms < best) {
			best = ms
		}
		time.Sleep(100 * time.Millisecond)
	}
	if best == 0 {
		return 0, fmt.Errorf("yandex ping: no response")
	}
	return best, nil
}

func pingOnce() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pingURL, nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := client(pingTimeout).Do(req)
	if err != nil {
		return 0, err
	}
	// Дренируем тело, чтобы keep-alive соединение вернулось в пул.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	return time.Since(start).Milliseconds(), nil
}

// pingRepo — запасной пинг: TTFB тёплого запроса к каталогу репозитория
// Яндекс.Браузера (тот же запасной источник, что у скорости). Крупнее
// ответ /ping (страница каталога), но порядок величины тот же.
func pingRepo() (int64, error) {
	ping := func() (int64, error) {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, RepoListing, nil)
		if err != nil {
			return 0, err
		}
		start := time.Now()
		resp, err := client(pingTimeout).Do(req)
		if err != nil {
			return 0, err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		return time.Since(start).Milliseconds(), nil
	}
	_, _ = ping() // прогрев DNS+TLS, в замер не идёт
	var best int64
	for i := 0; i < pingAttempts; i++ {
		if ms, err := ping(); err == nil && (best == 0 || ms < best) {
			best = ms
		}
		time.Sleep(100 * time.Millisecond)
	}
	if best == 0 {
		return 0, fmt.Errorf("repo ping: no response")
	}
	return best, nil
}

// PingFallback — пинг по CDN Яндекса; CDN недоступен (блокировка/гео) —
// запасной замер по репозиторию, чтобы пинг в отчётах не пропадал совсем.
func PingFallback() (int64, error) {
	if ms, err := Ping(pingAttempts); err == nil {
		return ms, nil
	}
	return pingRepo()
}

// Speed — пропускная способность, Мбит/с, по методике интернетометра:
// speedStreams параллельных ranged-потоков с probes/50mb, время считается
// от первого байта (рукопожатия и TTFB в знаменатель не попадают), а
// начальный разгон канала (slow-start) отсекается — скорость по окну после
// первой секунды. При недоступности CDN — запасной замер по repo.yandex.ru.
func Speed(budget time.Duration) (float64, error) {
	if budget <= 0 {
		budget = speedBudget
	}
	if mbps, err := speedProbe(budget); err == nil {
		return mbps, nil
	}
	return speedRepo(budget)
}

func speedProbe(budget time.Duration) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), budget+6*time.Second)
	defer cancel()

	type streamResult struct {
		bytes int64
		first time.Time
	}
	results := make([]streamResult, speedStreams)
	var total atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	deadline := start.Add(budget)
	timeout := budget + 6*time.Second

	// Периодические снимки суммарного объёма — для отсечки разгона.
	type sample struct {
		at    time.Time
		bytes int64
	}
	var samples []sample
	sampleStop := make(chan struct{})
	sampleDone := make(chan struct{})
	go func() {
		defer close(sampleDone)
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-sampleStop:
				return
			case <-t.C:
				samples = append(samples, sample{at: time.Now(), bytes: total.Load()})
			}
		}
	}()

	for i := 0; i < speedStreams; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
			if err != nil {
				return
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", int64(slot)*8<<20))
			resp, err := client(timeout).Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
				return
			}
			buf := make([]byte, 64<<10)
			first := time.Time{}
			for {
				now := time.Now()
				if now.After(deadline) || total.Load() >= speedMaxBytes {
					break
				}
				n, err := resp.Body.Read(buf)
				if n > 0 {
					if first.IsZero() {
						first = now
					}
					results[slot].bytes += int64(n)
					total.Add(int64(n))
				}
				if err != nil {
					break
				}
			}
			results[slot].first = first
		}(i)
	}
	wg.Wait()
	close(sampleStop)
	<-sampleDone

	var anchor time.Time
	for _, r := range results {
		if !r.first.IsZero() && r.first.After(anchor) {
			anchor = r.first
		}
	}
	downloaded := total.Load()
	if downloaded == 0 || anchor.IsZero() {
		return 0, fmt.Errorf("yandex speedtest: no data")
	}

	// Slow-start TCP занижает среднюю по всей загрузке: скорость считаем по
	// окну от секунды после anchor до конца. Окно короче 1.5с (короткий
	// бюджет) — считаем по-старому.
	end := time.Now()
	t1 := anchor.Add(time.Second)
	if end.Sub(t1) >= 1500*time.Millisecond {
		for _, s := range samples {
			if s.at.Before(t1) {
				continue
			}
			if d := downloaded - s.bytes; d > 0 && end.After(s.at) {
				return float64(d) * 8 / end.Sub(s.at).Seconds() / 1e6, nil
			}
			break
		}
	}
	elapsed := end.Sub(anchor)
	if elapsed <= 0 {
		return 0, fmt.Errorf("yandex speedtest: no data")
	}
	return float64(downloaded) * 8 / elapsed.Seconds() / 1e6, nil
}

// speedRepo — запасной однопоточный замер из репозитория Яндекс.Браузера.
func speedRepo(budget time.Duration) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), budget+4*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, RepoListing, nil)
	resp, err := client(6 * time.Second).Do(req)
	if err != nil {
		return 0, fmt.Errorf("yandex repo listing: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	m := debRe.FindStringSubmatch(string(body))
	if len(m) < 2 {
		return 0, fmt.Errorf("yandex repo: no package found")
	}
	fileURL := RepoListing + m[1]

	dl, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return 0, err
	}
	dl.Header.Set("Range", "bytes=0-")
	start := time.Now()
	dresp, err := client(budget + 6*time.Second).Do(dl)
	if err != nil {
		return 0, fmt.Errorf("yandex download: %w", err)
	}
	defer dresp.Body.Close()
	if dresp.StatusCode != http.StatusPartialContent && dresp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("yandex download: status %d", dresp.StatusCode)
	}

	buf := make([]byte, 64<<10)
	var total int64
	first := time.Time{}
	deadline := start.Add(budget)
	for {
		now := time.Now()
		if now.After(deadline) || total >= speedMaxBytes {
			break
		}
		n, err := dresp.Body.Read(buf)
		if n > 0 && first.IsZero() {
			first = now
		}
		total += int64(n)
		if err != nil {
			break
		}
	}
	anchor := first
	if anchor.IsZero() {
		anchor = start
	}
	elapsed := time.Since(anchor)
	if total == 0 || elapsed <= 0 {
		return 0, fmt.Errorf("yandex download: no data")
	}
	return float64(total) * 8 / elapsed.Seconds() / 1e6, nil
}

// Quick — компактный замер для автоподбора: пинг покороче, скорость
// с урезанным бюджетом.
func Quick() (pingMs int64, speedMbps float64, err error) {
	pingMs, perr := PingFallback()
	speedMbps, serr := Speed(quickSpeedMax)
	if perr != nil && serr != nil {
		return 0, 0, fmt.Errorf("%v; %v", perr, serr)
	}
	return pingMs, speedMbps, nil
}

// Measure — полный снапшот для монитора и отчёта.
func Measure() Result {
	r := Result{At: time.Now()}
	if ms, err := PingFallback(); err == nil {
		r.PingMs = ms
	}
	if mbps, err := Speed(speedBudget); err == nil {
		r.SpeedMbps = mbps
	}
	return r
}
