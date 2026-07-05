import { useT } from '../../i18n';
import { useServiceToggle } from '../../hooks/useServiceToggle';

const NAV = [
  {
    page: 'dashboard',
    label: 'nav.dashboard',
    icon: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z"/></svg>,
  },
  {
    page: 'zapret',
    label: 'nav.zapret',
    service: 'zapret',
    icon: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 1L3 5v6c0 5.5 3.8 10.7 9 12 5.2-1.3 9-6.5 9-12V5l-9-4zm-1 14.5L7.5 12 9 10.5l2 2 4-4L16.5 10l-5.5 5.5z"/></svg>,
  },
  {
    page: 'proxy',
    label: 'nav.proxy',
    service: 'proxy',
    icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 010 20M12 2a15 15 0 000 20"/></svg>,
  },
  {
    page: 'xboxdns',
    label: 'nav.xboxdns',
    service: 'xboxdns',
    icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="2" y="3" width="20" height="6" rx="2"/><rect x="2" y="15" width="20" height="6" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>,
  },
  {
    page: 'monitor',
    label: 'nav.monitor',
    icon: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M3 12h3l2 7 4-14 2 7h7v2h-5.3l-1.7 6L10 6 8 14H3z"/></svg>,
  },
  {
    page: 'settings',
    label: 'nav.settings',
    icon: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58a.49.49 0 00.12-.61l-1.92-3.32a.49.49 0 00-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54A.484.484 0 0014.12 2h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96a.49.49 0 00-.59.22L2.94 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58a.49.49 0 00-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/></svg>,
  },
];

const ICONS = {
  sun: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.2" y1="4.2" x2="5.6" y2="5.6"/><line x1="18.4" y1="18.4" x2="19.8" y2="19.8"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.2" y1="19.8" x2="5.6" y2="18.4"/><line x1="18.4" y1="5.6" x2="19.8" y2="4.2"/></svg>,
  moon: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/></svg>,
  fileText: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>,
};

export default function Sidebar({ activePage, onNavigate, onOpenChecker, onAutoSelect, onOpenHealth, onOpenDiagnostics, onOpenHelp, healthWarn, status, showToast, onOpenLogs, isDark, onToggleTheme }) {
  const { t } = useT();

  const zRun = status?.zapret?.status === 'running';
  const pRun = status?.proxy?.running === true;
  const xRun = status?.xbox_dns?.enabled === true;

  const zapret = useServiceToggle('zapret', zRun, showToast, { startMsg: t('header.zapretStarted'), stopMsg: t('header.zapretStopped') });
  const proxy = useServiceToggle('proxy', pRun, showToast, { startMsg: t('header.proxyStarted'), stopMsg: t('header.proxyStopped') });
  const xbox = useServiceToggle('xboxdns', xRun, showToast, { startMsg: t('header.xboxDnsOn'), stopMsg: t('header.xboxDnsOff') });

  const svcMap = {
    zapret: { running: zRun, toggle: zapret.toggle, loading: zapret.loading },
    proxy: { running: pRun, toggle: proxy.toggle, loading: proxy.loading },
    xboxdns: { running: xRun, toggle: xbox.toggle, loading: xbox.loading },
  };

  const healthColor = healthWarn ? {
    healthy: 'var(--success)',
    degraded: 'var(--warning)',
    critical: 'var(--danger)',
  }[healthWarn.overall] : null;

  return (
    <aside className="sidebar">
      <nav className="sidebar-nav">
        {NAV.map(item => {
          const svc = item.service ? svcMap[item.service] : null;
          return (
            <button
              key={item.page}
              className={'sidebar-nav-item' + (activePage === item.page ? ' active' : '') + (svc?.running ? ' svc-on' : '')}
              onClick={() => onNavigate(item.page)}
              aria-label={t(item.label)}
              aria-current={activePage === item.page ? 'page' : undefined}
            >
              {item.icon}
              {svc && (
                <span
                  className={'svc-dot' + (svc.running ? ' on' : '') + (svc.loading ? ' loading' : '')}
                  onClick={(e) => { e.stopPropagation(); if (!svc.loading) svc.toggle(); }}
                  role="switch"
                  aria-checked={svc.running}
                  aria-label={t(item.label)}
                />
              )}
            </button>
          );
        })}
      </nav>
      <div className="sidebar-spacer" />

      {healthWarn && (
        <button
          className="sidebar-footer-btn"
          onClick={onOpenHealth}
          style={healthColor ? { color: healthColor } : {}}
          aria-label="health"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
          {healthWarn.warnings?.length > 0 && (
            <span className="sidebar-health-count">{healthWarn.warnings.length}</span>
          )}
        </button>
      )}

      <button
        className={'sidebar-footer-btn' + (zRun ? '' : ' disabled')}
        onClick={zRun ? () => showToast(t('common.inDevelopment'), 'info') : undefined}
        aria-label="auto-select"
      >
        <svg viewBox="0 0 24 24" fill="currentColor"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>
      </button>
      <button className="sidebar-footer-btn" onClick={onOpenChecker} aria-label="checker">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
      </button>
      <button className="sidebar-footer-btn" onClick={onOpenDiagnostics} aria-label="diagnostics">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
      </button>

      <div className="sidebar-divider" />

      <button className="sidebar-footer-btn" onClick={onOpenLogs} aria-label="logs">
        {ICONS.fileText}
      </button>
      <button className="sidebar-footer-btn" onClick={onOpenHelp} aria-label="help">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
      </button>
      <button className="sidebar-footer-btn" onClick={onToggleTheme} aria-label="theme">
        {isDark ? ICONS.sun : ICONS.moon}
      </button>
    </aside>
  );
}
