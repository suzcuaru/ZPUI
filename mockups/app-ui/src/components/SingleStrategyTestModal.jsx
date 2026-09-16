import { useState, useRef, useCallback, useEffect } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { useT } from '../i18n';
import { Loader, Check, X as XIcon, AlertTriangle, Ban, FlaskConical } from 'lucide-react';

const POLL_INTERVAL = 2000;
const POLL_TIMEOUT = 120000;

// SingleStrategyTestModal — проверка одной стратегии (ПКМ на карточке).
// Бэкенд: применяет стратегию → ждёт 4с → проверяет ресурсы → пишет в БД
// только по этой стратегии → возвращает исходную.
// Фронт: запуск строго один раз, одна poll-цепочка, отмена, живые этапы.
export default function SingleStrategyTestModal({ open, filename, onClose, onDone, showToast }) {
  const { t } = useT();
  const [running, setRunning] = useState(false);
  const [phase, setPhase] = useState(''); // 'apply' | 'check' | ''
  const [result, setResult] = useState(null); // {passed, pct, ok, total, error, cancelled}
  const startedRef = useRef(false);
  const aliveRef = useRef(false);
  const pollTimerRef = useRef(null);
  const onDoneRef = useRef(onDone);
  onDoneRef.current = onDone;

  const stopPoll = useCallback(() => {
    aliveRef.current = false;
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  const start = useCallback(async () => {
    if (startedRef.current || !filename) return;
    startedRef.current = true;
    aliveRef.current = true;
    setRunning(true);
    setResult(null);
    setPhase('apply');

    const r = await api('POST', '/api/strategy/test-single', { filename });
    if (!aliveRef.current) return;
    if (r?.error) {
      setRunning(false);
      setResult({ error: r.error });
      return;
    }

    const startedAt = Date.now();
    const poll = async () => {
      if (!aliveRef.current) return;
      if (Date.now() - startedAt > POLL_TIMEOUT) {
        setRunning(false);
        setResult({ error: t('common.error'), timeout: true });
        return;
      }
      const st = await api('GET', '/api/strategy/test-single-result');
      if (!aliveRef.current) return;
      if (st?.running) {
        // Пока бэкенд работает — двигаем этап по времени (4с на применение).
        setPhase(Date.now() - startedAt > 5000 ? 'check' : 'apply');
        pollTimerRef.current = setTimeout(poll, POLL_INTERVAL);
        return;
      }
      const res = st?.result;
      setRunning(false);
      if (res?.phase === 'done') {
        setResult({
          passed: !!res.passed,
          pct: res.pct ?? 0,
          ok: res.ok_count ?? 0,
          total: res.total ?? 0,
          error: res.error || '',
        });
        if (onDoneRef.current) onDoneRef.current(res);
      } else if (res?.phase === 'error') {
        setResult({ error: res.error || t('common.error') });
      } else {
        // {running:false, result:null} сразу после старта — ещё не готово.
        pollTimerRef.current = setTimeout(poll, POLL_INTERVAL);
      }
    };
    pollTimerRef.current = setTimeout(poll, 3000);
  }, [filename, t]);

  const cancelTest = useCallback(async () => {
    stopPoll();
    await api('POST', '/api/strategy/cancel').catch(() => null);
    setRunning(false);
    setResult({ cancelled: true });
  }, [stopPoll]);

  useEffect(() => {
    if (open && filename) start();
    if (!open) {
      stopPoll();
      startedRef.current = false;
      setRunning(false);
      setResult(null);
      setPhase('');
    }
    return stopPoll;
  }, [open, filename, start, stopPoll]);

  const cleanName = (filename || '').replace('.bat', '');

  const headTone = running ? 'accent'
    : !result ? 'neutral'
      : (result.cancelled || result.error) ? 'danger'
        : result.passed ? 'success' : 'warning';

  let footer = null;
  if (running) {
    footer = (
      <button className="btn btn-sm" onClick={cancelTest}>
        <Ban size={13} strokeWidth={2.2} />
        {t('common.cancel')}
      </button>
    );
  } else if (result) {
    footer = (
      <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
    );
  }

  return (
    <Modal
      title={t('zapret.singleTestTitle')}
      onClose={running ? undefined : onClose}
      open={open}
      closable={!running}
      size="sm"
      icon={<FlaskConical size={18} strokeWidth={2} />}
      tone={headTone}
      footer={footer}
    >
      <div className="sst-modal">
        <div className="sst-name mono" data-tooltip={cleanName}>{t('zapret.singleTestOf', { strategy: cleanName })}</div>

        {running && (
          <>
            <div className="sst-state running">
              <Loader size={18} className="spinning" />
              <span>{phase === 'check' ? t('zapret.sstChecking') : t('zapret.sstApplying')}</span>
            </div>
            <div className="sst-phase-steps">
              <span className={'sst-phase-step' + (phase !== 'apply' ? ' done' : ' active')}>
                {phase === 'apply' ? <Loader size={11} className="spinning" /> : <Check size={11} />}
                {t('zapret.sstStepApply')}
              </span>
              <span className={'sst-phase-step' + (phase === 'check' ? ' active' : '')}>
                {phase === 'check' ? <Loader size={11} className="spinning" /> : phase === 'apply' ? null : <Check size={11} />}
                {t('zapret.sstStepCheck')}
              </span>
            </div>
          </>
        )}

        {!running && result && (
          <>
            {result.cancelled ? (
              <div className="sst-state error">
                <Ban size={18} />
                <span>{t('common.cancelled')}</span>
              </div>
            ) : result.error ? (
              <div className="sst-state error" role="alert">
                <AlertTriangle size={18} />
                <span>{result.error}</span>
              </div>
            ) : result.passed ? (
              <div className="sst-state passed">
                <Check size={18} />
                <span>{t('zapret.singleTestPassed', { pct: result.pct })}</span>
              </div>
            ) : (
              <div className="sst-state failed">
                <XIcon size={18} />
                <span>{t('zapret.singleTestFailed', { pct: result.pct })}</span>
              </div>
            )}
            {!result.cancelled && !result.error && (
              <div className="sst-detail">
                {t('zapret.singleTestDetail', { ok: result.ok, total: result.total })}
              </div>
            )}
          </>
        )}

        <div className="sst-note">{t('zapret.singleTestNote')}</div>
      </div>
    </Modal>
  );
}
