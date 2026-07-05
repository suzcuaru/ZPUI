import { useState, useEffect } from 'react';
import DiagnosticsModal from '../components/DiagnosticsModal';
import { api, apiCall } from '../api';
import { useT } from '../i18n';

export default function ZapretPage({ status, showToast }) {
  const { t } = useT();
  const [subtab, setSubtab] = useState('strategies');
  const [diagOpen, setDiagOpen] = useState(false);
  const [skipped, setSkipped] = useState(false);

  useEffect(() => {
    api('GET', '/api/config').then(c => {
      if (c) {
        setSkipped(c.zapret_skipped === true);
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
          <button className="btn btn-xs btn-ghost" data-tooltip={t('zapret.clearDiscord')} onClick={async () => {
            const r = await api('POST', '/api/zapret/cache/clear', { target: 'discord' });
            if (r?.cleared?.length) showToast(t('zapret.cacheCleared', { items: r.cleared.join(', ') }), 'success');
            else showToast(t('zapret.nothingToClear'), 'info');
          }}>
            <svg viewBox="0 0 24 24" width="12" height="12" fill="currentColor"><path d="M20.317 4.37a19.79 19.79 0 0 0-4.885-1.515.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0 12.64 12.64 0 0 0-.617-1.25.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057 19.9 19.9 0 0 0 5.993 3.03.078.078 0 0 0 .084-.028 14.09 14.09 0 0 0 1.226-1.994.076.076 0 0 0-.041-.106 13.107 13.107 0 0 1-1.872-.892.077.077 0 0 1-.008-.128 10.2 10.2 0 0 0 .372-.292.074.074 0 0 1 .077-.01c3.928 1.793 8.18 1.793 12.062 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127 12.3 12.3 0 0 1-1.873.892.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028 19.845 19.845 0 0 0 6.002-3.03.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.06.06 0 0 0-.031-.03zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z"/></svg>
            Discord
          </button>
          <button className="btn btn-xs btn-ghost" data-tooltip={t('zapret.clearNetwork')} onClick={async () => {
            const r = await api('POST', '/api/zapret/cache/clear', { target: 'network' });
            if (r?.cleared?.length) showToast(t('zapret.cacheCleared', { items: r.cleared.join(', ') }), 'success');
          }}>
            <svg viewBox="0 0 24 24" width="12" height="12" fill="currentColor"><path d="M3 3h18v18H3V3zm2 2v14h14V5H5zm4 2h6v2h-2v6h-2v-6H9V7z"/></svg>
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

      {subtab === 'strategies' && <StrategiesTab status={status} showToast={showToast} />}
      {subtab === 'lists' && <ListsTab showToast={showToast} />}

      <DiagnosticsModal open={diagOpen} onClose={() => setDiagOpen(false)} showToast={showToast} />
    </>
  );
}

function StrategiesTab({ status, showToast }) {
  const { t } = useT();
  const [strategies, setStrategies] = useState([]);
  const [changing, setChanging] = useState(null);
  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');

  useEffect(() => {
    loadStrategies();
    loadFilters();
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

  return (
    <>
      <div className="strat-grid">
        {strategies.map(s => (
          <button
            key={s.filename}
            className={'strat-card' + (s.current ? ' active' : '')}
            onClick={() => !s.current && handleSet(s.filename)}
            disabled={changing === s.filename}
          >
            <span className="strat-card-dot" />
            <span className="strat-card-name">{s.name}</span>
            {s.current && <span className="strat-card-badge">✓</span>}
            {changing === s.filename && <span className="strat-card-spin" />}
          </button>
        ))}
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
