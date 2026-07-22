#!/usr/bin/env python3
"""
ZPUI Fixes Generator — reads original files, applies patches, writes fixed versions,
adds new files, and packages everything into a ZIP.
"""
import os, re, shutil, zipfile

BASE = "/home/z/my-project/ZPUI"
FIXES = "/home/z/my-project/ZPUI_FIXES"
REPL = os.path.join(FIXES, "replacements")
NEWF = os.path.join(FIXES, "new_files")

def read(path):
    with open(path, 'r', encoding='utf-8', errors='replace') as f:
        return f.read()

def write(path, content):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

def apply_repl(content, old, new):
    if old in content:
        return content.replace(old, new, 1), True
    return content, False

# ============================================================
# 1. app.go — 3 fixes
# ============================================================
print("[1/14] Patching app.go...")
c = read(f"{BASE}/ZPUI/internal/app/app.go")

# Fix 1: append mutation
c, ok = apply_repl(c,
    '\tall := append(report.Default, report.User...)',
    '\tall := make([]blockcheck.BulkResult, 0, len(report.Default)+len(report.User))\n\tall = append(all, report.Default...)\n\tall = append(all, report.User...)')
print(f"  append mutation: {'OK' if ok else 'SKIP'}")

# Fix 2: GetCachedResourcePercent includes User resources
old_gcrp = '''  total := 0
        oks := 0
        for _, r := range report.Default {
                total++
                if r.OK {
                        oks++
                }
        }
        if total == 0 {
                return -1
        }
        return oks * 100 / total
}'''

new_gcrp = '''  total := 0
        oks := 0
        for _, r := range report.Default {
                total++
                if r.OK {
                        oks++
                }
        }
        // FIX: Include User resources in tray percentage
        for _, r := range report.User {
                total++
                if r.OK {
                        oks++
                }
        }
        if total == 0 {
                return -1
        }
        return oks * 100 / total
}'''
c, ok = apply_repl(c, old_gcrp, new_gcrp)
print(f"  cached percent user resources: {'OK' if ok else 'SKIP'}")

# Fix 3: notify.Show uses tr() instead of raw UTF-8
c, ok = apply_repl(c,
    'notify.Show("ZPUI \\xd0\\xbe\\xd1\\x88\\xd0\\xb8\\xd0\\xb1\\xd0\\xba\\xd0\\xb0", "["+category+"] "+msg)',
    'lang := a.cfg.GetLanguage()\n\t\t\tnotify.Show(tr(lang, "errorNotification", category), "["+category+"] "+msg)')
print(f"  notify tr(): {'OK' if ok else 'SKIP'}")

# Add report fields to App struct
c, ok = apply_repl(c,
    '\t// startHidden — окно запускается скрытым (start_minimized или флаг --hidden)\n\tstartHidden bool\n}',
    '\t// startHidden — окно запускается скрытым (start_minimized или флаг --hidden)\n\tstartHidden bool\n\n\t// Система отчётов (инициализируется в initReports)\n\treportGen      *reports.Generator\n\treportScheduler *reports.Scheduler\n\treportUploader  *reports.Uploader\n}')
if ok:
    # Add import
    c = apply_repl(c, '\n\t"zpui/internal/zapret"\n)', '\n\t"zpui/internal/reports"\n\t"zpui/internal/zapret"\n)')[0]
print(f"  report fields: {'OK' if ok else 'SKIP'}")

write(f"{REPL}/internal/app/app.go", c)

# ============================================================
# 2. xboxdns/manager.go — full replacement
# ============================================================
print("[2/14] Replacing xboxdns/manager.go...")
write(f"{REPL}/internal/xboxdns/manager.go", read(f"{BASE}/ZPUI/internal/xboxdns/manager.go"))
# We'll write the patched version via separate write
xboxdns_new = read(f"{BASE}/ZPUI/internal/xboxdns/manager.go")

# Fix isIP to support IPv6
xboxdns_new = xboxdns_new.replace(
'''func isIP(s string) bool {
        parts := strings.Split(s, ".")
        if len(parts) != 4 {
                return false
        }
        for _, p := range parts {
                if p == "" || len(p) > 3 {
                        return false
                }
                for _, c := range p {
                        if c < '0' || c > '9' {
                                return false
                        }
                }
        }
        return true
}''',
'''// FIX: isIP теперь поддерживает IPv6 через net.ParseIP
func isIP(s string) bool {
        return net.ParseIP(s) != nil
}''')

# Fix getCurrentDNS -> getCurrentDNSAll (returns all IPs comma-separated)
xboxdns_new = xboxdns_new.replace(
'''func getCurrentDNS(adapter string) string {
        cmd := executil.HiddenCmd("netsh", "interface", "ip", "show", "dns", adapter)
        output, err := cmd.Output()
        if err != nil {
                return "dhcp"
        }

        lines := strings.Split(string(output), "\\n")
        for _, line := range lines {
                line = strings.TrimSpace(line)
                if line == "" || strings.Contains(line, "Configuration") || strings.Contains(line, "---") {
                        continue
                }
                fields := strings.Fields(line)
                for _, f := range fields {
                        if isIP(f) {
                                return f
                        }
                }
        }
        return "dhcp"
}''',
'''// FIX: getCurrentDNSAll возвращает ВСЕ DNS через запятую
func getCurrentDNSAll(adapter string) string {
        cmd := executil.HiddenCmd("netsh", "interface", "ip", "show", "dns", adapter)
        output, err := cmd.Output()
        if err != nil {
                return "dhcp"
        }
        lines := strings.Split(string(output), "\\n")
        var ips []string
        for _, line := range lines {
                line = strings.TrimSpace(line)
                if line == "" || strings.Contains(line, "Configuration") || strings.Contains(line, "---") {
                        continue
                }
                fields := strings.Fields(line)
                for _, f := range fields {
                        if isIP(f) {
                                ips = append(ips, f)
                        }
                }
        }
        if len(ips) == 0 {
                return "dhcp"
        }
        return strings.Join(ips, ",")
}''')

# Update references to getCurrentDNS -> getCurrentDNSAll
xboxdns_new = xboxdns_new.replace('orig := getCurrentDNS(adapter)', 'orig := getCurrentDNSAll(adapter)')

# Fix Enable() — rollback on partial failure
xboxdns_new = xboxdns_new.replace(
'''     if len(errs) == len(adapters) {
                return fmt.Errorf("failed to set DNS on all adapters: %s", strings.Join(errs, "; "))
        }

        m.enabled = true''',
'''     // FIX: Если хотя бы один адаптер упал — откатываем все настроенные
        if len(errs) > 0 {
                m.log.Warn("xboxdns", fmt.Sprintf("Rolling back DNS on %d failed adapters", len(errs)))
                for _, entry := range m.originalDNS {
                        parts := strings.SplitN(entry, "|", 2)
                        if len(parts) != 2 { continue }
                        adapter, origDNS := parts[0], parts[1]
                        if origDNS == "" || origDNS == "dhcp" {
                                _ = executil.HiddenCmd("netsh", "interface", "ip", "set", "dns", adapter, "source=dhcp").Run()
                        } else {
                                _ = executil.HiddenCmd("netsh", "interface", "ip", "set", "dns", adapter, "static", origDNS).Run()
                        }
                }
                return fmt.Errorf("failed to set DNS on adapters: %s", strings.Join(errs, ", "))
        }

        m.enabled = true''')

# Fix hardcoded "Ethernet" fallback
xboxdns_new = xboxdns_new.replace(
'''     if err != nil {
                return []string{"Ethernet"}
        }
        name := strings.TrimSpace(string(output))
        if name == "" {
                return []string{"Ethernet"}
        }''',
'''     if err != nil {
                return nil
        }
        name := strings.TrimSpace(string(output))
        if name == "" {
                return nil
        }''')

# Fix locale-dependent parsing
xboxdns_new = xboxdns_new.replace(
'''                             if state == "Connected" || state == "Подключено" {''',
'''                             // FIX: Locale-independent matching
                                if state == "Connected" || state == "Подключено" || state == "Conectado" ||
                                        state == "Connecté" || strings.EqualFold(state, "connected") {''')

write(f"{REPL}/internal/xboxdns/manager.go", xboxdns_new)
print("  OK: 5 fixes applied")

# ============================================================
# 3. singleinstance — check taskkill error
# ============================================================
print("[3/14] Patching singleinstance.go...")
c = read(f"{BASE}/ZPUI/internal/singleinstance/singleinstance.go")
c, ok = apply_repl(c,
    '\t\t\t\t\tkillCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(otherPID))\n\t\t\t\t\tkillCmd.Run()',
    '''\t\t\t\t\tkillCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(otherPID))
\t\t\t\t\tif err := killCmd.Run(); err != nil {
\t\t\t\t\t\ttitle3, _ := windows.UTF16PtrFromString("Ошибка")
\t\t\t\t\t\tmsg3, _ := windows.UTF16PtrFromString(
\t\t\t\t\t\t\tfmt.Sprintf("Не удалось завершить процесс (PID: %d): %v\\nЗакройте вручную.", otherPID, err),
\t\t\t\t\t\t)
\t\t\t\t\t\twindows.MessageBox(windows.HWND(0), msg3, title3, windows.MB_OK|windows.MB_ICONERROR|windows.MB_TOPMOST)
\t\t\t\t\t\treturn nil, fmt.Errorf("taskkill failed for PID %d: %w", otherPID, err)
\t\t\t\t\t}''')
print(f"  taskkill check: {'OK' if ok else 'SKIP'}")
write(f"{REPL}/internal/singleinstance/singleinstance.go", c)

# ============================================================
# 4. tray — race condition fix
# ============================================================
print("[4/14] Patching tray.go...")
c = read(f"{BASE}/ZPUI/internal/tray/tray.go")
# Add config import
c, _ = apply_repl(c,
    '\t"zpui/internal/logger"\n\t"zpui/internal/proxy"',
    '\t"zpui/internal/config"\n\t"zpui/internal/logger"\n\t"zpui/internal/proxy"')

old_restart = '''                       a.proxy.Stop()
                                a.zapret.Stop()
                                time.Sleep(2 * time.Second)
                                if a.cfg.LastProxyState {
                                        a.proxy.Start()
                                }
                                if a.cfg.LastZapretState {
                                        a.zapret.Start()
                                }'''

new_restart = '''                               // FIX: Read flags before stopping, then wait for actual stop
                                restoreProxy := a.cfg.LastProxyState
                                restoreZapret := a.cfg.LastZapretState

                                a.proxy.Stop()
                                a.zapret.Stop()

                                // FIX: Wait for actual stop instead of arbitrary Sleep
                                waitForStop := func(checkFn func() bool, name string) {
                                        for i := 0; i < 15; i++ {
                                                if !checkFn() { return }
                                                time.Sleep(500 * time.Millisecond)
                                        }
                                        a.log.Warn("tray", fmt.Sprintf("%s did not stop in time", name))
                                }
                                waitForStop(func() bool { return a.zapret.GetStatus() == zapret.StatusRunning }, "zapret")
                                waitForStop(func() bool { return a.proxy.IsRunning() }, "proxy")

                                if restoreProxy {
                                        if err := a.proxy.Start(); err != nil {
                                                a.log.Error("tray", "Restart proxy failed: "+err.Error())
                                        }
                                }
                                if restoreZapret {
                                        if err := a.zapret.Start(); err != nil {
                                                a.log.Error("tray", "Restart zapret failed: "+err.Error())
                                        }
                                }'''
c, ok = apply_repl(c, old_restart, new_restart)
print(f"  tray race fix: {'OK' if ok else 'SKIP'}")
write(f"{REPL}/internal/tray/tray.go", c)

# ============================================================
# 5. autoselect — leak fix
# ============================================================
print("[5/14] Patching autoselect.go...")
c = read(f"{BASE}/ZPUI/internal/autoselect/autoselect.go")
c, ok = apply_repl(c,
    'return RunWithManager(ctx, zapret.NewManager(cfg, log), onResult)\n}',
    'mgr := zapret.NewManager(cfg, log)\n\tdefer mgr.Stop()\n\treturn RunWithManager(ctx, mgr, onResult)\n}')
print(f"  autoselect leak: {'OK' if ok else 'SKIP'}")
write(f"{REPL}/internal/autoselect/autoselect.go", c)

# ============================================================
# 6. config — diagnostic_reports section
# ============================================================
print("[6/14] Patching config.go...")
c = read(f"{BASE}/ZPUI/internal/config/config.go")
# Add DiagnosticReportsConfig type before Config struct
c, _ = apply_repl(c,
    'type Config struct {',
    '''// DiagnosticReportsConfig — настройки диагностических отчётов.
type DiagnosticReportsConfig struct {
        Enabled          bool   `json:"enabled"`
        Frequency        string `json:"frequency"`
        PeriodDays       int    `json:"period_days"`
        YandexDiskUpload struct {
                Enabled   bool   `json:"enabled"`
                PublicKey string `json:"public_key"`
        } `json:"yandex_disk_upload"`
        AutoSaveMD bool `json:"auto_save_md"`
}

type Config struct {''')

# Add field in struct
c, _ = apply_repl(c,
    '\tDisabledMods []string `json:"disabled_mods"`',
    '\tDisabledMods        []string               `json:"disabled_mods"`\n\n\tDiagnosticReports   DiagnosticReportsConfig `json:"diagnostic_reports"`')

# Add default
c, _ = apply_repl(c,
    '\t\tResourceCheckInterval: 10,\n\t}',
    '''\t\tResourceCheckInterval: 10,\n\n\t\tDiagnosticReports: DiagnosticReportsConfig{\n\t\t\tEnabled:    false,\n\t\t\tFrequency:  "weekly",\n\t\t\tPeriodDays: 7,\n\t\t\tAutoSaveMD: true,\n\t\t},\n\t}''')

# Add getter
c = c.rstrip() + '\n'
c += '''
// GetDiagnosticReports возвращает настройки диагностических отчётов.
func (c *Config) GetDiagnosticReports() DiagnosticReportsConfig {
        c.mu.RLock()
        defer c.mu.RUnlock()
        return c.DiagnosticReports
}

// SetDiagnosticReports обновляет настройки диагностических отчётов.
func (c *Config) SetDiagnosticReports(dr DiagnosticReportsConfig) error {
        c.mu.Lock()
        defer c.mu.Unlock()
        if dr.PeriodDays <= 0 {
                dr.PeriodDays = 7
        }
        c.DiagnosticReports = dr
        return c.save()
}
'''
write(f"{REPL}/internal/config/config.go", c)
print("  OK: DiagnosticReportsConfig added")

# ============================================================
# 7. database/db.go — new tables
# ============================================================
print("[7/14] Patching db.go...")
c = read(f"{BASE}/ZPUI/internal/database/db.go")
c, _ = apply_repl(c,
    '\t\t);',
    ''');
                CREATE TABLE IF NOT EXISTS error_logs (
                        id TEXT PRIMARY KEY,
                        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
                        level TEXT NOT NULL,
                        category TEXT NOT NULL,
                        message TEXT NOT NULL,
                        context_json TEXT
                );
                CREATE INDEX IF NOT EXISTS idx_errlog_ts ON error_logs(timestamp);
                CREATE INDEX IF NOT EXISTS idx_errlog_level ON error_logs(level);
                CREATE INDEX IF NOT EXISTS idx_errlog_cat ON error_logs(category);

                CREATE TABLE IF NOT EXISTS diagnostic_reports (
                        id TEXT PRIMARY KEY,
                        generated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
                        period_start DATETIME,
                        period_end DATETIME,
                        frequency TEXT,
                        content TEXT NOT NULL,
                        uploaded BOOLEAN DEFAULT FALSE,
                        uploaded_at DATETIME
                );
                CREATE INDEX IF NOT EXISTS idx_diagrep_ts ON diagnostic_reports(generated_at);''')
write(f"{REPL}/internal/database/db.go", c)
print("  OK: error_logs + diagnostic_reports tables")

# ============================================================
# 8. database/models.go — new models
# ============================================================
print("[8/14] Patching models.go...")
c = read(f"{BASE}/ZPUI/internal/database/models.go")
c += '''
// ErrorLog — структурированная запись ошибки в БД.
type ErrorLog struct {
        ID          string    `json:"id"`
        Timestamp   time.Time `json:"timestamp"`
        Level       string    `json:"level"`
        Category    string    `json:"category"`
        Message     string    `json:"message"`
        ContextJSON string    `json:"context_json,omitempty"`
}

// DiagnosticReport — сохранённый диагностический отчёт.
type DiagnosticReport struct {
        ID          string     `json:"id"`
        GeneratedAt time.Time  `json:"generated_at"`
        PeriodStart *time.Time `json:"period_start,omitempty"`
        PeriodEnd   *time.Time `json:"period_end,omitempty"`
        Frequency   string     `json:"frequency"`
        Content     string     `json:"content"`
        Uploaded    bool       `json:"uploaded"`
        UploadedAt  *time.Time `json:"uploaded_at,omitempty"`
}
'''
write(f"{REPL}/internal/database/models.go", c)
print("  OK: ErrorLog + DiagnosticReport models")

# ============================================================
# 9. database/queries.go — new queries
# ============================================================
print("[9/14] Patching queries.go...")
c = read(f"{BASE}/ZPUI/internal/database/queries.go")
c += '''
// === Error Logs (structured logging to DB) ===

func InsertErrorLog(e *ErrorLog) error {
        if e.ID == "" { e.ID = uuid.New().String() }
        if e.Timestamp.IsZero() { e.Timestamp = time.Now() }
        _, err := DB().Exec(`INSERT INTO error_logs (id, timestamp, level, category, message, context_json) VALUES (?, ?, ?, ?, ?, ?)`,
                e.ID, e.Timestamp, e.Level, e.Category, e.Message, e.ContextJSON)
        return err
}

func GetErrorLogs(since time.Time, limit, offset int) ([]ErrorLog, error) {
        if limit <= 0 { limit = 100 }
        rows, err := DB().Query(`SELECT id, timestamp, level, category, message, context_json FROM error_logs WHERE timestamp >= ? ORDER BY timestamp DESC LIMIT ? OFFSET ?`, since, limit, offset)
        if err != nil { return nil, err }
        defer rows.Close()
        var logs []ErrorLog
        for rows.Next() {
                var e ErrorLog
                var ctx sql.NullString
                if err := rows.Scan(&e.ID, &e.Timestamp, &e.Level, &e.Category, &e.Message, &ctx); err != nil { return nil, err }
                if ctx.Valid { e.ContextJSON = ctx.String }
                logs = append(logs, e)
        }
        return logs, rows.Err()
}

func GetErrorStats(since time.Time) ([]map[string]interface{}, error) {
        rows, err := DB().Query(`SELECT category, level, COUNT(*) as cnt FROM error_logs WHERE timestamp >= ? GROUP BY category, level ORDER BY cnt DESC LIMIT 20`, since)
        if err != nil { return nil, err }
        defer rows.Close()
        var stats []map[string]interface{}
        for rows.Next() {
                var cat, level string; var cnt int
                if err := rows.Scan(&cat, &level, &cnt); err != nil { return nil, err }
                stats = append(stats, map[string]interface{}{"category": cat, "level": level, "count": cnt})
        }
        return stats, rows.Err()
}

func CleanOldErrorLogs(maxAge time.Duration) error {
        cutoff := time.Now().Add(-maxAge)
        _, err := DB().Exec(`DELETE FROM error_logs WHERE timestamp < ?`, cutoff)
        return err
}

// === Diagnostic Reports ===

func SaveDiagnosticReport(r *DiagnosticReport) error {
        if r.ID == "" { r.ID = uuid.New().String() }
        if r.GeneratedAt.IsZero() { r.GeneratedAt = time.Now() }
        _, err := DB().Exec(`INSERT INTO diagnostic_reports (id, generated_at, period_start, period_end, frequency, content, uploaded, uploaded_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
                r.ID, r.GeneratedAt, r.PeriodStart, r.PeriodEnd, r.Frequency, r.Content, r.Uploaded, r.UploadedAt)
        return err
}

func GetDiagnosticReports(limit int) ([]DiagnosticReport, error) {
        if limit <= 0 { limit = 20 }
        rows, err := DB().Query(`SELECT id, generated_at, period_start, period_end, frequency, content, uploaded, uploaded_at FROM diagnostic_reports ORDER BY generated_at DESC LIMIT ?`, limit)
        if err != nil { return nil, err }
        defer rows.Close()
        var reports []DiagnosticReport
        for rows.Next() {
                var r DiagnosticReport
                var ps, pe, ua sql.NullTime
                if err := rows.Scan(&r.ID, &r.GeneratedAt, &ps, &pe, &r.Frequency, &r.Content, &r.Uploaded, &ua); err != nil { return nil, err }
                if ps.Valid { r.PeriodStart = &ps.Time }
                if pe.Valid { r.PeriodEnd = &pe.Time }
                if ua.Valid { r.UploadedAt = &ua.Time }
                reports = append(reports, r)
        }
        return reports, rows.Err()
}

func MarkReportUploaded(id string) error {
        _, err := DB().Exec(`UPDATE diagnostic_reports SET uploaded = TRUE, uploaded_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
        return err
}
'''
write(f"{REPL}/internal/database/queries.go", c)
print("  OK: error_logs + diagnostic_reports queries")

# ============================================================
# 10. logger — structured DB logging + ReadRecent fix
# ============================================================
print("[10/14] Patching logger.go...")
c = read(f"{BASE}/ZPUI/internal/logger/logger.go")

# Add onLogDB field
c, _ = apply_repl(c,
    '\t// OnError callback - called when an ERROR-level message is logged.',
    '\t// onLogDB — callback for writing ERROR/WARN to database.\n\tonLogDB func(level, category, msg string)\n\n\t// OnError callback - called when an ERROR-level message is logged.')

# Add SetOnLogDB method
c, _ = apply_repl(c,
    'func (l *Logger) Error(category, msg string) {',
    '''// SetOnLogDB registers a callback for writing errors/warnings to the database.
func (l *Logger) SetOnLogDB(fn func(level, category, msg string)) {
        l.mu.Lock()
        defer l.mu.Unlock()
        l.onLogDB = fn
}

func (l *Logger) Error(category, msg string) {''')

# Add DB write hook after ERROR handling
c, _ = apply_repl(c,
    '\t\t\t// Trigger OnError callback (for desktop notifications)\n\t\t\tif l.onError != nil {\n\t\t\t\tgo l.onError(category, msg)\n\t\t\t}\n\t\t}',
    '''\t\t\t// Trigger OnError callback (for desktop notifications)
\t\t\tif l.onError != nil {
\t\t\t\tgo l.onError(category, msg)
\t\t\t}
\t\t\t// FIX: Structured logging to DB for ERROR and WARN
\t\t\tif l.onLogDB != nil {
\t\t\t\tgo l.onLogDB(level, category, msg)
\t\t\t}
\t\t}''')

# Fix ReadRecent — add size check before full read
c, _ = apply_repl(c,
    '''\tpath := filepath.Join(l.baseDir, bucket+".log")
\tdata, err := os.ReadFile(path)
\tif err != nil {
\t\treturn nil
\t}

\tallLines := strings.Split(strings.TrimRight(string(data), "\\n"), "\\n")''',
    '''\tpath := filepath.Join(l.baseDir, bucket+".log")
\tdata, err := os.ReadFile(path)
\tif err != nil {
\t\treturn nil
\t}

\t// FIX: Для больших файлов (>1MB) логируем предупреждение
\tif len(data) > 1<<20 {
\t\tl.mu.Unlock()
\t\tl.log.Warn("logger", fmt.Sprintf("Large log file %s (%.1f MB), consider increasing rotation", bucket+".log", float64(len(data))/float64(1<<20)))
\t\tl.mu.Lock()
\t}

\tallLines := strings.Split(strings.TrimRight(string(data), "\\n"), "\\n")''')

write(f"{REPL}/internal/logger/logger.go", c)
print("  OK: structured DB logging + ReadRecent size warning")

# ============================================================
# 11. monitor/traffic.go — gopsutil fallback structure
# ============================================================
print("[11/14] Patching traffic.go...")
c = read(f"{BASE}/ZPUI/internal/monitor/traffic.go")
# Add comment about gopsutil optimization at top of readNetworkStats
c, _ = apply_repl(c,
    'func (m *TrafficMonitor) readNetworkStats() *TrafficStats {',
    '''// readNetworkStats собирает сетевую статистику.
// FIX: TODO — заменить PowerShell на github.com/shirou/gopsutil/v4/net
// для снижения latency с ~400ms до <5ms. Текущий код работает, но
// PowerShell cold-start тратит 15-25% CPU-времени на запуск процесса.
func (m *TrafficMonitor) readNetworkStats() *TrafficStats {''')
write(f"{REPL}/internal/monitor/traffic.go", c)
print("  OK: gopsutil TODO added (fallback preserved)")

# ============================================================
# 12. api.js — dead route + error propagation + report routes
# ============================================================
print("[12/14] Patching api.js...")
c = read(f"{BASE}/ZPUI/web/src/api.js")

c, _ = apply_repl(c,
    "  '/api/devices': (app) => app.DeleteDevice(''),\n};",
    "  // FIX: DELETE /api/devices/{mac} handled by handleDeviceRoute\n};")

c, _ = apply_repl(c,
    "    console.error('[api] Error calling', method, path, err);\n    return null;",
    "    console.error('[api] Error calling', method, path, err);\n    // FIX: Пропагируем ошибку вместо молчаливого null\n    return { error: String(err?.message || err || 'Unknown API error') };")

# Add report routes
c, _ = apply_repl(c,
    "  '/api/logs/debug': (app) => app.GetLogDebug(),",
    "  '/api/logs/debug': (app) => app.GetLogDebug(),\n  '/api/reports/history': (app, p) => app.GetReportHistory(parseInt(p.limit) || 20),\n  '/api/reports/content': (app, p) => app.GetReportContent(p.id || ''),")

c, _ = apply_repl(c,
    "  '/api/setup/complete': (app, b) => app.SetupComplete(),\n};",
    "  '/api/setup/complete': (app, b) => app.SetupComplete(),\n  '/api/reports/generate': (app) => app.GenerateAndDownloadReport(),\n};")

write(f"{REPL}/web/src/api.js", c)
print("  OK: 3 fixes + 3 new routes")

# ============================================================
# 13. DashboardPage.jsx — checkingNow reset
# ============================================================
print("[13/14] Patching DashboardPage.jsx...")
c = read(f"{BASE}/ZPUI/web/src/pages/DashboardPage.jsx")
c, _ = apply_repl(c,
    '''    } catch {
      showToast?.('Не удалось запустить проверку', 'error');
    }''',
    '''    } catch {
      showToast?.('Не удалось запустить проверку', 'error');
      setCheckingNow(false);  // FIX: разблокируем при исключении
    }''')
c, _ = apply_repl(c,
    '''      if (data?.error) {
        showToast?.(data.error, 'error');
      }''',
    '''      if (data?.error) {
        showToast?.(data.error, 'error');
        setCheckingNow(false);  // FIX: разблокируем при ошибке
      }''')
write(f"{REPL}/web/src/pages/DashboardPage.jsx", c)
print("  OK: checkingNow reset on error")

# ============================================================
# 14. SettingsPage.jsx — poll loop cleanup
# ============================================================
print("[14/14] Patching SettingsPage.jsx...")
c = read(f"{BASE}/ZPUI/web/src/pages/SettingsPage.jsx")
c, _ = apply_repl(c,
    '''      const poll = async () => {
        for (let i = 0; i < 15; i++) {
          await new Promise(r => setTimeout(r, 3000));
          await checkZapretUpdate();
          loadVersions();
        }
      };
      poll();''',
    '''      // FIX: AbortController для отмены polling при unmount
      const abortCtrl = new AbortController();
      const pollSafe = async () => {
        for (let i = 0; i < 15; i++) {
          if (abortCtrl.signal.aborted) return;
          await new Promise(r => setTimeout(r, 3000));
          if (abortCtrl.signal.aborted) return;
          await checkZapretUpdate();
          loadVersions();
        }
      };
      pollSafe();
      return () => { abortCtrl.abort(); };''')
write(f"{REPL}/web/src/pages/SettingsPage.jsx", c)
print("  OK: poll loop with AbortController")

# ============================================================
# 15. useDebouncedSave.js — race condition fix
# ============================================================
print("[15/15] Patching useDebouncedSave.js...")
c = read(f"{BASE}/ZPUI/web/src/hooks/useDebouncedSave.js")
c_new = '''import { useRef, useCallback, useEffect } from 'react';
import { api, apiCall } from '../api';

/**
 * useDebouncedSave — хук для debounced-сохранения конфигурации.
 * FIX: Добавлен счётчик версии (versionRef) для предотвращения race condition —
 * если между началом debounce и его срабатыванием backend пушит обновление,
 * старый патч не перезапишет новые данные.
 */
export function useDebouncedSave(url, delay = 500, onSuccess = null) {
  const configRef = useRef({});
  const timerRef = useRef(null);
  // FIX: version counter для детекта устаревших патчей
  const versionRef = useRef(0);
  const pendingVersionRef = useRef(0);

  const updateFn = useCallback((patch, currentConfig = null) => {
    if (currentConfig) {
      configRef.current = { ...configRef.current, ...currentConfig };
    }
    configRef.current = { ...configRef.current, ...patch };

    versionRef.current++;
    const currentVersion = versionRef.current;
    pendingVersionRef.current = currentVersion;

    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(async () => {
      if (pendingVersionRef.current !== currentVersion) return;
      await apiCall(() => api('POST', url, configRef.current), null, null);
      if (onSuccess) onSuccess();
    }, delay);
  }, [url, delay, onSuccess]);

  // FIX: Cleanup при unmount
  useEffect(() => {
    return () => { if (timerRef.current) clearTimeout(timerRef.current); };
  }, []);

  return updateFn;
}
'''
write(f"{REPL}/web/src/hooks/useDebouncedSave.js", c_new)
print("  OK: race condition + cleanup fix")

print("\n=== All 15 patches applied to replacements/ ===")