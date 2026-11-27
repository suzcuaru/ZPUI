import { useState, useCallback, useEffect } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { usePolling } from '../../hooks/usePolling';
import SidebarBanner from '../ui/SidebarBanner';
import { useEngineFail } from '../../engineFail';
import {
  LayoutDashboard, ShieldCheck, Globe, Server, Activity, Settings,
    FileText, ShieldAlert, Gauge, Search, CircleHelp, BatteryCharging, Eraser, TriangleAlert, Waypoints, Puzzle,
} from 'lucide-react';

const ic = (Comp) => <Comp size={18} strokeWidth={2} />;

const NAV = [
  { page: 'dashboard', label: 'nav.dashboard', icon: ic(LayoutDashboard) },
  { page: 'zapret',    label: 'nav.zapret',    service: 'zapret', icon: ic(ShieldCheck) },
  { page: 'proxy',     label: 'nav.proxy',     service: 'proxy',  icon: ic(Globe) },
  // Иконка DNS — та же, что на карточке дашборда (server rack).
  { page: 'network',    label: 'nav.network',    service: 'xboxdns', icon: ic(Server) },
  { page: 'monitor',   label: 'nav.monitor',   icon: ic(Activity) },
  { page: 'settings',  label: 'nav.settings',  icon: ic(Settings) },
];

export default function Sidebar({ activePage, onNavigate, onOpenChecker, onAutoSelect, onOpenHealth, onOpenHelp, healthWarn, status, onOpenLogs, onOpenCache, onOpenMcp, onOpenExtension, powerSaving, powerLevel, authEnabled }) {

  // Размер кеша Discord: переполнен — жёлтый значок предупреждения в панели.
  const [discordCacheMB, setDiscordCacheMB] = useState(0);
  const loadCache = useCallback(async () => {
    const r = await api('GET', '/api/cache/info');
    if (r && !r.error) {
      const warn = Number(r.discord_warn_bytes) || 100 * 1024 * 1024;
      const bytes = Number(r.discord_bytes) || 0;
      setDiscordCacheMB(bytes > warn ? Math.round(bytes / (1024 * 1024)) : 0);
    }
  }, []);
  usePolling(loadCache, 60000);
  const cacheOverflow = discordCacheMB > 0;
  const cacheOverflowMB = discordCacheMB;

  const { t } = useT();

  const zRun = status?.zapret?.status === 'running';
  const engineFail = useEngineFail();
  // «Прокси» — по факту работы TG-моста, как карточка на дашборде.
  const serviceOn = {
    zapret: zRun,
    proxy: status?.tg_proxy?.running === true,
    xboxdns: status?.xbox_dns?.enabled === true,
  };

  const nav = NAV;

  const healthColor = healthWarn ? {
    healthy: 'var(--success)',
    degraded: 'var(--warning)',
    critical: 'var(--danger)',
  }[healthWarn.overall] : null;

  return (
    <aside className="sidebar">
      <nav className="sidebar-nav">
        {nav.map(item => {
          const engineDown = item.service === 'zapret' && engineFail;
          return (
            <button
              key={item.page}
              className={'sidebar-nav-item' + (activePage === item.page ? ' active' : '') + (item.service && serviceOn[item.service] ? ' svc-on' : '') + (engineDown ? ' svc-fail' : '')}
              onClick={() => onNavigate(item.page)}
              aria-label={t(item.label)}
              aria-current={activePage === item.page ? 'page' : undefined}
              data-tooltip={engineDown ? t('engineFail.title') : undefined}
              data-tooltip-pos="right"
            >
              {item.icon}
              <span className="sidebar-nav-label">{t(item.label)}</span>
            </button>
          );
        })}
      </nav>
      <div className="sidebar-spacer" />

      <SidebarBanner />

      {healthWarn && (
        <button
          className="sidebar-footer-btn"
          onClick={onOpenHealth}
          style={healthColor ? { color: healthColor } : {}}
          aria-label={t('health.centerTitle')}
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
        data-tooltip={zRun ? t('sidebar.autoSelectLabel') : t('sidebar.autoSelectDisabledTip')}
        data-tooltip-pos="right"
      >
        <Gauge size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.autoSelectLabel')}</span>
      </button>
      {/* Кеш Discord переполнен — жёлтый значок-предупреждение, клик открывает очистку */}
      {cacheOverflow && (
        <button
          className="sidebar-footer-btn cache-warn"
          onClick={onOpenCache}
          aria-label={t('sidebar.cacheOverflowTip', { mb: cacheOverflowMB })}
          data-tooltip={t('sidebar.cacheOverflowTip', { mb: cacheOverflowMB })}
          data-tooltip-pos="right"
        >
          <TriangleAlert size={18} strokeWidth={2} className="cache-warn-icon" />
          <span className="sidebar-nav-label">{t('sidebar.cacheLabel')}</span>
        </button>
      )}
      <button
        className="sidebar-footer-btn"
        onClick={onOpenChecker}
        aria-label={t('sidebar.checkerLabel')}
        data-tooltip={t('sidebar.checkerLabel')}
        data-tooltip-pos="right"
      >
        <Search size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.checkerLabel')}</span>
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenCache}
        aria-label={t('sidebar.cacheLabel')}
        data-tooltip={t('sidebar.cacheLabel')}
        data-tooltip-pos="right"
      >
        <Eraser size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.cacheLabel')}</span>
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenMcp}
        aria-label={t('mcp.title')}
        data-tooltip={t('mcp.title')}
        data-tooltip-pos="right"
      >
        <Waypoints size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('mcp.title')}</span>
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenExtension}
        aria-label={t('extension.title')}
        data-tooltip={t('extension.title')}
        data-tooltip-pos="right"
      >
        <Puzzle size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('extension.title')}</span>
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
        data-tooltip={t('sidebar.logsLabel')}
        data-tooltip-pos="right"
      >
        <FileText size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.logsLabel')}</span>
      </button>
      <button
        className="sidebar-footer-btn"
        onClick={onOpenHelp}
        aria-label={t('sidebar.helpLabel')}
        data-tooltip={t('sidebar.helpLabel')}
        data-tooltip-pos="right"
      >
        <CircleHelp size={18} strokeWidth={2} />
        <span className="sidebar-nav-label">{t('sidebar.helpLabel')}</span>
      </button>
    </aside>
  );
}
