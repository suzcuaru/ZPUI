import { useState, useEffect, useRef } from 'react';
import DiagnosticsModal from '../components/DiagnosticsModal';
import { api, apiCall, createStream } from '../api';
import { useT } from '../i18n';

export default function ZapretPage({ status, showToast }) {
  const { t } = useT();
  const [subtab, setSubtab] = useState('strategies');
  const [diagOpen, setDiagOpen] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [showColors, setShowColors] = useState(true);

  useEffect(() => {
    api('GET', '/api/config').then(c => {
      if (c) {
        setSkipped(c.zapret_skipped === true);
        setShowColors(c.show_strategy_colors !== false);
      }
    });
  }, []);

  const handleReenable = async () => {
    await api('POST', '/api/config', { zapret_skipped: false });
    setSkipped(false);
    showToast(t('zapret.reenabled'), 'success');
  };

  if (skipped) {
    return (
      <>
        <div className="page-title">{t('zapret.title')}</div>
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
      </>
    );
  }

  return (
    <>
      <div className="zapret-head">
        <div className="page-title">{t('zapret.title')}</div>
        <div className="zapret-head-actions">
          <button className="btn btn-xs btn-ghost" onClick={async () => {
            const r = await api('POST', '/api/zapret/cache/clear', { target: 'discord' });
            if (r?.cleared?.length) showToast(t('zapret.cacheCleared', { items: r.cleared.join(', ') }), 'success');
            else showToast(t('zapret.nothingToClear'), 'info');
          }}>
            <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M3 12h18M3 6h18M3 18h18"/></svg>
            Discord
          </button>
          <button className="btn btn-xs btn-ghost" onClick={async () => {
            const r = await api('POST', '/api/zapret/cache/clear', { target: 'network' });
            if (r?.cleared?.length) showToast(t('zapret.cacheCleared', { items: r.cleared.join(', ') }), 'success');
          }}>
            <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/></svg>
            {t('zapret.clearSystemCache')}
          </button>
          <button className="btn btn-sm btn-ghost" onClick={() => setDiagOpen(true)}>
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
            {t('zapret.diagnostics')}
          </button>
        </div>
      </div>
      <div className="subtabs">
        <button className={'subtab' + (subtab === 'strategies' ? ' active' : '')} onClick={() => setSubtab('strategies')}>{t('zapret.strategies')}</button>
        <button className={'subtab' + (subtab === 'lists' ? ' active' : '')} onClick={() => setSubtab('lists')}>{t('zapret.lists')}</button>
      </div>

      {subtab === 'strategies' && <StrategiesTab status={status} showToast={showToast} showColors={showColors} />}
      {subtab === 'lists' && <ListsTab showToast={showToast} />}

      <DiagnosticsModal open={diagOpen} onClose={() => setDiagOpen(false)} showToast={showToast} />
    </>
  );
}

function StrategiesTab({ status, showToast, showColors }) {
  const { t } = useT();
  const [strategies, setStrategies] = useState([]);
  const [changing, setChanging] = useState(null);
  const [testing, setTesting] = useState(false);
  const [testProgress, setTestProgress] = useState(null);
  const [testResults, setTestResults] = useState({});
  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');
  const esRef = useRef(null);

  useEffect(() => {
    loadStrategies();
    loadFilters();
    if (status?.zapret?.auto_test_running) setTesting(true);
    const rt = window.runtime;
    if (rt) {
      rt.EventsOn('strategy:testing', (active) => {
        setTesting(active);
        if (!active) loadStrategies();
      });
    }
    return () => { if (esRef.current) { esRef.current.close(); esRef.current = null; } };
  }, []);

  const loadStrategies = async () => {
    const d = await api('GET', '/api/zapret/strategies');
    if (d) setStrategies(d.strategies || []);
  };

  const loadFilters = async () => {
    const gf = await api('GET', '/api/zapret/game-filter');
    if (gf) setGameFilter(gf.mode || 'disabled');
    const ip = await api('GET', '/api/zapret/ipset-status');
    if (ip) setIpsetStatus(ip.status || 'loaded');
  };

  const getResultColor = (ok, n) => {
    if (n === 0) return 'bad';
    const ratio = ok / n;
    if (ratio >= 0.8) return 'good';
    if (ratio >= 0.4) return 'mid';
    return 'bad';
  };

  const handleStartTest = () => {
    setTesting(true);
    setTestProgress(null);
    setTestResults({});
    const es = createStream('/api/strategy/stream');
    esRef.current = es;
    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      if (d.type === 'done') {
        es.close(); esRef.current = null; setTesting(false);
        if (!d.error) showToast(t('zapret.autoTestComplete'), 'success');
        return;
      }
      if (d.type === 'progress') setTestProgress(d);
      if (d.type === 'result' && d.strategy) {
        setTestResults(prev => ({
          ...prev,
          [d.strategy]: {
            ok: d.resources_ok || 0,
            n: d.resources_n || 0,
            ms: d.response_ms || 0,
            color: d.error ? 'bad' : getResultColor(d.resources_ok, d.resources_n),
          }
        }));
      }
    };
    es.onerror = () => { es.close(); esRef.current = null; setTesting(false); };
  };

  const cancelTest = async () => {
    await api('POST', '/api/strategy/cancel');
    if (esRef.current) { esRef.current.close(); esRef.current = null; }
    setTesting(false);
  };

  const handleSet = async (fn) => {
    if (testing) return;
    setChanging(fn);
    await apiCall(() => api('POST', '/api/zapret/set-strategy', { filename: fn }), t('zapret.strategyApplied'), showToast);
    await loadStrategies();
    setChanging(null);
  };

  const handleGameFilter = async (mode) => {
    setGameFilter(mode);
    await apiCall(() => api('POST', '/api/zapret/game-filter', { mode }), t('zapret.gameFilterUpdated'), showToast);
  };

  const handleIpsetToggle = async () => {
    const next = ipsetStatus === 'loaded' ? 'none' : ipsetStatus === 'none' ? 'any' : 'loaded';
    await apiCall(() => api('POST', '/api/zapret/ipset-toggle', { mode: next }), t('zapret.ipsetUpdated'), showToast);
    loadFilters();
  };

  const handleUpdateIpset = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-ipset'), t('zapret.ipsetUpdated'), showToast);
  };

  const handleUpdateHosts = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-hosts'), t('zapret.hostsUpdated'), showToast);
  };

  return (
    <>
      <div className="strat-grid">
        {strategies.map(s => {
          const result = testResults[s.filename];
          const isTestingThis = testing && testProgress?.strategy === s.filename;
          return (
            <button
              key={s.filename}
              className={'strat-card' + (s.current ? ' active' : '') + (result && showColors ? ' result-' + result.color : '') + (isTestingThis ? ' testing-now' : '')}
              onClick={() => !s.current && !testing && handleSet(s.filename)}
              disabled={testing || changing === s.filename}
            >
              <span className="strat-card-dot" />
              <span className="strat-card-name">{s.name}</span>
              {s.current && <span className="strat-card-badge">✓</span>}
              {changing === s.filename && <span className="strat-card-spin" />}
              {result && (
                <span className="strat-card-stats" data-tooltip={`${result.ok}/${result.n} · ${result.ms}ms`}>
                  {result.ok}/{result.n}
                </span>
              )}
            </button>
          );
        })}
        {strategies.length === 0 && <div className="strat-empty">{t('zapret.noStrategies')}</div>}
      </div>

      <div className="strat-filters">
        <div className="flt-section">
          <div className="flt-label">{t('zapret.gameFilter')}</div>
          <div className="flt-radios">
            {[
              { v: 'disabled', l: t('zapret.off') },
              { v: 'all', l: 'TCP+UDP' },
              { v: 'tcp', l: 'TCP' },
              { v: 'udp', l: 'UDP' },
            ].map(o => (
            <label key={o.v} className="flt-radio">
              <input type="radio" name="gf" checked={gameFilter === o.v} onChange={() => handleGameFilter(o.v)} />
              <span>{o.l}</span>
            </label>
          ))}
        </div>
      </div>

      <div className="flt-section">
        <div className="flt-label">{t('zapret.ipsetFilter')}</div>
        <div className="flt-row">
          <div className="flt-row-info">
            <span className="flt-row-title">{t('zapret.status') + ' '}<strong>{ipsetStatus}</strong></span>
          </div>
          <button className="btn btn-sm" onClick={handleIpsetToggle}>
            {ipsetStatus === 'loaded' ? t('common.stop') : ipsetStatus === 'none' ? t('common.start') : t('common.load')}
          </button>
        </div>
        <div className="btn-row">
          <button className="btn btn-sm" onClick={handleUpdateIpset}>{t('zapret.updateIpset')}</button>
          <button className="btn btn-sm" onClick={handleUpdateHosts}>{t('zapret.updateHosts')}</button>
        </div>
      </div>

      </div>
    </>
  );
}

function ListsTab({ showToast }) {
  const { t } = useT();
  const [lists, setLists] = useState([]);
  const [selected, setSelected] = useState(null);
  const [content, setContent] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => { loadLists(); }, []);

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

  const handleSelect = (name) => {
    setSelected(name);
    const list = lists.find(l => l.name === name);
    if (list) setContent(list.lines.join('\n'));
  };

  const handleSave = async () => {
    if (!selected) return;
    setSaving(true);
    await apiCall(() => api('POST', '/api/zapret/lists/save', { name: selected, content }), t('zapret.listSaved'), showToast);
    setSaving(false);
    loadLists();
  };

  const editableLists = lists.filter(l => l.editable);
  const readonlyLists = lists.filter(l => !l.editable);

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
        <div className="section-title">{selected ? selected : t('zapret.editor')}</div>
        {selected ? (
          <div className="list-editor">
            <textarea value={content} onChange={e => setContent(e.target.value)} placeholder={t('zapret.textareaPlaceholder')} />
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
