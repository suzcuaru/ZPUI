/**
 * api.js — Shim-слой: маршрутизация старых HTTP-вызовов через Wails bindings.
 *
 * Фронтенд вызывает api('GET', '/api/status') как и раньше,
 * а внутри вызов перенаправляется на window.go.app.App.Method().
 *
 * Также предоставляет createStream() для замены EventSource (SSE → Wails Events).
 */

import { tr } from './i18n';
import { subscribeEvent } from './events';

// ─── Helpers ──────────────────────────────────────────

function wailsApp() {
  if (typeof window !== 'undefined' && window.go) {
    if (window.go.app && window.go.app.App) return window.go.app.App;
    if (window.go.main && window.go.main.App) return window.go.main.App;
  }
  return null;
}

/**
 * withTimeout — ограничивает ожидание биндинга. Зависший вызов бэкенда
 * (нагрузка/дедлок) иначе висит вечно и молча ломает опрос статуса.
 * По таймауту возвращается null («нет данных»), отклонение проксируется.
 */
function withTimeout(promise, ms) {
  if (!ms) return promise;
  return new Promise((resolve, reject) => {
    let settled = false;
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      console.warn('[api] Binding call timed out after', ms, 'ms');
      resolve(null);
    }, ms);
    promise.then(
      (v) => { if (!settled) { settled = true; clearTimeout(timer); resolve(v); } },
      (e) => { if (!settled) { settled = true; clearTimeout(timer); reject(e); } },
    );
  });
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
  '/api/zapret/game-filter': (app) => app.GetGameFilter(),
  '/api/zapret/game-filter/ports': (app) => app.GetGameFilterPorts(),
  '/api/zapret/ipset-status': (app) => app.GetIpsetStatus(),
  '/api/zapret/diagnostics': (app) => app.RunDiagnostics(),
  '/api/zapret/lists': (app) => app.GetLists(),
  '/api/tgproxy/status': (app) => app.GetTgProxyStatus(),
  '/api/tgproxy/config': (app) => app.GetTgProxyConfig(),
  '/api/envscan/scan': (app) => app.ScanEnvironment(),
  '/api/monitor/devices': (app) => app.GetMonitorDevices(),
  '/api/monitor/traffic-history': (app) => app.GetTrafficHistory(),
  '/api/logs': (app, p) => app.GetLogs(p.table || 'app', p.level || '', parseInt(p.limit) || 200, parseInt(p.offset) || 0),
  '/api/logs/all': (app, p) => app.GetAllLogs(p.level || '', parseInt(p.limit) || 200, parseInt(p.offset) || 0),
  '/api/logs/stats': (app) => app.GetLogStats(),
  '/api/config': (app) => app.GetConfig(),
  '/api/custom-theme': (app) => app.GetCustomTheme(),
  '/api/custom-themes': (app) => app.GetCustomThemes(),
  '/api/resource-status': (app) => app.GetResourceStatus(),
  '/api/resource-status/refresh': (app) => app.RefreshResourceStatus(),
  '/api/devices': (app) => app.GetDevices(),
  '/api/updates/check/zpui': (app) => app.CheckZPUIUpdate(),
  '/api/updates/versions-panel': (app) => app.CheckVersionsPanel(),
  '/api/updates/zapret-versions-panel': (app) => app.CheckZapretVersionsPanel(),
  '/api/updates/zapret-release-notes': (app, p) => app.GetZapretReleaseInfo(p.version || ''),
  '/api/updates/check/zapret': (app) => app.CheckForUpdates(),
  '/api/updates/release-notes': (app, p) => app.GetReleaseNotes(p.version || ''),
  '/api/updates/source': (app) => app.GetUpdatesSource(),
  '/api/zapret/local': (app) => app.HasLocalZapret(),
  '/api/zapret/system-service': (app) => app.HasSystemZapretService(),
  '/api/zapret/install-log': (app) => app.GetInstallLog(),
  '/api/zapret/default-strategy': (app) => app.DefaultStrategy(),
  '/api/zapret/auto-test-results': (app) => app.GetAutoTestResults(),
  '/api/zapret/skip-resources': (app) => app.GetSkipResources(),
  '/api/versions': (app) => app.GetVersions(),
  '/api/strategy/test-single-result': (app) => app.GetSingleTestResult(),
  '/api/health': (app) => app.HealthCheck(),
  '/api/zapret/service-health': (app) => app.CheckServiceHealth(),
  '/api/zapret/autoswitch/status': (app) => app.GetAutoSwitchStatus(),
  '/api/zapret/strategy-changes': (app) => app.GetStrategyChangeHistory(),
  '/api/isp/current': (app) => app.GetCurrentOperator(),
  '/api/isp/history': (app) => app.GetOperatorHistory(),
  '/api/availability/history': (app, p) => app.GetAvailabilityHistory(parseInt(p.hours) || 24, p.type || ''),
  '/api/yandex/metrics': (app, b) => app.GetYandexNetMetrics((b && b.minutes) || 0),
  '/api/sync/info': (app) => app.GetSyncInfo(),
  '/api/sync/history': (app, p) => app.GetSyncHistory((p && p.period) || 'today'),
  '/api/server/status': (app) => app.GetServerStatus(),
  '/api/server/ping': (app) => app.PingServer(),
  '/api/account/me': (app) => app.AccountMe(),
  '/api/account/device-accounts': (app) => app.V4DeviceAccounts(),
  '/api/account/totp/qr': (app, p) => app.V4TOTPQR((p && p.uri) || ''),
  '/api/account/sync/devices': (app) => app.DeviceSyncList(),
  '/api/autoselect/cfg': (app) => app.AutoselectCfgInfo(),
  '/api/banner': (app) => app.V4BannerGet(),
  '/api/subscriptions': (app) => app.V4SubscriptionsList(),
  '/api/account/devices': (app) => app.V4DevicesList(),
  '/api/dns/config': (app) => app.GetDnsConfig(),
  '/api/dns/remote': (app) => app.GetDnsRemoteProviders(),
  '/api/dns/current': (app) => app.GetCurrentDns(),
  '/api/socks/status': (app) => app.GetSocksStatus(),
  '/api/mcp/info': (app) => app.GetMcpInfo(),
  '/api/extension/info': (app) => app.GetExtensionInfo(),
  '/api/system/hvci': (app) => app.GetHvciFix(),
  '/api/zapret/engine': (app) => app.GetZapretEngine(),
  '/api/zapret/expert-settings': (app) => app.GetZapret2Settings(),
  '/api/zapret/strategy-content': (app, p) => app.GetStrategyContent((p && p.filename) || ''),
  '/api/zapret/strategy-lua': (app, p) => app.GetStrategyLuaToggles((p && p.filename) || ''),
  '/api/zapret/strategy-validate-live': (app, p) => app.ValidateStrategyLive((p && p.filename) || ''),
  '/api/zapret/lua-scripts': (app) => app.GetLuaScripts(),
  '/api/zapret/lua-script': (app, p) => app.GetLuaScript((p && p.scope) || 'user', (p && p.name) || ''),
  '/api/service/panel': (app) => app.GetZsvcPanel(),
  '/api/startup/ready': (app) => app.GetStartupReady(),
  '/api/zapret/service-autostart': (app) => app.GetZapretServiceAutostart(),
  '/api/resources/details': (app, p) => app.ResourceDetails((p && p.type) || 'standard'),
  '/api/cache/info': (app) => app.GetCacheInfo(),
  '/api/game-optimize': (app) => app.GetGameOptimize(),
};

/**
 * Маршруты POST-запросов.
 * Ключ — путь, значение — функция(App, body) → Promise<any>.
 */
const POST_ROUTES = {
  '/api/zapret/start': (app) => app.ZapretStart(),
  '/api/zapret/stop': (app) => app.ZapretStop(),
  '/api/zapret/startup-diagnostics': (app) => app.RunStartupDiagnostics(),
  '/api/zapret/set-strategy': (app, b) => app.SetStrategy(b.filename || ''),
  '/api/zapret/service/install': (app, b) => app.InstallService(b.strategy || ''),
  '/api/zapret/service/remove': (app) => app.RemoveService(),
  '/api/zapret/service-autostart': (app, b) => app.SetZapretServiceAutostart(!!(b && b.enabled)),
  '/api/zapret/game-filter': (app, b) => app.SetGameFilter((b && b.mode) || '', !!(b && b.force)),
  '/api/zapret/game-filter/ports': (app, b) => app.SetGameFilterPorts((b && b.tcp_ports) || '', (b && b.udp_ports) || ''),
  '/api/zapret/ipset-toggle': (app, b) => app.ToggleIpset(b.mode || ''),
  '/api/zapret/cache/clear': (app, b) => app.ClearZapretCache((b && b.target) || ''),
  '/api/zapret/lists/save': (app, b) => app.SaveList(b.name || '', b.content || ''),
  '/api/zapret/skip-resources/save': (app, b) => app.SaveSkipResources(b.content || ''),
  '/api/zapret/skip-resources/add': (app, b) => app.AddSkipResource(b.host || ''),
  '/api/lists/refresh': (app) => app.RefreshServerLists(),
  '/api/zapret/full-reinstall': (app) => app.FullReinstall(),
  '/api/tgproxy/config': (app, b) => app.SetTgProxyConfig(b || {}),
  '/api/tgproxy/start': (app) => app.TgProxyStart(),
  '/api/tgproxy/stop': (app) => app.TgProxyStop(),
  '/api/tgproxy/regenerate-secret': (app) => app.RegenerateTgProxySecret(),
  '/api/socks/port': (app, b) => app.SetSocksPort(Number(b && b.port) || 1080),
  '/api/socks/enabled': (app, b) => app.SetSocksEnabled(!!(b && b.on)),
  '/api/socks/static-ip': (app, b) => app.SetProxyStaticIP(!!(b && b.on)),
  '/api/mcp/enabled': (app, b) => app.SetMcpEnabled(!!(b && b.on)),
  '/api/mcp/port': (app, b) => app.SetMcpPort(Number(b && b.port) || 0),
  '/api/mcp/regen-key': (app) => app.RegenerateMcpKey(),
  '/api/mcp/open-skill-folder': (app) => app.OpenMcpSkillFolder(),
  // ── Браузерный плагин: пакет из components\extension\ в Chromium-браузеры ──
  '/api/extension/install': (app, b) => app.InstallExtension((b && b.ids) || []),
  '/api/extension/uninstall': (app, b) => app.UninstallExtension((b && b.ids) || []),
  '/api/extension/open': (app, b) => app.OpenExtensionPage((b && b.id) || ''),
  '/api/extension/update': (app, b) => app.UpdateExtension(...((b && b.ids) || [])),
  '/api/system/fix-hvci': (app) => app.FixHvci(),
  '/api/zapret/install-engine': (app) => app.InstallZapretEngine(),
  '/api/zapret/restart': (app) => app.ZapretRestart(),
  '/api/zapret/expert-settings': (app, b) => app.SetZapret2Settings(b || {}),
  '/api/zapret/save-strategy': (app, b) => app.SaveStrategyContent((b && b.filename) || '', (b && b.content) || ''),
  '/api/zapret/copy-strategy': (app, b) => app.CopyStrategy((b && b.filename) || '', (b && b.name) || ''),
  '/api/zapret/validate-strategy': (app, b) => app.ValidateStrategy((b && b.content) || ''),
  '/api/zapret/strategy-lua/toggle': (app, b) => app.SetStrategyLuaToggle((b && b.filename) || '', (b && b.fn) || '', !!(b && b.enabled)),
  '/api/zapret/lua-script/save': (app, b) => app.SaveLuaScript((b && b.scope) || 'user', (b && b.name) || '', (b && b.content) || ''),
  '/api/zapret/lua-script/create': (app, b) => app.CreateLuaScript((b && b.name) || ''),
  '/api/zapret/lua-script/delete': (app, b) => app.DeleteLuaScript((b && b.name) || ''),
  // ── Аккаунт (конверт ZPUI Sync; email-2FA — второй шаг входа) ──
  '/api/auth/email/login': (app, b) => app.V4Login((b && b.login) || '', (b && b.password) || '', (b && b.code) || ''),
  '/api/auth/login/verify': (app, b) => app.V4LoginVerify((b && b.email) || '', (b && b.code) || '', (b && b.login_token) || ''),
  '/api/auth/email/register': (app, b) => app.V4RegisterStart((b && b.login) || '', (b && b.email) || '', (b && b.password) || '', (b && b.keyword) || ''),
  '/api/auth/email/register/verify': (app, b) => app.V4RegisterFinish((b && b.reg_token) || '', (b && b.code) || ''),
  '/api/auth/email/totp/confirm': (app, b) => app.V4TotpConfirm((b && b.login) || '', (b && b.password) || '', (b && b.code) || ''),
  '/api/auth/email/forgot': (app, b) => app.V4RecoveryStart((b && b.login) || '', (b && b.keyword) || ''),
  '/api/auth/email/forgot/verify': (app, b) => app.V4RecoveryFinish((b && b.reset_token) || '', (b && b.new_password) || ''),
  '/api/auth/logout': (app) => app.V4Logout(),
  '/api/auth/prompt/dismissed': (app, b) => app.SetAuthPromptDismissed(!!(b && b.on)),
  '/api/account/nick': (app, b) => app.V4NickChange((b && b.nick) || '', (b && b.code) || ''),
  '/api/account/email/change': (app, b) => app.V4EmailChange((b && b.email) || '', (b && b.code) || ''),
  '/api/account/password': (app, b) => app.V4PasswordChange((b && b.password) || '', (b && b.new_password) || '', (b && b.code) || ''),
  '/api/account/keyword/rotate': (app, b) => app.V4KeywordRotate((b && b.code) || '', (b && b.new_keyword) || ''),
  '/api/account/delete': (app, b) => app.V4DeleteAccount((b && b.code) || ''),
  '/api/account/devices/kill': (app, b) => app.V4SessionKill(Number(b && b.session_id) || 0),
  '/api/account/devices/unbind': (app, b) => app.V4DeviceUnbind((b && b.device_id) || ''),
  '/api/feedback/send': (app, b) => app.V4FeedbackSend((b && b.topic) || '', (b && b.message) || ''),
  // Баг-репорт из модалки ошибки: на сервер уходит обычное обращение (тема
  // bug), но в message — JSON с действием пользователя, кодом/текстом
  // ошибки, состоянием движка, диагностика́ми и хвостом журнала (собирает
  // бэкенд, см. app_bugreport.go).
  '/api/feedback/bug-report': (app, b) => app.BugReportSend(
    (b && b.code) || '', (b && b.error) || '', (b && b.action) || '', (b && b.comment) || ''
  ),
  '/api/account/sync/push': (app) => app.DeviceSyncPush(),
  '/api/account/sync/restore': (app, b) => app.DeviceSyncRestore((b && b.device_id) || ''),
  '/api/account/promo/activate': (app, b) => app.V4PromoActivate((b && b.code) || ''),
  '/api/dns/config': (app, b) => app.SetDnsConfig(b || {}),
  '/api/dns/provider/toggle': (app, b) => app.ToggleDnsProvider((b && b.id) || '', !!(b && b.on)),
  '/api/dns/custom/toggle': (app, b) => app.ToggleDnsCustom((b && b.id) || '', !!(b && b.on)),
  '/api/dns/custom/save': (app, b) => app.SaveDnsCustom(
    (b && b.id) || '',
    (b && b.name) || '',
    (b && b.primary) || '',
    (b && b.secondary) || '',
    (b && b.doh) || '',
    !!(b && b.enable)
  ),
  '/api/dns/custom/delete': (app, b) => app.DeleteDnsCustom((b && b.id) || ''),
  '/api/sync/overrides': (app, b) => app.SetSyncOverrides({
    channel_enabled: !!(b && b.channel_enabled),
    overrides: (b && b.overrides) || {},
  }),
  '/api/sync/snapshot': (app) => app.SaveSyncSnapshot(),
  '/api/component-states': (app) => app.SaveComponentStates(),
  '/api/resource-check': (app, b) => app.CheckResource(b.url || ''),
  '/api/resource-check-deep': (app, b) => app.CheckResourceDeep(b.url || ''),
  '/api/resource-add': (app, b) => app.AddHostToUserList(b.host || ''),
  '/api/resource-add-batch': (app, b) => app.AddHostsToUserList(b.hosts || []),
  '/api/strategy/test-single': (app, b) => app.TestSingleStrategy(b.filename || '', !!(b && b.game)),
  '/api/strategy/cancel': (app) => app.CancelAutoTest(),
  '/api/autostart/enable': (app) => app.EnableAutostart(),
  '/api/autostart/disable': (app) => app.DisableAutostart(),
  '/api/app/restart': (app) => app.RestartApp(),
  '/api/app/close': (app) => app.WindowClose(),
  '/api/app/minimize': (app) => app.WindowMinimise(),
  '/api/app/maximize': (app) => app.WindowToggleMaximise(),
  '/api/app/is-maximised': (app) => app.WindowIsMaximised(),
  '/api/app/is-shown': (app) => app.WindowIsShown(),
  '/api/app/request-show': (app) => app.RequestWindowShow(),
  '/api/app/restart-clean': (app) => app.RestartAppClean(),
  '/api/logs/frontend': (app, b) => app.FrontendLogs(b.events || []),
  '/api/config': (app, b) => app.SetConfig(b || {}),
  '/api/game-optimize': (app, b) => app.SetGameOptimize(!!(b && b.enabled)),
  '/api/custom-theme': (app, b) => app.SaveCustomTheme(b || {}),
  '/api/custom-themes': (app, b) => app.SaveCustomThemes(b || {}),
  '/api/custom-themes/activate': (app, b) => app.ActivateCustomTheme(b || {}),
  '/api/custom-themes/delete': (app, b) => app.DeleteCustomTheme(b || {}),
  '/api/telegram/open': (app, b) => app.OpenTelegramLink((b && b.url) || ''),
  '/api/zapret/remove-system-service': (app) => app.RemoveSystemZapretService(),
  '/api/envscan/remove-zapret': (app) => app.RemoveThirdPartyZapret(),
  '/api/updates/download': (app) => app.DownloadZPUISetup(),
  '/api/updates/apply': (app) => app.ApplyZPUISetup(),
  '/api/updates/download-zapret': (app) => app.DownloadZapretUpdate(),
  '/api/updates/source': (app, b) => app.SetUpdatesSource((b && b.source) || ''),
  '/api/updates/apply-zapret': (app) => app.ApplyZapretPackage(),
  '/api/zapret/auto-install': (app) => app.AutoInstallZapret(),
  '/api/zapret/install-service-logged': (app, b) => app.InstallServiceLogged(b.strategy || ''),
  '/api/zapret/autoswitch/toggle': (app, b) => app.SetAutoSwitchEnabled(!!b.enabled),
  '/api/zapret/autoswitch/config': (app, b) => app.SetAutoSwitchConfig(b.threshold, b.interval),
  '/api/logs/clear-bucket': (app, b) => app.ClearLogBucket(b.table || 'all'),
  '/api/logs/export': (app) => app.ExportLogs(),
  '/api/logs/error-code': (app, b) => app.LogErrorCode(b.code || '', b.category || '', b.message || ''),
  '/api/logs/ui-error': (app, b) => app.LogUiError(b.code || '', b.msg || ''),
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

export async function api(method, path, body, timeoutMs = 120000) {
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
      return await withTimeout(handleDeviceRoute(app, method, base, params, body), timeoutMs);
    }

    let result;
    switch (method) {
      case 'GET':
        if (GET_ROUTES[base]) result = await withTimeout(GET_ROUTES[base](app, params), timeoutMs);
        else { console.warn('[api] Unknown GET route:', base); return null; }
        break;
      case 'POST':
        if (POST_ROUTES[base]) result = await withTimeout(POST_ROUTES[base](app, body || {}), timeoutMs);
        else { console.warn('[api] Unknown POST route:', base); return null; }
        break;
      case 'DELETE':
        if (DELETE_ROUTES[base]) result = await withTimeout(DELETE_ROUTES[base](app), timeoutMs);
        else if (base.startsWith('/api/devices/')) result = await withTimeout(handleDeviceRoute(app, 'DELETE', base, params, body), timeoutMs);
        else { console.warn('[api] Unknown DELETE route:', base); return null; }
        break;
      default:
        console.warn('[api] Method not supported:', method);
        return null;
    }
    return result;
  } catch (err) {
    console.error('[api] Error calling', method, path, err);
    // Возвращаем текст исключения — сбои биндингов не должны маскироваться
    // под generic-ошибку интерфейса.
    return { error: 'client_exception', detail: String((err && (err.message || err)) || err) };
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
 *   const es = createStream('/api/autoselect/stream?mode=smart');
 *   es.onmessage = (e) => { const d = JSON.parse(e.data); ... };
 *   es.onerror = () => { ... };
 *   es.close();
 */
export function createStream(path) {
  const app = wailsApp();
  const { base, params } = parseQuery(path);

  const stream = {
    _onmessage: null,
    _onerror: null,
    _started: false,
    _offs: [],

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

      if (base === '/api/autoselect/stream') {
        // Автоподбор лучшей стратегии с применением.
        // params.mode=smart — «Умный подбор»: план с сервера по оператору;
        //   сервер недоступен — работает по локальным данным устройства
        //   (needs_fallback больше не используется).
        // params.mode=game — «Игровой подбор»: params.strategies (CSV имён
        //   *.bat, без пробелов) задаёт список; пусто — топ-5 по баллам;
        //   только игровые цели.
        // params.with_game=1 — игровой прогон финалистов (галочка, по умолчанию выкл).
        const KNOWN_MODES = ['game', 'smart', 'startup', 'full'];
        const selMode = KNOWN_MODES.includes(params?.mode) ? params.mode : '';
        const withGame = params?.with_game === '1' || params?.with_game === 1 || params?.with_game === true;
        const strategies = String(params?.strategies || '');
        this._offs.push(subscribeEvent('autoselect:event', (data) => {
          if (this._onmessage) this._onmessage({ data: JSON.stringify(data) });
        }));

        this._offs.push(subscribeEvent('autoselect:done', (data) => {
          if (this._onmessage) this._onmessage({ data: JSON.stringify({ ...(data || {}), type: 'done' }) });
        }));

        // Запускаем бэкенд-метод
        app.RunAutoSelectStream(selMode, '', false, withGame, strategies).catch(err => {
          console.error('[STREAM] RunAutoSelectStream error:', err);
          if (this._onerror) this._onerror(err);
        });
      }
    },

    /**
     * close(cancel) — отписаться от событий.
     * cancel=true — дополнительно отменить подбор на бэкенде.
     * После успешного done close() НЕ дёргает CancelAutoTest:
     * общий флаг отмены на бэкенде ломает чужие забеги.
     */
    close(cancel = false) {
      for (const off of this._offs) {
        try { off(); } catch {}
      }
      if (cancel && base === '/api/autoselect/stream') {
        try { app.CancelAutoTest(); } catch {}
      }
      this._offs = [];
    },
  };

  return stream;
}

// --- Кэш GET-ответов для мгновенного показа вкладок (SWR): при повторном
// открытии вкладки данные отдаются из кэша сразу, поллинг тихо обновляет.
// Ключ — маршрут; PUT делает вызывающий код после успешного fetch.
const _apiCache = new Map();

export function apiCacheGet(route) {
  const hit = _apiCache.get(route);
  return hit ? hit.data : null;
}

export function apiCachePut(route, data) {
  if (data && !data.error) _apiCache.set(route, { data, at: Date.now() });
}

export function apiCacheInvalidate(route) {
  if (route) _apiCache.delete(route);
  else _apiCache.clear();
}

// Прогрев ключей вкладок на простое после стартового экрана.
export function apiWarm(routes) {
  routes.forEach((r) => {
    api('GET', r).then((d) => apiCachePut(r, d)).catch(() => {});
  });
}
