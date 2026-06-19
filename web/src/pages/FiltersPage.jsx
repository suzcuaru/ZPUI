import { useState, useEffect, useRef } from 'react';
import Modal from '../components/ui/Modal';
import Switch from '../components/ui/Switch';
import { api, apiCall, createStream } from '../api';
import { cacheGet, cacheSet } from '../db';

export default function FiltersPage({ status, showToast }) {
  const z = status?.zapret || {};

  const [strategies, setStrategies] = useState([]);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [testModal, setTestModal] = useState(false);
  const [testResults, setTestResults] = useState([]);
  const [testing, setTesting] = useState(false);
  const [testProgress, setTestProgress] = useState(null);
  const [changing, setChanging] = useState(null);
  const [bestStrategy, setBestStrategy] = useState(null);
  const [testInfo, setTestInfo] = useState(null);
  const [expandedResult, setExpandedResult] = useState(null);
  const esRef = useRef(null);
  const dropdownRef = useRef(null);

  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');
  const [autoUpdate, setAutoUpdate] = useState(true);

  useEffect(() => { loadStrategies(); loadFilters(); }, []);

  useEffect(() => {
    const handleClick = (e) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target)) {
        setDropdownOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  const loadStrategies = async () => {
    const cached = await cacheGet('strategies');
    if (cached) setStrategies(cached.strategies || []);
    const d = await api('GET', '/api/zapret/strategies');
    if (d) { setStrategies(d.strategies || []); cacheSet('strategies', d); }
  };

  const handleSet = async (fn) => {
    setChanging(fn);
    await apiCall(() => api('POST', '/api/zapret/set-strategy', { filename: fn }), 'Стратегия применена', showToast);
    await loadStrategies();
    setChanging(null);
    setDropdownOpen(false);
  };

  const handleAutoTest = () => {
    setTestModal(true); setTesting(true); setTestResults([]); setTestProgress(null); setTestInfo(null); setBestStrategy(null);
    const es = createStream('/api/strategy/stream');
    esRef.current = es;
    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      if (d.type === 'done') {
        es.close(); esRef.current = null; setTesting(false);
        calcBest();
        if (!d.error) showToast('Автотест завершён', 'success');
        return;
      }
      if (d.type === 'info') { setTestInfo(d); }
      else if (d.type === 'progress') { setTestProgress(d); }
      else if (d.type === 'result') { setTestResults(prev => [...prev, d]); }
    };
    es.onerror = () => { es.close(); esRef.current = null; setTesting(false); };
  };

  const cancelTest = async () => {
    await api('POST', '/api/strategy/cancel');
    if (esRef.current) { esRef.current.close(); esRef.current = null; }
    setTesting(false);
  };

  const calcBest = () => {
    setTestResults(prev => {
      const results = prev.filter(r => !r.error);
      if (results.length === 0) return prev;
      results.sort((a, b) => {
        const scoreA = (a.discord_ok ? 1 : 0) + (a.youtube_ok ? 1 : 0) + (a.resources_ok / Math.max(a.resources_n, 1));
        const scoreB = (b.discord_ok ? 1 : 0) + (b.youtube_ok ? 1 : 0) + (b.resources_ok / Math.max(b.resources_n, 1));
        if (scoreB !== scoreA) return scoreB - scoreA;
        return a.response_ms - b.response_ms;
      });
      setBestStrategy(results[0]);
      return prev;
    });
  };

  const loadFilters = async () => {
    const gf = await api('GET', '/api/zapret/game-filter');
    if (gf) setGameFilter(gf.mode || 'disabled');
    const ip = await api('GET', '/api/zapret/ipset-status');
    if (ip) setIpsetStatus(ip.status || 'loaded');
    const au = await api('GET', '/api/zapret/auto-update-status');
    if (au) setAutoUpdate(au.enabled !== false);
  };

  const handleGameFilter = async (mode) => {
    setGameFilter(mode);
    await apiCall(() => api('POST', '/api/zapret/game-filter', { mode }), 'Game Filter обновлён', showToast);
  };

  const handleIpsetToggle = async () => {
    const next = ipsetStatus === 'loaded' ? 'none' : ipsetStatus === 'none' ? 'any' : 'loaded';
    await apiCall(() => api('POST', '/api/zapret/ipset-toggle', { mode: next }), 'IPSet обновлён', showToast);
    loadFilters();
  };

  const handleAutoUpdate = async () => {
    const next = !autoUpdate;
    setAutoUpdate(next);
    await apiCall(() => api('POST', '/api/zapret/auto-update-toggle', { enabled: next }), 'Настройка обновлений сохранена', showToast);
  };

  const currentStrategy = strategies.find(s => s.current);
  const strategyLabel = currentStrategy?.name || z.strategy || 'Не выбрана';

  return (
    <div className="filters-page">
      <div className="flt-section">
        <div className="flt-label">Стратегия обхода DPI</div>
        <div className="flt-desc">Текущая: <strong>{strategyLabel}</strong></div>

        <div className="strategy-dropdown-wrapper" ref={dropdownRef}>
          <button
            className="strategy-dropdown-btn"
            onClick={() => setDropdownOpen(!dropdownOpen)}
          >
            <span>{strategyLabel}</span>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <polyline points={dropdownOpen ? "18 15 12 9 6 15" : "6 9 12 15 18 9"}/>
            </svg>
          </button>

          {dropdownOpen && (
            <div className="strategy-dropdown-menu">
              {strategies.map(s => (
                <div
                  key={s.filename}
                  className={'strategy-dropdown-item' + (s.current ? ' active' : '') + (changing === s.filename ? ' loading' : '')}
                  onClick={() => !s.current && handleSet(s.filename)}
                >
                  <span className="sdi-name">{s.name}</span>
                  {s.current && <span className="sdi-badge">✓</span>}
                  {!s.current && changing === s.filename && <span className="sdi-loading">...</span>}
                </div>
              ))}
            </div>
          )}
        </div>

        <button className="btn btn-accent" onClick={handleAutoTest} disabled={testing} style={{ width: '100%' }}>
          {testing ? (
            <><span className="offline-spinner" /> Тестирование...</>
          ) : (
            <><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg> Запустить автотест</>
          )}
        </button>
      </div>

      <div className="flt-section">
        <div className="flt-label">Фильтры и настройки</div>
        <div className="flt-desc">Game Filter, IPSet, автообновления</div>
        <div className="flt-label">Game Filter</div>
        <div className="radio-group">
          {[
            { v: 'disabled', l: 'Выключен', d: 'Не добавлять игровые порты' },
            { v: 'all', l: 'TCP + UDP', d: 'Добавить все порты 1024-65535' },
            { v: 'tcp', l: 'Только TCP', d: 'Только TCP-порты' },
            { v: 'udp', l: 'Только UDP', d: 'Только UDP-порты' },
          ].map(o => (
            <label key={o.v} className="radio-item">
              <input type="radio" name="gf" checked={gameFilter === o.v} onChange={() => handleGameFilter(o.v)} />
              <div className="radio-content">
                <span className="radio-label">{o.l}</span>
                {gameFilter === o.v && <span className="radio-desc">{o.d}</span>}
              </div>
            </label>
          ))}
        </div>
        <div className="flt-divider"></div>
        <div className="flt-label">IPSet Filter</div>
        <div className="flt-row">
          <div className="flt-row-info">
            <span className="flt-row-title">IPSet фильтр</span>
            <span className="flt-row-desc">Текущий статус: <strong>{ipsetStatus}</strong></span>
          </div>
          <button className="btn btn-sm" onClick={handleIpsetToggle}>
            {ipsetStatus === 'loaded' ? 'Отключить' : ipsetStatus === 'none' ? 'Включить (пустой)' : 'Загрузить'}
          </button>
        </div>
        <div className="flt-divider"></div>
        <div className="flt-label">Auto-Update</div>
        <div className="flt-row">
          <div className="flt-row-info">
            <span className="flt-row-title">Автопроверка обновлений</span>
            <span className="flt-row-desc">Проверять обновления при запуске</span>
          </div>
          <Switch checked={autoUpdate} onChange={handleAutoUpdate} />
        </div>
      </div>

      <Modal open={testModal} onClose={() => {}} title="Автоподбор стратегии" wide>
        {testing && (
          <div className="at-active">
            {testProgress && (
              <div className="at-progress">
                <div className="at-progress-bar">
                  <div className="at-progress-fill" style={{ width: ((testProgress.current / Math.max(testProgress.total, 1)) * 100) + '%' }}></div>
                </div>
                <div className="at-progress-info">
                  <span className="at-progress-count">{testProgress.current} / {testProgress.total}</span>
                  <span className="at-current-str">{testProgress.strategy}</span>
                </div>
              </div>
            )}
            {testProgress && (
              <div className="at-phase">
                <span className={'at-phase-dot ' + testProgress.phase}></span>
                <span className="at-phase-text">{testProgress.message}</span>
              </div>
            )}
            {testResults.filter(r => r.type === 'result').map((r, i) => (
              <div key={i} className="at-live-result" style={{ animationDelay: (i * 0.05) + 's' }}>
                <span className="at-lr-name">{r.strategy}</span>
                {r.error ? (
                  <span className="at-lr-err">{r.error}</span>
                ) : (
                  <>
                    <span className="at-lr-score">{r.resources_ok}/{r.resources_n}</span>
                    <span className="at-lr-ms">{r.response_ms}мс</span>
                    {r.resources_detail && r.resources_detail.every(x => x.ok) ? (
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--success)" strokeWidth="2.5"><polyline points="20 6 9 17 4 12"/></svg>
                    ) : r.resources_ok > 0 ? (
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" strokeWidth="2.5"><polyline points="20 6 9 17 4 12"/></svg>
                    ) : (
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--danger)" strokeWidth="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                    )}
                  </>
                )}
              </div>
            ))}
            <button className="btn btn-danger btn-sm" onClick={cancelTest} style={{ marginTop: 10, width: '100%' }}>
              Отменить
            </button>
          </div>
        )}

        {!testing && bestStrategy && (
          <div className="at-best">
            <div className="at-best-header">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
              <span className="at-best-title">Лучшая стратегия</span>
            </div>
            <div className="at-best-card">
              <div className="at-best-left">
                <DonutChart
                  percent={(bestStrategy.resources_ok / Math.max(bestStrategy.resources_n, 1)) * 100}
                  color="var(--success)"
                  size={90}
                  stroke={8}
                />
              </div>
              <div className="at-best-right">
                <span className="at-best-name">{bestStrategy.strategy}</span>
                <div className="at-best-score">
                  <span className="at-best-score-num">{bestStrategy.resources_ok}</span>
                  <span className="at-best-score-den">{bestStrategy.resources_n} ресурсов</span>
                </div>
                <div className="at-best-ms">{bestStrategy.response_ms} мс средняя задержка</div>
                <button className="btn btn-accent btn-sm" onClick={() => { handleSet(bestStrategy.strategy); setTestModal(false); }}
                  style={{ marginTop: 4 }}>
                  Применить эту стратегию
                </button>
              </div>
            </div>
          </div>
        )}

        {testResults.filter(r => r.type === 'result').length > 0 && (
          <div className="at-results">
            <div className="at-results-title">Результаты ({testResults.filter(r => r.type === 'result').length})</div>
            <div className="at-results-list">
              {testResults.filter(r => r.type === 'result').map((r, i) => (
                <div key={i} className="at-result-item" style={{ animationDelay: (i * 0.04) + 's' }}>
                  <div className={'at-result-row' + (r.error ? ' error' : '')}
                    onClick={() => {
                      if (testing) return;
                      setExpandedResult(expandedResult === r.strategy ? null : r.strategy);
                    }}>
                    <span className="at-r-name">{r.strategy}</span>
                    {r.error ? (
                      <span className="at-r-err">{r.error}</span>
                    ) : (
                      <>
                        <div className="at-r-score-bar">
                          <div className="at-r-score-fill" style={{ width: ((r.resources_ok / Math.max(r.resources_n, 1)) * 100) + '%' }}></div>
                        </div>
                        <span className="at-r-res">{r.resources_ok}/{r.resources_n}</span>
                        <span className="at-r-ms">{r.response_ms}мс</span>
                        <span className="at-r-expand">{expandedResult === r.strategy ? '▲' : '▼'}</span>
                      </>
                    )}
                  </div>
                  {expandedResult === r.strategy && r.resources_detail && (
                    <div className="at-resources-detail">
                      <div className="at-rd-grid">
                        {r.resources_detail.map((rd, j) => (
                          <div key={j} className={'at-rd-card ' + (rd.ok ? 'ok' : 'fail')}>
                            <div className="at-rd-card-name">{rd.name}</div>
                            <div className="at-rd-card-status">{rd.ok ? '✓' : '✗'}</div>
                            <div className="at-rd-card-ms">{rd.ms}мс</div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {!testing && testResults.length > 0 && (
          <button className="btn btn-accent" onClick={() => setTestModal(false)} style={{ marginTop: 16, width: '100%' }}>
            Закрыть
          </button>
        )}
      </Modal>
    </div>
  );
}

function DonutChart({ percent, size = 80, stroke = 6, color = 'var(--accent)' }) {
  const r = (size - stroke) / 2;
  const circ = 2 * Math.PI * r;
  const offset = circ - (percent / 100) * circ;
  return (
    <svg width={size} height={size} style={{ transform: 'rotate(-90deg)' }}>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="var(--bg-hover)" strokeWidth={stroke} />
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke={color} strokeWidth={stroke}
        strokeDasharray={circ} strokeDashoffset={offset} strokeLinecap="round"
        style={{ transition: 'stroke-dashoffset 0.6s ease' }} />
      <text x={size/2} y={size/2} textAnchor="middle" dominantBaseline="central"
        fill="var(--text-primary)" fontSize="16" fontWeight="700" style={{ transform: 'rotate(90deg)', transformOrigin: 'center' }}>
        {Math.round(percent)}%
      </text>
    </svg>
  );
}
