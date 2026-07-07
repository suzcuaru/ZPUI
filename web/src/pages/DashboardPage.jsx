import { useState, useEffect, useCallback, useMemo } from 'react';
import { api } from '../api';
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
  const allRes = [...defRes, ...userRes];

  const defOk = defRes.filter(r => r.ok).length;
  const defPct = defRes.length > 0 ? Math.round(defOk / defRes.length * 100) : -1;
  const userOk = userRes.filter(r => r.ok).length;
  const userPct = userRes.length > 0 ? Math.round(userOk / userRes.length * 100) : -1;

  const bypassedRes = allRes.filter(r => r.bypassed);
  const blockedRes = allRes.filter(r => r.blocked && !r.bypassed);
  const allOk = blockedRes.length === 0;

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
        <div className="db-avail-stats">
          <div className="db-avail-stat">
            <span className="db-avail-pct" style={{ color: defPct >= 80 ? 'var(--success)' : defPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {defPct >= 0 ? `${defPct}%` : '—'}
            </span>
            <span className="db-avail-label"><span className="db-avail-dot" style={{ background: 'var(--accent)' }} />{t('dashboard.standard')}</span>
            <span className="db-avail-count">{defOk}/{defRes.length}</span>
          </div>
          <div className="db-avail-stat">
            <span className="db-avail-pct" style={{ color: userPct >= 80 ? 'var(--success)' : userPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {userPct >= 0 ? `${userPct}%` : '—'}
            </span>
            <span className="db-avail-label"><span className="db-avail-dot" style={{ background: 'var(--success)' }} />{t('dashboard.custom')}</span>
            <span className="db-avail-count">{userOk}/{userRes.length}</span>
          </div>
          {(bypassedRes.length > 0 || blockedRes.length > 0) && (
            <div className="db-avail-stat">
              <span className="db-avail-pct" style={{ fontSize: 16, color: bypassedRes.length > 0 ? 'var(--success)' : 'var(--danger)' }}>
                {bypassedRes.length}/{bypassedRes.length + blockedRes.length}
              </span>
              <span className="db-avail-label">обход</span>
              <span className="db-avail-count" style={{ color: blockedRes.length > 0 ? 'var(--danger)' : 'var(--success)' }}>
                {blockedRes.length > 0 ? `${blockedRes.length} не работают` : 'все работают'}
              </span>
            </div>
          )}
        </div>
        <div className="db-avail-chart">
          <DualSpark series={[
            { records: availStd, color: 'var(--accent)' },
            { records: availUser, color: 'var(--success)' },
          ]} />
        </div>
      </div>

      {bypassedRes.length > 0 && !loading && (
        <div className="card-section" style={{ gap: 6 }}>
          <span className="card-section-title" style={{ fontSize: 11, color: 'var(--success)' }}>
            Обход работает ({bypassedRes.length})
          </span>
          <div className="db-fail-grid">
            {bypassedRes.map((r, i) => (
              <span key={i} className="db-fail-tag" style={{ color: 'var(--success)' }}>{r.name}</span>
            ))}
          </div>
        </div>
      )}

      {blockedRes.length > 0 && !loading && (
        <div className="card-section" style={{ gap: 6 }}>
          <span className="card-section-title" style={{ fontSize: 11, color: 'var(--danger)' }}>
            {t('dashboard.unavailableResources')} ({blockedRes.length})
          </span>
          <div className="db-fail-grid">
            {blockedRes.map((r, i) => (
              <span key={i} className="db-fail-tag">{r.name}</span>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

function DualSpark({ series }) {
  const data = useMemo(() => {
    const W = 100, H = 40;
    const valid = series.filter(s => s.records && s.records.length >= 2);
    if (valid.length === 0) return null;

    const lines = valid.map(s => {
      const pts = s.records.map((r, i) => {
        const x = (i / (s.records.length - 1)) * W;
        const y = H - (r.pct / 100) * (H - 4) - 2;
        return [x, y];
      });
      const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
      const fill = `${line} L${W},${H} L0,${H} Z`;
      return { line, fill, color: s.color };
    });

    return { lines, W, H };
  }, [series]);

  if (!data) return <div className="mini-spark-empty" />;

  return (
    <svg className="mini-spark-svg" viewBox={`0 0 ${data.W} ${data.H}`} preserveAspectRatio="none">
      {data.lines.map((l, i) => (
        <path key={`f${i}`} d={l.fill} fill={l.color} fillOpacity="0.06" />
      ))}
      {data.lines.map((l, i) => (
        <path key={`l${i}`} d={l.line} fill="none" stroke={l.color} strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinecap="round" strokeLinejoin="round" />
      ))}
    </svg>
  );
}
