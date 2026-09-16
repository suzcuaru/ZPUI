import { useState, useEffect, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import ReleaseModal from '../components/ui/ReleaseModal';
import { api, apiCall } from '../api';
import { useT } from '../i18n';
import { ArrowRight, Trash2, RefreshCw, Download, AlertTriangle, Power, HardDriveDownload, List, Palette } from 'lucide-react';
import { usePolling } from '../hooks/usePolling';
import { useDebouncedSave } from '../hooks/useDebouncedSave';
import { useUpdateCheck } from '../hooks/useUpdateCheck';
import { openUpdateModal, openUpdateModalForVersion, showUpdateModal, useUpdateFlow } from '../hooks/useUpdateFlow';
import ZapretVersionsModal from '../components/ui/ZapretVersionsModal';
import ThemeEditorModal from '../components/ThemeEditorModal';
import { useConfirm } from '../components/ui/ConfirmDialog';
import { rangePct } from '../utils';

// Экономный режим: уровни степени экономии и пороги заряда батареи
// (0 = «всегда», -1 = «от АКБ», далее 50..10 % с шагом 5).
const POWER_LEVEL_KEYS = ['settings.powerLow', 'settings.powerMedium', 'settings.powerMax'];
const POWER_THRESHOLDS = [0, -1, 50, 45, 40, 35, 30, 25, 20, 15, 10];

function thresholdLabel(threshold, t) {
  if (threshold === 0) return t('settings.powerThresholdAlways');
  if (threshold === -1) return t('settings.powerThresholdOnBattery');
  return threshold + '%';
}

export default function SettingsPage({ status, showToast, onOpenLogs }) {
  const { t, lang, changeLang } = useT();
  const confirm = useConfirm();
  const [config, setConfig] = useState(null);
  const [versions, setVersions] = useState(null);
  const [serviceInstalled, setServiceInstalled] = useState(false);
  const [reinstalling, setReinstalling] = useState(false);
  const [fullReinstalling, setFullReinstalling] = useState(false);
  const [releaseVer, setReleaseVer] = useState(null);          // версия ZPUI, открытая в модалке «что нового»
  const [zapretReleaseVer, setZapretReleaseVer] = useState(null); // версия Zapret
  const [showZapretVersions, setShowZapretVersions] = useState(false);
  const [verPanel, setVerPanel] = useState(null);
  const [zapretVerPanel, setZapretVerPanel] = useState(null);
  const [autostartLive, setAutostartLive] = useState(null); // live Windows state (effective)
  const [themeEditorOpen, setThemeEditorOpen] = useState(false);
  const [hasSavedCustom, setHasSavedCustom] = useState(false);

  const { zpuiCheck, zapretCheck } = useUpdateCheck();
  const flow = useUpdateFlow();

  const loadConfig = async () => {
    const d = await api('GET', '/api/config');
    if (d) setConfig(d);
  };

  const loadAutostart = async () => {
    const d = await api('GET', '/api/autostart/status');
    if (d && typeof d.enabled === 'boolean') setAutostartLive(d.enabled);
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

  useEffect(() => { loadConfig(); loadAutostart(); loadServiceStatus(); loadVerPanel(); loadZapretVerPanel(); }, []);
  usePolling(loadVersions, 10000);
  usePolling(loadServiceStatus, 15000);
  usePolling(loadVerPanel, 30000);
  usePolling(loadZapretVerPanel, 30000);

  // Кастомная тема: есть ли сохранённая в «БД» (кнопка «Своя» активирует её
  // сразу) + локальная синхронизация активной темы с App (zpui:theme-changed).
  useEffect(() => {
    api('GET', '/api/custom-theme').then((r) => setHasSavedCustom(!!(r && r.theme)));
  }, []);

  useEffect(() => {
    const onThemeChanged = (e) => {
      const v = e.detail?.theme;
      if (v === 'system' || v === 'dark' || v === 'light' || v === 'custom') {
        setConfig((prev) => (prev ? { ...prev, theme: v } : prev));
      }
    };
    window.addEventListener('zpui:theme-changed', onThemeChanged);
    return () => window.removeEventListener('zpui:theme-changed', onThemeChanged);
  }, []);

  useEffect(() => {
    const onZapretInstalled = () => { loadZapretVerPanel(); loadVersions(); };
    window.addEventListener('zpui:zapret-installed', onZapretInstalled);
    return () => window.removeEventListener('zpui:zapret-installed', onZapretInstalled);
  }, []);

  const saveConfig = useDebouncedSave('/api/config', 500, null);


  const update = useCallback((patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      saveConfig(patch, prev);
      return next;
    });
    // App.jsx перечитывает конфиг на это событие (например, экономный режим
    // меняет интервалы опроса).
    window.dispatchEvent(new CustomEvent('zpui:config-changed'));
  }, [saveConfig]);

  const handleLanguage = (newLang) => {
    changeLang(newLang);
    update({ language: newLang });
  };

  // Тема применяется в App (единая точка): он же переключит иконку в тайтлбаре.
  // Здесь только локальный отклик кнопок — какая активна.
  const handleTheme = (theme) => {
    setConfig(prev => ({ ...prev, theme }));
    window.dispatchEvent(new CustomEvent('zpui:theme-changed', { detail: { theme } }));
  };

  // «Своя»: применяет сохранённую кастомную тему, если есть; иначе (и по
  // кнопке «Настроить») открывает редактор.
  const handleThemeCustom = () => {
    if (config.theme === 'custom') { setThemeEditorOpen(true); return; }
    if (hasSavedCustom) handleTheme('custom');
    else setThemeEditorOpen(true);
  };

  const closeThemeEditor = () => {
    setThemeEditorOpen(false);
    api('GET', '/api/custom-theme').then((r) => setHasSavedCustom(!!(r && r.theme)));
  };

  const handleAutostart = async (enabled) => {
    setAutostartLive(enabled);
    await apiCall(() => api('POST', enabled ? '/api/autostart/enable' : '/api/autostart/disable'), null, showToast);
    // Состояние автозапуска хранится только в системе (Планировщик заданий) —
    // перечитываем реальный статус оттуда.
    setTimeout(loadAutostart, 400);
  };

  // Обновления: карточки ниже открывают модалку скачивания/установки.
  const startUpdate = useCallback((component) => {
    // Если этот компонент уже скачан — показываем модалку с кнопкой «Установить».
    if (flow.component === component && flow.phase === 'downloaded') {
      showUpdateModal();
      return;
    }
    openUpdateModal(component);
  }, [flow.component, flow.phase]);

  // Конфиг ещё грузится — скелетон в той же сетке (как на XboxDns/User),
  // пустой возврат давал «прыжок» пустой вкладки.
  if (!config) {
    return (
      <div className="settings-page">
      <div className="set-columns stagger">
        <div className="section set-main-section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '70%' }} /></div>
        <div className="set-side-col">
          <div className="section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '60%' }} /></div>
          <div className="section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '60%' }} /></div>
        </div>
      </div>
      </div>
    );
  }

  return (
    <div className="settings-page">
      <div className="set-columns stagger">

        <div className="section set-main-section">
          <div className="section-title">{t('settings.appearanceSettings')}</div>
          <div className="set-row">
            <div className="set-row-info"><span className="set-row-title">{t('settings.theme')}</span></div>
            <div className="set-theme-row">
              <button className={'btn btn-sm' + (config.theme === 'system' ? ' active' : '')} onClick={() => handleTheme('system')}>{t('settings.systemTheme')}</button>
              <button className={'btn btn-sm' + (config.theme === 'light' ? ' active' : '')} onClick={() => handleTheme('light')}>{t('settings.lightTheme')}</button>
              <button className={'btn btn-sm' + (config.theme === 'dark' ? ' active' : '')} onClick={() => handleTheme('dark')}>{t('settings.darkTheme')}</button>
              <button className={'btn btn-sm' + (config.theme === 'custom' ? ' active' : '')} onClick={handleThemeCustom} data-tooltip={t('settings.themeCustomTip')}>{t('settings.themeCustom')}</button>
              <button className="btn btn-sm btn-ghost" onClick={() => setThemeEditorOpen(true)} data-tooltip={t('settings.themeCustomizeTip')}>
                <Palette size={12} strokeWidth={2} />
                {t('settings.themeCustomize')}
              </button>
            </div>
          </div>
          <div className="set-row">
            <div className="set-row-info"><span className="set-row-title">{t('settings.language')}</span></div>
            <div className="set-lang-row">
              <button className={'btn btn-sm' + (lang === 'ru' ? ' active' : '')} onClick={() => handleLanguage('ru')}>RU</button>
              <button className={'btn btn-sm' + (lang === 'en' ? ' active' : '')} onClick={() => handleLanguage('en')}>EN</button>
            </div>
          </div>
          <MiniRow label={t('settings.autoStartWindows')}><Switch checked={autostartLive ?? false} onChange={() => handleAutostart(!autostartLive)} /></MiniRow>
          <MiniRow label={t('settings.startMinimized')}><Switch checked={config.start_minimized || false} onChange={() => update({ start_minimized: !config.start_minimized })} /></MiniRow>
          <MiniRow label={t('settings.closeToTray')}><Switch checked={config.close_to_tray !== false} onChange={() => update({ close_to_tray: !config.close_to_tray })} /></MiniRow>
          <MiniRow label={t('settings.updateCheck')}><Switch checked={config.auto_update_check !== false} onChange={() => update({ auto_update_check: !config.auto_update_check })} /></MiniRow>
          <MiniRow label={t('settings.autoStartProxy')}><Switch checked={config.auto_start_proxy || false} onChange={() => update({ auto_start_proxy: !config.auto_start_proxy })} /></MiniRow>
          <MiniRow label={t('settings.autoStartXboxDns')}><Switch checked={config.auto_start_xbox_dns || false} onChange={() => update({ auto_start_xbox_dns: !config.auto_start_xbox_dns })} /></MiniRow>

          <div className="set-notif-sub">
            <span className="section-title">{t('settings.powerSaving')}</span>
            <MiniRow label={t('settings.powerSavingEnable')}>
              <Switch checked={config.power_saving || false} onChange={() => update({ power_saving: !config.power_saving })} />
            </MiniRow>
            <div className={'pw-slider-row' + (config.power_saving ? '' : ' disabled')}>
              <span className="pw-slider-label">{t('settings.powerLevel')}</span>
              <input type="range" min="1" max="3" step="1" disabled={!config.power_saving}
                value={config.power_level || 1}
                onChange={(e) => update({ power_level: +e.target.value })}
                className="pw-slider slider-ui"
                style={{ '--val': rangePct(config.power_level || 1, 1, 3) + '%' }} />
              <span className="pw-slider-val">{t(POWER_LEVEL_KEYS[(config.power_level || 1) - 1])}</span>
            </div>
            {status?.power_battery?.has && (
              <div className={'pw-slider-row' + (config.power_saving ? '' : ' disabled')}>
                <span className="pw-slider-label">{t('settings.powerThreshold')}</span>
                <input type="range" min="0" max={POWER_THRESHOLDS.length - 1} step="1" disabled={!config.power_saving}
                  value={POWER_THRESHOLDS.indexOf(config.power_threshold ?? 0)}
                  onChange={(e) => update({ power_threshold: POWER_THRESHOLDS[+e.target.value] })}
                  className="pw-slider slider-ui"
                  style={{ '--val': rangePct(POWER_THRESHOLDS.indexOf(config.power_threshold ?? 0), 0, POWER_THRESHOLDS.length - 1) + '%' }} />
                <span className="pw-slider-val">{thresholdLabel(config.power_threshold ?? 0, t)}</span>
              </div>
            )}
          </div>

          <div className="set-row set-row-bottom" style={{ padding: '3px 0', marginTop: 'auto' }}>
            <div className="set-row-info">
              <span className="set-row-title">{t('settings.checkInterval')}</span>
              <span className="set-row-desc">{t('settings.checkIntervalDesc')}</span>
            </div>
            <div className="set-interval-row">
              {[5, 10, 15, 30].map(m => (
                <button key={m} className={'btn btn-sm' + ((config.resource_check_interval || 10) === m ? ' active' : '')}
                  onClick={() => update({ resource_check_interval: m })}>{m}m</button>
              ))}
            </div>
          </div>
        </div>

        <div className="set-side-col">
        <div className="section">
          <div className="section-title set-section-title">
            <span>{t('settings.updates')}</span>
          </div>

          <SimpleUpdateCard
            name="ZPUI"
            current={verPanel?.current || versions?.zpui}
            latest={verPanel?.latest || zpuiCheck.latest}
            checkState={zpuiCheck.state}
            updateNeeded={zpuiCheck.state === 'available' || verPanel?.avail === true}
            downloaded={flow.component === 'ZPUI' && flow.phase === 'downloaded'}
            onStartUpdate={() => startUpdate('ZPUI')}
            onShowRelease={(ver) => setReleaseVer(ver)}
            t={t}
          />

          <SimpleUpdateCard
            name="ZAPRET"
            current={zapretVerPanel?.current || status?.zapret?.version || zapretCheck.current}
            latest={zapretVerPanel?.latest || zapretCheck.latest}
            checkState={zapretCheck.state}
            updateNeeded={zapretCheck.state === 'available' || zapretVerPanel?.avail === true}
            downloaded={flow.component === 'zapret' && flow.phase === 'downloaded'}
            onStartUpdate={() => startUpdate('zapret')}
            onShowRelease={(ver) => setZapretReleaseVer(ver)}
            onVersions={() => setShowZapretVersions(true)}
            t={t}
          />

          <div className="upd-card">
            <div className="upd-info">
              <div className="upd-simple-row">
                <span className="upd-name">{t('settings.tgProxyCard')}</span>
                <span className="upd-cur-ver">{versions?.tgproxy ? 'v' + versions.tgproxy : '—'}</span>
              </div>
            </div>
            <div className="upd-actions">
              <span className="upd-status latest">{t('settings.tgProxyUpdWithZpui')}</span>
            </div>
          </div>
        </div>

        <div className="section zpset-section">
          <div className="zpset-head">
            <span className="zpset-head-title">ZAPRET</span>
            <span className={'zpset-pill ' + (status?.zapret?.status === 'running' && !config.zapret_skipped ? 'on' : 'off')}>
              <span className="zpset-pill-dot" />
              {config.zapret_skipped ? t('zapret.skippedStatus') : status?.zapret?.status === 'running' ? t('status.running') : t('status.stopped')}
              {status?.zapret?.version && (
                <span className="zpset-pill-ver">v{status.zapret.version}</span>
              )}
            </span>
          </div>
          <div className="zpset-sub">
            {t('zapret.strategy')}: {(config.current_strategy || 'general.bat').replace('.bat', '')}
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

      </div>

      <ReleaseModal
        open={releaseVer !== null}
        onClose={() => setReleaseVer(null)}
        currentVersion={zpuiCheck.current || versions?.zpui}
        latestVersion={zpuiCheck.latest}
        version={releaseVer || ''}
        target="zpui"
      />
      <ReleaseModal
        open={zapretReleaseVer !== null}
        onClose={() => setZapretReleaseVer(null)}
        currentVersion={zapretVerPanel?.current || zapretCheck.current}
        latestVersion={zapretVerPanel?.latest || zapretCheck.latest}
        version={zapretReleaseVer || ''}
        target="zapret"
      />
      <ZapretVersionsModal
        open={showZapretVersions}
        onClose={() => setShowZapretVersions(false)}
        installBusy={flow.phase === 'downloading' || flow.phase === 'installing'}
        onInstall={(version, source) => {
          setShowZapretVersions(false);
          openUpdateModalForVersion(version, source);
        }}
      />
      <ThemeEditorModal
        open={themeEditorOpen}
        onClose={closeThemeEditor}
        showToast={showToast}
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

// SimpleUpdateCard — карточка версии: клик по версии = «что нового»,
// при наличии обновления — кнопка «Скачать» (открывает модалку обновления),
// если архив уже скачан — кнопка «Установить».
// onVersions — необязательная кнопка «Версии» (список доступных версий с площадок).
function SimpleUpdateCard({ name, current, latest, checkState, updateNeeded, downloaded, onStartUpdate, onShowRelease, onVersions, t }) {
  const fmtVer = (v) => v ? (v.startsWith('v') ? v : 'v' + v) : '—';
  const hasLatest = latest && latest !== current;
  const verChip = (className, ver) => onShowRelease ? (
    <button type="button" className={`upd-ver-btn ${className}`} data-tooltip={t('settings.whatsNewTip')} onClick={() => onShowRelease(ver)}>{fmtVer(ver)}</button>
  ) : (
    <span className={className}>{fmtVer(ver)}</span>
  );
  const isChecking = checkState === 'checking';

  return (
    <div className="upd-card">
      <div className="upd-info">
        <div className="upd-simple-row">
          <span className="upd-name">{name}</span>
          {verChip('upd-cur-ver', current)}
          {hasLatest && updateNeeded && (
            <>
              <ArrowRight size={11} strokeWidth={2.5} style={{ color: 'var(--text-tertiary)', flexShrink: 0 }} />
              {verChip('upd-new-ver', latest)}
            </>
          )}
        </div>
      </div>
      <div className="upd-actions">
        {onVersions && (
          <button className="upd-btn-check" data-tooltip={t('settings.zapretVersionsTitle')} onClick={onVersions}>
            <List size={11} strokeWidth={2.2} />
            {t('settings.versionsBtn')}
          </button>
        )}
        {isChecking ? (
          <span className="upd-status checking">
            <span className="mini-spin" style={{ display:'inline-block', width:10, height:10, borderWidth:1.5, marginRight:4, verticalAlign:'middle' }} />
            {t('settings.checkingUpdates')}
          </span>
        ) : downloaded ? (
          <button className="upd-btn-check" onClick={onStartUpdate}>
            <HardDriveDownload size={11} strokeWidth={2.2} />
            {t('update.install')}
          </button>
        ) : updateNeeded ? (
          <button className="upd-btn-check" onClick={onStartUpdate}>
            <Download size={11} strokeWidth={2.2} />
            {t('common.download')}
          </button>
        ) : (
          <span className="upd-status latest">{t('status.upToDate')}</span>
        )}
      </div>
    </div>
  );
}

