import { useState, useEffect, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import { api, apiCall } from '../api';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import { useDebouncedSave } from '../hooks/useDebouncedSave';

let autoChecked = false;

export default function SettingsPage({ status, showToast }) {
  const { t, lang, changeLang } = useT();
  const [config, setConfig] = useState(null);
  const [versions, setVersions] = useState(null);

  const [zpuiCheck, setZpuiCheck] = useState({ state: 'idle', current: null, latest: null });
  const [zapretCheck, setZapretCheck] = useState({ state: 'idle', current: null, latest: null });

  const loadConfig = async () => {
    const d = await api('GET', '/api/config');
    if (d) setConfig(d);
  };

  const loadVersions = async () => {
    const d = await api('GET', '/api/versions');
    if (d) setVersions(d);
  };

  useEffect(() => { loadConfig(); }, []);
  usePolling(loadVersions, 10000);

  useEffect(() => {
    if (autoChecked) return;
    autoChecked = true;
    const id = setTimeout(() => {
      checkZpuiUpdate();
      checkZapretUpdate();
    }, 1500);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const rt = window.runtime;
    if (!rt?.EventsOn) return;
    const handler = (data) => {
      if (!data?.latest) return;
      if (data.component === 'ZPUI') {
        setZpuiCheck({ state: 'available', current: data.current || versions?.zpui, latest: data.latest });
      } else if (data.component === 'zapret') {
        setZapretCheck({ state: 'available', current: data.current || status?.zapret?.version, latest: data.latest });
      }
    };
    rt.EventsOn('update:available', handler);
    return () => { try { rt.EventsOff('update:available'); } catch {} };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [versions, status]);

  const saveConfig = useDebouncedSave('/api/config', 500, null);
  const update = useCallback((patch) => {
    setConfig(prev => ({ ...prev, ...patch }));
    if (config) saveConfig(patch, config);
  }, [saveConfig, config]);

  const handleLanguage = (newLang) => {
    changeLang(newLang);
    update({ language: newLang });
  };

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
    if (d?.error) { setZpuiCheck({ state: 'error', current: versions?.zpui, latest: null }); return; }
    const latest = d?.latest || d?.current || versions?.zpui;
    const hasUpdate = d?.update_needed === true;
    setZpuiCheck({ state: hasUpdate ? 'available' : 'latest', current: d?.current || versions?.zpui, latest });
  };

  const checkZapretUpdate = async () => {
    setZapretCheck({ state: 'checking', current: status?.zapret?.version, latest: null });
    const d = await api('GET', '/api/updates/check/zapret');
    if (d?.error) { setZapretCheck({ state: 'error', current: status?.zapret?.version, latest: null }); return; }
    const latest = d?.latest_version || d?.latest;
    const hasUpdate = d?.update_needed === true;
    setZapretCheck({ state: hasUpdate ? 'available' : 'latest', current: d?.current_version || status?.zapret?.version, latest: latest || status?.zapret?.version });
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
      <div className="page-title">{t('settings.title')}</div>
      <div className="set-columns">

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
            <div className="set-lang-row">
              <button className="set-lang-arrow" onClick={() => {
                const langs = ['ru', 'en'];
                const idx = langs.indexOf(lang);
                const prev = langs[(idx - 1 + langs.length) % langs.length];
                handleLanguage(prev);
              }} data-tooltip={t('settings.prevLang')}>‹</button>
              <button className={'set-theme-btn sm' + (lang === 'ru' ? ' active' : '')} onClick={() => handleLanguage('ru')}>RU</button>
              <button className={'set-theme-btn sm' + (lang === 'en' ? ' active' : '')} onClick={() => handleLanguage('en')}>EN</button>
              <button className="set-lang-arrow" onClick={() => {
                const langs = ['ru', 'en'];
                const idx = langs.indexOf(lang);
                const next = langs[(idx + 1) % langs.length];
                handleLanguage(next);
              }} data-tooltip={t('settings.nextLang')}>›</button>
            </div>
          </div>
          <MiniRow label={t('settings.autoStartWindows')}><Switch checked={config.autostart || false} onChange={() => handleAutostart(!config.autostart)} /></MiniRow>
          <MiniRow label={t('settings.startMinimized')}><Switch checked={config.start_minimized || false} onChange={() => update({ start_minimized: !config.start_minimized })} /></MiniRow>
          <MiniRow label={t('settings.closeToTray')}><Switch checked={config.close_to_tray !== false} onChange={() => update({ close_to_tray: !config.close_to_tray })} /></MiniRow>
          <MiniRow label={t('settings.updateCheck')}><Switch checked={config.auto_update_check !== false} onChange={() => update({ auto_update_check: !config.auto_update_check })} /></MiniRow>
          <MiniRow label={t('settings.showStrategyColors')}><Switch checked={config.show_strategy_colors !== false} onChange={() => update({ show_strategy_colors: !config.show_strategy_colors })} /></MiniRow>
          <MiniRow label={t('settings.notifyStrategyTest')}><Switch checked={config.notify_strategy_test === true} onChange={() => update({ notify_strategy_test: !config.notify_strategy_test })} /></MiniRow>
        </div>

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
                <span className="upd-ver">v{status?.zapret?.version || zapretCheck.current || '—'}</span>
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
            {satellites.map(s => {
              const isInstalled = versions?.installed?.[s.key] !== false;
              const ver = versions?.[s.key];
              return (
                <div key={s.key} className="sat-card" style={{ padding: '5px 8px' }}>
                  <span className="sat-name" style={{ fontSize: 10, minWidth: 65 }}>{s.name}</span>
                  <span className="sat-ver" style={{ fontSize: 9, color: isInstalled ? 'var(--text-secondary)' : 'var(--text-tertiary)' }}>
                    {!isInstalled ? '—' : ver && ver !== '0.0.0' ? 'v' + ver : '—'}
                  </span>
                  <span className="sat-file" style={{ fontSize: 8 }}>{s.file}</span>
                </div>
              );
            })}
          </div>
        </div>

        <div className="section">
          <div className="section-title">{t('settings.zapretSection')}</div>
          <div className="set-row" style={{ padding: '3px 0' }}>
            <div className="set-row-info"><span className="set-row-title">{t('settings.localInstall')}</span></div>
            <span className="set-static mono" style={{ fontSize: 10 }}>{'<app>\\zapret\\'}</span>
          </div>
          <div className="set-row" style={{ padding: '3px 0' }}>
            <div className="set-row-info"><span className="set-row-title">{t('settings.status')}</span></div>
            <span className={'set-static ' + (config.zapret_skipped ? 'warn' : status?.zapret?.status === 'running' ? 'ok' : 'err')} style={{ fontSize: 10 }}>
              {config.zapret_skipped ? t('zapret.skippedStatus') : status?.zapret?.status === 'running' ? t('status.running') : t('status.stopped')}
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
          {config.zapret_skipped && (
            <div className="set-row" style={{ padding: '3px 0' }}>
              <div className="set-row-info">
                <span className="set-row-title">{t('settings.installZapret')}</span>
                <span className="set-row-desc">{t('settings.installZapretDesc')}</span>
              </div>
              <button className="btn btn-accent btn-xs" onClick={() => {
                if (window.confirm(t('settings.installZapretConfirm'))) {
                  api('POST', '/api/app/restart');
                }
              }}>
                {t('settings.installZapretBtn')}
              </button>
            </div>
          )}
        </div>

        <div className="section set-notif-section">
          <div className="set-notif-head">
            <span className="section-title">{t('settings.notifications')}</span>
            <div className="set-notif-actions">
              <Switch checked={config.notifications_enabled !== false} onChange={() => update({ notifications_enabled: !config.notifications_enabled })} />
              {config.notifications_enabled !== false && (
                <button className="btn btn-ghost btn-xs" onClick={async () => {
                  const d = await api('POST', '/api/test-notification');
                  if (d?.error) showToast(d.error, 'error');
                  else showToast(t('settings.testNotificationSent'), 'success');
                }}>
                  {t('settings.test')}
                </button>
              )}
            </div>
          </div>
          {config.notifications_enabled !== false && (
            <div className="set-notif-grid">
              <CompactNotif label={t('settings.notifZpuiUpdates')}
                checked={config.notify_zpui_updates !== false}
                onChange={() => update({ notify_zpui_updates: !config.notify_zpui_updates })} />
              <CompactNotif label={t('settings.notifZapretUpdates')}
                checked={config.notify_zapret_updates !== false}
                onChange={() => update({ notify_zapret_updates: !config.notify_zapret_updates })} />
              <CompactNotif label={t('settings.notifMissingFiles')}
                checked={config.notify_missing_files !== false}
                onChange={() => update({ notify_missing_files: !config.notify_missing_files })} />
              <CompactNotif label={t('settings.notifServiceStatus')}
                checked={config.notify_service_status === true}
                onChange={() => update({ notify_service_status: !config.notify_service_status })} />
              <div className={'set-cnotiff-wide' + (config.notify_resource_drop ? '' : ' dimmed')}>
                <div className="cnotiff-left">
                  <Switch checked={config.notify_resource_drop === true}
                    onChange={() => update({ notify_resource_drop: !config.notify_resource_drop })} />
                  <span className="set-cnotiff-label">{t('settings.notifResourceDrop')}</span>
                </div>
                <div className="cnotiff-right">
                  <input type="range" min="10" max="100" step="5" value={config.resource_drop_pct || 70}
                    onChange={e => update({ resource_drop_pct: parseInt(e.target.value) })}
                    className="set-threshold-slider" />
                  <span className="set-threshold-val">{config.resource_drop_pct || 70}%</span>
                </div>
              </div>
            </div>
          )}
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

function CompactNotif({ label, checked, onChange }) {
  return (
    <div className="set-cnotiff">
      <span className="set-cnotiff-label">{label}</span>
      <Switch checked={checked} onChange={onChange} />
    </div>
  );
}
