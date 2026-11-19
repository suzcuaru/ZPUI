import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import useDialogA11y from '../../hooks/useDialogA11y';
import { usePolling } from '../../hooks/usePolling';
import { useToast } from '../../context/ToastContext';
import { Trash2, Copy, Download, X, ScrollText, FileDown, AlertCircle } from 'lucide-react';
import FeedbackModal from '../feedback/FeedbackModal';
import { useConfirm } from '../ui/ConfirmDialog';
import Select from '../ui/Select';
import Spinner from '../ui/Spinner';

const TABLES = [
  { id: 'all', labelKey: 'logs.all' },
  { id: 'app', labelKey: 'logs.app' },
  { id: 'zapret', labelKey: 'logs.zapret' },
  // Журнал службы ZPUI (service.log): файл, а не таблица БД —
  // счётчик бейджа приходит в counts['service'] без суффикса _logs.
  { id: 'service', labelKey: 'logs.service' },
  { id: 'updater', labelKey: 'logs.updater' },
  { id: 'tgproxy', labelKey: 'logs.tgproxy' },
];

const LEVELS = ['ALL', 'ERROR', 'WARN', 'INFO'];

function modLabel(mod, t) {
  if (!mod) return '';
  const key = 'logs.' + mod.replace('_logs', '');
  const lbl = t(key);
  return lbl !== key ? lbl : mod;
}

export default function LogDrawer({ open, onClose, scrollToError }) {
  const showToast = useToast();
  const { t } = useT();
  const [fbOpen, setFbOpen] = useState(false);
  const confirm = useConfirm();
  // Фокус ставим после слайда (260мс): в момент анимации панель стоит за
  // translateX(100%), и zoom-квирк Chromium уводит прокрутку окна.
  const dialogRef = useDialogA11y({ open, onClose, closable: true, focusDelay: 280 });
  const [activeTab, setActiveTab] = useState('all');
  const [level, setLevel] = useState('ALL');
  const [modFilter, setModFilter] = useState('ALL');
  const [rows, setRows] = useState([]);
  const [stats, setStats] = useState({ total: 0, errors: 0, db_size: 0, counts: {} });
  const [tableCounts, setTableCounts] = useState({});
  const [loading, setLoading] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [snapshotBusy, setSnapshotBusy] = useState(false);
  const tableBodyRef = useRef(null);
  const tabsRef = useRef(null);
  const errRef = useRef(null);

  // Колесо мыши над полосой вкладок → горизонтальная прокрутка
  // (тачпад скроллит горизонтально нативно). Нативный listener с passive:false —
  // React onWheel пассивен и не позволяет preventDefault.
  useEffect(() => {
    const el = tabsRef.current;
    if (!el) return undefined;
    const onWheel = (e) => {
      if (e.deltaX !== 0) return;
      if (e.deltaY === 0) return;
      e.preventDefault();
      el.scrollLeft += e.deltaY;
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, []);

  const loadStats = useCallback(async () => {
    const d = await api('GET', '/api/logs/stats');
    if (d) {
      setStats(d);
      if (d.counts) setTableCounts(d.counts);
    }
  }, []);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    let d;
    if (activeTab === 'all') {
      d = await api('GET', `/api/logs/all?level=${level}&limit=500&offset=0`);
    } else {
      d = await api('GET', `/api/logs?table=${activeTab}&level=${level}&limit=500&offset=0`);
    }
    if (d?.lines) setRows(d.lines);
    else setRows([]);
    setLoading(false);
    loadStats();
  }, [activeTab, level, loadStats]);

  usePolling(loadLogs, 3000, [activeTab, level, loadLogs], open);

  // Смена таба/уровня — сбрасываем прокрутку, иначе после подгрузки
  // строк остаётся старая позиция и список «прыгает».
  useEffect(() => {
    if (tableBodyRef.current) tableBodyRef.current.scrollTop = 0;
  }, [activeTab, level]);

  useEffect(() => {
    if (!open || !scrollToError || !errRef.current || rows.length === 0) return;
    setTimeout(() => {
      errRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 300);
  }, [open, scrollToError, rows]);

  const filtered = useMemo(() => {
    if (modFilter === 'ALL') return rows;
    const mf = modFilter.toLowerCase();
    return rows.filter(l => (l.category || '').toLowerCase() === mf);
  }, [rows, modFilter]);

  const handleClear = async () => {
    if (clearing) return;
    const target = activeTab === 'all' ? t('logs.all') : t(`logs.${activeTab}`);
    const confirmMsg = activeTab === 'all'
      ? t('logs.clearAllConfirm')
      : t('logs.clearCatConfirm', { cat: target });
    if (!await confirm({ message: confirmMsg, variant: 'danger', confirmText: t('logs.clearBtn') })) return;

    setClearing(true);
    await api('POST', '/api/logs/clear-bucket', { table: activeTab });
    setClearing(false);
    setRows([]);
    loadLogs();
    loadStats();
    if (showToast) showToast(t('logs.clearedToast'), 'success');
  };

  const exportLogs = async () => {
    setExporting(true);
    try {
      const res = await api('POST', '/api/logs/export');
      if (res?.status === 'ok' && showToast) {
        showToast(t('logs.exportDone'), 'success');
      } else if (res?.error && showToast) {
        showToast(res.error, 'error');
      }
    } catch {
      if (showToast) showToast(t('logs.exportError'), 'error');
    }
    setExporting(false);
  };

  const copyAll = () => {
    const text = filtered.map(l =>
      `[${l.timestamp}] [${l.level}]${l.code ? ' [' + l.code + ']' : ''}${l.table ? ' [' + l.table + ']' : ''} ${l.message}`
    ).join('\n');
    if (text) navigator.clipboard.writeText(text);
  };

  const getLevelClass = (lv) => (lv || 'INFO').toLowerCase();

  const tabCount = (tabId) => {
    if (tabId === 'all') return stats.total;
    if (tabId === 'service') return tableCounts.service || 0;
    return tableCounts[tabId + '_logs'] || 0;
  };

  // Снапшот данных: локальный экспорт всех секций канала в JSON
  // (замена отчёту — файл удобно переслать в поддержку).
  const saveSnapshot = async () => {
    if (snapshotBusy) return;
    setSnapshotBusy(true);
    try {
      const r = await api('POST', '/api/sync/snapshot');
      if (r?.error) {
        if (showToast) showToast(r.error, 'error');
      } else if (r?.path && showToast) {
        showToast(t('syncd.snapshotSaved', { path: r.path }), 'success');
      }
    } catch {
      if (showToast) showToast(t('syncd.snapshotFail'), 'error');
    }
    setSnapshotBusy(false);
  };

  const modFilterItems = useMemo(() => {
    if (activeTab !== 'app') return [];
    const seen = new Set();
    const items = [{ id: 'ALL', label: t('logs.all') }];
    rows.forEach(l => {
      const m = l.category;
      if (m && !seen.has(m)) {
        seen.add(m);
        items.push({ id: m, label: m });
      }
    });
    return items;
  }, [rows, activeTab, t]);

  const isFiltering = modFilter !== 'ALL';

  // Обрезанное сообщение показываем целиком тултипом TooltipLayer. Атрибут
  // data-tooltip ставим императивно после отрисовки: факт переполнения строки
  // знает только DOM (scrollWidth против clientWidth), и вешаем его только
  // на реально обрезанные ячейки — тултип не мешает на коротких записях.
  const syncMsgTips = useCallback(() => {
    const body = tableBodyRef.current;
    if (!body) return;
    body.querySelectorAll('.lg-td-msg').forEach((el) => {
      if (el.scrollWidth - el.clientWidth > 1) el.setAttribute('data-tooltip', el.textContent || '');
      else el.removeAttribute('data-tooltip');
    });
  }, []);

  useEffect(() => {
    syncMsgTips();
  }, [syncMsgTips, filtered, open, loading]);

  useEffect(() => {
    window.addEventListener('resize', syncMsgTips);
    return () => window.removeEventListener('resize', syncMsgTips);
  }, [syncMsgTips]);

  return (
    <>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <div
        ref={dialogRef}
        className={'lg-drawer' + (open ? ' open' : '')}
        role="dialog"
        aria-modal="true"
        aria-label={t('logs.title')}
        aria-hidden={!open}
        tabIndex={-1}
      >
        <div className="lg-head">
          <span className="lg-head-icon"><ScrollText size={18} strokeWidth={2} /></span>
          <span className="lg-head-title">{t('logs.title')}</span>
          <div className="lg-spacer" />
          <button className="btn btn-sm" onClick={saveSnapshot} disabled={snapshotBusy} title={t('logs.snapshotTip')}>
            {snapshotBusy ? <Spinner size={14} /> : <FileDown size={13} strokeWidth={2} />}
            <span>{t('logs.snapshot')}</span>
          </button>
          <button className="btn btn-sm" onClick={() => setFbOpen(true)} title={t('feedback.title')}>
            <AlertCircle size={13} strokeWidth={2} />
            <span>{t('feedback.title')}</span>
          </button>
          <button className="btn btn-sm btn-danger-ghost" onClick={handleClear} disabled={clearing}>
            {clearing ? <Spinner size={14} /> : <Trash2 size={13} strokeWidth={2} />}
            <span>{t('logs.clear')}</span>
          </button>
          <button className="modal-close" data-sound="off" onClick={onClose} aria-label={t('common.close')}>
            <X size={16} strokeWidth={1.5} />
          </button>
        </div>

        <div className="lg-head-tabs" ref={tabsRef}>
          {TABLES.map(tab => {
            const cnt = tabCount(tab.id);
            return (
              <button
                key={tab.id}
                className={'lg-tab' + (activeTab === tab.id ? ' on' : '')}
                onClick={() => setActiveTab(tab.id)}
              >
                {t(tab.labelKey)}
                {cnt > 0 && <span className="lg-tab-badge">{cnt > 999 ? '999+' : cnt}</span>}
              </button>
            );
          })}
        </div>

        <div className="lg-toolbar">
          <div className="lg-toolbar-row">
            <div className="lg-seg" role="group" aria-label={t('logs.level')}>
              {LEVELS.map(lv => (
                <button
                  key={lv}
                  className={'flt2-pill' + (level === lv ? ' active' : '')}
                  onClick={() => setLevel(lv)}
                >
                  {lv}
                </button>
              ))}
            </div>
            {activeTab === 'app' && modFilterItems.length > 1 && (
              <Select
                items={modFilterItems}
                value={modFilter}
                onChange={setModFilter}
                ariaLabel={t('logs.module')}
                align="right"
              />
            )}
            <div className="lg-actions">
              <button className="btn btn-icon" onClick={copyAll} data-tooltip={t('logs.copyTip')} data-tooltip-pos="bottom">
                <Copy size={12} strokeWidth={2} />
              </button>
              <button className="btn btn-icon" onClick={exportLogs} disabled={exporting}
                data-tooltip={t('logs.exportTip')} data-tooltip-pos="bottom">
                {exporting ? <Spinner size={14} /> : <Download size={12} strokeWidth={2} />}
              </button>
            </div>
          </div>
        </div>

        <div className="lg-body">
          <div className="lg-table-wrap">
            <div className="lg-table-header">
              <span className="lg-th lg-th-code">{t('logs.level')}</span>
              <span className="lg-th lg-th-time">{t('logs.time')}</span>
              {(activeTab === 'app' || activeTab === 'all') && <span className="lg-th lg-th-mod">{t('logs.module')}</span>}
              <span className="lg-th lg-th-msg">{t('logs.message')}</span>
            </div>
            <div className="lg-table-body" ref={tableBodyRef}>
              {loading && rows.length === 0 ? (
                <div className="lg-skeleton" aria-hidden="true">
                  {Array.from({ length: 12 }).map((_, i) => (
                    <div key={i} className="lg-skel-row">
                      <span className="skeleton skeleton-pulse lg-skel-badge" />
                      <span className="skeleton skeleton-pulse lg-skel-time" />
                      <span className="skeleton skeleton-pulse lg-skel-msg" style={{ width: `${72 - (i % 3) * 14}%` }} />
                    </div>
                  ))}
                </div>
              ) : filtered.length > 0 ? (
                filtered.map((l, i) => {
                  const lv = getLevelClass(l.level);
                  const isError = lv === 'error';
                  const hasCode = l.code && l.code !== '';
                  const isTarget = scrollToError && l.code === scrollToError;
                  return (
                    <div
                      key={i}
                      ref={isTarget ? errRef : null}
                      className={'lg-tr' + (isError ? ' error' : '') + (isTarget ? ' highlight' : '')}
                    >
                      <span className={'lg-td lg-td-code ' + lv}>
                        {hasCode ? (
                          <span className="lg-code-badge">{l.code}</span>
                        ) : (
                          <span className={'lg-level-label lg-lv-' + lv}>{l.level || 'INFO'}</span>
                        )}
                      </span>
                      <span className="lg-td lg-td-time">{l.timestamp || ''}</span>
                      {(activeTab === 'app' || activeTab === 'all') && (
                        <span className="lg-td lg-td-mod">
                          <span className="lg-mod-badge">{l.category || modLabel(l.table, t)}</span>
                        </span>
                      )}
                      <span className="lg-td lg-td-msg">{l.message || ''}</span>
                    </div>
                  );
                })
              ) : (
                <div className="lg-empty">
                  <span className="lg-empty-icon"><ScrollText size={20} strokeWidth={1.6} /></span>
                  <span className="lg-empty-title">{isFiltering ? t('logs.nothingFound') : t('logs.noLogs')}</span>
                  <span className="lg-empty-hint">{isFiltering ? t('logs.emptyHintSearch') : t('logs.emptyHint')}</span>
                </div>
              )}
            </div>
          </div>
        </div>

      </div>
      <FeedbackModal open={fbOpen} onClose={() => setFbOpen(false)} />
    </>
  );
}
