package app

import (
	"fmt"
	"net"
	"time"

	"zpui/internal/blockcheck"
)

// nxDeadStreak — сколько подряд NXDOMAIN-сэмплов считают домен мёртвым.
const nxDeadStreak = 3

// trackNXDomains ведёт счётчики подряд идущих NXDOMAIN-сэмплов по хостам и
// после nxDeadStreak подряд добавляет хост в skip-resources.txt: домен больше
// не существует. Счётчик сбрасывается,
// когда хост снова доказанно резолвится; сэмплы других типов (таймаут, reset)
// счётчик не меняют. Защита от ложных срабатываний: IP-литералы и уже
// исключённые хосты не учитываются, а добавление происходит только если в этом
// же прогоне хотя бы один другой ресурс успешно резолвится/проходит (сеть в
// целом жива).
func (a *App) trackNXDomains(report *blockcheck.BulkReport) {
	if report == nil {
		return
	}
	// Авто-исключение выключено в настройках (по умолчанию): файл исключений
	// наполняется только руками. Счётчики сбрасываем — после включения
	// отсчёт начинается заново.
	if !a.cfg.GetAutoSkipDeadDomains() {
		a.nxStreakMu.Lock()
		a.nxStreak = nil
		a.nxStreakMu.Unlock()
		return
	}
	all := make([]blockcheck.BulkResult, 0, len(report.Default)+len(report.User))
	all = append(all, report.Default...)
	all = append(all, report.User...)

	alive := false
	for i := range all {
		if nxHostResolved(&all[i]) {
			alive = true
			break
		}
	}

	a.nxStreakMu.Lock()
	if a.nxStreak == nil {
		a.nxStreak = make(map[string]int)
	}
	var dead []string
	for i := range all {
		r := &all[i]
		host := r.Host
		if host == "" || net.ParseIP(host) != nil {
			continue
		}
		if a.excludedHost(host) {
			delete(a.nxStreak, host)
			continue
		}
		if r.DNSNotFound {
			a.nxStreak[host]++
			if alive && a.nxStreak[host] >= nxDeadStreak {
				dead = append(dead, host)
				delete(a.nxStreak, host)
			}
		} else if nxHostResolved(r) {
			delete(a.nxStreak, host)
		}
	}
	a.nxStreakMu.Unlock()

	for _, host := range dead {
		// Причина в комментарии обязательна: список видит юзер, и должно быть
		// понятно, кто и почему добавил домен.
		comment := fmt.Sprintf("auto: исключён автоматически — домен не существует (NXDOMAIN при %d проверках подряд), %s", nxDeadStreak, time.Now().Format("02.01.2006"))
		if err := a.cfg.AddSkipResourceWithComment(host, comment); err != nil {
			a.log.Warn("availability", "auto-exclude "+host+": "+err.Error())
			continue
		}
		a.log.Info("availability", fmt.Sprintf("host %s no longer exists (NXDOMAIN in %d checks in a row) — added to skip-resources.txt", host, nxDeadStreak))
	}
}

// nxHostResolved — хост доказанно резолвится: какой-то слой прошёл либо
// соединение прошло дальше DNS (reset/заглушка/ответ сервера — всё это
// означает, что IP-адрес был получен).
func nxHostResolved(r *blockcheck.BulkResult) bool {
	if r.OK || r.TCP.Ok || r.TLS.Ok || r.HTTP.Ok {
		return true
	}
	switch r.Verdict {
	case blockcheck.VerdictTCPBlock, blockcheck.VerdictTLSBlock,
		blockcheck.VerdictHTTPStub, blockcheck.VerdictDown:
		return true
	}
	return false
}

// collectDeadDomainsBeforeSelect — прогон проверки ресурсов перед автоподбором:
// устойчивые NXDOMAIN попадают в skip-resources.txt, чтобы мёртвые домены не
// занижали балл стратегий.
func (a *App) collectDeadDomainsBeforeSelect() {
	report := a.getResourceStatusForced()
	if report == nil {
		return
	}
	a.trackNXDomains(report)
}
