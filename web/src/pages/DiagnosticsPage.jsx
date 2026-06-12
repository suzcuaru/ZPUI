import { useState, useEffect, useCallback } from 'react';
import Card from '../components/Card';
import { api } from '../api';
import { LANG } from '../lang';
import { cacheGet, cacheSet } from '../db';

export default function DiagnosticsPage({ status, showToast }) {
  const [diagResults, setDiagResults] = useState(null);
  const [diagLoading, setDiagLoading] = useState(false);
  const [clearing, setClearing] = useState({});

  const handleDiagnostics = async () => {
    setDiagLoading(true);
    const cached = await cacheGet('diagnostics');
    if (cached) setDiagResults(cached);
    const d = await api('GET', '/api/zapret/diagnostics');
    if (d) { setDiagResults(d || {}); cacheSet('diagnostics', d); }
    setDiagLoading(false);
  };

  const handleUpdateIpset = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-ipset'), 'IPSet обновлён', showToast);
  };

  const handleUpdateHosts = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-hosts'), 'Hosts файл обновлён', showToast);
  };

  const handleCacheClear = async (target) => {
    setClearing(p => ({ ...p, [target]: true }));
    const d = await api('POST', '/api/zapret/cache/clear', { target });
    if (d && d.cleared && d.cleared.length > 0) {
      showToast(`Очищено: ${d.cleared.join(', ')}`, 'success');
    } else {
      showToast('Нечего очищать', 'info');
    }
    setClearing(p => ({ ...p, [target]: false }));
  };

  const statusOrder = { error: 0, warn: 1, ok: 2 };
  const sortedResults = diagResults ? Object.entries(diagResults)
    .sort((a, b) => (statusOrder[a[1]?.status] || 2) - (statusOrder[b[1]?.status] || 2)) : [];
  const okCount = sortedResults.filter(([,v]) => v.status === 'ok').length;
  const failCount = sortedResults.length - okCount;

  return (
    <>
      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Диагностика</h3>
          <p>Полная проверка системы</p>
        </div>
        <button className="btn btn-accent diag-run-btn" onClick={handleDiagnostics} disabled={diagLoading}>
          {diagLoading ? (
            <><span className="diag-spinner"></span> Проверка...</>
          ) : 'Запустить диагностику'}
        </button>
        {diagResults && sortedResults.length > 0 && (
          <div className="diag-summary">
            <div className={'diag-summary-item ok'}>
              <span className="diag-summary-num">{okCount}</span>
              <span className="diag-summary-label">ОК</span>
            </div>
            <div className={'diag-summary-item warn'}>
              <span className="diag-summary-num">{failCount}</span>
              <span className="diag-summary-label">Проблемы</span>
            </div>
          </div>
        )}
        {diagResults && sortedResults.length > 0 && (
          <div className="diag-results-list">
            {sortedResults.map(([key, val]) => (
              <div key={key} className={'diag-row' + (val.status !== 'ok' ? ' has-issue' : '')}>
                <span className={'diag-icon ' + val.status}>
                  {val.status === 'ok' ? '✓' : val.status === 'warn' ? '!' : '✗'}
                </span>
                <div className="diag-row-content">
                  <span className="diag-row-label">{val.label}</span>
                  <span className="diag-row-detail">{val.detail}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Очистка кэша</h3>
          <p>Может решить проблемы с подключением</p>
        </div>
        <div className="diag-cache-grid">
          <button className="diag-cache-btn" onClick={() => handleCacheClear('discord')} disabled={clearing.discord}>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><circle cx="12" cy="12" r="10"/><path d="M8 12h8M12 8v8"/></svg>
            <span>{clearing.discord ? '...' : 'Discord'}</span>
          </button>
          <button className="diag-cache-btn" onClick={() => handleCacheClear('network')} disabled={clearing.network}>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><circle cx="12" cy="12" r="10"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10"/></svg>
            <span>{clearing.network ? '...' : 'Сеть'}</span>
          </button>
          <button className="diag-cache-btn" onClick={() => handleCacheClear('all')} disabled={clearing.all}>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M3 6h18M8 6V4h8v2M5 6v14a2 2 0 002 2h10a2 2 0 002-2V6"/></svg>
            <span>{clearing.all ? '...' : 'Всё'}</span>
          </button>
        </div>
      </Card>

      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Обновления данных</h3>
        </div>
        <div className="btn-row">
          <button className="btn btn-accent" onClick={handleUpdateIpset}>IPSet</button>
          <button className="btn btn-accent" onClick={handleUpdateHosts}>Hosts</button>
        </div>
      </Card>
    </>
  );
}
