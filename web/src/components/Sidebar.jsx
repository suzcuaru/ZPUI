import { useState } from 'react';
import { api, apiCall } from '../api';
import { LANG } from '../lang';

const NAV = [
  {
    page: 'monitor', label: 'Мониторинг',
    icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/><circle cx="12" cy="12" r="10"/></svg>,
  },
  {
    page: 'devices', label: 'Устройства',
    icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="5" y="2" width="14" height="20" rx="2" ry="2"/><line x1="12" y1="18" x2="12.01" y2="18"/></svg>,
  },
  {
    page: 'filters', label: 'Фильтры',
    icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/></svg>,
  },
  {
    page: 'general', label: 'Настройки',
    icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>,
  },
  {
    page: 'diagnostics', label: 'Диагностика',
    icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg>,
  },
  {
    page: 'about', label: 'О программе',
    icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>,
  },
];

export default function Sidebar({ activePage, onNavigate, status, onOpenLogs, showToast, collapsed, onToggleCollapse }) {
  const zRun = status?.zapret?.status === 'running';
  const pRun = status?.proxy?.running === true;
  const ver = status?.zapret?.version || '—';
  const port = status?.proxy?.port || '—';
  const proxyIp = status?.network?.ips?.[0] || '127.0.0.1';
  const proxyAddr = pRun ? proxyIp + ':' + port : '';
  const strategy = status?.zapret?.strategy || 'Не выбрана';
  const resPct = status?.mod?.resource_pct ?? -1;

  const [zLoading, setZLoading] = useState(false);
  const [pLoading, setPLoading] = useState(false);
  const [copiedField, setCopiedField] = useState(null);

  const toggleZapret = async () => {
    setZLoading(true);
    await apiCall(() => api('POST', '/api/zapret/' + (zRun ? 'stop' : 'start')), zRun ? LANG.zapretStopped : LANG.zapretStarted, showToast);
    setZLoading(false);
  };

  const toggleProxy = async () => {
    setPLoading(true);
    await apiCall(() => api('POST', '/api/proxy/' + (pRun ? 'stop' : 'start')), pRun ? LANG.proxyStopped : LANG.proxyStarted, showToast);
    setPLoading(false);
  };

  const copyToClipboard = async (text, field) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedField(field);
      setTimeout(() => setCopiedField(null), 1500);
    } catch {}
  };

  return (
    <aside className={'sidebar' + (collapsed ? ' collapsed' : '')}>
      <div className="sidebar-logo" onClick={onToggleCollapse} title={collapsed ? 'Развернуть' : 'Свернуть'}>
        <span className="logo-icon" dangerouslySetInnerHTML={{ __html: '<svg width="28" height="28" viewBox="0 0 256 256"><defs><linearGradient id="sg" x1="0%" y1="0%" x2="0%" y2="100%"><stop offset="0%" stop-color="#5b8def"/><stop offset="100%" stop-color="#3b6fd6"/></linearGradient><linearGradient id="bg" x1="0%" y1="0%" x2="0%" y2="100%"><stop offset="0%" stop-color="#ffd400"/><stop offset="100%" stop-color="#ffaa00"/></linearGradient></defs><path d="M128 16 L224 48 L224 128 C224 176 180 216 128 240 C76 216 32 176 32 128 L32 48 Z" fill="url(#sg)"/><path d="M140 56 L96 136 L124 136 L112 200 L168 112 L136 112 L152 56 Z" fill="url(#bg)"/></svg>' }} />
        {!collapsed && <span className="logo-text">ZPUI</span>}
        {!collapsed && (
          <svg className="sidebar-collapse-arrow" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polyline points="15 18 9 12 15 6"/></svg>
        )}
      </div>

      <nav className="sidebar-nav">
        {NAV.map(item => (
          <a key={item.page} href="#" className={'nav-item' + (activePage === item.page ? ' active' : '') + (collapsed ? ' collapsed' : '')}
            title={collapsed ? item.label : ''}
            onClick={e => { e.preventDefault(); onNavigate(item.page); }}>
            {item.icon}
            {!collapsed && <span>{item.label}</span>}
          </a>
        ))}
      </nav>

      {/* Status block (moved from StatusBar) */}
      <div className="sidebar-status-block">
        <div className="sidebar-status-row">
          <span className="sidebar-status-dot-wrap">
            <span className={'sidebar-status-dot ' + (zRun ? 'on' : 'off')}></span>
            {!collapsed && <span className="sidebar-status-label">Zapret</span>}
          </span>
          {!collapsed && <span className={'sidebar-status-val ' + (zRun ? 'ok' : 'err')}>{zRun ? 'Работает' : 'Стоп'}</span>}
        </div>
        <div className="sidebar-status-row">
          <span className="sidebar-status-dot-wrap">
            <span className={'sidebar-status-dot ' + (pRun ? 'on' : 'off')}></span>
            {!collapsed && <span className="sidebar-status-label">Прокси</span>}
          </span>
          {!collapsed && <span className={'sidebar-status-val ' + (pRun ? 'ok' : 'err')}>{pRun ? ':' + port : 'Стоп'}</span>}
        </div>
        {!collapsed && (
          <div className="sidebar-status-row">
            <span className="sidebar-status-dot-wrap">
              <span className="sidebar-status-label">Стратегия</span>
            </span>
            <span className="sidebar-status-val mono" title={strategy}>{strategy}</span>
          </div>
        )}
        {!collapsed && resPct >= 0 && (
          <div className="sidebar-status-row">
            <span className="sidebar-status-dot-wrap">
              <span className="sidebar-status-label">Доступность</span>
            </span>
            <span className={'sidebar-status-val ' + (resPct >= 80 ? 'ok' : resPct >= 50 ? 'warn' : 'err')}>{resPct}%</span>
          </div>
        )}
      </div>

      <div className="sidebar-services">
        <div className={'sidebar-svc' + (zRun ? ' on' : '')} onClick={toggleZapret} disabled={zLoading} title={collapsed ? 'Zapret' : ''}>
          <div className="sidebar-svc-icon">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          </div>
          {!collapsed && (
            <div className="sidebar-svc-info">
              <span className="sidebar-svc-name">Zapret</span>
              {zRun && <span className="sidebar-svc-sub mono">{ver}</span>}
            </div>
          )}
          <div className={'sidebar-svc-dot' + (zRun ? ' on' : '')}></div>
        </div>

        <div className={'sidebar-svc' + (pRun ? ' on' : '')} onClick={toggleProxy} disabled={pLoading} title={collapsed ? 'Прокси' : ''}>
          <div className="sidebar-svc-icon">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10A15.3 15.3 0 0 1 12 2z"/></svg>
          </div>
          {!collapsed && (
            <div className="sidebar-svc-info">
              <span className="sidebar-svc-name">Прокси</span>
              {pRun && <span className="sidebar-svc-sub mono">{proxyAddr}</span>}
            </div>
          )}
          {pRun ? (
            <button className="sidebar-svc-copy" onClick={e => { e.stopPropagation(); copyToClipboard(proxyAddr, 'proxy'); }}>
              {copiedField === 'proxy' ? (
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
              ) : (
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
              )}
            </button>
          ) : (
            <div className={'sidebar-svc-dot' + (pRun ? ' on' : '')}></div>
          )}
        </div>
      </div>

      <div className="sidebar-footer">
        <button className="sidebar-logs-btn" onClick={onOpenLogs} title={collapsed ? 'Логи' : ''}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
          {!collapsed && <span>Логи</span>}
        </button>
      </div>
    </aside>
  );
}