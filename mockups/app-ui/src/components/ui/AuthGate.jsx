import { useState, useRef } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { Mail, Lock, KeyRound, AtSign, AlertTriangle, Loader2 } from 'lucide-react';

export default function AuthGate({ onAuthed, onSkip }) {
  const { t } = useT();
  const [mode, setMode] = useState('login'); // login | register | code
  const [email, setEmail] = useState('');
  const [pass, setPass] = useState('');
  const [pass2, setPass2] = useState('');
  const [tg, setTg] = useState('');
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [shake, setShake] = useState(0);
  const [closing, setClosing] = useState(false);
  const closedRef = useRef(false);

  const errText = (r) => t('user.err.' + (r && r.error ? r.error : 'unknown'), r && r.error ? r.error : '');

  const recheck = async () => {
    const s = await api('GET', '/api/server/status');
    if (s && s.account_set === true) {
      closedRef.current = true;
      setClosing(true);
      setTimeout(onAuthed, 200);
      return true;
    }
    return false;
  };

  const submit = async () => {
    if (busy) return;
    setErr('');
    setBusy(true);
    try {
      if (mode === 'register') {
        if (pass !== pass2) throw new Error(t('user.err.passwordMismatch'));
        const r = await api('POST', '/api/account/register-start', { email });
        if (r && r.error) throw new Error(errText(r));
        setMode('code');
      } else if (mode === 'code') {
        const r = await api('POST', '/api/account/register-confirm', { email, code, password: pass, contact_tg: tg });
        if (r && r.error) throw new Error(errText(r));
        if (!(await recheck())) throw new Error(t('user.err.unknown'));
      } else {
        const r = await api('POST', '/api/account/login', { email, password: pass });
        if (r && r.error) throw new Error(errText(r));
        if (!(await recheck())) throw new Error(t('user.err.unknown'));
      }
    } catch (e) {
      setErr(e.message || t('user.err.unknown'));
      setShake((s) => s + 1);
    } finally {
      setBusy(false);
    }
  };

  const skip = () => {
    try { sessionStorage.setItem('zpui-auth-skip', '1'); } catch {}
    onSkip();
  };

  const canSubmit = !!email && (mode === 'code' || !!pass);

  return (
    <div className={'ag' + (closing ? ' closing' : '')}>
      <div key={shake} className={'ag-card' + (err ? ' shake' : '')}>
        <div className="ag-logo">
          <span className="ag-logo-accent">Z</span>PUI
        </div>
        <div className="ag-subtitle">{t('auth.subtitle')}</div>

        <div className="ag-tabs">
          <button
            className={'ag-tab' + (mode === 'login' ? ' active' : '')}
            onClick={() => { setMode('login'); setErr(''); }}
          >{t('auth.loginTab')}</button>
          <button
            className={'ag-tab' + (mode === 'register' || mode === 'code' ? ' active' : '')}
            onClick={() => { setMode('register'); setErr(''); }}
          >{t('auth.regTab')}</button>
        </div>

        <div className="ag-form">
          {mode !== 'code' && (
            <label className="up-field-input">
              <Mail size={14} strokeWidth={2} />
              <input type="email" placeholder={t('auth.email')} value={email} onChange={(e) => setEmail(e.target.value)} />
            </label>
          )}
          {mode !== 'code' && (
            <label className="up-field-input">
              <Lock size={14} strokeWidth={2} />
              <input type="password" placeholder={t('auth.password')} value={pass} onChange={(e) => setPass(e.target.value)} />
            </label>
          )}
          {mode === 'register' && (
            <>
              <label className="up-field-input">
                <Lock size={14} strokeWidth={2} />
                <input type="password" placeholder={t('auth.passwordRepeat')} value={pass2} onChange={(e) => setPass2(e.target.value)} />
              </label>
              <label className="up-field-input">
                <AtSign size={14} strokeWidth={2} />
                <input placeholder={t('user.contactTg')} value={tg} onChange={(e) => setTg(e.target.value)} />
              </label>
            </>
          )}
          {mode === 'code' && (
            <>
              <div className="ag-code-hint">{t('auth.codeSent', { email })}</div>
              <label className="up-field-input">
                <KeyRound size={14} strokeWidth={2} />
                <input placeholder={t('auth.code')} value={code} onChange={(e) => setCode(e.target.value)} />
              </label>
            </>
          )}
          {err && (
            <div className="ag-error">
              <AlertTriangle size={13} strokeWidth={2.2} />
              <span>{err}</span>
            </div>
          )}
          <button className="up-pill up-pill-accent up-pill-lg ag-submit" disabled={busy || !canSubmit} onClick={submit}>
            {busy ? <Loader2 size={14} strokeWidth={2.2} className="up-spin" /> : null}
            {mode === 'login' ? t('auth.loginTab') : mode === 'register' ? t('auth.regTab') : t('auth.confirmCode')}
          </button>
        </div>

        <button className="ag-skip" onClick={skip}>{t('auth.continueWithout')}</button>
      </div>
    </div>
  );
}
