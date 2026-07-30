import { useState, useEffect, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import ReleaseModal from '../components/ui/ReleaseModal';
import Modal from '../components/ui/Modal';
import { api, apiCall } from '../api';
import { useT } from '../i18n';
import { ArrowRight, Trash2, RefreshCw, Download, AlertTriangle, Power, Info, ExternalLink, AlertOctagon } from 'lucide-react';
import { usePolling } from '../hooks/usePolling';
import { useDebouncedSave } from '../hooks/useDebouncedSave';
import { useUpdateCheck } from '../hooks/useUpdateCheck';
import { useConfirm } from '../components/ui/ConfirmDialog';

export default function SettingsPage({ status, showToast, onOpenLogs }) {
  const { t, lang, changeLang } = useT();
  const confirm = useConfirm();
  const [config, setConfig] = useState(null);
  const [versions, setVersions] = useState(null);
  const [serviceInstalled, setServiceInstalled] = useState(false);
  const [reinstalling, setReinstalling] = useState(false);
  const [fullReinstalling, setFullReinstalling] = useState(false);
  const [showReleaseModal, setShowReleaseModal] = useState(false);
  const [showZapretRelease, setShowZapretRelease] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [verPanel, setVerPanel] = useState(null);
  const [zapretVerPanel, setZapretVerPanel] = useState(null);
  const [dlState, setDlState] = useState('idle');
  const [dlPct, setDlPct] = useState(0);
  const [dlDetail, setDlDetail] = useState('');
  const [devWarn, setDevWarn] = useState(null); // {version, latest, action}

  const { zpuiCheck, zapretCheck } = useUpdateCheck();

  const loadConfig = async () => {
    const d = await api('GET', '/api/config');
    if (d) setConfig(d);
  };

  const loadVerPanel = async () => {
    const d = await api('GET', '/api/updates/versions-panel');
    if (d && !d.error) setVerPanel(d);
  };

  const loadZapretVerPanel = async () => {
    const d = await api('GET', '/api/updates/zapret-versions-panel');
    if (d && !d.error) setZapretVerPanel(d);
  };

  const loadVersions = async () => {
    const d = await api('GET', '/api/versions');
    if (d) setVersions(d);
  };

  const loadServiceStatus = async () => {
    const d = await api('GET', '/api/zapret/service-installed');
    if (d) setServiceInstalled(!!d.installed);
  };

  useEffect(() => { loadConfig(); loadServiceStatus(); loadVerPanel(); loadZapretVerPanel(); }, []);
  usePolling(loadVersions, 10000);
  usePolling(loadServiceStatus, 15000);
  usePolling(loadVerPanel, 30000);
  usePolling(loadZapretVerPanel, 30000);

  useEffect(() => {
    api('GET', '/api/updates/downloaded').then(d => {
      if (d?.downloaded) setDlState('downloaded');
    });
  }, []);

  useEffect(() => {
    const rt = window.runtime;
    if (!rt) return;
    const onProgress = (data) => {
      setDlState('downloading');
      setDlPct(data?.percent || 0);
      if (data?.status) setDlDetail(t('settings.downloading'));
    };
    const onDone = (data) => {
      if (data?.ok) {
        setDlState('downloaded');
        setDlPct(100);
        setDlDetail('');
        loadVerPanel();
        loadVersions();
      } else {
        setDlState('error');
        setDlDetail(data?.error || t('common.error'));
        if (showToast && data?.error) showToast(data.error, 'error');
      }
    };
    rt.EventsOn('update:download', onProgress);
    rt.EventsOn('update:download-done', onDone);
    return () => {
      try { rt.EventsOff('update:download'); } catch {}
      try { rt.EventsOff('update:download-done'); } catch {}
    };
  }, [t, showToast]);

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

  // Определяет, dev ли версия (4+ сегментов и последний != 0)
  const isDevVersion = (v) => v && v.split('.').length >= 4 && v.split('.').pop() !== '0';

  // Inline-обновление ZPUI: проверка dev → подтверждение → скачивание
  const handleLaunchUpdater = async () => {
    const d = await api('POST', '/api/updates/launch-updater');
    if (d?.error) showToast(d.error, 'error');
  };

  const handleZpuiUpdate = async (latestVer) => {
    if (isDevVersion(latestVer)) {
      // Показываем диалог предупреждения о dev-версии
      setDevWarn({ type: 'zpui', version: latestVer });
      return;
    }
    // Стабильная версия — сразу качаем
    setDlState('downloading');
    setDlPct(0);
    setDlDetail(t('settings.downloading'));
    const d = await api('POST', '/api/updates/download');
    if (d?.error) {
      setDlState('error');
      setDlDetail(d.error);
      showToast(d.error, 'error');
    }
  };

  // Подтверждение установки dev-версии
  const confirmDevUpdate = async () => {
    if (!devWarn) return;
    setDevWarn(null);
    if (devWarn.type === 'zpui') {
      setDlState('downloading');
      setDlPct(0);
      setDlDetail(t('settings.downloading'));
      const d = await api('POST', '/api/updates/download');
      if (d?.error) {
        setDlState('error');
        setDlDetail(d.error);
        showToast(d.error, 'error');
      }
    } else if (devWarn.type === 'zapret') {
      await handleZapretDownload();
    }
  };

  // Inline-обновление Zapret
  const handleZapretUpdate = async (latestVer) => {
    if (isDevVersion(latestVer)) {
      setDevWarn({ type: 'zapret', version: latestVer });
      return;
    }
    await handleZapretDownload();
  };

  const handleZapretDownload = async () => {
    showToast(t('settings.updateStarted'), 'info');
    const r = await api('POST', '/api/zapret/full-reinstall');
    if (r?.error) {
      showToast(r.error, 'error');
    } else {
      showToast(t('settings.fullReinstallComplete'), 'success');
      loadZapretVerPanel();
      loadVerPanel();
      loadVersions();
    }
  };

  const handleApplyUpdate = async () => {
    setUpdating(true);
    await apiCall(() => api('POST', '/api/updates/apply'), t('settings.updateStarted'), showToast);
    setUpdating(false);
  };

  if (!config) return null;

  // Определяем ветку для отображения
  const currentBranch = verPanel?.branch || versions?.branch || 'stable';

  return (
    <div className="settings-page">
      <div className="set-columns">

        <div className="section set-main-section">
          <div className="section-title">{t('settings.appearanceSettings')}</div>
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
              }}>‹</button>
              <button className={'set-theme-btn sm' + (lang === 'ru' ? ' active' : '')} onClick={() => handleLanguage('ru')}>RU</button>
              <button className={'set-theme-btn sm' + (lang === 'en' ? ' active' : '')} onClick={() => handleLanguage('en')}>EN</button>
              <button className="set-lang-arrow" onClick={() => {
                const langs = ['ru', 'en'];
                const idx = langs.indexOf(lang);
                const next = langs[(idx + 1) % langs.length];
                handleLanguage(next);
              }}>›</button>
            </div>
          </div>
          <MiniRow label={t('settings.autoStartWindows')}><Switch checked={config.autostart || false} onChange={() => handleAutostart(!config.autostart)} /></MiniRow>
          <MiniRow label={t('settings.startMinimized')}><Switch checked={config.start_minimized || false} onChange={() => update({ start_minimized: !config.start_minimized })} /></MiniRow>
          <MiniRow label={t('settings.closeToTray')}><Switch checked={config.close_to_tray !== false} onChange={() => update({ close_to_tray: !config.close_to_tray })} /></MiniRow>
          <MiniRow label={t('settings.updateCheck')}><Switch checked={config.auto_update_check !== false} onChange={() => update({ auto_update_check: !config.auto_update_check })} /></MiniRow>

          <div className="set-notif-sub">
            <div className="set-notif-head" onClick={async () => {
              const r = await api('POST', '/api/test-notification');
              if (r?.error) showToast(r.error, 'error');
            }}>
              <span className="section-title">{t('settings.notifications')}</span>
            </div>
            <MiniRow label={t('settings.notifErrors')}><Switch checked={config.notify_errors === true} onChange={() => update({ notify_errors: !config.notify_errors })} /></MiniRow>
            <MiniRow label={t('settings.notifServiceCrash')}><Switch checked={config.notify_service_crash !== false} onChange={() => update({ notify_service_crash: !config.notify_service_crash })} /></MiniRow>
            <MiniRow label={t('settings.notifMissingFiles')}><Switch checked={config.notify_missing_files !== false} onChange={() => update({ notify_missing_files: !config.notify_missing_files })} /></MiniRow>
          </div>

          <div className="set-row set-row-bottom" style={{ padding: '3px 0', marginTop: 'auto' }}>
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
          <div className="section-title set-section-title">
            <div className="set-section-left">
              <span>{t('settings.updates')}</span>
              {currentBranch === 'dev' && (
                <span className="upd-badge-dev">
                  <AlertOctagon size={11} strokeWidth={2.5} />
                  DEV
                </span>
              )}
            </div>
            <button className="upd-btn-open upd-open-header" onClick={handleLaunchUpdater} title={t('settings.updaterWindow')}>
              <ExternalLink size={12} strokeWidth={2.2} style={{ marginRight: 4 }} />
              {t('settings.openUpdaterBtn')}
            </button>
          </div>

          <SimpleUpdateCard
            name="ZPUI"
            current={verPanel?.current || versions?.zpui}
            latest={verPanel?.cloud?.version}
            branch={currentBranch}
            checkState={zpuiCheck.state}
            updateNeeded={zpuiCheck.state === 'available'}
            onShowRelease={() => setShowReleaseModal(true)}
            onUpdate={() => handleZpuiUpdate(verPanel?.cloud?.version || zpuiCheck.latest)}
            onApply={handleApplyUpdate}
            onDownload={handleZpuiUpdate}
            dlState={dlState}
            dlPct={dlPct}
            dlDetail={dlDetail}
            t={t}
          />

          <SimpleUpdateCard
            name="Zapret"
            current={zapretVerPanel?.current || status?.zapret?.version || zapretCheck.current}
            latest={zapretVerPanel?.cloud?.version || zapretCheck.latest}
            checkState={zapretCheck.state}
            updateNeeded={zapretCheck.state === 'available'}
            onShowRelease={() => setShowZapretRelease(true)}
            onUpdate={() => handleZapretUpdate(zapretVerPanel?.cloud?.version || zapretCheck.latest)}
            onApply={handleZapretDownload}
            dlState={dlState}
            dlPct={dlPct}
            dlDetail={dlDetail}
            t={t}
          />

          {/* Кнопка скачать стабильный установщик (только на dev-версиях) */}
          {currentBranch === 'dev' && (
            <div className="upd-stable-prompt">
              <div className="upd-stable-info">
                <AlertOctagon size={14} strokeWidth={2.2} className="upd-stable-icon" />
                <div>
                  <span className="upd-stable-title">{t('settings.stablePrompt')}</span>
                  <span className="upd-stable-desc">{t('settings.stablePromptDesc')}</span>
                </div>
              </div>
              <button className="btn btn-accent btn-sm" onClick={() => {
                window.open('https://github.com/suzcuaru/ZPUI/releases/latest', '_blank');
              }}>
                <Download size={12} strokeWidth={2.2} />
                {t('settings.downloadInstallerBtn')}
              </button>
            </div>
          )}

          {/* Dev-предупреждение */}
          <DevWarningModal
            open={devWarn !== null}
            version={devWarn?.version || ''}
            onConfirm={confirmDevUpdate}
            onCancel={() => setDevWarn(null)}
            t={t}
          />
        </div>

        <div className="section zpset-section">
          <div className="section-title" style={{ marginBottom: '10px' }}>Zapret</div>

          <div className="zpset-status-row">
            <span className={'zpset-status-dot ' + (config.zapret_skipped ? 'off' : status?.zapret?.status === 'running' ? 'on' : 'off')} />
            <div className="zpset-status-info">
              <span className="zpset-status-label">
                {config.zapret_skipped ? t('zapret.skippedStatus') : status?.zapret?.status === 'running' ? t('status.running') : t('status.stopped')}
              </span>
              <span className="zpset-status-meta">
                {status?.zapret?.version ? `v${status.zapret.version}` : '—'}
                {' · '}
                {(config.current_strategy || 'general.bat').replace('.bat', '')}
              </span>
            </div>
          </div>

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
                  const strategy = config.current_strategy || '';
                  const result = await api('POST', '/api/zapret/service/install', { strategy });
                  if (result?.error) {
                    showToast(result.error, 'error');
                  } else {
                    showToast(t('settings.serviceInstalled'), 'success');
                  }
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
                if (result?.error) {
                  showToast(result.error, 'error');
                } else {
                  showToast(t('settings.serviceReinstalled'), 'success');
                }
                setReinstalling(false);
                loadServiceStatus();
              }}>
                {reinstalling ? <span className="mini-spin" /> : <RefreshCw size={13} strokeWidth={2.2} />}
                {t('settings.reinstallServiceBtn')}
              </button>
            </div>
          </div>

          <div className="zpset-danger-zone">
            <div className="zpset-danger-info">
              <AlertTriangle size={14} strokeWidth={2.2} className="zpset-danger-icon" />
              <div>
                <span className="zpset-danger-title">{t('settings.fullReinstall')}</span>
                <span className="zpset-danger-desc">{t('settings.fullReinstallDesc')}</span>
              </div>
            </div>
            <button
              className="btn btn-danger btn-sm"
              disabled={fullReinstalling}
              onClick={async () => {
                if (!await confirm({ message: t('settings.fullReinstallConfirm'), variant: 'danger', confirmText: t('settings.fullReinstallBtn') })) return;
                setFullReinstalling(true);
                showToast(t('settings.fullReinstallStarted'), 'info');
                const result = await api('POST', '/api/zapret/full-reinstall');
                setFullReinstalling(false);
                if (result?.error) {
                  showToast(result.error, 'error');
                } else {
                  showToast(t('settings.fullReinstallComplete'), 'success');
                }
                loadServiceStatus();
                loadVerPanel();
                loadZapretVerPanel();
                loadVersions();
              }}
            >
              {fullReinstalling ? <span className="mini-spin" /> : <RefreshCw size={13} strokeWidth={2.2} />}
              {t('settings.fullReinstallBtn')}
            </button>
          </div>

          {config.zapret_skipped && (
            <div className="zpset-install-prompt">
              <div className="zpset-install-info">
                <Download size={14} strokeWidth={2.2} className="zpset-install-icon" />
                <div>
                  <span className="zpset-install-title">{t('settings.installZapret')}</span>
                  <span className="zpset-install-desc">{t('settings.installZapretDesc')}</span>
                </div>
              </div>
              <button className="btn btn-accent btn-sm" onClick={async () => {
                if (await confirm({ message: t('settings.installZapretConfirm'), confirmText: t('settings.installZapretBtn') })) {
                  api('POST', '/api/app/restart');
                }
              }}>
                {t('settings.installZapretBtn')}
              </button>
            </div>
          )}
        </div>

      </div>

      <ReleaseModal
        open={showReleaseModal}
        onClose={() => setShowReleaseModal(false)}
        currentVersion={zpuiCheck.current || versions?.zpui}
        latestVersion={zpuiCheck.latest}
        target="zpui"
      />
      <ReleaseModal
        open={showZapretRelease}
        onClose={() => setShowZapretRelease(false)}
        currentVersion={zapretVerPanel?.current || zapretCheck.current}
            latestVersion={zapretVerPanel?.cloud?.version || zapretCheck.latest}
        target="zapret"
      />
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

function SimpleUpdateCard({ name, current, latest, checkState, updateNeeded, onShowRelease, onUpdate, onApply, dlState, dlPct, dlDetail, t }) {
  const fmtVer = (v) => v ? (v.startsWith('v') ? v : 'v' + v) : '—';
  const hasLatest = latest && latest !== current;
  const isDev = (v) => v && v.split('.').length >= 4 && v.split('.').pop() !== '0';

  // Состояния: idle → downloading → downloaded → applying
  const isDownloading = dlState === 'downloading' || dlState === 'download' || dlState === 'preparing';
  const isDownloaded = dlState === 'downloaded';
  const isError = dlState === 'error';
  const isChecking = checkState === 'checking';

  return (
    <div className={'upd-card' + (isDownloading ? ' downloading' : '')}>
      <div className="upd-info">
        <div className="upd-simple-row">
          <span className="upd-name">{name}</span>
          <span className="upd-cur-ver">{fmtVer(current)}</span>
          {hasLatest && updateNeeded && (
            <>
              <ArrowRight size={11} strokeWidth={2.5} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
              <span className="upd-new-ver">{fmtVer(latest)}</span>
            </>
          )}
          {isDev(latest || current) && (
            <span className="upd-badge-dev-sm">DEV</span>
          )}
        </div>
      </div>
      <div className="upd-actions">
        {isDownloading && (
          <div className="upd-progress-wrap">
            <div className="upd-progress-bar">
              <div className="upd-progress-fill" style={{ width: (dlPct || 0) + '%' }} />
            </div>
            <span className="upd-progress-pct">{Math.round(dlPct || 0)}%</span>
            {dlDetail && <span className="upd-progress-label">{dlDetail}</span>}
          </div>
        )}
        {isDownloaded && !isDownloading ? (
          <button className="upd-btn-apply" onClick={onApply}>
            <RefreshCw size={11} strokeWidth={2.2} />
            {t('settings.applyUpdate')}
          </button>
        ) : isError && !isDownloading ? (
          <span className="upd-status error">{t('common.error')}</span>
        ) : isChecking ? (
          <span className="upd-status checking">
            <span className="mini-spin" style={{ display:'inline-block', width:10, height:10, borderWidth:1.5, marginRight:4, verticalAlign:'middle' }} />
            {t('settings.checkingUpdates')}
          </span>
        ) : updateNeeded && !isDownloading ? (
          <button className="upd-btn-check" onClick={onUpdate}>
            <Download size={11} strokeWidth={2.2} />
            {t('common.update')}
          </button>
        ) : (
          <span className="upd-status latest">{t('status.upToDate')}</span>
        )}
      </div>
    </div>
  );
}

function DevWarningModal({ open, version, onConfirm, onCancel, t }) {
  if (!open) return null;
  return (
    <div className="modal-overlay" onClick={onCancel}>
      <div className="modal-box wide-sm" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-title">{t('settings.devWarningTitle')}</span>
          <button className="modal-close" onClick={onCancel}>×</button>
        </div>
        <div className="modal-body">
          <div className="dev-warn-content">
            <AlertOctagon size={36} strokeWidth={1.8} className="dev-warn-icon" />
            <div className="dev-warn-text">
              <p><strong>{version}</strong> — {t('settings.devWarningLine1')}</p>
              <p>{t('settings.devWarningLine2')}</p>
              <p>{t('settings.devWarningLine3')}</p>
            </div>
          </div>
        </div>
        <div className="modal-footer">
          <button className="btn" onClick={onCancel}>{t('common.cancel')}</button>
          <button className="btn btn-accent" onClick={onConfirm}>
            <Download size={13} strokeWidth={2.2} style={{ marginRight: 6 }} />
            {t('settings.downloadDevBtn')}
          </button>
        </div>
      </div>
    </div>
  );
}
