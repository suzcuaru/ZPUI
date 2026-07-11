import { useState, useEffect, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import { api, apiCall } from '../api';
import { useT } from '../i18n';
import { ArrowRight, Trash2, RefreshCw, Download, AlertTriangle, Power } from 'lucide-react';
import { usePolling } from '../hooks/usePolling';
import { useDebouncedSave } from '../hooks/useDebouncedSave';
import { useUpdateCheck, resetZapretCheck, checkZapretUpdate } from '../hooks/useUpdateCheck';
import { useConfirm } from '../components/ui/ConfirmDialog';

export default function SettingsPage({ status, showToast, onOpenLogs }) {
  const { t, lang, changeLang } = useT();
  const confirm = useConfirm();
  const [config, setConfig] = useState(null);
  const [versions, setVersions] = useState(null);
  const [serviceInstalled, setServiceInstalled] = useState(false);
  const [reinstalling, setReinstalling] = useState(false);
  const [fullReinstalling, setFullReinstalling] = useState(false);
  const [engineStatus, setEngineStatus] = useState(null);
  const [switchingEngine, setSwitchingEngine] = useState(false);
  const [downloadingV2, setDownloadingV2] = useState(false);
  const [zapret2Check, setZapret2Check] = useState({ state: 'idle', current: '', latest: '' });

  const { zpuiCheck, zapretCheck } = useUpdateCheck();

  const loadConfig = async () => {
    const d = await api('GET', '/api/config');
    if (d) setConfig(d);
  };

  const loadVersions = async () => {
    const d = await api('GET', '/api/versions');
    if (d) setVersions(d);
  };

  const loadServiceStatus = async () => {
    const d = await api('GET', '/api/zapret/service-installed');
    if (d) setServiceInstalled(!!d.installed);
  };

  const loadEngineStatus = async () => {
    const d = await api('GET', '/api/engine/status');
    if (d) setEngineStatus(d);
  };

  useEffect(() => { loadConfig(); loadServiceStatus(); loadEngineStatus(); }, []);
  usePolling(loadVersions, 10000);
  usePolling(loadServiceStatus, 15000);
  usePolling(loadEngineStatus, 15000);

  useEffect(() => {
    const rt = typeof window !== "undefined" ? window.runtime : null;
    if (!rt) return;
    const onDownloaded = () => {
      setDownloadingV2(false);
      showToast(t("settings.engineV2Downloaded"), "success");
      loadEngineStatus();
    };
    const onError = (data) => {
      setDownloadingV2(false);
      showToast(t("settings.engineV2DownloadError", { error: data?.error || "?" }), "error");
    };
    rt.EventsOn("zapret2:downloaded", onDownloaded);
    rt.EventsOn("zapret2:download_error", onError);
    return () => {
      try { rt.EventsOff("zapret2:downloaded"); } catch {}
      try { rt.EventsOff("zapret2:download_error"); } catch {}
    };
  }, []);

  const saveConfig = useDebouncedSave('/api/config', 500, null);
  const update = useCallback((patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      saveConfig(patch, prev);
      return next;
    });
  }, [saveConfig]);

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

  const handleApplyUpdate = async () => {
    await apiCall(() => api('POST', '/api/components/update', { name: 'ZPUI' }), t('settings.updateStarted'), showToast);
  };

  const handleSwitchEngine = async (engine) => {
    if (engineStatus?.engine === engine) return;
    const msg = engine === 'v2' ? t('settings.engineSwitchConfirmV2') : t('settings.engineSwitchConfirm', { engine });
    if (!await confirm({ message: msg, variant: 'danger', confirmText: t('settings.engineSwitchBtn') })) return;
    setSwitchingEngine(true);
    const result = await api('POST', '/api/engine/switch', { engine });
    setSwitchingEngine(false);
    if (result?.error) {
      showToast(result.error, 'error');
      return;
    }
    if (result?.status === 'need_download') {
      showToast(t('settings.engineV2NeedDownload'), 'info');
    } else {
      showToast(t('settings.engineSwitched', { engine }), 'success');
    }
    loadEngineStatus();
    loadServiceStatus();
  };

  const handleDownloadV2 = async () => {
    setDownloadingV2(true);
    showToast(t('settings.engineV2Downloading'), 'info');
    const result = await api('POST', '/api/zapret2/download');
    if (result?.error) {
      showToast(result.error, 'error');
      setDownloadingV2(false);
    }
  };


  const checkZapret2Update = async () => {
    setZapret2Check(prev => ({ ...prev, state: 'checking' }));
    try {
      const info = await api('GET', '/api/updates/check/zapret2');
      if (info?.error) {
        setZapret2Check({ state: 'error', current: '', latest: '' });
        return;
      }
      setZapret2Check({
        state: info.update_needed ? 'available' : 'latest',
        current: info.current_version || '',
        latest: info.latest_version || '',
      });
    } catch {
      setZapret2Check({ state: 'error', current: '', latest: '' });
    }
  };

  const handleZapret2Update = async () => {
    showToast(t('common.updating'), 'info');
    await api('POST', '/api/zapret2/update/apply');
    setTimeout(() => checkZapret2Update(), 5000);
  };

  const handleComponentUpdate = async (name) => {
    const d = await api('POST', '/api/components/update', { name });
    if (d?.error) { showToast(t('settings.errorPrefix', { error: d.error }), 'error'); return; }
    showToast(t('settings.componentUpdateStarted', { name }));
    if (name === 'Zapret') {
      resetZapretCheck();
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

  return (
    <div className="settings-page">
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
          <div className="set-row" style={{ padding: '3px 0' }}>
            <div className="set-row-info">
              <span className="set-row-title">{t('settings.checkInterval')}</span>
              <span className="set-row-desc">{t('settings.checkIntervalDesc')}</span>
            </div>
            <div className="set-interval-row">
              {[5, 10, 15, 30].map(m => (
                <button key={m} className={'set-theme-btn sm' + ((config.resource_check_interval || 10) === m ? ' active' : '')}
                  onClick={() => update({ resource_check_interval: m })}>{m}m</button>
              ))}
            </div>
          </div>
        </div>

        <div className="section">
          <div className="section-title">{t('settings.componentsUpdates')}</div>
          <div className="upd-card" style={{ padding: '8px 10px' }}>
            <div className="upd-info">
              <span className="upd-name">ZPUI</span>
              <div className="upd-ver-row">
                <span className="upd-ver">v{zpuiCheck.current || versions?.zpui || '—'}</span>
                {zpuiCheck.state === 'available' && zpuiCheck.latest && (
                  <span className="upd-ver-new"><ArrowRight size={11} strokeWidth={2.5} /> v{zpuiCheck.latest}</span>
                )}
              </div>
            </div>
            {zpuiCheck.state === 'checking' ? (
              <span className="mini-spin" />
            ) : zpuiCheck.state === 'available' ? (
              <button className="upd-btn-check" onClick={handleApplyUpdate}
                style={{ borderColor: 'var(--warning)', color: 'var(--warning)', height: 22, fontSize: 10 }}>
                {t('common.update')}
              </button>
            ) : zpuiCheck.state === 'error' ? (
              <button className="upd-btn-check" onClick={onOpenLogs}
                style={{ borderColor: 'var(--danger)', color: 'var(--danger)', height: 22, fontSize: 10 }}
                data-tooltip={t('logs.title')}>
                {t('common.error')}
              </button>
            ) : zpuiCheck.state === 'latest' ? (
              <span className="upd-status latest" style={{ fontSize: 9, padding: '1px 6px' }}>{t('status.upToDate')}</span>
            ) : null}
          </div>
          <div className="upd-card" style={{ padding: '8px 10px' }}>
            <div className="upd-info">
              <span className="upd-name">Zapret</span>
              <div className="upd-ver-row">
                <span className="upd-ver">v{status?.zapret?.version || zapretCheck.current || '—'}</span>
                {zapretCheck.state === 'available' && zapretCheck.latest && (
                  <span className="upd-ver-new"><ArrowRight size={11} strokeWidth={2.5} /> v{zapretCheck.latest}</span>
                )}
              </div>
            </div>
            {zapretCheck.state === 'checking' ? (
              <span className="mini-spin" />
            ) : zapretCheck.state === 'available' ? (
              <button className="upd-btn-check" onClick={() => handleComponentUpdate('Zapret')}
                style={{ borderColor: 'var(--warning)', color: 'var(--warning)', height: 22, fontSize: 10 }}>
                {t('common.update')}
              </button>
            ) : zapretCheck.state === 'error' ? (
              <button className="upd-btn-check" onClick={onOpenLogs}
                style={{ borderColor: 'var(--danger)', color: 'var(--danger)', height: 22, fontSize: 10 }}
                data-tooltip={t('logs.title')}>
                {t('common.error')}
              </button>
            ) : zapretCheck.state === 'latest' ? (
              <span className="upd-status latest" style={{ fontSize: 9, padding: '1px 6px' }}>{t('status.upToDate')}</span>
            ) : null}
          </div>
          <div className="upd-card" style={{ padding: '8px 10px' }}>
            <div className="upd-info">
              <span className="upd-name">Zapret2 <span style={{ fontSize: 8, opacity: 0.5, verticalAlign: 'super' }}>{t('settings.engineV2DevLabel')}</span></span>
              <div className="upd-ver-row">
                <span className="upd-ver">{engineStatus?.v2?.version || zapret2Check.current || '—'}</span>
                {zapret2Check.state === 'available' && zapret2Check.latest && (
                  <span className="upd-ver-new"><ArrowRight size={11} strokeWidth={2.5} /> v{zapret2Check.latest}</span>
                )}
              </div>
            </div>
            {zapret2Check.state === 'checking' ? (
              <span className="mini-spin" />
            ) : zapret2Check.state === 'available' ? (
              <button className="upd-btn-check" onClick={handleZapret2Update}
                style={{ borderColor: 'var(--warning)', color: 'var(--warning)', height: 22, fontSize: 10 }}>
                {t('common.update')}
              </button>
            ) : zapret2Check.state === 'latest' ? (
              <span className="upd-status latest" style={{ fontSize: 9, padding: '1px 6px' }}>{t('status.upToDate')}</span>
            ) : (
              <button className="upd-btn-check" onClick={checkZapret2Update}
                style={{ height: 22, fontSize: 10 }}>
                {t('common.check')}
              </button>
            )}
          </div>
        </div>

        <div className="section zpset-section">
          <div className="zpset-head">
            <span className="zpset-head-title">Zapret</span>
            <div className='zpset-engine-toggle'>
              <button className={'zpset-engine-btn' + (engineStatus?.engine === 'v1' || !engineStatus?.engine ? ' active' : '')} disabled={switchingEngine || downloadingV2 || (engineStatus?.engine === 'v1' || !engineStatus?.engine)} onClick={() => handleSwitchEngine('v1')}>v1</button>
              <button className={'zpset-engine-btn' + (engineStatus?.engine === 'v2' ? ' active' : '')} disabled={switchingEngine || downloadingV2 || engineStatus?.engine === 'v2'} onClick={() => handleSwitchEngine('v2')}>
                v2<span style={{ fontSize: 8, opacity: 0.6, marginLeft: 2, verticalAlign: 'super' }}>{t('settings.engineV2DevLabel')}</span>
              </button>
            </div>
            {switchingEngine && <span className='mini-spin' />}
            <span className='zpset-head-meta'>
              <span className={'zpset-status-dot ' + (config.zapret_skipped ? 'off' : status?.zapret?.status === 'running' ? 'on' : 'off')} />
              <span>{config.zapret_skipped ? t('zapret.skippedStatus') : status?.zapret?.status === 'running' ? t('status.running') : t('status.stopped')}</span>
              <span className='zpset-head-sep'>·</span>
              <span>{engineStatus?.engine === 'v2' ? (engineStatus?.v2?.version || '—') : (status?.zapret?.version ? 'v' + status.zapret.version : '—')}</span>
              <span className='zpset-head-sep'>·</span>
              <span>{engineStatus?.engine === 'v2' ? (engineStatus?.v2?.strategy || 'general2') : (config.current_strategy || 'general.bat').replace('.bat', '')}</span>
            </span>
          </div>

          {engineStatus?.engine === 'v2' && (
            <div style={{ fontSize: 10, color: 'var(--warning, #e6a817)', padding: '4px 10px', display: 'flex', alignItems: 'center', gap: 5 }}>
              <AlertTriangle size={12} strokeWidth={2.2} />
              <span>{t('settings.engineV2DevWarning')}</span>
            </div>
          )}

          {engineStatus?.engine === 'v2' && !engineStatus?.v2?.files_present && (
            <div className='zpset-v2-prompt'>
              <Download size={13} strokeWidth={2.2} />
              <span className='zpset-v2-title'>{t('settings.engineV2DownloadTitle')}</span>
              <button className='btn btn-accent btn-sm' disabled={downloadingV2} onClick={handleDownloadV2}>
                {downloadingV2 ? <span className='mini-spin' /> : null}
                {t('settings.engineV2DownloadBtn')}
              </button>
            </div>
          )}

          <div className="zpset-svc-row">
            <div className="zpset-svc-label">
              <Power size={13} strokeWidth={2} />
              <span>{serviceInstalled ? t('settings.serviceMode') : t('settings.processMode')}</span>
            </div>
            <div className="zpset-svc-actions">
              {serviceInstalled ? (
                <button className="btn btn-danger btn-sm" disabled={reinstalling} onClick={async () => {
                  if (!await confirm({ message: t('settings.removeServiceConfirm'), variant: 'danger', confirmText: t('settings.removeServiceBtn') })) return;
                  setReinstalling(true);
                  await apiCall(async () => api('POST', '/api/zapret/service/remove'), t('settings.serviceRemoved'), showToast);
                  setReinstalling(false);
                  loadServiceStatus();
                }}>
                  {reinstalling ? <span className="mini-spin" /> : <Trash2 size={13} strokeWidth={2.2} />}
                  {t('settings.removeServiceBtn')}
                </button>
              ) : (
                <button className="btn btn-accent btn-sm" disabled={reinstalling} onClick={async () => {
                  setReinstalling(true);
                  const strategy = engineStatus?.engine === 'v2' ? (engineStatus?.v2?.strategy || '') : (config.current_strategy || '');
                  const result = await api('POST', '/api/zapret/service/install', { strategy });
                  if (result?.error) { showToast(result.error, 'error'); } else { showToast(t('settings.serviceInstalled'), 'success'); }
                  setReinstalling(false);
                  loadServiceStatus();
                }}>
                  {reinstalling ? <span className="mini-spin" /> : <Download size={13} strokeWidth={2.2} />}
                  {t('settings.installServiceBtn')}
                </button>
              )}
              <button className="btn btn-sm" disabled={reinstalling} onClick={async () => {
                if (!await confirm({ message: t('settings.reinstallServiceConfirm'), variant: 'danger', confirmText: t('settings.reinstallServiceBtn') })) return;
                setReinstalling(true);
                await apiCall(async () => api('POST', '/api/zapret/stop'), null, showToast);
                await apiCall(async () => api('POST', '/api/zapret/service/remove'), null, showToast);
                await new Promise(r => setTimeout(r, 1000));
                const result = await api('POST', '/api/zapret/start');
                if (result?.error) { showToast(result.error, 'error'); } else { showToast(t('settings.serviceReinstalled'), 'success'); }
                setReinstalling(false);
                loadServiceStatus();
              }}>
                {reinstalling ? <span className="mini-spin" /> : <RefreshCw size={13} strokeWidth={2.2} />}
                {t('settings.reinstallServiceBtn')}
              </button>
            </div>
          </div>

          <div className="zpset-danger-zone">
            <AlertTriangle size={14} strokeWidth={2.2} className="zpset-danger-icon" />
            <span className="zpset-danger-title">{t('settings.fullReinstall')}</span>
            <button className="btn btn-danger btn-sm" disabled={fullReinstalling} onClick={async () => {
              if (!await confirm({ message: t('settings.fullReinstallConfirm'), variant: 'danger', confirmText: t('settings.fullReinstallBtn') })) return;
              setFullReinstalling(true);
              showToast(t('settings.fullReinstallStarted'), 'info');
              const result = await api('POST', '/api/zapret/full-reinstall');
              setFullReinstalling(false);
              if (result?.error) { showToast(result.error, 'error'); } else { showToast(t('settings.fullReinstallComplete'), 'success'); }
              loadServiceStatus();
            }}>
              {fullReinstalling ? <span className="mini-spin" /> : <RefreshCw size={13} strokeWidth={2.2} />}
              {t('settings.fullReinstallBtn')}
            </button>
          </div>

          {config.zapret_skipped && (
            <div className="zpset-install-prompt">
              <Download size={14} strokeWidth={2.2} className="zpset-install-icon" />
              <span className="zpset-install-title">{t('settings.installZapret')}</span>
              <button className="btn btn-accent btn-sm" onClick={async () => {
                if (await confirm({ message: t('settings.installZapretConfirm'), confirmText: t('settings.installZapretBtn') })) { api('POST', '/api/app/restart'); }
              }}>{t('settings.installZapretBtn')}</button>
            </div>
          )}
        </div>

        <div className="section set-notif-section">
          <div className="set-notif-head">
            <span className="section-title">{t('settings.notifications')}</span>
          </div>
          <div className="set-notif-grid">
            <CompactNotif label={t('settings.notifZpuiUpdates')}
              checked={config.notify_zpui_updates !== false}
              onChange={() => update({ notify_zpui_updates: !config.notify_zpui_updates })} />
            <CompactNotif label={t('settings.notifZapretUpdates')}
              checked={config.notify_zapret_updates !== false}
              onChange={() => update({ notify_zapret_updates: !config.notify_zapret_updates })} />
            <CompactNotif label={t('settings.notifServiceCrash')}
              checked={config.notify_service_crash !== false}
              onChange={() => update({ notify_service_crash: !config.notify_service_crash })} />
            <CompactNotif label={t('settings.notifErrors')}
              checked={config.notify_errors === true}
              onChange={() => update({ notify_errors: !config.notify_errors })} />
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
