import { useState, useEffect, useRef, useMemo } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { X, Trash2, Copy, Download, Search, AlertTriangle, ChevronDown, Check } from 'lucide-react';
import { useConfirm } from '../ui/ConfirmDialog';

const CATS = ['all', 'app', 'zapret', 'network', 'availability', 'tray', 'xboxdns'];
const ALL_FETCH = ['app', 'zapret', 'network', 'availability', 'tray', 'xboxdns'];

const FE_KEY = '__zpui_fe_logs';
const MAX_FE = 500;

function getFe() {
  try { return JSON.parse(sessionStorage.getItem(FE_KEY) || '[]'); } catch { return []; }
}
function addFe(l, m) {
  try {
    const logs = getFe();
    const time = new Date().toLocaleTimeString('ru-RU', { hour12: false });
    logs.push({ time, level: l, message: m, source: 'fe' });
    if (logs.length > MAX_FE) logs.shift();
    sessionStorage.setItem(FE_KEY, JSON.stringify(logs));
  } catch {}
}
if (typeof window !== 'undefined' && !window.__zpui_log_init) {
  window.__zpui_log_init = true;
  const oL = console.log, oE = console.error, oW = console.warn;
  console.log = (...a) => { addFe('INFO', a.map(String).join(' ')); oL(...a); };
  console.error = (...a) => { addFe('ERROR', a.map(String).join(' ')); oE(...a); };
  console.warn = (...a) => { addFe('WARN', a.map(String).join(' ')); oW(...a); };
  window.addEventListener('error', e => addFe('ERROR', `${e.message} @ ${e.filename}:${e.lineno}`));
  window.addEventListener('unhandledrejection', e => addFe('ERROR', `Promise: ${e.reason}`));
}

const LEVELS = ['ALL', 'ERROR', 'WARN', 'INFO'];

function CatDropdown({ cat, setCat, catLabel }) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e) => {
      if (ref.current && !ref.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  return (
    <div className="lg-cat-dd" ref={ref}>
      <button className="lg-cat-dd-btn" onClick={() => setOpen(!open)}>
        <span>{catLabel(cat)}</span>
        <ChevronDown size={12} strokeWidth={2.5} className={'lg-cat-dd-arrow' + (open ? ' open' : '')} />
      </button>
      {open && (
        <div className="lg-cat-dd-menu">
          {CATS.map(c => (
            <button
              key={c}
              className={'lg-cat-dd-item' + (cat === c ? ' active' : '')}
              onClick={() => { setCat(c); setOpen(false); }}
            >
              <span>{catLabel(c)}</span>
              {cat === c && <Check size={12} strokeWidth={2.5} />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export default function LogDrawer({ open, onClose }) {
  const { t } = useT();
  const confirm = useConfirm();
  const [cat, setCat] = useState('all');
  const [level, setLevel] = useState('ALL');
  const [search, setSearch] = useState('');
  const [raw, setRaw] = useState([]);
  const [errorSnapshots, setErrorSnapshots] = useState([]);
  const [showErrors, setShowErrors] = useState(false);
  const [selectedError, setSelectedError] = useState(null);
  const [errorContent, setErrorContent] = useState('');
  const [exporting, setExporting] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const bodyRef = useRef(null);
  const prevLen = useRef(0);

  useEffect(() => {
    if (!open) return;

    const load = async () => {
      let be = [];
      if (cat === 'all') {
        const results = await Promise.all(
          ALL_FETCH.map(c => api('GET', `/api/logs?category=${c}&lines=100`))
        );
        be = results.flatMap((d, i) => {
          const categoryName = ALL_FETCH[i];
          return (d?.lines || []).map(l => ({
            ...l,
            source: 'be',
            category: l.category || categoryName,
          }));
        });
      } else {
        const d = await api('GET', `/api/logs?category=${cat}&lines=250`);
        be = (d?.lines || []).map(l => ({ ...l, source: 'be', category: l.category || cat }));
      }
      const fe = getFe().map(l => ({ ...l, source: 'fe', category: 'fe' }));
      setRaw([...be, ...fe].slice(-500));
    };
    load();
    const iv = setInterval(load, 3000);
    return () => clearInterval(iv);
  }, [open, cat]);

  useEffect(() => {
    if (!open) return;
    const loadErr = async () => {
      const d = await api('GET', '/api/logs/errors');
      if (d?.files) setErrorSnapshots(d.files);
    };
    loadErr();
  }, [open]);

  const readError = async (name) => {
    setSelectedError(name);
    setErrorContent('');
    const d = await api('GET', `/api/logs/error?name=${encodeURIComponent(name)}`);
    if (d?.content) setErrorContent(d.content);
  };

  const deleteError = async (name, e) => {
    e.stopPropagation();
    const d = await api('POST', '/api/logs/error/delete', { name });
    if (d?.error) return;
    const refresh = await api('GET', '/api/logs/errors');
    if (refresh?.files) setErrorSnapshots(refresh.files);
    if (selectedError === name) {
      setSelectedError(null);
      setErrorContent('');
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return raw.filter(l => {
      if (level !== 'ALL' && (l.level || '').toUpperCase() !== level) return false;
      if (q) {
        const msg = (l.message || '').toLowerCase();
        const c = (l.category || '').toLowerCase();
        if (!msg.includes(q) && !c.includes(q)) return false;
      }
      return true;
    });
  }, [raw, level, search]);

  const counts = useMemo(() => {
    const c = { ALL: raw.length, ERROR: 0, WARN: 0, INFO: 0 };
    raw.forEach(l => {
      const lv = (l.level || 'INFO').toUpperCase();
      if (c[lv] !== undefined) c[lv]++;
      else c.INFO++;
    });
    return c;
  }, [raw]);

  useEffect(() => {
    const el = bodyRef.current;
    if (!el || !open || !autoScroll) return;
    if (filtered.length !== prevLen.current) {
      el.scrollTop = el.scrollHeight;
      prevLen.current = filtered.length;
    }
  }, [filtered, open, autoScroll]);

  const copyAll = () => {
    const text = filtered.map(l => `[${l.time}] [${l.level}] ${l.source === 'fe' ? '[FE] ' : ''}${l.message}`).join('\n');
    if (text) navigator.clipboard.writeText(text);
  };

  const handleClear = async () => {
    if (clearing) return;
    const confirmMsg = cat === 'all'
      ? t('logs.clearAllConfirm')
      : t('logs.clearCatConfirm', { cat });
    if (!await confirm({ message: confirmMsg, variant: 'danger', confirmText: t('logs.clearBtn') })) return;

    setClearing(true);
    sessionStorage.removeItem(FE_KEY);
    const d = await api('POST', '/api/logs/clear-bucket', { category: cat });
    setClearing(false);

    if (d?.error) {
      console.error('Clear failed:', d.error);
      return;
    }

    setRaw([]);
    setTimeout(async () => {
      let be = [];
      if (cat === 'all') {
        const results = await Promise.all(
          ALL_FETCH.map(c => api('GET', `/api/logs?category=${c}&lines=100`))
        );
        be = results.flatMap((d, i) => (d?.lines || []).map(l => ({
          ...l, source: 'be', category: l.category || ALL_FETCH[i],
        })));
      } else {
        const d = await api('GET', `/api/logs?category=${cat}&lines=250`);
        be = (d?.lines || []).map(l => ({ ...l, source: 'be', category: l.category || cat }));
      }
      setRaw(be);
    }, 200);
  };

  const exportLogs = async () => {
    setExporting(true);
    const d = await api('POST', '/api/logs/export');
    setExporting(false);
    if (d?.path) {
      console.log('Logs exported to', d.path);
    }
  };

  const fmtSize = (b) => b < 1024 ? b + ' B' : (b / 1024).toFixed(1) + ' KB';
  const catLabel = (c) => c === 'all' ? t('logs.all') : c.charAt(0).toUpperCase() + c.slice(1);

  return (
    <>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <div className={'lg-drawer' + (open ? ' open' : '')}>
        <div className="lg-head">
          <span className="lg-head-title">{t('logs.title')}</span>
          <CatDropdown cat={cat} setCat={setCat} catLabel={catLabel} />
          {counts.ERROR > 0 && <span className="lg-head-badge">{counts.ERROR}</span>}
          <div className="lg-spacer" />
          <button
            className={'lg-head-errors-btn' + (errorSnapshots.length > 0 ? ' has-errors' : '')}
            onClick={() => setShowErrors(!showErrors)}
            aria-label="Срезы ошибок"
          >
            <AlertTriangle size={14} strokeWidth={2} />
            {errorSnapshots.length > 0 && <span className="lg-err-count">{errorSnapshots.length}</span>}
          </button>
          <button className="lg-head-close" onClick={onClose}><X size={16} strokeWidth={2.5} /></button>
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
                  {lv}{lv !== 'ALL' && counts[lv] > 0 && <span className="lg-level-count">{counts[lv]}</span>}
                </button>
              ))}
            </div>
            <div className="lg-actions">
              <button
                className={'lg-btn' + (autoScroll ? ' on' : '')}
                onClick={() => setAutoScroll(!autoScroll)}
                title={autoScroll ? 'Автопрокрутка вкл' : 'Автопрокрутка выкл'}
                aria-label="Автопрокрутка"
              >
                ↓
              </button>
              <button
                className={'lg-btn' + (clearing ? ' on' : '')}
                onClick={handleClear}
                disabled={clearing}
                title={clearing ? 'Очистка...' : (cat === 'all' ? 'Очистить все логи' : `Очистить «${catLabel(cat)}»`)}
                aria-label="Очистить"
              >
                {clearing ? <span className="mini-spin" /> : <Trash2 size={15} strokeWidth={2} />}
              </button>
              <button className={'lg-btn' + (exporting ? ' on' : '')} onClick={exportLogs} disabled={exporting} title="Экспорт в ZIP">
                {exporting ? <span className="mini-spin" /> : <Download size={15} strokeWidth={2} />}
              </button>
              <button className="lg-btn" onClick={copyAll} title={t('common.copy')}><Copy size={15} strokeWidth={2} /></button>
            </div>
            <div className="lg-search-wrap">
              <Search size={13} strokeWidth={2} className="lg-search-icon" />
              <input
                type="text"
                className="lg-search"
                placeholder={t('logs.searchPlaceholder') || 'Поиск...'}
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
            </div>
          </div>
          <div className="lg-filter-info">
            {filtered.length !== raw.length ? (
              <span>Показано {filtered.length} из {raw.length}</span>
            ) : (
              <span>Всего {raw.length}</span>
            )}
          </div>
        </div>

        <div className="lg-body" ref={bodyRef}>
          {showErrors ? (
            <div className="lg-errors-view">
              <div className="lg-errors-head">
                <span>Срезы ошибок ({errorSnapshots.length})</span>
                <button className="lg-btn" onClick={() => setShowErrors(false)}>← К логам</button>
              </div>
              <div className="lg-split">
                <div className="lg-file-list">
                  {errorSnapshots.length > 0 ? errorSnapshots.map(f => (
                    <div key={f.name} className={'lg-file-row' + (selectedError === f.name ? ' active' : '')} onClick={() => readError(f.name)}>
                      <button className="lg-file-item">
                        <span className="lg-file-name">{f.name}</span>
                        <span className="lg-file-meta">{fmtSize(f.size)}</span>
                      </button>
                      <button className="lg-file-del" onClick={(e) => deleteError(f.name, e)} title="Удалить срез">
                        <Trash2 size={12} strokeWidth={2} />
                      </button>
                    </div>
                  )) : <div className="lg-empty">Нет срезов ошибок</div>}
                </div>
                <div className="lg-file-content">
                  {selectedError && errorContent ? (
                    <pre className="lg-pre">{errorContent}</pre>
                  ) : <div className="lg-empty">Выберите файл слева</div>}
                  {selectedError && (
                    <div className="lg-file-actions">
                      <button className="lg-btn" title={t('common.copy')} onClick={() => navigator.clipboard.writeText(errorContent)}>⎘</button>
                    </div>
                  )}
                </div>
              </div>
            </div>
          ) : (
            filtered.length > 0 ? filtered.map((l, i) => {
              const lv = (l.level || 'INFO').toLowerCase();
              return (
                <div key={i} className={'lg-row ' + lv}>
                  <span className={'lg-dot ' + lv} />
                  <span className="lg-time">{l.time || ''}</span>
                  {l.source === 'fe' && <span className="lg-fe">FE</span>}
                  <span className="lg-msg">{l.message || ''}</span>
                </div>
              );
            }) : <div className="lg-empty">{search || level !== 'ALL' ? (t('logs.nothingFound') || 'Ничего не найдено') : (t('logs.noLogs') || 'Нет логов')}</div>
          )}
        </div>
      </div>
    </>
  );
}
