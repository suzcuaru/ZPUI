import { useState, useEffect, useRef, useCallback } from 'react';
import { api, createStream } from '../api';
import { useT } from '../i18n';

function formatBytes(bytes) {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}

const STEP_PROGRESS = {
  connect: 8,
  health: 16,
  'decide-service': 24,
  'check-local': 32,
  install: 48,
  'install-service': 62,
  'strategy-prompt': 76,
  'auto-strategy': 90,
  done: 100,
};

function applyThemeAtStart() {
  const saved = localStorage.getItem('zpui-theme');
  if (saved === 'dark' || saved === 'light') {
    document.documentElement.setAttribute('data-theme', saved);
  } else {
    const sys = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', sys);
  }
}

export default function StartupScreen({ onComplete }) {
  const { t } = useT();
  const tRef = useRef(t);
  tRef.current = t;
  const [currentStep, setCurrentStep] = useState('connect');
  const [stepDone, setStepDone] = useState(false);
  const [progress, setProgress] = useState(2);
  const [error, setError] = useState(null);
  const [errorTitle, setErrorTitle] = useState('');
  const [errorLog, setErrorLog] = useState(null);

  // Модалка удаления службы (служебная, без локальной папки)
  const [removeServicePrompt, setRemoveServicePrompt] = useState(false);

  // Прогресс скачивания
  const [downloadProgress, setDownloadProgress] = useState(null);

  // Модалка "Запрет установлен vX"
  const [installPrompt, setInstallPrompt] = useState(null); // { version, strategy }

  // Автоподбор
  const [autoSelect, setAutoSelect] = useState(null); // { current, total, strategy, phase, detail }

  const aliveRef = useRef(true);
  const waitResolveRef = useRef(null);
  const eventsCleanupRef = useRef(null);
  const installedNowRef = useRef(false);
  const installedServiceRef = useRef(false);

  const go = useCallback(async (id, fn) => {
    if (id) setCurrentStep(id);
    setStepDone(false);
    try {
      await fn();
    } catch (e) {
      setErrorTitle(tRef.current('startup.errors.title'));
      setError(e.message || String(e));
      return false;
    }
    setStepDone(true);
    return true;
  }, []);

  const sleep = useCallback(ms => new Promise(r => setTimeout(r, ms)), []);

  const waitForAction = useCallback(() => new Promise(resolve => { waitResolveRef.current = resolve; }), []);

  // --- Обработчики модалки удаления службы ---
  const handleRemoveService = useCallback(async () => {
    const r = await api('POST', '/api/zapret/remove-system-service');
    if (r?.error) {
      setErrorTitle(tRef.current('startup.errors.title'));
      setError(r.error);
      return;
    }
    setRemoveServicePrompt(false);
    if (waitResolveRef.current) { waitResolveRef.current(true); waitResolveRef.current = null; }
  }, []);

  const handleSkipService = useCallback(() => {
    setRemoveServicePrompt(false);
    if (waitResolveRef.current) { waitResolveRef.current(false); waitResolveRef.current = null; }
  }, []);

  // --- Обработчики модалки "Запрет установлен" ---
  const handleChooseStrategy = useCallback(() => {
    setInstallPrompt(null);
    if (waitResolveRef.current) { waitResolveRef.current('strategy'); waitResolveRef.current = null; }
  }, []);

  const handleSkipStrategy = useCallback(() => {
    setInstallPrompt(null);
    if (waitResolveRef.current) { waitResolveRef.current('skip'); waitResolveRef.current = null; }
  }, []);

  const run = useCallback(async () => {
    // 1. Подключение к бэкенду
    if (!await go('connect', async () => {
      for (let i = 0; i < 30 && aliveRef.current; i++) {
        const data = await api('GET', '/api/status');
        if (data) return;
        await sleep(1000);
      }
      throw new Error(tRef.current('startup.errors.backendConnect'));
    })) return;
    setProgress(STEP_PROGRESS.connect);

    // Тема
    const cfg = await api('GET', '/api/config').catch(() => null);
    if (cfg?.theme === 'dark' || cfg?.theme === 'light') {
      document.documentElement.setAttribute('data-theme', cfg.theme);
    }

    // 2. Проверка целостности
    if (!await go('health', async () => {
      const health = await api('GET', '/api/health');
      if (health?.warnings?.length > 0) {
        const critical = health.warnings.filter(w =>
          w.includes('не найден') || w.includes('missing') || w.includes('exe')
        );
        if (critical.length > 0) throw new Error(critical.join('\n'));
      }
    })) return;
    setProgress(STEP_PROGRESS.health);

    // 3-7. Запрет (пропускаем если zapret_skipped=true)
    const zapretSkipped = cfg?.zapret_skipped === true;
    let userSkipped = false;

    if (!zapretSkipped) {
      // 3. Решение по службе + локальной копии
      const hasService = await api('GET', '/api/zapret/system-service');
      const hasLocal = await api('GET', '/api/zapret/local');

      if (hasService === true || hasService?.result === true) {
        if (!(hasLocal === true || hasLocal?.result === true)) {
          setCurrentStep('decide-service');
          setStepDone(false);
          setRemoveServicePrompt(true);
          const removed = await waitForAction();
          if (!aliveRef.current) return;
          if (!removed) {
            await api('POST', '/api/config', { zapret_skipped: true }).catch(() => null);
            userSkipped = true;
          }
          setStepDone(true);
        }
      }
      setProgress(STEP_PROGRESS['decide-service']);

      if (!userSkipped) {
        // 4. Скачивание/установка локальной копии
        const hasLocal2 = await api('GET', '/api/zapret/local');
        if (!(hasLocal2 === true || hasLocal2?.result === true)) {
          if (!await go('install', async () => {
            setDownloadProgress(null);
            const rt = window.runtime;
            let cleanup = null;
            if (rt) {
              const handler = (data) => setDownloadProgress(data);
              rt.EventsOn('download:progress', handler);
              cleanup = () => { try { rt.EventsOff('download:progress'); } catch {} };
              eventsCleanupRef.current = cleanup;
            }
            const r = await api('POST', '/api/zapret/auto-install');
            if (cleanup) cleanup();
            eventsCleanupRef.current = null;
            if (r?.error) throw new Error(r.error);
            if (r?.start_error) throw new Error(tRef.current('startup.errors.zapretFailed', { error: r.start_error }));
            installedNowRef.current = true;
            setStepDone(true);
          })) return;
        }
        setProgress(STEP_PROGRESS['check-local']);

        // 5. Установка службы с логом
        const serviceInstalledNow = await api('GET', '/api/zapret/system-service');
        if (!(serviceInstalledNow === true || serviceInstalledNow?.result === true)) {
          if (!await go('install-service', async () => {
            const def = await api('GET', '/api/zapret/default-strategy').catch(() => null);
            const strategy = def?.strategy || 'general (ALT).bat';
            const r = await api('POST', '/api/zapret/install-service-logged', { strategy });
            if (r?.error) {
              const log = await api('GET', '/api/zapret/install-log');
              setErrorLog(log?.lines || null);
              throw new Error(tRef.current('startup.errors.serviceInstallFailed', { error: r.error }));
            }
            if (r?.errors?.length) {
              const log = await api('GET', '/api/zapret/install-log');
              setErrorLog(log?.lines || null);
              throw new Error(r.errors.join('\n'));
            }
            if (r && r.success === false) {
              const log = await api('GET', '/api/zapret/install-log');
              setErrorLog(log?.lines || null);
              throw new Error(tRef.current('startup.errors.serviceInstallFailedLog'));
            }
            if (r && r.running === false) {
              console.warn('[startup] service created but not running');
            }
            installedServiceRef.current = true;
          })) return;
        }
        setProgress(STEP_PROGRESS['install-service']);

        // 6. Модалка "Запрет установлен vX"
        const firstRun = !cfg?.first_run_done;
        if (firstRun || installedNowRef.current || installedServiceRef.current) {
          const status = await api('GET', '/api/status').catch(() => null);
          const version = status?.zapret?.version || '—';
          const strategy = status?.zapret?.strategy || 'general (ALT).bat';

          setCurrentStep('strategy-prompt');
          setStepDone(false);
          setInstallPrompt({ version, strategy });
          const decision = await waitForAction();
          setInstallPrompt(null);
          if (!aliveRef.current) return;
          setStepDone(true);

          if (decision === 'strategy') {
            // 7. Авто-подбор
            if (!await go('auto-strategy', async () => {
              await new Promise((resolve, reject) => {
                const es = createStream('/api/autoselect/stream');
                es.onmessage = (e) => {
                  const d = JSON.parse(e.data);
                  if (d.type === 'progress') {
                    setAutoSelect({ current: d.current, total: d.total, strategy: d.strategy, phase: d.phase });
                  } else if (d.type === 'result') {
                    setAutoSelect(prev => ({ ...prev, lastResult: d }));
                  } else if (d.type === 'done') {
                    es.close();
                    if (d.error) reject(new Error(d.error));
                    else resolve();
                  }
                };
                es.onerror = (err) => { es.close(); reject(err); };
              });
            })) return;
            setAutoSelect(null);
          }

          await api('POST', '/api/config', { first_run_done: true }).catch(() => null);
        }
        setProgress(STEP_PROGRESS['strategy-prompt']);
      }
    } else {
      setProgress(STEP_PROGRESS['strategy-prompt']);
    }

    // 8. Готово
    if (!await go('done', async () => { await sleep(400); })) return;
    await sleep(200);
    if (aliveRef.current) onComplete();
  }, [go, sleep, waitForAction, onComplete]);

  useEffect(() => {
    applyThemeAtStart();
    run();
    return () => {
      aliveRef.current = false;
      if (eventsCleanupRef.current) { eventsCleanupRef.current(); eventsCleanupRef.current = null; }
    };
  }, [run]);

  const pct = Math.round(progress);
  const label = t('startup.steps.' + currentStep);

  const handleCopyError = useCallback(() => {
    if (error) navigator.clipboard.writeText((errorTitle || t('startup.errors.title')) + '\n' + error).catch(() => {});
  }, [error, errorTitle, t]);

  return (
    <div className="startup-overlay">
      <div className="startup-card">
        <div className="startup-title">ZPUI</div>
        <div className="startup-subtitle">{t('startup.loadingSystem')}</div>

        <div className="startup-bar-wrap">
          <div className="startup-bar">
            <div className={'startup-bar-fill' + (stepDone ? '' : ' pulse')} style={{ width: pct + '%' }} />
          </div>
          <div className="startup-bar-label">{pct}%</div>
        </div>

        <div className="startup-step-line">
          {stepDone ? '✓' : '⟳'} {label}
          {downloadProgress && currentStep === 'install' && downloadProgress.downloaded > 0
            ? ` — ${t('startup.downloaded')} ${formatBytes(downloadProgress.downloaded)}` + (downloadProgress.total > 0 ? ' / ' + formatBytes(downloadProgress.total) : '')
            : ''}
          {downloadProgress && currentStep === 'install' && downloadProgress.downloaded === -1 && downloadProgress.total === -1
            ? ' — ' + t('startup.cloning')
            : ''}
          {autoSelect && currentStep === 'auto-strategy' && autoSelect.total
            ? ` — [${autoSelect.current || '?'}/${autoSelect.total}] ${autoSelect.strategy || ''}`
            : ''}
        </div>
      </div>

      {removeServicePrompt && (
        <div className="startup-modal-overlay">
          <div className="startup-modal">
            <strong>{t('startup.serviceConflict.title')}</strong>
            <p>{t('startup.serviceConflict.desc')}</p>
            <p style={{ fontSize: 11, opacity: 0.7 }}>{t('startup.serviceConflict.skipHint')}</p>
            <div className="startup-modal-actions">
              <button className="btn btn-danger btn-sm" onClick={handleRemoveService}>{t('startup.serviceConflict.remove')}</button>
              <button className="btn btn-ghost btn-sm" onClick={handleSkipService}>{t('startup.serviceConflict.workWithout')}</button>
            </div>
          </div>
        </div>
      )}

      {installPrompt && (
        <div className="startup-modal-overlay">
          <div className="startup-modal">
            <strong>{t('startup.zapretInstalled.title')}</strong>
            <p>{t('startup.zapretInstalled.version')}<b>{installPrompt.version}</b></p>
            <p>{t('startup.zapretInstalled.strategy')}<b>{installPrompt.strategy}</b></p>
            <p style={{ fontSize: 11, opacity: 0.7 }}>{t('startup.zapretInstalled.desc')}</p>
            <div className="startup-modal-actions">
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={handleChooseStrategy}>{t('startup.zapretInstalled.selectStrategy')}</button>
              <button className="btn btn-ghost btn-sm" onClick={handleSkipStrategy}>{t('common.skip')}</button>
            </div>
          </div>
        </div>
      )}

      {error && (
        <div className="startup-modal-overlay">
          <div className="startup-modal startup-modal-error">
            <strong>{errorTitle}</strong>
            <p className="startup-modal-err-text">{error}</p>
            {errorLog && errorLog.length > 0 && (
              <details className="startup-modal-log">
                <summary>{t('startup.installLog')}</summary>
                <pre>{errorLog.join('\n')}</pre>
              </details>
            )}
            <div className="startup-modal-actions">
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={() => window.location.reload()}>{t('common.restart')}</button>
              <button className="btn btn-ghost btn-sm" onClick={handleCopyError}>{t('common.copy')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
