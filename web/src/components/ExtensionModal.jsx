import { useState, useEffect, useCallback, useRef } from 'react';
import Modal from './ui/Modal';
import StatePill from './ui/StatePill';
import { api, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { useToast } from '../context/ToastContext';
import { useConfirm } from './ui/ConfirmDialog';
import { usePolling } from '../hooks/usePolling';
import { useUpdateFlow, startExtensionUpdate } from '../hooks/useUpdateFlow';
import { MonitorCheck, Puzzle, RefreshCw } from 'lucide-react';

const EXT_INFO_ROUTE = '/api/extension/info';

// ExtensionModal — «Браузерный плагин»: регистрация расширения ZPUI в
// Chromium-браузерах, обновление пакета с сервера и включение на карточке
// (Chromium ставит внешние расширения выключенными — нужен заход юзера).
// Каркас и поведение — McpModal: SWR-кэш + поллинг + тосты + confirm.
// Скачивание и установка — через общую модалку прогресса (useUpdateFlow,
// component "extension"), эта модалка остаётся под ней и досматривает итог.
export default function ExtensionModal({ open, onClose }) {
  const showToast = useToast();
  const { t } = useT();
  const confirm = useConfirm();
  const flow = useUpdateFlow();
  const [info, setInfo] = useState(null);
  const [selected, setSelected] = useState(() => new Set());
  const [busy, setBusy] = useState(false);
  const selectionInitRef = useRef(false);

  const load = useCallback(async () => {
    const r = await api('GET', EXT_INFO_ROUTE);
    if (!r || r.error) return;
    apiCachePut(EXT_INFO_ROUTE, r);
    setInfo(r);
  }, []);

  useEffect(() => {
    if (!open) return;
    selectionInitRef.current = false;
    const cached = apiCacheGet(EXT_INFO_ROUTE);
    if (cached) setInfo(cached);
    load();
  }, [open, load]);

  // Чекбоксы выставляются один раз за открытие, из ответа бэка
  // (selected = расширение уже стоит), чтобы поллинг не перетирал выбор.
  useEffect(() => {
    if (!info || selectionInitRef.current) return;
    selectionInitRef.current = true;
    setSelected(new Set((info.browsers || []).filter((b) => b.selected).map((b) => b.id)));
  }, [info]);

  // Итог установки из модалки прогресса: перечитать инфо и показать подсказку
  // про включение (пока модалка прогресса не закрыта — просто перечитать).
  useEffect(() => {
    if (!open) return;
    const onInstalled = () => load();
    window.addEventListener('zpui:extension-installed', onInstalled);
    return () => window.removeEventListener('zpui:extension-installed', onInstalled);
  }, [open, load]);

  usePolling(load, 10000, [load], open);

  const toggle = useCallback((id) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // Идёт скачивание/установка плагина (модалка прогресса поверх).
  const updating = flow.open && flow.component === 'extension'
    && (flow.phase === 'downloading' || flow.phase === 'installing');
  const working = busy || updating || !!info?.updating;

  const allBrowsers = info?.browsers || [];
  const browsers = allBrowsers.filter((b) => b.found);
  const byId = Object.fromEntries(allBrowsers.map((b) => [b.id, b]));
  const checkedIds = browsers.filter((b) => selected.has(b.id)).map((b) => b.id);
  // «Актуально в браузере»: стоит ровно тот пакет, с которым работает приложение.
  const isCurrent = (b) => b.has_extension && b.installed_version === info?.version;
  const needInstall = checkedIds.filter((id) => !isCurrent(byId[id]));
  const removableIds = checkedIds.filter((id) => byId[id]?.has_extension);
  // Поставлено, но ещё не включено: браузер не перезапущен или плагин выключен.
  const enableBrowsers = browsers.filter((b) => b.has_extension && b.state !== 'enabled');
  const serverOffline = !!info?.server_error && !info?.latest;

  const doUpdate = async () => {
    if (working) return;
    // Обновляем выбранные; ничего не выбрано — где уже стоит.
    const ids = checkedIds.length > 0 ? checkedIds : [];
    startExtensionUpdate(ids);
  };

  // В браузерах стоит не тот пакет, что есть сейчас (например, после
  // обновления программы) — предложить перерегистрировать.
  const outdated = browsers.filter((b) => b.has_extension && b.installed_version !== info?.version);

  const doInstall = async (ids) => {
    const want = ids || needInstall;
    if (working || want.length === 0) return;
    // Пакета нет — качаем с сервера и ставим в выбранные той же модалкой.
    if (!info.available) {
      startExtensionUpdate(want);
      return;
    }
    setBusy(true);
    const r = await api('POST', '/api/extension/install', { ids: want });
    setBusy(false);
    if (r && r.error) {
      showToast(r.error, 'error');
      return;
    }
    showToast(t('extension.installDone'), 'success');
    load();
  };

  const doUninstall = async () => {
    if (working || removableIds.length === 0) return;
    const ok = await confirm({
      message: t('extension.uninstallConfirm'),
      variant: 'danger',
      confirmText: t('common.delete'),
    });
    if (!ok) return;
    setBusy(true);
    const r = await api('POST', '/api/extension/uninstall', { ids: removableIds });
    setBusy(false);
    if (r && r.error) showToast(r.error, 'error');
    else showToast(t('extension.uninstallDone'), 'success');
    load();
  };

  const openPage = async (id) => {
    const r = await api('POST', '/api/extension/open', { id });
    if (r && r.error) showToast(r.error, 'error');
  };

  // Пилюля состояния одного браузера: расширение видно, даже когда выключено.
  const pillFor = (b) => {
    if (b.state === 'enabled') {
      return (
        <StatePill on>
          {t('extension.stateEnabled')}
          {b.installed_version && <span className="mono">{` · ${b.installed_version}`}</span>}
        </StatePill>
      );
    }
    if (b.state === 'disabled') {
      return (
        <StatePill on variant="warn">
          {t('extension.stateDisabled')}
          {b.installed_version && <span className="mono">{` · ${b.installed_version}`}</span>}
        </StatePill>
      );
    }
    if (b.has_extension) {
      return (
        <StatePill on variant="warn">
          {t('extension.statePending')}
          {b.installed_version && <span className="mono">{` · ${b.installed_version}`}</span>}
        </StatePill>
      );
    }
    return <StatePill on={false}>{t('extension.notInstalled')}</StatePill>;
  };

  // Напоминание: куда поставили и что плагин надо включить (или перезапустить браузер).
  const reminder = () => {
    const off = enableBrowsers.filter((b) => b.state === 'disabled');
    const pending = enableBrowsers.filter((b) => b.state !== 'disabled');
    if (off.length === 0 && pending.length === 0) return null;
    return (
      <div className="ext-hint">
        <span className="ext-hint-text">
          {t('extension.installedWhere', { browsers: enableBrowsers.map((b) => b.name).join(', ') })}
          {' '}
          {pending.length > 0 && t('extension.restartHint')}
          {' '}
          {t('extension.enableHint')}
        </span>
        <div className="ext-enable-rows">
          {enableBrowsers.map((b) => (
            <div key={b.id} className="ext-enable-row">
              <span className="ext-enable-name">{b.name}</span>
              <span className="ext-enable-state">
                {b.state === 'disabled' ? t('extension.rowDisabled') : t('extension.rowPending')}
              </span>
              <button className="btn btn-sm" onClick={() => openPage(b.id)}>
                <MonitorCheck size={13} strokeWidth={2.2} />
                {t('extension.enable')}
              </button>
            </div>
          ))}
        </div>
      </div>
    );
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('extension.title')}
      icon={<Puzzle size={18} strokeWidth={2} />}
      tone={!info ? 'neutral' : enableBrowsers.length > 0 ? 'warning' : 'accent'}
      className="sst-frame"
      titleExtra={info?.version || null}
      footer={
        <>
          <button
            className="btn btn-accent"
            onClick={() => doInstall()}
            disabled={working || needInstall.length === 0}
          >
            {busy || updating ? t('extension.installing') : !info?.available ? t('extension.installDownload') : t('extension.install')}
          </button>
          <button className="btn" onClick={doUninstall} disabled={working || removableIds.length === 0}>
            {t('common.delete')}
          </button>
        </>
      }
    >
      <div className="ext-body">
        {!info ? (
          <div className="ext-empty">{t('common.loading')}</div>
        ) : (
          <>
            {!info.available && (
              <div className="ext-missing">
                <span className="ext-missing-text">
                  {info.latest ? t('extension.packageWillDownload') : serverOffline ? t('extension.packageMissingOffline') : t('extension.packageMissing')}
                </span>
              </div>
            )}
            {info.update_ready && info.available && (
              <div className="ext-update">
                <span className="ext-update-text">
                  <span className="ext-update-title">{t('extension.updateAvailable', { version: info.latest })}</span>
                  {info.notes && <span className="ext-update-notes">{info.notes}</span>}
                </span>
                <button className="btn btn-accent btn-sm" onClick={doUpdate} disabled={working}>
                  <RefreshCw size={13} strokeWidth={2.2} />
                  {updating ? t('extension.updating') : t('extension.update')}
                </button>
              </div>
            )}
            {serverOffline && (
              <div className="ext-server-hint">{t('extension.serverOffline')}</div>
            )}
            {info.available && outdated.length > 0 && !info.update_ready && (
              <div className="ext-update">
                <span className="ext-update-text">
                  <span className="ext-update-title">
                    {t('extension.outdatedInBrowsers', { browsers: outdated.map((b) => b.name).join(', '), version: info.version })}
                  </span>
                </span>
                <button
                  className="btn btn-accent btn-sm"
                  onClick={() => doInstall(outdated.map((b) => b.id))}
                  disabled={working}
                >
                  <RefreshCw size={13} strokeWidth={2.2} />
                  {t('extension.updateBrowsers')}
                </button>
              </div>
            )}

            {reminder()}

            <div className="ext-section">
              <div className="section-title set-section-title">
                <span>{t('extension.secBrowsers')}</span>
              </div>
              {browsers.length > 0 ? (
                <div className="ext-browsers">
                  {browsers.map((b) => (
                    <label key={b.id} className="ext-browser">
                      <input
                        type="checkbox"
                        checked={selected.has(b.id)}
                        onChange={() => toggle(b.id)}
                      />
                      <span className="ext-browser-name">{b.name}</span>
                      {b.running && b.has_extension && b.state !== 'enabled' && (
                        <span className="ext-browser-hint">
                          <RefreshCw size={11} strokeWidth={2} />
                          {t('extension.restartBrowser')}
                        </span>
                      )}
                      {pillFor(b)}
                    </label>
                  ))}
                </div>
              ) : (
                <div className="ext-empty">{t('extension.noBrowsers')}</div>
              )}
              {info.available && (
                <div className="ext-meta">
                  {t('extension.packageAt', {
                    version: info.version,
                    source: info.package_source === 'local' ? t('extension.sourceLocal') : t('extension.sourceApp'),
                  })}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
