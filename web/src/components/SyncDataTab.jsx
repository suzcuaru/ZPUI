import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import Switch from './ui/Switch';
import Spinner from './ui/Spinner';
import SyncHistoryDrawer from './SyncHistoryDrawer';
import { useToast } from '../context/ToastContext';
import { FileDown, History } from 'lucide-react';

// SyncDataTab — вкладка «Данные и диагностика»: канал ZPUI Sync (статус,
// каталог секций с юзер-оверрайдами, журнал просьб сервера) и снапшот
// данных в файл (замена отчёту). Спека: Документы/Задачи/ZPUI-Sync/06 §4.

// Короткое имя секции из операции: "data.status" → "status".
const secName = (op) => String(op || '').replace(/^data\./, '');

function fmtTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return '';
  const opts = d.toDateString() === new Date().toDateString()
    ? { hour: '2-digit', minute: '2-digit' }
    : { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' };
  return d.toLocaleString(undefined, opts);
}

export default function SyncDataTab() {
  const showToast = useToast();
  const { t } = useT();
  const [info, setInfo] = useState(null);      // GET /api/sync/info
  const [ready, setReady] = useState(false);   // конфиг (sync_data) загружен
  const [channel, setChannel] = useState(true); // мастер-тумблер (config.sync_data.channel_enabled)
  const [ov, setOv] = useState({});            // оверрайды {status:{mult:2,off:false}, ...}
  const [saving, setSaving] = useState(false);
  const [channelBusy, setChannelBusy] = useState(false); // спиннер мастер-тумблера
  const [sectionBusy, setSectionBusy] = useState(null);  // имя секции, чей тумблер сохраняется
  const [snapBusy, setSnapBusy] = useState(false);
  const [histOpen, setHistOpen] = useState(false);

  const loadInfo = useCallback(async () => {
    const d = await api('GET', '/api/sync/info');
    if (d && !d.error) setInfo(d);
  }, []);

  useEffect(() => {
    let alive = true;
    api('GET', '/api/config').then((c) => {
      if (!alive || !c) { if (alive) setReady(true); return; }
      const sd = c.sync_data && typeof c.sync_data === 'object' ? c.sync_data : {};
      const map = {};
      if (sd.overrides && typeof sd.overrides === 'object') {
        for (const [k, v] of Object.entries(sd.overrides)) {
          if (!v || typeof v !== 'object') continue;
          const mult = Number(v.mult);
          if ((mult === 2 || mult === 5) || v.off === true) {
            map[k] = { mult: mult === 2 || mult === 5 ? mult : 1, off: v.off === true };
          }
        }
      }
      setChannel(sd.channel_enabled !== false);
      setOv(map);
      setReady(true);
    }).catch(() => { if (alive) setReady(true); });
    return () => { alive = false; };
  }, []);

  usePolling(loadInfo, 15000);

  // Сохранение: POST уносит ПОЛНОЕ состояние (мастер-тумблер + все оверрайды) —
  // SetSyncOverrides на бэке заменяет карту целиком.
  const persist = useCallback(async (ch, map) => {
    setSaving(true);
    try {
      const r = await api('POST', '/api/sync/overrides', { channel_enabled: ch, overrides: map });
      if (r && r.error && showToast) showToast(r.error, 'error');
    } catch {
      if (showToast) showToast(t('common.error'), 'error');
    }
    setSaving(false);
    loadInfo();
  }, [loadInfo, showToast, t]);

  const toggleChannel = async (next) => {
    if (channelBusy) return;
    setChannel(next);
    setChannelBusy(true);
    await persist(next, ov);
    setChannelBusy(false);
  };

  const setSection = async (name, patch) => {
    const cur = ov[name] || { mult: 1, off: false };
    const next = { ...cur, ...patch };
    const map = { ...ov };
    if (next.mult !== 2 && next.mult !== 5 && !next.off) {
      // {mult:1, off:false} — значение по умолчанию, ключ не нужен
      delete map[name];
    } else {
      map[name] = next;
    }
    setOv(map);
    setSectionBusy(name);
    await persist(channel, map);
    setSectionBusy(null);
  };

  const saveSnapshot = async () => {
    if (snapBusy) return;
    setSnapBusy(true);
    try {
      const r = await api('POST', '/api/sync/snapshot');
      if (r && r.error) {
        if (showToast) showToast(r.error, 'error');
      } else if (r && r.path && showToast) {
        showToast(t('syncd.snapshotSaved', { path: r.path }), 'success');
      }
    } catch {
      if (showToast) showToast(t('syncd.snapshotFail'), 'error');
    }
    setSnapBusy(false);
  };

  const online = !!(info && info.online);
  const authOff = ready && channel && info && info.enabled === false;

  return (
    <div className="syncd-page tab-fade stagger">
      <div className="section">
        <div className="section-title set-section-title">
          <span>{t('syncd.statusTitle')}</span>
          <span className={'syncd-pill' + (online ? ' on' : '')}>
            <span className="syncd-pill-dot" />
            {online ? t('syncd.online') : t('syncd.offline')}
          </span>
        </div>

        <div className="set-row">
          <div className="set-row-info">
            <span className="set-row-title">{t('syncd.channelRow')}</span>
            <span className="set-row-desc">{t('syncd.channelDesc')}</span>
          </div>
          <Switch checked={channel} onChange={toggleChannel} loading={channelBusy} />
        </div>

        {saving && <div className="syncd-saving"><Spinner size={11} />{t('common.saving')}</div>}

        {!channel && <div className="syncd-hint">{t('syncd.channelOffHint')}</div>}
        {authOff && <div className="syncd-hint">{t('syncd.authHint')}</div>}

        {channel && info && (
          <>
            <div className="set-row">
              <div className="set-row-info"><span className="set-row-title">{t('syncd.lastTick')}</span></div>
              <span className="syncd-val mono">{fmtTime(info.last_tick) || '—'}</span>
            </div>
            <div className="set-row">
              <div className="set-row-info"><span className="set-row-title">{t('syncd.rtt')}</span></div>
              <span className="syncd-val mono">{typeof info.rtt_ms === 'number' ? `${info.rtt_ms} ${t('common.ms')}` : '—'}</span>
            </div>
            <div className="set-row">
              <div className="set-row-info"><span className="set-row-title">{t('syncd.lastError')}</span></div>
              <span className={'syncd-val mono' + (info.last_error ? ' err' : '')}>{info.last_error || '—'}</span>
            </div>
          </>
        )}

        <div className="set-row set-row-bottom">
          <div className="set-row-info">
            <span className="set-row-title">{t('syncd.snapshotTitle')}</span>
            <span className="set-row-desc">{t('syncd.snapshotDesc')}</span>
          </div>
          <button className="btn btn-accent btn-sm" onClick={saveSnapshot} disabled={snapBusy}>
            {snapBusy ? <Spinner size={13} /> : <FileDown size={13} strokeWidth={2.2} />}
            {t('syncd.snapshotBtn')}
          </button>
        </div>
      </div>

      <div className="section">
        <div className="section-title set-section-title">
          <span>{t('syncd.catalogTitle')}</span>
          {!online && <span className="syncd-saving">{t('syncd.offline')}</span>}
        </div>
        <div className="syncd-catalog-desc">{t('syncd.catalogHint')}</div>
        {!ready || !info ? (
          <div className="syncd-skel" aria-hidden="true">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="syncd-skel-row">
                <span className="skeleton skeleton-pulse skeleton-line" style={{ width: '40%' }} />
                <span className="skeleton skeleton-pulse skeleton-line" style={{ width: '22%' }} />
              </div>
            ))}
          </div>
        ) : (
          <div className={'syncd-list' + (channel ? '' : ' muted')}>
            {(info.sections || []).map((s) => {
              const name = secName(s.op);
              const mult = (ov[name] && ov[name].mult) || 1;
              const off = !!(ov[name] && ov[name].off);
              return (
                <div key={s.op} className={'syncd-sec' + (off ? ' off' : '')}>
                  <div className="syncd-sec-info">
                    <span className="syncd-sec-name">{t('syncd.sec.' + name)}</span>
                    <span className="syncd-sec-meta">
                      {s.on_change && <span className="syncd-badge">{t('syncd.onchange')}</span>}
                      {s.diff && <span className="syncd-badge">{t('syncd.diff')}</span>}
                      {!off && t('syncd.every', { interval: fmtIntervalSec(s.interval, t) })}
                      {s.last_ok ? ` · ${t('syncd.sentAt', { time: fmtTime(s.last_ok) })}` : ''}
                    </span>
                  </div>
                  <div className="syncd-sec-ctl">
                    <div className="syncd-mult" role="group" aria-label={t('syncd.multLabel')}>
                      {[1, 2, 5].map((m) => (
                        <button
                          key={m}
                          className={'btn btn-sm' + (mult === m && !off ? ' active' : '')}
                          disabled={off}
                          onClick={() => setSection(name, { mult: m, off: false })}
                        >
                          ×{m}
                        </button>
                      ))}
                    </div>
                    <Switch checked={!off} onChange={(v) => setSection(name, { off: !v })} loading={sectionBusy === name} hideState />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <div className="section">
        <div className="section-title set-section-title">
          <span>{t('syncd.reqTitle')}</span>
          <button className="btn btn-sm" onClick={() => setHistOpen(true)}>
            <History size={13} strokeWidth={2.2} />
            {t('syncd.hist.btn')}
          </button>
        </div>
        {(!info || !info.requests_log || info.requests_log.length === 0) ? (
          <div className="syncd-empty">{t('syncd.reqEmpty')}</div>
        ) : (
          <div className="syncd-reqs">
            {[...info.requests_log].reverse().map((e, i) => {
              const names = (e.sections || []).map((op) => t('syncd.sec.' + secName(op))).join(', ');
              return (
                <div key={i} className="syncd-req">
                  <span className="syncd-req-time mono">{fmtTime(e.at) || '—'}</span>
                  <span className="syncd-req-text">
                    {names || '—'}
                    {e.skips ? ` · ${t('syncd.reqSkips')}` : ''}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <SyncHistoryDrawer open={histOpen} onClose={() => setHistOpen(false)} />
    </div>
  );
}

const UNITS = { s: 'syncd.unitSec', min: 'syncd.unitMin', hr: 'syncd.unitHr' };

function fmtIntervalSec(sec, t) {
  const n = Number(sec) || 0;
  if (n < 90) return `${n} ${t(UNITS.s)}`;
  if (n < 3600) return `${Math.round(n / 60)} ${t(UNITS.min)}`;
  return `${Math.round(n / 3600)} ${t(UNITS.hr)}`;
}
