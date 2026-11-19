package app

// История скорости сети для графика «Трафик» на странице Монитор. Раньше
// график наполнялся с момента открытия страницы (фронтовый буфер) — данные
// копятся в бэкенде с самого старта приложения, как у соседнего графика
// «Интернетметр» (тот берёт историю из базы). Кольцо на час при опросе раз
// в 5 секунд; фронт при открытии забирает буфер целиком и дальше рисует
// живые точки сам.

import (
	"sync"
	"time"

	"zpui/internal/power"
)

const (
	trafficTick     = 5 * time.Second
	trafficKeep     = time.Hour
	trafficCapGuess = int(trafficKeep / trafficTick) // 720
)

type trafficSample struct {
	Ts int64   `json:"ts"`
	Dl float64 `json:"dl"` // B/s
	Ul float64 `json:"ul"` // B/s
}

type trafficRing struct {
	mu      sync.Mutex
	samples []trafficSample
}

var trafficHist trafficRing

func (r *trafficRing) append(s trafficSample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, s)
	cut := time.Now().Add(-trafficKeep).Unix()
	i := 0
	for i < len(r.samples) && r.samples[i].Ts < cut {
		i++
	}
	if i > 0 {
		r.samples = append(r.samples[:0], r.samples[i:]...)
	}
}

func (r *trafficRing) snapshot() []trafficSample {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]trafficSample, len(r.samples))
	copy(out, r.samples)
	return out
}

// trafficHistoryLoop — стартует в startup(): раз в 5 секунд снимает скорости
// из монитора (он и так считает их для /api/status). Живёт до Shutdown.
func (a *App) trafficHistoryLoop() {
	t := time.NewTicker(trafficTick)
	defer t.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-t.C:
			// Экономный режим уровня 2+: монитор отдаёт нули — в ring пишем
			// только живые скорости (иначе нулей больше, чем тиков источника).
			if power.MonitoringPaused() {
				continue
			}
			st := a.monitor.GetCurrentStats()
			trafficHist.append(trafficSample{
				Ts: time.Now().Unix(),
				Dl: st.DownloadSpeed,
				Ul: st.UploadSpeed,
			})
		}
	}
}

// GetTrafficHistory — буфер графика: {samples: [{ts, dl, ul}...]}.
func (a *App) GetTrafficHistory() map[string]interface{} {
	return okRespWithData(map[string]interface{}{
		"samples": trafficHist.snapshot(),
	})
}
