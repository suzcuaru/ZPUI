import { useState, useRef, useEffect } from 'react';
import Modal from './ui/Modal';
import { createStream, api } from '../api';
import { useT } from '../i18n';

export default function AutoSelectModal({ open, onClose, showToast }) {
  const { t } = useT();
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(null);
  const [finalState, setFinalState] = useState(null);
  const esRef = useRef(null);
  const bodyRef = useRef(null);

  useEffect(() => {
    if (!open) {
      reset();
      return;
    }
    reset();
    start();
    return () => { if (esRef.current) { esRef.current.close(); esRef.current = null; } };
  }, [open]);

  const reset = () => {
    setRunning(false);
    setProgress(null);
    setFinalState(null);
  };

  const start = () => {
    setRunning(true);
    setProgress(null);
    setFinalState(null);

    const es = createStream('/api/autoselect/stream');
    esRef.current = es;
    let appliedStrategy = null;

    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      if (d.type === 'done') {
        es.close(); esRef.current = null; setRunning(false);
        if (d.error) {
          setFinalState({ error: d.error });
          showToast(t('zapret.autoSelectFailed'), 'error');
        } else {
          setFinalState({ success: true, strategy: appliedStrategy });
          showToast(t('zapret.autoSelectComplete'), 'success');
        }
        return;
      }
      if (d.type === 'progress') {
        setProgress(d);
      } else if (d.type === 'info') {
        const prefix = 'Применена стратегия:';
        if (d.message && d.message.startsWith(prefix)) {
          appliedStrategy = d.message.replace(prefix, '').trim().replace('.bat', '');
        }
      }
    };

    es.onerror = () => {
      es.close(); esRef.current = null; setRunning(false);
      setFinalState({ error: 'Connection error' });
    };
  };

  const cancel = async () => {
    try { await api('POST', '/api/strategy/cancel'); } catch {}
    if (esRef.current) { esRef.current.close(); esRef.current = null; }
    setRunning(false);
    setFinalState({ error: t('common.cancel') });
  };

  const pct = progress ? Math.round((progress.current / Math.max(progress.total, 1)) * 100) : 0;
  const appliedStrategy = finalState?.strategy || null;

  return (
    <Modal open={open} onClose={() => !running && onClose()} title={t('zapret.autoSelect')} wide>
      <div className="asm-content" ref={bodyRef}>
        {progress && (
          <div className="asm-progress-section">
            <div className="asm-progress-header">
              <span className="asm-pct">{pct}%</span>
              <span className="asm-count">{progress.current} / {progress.total}</span>
            </div>
            <div className="asm-progress-track">
              <div className="asm-progress-fill" style={{ width: pct + '%' }} />
            </div>
            {progress.strategy && (
              <div className="asm-current-strat">{progress.strategy.replace('.bat', '')}</div>
            )}
          </div>
        )}

        {finalState?.success && appliedStrategy && (
          <div className="asm-success-banner">
            <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
            <span>{t('zapret.appliedStrategy')}: <strong>{appliedStrategy}</strong></span>
          </div>
        )}

        {finalState?.error && !running && (
          <div className="asm-error-banner">{finalState.error}</div>
        )}

        {!progress && !running && !finalState && (
          <div className="asm-idle">{t('zapret.starting')}</div>
        )}
      </div>

      {running && (
        <button className="btn btn-danger btn-sm asm-cancel" onClick={cancel}>
          {t('common.cancel')}
        </button>
      )}
      {!running && (
        <button className="btn btn-accent asm-close" onClick={onClose}>
          {t('common.close')}
        </button>
      )}
    </Modal>
  );
}
