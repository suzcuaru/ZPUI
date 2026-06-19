import { useState, useEffect, useCallback, useRef } from 'react';
import Switch from '../components/ui/Switch';
import { api, apiCall } from '../api';
import { LANG } from '../lang';
import { cacheGet, cacheSet } from '../db';

export default function GeneralPage({ status, showToast }) {
  const [config, setConfig] = useState({ zapret_path: '', proxy_port: 1080, autostart: false, proxy_auto_start: false, auto_path: true });
  const [svcStatus, setSvcStatus] = useState(null);
  const [svcLoading, setSvcLoading] = useState(false);
  const saveTimer = useRef(null);

  const z = status?.zapret || {};
  const detectedPath = z.zapretDir || '';

  useEffect(() => { loadConfig(); loadSvcStatus(); }, []);

  useEffect(() => {
    const iv = setInterval(loadSvcStatus, 5000);
    return () => clearInterval(iv);
  }, []);

  const loadConfig = async () => {
    const cached = await cacheGet('config');
    if (cached) applyConfig(cached);
    const d = await api('GET', '/api/config');
    if (d) { applyConfig(d); cacheSet('config', d); }
  };

  const applyConfig = (d) => {
    const savedPath = d.zapret_path || '';
    const isAuto = !savedPath || savedPath === detectedPath;
    setConfig({
      zapret_path: savedPath || detectedPath,
      proxy_port: d.proxy?.port || 1080,
      autostart: d.autostart || false,
      proxy_auto_start: d.proxy?.auto_start || false,
      auto_path: isAuto,
    });
  };

  const loadSvcStatus = async () => { const d = await api('GET', '/api/zapret/service/status'); if (d) setSvcStatus(d); };

  const handleSvcInstall = async () => {
    setSvcLoading(true);
    await apiCall(() => api('POST', '/api/zapret/service/install', { strategy: status?.zapret?.strategy || '' }), LANG.serviceInstalled, showToast);
    setSvcLoading(false);
    setTimeout(loadSvcStatus, 2000);
  };

  const handleSvcRemove = async () => {
    setSvcLoading(true);
    await apiCall(() => api('POST', '/api/zapret/service/remove'), LANG.serviceRemoved, showToast);
    setSvcLoading(false);
    setTimeout(loadSvcStatus, 2000);
  };

  const save = useCallback(async (cfg) => {
    const pathToSave = cfg.auto_path ? detectedPath : cfg.zapret_path;
    await api('POST', '/api/config', { zapret_path: pathToSave }).catch(() => {});
    await api('POST', '/api/proxy/config', { port: parseInt(cfg.proxy_port), auto_start: cfg.proxy_auto_start }).catch(() => {});
    if (cfg.autostart) {
      await api('POST', '/api/autostart/enable').catch(() => {});
    } else {
      await api('POST', '/api/autostart/disable').catch(() => {});
    }
    showToast(LANG.saved, 'success');
  }, [detectedPath, showToast]);

  const update = useCallback((patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      clearTimeout(saveTimer.current);
      saveTimer.current = setTimeout(() => save(next), 500);
      return next;
    });
  }, [save]);

  const svcInstalled = svcStatus?.installed;
  const svcRunning = svcStatus?.running;

  return (
    <>
      <div className="page-title">Настройки</div>
      <div className="gen-section">
        <div className="flt-label">Основные настройки</div>
        <div className="gen-row-desc" style={{ marginBottom: 4 }}>Путь к Zapret, порт прокси и автозапуск</div>
        <div className="form-group">
          <label>Путь к папке Zapret</label>
          {config.auto_path && detectedPath ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span className="mono">{detectedPath}</span>
              <span className="badge badge-accent" style={{ fontSize: 10 }}>авто</span>
            </div>
          ) : (
            <input type="text" className="form-input" value={config.zapret_path} onChange={e => update({ zapret_path: e.target.value })} />
          )}
          <div className="form-hint">
            {config.auto_path && detectedPath ? 'Модуль рядом с Zapret — путь определён автоматически' : 'Директория, где находятся winws.exe и service.bat'}
          </div>
        </div>
        {detectedPath && (
          <div className="gen-row">
            <div className="gen-row-info">
              <span className="gen-row-title">Автоопределение пути</span>
              <span className="gen-row-desc">Модуль и Zapret в одной папке</span>
            </div>
            <Switch checked={config.auto_path} onChange={() => update({ auto_path: !config.auto_path, zapret_path: config.auto_path ? '' : detectedPath })} />
          </div>
        )}
        <div className="form-group">
          <label>Порт прокси (SOCKS5)</label>
          <input type="number" className="form-input" value={config.proxy_port} min="1" max="65535" onChange={e => update({ proxy_port: parseInt(e.target.value) || 1080 })} />
        </div>
        <div className="gen-row">
          <div className="gen-row-info">
            <span className="gen-row-title">Автозапуск приложения</span>
            <span className="gen-row-desc">Автоматически запускать при старте системы</span>
          </div>
          <Switch checked={config.autostart} onChange={() => update({ autostart: !config.autostart })} />
        </div>
        <div className="gen-row">
          <div className="gen-row-info">
            <span className="gen-row-title">Автозапуск прокси</span>
            <span className="gen-row-desc">Запускать SOCKS5-прокси вместе с приложением</span>
          </div>
          <Switch checked={config.proxy_auto_start} onChange={() => update({ proxy_auto_start: !config.proxy_auto_start })} />
        </div>
        <div className="flt-divider" />
        <div className="flt-label">Служба Windows</div>
        <div className="svc-info-grid">
          <div className="svc-info-row">
            <span className="svc-info-label">Статус</span>
            <span className={'svc-info-badge ' + (svcRunning ? 'ok' : svcInstalled ? 'warn' : 'off')}>
              {svcRunning ? 'Работает' : svcInstalled ? 'Установлена' : 'Не установлена'}
            </span>
          </div>
          {svcInstalled && (
            <>
              <div className="svc-info-row">
                <span className="svc-info-label">Стратегия</span>
                <span className="svc-info-value mono">{svcStatus?.strategy || '—'}</span>
              </div>
              {svcStatus?.pid > 0 && (
                <div className="svc-info-row">
                  <span className="svc-info-label">PID</span>
                  <span className="svc-info-value mono">{svcStatus.pid}</span>
                </div>
              )}
            </>
          )}
        </div>
        <button
          className={'btn ' + (svcInstalled ? 'btn-danger' : 'btn-accent')}
          onClick={svcInstalled ? handleSvcRemove : handleSvcInstall}
          disabled={svcLoading}
          style={{ width: '100%' }}
        >
          {svcLoading ? '...' : svcInstalled ? 'Удалить службу' : 'Установить службу'}
        </button>
      </div>
    </>
  );
}
