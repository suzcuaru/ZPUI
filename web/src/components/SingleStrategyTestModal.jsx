import { useState, useRef, useCallback, useEffect } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { useT } from '../i18n';
import { useToast } from '../context/ToastContext';
import { Check, X as XIcon, AlertTriangle, Ban, FlaskConical, Gamepad2 } from 'lucide-react';
import Spinner from './ui/Spinner';

const POLL_INTERVAL = 2000;
const POLL_TIMEOUT = 180000;

// Единая шкала игровых цветов: зелёный 80+, жёлтый 60–79, красный ниже.
const gameTone = (v) => (v >= 80 ? 'success' : v >= 60 ? 'warning' : 'danger');

// SingleStrategyTestModal — проверка одной стратегии (ПКМ на карточке).
// game=false: применяет стратегию → ждёт 4с → проверяет ресурсы → пишет в БД
// только по этой стратегии → возвращает исходную.
// game=true: та же модалка в игровом режиме — применяет стратегию с игровым
// фильтром, снимает игровой балл (Steam/Epic/Battle.net/…) и пишет его в
// игровой балл стратегии (значок геймпада обновляется сразу).
export default function SingleStrategyTestModal({ open, filename, game, onClose, onDone }) {
  const showToast = useToast();
  const { t } = useT();
  const [running, setRunning] = useState(false);
  const [phase, setPhase] = useState(''); // 'apply' | 'check' | ''
  const [result, setResult] = useState(null); // {passed, pct, ok, total, error, cancelled} | {game, score, ping, ok, total}
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

    const r = await api('POST', '/api/strategy/test-single', { filename, game: !!game });
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
      if (res?.phase === 'done' && res?.game) {
        setResult({
          game: true,
          score: res.game_score ?? 0,
          ping: res.game_ping_ms ?? 0,
          ok: res.game_ok ?? 0,
          total: res.game_total ?? 0,
          error: res.error || '',
        });
        if (onDoneRef.current) onDoneRef.current(res);
      } else if (res?.phase === 'done') {
        setResult({
          passed: !!res.passed,
          pct: res.pct ?? 0,
          ok: res.ok_count ?? 0,
          total: res.total ?? 0,
          score: res.score ?? 0,
          grade: res.grade || '',
          speed: res.speed_mbps ?? 0,
          ping: res.ping_ms ?? 0,
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
  }, [filename, game, t]);

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
        : result.game ? (result.score >= 60 ? 'success' : 'warning')
          : result.passed ? 'success' : 'warning';

  let footer = null;
  if (running) {
    footer = (
      <button className="btn btn-danger-ghost btn-sm asm2-cancel" onClick={cancelTest}>
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
      title={game ? t('zapret.singleGameTestTitle') : t('zapret.singleTestTitle')}
      onClose={running ? undefined : onClose}
      open={open}
      closable={!running}
      size="sm"
      icon={game ? <Gamepad2 size={18} strokeWidth={2} /> : <FlaskConical size={18} strokeWidth={2} />}
      tone={headTone}
      footer={footer}
    >
      <div className="sst-modal">
        <div className="sst-name mono" data-tooltip={cleanName}>{t('zapret.singleTestOf', { strategy: cleanName })}</div>

        {running && (
          <>
            <div className="sst-state running">
              <Spinner size={18} />
              <span>{phase === 'check' ? t('zapret.sstChecking') : t('zapret.sstApplying')}</span>
            </div>
            {/* Этапы — тем же степпером, что и фазы автоподбора */}
            <div className="startup-rail asm2-rail sst-rail" aria-hidden="true">
              <div className={'startup-stage ' + (phase !== 'apply' ? 'is-done' : 'is-active')}>
                <span className="startup-stage-dot">{phase !== 'apply' ? '✓' : ''}</span>
                <span className="startup-stage-label">{t('zapret.sstStepApply')}</span>
                <span className="startup-stage-line" />
              </div>
              <div className={'startup-stage ' + (phase === 'check' ? 'is-active' : '')}>
                <span className="startup-stage-dot" />
                <span className="startup-stage-label">{t('zapret.sstStepCheck')}</span>
              </div>
            </div>
          </>
        )}

        {!running && result && result.game && (
          <>
            {result.cancelled || result.error ? (
              <div className="sst-state error" role="alert">
                <AlertTriangle size={18} />
                <span>{result.error || t('common.cancelled')}</span>
              </div>
            ) : (
              <>
                {/* Результат в стиле модалки подбора: чип-геймпад, полоса балла,
                    метрики прогона и рекомендация по баллу */}
                <div className="sst-game-head">
                  <span className={'stip-grade c-' + gameTone(result.score)}><Gamepad2 size={16} strokeWidth={2.2} /></span>
                  <span className="stip-title">{t('zapret.tipGameTitle')}</span>
                  <span className={'stip-score c-' + gameTone(result.score)}>{Math.round(result.score)}</span>
                </div>
                <div className={'stip-bar b-' + gameTone(result.score)}>
                  <span style={{ width: Math.max(3, Math.min(100, Math.round(result.score))) + '%' }} />
                </div>
                <div className="stip-rows">
                  {result.ping > 0 && (
                    <div className="stip-row"><span>{t('zapret.tipGamePing')}</span><b>{result.ping}{t('common.ms')}</b></div>
                  )}
                  {result.total > 0 && (
                    <div className="stip-row"><span>{t('zapret.tipGamePlatforms')}</span><b>{result.ok}/{result.total}</b></div>
                  )}
                </div>
                <div className={'stip-hint c-' + gameTone(result.score)}>
                  {result.score >= 80 ? t('zapret.tipGameGood') : result.score >= 60 ? t('zapret.tipGameMid') : t('zapret.tipGameBad')}
                </div>
              </>
            )}
          </>
        )}

        {!running && result && !result.game && (
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
                {result.score > 0 && (
                  <span className="sst-kv mono"> · {t('zapret.sstScore', { score: Math.round(result.score), grade: result.grade })}</span>
                )}
                {result.speed > 0 && (
                  <span className="sst-kv mono"> · {result.speed.toFixed(1)} {t('monitor.mbps')}</span>
                )}
                {result.ping > 0 && (
                  <span className="sst-kv mono"> · ping {result.ping}{t('common.ms')}</span>
                )}
              </div>
            )}
          </>
        )}

        <div className="sst-note">{game ? t('zapret.singleGameNote') : t('zapret.singleTestNote')}</div>
      </div>
    </Modal>
  );
}
