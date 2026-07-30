import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { Trash2, Copy, Download, AlertTriangle, ChevronDown, FileBarChart, ArrowLeft } from 'lucide-react';
import { useConfirm } from '../ui/ConfirmDialog';

const TABLES = [
  { id: 'all', labelKey: 'logs.all' },
  { id: 'app', labelKey: 'logs.app' },
  { id: 'zapret', labelKey: 'logs.zapret' },
  { id: 'updater', labelKey: 'logs.updater' },
  { id: 'report', labelKey: 'logs.report' },
];

const LEVELS = ['ALL', 'ERROR', 'WARN', 'INFO'];

function formatSize(bytes) {
  if (!bytes) return '0 B';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1048576).toFixed(1) + ' MB';
}

function modLabel(mod, t) {
  if (!mod) return '';
  const key = 'logs.' + mod.replace('_logs', '');
  const lbl = t(key);
  return lbl !== key ? lbl : mod;
}

export default function LogDrawer({ open, onClose, scrollToError, onGenerateReport, showToast }) {
  const { t } = useT();
  const confirm = useConfirm();
  const [activeTab, setActiveTab] = useState('all');
  const [level, setLevel] = useState('ALL');
  const [modFilter, setModFilter] = useState('ALL');
  const [modFilterOpen, setModFilterOpen] = useState(false);
  const [rows, setRows] = useState([]);
  const [stats, setStats] = useState({ total: 0, errors: 0, db_size: 0, counts: {} });
  const [tableCounts, setTableCounts] = useState({});
  const [loading, setLoading] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [errorSnapshots, setErrorSnapshots] = useState([]);
  const [showErrors, setShowErrors] = useState(false);
  const [selectedError, setSelectedError] = useState(null);
  const [errorContent, setErrorContent] = useState('');
  const bodyRef = useRef(null);
  const tableBodyRef = useRef(null);
  const errRef = useRef(null);

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

  useEffect(() => {
    if (!open) return;
    loadLogs();
    const iv = setInterval(loadLogs, 3000);
    return () => clearInterval(iv);
  }, [open, activeTab, level, loadLogs]);

  useEffect(() => {
    if (!open) return;
    const loadErr = async () => {
      const d = await api('GET', '/api/logs/errors');
      if (d?.files) setErrorSnapshots(d.files);
    };
    loadErr();
  }, [open]);

  useEffect(() => {
    if (!open || !scrollToError || !errRef.current || rows.length === 0) return;
    setTimeout(() => {
      errRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 300);
  }, [open, scrollToError, rows]);

  const readError = async (name) => {
    setSelectedError(name);
    setErrorContent('');
    const d = await api('GET', `/api/logs/error?name=${encodeURIComponent(name)}`);
    if (d?.content) setErrorContent(d.content);
  };

  const deleteError = async (name, e) => {
    e.stopPropagation();
    await api('POST', '/api/logs/error/delete', { name });
    const refresh = await api('GET', '/api/logs/errors');
    if (refresh?.files) setErrorSnapshots(refresh.files);
    if (selectedError === name) {
      setSelectedError(null);
      setErrorContent('');
    }
  };

  const filtered = useMemo(() => {
    let result = rows;
    if (modFilter !== 'ALL') {
      const mf = modFilter.toLowerCase();
      result = result.filter(l => (l.category || '').toLowerCase() === mf);
    }
    return result;
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
  };

  const exportLogs = async () => {
    setExporting(true);
    try {
      const res = await api('POST', '/api/logs/export');
      if (res?.status === 'ok' && showToast) {
        showToast(t('logs.exportDone') || 'ZIP-архив создан', 'success');
      } else if (res?.error && showToast) {
        showToast(res.error, 'error');
      }
    } catch {
      if (showToast) showToast(t('logs.exportError') || 'Ошибка экспорта', 'error');
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
    return tableCounts[tabId + '_logs'] || 0;
  };

  const handleGenerateReport = () => {
    if (onGenerateReport) onGenerateReport();
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

  const formatTime = (ts) => {
    if (!ts) return '';
    const parts = ts.split(' ');
    if (parts.length === 2) {
      return parts[0] + '\n' + parts[1];
    }
    return ts;
  };

  return (
    <>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <div className={'lg-drawer' + (open ? ' open' : '')}>
        <div className="lg-head">
          <button className="lg-head-close" onClick={onClose}><ArrowLeft size={15} strokeWidth={2.3} /></button>
          <div className="lg-head-info">
            <span className="lg-head-title">{t('logs.title')}</span>
            <span className="lg-head-meta">{stats.errors > 0 ? `${stats.errors} ${t('logs.errorsShort')} · ${formatSize(stats.db_size)}` : formatSize(stats.db_size)}</span>
          </div>
          <div className="lg-spacer" />
          <button
            className="lg-btn"
            onClick={() => setShowErrors(!showErrors)}
            data-tooltip={t('logs.errorSnapshots')}
            data-tooltip-pos="bottom"
          >
            <AlertTriangle size={14} strokeWidth={2} />
            {errorSnapshots.length > 0 && <span className="lg-err-count">{errorSnapshots.length}</span>}
          </button>
          <button
            className="lg-btn"
            onClick={handleGenerateReport}
            data-tooltip={t('sidebar.reportLabel')}
            data-tooltip-pos="bottom"
          >
            <FileBarChart size={14} strokeWidth={2} />
          </button>
          <button
            className="lg-btn"
            onClick={handleClear}
            disabled={clearing}
            data-tooltip={t('logs.clearTip')}
            data-tooltip-pos="left"
          >
            {clearing ? <span className="mini-spin" /> : <Trash2 size={14} strokeWidth={2} />}
          </button>
        </div>

        <div className="lg-head-tabs">
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
            <div className="lg-levels">
              {LEVELS.map(lv => (
                <button
                  key={lv}
                  className={'lg-level' + (level === lv ? ' on' : '') + ' lg-level-' + lv.toLowerCase()}
                  onClick={() => setLevel(lv)}
                >
                  {lv}
                </button>
              ))}
              {activeTab === 'app' && modFilterItems.length > 1 && (
                <div className="lg-cat-dd">
                  <button className="lg-cat-dd-btn" onClick={() => setModFilterOpen(!modFilterOpen)}>
                    {modFilter === 'ALL' ? t('logs.all') : modFilter}
                    <ChevronDown size={10} strokeWidth={2.5} className={'lg-cat-dd-arrow' + (modFilterOpen ? ' open' : '')} />
                  </button>
                  {modFilterOpen && (
                    <div className="lg-cat-dd-menu">
                      {modFilterItems.map(item => (
                        <button
                          key={item.id}
                          className={'lg-cat-dd-item' + (modFilter === item.id ? ' on' : '')}
                          onClick={() => { setModFilter(item.id); setModFilterOpen(false); }}
                        >
                          {item.label}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
            <div className="lg-actions">
              <button className="lg-btn" onClick={copyAll} data-tooltip={t('logs.copyTip')} data-tooltip-pos="bottom">
                <Copy size={14} strokeWidth={2} />
              </button>
              <button className={'lg-btn' + (exporting ? ' on' : '')} onClick={exportLogs} disabled={exporting}
                data-tooltip={t('logs.exportTip')} data-tooltip-pos="bottom">
                {exporting ? <span className="mini-spin" /> : <Download size={14} strokeWidth={2} />}
              </button>
            </div>
          </div>
        </div>

        <div className="lg-body" ref={bodyRef}>
          {showErrors ? (
            <div className="lg-errors-view">
              <div className="lg-errors-head">
                <span>{t('logs.errorSnapshots')} ({errorSnapshots.length})</span>
                <button className="lg-btn" onClick={() => setShowErrors(false)}>{t('logs.backToLogs')}</button>
              </div>
              <div className="lg-split">
                <div className="lg-file-list">
                  {errorSnapshots.length > 0 ? errorSnapshots.map(f => (
                    <div key={f.name} className={'lg-file-row' + (selectedError === f.name ? ' active' : '')} onClick={() => readError(f.name)}>
                      <button className="lg-file-item">
                        <span className="lg-file-name">{f.name}</span>
                        <span className="lg-file-meta">{formatSize(f.size)}</span>
                      </button>
                      <button className="lg-file-del" onClick={(e) => deleteError(f.name, e)} data-tooltip={t('logs.deleteSnapshot')} data-tooltip-pos="right">
                        <Trash2 size={12} strokeWidth={2} />
                      </button>
                    </div>
                  )) : <div className="lg-empty">{t('logs.noSnapshots')}</div>}
                </div>
                <div className="lg-file-content">
                  {selectedError && errorContent ? (
                    <pre className="lg-pre">{errorContent}</pre>
                  ) : <div className="lg-empty">{t('logs.selectFile')}</div>}
                  {selectedError && (
                    <div className="lg-file-actions">
                      <button className="lg-btn" onClick={() => navigator.clipboard.writeText(errorContent)}>
                        <Copy size={12} strokeWidth={2} />
                      </button>
                    </div>
                  )}
                </div>
              </div>
            </div>
          ) : (
            <div className="lg-table-wrap">
              <div className="lg-table-header">
                <span className="lg-th lg-th-code">{t('logs.level')}</span>
                <span className="lg-th lg-th-time">{t('logs.time')}</span>
                {(activeTab === 'app' || activeTab === 'all') && <span className="lg-th lg-th-mod">{t('logs.module')}</span>}
                <span className="lg-th lg-th-msg">{t('logs.message')}</span>
              </div>
              <div className="lg-table-body" ref={tableBodyRef}>
                {loading && rows.length === 0 ? (
                  <div className="lg-empty">{t('common.loading')}</div>
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
                            <span className="lg-level-label">{l.level || 'INFO'}</span>
                          )}
                        </span>
                        <span className="lg-td lg-td-time">{formatTime(l.timestamp || '')}</span>
                        {(activeTab === 'app' || activeTab === 'all') && <span className="lg-td lg-td-mod">{l.category || modLabel(l.table, t)}</span>}
                        <span className="lg-td lg-td-msg">{l.message || ''}</span>
                      </div>
                    );
                  })
                ) : (
                  <div className="lg-empty">{t('logs.noLogs')}</div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
