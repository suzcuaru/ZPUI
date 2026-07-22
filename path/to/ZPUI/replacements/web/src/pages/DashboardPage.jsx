import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { api } from '../api';
import { useT } from '../i18n';
import { AlertTriangle } from 'lucide-react';

export default function DashboardPage({ status, showToast, onNavigate, onOpenCheckerWithUrl }) {
  const { t } = useT();
  const [resources, setResources] = useState(null);
  const [loading, setLoading] = useState(true);
  const [lastCheckUnix, setLastCheckUnix] = useState(null);
  const [checkingNow, setCheckingNow] = useState(false);
  const [cached, setCached] = useState(false);
  const [availStd, setAvailStd] = useState([]);
  const [availUser, setAvailUser] = useState([]);
  const [now, setNow] = useState(Date.now());
  const [checkIntervalMin, setCheckIntervalMin] = useState(10);
  const [failAction, setFailAction] = useState(null); // { name, reason }

  // "обновлено N сек назад" — тикает каждую секунду
  useEffect(() => {
    const iv = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(iv);
  }, []);

  // Опрос статуса ресурсов: каждые 30 сек обычно, 2 сек во время проверки
  const fetchResources = useCallback(async () => {
    const data = await api('GET', '/api/resource-status');
    if (data) {
      setResources(data);
      setCached(!!data.cached);
      setCheckingNow(!!data.checking_now);
      if (data.checked_at_unix) {
        setLastCheckUnix(data.checked_at_unix * 1000);
      }
      if (data.resource_check_interval) {
        setCheckIntervalMin(data.resource_check_interval);
      }
    }
    setLoading(false);
    return data;
  }, []);

  // Опрос истории из БД: каждые 60 сек (окно 2 часа)
  const fetchAvailHistory = useCallback(async () => {
    const [std, usr] = await Promise.all([
      api('GET', '/api/availability/history?hours=2&type=standard'),
      api('GET', '/api/availability/history?hours=2&type=user'),
    ]);
    if (std?.records) setAvailStd(std.records);
    if (usr?.records) setAvailUser(usr.records);
  }, []);

  // Авто-опрос: 30 сек обычно, 2 сек когда идёт проверка
  useEffect(() => {
    fetchResources();
    fetchAvailHistory();
    let ivStatus, ivHistory;
    const setupTimers = () => {
      clearInterval(ivStatus);
      clearInterval(ivHistory);
      const statusInterval = checkingNow ? 2000 : 30000;
      ivStatus = setInterval(fetchResources, statusInterval);
      ivHistory = setInterval(fetchAvailHistory, 60000);
    };
    setupTimers();
    return () => { clearInterval(ivStatus); clearInterval(ivHistory); };
  }, [fetchResources, fetchAvailHistory, checkingNow]);

  // Когда проверка завершена (checked_at_unix изменился) — сразу обновляем график
  const prevCheckedAtRef = useRef(null);
  useEffect(() => {
    if (lastCheckUnix !== null) {
      if (prevCheckedAtRef.current !== null && prevCheckedAtRef.current !== lastCheckUnix) {
        fetchAvailHistory();
      }
      prevCheckedAtRef.current = lastCheckUnix;
    }
  }, [lastCheckUnix, fetchAvailHistory]);

  // Ручная проверка: запускает async-проверку на бэке, кнопка блокируется
  // пока checking_now=true (фронтенд опрашивает каждые 2 сек)
  const handleRecheck = async () => {
    if (checkingNow) return;
    setCheckingNow(true);  // сразу блокируем кнопку
    try {
      const data = await api('GET', '/api/resource-status/refresh');
      if (data?.error) {
        showToast?.(data.error, 'error');
      }
      // Не ждём завершения — useEffect с checkingNow будет опрашивать каждые 2с
      // и сам разблокирует кнопку когда checking_now станет false
    } catch {
      showToast?.('Не удалось запустить проверку', 'error');
      setCheckingNow(false);  // FIX
    }
  };

  const fmtAgo = (unixMs) => {
    if (!unixMs) return '—';
    const sec = Math.max(0, Math.floor((now - unixMs) / 1000));
    if (sec < 5) return t('dashboard.justNow');
    if (sec < 60) return t('dashboard.secAgo', { n: sec });
    const min = Math.floor(sec / 60);
    if (min < 60) return t('dashboard.minAgo', { n: min });
    return t('dashboard.hourAgo', { n: Math.floor(min / 60) });
  };

  // Отсчёт до следующей автоматической проверки (следующая 10-мин граница по часам)
  const intervalMs = checkIntervalMin * 60 * 1000;
  const nextCheckMs = Math.ceil(now / intervalMs) * intervalMs;
  const nextInMs = Math.max(0, nextCheckMs - now);
  const nextIn =
    nextInMs >= 60000
      ? `${Math.floor(nextInMs / 60000)}:${String(Math.floor((nextInMs % 60000) / 1000)).padStart(2, '0')}`
      : `${Math.floor(nextInMs / 1000)}${t('dashboard.secShort')}`;

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

  const handleSkipResource = async (host) => {
    const r = await api('POST', '/api/zapret/skip-resources/add', { host });
    if (r?.status === 'ok') {
      showToast?.(t('dashboard.addedToSkip', { host }), 'success');
    } else if (r?.error) {
      showToast?.(r.error, 'error');
    }
    setFailAction(null);
  };

  const handleCheckResource = (name) => {
    const url = name.startsWith('http') ? name : `https://${name}`;
    setFailAction(null);
    onOpenCheckerWithUrl?.(url);
  };

  return (
    <div className="db-page">
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

      {/* === Availability panel — redesigned === */}
      <div className="card-section avail-panel">
        <div className="avail-head">
          <div className="avail-title-group">
            <span className="card-section-title">{t('dashboard.resourceAvailability')}</span>
            <span className={'avail-status ' + (checkingNow ? 'busy' : 'idle')}
              data-tooltip={checkingNow ? '' : (lastCheckUnix ? `${t('dashboard.lastCheckedAt')} ${fmtAgo(lastCheckUnix)}${cached ? ' · ' + t('dashboard.cache') : ''}` : '')}
            >
              {checkingNow ? (
                <><span className="avail-pulse" />{t('dashboard.checking')}</>
              ) : (
                <><span className="avail-live-dot" />{t('dashboard.nextCheck')} <span className="mono avail-status-time">{nextIn}</span></>
              )}
            </span>
          </div>
          <button
            className={'avail-recheck' + (checkingNow ? ' busy' : '')}
            onClick={handleRecheck}
            disabled={checkingNow}
            data-tooltip={checkingNow ? t('dashboard.checkingHint') : t('dashboard.checkNow')}
            data-tooltip-pos="top"
            aria-label={t('dashboard.checkNow')}
          >
            <svg className="avail-recheck-icon" viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 2v6h-6"/><path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M3 22v-6h6"/><path d="M21 12a9 9 0 0 1-15 6.7L3 16"/>
            </svg>
          </button>
        </div>

        {/* Chart */}
        <div className="avail-chart-wrap">
          <AvailChart
            series={[
              { records: availStd, color: 'var(--accent)', label: t('dashboard.standard') },
              { records: availUser, color: 'var(--success)', label: t('dashboard.custom') },
            ]}
            now={now}
            intervalMin={checkIntervalMin}
            t={t}
          />
        </div>

        {/* Legend + current stats */}
        <div className="avail-legend">
          <div className="avail-chip">
            <span className="avail-chip-dot" style={{ background: 'var(--accent)' }} />
            <span className="avail-chip-label">{t('dashboard.standard')}</span>
            <span className="avail-chip-val" style={{ color: defPct >= 80 ? 'var(--success)' : defPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {defPct >= 0 ? `${defPct}%` : '—'}
            </span>
            <span className="avail-chip-count mono">{defOk}/{defRes.length}</span>
          </div>
          <div className="avail-chip">
            <span className="avail-chip-dot" style={{ background: 'var(--success)' }} />
            <span className="avail-chip-label">{t('dashboard.custom')}</span>
            <span className="avail-chip-val" style={{ color: userPct >= 80 ? 'var(--success)' : userPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {userPct >= 0 ? `${userPct}%` : '—'}
            </span>
            <span className="avail-chip-count mono">{userOk}/{userRes.length}</span>
          </div>
        </div>
      </div>

      {/* Unavailable resources */}
      <div className="card-section avail-fail-panel">
        <div className="avail-fail-head">
          <span className="card-section-title" style={{ color: blockedRes.length > 0 ? 'var(--danger)' : 'var(--text-tertiary)' }}>
            {t('dashboard.unavailableResources')} ({blockedRes.length})
          </span>
        </div>
        {blockedRes.length > 0 && !loading ? (
          <div className="avail-fail-scroll">
            <div className="avail-fail-grid">
              {blockedRes.map((r, i) => (
                <button key={i} className="avail-fail-tag" data-tooltip={r.reason || r.verdict} onClick={() => setFailAction({ name: r.name, reason: r.reason || r.verdict })}>
                  {r.name}
                </button>
              ))}
            </div>
          </div>
        ) : (
          <div className="avail-fail-empty">{t('dashboard.noProblems')}</div>
        )}
      </div>

      {failAction && (
        <div className="modal-overlay" onClick={() => setFailAction(null)}>
          <div className="modal modal-sm fa-modal" onClick={e => e.stopPropagation()}>
            <div className="fa-modal-icon"><AlertTriangle size={20} strokeWidth={2.5} /></div>
            <div className="fa-modal-title">{failAction.name}</div>
            <div className="fa-modal-reason">{failAction.reason}</div>
            <div className="fa-modal-actions">
              <button className="btn btn-sm btn-ghost" onClick={() => setFailAction(null)}>{t('common.close')}</button>
              <button className="btn btn-sm" onClick={() => handleSkipResource(failAction.name)}>
                {t('dashboard.addToSkip')}
              </button>
              <button className="btn btn-sm btn-accent" onClick={() => handleCheckResource(failAction.name)}>
                {t('dashboard.checkResource')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// AvailChart — график доступности с 10-минутной сеткой по часам.
// Плановые проверки попадают точно на линии сетки, ручные — между ними.
function AvailChart({ series, now, intervalMin, t }) {
  const wrapRef = useRef(null);
  const [w, setW] = useState(0);
  const [hover, setHover] = useState(null); // { x, pts: [{label,color,pct,ts}] }

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

  const H = 132;                  // полная высота SVG
  const padL = 30, padR = 10, padT = 8, padB = 18;
  const plotW = Math.max(0, w - padL - padR);
  const plotH = H - padT - padB;

  const data = useMemo(() => {
    const valid = series.filter((s) => s.records && s.records.length >= 1);
    if (valid.length === 0 || plotW <= 0) return null;

    // Окно: [текущее время - 1ч, следующая 10-мин граница]
    const STEP = (intervalMin || 10) * 60 * 1000;
    const cur = now || Date.now();
    const gridEnd = Math.ceil(cur / STEP) * STEP;
    const gridStart = gridEnd - 2 * 60 * 60 * 1000;
    const span = gridEnd - gridStart;

    const xOf = (ts) => padL + ((ts - gridStart) / span) * plotW;
    const yOf = (pct) => padT + (1 - Math.max(0, Math.min(100, pct)) / 100) * plotH;

    const lines = valid.map((s) => {
      const pts = s.records
        .map((r) => {
          const ts = r.timestamp instanceof Date ? r.timestamp.getTime() : new Date(r.timestamp).getTime();
          if (!isFinite(ts) || ts < gridStart || ts > gridEnd) return null;
          return { x: xOf(ts), y: yOf(r.pct), ts, pct: Math.round(r.pct) };
        })
        .filter(Boolean)
        .sort((a, b) => a.x - b.x);
      const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
      const fill = pts.length > 0 ? `${line} L${pts[pts.length - 1].x.toFixed(1)},${(padT + plotH).toFixed(1)} L${pts[0].x.toFixed(1)},${(padT + plotH).toFixed(1)} Z` : '';
      return { line, fill, color: s.color, label: s.label, pts };
    });

    // Вертикальные линии сетки каждые 10 мин, подписи каждые 30 мин
    const ticks = [];
    for (let tm = gridStart; tm <= gridEnd; tm += STEP) {
      const x = xOf(tm);
      const d = new Date(tm);
      const label = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      ticks.push({ x, label, major: d.getMinutes() === 0 || d.getMinutes() === 30 });
    }

    // «Сейчас»
    const nowX = xOf(cur);

    return { lines, ticks, nowX, gridStart, gridEnd, span };
  }, [series, now, plotW, intervalMin]);

  const onMove = (e) => {
    if (!data) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    // ближайшее время по X
    const { gridStart, span } = data;
    const STEP = (intervalMin || 10) * 60 * 1000;
    let bestTs = null, bestDist = Infinity;
    data.lines.forEach((l) => l.pts.forEach((p) => {
      const d = Math.abs(p.x - mx);
      if (d < bestDist) { bestDist = d; bestTs = p.ts; }
    }));
    if (bestTs === null) { setHover(null); return; }
    const hx = Math.max(padL, Math.min(padL + plotW, data.lines[0] ? (padL + ((bestTs - gridStart) / span) * plotW) : mx));
    const pts = data.lines
      .map((l) => {
        let near = null, nd = Infinity;
        l.pts.forEach((p) => { const d = Math.abs(p.ts - bestTs); if (d < nd) { nd = d; near = p; } });
        return near ? { label: l.label, color: l.color, pct: near.pct, ts: near.ts } : null;
      })
      .filter(Boolean);
    setHover({ x: hx, pts });
  };

  if (!data) {
    return (
      <div className="avail-chart-empty" ref={wrapRef}>
        <span>{t ? t('dashboard.noChartData') : '—'}</span>
      </div>
    );
  }

  const fmtTime = (ts) => new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

  return (
    <div className="avail-chart" ref={wrapRef}>
      <svg
        width={w || '100%'} height={H} className="avail-chart-svg"
        onMouseMove={onMove}
        onMouseLeave={() => setHover(null)}
      >
        <defs>
          {data.lines.map((l, i) => (
            <linearGradient key={i} id={`avail-grad-${i}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={l.color} stopOpacity="0.22" />
              <stop offset="100%" stopColor={l.color} stopOpacity="0" />
            </linearGradient>
          ))}
        </defs>

        {/* Горизонтальная сетка Y: 0/50/100 */}
        {[0, 50, 100].map((pct) => {
          const y = padT + (1 - pct / 100) * plotH;
          return (
            <g key={`y${pct}`}>
              <line x1={padL} y1={y} x2={padL + plotW} y2={y} stroke="var(--border-light)" strokeWidth="1" strokeDasharray={pct === 0 || pct === 100 ? '0' : '3 3'} />
              <text x={padL - 6} y={y + 3} textAnchor="end" className="avail-axis-text">{pct}%</text>
            </g>
          );
        })}

        {/* Вертикальная сетка X каждые 10 мин */}
        {data.ticks.map((tk, i) => (
          <g key={`x${i}`}>
            <line x1={tk.x} y1={padT} x2={tk.x} y2={padT + plotH}
              stroke={tk.major ? 'var(--border)' : 'var(--border-light)'}
              strokeWidth="1" strokeDasharray={tk.major ? '0' : '2 3'} opacity={tk.major ? 0.7 : 0.5} />
            {tk.major && <text x={tk.x} y={H - 5} textAnchor="middle" className="avail-axis-text">{tk.label}</text>}
          </g>
        ))}

        {/* Заливки + линии серий */}
        {data.lines.map((l, i) => (
          <path key={`f${i}`} d={l.fill} fill={`url(#avail-grad-${i})`} />
        ))}
        {data.lines.map((l, i) => (
          <path key={`l${i}`} d={l.line} fill="none" stroke={l.color} strokeWidth="2"
            strokeLinecap="round" strokeLinejoin="round" />
        ))}

        {/* Точки-проверки */}
        {data.lines.map((l, i) => l.pts.map((p, j) => (
          <circle key={`p${i}-${j}`} cx={p.x} cy={p.y} r="2.6" fill="var(--bg-card)" stroke={l.color} strokeWidth="1.8" />
        )))}

        {/* Маркер «сейчас» */}
        <line x1={data.nowX} y1={padT} x2={data.nowX} y2={padT + plotH}
          stroke="var(--accent)" strokeWidth="1" strokeDasharray="2 3" opacity="0.6" />
        <circle cx={data.nowX} cy={padT + 2} r="2" fill="var(--accent)" />

        {/* Наведение */}
        {hover && (
          <g className="avail-hover">
            <line x1={hover.x} y1={padT} x2={hover.x} y2={padT + plotH} stroke="var(--text-tertiary)" strokeWidth="1" />
            {hover.pts.map((p, i) => {
              const y = padT + (1 - Math.max(0, Math.min(100, p.pct)) / 100) * plotH;
              return <circle key={i} cx={hover.x} cy={y} r="3.5" fill={p.color} stroke="var(--bg-card)" strokeWidth="1.5" />;
            })}
          </g>
        )}
      </svg>

      {/* Тултип при наведении */}
      {hover && hover.pts.length > 0 && (
        <div className="avail-tooltip" style={{ left: Math.min(Math.max(hover.x, 60), (w || 300) - 60) }}>
          <span className="avail-tooltip-time mono">{fmtTime(hover.pts[0].ts)}</span>
          {hover.pts.map((p, i) => (
            <span key={i} className="avail-tooltip-row">
              <span className="avail-tooltip-dot" style={{ background: p.color }} />
              {p.label}: <b>{p.pct}%</b>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
