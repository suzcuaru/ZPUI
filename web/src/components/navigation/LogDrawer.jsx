import { useState, useEffect, useRef } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { X, SlidersHorizontal, Trash2, Copy } from 'lucide-react';

const CATS = ['all', 'app', 'zapret', 'network', 'availability', 'config', 'tray', 'xboxdns'];
const ALL_FETCH = ['app', 'zapret', 'network', 'availability', 'tray', 'config', 'xboxdns'];
const DEBUG_CATS = ['app', 'zapret', 'network', 'availability'];

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
  const { t } = useT();
  const [tab, setTab] = useState('live');
  const [cat, setCat] = useState('all');
  const [raw, setRaw] = useState([]);
  const [debugState, setDebugState] = useState({});
  const [showDebug, setShowDebug] = useState(false);
  const [errorFiles, setErrorFiles] = useState([]);
  const [selectedError, setSelectedError] = useState(null);
  const [errorContent, setErrorContent] = useState('');
  const [archiveFiles, setArchiveFiles] = useState([]);
  const [selectedArchive, setSelectedArchive] = useState(null);
  const [archiveContent, setArchiveContent] = useState('');
  const bodyRef = useRef(null);
  const prevLen = useRef(0);

  useEffect(() => {
    if (!open) return;
    api('GET', '/api/logs/debug').then(d => {
      if (d?.categories) setDebugState(d.categories);
    });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    if (tab === 'live') {
      const load = async () => {
        let be = [];
        if (cat === 'all') {
          const results = await Promise.all(
            ALL_FETCH.map(c => api('GET', `/api/logs?category=${c}&lines=100`))
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
    }
    if (tab === 'errors') {
      const load = async () => {
        const d = await api('GET', '/api/logs/errors');
        if (d?.files) setErrorFiles(d.files);
      };
      load();
    }
    if (tab === 'archive') {
      const load = async () => {
        const d = await api('GET', '/api/logs/archive');
        if (d?.files) setArchiveFiles(d.files);
      };
      load();
    }
  }, [open, tab, cat]);

  const toggleDebug = async (category) => {
    const next = !debugState[category];
    setDebugState(prev => ({ ...prev, [category]: next }));
    await api('POST', '/api/logs/debug', { category, enabled: next });
  };

  const readError = async (name) => {
    setSelectedError(name);
    setErrorContent('');
    const d = await api('GET', `/api/logs/error?name=${encodeURIComponent(name)}`);
    if (d?.content) setErrorContent(d.content);
  };

  const readArchive = async (name) => {
    setSelectedArchive(name);
    setArchiveContent('');
    const d = await api('GET', `/api/logs/archive/read?name=${encodeURIComponent(name)}`);
    if (d?.content) setArchiveContent(d.content);
  };

  const filtered = raw;

  useEffect(() => {
    const el = bodyRef.current;
    if (!el || !open || tab !== 'live') return;
    if (filtered.length !== prevLen.current) {
      el.scrollTop = el.scrollHeight;
      prevLen.current = filtered.length;
    }
  }, [filtered, open, tab]);

  const copyAll = () => {
    let text;
    if (tab === 'live') {
      text = filtered.map(l => `[${l.time}] [${l.level}] ${l.source === 'fe' ? '[FE] ' : ''}${l.message}`).join('\n');
    } else if (tab === 'errors' && errorContent) {
      text = errorContent;
    } else if (tab === 'archive' && archiveContent) {
      text = archiveContent;
    }
    if (text) navigator.clipboard.writeText(text);
  };

  const clearAll = async () => {
    sessionStorage.removeItem(FE_KEY);
    await api('POST', '/api/logs/clear');
    setRaw([]);
    setErrorFiles([]);
    setSelectedError(null);
    setErrorContent('');
  };

  const errCount = raw.filter(l => (l.level || '').toLowerCase() === 'error').length;
  const fmtSize = (b) => b < 1024 ? b + ' B' : (b / 1024).toFixed(1) + ' KB';
  const catLabel = (c) => c === 'all' ? t('logs.all') : c.charAt(0).toUpperCase() + c.slice(1);

  return (
    <>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <div className={'lg-drawer' + (open ? ' open' : '')}>
        <div className="lg-head">
          <span className="lg-head-title">{t('logs.title')}</span>
          {errCount > 0 && <span className="lg-head-badge">{errCount}</span>}
          <div className="lg-head-tabs">
            <button className={'lg-tab' + (tab === 'live' ? ' on' : '')} onClick={() => setTab('live')}>
              {t('logs.title')}
            </button>
            <button className={'lg-tab' + (tab === 'errors' ? ' on' : '')} onClick={() => setTab('errors')}>
              {t('logs.errors', { defaultValue: 'Errors' })}
              {errorFiles.length > 0 && <span className="lg-tab-badge">{errorFiles.length}</span>}
            </button>
            <button className={'lg-tab' + (tab === 'archive' ? ' on' : '')} onClick={() => setTab('archive')}>
              {t('logs.archive', { defaultValue: 'Archive' })}
            </button>
          </div>
          <div className="lg-spacer" />
          <button className="lg-head-close" data-tooltip={t('common.close')} onClick={onClose}><X size={16} strokeWidth={2.5} /></button>
        </div>

        {tab === 'live' && (
          <>
            <div className="lg-toolbar">
              <div className="lg-cats">
                {CATS.map(c => (
                  <button key={c} className={'lg-chip' + (cat === c ? ' on' : '')} onClick={() => setCat(c)}>
                    {catLabel(c)}
                  </button>
                ))}
              </div>
              <div className="lg-actions">
                <button
                  className={'lg-btn' + (showDebug || Object.values(debugState).some(Boolean) ? ' on' : '')}
                  onClick={() => setShowDebug(!showDebug)}
                  data-tooltip={t('logs.debugMode', { defaultValue: 'Debug mode' })}
                ><SlidersHorizontal size={15} strokeWidth={2} /></button>
                <button className="lg-btn" onClick={clearAll} data-tooltip={t('common.clear')}><Trash2 size={15} strokeWidth={2} /></button>
                <button className="lg-btn" onClick={copyAll} data-tooltip={t('common.copy')}><Copy size={15} strokeWidth={2} /></button>
              </div>
            </div>
            {showDebug && (
              <div className="lg-debug-bar">
                <span className="lg-debug-label">{t('logs.debugMode', { defaultValue: 'Debug' })}:</span>
                {DEBUG_CATS.map(c => (
                  <button
                    key={c}
                    className={'lg-dbg' + (debugState[c] ? ' on' : '')}
                    onClick={() => toggleDebug(c)}
                    data-tooltip={debugState[c] ? t('common.disable') : t('common.enable')}
                  >{c}</button>
                ))}
              </div>
            )}
          </>
        )}

        <div className="lg-body" ref={bodyRef}>
          {tab === 'live' && (
            filtered.length > 0 ? filtered.map((l, i) => {
              const lv = (l.level || 'INFO').toLowerCase();
              return (
                <div key={i} className={'lg-row ' + lv}>
                  <span className="lg-time">{l.time || ''}</span>
                  <span className={'lg-lv ' + lv}>{lv === 'error' ? 'E' : lv === 'warn' ? 'W' : lv === 'debug' ? 'D' : 'I'}</span>
                  {l.source === 'fe' && <span className="lg-fe">FE</span>}
                  <span className="lg-msg">{l.message || ''}</span>
                </div>
              );
            }) : <div className="lg-empty">{t('logs.noLogs')}</div>
          )}

          {tab === 'errors' && (
            <div className="lg-split">
              <div className="lg-file-list">
                {errorFiles.length > 0 ? errorFiles.map(f => (
                  <button key={f.name} className={'lg-file-item' + (selectedError === f.name ? ' active' : '')} onClick={() => readError(f.name)}>
                    <span className="lg-file-name">{f.name}</span>
                    <span className="lg-file-meta">{fmtSize(f.size)}</span>
                  </button>
                )) : <div className="lg-empty">{t('logs.noErrors', { defaultValue: 'No errors' })}</div>}
              </div>
              <div className="lg-file-content">
                {selectedError && errorContent ? (
                  <pre className="lg-pre">{errorContent}</pre>
                ) : <div className="lg-empty">{t('logs.selectFile', { defaultValue: 'Select a file' })}</div>}
                {selectedError && (
                  <div className="lg-file-actions">
                    <button className="lg-btn" data-tooltip={t('common.copy')} onClick={() => navigator.clipboard.writeText(errorContent)}>⎘</button>
                  </div>
                )}
              </div>
            </div>
          )}

          {tab === 'archive' && (
            <div className="lg-split">
              <div className="lg-file-list">
                {archiveFiles.length > 0 ? archiveFiles.map(f => (
                  <button key={f.name} className={'lg-file-item' + (selectedArchive === f.name ? ' active' : '')} onClick={() => readArchive(f.name)}>
                    <span className="lg-file-name">{f.name}</span>
                    <span className="lg-file-meta">{fmtSize(f.size)}</span>
                  </button>
                )) : <div className="lg-empty">{t('logs.noArchive', { defaultValue: 'No archives' })}</div>}
              </div>
              <div className="lg-file-content">
                {selectedArchive && archiveContent ? (
                  <pre className="lg-pre">{archiveContent}</pre>
                ) : <div className="lg-empty">{t('logs.selectFile', { defaultValue: 'Select a file' })}</div>}
                {selectedArchive && (
                  <div className="lg-file-actions">
                    <button className="lg-btn" data-tooltip={t('common.copy')} onClick={() => navigator.clipboard.writeText(archiveContent)}>⎘</button>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
