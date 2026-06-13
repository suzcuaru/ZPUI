import { useState, useEffect, useRef, useCallback } from 'react';
import Card from '../components/Card';
import Switch from '../components/Switch';
import Modal from '../components/Modal';
import Skeleton from '../components/Skeleton';
import { api, apiCall, openExternal } from '../api';
import { LANG } from '../lang';
import { cacheGet, cacheSet } from '../db';

export default function SettingsPage({ status, showToast, settingsTab, onSettingsTab }) {
  const z = status?.zapret || {};
  const mod = status?.mod || {};

  return (
    <div className="page-fade settings-page">
      <div className="page-title">Настройки</div>
      <div className="settings-content">
        {settingsTab === 'general' && <GeneralTab status={status} showToast={showToast} />}
        {settingsTab === 'strategy' && <StrategyTab status={status} showToast={showToast} />}
        {settingsTab === 'filter' && <FilterTab status={status} showToast={showToast} />}
        {settingsTab === 'diag' && <DiagTab status={status} showToast={showToast} />}
        {settingsTab === 'about' && <AboutTab status={status} />}
      </div>
      <BottomBar status={status} showToast={showToast} />
    </div>
  );
}

/* ── General ────────────────────────────────── */
function GeneralTab({ status, showToast }) {
  const [config, setConfig] = useState({ zapret_path: '', proxy_port: 1080, autostart: false, proxy_auto_start: false, auto_path: true });
  const [svcStatus, setSvcStatus] = useState(null);
  const [svcLoading, setSvcLoading] = useState(false);

  const z = status?.zapret || {};
  const detectedPath = z.zapretDir || '';
  const saveTimer = useRef(null);

  useEffect(() => { loadConfig(); loadSvcStatus(); }, []);

  useEffect(() => {
    const iv = setInterval(loadSvcStatus, 5000);
    return () => clearInterval(iv);
  }, []);

  const loadConfig = async () => {
    const cached = await cacheGet('config');
    if (cached) applyConfig(cached);
    const d = await api('GET', '/api/config');
    if (d) { applyConfig(d); cacheSet('config', d); }
  };

  const applyConfig = (d) => {
    const savedPath = d.zapret_path || '';
    const isAuto = !savedPath || savedPath === detectedPath;
    setConfig({
      zapret_path: savedPath || detectedPath,
      proxy_port: d.proxy?.port || 1080,
      autostart: d.autostart || false,
      proxy_auto_start: d.proxy?.auto_start || false,
      auto_path: isAuto,
    });
  };

  const loadSvcStatus = async () => { const d = await api('GET', '/api/zapret/service/status'); if (d) setSvcStatus(d); };

  const handleSvcInstall = async () => {
    setSvcLoading(true);
    await apiCall(() => api('POST', '/api/zapret/service/install', { strategy: status?.zapret?.strategy || '' }), LANG.serviceInstalled, showToast);
    setSvcLoading(false);
    setTimeout(loadSvcStatus, 2000);
  };

  const handleSvcRemove = async () => {
    setSvcLoading(true);
    await apiCall(() => api('POST', '/api/zapret/service/remove'), LANG.serviceRemoved, showToast);
    setSvcLoading(false);
    setTimeout(loadSvcStatus, 2000);
  };

  const save = useCallback(async (cfg) => {
    const pathToSave = cfg.auto_path ? detectedPath : cfg.zapret_path;
    await api('POST', '/api/config', { zapret_path: pathToSave }).catch(() => {});
    await api('POST', '/api/proxy/config', { port: parseInt(cfg.proxy_port), auto_start: cfg.proxy_auto_start }).catch(() => {});
    if (cfg.autostart) {
      await api('POST', '/api/autostart/enable').catch(() => {});
    } else {
      await api('POST', '/api/autostart/disable').catch(() => {});
    }
    showToast(LANG.saved, 'success');
  }, [detectedPath, showToast]);

  const update = useCallback((patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      clearTimeout(saveTimer.current);
      saveTimer.current = setTimeout(() => save(next), 500);
      return next;
    });
  }, [save]);

  const svcInstalled = svcStatus?.installed;
  const svcRunning = svcStatus?.running;

  return (
    <Card className="settings-card">
      <div className="settings-card-header">
        <h3>Основные настройки</h3>
        <p>Путь к Zapret, порт веб-интерфейса и автозапуск</p>
      </div>
      <div className="settings-form">
        <div className="form-group">
          <label>Путь к папке Zapret</label>
          {config.auto_path && detectedPath ? (
            <div className="ov-cdevice">
              <span className="ov-cdevice-ip mono" style={{ flex: 1 }}>{detectedPath}</span>
              <span className="ov-status-chip accent">авто</span>
            </div>
          ) : (
            <input type="text" className="form-input" value={config.zapret_path} onChange={e => update({ zapret_path: e.target.value })} />
          )}
          <div className="form-hint">
            {config.auto_path && detectedPath ? 'Модуль рядом с Zapret — путь определён автоматически' : 'Директория, где находятся winws.exe и service.bat'}
          </div>
        </div>
        {detectedPath && (
          <div className="settings-switch-row">
            <div className="settings-switch-info">
              <span className="settings-switch-title">Автоопределение пути</span>
              <span className="settings-switch-desc">Модуль и Zapret в одной папке</span>
            </div>
            <Switch checked={config.auto_path} onChange={() => update({ auto_path: !config.auto_path, zapret_path: config.auto_path ? '' : detectedPath })} />
          </div>
        )}
        <div className="form-group">
          <label>Порт прокси (SOCKS5)</label>
          <input type="number" className="form-input" value={config.proxy_port} min="1" max="65535" onChange={e => update({ proxy_port: parseInt(e.target.value) || 1080 })} />
        </div>
        <div className="settings-switch-row">
          <div className="settings-switch-info">
            <span className="settings-switch-title">Автозапуск приложения</span>
            <span className="settings-switch-desc">Автоматически запускать при старте системы</span>
          </div>
          <Switch checked={config.autostart} onChange={() => update({ autostart: !config.autostart })} />
        </div>
        <div className="settings-switch-row">
          <div className="settings-switch-info">
            <span className="settings-switch-title">Автозапуск прокси</span>
            <span className="settings-switch-desc">Запускать SOCKS5-прокси вместе с приложением</span>
          </div>
          <Switch checked={config.proxy_auto_start} onChange={() => update({ proxy_auto_start: !config.proxy_auto_start })} />
        </div>
      </div>

      <div className="settings-divider"></div>

      <div className="settings-section-label">Служба Windows</div>
      <div className="svc-info-grid">
        <div className="svc-info-row">
          <span className="svc-info-label">Статус</span>
          <span className={'svc-info-badge ' + (svcRunning ? 'ok' : svcInstalled ? 'warn' : 'off')}>
            {svcRunning ? 'Работает' : svcInstalled ? 'Установлена' : 'Не установлена'}
          </span>
        </div>
        {svcInstalled && (
          <>
            <div className="svc-info-row">
              <span className="svc-info-label">Стратегия</span>
              <span className="svc-info-value mono">{svcStatus?.strategy || '—'}</span>
            </div>
            {svcStatus?.pid > 0 && (
              <div className="svc-info-row">
                <span className="svc-info-label">PID</span>
                <span className="svc-info-value mono">{svcStatus.pid}</span>
              </div>
            )}
          </>
        )}
      </div>
      <button
        className={'btn ' + (svcInstalled ? 'btn-danger' : 'btn-accent') + ' svc-toggle-btn'}
        onClick={svcInstalled ? handleSvcRemove : handleSvcInstall}
        disabled={svcLoading}
      >
        {svcLoading ? '...' : svcInstalled ? 'Удалить службу' : 'Установить службу'}
      </button>
    </Card>
  );
}

/* ── Strategy ───────────────────────────────── */

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

function StrategyTab({ status, showToast }) {
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

  const getDesc = () => '';

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
                  <span className="at-best-score-den">/{bestStrategy.resources_n} ресурсов</span>
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

/* ── Filter ─────────────────────────────────── */
function FilterTab({ status, showToast }) {
  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');
  const [autoUpdate, setAutoUpdate] = useState(true);

  useEffect(() => { loadFilters(); }, []);

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
    await apiCall(() => api('POST', '/api/zapret/game-filter', { mode }), LANG.gameFilterSet, showToast);
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

  return (
    <Card className="settings-card">
      <div className="settings-card-header">
        <h3>Фильтры и настройки</h3>
        <p>Game Filter, IPSet, автообновления</p>
      </div>

      <div className="settings-section-label">Game Filter</div>
      <div className="radio-group settings-radio">
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

      <div className="settings-divider"></div>

      <div className="settings-section-label">IPSet Filter</div>
      <div className="settings-switch-row">
        <div className="settings-switch-info">
          <span className="settings-switch-title">IPSet фильтр</span>
          <span className="settings-switch-desc">Текущий статус: <strong>{ipsetStatus}</strong></span>
        </div>
        <button className="btn btn-sm" onClick={handleIpsetToggle}>
          {ipsetStatus === 'loaded' ? 'Отключить' : ipsetStatus === 'none' ? 'Включить (пустой)' : 'Загрузить'}
        </button>
      </div>

      <div className="settings-divider"></div>

      <div className="settings-section-label">Auto-Update</div>
      <div className="settings-switch-row">
        <div className="settings-switch-info">
          <span className="settings-switch-title">Автопроверка обновлений</span>
          <span className="settings-switch-desc">Проверять обновления при запуске</span>
        </div>
        <Switch checked={autoUpdate} onChange={handleAutoUpdate} />
      </div>
    </Card>
  );
}

/* ── Diagnostics ────────────────────────────── */
function DiagTab({ status, showToast }) {
  const [diagResults, setDiagResults] = useState(null);
  const [diagLoading, setDiagLoading] = useState(false);
  const [clearing, setClearing] = useState({});

  const handleDiagnostics = async () => {
    setDiagLoading(true);
    const cached = await cacheGet('diagnostics');
    if (cached) setDiagResults(cached);
    const d = await api('GET', '/api/zapret/diagnostics');
    if (d) { setDiagResults(d || {}); cacheSet('diagnostics', d); }
    setDiagLoading(false);
  };

  const handleUpdateIpset = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-ipset'), 'IPSet обновлён', showToast);
  };

  const handleUpdateHosts = async () => {
    await apiCall(() => api('POST', '/api/zapret/update-hosts'), 'Hosts файл обновлён', showToast);
  };

  const handleCacheClear = async (target) => {
    setClearing(p => ({ ...p, [target]: true }));
    const d = await api('POST', '/api/zapret/cache/clear', { target });
    if (d && d.cleared && d.cleared.length > 0) {
      showToast(`Очищено: ${d.cleared.join(', ')}`, 'success');
    } else {
      showToast('Нечего очищать', 'info');
    }
    setClearing(p => ({ ...p, [target]: false }));
  };

  const statusOrder = { error: 0, warn: 1, ok: 2 };
  const sortedResults = diagResults ? Object.entries(diagResults)
    .sort((a, b) => (statusOrder[a[1]?.status] || 2) - (statusOrder[b[1]?.status] || 2)) : [];
  const okCount = sortedResults.filter(([,v]) => v.status === 'ok').length;
  const failCount = sortedResults.length - okCount;

  return (
    <>
      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Диагностика</h3>
          <p>Полная проверка системы</p>
        </div>
        <button className="btn btn-accent diag-run-btn" onClick={handleDiagnostics} disabled={diagLoading}>
          {diagLoading ? (
            <><span className="diag-spinner"></span> Проверка...</>
          ) : 'Запустить диагностику'}
        </button>
        {diagResults && sortedResults.length > 0 && (
          <div className="diag-summary">
            <div className={'diag-summary-item ok'}>
              <span className="diag-summary-num">{okCount}</span>
              <span className="diag-summary-label">ОК</span>
            </div>
            <div className={'diag-summary-item warn'}>
              <span className="diag-summary-num">{failCount}</span>
              <span className="diag-summary-label">Проблемы</span>
            </div>
          </div>
        )}
        {diagResults && sortedResults.length > 0 && (
          <div className="diag-results-list">
            {sortedResults.map(([key, val]) => (
              <div key={key} className={'diag-row' + (val.status !== 'ok' ? ' has-issue' : '')}>
                <span className={'diag-icon ' + val.status}>
                  {val.status === 'ok' ? '✓' : val.status === 'warn' ? '!' : '✗'}
                </span>
                <div className="diag-row-content">
                  <span className="diag-row-label">{val.label}</span>
                  <span className="diag-row-detail">{val.detail}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Очистка кэша</h3>
          <p>Может решить проблемы с подключением</p>
        </div>
        <div className="diag-cache-grid">
          <button className="diag-cache-btn" onClick={() => handleCacheClear('discord')} disabled={clearing.discord}>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><circle cx="12" cy="12" r="10"/><path d="M8 12h8M12 8v8"/></svg>
            <span>{clearing.discord ? '...' : 'Discord'}</span>
          </button>
          <button className="diag-cache-btn" onClick={() => handleCacheClear('network')} disabled={clearing.network}>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><circle cx="12" cy="12" r="10"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10"/></svg>
            <span>{clearing.network ? '...' : 'Сеть'}</span>
          </button>
          <button className="diag-cache-btn" onClick={() => handleCacheClear('all')} disabled={clearing.all}>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M3 6h18M8 6V4h8v2M5 6v14a2 2 0 002 2h10a2 2 0 002-2V6"/></svg>
            <span>{clearing.all ? '...' : 'Всё'}</span>
          </button>
        </div>
      </Card>

      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Обновления данных</h3>
        </div>
        <div className="btn-row">
          <button className="btn btn-accent" onClick={handleUpdateIpset}>IPSet</button>
          <button className="btn btn-accent" onClick={handleUpdateHosts}>Hosts</button>
        </div>
      </Card>
    </>
  );
}

/* ── About ──────────────────────────────────── */
function AboutTab({ status }) {
  const z = status?.zapret || {};
  const mod = status?.mod || {};
  const zapretRepo = mod.zapret_repo || 'https://github.com/bol-van/zapret';
  const modRepo = mod.mod_repo || 'https://github.com/bol-van/zapret';

  return (
    <>
      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>О системе</h3>
          <p>ZPUI — Модуль управления Zapret</p>
        </div>
        <div className="about-desc">
          <p><strong>Zapret</strong> — утилита обхода DPI (Deep Packet Inspection) блокировок провайдеров. Работает на уровне пакетов Windows, модифицируя TCP/HTTP трафик чтобы провайдер не мог определить заблокированные ресурсы.</p>
          <p><strong>ZPUI</strong> — графический веб-интерфейс для управления Zapret. Предоставляет мониторинг, настройку стратегий, SOCKS5 прокси, диагностику и автообновление.</p>
        </div>
        <div className="about-grid">
          <div className="about-item">
            <span className="about-label">Версия модуля</span>
            <span className="about-value">{mod.version || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Версия Zapret</span>
            <span className="about-value">{z.version || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Стратегия</span>
            <span className="about-value">{z.strategy || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Веб-порт</span>
            <span className="about-value mono">{mod.web_port || 8080}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Прокси-порт</span>
            <span className="about-value mono">{status?.proxy?.port || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Путь Zapret</span>
            <span className="about-value mono" style={{ fontSize: 10 }}>{z.zapretDir || '—'}</span>
          </div>
        </div>
      </Card>

      <Card className="settings-card" style={{ marginTop: 12 }}>
        <div className="settings-card-header">
          <h3>Ссылки</h3>
        </div>
        <div className="about-links">
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal(zapretRepo); }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            <span>Zapret на GitHub</span>
            <span className="about-link-url mono">{zapretRepo}</span>
          </a>
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal(modRepo); }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            <span>ZPUI на GitHub</span>
            <span className="about-link-url mono">{modRepo}</span>
          </a>
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal('https://github.com/bol-van/zapret/issues'); }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/></svg>
            <span>Сообщить о проблеме</span>
            <span className="about-link-url mono">GitHub Issues</span>
          </a>
        </div>
      </Card>
    </>
  );
}

/* ── Bottom Bar ─────────────────────────────── */
function BottomBar({ status, showToast }) {
  const z = status?.zapret || {};
  const mod = status?.mod || {};
  const [zapretUpdate, setZapretUpdate] = useState(null);
  const [modUpdate, setModUpdate] = useState(null);
  const [checkingZapret, setCheckingZapret] = useState(false);
  const [checkingMod, setCheckingMod] = useState(false);
  const [zapretProgress, setZapretProgress] = useState(0);
  const [modProgress, setModProgress] = useState(0);
  const [updateModal, setUpdateModal] = useState(false);
  const [upProg, setUpProg] = useState(null);

  const checkZapretUpdate = async () => {
    setCheckingZapret(true); setZapretProgress(0);
    const steps = 5;
    for (let i = 1; i <= steps; i++) {
      await new Promise(r => setTimeout(r, 300));
      setZapretProgress(Math.round((i / steps) * 100));
    }
    const d = await api('POST', '/api/update/check');
    setCheckingZapret(false); setZapretProgress(100);
    if (d && !d.error) setZapretUpdate(d);
  };

  const checkModUpdate = async () => {
    setCheckingMod(true); setModProgress(0);
    const steps = 5;
    for (let i = 1; i <= steps; i++) {
      await new Promise(r => setTimeout(r, 300));
      setModProgress(Math.round((i / steps) * 100));
    }
    const d = await api('POST', '/api/update/check');
    setCheckingMod(false); setModProgress(100);
    if (d && !d.error) setModUpdate(d);
  };

  const handleApplyUpdate = (type) => {
    setUpProg({ percent: 0, step: '', type }); setUpdateModal(true);
    const es = new EventSource('/api/update/stream');
    es.onmessage = (e) => {
      const d = JSON.parse(e.data);
      setUpProg({ percent: d.percent, step: d.step || '', type });
      if (d.error) showToast(d.error, 'error');
      if (d.percent >= 100) {
        es.close();
        setTimeout(() => { setUpProg(null); setUpdateModal(false); showToast(LANG.updateComplete, 'success'); }, 1000);
      }
    };
    es.onerror = () => es.close();
  };

  const zapretHasUpdate = zapretUpdate?.update_needed;
  const modHasUpdate = modUpdate?.update_needed;

  return (
    <>
      <div className="settings-bottom-bar">
        <button
          className={'bottom-bar-btn' + (zapretHasUpdate ? ' update-available' : '')}
          onClick={() => zapretHasUpdate ? handleApplyUpdate('zapret') : checkZapretUpdate()}
          disabled={checkingZapret}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          <span className="bottom-bar-text">
            {checkingZapret ? 'Проверка...' : zapretHasUpdate ? 'Обновить Zapret' : 'Zapret ' + (z.version || '—')}
          </span>
          {checkingZapret && <div className="bottom-bar-progress"><div className="bottom-bar-progress-fill" style={{ width: zapretProgress + '%' }}></div></div>}
        </button>

        <button
          className={'bottom-bar-btn' + (modHasUpdate ? ' update-available' : '')}
          onClick={() => modHasUpdate ? handleApplyUpdate('mod') : checkModUpdate()}
          disabled={checkingMod}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>
          <span className="bottom-bar-text">
            {checkingMod ? 'Проверка...' : modHasUpdate ? 'Обновить модуль' : 'Модуль ' + (mod.version || '—')}
          </span>
          {checkingMod && <div className="bottom-bar-progress"><div className="bottom-bar-progress-fill" style={{ width: modProgress + '%' }}></div></div>}
        </button>
      </div>

      <Modal open={updateModal} onClose={() => { if (!upProg) setUpdateModal(false); }} title="Обновление">
        {upProg !== null && (
          <div>
            <div className="progress-bar"><div className="progress-fill" style={{ width: upProg.percent + '%' }}></div></div>
            <span className="progress-text">{upProg.step} ({upProg.percent}%)</span>
          </div>
        )}
        {upProg === null && <div style={{ color: 'var(--success)' }}>Обновление завершено</div>}
      </Modal>
    </>
  );
}
