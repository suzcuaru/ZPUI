import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../api';
import { useT } from '../i18n';

// Demo verdicts (until the real strategy-test backend is wired in).
const MOCK_VERDICTS = {
  'general (ALT).bat': 'green',
  'general (discord).bat': 'yellow',
  'quic (ALT).bat': 'green',
  'discord_voice.bat': 'red',
  'youtube_unblock.bat': 'white',
};

const sleep = (ms) => new Promise(r => setTimeout(r, ms));

export default function SetupWizard({ onComplete, onCancel }) {
  const { t } = useT();
  const [phase, setPhase] = useState('boot');
  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState('');
  const [error, setError] = useState(null);

  // env scan results
  const [env, setEnv] = useState({ thirdParty: false, vpn: false });

  // strategy test
  const [strategies, setStrategies] = useState([]);
  const [testCurrent, setTestCurrent] = useState('');
  const [results, setResults] = useState([]);
  const [selected, setSelected] = useState('');
  const [bestStrategy, setBestStrategy] = useState('');

  // config
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
    // ── boot ──────────────────────────────
    setPhase('boot');
    setStatus(t('setup.boot'));
    await tickProgress(0, 15, 1100);
    if (!aliveRef.current) return;

    // ── self-check ────────────────────────
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

    // ── env scan ──────────────────────────
    setPhase('envscan');
    setStatus(t('setup.envscan.scanning'));
    await tickProgress(28, 38, 700);
    const tp = await api('GET', '/api/setup/detect-thirdparty').catch(() => null);
    const hasThird = !!(tp && tp.has_third_party);
    const hasVpn = false; // TODO: backend VPN detection
    setEnv({ thirdParty: hasThird, vpn: hasVpn });
    await sleep(300);
    if (!aliveRef.current) return;

    // ── download zapret ───────────────────
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

    // ── third-party prompt ────────────────
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

    // ── first-run offer ───────────────────
    const cfg = await api('GET', '/api/config').catch(() => null);
    const firstRun = !(cfg && cfg.first_run_done);
    let wantSetup = false;
    if (firstRun) {
      setPhase('firstrun');
      const d = await waitForUser();
      wantSetup = d === 'setup';
      if (!aliveRef.current) return;
    }

    // ── strategy auto-test ────────────────
    setPhase('test');
    setStatus(t('setup.test.control'));
    await tickProgress(64, 70, 700);
    const sRes = await api('GET', '/api/setup/strategies').catch(() => null);
    const list = (sRes && sRes.strategies) || [];
    const current = (sRes && sRes.current) || (list[0] || '');
    setStrategies(list);
    const scored = [];
    const span = list.length ? Math.min(25, 95 - 70) / list.length : 0;
    for (let i = 0; i < list.length; i++) {
      const name = list[i];
      setTestCurrent(name);
      setStatus(t('setup.test.testing', { name: name.replace('.bat', '') }));
      const fromP = 70 + span * i;
      await tickProgress(fromP, fromP + span, 650);
      setStatus(t('setup.test.waiting'));
      await api('POST', '/api/setup/apply-strategy', { strategy: name }).catch(() => null);
      await sleep(250);
      const verdict = (MOCK_VERDICTS[name]) || ['green', 'yellow', 'red', 'white'][i % 4];
      scored.push({ name, verdict });
      if (!aliveRef.current) return;
    }
    setResults(scored);
    setStatus(t('setup.test.done'));
    setProgress(95);

    // pick best (green > yellow > red > white)
    const rank = { green: 0, yellow: 1, red: 2, white: 3 };
    const best = [...scored].sort((a, b) => rank[a.verdict] - rank[b.verdict])[0];
    setBestStrategy(best ? best.name : current);
    setSelected(best ? best.name : current);

    // ── results selection ─────────────────
    setPhase('results');
    await waitForUser();
    if (!aliveRef.current) return;

    // ── quick config (first run only) ─────
    if (wantSetup) {
      setPhase('config');
      await waitForUser();
      if (!aliveRef.current) return;
    }

    // ── done ──────────────────────────────
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

  // ── interactive screens ──────────────────
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
    if (phase === 'results') {
      return (
        <div className="zw-card zw-card-wide">
          <div className="zw-card-title">{t('setup.results.title')}</div>
          <p className="zw-card-desc">{t('setup.results.desc')}</p>
          <div className="zw-strategies">
            {results.map(r => (
              <button
                key={r.name}
                className={'zw-strategy zw-v-' + r.verdict + (selected === r.name ? ' selected' : '')}
                onClick={() => setSelected(r.name)}
              >
                <span className="zw-strategy-name">{r.name.replace('.bat', '')}</span>
                {r.name === bestStrategy && <span className="zw-best">{t('setup.results.best')}</span>}
                <span className="zw-strategy-verdict">{t('setup.verdict.' + r.verdict)}</span>
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

  const pct = Math.round(progress);

  return (
    <div className="zw-overlay">
      <button className="zw-cancel" onClick={handleCancel} aria-label="cancel">✕</button>
      <div className="zw-brand">
        <img src="logo.svg" alt="ZPUI" className="zw-logo" />
      </div>
      <div className="zw-bar"><div className="zw-bar-fill" style={{ width: pct + '%' }} /></div>
      <div className="zw-status-line">
        <span className="zw-pct">{pct}%</span>
        <span className="zw-status">{status}{testCurrent && phase === 'test' ? '' : ''}</span>
      </div>
      {interactive}
      {error && <div className="zw-error">{error}</div>}
    </div>
  );
}
