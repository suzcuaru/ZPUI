import { useState, useEffect, useCallback, useRef } from 'react';
import {
  UserRound, Check, RefreshCw, ArrowLeft, KeyRound, ShieldCheck, Copy, Mail,
} from 'lucide-react';
import { api } from '../../api';
import { useT } from '../../i18n';
import ProgressBar from '../ui/ProgressBar';
import Spinner from '../ui/Spinner';
import { useToast } from '../../context/ToastContext';

// AuthForm — панель авторизации/регистрации (вход пароль + код аутентификатора,
// регистрация с QR и ключевым словом, recovery с перепривязкой). Протокол v4.
// Используется в панели профиля.

export const INPUT_LIMITS = { login: 32, email: 254, password: 72, code: 6, keyword: 64, nick: 32 };
export const stripCtl = (s) => s.replace(/[\u0000-\u001F\u007F]/g, '');
export const limited = (setter, max) => (e) => setter(stripCtl(e.target.value).slice(0, max));

function fmtMMSS(sec) {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return m + ':' + String(s).padStart(2, '0');
}

export function useErrT(t) {
  return useCallback((res) => {
    if (res && res.error === 'client_exception') {
      return t('auth.err.callFailed', { detail: res.detail || 'unknown' });
    }
    const k = 'auth.err.' + (res && res.error);
    const v = t(k);
    if (v !== k) return v;
    if (res && res.error) return t('auth.err.generic') + ' [' + res.error + ']';
    return t('auth.err.network');
  }, [t]);
}

async function fetchQr(uri) {
  if (!uri) return '';
  const r = await api('GET', '/api/account/totp/qr?uri=' + encodeURIComponent(uri));
  return (r && r.png) || '';
}

// CopyBtn — кнопка копирования в буфер (секрет, ключевое слово).
export function CopyBtn({ text, t }) {
  const showToast = useToast();
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      showToast(t('profile.copied'), 'success');
    } catch {
      showToast(t('profile.copyFailed'), 'error');
    }
  };
  return (
    <button type="button" className="pf-copy" onClick={copy} aria-label={t('profile.copy')}>
      <Copy size={13} strokeWidth={2} />
    </button>
  );
}

// PwStrength — индикатор надёжности пароля: длина + разнообразие символов.
export function PwStrength({ pw, t }) {
  if (!pw) return null;
  let classes = 0;
  if (/[a-zа-я]/.test(pw)) classes++;
  if (/[A-ZА-Я]/.test(pw)) classes++;
  if (/\d/.test(pw)) classes++;
  if (/[^A-Za-zА-Яа-я0-9]/.test(pw)) classes++;
  const score = (pw.length >= 8 ? 1 : 0) + (pw.length >= 12 ? 1 : 0) + (classes >= 3 ? 1 : 0) + (classes === 4 ? 1 : 0);
  const level = pw.length < 6 ? 0 : score >= 3 ? 2 : score >= 2 ? 1 : 0;
  const variant = ['danger', 'warning', 'success'][level];
  const label = [t('profile.pwWeak'), t('profile.pwMedium'), t('profile.pwStrong')][level];
  return (
    <div className="pw-strength">
      <ProgressBar value={((level + 1) / 3) * 100} variant={variant} size="sm" />
      <span className={'pw-strength-label ' + variant}>{label}</span>
    </div>
  );
}

export default function AuthForm({ onAuthed }) {
  const showToast = useToast();
  const { t } = useT();
  const errT = useErrT(t);
  const [mode, setMode] = useState('login'); // login | register | forgot
  const [step, setStep] = useState('form'); // form | enroll | code | newpass | mailcode
  const [login, setLogin] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [password2, setPassword2] = useState(''); // повтор при регистрации
  const [code, setCode] = useState('');
  const [keyword, setKeyword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  // данные незавершённой регистрации / пере-привязки аутентификатора
  const [regToken, setRegToken] = useState('');
  const [enroll, setEnroll] = useState(null); // {secret, uri, keyword?, qr}
  // контекст входа для totp.confirm после recovery/totp_setup_required
  const pendingRef = useRef(null); // {login, password}
  const resetTokenRef = useRef(''); // одноразовый токен сброса пароля
  // Email-2FA: второй шаг входа — код из письма (op auth.login.verify).
  const loginTokenRef = useRef(''); // временный токен второго шага из auth.login
  const [mailEmail, setMailEmail] = useState(''); // почта для verify (иф из идентификатора/истории)
  const [history, setHistory] = useState([]);
  // Срок жизни QR регистрации: заявка на сервере живёт 15 минут, потом
  // подтверждение кода вернуло бы «registration expired». Таймер идёт только
  // для регистрации; при обнулении заявка создаётся заново (новый QR).
  const [qrLeft, setQrLeft] = useState(0);
  const [qrRefreshed, setQrRefreshed] = useState(false);

  const reset = useCallback(() => {
    setStep('form');
    setCode('');
    setNewPassword('');
    setPassword2('');
    setError('');
    setBusy(false);
    setEnroll(null);
    setQrLeft(0);
    setQrRefreshed(false);
    setMailEmail('');
    loginTokenRef.current = '';
    pendingRef.current = null;
  }, []);

  const switchMode = useCallback((m) => {
    setMode(m);
    reset();
  }, [reset]);

  // История аккаунтов устройства — подсказки для быстрого входа.
  useEffect(() => {
    if (mode !== 'login' || step !== 'form') return;
    let alive = true;
    api('GET', '/api/account/device-accounts').then((r) => {
      if (alive && r && !r.error && Array.isArray(r.accounts) && r.accounts.length > 0) {
        setHistory(r.accounts.slice(0, 5));
      }
    }).catch(() => {});
    return () => { alive = false; };
  }, [mode, step]);

  const showEnroll = useCallback(async (secret, uri, keyword) => {
    setEnroll({ secret, uri, keyword: keyword || '', qr: '' });
    setStep('enroll');
    setCode('');
    const qr = await fetchQr(uri);
    setEnroll((prev) => (prev ? { ...prev, qr } : prev));
  }, []);

  // Истёк срок заявки регистрации: создаём новую с теми же данными формы
  // (поля никуда не девались) — пользователь сканирует свежий QR, ничего
  // заново вводить не нужно.
  const refreshRegistration = useCallback(async () => {
    if (busy) return;
    if (!login.trim() || !email.trim() || !password || keyword.trim().length < 8) {
      setError(t('auth.err.fillAll'));
      return;
    }
    setBusy(true);
    setError('');
    const res = await api('POST', '/api/auth/email/register', {
      login: login.trim(), email: email.trim(), password, keyword: keyword.trim(),
    });
    setBusy(false);
    if (!res || res.error || !res.reg_token) {
      // например, логин/почту успел занять кто-то другой — только форма
      setError(errT(res));
      return;
    }
    setRegToken(res.reg_token || '');
    setQrRefreshed(true);
    await showEnroll(res.totp_secret || '', res.totp_uri || '', '');
  }, [busy, login, email, password, keyword, errT, showEnroll, t]);

  // Обратный отсчёт QR (только регистрация — только у неё заявка на сервере
  // тухнет; при пере-привязке после recovery секрет уже на аккаунте).
  // Само обновление — отдельным эффектом по достижении нуля: сайд-эффект
  // внутри setState-updater'а StrictMode вызывает дважды.
  useEffect(() => {
    if (step !== 'enroll' || mode !== 'register' || !enroll || !enroll.secret) {
      setQrLeft(0);
      return undefined;
    }
    setQrLeft(15 * 60);
    const iv = setInterval(() => {
      setQrLeft((s) => (s > 0 ? s - 1 : 0));
    }, 1000);
    return () => clearInterval(iv);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, mode, enroll && enroll.secret]);

  useEffect(() => {
    if (qrLeft === 0 && step === 'enroll' && mode === 'register' && enroll && enroll.secret) {
      refreshRegistration();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qrLeft]);

  // Email-2FA: сервер просит подтвердить вход кодом из письма.
  const beginMailCode = useCallback((res, ident) => {
    loginTokenRef.current = res.login_token || '';
    // почта: если вошли по нику — пробуем восстановить из истории устройства
    const guessed = ident && ident.includes('@')
      ? ident
      : (history.find((h) => h.nick === ident || h.email === ident) || {}).email || '';
    setMailEmail(guessed);
    setCode('');
    setError('');
    setStep('mailcode');
  }, [history]);

  const submitLogin = useCallback(async () => {
    // Сервер резолвит в «login» и логин, и почту. Код аутентификатора — второй
    // шаг: сначала подтверждается пара логин/пароль (bad_totp = «пароль верен,
    // нужен код»), поле кода показывается отдельным шагом.
    const ident = login.trim() || email.trim();
    if (!ident || !password) { setError(t('auth.err.fillAll')); return; }
    const codeStep = step === 'code';
    const c = code.trim();
    if (codeStep && !c) { setError(t('auth.err.fillCode')); return; }
    setBusy(true);
    setError('');
    const res = await api('POST', '/api/auth/email/login', { login: ident, password, code: codeStep ? c : '' });
    setBusy(false);
    if (!res || res.error) {
      // неверный пароль/логин — остаёмся на форме без поля кода
      if (res && res.error === 'bad_login') {
        setError(t('auth.err.invalid_credentials'));
        setStep('form');
        return;
      }
      // пара подтверждена — запрашиваем код аутентификатора
      if (res && res.error === 'bad_totp' && !codeStep) {
        setCode('');
        setStep('code');
        setError(t('auth.err.fillCode'));
        return;
      }
      // аутентификатор аннулирован (после recovery) — показываем привязку
      if (res && res.error === 'totp_setup_required' && res.totp_secret) {
        pendingRef.current = { login: login.trim(), password };
        await showEnroll(res.totp_secret, res.totp_uri, '');
        return;
      }
      setError(errT(res));
      return;
    }
    if (res.two_fa && res.login_token) {
      beginMailCode(res, ident);
      return;
    }
    if (onAuthed) onAuthed(res);
  }, [login, email, password, code, step, errT, onAuthed, t, showEnroll, beginMailCode]);

  const submitRegister = useCallback(async () => {
    if (!login.trim() || !email.trim() || !password) { setError(t('auth.err.fillAll')); return; }
    if (password !== password2) { setError(t('auth.err.passMismatch')); return; }
    if (keyword.trim().length < 8) { setError(t('auth.err.shortKeyword')); return; }
    setBusy(true);
    setError('');
    const res = await api('POST', '/api/auth/email/register', {
      login: login.trim(), email: email.trim(), password, keyword: keyword.trim(),
    });
    setBusy(false);
    if (!res || res.error) { setError(errT(res)); return; }
    setRegToken(res.reg_token || '');
    await showEnroll(res.totp_secret || '', res.totp_uri || '', '');
  }, [login, email, password, password2, keyword, errT, showEnroll]);

  const submitForgot = useCallback(async () => {
    if (!login.trim() || !keyword.trim()) { setError(t('auth.err.fillAll')); return; }
    setBusy(true);
    setError('');
    const res = await api('POST', '/api/auth/email/forgot', { login: login.trim(), keyword: keyword.trim() });
    setBusy(false);
    if (!res || res.error) { setError(errT(res)); return; }
    resetTokenRef.current = res.reset_token || '';
    setStep('newpass');
  }, [login, keyword, errT]);

  const submitNewPassword = useCallback(async () => {
    if (newPassword.length < 8) { setError(t('auth.err.weak')); return; }
    setBusy(true);
    setError('');
    const res = await api('POST', '/api/auth/email/forgot/verify', { reset_token: resetTokenRef.current, new_password: newPassword });
    setBusy(false);
    if (!res || res.error) { setError(errT(res)); return; }
    showToast(t('profile.passwordChanged'), 'success');
    // пароль сброшен, аутентификатор аннулирован: сразу логинимся — сервер
    // выдаст totp_setup_required с новым секретом, дальше — привязка.
    setLogin(login.trim());
    setPassword(newPassword);
    await submitLoginInner(login.trim(), newPassword);
  }, [newPassword, showToast, login]); // eslint-disable-line react-hooks/exhaustive-deps

  // единый обработчик кода: регистрация / пере-привязка входа
  const submitCode = useCallback(async () => {
    const c = code.trim();
    if (!c) { setError(t('auth.err.fillCode')); return; }
    setBusy(true);
    setError('');
    if (mode === 'register') {
      const res = await api('POST', '/api/auth/email/register/verify', { reg_token: regToken, code: c });
      setBusy(false);
      if (!res || res.error) { setError(errT(res)); return; }
      if (onAuthed) onAuthed(res, { registered: true });
      return;
    }
    if (mode === 'login') {
      // второй шаг входа: пара логин/пароль уже подтверждена сервером
      const ident = login.trim() || email.trim();
      if (!ident || !password) { setError(t('auth.err.fillAll')); return; }
      setBusy(true);
      setError('');
      const res = await api('POST', '/api/auth/email/login', { login: ident, password, code: c });
      setBusy(false);
      if (!res || res.error) {
        if (res && res.error === 'bad_login') { setError(t('auth.err.invalid_credentials')); return; }
        setError(errT(res));
        return;
      }
      if (res.two_fa && res.login_token) { beginMailCode(res, ident); return; }
      if (onAuthed) onAuthed(res);
      return;
    }
    // пере-привязка при входе (после recovery): подтверждаем новый секрет
    const p = pendingRef.current || { login: login.trim(), password };
    const res = await api('POST', '/api/auth/email/totp/confirm', { login: p.login, password: p.password, code: c });
    setBusy(false);
    if (!res || res.error) { setError(errT(res)); return; }
    if (onAuthed) onAuthed(res);
  }, [code, mode, regToken, login, password, errT, onAuthed, t]);

  // forgot: после recovery.finish пробуем войти, чтобы получить новый секрет
  const submitLoginInner = useCallback(async (lg, pw) => {
    const res = await api('POST', '/api/auth/email/login', { login: lg, password: pw });
    if (res && res.error === 'totp_setup_required' && res.totp_secret) {
      pendingRef.current = { login: lg, password: pw };
      await showEnroll(res.totp_secret, res.totp_uri, '');
      return;
    }
    if (res && !res.error && res.two_fa && res.login_token) {
      beginMailCode(res, lg);
      return;
    }
    if (res && !res.error && onAuthed) onAuthed(res);
  }, [showEnroll, onAuthed, beginMailCode]);

  // Второй шаг входа: код из письма → auth.login.verify → сессия.
  const submitMailCode = useCallback(async () => {
    const em = mailEmail.trim();
    const c = code.trim();
    if (!em) { setError(t('auth.err.badEmail')); return; }
    if (!c) { setError(t('auth.err.fillCode')); return; }
    setBusy(true);
    setError('');
    const res = await api('POST', '/api/auth/login/verify', {
      email: em, code: c, login_token: loginTokenRef.current,
    });
    setBusy(false);
    if (!res || res.error) { setError(errT(res)); return; }
    if (onAuthed) onAuthed(res);
  }, [code, mailEmail, errT, onAuthed, t]);

  const onFieldKey = (e, submit) => {
    if (e.key === 'Enter' && !busy) submit();
  };

  return (
    <div className="authf pf-auth">
      <div className="authf-tabs" role="tablist">
        <button
          className={'authf-tab' + (mode !== 'register' ? ' is-active' : '')}
          onClick={() => switchMode('login')}
        >
          {t('profile.tabLogin')}
        </button>
        <button
          className={'authf-tab' + (mode === 'register' ? ' is-active' : '')}
          onClick={() => switchMode('register')}
        >
          {t('profile.tabRegister')}
        </button>
      </div>

      {step === 'enroll' ? (
        <>
          <div className="authf-mailhint">
            <ShieldCheck size={15} strokeWidth={2} />
            <span>{t('profile.enrollTitle')}</span>
          </div>
          <p className="pf-hint">{t('profile.enrollHint')}</p>
          {enroll && enroll.uri && (
            <div className="pf-qr">
              {enroll.qr
                ? <img src={enroll.qr} alt={t('profile.qrAlt')} width="180" height="180" />
                : <Spinner size={22} />}
            </div>
          )}
          {mode === 'register' && qrLeft > 0 && (
            <div className={'pf-qr-timer' + (qrLeft <= 60 ? ' is-low' : '')}>
              <RefreshCw size={12} strokeWidth={2} />
              <span>{t('profile.qrRefreshIn', { time: fmtMMSS(qrLeft) })}</span>
            </div>
          )}
          {qrRefreshed && step === 'enroll' && (
            <div className="pf-qr-refreshed">{t('profile.qrRefreshed')}</div>
          )}
          {enroll && enroll.secret && (
            <div className="pf-secret mono">
              <span className="pf-secret-text">{enroll.secret}</span>
              <CopyBtn text={enroll.secret} t={t} />
            </div>
          )}
          {mode === 'register' && (
            <div className="pf-keyword">
              <div className="pf-keyword-title"><KeyRound size={14} strokeWidth={2} /> {t('profile.keywordTitle')}</div>
              <div className="pf-keyword-warn">{t('profile.keywordSelfNote')}</div>
            </div>
          )}
          {error && <div className="authf-err">{error}</div>}
          <label className="authf-field">
            <span>{t('profile.fldTotp')}</span>
            <input
              className="authf-input authf-code mono"
              value={code}
              inputMode="numeric"
              autoComplete="one-time-code"
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} maxLength={INPUT_LIMITS.code}
              onKeyDown={(e) => onFieldKey(e, submitCode)}
              placeholder="000000"
            />
          </label>
          <button className="btn btn-accent authf-submit" onClick={submitCode} disabled={busy}>
            {busy ? <Spinner size={15} /> : null}
            {t('profile.verifyBtn')}
          </button>
        </>
      ) : step === 'newpass' ? (
        <>
          <div className="authf-mailhint">
            <KeyRound size={15} strokeWidth={2} />
            <span>{t('profile.newPassTitle')}</span>
          </div>
          {error && <div className="authf-err">{error}</div>}
          <label className="authf-field">
            <span>{t('profile.fldNewPassword')}</span>
            <input
              className="authf-input"
              type="password"
              value={newPassword}
              onChange={limited(setNewPassword, INPUT_LIMITS.password)} maxLength={INPUT_LIMITS.password}
              onKeyDown={(e) => onFieldKey(e, submitNewPassword)}
              autoComplete="new-password"
            />
          </label>
          <PwStrength pw={newPassword} t={t} />
          <button className="btn btn-accent authf-submit" onClick={submitNewPassword} disabled={busy}>
            {busy ? <Spinner size={15} /> : null}
            {t('profile.verifyBtn')}
          </button>
          <button className="pf-back" onClick={() => { reset(); setMode('login'); }}>
            <ArrowLeft size={13} strokeWidth={2} />
            {t('profile.backToLogin')}
          </button>
        </>
      ) : step === 'mailcode' ? (
        <>
          <div className="authf-mailhint">
            <Mail size={15} strokeWidth={2} />
            <span>{t('profile.mailCodeTitle')}</span>
          </div>
          <p className="pf-hint">{t('profile.mailCodeHint')}</p>
          {error && <div className="authf-err">{error}</div>}
          <label className="authf-field">
            <span>{t('profile.fldEmail')}</span>
            <input
              className="authf-input"
              type="email"
              value={mailEmail}
              onChange={limited(setMailEmail, INPUT_LIMITS.email)} maxLength={INPUT_LIMITS.email}
              onKeyDown={(e) => onFieldKey(e, submitMailCode)}
              autoComplete="email"
            />
          </label>
          <label className="authf-field">
            <span>{t('profile.fldMailCode')}</span>
            <input
              className="authf-input authf-code mono"
              value={code}
              inputMode="numeric"
              autoComplete="one-time-code"
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} maxLength={INPUT_LIMITS.code}
              onKeyDown={(e) => onFieldKey(e, submitMailCode)}
              placeholder="000000"
            />
          </label>
          <button className="btn btn-accent authf-submit" onClick={submitMailCode} disabled={busy}>
            {busy ? <Spinner size={15} /> : null}
            {t('profile.verifyBtn')}
          </button>
          <button className="pf-back" onClick={() => { reset(); setMode('login'); }}>
            <ArrowLeft size={13} strokeWidth={2} />
            {t('profile.backToLogin')}
          </button>
        </>
      ) : step === 'code' ? (
        <>
          {error && <div className="authf-err">{error}</div>}
          <label className="authf-field">
            <span>{t('profile.fldTotp')}</span>
            <input
              className="authf-input authf-code mono"
              value={code}
              inputMode="numeric"
              autoComplete="one-time-code"
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} maxLength={INPUT_LIMITS.code}
              onKeyDown={(e) => onFieldKey(e, submitCode)}
              placeholder="000000"
            />
          </label>
          <button className="btn btn-accent authf-submit" onClick={submitCode} disabled={busy}>
            {busy ? <Spinner size={15} /> : null}
            {t('profile.verifyBtn')}
          </button>
          <button className="pf-back" onClick={() => { if (mode === 'login') { setStep('form'); setError(''); } else { reset(); setMode('login'); } }}>
            <ArrowLeft size={13} strokeWidth={2} />
            {t('profile.backToLogin')}
          </button>
        </>
      ) : mode === 'login' ? (
        <>
          {error && <div className="authf-err">{error}</div>}
          {history.length > 0 && (
            <div className="pf-history">
              <div className="pf-history-title">{t('profile.accountsHere')}</div>
              <div className="pf-history-list">
                {history.map((h) => (
                  <button
                    key={h.email}
                    type="button"
                    className="pf-history-chip"
                    onClick={() => { setLogin(h.nick || h.email); setEmail(h.email); }}
                  >
                    <UserRound size={12} strokeWidth={2} />
                    <span>{h.nick}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
          <label className="authf-field">
            <span>{t('profile.fldLogin')}</span>
            <input
              className="authf-input"
              value={login}
              onChange={limited(setLogin, INPUT_LIMITS.login)} maxLength={INPUT_LIMITS.login}
              onKeyDown={(e) => onFieldKey(e, submitLogin)}
              autoComplete="username"
            />
          </label>
          <label className="authf-field">
            <span>{t('profile.fldEmail')}</span>
            <input
              className="authf-input"
              type="email"
              value={email}
              onChange={limited(setEmail, INPUT_LIMITS.email)} maxLength={INPUT_LIMITS.email}
              onKeyDown={(e) => onFieldKey(e, submitLogin)}
              autoComplete="email"
            />
          </label>
          <label className="authf-field">
            <span>{t('profile.fldPassword')}</span>
            <input
              className="authf-input"
              type="password"
              value={password}
              onChange={limited(setPassword, INPUT_LIMITS.password)} maxLength={INPUT_LIMITS.password}
              onKeyDown={(e) => onFieldKey(e, submitLogin)}
              autoComplete="current-password"
            />
          </label>
          <button className="btn btn-accent authf-submit" onClick={submitLogin} disabled={busy}>
            {busy ? <Spinner size={15} /> : null}
            {t('profile.loginBtn')}
          </button>
          <button className="pf-back" onClick={() => { reset(); setMode('forgot'); }}>
            {t('profile.forgotLink')}
          </button>
        </>
      ) : mode === 'register' ? (
        <>
          {error && <div className="authf-err">{error}</div>}
          <label className="authf-field">
            <span>{t('profile.fldLogin')}</span>
            <input
              className="authf-input"
              value={login}
              onChange={limited(setLogin, INPUT_LIMITS.login)} maxLength={INPUT_LIMITS.login}
              onKeyDown={(e) => onFieldKey(e, submitRegister)}
              autoComplete="username"
            />
          </label>
          <label className="authf-field">
            <span>{t('profile.fldEmail')}</span>
            <input
              className="authf-input"
              type="email"
              value={email}
              onChange={limited(setEmail, INPUT_LIMITS.email)} maxLength={INPUT_LIMITS.email}
              onKeyDown={(e) => onFieldKey(e, submitRegister)}
              autoComplete="email"
            />
          </label>
          <label className="authf-field">
            <span>{t('profile.fldPassword')}</span>
            <input
              className="authf-input"
              type="password"
              value={password}
              onChange={limited(setPassword, INPUT_LIMITS.password)} maxLength={INPUT_LIMITS.password}
              onKeyDown={(e) => onFieldKey(e, submitRegister)}
              autoComplete="new-password"
            />
          </label>
          <PwStrength pw={password} t={t} />
          <label className="authf-field">
            <span>{t('profile.fldPassword2')}</span>
            <input
              className="authf-input"
              type="password"
              value={password2}
              onChange={limited(setPassword2, INPUT_LIMITS.password)} maxLength={INPUT_LIMITS.password}
              onKeyDown={(e) => onFieldKey(e, submitRegister)}
              autoComplete="new-password"
            />
          </label>
          <label className="authf-field">
            <span>{t('profile.fldSecretWord')}</span>
            <input
              className="authf-input"
              value={keyword}
              onChange={limited(setKeyword, INPUT_LIMITS.keyword)} maxLength={INPUT_LIMITS.keyword}
              onKeyDown={(e) => onFieldKey(e, submitRegister)}
              placeholder={t('profile.secretWordPlaceholder')}
            />
          </label>
          <p className="pf-hint">{t('profile.secretWordHint')}</p>
          <button className="btn btn-accent authf-submit" onClick={submitRegister} disabled={busy}>
            {busy ? <Spinner size={15} /> : null}
            {t('profile.registerBtn')}
          </button>
        </>
      ) : (
        <>
          {error && <div className="authf-err">{error}</div>}
          <label className="authf-field">
            <span>{t('profile.recoveryLogin')}</span>
            <input
              className="authf-input"
              value={login}
              onChange={limited(setLogin, INPUT_LIMITS.login)} maxLength={INPUT_LIMITS.login}
              onKeyDown={(e) => onFieldKey(e, submitForgot)}
              autoComplete="username"
            />
          </label>
          <label className="authf-field">
            <span>{t('profile.recoveryKeyword')}</span>
            <input
              className="authf-input mono"
              value={keyword}
              onChange={limited(setKeyword, INPUT_LIMITS.keyword)} maxLength={INPUT_LIMITS.keyword}
              onKeyDown={(e) => onFieldKey(e, submitForgot)}
              placeholder="word-word-word-word"
            />
          </label>
          <button className="btn btn-accent authf-submit" onClick={submitForgot} disabled={busy}>
            {busy ? <Spinner size={15} /> : null}
            {t('profile.forgotBtn')}
          </button>
          <button className="pf-back" onClick={() => { reset(); setMode('login'); }}>
            <ArrowLeft size={13} strokeWidth={2} />
            {t('profile.backToLogin')}
          </button>
        </>
      )}

      <div className="pf-base-note">
        <Check size={14} strokeWidth={2.5} />
        <span>{t('profile.baseIncluded')}</span>
      </div>
    </div>
  );
}
