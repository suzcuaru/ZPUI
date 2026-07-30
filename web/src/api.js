/**
 * api.js — Shim-слой: маршрутизация старых HTTP-вызовов через Wails bindings.
 *
 * Фронтенд вызывает api('GET', '/api/status') как и раньше,
 * а внутри вызов перенаправляется на window.go.app.App.Method().
 *
 * Также предоставляет createStream() для замены EventSource (SSE → Wails Events).
 */

import { tr } from './i18n';

// ─── Helpers ──────────────────────────────────────────

function wailsApp() {
  if (typeof window !== 'undefined' && window.go) {
    if (window.go.app && window.go.app.App) return window.go.app.App;
    if (window.go.main && window.go.main.App) return window.go.main.App;
  }
  return null;
}

function wailsRuntime() {
  if (typeof window !== 'undefined' && window.runtime) {
    return window.runtime;
  }
  return null;
}

/** Парсит query-параметры из path (напр. "/api/logs?category=main&lines=50"). */
function parseQuery(path) {
  const qIndex = path.indexOf('?');
  const params = {};
  if (qIndex === -1) return { base: path, params };
  const base = path.substring(0, qIndex);
  const search = path.substring(qIndex + 1);
  for (const pair of search.split('&')) {
    const [k, v] = pair.split('=');
    if (k) params[decodeURIComponent(k)] = decodeURIComponent(v || '');
  }
  return { base, params };
}

// ─── Route Map ────────────────────────────────────────

/**
 * Маршруты GET-запросов.
 * Ключ — путь, значение — функция(App, params) → Promise<any>.
 */
const GET_ROUTES = {
  '/api/status': (app) => app.GetStatus(),
  '/api/system-theme': (app) => app.GetSystemTheme(),
  '/api/zapret/strategies': (app) => app.GetStrategies(),
  '/api/zapret/service/status': (app) => app.GetServiceStatus(),
  '/api/zapret/game-filter': (app) => app.GetGameFilter(),
  '/api/zapret/ipset-status': (app) => app.GetIpsetStatus(),
  '/api/zapret/auto-update-status': (app) => app.GetAutoUpdateStatus(),
  '/api/zapret/diagnostics': (app) => app.RunDiagnostics(),
  '/api/zapret/lists': (app) => app.GetLists(),
  '/api/proxy/status': (app) => app.GetProxyStatus(),
  '/api/proxy/connections': (app) => app.GetProxyConnections(),
  '/api/proxy/config': (app) => app.GetProxyConfig(),
  '/api/proxy/qrcode': (app) => app.GetProxyQRCode(),
  '/api/xbox-dns/config': (app) => app.GetXboxDnsConfig(),
  '/api/monitor/devices': (app) => app.GetMonitorDevices(),
  '/api/system-resources': (app) => app.GetSystemResources(),
  '/api/autostart/status': (app) => app.GetAutostartStatus(),
  '/api/logs': (app, p) => app.GetLogs(p.table || 'app', p.level || '', parseInt(p.limit) || 200, parseInt(p.offset) || 0),
  '/api/logs/all': (app, p) => app.GetAllLogs(p.level || '', parseInt(p.limit) || 200, parseInt(p.offset) || 0),
  '/api/logs/stats': (app) => app.GetLogStats(),
  '/api/logs/count': (app, p) => app.GetLogCount(p.table || 'all', p.level || ''),
  '/api/logs/tables': (app) => app.GetLogTableNames(),
  '/api/logs/find': (app, p) => app.FindLogEntryByMsg(p.table || 'app', p.query || ''),
  '/api/config': (app) => app.GetConfig(),
  '/api/resource-status': (app) => app.GetResourceStatus(),
  '/api/resource-status/refresh': (app) => app.RefreshResourceStatus(),
  '/api/up/info': (app) => app.GetUpInfo(),
  '/api/devices': (app) => app.GetDevices(),
  '/api/updates/check/zpui': (app) => app.CheckZPUIUpdate(),
  '/api/updates/versions-panel': (app) => app.CheckVersionsPanel(),
  '/api/updates/zapret-versions-panel': (app) => app.CheckZapretVersionsPanel(),
  '/api/updates/zapret-release-notes': (app) => app.GetZapretReleaseInfo(),
  '/api/updates/downloaded': (app) => app.IsUpdateDownloaded(),
  '/api/updates/check/zapret': (app) => app.CheckForUpdates(),
  '/api/updates/release-notes': (app) => app.GetReleaseNotes(),
  '/api/updates/prepare': (app) => app.PrepareZPUIUpdate(),
  '/api/zapret/local': (app) => app.HasLocalZapret(),
  '/api/zapret/verify-files': (app) => app.VerifyZapretFiles(),
  '/api/zapret/system-service': (app) => app.HasSystemZapretService(),
  '/api/zapret/install-log': (app) => app.GetInstallLog(),
  '/api/zapret/default-strategy': (app) => app.DefaultStrategy(),
  '/api/zapret/auto-test-results': (app) => app.GetAutoTestResults(),
  '/api/zapret/skip-resources': (app) => app.GetSkipResources(),
  '/api/zapret/service-installed': (app) => app.IsServiceInstalled(),
  '/api/versions': (app) => app.GetVersions(),
  '/api/components/check': (app) => app.CheckComponentUpdates(),
  '/api/health': (app) => app.HealthCheck(),
  '/api/zapret/service-health': (app) => app.CheckServiceHealth(),
  '/api/zapret/autoselect/last-check': (app) => app.GetAutoSelectLastCheck(),
  '/api/zapret/autoswitch/status': (app) => app.GetAutoSwitchStatus(),
  '/api/zapret/autoswitch/history': (app) => app.GetAutoSwitchHistory(),
  '/api/zapret/strategy-changes': (app) => app.GetStrategyChangeHistory(),
  '/api/network-info': (app) => app.GetNetworkInfo(),
  '/api/isp/current': (app) => app.GetCurrentOperator(),
  '/api/isp/history': (app) => app.GetOperatorHistory(),
  '/api/availability/history': (app, p) => app.GetAvailabilityHistory(parseInt(p.hours) || 24, p.type || ''),
  '/api/backups': (app, p) => app.GetBackups(p.component || ''),
  '/api/ignored-versions': (app) => app.GetIgnoredVersions(),
  '/api/logs/errors': (app) => app.GetErrorSnapshots(),
  '/api/logs/error': (app, p) => app.ReadErrorSnapshot(p.name || ''),

  '/api/logs/debug': (app) => app.GetLogDebug(),
  '/api/setup/detect-thirdparty': (app) => app.DetectThirdPartyZapret(),
  '/api/setup/strategies': (app, p) => app.SetupListStrategies(p.strategy || ''),
  '/api/setup/test-strategy': (app, p) => app.SetupTestStrategy(p.strategy || ''),
  '/api/setup/control-check': (app) => app.SetupControlCheck(),
  '/api/report/generate': (app) => app.GenerateReport(),
  '/api/report/get': (app) => app.GetReport(),
};

/**
 * Маршруты POST-запросов.
 * Ключ — путь, значение — функция(App, body) → Promise<any>.
 */
const POST_ROUTES = {
  '/api/zapret/start': (app) => app.ZapretStart(),
  '/api/zapret/stop': (app) => app.ZapretStop(),
  '/api/zapret/restart': (app) => app.ZapretRestart(),
  '/api/zapret/set-strategy': (app, b) => app.SetStrategy(b.filename || ''),
  '/api/zapret/service/install': (app, b) => app.InstallService(b.strategy || ''),
  '/api/zapret/service/remove': (app) => app.RemoveService(),
  '/api/zapret/game-filter': (app, b) => app.SetGameFilter(b.mode || ''),
  '/api/zapret/ipset-toggle': (app, b) => app.ToggleIpset(b.mode || ''),
  '/api/zapret/auto-update-toggle': (app, b) => app.ToggleAutoUpdate(!!b.enabled),
  '/api/zapret/update-ipset': (app) => app.UpdateIpset(),
  '/api/zapret/update-hosts': (app) => app.UpdateHosts(),
  '/api/zapret/install': (app, b) => app.InstallZapret(b.source_dir || ''),
  '/api/zapret/cache/clear': (app, b) => app.ClearCache(b.target || ''),
  '/api/zapret/lists/save': (app, b) => app.SaveList(b.name || '', b.content || ''),
  '/api/zapret/skip-resources/save': (app, b) => app.SaveSkipResources(b.content || ''),
  '/api/zapret/skip-resources/add': (app, b) => app.AddSkipResource(b.host || ''),
  '/api/zapret/full-reinstall': (app) => app.FullReinstall(),
  '/api/proxy/start': (app) => app.ProxyStart(),
  '/api/proxy/stop': (app) => app.ProxyStop(),
  '/api/proxy/config': (app, b) => app.SetProxyConfig(b || {}),
  '/api/xbox-dns/config': (app, b) => app.SetXboxDnsConfig(b || {}),
  '/api/report/save': (app, b) => app.SaveReport(b || {}),
  '/api/component-states': (app) => app.SaveComponentStates(),
  '/api/resource-check': (app, b) => app.CheckResource(b.url || ''),
  '/api/resource-add': (app, b) => app.AddHostToUserList(b.host || ''),
  '/api/update/check': (app) => app.CheckForUpdates(),
  '/api/update/apply': (app) => app.ApplyUpdate(),
  '/api/strategy/auto': (app) => app.StartAutoTest(),
  '/api/strategy/cancel': (app) => app.CancelAutoTest(),
  '/api/autostart/enable': (app) => app.EnableAutostart(),
  '/api/autostart/disable': (app) => app.DisableAutostart(),
  '/api/app/restart': (app) => app.RestartApp(),
  '/api/logs/frontend': (app, b) => app.FrontendLogs(b.events || []),
  '/api/config': (app, b) => app.SetConfig(b || {}),
  '/api/external': (app, b) => app.OpenExternal(b.url || ''),
  '/api/zapret/remove-system-service': (app) => app.RemoveSystemZapretService(),
  '/api/updates/launch-updater': (app) => app.LaunchZPUIUpdater(),
  '/api/updates/download': (app) => app.DownloadZPUIUpdate(),
  '/api/updates/apply': (app) => app.ApplyDownloadedZPUIUpdate(),
  '/api/restore-backup': (app, b) => app.RestoreFromBackup(b.name || ''),
  '/api/ignore-version': (app, b) => app.AddIgnoredVersion(b.component || '', b.version || '', b.reason || ''),
  '/api/unignore-version': (app, b) => app.RemoveIgnoredVersion(b.component || '', b.version || ''),
  '/api/zapret/auto-install': (app) => app.AutoInstallZapret(),
  '/api/zapret/install-service-logged': (app, b) => app.InstallServiceLogged(b.strategy || ''),
  '/api/zapret/autoselect': (app) => app.RunAutoSelectStream(),
  '/api/zapret/autoswitch/toggle': (app, b) => app.SetAutoSwitchEnabled(!!b.enabled),
  '/api/zapret/autoswitch/config': (app, b) => app.SetAutoSwitchConfig(b.threshold, b.interval),
  '/api/logs/clear': (app) => app.ClearLogs(),
  '/api/logs/clear-bucket': (app, b) => app.ClearLogBucket(b.table || 'all'),
  '/api/logs/export': (app) => app.ExportLogs(),
  '/api/logs/error/delete': (app, b) => app.DeleteErrorSnapshot(b.name || ''),
  '/api/logs/debug': (app, b) => app.SetLogDebug(b.category || '', b.enabled === true),
  '/api/logs/error-code': (app, b) => app.LogErrorCode(b.code || '', b.category || '', b.message || ''),
  '/api/test-notification': (app) => app.SendTestNotification(),
  '/api/setup/remove-thirdparty': (app) => app.RemoveThirdPartyZapret(),
  '/api/setup/install': (app) => app.InstallOurZapret(),
  '/api/setup/start': (app) => app.StartOurZapret(),
  '/api/setup/apply-strategy': (app, b) => app.SetupApplyStrategy(b.strategy || ''),
  '/api/setup/configure-filters': (app, b) => app.SetupConfigureFilters(b.mode || ''),
  '/api/setup/configure-dns': (app, b) => app.SetupConfigureDNS(b.enable === true),
  '/api/setup/configure-proxy': (app, b) => app.SetupConfigureProxy(b.enable === true, parseInt(b.port) || 1080, b.bind_host || ''),
  '/api/setup/skip': (app) => app.SetupSkip(),
  '/api/setup/complete': (app) => app.SetupComplete(),
};

const DELETE_ROUTES = {
  '/api/devices': (app) => app.DeleteDevice(''),
};

// ─── Device sub-routes (path parameters) ──────────────

function handleDeviceRoute(app, method, base, params, body) {
  // /api/devices/{mac}
  // /api/devices/{mac}/connections
  // /api/devices/{mac}/ping
  const parts = base.replace('/api/devices/', '').split('/');
  const mac = decodeURIComponent(parts[0] || '');

  if (method === 'GET' && parts.length === 1) {
    return app.GetDevice(mac);
  }
  if (method === 'GET' && parts[1] === 'connections') {
    return app.GetDeviceConnections(mac, parseInt(params.limit) || 50, parseInt(params.offset) || 0);
  }
  if (method === 'POST' && parts[1] === 'ping') {
    return app.PingDevice(mac);
  }
  if (method === 'DELETE' && parts.length === 1) {
    return app.DeleteDevice(mac);
  }
  return Promise.resolve(null);
}

// ─── Main API function ────────────────────────────────

export async function api(method, path, body) {
  const app = wailsApp();
  if (!app) {
    console.warn('[api] Wails runtime not available');
    return null;
  }

  method = method.toUpperCase();
  const { base, params } = parseQuery(path);

  try {
    // Device sub-routes with path parameters
    if (base.startsWith('/api/devices/') && base !== '/api/devices') {
      return await handleDeviceRoute(app, method, base, params, body);
    }

    let result;
    switch (method) {
      case 'GET':
        if (GET_ROUTES[base]) result = await GET_ROUTES[base](app, params);
        else { console.warn('[api] Unknown GET route:', base); return null; }
        break;
      case 'POST':
        if (POST_ROUTES[base]) result = await POST_ROUTES[base](app, body || {});
        else { console.warn('[api] Unknown POST route:', base); return null; }
        break;
      case 'DELETE':
        if (DELETE_ROUTES[base]) result = await DELETE_ROUTES[base](app);
        else if (base.startsWith('/api/devices/')) result = await handleDeviceRoute(app, 'DELETE', base, params, body);
        else { console.warn('[api] Unknown DELETE route:', base); return null; }
        break;
      default:
        console.warn('[api] Method not supported:', method);
        return null;
    }
    return result;
  } catch (err) {
    console.error('[api] Error calling', method, path, err);
    return null;
  }
}

// ─── apiCall helper (unchanged) ───────────────────────

export async function apiCall(fn, successMsg, showToast) {
  try {
    const result = await fn();
    if (result?.error) {
      if (showToast) showToast(result.error, 'error');
      return false;
    }
    if (successMsg && showToast) showToast(successMsg, 'success');
    return true;
  } catch {
    if (showToast) showToast(tr('toast.requestFailed'), 'error');
    return false;
  }
}

// ─── openExternal ─────────────────────────────────────

export async function openExternal(url) {
  try {
    const app = wailsApp();
    if (app) await app.OpenExternal(url);
  } catch {}
}

export function cancelAutoTest() {
  try { const app = wailsApp(); if (app) app.CancelAutoTest(); } catch {}
}

// ─── SSE replacement (EventSource → Wails Events) ─────

/**
 * createStream — замена new EventSource(path) для Wails.
 *
 * Использование (совместимо со старым кодом):
 *   const es = createStream('/api/strategy/stream');
 *   es.onmessage = (e) => { const d = JSON.parse(e.data); ... };
 *   es.onerror = () => { ... };
 *   es.close();
 */
export function createStream(path) {
  const rt = wailsRuntime();
  const app = wailsApp();

  const stream = {
    _onmessage: null,
    _onerror: null,
    _started: false,
    _eventNames: [],

    get onmessage() { return this._onmessage; },
    set onmessage(fn) {
      this._onmessage = fn;
      this._tryStart();
    },
    get onerror() { return this._onerror; },
    set onerror(fn) { this._onerror = fn; },

    _tryStart() {
      if (this._started || !this._onmessage) return;
      this._started = true;

      if (path === '/api/strategy/stream') {
        // Стратегия: автотест
        rt.EventsOn('strategy:event', (data) => {
          if (this._onmessage) this._onmessage({ data: JSON.stringify(data) });
        });
        this._eventNames.push('strategy:event');

        rt.EventsOn('strategy:done', (data) => {
          if (this._onmessage) this._onmessage({ data: JSON.stringify({ ...(data || {}), type: 'done' }) });
        });
        this._eventNames.push('strategy:done');

        // Запускаем бэкенд-метод
        app.RunAutoTestStream().catch(err => {
          if (this._onerror) this._onerror(err);
        });

      } else if (path === '/api/update/stream') {
        // Обновление
        rt.EventsOn('update:progress', (data) => {
          if (this._onmessage) this._onmessage({ data: JSON.stringify(data) });
        });
        this._eventNames.push('update:progress');

        rt.EventsOn('update:done', (data) => {
          // Эмитим финальное сообщение с percent=100
          if (this._onmessage) this._onmessage({ data: JSON.stringify({ percent: 100, step: tr('common.completed') }) });
        });
        this._eventNames.push('update:done');

        // Запускаем бэкенд-метод
        app.RunUpdateStream().catch(err => {
          if (this._onerror) this._onerror(err);
        });

      } else if (path === '/api/autoselect/stream') {
        // Автоподбор лучшей стратегии с применением
        console.log('[STREAM] subscribing to autoselect:event and autoselect:done');
        rt.EventsOn('autoselect:event', (data) => {
          console.log('[STREAM] autoselect:event received:', data?.type, data?.strategy || '');
          if (this._onmessage) this._onmessage({ data: JSON.stringify(data) });
        });
        this._eventNames.push('autoselect:event');

        rt.EventsOn('autoselect:done', (data) => {
          console.log('[STREAM] autoselect:done received:', data);
          if (this._onmessage) this._onmessage({ data: JSON.stringify({ ...(data || {}), type: 'done' }) });
        });
        this._eventNames.push('autoselect:done');

        // Запускаем бэкенд-метод
        console.log('[STREAM] calling RunAutoSelectStream()');
        app.RunAutoSelectStream().catch(err => {
          console.error('[STREAM] RunAutoSelectStream error:', err);
          if (this._onerror) this._onerror(err);
        });
      }
    },

    close() {
      console.log('[STREAM] close() called for', path, '— unsubscribing', this._eventNames.length, 'events');
      for (const name of this._eventNames) {
        try { rt.EventsOff(name); } catch {}
      }
      if (path === '/api/strategy/stream' || path === '/api/autoselect/stream') {
        try { app.CancelAutoTest(); } catch {}
      }
      this._eventNames = [];
    },
  };

  return stream;
}
