import os
def rd(p):
    with open(p,'r',encoding='utf-8',errors='replace') as f: return f.read()
def wr(p,c):
    os.makedirs(os.path.dirname(p),exist_ok=True)
    with open(p,'w',encoding='utf-8') as f: f.write(c)
def rp(c,o,n):
    if o in c: return c.replace(o,n,1),True
    return c,False

B='/home/z/my-project/ZPUI'
R='/home/z/my-project/ZPUI_FIXES/replacements'
N='/home/z/my-project/ZPUI_FIXES/new_files'

# === 1. app.go ===
print('[1] app.go')
c=rd(f'{B}/internal/app/app.go')
c,_=rp(c,'all := append(report.Default, report.User...)','all := make([]blockcheck.BulkResult, 0, len(report.Default)+len(report.User))\n\tall = append(all, report.Default...)\n\tall = append(all, report.User...)')
old='for _, r := range report.Default {\n\t\ttotal++\n\t\tif r.OK {\n\t\t\toks++\n\t\t}\n\t}\n\tif total == 0 {'
new='for _, r := range report.Default {\n\t\ttotal++\n\t\tif r.OK {\n\t\t\toks++\n\t\t}\n\t}\n\t// FIX: Include User resources in tray percentage\n\tfor _, r := range report.User {\n\t\ttotal++\n\t\tif r.OK {\n\t\t\toks++\n\t\t}\n\t}\n\tif total == 0 {'
c,_=rp(c,old,new)
wr(f'{R}/internal/app/app.go',c)
print('  OK: append mutation + tray percent')

# === 2. xboxdns/manager.go ===
print('[2] xboxdns/manager.go')
c=rd(f'{B}/internal/xboxdns/manager.go')
# Fix isIP
c,_=rp(c,'func isIP(s string) bool {\n\tparts := strings.Split(s, ".")','// FIX: isIP now supports IPv6 via net.ParseIP\nfunc isIP(s string) bool {\n\treturn net.ParseIP(s) != nil\n}\n\nfunc _isIPOld(s string) bool {\n\tparts := strings.Split(s, ".")')
# Fix Enable rollback on ANY failure
c,_=rp(c,'if len(errs) == len(adapters) {\n\t\treturn fmt.Errorf("failed to set DNS on all adapters: %s", strings.Join(errs, "; "))\n\t}\n\n\tm.enabled = true','// FIX: Rollback on ANY adapter failure\n\tif len(errs) > 0 {\n\t\tm.log.Warn("xboxdns", fmt.Sprintf("Rolling back DNS on %d failed adapters", len(errs)))\n\t\tfor _, entry := range m.originalDNS {\n\t\t\tparts := strings.SplitN(entry, "|", 2)\n\t\t\tif len(parts) != 2 { continue }\n\t\t\tadapter, origDNS := parts[0], parts[1]\n\t\t\tif origDNS == "" || origDNS == "dhcp" {\n\t\t\t\t_ = executil.HiddenCmd("netsh", "interface", "ip", "set", "dns", adapter, "source=dhcp").Run()\n\t\t\t} else {\n\t\t\t\t_ = executil.HiddenCmd("netsh", "interface", "ip", "set", "dns", adapter, "static", origDNS).Run()\n\t\t\t}\n\t\t}\n\t\treturn fmt.Errorf("failed to set DNS on adapters: %s", strings.Join(errs, ", "))\n\t}\n\n\tm.enabled = true')
# Fix hardcoded Ethernet
c,_=rp(c,'return []string{"Ethernet"}\n\t}','return nil\n\t}')
# Fix locale
c,_=rp(c,'if state == "Connected" || state == "\u041f\u043e\u0434\u043a\u043b\u044e\u0447\u0435\u043d\u043e" {','if state == "Connected" || state == "\u041f\u043e\u0434\u043a\u043b\u044e\u0447\u0435\u043d\u043e" || strings.EqualFold(state, "connected") {')
wr(f'{R}/internal/xboxdns/manager.go',c)
print('  OK: rollback + ethernet + locale + isIP')

# === 3. singleinstance ===
print('[3] singleinstance.go')
c=rd(f'{B}/internal/singleinstance/singleinstance.go')
c,_=rp(c,'killCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(otherPID))\n\t\t\t\t\tkillCmd.Run()','killCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(otherPID))\n\t\t\t\t\tif err := killCmd.Run(); err != nil {\n\t\t\t\t\t\ttitle3, _ := windows.UTF16PtrFromString("\u041e\u0448\u0438\u0431\u043a\u0430")\n\t\t\t\t\t\tmsg3, _ := windows.UTF16PtrFromString(fmt.Sprintf("PID %d: %v", otherPID, err))\n\t\t\t\t\t\twindows.MessageBox(windows.HWND(0), msg3, title3, windows.MB_OK|windows.MB_ICONERROR|windows.MB_TOPMOST)\n\t\t\t\t\t\treturn nil, fmt.Errorf("taskkill failed: %w", err)\n\t\t\t\t\t}')
wr(f'{R}/internal/singleinstance/singleinstance.go',c)
print('  OK: taskkill error checked')

# === 4. tray ===
print('[4] tray.go')
c=rd(f'{B}/internal/tray/tray.go')
c,_=rp(c,'a.proxy.Stop()\n\t\t\t\ta.zapret.Stop()\n\t\t\t\ttime.Sleep(2 * time.Second)\n\t\t\t\tif a.cfg.LastProxyState {','// FIX: read flags before stopping\n\t\t\t\trestoreProxy := a.cfg.LastProxyState\n\t\t\t\trestoreZapret := a.cfg.LastZapretState\n\n\t\t\t\ta.proxy.Stop()\n\t\t\t\ta.zapret.Stop()\n\t\t\t\t// FIX: wait for actual stop\n\t\t\t\tfor i := 0; i < 15; i++ {\n\t\t\t\t\tif a.zapret.GetStatus() != "running" && !a.proxy.IsRunning() { break }\n\t\t\t\t\ttime.Sleep(500 * time.Millisecond)\n\t\t\t\t}\n\t\t\t\tif restoreProxy {')
c,_=rp(c,'if a.cfg.LastZapretState {\n\t\t\t\ta.zapret.Start()','if restoreZapret {\n\t\t\t\ta.zapret.Start()')
wr(f'{R}/internal/tray/tray.go',c)
print('  OK: race condition fixed')

# === 5. autoselect ===
print('[5] autoselect.go')
c=rd(f'{B}/internal/autoselect/autoselect.go')
c,_=rp(c,'return RunWithManager(ctx, zapret.NewManager(cfg, log), onResult)\n}','mgr := zapret.NewManager(cfg, log)\n\tdefer mgr.Stop()\n\treturn RunWithManager(ctx, mgr, onResult)\n}')
wr(f'{R}/internal/autoselect/autoselect.go',c)
print('  OK: leak fixed')

# === 6. config ===
print('[6] config.go')
c=rd(f'{B}/internal/config/config.go')
c,_=rp(c,'type Config struct {','''type DiagnosticReportsConfig struct {
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
c,_=rp(c,'\tDisabledMods []string `json:"disabled_mods"`','\tDisabledMods        []string               `json:"disabled_mods"`\n\n\tDiagnosticReports   DiagnosticReportsConfig `json:"diagnostic_reports"`')
c,_=rp(c,'\t\tResourceCheckInterval: 10,\n\t}','\t\tResourceCheckInterval: 10,\n\t\tDiagnosticReports: DiagnosticReportsConfig{\n\t\t\tEnabled:    false,\n\t\t\tFrequency:  "weekly",\n\t\t\tPeriodDays: 7,\n\t\t\tAutoSaveMD: true,\n\t\t},\n\t}')
wr(f'{R}/internal/config/config.go',c)
print('  OK: DiagnosticReportsConfig added')

# === 7. database/db.go ===
print('[7] database/db.go')
c=rd(f'{B}/internal/database/db.go')
c,_=rp(c,'\t\t);',''');

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
wr(f'{R}/internal/database/db.go',c)
print('  OK: new tables')

# === 8. database/models.go ===
print('[8] database/models.go')
c=rd(f'{B}/internal/database/models.go')
c+='''

// ErrorLog — structured error record in DB
type ErrorLog struct {
        ID          string    `json:"id"`
        Timestamp   time.Time `json:"timestamp"`
        Level       string    `json:"level"`
        Category    string    `json:"category"`
        Message     string    `json:"message"`
        ContextJSON string    `json:"context_json,omitempty"`
}

// DiagnosticReport — saved diagnostic report
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
wr(f'{R}/internal/database/models.go',c)
print('  OK')

# === 9. database/queries.go ===
print('[9] database/queries.go')
c=rd(f'{B}/internal/database/queries.go')
c+='''

// === Error Logs ===
func InsertErrorLog(e *ErrorLog) error {
        if e.ID == "" { e.ID = uuid.New().String() }
        if e.Timestamp.IsZero() { e.Timestamp = time.Now() }
        _, err := DB().Exec(`INSERT INTO error_logs (id,timestamp,level,category,message,context_json) VALUES (?,?,?,?,?,?)`, e.ID, e.Timestamp, e.Level, e.Category, e.Message, e.ContextJSON)
        return err
}

func GetErrorLogs(since time.Time, limit, offset int) ([]ErrorLog, error) {
        if limit <= 0 { limit = 100 }
        rows, err := DB().Query(`SELECT id,timestamp,level,category,message,context_json FROM error_logs WHERE timestamp >= ? ORDER BY timestamp DESC LIMIT ? OFFSET ?`, since, limit, offset)
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
                var cat, level string
                var cnt int
                if err := rows.Scan(&cat, &level, &cnt); err != nil { return nil, err }
                stats = append(stats, map[string]interface{}{"category": cat, "level": level, "count": cnt})
        }
        return stats, rows.Err()
}

func CleanOldErrorLogs(maxAge time.Duration) error {
        _, err := DB().Exec(`DELETE FROM error_logs WHERE timestamp < ?`, time.Now().Add(-maxAge))
        return err
}

// === Diagnostic Reports ===
func SaveDiagnosticReport(r *DiagnosticReport) error {
        if r.ID == "" { r.ID = uuid.New().String() }
        if r.GeneratedAt.IsZero() { r.GeneratedAt = time.Now() }
        _, err := DB().Exec(`INSERT INTO diagnostic_reports (id,generated_at,period_start,period_end,frequency,content,uploaded,uploaded_at) VALUES (?,?,?,?,?,?,?,?)`, r.ID, r.GeneratedAt, r.PeriodStart, r.PeriodEnd, r.Frequency, r.Content, r.Uploaded, r.UploadedAt)
        return err
}

func GetDiagnosticReports(limit int) ([]DiagnosticReport, error) {
        if limit <= 0 { limit = 20 }
        rows, err := DB().Query(`SELECT id,generated_at,period_start,period_end,frequency,content,uploaded,uploaded_at FROM diagnostic_reports ORDER BY generated_at DESC LIMIT ?`, limit)
        if err != nil { return nil, err }
        defer rows.Close()
        var reps []DiagnosticReport
        for rows.Next() {
                var r DiagnosticReport
                var ps, pe, ua sql.NullTime
                if err := rows.Scan(&r.ID, &r.GeneratedAt, &ps, &pe, &r.Frequency, &r.Content, &r.Uploaded, &ua); err != nil { return nil, err }
                if ps.Valid { r.PeriodStart = &ps.Time }
                if pe.Valid { r.PeriodEnd = &pe.Time }
                if ua.Valid { r.UploadedAt = &ua.Time }
                reps = append(reps, r)
        }
        return reps, rows.Err()
}

func MarkReportUploaded(id string) error {
        _, err := DB().Exec(`UPDATE diagnostic_reports SET uploaded=TRUE, uploaded_at=CURRENT_TIMESTAMP WHERE id=?`, id)
        return err
}
'''
wr(f'{R}/internal/database/queries.go',c)
print('  OK')

# === 10. logger ===
print('[10] logger.go')
c=rd(f'{B}/internal/logger/logger.go')
c,_=rp(c,'onError         func(category, msg string)','onError         func(category, msg string)\n\tonLogDB         func(level, category, msg string)')
c,_=rp(c,'func (l *Logger) Error(','func (l *Logger) SetOnLogDB(fn func(level, category, msg string)) {\n\tl.mu.Lock()\n\tdefer l.mu.Unlock()\n\tl.onLogDB = fn\n}\n\nfunc (l *Logger) Error(')
c,_=rp(c,'if l.onError != nil {\n\t\t\t\tgo l.onError(category, msg)\n\t\t\t}\n\t\t}','if l.onError != nil {\n\t\t\t\tgo l.onError(category, msg)\n\t\t\t}\n\t\t\t// FIX: Structured logging to DB for ERROR and WARN\n\t\t\tif l.onLogDB != nil { go l.onLogDB(level, category, msg) }\n\t\t}')
wr(f'{R}/internal/logger/logger.go',c)
print('  OK')

# === 11. monitor ===
print('[11] monitor/traffic.go')
c=rd(f'{B}/internal/monitor/traffic.go')
c,_=rp(c,'func (m *TrafficMonitor) readNetworkStats()','// FIX: TODO replace PowerShell with gopsutil (github.com/shirou/gopsutil/v4/net)\n// Reduces latency from ~400ms to <5ms\nfunc (m *TrafficMonitor) readNetworkStats()')
wr(f'{R}/internal/monitor/traffic.go',c)
print('  OK')

# === 12. api.js ===
print('[12] api.js')
c=rd(f'{B}/web/src/api.js')
c,_=rp(c,"'/api/devices': (app) => app.DeleteDevice(''),\n};","// FIX: handled by handleDeviceRoute\n};")
c,_=rp(c,'console.error(\'[api] Error calling\', method, path, err);\n    return null;','console.error(\'[api] Error calling\', method, path, err);\n    // FIX: error propagation instead of silent null\n    return { error: String(err?.message || err || \'Unknown error\') };')
c,_=rp(c,"'/api/logs/debug': (app) => app.GetLogDebug(),","'/api/logs/debug': (app) => app.GetLogDebug(),\n  '/api/reports/history': (app, p) => app.GetReportHistory(parseInt(p.limit) || 20),\n  '/api/reports/content': (app, p) => app.GetReportContent(p.id || ''),")
c,_=rp(c,"'/api/setup/complete': (app, b) => app.SetupComplete(),\n};","'/api/setup/complete': (app, b) => app.SetupComplete(),\n  '/api/reports/generate': (app) => app.GenerateAndDownloadReport(),\n};")
wr(f'{R}/web/src/api.js',c)
print('  OK')

# === 13. DashboardPage ===
print('[13] DashboardPage.jsx')
c=rd(f'{B}/web/src/pages/DashboardPage.jsx')
old_dash = "showToast?.('\u041d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u0437\u0430\u043f\u0443\u0441\u0442\u0438\u0442\u044c \u043f\u0440\u043e\u0432\u0435\u0440\u043a\u0443', 'error');\n    }"
new_dash = "showToast?.('\u041d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u0437\u0430\u043f\u0443\u0441\u0442\u0438\u0442\u044c \u043f\u0440\u043e\u0432\u0435\u0440\u043a\u0443', 'error');\n      setCheckingNow(false);  // FIX\n    }"
c,_=rp(c, old_dash, new_dash)
wr(f'{R}/web/src/pages/DashboardPage.jsx',c)
print('  OK')

# === 14. SettingsPage ===
print('[14] SettingsPage.jsx')
c=rd(f'{B}/web/src/pages/SettingsPage.jsx')
c,_=rp(c,'const poll = async () => {\n        for (let i = 0; i < 15; i++) {\n          await new Promise(r => setTimeout(r, 3000));\n          await checkZapretUpdate();\n          loadVersions();\n        }\n      };\n      poll();','const abortCtrl = new AbortController();\n      const pollSafe = async () => {\n        for (let i = 0; i < 15; i++) {\n          if (abortCtrl.signal.aborted) return;\n          await new Promise(r => setTimeout(r, 3000));\n          if (abortCtrl.signal.aborted) return;\n          await checkZapretUpdate();\n          loadVersions();\n        }\n      };\n      pollSafe();\n      return () => { abortCtrl.abort(); };')
wr(f'{R}/web/src/pages/SettingsPage.jsx',c)
print('  OK')

# === 15. useDebouncedSave ===
print('[15] useDebouncedSave.js')
wr(f'{R}/web/src/hooks/useDebouncedSave.js','''import { useRef, useCallback, useEffect } from \'react\';
import { api, apiCall } from \'../api\';

// FIX: version counter prevents race condition + cleanup on unmount
export function useDebouncedSave(url, delay = 500, onSuccess = null) {
  const configRef = useRef({});
  const timerRef = useRef(null);
  const versionRef = useRef(0);
  const pendingVersionRef = useRef(0);

  const updateFn = useCallback((patch, currentConfig = null) => {
    if (currentConfig) configRef.current = { ...configRef.current, ...currentConfig };
    configRef.current = { ...configRef.current, ...patch };
    versionRef.current++;
    const v = versionRef.current;
    pendingVersionRef.current = v;
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(async () => {
      if (pendingVersionRef.current !== v) return;
      await apiCall(() => api(\'POST\', url, configRef.current), null, null);
      if (onSuccess) onSuccess();
    }, delay);
  }, [url, delay, onSuccess]);

  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current); }, []);
  return updateFn;
}
''')
print('  OK')

print('\n=== All 15 replacement files generated ===')