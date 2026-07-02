import { useState, useEffect, useCallback, useRef } from 'react';
import Switch from '../components/ui/Switch';
import { api, apiCall } from '../api';
import { useT } from '../i18n';

export default function SettingsPage({ status, showToast }) {
  const { t, lang, changeLang } = useT();
  const [config, setConfig] = useState(null);
  const [versions, setVersions] = useState(null);
  const saveTimer = useRef(null);

  const [zpuiCheck, setZpuiCheck] = useState({ state: 'idle', current: null, latest: null });
  const [zapretCheck, setZapretCheck] = useState({ state: 'idle', current: null, latest: null });

  useEffect(() => { loadConfig(); loadVersions(); }, []);
  useEffect(() => { const iv = setInterval(loadVersions, 10000); return () => clearInterval(iv); }, []);

  const loadConfig = async () => {
    const d = await api('GET', '/api/config');
    if (d) setConfig(d);
  };

  const loadVersions = async () => {
    const d = await api('GET', '/api/versions');
    if (d) setVersions(d);
  };

  const update = useCallback((patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      clearTimeout(saveTimer.current);
      saveTimer.current = setTimeout(async () => {
        await api('POST', '/api/config', next);
      }, 500);
      return next;
    });
  }, []);

  const handleTheme = async (theme) => {
    update({ theme });
    if (theme === 'system') {
      const sys = await api('GET', '/api/system-theme');
      if (sys) document.documentElement.setAttribute('data-theme', sys);
    } else {
      document.documentElement.setAttribute('data-theme', theme);
    }
  };

  const handleAutostart = async (enabled) => {
    update({ autostart: enabled });
    await apiCall(() => api('POST', enabled ? '/api/autostart/enable' : '/api/autostart/disable'), null, showToast);
  };

  const checkZpuiUpdate = async () => {
    setZpuiCheck({ state: 'checking', current: versions?.zpui, latest: null });
    const d = await api('GET', '/api/updates/check/zpui');
    if (d?.error || !d?.repo_available) {
      setZpuiCheck({ state: 'latest', current: versions?.zpui, latest: versions?.zpui });
      return;
    }
    setZpuiCheck({
      state: d.update_needed ? 'available' : 'latest',
      current: d.current || versions?.zpui,
      latest: d.latest || versions?.zpui,
    });
  };

  const checkZapretUpdate = async () => {
    setZapretCheck({ state: 'checking', current: status?.zapret?.version, latest: null });
    const d = await api('GET', '/api/updates/check/zapret');
    if (d?.error) { setZapretCheck({ state: 'error', current: status?.zapret?.version, latest: null }); return; }
    const latest = d?.latest_version || d?.version || d?.latest;
    const hasUpdate = d?.update_available || d?.has_update || (latest && latest !== status?.zapret?.version);
    setZapretCheck({ state: hasUpdate ? 'available' : 'latest', current: status?.zapret?.version, latest: latest || status?.zapret?.version });
  };

  const handleApplyUpdate = async () => {
    await apiCall(() => api('POST', '/api/components/update', { name: 'ZPUI' }), t('settings.updateStarted'), showToast);
  };

  const handleComponentUpdate = async (name) => {
    const d = await api('POST', '/api/components/update', { name });
    if (d?.error) { showToast(t('settings.errorPrefix', { error: d.error }), 'error'); return; }
    showToast(t('settings.componentUpdateStarted', { name }));
    if (name === 'Zapret') {
      setZapretCheck({ state: 'idle', current: null, latest: null });
      const poll = async () => {
        for (let i = 0; i < 15; i++) {
          await new Promise(r => setTimeout(r, 3000));
          await checkZapretUpdate();
          loadVersions();
        }
      };
      poll();
    } else {
      setTimeout(loadVersions, 3000);
    }
  };

  if (!config) return null;

  const satellites = [
    { key: 'wizard',       name: 'Wizard',        file: 'wizard.exe' },
    { key: 'autoselect',   name: 'AutoSelect',    file: 'autoselect.exe' },
    { key: 'selfupdate',   name: 'SelfUpdate',    file: 'selfupdate.exe' },
    { key: 'zapretupdate', name: 'ZapretUpdate',  file: 'zapretupdate.exe' },
  ];

  const st = (state) => {
    if (state === 'latest') return t('status.upToDate');
    if (state === 'available') return t('status.updateAvailable');
    return t('common.error');
  };

  return (
    <div className="settings-page">
      <div className="set-columns">
        <div className="set-col">
          <div className="section">
            <div className="section-title">{t('settings.appearance')}</div>
            <div className="set-row">
              <div className="set-row-info"><span className="set-row-title">{t('settings.theme')}</span></div>
              <div className="set-theme-row">
                <button className={'set-theme-btn sm' + (config.theme === 'system' ? ' active' : '')} onClick={() => handleTheme('system')}>{t('settings.systemTheme')}</button>
                <button className={'set-theme-btn sm' + (config.theme === 'light' ? ' active' : '')} onClick={() => handleTheme('light')}>{t('settings.lightTheme')}</button>
                <button className={'set-theme-btn sm' + (config.theme === 'dark' ? ' active' : '')} onClick={() => handleTheme('dark')}>{t('settings.darkTheme')}</button>
              </div>
            </div>
            <div className="set-row">
              <div className="set-row-info"><span className="set-row-title">{t('settings.language')}</span></div>
              <div className="set-theme-row">
                <button className={'set-theme-btn sm' + (lang === 'ru' ? ' active' : '')} onClick={() => changeLang('ru')}>RU</button>
                <button className={'set-theme-btn sm' + (lang === 'en' ? ' active' : '')} onClick={() => changeLang('en')}>EN</button>
              </div>
            </div>
            <MiniRow label={t('settings.autoStartWindows')}><Switch checked={config.autostart || false} onChange={() => handleAutostart(!config.autostart)} /></MiniRow>
            <MiniRow label={t('settings.startMinimized')}><Switch checked={config.start_minimized || false} onChange={() => update({ start_minimized: !config.start_minimized })} /></MiniRow>
            <MiniRow label={t('settings.closeToTray')}><Switch checked={config.close_to_tray !== false} onChange={() => update({ close_to_tray: !config.close_to_tray })} /></MiniRow>
            <MiniRow label={t('settings.updateCheck')}><Switch checked={config.auto_update_check !== false} onChange={() => update({ auto_update_check: !config.auto_update_check })} /></MiniRow>
          </div>

          <div className="section">
            <div className="section-title">Zapret</div>
            <div className="set-row" style={{ padding: '3px 0' }}>
              <div className="set-row-info"><span className="set-row-title">{t('settings.localInstall')}</span></div>
              <span className="set-static mono" style={{ fontSize: 10 }}>{'<app>\\zapret\\'}</span>
            </div>
            <div className="set-row" style={{ padding: '3px 0' }}>
              <div className="set-row-info"><span className="set-row-title">{t('settings.status')}</span></div>
              <span className={'set-static ' + (status?.zapret?.status === 'running' ? 'ok' : 'err')} style={{ fontSize: 10 }}>
                {status?.zapret?.status === 'running' ? t('status.running') : t('status.stopped')}
              </span>
            </div>
            <div className="set-row" style={{ padding: '3px 0' }}>
              <div className="set-row-info">
                <span className="set-row-title">{t('settings.removeService')}</span>
                <span className="set-row-desc">{t('settings.removeServiceDesc')}</span>
              </div>
              <button className="btn btn-danger btn-xs" onClick={() => {
                if (window.confirm(t('settings.removeServiceConfirm'))) {
                  apiCall(async () => api('POST', '/api/zapret/service/remove'), t('settings.serviceRemoved'), showToast);
                }
              }}>
                {t('settings.removeServiceBtn')}
              </button>
            </div>
          </div>
        </div>

        <div className="set-col">
          <div className="section">
            <div className="section-title">{t('settings.componentsUpdates')}</div>
            <div className="upd-card" style={{ padding: '8px 10px' }}>
              <div className="upd-info">
                <span className="upd-name">ZPUI</span>
                <div className="upd-ver-row">
                  <span className="upd-ver">v{zpuiCheck.current || versions?.zpui || '—'}</span>
                  {zpuiCheck.state === 'available' && zpuiCheck.latest && (
                    <span className="upd-ver-new">→ v{zpuiCheck.latest}</span>
                  )}
                  {zpuiCheck.state !== 'idle' && zpuiCheck.state !== 'checking' && (
                    <span className={'upd-status ' + zpuiCheck.state} style={{ fontSize: 9, padding: '1px 6px' }}>{st(zpuiCheck.state)}</span>
                  )}
                </div>
              </div>
              {zpuiCheck.state === 'available' ? (
                <button className="upd-btn-check" onClick={handleApplyUpdate} style={{ borderColor: 'var(--warning)', color: 'var(--warning)', height: 22, fontSize: 10 }}>
                  {t('common.update')}
                </button>
              ) : (
                <button className={'upd-btn-check' + (zpuiCheck.state === 'checking' ? ' checking' : '')} onClick={checkZpuiUpdate} style={{ height: 22, fontSize: 10 }}>
                  {zpuiCheck.state === 'checking' ? <span className="mini-spin" /> : t('common.check')}
                </button>
              )}
            </div>
            <div className="upd-card" style={{ padding: '8px 10px' }}>
              <div className="upd-info">
                <span className="upd-name">Zapret</span>
                <div className="upd-ver-row">
                  <span className="upd-ver">v{zapretCheck.current || status?.zapret?.version || '—'}</span>
                  {zapretCheck.state === 'available' && zapretCheck.latest && (
                    <span className="upd-ver-new">→ v{zapretCheck.latest}</span>
                  )}
                  {zapretCheck.state !== 'idle' && zapretCheck.state !== 'checking' && (
                    <span className={'upd-status ' + zapretCheck.state} style={{ fontSize: 9, padding: '1px 6px' }}>{st(zapretCheck.state)}</span>
                  )}
                </div>
              </div>
              {zapretCheck.state === 'available' ? (
                <button className="upd-btn-check" onClick={() => handleComponentUpdate('Zapret')} style={{ borderColor: 'var(--warning)', color: 'var(--warning)', height: 22, fontSize: 10 }}>
                  {t('common.update')}
                </button>
              ) : (
                <button className={'upd-btn-check' + (zapretCheck.state === 'checking' ? ' checking' : '')} onClick={checkZapretUpdate} style={{ height: 22, fontSize: 10 }}>
                  {zapretCheck.state === 'checking' ? <span className="mini-spin" /> : t('common.check')}
                </button>
              )}
            </div>
            <div className="section-title" style={{ marginTop: 8, fontSize: 10 }}>{t('settings.satellites')}</div>
            <div className="sat-grid">
              {satellites.map(s => (
                <div key={s.key} className="sat-card" style={{ padding: '5px 8px' }}>
                  <span className="sat-name" style={{ fontSize: 10, minWidth: 65 }}>{s.name}</span>
                  <span className="sat-ver" style={{ fontSize: 9 }}>v{versions?.[s.key] || '—'}</span>
                  <span className="sat-file" style={{ fontSize: 8 }}>{s.file}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function MiniRow({ label, children }) {
  return (
    <div className="set-row" style={{ padding: '3px 0' }}>
      <div className="set-row-info"><span className="set-row-title">{label}</span></div>
      {children}
    </div>
  );
}
