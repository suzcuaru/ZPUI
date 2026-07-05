import { useState, useEffect } from 'react';
import { api, apiCall } from '../api';
import { useT } from '../i18n';

export default function ZapretPage({ status, showToast }) {
  const { t } = useT();
  const [subtab, setSubtab] = useState('strategies');
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
      <div className="page-title">{t('zapret.title')}</div>
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
