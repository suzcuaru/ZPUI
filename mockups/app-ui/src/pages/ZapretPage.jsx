import { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { api, apiCall } from '../api';
import { useT } from '../i18n';
import { Play, Square, RefreshCw, Stethoscope, Settings, Gamepad2, Shield, Download } from 'lucide-react';
import { useConfirm } from '../components/ui/ConfirmDialog';
import { useDebouncedSave } from '../hooks/useDebouncedSave';
import SingleStrategyTestModal from '../components/SingleStrategyTestModal';
import { rangePct } from '../utils';

function sortStrategiesNatural(strategies) {
  return [...strategies].sort((a, b) => {
    const an = a.filename || a.name;
    const bn = b.filename || b.name;
    if (an === 'general.bat') return -1;
    if (bn === 'general.bat') return 1;
    const parseStrat = (s) => {
      const m = s.match(/^(.+?)\s*\(([^)]+)\)\.bat$/i);
      if (m) return { base: m[1].toLowerCase(), variant: m[2] };
      return { base: s.replace(/\.bat$/i, '').toLowerCase(), variant: '' };
    };
    const pa = parseStrat(an);
    const pb = parseStrat(bn);
    if (pa.base !== pb.base) return pa.base.localeCompare(pb.base);
    const parseVariant = (v) => {
      const m = v.match(/^([A-Za-z ]+?)(\d+)?$/);
      if (m) return { letters: m[1].trim().toUpperCase(), num: m[2] ? parseInt(m[2], 10) : 0 };
      return { letters: v.toUpperCase(), num: 0 };
    };
    const va = parseVariant(pa.variant);
    const vb = parseVariant(pb.variant);
    if (va.letters !== vb.letters) {
      if (va.letters === '') return -1;
      if (vb.letters === '') return 1;
      return va.letters.localeCompare(vb.letters);
    }
    return va.num - vb.num;
  });
}

const GREEN_THRESHOLD = 70;
const YELLOW_THRESHOLD = 30;

export default function ZapretPage({ status, showToast, onOpenDiagnostics, zapretLoading, onZapretToggle }) {
  const { t } = useT();
  const confirm = useConfirm();
  const [subtab, setSubtab] = useState('strategies');
  const [skipped, setSkipped] = useState(false);
  const [serviceBusy, setServiceBusy] = useState(false);
  const [operatorName, setOperatorName] = useState('');

  useEffect(() => {
    api('GET', '/api/config').then(c => {
      if (c) setSkipped(c.zapret_skipped === true);
    });
    api('GET', '/api/isp/current').then(d => {
      if (d?.name) setOperatorName(d.name);
    });
  }, []);

  useEffect(() => {
    if (!window.runtime?.EventsOn) return;
    const handler = (data) => {
      if (data?.name) {
        setOperatorName(data.name);
        showToast(t('zapret.operatorChanged', { name: data.name }), 'info');
      }
    };
    const strategyHandler = (data) => {
      // Во время проверки/подбора стратегий бэкенд переключает стратегии —
      // это не ручное действие, тост не нужен.
      if (data?.source === 'autotest' || data?.source === 'single_test') return;
      if (data?.strategy) {
        showToast(t('zapret.strategySwitched', { strategy: data.strategy }), 'success');
      }
    };
    window.runtime.EventsOn('operator:changed', handler);
    window.runtime.EventsOn('strategy:changed', strategyHandler);
    return () => {
      if (window.runtime?.EventsOff) {
        window.runtime.EventsOff('operator:changed');
        window.runtime.EventsOff('strategy:changed');
      }
    };
  }, [showToast, t]);

  const handleReenable = async () => {
    await api('POST', '/api/config', { zapret_skipped: false });
    setSkipped(false);
    showToast(t('zapret.reenabled'), 'success');
  };

  const handleReinstall = async () => {
    if (!await confirm({ message: t('zapret.reinstallConfirm'), variant: 'danger' })) return;
    setServiceBusy(true);
    await apiCall(() => api('POST', '/api/zapret/stop'), null, showToast);
    await apiCall(() => api('POST', '/api/zapret/service/remove'), null, showToast);
    await new Promise(r => setTimeout(r, 1000));
    await apiCall(() => api('POST', '/api/zapret/start'), t('zapret.reinstalled'), showToast);
    setServiceBusy(false);
  };

  if (skipped) {
    return (
      <div className="zapret-skipped-banner">
        <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="var(--warning)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>
        </svg>
        <div className="zapret-skipped-text">
          <strong>{t('zapret.skippedTitle')}</strong>
          <p>{t('zapret.skippedDesc')}</p>
        </div>
        <button className="btn btn-accent btn-sm" onClick={handleReenable}>{t('zapret.reenable')}</button>
      </div>
    );
  }

  const zRun = status?.zapret?.status === 'running';
  const zVersion = status?.zapret?.version || '—';
  const busy = serviceBusy || zapretLoading;

  return (
    <div className="zp-page stagger">
      <div className="zp-service-bar">
        <div className="zp-service-status">
          <span className={'zp-status-dot ' + (zRun ? 'on' : 'off')} />
          {/* До первого /api/status состояние движка неизвестно — нейтральное «Нет данных» */}
          <span className="zp-status-text">{status ? (zRun ? t('status.running') : t('status.stopped')) : t('status.noData')}</span>
          <span className="zp-version">v{zVersion}</span>
          {operatorName && (
            <span className="zp-operator">
              {t('zapret.operator')}: {operatorName}
            </span>
          )}
        </div>
        <div className="zp-service-actions">
          <button
            className={'btn btn-sm zp-toggle-btn ' + (zRun ? 'btn-danger' : 'btn-accent') + (busy ? ' busy' : '')}
            onClick={onZapretToggle}
            disabled={busy}
          >
            {busy ? <RefreshCw size={13} className="spinning" /> : zRun ? <Square size={13} /> : <Play size={13} />}
            {busy ? t('common.wait') : (zRun ? t('common.stop') : t('common.start'))}
          </button>
          <button
            className="btn btn-sm zp-btn-wide"
            onClick={handleReinstall}
            disabled={busy}
            data-tooltip={t('zapret.reinstallTip')}
            data-tooltip-pos="bottom"
          >
            <RefreshCw size={13} />
            {t('zapret.reinstall')}
          </button>
          <button
            className="btn btn-sm zp-btn-wide"
            onClick={onOpenDiagnostics}
            data-tooltip={t('zapret.diagnosticsTip')}
            data-tooltip-pos="bottom"
          >
            <Stethoscope size={13} />
            {t('zapret.diagnostics')}
          </button>
          <AutoSwitchToggle showToast={showToast} />
        </div>
      </div>

      <div className="subtabs">
        <button className={'subtab' + (subtab === 'strategies' ? ' active' : '')} onClick={() => setSubtab('strategies')}>{t('zapret.strategies')}</button>
        <button className={'subtab' + (subtab === 'lists' ? ' active' : '')} onClick={() => setSubtab('lists')}>{t('zapret.lists')}</button>
      </div>

      <div className="zp-tab-body">
        {subtab === 'strategies' && <StrategiesTab status={status} showToast={showToast} zRun={zRun} />}
        {subtab === 'lists' && <ListsTab showToast={showToast} />}
      </div>

      {subtab === 'strategies' && <FiltersPanel showToast={showToast} />}
    </div>
  );
}

function AutoSwitchToggle({ showToast }) {
  const { t } = useT();
  const [enabled, setEnabled] = useState(false);
  const [threshold, setThreshold] = useState(50);
  const [showMenu, setShowMenu] = useState(false);
  const gearRef = useRef(null);
  const saveAutoSwitchConfig = useDebouncedSave('/api/zapret/autoswitch/config', 400);

  useEffect(() => {
    api('GET', '/api/zapret/autoswitch/status').then(d => {
      if (d) {
        setEnabled(d.enabled);
        setThreshold(d.threshold || 50);
      }
    });
  }, []);

  useEffect(() => {
    if (!showMenu) return;
    const handleClick = (e) => {
      if (gearRef.current && !gearRef.current.contains(e.target)) setShowMenu(false);
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [showMenu]);

  const toggle = async () => {
    const next = !enabled;
    setEnabled(next);
    await apiCall(() => api('POST', '/api/zapret/autoswitch/toggle', { enabled: next }), next ? t('zapret.autoSwitchEnabled') : t('zapret.autoSwitchDisabled'), showToast);
  };

  const saveThreshold = (val) => {
    setThreshold(val);
    saveAutoSwitchConfig({ threshold: val });
  };

  return (
    <div className="zp-autoswitch">
      <span className="switch-wrap">
        <button
          className="switch-track"
          role="switch"
          aria-checked={enabled}
          aria-label={t('zapret.autoSwitch')}
          onClick={toggle}
        >
          <span className="switch-thumb" />
        </button>
        <span className="switch-label">{t('zapret.autoSwitch')}</span>
      </span>
      <div className="zp-autoswitch-gear" ref={gearRef}>
        <button className="zp-gear-btn" onClick={() => setShowMenu(!showMenu)}>
          <Settings size={14} />
        </button>
        {showMenu && (
          <div className="zp-autoswitch-menu">
            <div className="zp-autoswitch-menu-header">{t('zapret.autoSwitch')}</div>
            <div className="zp-autoswitch-menu-row">
              <div className="zp-autoswitch-menu-label-row">
                <span className="zp-autoswitch-menu-label">{t('zapret.autoSwitchThreshold')}</span>
                <span className="zp-autoswitch-menu-val">{threshold}%</span>
              </div>
              <input type="range" min="50" max="80" step="5" value={threshold}
                onChange={e => saveThreshold(parseInt(e.target.value))}
                className="slider-ui"
                style={{ '--val': rangePct(threshold, 50, 80) + '%' }} />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function StrategiesTab({ status, showToast, zRun }) {
  const { t } = useT();
  const [strategies, setStrategies] = useState([]);
  const [changing, setChanging] = useState(null);
  const [testResults, setTestResults] = useState({});
  const [activeSubtab, setActiveSubtab] = useState('all');
  const [resultsLoaded, setResultsLoaded] = useState(false);
  const subtabInitRef = useRef(false);
  const [ctxMenu, setCtxMenu] = useState(null); // {x, y, filename}
  const [testingFile, setTestingFile] = useState(null);
  const [singleTestFile, setSingleTestFile] = useState(null); // ПКМ-проверка: модалка
  const testingTimerRef = useRef(null);

  const loadStrategies = useCallback(async () => {
    const d = await api('GET', '/api/zapret/strategies');
    if (d) setStrategies(d.strategies || []);
  }, []);

  const loadTestResults = useCallback(async () => {
    const d = await api('GET', '/api/zapret/auto-test-results');
    if (d?.results) {
      const map = {};
      for (const r of d.results) {
        if (r.strategy) {
          // pct приходит из БД (availability_pct); fallback — ok/total.
          const pct = r.pct ?? (r.total > 0 ? Math.round((r.ok / r.total) * 100) : 0);
          map[r.strategy] = { ...r, pct };
        }
      }
      setTestResults(map);
      setResultsLoaded(true);
    }
  }, []);

  // Мгновенное обновление одного результата (соло-тест) — без перезагрузки.
  const mergeSingleResult = useCallback((res) => {
    if (!res?.strategy) return;
    const pct = res.pct ?? 0;
    setTestResults(prev => ({
      ...prev,
      [res.strategy]: { ...prev[res.strategy], strategy: res.strategy, pct, ok_count: res.ok_count, total: res.total },
    }));
  }, []);

  useEffect(() => {
    loadStrategies();
    loadTestResults();
  }, [loadStrategies, loadTestResults]);

  useEffect(() => {
    if (!window.runtime?.EventsOn) return;
    const handler = () => { loadStrategies(); };
    window.runtime.EventsOn('strategy:changed', handler);
    return () => {
      if (window.runtime?.EventsOff) window.runtime.EventsOff('strategy:changed');
    };
  }, [loadStrategies]);

  // Правый клик → SingleStrategyTestModal (свой polling + отмена).
  // Событие strategy:single_test слушает только модалка — дублирующий
  // обработчик здесь вызывал спам тостов.

  const handleCtxMenu = useCallback((e, filename) => {
    e.preventDefault();
    e.stopPropagation();
    // Контекстное меню в пределах окна, не за краями.
    const x = Math.min(e.clientX, window.innerWidth - 220);
    const y = Math.min(e.clientY, window.innerHeight - 60);
    setCtxMenu({ x, y, filename });
  }, []);

  const runSingleTest = useCallback((filename) => {
    setCtxMenu(null);
    if (!zRun) {
      showToast(t('zapret.stratLockedOff'), 'warning');
      return;
    }
    setSingleTestFile(filename); // открывает отдельную модалку проверки
  }, [zRun, showToast, t]);

  // Закрытие контекстного меню при клике ВНЕ его.
  // mousedown на самой кнопке меню НЕ закрывает — иначе click не сработает
  // (React размонтирует кнопку до события click).
  useEffect(() => {
    if (!ctxMenu) return;
    const close = (e) => {
      if (e.target.closest && e.target.closest('.strat-ctx-menu')) return;
      setCtxMenu(null);
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [ctxMenu]);

  const sortedStrategies = useMemo(() => sortStrategiesNatural(strategies), [strategies]);

  // Одноразовая инициализация: как только пришли и стратегии, и результаты
  // тестов, открываем сабтаб «Зелёные», если рабочие стратегии есть. Дальше
  // обновления данных вкладку не переключают — пользователь решает сам.
  useEffect(() => {
    if (subtabInitRef.current || sortedStrategies.length === 0 || !resultsLoaded) return;
    subtabInitRef.current = true;
    const hasGreen = sortedStrategies.some(s => {
      const tr = testResults[s.filename];
      return tr && tr.pct >= GREEN_THRESHOLD;
    });
    if (hasGreen && activeSubtab === 'all') {
      setActiveSubtab('green');
    }
  }, [testResults, sortedStrategies, resultsLoaded, activeSubtab]);

  const filteredStrategies = useMemo(() => {
    return sortedStrategies.filter(s => {
      if (activeSubtab === 'all') return true;
      const tr = testResults[s.filename];
      if (!tr) return activeSubtab === 'red';
      if (tr.pct >= GREEN_THRESHOLD) return activeSubtab === 'green';
      if (tr.pct >= YELLOW_THRESHOLD) return activeSubtab === 'yellow';
      return activeSubtab === 'red';
    });
  }, [sortedStrategies, testResults, activeSubtab]);

  const handleSet = async (fn) => {
    setChanging(fn);
    const ok = await apiCall(() => api('POST', '/api/zapret/set-strategy', { filename: fn }), t('zapret.strategyApplied'), showToast);
    await loadStrategies();
    setChanging(null);
    if (ok) {
      setTimeout(() => api('GET', '/api/resource-status/refresh'), 2500);
    }
  };

  // Стратегии заблокированы, пока запрет выключен (в т.ч. стартует/перезапускается)
  // или идёт переключение стратегии.
  const stratLocked = !zRun || changing !== null;

  const pctColor = (pct) => {
    if (pct === null || pct === undefined) return 'var(--text-tertiary)';
    if (pct >= GREEN_THRESHOLD) return 'var(--success)';
    if (pct >= YELLOW_THRESHOLD) return 'var(--warning)';
    return 'var(--danger)';
  };

  const pctBadgeClass = (pct) => {
    if (pct === null || pct === undefined) return '';
    if (pct >= GREEN_THRESHOLD) return ' pct-green';
    if (pct >= YELLOW_THRESHOLD) return ' pct-yellow';
    return ' pct-red';
  };

  return (
    <>
      <div className="zp-strat-tabs">
        <button className={'zp-strat-tab' + (activeSubtab === 'all' ? ' active' : '')} onClick={() => setActiveSubtab('all')}>
          {t('zapret.tabAll')}
          <span className="zp-strat-tab-count">{sortedStrategies.length}</span>
        </button>
        <button className={'zp-strat-tab zp-strat-tab-green' + (activeSubtab === 'green' ? ' active' : '')} onClick={() => setActiveSubtab('green')}>
          {t('zapret.tabGreen')}
          <span className="zp-strat-tab-count">{sortedStrategies.filter(s => { const tr = testResults[s.filename]; return tr && tr.pct >= GREEN_THRESHOLD; }).length}</span>
        </button>
        <button className={'zp-strat-tab zp-strat-tab-yellow' + (activeSubtab === 'yellow' ? ' active' : '')} onClick={() => setActiveSubtab('yellow')}>
          {t('zapret.tabYellow')}
          <span className="zp-strat-tab-count">{sortedStrategies.filter(s => { const tr = testResults[s.filename]; return tr && tr.pct >= YELLOW_THRESHOLD && tr.pct < GREEN_THRESHOLD; }).length}</span>
        </button>
        <button className={'zp-strat-tab zp-strat-tab-red' + (activeSubtab === 'red' ? ' active' : '')} onClick={() => setActiveSubtab('red')}>
          {t('zapret.tabRed')}
          <span className="zp-strat-tab-count">{sortedStrategies.filter(s => { const tr = testResults[s.filename]; return !tr || tr.pct < YELLOW_THRESHOLD; }).length}</span>
        </button>
      </div>

      {!zRun && <div className="strat-lock-hint">{t('zapret.stratLockedOff')}</div>}

      <div className="strat-grid">
        {filteredStrategies.map(s => {
          const tr = testResults[s.filename];
          const pct = tr ? tr.pct : null;
          const isTesting = testingFile === s.filename;
          return (
            <button
              key={s.filename}
              className={'strat-card' + (s.current ? ' active' : '') + (stratLocked && !s.current ? ' locked' : '') + (isTesting ? ' testing' : '')}
              onClick={() => !s.current && !stratLocked && !isTesting && handleSet(s.filename)}
              onContextMenu={(e) => handleCtxMenu(e, s.filename)}
              disabled={stratLocked || isTesting}
              data-tooltip={isTesting ? t('zapret.testingStrategy') : undefined}
            >
              <span className="strat-card-dot" style={pct !== null ? { background: pctColor(pct) } : undefined} />
              <span className="strat-card-name">{s.name}</span>
              {isTesting && <span className="strat-card-spin" />}
              {pct !== null && !isTesting && (
                <span className={'strat-card-pct' + pctBadgeClass(pct)}>
                  {pct}%
                </span>
              )}
              {changing === s.filename && <span className="strat-card-spin" />}
            </button>
          );
        })}
        {filteredStrategies.length === 0 && <div className="strat-empty">{t('zapret.noStrategies')}</div>}
      </div>

      {ctxMenu && (
        <div
          className="strat-ctx-menu"
          style={{ left: ctxMenu.x, top: ctxMenu.y }}
          onMouseDown={e => e.stopPropagation()}
        >
          <button
            className="strat-ctx-item"
            disabled={!zRun || !!testingFile}
            onMouseDown={e => e.stopPropagation()}
            onClick={() => runSingleTest(ctxMenu.filename)}
          >
            <RefreshCw size={12} strokeWidth={2.2} />
            {t('zapret.testSingleStrategy')}
          </button>
        </div>
      )}

      <SingleStrategyTestModal
        open={!!singleTestFile}
        filename={singleTestFile}
        onClose={() => setSingleTestFile(null)}
        onDone={(res) => { mergeSingleResult(res); loadStrategies(); }}
        showToast={showToast}
      />
    </>
  );
}

function FiltersPanel({ showToast }) {
  const { t } = useT();
  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');
  const [updatingIpset, setUpdatingIpset] = useState(false);
  const [updatingHosts, setUpdatingHosts] = useState(false);

  useEffect(() => {
    loadFilters();
  }, []);

  const loadFilters = async () => {
    const gf = await api('GET', '/api/zapret/game-filter');
    if (gf) setGameFilter(gf.mode || 'disabled');
    const ip = await api('GET', '/api/zapret/ipset-status');
    if (ip) setIpsetStatus(ip.status || 'loaded');
  };

  const handleGameFilter = async (mode) => {
    setGameFilter(mode);
    await apiCall(() => api('POST', '/api/zapret/game-filter', { mode }), null, showToast);
  };

  const handleIpsetMode = async (mode) => {
    await apiCall(() => api('POST', '/api/zapret/ipset-toggle', { mode }), null, showToast);
    loadFilters();
  };

  const handleUpdateIpset = async () => {
    setUpdatingIpset(true);
    await apiCall(() => api('POST', '/api/zapret/update-ipset'), null, showToast);
    setUpdatingIpset(false);
  };

  const handleUpdateHosts = async () => {
    setUpdatingHosts(true);
    await apiCall(() => api('POST', '/api/zapret/update-hosts'), null, showToast);
    setUpdatingHosts(false);
  };

  return (
    <div className="zp-filter-bar">
      <span className="zp-filter-bar-label" data-tooltip={t('zapret.gameFilterTip')} data-tooltip-pos="top-left">
        <Gamepad2 size={12} />
        {t('zapret.gameFilter')}
      </span>
      <div className="zp-filter-bar-pills">
        {[
          { v: 'disabled', l: t('zapret.off') },
          { v: 'all', l: t('zapret.filterAll') },
          { v: 'tcp', l: t('zapret.filterTcp') },
          { v: 'udp', l: t('zapret.filterUdp') },
        ].map(o => (
          <button key={o.v} className={'flt2-pill' + (gameFilter === o.v ? ' active' : '')} onClick={() => handleGameFilter(o.v)}>
            {o.l}
          </button>
        ))}
      </div>

      <span className="zp-filter-bar-sep" />

      <span className="zp-filter-bar-label" data-tooltip={t('zapret.ipsetTip')}>
        <Shield size={12} />
        {t('zapret.ipsetFilter')}
      </span>
      <div className="zp-filter-bar-pills">
        {[
          { v: 'loaded', l: t('zapret.ipsetModeLoaded') },
          { v: 'none', l: t('zapret.ipsetModeNone') },
          { v: 'any', l: t('zapret.ipsetModeAny') },
        ].map(o => (
          <button key={o.v} className={'flt2-pill' + (ipsetStatus === o.v ? ' active' : '')} onClick={() => handleIpsetMode(o.v)}>
            {o.l}
          </button>
        ))}
      </div>

      <span className="zp-filter-bar-sep" />

      <button className="btn btn-xs" onClick={handleUpdateIpset} disabled={updatingIpset} data-tooltip={t('zapret.updateIpsetTip')} data-tooltip-pos="top">
        {updatingIpset ? <RefreshCw size={11} className="spinning" /> : <Download size={11} />}
        {t('zapret.updateIpset')}
      </button>
      <button className="btn btn-xs" onClick={handleUpdateHosts} disabled={updatingHosts} data-tooltip={t('zapret.updateHostsTip')} data-tooltip-pos="top">
        {updatingHosts ? <RefreshCw size={11} className="spinning" /> : <Download size={11} />}
        {t('zapret.updateHosts')}
      </button>
    </div>
  );
}

function ListsTab({ showToast }) {
  const { t } = useT();
  const [lists, setLists] = useState([]);
  const [selected, setSelected] = useState(null);
  const [content, setContent] = useState('');
  const [saving, setSaving] = useState(false);
  const [skipRes, setSkipRes] = useState('');

  useEffect(() => { loadLists(); loadSkipRes(); }, []);

  const loadLists = async () => {
    const d = await api('GET', '/api/zapret/lists');
    if (d) {
      setLists(d.lists || []);
      const firstEditable = (d.lists || []).find(l => l.editable);
      if (firstEditable) {
        setSelected(firstEditable.name);
        setContent(firstEditable.lines.join('\n'));
      }
    }
  };

  const loadSkipRes = async () => {
    const d = await api('GET', '/api/zapret/skip-resources');
    if (d && d.content != null) setSkipRes(d.content);
  };

  const handleSelect = (name) => {
    setSelected(name);
    if (name === '__skip_resources__') {
      setContent(skipRes);
    } else {
      const list = lists.find(l => l.name === name);
      if (list) setContent(list.lines.join('\n'));
    }
  };

  const handleSave = async () => {
    if (!selected) return;
    setSaving(true);
    if (selected === '__skip_resources__') {
      setSkipRes(content);
      await apiCall(() => api('POST', '/api/zapret/skip-resources/save', { content }), t('zapret.skipResSaved'), showToast);
    } else {
      await apiCall(() => api('POST', '/api/zapret/lists/save', { name: selected, content }), t('zapret.listSaved'), showToast);
      loadLists();
    }
    setSaving(false);
  };

  const editableLists = lists.filter(l => l.editable);
  const readonlyLists = lists.filter(l => !l.editable);
  const skipResCount = skipRes ? skipRes.split('\n').filter(l => l.trim()).length : 0;

  return (
    <div className="lists-2col">
      <div className="lists-left">
        <div className="section lists-list-section">
          <div className="section-title">{t('zapret.userLists')}</div>
          {editableLists.map(l => (
            <button key={l.name} className={'strat-item compact' + (selected === l.name ? ' active' : '')} onClick={() => handleSelect(l.name)}>
              <span className="strat-name">{l.name}</span>
              <span className="strat-badge">{l.count}</span>
            </button>
          ))}
          <button className={'strat-item compact' + (selected === '__skip_resources__' ? ' active' : '')} onClick={() => handleSelect('__skip_resources__')}>
            <span className="strat-name">{t('zapret.skipResourcesTitle')}</span>
            <span className="strat-badge">{skipResCount}</span>
          </button>
        </div>

        {readonlyLists.length > 0 && (
          <div className="section lists-sys-section">
            <div className="section-title">{t('zapret.systemLists')}</div>
            <div className="lists-sys-grid">
              {readonlyLists.map(l => (
                <div key={l.name} className="lists-sys-item">
                  <span className="lists-sys-name">{l.name}</span>
                  <span className="lists-sys-count">{l.count}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="section lists-editor-section">
        <div className="section-title">
          {selected === '__skip_resources__' ? t('zapret.skipResourcesTitle')
            : selected ? selected : t('zapret.editor')}
        </div>
        {selected === '__skip_resources__' && (
          <div className="zp-skip-desc">{t('zapret.skipResourcesDesc')}</div>
        )}
        {selected ? (
          <div className="list-editor">
            <textarea value={content} onChange={e => setContent(e.target.value)}
              placeholder={selected === '__skip_resources__' ? t('zapret.skipResourcesPlaceholder') : t('zapret.textareaPlaceholder')} />
            <button className="btn btn-accent btn-sm" onClick={handleSave} disabled={saving}>
              {saving ? t('common.saving') : t('common.save')}
            </button>
          </div>
        ) : (
          <div className="proxy-empty">{t('zapret.selectList')}</div>
        )}
      </div>
    </div>
  );
}
