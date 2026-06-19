import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { api } from '../../api';

const CATS = ['all', 'zapret', 'network', 'config'];
const CAT_LABELS = { all: 'Все', zapret: 'Zapret', network: 'Сеть', config: 'Конфиг' };

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

export default function LogDrawer({ open, onClose }) {
  const [cat, setCat] = useState('all');
  const [search, setSearch] = useState('');
  const [raw, setRaw] = useState([]);
  const bodyRef = useRef(null);
  const prevLen = useRef(0);

  useEffect(() => {
    if (!open) return;
    const load = async () => {
      let be = [];
      if (cat === 'all') {
        const results = await Promise.all(
          ['zapret', 'network', 'config', 'tray'].map(c =>
            api('GET', `/api/logs?category=${c}&lines=150`)
          )
        );
        be = results.flatMap(d => (d?.lines || []).map(l => ({ ...l, source: 'be' })));
      } else {
        const d = await api('GET', `/api/logs?category=${cat}&lines=250`);
        be = (d?.lines || []).map(l => ({ ...l, source: 'be' }));
      }
      const fe = getFe().map(l => ({ ...l, source: 'fe' }));
      setRaw([...be, ...fe].slice(-400));
    };
    load();
    const iv = setInterval(load, 3000);
    return () => clearInterval(iv);
  }, [open, cat]);

  const filtered = useMemo(() => {
    if (!search) return raw;
    const s = search.toLowerCase();
    return raw.filter(l => (l.message || '').toLowerCase().includes(s));
  }, [raw, search]);

  useEffect(() => {
    const el = bodyRef.current;
    if (!el || !open) return;
    if (filtered.length !== prevLen.current) {
      el.scrollTop = el.scrollHeight;
      prevLen.current = filtered.length;
    }
  }, [filtered, open]);

  const copyAll = () => {
    const text = filtered.map(l => `[${l.time}] [${l.level}] ${l.source === 'fe' ? '[FE] ' : ''}${l.message}`).join('\n');
    navigator.clipboard.writeText(text);
  };

  const errCount = raw.filter(l => (l.level || '').toLowerCase() === 'error').length;

  return (
    <>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <div className={'lg-drawer' + (open ? ' open' : '')}>
        <div className="lg-head">
          <span className="lg-title">Логи</span>
          <span className="lg-count">{filtered.length}</span>
          {errCount > 0 && <span className="lg-err">{errCount} err</span>}
          <div className="lg-spacer" />
          <button className={'lg-chip' + (cat === 'all' ? ' on' : '')} onClick={() => setCat('all')}>Все</button>
          {CATS.filter(c => c !== 'all').map(c => (
            <button key={c} className={'lg-chip' + (cat === c ? ' on' : '')} onClick={() => setCat(c)}>{CAT_LABELS[c]}</button>
          ))}
          <input className="lg-search" placeholder="Поиск..." value={search} onChange={e => setSearch(e.target.value)} />
          <button className="lg-btn" onClick={copyAll} title="Копировать">⎘</button>
          <button className="lg-btn" onClick={() => { sessionStorage.removeItem(FE_KEY); setRaw([]); }} title="Очистить FE">⊘</button>
          <button className="lg-x" onClick={onClose}>✕</button>
        </div>
        <div className="lg-body" ref={bodyRef}>
          {filtered.length > 0 ? filtered.map((l, i) => {
            const lv = (l.level || 'INFO').toLowerCase();
            return (
              <div key={i} className={'lg-row ' + lv}>
                <span className="lg-time">{l.time || ''}</span>
                <span className={'lg-lv ' + lv}>{lv === 'error' ? 'E' : lv === 'warn' ? 'W' : 'I'}</span>
                {l.source === 'fe' && <span className="lg-fe">FE</span>}
                <span className="lg-msg">{l.message || ''}</span>
              </div>
            );
          }) : <div className="lg-empty">{search ? 'Ничего не найдено' : 'Нет логов'}</div>}
        </div>
      </div>
    </>
  );
}
