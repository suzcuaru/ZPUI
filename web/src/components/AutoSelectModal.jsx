import { useState, useRef, useEffect, useCallback } from 'react';
import { createStream } from '../api';
import { useT } from '../i18n';
import { Check, X, AlertTriangle, Loader } from 'lucide-react';

export default function AutoSelectModal({ open, onClose, showToast }) {
  const { t } = useT();
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(null);
  const [finalState, setFinalState] = useState(null);
  const [results, setResults] = useState([]);
  const [statusText, setStatusText] = useState('');
  const esRef = useRef(null);

  const reset = useCallback(() => {
    setRunning(false);
    setProgress(null);
    setFinalState(null);
    setResults([]);
    setStatusText('');
  }, []);

  const start = useCallback(() => {
    setRunning(true);
    setProgress(null);
    setFinalState(null);
    setResults([]);
    setStatusText(t('zapret.autoSelectStarting'));

    const es = createStream('/api/autoselect/stream');
    esRef.current = es;
    let appliedStrategy = null;

    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      if (d.type === 'done') {
        es.close(); esRef.current = null; setRunning(false);
        if (d.error) {
          setFinalState({ error: d.error });
          showToast(d.error, 'error');
        } else {
          setFinalState({ success: true, strategy: appliedStrategy });
          showToast(t('zapret.autoSelectComplete'), 'success');
        }
        return;
      }
      if (d.type === 'progress') {
        setProgress(d);
        if (d.message) setStatusText(d.message);
      } else if (d.type === 'result') {
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
        const msg = d.message || '';
        const prefix = 'Применена стратегия:';
        if (msg.startsWith(prefix)) {
          appliedStrategy = msg.replace(prefix, '').trim().replace('.bat', '');
        }
        if (msg) setStatusText(msg);
      }
    };

    es.onerror = () => {
      es.close(); esRef.current = null; setRunning(false);
      setFinalState({ error: 'Connection error' });
    };
  }, [t, showToast]);

  useEffect(() => {
    if (!open) {
      reset();
      return;
    }
    reset();
    start();
    return () => { if (esRef.current) { esRef.current.close(); esRef.current = null; } };
  }, [open, reset, start]);

  const cancel = useCallback(() => {
    if (esRef.current) { esRef.current.close(); esRef.current = null; }
    setRunning(false);
    setFinalState({ error: t('common.cancelled') });
  }, [t]);

  const pct = progress ? Math.round((progress.current / Math.max(progress.total, 1)) * 100) : 0;
  const appliedStrategy = finalState?.strategy || null;

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
            <div className="asm2-status">{statusText}</div>
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
                {bestResult.response_ms > 0 && <span className="asm2-stat-ms">{bestResult.response_ms}мс</span>}
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

        {!running && !finalState && (
          <div className="asm2-status">{t('zapret.autoSelectStarting')}</div>
        )}
      </div>
    </div>
  );
}
