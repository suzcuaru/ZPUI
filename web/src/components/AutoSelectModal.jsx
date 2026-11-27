import { useState, useRef, useEffect, useCallback } from 'react';
import { api, createStream, cancelAutoTest } from '../api';
import { useT } from '../i18n';
import { errorText } from '../errors';
import { frontlog, logError } from '../frontlog';
import useIspName from '../hooks/useIspName';
import { useToast } from '../context/ToastContext';
import Modal from './ui/Modal';
import ProgressBar from './ui/ProgressBar';
import { AlertTriangle, Ban, Trophy, Sparkles, Gamepad2, Gauge, Check, Network, ChevronDown, Filter, FlaskConical, Layers, Server, User, CircleDashed } from 'lucide-react';
import Spinner from './ui/Spinner';

function formatEta(seconds) {
  if (seconds < 0 || !isFinite(seconds)) return '';
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, '0')}`;
}

// Версия набора подбора — unix-timestamp публикации (секунды или миллисекунды).
// Человекочитаемый вид «дд.мм чч:мм»; малые целые (не timestamp) — как есть.
function formatCfgVersion(v) {
  const n = Number(v);
  if (!isFinite(n) || n <= 0) return '';
  const ms = n >= 1e12 ? n : n >= 1e8 ? n * 1000 : null;
  if (ms == null) return String(v);
  const d = new Date(ms);
  if (isNaN(d.getTime())) return String(v);
  const p = (x) => String(x).padStart(2, '0');
  return `${p(d.getDate())}.${p(d.getMonth() + 1)} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

// Пороги грейдов S/A/B/C/D и фильтра ключевых — серверный набор алгоритма
// подбора (CRM → «Подбор», GET /api/autoselect/cfg); дефолт = константы клиента.
const CFG_DEFAULTS = { grade_s: 90, grade_a: 80, grade_b: 70, grade_c: 60, key_pct: 70, acceptable: 70, version: 0 };

// sessionStorage-ключ развёрнутости списка плана (сессия WebView)
const PLAN_OPEN_KEY = 'zpui-asm2-plan-open';

const pctOf = (r) => (r.resources_n > 0 ? Math.round(((r.resources_ok ?? 0) * 100) / r.resources_n) : 0);

const KEY_FILTER_DEFAULT = 70;
// Живой снапшот порогов для вложенных компонентов (PlanRow/ScoreScale):
// обновляется вместе с cfg при загрузке набора с сервера.
const SCALE = { s: 90, a: 80, b: 70, c: 60, keyPct: KEY_FILTER_DEFAULT };

// Цвет доступности по той же шкале, что грейды: зелёный — уровень B и выше,
// жёлтый — C, красный — D.
const pctClass = (pct, cfg = CFG_DEFAULTS) => {
  if (pct >= (cfg.grade_b ?? CFG_DEFAULTS.grade_b)) return ' pct-green';
  if (pct >= (cfg.grade_c ?? CFG_DEFAULTS.grade_c)) return ' pct-yellow';
  return ' pct-red';
};

function ResultRow({ r, className = '', ri = 0 }) {
  const { t } = useT();
  const pct = pctOf(r);
  return (
    <div className={'asm2-live-item' + (r.error ? ' err' : '') + className} style={{ '--ri': ri }}>
      <span className="asm2-r-name">{(r.strategy || '').replace('.bat', '')}</span>
      {r.error ? (
        <span className="asm2-r-err">{r.error}</span>
      ) : (
        <>
          {r.grade && <span className={'asm2-grade g-' + r.grade}>{r.grade}</span>}
          {r.score > 0 && <span className="asm2-r-score mono">{Math.round(r.score)}</span>}
          {/* Игровые строки (подбор только по игре) без ресурсных данных —
              проценты и ok/total не показываем, чтобы не мельтешили нули. */}
          {(r.resources_n > 0 || r.resources_ok > 0) && (
            <>
              <span className={'asm2-r-pct' + pctClass(pct, SCALE)}>{pct}%</span>
              <span className="asm2-r-n">{r.resources_ok}/{r.resources_n}</span>
            </>
          )}
          {r.key_n > 0 && <span className="asm2-r-n asm2-r-key" title={t('zapret.scoreKeyTitle')}>★ {r.key_ok}/{r.key_n}</span>}
          {r.speed_mbps > 0 && <span className="asm2-r-n mono">{r.speed_mbps.toFixed(1)} {t('monitor.mbps')}</span>}
          {r.ping_ms > 0 && <span className="asm2-r-n mono">{r.ping_ms}{t('common.ms')}</span>}
          {(r.game_score || 0) > 0 && (
            <span className={'asm2-r-game' + (r.game_score >= 80 ? ' ok' : r.game_score >= 60 ? ' mid' : ' low')} title={t('zapret.gameScoreTitle')}>
              <Gamepad2 size={12} strokeWidth={2.2} />
              {Math.round(r.game_score)}
            </span>
          )}
        </>
      )}
    </div>
  );
}

// Фазы подбора (autoselect:event phase). Умный v5: подготовка (operator →
// server → local → analyze) → план (select) → фильтр → полный тест (final).
const PHASES = ['operator', 'server', 'local', 'analyze', 'select'];

// Шаги конвейера для степпера: заголовок и фазы, в которых шаг активен.
// Шага «Проверка» больше нет — контроль YouTube после подбора убран.
const STEPS = [
  { key: 'prep', labelKey: 'asmStepPrep', phases: ['operator', 'server', 'local', 'analyze'] },
  { key: 'plan', labelKey: 'asmStepPlan', phases: ['select'] },
  { key: 'filter', labelKey: 'asmStepFilter', phases: ['filter'] },
  { key: 'final', labelKey: 'asmStepFinal', phases: ['final'] },
];

// Подпись сценария умного подбора (визуализация «почему этот план»).
const SCENARIO_LABEL_KEY = {
  1: 'asmScenarioFull',
  2: 'asmScenarioServer',
  3: 'asmScenarioMixed',
  4: 'asmScenarioLocal',
  0: 'asmScenarioStartup',
};

// Бейдж источника кандидата.
function SourceBadge({ source, prior, samples }) {
  const { t } = useT();
  const cfg = {
    server: { icon: Server, label: t('zapret.asmSrcServer'), cls: 'srv' },
    device: { icon: User, label: t('zapret.asmSrcDevice'), cls: 'dev' },
    explore: { icon: FlaskConical, label: t('zapret.asmSrcExplore'), cls: 'exp' },
    all: { icon: CircleDashed, label: t('zapret.asmSrcAll'), cls: 'all' },
  }[source] || { icon: CircleDashed, label: '', cls: 'all' };
  const Icon = cfg.icon;
  return (
    <span className={'asm2-src s-' + cfg.cls} title={t('zapret.asmSrcTip.' + (source || 'all'), { prior: Math.round(prior || 0) })}>
      <Icon size={10} strokeWidth={2.4} />
      {cfg.label}
      {prior > 0 && <b className="mono">~{Math.round(prior)}</b>}
      {source === 'explore' && <FlaskConical size={0} />}
    </span>
  );
}

// Строка плана: имя + источник + статус по ходу конвейера.
// testing — строка проверяется прямо сейчас в полном тесте (MOTION LAYER).
function PlanRow({ r, item, keyRes, filtering, phase, testing, ri }) {
  const { t } = useT();
  const isTesting = false;
  const result = !r.pending && (r.score > 0 || r.error) ? r : null;
  const dropped = keyRes && (keyRes.ok / Math.max(1, keyRes.n)) * 100 < SCALE.keyPct;
  const passed = keyRes && !dropped;
  return (
    <div
      data-name={r.strategy}
      style={{ '--ri': ri }}
      className={
      'asm2-plan-row'
      + (filtering ? ' filtering' : '')
      + (testing ? ' testing' : '')
      + (dropped ? ' dropped' : '')
      + (result && result.score > 0 ? ' scored' : '')
      + (result && result.error ? ' err' : '')
    }>
      <span className="asm2-r-name">{(r.strategy || '').replace('.bat', '')}</span>
      {item && <SourceBadge source={item.source} prior={item.prior} samples={item.samples} />}
      <span className="asm2-plan-status">
        {filtering && <><Spinner size={10} className="asm2-live-spin" /><span className="asm2-plan-st-txt">{t('zapret.asmFiltering')}</span></>}
        {!filtering && keyRes && (
          <span className={'asm2-plan-key' + (passed ? ' ok' : ' bad')}>
            <Filter size={10} strokeWidth={2.4} />
            {passed ? t('zapret.asmFilterPassed', { ok: keyRes.ok, n: keyRes.n }) : t('zapret.asmFilterDropped', { ok: keyRes.ok, n: keyRes.n })}
          </span>
        )}
        {/* Не прошла быструю проверку — грейд D сразу (фильтр ключевых). */}
        {dropped && <span className="asm2-grade g-D">D</span>}
        {isTesting && <><Spinner size={10} className="asm2-live-spin" /><span className="asm2-plan-st-txt">{t('zapret.asmFullTesting')}</span></>}
        {result && !result.error && result.score > 0 && (
          <>
            <span className={'asm2-grade g-' + (result.grade || 'D')}>{result.grade || ''}</span>
            <span className="asm2-r-score mono">{Math.round(result.score)}</span>
          </>
        )}
        {result && result.error && <span className="asm2-r-err">{result.error}</span>}
        {!result && pendingLabel(phase, keyRes)}
      </span>
    </div>
  );
}

function pendingLabel(phase, keyRes) {
  // Метка ожидания: до фильтра — прочерк; после фильтра у финалистов — «ждёт полного теста».
  if (phase === 'final' && keyRes) return <span className="asm2-plan-wait">…</span>;
  return <span className="asm2-plan-wait">—</span>;
}

// Шкала баллов: границы грейдов S/A/B/C/D и живой маркер лучшего результата.
function ScoreScale({ best }) {
  const { t } = useT();
  const w = Math.max(0, Math.min(100, best ?? 0));
  const grades = [['S', SCALE.s], ['A', SCALE.a], ['B', SCALE.b], ['C', SCALE.c]];
  return (
    <div className="asm2-scale">
      <div className="asm2-scale-track">
        <span className="asm2-scale-fill" style={{ width: w + '%' }} />
        {grades.map(([g, at]) => (
          <span
            key={g}
            className="asm2-scale-mark"
            style={{ left: at + '%' }}
            data-tooltip={t('zapret.asmGradeTip', { g, n: at })}
            data-tooltip-pos="top"
          />
        ))}
        <span className={'asm2-scale-dot' + (best >= SCALE.c ? ' above' : '')} style={{ left: w + '%' }} />
      </div>
      <div className="asm2-scale-labels mono">
        <span>0</span>
        {grades.map(([g, at]) => (
          <span key={g} className={'asm2-scale-thr' + (best >= at ? ' hit' : '')} style={{ left: at + '%' }}>{g}</span>
        ))}
        <span>100</span>
      </div>
    </div>
  );
}

// AutoSelectModal — подбор лучшей стратегии. Два режима:
//   smart — «Умный подбор» v5: сценарий по количеству данных, двухэтапная
//           проверка (быстрый фильтр по ключевым → полный тест финалистов);
//   game  — «Игровой подбор»: прогон только по выбранным, игровые ресурсы.
// autoStart — авто-открытие на чистой базе (первый запуск): модалка сразу
//   стартует умный подбор, один раз за открытую сессию; ручное открытие
//   (autoStart=false) всегда показывает интро.
export default function AutoSelectModal({ open, onClose, autoStart = false }) {
  const showToast = useToast();
  const { t } = useT();
  const [running, setRunning] = useState(false);
  const [mode, setMode] = useState(null);       // smart | game
  const [phase, setPhase] = useState('operator');
  const [detail, setDetail] = useState('');     // имя проверяемой стратегии
  const [progress, setProgress] = useState(null); // {current, total, strategy, ...}
  const [results, setResults] = useState([]);     // результаты полного теста
  const [keyRes, setKeyRes] = useState({});       // name -> {ok, n, current}
  const [planItems, setPlanItems] = useState({}); // name -> {prior, samples, source}
  const [scenario, setScenario] = useState(null);
  const [finalStats, setFinalStats] = useState(null); // {finalists, probed}
  const [finalState, setFinalState] = useState(null);
  const [cancelled, setCancelled] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [gamePick, setGamePick] = useState(false);
  const [strategies, setStrategies] = useState([]);
  const [selected, setSelected] = useState(() => new Set());
  const [stratLoading, setStratLoading] = useState(false);
  const provider = useIspName(open);
  const [cfg, setCfgState] = useState(CFG_DEFAULTS); // серверный набор подбора
  const setCfg = useCallback((next) => {
    setCfgState(next);
    SCALE.s = next.grade_s ?? 90;
    SCALE.a = next.grade_a ?? 80;
    SCALE.b = next.grade_b ?? 70;
    SCALE.c = next.grade_c ?? 60;
    SCALE.keyPct = next.key_pct ?? KEY_FILTER_DEFAULT;
  }, []);
  const esRef = useRef(null);
  const startTimeRef = useRef(null);
  const settleTimerRef = useRef(null);
  // Автостарт (чистая база): armed — сработал ли в текущей сессии, retried —
  // сделана ли повторная попытка после already_running, autoRetry — таймер её.
  const autoStartArmedRef = useRef(false);
  const autoStartRetriedRef = useRef(false);
  const autoRetryRef = useRef(null);
  const clearAutoRetry = useCallback(() => {
    if (autoRetryRef.current) {
      clearTimeout(autoRetryRef.current);
      autoRetryRef.current = null;
    }
  }, []);

  // Версия набора подбора: в углу шапки модалки бейдж «подбор v{n}»
  // (решение владельца 10.11.2026); пороги шкалы — из того же набора.
  useEffect(() => {
    if (!open) return;
    api('GET', '/api/autoselect/cfg').then((r) => {
      if (r && !r.error) setCfg({ ...CFG_DEFAULTS, ...r });
    }).catch(() => {});
  }, [open]);

  const reset = useCallback(() => {
    clearAutoRetry();
    setRunning(false);
    setMode(null);
    setPhase('operator');
    setDetail('');
    setProgress(null);
    setResults([]);
    setKeyRes({});
    setPlanItems({});
    setScenario(null);
    setFinalStats(null);
    setFinalState(null);
    setCancelled(false);
    setCancelling(false);
    setGamePick(false);
  }, [clearAutoRetry]);

  const startStream = useCallback((runMode, stratCsv = '') => {
    if (esRef.current) {
      esRef.current.close(false);
      esRef.current = null;
    }
    setMode(runMode);
    setPhase('operator');
    setDetail('');
    setGamePick(false);
    setRunning(true);
    setProgress(null);
    setResults([]);
    setKeyRes({});
    setPlanItems({});
    setScenario(null);
    setFinalStats(null);
    setFinalState(null);
    setCancelled(false);
    startTimeRef.current = Date.now();
    frontlog('autoselect', {
      level: 'info',
      step: 'start',
      mode: runMode,
      strategies: stratCsv ? stratCsv.split(',').length : 0,
    });

    const url = runMode === 'smart'
      ? '/api/autoselect/stream?mode=smart'
      : runMode === 'full'
        ? '/api/autoselect/stream?mode=full'
        : '/api/autoselect/stream?mode=game&strategies=' + encodeURIComponent(stratCsv);
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
      if (d.mode) setMode(d.mode);
      if (d.type === 'done') {
        if (esRef.current) {
          esRef.current.close(false);
          esRef.current = null;
        }
        setRunning(false);
        if (d.cancelled) {
          setCancelled(true);
          setCancelling(false);
          setFinalState({ cancelled: true });
        } else if (d.error === 'already_running') {
          setCancelling(false);
          setFinalState(null);
          setCancelled(false);
          showToast(t('zapret.autoSelectAlreadyRunning'), 'info');
          // Автостарт попал в хвост чужого прогона (selectionRunning снимается
          // последним): одна повторная попытка через 5с — к тому времени хвост
          // (5с-такт, restore) заканчивает. Ручной запуск в окне ожидания
          // отменяет попытку (esRef-гард в колбэке).
          if (autoStartArmedRef.current && !autoStartRetriedRef.current) {
            autoStartRetriedRef.current = true;
            clearAutoRetry();
            autoRetryRef.current = setTimeout(() => {
              autoRetryRef.current = null;
              if (esRef.current) return;
              startStream('smart');
            }, 5000);
          }
        } else if (d.error) {
          logError('autoselect', d.error, 'modal');
          frontlog('autoselect', { step: 'done', error: String(d.error) });
          setFinalState({ error: errorText(t, d.error) });
          showToast(errorText(t, d.error), 'error');
        } else {
          frontlog('autoselect', { level: 'info', step: 'done', reason: String(d.reason || '') });
          setFinalStats({ finalists: d.finalists, probed: d.probed, scenario: d.scenario });
          setFinalState({ success: true, strategy: d.strategy || appliedStrategy, applied: d.applied !== false });
          showToast(t('zapret.autoSelectComplete'), 'success');
        }
        return;
      }
      if (d.type === 'plan') {
        // План целиком: строки + бейджи источников + сценарий.
        if (Array.isArray(d.names) && d.names.length > 0) {
          setResults(d.names.map(n => ({ strategy: n, pending: true })));
        }
        if (Array.isArray(d.items)) {
          const map = {};
          for (const it of d.items) map[it.name] = it;
          setPlanItems(map);
        }
        if (d.scenario !== undefined) setScenario(d.scenario);
      } else if (d.type === 'keyprogress') {
        // Этап 1: текущая стратегия фильтра.
        setKeyRes(prev => ({ ...prev, [d.strategy]: { current: d.current, total: d.total } }));
        setPhase('filter');
        setDetail(d.strategy ? d.strategy.replace(/\.bat$/i, '') : '');
      } else if (d.type === 'key') {
        // Этап 1: итог фильтра стратегии.
        setKeyRes(prev => ({ ...prev, [d.strategy]: { ok: d.resources_ok ?? 0, n: d.resources_n ?? 0 } }));
      } else if (d.type === 'progress') {
        setProgress({ ...d, _ts: Date.now() });
        if (d.phase) setPhase(d.phase);
        setDetail(d.strategy ? d.strategy.replace(/\.bat$/i, '') : '');
      } else if (d.type === 'result') {
        if (d.strategy) {
          setResults(prev => {
            const existing = prev.find(r => r.strategy === d.strategy);
            if (existing) {
              return prev.map(r => (r.strategy === d.strategy ? { ...r, ...d, pending: false } : r));
            }
            return [...prev, { ...d, pending: false }];
          });
          if (!d.error && (d.game_score === undefined || d.phase === 'apply')) appliedStrategy = d.strategy;
        }
      } else if (d.type === 'info') {
        if (d.phase) {
          setPhase(d.phase);
          setDetail(d.message || '');
        }
      }
    };

    es.onerror = () => {
      if (esRef.current) {
        esRef.current.close(false);
        esRef.current = null;
      }
      logError('autoselect', 'stream_error', 'modal');
      setRunning(false);
      setFinalState({ error: t('zapret.autoSelectFailed') });
    };
  }, [t, showToast, clearAutoRetry]);

  const start = useCallback((runMode) => {
    if (running || cancelling) {
      showToast(t('zapret.autoSelectAlreadyRunning'), 'info');
      return;
    }
    if (runMode === 'game') {
      const csv = strategies
        .filter(s => selected.has(s.filename))
        .map(s => s.filename)
        .join(',');
      if (!csv) return;
      startStream('game', csv);
      return;
    }
    if (runMode === 'full') {
      startStream('full');
      return;
    }
    startStream('smart');
  }, [running, cancelling, strategies, selected, startStream, showToast, t]);

  useEffect(() => {
    if (!open) {
      if (esRef.current) {
        esRef.current.close(false);
        esRef.current = null;
      }
      reset();
      return;
    }
    reset();
    frontlog('autoselect', { level: 'info', step: 'open' });
    return () => {
      if (esRef.current) {
        esRef.current.close(false);
        esRef.current = null;
      }
    };
  }, [open, reset]);

  // Автостарт умного подбора: один раз за открытую сессию. Эффект отделён от
  // open-эффекта выше: start меняет идентичность вместе с running — общий
  // эффект перезапускал бы reset() посреди прогона.
  useEffect(() => {
    if (!open) {
      autoStartArmedRef.current = false;
      autoStartRetriedRef.current = false;
      return;
    }
    if (!autoStart || autoStartArmedRef.current) return;
    autoStartArmedRef.current = true;
    start('smart');
  }, [open, autoStart, start]);

  useEffect(() => () => {
    if (settleTimerRef.current) clearTimeout(settleTimerRef.current);
    clearAutoRetry();
  }, [clearAutoRetry]);

  // Проверяемая строка (фильтр/полный тест) всегда видима: список
  // плавно подскролливается к ней, если она ушла из виду.
  const planListRef = useRef(null);
  useEffect(() => {
    if (!open || !detail) return;
    const list = planListRef.current;
    if (!list) return;
    const row = list.querySelector(`[data-name="${CSS.escape(detail)}"]`);
    if (!row) return;
    const lr = list.getBoundingClientRect();
    const rr = row.getBoundingClientRect();
    if (rr.top < lr.top + 4 || rr.bottom > lr.bottom - 4) {
      list.scrollTo({
        top: row.offsetTop - list.offsetTop - list.clientHeight / 2 + row.offsetHeight / 2,
        behavior: 'smooth',
      });
    }
  }, [open, detail, phase]);

  const openGamePick = useCallback(async () => {
    if (running || cancelling) {
      showToast(t('zapret.autoSelectAlreadyRunning'), 'info');
      return;
    }
    setGamePick(true);
    setStratLoading(true);
    setStrategies([]);
    setSelected(new Set());
    const d = await api('GET', '/api/zapret/strategies').catch(() => null);
    const raw = d && !d.error && Array.isArray(d.strategies) ? d.strategies : [];
    const items = raw
      .map(s => (typeof s === 'string' ? { filename: s } : s))
      .filter(s => s && s.filename);
    // Сортировка по баллу: лучшие сверху, «нет данных» — в конце по имени.
    items.sort((a, b) => {
      const sa = a.score || 0;
      const sb = b.score || 0;
      if ((sa > 0) !== (sb > 0)) return sa > 0 ? -1 : 1;
      if (sa !== sb) return sb - sa;
      return a.filename.localeCompare(b.filename);
    });
    setStrategies(items);
    // Предвыбор: прошедшие подбор с баллом выше 70. Если данных нет вовсе —
    // выбираем все (прежнее поведение, чтобы кнопка не была мёртвой).
    const withData = items.some(s => (s.score || 0) > 0);
    setSelected(new Set(withData
      ? items.filter(s => (s.score || 0) > 70).map(s => s.filename)
      : items.map(s => s.filename)));
    setStratLoading(false);
  }, [running, cancelling, showToast, t]);

  const toggleStrategy = useCallback((name) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name); else next.add(name);
      return next;
    });
  }, []);

  const selectAllStrategies = useCallback(() => setSelected(new Set(strategies.map(s => s.filename))), [strategies]);
  const selectNoneStrategies = useCallback(() => setSelected(new Set()), []);

  const cancel = useCallback(() => {
    if (cancelling) return;
    setCancelling(true);
    clearAutoRetry();
    if (esRef.current) {
      esRef.current.close(true);
      esRef.current = null;
    }
    cancelAutoTest();
    setRunning(false);
    setCancelled(true);
    setFinalState({ cancelled: true });
    if (settleTimerRef.current) clearTimeout(settleTimerRef.current);
    settleTimerRef.current = setTimeout(() => setCancelling(false), 2000);
  }, [cancelling, clearAutoRetry]);

  const current = progress?.current ?? 0;
  const total = progress?.total ?? 0;
  const pct = total > 0 ? Math.round((current / total) * 100) : 0;

  const eta = (() => {
    if (!progress || current < 1) return '';
    const elapsed = (Date.now() - startTimeRef.current) / 1000;
    const avg = elapsed / current;
    return formatEta((total - current) * avg);
  })();

  // Список плана свёрнут по умолчанию: компактная строка-тоггл «План (N)»,
  // выбор пользователя живёт всю сессию (sessionStorage, решение 22.11.2026).
  const [listOpen, setListOpen] = useState(() => {
    try { return sessionStorage.getItem(PLAN_OPEN_KEY) === '1'; } catch { return false; }
  });
  const togglePlanList = useCallback(() => {
    setListOpen(prev => {
      const next = !prev;
      try {
        if (next) sessionStorage.setItem(PLAN_OPEN_KEY, '1');
        else sessionStorage.removeItem(PLAN_OPEN_KEY);
      } catch {}
      return next;
    });
  }, []);

  const sortedResults = [...results].sort((a, b) => {
    if (a.error && !b.error) return 1;
    if (!a.error && b.error) return -1;
    const sa = a.score ?? a.resources_ok ?? 0;
    const sb = b.score ?? b.resources_ok ?? 0;
    if (sa !== sb) return sb - sa;
    return (a.response_ms ?? 9999) - (b.response_ms ?? 9999);
  });
  const bestResult = sortedResults.find(r => !r.error && r.score > 0);

  if (!open) return null;

  // Степпер: активный шаг по текущей фазе; пройденные — галочки.
  // start/test — фазы прогона очередной стратегии в полном тесте, apply и
  // game — применение лучшей и игровой прогон финалистов: это всё шаг 4.
  const activeStep = phase === 'filter' ? 'filter'
    : phase === 'final' || phase === 'start' || phase === 'test' || phase === 'apply' || phase === 'game' ? 'final'
      : PHASES.includes(phase) ? (phase === 'select' ? 'plan' : 'prep')
        : 'prep';
  const activeIdx = STEPS.findIndex(s => s.key === activeStep);

  const scenarioLabel = mode === 'smart' && scenario !== null && SCENARIO_LABEL_KEY[scenario]
    ? t('zapret.' + SCENARIO_LABEL_KEY[scenario])
    : null;

  const filterDone = Object.values(keyRes).filter(v => v.n > 0).length;
  const bestScore = bestResult?.score ?? 0;

  const phaseKey = PHASES.includes(phase)
    ? phase
    : phase === 'game' ? 'game'
      : phase === 'apply' ? 'apply'
        : 'testing';
  const phaseLabel = t('zapret.autoSelectPhase.' + phaseKey);
  const modeLabel = mode === 'smart' ? t('zapret.autoSelectModeSmart')
    : mode === 'game' ? t('zapret.autoSelectModeGame')
      : mode === 'full' ? t('zapret.autoSelectModeFull') : null;

  const appliedOk = finalState?.success && finalState.applied !== false;
  const headIcon = running
    ? <Spinner size={18} />
    : finalState?.error
      ? <AlertTriangle size={18} strokeWidth={2} />
      : appliedOk
        ? <Trophy size={18} strokeWidth={2} />
        : <Gauge size={18} strokeWidth={2} />;
  const headTone = appliedOk && !running ? 'success'
    : finalState?.error && !running ? 'danger'
      : 'accent';

  let footer = null;
  if (running || cancelling) {
    footer = (
      <button className="btn btn-danger-ghost btn-sm asm2-cancel" onClick={cancel} disabled={cancelling}>
        <Ban size={13} strokeWidth={2.2} />
        {t('common.cancel')}
      </button>
    );
  } else if (cancelled || finalState) {
    footer = (
      <>
        {(cancelled || finalState?.error) && (
          <button className="btn btn-sm" onClick={reset}>
            {t('zapret.autoSelectAgain')}
          </button>
        )}
        <button className="btn btn-accent btn-sm" onClick={onClose}>{t('common.close')}</button>
      </>
    );
  }

  const providerRow = (
    <div className="asm2-provider">
      <Network size={11} strokeWidth={2.2} />
      <span>{t('zapret.autoSelectProvider')}:</span>
      <strong>{provider || t('zapret.autoSelectProviderDetecting')}</strong>
    </div>
  );

  const intro = !running && !cancelled && !finalState && !gamePick;

  // План-строки: план + статусы. Порядок: результаты полного теста по
  // баллу, затем фильтр-статусы, затем pending в порядке плана.
  // oi — индекс в плане: от него считается --ri входной анимации строки,
  // чтобы live-пересортировка по баллу не перезапускала её.
  const planRows = results.map((r, oi) => ({
    r,
    oi,
    item: planItems[r.strategy],
    keyRes: keyRes[r.strategy]?.n > 0 ? keyRes[r.strategy] : null,
    filtering: keyRes[r.strategy]?.total > 0 && !keyRes[r.strategy]?.n && progress?.strategy === r.strategy && phase === 'filter',
  }));
  const hasFull = (p) => !p.r.pending && ((p.r.score ?? 0) > 0 || !!p.r.error);
  // Измеренные строки полного теста — по текущему баллу: лучший сверху.
  // Ошибки без балла ниже проверенных; сортировка стабильна — при равном
  // балле сохраняется порядок плана, неизмеренные остаются внизу как были.
  const fullByScore = planRows.filter(hasFull).sort((a, b) => {
    if (!!a.r.error !== !!b.r.error) return a.r.error ? 1 : -1;
    return (b.r.score ?? 0) - (a.r.score ?? 0);
  });
  const orderedPlanRows = [
    ...fullByScore,
    ...planRows.filter(p => !hasFull(p) && p.keyRes && !p.filtering),
    ...planRows.filter(p => !hasFull(p) && (!p.keyRes || p.filtering)),
  ];
  const survivors = planRows.filter(p => p.keyRes && (p.keyRes.ok / Math.max(1, p.keyRes.n)) * 100 >= (cfg.key_pct ?? KEY_FILTER_DEFAULT)).length;

  // Общий процент хода подбора: база ~5% → перебор k/N → финал ~5%, кап 99.
  // null — счётчиков ещё нет (бар уходит в indeterminate-режим).
  const runPct = (() => {
    const n = results.length;
    const enumK = progress?.total > 0 ? Math.min(progress.current / progress.total, 1) : null;
    const enumFb = n > 0 ? Math.min(planRows.filter(hasFull).length / n, 1) : null;
    const enumR = enumK ?? enumFb;
    const filterR = n > 0 ? Math.min(filterDone / n, 1) : null;
    const ownRun = mode === 'game' || mode === 'full';
    if (phase === 'filter') return filterR != null ? Math.round(5 + 45 * filterR) : 5;
    if (phase === 'final' || phase === 'start' || phase === 'test') {
      return enumR != null ? Math.round((ownRun ? 5 : 50) + (ownRun ? 90 : 45) * enumR) : null;
    }
    if (phase === 'game') {
      // В игровом подборе game — основной перебор; в умном — хвостовой прогон финалистов.
      if (mode === 'game') return enumR != null ? Math.round(5 + 90 * enumR) : null;
      return enumR != null ? Math.round(95 + 4 * enumR) : 95;
    }
    if (phase === 'apply') return 95;
    return 5;
  })();

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('zapret.autoSelect')}
      titleExtra={cfg.version > 0 && (
        <span
          className={'asm2-cfg-chip' + (cfg.custom ? ' is-custom' : '')}
          data-tooltip={cfg.custom ? t('zapret.autoCfgCustomTip', { v: formatCfgVersion(cfg.version) }) : t('zapret.autoCfgDefaultTip')}
        >
          {t('zapret.autoCfgBadge', { v: formatCfgVersion(cfg.version) })}
        </span>
      )}
      icon={headIcon}
      tone={headTone}
      className="asm2-modal"
      closeOnOverlay={false}
      closable={!(running || cancelling)}
      footer={footer}
    >
      <div className="asm2-content">
        {running && (
          <>
            <div className="asm2-topbar">
              {modeLabel && (
                <span className="asm2-mode-chip">
                  {mode === 'smart' ? <Sparkles size={11} strokeWidth={2.2} />
                    : mode === 'game' ? <Gamepad2 size={11} strokeWidth={2.2} />
                      : <Layers size={11} strokeWidth={2.2} />}
                  {modeLabel}
                </span>
              )}
              <span className="asm2-topbar-sp" />
              {providerRow}
            </div>

            {/* Конвейер: чипы этапов с галочками (в полном прогоне этапов нет) */}
            {mode !== 'full' && (
              <div className="asm2-steps">
                {STEPS.map((s, i) => (
                  <span key={s.key} className={'asm2-step' + (i < activeIdx ? ' done' : i === activeIdx ? ' active' : '')}>
                    {i < activeIdx ? <Check size={10} strokeWidth={3} /> : i === activeIdx ? <Spinner size={9} className="asm2-step-spin" /> : <span className="asm2-step-n">{i + 1}</span>}
                    {t('zapret.' + s.labelKey)}
                  </span>
                ))}
              </div>
            )}
            {scenarioLabel && <div className="asm2-scenario">{scenarioLabel}</div>}

            {/* План: свёрнут по умолчанию — компактный тоггл, список по желанию */}
            {orderedPlanRows.length > 0 && (
              <button
                type="button"
                className="asm2-plan-toggle"
                onClick={togglePlanList}
                aria-expanded={listOpen}
              >
                <span className={'asm2-plan-toggle-chev' + (listOpen ? ' open' : '')}>
                  <ChevronDown size={12} strokeWidth={2.2} />
                </span>
                {t('zapret.asmPlanToggle', { n: orderedPlanRows.length })}
              </button>
            )}
            {orderedPlanRows.length > 0 && listOpen && (
              <div className="asm2-plan-list" ref={planListRef}>
                {orderedPlanRows.map((p) => (
                  <PlanRow
                    key={p.r.strategy}
                    r={p.r}
                    item={p.item}
                    keyRes={p.keyRes}
                    filtering={p.filtering}
                    phase={phase}
                    testing={phase === 'final' && progress?.strategy === p.r.strategy && !hasFull(p)}
                    ri={p.oi}
                  />
                ))}
              </div>
            )}

            {/* Шкала баллов: границы грейдов и живой лучший результат */}
            {mode !== 'game' && (bestScore > 0 || filterDone > 0) && (
              <ScoreScale best={bestScore} />
            )}

            {/* Общий ход подбора: бар дизайн-системы + проценты рядом */}
            <div className="asm2-progress-row">
              <ProgressBar value={runPct} size="sm" className="asm2-run-bar" />
              {runPct != null && <span className="asm2-run-pct mono">{runPct}%</span>}
            </div>

            {/* Текущая стратегия + счётчики + ETA — одна строка без карточки */}
            <div className="asm2-now">
              {detail ? <span className="asm2-now-name mono">{detail}</span> : <span className="asm2-now-name">{phaseLabel}</span>}
              <span className="asm2-now-sp" />
              {phase === 'filter' && results.length > 0 && (
                <span className="asm2-now-count">{t('zapret.asmCountFilter', { done: filterDone, total: results.length, survivors })}</span>
              )}
              {phase !== 'filter' && total > 0 && (
                <span className="asm2-now-count">{current}/{total}</span>
              )}
              {eta && <span className="asm2-eta">~{eta}</span>}
            </div>
          </>
        )}

        {!running && cancelled && (
          <>
            <div className="asm2-result-banner neutral">
              <span className="asm2-rb-icon"><Ban size={16} strokeWidth={2.2} /></span>
              <span className="asm2-rb-text">{t('zapret.autoSelectStopped')}</span>
            </div>
            {sortedResults.length > 0 && (
              <div className="asm2-live-list">
                {sortedResults.map((r, i) => (
                  <ResultRow key={r.strategy || i} r={r} ri={i} />
                ))}
              </div>
            )}
          </>
        )}

        {!running && !cancelled && finalState?.success && (
          <>
            {finalState.applied === false ? (
              <div className="asm2-result-banner neutral">
                <span className="asm2-rb-icon"><Gauge size={16} strokeWidth={2.2} /></span>
                <span className="asm2-rb-text">
                  {t('zapret.autoSelectKept')}: <strong>{(finalState.strategy || '').replace('.bat', '')}</strong>
                </span>
              </div>
            ) : (
              <div className="asm2-result-banner success">
                <span className="asm2-rb-icon"><Trophy size={16} strokeWidth={2.2} /></span>
                <span className="asm2-rb-text">
                  {t('zapret.appliedStrategy')}: <strong>{(finalState.strategy || '').replace('.bat', '')}</strong>
                </span>
              </div>
            )}
            {/* Как отработал алгоритм: фильтр и полный тест в цифрах */}
            {mode === 'smart' && finalStats && (
              <div className="asm2-algo-line">
                <Filter size={11} strokeWidth={2.2} />
                {t('zapret.asmAlgoStats', {
                  probed: finalStats.probed ?? 0,
                  finalists: finalStats.finalists ?? 0,
                  threshold: SCALE.keyPct,
                })}
              </div>
            )}
            {mode !== 'game' && bestResult && (() => {
              const s = bestResult;
              return (
                <div className="asm2-best-stats">
                  {s.score > 0 && (
                    <span className="asm2-stat asm2-stat-score">
                      <span className={'asm2-grade g-' + (s.grade || 'D')}>{s.grade || ''}</span>
                      {Math.round(s.score)}
                    </span>
                  )}
                  {s.availability != null
                    ? <span className="asm2-stat">{Math.round(s.availability)}%</span>
                    : <span className="asm2-stat">{pctOf(s)}%</span>}
                  {(s.key_n > 0) && <span className="asm2-stat-ms" title={t('zapret.scoreKeyTitle')}>★ {s.key_ok}/{s.key_n}</span>}
                  {(s.resources_n > 0) && <span className="asm2-stat-ms">{s.resources_ok}/{s.resources_n}</span>}
                  {s.speed_mbps > 0 && <span className="asm2-stat-ms">{s.speed_mbps.toFixed(1)} {t('monitor.mbps')}</span>}
                  {s.ping_ms > 0 && <span className="asm2-stat-ms">{s.ping_ms}{t('common.ms')}</span>}
                  {s.game_score > 0 && (
                    <span className={'asm2-stat-ms asm2-r-game' + (s.game_score >= 60 ? ' ok' : '')}>
                      <Gamepad2 size={12} strokeWidth={2.2} /> {Math.round(s.game_score)}
                    </span>
                  )}
                </div>
              );
            })()}
            {sortedResults.length > 1 && (
              <div className="asm2-live-list">
                {sortedResults.map((r, i) => (
                  <ResultRow key={r.strategy || i} r={r} className={r.strategy === finalState.strategy ? ' best' : ''} ri={i} />
                ))}
              </div>
            )}
          </>
        )}

        {!running && !cancelled && finalState?.error && (
          <>
            <div className="asm2-result-banner error" role="alert">
              <span className="asm2-rb-icon"><AlertTriangle size={16} strokeWidth={2.2} /></span>
              <span className="asm2-rb-text">{finalState.error}</span>
            </div>
            {sortedResults.length > 0 && (
              <div className="asm2-live-list">
                {sortedResults.map((r, i) => (
                  <ResultRow key={r.strategy || i} r={r} ri={i} />
                ))}
              </div>
            )}
          </>
        )}

        {gamePick && !running && !finalState && (
          <div className="asm2-gamepick">
            <div className="asm2-gamepick-head">
              <span className="asm2-gamepick-title">
                <Gamepad2 size={14} strokeWidth={2.2} />
                {t('zapret.autoSelectGameTitle')}
              </span>
              <span className="asm2-gamepick-count">
                {t('zapret.autoSelectGameSelected', { n: selected.size, total: strategies.length })}
              </span>
              <span className="asm2-gamepick-links">
                <button type="button" className="asm2-linkbtn" onClick={selectAllStrategies} disabled={stratLoading}>
                  {t('zapret.autoSelectGameAll')}
                </button>
                <button type="button" className="asm2-linkbtn" onClick={selectNoneStrategies} disabled={stratLoading}>
                  {t('zapret.autoSelectGameNone')}
                </button>
              </span>
            </div>
            <div className="asm2-gamepick-hint">{t('zapret.asmGamePickHint')}</div>
            <div className="asm2-strat-list" role="group" aria-label={t('zapret.autoSelectGameTitle')}>
              {stratLoading && (
                <div className="asm2-strat-empty"><Spinner size={12} /></div>
              )}
              {!stratLoading && strategies.length === 0 && (
                <div className="asm2-strat-empty">{t('zapret.autoSelectGameEmpty')}</div>
              )}
              {strategies.map((s, i) => {
                const on = selected.has(s.filename);
                const score = s.score || 0;
                // Фолбэк по баллу — теми же порогами, что GradeOf (серверный набор).
                const grade = s.grade || (score > 0
                  ? (score >= SCALE.s ? 'S' : score >= SCALE.a ? 'A' : score >= SCALE.b ? 'B' : score >= SCALE.c ? 'C' : 'D')
                  : '');
                return (
                  <label key={s.filename} className={'asm2-strat-item' + (on ? ' on' : '')} style={{ '--ri': Math.min(i, 10) }}>
                    <input
                      type="checkbox"
                      className="asm2-strat-input"
                      checked={on}
                      onChange={() => toggleStrategy(s.filename)}
                    />
                    <span className="asm2-check" aria-hidden="true">
                      <Check size={11} strokeWidth={3} />
                    </span>
                    <span className="asm2-strat-name">{s.filename.replace(/\.bat$/i, '')}</span>
                    <span className="asm2-strat-meta">
                      {grade && <span className={'strat-card-tier t-' + grade}>{grade}</span>}
                      {score > 0
                        ? <span className={'asm2-strat-score ' + (score > 70 ? 'good' : score >= 50 ? 'mid' : 'low')}>{t('zapret.asmScore', { n: Math.round(score) })}</span>
                        : <span className="asm2-strat-score">{t('zapret.asmNoData')}</span>}
                    </span>
                  </label>
                );
              })}
            </div>
            {providerRow}
            <div className="asm2-strat-actions">
              <button type="button" className="btn btn-sm" onClick={() => setGamePick(false)}>
                {t('zapret.autoSelectGameBack')}
              </button>
              <button
                type="button"
                className="btn btn-accent btn-sm"
                disabled={stratLoading || selected.size === 0}
                onClick={() => start('game')}
              >
                <Gamepad2 size={13} strokeWidth={2.2} />
                {t('zapret.autoSelectGameRun')}
              </button>
            </div>
          </div>
        )}

        {intro && (
          <>
            <div className="asm2-intro-hero">
              <span className="asm2-intro-icon"><Gauge size={22} strokeWidth={2} /></span>
              <div className="asm2-intro-title">{t('zapret.asmIntroTitle')}</div>
              <div className="asm2-intro">{t('zapret.autoSelectIntro')}</div>
            </div>
            {providerRow}
            <div className="asm2-mode-pick">
              <button
                type="button"
                className="asm2-mode-opt"
                onClick={() => start('smart')}
              >
                <span className="asm2-mode-opt-ic"><Sparkles size={16} strokeWidth={2} /></span>
                <span className="asm2-mode-opt-body">
                  <span className="asm2-mode-opt-title">
                    {t('zapret.autoSelectModeSmart')}
                    <span className="asm2-mode-opt-badge">{t('zapret.asmRecommended')}</span>
                  </span>
                  <span className="asm2-mode-opt-desc">{t('zapret.autoSelectModeSmartDesc')}</span>
                </span>
              </button>
              <button
                type="button"
                className="asm2-mode-opt"
                onClick={openGamePick}
              >
                <span className="asm2-mode-opt-ic"><Gamepad2 size={16} strokeWidth={2} /></span>
                <span className="asm2-mode-opt-body">
                  <span className="asm2-mode-opt-title">
                    {t('zapret.autoSelectModeGame')}
                  </span>
                  <span className="asm2-mode-opt-desc">{t('zapret.autoSelectModeGameDesc')}</span>
                </span>
              </button>
              <button
                type="button"
                className="asm2-mode-opt"
                onClick={() => start('full')}
              >
                <span className="asm2-mode-opt-ic"><Layers size={16} strokeWidth={2} /></span>
                <span className="asm2-mode-opt-body">
                  <span className="asm2-mode-opt-title">
                    {t('zapret.autoSelectModeFull')}
                  </span>
                  <span className="asm2-mode-opt-desc">{t('zapret.autoSelectModeFullDesc')}</span>
                </span>
              </button>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
