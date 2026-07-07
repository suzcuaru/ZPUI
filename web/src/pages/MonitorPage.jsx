import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../api';
import { formatBytes } from '../utils';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import { ArrowDown, ArrowUp } from 'lucide-react';

function TrafficChart({ data, color, height }) {
  const canvasRef = useRef(null);
  const h = height || 64;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const cw = canvas.offsetWidth;
    const ch = h;
    canvas.width = cw * dpr;
    canvas.height = ch * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    ctx.clearRect(0, 0, cw, ch);

    const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
    const gridColor = isDark ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.04)';
    for (let i = 0; i < 4; i++) {
      const y = (ch / 4) * i;
      ctx.strokeStyle = gridColor;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(cw, y);
      ctx.stroke();
    }

    if (!data || data.length < 2) return;

    const mx = Math.max(...data, 1);
    const step = cw / (data.length - 1);

    const cMain = color === 'accent'
      ? (isDark ? '#6ba3ff' : '#4f8ff7')
      : (isDark ? '#34d058' : '#28a745');
    const cFill = color === 'accent'
      ? (isDark ? 'rgba(107,163,255,0.2)' : 'rgba(79,143,247,0.15)')
      : (isDark ? 'rgba(52,208,88,0.2)' : 'rgba(40,167,69,0.15)');

    ctx.beginPath();
    ctx.moveTo(0, ch);
    data.forEach((v, i) => {
      const x = i * step;
      const y = ch - (v / mx) * (ch - 6) - 3;
      ctx.lineTo(x, y);
    });
    ctx.lineTo(cw, ch);
    ctx.closePath();
    ctx.fillStyle = cFill;
    ctx.fill();

    ctx.beginPath();
    data.forEach((v, i) => {
      const x = i * step;
      const y = ch - (v / mx) * (ch - 6) - 3;
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.strokeStyle = cMain;
    ctx.lineWidth = 1.5;
    ctx.stroke();

    const lastVal = data[data.length - 1];
    const lastX = (data.length - 1) * step;
    const lastY = ch - (lastVal / mx) * (ch - 6) - 3;
    ctx.beginPath();
    ctx.arc(lastX, lastY, 2.5, 0, Math.PI * 2);
    ctx.fillStyle = cMain;
    ctx.fill();
  }, [data, color, h]);

  return <canvas ref={canvasRef} style={{ width: '100%', height: h, display: 'block' }} />;
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
      <div className="page-title">{t('monitor.title')}</div>

      <div className="mon-grid">
        <div className="card-section" style={{ gap: 4 }}>
          <div className="mon-chart-header">
            <span className="card-section-title">{t('monitor.download')}</span>
            <span className="mon-chart-val mono" style={{ color: 'var(--accent)' }}>{m.dl_speed_fmt || '0 B/s'}</span>
          </div>
          <TrafficChart data={liveHistory.dl} color="accent" height={64} />
        </div>

        <div className="card-section" style={{ gap: 4 }}>
          <div className="mon-chart-header">
            <span className="card-section-title">{t('monitor.upload')}</span>
            <span className="mon-chart-val mono" style={{ color: 'var(--success)' }}>{m.ul_speed_fmt || '0 B/s'}</span>
          </div>
          <TrafficChart data={liveHistory.ul} color="success" height={64} />
        </div>
      </div>

      <div className="mon-stats-row">
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.totalDown')}</span>
          <span className="mon-stat-val mono">{m.download_fmt || '0 B'}</span>
        </div>
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.totalUp')}</span>
          <span className="mon-stat-val mono">{m.upload_fmt || '0 B'}</span>
        </div>
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.viaProxy')}</span>
          <span className="mon-stat-val mono"><ArrowDown size={12} strokeWidth={2.5} />{formatBytes(totalDown)} <ArrowUp size={12} strokeWidth={2.5} />{formatBytes(totalUp)}</span>
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
