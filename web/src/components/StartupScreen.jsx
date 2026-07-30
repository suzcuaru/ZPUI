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

function formatEta(seconds) {
  if (seconds < 0) return '';
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, '0')}`;
}

const STEP_PROGRESS = {
  connect: 6,
  health: 12,
  'check-service': 18,
  'decide-service': 24,
  'check-local': 30,
  install: 44,
  'install-service': 56,
  'auto-select': 78,
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

  const [downloadProgress, setDownloadProgress] = useState(null);
  const [autoSelect, setAutoSelect] = useState(null);
  const [autoSelectStartTime, setAutoSelectStartTime] = useState(null);
  const [serviceCritical, setServiceCritical] = useState(false);
  const [autoSelectSkipped, setAutoSelectSkipped] = useState(false);
  const autoSelectSkippedRef = useRef(false);

  const aliveRef = useRef(true);
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

  const handleRetryDiagnostics = useCallback(() => {
    window.location.reload();
  }, []);

  const handleContinueWithout = useCallback(async () => {
    setServiceCritical(false);
    await api('POST', '/api/config', { zapret_skipped: true }).catch(() => null);
    if (aliveRef.current) onComplete();
  }, [onComplete]);

  const handleSkipAutoSelect = useCallback(() => {
    setAutoSelectSkipped(true);
    autoSelectSkippedRef.current = true;
  }, []);

  const withTimeout = useCallback((promise, ms, errorMsg) => {
    return Promise.race([
      promise,
      new Promise((_, reject) => setTimeout(() => reject(new Error(errorMsg || 'Timeout')), ms))
    ]);
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

    // 3. Проверка службы Windows
    const zapretSkipped = cfg?.zapret_skipped === true;
    let userSkipped = false;

    if (!zapretSkipped) {
      const hasService = await api('GET', '/api/zapret/system-service');
      const hasLocal = await api('GET', '/api/zapret/local');

      if (hasService === true || hasService?.result === true) {
        if (!await go('check-service', async () => {
          setCurrentStep('check-service');
          let healthResult;
          try {
            healthResult = await withTimeout(
              api('GET', '/api/zapret/service-health'),
              50000,
              tRef.current('startup.errors.serviceCheckTimeout')
            );
          } catch (e) {
            setServiceCritical(true);
            setErrorLog([`[${new Date().toLocaleTimeString()}] ${e.message || 'Timeout'}`]);
            throw new Error(tRef.current('startup.serviceHealth.criticalDesc'));
          }
          if (healthResult?.status === 'critical') {
            if (healthResult.log) setErrorLog(healthResult.log);
            setServiceCritical(true);
            throw new Error(healthResult.message || tRef.current('startup.serviceHealth.criticalDesc'));
          }
        })) return;
      }
      setProgress(STEP_PROGRESS['check-service']);

      // 4. Решение по службе + локальной копии
      if (hasService === true || hasService?.result === true) {
        if (!(hasLocal === true || hasLocal?.result === true)) {
          setCurrentStep('decide-service');
          setStepDone(false);
          const r = await api('POST', '/api/zapret/remove-system-service');
          if (r?.error) {
            await api('POST', '/api/config', { zapret_skipped: true }).catch(() => null);
            userSkipped = true;
          }
          setStepDone(true);
        }
      }
      setProgress(STEP_PROGRESS['decide-service']);

      if (!userSkipped) {
        // 5. Скачивание/установка локальной копии
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

        // 6. Установка службы
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
            installedServiceRef.current = true;
          })) return;
        }
        setProgress(STEP_PROGRESS['install-service']);

        // 7. Автоподбор стратегии — только если нет результатов тестов в базе
        const existingResults = await api('GET', '/api/zapret/auto-test-results').catch(() => null);
        const hasResults = existingResults?.results && existingResults.results.length > 0;

        if (!hasResults) {
          if (!await go('auto-select', async () => {
            setAutoSelectStartTime(Date.now());
            setAutoSelect({ current: 0, total: 0, strategy: '', phase: 'starting' });
            autoSelectSkippedRef.current = false;

            await new Promise((resolve, reject) => {
              const es = createStream('/api/autoselect/stream');
              const finish = () => { es.close(); resolve(); };

              es.onmessage = (e) => {
                if (!aliveRef.current || autoSelectSkippedRef.current) { finish(); return; }
                const d = JSON.parse(e.data);
                if (d.type === 'done') {
                  if (d.error) {
                    reject(new Error(d.error));
                  } else {
                    finish();
                  }
                  return;
                }
                if (d.type === 'progress') {
                  setAutoSelect(prev => ({
                    ...prev,
                    current: d.current || 0,
                    total: d.total || 0,
                    strategy: d.strategy || prev.strategy,
                    phase: 'testing',
                  }));
                } else if (d.type === 'result') {
                  if (d.strategy && !d.error) {
                    setAutoSelect(prev => ({
                      ...prev,
                      strategy: d.strategy,
                    }));
                  }
                } else if (d.type === 'info') {
                  if (d.message) {
                    setAutoSelect(prev => ({ ...prev, phase: d.message }));
                  }
                }
              };
              es.onerror = () => { finish(); };
            });

            await api('POST', '/api/config', { first_run_done: true }).catch(() => null);
            const now = new Date().toISOString();
            await api('POST', '/api/config', { last_auto_select_time: now }).catch(() => null);
          })) return;
        }
        setProgress(STEP_PROGRESS['auto-select']);
      }
    } else {
      setProgress(STEP_PROGRESS['auto-select']);
    }

    // 8. Готово
    if (!await go('done', async () => { await sleep(400); })) return;
    await sleep(200);
    if (aliveRef.current) onComplete();
  }, [go, sleep, onComplete]);

  useEffect(() => {
    applyThemeAtStart();
    run();
    return () => {
      aliveRef.current = false;
      if (eventsCleanupRef.current) { eventsCleanupRef.current(); eventsCleanupRef.current = null; }
    };
  }, [run]);

  const pct = (() => {
    if (currentStep === 'auto-select' && autoSelect && autoSelect.total > 0 && autoSelect.current > 0) {
      const base = STEP_PROGRESS['install-service'];
      const target = STEP_PROGRESS['auto-select'];
      const range = target - base;
      const autoPct = Math.min(autoSelect.current / autoSelect.total, 1);
      const calc = Math.round(base + range * autoPct);
      return Math.max(calc, progress);
    }
    return Math.round(progress);
  })();
  const label = t('startup.steps.' + currentStep);

  const handleCopyError = useCallback(() => {
    if (error) navigator.clipboard.writeText((errorTitle || t('startup.errors.title')) + '\n' + error).catch(() => {});
  }, [error, errorTitle, t]);

  const autoSelectEta = (() => {
    if (!autoSelect || !autoSelectStartTime || !autoSelect.total || autoSelect.current < 1) return '';
    const elapsed = (Date.now() - autoSelectStartTime) / 1000;
    const avgPerCheck = elapsed / autoSelect.current;
    const remaining = (autoSelect.total - autoSelect.current) * avgPerCheck;
    if (autoSelect.current >= autoSelect.total) return t('startup.autoSelect.finishing');
    return '~' + formatEta(remaining);
  })();

  return (
    <div className="startup-overlay">
      <div className="startup-card">
        <div className="startup-top-area">
          <div className="startup-title">ZPUI</div>
        </div>

        <div className="startup-bar-wrap">
          <div className="startup-bar">
            <div className={'startup-bar-fill' + (stepDone ? '' : ' pulse')} style={{ width: pct + '%' }} />
          </div>
          <div className="startup-bar-label">{pct}%</div>
        </div>

        <div className="startup-bottom-area">
          <div className="startup-step-line">
            {label}
            {downloadProgress && currentStep === 'install' && downloadProgress.downloaded > 0
              ? ` — ${t('startup.downloaded')} ${formatBytes(downloadProgress.downloaded)}` + (downloadProgress.total > 0 ? ' / ' + formatBytes(downloadProgress.total) : '')
              : ''}
            {downloadProgress && currentStep === 'install' && downloadProgress.downloaded === -1 && downloadProgress.total === -1
              ? ' — ' + t('startup.cloning')
              : ''}
          </div>

          <div className={'startup-autoselect-info' + (currentStep === 'auto-select' && autoSelect && autoSelect.total > 0 ? ' visible' : '')}>
            <div className="startup-autoselect-detail">
              {currentStep === 'auto-select' && autoSelect ? `[${autoSelect.current || '?'}/${autoSelect.total}] ${autoSelect.strategy || ''}` : '\u00A0'}
            </div>
            <div className="startup-autoselect-eta">
              {currentStep === 'auto-select' && autoSelectEta ? autoSelectEta : '\u00A0'}
            </div>
            <button className={'btn btn-ghost btn-sm startup-skip-btn' + (currentStep === 'auto-select' && !autoSelectSkipped ? '' : ' hidden')} onClick={handleSkipAutoSelect}>
              {t('startup.autoSelect.skip')}
            </button>
          </div>
        </div>
      </div>

      {serviceCritical && (
        <div className="startup-modal-overlay">
          <div className="startup-modal startup-modal-error">
            <strong>{t('startup.serviceHealth.critical')}</strong>
            <p>{t('startup.serviceHealth.criticalDesc')}</p>
            {errorLog && errorLog.length > 0 && (
              <details className="startup-modal-log">
                <summary>{t('startup.installLog')}</summary>
                <pre>{errorLog.join('\n')}</pre>
              </details>
            )}
            <div className="startup-modal-actions">
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={handleRetryDiagnostics}>{t('startup.serviceHealth.retry')}</button>
              <button className="btn btn-ghost btn-sm" onClick={handleContinueWithout}>{t('startup.serviceHealth.continueWithout')}</button>
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
