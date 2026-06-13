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
      const ok = defaults.filter(r => r.status === 'ok').length;
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

  const pctColor = resourcePct === null ? '#888' :
    resourcePct >= 80 ? '#4caf50' :
    resourcePct >= 50 ? '#ff9800' : '#f44336';

  return (
    <div className="status-bar">
      <div className="status-bar-item">
        <span className={`status-dot ${zapretRunning ? 'dot-green' : 'dot-red'}`} />
        <span>Запрет: {zapretRunning ? 'Работает' : 'Остановлен'}</span>
      </div>
      <div className="status-bar-item">
        <span className={`status-dot ${proxyRunning ? 'dot-green' : 'dot-red'}`} />
        <span>Прокси: {proxyRunning ? `:${proxyPort}` : 'Остановлен'}</span>
      </div>
      <div className="status-bar-item">
        <span className="status-bar-label">Стратегия:</span>
        <span className="status-bar-value">{strategy}</span>
      </div>
      <div className="status-bar-item status-bar-resource">
        <span className="status-bar-label">Доступность:</span>
        <div className="status-bar-pct-bg">
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