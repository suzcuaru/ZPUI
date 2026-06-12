import { useState, useEffect, useRef } from 'react';
import Card from '../components/Card';
import Modal from '../components/Modal';
import { api, apiCall } from '../api';
import { LANG } from '../lang';
import { cacheGet, cacheSet } from '../db';

export default function StrategyPage({ status, showToast }) {
  const [strategies, setStrategies] = useState([]);
  const [testModal, setTestModal] = useState(false);
  const [testResults, setTestResults] = useState([]);
  const [testing, setTesting] = useState(false);
  const [testProgress, setTestProgress] = useState(null);
  const [changing, setChanging] = useState(null);
  const [bestStrategy, setBestStrategy] = useState(null);
  const [testInfo, setTestInfo] = useState(null);
  const esRef = useRef(null);
  const z = status?.zapret || {};

  useEffect(() => { loadStrategies(); }, []);

  const loadStrategies = async () => {
    const cached = await cacheGet('strategies');
    if (cached) setStrategies(cached.strategies || []);
    const d = await api('GET', '/api/zapret/strategies');
    if (d) { setStrategies(d.strategies || []); cacheSet('strategies', d); }
  };

  const handleSet = async (fn) => {
    setChanging(fn);
    await apiCall(() => api('POST', '/api/zapret/set-strategy', { filename: fn }), LANG.strategySet, showToast);
    await loadStrategies();
    setChanging(null);
  };

  const handleAutoTest = () => {
    setTestModal(true); setTesting(true); setTestResults([]); setTestProgress(null); setTestInfo(null); setBestStrategy(null);
    const es = new EventSource('/api/strategy/stream');
    esRef.current = es;
    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      if (d.type === 'done') {
        es.close(); esRef.current = null; setTesting(false);
        calcBest();
        if (!d.error) showToast(LANG.autoTestComplete, 'success');
        return;
      }
      if (d.type === 'info') {
        setTestInfo(d);
      } else if (d.type === 'progress') {
        setTestProgress(d);
      } else if (d.type === 'result') {
        setTestResults(prev => [...prev, d]);
      }
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

  const [expandedResult, setExpandedResult] = useState(null);

  return (
    <>
      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Стратегия обхода DPI</h3>
          <p>Текущая: <strong>{z.strategy || '—'}</strong></p>
        </div>
        <div className="strategy-grid">
          {strategies.length > 0 ? strategies.map(s => (
            <div
              key={s.filename}
              className={'strategy-card' + (s.current ? ' active' : '') + (changing === s.filename ? ' loading' : '')}
              onClick={() => !s.current && handleSet(s.filename)}
            >
              <div className="strategy-card-top">
                <span className="strategy-card-name">{s.name}</span>
                {s.current ? (
                  <span className="strategy-card-badge">Активна</span>
                ) : (
                  <span className="strategy-card-action">{changing === s.filename ? '...' : 'Выбрать'}</span>
                )}
              </div>
            </div>
          )) : <Skeleton lines={4} height={60} />}
        </div>
        <button className="btn btn-accent" onClick={handleAutoTest} disabled={testing} style={{ marginTop: 14 }}>
          {testing ? 'Тестирование...' : 'Запустить автотест'}
        </button>
      </Card>

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
                      <span className="at-lr-star">⭐</span>
                    ) : r.resources_ok > 0 ? (
                      <span className="at-lr-star">👍</span>
                    ) : (
                      <span className="at-lr-star">👎</span>
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
              <span className="at-best-icon">🏆</span>
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
                      if (expandedResult === r.strategy) {
                        setExpandedResult(null);
                      } else {
                        setExpandedResult(r.strategy);
                      }
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
    </>
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
        fill="var(--text-1)" fontSize="16" fontWeight="700" style={{ transform: 'rotate(90deg)', transformOrigin: 'center' }}>
        {Math.round(percent)}%
      </text>
    </svg>
  );
}
