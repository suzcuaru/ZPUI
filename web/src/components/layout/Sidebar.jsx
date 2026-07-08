import { useT } from '../../i18n';
import { useServiceToggle } from '../../hooks/useServiceToggle';
import {
  LayoutDashboard, ShieldCheck, Globe, Gamepad2, Activity, Settings,
  FileText, ShieldAlert, Zap, Search, CircleHelp,
} from 'lucide-react';

const ic = (Comp) => <Comp size={18} strokeWidth={2} />;

const NAV = [
  { page: 'dashboard', label: 'nav.dashboard', tooltip: 'sidebar.dashboardTip', icon: ic(LayoutDashboard) },
  { page: 'zapret',    label: 'nav.zapret',    tooltip: 'sidebar.zapretTip', service: 'zapret', icon: ic(ShieldCheck) },
  { page: 'proxy',     label: 'nav.proxy',     tooltip: 'sidebar.proxyTip', service: 'proxy',  icon: ic(Globe) },
  { page: 'xboxdns',   label: 'nav.xboxdns',   tooltip: 'sidebar.dnsTip', service: 'xboxdns', icon: ic(Gamepad2) },
  { page: 'monitor',   label: 'nav.monitor',   tooltip: 'sidebar.monitorTip', icon: ic(Activity) },
  { page: 'settings',  label: 'nav.settings',  tooltip: 'sidebar.settingsTip', icon: ic(Settings) },
];

export default function Sidebar({ activePage, onNavigate, onOpenChecker, onAutoSelect, onOpenHealth, onOpenHelp, healthWarn, status, showToast, onOpenLogs }) {
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
              data-tooltip={t(item.tooltip)}
              data-tooltip-pos="right"
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
          data-tooltip={t('sidebar.healthTip')}
          data-tooltip-pos="right"
        >
          <ShieldAlert size={18} strokeWidth={2} />
          {healthWarn.warnings?.length > 0 && (
            <span className="sidebar-health-count">{healthWarn.warnings.length}</span>
          )}
        </button>
      )}

      <button
        className={'sidebar-footer-btn' + (zRun ? '' : ' disabled')}
        onClick={zRun ? onAutoSelect : undefined}
        aria-label="auto-select"
        data-tooltip={zRun ? t('sidebar.autoSelectTip') : t('sidebar.autoSelectDisabledTip')}
        data-tooltip-pos="right"
      >
        <Zap size={18} strokeWidth={2} />
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenChecker}
        aria-label="checker"
        data-tooltip={t('sidebar.checkerTip')}
        data-tooltip-pos="right"
      >
        <Search size={18} strokeWidth={2} />
      </button>

      <div className="sidebar-divider" />

      <button
        className="sidebar-footer-btn"
        onClick={onOpenLogs}
        aria-label="logs"
        data-tooltip={t('sidebar.logsTip')}
        data-tooltip-pos="right"
      >
        <FileText size={18} strokeWidth={2} />
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenHelp}
        aria-label="help"
        data-tooltip={t('sidebar.helpTip')}
        data-tooltip-pos="right"
      >
        <CircleHelp size={18} strokeWidth={2} />
      </button>
    </aside>
  );
}
