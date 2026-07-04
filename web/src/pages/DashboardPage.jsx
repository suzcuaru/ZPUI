import { useState, useEffect, useCallback, useMemo } from 'react';
import { api } from '../api';
import { formatBytes } from '../utils';
import { useT } from '../i18n';

export default function DashboardPage({ status, showToast, onNavigate }) {
  const { t } = useT();
  const [resources, setResources] = useState(null);
  const [loading, setLoading] = useState(true);
  const [lastCheck, setLastCheck] = useState(null);
  const [rechecking, setRechecking] = useState(false);
  const [availStd, setAvailStd] = useState([]);
  const [availUser, setAvailUser] = useState([]);

  const fetchResources = useCallback(async () => {
    const data = await api('GET', '/api/resource-status');
    if (data) setResources(data);
    setLastCheck(new Date());
    setLoading(false);
  }, []);

  const fetchAvailHistory = useCallback(async () => {
    const [std, usr] = await Promise.all([
      api('GET', '/api/availability/history?hours=24&type=standard'),
      api('GET', '/api/availability/history?hours=24&type=user'),
    ]);
    if (std?.records) setAvailStd(std.records);
    if (usr?.records) setAvailUser(usr.records);
  }, []);

  useEffect(() => {
    fetchResources();
    fetchAvailHistory();
    const iv = setInterval(fetchResources, 10000);
    return () => clearInterval(iv);
  }, [fetchResources, fetchAvailHistory]);

  const handleRecheck = async () => {
    setRechecking(true);
    await fetchResources();
    setRechecking(false);
  };

  const fmtTime = (d) => {
    if (!d) return '—';
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  };

  const zRun = status?.zapret?.status === 'running';
  const pRun = status?.proxy?.running === true;
  const xRun = status?.xbox_dns?.enabled === true;

  const defRes = resources?.default || [];
  const userRes = resources?.user || [];
  const defOk = defRes.filter(r => r.ok).length;
  const defPct = defRes.length > 0 ? Math.round(defOk / defRes.length * 100) : -1;
  const userOk = userRes.filter(r => r.ok).length;
  const userPct = userRes.length > 0 ? Math.round(userOk / userRes.length * 100) : -1;

  const defFails = defRes.filter(r => !r.ok);
  const userFails = userRes.filter(r => !r.ok);
  const fails = [...defFails, ...userFails];
  const allOk = fails.length === 0;

  const mon = status?.monitor || {};
  const dlSpeed = mon.dl_speed_fmt || '0 B/s';
  const ulSpeed = mon.ul_speed_fmt || '0 B/s';
  const dlTotal = mon.download_fmt || formatBytes(mon.download_bytes || 0);
  const ulTotal = mon.upload_fmt || formatBytes(mon.upload_bytes || 0);

  return (
    <>
      <div className="page-title">{t('dashboard.title')}</div>
      <div className="page-grid-3">
        <div className={'svc-card' + (zRun ? ' on' : '')} onClick={() => onNavigate('zapret')}>
          <div className="svc-card-icon">
            <svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 1L3 5v6c0 5.5 3.8 10.7 9 12 5.2-1.3 9-6.5 9-12V5l-9-4zm-1 14.5L7.5 12 9 10.5l2 2 4-4L16.5 10l-5.5 5.5z"/></svg>
          </div>
          <div className="svc-card-body">
            <span className="svc-card-name">{t('nav.zapret')}</span>
            <span className={'svc-card-state ' + (zRun ? 'on' : 'off')}>{zRun ? t('status.running') : t('status.stopped')}</span>
            <span className="svc-card-sub mono">{status?.zapret?.strategy?.replace('.bat', '') || '—'}</span>
          </div>
          <span className={'svc-card-dot ' + (zRun ? 'on' : 'off')} />
        </div>

        <div className={'svc-card' + (pRun ? ' on' : '')} onClick={() => onNavigate('proxy')}>
          <div className="svc-card-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 010 20M12 2a15 15 0 000 20"/></svg>
          </div>
          <div className="svc-card-body">
            <span className="svc-card-name">{t('nav.proxy')}</span>
            <span className={'svc-card-state ' + (pRun ? 'on' : 'off')}>{pRun ? t('status.running') : t('status.stopped')}</span>
            <span className="svc-card-sub mono">{pRun ? ':' + (status?.proxy?.port || '') : '—'}</span>
          </div>
          <span className={'svc-card-dot ' + (pRun ? 'on' : 'off')} />
        </div>

        <div className={'svc-card' + (xRun ? ' on' : '')} onClick={() => onNavigate('xboxdns')}>
          <div className="svc-card-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="2" y="3" width="20" height="6" rx="2"/><rect x="2" y="15" width="20" height="6" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
          </div>
          <div className="svc-card-body">
            <span className="svc-card-name">DNS</span>
            <span className={'svc-card-state ' + (xRun ? 'on' : 'off')}>{xRun ? t('status.running') : t('status.stopped')}</span>
            <span className="svc-card-sub mono">{status?.xbox_dns?.primary_dns || '—'}</span>
          </div>
          <span className={'svc-card-dot ' + (xRun ? 'on' : 'off')} />
        </div>
      </div>

      <div className="card-section">
        <div className="db-res-head">
          <span className="card-section-title">{t('dashboard.resourceAvailability')}</span>
          <div className="db-res-meta">
            <span className="db-res-time">{t('dashboard.lastCheck')}: {fmtTime(lastCheck)}</span>
            <button className={'db-res-recheck' + (rechecking ? ' spinning' : '')} onClick={handleRecheck} disabled={rechecking}>
              {rechecking ? <span className="mini-spin" /> : (
                <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 2v6h-6"/><path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M3 22v-6h6"/><path d="M21 12a9 9 0 0 1-15 6.7L3 16"/></svg>
              )}
            </button>
          </div>
        </div>
        <div className="db-resource-row">
          <div className="db-resource-item">
            <span className="db-resource-pct" style={{ color: defPct >= 80 ? 'var(--success)' : defPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {defPct >= 0 ? `${defPct}%` : '—'}
            </span>
            <span className="db-resource-label">{t('dashboard.standard')}</span>
            <span className="db-resource-sub">{defOk}/{defRes.length}</span>
          </div>
          <div className="db-resource-divider" />
          <div className="db-resource-item">
            <span className="db-resource-pct" style={{ color: userPct >= 80 ? 'var(--success)' : userPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {userPct >= 0 ? `${userPct}%` : '—'}
            </span>
            <span className="db-resource-label">{t('dashboard.custom')}</span>
            <span className="db-resource-sub">{userOk}/{userRes.length}</span>
          </div>
        </div>
        <div className="db-mini-charts">
          <MiniSpark records={availStd} color="var(--accent)" />
          <MiniSpark records={availUser} color="var(--success)" />
        </div>
      </div>

      <div className="card-section">
        <div className="card-section-title">{t('dashboard.traffic')}</div>
        <div className="db-traffic-grid">
          <div className="db-traffic-box">
            <span className="db-traffic-arrow down">↓</span>
            <span className="db-traffic-val">{dlSpeed}</span>
          </div>
          <div className="db-traffic-box">
            <span className="db-traffic-arrow up">↑</span>
            <span className="db-traffic-val">{ulSpeed}</span>
          </div>
        </div>
        <div className="db-traffic-totals">
          <span className="db-total-item">↓ {dlTotal}</span>
          <span className="db-total-item">↑ {ulTotal}</span>
        </div>
      </div>

      {!allOk && !loading && (
        <div className="card-section" style={{ gap: 6 }}>
          <div className="db-fail-grid">
            {fails.slice(0, 20).map((r, i) => (
              <span key={i} className="db-fail-tag">{r.name}</span>
            ))}
          </div>
          {fails.length > 20 && <span className="db-fail-more">+{fails.length - 20}</span>}
        </div>
      )}
    </>
  );
}

function MiniSpark({ records, color }) {
  const path = useMemo(() => {
    if (records.length < 2) return null;
    const w = 140, h = 28;
    const pts = records.map((r, i) => {
      const x = (i / (records.length - 1)) * w;
      const y = h - (r.pct / 100) * (h - 4) - 2;
      return `${x},${y}`;
    });
    return { line: pts.join(' '), fill: `M0,${h} L${pts.join(' L')} L${w},${h} Z` };
  }, [records]);

  if (!path) return <div className="mini-spark-wrap" />;
  return (
    <div className="mini-spark-wrap">
      <svg width="140" height="28" viewBox="0 0 140 28" className="mini-spark">
        <path d={path.fill} fill={color} fillOpacity="0.1" />
        <polyline points={path.line} fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </div>
  );
}
