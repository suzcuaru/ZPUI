import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { api } from '../../api';

const CATS = ['all', 'zapret', 'network', 'web', 'config'];
const CAT_LABELS = { all: 'Все', zapret: 'Zapret', network: 'Сеть', web: 'Веб', config: 'Конфиг' };
const CAT_TOOLTIPS = { all: 'Все категории', zapret: 'Логи Zapret (DPI bypass)', network: 'Сетевые логи', web: 'Логи веб-сервера', config: 'Логи конфигурации' };

const FE_KEY = '__zpui_fe_logs';
const MAX_FE = 500;

function getFe() {
  try { return JSON.parse(sessionStorage.getItem(FE_KEY) || '[]'); } catch { return []; }
}

function addFe(level, msg) {
  try {
    const logs = getFe();
    const time = new Date().toLocaleTimeString('ru-RU', { hour12: false });
    logs.push({ time, level, message: msg, source: 'fe' });
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

/* Tooltip wrapper */
function Tip({ text, children }) {
  return (
    <div className="log-filter-tooltip">
      {children}
      <span className="log-filter-tooltip-text">{text}</span>
    </div>
  );
}

export default function LogDrawer({ open, onClose }) {
  const [cat, setCat] = useState('all');
  const [onlyErrors, setOnlyErrors] = useState(false);
  const [search, setSearch] = useState('');
  const [newest, setNewest] = useState(true);
  const [raw, setRaw] = useState([]);
  const [render, setRender] = useState(false);
  const bodyRef = useRef(null);
  const userScrolledRef = useRef(false);
  const prevCountRef = useRef(0);

  /* mount/unmount animation */
  useEffect(() => {
    if (open) {
      setRender(true);
    } else {
      const t = setTimeout(() => setRender(false), 250);
      return () => clearTimeout(t);
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const load = async () => {
      let be = [];
      if (cat === 'all') {
        const results = await Promise.all(
          ['zapret', 'network', 'config', 'tray', 'web'].map(c =>
            api('GET', `/api/logs?category=${c}&lines=150`)
          )
        );
        be = results.flatMap((d) => (d?.lines || []).map(l => ({ ...l, source: 'be' })));
      } else if (cat !== 'web') {
        const d = await api('GET', `/api/logs?category=${cat}&lines=250`);
        be = (d?.lines || []).map(l => ({ ...l, source: 'be' }));
      }
      const fe = getFe().map(l => ({ ...l, source: 'fe' }));
      let combined = [...be, ...fe];
      if (cat === 'all' || cat === 'web') {
        combined.sort((a, b) => (a.time || '').localeCompare(b.time || ''));
      }
      setRaw(combined.slice(-400));
    };
    load();
    const iv = setInterval(load, 2000);
    return () => clearInterval(iv);
  }, [open, cat]);

  const filtered = useMemo(() => {
    let r = raw;
    if (onlyErrors) r = r.filter(l => (l.level || '').toLowerCase() === 'error');
    if (search) {
      const s = search.toLowerCase();
      r = r.filter(l => (l.message || '').toLowerCase().includes(s));
    }
    return newest ? [...r].reverse() : r;
  }, [raw, onlyErrors, search, newest]);

  /* auto-scroll: only if user hasn't scrolled manually AND new logs arrived */
  useEffect(() => {
    const el = bodyRef.current;
    if (!el || !open) return;
    const isNew = filtered.length !== prevCountRef.current;
    if (isNew && !userScrolledRef.current) {
      el.scrollTop = newest ? 0 : el.scrollHeight;
    }
    prevCountRef.current = filtered.length;
  }, [filtered, open, newest]);

  /* reset user-scroll flag on cat/search/order change */
  useEffect(() => {
    userScrolledRef.current = false;
    prevCountRef.current = 0;
  }, [cat, search, newest, onlyErrors]);

  const onBodyScroll = useCallback(() => {
    const el = bodyRef.current;
    if (!el) return;
    const atTop = el.scrollTop <= 5;
    const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 5;
    if (newest) {
      userScrolledRef.current = !atTop;
    } else {
      userScrolledRef.current = !atBottom;
    }
  }, [newest]);

  if (!render) return null;

  const copyAll = () => {
    const text = filtered.map(l => `[${l.time}] [${l.level}] ${l.source === 'fe' ? '[FE] ' : ''}${l.message}`).join('\n');
    navigator.clipboard.writeText(text);
  };

  const errCount = raw.filter(l => (l.level || '').toLowerCase() === 'error').length;

  return (
    <>
      <div className={'drawer-overlay' + (open ? ' open' : '')} onClick={onClose}></div>
      <div className={'drawer' + (open ? ' open' : '')}>
        <div className="drawer-header">
          <div className="dr-h1">
            <span className="dr-title">Консоль</span>
            <span className="dr-count">{filtered.length}{errCount > 0 && <span className="dr-err">{errCount} err</span>}</span>
            <div className="dr-spacer"></div>
            <Tip text={newest ? 'Новые сверху' : 'Новые снизу'}>
              <button className="dr-btn" onClick={() => setNewest(!newest)}>{newest ? '↓9' : '↑1'}</button>
            </Tip>
            <Tip text={onlyErrors ? 'Показать все' : 'Только ошибки'}>
              <button className={'dr-btn' + (onlyErrors ? ' on' : '')} onClick={() => setOnlyErrors(!onlyErrors)}>ERR</button>
            </Tip>
            <Tip text="Копировать все">
              <button className="dr-btn" onClick={copyAll}>⎘</button>
            </Tip>
            <Tip text="Очистить логи FE">
              <button className="dr-btn" onClick={() => { sessionStorage.removeItem(FE_KEY); setRaw([]); }}>⊘</button>
            </Tip>
            <button className="dr-x" onClick={onClose}>✕</button>
          </div>
          <div className="dr-h2">
            <div className="dr-cats">
              {CATS.map(c => (
                <Tip key={c} text={CAT_TOOLTIPS[c]}>
                  <button className={'dr-chip' + (cat === c ? ' on' : '')} onClick={() => setCat(c)}>{CAT_LABELS[c]}</button>
                </Tip>
              ))}
            </div>
            <input className="dr-search" type="text" placeholder="Поиск..." value={search} onChange={e => setSearch(e.target.value)} />
          </div>
        </div>
        <div className="drawer-body" ref={bodyRef} onScroll={onBodyScroll}>
          {filtered.length > 0 ? filtered.map((l, i) => {
            const lv = (l.level || 'INFO').toLowerCase();
            return (
              <div key={i} className={'dr-row ' + lv}>
                <span className="dr-time">{l.time || ''}</span>
                <span className={'dr-lv ' + lv}>{lv === 'error' ? 'E' : lv === 'warn' ? 'W' : 'I'}</span>
                {l.source === 'fe' && <span className="dr-fe">FE</span>}
                <span className="dr-msg">{l.message || ''}</span>
              </div>
            );
          }) : (
            <div className="dr-empty">{search ? 'Ничего не найдено' : 'Нет логов'}</div>
          )}
        </div>
      </div>
    </>
  );
}