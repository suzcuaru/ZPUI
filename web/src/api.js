function wailsApp() {
  if (typeof window !== 'undefined' && window.go) {
    if (window.go.app && window.go.app.App) return window.go.app.App;
    if (window.go.main && window.go.main.App) return window.go.main.App;
  }
  return null;
}

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

const GET_ROUTES = {
  '/api/status': (app) => app.GetStatus(),
  '/api/version': (app) => app.GetVersion(),
  '/api/config': (app) => app.GetConfig(),
  '/api/system-theme': (app) => app.GetSystemTheme(),
  '/api/modules': (app) => app.GetModules(),
  '/api/modules/reload': (app) => app.ReloadModules(),
  '/api/logs': (app, p) => app.GetLogs(p.category || '', parseInt(p.lines) || 200),
};

const POST_ROUTES = {
  '/api/config': (app, b) => app.SetConfig(b || {}),
  '/api/language': (app, b) => app.SetLanguage(b.lang || 'ru'),
  '/api/modules/start': (app, b) => app.StartModule(b.id || ''),
  '/api/modules/stop': (app, b) => app.StopModule(b.id || ''),
  '/api/modules/restart': (app, b) => app.RestartModule(b.id || ''),
  '/api/modules/enabled': (app, b) => app.SetModuleEnabled(b.id || '', b.enabled === true),
  '/api/modules/open-folder': (app) => app.OpenModulesFolder(),
  '/api/external': (app, b) => app.OpenExternal(b.url || ''),
};

export async function api(method, path, body) {
  const app = wailsApp();
  if (!app) {
    console.warn('[api] Wails runtime not available');
    return null;
  }
  method = method.toUpperCase();
  const { base } = parseQuery(path);
  try {
    const route = method === 'GET' ? GET_ROUTES[base] : POST_ROUTES[base];
    if (!route) {
      console.warn('[api] Unknown route:', method, base);
      return null;
    }
    return await route(app, method === 'GET' ? parseQuery(path).params : (body || {}));
  } catch (err) {
    console.error('[api] Error', method, path, err);
    return null;
  }
}

export async function apiCall(fn, successMsg, showToast) {
  try {
    const result = await fn();
    if (result && result.error) {
      if (showToast) showToast(result.error, 'error');
      return false;
    }
    if (successMsg && showToast) showToast(successMsg, 'success');
    return true;
  } catch {
    return false;
  }
}

export async function openExternal(url) {
  try {
    const app = wailsApp();
    if (app) await app.OpenExternal(url);
  } catch {}
}
