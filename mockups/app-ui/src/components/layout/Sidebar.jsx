import { useT } from '../../i18n';
import {
  LayoutDashboard, ShieldCheck, Globe, Gamepad2, Activity, Settings, User,
  FileText, ShieldAlert, Zap, Search, CircleHelp, BatteryCharging,
} from 'lucide-react';

const ic = (Comp) => <Comp size={18} strokeWidth={2} />;

const NAV = [
  { page: 'dashboard', label: 'nav.dashboard', icon: ic(LayoutDashboard) },
  { page: 'zapret',    label: 'nav.zapret',    service: 'zapret', icon: ic(ShieldCheck) },
  { page: 'proxy',     label: 'nav.proxy',     service: 'proxy',  icon: ic(Globe) },
  { page: 'xboxdns',   label: 'nav.xboxdns',   service: 'xboxdns', icon: ic(Gamepad2) },
  { page: 'monitor',   label: 'nav.monitor',   icon: ic(Activity) },
  { page: 'user',      label: 'nav.user',      icon: ic(User) },
  { page: 'settings',  label: 'nav.settings',  icon: ic(Settings) },
];

export default function Sidebar({ activePage, onNavigate, onOpenChecker, onAutoSelect, onOpenHealth, onOpenHelp, healthWarn, status, onOpenLogs, powerSaving, powerLevel }) {
  const { t } = useT();

  const zRun = status?.zapret?.status === 'running';
  const serviceOn = {
    zapret: zRun,
    proxy: status?.proxy?.running === true,
    xboxdns: status?.xbox_dns?.enabled === true,
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
          return (
            <button
              key={item.page}
              className={'sidebar-nav-item' + (activePage === item.page ? ' active' : '') + (item.service && serviceOn[item.service] ? ' svc-on' : '')}
              onClick={() => onNavigate(item.page)}
              aria-label={t(item.label)}
              aria-current={activePage === item.page ? 'page' : undefined}
            >
              {item.icon}
              <span className="sidebar-nav-label">{t(item.label)}</span>
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
          aria-label={t('sidebar.healthLabel')}
        >
          <ShieldAlert size={18} strokeWidth={2} />
          <span className="sidebar-nav-label">{t('sidebar.healthLabel')}</span>
          {healthWarn.warnings?.length > 0 && (
            <span className="sidebar-health-count">{healthWarn.warnings.length}</span>
          )}
        </button>
      )}

      <button
        className={'sidebar-footer-btn' + (zRun ? '' : ' disabled')}
        onClick={zRun ? onAutoSelect : undefined}
        aria-label={t('sidebar.autoSelectLabel')}
        aria-disabled={!zRun}
        data-tooltip={zRun ? undefined : t('sidebar.autoSelectDisabledTip')}
        data-tooltip-pos="right"
      >
        <Zap size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.autoSelectLabel')}</span>
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenChecker}
        aria-label={t('sidebar.checkerLabel')}
      >
        <Search size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.checkerLabel')}</span>
      </button>

      {/* Экономный режим должен быть виден всегда — в том числе когда окно
          открыли заново и непонятно, почему трафик «не считает». */}
      {powerSaving && (
        <button
          className="sidebar-footer-btn power-saving"
          onClick={() => onNavigate('settings')}
          aria-label={t('sidebar.powerSavingTip')}
        >
          <BatteryCharging size={18} strokeWidth={2} />
          <span className="sidebar-nav-label">{t('sidebar.powerSavingTip')}</span>
        </button>
      )}

      <div className="sidebar-divider" />

      <button
        className="sidebar-footer-btn"
        onClick={onOpenLogs}
        aria-label={t('sidebar.logsLabel')}
      >
        <FileText size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.logsLabel')}</span>
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenHelp}
        aria-label={t('sidebar.helpLabel')}
      >
        <CircleHelp size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.helpLabel')}</span>
      </button>
    </aside>
  );
}
