import { useState, useEffect, useCallback, useMemo } from 'react';
import { api } from '../api';
import { useT } from '../i18n';

export default function DashboardPage({ status, showToast, onNavigate }) {
  const { t } = useT();
  const [resources, setResources] = useState(null);
  const [loading, setLoading] = useState(true);
  const [lastCheck, setLastCheck] = useState(null);       // Date — когда реально пришли данные
  const [lastCheckUnix, setLastCheckUnix] = useState(null); // unix с бэка (точное время проверки)
  const [rechecking, setRechecking] = useState(false);
  const [cached, setCached] = useState(false);
  const [availStd, setAvailStd] = useState([]);
  const [availUser, setAvailUser] = useState([]);
  const [now, setNow] = useState(Date.now()); // тикает каждую секунду для "N сек назад"

  // Тикалка "N сек назад" — обновляет now() раз в секунду, чтобы пользователь
  // видел как меняется "обновлено 5 сек назад" → "обновлено 6 сек назад".
  useEffect(() => {
    const iv = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(iv);
  }, []);

  const fetchResources = useCallback(async (force = false) => {
    const path = force ? '/api/resource-status/refresh' : '/api/resource-status';
    const data = await api('GET', path);
    if (data) {
      setResources(data);
      setCached(!!data.cached);
      // Используем unix-время с бэка если есть, иначе локальное
      if (data.checked_at_unix) {
        setLastCheckUnix(data.checked_at_unix * 1000);
        setLastCheck(new Date(data.checked_at_unix * 1000));
      } else {
        setLastCheck(new Date());
        setLastCheckUnix(Date.now());
      }
    }
    setLoading(false);
    return data;
  }, []);

  const fetchAvailHistory = useCallback(async () => {
    const [std, usr] = await Promise.all([
      api('GET', '/api/availability/history?hours=1&type=standard'),
      api('GET', '/api/availability/history?hours=1&type=user'),
    ]);
    if (std?.records) setAvailStd(std.records);
    if (usr?.records) setAvailUser(usr.records);
  }, []);

  useEffect(() => {
    fetchResources();
    fetchAvailHistory();
    const iv = setInterval(() => fetchResources(false), 10000);
    return () => clearInterval(iv);
  }, [fetchResources, fetchAvailHistory]);

  // Ручная проверка: обязательно force=true, обходит кэш.
  // Не даём нажать повторно пока идёт проверка.
  const handleRecheck = async () => {
    if (rechecking) return;
    setRechecking(true);
    try {
      const data = await fetchResources(true);
      if (!data) {
        showToast?.('Бэкенд недоступен', 'error');
      } else if (data.error) {
        showToast?.(data.error, 'error');
      }
    } finally {
      setRechecking(false);
    }
  };

  const fmtTime = (d) => {
    if (!d) return '—';
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  };

  // "обновлено 5 сек назад" / "обновлено 2 мин назад"
  const fmtAgo = (unixMs) => {
    if (!unixMs) return '—';
    const sec = Math.max(0, Math.floor((now - unixMs) / 1000));
    if (sec < 5) return 'только что';
    if (sec < 60) return `${sec} сек назад`;
    const min = Math.floor(sec / 60);
    if (min < 60) return `${min} мин назад`;
    const hr = Math.floor(min / 60);
    return `${hr} ч назад`;
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

  const blockedRes = allRes.filter(r => !r.ok);
  const allOk = blockedRes.length === 0 && allRes.length > 0;

  return (
    <div className="db-page">
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
            {rechecking ? (
              <span className="db-res-status checking">
                <span className="db-res-spin" />
                {t('dashboard.checking') || 'Проверка...'}
              </span>
            ) : (
              <span className="db-res-status" data-tooltip={lastCheck ? fmtTime(lastCheck) : ''}>
                {allOk
                  ? (t('dashboard.upToDate') || 'Всё доступно')
                  : cached
                    ? <>обновлено {fmtAgo(lastCheckUnix)}{cached ? ' · кэш' : ''}</>
                    : <>обновлено {fmtAgo(lastCheckUnix)}</>}
              </span>
            )}
            <button
              className={'db-res-recheck' + (rechecking ? ' spinning disabled' : '')}
              onClick={handleRecheck}
              disabled={rechecking}
              data-tooltip={rechecking ? 'Идёт проверка...' : 'Проверить сейчас'}
              aria-label="Проверить ресурсы"
            >
              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 2v6h-6"/><path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M3 22v-6h6"/><path d="M21 12a9 9 0 0 1-15 6.7L3 16"/></svg>
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
        </div>
        <div className="db-avail-chart">
          <DualSpark series={[
            { records: availStd, color: 'var(--accent)' },
            { records: availUser, color: 'var(--success)' },
          ]} />
        </div>
      </div>

      <div className="card-section db-fail-panel">
        <div className="db-fail-head">
          <span className="card-section-title" style={{ color: blockedRes.length > 0 ? 'var(--danger)' : 'var(--text-tertiary)' }}>
            {t('dashboard.unavailableResources')} ({blockedRes.length})
          </span>
        </div>
        {blockedRes.length > 0 && !loading ? (
          <div className="db-fail-scroll">
            <div className="db-fail-grid">
              {blockedRes.map((r, i) => (
                <span key={i} className="db-fail-tag" data-tooltip={r.reason || r.verdict}>{r.name}</span>
              ))}
            </div>
          </div>
        ) : (
          <div className="db-fail-empty">{t('dashboard.noProblems')}</div>
        )}
      </div>
    </div>
  );
}

// DualSpark — компактный двойной график доступности с метками времени.
// records: [{ timestamp, pct }] (из database.AvailabilityRecord)
function DualSpark({ series }) {
  const data = useMemo(() => {
    const W = 100, H = 40;
    const valid = series.filter(s => s.records && s.records.length >= 2);
    if (valid.length === 0) return null;

    // Найдём общий диапазон времени
    let minT = Infinity, maxT = -Infinity;
    valid.forEach(s => s.records.forEach(r => {
      const ts = r.timestamp instanceof Date ? r.timestamp.getTime() : new Date(r.timestamp).getTime();
      if (!isNaN(ts)) {
        if (ts < minT) minT = ts;
        if (ts > maxT) maxT = ts;
      }
    }));
    if (!isFinite(minT) || !isFinite(maxT) || maxT === minT) {
      // Fallback на индексы если нет timestamps
      minT = 0; maxT = 1;
      valid.forEach(s => s.records.forEach((_, i) => { if (i > maxT) maxT = i; }));
    }

    const lines = valid.map(s => {
      const pts = s.records.map(r => {
        const ts = r.timestamp instanceof Date ? r.timestamp.getTime() : new Date(r.timestamp).getTime();
        const x = isFinite(ts) && maxT > minT
          ? ((ts - minT) / (maxT - minT)) * W
          : 0;
        const y = H - (r.pct / 100) * (H - 4) - 2;
        return [x, y, ts];
      });
      const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
      const fill = `${line} L${W},${H} L0,${H} Z`;
      return { line, fill, color: s.color, pts };
    });

    // Метки времени: 4-5 делений по оси X
    const ticks = [];
    const nTicks = 4;
    for (let i = 0; i <= nTicks; i++) {
      const t = minT + (maxT - minT) * (i / nTicks);
      const x = (i / nTicks) * W;
      const d = new Date(t);
      const label = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      ticks.push({ x, label });
    }

    return { lines, W, H, ticks };
  }, [series]);

  if (!data) return <div className="mini-spark-empty" />;

  return (
    <div className="db-spark-wrap">
      <svg className="mini-spark-svg" viewBox={`0 0 ${data.W} ${data.H + 8}`} preserveAspectRatio="none">
        {data.lines.map((l, i) => (
          <path key={`f${i}`} d={l.fill} fill={l.color} fillOpacity="0.06" />
        ))}
        {data.lines.map((l, i) => (
          <path key={`l${i}`} d={l.line} fill="none" stroke={l.color} strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinecap="round" strokeLinejoin="round" />
        ))}
      </svg>
      <div className="db-spark-ticks">
        {data.ticks.map((t, i) => (
          <span key={i} className="db-spark-tick" style={{ left: `${t.x}%` }}>{t.label}</span>
        ))}
      </div>
    </div>
  );
}
