import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../api';
import { useT } from '../i18n';

export default function SetupWizard({ onComplete, onCancel }) {
  const { t } = useT();
  const wrapRef = useRef(null);

  const [step, setStep] = useState('detect'); // detect → confirm → install → success → strategy → filters → dns → proxy → done

  // Detection
  const [thirdParty, setThirdParty] = useState(null); // null | { has_third_party, third_party_detail }

  // Install progress
  const [progress, setProgress] = useState(null); // { phase, current, total, percent }

  // Strategy
  const [strategies, setStrategies] = useState([]);
  const [currentStrategy, setCurrentStrategy] = useState('');
  const [resourceResults, setResourceResults] = useState([]);
  const [strategyLoading, setStrategyLoading] = useState(false);

  // Strategy details
  const [selectedStrategy, setSelectedStrategy] = useState('');
  const [configuringStrategy, setConfiguringStrategy] = useState(false);

  // Questions
  const [wantFilters, setWantFilters] = useState(null); // null | true | false
  const [wantDNS, setWantDNS] = useState(null);
  const [wantProxy, setWantProxy] = useState(null);

  // Apply state
  const [applying, setApplying] = useState(false);
  const [applyError, setApplyError] = useState(null);

  // Step 1: Detect third-party
  const runDetect = useCallback(async () => {
    const r = await api('GET', '/api/setup/detect-thirdparty');
    if (r?.error) {
      setThirdParty({ has_third_party: false, third_party_detail: '' });
      setStep('install');
      return;
    }
    setThirdParty(r);
    if (r.has_third_party) {
      setStep('confirm');
    } else {
      setStep('install');
    }
  }, []);

  // Step 2: Remove & Install
  const handleRemove = useCallback(async () => {
    await api('POST', '/api/setup/remove-thirdparty');
    setStep('install');
  }, []);

  const handleCancel = useCallback(() => {
    if (onCancel) onCancel();
  }, [onCancel]);

  const runInstall = useCallback(async () => {
    setStep('installing');
    setProgress({ phase: 'download', current: 0, total: 0, percent: 0 });

    // Listen for progress
    const rt = window.runtime;
    let cleanup = null;
    if (rt) {
      const handler = (data) => setProgress(data);
      rt.EventsOn('setup:progress', handler);
      cleanup = () => { try { rt.EventsOff('setup:progress'); } catch {} };
    }

    const r = await api('POST', '/api/setup/install');
    if (cleanup) cleanup();
    if (r?.error) {
      setApplyError(r.error);
      return;
    }

    // Start zapret
    const startR = await api('POST', '/api/setup/start');
    if (startR?.error) {
      setApplyError(startR.error);
      return;
    }
    setProgress(null);
    setStep('success');
  }, []);

  // Step 3: Success - choose Skip or Configure
  const handleSkip = useCallback(async () => {
    await api('POST', '/api/setup/skip');
    if (onComplete) onComplete();
  }, [onComplete]);

  const handleConfigure = useCallback(async () => {
    setStep('strategy');
    setStrategyLoading(true);
    const r = await api('GET', '/api/setup/strategies');
    if (r?.error) {
      setApplyError(r.error);
      return;
    }
    setStrategies(r.strategies || []);
    setCurrentStrategy(r.current || '');
    setResourceResults(r.resources || []);
    setSelectedStrategy(r.current || '');
    setStrategyLoading(false);
  }, []);

  // Step 4: Strategy selection
  const handleStrategySelect = useCallback(async (name) => {
    setConfiguringStrategy(true);
    setSelectedStrategy(name);
    setResourceResults([]);
    const r = await api('GET', '/api/setup/strategies', { strategy: name });
    if (r?.error) { setApplyError(r.error); setConfiguringStrategy(false); return; }
    setResourceResults(r.resources || []);
    setConfiguringStrategy(false);
  }, []);

  const handleStrategyConfirm = useCallback(async () => {
    const r = await api('POST', '/api/setup/apply-strategy', { strategy: selectedStrategy });
    if (r?.error) { setApplyError(r.error); return; }
    setStep('filters');
  }, [selectedStrategy]);

  // Step 5-7: Questions
  const handleFiltersAnswer = useCallback(async (answer) => {
    setWantFilters(answer);
    setStep('dns');
  }, []);

  const handleDNSAnswer = useCallback(async (answer) => {
    setWantDNS(answer);
    setStep('proxy');
  }, [wantDNS]);

  const handleProxyAnswer = useCallback(async (answer) => {
    setWantProxy(answer);
    setStep('applying');
    // Apply all settings
    setApplying(true);

    // Apply filters
    if (answer === false || answer === true) {
      // We already know wantFilters, wantDNS
    }
    await api('POST', '/api/setup/configure-filters', { mode: wantFilters ? 'all' : 'disabled' });
    await api('POST', '/api/setup/configure-dns', { enable: wantDNS, primary: '77.88.8.8', secondary: '77.88.8.1' });
    await api('POST', '/api/setup/configure-proxy', { enable: wantProxy, port: 1080, bind_host: '127.0.0.1' });
    await api('POST', '/api/setup/complete');

    setApplying(false);
    if (onComplete) onComplete();
  }, [wantFilters, wantDNS, wantProxy, onComplete]);

  useEffect(() => { runDetect(); }, [runDetect]);

  const blockedResources = resourceResults.filter(r => r.blocked && !r.bypassed);
  const okResources = resourceResults.filter(r => r.ok);

  return (
    <div className="startup-overlay">
      <div className="startup-card" style={{ maxWidth: 520, textAlign: 'left', padding: 28 }}>
        {/* Step: Detect */}
        {step === 'detect' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8 }}>ZPUI</div>
            <p>{t('startup.checking') || 'Проверка системы...'}</p>
          </div>
        )}

        {/* Step: Confirm third-party removal */}
        {step === 'confirm' && thirdParty?.has_third_party && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8, color: '#e06c75' }}>⚠ {t('startup.serviceConflict.title')}</div>
            <p style={{ fontSize: 13, opacity: 0.9, marginBottom: 12 }}>
              {t('startup.serviceConflict.desc')}
            </p>
            {thirdParty.third_party_detail && (
              <p style={{ fontSize: 12, opacity: 0.7, marginBottom: 12 }}>{thirdParty.third_party_detail}</p>
            )}
            <p style={{ fontSize: 12, opacity: 0.8, background: 'rgba(255,255,255,0.05)', padding: 10, borderRadius: 6, marginBottom: 16 }}>
              {t('startup.serviceConflict.recommend') || 'Рекомендуется удалить сторонний Zapret и установить версию ZPUI.'}
            </p>
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-danger btn-sm" style={{ flex: 1 }} onClick={handleRemove}>
                {t('startup.serviceConflict.remove')}
              </button>
              <button className="btn btn-ghost btn-sm" onClick={handleCancel}>
                {t('common.cancel')}
              </button>
            </div>
          </div>
        )}

        {/* Step: Installing */}
        {step === 'installing' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8 }}>ZPUI</div>
            <div className="startup-bar-wrap" style={{ margin: '16px 0' }}>
              <div className="startup-bar">
                <div className="startup-bar-fill pulse" style={{ width: (progress?.percent || 0) + '%' }} />
              </div>
              <div className="startup-bar-label">{progress?.percent || 0}%</div>
            </div>
            <p style={{ fontSize: 12, opacity: 0.7 }}>
              {progress?.phase === 'download'
                ? t('startup.downloading') || 'Скачивание Zapret...'
                : progress?.phase === 'start'
                  ? t('startup.starting') || 'Запуск Zapret...'
                  : t('startup.installing') || 'Установка...'}
            </p>
          </div>
        )}

        {/* Step: Success */}
        {step === 'success' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8, color: '#98c379' }}>✓ {t('startup.zapretInstalled.title')}</div>
            <p style={{ fontSize: 13, opacity: 0.9, marginBottom: 12 }}>
              {t('startup.zapretInstalled.desc') || 'Zapret установлен и запущен. Система готова к работе.'}
            </p>
            <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={handleConfigure}>
                {t('startup.zapretInstalled.selectStrategy') || 'Настроить'}
              </button>
              <button className="btn btn-ghost btn-sm" onClick={handleSkip}>
                {t('common.skip')}
              </button>
            </div>
          </div>
        )}

        {/* Step: Strategy selection */}
        {step === 'strategy' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8 }}>
              {t('setup.strategy.title') || 'Выбор стратегии обхода'}
            </div>
            <p style={{ fontSize: 12, opacity: 0.8, marginBottom: 16 }}>
              {t('setup.strategy.desc') || 'Стратегия определяет, как именно будет обходиться блокировка. Выберите подходящую:'}
            </p>

            {strategyLoading ? (
              <p style={{ fontSize: 12, opacity: 0.6 }}>{t('common.loading')}</p>
            ) : (
              <div style={{ marginBottom: 16 }}>
                {strategies.map(name => (
                  <div key={name}
                    onClick={() => handleStrategySelect(name)}
                    style={{
                      padding: '8px 12px', marginBottom: 4, borderRadius: 6, cursor: 'pointer', fontSize: 13,
                      background: selectedStrategy === name ? 'rgba(86, 156, 214, 0.2)' : 'rgba(255,255,255,0.04)',
                      border: selectedStrategy === name ? '1px solid rgba(86, 156, 214, 0.5)' : '1px solid transparent',
                    }}
                  >
                    {name}
                    {name === currentStrategy && (
                      <span style={{ fontSize: 10, opacity: 0.6, marginLeft: 6 }}>
                        ({t('setup.strategy.current') || 'текущая'})
                      </span>
                    )}
                  </div>
                ))}
              </div>
            )}

            {/* Resource availability */}
            {configuringStrategy && (
              <p style={{ fontSize: 12, opacity: 0.6 }}>{t('common.loading')}...</p>
            )}
            {!configuringStrategy && resourceResults.length > 0 && (
              <div style={{ marginBottom: 16, padding: 12, background: 'rgba(255,255,255,0.03)', borderRadius: 6 }}>
                <p style={{ fontSize: 11, opacity: 0.7, marginBottom: 8 }}>
                  {t('setup.strategy.results') || 'Доступность ресурсов:'}
                </p>
                <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                  {okResources.map(r => (
                    <span key={r.name} style={{ fontSize: 11, padding: '2px 6px', borderRadius: 4, background: 'rgba(152, 195, 121, 0.15)' }}>
                      ✓ {r.name}
                    </span>
                  ))}
                  {blockedResources.map(r => (
                    <span key={r.name} style={{ fontSize: 11, padding: '2px 6px', borderRadius: 4, background: 'rgba(224, 108, 117, 0.15)' }}>
                      ✗ {r.name} <span style={{ opacity: 0.6 }}>({r.verdict})</span>
                    </span>
                  ))}
                </div>
              </div>
            )}

            <div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }}
                disabled={!selectedStrategy || configuringStrategy}
                onClick={handleStrategyConfirm}
              >
                {t('common.next')}
              </button>
            </div>
          </div>
        )}

        {/* Step: Filters question */}
        {step === 'filters' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8 }}>
              {t('setup.filters.title') || 'Игровые фильтры'}
            </div>
            <p style={{ fontSize: 12, opacity: 0.8, marginBottom: 12 }}>
              {t('setup.filters.desc') || 'Фильтры ускоряют работу игр, но могут снизить скорость загрузки. Рекомендуется включить, если вы играете онлайн.'}
            </p>
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={() => handleFiltersAnswer(true)}>
                {t('common.yes')}
              </button>
              <button className="btn btn-ghost btn-sm" style={{ flex: 1 }} onClick={() => handleFiltersAnswer(false)}>
                {t('common.no')}
              </button>
            </div>
          </div>
        )}

        {/* Step: DNS question */}
        {step === 'dns' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8 }}>
              {t('setup.dns.title') || 'Xbox DNS'}
            </div>
            <p style={{ fontSize: 12, opacity: 0.8, marginBottom: 12 }}>
              {t('setup.dns.desc') || 'Использовать Яндекс DNS (77.88.8.8 / 77.88.8.1) для обхода блокировок на уровне DNS? Помогает, если провайдер подменяет DNS-ответы. Рекомендуется для Xbox и других консолей.'}
            </p>
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={() => handleDNSAnswer(true)}>
                {t('common.yes')}
              </button>
              <button className="btn btn-ghost btn-sm" style={{ flex: 1 }} onClick={() => handleDNSAnswer(false)}>
                {t('common.no')}
              </button>
            </div>
          </div>
        )}

        {/* Step: Proxy question */}
        {step === 'proxy' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8 }}>
              {t('setup.proxy.title') || 'SOCKS5 прокси'}
            </div>
            <p style={{ fontSize: 12, opacity: 0.8, marginBottom: 12 }}>
              {t('setup.proxy.desc') || 'Прокси-сервер для маршрутизации трафика через Zapret. Нужен для приложений, которые поддерживают SOCKS5 (браузеры, мессенджеры).'}
            </p>
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={() => handleProxyAnswer(true)}>
                {t('common.yes')}
              </button>
              <button className="btn btn-ghost btn-sm" style={{ flex: 1 }} onClick={() => handleProxyAnswer(false)}>
                {t('common.no')}
              </button>
            </div>
          </div>
        )}

        {/* Step: Applying */}
        {step === 'applying' && (
          <div>
            <div className="startup-title" style={{ marginBottom: 8 }}>
              {t('setup.applying') || 'Применение настроек...'}
            </div>
            {applying && <p style={{ fontSize: 12, opacity: 0.6 }}>{t('common.loading')}</p>}
          </div>
        )}

        {/* Error */}
        {applyError && (
          <div style={{ marginTop: 12, padding: 10, background: 'rgba(224, 108, 117, 0.15)', borderRadius: 6 }}>
            <p style={{ fontSize: 12, color: '#e06c75' }}>{applyError}</p>
            <button className="btn btn-ghost btn-sm" style={{ marginTop: 8 }}
              onClick={() => { setApplyError(null); if (onComplete) onComplete(); }}>
              {t('common.skip')}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
