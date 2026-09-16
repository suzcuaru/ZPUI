import { useState, useEffect, useCallback, useMemo } from 'react';
import { api, apiCall } from '../api';
import { useT } from '../i18n';
import { CreditCard, Monitor, Unlink, LogOut, Loader2, UserPlus } from 'lucide-react';
import { usePolling } from '../hooks/usePolling';
import { useConfirm } from '../components/ui/ConfirmDialog';
import { renewalDue } from '../db';

const fmtInvite = (v) => {
  const s = String(v).toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 8);
  return s.length > 4 ? `${s.slice(0, 4)}-${s.slice(4)}` : s;
};

const inviteFilled = (v) => String(v).replace(/[^A-Z0-9]/g, '').length === 8;

function AuthCard({ onAuthed, showToast }) {
  const { t } = useT();
  const [mode, setMode] = useState('login');
  const [invite, setInvite] = useState('');
  const [login, setLogin] = useState('');
  const [pass, setPass] = useState('');
  const [pass2, setPass2] = useState('');
  const [touched, setTouched] = useState({});
  const [inviteErr, setInviteErr] = useState('');
  const [formErr, setFormErr] = useState('');
  const [busy, setBusy] = useState(false);

  const inviteOk = inviteFilled(invite);
  const loginOk = login.trim().length >= 3;
  const passOk = pass.length >= 6;
  const matchOk = pass2.length > 0 && pass === pass2;
  const loginValid = loginOk && passOk;
  const regValid = inviteOk && loginValid && matchOk;

  const blur = (f) => setTouched((p) => ({ ...p, [f]: true }));
  const switchMode = (m) => {
    setMode(m);
    setFormErr('');
    setInviteErr('');
    setTouched({});
  };

  const submitLogin = async () => {
    setTouched({ login: true, pass: true });
    setFormErr('');
    if (!loginValid) return;
    setBusy(true);
    const r = await api('POST', '/api/account/login', { login: login.trim(), password: pass });
    setBusy(false);
    if (r && r.error) {
      setFormErr(t('user.err.' + (r.error === 'bad_credentials' ? 'bad_credentials' : 'unknown')));
      return;
    }
    showToast(t('user_new.loginOk'), 'success');
    onAuthed();
  };

  const submitRegister = async () => {
    setTouched({ invite: true, login: true, pass: true, pass2: true });
    setFormErr('');
    if (!regValid) return;
    setBusy(true);
    const c = await api('GET', `/api/account/check-invite?code=${encodeURIComponent(invite.trim())}`);
    if (!c || c.error || c.valid !== true) {
      setBusy(false);
      setInviteErr(t('user_new.invalidInvite'));
      return;
    }
    const r = await api('POST', '/api/account/activate', {
      code: invite.trim(), login: login.trim(), password: pass,
    });
    setBusy(false);
    if (r && r.error) {
      if (r.error === 'invalid_invite') setInviteErr(t('user_new.invalidInvite'));
      else setFormErr(t('user.err.' + (r.error === 'invalid_input' ? 'invalid_input' : 'unknown')));
      return;
    }
    showToast(t('user_new.activatedOk'), 'success');
    onAuthed();
  };

  const canSubmit = !busy && (mode === 'login' ? loginValid : regValid);

  return (
    <div className="us">
      <div className="us-act-wrap">
        <div className="us-act">
          <div className="us-act-head">
            <div className="us-act-icon"><UserPlus size={20} strokeWidth={2} /></div>
            <div className="us-act-head-body">
              <div className="us-act-title">{t(mode === 'login' ? 'user_new.loginTitle' : 'user_new.regTitle')}</div>
              <div className="us-act-sub">{t(mode === 'login' ? 'user_new.loginSub' : 'user_new.regSub')}</div>
            </div>
          </div>
          <div className="us-auth-tabs" role="tablist" aria-label={t('user_st.profile')}>
            <button
              type="button" role="tab" aria-selected={mode === 'login'}
              className={'flt2-pill' + (mode === 'login' ? ' active' : '')}
              onClick={() => switchMode('login')}
            >{t('user_new.modeLogin')}</button>
            <button
              type="button" role="tab" aria-selected={mode === 'register'}
              className={'flt2-pill' + (mode === 'register' ? ' active' : '')}
              onClick={() => switchMode('register')}
            >{t('user_new.modeRegister')}</button>
          </div>
          <div className="us-act-form" key={mode}>
            {mode === 'register' && (
              <div className="us-field">
                <label htmlFor="us-invite">{t('user_new.inviteLabel')}</label>
                <input
                  id="us-invite"
                  className={'form-input us-invite-input' + (inviteErr ? ' input-error' : '')}
                  value={invite}
                  onChange={(e) => { setInvite(fmtInvite(e.target.value)); setInviteErr(''); }}
                  onBlur={() => blur('invite')}
                  placeholder={t('user_new.invitePlaceholder')}
                />
                {inviteErr
                  ? <div className="us-field-err">{inviteErr}</div>
                  : <div className="us-act-hint">{t('user_new.inviteHint')}</div>}
              </div>
            )}
            <div className="us-field">
              <label htmlFor="us-login">{t('user_new.loginLabel')}</label>
              <input
                id="us-login"
                className={'form-input' + (touched.login && !loginOk ? ' input-error' : '')}
                value={login}
                onChange={(e) => setLogin(e.target.value)}
                onBlur={() => blur('login')}
                placeholder={t('user_new.loginPlaceholder')}
                autoFocus
              />
              {touched.login && !loginOk && <div className="us-field-err">{t('user_new.loginShort')}</div>}
            </div>
            <div className="us-field">
              <label htmlFor="us-pass">{t('user.password')}</label>
              <input
                id="us-pass"
                type="password"
                className={'form-input' + (touched.pass && !passOk ? ' input-error' : '')}
                value={pass}
                onChange={(e) => setPass(e.target.value)}
                onBlur={() => blur('pass')}
              />
              {touched.pass && !passOk && <div className="us-field-err">{t('user_new.passShort')}</div>}
            </div>
            {mode === 'register' && (
              <div className="us-field">
                <label htmlFor="us-pass2">{t('user.passwordRepeat')}</label>
                <input
                  id="us-pass2"
                  type="password"
                  className={'form-input' + (touched.pass2 && !matchOk ? ' input-error' : '')}
                  value={pass2}
                  onChange={(e) => setPass2(e.target.value)}
                  onBlur={() => blur('pass2')}
                />
                {touched.pass2 && !matchOk && <div className="us-field-err">{t('user.err.passwordMismatch')}</div>}
              </div>
            )}
            {formErr && <div className="us-field-err">{formErr}</div>}
            <button
              className="btn btn-accent us-act-submit"
              disabled={!canSubmit}
              onClick={mode === 'login' ? submitLogin : submitRegister}
            >
              {busy ? <Loader2 size={13} className="spinning" /> : null}
              {t(mode === 'login' ? 'user_new.submitLogin' : 'user_new.submitRegister')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default function UserPage({ status, showToast }) {
  const { t, lang } = useT();
  const confirm = useConfirm();
  const [me, setMe] = useState(null);
  const [phase, setPhase] = useState('loading');
  const [busy, setBusy] = useState(false);

  const loadMe = useCallback(async () => {
    const d = await api('GET', '/api/account/me');
    if (d && !d.error) {
      setMe(d);
      setPhase('ready');
    } else if (d && d.error === 'not_authorized') {
      setMe(null);
      setPhase('unregistered');
    } else {
      setMe(null);
      setPhase('error');
    }
  }, []);
  usePolling(loadMe, 20000);
  useEffect(() => { loadMe(); }, [loadMe]);

  const devices = useMemo(() => me?.devices || [], [me]);
  const sub = me?.subscription;
  const max = sub?.devices_max || 5;
  const atLimit = devices.length >= max;
  const thisDevice = useMemo(() => {
    const host = (status?.network?.hostname || '').toLowerCase();
    return devices.find((d) => (d.name || '').toLowerCase() === host) || devices[0] || null;
  }, [devices, status]);

  const logout = async () => {
    showToast(t('user_st.loggingOut'), 'info');
    await api('POST', '/api/account/logout');
    await loadMe();
  };

  const unbind = async (d) => {
    const yes = await confirm({
      title: t('user_new.unbindTitle'),
      message: t('user_new.unbindConfirm', { name: d.name || d.id }),
      confirmText: t('user.unbind'),
      variant: 'danger',
    });
    if (!yes) return;
    setBusy(true);
    try {
      await apiCall(() => api('POST', '/api/server/unbind', { device_id: d.id }), null, showToast);
      await loadMe();
    } finally { setBusy(false); }
  };

  const loading = phase === 'loading';
  const fmtDate = (ts) => (ts ? new Date(ts * 1000).toLocaleDateString(lang === 'en' ? 'en-US' : 'ru-RU') : '—');
  const zpuiVersion = status?.mod?.version
    ? `${status.mod.version}${status?.mod?.arch ? ` (${status.mod.arch})` : ''}`
    : '—';
  const planLabel = t('user_new.planTester');

  // «Продлить» появляется только когда подписка на исходе: порог зависит
  // от периода тарифа (месячная — сутки, квартальная — 2 недели, годовая — месяц).
  const expired = !!sub && sub.until * 1000 <= Date.now();
  const due = !!sub && renewalDue(sub.until, sub.period);
  const showRenew = due || expired;
  const accStatus = expired ? 'expired' : due ? 'expiring' : (me?.user?.status || 'active');
  const statusBadge = {
    active: { cls: 'ok', key: 'user_st.statusActive' },
    expiring: { cls: 'warn', key: 'user_new.subExpiring' },
    expired: { cls: 'revoked', key: 'user_new.subExpired' },
    blocked: { cls: 'revoked', key: 'user_st.statusBlocked' },
  }[accStatus] || { cls: 'ok', key: 'user_st.statusActive' };
  const kpiSub = devices.length >= 2 ? t('user_st.bothOnline') : devices.length === 1 ? t('user_st.oneOnline') : null;

  if (loading) {
    return (
      <div className="us">
        <div className="us-cols">
          <div className="us-metric us-kpi1 skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '60%' }} /><div className="skeleton-line" style={{ width: '40%' }} /></div>
          <div className="us-metric us-kpi2 skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '60%' }} /><div className="skeleton-line" style={{ width: '40%' }} /></div>
          <div className="us-tile us-profile us-prof skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '80%' }} /><div className="skeleton-line" style={{ width: '65%' }} /><div className="skeleton-line" style={{ width: '70%' }} /></div>
          <div className="us-panel us-dev us-devrows skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '45%' }} /><div className="skeleton-line" style={{ width: '90%' }} /><div className="skeleton-line" style={{ width: '90%' }} /></div>
          <div className="us-tile us-this skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '55%' }} /><div className="skeleton-line" style={{ width: '75%' }} /></div>
        </div>
      </div>
    );
  }

  if (phase === 'unregistered') {
    return <AuthCard onAuthed={loadMe} showToast={showToast} />;
  }

  return (
    <div className="us">
      {phase === 'error' && (
        <div className="us-error" role="alert">
          <span>{t('user_st.loadError')}</span>
          <button className="btn btn-sm" onClick={loadMe}>{t('user_st.retry')}</button>
        </div>
      )}
      {/* Единая сетка: KPI×2 (Устройства, Подписка) / Устройства / Профиль+Подписка / Это устройство */}
      <div className="us-cols stagger">
        <div className="us-metric us-kpi1">
          <div className="us-metric-icon on"><Monitor size={22} strokeWidth={2} /></div>
          <div className="us-metric-body">
            <span className="us-metric-label">{t('user_st.devicesMetric')}</span>
            <span className="us-metric-value">{devices.length} <span className="us-dim">{t('user_st.devicesOf', { n: max })}</span></span>
            {kpiSub && <span className="us-metric-sub">{kpiSub}</span>}
          </div>
        </div>

        <div className="us-metric us-kpi2">
          <div className={'us-metric-icon' + (accStatus === 'active' ? ' on' : '')}><CreditCard size={22} strokeWidth={2} /></div>
          <div className="us-metric-body">
            <span className="us-metric-label">{t('user_new.subscription')}</span>
            <span className="us-metric-value">{planLabel}</span>
            <span className="us-metric-sub">
              {expired ? t('user_new.subExpired') : t('user_new.kpiUntil', { d: fmtDate(sub?.until) })}
            </span>
          </div>
        </div>

        <div className="us-tile us-profile us-prof">
          <div className="us-tile-title">{t('user_st.profile')}</div>
          <div className="us-pf-head">
            <div className="us-avatar">{(me?.user?.login || '?')[0].toUpperCase()}</div>
            <div className="us-pf-name">
              {me?.user?.login}
              <span className="us-badge accent">{planLabel}</span>
            </div>
          </div>

          <div className="us-sub-block">
            <div className="us-sub-title">{t('user_new.subscription')}</div>
            <div className="us-kv">
              <span className="k">{t('user_new.accountStatus')}</span>
              <span className={'us-badge ' + statusBadge.cls}>{t(statusBadge.key)}</span>
            </div>
            <div className="us-kv"><span className="k">{t('user_new.validUntil')}</span><span className="v">{fmtDate(sub?.until)}</span></div>
            <div className="us-kv">
              <span className="k">{t('user_st.devicesMetric')}</span>
              <span className="v">{devices.length} <span className="us-dim">{t('user_st.devicesOf', { n: max })}</span></span>
            </div>
          </div>

          <div className="us-pf-actions">
            {showRenew && (
              <button className="btn btn-sm btn-warn" onClick={() => showToast(t('user_new.availableInProduct'), 'info')}>
                {t('user_new.renew')}
              </button>
            )}
            <button className="btn btn-sm btn-danger-ghost" onClick={logout}><LogOut size={11} />{t('user.logout')}</button>
          </div>
        </div>

        <div className="us-panel us-dev us-devrows">
          <div className="us-panel-head">
            <div className="us-panel-title">{t('user_st.devicesMetric')} <span>· {devices.length} {t('user_st.devicesOf', { n: max })}</span></div>
            {atLimit && (
              <span className="us-badge warn us-limit" data-tooltip={t('user_new.limitReason')}>{t('user_new.limitReached')}</span>
            )}
          </div>
          <div className="us-list">
            {devices.length === 0 && <div className="us-empty">{t('user.noDevices')}</div>}
            {devices.map((d) => {
              const current = !!thisDevice && d.id === thisDevice.id;
              return (
                <div key={d.id} className="us-dev">
                  <span className="us-dot" style={{ flex: 'none' }} />
                  <div className="us-dev-info">
                    <div className="us-dev-name">{d.name} {current && <span className="us-badge accent">{t('user_st.thisDevice')}</span>}</div>
                    <div className="us-dev-meta">
                      <span className="us-mono">{d.fingerprint}</span> · {d.os} · {t('user_new.boundDate', { d: fmtDate(d.bound_at) })}
                    </div>
                  </div>
                  <button className="btn btn-sm" disabled={busy} onClick={() => unbind(d)}><Unlink size={11} />{t('user.unbind')}</button>
                </div>
              );
            })}
          </div>
        </div>

        <div className="us-tile us-this">
          <div className="us-tile-title">{t('user_st.thisDevice')}</div>
          <div className="us-dev-name">{status?.network?.hostname || '—'}</div>
          <div className="us-metric-sub"><span className="us-dot" />{t('user_st.connected')}</div>
          <div className="us-kv"><span className="k">{t('user_new.fingerprint')}</span><span className="v us-mono">{thisDevice?.fingerprint || '—'}</span></div>
          <div className="us-this-hint">{t('user_new.fpHint')}</div>
          <div className="us-kv"><span className="k">{t('user_st.osLabel')}</span><span className="v">{status?.system?.os || '—'}</span></div>
          <div className="us-kv"><span className="k">{t('user_st.zpuiVersion')}</span><span className="v us-mono">{zpuiVersion}</span></div>
          <span className="us-chip"><span className="us-dot" />{t('user_new.telemetryChip')}</span>
        </div>
      </div>
    </div>
  );
}
