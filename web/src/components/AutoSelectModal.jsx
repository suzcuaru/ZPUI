import { useState, useRef, useEffect, useCallback } from 'react';
import { createStream, api, cancelAutoTest } from '../api';
import { useT } from '../i18n';
import { Check, X, AlertTriangle, Loader } from 'lucide-react';

function formatEta(seconds) {
  if (seconds < 0 || !isFinite(seconds)) return '';
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, '0')}`;
}

export default function AutoSelectModal({ open, onClose, showToast, autoStart = true }) {
  const { t } = useT();
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(null);
  const [finalState, setFinalState] = useState(null);
  const [results, setResults] = useState([]);
  const [statusText, setStatusText] = useState('');
  const [intro, setIntro] = useState(false);
  const esRef = useRef(null);
  const startTimeRef = useRef(null);

  const reset = useCallback(() => {
    setRunning(false);
    setProgress(null);
    setFinalState(null);
    setResults([]);
    setStatusText('');
    setIntro(false);
  }, []);

  const start = useCallback(() => {
    setRunning(true);
    setProgress(null);
    setFinalState(null);
    setResults([]);
    setStatusText(t('zapret.autoSelectStarting'));
    startTimeRef.current = Date.now();

    const es = createStream('/api/autoselect/stream');
    esRef.current = es;
    let appliedStrategy = null;

    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      console.log('[ASM2] event:', d.type, d);
      if (d.type === 'done') {
        console.log('[ASM2] DONE:', d.error ? `error="${d.error}"` : 'success', 'appliedStrategy=', appliedStrategy);
        es.close(); esRef.current = null; setRunning(false);
        if (d.error) {
          setFinalState({ error: d.error });
          showToast(d.error, 'error');
        } else {
          setFinalState({ success: true, strategy: appliedStrategy });
          showToast(t('zapret.autoSelectComplete'), 'success');
          setTimeout(() => api('GET', '/api/resource-status/refresh'), 2500);
        }
        return;
      }
      if (d.type === 'progress') {
        console.log('[ASM2] progress:', d.current, '/', d.total, d.message);
        setProgress({ ...d, _ts: Date.now() });
        if (d.message) setStatusText(d.message);
      } else if (d.type === 'result') {
        console.log('[ASM2] result:', d.strategy, d.error ? `error="${d.error}"` : `ok=${d.resources_ok}/${d.resources_n}`);
        if (d.strategy) {
          setResults(prev => {
            const existing = prev.find(r => r.strategy === d.strategy);
            if (existing) {
              return prev.map(r => r.strategy === d.strategy ? d : r);
            }
            return [...prev, d];
          });
          if (!d.error) {
            appliedStrategy = d.strategy;
          }
        }
      } else if (d.type === 'info') {
        console.log('[ASM2] info:', d.message);
        const msg = d.message || '';
        const prefix = 'Применена стратегия:';
        if (msg.startsWith(prefix)) {
          appliedStrategy = msg.replace(prefix, '').trim().replace('.bat', '');
        }
        if (msg) setStatusText(msg);
      }
    };

    es.onerror = (err) => {
      console.error('[ASM2] stream error:', err);
      es.close(); esRef.current = null; setRunning(false);
      cancelAutoTest();
      setFinalState({ error: t('zapret.autoSelectFailed') });
    };
  }, [t, showToast]);

  const startRef = useRef(start);
  startRef.current = start;

  useEffect(() => {
    if (!open) {
      reset();
      return;
    }
    reset();
    // Автоподбор запускается автоматически только при явном действии
    // пользователя (кнопка в сайдбаре). При открытии модалки системным
    // событием (смена оператора, обновление zapret) показываем вступление
    // с кнопкой — без молчаливого перебора стратегий.
    if (autoStart) {
      startRef.current();
    } else {
      setIntro(true);
    }
    return () => { if (esRef.current) { esRef.current.close(); esRef.current = null; } };
  }, [open, reset, autoStart]);

  const beginManually = useCallback(() => {
    setIntro(false);
    startRef.current();
  }, []);

  const cancel = useCallback(() => {
    if (esRef.current) { esRef.current.close(); esRef.current = null; }
    cancelAutoTest();
    setRunning(false);
    setFinalState({ error: t('common.cancelled') });
  }, [t]);

  const pct = progress ? Math.round((progress.current / Math.max(progress.total, 1)) * 100) : 0;
  const appliedStrategy = finalState?.strategy || null;

  const eta = (() => {
    if (!progress || !startTimeRef.current || progress.current < 1) return '';
    const elapsed = (Date.now() - startTimeRef.current) / 1000;
    const avgPerCheck = elapsed / progress.current;
    const remaining = (progress.total - progress.current) * avgPerCheck;
    return formatEta(remaining);
  })();

  const sortedResults = [...results].sort((a, b) => {
    if (a.error && !b.error) return 1;
    if (!a.error && b.error) return -1;
    if (a.error && b.error) return 0;
    const sa = a.resources_ok ?? 0;
    const sb = b.resources_ok ?? 0;
    if (sa !== sb) return sb - sa;
    return (a.response_ms ?? 9999) - (b.response_ms ?? 9999);
  });

  const bestResult = sortedResults.find(r => !r.error);

  if (!open) return null;

  return (
    <div className="asm2-overlay">
      <div className="asm2-modal">
        <div className="asm2-header">
          <span className="asm2-title">{t('zapret.autoSelect')}</span>
          {running && (
            <span className="asm2-running-badge">
              <Loader size={12} className="spinning" />
              {progress ? `${progress.current}/${progress.total}` : '...'}
            </span>
          )}
        </div>

        {running && (
          <>
            <div className="asm2-progress-bar">
              <div className="asm2-progress-fill" style={{ width: pct + '%' }} />
            </div>
            <div className="asm2-status-row">
              <span className="asm2-status">{statusText}</span>
              {eta && <span className="asm2-eta">~{eta}</span>}
            </div>
            <button className="btn btn-danger btn-sm asm2-cancel" onClick={cancel}>
              {t('common.cancel')}
            </button>
          </>
        )}

        {!running && finalState?.success && (
          <>
            <div className="asm2-result-banner success">
              <Check size={18} strokeWidth={2.5} />
              <span>{t('zapret.appliedStrategy')}: <strong>{appliedStrategy}</strong></span>
            </div>
            {bestResult && (
              <div className="asm2-best-stats">
                <span className="asm2-stat">{bestResult.resources_ok}/{bestResult.resources_n}</span>
                {bestResult.response_ms > 0 && <span className="asm2-stat-ms">{bestResult.response_ms}{t('common.ms')}</span>}
              </div>
            )}
            {sortedResults.length > 1 && (
              <div className="asm2-results-list">
                {sortedResults.slice(0, 5).map((r, i) => (
                  <div key={i} className={'asm2-result-item' + (r.error ? ' err' : '') + (r.strategy === appliedStrategy ? ' best' : '')}>
                    <span className="asm2-r-name">{(r.strategy || '').replace('.bat', '')}</span>
                    {r.error ? (
                      <span className="asm2-r-err">{r.error}</span>
                    ) : (
                      <span className="asm2-r-ok">{r.resources_ok}/{r.resources_n}</span>
                    )}
                  </div>
                ))}
              </div>
            )}
            <button className="btn btn-accent btn-sm asm2-close" onClick={onClose}>
              {t('common.close')}
            </button>
          </>
        )}

        {!running && finalState?.error && (
          <>
            <div className="asm2-result-banner error">
              <AlertTriangle size={18} strokeWidth={2.5} />
              <span>{finalState.error}</span>
            </div>
            {sortedResults.length > 0 && (
              <div className="asm2-results-list">
                {sortedResults.slice(0, 5).map((r, i) => (
                  <div key={i} className={'asm2-result-item' + (r.error ? ' err' : '')}>
                    <span className="asm2-r-name">{(r.strategy || '').replace('.bat', '')}</span>
                    {r.error ? (
                      <span className="asm2-r-err">{r.error}</span>
                    ) : (
                      <span className="asm2-r-ok">{r.resources_ok}/{r.resources_n}</span>
                    )}
                  </div>
                ))}
              </div>
            )}
            <button className="btn btn-accent btn-sm asm2-close" onClick={onClose}>
              {t('common.close')}
            </button>
          </>
        )}

        {!running && !finalState && !intro && (
          <div className="asm2-status">{t('zapret.autoSelectStarting')}</div>
        )}

        {intro && !running && !finalState && (
          <>
            <div className="asm2-intro">{t('zapret.autoSelectIntro')}</div>
            <div className="asm2-intro-actions">
              <button className="btn btn-accent btn-sm" onClick={beginManually}>{t('zapret.autoSelectRun')}</button>
              <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
