import { useState, useEffect, useRef, useCallback } from 'react';
import { api, createStream } from '../api';
import { useT } from '../i18n';
import { Loader, Check, AlertTriangle } from 'lucide-react';

const sleep = (ms) => new Promise(r => setTimeout(r, ms));

export default function SetupWizard({ onComplete, onCancel }) {
  const { t } = useT();
  const [phase, setPhase] = useState('boot');
  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState('');
  const [error, setError] = useState(null);

  const [env, setEnv] = useState({ thirdParty: false, vpn: false });

  const [results, setResults] = useState([]);
  const [selected, setSelected] = useState('');
  const [bestStrategy, setBestStrategy] = useState('');
  const [testIdx, setTestIdx] = useState(0);
  const [testTotal, setTestTotal] = useState(0);

  const [gameFilter, setGameFilter] = useState(true);
  const [ipsetMode, setIpsetMode] = useState('all');
  const [customRes, setCustomRes] = useState('');
  const [wantDns, setWantDns] = useState(false);
  const [wantProxy, setWantProxy] = useState(true);
  const [wantNotif, setWantNotif] = useState(true);
  const [wantAutostart, setWantAutostart] = useState(false);

  const startedRef = useRef(false);
  const aliveRef = useRef(true);
  const resolverRef = useRef(null);

  const waitForUser = useCallback(() => new Promise(res => { resolverRef.current = res; }), []);
  const resolveUser = useCallback((v) => {
    if (resolverRef.current) { resolverRef.current(v); resolverRef.current = null; }
  }, []);

  const tickProgress = useCallback((from, to, ms) => new Promise(resolve => {
    const start = performance.now();
    const step = (now) => {
      if (!aliveRef.current) return resolve();
      const k = Math.min(1, (now - start) / ms);
      setProgress(Math.round(from + (to - from) * k));
      if (k < 1) requestAnimationFrame(step); else resolve();
    };
    requestAnimationFrame(step);
  }), []);

  const run = useCallback(async () => {
    setPhase('boot');
    setStatus(t('setup.boot'));
    await tickProgress(0, 15, 1100);
    if (!aliveRef.current) return;

    setPhase('selfcheck');
    setStatus(t('setup.selfcheck.checking'));
    await tickProgress(15, 28, 700);
    const health = await api('GET', '/api/health').catch(() => null);
    if (health?.warnings?.length) {
      setStatus(t('setup.selfcheck.repairing'));
      await sleep(900);
    } else {
      await sleep(300);
    }
    if (!aliveRef.current) return;

    setPhase('envscan');
    setStatus(t('setup.envscan.scanning'));
    await tickProgress(28, 38, 700);
    const tp = await api('GET', '/api/setup/detect-thirdparty').catch(() => null);
    const hasThird = !!(tp && tp.has_third_party);
    const hasVpn = false;
    setEnv({ thirdParty: hasThird, vpn: hasVpn });
    await sleep(300);
    if (!aliveRef.current) return;

    setPhase('download');
    const local = await api('GET', '/api/zapret/local').catch(() => null);
    if (!(local === true || local?.result === true)) {
      setStatus(t('setup.download.downloading'));
      await tickProgress(38, 58, 1200);
      await api('POST', '/api/setup/install').catch(() => null);
      await api('POST', '/api/setup/start').catch(() => null);
    }
    setStatus(t('setup.download.verifying'));
    await tickProgress(58, 64, 600);
    if (!aliveRef.current) return;

    if (hasThird) {
      setPhase('thirdparty');
      const decision = await waitForUser();
      if (decision === 'remove') {
        setStatus(t('setup.thirdparty.remove'));
        await api('POST', '/api/setup/remove-thirdparty').catch(() => null);
        await sleep(600);
      }
      if (!aliveRef.current) return;
    }

    const cfg = await api('GET', '/api/config').catch(() => null);
    const firstRun = !(cfg && cfg.first_run_done);
    let wantSetup = false;
    if (firstRun) {
      setPhase('firstrun');
      const d = await waitForUser();
      wantSetup = d === 'setup';
      if (!aliveRef.current) return;
    }

    if (!wantSetup) {
      setPhase('done');
      setStatus(t('setup.done.desc'));
      await tickProgress(85, 100, 600);
      await api('POST', '/api/setup/complete').catch(() => null);
      if (aliveRef.current && onComplete) onComplete();
      return;
    }

    // ── control check (baseline without zapret) ──
    setPhase('test');
    setStatus(t('setup.test.baseline'));
    await tickProgress(64, 70, 800);
    await api('GET', '/api/setup/control-check').catch(() => null);
    if (!aliveRef.current) return;

    // ── strategy autoselect (streaming) ──
    setStatus(t('setup.test.autoSelect'));
    setTestIdx(0);

    const scored = [];
    let appliedStrategy = null;

    await new Promise((resolve) => {
      const es = createStream('/api/autoselect/stream');
      const finish = () => { es.close(); resolve(); };

      es.onmessage = (e) => {
        if (!aliveRef.current) { finish(); return; }
        const d = JSON.parse(e.data);
        if (d.type === 'done') {
          if (d.error && !appliedStrategy) {
            setError(d.error);
          }
          finish();
          return;
        }
        if (d.type === 'progress') {
          setTestTotal(d.total || 0);
          setTestIdx(d.current || 0);
          if (d.message) setStatus(d.message);
          const pctProg = d.total > 0 ? (d.current / d.total) : 0;
          setProgress(Math.round(70 + pctProg * 22));
        } else if (d.type === 'result') {
          if (d.strategy && !d.error) {
            scored.push({
              name: d.strategy,
              percentage: d.resources_n > 0 ? Math.round((d.resources_ok / d.resources_n) * 100) : 0,
              blocked: [],
            });
            appliedStrategy = d.strategy;
          } else if (d.strategy && d.error) {
            scored.push({ name: d.strategy, percentage: 0, blocked: [] });
          }
        } else if (d.type === 'info') {
          const msg = d.message || '';
          const prefix = 'Применена стратегия:';
          if (msg.startsWith(prefix)) {
            appliedStrategy = msg.replace(prefix, '').trim().replace('.bat', '');
          }
          if (msg) setStatus(msg);
        }
      };
      es.onerror = () => { finish(); };
    });

    if (!aliveRef.current) return;

    scored.sort((a, b) => b.percentage - a.percentage);
    const viable = scored.filter(s => s.percentage >= 50);
    const display = viable.length > 0 ? viable : scored;
    setResults(display);
    setBestStrategy(appliedStrategy || (display[0] ? display[0].name : ''));
    setSelected(appliedStrategy || (display[0] ? display[0].name : ''));

    setStatus(t('setup.test.done'));
    setProgress(95);

    setPhase('results');
    await waitForUser();
    if (!aliveRef.current) return;

    setPhase('config');
    await waitForUser();
    if (!aliveRef.current) return;

    setPhase('done');
    setStatus(t('setup.done.desc'));
    await tickProgress(95, 100, 500);
    await api('POST', '/api/setup/complete').catch(() => null);
    if (aliveRef.current && onComplete) onComplete();
  }, [t, tickProgress, waitForUser, onComplete]);

  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    aliveRef.current = true;
    run();
    return () => { aliveRef.current = false; };
  }, [run]);

  const handleCancel = useCallback(() => {
    if (onCancel) onCancel();
  }, [onCancel]);

  const pct = Math.round(progress);

  const pctColor = (p) => p >= 90 ? '#34d058' : p >= 70 ? '#5b9cf5' : '#f55e5e';

  const blockedTitle = (r) => {
    if (!r.blocked || r.blocked.length === 0) return t('setup.results.allOk');
    return r.blocked.map(b => b.name || b.url).join('\n');
  };

  const interactive = (() => {
    if (phase === 'thirdparty') {
      return (
        <div className="zw-card">
          <div className="zw-card-title">{t('setup.thirdparty.title')}</div>
          <p className="zw-card-desc">{t('setup.thirdparty.desc')}</p>
          {env.vpn && <p className="zw-warn">{t('setup.thirdparty.vpnWarn')}</p>}
          <div className="zw-card-actions">
            <button className="btn btn-danger btn-sm zw-flex" onClick={() => resolveUser('remove')}>{t('setup.thirdparty.remove')}</button>
            <button className="btn btn-ghost btn-sm zw-flex" onClick={() => resolveUser('keep')}>{t('setup.thirdparty.keep')}</button>
          </div>
        </div>
      );
    }
    if (phase === 'firstrun') {
      return (
        <div className="zw-card">
          <div className="zw-card-title">{t('setup.firstrun.title')}</div>
          <p className="zw-card-desc">{t('setup.firstrun.desc')}</p>
          <div className="zw-card-actions">
            <button className="btn btn-accent btn-sm zw-flex" onClick={() => resolveUser('setup')}>{t('setup.firstrun.setup')}</button>
            <button className="btn btn-ghost btn-sm zw-flex" onClick={() => resolveUser('skip')}>{t('setup.firstrun.skip')}</button>
          </div>
        </div>
      );
    }
    if (phase === 'test') {
      return (
        <div className="zw-test-info">
          <Loader size={16} className="spinning" />
          {testTotal > 0 && (
            <span className="zw-test-counter">{testIdx} / {testTotal}</span>
          )}
        </div>
      );
    }
    if (phase === 'results') {
      const allViable = results.length > 0 && results.every(r => r.percentage >= 50);
      return (
        <div className="zw-card zw-card-wide">
          <div className="zw-card-title">{t('setup.results.title')}</div>
          <p className="zw-card-desc">
            {bestStrategy
              ? t('setup.results.applied', { name: bestStrategy.replace('.bat', '') })
              : (allViable ? t('setup.results.desc') : t('setup.results.noViable'))
            }
          </p>
          <div className="zw-strategies">
            {results.map(r => (
              <button
                key={r.name}
                className={'zw-strategy' + (selected === r.name ? ' selected' : '')}
                onClick={() => setSelected(r.name)}
              >
                <span className="zw-strategy-pct" style={{ color: pctColor(r.percentage) }}>{r.percentage}%</span>
                <span className="zw-strategy-name">{r.name.replace('.bat', '')}</span>
                {r.name === bestStrategy && <span className="zw-best">{t('setup.results.best')}</span>}
              </button>
            ))}
          </div>
          <div className="zw-card-actions">
            <button className="btn btn-accent btn-sm zw-flex" disabled={!selected} onClick={() => resolveUser('apply')}>{t('setup.results.apply')}</button>
          </div>
        </div>
      );
    }
    if (phase === 'config') {
      return (
        <div className="zw-card zw-card-wide">
          <div className="zw-card-title">{t('setup.config.title')}</div>
          <div className="zw-config">
            <label className="zw-row"><span><b>{t('setup.config.gameFilter')}</b><i>{t('setup.config.gameFilterDesc')}</i></span>
              <input type="checkbox" checked={gameFilter} onChange={e => setGameFilter(e.target.checked)} /></label>
            <label className="zw-row"><span><b>{t('setup.config.dns')}</b><i>{t('setup.config.dnsDesc')}</i></span>
              <input type="checkbox" checked={wantDns} onChange={e => setWantDns(e.target.checked)} /></label>
            <label className="zw-row"><span><b>{t('setup.config.proxy')}</b><i>{t('setup.config.proxyDesc')}</i></span>
              <input type="checkbox" checked={wantProxy} onChange={e => setWantProxy(e.target.checked)} /></label>
            <label className="zw-row"><span><b>{t('setup.config.notifications')}</b></span>
              <input type="checkbox" checked={wantNotif} onChange={e => setWantNotif(e.target.checked)} /></label>
            <label className="zw-row"><span><b>{t('setup.config.autostart')}</b></span>
              <input type="checkbox" checked={wantAutostart} onChange={e => setWantAutostart(e.target.checked)} /></label>
            <label className="zw-row zw-col"><span><b>{t('setup.config.customResources')}</b></span>
              <textarea className="zw-textarea" placeholder={t('setup.config.customResourcesPh')} value={customRes} onChange={e => setCustomRes(e.target.value)} /></label>
          </div>
          <div className="zw-card-actions">
            <button className="btn btn-accent btn-sm zw-flex" onClick={() => resolveUser('apply')}>{t('setup.config.apply')}</button>
          </div>
        </div>
      );
    }
    return null;
  })();

  return (
    <div className="zw-overlay">
      <div className="zw-progress-section">
        <div className="zw-bar"><div className="zw-bar-fill" style={{ width: pct + '%' }} /></div>
        <div className="zw-status-row">
          <span className="zw-status">{status}</span>
          <span className="zw-pct">{pct}%</span>
        </div>
      </div>
      {interactive}
      {error && <div className="zw-error">{error}</div>}
    </div>
  );
}
