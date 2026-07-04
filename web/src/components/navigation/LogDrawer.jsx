import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';

const CATS = ['all', 'main', 'app', 'zapret', 'network', 'tray', 'config', 'xboxdns'];

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
  const CAT_LABELS = {
    all: t('logs.all'),
    main: 'Main',
    app: 'App',
    zapret: 'Zapret',
    network: t('logs.network'),
    tray: 'Tray',
    config: t('logs.config'),
    xboxdns: 'Xbox DNS',
  };
  const [tab, setTab] = useState('live');
  const [cat, setCat] = useState('all');
  const [raw, setRaw] = useState([]);
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
    if (tab === 'live') {
      const load = async () => {
        let be = [];
         if (cat === 'all') {
           const results = await Promise.all(
             ['main', 'app', 'zapret', 'network', 'tray', 'config', 'xboxdns'].map(c =>
               api('GET', `/api/logs?category=${c}&lines=100`)
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

  const errCount = raw.filter(l => (l.level || '').toLowerCase() === 'error').length;

  const fmtSize = (b) => b < 1024 ? b + ' B' : (b / 1024).toFixed(1) + ' KB';

  return (
    <>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <div className={'lg-drawer' + (open ? ' open' : '')}>
        <div className="lg-head">
          <div className="lg-tabs">
            <button className={'lg-tab' + (tab === 'live' ? ' on' : '')} onClick={() => setTab('live')}>{t('logs.title')}</button>
            <button className={'lg-tab' + (tab === 'errors' ? ' on' : '')} onClick={() => setTab('errors')}>
              {t('logs.errors', { defaultValue: 'Errors' })}
              {errorFiles.length > 0 && <span className="lg-tab-badge">{errorFiles.length}</span>}
            </button>
            <button className={'lg-tab' + (tab === 'archive' ? ' on' : '')} onClick={() => setTab('archive')}>
              {t('logs.archive', { defaultValue: 'Archive' })}
            </button>
          </div>
          <div className="lg-spacer" />
          <button className="lg-btn" onClick={copyAll} data-tooltip={t('common.copy')}>⎘</button>
          <button className="lg-btn lg-close" onClick={onClose}>✕</button>
        </div>

        {tab === 'live' && (
          <div className="lg-subbar">
            {CATS.map(c => (
              <button key={c} className={'lg-chip' + (cat === c ? ' on' : '')} onClick={() => setCat(c)}>{CAT_LABELS[c]}</button>
            ))}
            {errCount > 0 && <span className="lg-err">{errCount} err</span>}
            <button className="lg-btn" onClick={async () => {
              sessionStorage.removeItem(FE_KEY);
              await api('POST', '/api/logs/clear');
              setRaw([]);
            }}>⊘</button>
          </div>
        )}

        <div className="lg-body" ref={bodyRef}>
          {tab === 'live' && (
            filtered.length > 0 ? filtered.map((l, i) => {
              const lv = (l.level || 'INFO').toLowerCase();
              return (
                <div key={i} className={'lg-row ' + lv}>
                  <span className="lg-time">{l.time || ''}</span>
                  <span className={'lg-lv ' + lv}>{lv === 'error' ? 'E' : lv === 'warn' ? 'W' : 'I'}</span>
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
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
