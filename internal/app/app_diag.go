package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/xboxdns"
	"zpui/internal/zsvc"
)

// ============================================================
// DIAGNOSTICS
// ============================================================

// diagResult — результат одной проверки диагностики.
type diagResult struct {
	Status string `json:"status"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// RunDiagnostics — полная диагностика системы (замена GET /api/zapret/diagnostics).
func (a *App) RunDiagnostics() map[string]interface{} {
	results := map[string]interface{}{}

	// Служба ZPUI (watchdog): установлена и отвечает ли по IPC.
	results["zpui_service"] = a.checkZpuiService()
	results["bfe_service"] = a.checkService("BFE", "Base Filtering Engine")
	results["zapret_service"] = a.checkService("zapret", "Служба Zapret")
	results["windivert"] = checkWinDivert(a.cfg.GetZapretPath())
	results["winws_process"] = a.checkProcess("winws.exe", "winws.exe (Zapret)")
	results["tcp_timestamps"] = checkTCPTimestamps()
	results["firewall"] = checkFirewallRule()
	results["system_proxy"] = a.checkSystemProxy()
	results["conflicting"] = a.checkConflictingServices()
	results["killer"] = a.checkServiceList("Killer", "Killer Network Service")
	results["intel"] = a.checkIntelConnectivity()
	results["checkpoint"] = a.checkCheckPoint()
	results["smartbyte"] = a.checkServiceList("SmartByte", "SmartByte")
	// Adguard — источник конфликта: не запущен = хорошо (иначе подсказка
	// «поставь на паузу» при выключенном Adguard сбивала с толку).
	results["adguard"] = a.checkAdguard()
	results["vpn"] = a.checkVPN()
	results["dns"] = a.checkDNS()
	results["hosts_file"] = checkHostsFile()
	results["proxy"] = checkProxy(a)
	results["cache_discord"] = checkCacheDiscord()
	// «Кеш DNS» и «Кеш сети» — один системный кеш резолвера (flushdns):
	// в списке проблем должна быть одна строка, а не дубль.
	results["cache_dns"] = checkCacheDNSState("Кеш DNS")

	return results
}

// checkCacheDiscord — заполненность и размер кеша приложения Discord:
// переполненный кеш часто отдаёт мёртвые адреса и мешает переподключению.
func checkCacheDiscord() diagResult {
	base, err := os.UserConfigDir()
	if err != nil {
		return diagResult{Status: "ok", Label: "Кеш Discord", Detail: "Пусто"}
	}
	var total int64
	found := false
	for _, sub := range []string{"Cache", "Code Cache", "GPUCache"} {
		dir := filepath.Join(base, "discord", sub)
		if st, serr := os.Stat(dir); serr == nil && st.IsDir() {
			found = true
			total += dirSize(dir)
		}
	}
	if !found {
		return diagResult{Status: "ok", Label: "Кеш Discord", Detail: "Пусто"}
	}
	return diagResult{Status: "warn", Label: "Кеш Discord", Detail: "Заполнен, " + fmtBytesRu(total) + " — стоит очистить"}
}

// displayDNSCacheRaw — вывод ipconfig /displaydns (пуст — кеш чист).
func displayDNSCacheRaw() (string, bool) {
	out, err := executil.HiddenCmd("ipconfig", "/displaydns").Output()
	if err != nil {
		return "", false
	}
	body := strings.TrimSpace(string(out))
	if strings.Contains(body, "Could not display") || strings.Contains(body, "не удалось отобразить") {
		return "", false
	}
	return body, len(body) > 0
}

// checkCacheDNSState — кеш резолвера Windows; для панелей «Кеш DNS» и
// «Кеш сети» (чистятся одной командой flushdns).
func checkCacheDNSState(label string) diagResult {
	body, filled := displayDNSCacheRaw()
	if !filled {
		return diagResult{Status: "ok", Label: label, Detail: "Пусто"}
	}
	return diagResult{Status: "warn", Label: label, Detail: "Заполнен, " + fmtBytesRu(int64(len(body))) + " — стоит очистить"}
}

// dirSize — суммарный размер файлов каталога (рекурсивно, best-effort).
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, ierr := d.Info(); ierr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// fmtBytesRu — человекочитаемый размер для деталей диагностики.
func fmtBytesRu(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f КБ", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d Б", b)
	}
}

// checkZpuiService — служба ZPUI: установлена ли и отвечает ли по IPC.
func (a *App) checkZpuiService() diagResult {
	installed, err := zsvc.IsInstalled()
	if err != nil || !installed {
		return diagResult{Status: "warn", Label: "Служба ZPUI", Detail: "Не установлена"}
	}
	if a.svc == nil {
		a.svc = zsvc.NewClient()
	}
	ctx, cancel := context.WithTimeout(context.Background(), svcPingTimeout)
	defer cancel()
	version, pingErr := a.svc.Ping(ctx)
	if pingErr == nil {
		detail := "Работает"
		if version != "" {
			detail += " (v" + version + ")"
		}
		return diagResult{Status: "ok", Label: "Служба ZPUI", Detail: detail}
	}
	return diagResult{Status: "warn", Label: "Служба ZPUI", Detail: "Установлена, но не отвечает"}
}

// dnsCacheFilled — заполнен ли кеш резолвера (для верификации очистки).
func dnsCacheFilled() bool {
	_, filled := displayDNSCacheRaw()
	return filled
}

// discordCacheLeft — остались ли папки кеша Discord после очистки.
func discordCacheLeft() (string, bool) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	for _, sub := range []string{"Cache", "Code Cache", "GPUCache"} {
		dir := filepath.Join(base, "discord", sub)
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir, true
		}
	}
	return "", false
}

// ClearZapretCache — очистка кешей, которые портят работу запрета.
// target: discord | network | dns | all. Ответ: cleared — ключи того, что
// реально чистилось, lines — по строке на каждый метод с результатом
// верификации (фронт показывает списком, пока открыта панель).

// ============================================================
// CACHE PANEL (отдельная модалка «Кеш»)
// ============================================================

// cacheInfoItem — состояние одного кеша для модалки «Кеш».
type cacheInfoItem struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Bytes  int64  `json:"bytes"`
}

// discordCacheWarnBytes — порог «переполнен» для жёлтого значка в сайдбаре.
const discordCacheWarnBytes = int64(100 << 20)

// webviewCacheCandidates — где может лежать кеш WebView2 (только страничный
// кеш: Cache/Code Cache/GPUCache — куки и логины не трогаются).
func webviewCacheCandidates() []string {
	out := []string{}
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "WebView2"))
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		out = append(out, filepath.Join(la, "ZPUI", "EBWebView"))
		out = append(out, filepath.Join(la, "ZPUI", "WebView2"))
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		out = append(out, filepath.Join(ad, "ZPUI", "WebView2"))
	}
	return out
}

// webviewCacheDir — найденный корень кеша WebView2 (ok=false — не нашли).
func webviewCacheDir() (string, bool) {
	for _, root := range webviewCacheCandidates() {
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			return root, true
		}
	}
	return "", false
}

// GetCacheInfo — состояние трёх кешей для модалки «Кеш»: сеть (резолвер),
// Discord (папки), WebView2 (страничный кеш).
func (a *App) GetCacheInfo() map[string]interface{} {
	items := make([]cacheInfoItem, 0, 3)

	if body, filled := displayDNSCacheRaw(); filled {
		items = append(items, cacheInfoItem{Key: "network", Label: "Кеш сети", Status: "warn",
			Detail: "Заполнен, " + fmtBytesRu(int64(len(body))), Bytes: int64(len(body))})
	} else {
		items = append(items, cacheInfoItem{Key: "network", Label: "Кеш сети", Status: "ok", Detail: "Пусто"})
	}

	base, err := os.UserConfigDir()
	discordBytes := int64(0)
	if err == nil {
		for _, sub := range []string{"Cache", "Code Cache", "GPUCache"} {
			discordBytes += dirSize(filepath.Join(base, "discord", sub))
		}
	}
	if discordBytes > 0 {
		items = append(items, cacheInfoItem{Key: "discord", Label: "Кеш Discord", Status: "warn",
			Detail: "Заполнен, " + fmtBytesRu(discordBytes), Bytes: discordBytes})
	} else {
		items = append(items, cacheInfoItem{Key: "discord", Label: "Кеш Discord", Status: "ok", Detail: "Пусто"})
	}

	wvBytes := int64(0)
	if root, ok := webviewCacheDir(); ok {
		wvBytes = dirSize(root)
	}
	if wvBytes > 0 {
		items = append(items, cacheInfoItem{Key: "webview", Label: "Кеш WebView2", Status: "warn",
			Detail: "Заполнен, " + fmtBytesRu(wvBytes), Bytes: wvBytes})
	} else {
		items = append(items, cacheInfoItem{Key: "webview", Label: "Кеш WebView2", Status: "ok", Detail: "Пусто"})
	}

	return okRespWithData(map[string]interface{}{
		"items":              items,
		"discord_bytes":      discordBytes,
		"discord_warn_bytes": discordCacheWarnBytes,
	})
}

// clearWebViewCache — удаляет страничный кеш WebView2 (Cache/Code Cache/
// GPUCache); возвращает число очищенных каталогов.
func clearWebViewCache() int {
	root, ok := webviewCacheDir()
	if !ok {
		return 0
	}
	n := 0
	for _, sub := range []string{
		filepath.Join("EBWebView", "Default", "Cache"),
		filepath.Join("EBWebView", "Default", "Code Cache"),
		filepath.Join("EBWebView", "Default", "GPUCache"),
		"Cache", "Code Cache", "GPUCache",
	} {
		dir := filepath.Join(root, sub)
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			if os.RemoveAll(dir) == nil {
				n++
			}
		}
	}
	return n
}

// criticalDiagChecks — проверки, чей не-ok статус реально ломает работу
// запрета. По ним автодиагностика стартового экрана предупреждает; прочие
// предупреждения (warn) молча записываются и нигде не всплывают.
var criticalDiagChecks = map[string]bool{
	"zapret_service": true,
	"winws_process":  true,
	"windivert":      true,
	"conflicting":    true,
	"vpn":            true,
	"killer":         true,
	"intel":          true,
	"checkpoint":     true,
	"smartbyte":      true,
	"adguard":        true,
}

// RunStartupDiagnostics — автодиагностика стартового экрана (после установки
// запрета и статуса «работает»): полный прогон, сохранение последнего
// результата (уходит в отчёт) и отбор критичных проблем.
func (a *App) RunStartupDiagnostics() map[string]interface{} {
	results := a.RunDiagnostics()
	a.storeLastDiagnostics(results)
	critical := make([]map[string]interface{}, 0, 4)
	for key, v := range results {
		r, ok := v.(diagResult)
		if !ok || !criticalDiagChecks[key] || r.Status == "ok" {
			continue
		}
		critical = append(critical, map[string]interface{}{
			"key":    key,
			"status": r.Status,
			"label":  r.Label,
			"detail": r.Detail,
		})
	}
	return okRespWithData(map[string]interface{}{
		"critical":     critical,
		"has_critical": len(critical) > 0,
	})
}

func (a *App) storeLastDiagnostics(results map[string]interface{}) {
	raw, err := json.Marshal(results)
	if err != nil {
		return
	}
	_ = database.SetFlag("last_diagnostics", string(raw))
	_ = database.SetFlag("last_diagnostics_at", time.Now().Format(time.RFC3339))
}

// lastDiagnosticsForReport — сохранённый результат последней диагностики для
// отчёта; нет или старше 10 минут — свежий прогон (отчёт при запуске должен
// нести актуальную диагностику).
func (a *App) lastDiagnosticsForReport() map[string]interface{} {
	if raw, err := database.GetFlag("last_diagnostics"); err == nil && raw != "" {
		at, _ := database.GetFlag("last_diagnostics_at")
		if ts, perr := time.Parse(time.RFC3339, at); perr == nil && time.Since(ts) < 10*time.Minute {
			var checks map[string]interface{}
			if json.Unmarshal([]byte(raw), &checks) == nil && len(checks) > 0 {
				return map[string]interface{}{"checked_at": at, "checks": checks}
			}
		}
	}
	results := a.RunDiagnostics()
	a.storeLastDiagnostics(results)
	at, _ := database.GetFlag("last_diagnostics_at")
	return map[string]interface{}{"checked_at": at, "checks": results}
}

func (a *App) ClearZapretCache(target string) map[string]interface{} {
	var cleared []string
	var lines []string
	add := func(key, detail string) {
		lines = append(lines, key+": "+detail)
	}

	if target == "discord" || target == "all" {
		n, err := clearDiscordCache()
		switch {
		case err != nil:
			add("Discord", "ошибка очистки: "+err.Error())
		case n > 0:
			if _, still := discordCacheLeft(); still {
				add("Discord", fmt.Sprintf("очищено папок: %d, но кеш остался — проверить вручную", n))
			} else {
				add("Discord", fmt.Sprintf("очищено папок кеша: %d — проверено, кеш пуст", n))
				cleared = append(cleared, "discord")
			}
		default:
			add("Discord", "кеш не найден — чистить нечего")
		}
	}
	if target == "webview" || target == "all" {
		if n := clearWebViewCache(); n > 0 {
			add("WebView2", fmt.Sprintf("очищено каталогов кеша: %d", n))
			cleared = append(cleared, "webview")
		} else {
			add("WebView2", "кеш не найден — чистить нечего")
		}
	}
	if target == "dns" || target == "network" || target == "all" {
		if err := executil.HiddenCmd("ipconfig", "/flushdns").Run(); err != nil {
			add("DNS-кеш Windows", "не удалось очистить: "+err.Error())
		} else if dnsCacheFilled() {
			add("DNS-кеш Windows", "очищен, но кеш снова заполнен (активные запросы) — не критично")
			if target == "dns" {
				cleared = append(cleared, "dns")
			} else {
				cleared = append(cleared, "network")
			}
		} else {
			add("DNS-кеш Windows", "очищен — проверено, кеш пуст")
			if target == "dns" {
				cleared = append(cleared, "dns")
			} else {
				cleared = append(cleared, "network")
			}
		}
	}
	database.InsertActionLog(&database.ActionLog{
		Category: "diagnostics",
		Action:   "cache_clear",
		Details:  "Очистка кеша (" + target + "): " + strings.Join(lines, "; "),
	})
	return okRespWithData(map[string]interface{}{"cleared": cleared, "lines": lines})
}

// clearDiscordCache — удаляет папки кеша Discord; возвращает число удалённых
// папок и последнюю ошибку (если удались все — nil).
func clearDiscordCache() (int, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return 0, err
	}
	n := 0
	var lastErr error
	for _, sub := range []string{"Cache", "Code Cache", "GPUCache"} {
		dir := filepath.Join(base, "discord", sub)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			lastErr = err
			continue
		}
		n++
	}
	return n, lastErr
}

func (a *App) checkService(name, label string) diagResult {
	cmd := executil.HiddenCmd("sc", "query", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if a != nil {
			a.log.Debug("diag", "checkService error: "+err.Error())
		}
		return diagResult{Status: "warn", Label: label, Detail: "Не удалось проверить"}
	}
	out := string(output)
	if strings.Contains(out, "RUNNING") {
		return diagResult{Status: "ok", Label: label, Detail: "Работает"}
	}
	if strings.Contains(out, "STOPPED") {
		return diagResult{Status: "warn", Label: label, Detail: "Остановлен"}
	}
	if strings.Contains(out, "does not exist") {
		return diagResult{Status: "warn", Label: label, Detail: "Не найден"}
	}
	return diagResult{Status: "ok", Label: label, Detail: "Присутствует"}
}

// checkAdguard — Adguard перехватывает трафик на системном уровне: запущен —
// предупреждение с советом, не запущен — всё хорошо.
func (a *App) checkAdguard() diagResult {
	cmd := executil.HiddenCmd("tasklist", "/FI", "IMAGENAME eq AdguardSvc.exe")
	output, err := cmd.CombinedOutput()
	if err != nil && a != nil {
		a.log.Debug("diag", "checkAdguard error: "+err.Error())
	}
	if strings.Contains(string(output), "AdguardSvc.exe") {
		return diagResult{Status: "warn", Label: "Adguard", Detail: "Запущен — может конфликтовать с обходом: попробуйте паузу защиты"}
	}
	return diagResult{Status: "ok", Label: "Adguard", Detail: "Не запущен — конфликтов нет"}
}

func checkWinDivert(zapretDir string) diagResult {
	if zapretDir == "" {
		return diagResult{Status: "warn", Label: "WinDivert", Detail: "Путь не задан"}
	}
	paths := []string{
		zapretDir + "\\WinDivert.dll",
		zapretDir + "\\bin\\WinDivert.dll",
		zapretDir + "\\WinDivert\\WinDivert.dll",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return diagResult{Status: "ok", Label: "WinDivert", Detail: "Найден"}
		}
	}
	return diagResult{Status: "error", Label: "WinDivert", Detail: "Не найден"}
}

func checkTCPTimestamps() diagResult {
	cmd := executil.HiddenCmd("netsh", "interface", "tcp", "show", "global")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: "TCP Timestamps", Detail: "Не удалось проверить"}
	}
	out := string(output)
	if strings.Contains(out, "disabled") || strings.Contains(out, "Отключено") {
		return diagResult{Status: "ok", Label: "TCP Timestamps", Detail: "Отключены (OK)"}
	}
	return diagResult{Status: "warn", Label: "TCP Timestamps", Detail: "Включены — могут мешать"}
}

func checkFirewallRule() diagResult {
	cmd := executil.HiddenCmd("netsh", "advfirewall", "firewall", "show", "rule", "name=ZPUI_SOCKS5")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: "Firewall (SOCKS5)", Detail: "Правило не найдено"}
	}
	out := string(output)
	if strings.Contains(out, "ZPUI_SOCKS5") {
		return diagResult{Status: "ok", Label: "Firewall (SOCKS5)", Detail: "Правило активно"}
	}
	return diagResult{Status: "warn", Label: "Firewall (SOCKS5)", Detail: "Правило не найдено"}
}

func (a *App) checkConflictingServices() diagResult {
	conflicts := []string{}
	services := map[string]string{
		"AdguardService": "Adguard",
		"SmartByte":      "SmartByte",
		"SAB":            "McAfee",
		"ekrn":           "ESET",
		"ksvlasst":       "Kaspersky",
	}
	for svc, name := range services {
		cmd := executil.HiddenCmd("sc", "query", svc)
		output, err := cmd.CombinedOutput()
		if err != nil && a != nil {
			a.log.Debug("diag", "checkConflictingServices cmd error: "+err.Error())
		}
		if strings.Contains(string(output), "RUNNING") {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) > 0 {
		return diagResult{Status: "warn", Label: "Конфликты", Detail: strings.Join(conflicts, ", ")}
	}
	return diagResult{Status: "ok", Label: "Конфликты", Detail: "Не обнаружены"}
}

func (a *App) checkDNS() diagResult {
	// Реестр адаптеров (нативно, без PowerShell): ищем loopback-DNS,
	// оставшийся от удалённого encrypted-режима.
	for _, ad := range xboxdns.SystemDNS() {
		for _, ip := range ad.DNS {
			if ip == "127.0.0.1" || ip == "::1" || strings.EqualFold(ip, "localhost") {
				return diagResult{Status: "warn", Label: "DNS", Detail: "Обнаружен локальный DNS"}
			}
		}
	}
	return diagResult{Status: "ok", Label: "DNS", Detail: "Стандартный"}
}

func checkProxy(a *App) diagResult {
	if a.tgproxy.IsRunning() {
		return diagResult{Status: "ok", Label: "Telegram-прокси", Detail: "Работает"}
	}
	return diagResult{Status: "warn", Label: "Telegram-прокси", Detail: "Остановлен"}
}

func (a *App) checkProcess(name, label string) diagResult {
	cmd := executil.HiddenCmd("tasklist", "/FI", "IMAGENAME eq "+name)
	output, err := cmd.CombinedOutput()
	if err != nil && a != nil {
		a.log.Debug("diag", "checkProcess cmd error ("+name+"): "+err.Error())
	}
	if strings.Contains(string(output), name) {
		return diagResult{Status: "ok", Label: label, Detail: "Запущен"}
	}
	return diagResult{Status: "warn", Label: label, Detail: "Не запущен"}
}

func (a *App) checkSystemProxy() diagResult {
	cmd := executil.HiddenCmd("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable")
	output, err := cmd.CombinedOutput()
	if err != nil && a != nil {
		a.log.Debug("diag", "checkSystemProxy cmd error: "+err.Error())
	}
	out := string(output)
	if strings.Contains(out, "0x1") {
		cmd2 := executil.HiddenCmd("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer")
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil && a != nil {
			a.log.Debug("diag", "checkSystemProxy cmd2 error: "+err2.Error())
		}
		proxy := strings.TrimSpace(strings.ReplaceAll(string(output2), "ProxyServer", ""))
		proxy = strings.TrimSpace(strings.ReplaceAll(proxy, "REG_SZ", ""))
		return diagResult{Status: "warn", Label: "Системный прокси", Detail: "Включен: " + proxy}
	}
	return diagResult{Status: "ok", Label: "Системный прокси", Detail: "Отключен"}
}

func (a *App) checkServiceList(keyword, label string) diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, err := cmd.CombinedOutput()
	if err != nil && a != nil {
		a.log.Debug("diag", "checkServiceList cmd error ("+keyword+"): "+err.Error())
	}
	if strings.Contains(string(output), keyword) {
		return diagResult{Status: "warn", Label: label, Detail: "Обнаружен — может конфликтовать"}
	}
	return diagResult{Status: "ok", Label: label, Detail: "Не обнаружен"}
}

func (a *App) checkIntelConnectivity() diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, err := cmd.CombinedOutput()
	if err != nil && a != nil {
		a.log.Debug("diag", "checkIntelConnectivity cmd error: "+err.Error())
	}
	out := string(output)
	if strings.Contains(out, "Intel") && strings.Contains(out, "Connectivity") && strings.Contains(out, "Network") {
		return diagResult{Status: "warn", Label: "Intel Connectivity", Detail: "Обнаружен — конфликтует с Zapret"}
	}
	return diagResult{Status: "ok", Label: "Intel Connectivity", Detail: "Не обнаружен"}
}

func (a *App) checkCheckPoint() diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, err := cmd.CombinedOutput()
	if err != nil && a != nil {
		a.log.Debug("diag", "checkCheckPoint cmd error: "+err.Error())
	}
	out := string(output)
	if strings.Contains(out, "TracSrvWrapper") || strings.Contains(out, "EPWD") {
		return diagResult{Status: "warn", Label: "Check Point", Detail: "Обнаружен — конфликтует с Zapret"}
	}
	return diagResult{Status: "ok", Label: "Check Point", Detail: "Не обнаружен"}
}

func (a *App) checkVPN() diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, err := cmd.CombinedOutput()
	if err != nil && a != nil {
		a.log.Debug("diag", "checkVPN cmd error: "+err.Error())
	}
	out := string(output)
	var found []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToUpper(line), "VPN") && strings.Contains(line, "SERVICE_NAME") {
			name := strings.TrimPrefix(line, "SERVICE_NAME: ")
			found = append(found, name)
		}
	}
	if len(found) > 0 {
		return diagResult{Status: "warn", Label: "VPN сервисы", Detail: strings.Join(found, ", ")}
	}
	return diagResult{Status: "ok", Label: "VPN сервисы", Detail: "Не обнаружены"}
}

func checkHostsFile() diagResult {
	hostsPath := os.Getenv("SystemRoot") + `\System32\drivers\etc\hosts`
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return diagResult{Status: "warn", Label: "Hosts файл", Detail: "Не удалось прочитать"}
	}
	content := strings.ToLower(string(data))
	blocked := []string{}
	for _, domain := range []string{"youtube.com", "youtu.be", "discord.com", "google.com"} {
		if strings.Contains(content, domain) && !strings.HasPrefix(strings.TrimSpace(content), "#") {
			blocked = append(blocked, domain)
		}
	}
	if len(blocked) > 0 {
		return diagResult{Status: "warn", Label: "Hosts файл", Detail: "Найдены записи: " + strings.Join(blocked, ", ")}
	}
	return diagResult{Status: "ok", Label: "Hosts файл", Detail: "Чистый"}
}

// ============================================================
// NETWORK HELPERS
// ============================================================

func getLocalIPs() []string {
	var ips []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range interfaces {
		nameLower := strings.ToLower(iface.Name)
		if strings.Contains(nameLower, "loopback") ||
			strings.Contains(nameLower, "vethernet") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
				continue
			}
			ip := ipNet.IP
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			if bytes.HasPrefix(ip, []byte{172, 26}) ||
				bytes.HasPrefix(ip, []byte{172, 20}) ||
				bytes.HasPrefix(ip, []byte{172, 17}) ||
				bytes.HasPrefix(ip, []byte{172, 18}) ||
				bytes.HasPrefix(ip, []byte{172, 19}) {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
}

func getMACAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if len(iface.HardwareAddr) > 0 && !strings.HasPrefix(iface.Name, "Loopback") {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}

func getHostname() string {
	// os.Hostname() вместо спавна процесса hostname: вызывается из GetStatus
	// при каждом опросе фронтенда.
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}

func getARPTable() map[string]string {
	arp := make(map[string]string)
	cmd := executil.HiddenCmd("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return arp
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Interface") || strings.HasPrefix(line, "---") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		host := strings.Trim(fields[0], " ")
		ipRaw := fields[0]
		mac := ""
		if len(fields) >= 2 {
			ipRaw = strings.Trim(fields[0], "()")
			if strings.Contains(fields[0], "(") {
				ipRaw = strings.Trim(fields[1], "()")
				mac = fields[3]
			} else if len(fields) >= 4 {
				mac = fields[1]
				ipRaw = strings.Trim(host, "()")
			}
		}
		if mac != "" && mac != "FF-FF-FF-FF-FF-FF" && mac != "ff-ff-ff-ff-ff-ff" {
			arp[ipRaw] = mac
		}
	}
	return arp
}

func resolveHostname(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	name := names[0]
	name = strings.TrimSuffix(name, ".")
	return name
}
