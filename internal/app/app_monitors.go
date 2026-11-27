package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/blockcheck"
	"zpui/internal/database"
	"zpui/internal/isp"
	"zpui/internal/netcheck"
	"zpui/internal/netmetrics"
	"zpui/internal/notify"
	"zpui/internal/power"
	"zpui/internal/zapret"
)

func (a *App) startResourceMonitor() {
	// Первая проверка — после стартового подбора: пока перебор переключает
	// стратегии, замерять рано (данные доступности и стартовый замер
	// Интернетметра легли бы на промежуточную стратегию, а не на итоговую).
	// Ждём завершения подбора; если он не заявлен (запрет пропущен) —
	// минуту на то, чтобы экран запуска дошёл до подбора, и меряем как раньше.
	// Выход — по флагу: закрытый канал остаётся готовым навсегда, и вечный
	// select по нему крутится горячим спином, не давая циклу продолжиться.
	noSelectWait := time.Now().Add(60 * time.Second)
	hardDeadline := time.Now().Add(2 * time.Minute)
	selectDone := false
	for !selectDone {
		select {
		case <-a.stopCh:
			return
		case <-a.startupSelectDone:
			selectDone = true
		case <-time.After(500 * time.Millisecond):
		}
		if selectDone || a.cfg.GetZapretSkipped() || time.Now().After(hardDeadline) {
			break
		}
		if !a.startupSelectRequested.Load() && time.Now().After(noSelectWait) {
			break
		}
	}

	// Проверка доступности в стартовом меню не нужна: стратегию уже проверил
	// стартовый подбор (быстрый путь — прямой замер доступности, полный путь —
	// тест стратегий с замерами). График заполнят плановые такты, гейт
	// готовности закрывается сразу — старт не ждёт лишнего замера.
	a.resInitOnce.Do(func() { close(a.resInitDone) })
	// Здоровье DNS-провайдеров — в том же такте, что и проверка ресурсов.
	go a.startDnsWatchCycle()

	for {
		// В экономном режиме проверка доступности раз в 30 минут
		// (независимо от настройки); при выходе из режима — как настроено.
		interval := time.Duration(a.cfg.GetResourceCheckInterval()) * time.Minute
		if power.Active() {
			interval = 30 * time.Minute
		}
		next := time.Now().Truncate(interval).Add(interval)
		wait := time.Until(next)
		if wait <= 0 {
			wait = interval
		}

		select {
		case <-a.stopCh:
			return
		case <-time.After(wait):
			// Такт на время подбора стратегий откладывается на следующий
			// интервал: параллельный плановый спидтест ломает замеры подбора
			// (и получает искажённые сам). Запись всё равно молчала бы —
			// такт лучше просто отложить.
			if a.selectionRunning.Load() || a.zapret.IsAutoTestRunning() {
				continue
			}
			a.doResourceCheckAndSave()
			go a.startDnsWatchCycle()
		}
	}
}

func (a *App) scheduleResourceCheckAfterStrategyChange() {
	go func() {
		time.Sleep(4 * time.Second)
		// Во время подбора хук молчит даже на промежуточных кандидатах свипа:
		// замеры под плохими стратегиями портят данные доступности. ПКМ и
		// ручная смена selectionRunning не ставят — проходят как раньше.
		if a.selectionRunning.Load() || a.IsCheckingNow() || a.zapret.IsAutoTestRunning() {
			return
		}
		a.doResourceCheckAndSave()
	}()
}

func (a *App) watchStartupReady() {
	go func() {
		deadline := time.After(2 * time.Minute)
		svcDone, resDone := false, false
		for !(svcDone && resDone) {
			select {
			case <-a.svcInitDone:
				svcDone = true
			case <-a.resInitDone:
				resDone = true
			case <-deadline:
				svcDone, resDone = true, true
			}
			a.startupReadyMu.Lock()
			a.startupSvcOK = svcDone
			a.startupResOK = resDone
			a.startupReadyMu.Unlock()
		}
		a.startupReadyMu.Lock()
		a.startupReady = true
		a.startupReadyMu.Unlock()
		close(a.startupReadyCh)
		a.log.Info("startup", "backend fully ready — UI unlocked")
	}()
}

func (a *App) GetStartupReady() map[string]interface{} {
	a.startupReadyMu.Lock()
	ready, svc, res := a.startupReady, a.startupSvcOK, a.startupResOK
	a.startupReadyMu.Unlock()
	return okRespWithData(map[string]interface{}{"ready": ready, "svc": svc, "resources": res})
}

func (a *App) startDnsWatchCycle() {
	a.dnsReconfigure()
	enabled := a.cfg.GetXboxDnsConfig().PlainIPs()
	// Пробуем все известные адреса (включая выключенные провайдеры) —
	// вкладка DNS показывает живость каждого; живость включённых решает,
	// продолжать ли работу.
	h := a.xboxDns.CheckServersOf(a.dnsHealthProbeList(enabled))
	enabledSet := map[string]bool{}
	for _, ip := range enabled {
		enabledSet[strings.ToLower(ip)] = true
	}
	okCount, enabledOK := 0, false
	for _, s := range h.Servers {
		if !s.OK {
			continue
		}
		okCount++
		if enabledSet[strings.ToLower(s.Server)] {
			enabledOK = true
		}
	}
	// итог цикла в логе: сколько адресов ответили (видно и по файлу
	// лога, что проверки идут по расписанию)
	a.log.Info("dns", fmt.Sprintf("health check: %d/%d servers responded", okCount, len(h.Servers)))
	// Включённый источник со всеми мёртвыми адресами (2 цикла подряд)
	// выключается сам.
	a.dnsSourcesAutoOff(h)
	if enabledOK || len(enabled) == 0 {
		a.dnsFailStreak.Store(0)
		return
	}
	// Одна неудача может быть флаком (потеря UDP) — отключаем после двух подряд.
	// Источник истины — флаг конфига: после рестарта DNS может быть
	// прописан в системе, а менеджер в Enable-состоянии не участвовал.
	streak := a.dnsFailStreak.Add(1)
	if !a.cfg.GetXboxDnsConfig().Enabled || streak < 2 {
		return
	}
	a.dnsFailStreak.Store(0)
	a.log.Warn("dns", "DNS servers unreachable, auto-disabling: "+h.FailText())
	if err := a.xboxDns.Disable(); err != nil {
		a.log.Warn("dns", "auto-disable restore: "+err.Error())
	}
	a.persistDnsPrevious()
	if err := a.cfg.SetXboxDnsEnabled(false); err != nil {
		a.log.Warn("dns", "auto-disable flag: "+err.Error())
	}
	if ctx := a.getContext(); ctx != nil {
		runtime.EventsEmit(ctx, "xboxdns:auto_disabled", map[string]any{"reason": h.FailText()})
	}
}

func (a *App) doResourceCheckAndSave() {
	a.checkingNowMu.Lock()
	a.checkingNow = true
	a.checkingNowMu.Unlock()
	emitCheckEvent := func(name string) {
		if ctx := a.getContext(); ctx != nil {
			runtime.EventsEmit(ctx, name, nil)
		}
	}
	emitCheckEvent("resource-check:started")
	defer func() {
		a.checkingNowMu.Lock()
		a.checkingNow = false
		a.checkingNowMu.Unlock()
		emitCheckEvent("resource-check:done")
	}()

	// Замер Интернетметра (скорость+пинг) едет параллельно с проверкой
	// ресурсов: один цикл — один сэмпл и для графика доступности,
	// и для графиков мониторинга. Подбор/автотест — мимо: чужой спидтест
	// искажает их замеры и сам получает искажённые.
	if !a.historyWritesBlocked() {
		go a.measureNetIfDue()
	}

	report := a.getResourceStatusForced()
	if report == nil {
		return
	}

	// Нет сети (пробуждение ноутбука, отвал Wi-Fi, перезагрузка роутера):
	// нулевые сэмплы не по вине стратегии — не пишем их в историю и баллы,
	// автосмену не провоцируем.
	if !netcheck.Online() {
		a.log.Warn("availability", "network offline — замер пропущен, история и автосмена не тронуты")
		return
	}

	all := append(report.Default, report.User...)
	if len(all) == 0 {
		return
	}

	a.persistResourceTick(report, all)
}

// historyWritesBlocked — запись истории доступности глушится на время подбора
// стратегий и автотеста: замер под промежуточным кандидатом или поверх
// переустановки службы не соответствует никакой реальной стратегии —
// такие такты портят график, баллы и живой балл активной стратегии.
func (a *App) historyWritesBlocked() bool {
	return a.selectionRunning.Load() || a.zapret.IsAutoTestRunning()
}

// persistResourceTick — единственная точка записи такта доступности:
// resource_availability + availability_history (по группам standard/user),
// детект мёртвых доменов и хуки (автосмена, живой балл). Вызов во время
// подбора (ручной refresh, хвост подбора) доходит сюда и молча выходит —
// отчёт остаётся в кэше UI, история и лог не пишутся.
func (a *App) persistResourceTick(report *blockcheck.BulkReport, all []blockcheck.BulkResult) {
	if a.historyWritesBlocked() {
		a.log.Info("availability", "подбор/автотест идёт — такт не пишется в историю (только кэш UI)")
		return
	}

	// Детекция мёртвых доменов: устойчивый NXDOMAIN → авто-исключение.
	a.trackNXDomains(report)

	var operatorKey string
	if op, _ := database.GetCurrentOperator(); op != nil {
		operatorKey = op.OperatorKey
	}
	now := time.Now()

	saveSet := func(typ string, res []blockcheck.BulkResult) {
		if len(res) == 0 {
			return
		}
		oks := 0
		var newlyFailed, recovered []string
		failedCount := 0
		for _, r := range res {
			key := typ + ":" + r.Name
			a.prevResourceStateMu.Lock()
			prevOK, wasKnown := a.prevResourceState[key]
			a.prevResourceStateMu.Unlock()

			if r.OK {
				oks++
				if wasKnown && !prevOK {
					recovered = append(recovered, r.Name)
				}
			} else {
				failedCount++
				if !wasKnown || prevOK {
					newlyFailed = append(newlyFailed, r.Name)
				}
			}

			a.prevResourceStateMu.Lock()
			a.prevResourceState[key] = r.OK
			a.prevResourceStateMu.Unlock()

			if err := database.InsertResourceAvailability(&database.ResourceAvailability{
				Timestamp:   now,
				OperatorKey: operatorKey,
				Host:        r.Name,
				Type:        typ,
				Ok:          r.OK,
				Verdict:     r.Verdict,
				LatencyMs:   int(r.LatencyMs),
			}); err != nil {
				a.log.Warn("availability", "resource insert failed: "+err.Error())
			}
		}
		pct := 0
		if len(res) > 0 {
			pct = oks * 100 / len(res)
		}
		// Тот же now, что у построчных записей тика: серия графика «основные»
		// агрегируется по времени построчных — разные метки одного тика
		// разъезжались по корзинам шкалы, и линия обрывалась не доходя до края.
		database.InsertAvailabilitySnapshot(&database.AvailabilityRecord{
			Timestamp:      now,
			Type:           typ,
			TotalResources: len(res),
			OKResources:    oks,
			Pct:            float64(pct),
		})
		// Only log summary of state changes (not per-resource)
		if len(newlyFailed) > 0 || len(recovered) > 0 {
			a.log.Info("availability", fmt.Sprintf("[%s] check: %d/%d ok (%d%%) — changed: -%d/+%d", typ, oks, len(res), pct, len(newlyFailed), len(recovered)))
		}
	}
	saveSet("standard", report.Default)
	saveSet("user", report.User)

	// Автосмена по доступности: 2 такта подряд ниже порога — смена на лучшую.
	autoOK := 0
	keyOK, keyTotal := 0, 0
	keySeen := make(map[string]bool, 8)
	for _, r := range all {
		if r.OK {
			autoOK++
		}
		// Ключевые — цели листов, чей хост матчится с основными ресурсами;
		// дедуп по хосту: запись листа и её каноничная строка не считаются
		// дважды.
		if zapret.IsKeyTarget(r.Name, r.URL) {
			if h := zapret.KeyTargetHost(r.Name, r.URL); !keySeen[h] {
				keySeen[h] = true
				keyTotal++
				if r.OK {
					keyOK++
				}
			}
		}
	}
	a.autoSwitchTick(autoOK, len(all))
	a.liveScoreTick(autoOK, len(all), keyOK, keyTotal)
}

// liveScoreTick — хук после такта проверки ресурсов: живой балл активной
// стратегии в operator_strategies (панель стратегий видит текущую доступность,
// а не замер последнего прогона автоподбора). Гард как у autoSwitchTick:
// движок работает и не занят подбором/проверкой стратегий — при переборе
// замер ложился бы на промежуточные стратегии.
func (a *App) liveScoreTick(ok, total, keyOk, keyTotal int) {
	if total <= 0 {
		return
	}
	if a.selectionRunning.Load() || a.zapret.IsAutoTestRunning() || a.zapret.GetStatus() != zapret.StatusRunning {
		return
	}
	strategy := a.cfg.GetCurrentStrategy()
	if strategy == "" {
		return
	}
	if err := a.zapret.UpdateLiveScore(strategy, ok, total, keyOk, keyTotal); err != nil {
		a.log.Warn("availability", "live score: "+err.Error())
	}
}

func (a *App) getResourceStatusForced() *blockcheck.BulkReport {
	def, user := blockcheck.ReadCheckTargets(a.cfg.GetZapretPath(), a.cfg.ListsDir())
	def = blockcheck.FilterCheckTargets(def, a.cfg.IsExcludedHost)
	user = blockcheck.FilterCheckTargets(user, a.cfg.IsExcludedHost)

	bc := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc.CheckTCP, bc.CheckTLS, bc.CheckHTTP, bc.TimeoutSec)
	report := checker.BulkCheck(def, user)

	now := time.Now()
	a.resourceCacheMu.Lock()
	a.resourceCache = report
	a.resourceCacheTime = now
	a.resourceCacheMu.Unlock()

	return report
}

func (a *App) IsCheckingNow() bool {
	a.checkingNowMu.Lock()
	defer a.checkingNowMu.Unlock()
	return a.checkingNow
}

func (a *App) checkAndRecoverZapret() bool {
	if devNoZapretAutostart {
		return false
	}
	data, err := database.GetZapretBackup()
	if err != nil || data == "" {
		return false
	}

	svc := a.zapret.GetServiceStatus()
	if svc.Running {
		// zapret работает — прошлое обновление завершилось нормально
		database.DeleteZapretBackup()
		return false
	}

	a.log.Warn("app", "Обнаружено прерванное обновление zapret — восстановление состояния")
	var snap zapret.BackupSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		a.log.Error("app", "Чтение backup не удалось: "+err.Error())
		database.DeleteZapretBackup()
		return false
	}

	a.safeGo(func() {
		time.Sleep(1 * time.Second)
		a.zapret.RestoreState(&snap)
		database.DeleteZapretBackup()
		a.log.Info("app", "Состояние zapret восстановлено после прерванного обновления")
	})
	return true
}

func (a *App) startTrafficSnapshots() {
	for {
		interval := power.Stretch(5 * time.Second)
		select {
		case <-time.After(interval):
			// Экономный режим уровня 2+: в БД не пишем, на восстановление не влияет.
			if power.MonitoringPaused() {
				continue
			}
			stats := a.monitor.GetCurrentStats()
			a.saveTrafficSnapshot(
				stats.DownloadSpeed,
				stats.UploadSpeed,
				int64(stats.DownloadBytes),
				int64(stats.UploadBytes),
				len(a.tgproxy.ClientsByIP()),
			)
		case <-a.stopCh:
			return
		}
	}
}

// netMeasure — точка подмены в тестах (в тестах сети нет).
var netMeasure = netmetrics.Measure

func (a *App) measureAndStoreNet() {
	m := netMeasure()
	if m.PingMs > 0 || m.SpeedMbps > 0 {
		if err := database.InsertYandexMetric(m.PingMs, m.SpeedMbps); err != nil {
			a.log.Warn("netmetrics", "save: "+err.Error())
		}
		if err := database.CleanOldYandexMetrics(time.Hour); err != nil {
			a.log.Warn("netmetrics", "prune: "+err.Error())
		}
		a.log.Info("netmetrics", fmt.Sprintf("netmeter: ping %d ms, speed %.1f Mbps", m.PingMs, m.SpeedMbps))
	} else {
		a.log.Warn("netmetrics", "measure failed: no data (ping and speed both zero)")
	}
}

func (a *App) measureNetIfDue() {
	// Замер гоняет спидтест по CDN Яндекса — в глубокой паузе это лишняя
	// сетевая активность, пропускаем (данные за весь период паузы оседают
	// без сэмплов, это штатно для уровня 2+).
	if power.MonitoringPaused() {
		return
	}
	if latest, err := database.GetLatestYandexMetric(); err == nil && latest != nil {
		if time.Since(latest.Timestamp) < 2*time.Minute {
			return
		}
	}
	a.measureAndStoreNet()
}

func (a *App) startDataRotation() {
	cleanOldSnapshots(time.Hour)
	cleanOldConnections(7 * 24 * time.Hour)
	database.CleanOldAvailability(7 * 24 * time.Hour)
	database.CleanOldResourceAvailability(7 * 24 * time.Hour)
	database.CleanOldAppMetrics(7 * 24 * time.Hour)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if err := database.RollupResourceDaily(yesterday); err != nil {
		a.log.Warn("availability", "daily rollup failed: "+err.Error())
	}

	ticker := time.NewTicker(power.Stretch(1 * time.Hour))
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ticker.Reset(power.Stretch(1 * time.Hour))
			// Косметическая уборка: в глубокой паузе новых данных почти не
			// пишется — пропускаем, чистка догонит после выхода из режима.
			if power.MonitoringPaused() {
				continue
			}
			cleanOldSnapshots(time.Hour)
			cleanOldConnections(7 * 24 * time.Hour)
			database.CleanOldAvailability(7 * 24 * time.Hour)
			database.CleanOldResourceAvailability(7 * 24 * time.Hour)
			database.CleanOldYandexMetrics(time.Hour)

			y := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
			if err := database.RollupResourceDaily(y); err != nil {
				a.log.Warn("availability", "daily rollup failed: "+err.Error())
			}
		case <-a.stopCh:
			return
		}
	}
}

// defaultSkipContent is the default content for skip-resources.txt.
// Used when the file is missing and needs to be created.
const defaultSkipContent = `# skip-resources.txt - resources excluded from availability checks.
#
# These domains are always down (dead CDN, retired subdomains, etc) so
# there is no point in checking them. Edit this file manually to add/remove.
# One host per line. Lines starting with # are comments. Blank lines ignored.
# Subdomains are matched automatically: "google.com" excludes "drive.google.com".

# Cloudflare service/test domains (always unavailable)
cloudflareapps.com
cloudflarebolt.com
cloudflareclient.com
cloudflarepartners.com
cloudflareresolve.com
cloudflaressl.com
cloudflarestatus.com
cloudflarestorage.com
cloudflaretest.com

# Cloudfront CDN
cloudfront.net

# Discord service subdomains
discord.dev
discord.media
discord.status
discord-activities.com
discordactivities.com
discordapp.net
discordpartygames.com

# Other service/unavailable
localizeapi.com
live-video.net

# PornHub CDN - always unavailable in RU
phncdn.com
pix-cdn77.phncdn.com
winhanced.com
`

func (a *App) ensureSkipResourcesFile() {
	path := a.cfg.GetSkipResourcesFilePath()
	if _, err := os.Stat(path); err == nil {
		return // file exists, do not overwrite
	}
	if err := os.WriteFile(path, []byte(defaultSkipContent), 0644); err != nil {
		a.log.Warn("app", "Failed to create skip-resources.txt: "+err.Error())
		return
	}
	a.log.Info("app", "Created skip-resources.txt with default exclusions")
}

// defaultSkipSystemContent — вшитый дефолт системного skip-файла: только
// шапка, домены приходят с сервера (state.sync) и целиком заменяют файл.
const defaultSkipSystemContent = `# skip-resources-system.txt — системный список пропускаемых ресурсов, обновляется с сервера ZPUI.
# Редактирование не предусмотрено: файл целиком заменяется при смене версии набора с сервера.
# Формат как у skip-resources.txt: один домен на строку, строки с # — комментарии.
`

func (a *App) ensureSkipResourcesSystemFile() {
	path := a.cfg.GetSkipResourcesSystemFilePath()
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := os.WriteFile(path, []byte(defaultSkipSystemContent), 0644); err != nil {
		a.log.Warn("app", "Failed to create skip-resources-system.txt: "+err.Error())
		return
	}
	a.log.Info("app", "Created skip-resources-system.txt (header-only default)")
}

func (a *App) startZapretHealthMonitor() {
	time.Sleep(5 * time.Second)

	for {
		select {
		case <-a.stopCh:
			return
		case <-time.After(power.Stretch(5 * time.Second)):
		}
		running := a.zapret.GetStatus() == zapret.StatusRunning
		if running {
			// Движок работает — он «должен работать» и дальше.
			a.zapretExpected.Store(true)
			continue
		}
		// Упал, пока должен работать (пользователь его не останавливал) —
		// восстанавливаем. После неудачной попытки даём паузу 60 секунд.
		// Во время проверки/подбора стратегий служба пересоздаётся — не мешаем.
		// Как и во время установки обновления запрета: остановка штатная.
		if a.selectionRunning.Load() || a.zapret.IsAutoTestRunning() || a.zapret.IsUpdating() || installActive.Load() {
			continue
		}
		if !a.zapretExpected.Load() || a.zapret.StopRequested() || a.isRecoveryRunning() {
			continue
		}
		if fail := a.lastRecoveryFail.Load(); fail > 0 && time.Since(time.Unix(fail, 0)) < 60*time.Second {
			continue
		}
		a.log.Warn("zapret", "health monitor: engine down while expected running — recovering")
		a.startZapretRecovery("service_stopped_health")
	}
}

func (a *App) startISPMonitor() {
	a.ispMonitor = isp.NewMonitor(a.log.Info, 60*time.Second, func(change isp.OperatorChange) {
		if change.OldKey != "" {
			ev := map[string]interface{}{
				"old_key":  change.OldKey,
				"old_name": change.OldName,
				"new_key":  change.NewKey,
				"new_name": change.NewName,
			}
			if change.Op != nil {
				ev["asn"] = change.Op.ASN
			}
			runtime.EventsEmit(a.ctx, "operator:changed", ev)
		}

		saved, _ := database.GetOperatorStrategy(change.NewKey)
		// Старые записи БД (эра zapret2) хранят имена с припиской «ZP2 »:
		// движку нужен чистый имя файла, иначе автосмена падает на stat.
		saved = strings.TrimPrefix(saved, "ZP2 ")

		// Движок остановлен пользователем вручную: автосмена стратегии и
		// авто-подбор не запускаем — они переустанавливают/запускают службу
		// и молча поднимали остановленный движок. Только фиксируем оператора.
		if !a.zapretExpected.Load() {
			a.log.Info("isp", "engine stopped manually — recording operator without strategy switch")
			if change.OldKey != "" {
				database.SetCurrentOperator(change.NewKey, change.NewName, saved)
			}
			a.syncStrategiesForOperator()
			return
		}

		if saved != "" && !a.zapret.IsAutoTestRunning() && !a.selectionRunning.Load() {
			if saved == a.cfg.GetCurrentStrategy() {
				// Для «нового» оператора сохранена та же стратегия, что уже
				// работает: внешние сервисы часто называют одну и ту же сеть
				// по-разному, и смена «оператора» без смены стратегии
				// переустанавливала службу — движок пропадал на 5–15 секунд.
				// Не дёргаем службу, только фиксируем оператора.
				a.log.Info("isp", "operator changed but strategy is the same ("+saved+") — keeping service")
				database.SetCurrentOperator(change.NewKey, change.NewName, saved)
			} else {
				// Во время проверки/подбора стратегий переключение запрещено.
				a.log.Info("isp", "auto-switching strategy for new operator: "+saved)
				if err := a.zapret.SetStrategy(saved); err != nil {
					a.log.Error("isp", "auto-switch strategy failed: "+err.Error())
				} else {
					database.SetCurrentOperator(change.NewKey, change.NewName, saved)
					runtime.EventsEmit(a.ctx, "strategy:changed", map[string]interface{}{
						"strategy": saved,
						"operator": change.NewName,
					})
					a.operatorChangedNotify(change.NewKey, "operator_changed_applied", change.NewName, strings.TrimSuffix(saved, ".bat"))
				}
			}
		} else if change.OldKey != "" {
			a.log.Info("isp", "no saved strategy for operator: "+change.NewName+", auto-select needed")
			runtime.EventsEmit(a.ctx, "operator:auto_select_needed", map[string]interface{}{
				"operator_key":  change.NewKey,
				"operator_name": change.NewName,
			})
			a.startOperatorAutoSelect(change.NewKey, change.NewName)
		}

		a.syncStrategiesForOperator()
	})
	a.supervisedGo("isp", a.ispMonitor.Start)
}

// operatorAutoSelectTTL — анти-дребезг автоматических реакций на смену
// оператора (toast, фоновый подбор): один прогон на ключ оператора за TTL,
// пограничные сети могут мигать между операторами на каждом 60-секундном такте.
const operatorAutoSelectTTL = 30 * time.Minute

// opEventGate — троттл «один прогон на ключ за TTL».
type opEventGate struct {
	mu  sync.Mutex
	key string
	at  time.Time
}

func (g *opEventGate) allow(key string, ttl time.Duration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.key == key && time.Since(g.at) < ttl {
		return false
	}
	g.key, g.at = key, time.Now()
	return true
}

// operatorChangedNotify — toast о смене оператора: не чаще TTL на ключ и
// только при включённых уведомлениях.
func (a *App) operatorChangedNotify(operatorKey, trKey string, trArgs ...interface{}) {
	if !a.opNotifyGate.allow(operatorKey, operatorAutoSelectTTL) {
		return
	}
	if !a.cfg.GetNotificationsEnabled() {
		return
	}
	notify.Show("ZPUI · "+a.trCur("operator_changed_title"), a.trCur(trKey, trArgs...))
}

// startOperatorAutoSelect — фоновый умный подбор при смене оператора без
// сохранённой стратегии: тот же RunAutoSelectStream(mode=smart), что и
// ручной запуск, — дашборд и страница стратегий видят прогресс через штатные
// autoselect:event / autoselect:done. Гварды: идущий прогон и троттл на ключ
// оператора (TTL 30 мин) — повторные запуски не дублируются.
func (a *App) startOperatorAutoSelect(operatorKey, operatorName string) {
	if a.zapret.IsAutoTestRunning() || a.selectionRunning.Load() {
		a.log.Info("isp", "auto-select for "+operatorName+" skipped: selection already running")
		return
	}
	if !a.opAutoSelGate.allow(operatorKey, operatorAutoSelectTTL) {
		a.log.Info("isp", "auto-select for "+operatorName+" skipped: already ran for this operator recently")
		return
	}
	a.log.Info("isp", "starting background auto-select for new operator: "+operatorName)
	runtime.EventsEmit(a.ctx, "operator:auto_select_started", map[string]interface{}{
		"operator_key":  operatorKey,
		"operator_name": operatorName,
	})
	a.operatorChangedNotify(operatorKey, "operator_changed_autoselect", operatorName)
	a.RunAutoSelectStream("smart", "", false, false, "")
}

// strategiesSyncMu не даёт запустить синхронизацию стратегий параллельно:
// она вызывается и на старте, и из колбэка смены оператора, и двойной прогон
// создавал лишнюю конкуренцию за запись в базу.
var strategiesSyncMu sync.Mutex

func (a *App) syncStrategiesForOperator() {
	strategiesSyncMu.Lock()
	defer strategiesSyncMu.Unlock()

	op, err := database.GetCurrentOperator()
	if err != nil || op == nil || op.OperatorKey == "" {
		a.log.Info("strategies", "operator not yet detected, skipping strategy sync")
		return
	}

	strategies := a.zapret.ListStrategies()
	if len(strategies) == 0 {
		a.log.Info("strategies", "no strategies found in folder, skipping sync")
		return
	}

	filenames := make([]string, len(strategies))
	displayNames := make([]string, len(strategies))
	for i, s := range strategies {
		filenames[i] = s.Filename
		displayNames[i] = s.Name
	}
	// Одна транзакция вместо записи на каждую стратегию: 37 отдельных
	// записей на старте изредка ловили SQLITE_BUSY (конкуренция с ротацией
	// данных и мониторами).
	if err := database.EnsureOperatorStrategiesBatch(op.OperatorKey, filenames, displayNames); err != nil {
		a.log.Error("strategies", "sync strategies batch: "+err.Error())
	}

	current := a.cfg.GetCurrentStrategy()
	if current != "" {
		// Наблюдение, а не применение: пометка applied_at/use_count на старте
		// делала базу оператора «непустой» и полный перебор на чистой системе
		// не запускался вовсе.
		if err := database.MarkObservedStrategy(op.OperatorKey, current); err != nil {
			a.log.Error("strategies", "mark observed "+current+": "+err.Error())
		}
	}

	a.log.Info("strategies", fmt.Sprintf("synced %d strategies for operator %s", len(strategies), op.OperatorKey))
}

func (a *App) checkZapretUpdateFlag() {
	if database.GetZapretJustUpdated() {
		database.SetZapretJustUpdated(false)
		op, _ := database.GetCurrentOperator()
		opName := ""
		if op != nil {
			opName = op.OperatorName
		}
		a.log.Info("app", "zapret was just updated — triggering auto-select modal")
		runtime.EventsEmit(a.ctx, "zapret:needs_autoselect", map[string]interface{}{
			"operator_name": opName,
		})
	}
}

// ── Реализация tray.Controller ──
