import { useState } from 'react';
import { api, apiCall } from '../../api';
import { LANG } from '../../lang';

const ICONS = {
  bolt: <svg viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>,
  globe: <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10A15.3 15.3 0 0 1 12 2z"/></svg>,
  sun: <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>,
  moon: <svg viewBox="0 0 24 24"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>,
  fileText: <svg viewBox="0 0 24 24"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>,
  menu: <svg viewBox="0 0 24 24"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg>,
};

export default function Header({ status, onOpenLogs, isDark, onToggleTheme, collapsed, onToggleCollapse, showToast, onNavigate }) {
  const zRun = status?.zapret?.status === 'running';
  const pRun = status?.proxy?.running === true;
  const port = status?.proxy?.port || '—';

  const [zLoading, setZLoading] = useState(false);
  const [pLoading, setPLoading] = useState(false);

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

  return (
    <header className="header">
      <button className="header-btn" onClick={onToggleCollapse} data-tooltip={collapsed ? 'Развернуть' : 'Свернуть'}>
        {ICONS.menu}
      </button>

      <div className="header-logo" onClick={() => onNavigate('about')} data-tooltip="О программе">
        <span className="header-logo-text">ZPUI</span>
      </div>

      <div className="header-svc-group">
        <button
          className={'header-svc' + (zRun ? ' on' : '') + (zLoading ? ' loading' : '')}
          onClick={toggleZapret}
          disabled={zLoading}
          data-tooltip={zRun ? 'Остановить Zapret' : 'Запустить Zapret'}
        >
          {zLoading ? (
            <span className="header-svc-spinner" />
          ) : (
            <span className={'header-svc-dot' + (zRun ? '' : '')} />
          )}
          <span>Zapret</span>
          <span>{zRun ? 'Работает' : 'Стоп'}</span>
        </button>

        <button
          className={'header-svc' + (pRun ? ' on' : '') + (pLoading ? ' loading' : '')}
          onClick={toggleProxy}
          disabled={pLoading}
          data-tooltip={pRun ? 'Остановить прокси' : 'Запустить прокси'}
        >
          {pLoading ? (
            <span className="header-svc-spinner" />
          ) : (
            <span className={'header-svc-dot' + (pRun ? '' : '')} />
          )}
          <span>Прокси</span>
          <span>{pRun ? ':' + port : 'Стоп'}</span>
        </button>
      </div>

      <span className="header-sep" />

      <button className="header-btn" onClick={onOpenLogs} data-tooltip="Логи">
        {ICONS.fileText}
      </button>

      <button className="header-btn" onClick={onToggleTheme} data-tooltip={isDark ? 'Светлая тема' : 'Тёмная тема'}>
        {isDark ? ICONS.sun : ICONS.moon}
      </button>
    </header>
  );
}
