import { useState, useEffect, useRef, useMemo } from 'react';
import { useT } from '../../i18n';
import { formatMetricValue } from '../../utils';

// metric: 'rate' — загрузка/отдача (B/s→KB/s→MB/s), 'mbps' — Мбит/с, 'ms' — мс,
// 'raw' — целые числа (жалобы/ошибки). Тултип и подпись оси используют единицы
// своей метрики (не «всё в байтах»).
// series — несколько линий в общих осях (общая шкала: max по всем данным).
// times — необязательный массив времён (мс) по индексам точек: в тултипе
// добавляется строка времени в 24-часовом формате HH:MM.
function TrafficChart({ data, color, label, height, metric = 'rate', series, times }) {
  const { t } = useT();
  const wrapRef = useRef(null);
  const [w, setW] = useState(0);
  const [hoverIdx, setHoverIdx] = useState(null);
  const H = height || 150;
  const padL = 8, padR = 8, padT = 8, padB = 16;
  const lines = series || (data ? [{ data, color, label }] : []);

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
    if (!lines.length || plotW <= 0) return null;
    const all = lines.flatMap(l => l.data || []);
    if (all.length < 1) return null;
    const mx = Math.max(...all, 1);
    const len = Math.max(...lines.map(l => (l.data || []).length));
    const step = len > 1 ? plotW / (len - 1) : plotW;
    const ser = lines.map(l => {
      const pts = (l.data || []).map((v, i) => ({
        x: padL + i * step,
        y: padT + (1 - v / mx) * plotH,
        val: v,
      }));
      const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
      const fill = pts.length > 0
        ? `${line} L${pts[pts.length - 1].x.toFixed(1)},${(padT + plotH).toFixed(1)} L${pts[0].x.toFixed(1)},${(padT + plotH).toFixed(1)} Z`
        : '';
      return { color: l.color, label: l.label, pts, line, fill };
    });
    return { ser, mx, step, len };
  }, [lines, plotW, plotH, padL, padT]);

  const onMove = (e) => {
    if (!computed) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    let bestIdx = 0, bestDist = Infinity;
    for (let i = 0; i < computed.len; i++) {
      const x = padL + i * computed.step;
      const d = Math.abs(x - mx);
      if (d < bestDist) { bestDist = d; bestIdx = i; }
    }
    setHoverIdx(bestIdx);
  };

  if (!computed) {
    return <div className="avail-chart-empty" ref={wrapRef} style={{height: H}}><span>—</span></div>;
  }

  const unit = metric === 'mbps' ? t('monitor.mbps') : metric === 'ms' ? t('common.ms') : '';
  const fmt = (v) => {
    const num = formatMetricValue(v, metric);
    return unit ? `${num} ${unit}` : num;
  };
  const hoverX = padL + hoverIdx * computed.step;
  const hoverTime = times && times[hoverIdx]
    ? `${String(new Date(times[hoverIdx]).getHours()).padStart(2, '0')}:${String(new Date(times[hoverIdx]).getMinutes()).padStart(2, '0')}`
    : null;

  return (
    <div className="tchart-wrap" ref={wrapRef}>
      <svg
        width={w || '100%'} height={H} className="avail-chart-svg"
        onMouseMove={onMove}
        onMouseLeave={() => setHoverIdx(null)}
      >
        <defs>
          {computed.ser.map(s => (
            <linearGradient key={s.label} id={`tc-grad-${s.label}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={s.color} stopOpacity="0.22" />
              <stop offset="100%" stopColor={s.color} stopOpacity="0" />
            </linearGradient>
          ))}
        </defs>

        {[0, 0.5, 1].map((f) => {
          const y = padT + f * plotH;
          return <line key={f} x1={padL} y1={y} x2={padL + plotW} y2={y}
            stroke="var(--border-light)" strokeWidth="1" strokeDasharray={f === 0 || f === 1 ? '0' : '3 3'} />;
        })}

        {computed.ser.map(s => (
          <g key={s.label}>
            <path d={s.fill} fill={`url(#tc-grad-${s.label})`} />
            <path d={s.line} fill="none" stroke={s.color} strokeWidth="2"
              strokeLinecap="round" strokeLinejoin="round" />
            {s.pts.map((p, i) => (
              (i === s.pts.length - 1 || (i === hoverIdx && lines.length === 1)) && (
                <circle key={i} cx={p.x} cy={p.y} r={i === hoverIdx ? 3.5 : 2.6}
                  fill="var(--bg-card)" stroke={s.color} strokeWidth="1.8" />
              )
            ))}
          </g>
        ))}

        {hoverIdx !== null && (
          <line x1={hoverX} y1={padT}
            x2={hoverX} y2={padT + plotH}
            stroke="var(--text-tertiary)" strokeWidth="1" opacity="0.5" />
        )}

        {/* Подпись максимума — последним элементом: поверх линии и заливки,
            иначе график в пике перекрывает текст. */}
        <text className="tchart-axis" x={padL + 2} y={padT + 9}>{fmt(computed.mx)}</text>
      </svg>

      {hoverIdx !== null && (
        <div className="tchart-tooltip" style={{
          left: Math.min(Math.max(hoverX, 50), (w || 300) - 50),
        }}>
          {hoverTime && <div className="tchart-tooltip-time">{hoverTime}</div>}
          {computed.ser.map(s => s.pts[hoverIdx] ? (
            <span key={s.label} className="avail-tooltip-row">
              <span className="avail-tooltip-dot" style={{ background: s.color }} />
              {fmt(s.pts[hoverIdx].val)}
            </span>
          ) : null)}
        </div>
      )}
    </div>
  );
}

export default TrafficChart;
