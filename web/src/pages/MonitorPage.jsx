import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { api } from '../api';
import { formatBytes } from '../utils';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';

function TrafficChart({ data, color, label, height }) {
  const { t } = useT();
  const wrapRef = useRef(null);
  const [w, setW] = useState(0);
  const [hoverIdx, setHoverIdx] = useState(null);
  const H = height || 110;
  const padL = 8, padR = 8, padT = 8, padB = 16;

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const cw = entries[0]?.contentRect?.width;
      if (cw) setW(cw);
    });
    ro.observe(el);
    setW(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const plotW = Math.max(0, w - padL - padR);
  const plotH = H - padT - padB;

  const computed = useMemo(() => {
    if (!data || data.length < 1 || plotW <= 0) return null;
    const mx = Math.max(...data, 1);
    const step = data.length > 1 ? plotW / (data.length - 1) : plotW;
    const pts = data.map((v, i) => ({
      x: padL + i * step,
      y: padT + (1 - v / mx) * plotH,
      val: v,
    }));
    const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
    const fill = pts.length > 0
      ? `${line} L${pts[pts.length - 1].x.toFixed(1)},${(padT + plotH).toFixed(1)} L${pts[0].x.toFixed(1)},${(padT + plotH).toFixed(1)} Z`
      : '';
    return { pts, line, fill, mx, step };
  }, [data, plotW, plotH, padL, padT]);

  const onMove = (e) => {
    if (!computed) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    let bestIdx = 0, bestDist = Infinity;
    computed.pts.forEach((p, i) => {
      const d = Math.abs(p.x - mx);
      if (d < bestDist) { bestDist = d; bestIdx = i; }
    });
    setHoverIdx(bestIdx);
  };

  if (!computed) {
    return <div className="avail-chart-empty" ref={wrapRef} style={{height: H}}><span>—</span></div>;
  }

  return (
    <div className="tchart-wrap" ref={wrapRef}>
      <svg
        width={w || '100%'} height={H} className="avail-chart-svg"
        onMouseMove={onMove}
        onMouseLeave={() => setHoverIdx(null)}
      >
        <defs>
          <linearGradient id={`tc-grad-${label}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.22" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>

        {[0, 0.5, 1].map((f) => {
          const y = padT + f * plotH;
          return <line key={f} x1={padL} y1={y} x2={padL + plotW} y2={y}
            stroke="var(--border-light)" strokeWidth="1" strokeDasharray={f === 0 || f === 1 ? '0' : '3 3'} />;
        })}

        <path d={computed.fill} fill={`url(#tc-grad-${label})`} />
        <path d={computed.line} fill="none" stroke={color} strokeWidth="2"
          strokeLinecap="round" strokeLinejoin="round" />

        {computed.pts.map((p, i) => (
          (i === computed.pts.length - 1 || i === hoverIdx) && (
            <circle key={i} cx={p.x} cy={p.y} r={i === hoverIdx ? 3.5 : 2.6}
              fill="var(--bg-card)" stroke={color} strokeWidth="1.8" />
          )
        ))}

        {hoverIdx !== null && computed.pts[hoverIdx] && (
          <line x1={computed.pts[hoverIdx].x} y1={padT}
            x2={computed.pts[hoverIdx].x} y2={padT + plotH}
            stroke="var(--text-tertiary)" strokeWidth="1" opacity="0.5" />
        )}
      </svg>

      {hoverIdx !== null && computed.pts[hoverIdx] && (
        <div className="tchart-tooltip" style={{
          left: Math.min(Math.max(computed.pts[hoverIdx].x, 50), (w || 300) - 50),
        }}>
          <span className="avail-tooltip-row">
            <span className="avail-tooltip-dot" style={{ background: color }} />
            {formatBytes(computed.pts[hoverIdx].val)}/s
          </span>
        </div>
      )}
    </div>
  );
}

export default function MonitorPage({ status, showToast }) {
  const { t } = useT();
  const [liveHistory, setLiveHistory] = useState({ dl: [], ul: [], maxLen: 60 });
  const [devices, setDevices] = useState([]);
  const [connections, setConnections] = useState([]);

  const m = status?.monitor || {};

  const loadData = useCallback(async () => {
    const [devs, conns] = await Promise.all([
      api('GET', '/api/monitor/devices'),
      api('GET', '/api/proxy/connections'),
    ]);

    if (devs?.devices) setDevices(devs.devices);
    if (conns?.connections) setConnections(conns.connections);
  }, []);

  usePolling(loadData, 5000);

  useEffect(() => {
    const dl = m.download_speed || 0;
    const ul = m.upload_speed || 0;

    setLiveHistory(prev => {
      const newDl = [...prev.dl, dl];
      const newUl = [...prev.ul, ul];
      if (newDl.length > prev.maxLen) newDl.shift();
      if (newUl.length > prev.maxLen) newUl.shift();
      return { ...prev, dl: newDl, ul: newUl };
    });
  }, [m.download_speed, m.upload_speed]);

  const totalConns = connections.length;
  const totalDown = connections.reduce((s, c) => s + (c.bytes_recv || 0), 0);
  const totalUp = connections.reduce((s, c) => s + (c.bytes_sent || 0), 0);

  return (
    <>
      <div className="mon-grid">
        <div className="card-section" style={{ gap: 4 }}>
          <div className="mon-chart-header">
            <span className="card-section-title">{t('monitor.download')}</span>
            <span className="mon-chart-val mono mon-chart-dl">{m.dl_speed_fmt || '0 B/s'}</span>
          </div>
          <TrafficChart data={liveHistory.dl} color="var(--accent)" label="dl" height={110} />
        </div>

        <div className="card-section" style={{ gap: 4 }}>
          <div className="mon-chart-header">
            <span className="card-section-title">{t('monitor.upload')}</span>
            <span className="mon-chart-val mono mon-chart-ul">{m.ul_speed_fmt || '0 B/s'}</span>
          </div>
          <TrafficChart data={liveHistory.ul} color="var(--success)" label="ul" height={110} />
        </div>
      </div>

      <div className="mon-stats-row">
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.totalDown')}</span>
          <span className="mon-stat-val mono mon-stat-dl">{m.download_fmt || '0 B'}</span>
        </div>
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.totalUp')}</span>
          <span className="mon-stat-val mono mon-stat-ul">{m.upload_fmt || '0 B'}</span>
        </div>
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.viaProxy')}</span>
          <div className="mon-stat-split">
            <div className="mon-stat-split-item">
              <span className="mon-stat-split-tag dl">DL</span>
              <span className="mono">{formatBytes(totalDown)}</span>
            </div>
            <div className="mon-stat-split-item">
              <span className="mon-stat-split-tag ul">UP</span>
              <span className="mono">{formatBytes(totalUp)}</span>
            </div>
          </div>
        </div>
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.devices')}</span>
          <span className="mon-stat-val">{devices.length}</span>
        </div>
      </div>

      <div className="section mon-devices-section">
        <div className="section-title">{t('monitor.devices')}</div>
        {devices.length > 0 ? (
          <div className="mon-devices">
            {devices.slice(0, 8).map(d => (
              <div key={d.ip} className="mon-device">
                <span className="mon-device-dot" />
                <span className="mon-device-name">{d.hostname || d.ip}</span>
                <span className="mon-device-ip">{d.ip}</span>
                <span className="mon-device-conns">{d.connections || 0}</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="proxy-empty">{t('monitor.noDevices')}</div>
        )}
      </div>
    </>
  );
}
