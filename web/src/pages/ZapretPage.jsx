import { useState, useEffect, useRef } from 'react';
import Modal from '../components/ui/Modal';
import Switch from '../components/ui/Switch';
import { api, apiCall, createStream } from '../api';
import { useT } from '../i18n';

export default function ZapretPage({ status, showToast }) {
  const { t } = useT();
  const [subtab, setSubtab] = useState('strategies');

  return (
    <>
      <div className="subtabs">
        <button className={'subtab' + (subtab === 'strategies' ? ' active' : '')} onClick={() => setSubtab('strategies')}>{t('zapret.strategies')}</button>
        <button className={'subtab' + (subtab === 'filters' ? ' active' : '')} onClick={() => setSubtab('filters')}>{t('zapret.filters')}</button>
        <button className={'subtab' + (subtab === 'lists' ? ' active' : '')} onClick={() => setSubtab('lists')}>{t('zapret.lists')}</button>
      </div>

      {subtab === 'strategies' && <StrategiesTab status={status} showToast={showToast} />}
      {subtab === 'filters' && <FiltersTab showToast={showToast} />}
      {subtab === 'lists' && <ListsTab showToast={showToast} />}
    </>
  );
}

function StrategiesTab({ status, showToast }) {
  const { t } = useT();
  const [strategies, setStrategies] = useState([]);
  const [changing, setChanging] = useState(null);
  const [testModal, setTestModal] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testProgress, setTestProgress] = useState(null);
  const [bestStrategy, setBestStrategy] = useState(null);
  const esRef = useRef(null);

  useEffect(() => { loadStrategies(); }, []);

  const loadStrategies = async () => {
    const d = await api('GET', '/api/zapret/strategies');
    if (d) setStrategies(d.strategies || []);
  };

  const handleSet = async (fn) => {
    setChanging(fn);
    await apiCall(() => api('POST', '/api/zapret/set-strategy', { filename: fn }), t('zapret.strategyApplied'), showToast);
    await loadStrategies();
    setChanging(null);
  };

  const handleAutoTest = () => {
    setTestModal(true); setTesting(true); setTestProgress(null); setBestStrategy(null);
    const es = createStream('/api/strategy/stream');
    esRef.current = es;
    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      if (d.type === 'done') {
        es.close(); esRef.current = null; setTesting(false);
        showToast(t('zapret.autoTestComplete'), 'success');
        return;
      }
      if (d.type === 'result' && d.resources_n > 0 && !d.error) {
        if (!bestStrategy || (d.resources_ok / d.resources_n) > (bestStrategy.resources_ok / bestStrategy.resources_n)) {
          setBestStrategy(d);
        }
      }
      if (d.type === 'progress') { setTestProgress(d); }
    };
    es.onerror = () => { es.close(); esRef.current = null; setTesting(false); };
  };

  const cancelTest = async () => {
    await api('POST', '/api/strategy/cancel');
    if (esRef.current) { esRef.current.close(); esRef.current = null; }
    setTesting(false);
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

      <button className="btn btn-sm" onClick={handleAutoTest} style={{ marginTop: 8 }}>
        {t('zapret.testStrategies')}
      </button>

      <Modal open={testModal} onClose={() => !testing && setTestModal(false)} title={t('zapret.autoSelect')} wide>
        {testing && (
          <div className="at-active">
            {testProgress && (
              <>
                <div className="at-progress-bar">
                  <div className="at-progress-fill" style={{ width: ((testProgress.current / Math.max(testProgress.total, 1)) * 100) + '%' }} />
                </div>
                <div className="at-progress-info">
                  <span className="at-progress-count">{testProgress.current} / {testProgress.total}</span>
                  <span className="at-current-str">{testProgress.strategy}</span>
                </div>
                {testProgress.phase && (
                  <div className="at-phase">
                    <span className={'at-phase-dot ' + testProgress.phase} />
                    <span className="at-phase-text">{testProgress.message || ''}</span>
                  </div>
                )}
              </>
            )}
            <button className="btn btn-danger btn-sm" onClick={cancelTest} style={{ marginTop: 8, width: '100%' }}>
              {t('common.cancel')}
            </button>
          </div>
        )}

        {!testing && bestStrategy && (
          <div className="at-best">
            <div className="at-best-header">
              <span className="at-best-title">{t('zapret.bestStrategy')}</span>
            </div>
            <div className="at-best-card">
              <div className="at-best-right">
                <span className="at-best-name">{bestStrategy.strategy}</span>
                <div className="at-best-score">
                  <span className="at-best-score-num">{bestStrategy.resources_ok}</span>
                  <span className="at-best-score-den">/ {bestStrategy.resources_n} {t('zapret.resources')}</span>
                </div>
                <div className="at-best-ms">{bestStrategy.response_ms} мс</div>
                <button className="btn btn-accent btn-sm" onClick={() => { handleSet(bestStrategy.strategy); setTestModal(false); }} style={{ marginTop: 6 }}>
                  {t('common.apply')}
                </button>
              </div>
            </div>
          </div>
        )}

        {!testing && !bestStrategy && (
          <div className="at-empty">{t('zapret.noWorkingStrategies')}</div>
        )}

        {!testing && (
          <button className="btn btn-accent" onClick={() => setTestModal(false)} style={{ marginTop: 12, width: '100%' }}>
            {t('common.close')}
          </button>
        )}
      </Modal>
    </>
  );
}

function FiltersTab({ showToast }) {
  const { t } = useT();
  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');
  const [autoUpdate, setAutoUpdate] = useState(true);
  const [autoStart, setAutoStart] = useState(false);

  useEffect(() => { loadFilters(); }, []);

  const loadFilters = async () => {
    const gf = await api('GET', '/api/zapret/game-filter');
    if (gf) setGameFilter(gf.mode || 'disabled');
    const ip = await api('GET', '/api/zapret/ipset-status');
    if (ip) setIpsetStatus(ip.status || 'loaded');
    const au = await api('GET', '/api/zapret/auto-update-status');
    if (au) setAutoUpdate(au.enabled !== false);
    const c = await api('GET', '/api/config');
    if (c) setAutoStart(c.auto_start_zapret || false);
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

  const handleAutoUpdate = async () => {
    const next = !autoUpdate;
    setAutoUpdate(next);
    await apiCall(() => api('POST', '/api/zapret/auto-update-toggle', { enabled: next }), null, showToast);
  };

  const handleAutoStart = async () => {
    const next = !autoStart;
    setAutoStart(next);
    const c = await api('GET', '/api/config');
    if (c) await api('POST', '/api/config', { ...c, auto_start_zapret: next });
  };

  const handleUpdateIpset = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-ipset'), t('zapret.ipsetUpdated'), showToast);
  };

  const handleUpdateHosts = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-hosts'), t('zapret.hostsUpdated'), showToast);
  };

  return (
    <>
      <div className="flt-section">
        <div className="flt-label">Game Filter</div>
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
        <div className="flt-label">IPSet Filter</div>
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

      <div className="flt-section">
        <div className="flt-row">
          <div className="flt-row-info">
            <span className="flt-row-title">{t('zapret.autoStartZapret')}</span>
            <span className="flt-row-desc">{t('zapret.autoStartZapretDesc')}</span>
          </div>
          <Switch checked={autoStart} onChange={handleAutoStart} />
        </div>
      </div>

      <div className="flt-section">
        <div className="flt-row">
          <div className="flt-row-info">
            <span className="flt-row-title">{t('zapret.autoUpdateCheck')}</span>
            <span className="flt-row-desc">{t('zapret.autoUpdateCheckDesc')}</span>
          </div>
          <Switch checked={autoUpdate} onChange={handleAutoUpdate} />
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
    <>
      <div className="section">
        <div className="section-title">{t('zapret.userLists')}</div>
        {editableLists.map(l => (
          <button key={l.name} className={'strat-item' + (selected === l.name ? ' active' : '')} onClick={() => handleSelect(l.name)}>
            <span className="strat-name">{l.name}</span>
            <span className="strat-badge">{l.count}</span>
          </button>
        ))}
      </div>

      {selected && (
        <div className="section">
          <div className="section-title">{t('zapret.editor')} {selected}</div>
          <div className="list-editor">
            <textarea value={content} onChange={e => setContent(e.target.value)} placeholder={t('zapret.textareaPlaceholder')} />
            <div className="btn-row">
              <button className="btn btn-accent btn-sm" onClick={handleSave} disabled={saving}>
                {saving ? t('common.saving') : t('common.save')}
              </button>
            </div>
          </div>
        </div>
      )}

      {readonlyLists.length > 0 && (
        <div className="section">
          <div className="section-title">{t('zapret.systemLists')}</div>
          {readonlyLists.map(l => (
            <div key={l.name} className="info-row">
              <span className="info-label">{l.name}</span>
              <span className="info-value mono">{l.count} {t('common.rows')}</span>
            </div>
          ))}
        </div>
      )}
    </>
  );
}
