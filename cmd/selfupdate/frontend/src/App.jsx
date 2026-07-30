import { useState, useEffect, useCallback } from 'react';

const api = () => window.go?.main?.App;

const ERROR_CODES = {
  download: 'UPD-E001', extract: 'UPD-E002', verify: 'UPD-E003',
  stop: 'UPD-E004', service: 'UPD-E005', restore: 'UPD-E006',
  apply: 'UPD-E007', unknown: 'UPD-E999',
};

function classifyError(msg) {
  if (!msg) return ERROR_CODES.unknown;
  const m = msg.toLowerCase();
  if (m.includes('скач') || m.includes('download') || m.includes('http') || m.includes('нет доступных url')) return ERROR_CODES.download;
  if (m.includes('распак') || m.includes('extract') || m.includes('заняты') || m.includes('zip')) return ERROR_CODES.extract;
  if (m.includes('checksum') || m.includes('sha') || m.includes('целост') || m.includes('вериф')) return ERROR_CODES.verify;
  if (m.includes('останов') || m.includes('stop') || m.includes('kill') || m.includes('процесс')) return ERROR_CODES.stop;
  if (m.includes('service') || m.includes('служб') || m.includes('sc ')) return ERROR_CODES.service;
  if (m.includes('restore') || m.includes('восстан')) return ERROR_CODES.restore;
  if (m.includes('apply') || m.includes('установ') || m.includes('install')) return ERROR_CODES.apply;
  return ERROR_CODES.unknown;
}

function fmtVer(v) {
  if (!v || v === 'unknown') return '—';
  return v.startsWith('v') ? v : 'v' + v;
}

const COMPONENT_META = {
  zpui:       { name: 'ZPUI',       desc: 'Приложение' },
  zapret:     { name: 'Zapret',     desc: 'Обход блокировок DPI' },
  selfupdate: { name: 'Центр обновлений', desc: 'Модуль обновлений' },
  report:     { name: 'Отчёты',     desc: 'Модуль отчётов' },
  security:   { name: 'Безопасность', desc: 'Модуль проверки' },
};

const Icon = {
  download: () => <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>,
  check: () => <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polyline points="20 6 9 17 4 12"/></svg>,
  refresh: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>,
  arrow: () => <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>,
  close: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>,
};

export default function App() {
  const [state, setState] = useState(null);
  const [theme, setTheme] = useState('system');
  const [loading, setLoading] = useState(true);
  const [checking, setChecking] = useState(false);
  const [checkedAt, setCheckedAt] = useState(null);
  const [dl, setDl] = useState({});
  const [apply, setApply] = useState({});
  const [notifications, setNotifications] = useState([]);

  const addNotification = useCallback((code, message) => {
    setNotifications(prev => [...prev.slice(-2), { id: Date.now(), code, message, ts: new Date().toLocaleTimeString('ru-RU', { hour12: false }) }]);
  }, []);
  const dismissNotification = useCallback((id) => setNotifications(prev => prev.filter(n => n.id !== id)), []);
  const copyNotification = useCallback((code, message) => {
    navigator.clipboard.writeText(`${code}: ${message}`).catch(() => {});
  }, []);

  const refresh = useCallback(async (retries = 5, delay = 200) => {
    const a = api();
    if (!a) {
      if (retries > 0) { await new Promise(r => setTimeout(r, delay)); return refresh(retries - 1, delay); }
      setLoading(false);
      setChecking(false);
      return;
    }
    setChecking(true);
    try {
      const s = await Promise.race([a.GetState(), new Promise((_, rej) => setTimeout(() => rej(new Error('timeout')), 60000))]);
      setState(s);
      if (s?.theme) setTheme(s.theme);
      if (s?.checked_at) setCheckedAt(s.checked_at);
    } catch (e) {
      console.error(e);
      addNotification('UPD-E999', 'Не удалось проверить версии');
    }
    finally { setLoading(false); setChecking(false); }
  }, [addNotification]);

  const handleRefresh = useCallback(() => { refresh(); }, [refresh]);

  useEffect(() => {
    refresh();
    const rt = window.runtime;
    if (!rt) return;
    const onDlProgress = (d) => { const t = d?.target || 'zpui'; setDl(p => ({ ...p, [t]: { s: 'downloading', pct: d?.percent || 0 } })); };
    const onDlDone = (d) => {
      const t = d?.target || 'zpui';
      if (d?.ok) {
        if (d?.no_update) { setDl(p => ({ ...p, [t]: { s: 'idle', pct: 0 } })); }
        else { setDl(p => ({ ...p, [t]: { s: 'downloaded', pct: 100 } })); }
        setTimeout(refresh, 500);
      } else { setDl(p => ({ ...p, [t]: { s: 'error', pct: p[t]?.pct } })); addNotification(classifyError(d?.error), d?.error || 'Не удалось скачать'); }
    };
    const onApplyProgress = (d) => { const t = d?.target || 'zpui'; setApply(p => ({ ...p, [t]: { s: 'applying', pct: d?.percent || 0, msg: d?.status || '' } })); };
    const onApplyDone = (d) => {
      const t = d?.target || 'zpui';
      if (d?.ok) { setApply(p => ({ ...p, [t]: { s: 'done', msg: 'Готово' } })); }
      else { setApply(p => ({ ...p, [t]: { s: 'error', msg: d?.error || 'Ошибка' } })); addNotification(d?.code || classifyError(d?.error), d?.error || 'Ошибка обновления'); }
    };
    rt.EventsOn('dl:progress', onDlProgress);
    rt.EventsOn('dl:done', onDlDone);
    rt.EventsOn('apply:progress', onApplyProgress);
    rt.EventsOn('apply:done', onApplyDone);
    return () => { ['dl:progress', 'dl:done', 'apply:progress', 'apply:done'].forEach(e => { try { rt.EventsOff(e); } catch {} }); };
  }, [refresh, addNotification]);

  const doDownload = (target) => { const a = api(); if (!a) return; setDl(p => ({ ...p, [target]: { s: 'downloading', pct: 0 } })); target === 'zpui' ? a.DownloadZPUI() : a.DownloadZapret(); };
  const doApply = (target) => { const a = api(); if (!a) return; setApply(p => ({ ...p, [target]: { s: 'applying', pct: 0, msg: 'Подготовка...' } })); target === 'zpui' ? a.ApplyZPUI() : a.ApplyZapret(); };

  useEffect(() => {
    const t = theme === 'system' ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : theme;
    document.documentElement.setAttribute('data-theme', t);
  }, [theme]);

  if (loading) return <div className="app"><div className="splash"><div className="spinner" /><span>Проверка версий…</span></div></div>;
  if (!state) return <div className="app"><div className="splash err">Не удалось получить состояние</div></div>;

  const components = [];
  if (state.zpui) {
    components.push({
      key: 'zpui',
      meta: COMPONENT_META.zpui,
      current: state.zpui.current,
      latest: state.zpui.cloud?.version,
      needsUpdate: state.zpui.update_needed,
      primary: true,
    });
  }
  if (state.zapret) {
    components.push({
      key: 'zapret',
      meta: COMPONENT_META.zapret,
      current: state.zapret.current,
      latest: state.zapret.cloud?.version,
      needsUpdate: state.zapret.update_needed,
      primary: true,
    });
  }
  for (const m of state.modules || []) {
    components.push({
      key: m.name,
      meta: COMPONENT_META[m.name] || { name: m.name, desc: 'Модуль' },
      current: m.current,
      latest: m.latest,
      needsUpdate: m.needs_update,
      primary: false,
    });
  }

  return (
    <div className="app">
      <header className="hdr">
        <span className="logo">ZPUI</span>
        <span className="hdr-sub">Центр обновлений</span>
        {checkedAt && <span className="hdr-checked">Проверено: {checkedAt}</span>}
        <button className="hdr-refresh" onClick={handleRefresh} title="Обновить" disabled={checking}>
          {checking ? <span className="spin-sm" /> : <Icon.refresh />}
        </button>
      </header>

      <main className="main">
        {notifications.length > 0 && (
          <div className="notifs">
            {notifications.map(n => (
              <div key={n.id} className="notif">
                <div className="notif-top">
                  <span className="notif-code">{n.code}</span>
                  <span className="notif-time">{n.ts}</span>
                  <button className="notif-x" onClick={() => dismissNotification(n.id)}><Icon.close /></button>
                </div>
                <div className="notif-msg">{n.message}</div>
                <button className="notif-copy" onClick={() => copyNotification(n.code, n.message)}>Копировать</button>
              </div>
            ))}
          </div>
        )}

        <div className="comp-list">
          {components.map((c, i) => (
            <ComponentRow
              key={c.key}
              index={i}
              meta={c.meta}
              current={c.current}
              latest={c.latest}
              needsUpdate={c.needsUpdate}
              primary={c.primary}
              target={c.key}
              dl={dl[c.key]}
              apply={apply[c.key]}
              onDownload={doDownload}
              onApply={doApply}
            />
          ))}
        </div>
      </main>
    </div>
  );
}

function ComponentRow({ index, meta, current, latest, needsUpdate, primary, target, dl, apply, onDownload, onApply }) {
  const isUnknown = !current || current === 'unknown';
  const downloading = dl?.s === 'downloading';
  const downloaded = dl?.s === 'downloaded';
  const dlError = dl?.s === 'error';
  const applying = apply?.s === 'applying';
  const applyDone = apply?.s === 'done';
  const applyError = apply?.s === 'error';

  let action;
  let statusBadge = null;

  if (primary) {
    if (applying) {
      action = null;
    } else if (applyDone) {
      action = <span className="badge-ok"><Icon.check /></span>;
    } else if (applyError || dlError) {
      action = <button className="btn btn-primary btn-sm" onClick={() => onDownload(target)}>Повторить</button>;
    } else if (downloaded) {
      action = <button className="btn btn-warn btn-sm" onClick={() => onApply(target)}>Обновить</button>;
    } else if (downloading) {
      action = null;
    } else if (needsUpdate) {
      action = <button className="btn btn-primary btn-sm" onClick={() => onDownload(target)}><Icon.download /> Скачать</button>;
    } else {
      statusBadge = isUnknown ? <span className="badge-muted">—</span> : <span className="badge-ok"><Icon.check /></span>;
    }
  } else {
    if (needsUpdate) {
      statusBadge = <span className="badge-warn">обновление</span>;
    } else {
      statusBadge = <span className="badge-ok"><Icon.check /></span>;
    }
  }

  const pct = (downloading ? dl?.pct : apply?.pct) || 0;
  const hasVersionInfo = latest && latest !== current;

  return (
    <section className="comp-row" style={{ animationDelay: `${index * 0.04}s` }}>
      <div className="comp-info">
        <div className="comp-name">{meta.name}</div>
        <div className="comp-desc">{meta.desc}</div>
      </div>
      <div className="comp-versions">
        <span className="comp-cur">{isUnknown ? '—' : fmtVer(current)}</span>
        {hasVersionInfo && <Icon.arrow />}
        {hasVersionInfo && <span className="comp-new">{fmtVer(latest)}</span>}
      </div>
      <div className="comp-action">
        {(downloading || applying) && (
          <div className="prog">
            <div className="prog-bar"><span style={{ width: `${pct}%` }} /></div>
            <span className="prog-pct">{pct}%</span>
          </div>
        )}
        {action || statusBadge}
      </div>
    </section>
  );
}
