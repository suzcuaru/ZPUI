import { useState, useRef, useEffect, useCallback } from 'react';
import { createStream, cancelAutoTest, api } from '../api';
import { useT } from '../i18n';
import Modal from './ui/Modal';
import { AlertTriangle, Ban, Trophy, Wand2, Loader2 } from 'lucide-react';

function formatEta(seconds) {
  if (seconds < 0 || !isFinite(seconds)) return '';
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, '0')}`;
}

const GREEN_THRESHOLD = 70;
const YELLOW_THRESHOLD = 30;

const pctOf = (r) => (r.resources_n > 0 ? Math.round(((r.resources_ok ?? 0) * 100) / r.resources_n) : 0);

const pctClass = (pct) => {
  if (pct >= GREEN_THRESHOLD) return ' pct-green';
  if (pct >= YELLOW_THRESHOLD) return ' pct-yellow';
  return ' pct-red';
};

function ResultRow({ r, className = '' }) {
  const pct = pctOf(r);
  return (
    <div className={'asm2-live-item' + (r.error ? ' err' : '') + className}>
      <span className="asm2-r-name">{(r.strategy || '').replace('.bat', '')}</span>
      {r.error ? (
        <span className="asm2-r-err">{r.error}</span>
      ) : (
        <>
          <span className={'asm2-r-pct' + pctClass(pct)}>{pct}%</span>
          <span className="asm2-r-n">{r.resources_ok}/{r.resources_n}</span>
        </>
      )}
    </div>
  );
}

// AutoSelectModal — автоподбор лучшей стратегии с применением.
// Общий каркас Modal (широкая контентная модалка). Live-список стратегий во
// время работы, прогресс-бар, ETA, отмена (возвращает исходную стратегию).
// Данные пишутся в БД по ходу цикла. Клик по фону модалку не закрывает
// (чтобы не потерять живой прогон случайным кликом), Esc — закрывает.
export default function AutoSelectModal({ open, onClose, showToast, autoStart = true }) {
  const { t } = useT();
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(null); // {current, total, strategy, message}
  const [results, setResults] = useState([]);     // live по ходу
  const [statusText, setStatusText] = useState('');
  const [finalState, setFinalState] = useState(null);
  const [cancelled, setCancelled] = useState(false);
  const [intro, setIntro] = useState(false);
  const [hasData, setHasData] = useState(false);
  const esRef = useRef(null);
  const startTimeRef = useRef(null);

  const reset = useCallback(() => {
    setRunning(false);
    setProgress(null);
    setResults([]);
    setStatusText('');
    setFinalState(null);
    setCancelled(false);
    setIntro(false);
    setHasData(false);
  }, []);

  const start = useCallback((mode) => {
    // Закрываем предыдущий стрим без отмены (если был).
    if (esRef.current) {
      esRef.current.close(false);
      esRef.current = null;
    }
    setRunning(true);
    setProgress(null);
    setResults([]);
    setStatusText(t('zapret.autoSelectStarting'));
    setFinalState(null);
    setCancelled(false);
    startTimeRef.current = Date.now();

    const url = mode === 'missing' ? '/api/autoselect/stream?mode=missing' : '/api/autoselect/stream';
    const es = createStream(url);
    esRef.current = es;
    let appliedStrategy = null;

    es.onmessage = (e) => {
      let d;
      try {
        d = JSON.parse(e.data);
      } catch {
        return;
      }
      if (d.type === 'done') {
        if (esRef.current) {
          esRef.current.close(false); // успешное завершение — НЕ отменяем
          esRef.current = null;
        }
        setRunning(false);
        if (d.cancelled) {
          setCancelled(true);
          setFinalState({ cancelled: true });
        } else if (d.error) {
          setFinalState({ error: d.error });
          showToast(d.error, 'error');
        } else {
          setFinalState({ success: true, strategy: appliedStrategy });
          showToast(t('zapret.autoSelectComplete'), 'success');
          setTimeout(() => fetch('/api/resource-status/refresh').catch(() => {}), 2500);
        }
        return;
      }
      if (d.type === 'progress') {
        setProgress({ ...d, _ts: Date.now() });
        if (d.message) setStatusText(d.message);
      } else if (d.type === 'result') {
        if (d.strategy) {
          setResults(prev => {
            const existing = prev.find(r => r.strategy === d.strategy);
            if (existing) {
              return prev.map(r => (r.strategy === d.strategy ? d : r));
            }
            return [...prev, d];
          });
          if (!d.error) appliedStrategy = d.strategy;
        }
      } else if (d.type === 'info') {
        if (d.message) setStatusText(d.message);
      }
    };

    es.onerror = () => {
      if (esRef.current) {
        esRef.current.close(false);
        esRef.current = null;
      }
      setRunning(false);
      setFinalState({ error: t('zapret.autoSelectFailed') });
    };
  }, [t, showToast]);

  useEffect(() => {
    let alive = true;
    if (!open) {
      // При закрытии: если стрим жив — закрываем без отмены (бэк завершится сам).
      if (esRef.current) {
        esRef.current.close(false);
        esRef.current = null;
      }
      reset();
      return;
    }
    reset();
    if (autoStart) {
      // Если по каким-то стратегиям уже есть данные — сначала выбор режима.
      api('GET', '/api/zapret/auto-test-results')
        .then((d) => {
          if (!alive) return;
          if (d?.results?.length > 0) {
            setHasData(true);
            setIntro(true);
          } else {
            start('full');
          }
        })
        .catch(() => {
          if (alive) start('full');
        });
    } else {
      setIntro(true);
    }
    // Cleanup: закрываем стрим без отмены — отмена только по кнопке.
    return () => {
      alive = false;
      if (esRef.current) {
        esRef.current.close(false);
        esRef.current = null;
      }
    };
  }, [open, autoStart, reset, start]);

  const beginManually = useCallback((mode) => {
    setIntro(false);
    start(mode);
  }, [start]);

  const cancel = useCallback(() => {
    if (esRef.current) {
      esRef.current.close(true); // cancel=true → CancelAutoTest на бэкенде
      esRef.current = null;
    }
    cancelAutoTest();
    setRunning(false);
    setCancelled(true);
    setFinalState({ cancelled: true });
  }, []);

  const current = progress?.current ?? 0;
  const total = progress?.total ?? 0;
  const pct = total > 0 ? Math.round((current / total) * 100) : 0;

  const eta = (() => {
    if (!progress || current < 1) return '';
    const elapsed = (Date.now() - startTimeRef.current) / 1000;
    const avg = elapsed / current;
    return formatEta((total - current) * avg);
  })();

  const sortedResults = [...results].sort((a, b) => {
    if (a.error && !b.error) return 1;
    if (!a.error && b.error) return -1;
    const sa = a.resources_ok ?? 0;
    const sb = b.resources_ok ?? 0;
    if (sa !== sb) return sb - sa;
    return (a.response_ms ?? 9999) - (b.response_ms ?? 9999);
  });
  const bestResult = sortedResults.find(r => !r.error);

  if (!open) return null;

  const headIcon = running
    ? <Loader2 size={18} strokeWidth={2} className="spinning" />
    : (cancelled || finalState?.error)
      ? <AlertTriangle size={18} strokeWidth={2} />
      : finalState?.success
        ? <Trophy size={18} strokeWidth={2} />
        : <Wand2 size={18} strokeWidth={2} />;
  const headTone = finalState?.success && !running ? 'success'
    : (finalState?.error || cancelled) && !running ? 'danger'
      : 'accent';

  let footer = null;
  if (running) {
    footer = (
      <button className="btn btn-danger btn-sm" onClick={cancel}>
        <Ban size={13} strokeWidth={2.2} />
        {t('common.cancel')}
      </button>
    );
  } else if (cancelled || finalState) {
    footer = (
      <button className="btn btn-accent btn-sm" onClick={onClose}>{t('common.close')}</button>
    );
  } else if (intro) {
    footer = (
      <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
    );
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('zapret.autoSelect')}
      icon={headIcon}
      tone={headTone}
      closeOnOverlay={false}
      footer={footer}
    >
      <div className="asm2-content">
        {running && total > 0 && (
          <div className="asm2-status-row">
            <span className="asm2-running-badge">
              <span className="asm2-ring" />
              {current}/{total}
            </span>
            <span className="asm2-right-group">
              {eta && <span className="asm2-eta">~{eta}</span>}
              <span className="asm2-pct-big">{pct}<span className="asm2-pct-sign">%</span></span>
            </span>
          </div>
        )}
        {running && total === 0 && (
          <div className="asm2-status-row">
            <span className="asm2-running-badge">
              <span className="asm2-ring" />
            </span>
          </div>
        )}

        {running && (
          <>
            <div className="asm2-progress-bar">
              <div className="asm2-progress-fill" style={{ width: pct + '%' }} />
            </div>
            {statusText && (
              <div className="asm2-phase-row">
                <span className="asm2-ring" />
                <span className="asm2-status">{statusText}</span>
              </div>
            )}
            {results.length > 0 && (
              <div className="asm2-live-list">
                {sortedResults.map((r, i) => (
                  <ResultRow key={r.strategy || i} r={r} />
                ))}
                {progress?.strategy && !results.find(r => r.strategy === progress.strategy) && (
                  <div className="asm2-live-item current pending">
                    <span className="asm2-r-name">{(progress.strategy || '').replace('.bat', '')}</span>
                    <span className="asm2-ring asm2-live-spin" />
                  </div>
                )}
              </div>
            )}
          </>
        )}

        {!running && cancelled && (
          <>
            <div className="asm2-result-banner error">
              <Ban size={18} strokeWidth={2.5} />
              <span>{t('zapret.autoSelectCancelled')}</span>
            </div>
            {sortedResults.length > 0 && (
              <div className="asm2-live-list">
                {sortedResults.slice(0, 5).map((r, i) => (
                  <ResultRow key={r.strategy || i} r={r} />
                ))}
              </div>
            )}
          </>
        )}

        {!running && !cancelled && finalState?.success && (
          <>
            <div className="asm2-result-banner success">
              <Trophy size={18} strokeWidth={2.5} />
              <span>{t('zapret.appliedStrategy')}: <strong>{(finalState.strategy || '').replace('.bat', '')}</strong></span>
            </div>
            {bestResult && (
              <div className="asm2-best-stats">
                <span className="asm2-stat">{pctOf(bestResult)}%</span>
                {bestResult.resources_n > 0 && <span className="asm2-stat-ms">{bestResult.resources_ok}/{bestResult.resources_n}</span>}
                {bestResult.response_ms > 0 && <span className="asm2-stat-ms">{bestResult.response_ms}{t('common.ms')}</span>}
              </div>
            )}
            {sortedResults.length > 1 && (
              <div className="asm2-live-list">
                {sortedResults.slice(0, 5).map((r, i) => (
                  <ResultRow key={r.strategy || i} r={r} className={r.strategy === finalState.strategy ? ' best' : ''} />
                ))}
              </div>
            )}
          </>
        )}

        {!running && !cancelled && finalState?.error && (
          <>
            <div className="asm2-result-banner error" role="alert">
              <AlertTriangle size={18} strokeWidth={2.5} />
              <span>{finalState.error}</span>
            </div>
            {sortedResults.length > 0 && (
              <div className="asm2-live-list">
                {sortedResults.slice(0, 5).map((r, i) => (
                  <ResultRow key={r.strategy || i} r={r} />
                ))}
              </div>
            )}
          </>
        )}

        {!running && !finalState && !intro && (
          <div className="asm2-status">{t('zapret.autoSelectStarting')}</div>
        )}

        {intro && !running && !finalState && (
          <>
            <div className="asm2-intro">{t('zapret.autoSelectIntro')}</div>
            {hasData && <div className="asm2-intro-note">{t('zapret.autoSelectIntroHasData')}</div>}
            <div className="asm2-mode-actions">
              <button className="btn btn-accent btn-sm" onClick={() => beginManually('full')}>
                {t('zapret.autoSelectModeFull')}
              </button>
              <button className="btn btn-accent btn-sm" onClick={() => beginManually('missing')}>
                {t('zapret.autoSelectModeMissing')}
              </button>
            </div>
            <div className="asm2-mode-desc">
              <span>{t('zapret.autoSelectModeFullDesc')}</span>
              <span>{t('zapret.autoSelectModeMissingDesc')}</span>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
