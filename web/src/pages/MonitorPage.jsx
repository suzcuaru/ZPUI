import { useState, useEffect, useRef } from 'react';
import Card from '../components/Card';
import Skeleton from '../components/Skeleton';
import { api } from '../api';
import { formatBytes } from '../utils';
import { getSnapshots, cacheGet, cacheSet } from '../db';

function MiniChart({ data, color, label }) {
  if (!data || data.length < 2) return null;
  const w = 200, h = 28;
  const mx = Math.max(...data, 1);
  const pts = data.map((v, i) => {
    const x = (i / (data.length - 1)) * w;
    const y = h - (v / mx) * (h - 4);
    return `${x},${y}`;
  });
  const fill = `M0,${h} L${pts.join(' L')} L${w},${h} Z`;
  return (
    <div className="mn-chart-wrap">
      <span className="mn-chart-label">{label}</span>
      <svg width="100%" height={h} viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" style={{ display: 'block' }}>
        <defs>
          <linearGradient id={`mc-${color}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.25" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={fill} fill={`url(#mc-${color})`} />
        <polyline points={pts.join(' ')} fill="none" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
      </svg>
    </div>
  );
}

export default function MonitorPage({ status, showToast }) {
  const [connections, setConnections] = useState([]);
  const [visibleCount, setVisibleCount] = useState(20);
  const [defaultRes, setDefaultRes] = useState(null);
  const [userRes, setUserRes] = useState(null);
  const [resLoading, setResLoading] = useState(true);
  const [history, setHistory] = useState({ dl: [], ul: [] });
  const connsCardRef = useRef(null);
  const resGridRef = useRef(null);
  const [resCols, setResCols] = useState(4);
  const ROW_HEIGHT = 44;

  const monitor = status?.monitor || {};

  useEffect(() => {
    let alive = true;
    const load = async () => {
      const c = await api('GET', '/api/proxy/connections');
      if (alive && c) setConnections(c.connections || []);
    };
    load();
    const iv = setInterval(load, 3000);
    return () => { alive = false; clearInterval(iv); };
  }, []);

  useEffect(() => {
    let active = true;
    const load = async (initial) => {
      if (initial) {
        const cached = await cacheGet('resource-status');
        if (cached && active) {
          setDefaultRes(cached.default || []);
          setUserRes(cached.user || []);
          setResLoading(false);
        }
      }
      const d = await api('GET', '/api/resource-status');
      if (d && active) {
        setDefaultRes(d.default || []);
        setUserRes(d.user || []);
        cacheSet('resource-status', d);
        setResLoading(false);
      }
    };
    load(true);
    const iv = setInterval(() => load(false), 30000);
    return () => { active = false; clearInterval(iv); };
  }, []);

  useEffect(() => {
    const loadHistory = async () => {
      try {
        const snaps = await getSnapshots(Date.now() - 30 * 60 * 1000);
        setHistory({ dl: snaps.map(s => s.dl || 0), ul: snaps.map(s => s.ul || 0) });
      } catch {}
    };
    loadHistory();
    const iv = setInterval(loadHistory, 5000);
    return () => clearInterval(iv);
  }, []);

  useEffect(() => {
    const el = connsCardRef.current;
    if (!el) return;
    const measure = () => {
      const rect = el.getBoundingClientRect();
      const available = window.innerHeight - rect.top - 48 - 16;
      setVisibleCount(Math.max(3, Math.floor(available / ROW_HEIGHT)));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    window.addEventListener('resize', measure);
    return () => { ro.disconnect(); window.removeEventListener('resize', measure); };
  }, [status]);

  useEffect(() => {
    const el = resGridRef.current;
    if (!el) return;
    const measure = () => setResCols(Math.max(2, Math.floor(el.clientWidth / 140)));
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [status]);

  const targetByHost = {};
  connections.forEach(c => {
    const h = c.target_addr || 'unknown';
    if (!targetByHost[h]) targetByHost[h] = { bytes: 0, conns: 0, down: 0, up: 0 };
    targetByHost[h].bytes += (c.bytes_recv || 0) + (c.bytes_sent || 0);
    targetByHost[h].down += c.bytes_recv || 0;
    targetByHost[h].up += c.bytes_sent || 0;
    targetByHost[h].conns++;
  });

  const entries = Object.entries(targetByHost).map(([host, s]) => ({
    host, hostShort: host.replace(/:\d+$/, '').split('.').slice(-2).join('.'), ...s,
  })).sort((a, b) => b.bytes - a.bytes);

  const maxBytes = entries.length > 0 ? entries[0].bytes : 1;
  const totalConns = connections.length;
  const totalDown = connections.reduce((s, c) => s + (c.bytes_recv || 0), 0);
  const totalUp = connections.reduce((s, c) => s + (c.bytes_sent || 0), 0);

  if (!status) {
    return (
      <>
        <div className="page-title">Мониторинг</div>
        <div className="ov-grid">
          <Skeleton lines={1} height={80} />
          <Skeleton lines={4} height={40} />
        </div>
      </>
    );
  }

  return (
    <>
      <div className="page-title">Мониторинг</div>
      <div className="ov-grid">

        {/* ── Speed + History Charts ──────── */}
        <div className="ov-speed-card">
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">↓ Скачивание</span>
              <span className="ov-speed-value">{monitor.dl_speed_fmt || '0 B/s'}</span>
            </div>
            <MiniChart data={history.dl} color="var(--accent)" label="30 мин" />
            <span className="ov-speed-total">Всего {monitor.download_fmt || '0'}</span>
          </div>
          <div className="ov-speed-divider"></div>
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">↑ Отправка</span>
              <span className="ov-speed-value">{monitor.ul_speed_fmt || '0 B/s'}</span>
            </div>
            <MiniChart data={history.ul} color="var(--success)" label="30 мин" />
            <span className="ov-speed-total">Всего {monitor.upload_fmt || '0'}</span>
          </div>
          <div className="ov-speed-divider"></div>
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">Подключения</span>
              <span className="ov-speed-value">{totalConns}</span>
            </div>
            <div className="ov-speed-bar-wrap">
              <div className="ov-speed-bar" style={{ width: Math.min(totalConns * 2, 100) + '%', background: 'var(--warning)' }}></div>
            </div>
            <span className="ov-speed-total">↓{formatBytes(totalDown)} ↑{formatBytes(totalUp)}</span>
          </div>
        </div>

        {/* ── Resource Status ─────────────── */}
        <div className="ov-res-card">
          <div className="ov-res-header">
            <span className="ov-res-title">Статус ресурсов</span>
            {defaultRes && (
              <span className="ov-res-summary">
                <span className="ov-res-count ok">{defaultRes.filter(r => r.ok).length}</span>
                <span className="ov-res-sep">/</span>
                <span className="ov-res-count">{defaultRes.length}</span>
              </span>
            )}
          </div>
          <div className="ov-res-grid" ref={resGridRef} style={{ gridTemplateColumns: `repeat(${resCols}, 1fr)` }}>
            {resLoading && !defaultRes ? (
              Array.from({ length: 8 }).map((_, i) => <div key={i} className="ov-res-item skeleton-pulse"><span className="ov-res-dot"></span><span className="ov-res-name">········</span></div>)
            ) : (defaultRes || []).map(r => (
              <div key={r.name} className={'ov-res-item' + (r.ok ? ' ok' : ' fail')}>
                <span className={'ov-res-dot' + (r.ok ? ' ok' : '')}></span>
                <span className="ov-res-name">{r.name}</span>
              </div>
            ))}
          </div>
        </div>

        {/* ── User Resources ──────────────── */}
        {((resLoading && !userRes) || (userRes && userRes.length > 0)) && (
          <div className="ov-res-card">
            <div className="ov-res-header">
              <span className="ov-res-title">Пользовательские</span>
              {userRes && (
                <span className="ov-res-summary">
                  <span className="ov-res-count ok">{userRes.filter(r => r.ok).length}</span>
                  <span className="ov-res-sep">/</span>
                  <span className="ov-res-count">{userRes.length}</span>
                </span>
              )}
            </div>
            <div className="ov-res-grid" style={{ gridTemplateColumns: `repeat(${resCols}, 1fr)` }}>
              {resLoading && !userRes ? (
                Array.from({ length: 4 }).map((_, i) => <div key={i} className="ov-res-item skeleton-pulse"><span className="ov-res-dot"></span><span className="ov-res-name">········</span></div>)
              ) : (userRes || []).map(r => (
                <div key={r.name} className={'ov-res-item' + (r.ok ? ' ok' : ' fail')}>
                  <span className={'ov-res-dot' + (r.ok ? ' ok' : '')}></span>
                  <span className="ov-res-name">{r.name}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* ── Connections List ──────────────── */}
        <div className="ov-conns-card" ref={connsCardRef}>
          <div className="ov-conns-header">
            <span className="ov-conns-title">Соединения</span>
            {entries.length > visibleCount && <span className="ov-conns-count">{visibleCount} из {entries.length}</span>}
          </div>
          {entries.length > 0 ? (
            <div className="ov-conns-list" style={{ maxHeight: visibleCount * ROW_HEIGHT, overflowY: 'auto' }}>
              {entries.slice(0, visibleCount).map((e, i) => (
                <div key={e.host} className="ov-conn-row">
                  <span className="ov-conn-rank">{i + 1}</span>
                  <div className="ov-conn-info">
                    <span className="ov-conn-name">{e.hostShort}</span>
                    <span className="ov-conn-addr mono">{e.host}</span>
                  </div>
                  <div className="ov-conn-bar-wrap">
                    <div className="ov-conn-bar" style={{ width: Math.max((e.bytes / maxBytes) * 100, 2) + '%' }}></div>
                  </div>
                  <div className="ov-conn-stats">
                    <span className="ov-conn-dn">↓{formatBytes(e.down)}</span>
                    <span className="ov-conn-up">↑{formatBytes(e.up)}</span>
                    <span className="ov-conn-conns">{e.conns}</span>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="ov-empty">Нет активных соединений</div>
          )}
        </div>
      </div>
    </>
  );
}
