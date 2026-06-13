import { useState, useEffect, useRef, useCallback } from 'react';
import Card from '../components/Card';
import CopyBtn from '../components/CopyBtn';
import Skeleton from '../components/Skeleton';
import { api } from '../api';
import { LANG } from '../lang';
import { formatBytes } from '../utils';

/* ── Equalizer Chart ── */
function Equalizer({ data, type }) {
  if (!data || data.length < 2) return <div className="eq-container" />;
  const mx = Math.max(...data, 1);
  return (
    <div className="eq-container">
      {data.map((v, i) => {
        const h = Math.max(3, (v / mx) * 100);
        return (
          <div className={'eq-bar ' + type} key={i} style={{ height: h + '%' }} />
        );
      })}
    </div>
  );
}

export default function MonitorPage({ status, showToast }) {
  const [connections, setConnections] = useState([]);
  const [devices, setDevices] = useState([]);
  const [deviceInfo, setDeviceInfo] = useState({});
  const [history, setHistory] = useState({ dl: [], ul: [] });
  const [defaultRes, setDefaultRes] = useState(null);
  const [userRes, setUserRes] = useState(null);
  const [resLoading, setResLoading] = useState(true);
  const resGridRef = useRef(null);
  const [resCols, setResCols] = useState(4);

  const m = status?.monitor || {};
  const p = status?.proxy || {};
  const net = status?.network || {};
  const pRun = p.running === true;
  const zRun = status?.zapret?.status === 'running';

  /* connections + devices */
  useEffect(() => {
    if (!pRun) { setConnections([]); setDevices([]); setDeviceInfo({}); return; }
    const load = async () => {
      const [c, d] = await Promise.all([
        api('GET', '/api/proxy/connections'),
        api('GET', '/api/monitor/devices'),
      ]);
      if (c) { setConnections(c.connections || []); setDeviceInfo(c.device_info || {}); }
      if (d) setDevices(d.devices || []);
    };
    load();
    const iv = setInterval(load, 3000);
    return () => clearInterval(iv);
  }, [pRun]);

  /* history from backend API (SQLite) */
  useEffect(() => {
    const loadHistory = async () => {
      try {
        const d = await api('GET', '/api/monitor/history?minutes=30');
        if (d && d.snapshots) {
          setHistory({
            dl: d.snapshots.map(s => s.dl || 0),
            ul: d.snapshots.map(s => s.ul || 0),
          });
        }
      } catch {}
    };
    loadHistory();
    const iv = setInterval(loadHistory, 5000);
    return () => clearInterval(iv);
  }, []);

  /* resource status */
  useEffect(() => {
    let active = true;
    const load = async () => {
      const d = await api('GET', '/api/resource-status');
      if (d && active) {
        setDefaultRes(d.default || []);
        setUserRes(d.user || []);
        setResLoading(false);
      }
    };
    load();
    const iv = setInterval(load, 30000);
    return () => { active = false; clearInterval(iv); };
  }, []);

  useEffect(() => {
    const el = resGridRef.current;
    if (!el) return;
    const measure = () => setResCols(Math.max(2, Math.floor(el.clientWidth / 140)));
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [status]);

  /* aggregate by host */
  const targetByHost = {};
  connections.forEach(c => {
    const h = c.target_addr || 'unknown';
    if (!targetByHost[h]) targetByHost[h] = { bytes: 0, conns: 0, down: 0, up: 0 };
    targetByHost[h].bytes += (c.bytes_recv || 0) + (c.bytes_sent || 0);
    targetByHost[h].down += c.bytes_recv || 0;
    targetByHost[h].up += c.bytes_sent || 0;
    targetByHost[h].conns++;
  });

  const entries = Object.entries(targetByHost)
    .map(([host, s]) => ({
      host, hostShort: host.replace(/:\d+$/, '').split('.').slice(-2).join('.'), ...s,
    }))
    .sort((a, b) => b.bytes - a.bytes);

  const maxBytes = entries.length > 0 ? entries[0].bytes : 1;
  const totalConns = connections.length;
  const totalDown = connections.reduce((s, c) => s + (c.bytes_recv || 0), 0);
  const totalUp = connections.reduce((s, c) => s + (c.bytes_sent || 0), 0);
  const top3 = entries.slice(0, 3);

  const lanIP = net.ips?.[0] || '127.0.0.1';
  const proxyUrl = pRun ? 'socks5://' + lanIP + ':' + (p.port || 1080) : null;

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

        {/* ── Speed + Equalizer ─────────────── */}
        <div className="ov-speed-card">
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">↓ Загрузка</span>
              <span className="ov-speed-value">{m.dl_speed_fmt || '0 B/s'}</span>
            </div>
            <Equalizer data={history.dl} type="dl" />
            <span className="ov-speed-total">Всего {m.download_fmt || '0 B'}</span>
          </div>
          <div className="ov-speed-divider"></div>
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">↑ Отдача</span>
              <span className="ov-speed-value">{m.ul_speed_fmt || '0 B/s'}</span>
            </div>
            <Equalizer data={history.ul} type="ul" />
            <span className="ov-speed-total">Всего {m.upload_fmt || '0 B'}</span>
          </div>
          <div className="ov-speed-divider"></div>
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">Подключения</span>
              <span className="ov-speed-value">{totalConns}</span>
            </div>
            <div className="ov-speed-bar-wrap" style={{ marginTop: 8 }}>
              <div className="ov-speed-bar" style={{ width: Math.min(totalConns * 3, 100) + '%', background: 'var(--warning)' }}></div>
            </div>
            <span className="ov-speed-total">↓{formatBytes(totalDown)} · ↑{formatBytes(totalUp)}</span>
          </div>
        </div>

        {/* ── Proxy Status + Devices ───────────────── */}
        <div className="ov-conns-card">
          <div className="ov-conns-header">
            <span className="ov-conns-title">Прокси (SOCKS5)</span>
            <span className={'ov-status-badge' + (pRun ? ' ok' : ' off')}>{pRun ? 'Работает' : 'Остановлен'}</span>
          </div>
          {pRun ? (
            <div className="proxy-devices">
              {proxyUrl && (
                <div className="proxy-url-bar">
                  <span className="proxy-url-label">URL</span>
                  <span className="proxy-url-value mono">{proxyUrl}</span>
                  <CopyBtn text={proxyUrl} onCopied={() => showToast(LANG.copied, 'success')} />
                </div>
              )}
              {devices.length > 0 && (
                <div className="proxy-device-grid">
                  {[...devices].sort((a, b) => (b.connections || 0) - (a.connections || 0)).slice(0, 3).map(dev => {
                    const host = dev.hostname || deviceInfo[dev.ip]?.hostname || '';
                    const mac = deviceInfo[dev.ip]?.mac || '';
                    return (
                      <div key={dev.ip} className="proxy-device-card">
                        <span className="proxy-device-icon">
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
                        </span>
                        <div className="proxy-device-info">
                          <span className="proxy-device-ip mono">{dev.ip}</span>
                          <span className="proxy-device-host">{host || mac || '—'}</span>
                        </div>
                        <span className="proxy-device-conns">{dev.connections} соед.</span>
                      </div>
                    );
                  })}
                </div>
              )}
              {devices.length === 0 && <div className="ov-empty">Нет подключённых устройств</div>}
            </div>
          ) : (
            <div className="ov-empty">Запустите прокси в боковой панели</div>
          )}
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

        {/* ── Top-3 Connections by Traffic ── */}
        <div className="ov-conns-card">
          <div className="ov-conns-header">
            <span className="ov-conns-title">Топ-3 соединения</span>
            {entries.length > 0 && <span className="ov-conns-count">{entries.length} всего</span>}
          </div>
          {top3.length > 0 ? (
            <div style={{ padding: 16 }}>
              <div className="top3-grid">
                {top3.map((e, i) => (
                  <div key={e.host} className="top3-card">
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <span className="top3-rank">{i + 1}</span>
                      <span className="top3-name">{e.hostShort}</span>
                    </div>
                    <span className="ov-conn-addr mono" style={{ fontSize: 10 }}>{e.host}</span>
                    <div className="top3-bar-wrap">
                      <div className="top3-bar-fill" style={{ width: Math.max((e.bytes / maxBytes) * 100, 5) + '%' }} />
                    </div>
                    <div className="top3-stats">
                      <span className="top3-dn">↓{formatBytes(e.down)}</span>
                      <span className="top3-up">↑{formatBytes(e.up)}</span>
                      <span style={{ color: 'var(--text-3)' }}>{e.conns} соед.</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="ov-empty">{pRun ? 'Нет активных соединений' : 'Запустите прокси для просмотра'}</div>
          )}
        </div>

        {/* ── Network Info Bar ─────────────── */}
        <div className="ov-net-bar">
          <span className="ov-net-item-inline">{net.hostname || '—'}</span>
          <span className="ov-net-sep">·</span>
          <span className="ov-net-item-inline mono">{(net.ips || [])[0] || '—'}</span>
          {proxyUrl && <>
            <span className="ov-net-sep">·</span>
            <span className="ov-net-item-inline mono">{proxyUrl.replace('socks5://', '')}</span>
            <CopyBtn text={proxyUrl.replace('socks5://', '')} onCopied={() => showToast(LANG.copied, 'success')} />
          </>}
          <span className="ov-net-sep">·</span>
          <span className="ov-net-item-inline" style={{ color: zRun ? 'var(--success)' : 'var(--danger)' }}>
            Zapret: {zRun ? 'Работает' : 'Остановлен'}
          </span>
        </div>

      </div>
    </>
  );
}