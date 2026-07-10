import { useState, useEffect, useMemo } from 'react';
import { api, apiCall } from '../api';
import { useT } from '../i18n';
import { Play, Square, RefreshCw, Stethoscope } from 'lucide-react';
import { useConfirm } from '../components/ui/ConfirmDialog';

// sortStrategiesNatural — natural sort by name + number.
// "general (ALT).bat" < "general (ALT2).bat" < ... < "general (ALT10).bat"
// "general (FAKE TLS AUTO).bat" < "general (FAKE TLS AUTO ALT).bat"
// "general.bat" goes first.
function sortStrategiesNatural(strategies) {
  return [...strategies].sort((a, b) => {
    const an = a.filename || a.name;
    const bn = b.filename || b.name;
    // "general.bat" first
    if (an === 'general.bat') return -1;
    if (bn === 'general.bat') return 1;
    // Extract base name (before first "(") and the part inside "(...)"
    const parseStrat = (s) => {
      // "general (ALT12).bat" -> base="general", variant="ALT12"
      const m = s.match(/^(.+?)\s*\(([^)]+)\)\.bat$/i);
      if (m) return { base: m[1].toLowerCase(), variant: m[2] };
      // fallback - whole name without .bat
      return { base: s.replace(/\.bat$/i, '').toLowerCase(), variant: '' };
    };
    const pa = parseStrat(an);
    const pb = parseStrat(bn);
    // Compare base names first
    if (pa.base !== pb.base) {
      return pa.base.localeCompare(pb.base);
    }
    // Same base — compare variants by natural sort (ALT, ALT2, ALT10)
    // Extract leading letters and trailing digits
    const parseVariant = (v) => {
      const m = v.match(/^([A-Za-z ]+?)(\d+)?$/);
      if (m) return { letters: m[1].trim().toUpperCase(), num: m[2] ? parseInt(m[2], 10) : 0 };
      return { letters: v.toUpperCase(), num: 0 };
    };
    const va = parseVariant(pa.variant);
    const vb = parseVariant(pb.variant);
    if (va.letters !== vb.letters) {
      // Empty variant (just "ALT") sorts before "ALT X"
      if (va.letters === '') return -1;
      if (vb.letters === '') return 1;
      return va.letters.localeCompare(vb.letters);
    }
    return va.num - vb.num;
  });
}

export default function ZapretPage({ status, showToast, onOpenDiagnostics }) {
  const { t } = useT();
  const confirm = useConfirm();
  const [subtab, setSubtab] = useState('strategies');
  const [skipped, setSkipped] = useState(false);
  const [serviceBusy, setServiceBusy] = useState(false);

  useEffect(() => {
    api('GET', '/api/config').then(c => {
      if (c) setSkipped(c.zapret_skipped === true);
    });
  }, []);

  const handleReenable = async () => {
    await api('POST', '/api/config', { zapret_skipped: false });
    setSkipped(false);
    showToast(t('zapret.reenabled'), 'success');
  };

  const handleServiceToggle = async () => {
    setServiceBusy(true);
    const running = status?.zapret?.status === 'running';
    if (running) {
      const result = await api('POST', '/api/zapret/stop');
      if (result?.error) {
        showToast(result.error, 'error');
      } else {
        showToast(t('header.zapretStopped'), 'success');
      }
    } else {
      const result = await api('POST', '/api/zapret/start');
      if (result?.error) {
        showToast(result.error, 'error');
      } else {
        showToast(t('header.zapretStarted'), 'success');
      }
    }
    setServiceBusy(false);
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
      <>
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

  const zRun = status?.zapret?.status === 'running';
  const zVersion = status?.zapret?.version || '—';

  return (
    <>
      {/* Compact service bar — fits one line */}
      <div className="zp-service-bar">
        <div className="zp-service-status">
          <span className={'zp-status-dot ' + (zRun ? 'on' : 'off')} />
          <span className="zp-status-text">{zRun ? t('status.running') : t('status.stopped')}</span>
          <span className="zp-version">v{zVersion}</span>
        </div>
        <div className="zp-service-actions">
          <button
            className={'btn btn-sm zp-toggle-btn ' + (zRun ? 'btn-danger' : 'btn-accent') + (serviceBusy ? ' busy' : '')}
            onClick={handleServiceToggle}
            disabled={serviceBusy}
          >
            {serviceBusy ? <RefreshCw size={13} className="spinning" /> : zRun ? <Square size={13} /> : <Play size={13} />}
            {serviceBusy ? t('common.wait') : (zRun ? t('common.stop') : t('common.start'))}
          </button>
          <button
            className="btn btn-sm zp-btn-wide"
            onClick={handleReinstall}
            disabled={serviceBusy}
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
        </div>
      </div>

      <div className="subtabs">
        <button className={'subtab' + (subtab === 'strategies' ? ' active' : '')} onClick={() => setSubtab('strategies')}>{t('zapret.strategies')}</button>
        <button className={'subtab' + (subtab === 'lists' ? ' active' : '')} onClick={() => setSubtab('lists')}>{t('zapret.lists')}</button>
      </div>

      {subtab === 'strategies' && <StrategiesTab status={status} showToast={showToast} />}
      {subtab === 'lists' && <ListsTab showToast={showToast} />}
    </>
  );
}

function StrategiesTab({ status, showToast }) {
  const { t } = useT();
  const [strategies, setStrategies] = useState([]);
  const [changing, setChanging] = useState(null);
  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');
  const [testResults, setTestResults] = useState({});

  useEffect(() => {
    loadStrategies();
    loadFilters();
    loadTestResults();
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

  const loadTestResults = async () => {
    const d = await api('GET', '/api/zapret/auto-test-results');
    if (d?.results) {
      const map = {};
      for (const r of d.results) {
        if (r.strategy) {
          map[r.strategy] = r;
        }
      }
      setTestResults(map);
    }
  };

  // Natural sort: ALT, ALT2, ALT3, ... ALT10, ALT11, ALT12
  const sortedStrategies = useMemo(() => sortStrategiesNatural(strategies), [strategies]);

  const handleSet = async (fn) => {
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

  const stratCardClass = (s) => {
    const tr = testResults[s.filename];
    if (!tr) return '';
    const pct = tr.total > 0 ? (tr.ok / tr.total) : 0;
    if (pct >= 0.7) return ' result-good';
    if (pct >= 0.3) return ' result-mid';
    return ' result-bad';
  };

  return (
    <>
      {/* Strategies grid */}
      <div className="strat-grid">
        {sortedStrategies.map(s => {
          const tr = testResults[s.filename];
          return (
            <button
              key={s.filename}
              className={'strat-card' + (s.current ? ' active' : '') + stratCardClass(s)}
              onClick={() => !s.current && handleSet(s.filename)}
              disabled={changing === s.filename}
            >
              <span className="strat-card-dot" />
              <span className="strat-card-name">{s.name}</span>
              {tr && (
                <span className="strat-card-stats">
                  {tr.ok}/{tr.total}
                </span>
              )}
              {changing === s.filename && <span className="strat-card-spin" />}
            </button>
          );
        })}
        {strategies.length === 0 && <div className="strat-empty">{t('zapret.noStrategies')}</div>}
      </div>

      {/* Filters panel — reworked */}
      <div className="zp-flt2-card">
        <div className="zp-flt2-title">{t('zapret.filtersTitle')}</div>

        {/* Game filter */}
        <div className="zp-flt2-row">
          <div className="zp-flt2-left">
            <span className="zp-flt2-label">{t('zapret.gameFilter')}</span>
            <span className="zp-flt2-hint">{t('zapret.gameFilterDescShort')}</span>
          </div>
          <div className="zp-flt2-pills">
            {[
              { v: 'disabled', l: t('zapret.off') },
              { v: 'all', l: 'TCP+UDP' },
              { v: 'tcp', l: 'TCP' },
              { v: 'udp', l: 'UDP' },
            ].map(o => (
              <button key={o.v} className={'flt2-pill' + (gameFilter === o.v ? ' active' : '')} onClick={() => handleGameFilter(o.v)}>
                {o.l}
              </button>
            ))}
          </div>
        </div>

        <div className="zp-flt2-sep" />

        {/* IPSet */}
        <div className="zp-flt2-row">
          <div className="zp-flt2-left">
            <span className="zp-flt2-label">{t('zapret.ipsetFilter')}</span>
            <span className="zp-flt2-hint">{t('zapret.ipsetDescShort')}</span>
          </div>
          <div className="zp-flt2-right">
            <span className={'zp-flt2-ipset ' + ipsetStatus}>{ipsetStatus}</span>
            <button className="btn btn-xs" onClick={handleIpsetToggle}>
              {ipsetStatus === 'loaded' ? t('common.stop') : ipsetStatus === 'none' ? t('common.start') : t('common.load')}
            </button>
            <button className="btn btn-xs" onClick={handleUpdateIpset}>{t('zapret.updateIpset')}</button>
            <button className="btn btn-xs" onClick={handleUpdateHosts}>{t('zapret.updateHosts')}</button>
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
