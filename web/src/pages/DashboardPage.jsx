import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { formatBytes } from '../utils';
import { useT } from '../i18n';

export default function DashboardPage({ status, showToast, onNavigate }) {
  const { t } = useT();
  const [resources, setResources] = useState(null);
  const [loading, setLoading] = useState(true);

  const fetchResources = useCallback(async () => {
    const data = await api('GET', '/api/resource-status');
    if (data) setResources(data);
    setLoading(false);
  }, []);

  useEffect(() => {
    fetchResources();
    const iv = setInterval(fetchResources, 10000);
    return () => clearInterval(iv);
  }, [fetchResources]);

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
            <span className={'svc-card-state ' + (pRun ? 'on' : 'off')}>{pRun ? ':' + (status?.proxy?.port || '') : t('status.stopped')}</span>
            <span className="svc-card-sub">{pRun ? `${status?.proxy?.devices || 0} ${t('proxy.devicesSuffix')}` : '—'}</span>
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
        <div className="card-section-title">{t('dashboard.resourceAvailability')}</div>
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
          {fails.slice(0, 20).map((r, i) => (
            <span key={i} className="db-fail-tag">{r.name}</span>
          ))}
          {fails.length > 20 && <span className="db-fail-more">+{fails.length - 20}</span>}
        </div>
      )}
    </>
  );
}
