import { useState, useEffect, useRef, useCallback } from 'react';
import Card from '../components/Card';
import CopyBtn from '../components/CopyBtn';
import Skeleton from '../components/Skeleton';
import { api } from '../api';
import { LANG } from '../lang';
import { formatBytes } from '../utils';
import { strategyDisplayName } from '../utils';
import { getSnapshots, cacheGet, cacheSet } from '../db';

function MiniChart({ data, color, maxVal }) {
  if (!data || data.length < 2) return null;
  const w = 200, h = 32;
  const mx = maxVal || Math.max(...data, 1);
  const pts = data.map((v, i) => {
    const x = (i / (data.length - 1)) * w;
    const y = h - (v / mx) * (h - 4);
    return `${x},${y}`;
  });
  const fill = `M0,${h} L${pts.join(' L')} L${w},${h} Z`;
  return (
    <svg width="100%" height={h} viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" style={{ display: 'block' }}>
      <defs>
        <linearGradient id={`g-${color}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.3" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={fill} fill={`url(#g-${color})`} />
      <polyline points={pts.join(' ')} fill="none" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  );
}

export default function OverviewPage({ status, showToast }) {
  const [connections, setConnections] = useState([]);
  const [topConns, setTopConns] = useState([]);
  const [history, setHistory] = useState({ dl: [], ul: [] });
  const [connsExpanded, setConnsExpanded] = useState(false);
  const [defaultRes, setDefaultRes] = useState(null);
  const [userRes, setUserRes] = useState(null);
  const [resLoading, setResLoading] = useState(true);
  const resGridRef = useRef(null);
  const [resCols, setResCols] = useState(4);

  const z = status?.zapret || {};
  const p = status?.proxy || {};
  const m = status?.monitor || {};
  const net = status?.network || {};
  const mod = status?.mod || {};
  const zRun = z.status === 'running';
  const pRun = p.running === true;

  useEffect(() => {
    if (!pRun) { setConnections([]); setTopConns([]); return; }
    const load = async () => {
      const c = await api('GET', '/api/proxy/connections');
      if (c) {
        const conns = c.connections || [];
        setConnections(conns);
        const byTarget = {};
        conns.forEach(cn => {
          const k = cn.target_addr || '?';
          if (!byTarget[k]) byTarget[k] = { bytes: 0, conns: 0, down: 0, up: 0 };
          byTarget[k].bytes += (cn.bytes_recv || 0) + (cn.bytes_sent || 0);
          byTarget[k].down += cn.bytes_recv || 0;
          byTarget[k].up += cn.bytes_sent || 0;
          byTarget[k].conns++;
        });
        const sorted = Object.entries(byTarget)
          .map(([addr, s]) => ({ addr, short: addr.replace(/:\d+$/, '').split('.').slice(-2).join('.'), ...s }))
          .sort((a, b) => b.bytes - a.bytes)
          .slice(0, 6);
        setTopConns(sorted);
      }
    };
    load();
    const iv = setInterval(load, 4000);
    return () => clearInterval(iv);
  }, [pRun]);

  useEffect(() => {
    const loadHistory = async () => {
      try {
        const snaps = await getSnapshots(Date.now() - 30 * 60 * 1000);
        const dl = snaps.map(s => s.dl || 0);
        const ul = snaps.map(s => s.ul || 0);
        setHistory({ dl, ul });
      } catch {}
    };
    loadHistory();
    const iv = setInterval(loadHistory, 5000);
    return () => clearInterval(iv);
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
    const el = resGridRef.current;
    if (!el) return;
    const measure = () => {
      const w = el.clientWidth;
      setResCols(Math.max(2, Math.floor(w / 140)));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [status]);

  const lanIP = net.ips?.[0] || '127.0.0.1';
  const proxyUrl = pRun ? 'socks5://' + lanIP + ':' + (p.port || 1080) : null;
  const totalDown = connections.reduce((s, c) => s + (c.bytes_recv || 0), 0);
  const totalUp = connections.reduce((s, c) => s + (c.bytes_sent || 0), 0);
  const maxBytes = topConns.length > 0 ? topConns[0].bytes : 1;

  if (!status) {
    return (
      <>
        <div className="page-title">Обзор</div>
        <div className="ov-grid">
          <div className="ov-status-row">
            <Skeleton lines={1} height={64} />
            <Skeleton lines={1} height={64} />
          </div>
          <Skeleton lines={1} height={80} />
          <Skeleton lines={4} height={40} />
        </div>
      </>
    );
  }

  return (
    <>
      <div className="page-title">Обзор</div>
      <div className="ov-grid">

        {/* ── Status Cards ─────────────────── */}
        <div className="ov-status-row">
          <div className={'ov-status-card' + (zRun ? ' on' : '')}>
            <div className="ov-status-top">
              <div className="ov-status-icon">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
              </div>
              <div className="ov-status-name">Zapret</div>
              <div className={'ov-status-dot' + (zRun ? ' on' : '')}></div>
            </div>
            <div className="ov-status-details">
              <div className="ov-status-row-items">
                <span className="ov-status-chip">v{z.version || '—'}</span>
                {zRun && <span className="ov-status-chip accent">{z.strategy}</span>}
              </div>
            </div>
          </div>

          <div className={'ov-status-card' + (pRun ? ' on' : '')}>
            <div className="ov-status-top">
              <div className="ov-status-icon proxy">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10A15.3 15.3 0 0 1 12 2z"/></svg>
              </div>
              <div className="ov-status-name">Прокси</div>
              <div className={'ov-status-dot' + (pRun ? ' on' : '')}></div>
            </div>
            <div className="ov-status-details">
              <div className="ov-status-row-items">
                <span className="ov-status-chip">:{p.port || '—'}</span>
                {pRun && <span className="ov-status-chip">{connections.length} подкл</span>}
                {pRun && <span className="ov-status-chip">↓{formatBytes(totalDown)} ↑{formatBytes(totalUp)}</span>}
              </div>
            </div>
          </div>
        </div>

        {/* ── Speed Card ───────────────────── */}
        <div className="ov-speed-card">
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">↓ Скачивание</span>
              <span className="ov-speed-value">{m.dl_speed_fmt || '0 B/s'}</span>
            </div>
            <div className="ov-speed-bar-wrap">
              <div className="ov-speed-bar down" style={{ width: '60%' }}></div>
            </div>
            <span className="ov-speed-total">Всего {m.download_fmt || '0'}</span>
          </div>
          <div className="ov-speed-divider"></div>
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">↑ Отправка</span>
              <span className="ov-speed-value">{m.ul_speed_fmt || '0 B/s'}</span>
            </div>
            <div className="ov-speed-bar-wrap">
              <div className="ov-speed-bar up" style={{ width: '30%' }}></div>
            </div>
            <span className="ov-speed-total">Всего {m.upload_fmt || '0'}</span>
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

        {/* ── Active Connections ────────────── */}
        <div className="ov-conns-card">
          <div className="ov-conns-header" onClick={() => setConnsExpanded(!connsExpanded)} style={{ cursor: 'pointer' }}>
            <div className="ov-conns-title-row">
              <span className="ov-conns-title">Активные подключения</span>
              <svg className={'ov-conns-chevron' + (connsExpanded ? ' open' : '')} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
            {pRun && <span className="ov-conns-count">{topConns.length} ресурсов</span>}
          </div>
          {topConns.length > 0 ? (
            <div className={'ov-conns-list' + (connsExpanded ? ' expanded' : '')}>
              {(connsExpanded ? topConns : topConns.slice(0, 4)).map((c, i) => (
                <div key={c.addr} className="ov-conn-row">
                  <span className="ov-conn-rank">{i + 1}</span>
                  <div className="ov-conn-info">
                    <span className="ov-conn-name">{c.short}</span>
                    <span className="ov-conn-addr mono">{c.addr}</span>
                  </div>
                  <div className="ov-conn-bar-wrap">
                    <div className="ov-conn-bar" style={{ width: Math.max((c.bytes / maxBytes) * 100, 2) + '%' }}></div>
                  </div>
                  <div className="ov-conn-stats">
                    <span className="ov-conn-dn">↓{formatBytes(c.down)}</span>
                    <span className="ov-conn-up">↑{formatBytes(c.up)}</span>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="ov-empty">
              {pRun ? 'Нет активных подключений' : 'Запустите прокси для просмотра'}
            </div>
          )}
        </div>

        {/* ── Network Info ─────────────────── */}
        <div className="ov-net-bar">
          <span className="ov-net-item-inline">{net.hostname || '—'}</span>
          <span className="ov-net-sep">·</span>
          <span className="ov-net-item-inline mono">{(net.ips || [])[0] || '—'}</span>
          {proxyUrl && <>
            <span className="ov-net-sep">·</span>
            <span className="ov-net-item-inline mono">{proxyUrl.replace('socks5://', '')}</span>
            <CopyBtn text={proxyUrl.replace('socks5://', '')} onCopied={() => showToast(LANG.copied, 'success')} />
          </>}
        </div>

      </div>
    </>
  );
}
