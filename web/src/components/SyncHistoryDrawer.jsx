import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../api';
import { useT } from '../i18n';
import useDialogA11y from '../hooks/useDialogA11y';
import { usePolling } from '../hooks/usePolling';
import { OverlayPortal } from './ui/Modal';
import { History, X } from 'lucide-react';

// SyncHistoryDrawer — боковая панель истории запросов канала ZPUI Sync
// (каркас лог-панели: lg-*). Периоды сегодня/7/30 дней — старше в БД не
// хранится (internal/database, sync_history). Панель absolutely-позиционна
// (lg-*), поэтому монтируется порталом в «окно приложения» — как LogDrawer
// в корне App; иначе якорится к скроллящемуся контенту вкладки и ездит
// вместе с ним (то под панелями, то над ними).

const PERIODS = [
  { id: 'today', labelKey: 'syncd.hist.today' },
  { id: '7d', labelKey: 'syncd.hist.7d' },
  { id: '30d', labelKey: 'syncd.hist.30d' },
];

const secName = (op) => String(op || '').replace(/^data\./, '');

function opLabel(op, t) {
  if (op === 'sync.hello') return t('syncd.hist.hello');
  if (op === 'sync.pull') return t('syncd.hist.pull');
  const key = 'syncd.sec.' + secName(op);
  const lbl = t(key);
  return lbl !== key ? lbl : op;
}

function fmtDate(iso) {
  const d = new Date(iso);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return '';
  return d.toLocaleDateString(undefined, { day: '2-digit', month: '2-digit' });
}

function fmtTime(iso) {
  const d = new Date(iso);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return '';
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export default function SyncHistoryDrawer({ open, onClose }) {
  const { t } = useT();
  // Фокус после слайда (260мс) — как в LogDrawer: панель ещё за translateX(100%).
  const dialogRef = useDialogA11y({ open, onClose, closable: true, focusDelay: 280 });
  const [period, setPeriod] = useState('today');
  const [who, setWho] = useState('all');
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(false);
  const bodyRef = useRef(null);

  const load = useCallback(async () => {
    const d = await api('GET', `/api/sync/history?period=${period}`);
    setRows(d && !d.error && Array.isArray(d.rows) ? d.rows : []);
    setLoading(false);
  }, [period]);

  const visible = who === 'all' ? rows : rows.filter((r) => r.who === who);

  usePolling(load, 5000, [load], open);

  // Скелетон — на открытие и смену периода; тики поллинга дозагружают тихо.
  useEffect(() => {
    if (open) setLoading(true);
  }, [open, period]);

  // Смена периода — прокрутка наверх (как смена таба в LogDrawer).
  useEffect(() => {
    if (bodyRef.current) bodyRef.current.scrollTop = 0;
  }, [period]);

  return (
    <OverlayPortal>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <div
        ref={dialogRef}
        className={'lg-drawer' + (open ? ' open' : '')}
        role="dialog"
        aria-modal="true"
        aria-label={t('syncd.hist.title')}
        aria-hidden={!open}
        tabIndex={-1}
      >
        <div className="lg-head">
          <span className="lg-head-icon"><History size={18} strokeWidth={2} /></span>
          <span className="lg-head-title">{t('syncd.hist.title')}</span>
          <div className="lg-spacer" />
          <button className="modal-close" data-sound="off" onClick={onClose} aria-label={t('common.close')}>
            <X size={16} strokeWidth={1.5} />
          </button>
        </div>

        <div className="lg-toolbar">
          <div className="lg-toolbar-row">
            <div className="lg-seg" role="group" aria-label={t('syncd.hist.period')}>
              {PERIODS.map((p) => (
                <button
                  key={p.id}
                  className={'flt2-pill' + (period === p.id ? ' active' : '')}
                  onClick={() => setPeriod(p.id)}
                >
                  {t(p.labelKey)}
                </button>
              ))}
            </div>
            <div className="lg-spacer" />
            <span className="lg-count mono">{t('syncd.hist.count', { n: visible.length })}</span>
          </div>
          <div className="lg-toolbar-row">
            <div className="lg-seg" role="group" aria-label={t('syncd.hist.who')}>
              {[['all', 'whoAll'], ['server', 'whoServer'], ['device', 'whoDevice']].map(([id, key]) => (
                <button
                  key={id}
                  className={'flt2-pill' + (who === id ? ' active' : '')}
                  onClick={() => setWho(id)}
                >
                  {t('syncd.hist.' + key)}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="lg-body">
          <div className="lg-table-wrap">
            <div className="lg-table-header">
              <span className="lg-th sh-th-date">{t('syncd.hist.date')}</span>
              <span className="lg-th sh-th-time">{t('syncd.hist.time')}</span>
              <span className="lg-th sh-th-who">{t('syncd.hist.who')}</span>
              <span className="lg-th sh-th-what">{t('syncd.hist.what')}</span>
            </div>
            <div className="lg-table-body" ref={bodyRef}>
              {loading && rows.length === 0 ? (
                <div className="lg-skeleton" aria-hidden="true">
                  {Array.from({ length: 12 }).map((_, i) => (
                    <div key={i} className="lg-skel-row">
                      <span className="skeleton skeleton-pulse lg-skel-time" />
                      <span className="skeleton skeleton-pulse lg-skel-msg" style={{ width: `${58 - (i % 3) * 12}%` }} />
                    </div>
                  ))}
                </div>
              ) : visible.length > 0 ? (
                visible.map((r, i) => (
                  <div key={i} className={'lg-tr' + (r.result ? ' error' : '')}>
                    <span className="lg-td sh-th-date mono">{fmtDate(r.ts)}</span>
                    <span className="lg-td sh-th-time mono">{fmtTime(r.ts)}</span>
                    <span className="lg-td sh-th-who">
                      <span className={'sh-who-badge' + (r.who === 'server' ? ' server' : '')}>
                        {r.who === 'server' ? t('syncd.hist.whoServer') : t('syncd.hist.whoDevice')}
                      </span>
                    </span>
                    <span className="lg-td sh-th-what">
                      {r.who === 'server' ? t('syncd.hist.asked', { what: opLabel(r.op, t) }) : opLabel(r.op, t)}
                      {r.result ? <span className="sh-err"> — {t('syncd.hist.fail', { code: r.result })}</span> : ''}
                    </span>
                  </div>
                ))
              ) : rows.length > 0 ? (
                <div className="lg-empty">
                  <span className="lg-empty-hint">{t('syncd.hist.filterEmpty')}</span>
                </div>
              ) : (
                <div className="lg-empty">
                  <span className="lg-empty-icon"><History size={20} strokeWidth={1.6} /></span>
                  <span className="lg-empty-title">{t('syncd.hist.empty')}</span>
                  <span className="lg-empty-hint">{t('syncd.hist.emptyHint')}</span>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </OverlayPortal>
  );
}
