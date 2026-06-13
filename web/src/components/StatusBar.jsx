import { useState, useEffect } from 'react';
import { api } from '../api';

export default function StatusBar({ status }) {
  const [resourcePct, setResourcePct] = useState(null);

  useEffect(() => {
    let alive = true;
    const fetchResources = async () => {
      const data = await api('GET', '/api/resource-status');
      if (!alive || !data) return;
      const defaults = data.default || [];
      if (defaults.length === 0) return;
      const ok = defaults.filter(r => r.ok === true).length;
      setResourcePct(Math.round((ok / defaults.length) * 100));
    };
    fetchResources();
    const iv = setInterval(fetchResources, 30000);
    return () => { alive = false; clearInterval(iv); };
  }, []);

  const zapretRunning = status?.zapret?.status === 'running';
  const proxyRunning = status?.proxy?.running === true;
  const strategy = status?.zapret?.strategy || 'не выбрана';
  const proxyPort = status?.proxy?.port;

  const pctColor = resourcePct === null ? 'var(--text-3)' :
    resourcePct >= 80 ? 'var(--success)' :
    resourcePct >= 50 ? 'var(--warning)' : 'var(--danger)';

  return (
    <div className="status-bar">
      <div className="status-bar-item">
        <span className={`status-bar-dot ${zapretRunning ? 'on' : 'off'}`} />
        <span>Zapret: <span className="status-bar-label">{zapretRunning ? 'Работает' : 'Остановлен'}</span></span>
      </div>
      <div className="status-bar-sep" />
      <div className="status-bar-item">
        <span className={`status-bar-dot ${proxyRunning ? 'on' : 'off'}`} />
        <span>Прокси: <span className="status-bar-label">{proxyRunning ? `:${proxyPort}` : 'Остановлен'}</span></span>
      </div>
      <div className="status-bar-sep" />
      <div className="status-bar-item">
        <span>Стратегия: <span className="status-bar-label">{strategy}</span></span>
      </div>
      <div className="status-bar-sep" />
      <div className="status-bar-item status-bar-pct">
        <span>Доступность:</span>
        <div className="status-bar-pct-bar">
          <div
            className="status-bar-pct-fill"
            style={{ width: `${resourcePct ?? 0}%`, background: pctColor }}
          />
        </div>
        <span className="status-bar-pct-text" style={{ color: pctColor }}>
          {resourcePct !== null ? `${resourcePct}%` : '...'}
        </span>
      </div>
    </div>
  );
}