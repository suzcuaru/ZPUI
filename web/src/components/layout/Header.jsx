import { useState } from 'react';
import { api, apiCall } from '../../api';
import { useT } from '../../i18n';

const ICONS = {
  sun: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.2" y1="4.2" x2="5.6" y2="5.6"/><line x1="18.4" y1="18.4" x2="19.8" y2="19.8"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.2" y1="19.8" x2="5.6" y2="18.4"/><line x1="18.4" y1="5.6" x2="19.8" y2="4.2"/></svg>,
  moon: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/></svg>,
  fileText: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>,
  xbox: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="2" y="3" width="20" height="6" rx="2"/><rect x="2" y="15" width="20" height="6" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>,
};

export default function Header({ status, onOpenLogs, isDark, onToggleTheme, showToast, onNavigate }) {
  const { t } = useT();
  const zRun = status?.zapret?.status === 'running';
  const pRun = status?.proxy?.running === true;
  const xRun = status?.xbox_dns?.enabled === true;
  const port = status?.proxy?.port || '—';

  const [zLoading, setZLoading] = useState(false);
  const [pLoading, setPLoading] = useState(false);
  const [xLoading, setXLoading] = useState(false);

  const toggleZapret = async () => {
    setZLoading(true);
    await apiCall(() => api('POST', '/api/zapret/' + (zRun ? 'stop' : 'start')), zRun ? t('header.zapretStopped') : t('header.zapretStarted'), showToast);
    await apiCall(() => api('POST', '/api/component-states'));
    setZLoading(false);
  };

  const toggleProxy = async () => {
    setPLoading(true);
    await apiCall(() => api('POST', '/api/proxy/' + (pRun ? 'stop' : 'start')), pRun ? t('header.proxyStopped') : t('header.proxyStarted'), showToast);
    await apiCall(() => api('POST', '/api/component-states'));
    setPLoading(false);
  };

  const toggleXboxDns = async () => {
    setXLoading(true);
    const cfg = await api('GET', '/api/xbox-dns/config');
    if (cfg) {
      await apiCall(() => api('POST', '/api/xbox-dns/config', { ...cfg, enabled: !xRun }), xRun ? t('header.xboxDnsOff') : t('header.xboxDnsOn'), showToast);
      await apiCall(() => api('POST', '/api/component-states'));
    }
    setXLoading(false);
  };

  return (
    <header className="header">
      <div className="header-svc-group">
        <button
          className={'header-svc' + (zRun ? ' on' : '') + (zLoading ? ' loading' : '')}
          onClick={toggleZapret}
          disabled={zLoading}
        >
          {zLoading ? <span className="header-svc-spinner" /> : <span className="header-svc-dot" />}
          <span>{t('header.zapret')}</span>
          <span>{zRun ? 'ON' : 'OFF'}</span>
        </button>

        <button
          className={'header-svc' + (pRun ? ' on' : '') + (pLoading ? ' loading' : '')}
          onClick={toggleProxy}
          disabled={pLoading}
        >
          {pLoading ? <span className="header-svc-spinner" /> : <span className="header-svc-dot" />}
          <span>{t('header.proxy')}</span>
          <span>{pRun ? ':' + port : 'OFF'}</span>
        </button>

        <button
          className={'header-svc' + (xRun ? ' on' : '') + (xLoading ? ' loading' : '')}
          onClick={toggleXboxDns}
          disabled={xLoading}
        >
          {xLoading ? <span className="header-svc-spinner" /> : <span className="header-svc-dot" />}
          {ICONS.xbox}
          <span>{xRun ? 'ON' : 'OFF'}</span>
        </button>
      </div>

      <span className="header-sep" />

      <span className="header-right" />

      <button className="header-btn" onClick={onOpenLogs}>
        {ICONS.fileText}
      </button>

      <button className="header-btn" onClick={onToggleTheme}>
        {isDark ? ICONS.sun : ICONS.moon}
      </button>
    </header>
  );
}
