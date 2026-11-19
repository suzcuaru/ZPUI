import { useState, useEffect, useCallback, useRef } from 'react';
import {
  UserRound, X, Monitor, LogOut, Check, RefreshCw,
  ArrowLeft, KeyRound, ShieldCheck, Trash2, Copy, Mail, TicketPercent,
  CheckCircle2, AlertCircle, Unlink, Plus,
} from 'lucide-react';
import { api, openExternal } from '../../api';
import { useT } from '../../i18n';
import ProgressBar from '../ui/ProgressBar';
import Spinner from '../ui/Spinner';
import Modal from '../ui/Modal';
import { useConfirm } from '../ui/ConfirmDialog';
import useDialogA11y from '../../hooks/useDialogA11y';
import { usePolling } from '../../hooks/usePolling';
import FeedbackModal from '../feedback/FeedbackModal';
import { useToast } from '../../context/ToastContext';
import AuthForm, { useErrT, CopyBtn, PwStrength, limited, stripCtl, INPUT_LIMITS } from '../account/AuthForm';

// ProfileDrawer — боковая панель профиля. Протокол v4: вход всегда
// пароль + код из приложения-аутентификатора, регистрация в приложении
// (QR + ключевое слово восстановления), recovery по ключевому слову с
// перепривязкой аутентификатора. Подписка — Boosty снаружи (привязка по
// email аккаунта). Сайт не используется.

const BOOSTY_URL = 'https://boosty.to/zpui';

// Границы ввода форм авторизации — зеркала серверных правил (логин 3–32,
// пароль до 72 — лимит bcrypt, почта до 254, код — 6 цифр, ключевое слово 64).
// stripCtl вырезает управляющие символы (нуль-байты, переводы строк) — они
// ломают однострочные поля и парсинг на сервере. Экранирование при выводе
// делает React, SQL-инъекции закрыты параметризованными запросами сервера;
// эти ограничения — клиентский второй рубеж.

function fmtBytes(b) {
  if (!b || b < 1024) return Math.round(b || 0) + ' B';
  if (b < 1048576) return (b / 1024).toFixed(0) + ' КБ';
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' МБ';
  return (b / 1073741824).toFixed(2) + ' ГБ';
}

function day(s) {
  const d = s && !isNaN(new Date(s).getTime()) ? new Date(s) : null;
  return d ? d.toLocaleDateString(undefined, { day: '2-digit', month: '2-digit', year: 'numeric' }) : null;
}


// AccountAction — встроенная форма чувствительной операции (ник/пароль/
// ключевое слово/удаление): все требуют код из аутентификатора.
function AccountAction({ kind, me, onDone, onClose }) {
  const showToast = useToast();
  const { t } = useT();
  const errT = useErrT(t);
  const [code, setCode] = useState('');
  const [f1, setF1] = useState(''); // nick | old password | —
  const [f2, setF2] = useState(''); // new password
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [keyword, setKeyword] = useState('');

  const titles = {
    nick: t('profile.changeNickBtn'),
    password: t('profile.changePassBtn'),
    keyword: t('profile.keywordBtn'),
    email: t('profile.changeEmailBtn'),
    delete: t('profile.deleteBtn'),
  };

  const submit = useCallback(async () => {
    if (kind === 'email') {
      if (!f1.trim()) { setError(t('auth.err.badEmail')); return; }
      if (!code.trim()) { setError(t('auth.err.fillCode')); return; }
      setBusy(true);
      setError('');
      const res = await api('POST', '/api/account/email/change', { email: f1.trim(), code: code.trim() });
      setBusy(false);
      if (!res || res.error) { setError(errT(res)); return; }
      showToast(t('profile.emailChanged'), 'success');
      onDone('email');
      return;
    }
    setBusy(true);
    setError('');
    let res = null;
    if (kind === 'nick') res = await api('POST', '/api/account/nick', { nick: f1.trim(), code: code.trim() });
    else if (kind === 'password') res = await api('POST', '/api/account/password', { password: f1, new_password: f2, code: code.trim() });
    else if (kind === 'keyword') res = await api('POST', '/api/account/keyword/rotate', { code: code.trim(), new_keyword: f1.trim() });
    else res = await api('POST', '/api/account/delete', { code: code.trim() });
    setBusy(false);
    if (!res || res.error) { setError(errT(res)); return; }
    if (kind === 'keyword') {
      setKeyword(res.keyword || '');
      showToast(t('profile.keywordRotated'), 'success');
      return;
    }
    showToast(kind === 'delete' ? t('profile.deleted') : t('profile.saved'), 'success');
    onDone(kind);
  }, [kind, f1, f2, code, errT, onDone, showToast, t]);

  const title = keyword ? t('profile.keywordTitle') : titles[kind];

  return (
    <div className="authf pf-auth">
      <div className="authf-mailhint">
        <ShieldCheck size={15} strokeWidth={2} />
        <span>{title}</span>
      </div>
      {keyword ? (
        <>
          <div className="pf-keyword">
            <div className="pf-keyword-value mono">
              <span>{keyword}</span>
              <CopyBtn text={keyword} t={t} />
            </div>
            <div className="pf-keyword-warn">{t('profile.keywordWarn')}</div>
          </div>
          <button className="btn btn-accent authf-submit" onClick={onClose}>{t('common.close')}</button>
        </>
      ) : (
        <>
          {kind === 'delete' && <p className="pf-hint pf-hint-danger">{t('profile.deleteWarn')}</p>}
          {kind === 'email' && (
            <label className="authf-field">
              <span>{t('profile.emailNew')}</span>
              <input
                className="authf-input"
                type="email"
                value={f1}
                onChange={limited(setF1, INPUT_LIMITS.email)} maxLength={INPUT_LIMITS.email}
                onKeyDown={(e) => { if (e.key === 'Enter' && !busy) submit(); }}
                autoComplete="email"
              />
            </label>
          )}
          {kind === 'keyword' && (
            <label className="authf-field">
              <span>{t('profile.keywordNew')}</span>
              <input
                className="authf-input mono"
                value={f1}
                onChange={(e) => setF1(stripCtl(e.target.value).slice(0, kind === "nick" ? INPUT_LIMITS.nick : kind === "keyword" ? INPUT_LIMITS.keyword : INPUT_LIMITS.password))}
                placeholder={t('profile.keywordNewPlaceholder')}
              />
            </label>
          )}
          {kind === 'nick' && (
            <label className="authf-field">
              <span>{t('profile.newNick')}</span>
              <input className="authf-input" value={f1} onChange={(e) => setF1(stripCtl(e.target.value).slice(0, kind === "nick" ? INPUT_LIMITS.nick : kind === "keyword" ? INPUT_LIMITS.keyword : INPUT_LIMITS.password))} placeholder={me && me.login} />
            </label>
          )}
          {kind === 'password' && (
            <>
              <label className="authf-field">
                <span>{t('profile.curPassword')}</span>
                <input className="authf-input" type="password" value={f1} onChange={(e) => setF1(stripCtl(e.target.value).slice(0, kind === "nick" ? INPUT_LIMITS.nick : kind === "keyword" ? INPUT_LIMITS.keyword : INPUT_LIMITS.password))} autoComplete="current-password" />
              </label>
              <label className="authf-field">
                <span>{t('profile.fldNewPassword')}</span>
                <input className="authf-input" type="password" value={f2} onChange={(e) => setF2(stripCtl(e.target.value).slice(0, INPUT_LIMITS.password))} autoComplete="new-password" />
              </label>
              <PwStrength pw={f2} t={t} />
            </>
          )}
          {error && <div className="authf-err">{error}</div>}
            <label className="authf-field">
              <span>{t('profile.fldTotp')}</span>
              <input
                className="authf-input authf-code mono"
                value={code}
                inputMode="numeric"
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} maxLength={INPUT_LIMITS.code}
                onKeyDown={(e) => { if (e.key === 'Enter' && !busy) submit(); }}
                placeholder="000000"
              />
            </label>
            <button className={'authf-submit ' + (kind === 'delete' ? 'btn btn-danger' : 'btn btn-accent')} onClick={submit} disabled={busy}>
              {busy ? <Spinner size={15} /> : kind === 'delete' ? <Trash2 size={14} strokeWidth={2} /> : null}
              {kind === 'delete' ? t('profile.deleteConfirmBtn') : t('profile.saveBtn')}
            </button>
            <button className="pf-back" onClick={onClose}>
              <ArrowLeft size={13} strokeWidth={2} />
              {t('common.close')}
            </button>
        </>
      )}
    </div>
  );
}

// SecCard — секция кабинета: маленький ярлык-заголовок + карточка.
// Карточки оформлены одинаково, заголовки секций дают быстрый скан панели.
function SecCard({ title, children }) {
  return (
    <div className="pf-sec">
      <div className="pf-sec-title">{title}</div>
      <div className="pf-card">{children}</div>
    </div>
  );
}

// SubscriptionCard — герой подвкладки «Подписка»: КРУПНОЕ название текущего
// тарифа, статус/срок и строка действий (Boosty — продлить/оформить).
// Промокод вынесен в отдельную подвкладку (PromoCard).
function SubscriptionCard({ me }) {
  const { t } = useT();
  const sub = me && me.subscription ? me.subscription : { active: false };
  const premium = !!sub.active;
  // срок: с датой — «до {дата}» + бар остатка (из 30 дней месяца подписки);
  // активная без даты — бессрочная, бар не рисуем
  const daysLeft = premium ? Number(sub.days_left) || 0 : 0;
  const termVariant = daysLeft <= 2 ? 'danger' : daysLeft <= 7 ? 'warning' : 'accent';
  const planName = premium ? (sub.name || 'Plus') : 'Base';
  // устройств по текущему плану: приходит в me.subscription.devices_limit
  // (биндинг /api/account/me → subscriptionMap); 0 = без лимита
  const hasDev = sub.devices_limit !== undefined && sub.devices_limit !== null;
  const devN = Number(sub.devices_limit) || 0;
  return (
    <div className={'pf-plan-hero' + (premium ? ' plus' : '')}>
      <div className="pf-plan-label">{t('profile.planCurrent')}</div>
      <div className="pf-plan-name">{planName}</div>
      <div className="pf-plan-meta">
        <span className={'plans-status ' + (premium ? 'on' : 'off')}>
          {premium ? t('profile.subActive') : t('profile.subInactive')}
        </span>
        {hasDev && (
          <span className="pf-plan-dev">
            <Monitor size={11} strokeWidth={2} />
            {devN > 0 ? t('profile.subsDevices', { n: devN }) : t('profile.subsDevicesUnlim')}
          </span>
        )}
        {premium && sub.ends_at && (
          <span className="pf-plan-term">
            {t('plans.until', { date: day(sub.ends_at) })}
            {daysLeft > 0 ? ` · ${t('profile.daysLeftShort', { n: daysLeft })}` : ''}
          </span>
        )}
        {premium && !sub.ends_at && <span className="pf-plan-term">{t('profile.subForever')}</span>}
      </div>
      {premium && sub.ends_at && (
        <ProgressBar value={Math.min(100, (daysLeft / 30) * 100)} variant={termVariant} size="sm" />
      )}
      {/* у бессрочной подписки (активна, без даты окончания) действий нет;
          «Продлить» появляется за 3 дня до окончания срока */}
      {!premium || (sub.ends_at && daysLeft <= 3) ? (
        <div className="pf-plan-actions">
          <button className="btn btn-accent" onClick={() => openExternal(sub.boosty_url || BOOSTY_URL)}>
            {premium ? t('profile.renewBtn') : t('profile.boostyBtn')}
          </button>
        </div>
      ) : null}
      {!premium && <p className="pf-hint pf-hint-plain">{t('profile.boostyHint')}</p>}
    </div>
  );
}

// PromoCard — активация промокода отдельной подвкладкой: акцентная карточка
// с крупным полем и кнопкой-акцентом, инлайн-состояния успеха/ошибки.
function PromoCard({ onRefresh }) {
  const { t } = useT();
  const errT = useErrT(t);
  const [promo, setPromo] = useState('');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(null); // {sub, days}
  const [error, setError] = useState('');
  const activate = async () => {
    const code = promo.trim();
    if (!code || busy) return;
    setBusy(true);
    setError('');
    setDone(null);
    const r = await api('POST', '/api/account/promo/activate', { code });
    setBusy(false);
    if (!r || r.error) { setError(errT(r)); return; }
    setDone({ sub: r.sub_name || 'Plus', days: r.days });
    setPromo('');
    onRefresh && onRefresh();
  };
  return (
    <div className="pf-promo-card">
      <div className="pf-promo-head">
        <div className="pf-promo-ico"><TicketPercent size={19} strokeWidth={2} /></div>
        <div className="pf-promo-head-text">
          <div className="pf-promo-title">{t('profile.promoTitle')}</div>
          <div className="pf-promo-hint">{t('profile.promoHint')}</div>
        </div>
      </div>
      <div className="pf-promo-field">
        <input
          className="pf-promo-input mono"
          value={promo}
          onChange={(e) => { setPromo(stripCtl(e.target.value).slice(0, 32)); setError(''); setDone(null); }}
          placeholder={t('profile.promoPlaceholder')}
          aria-label={t('profile.promoTitle')}
          onKeyDown={(e) => { if (e.key === 'Enter' && !busy) activate(); }}
        />
        <button className="btn btn-accent pf-promo-btn" onClick={activate} disabled={busy || !promo.trim()}>
          {busy ? <Spinner size={13} /> : <TicketPercent size={13} strokeWidth={2} />}
          {t('profile.promoActivate')}
        </button>
      </div>
      {done && (
        <div className="pf-promo-ok" role="status">
          <CheckCircle2 size={15} strokeWidth={2} />
          <span>{t('profile.promoOk', { sub: done.sub, days: done.days })}</span>
        </div>
      )}
      {error && (
        <div className="pf-promo-err" role="alert">
          <AlertCircle size={15} strokeWidth={2} />
          <span>{error}</span>
        </div>
      )}
    </div>
  );
}

// DevicesCard — устройства аккаунта: ПК с ZPUI (сессии с сервера) и телефон
// (Happ; активность = трафик туннеля). Чужие сессии и телефон можно отключить.
function DevicesCard() {
  const showToast = useToast();
  const { t } = useT();
  const errT = useErrT(t);
  const confirm = useConfirm();
  const [devices, setDevices] = useState(null); // null = грузится
  const [busy, setBusy] = useState('');
  const reload = useCallback(() => {
    api('GET', '/api/account/devices').then((r) => {
      setDevices(r && !r.error && Array.isArray(r.devices) ? r.devices : []);
    }).catch(() => setDevices([]));
  }, []);
  useEffect(() => { reload(); }, [reload]);
  const kill = async (sid) => {
    setBusy('s' + sid);
    const r = await api('POST', '/api/account/devices/kill', { session_id: sid });
    setBusy('');
    if (!r || r.error) { showToast(errT(r), 'error'); return; }
    showToast(t('profile.sessionKilled'), 'success');
    reload();
  };
  // Отвязать ПК-устройство от аккаунта: сессии завершаются (приложение само
  // выйдет), устройство исчезает из списка — вернётся при повторном входе.
  const unbind = async (devId) => {
    setBusy('u' + devId);
    const r = await api('POST', '/api/account/devices/unbind', { device_id: devId });
    setBusy('');
    if (!r || r.error) { showToast(errT(r), 'error'); return; }
    showToast(t('profile.deviceUnbound'), 'success');
    reload();
  };
  if (!devices || devices.length === 0) return null;
  // Список строится на сервере по id записей; страховка от дублей id —
  // первая строка с device_id выигрывает.
  const seen = new Set();
  const rows = devices.filter((d) => (seen.has(d.device_id) ? false : seen.add(d.device_id)));
  return (
    <SecCard title={t('profile.secDevices')}>
      <div className="pf-devices">
        {rows.map((d) => {
          const name = d.name || t('profile.deviceUnnamed');
          const meta = (d.os ? d.os + ' · ' : '') + t('profile.devicesLast', { date: day(d.last_login) });
          const killable = !d.current && d.session_id;
          const unbindable = !d.current;
          return (
            <div key={d.device_id} className={'pf-device-row' + (d.disabled ? ' off' : '')}>
              <Monitor size={15} strokeWidth={1.8} />
              <div className="pf-device-info">
                <span className="pf-device-name">
                  {name}{d.current ? ` · ${t('profile.devicesCurrent')}` : ''}{d.disabled ? ` · ${t('profile.deviceDisabled')}` : ''}
                </span>
                <span className="pf-device-sub">{meta}</span>
              </div>
              {killable ? (
                <button
                  className="pf-device-kill"
                  onClick={() => kill(d.session_id)}
                  disabled={busy === 's' + d.session_id}
                  aria-label={t('profile.sessionKill')}
                  data-tooltip={t('profile.sessionKill')}
                  data-tooltip-pos="left"
                >
                  {busy === 's' + d.session_id ? <Spinner size={13} /> : <X size={13} strokeWidth={2} />}
                </button>
              ) : null}
              {unbindable ? (
                <button
                  className="pf-device-kill"
                  onClick={() => confirm({
                    message: t('profile.unbindConfirm', { name }),
                    variant: 'danger',
                    confirmText: t('profile.unbindBtn'),
                  }).then((ok) => { if (ok) unbind(d.device_id); })}
                  disabled={busy === 'u' + d.device_id}
                  aria-label={t('profile.unbindBtn')}
                  data-tooltip={t('profile.unbindBtn')}
                  data-tooltip-pos="left"
                >
                  {busy === 'u' + d.device_id ? <Spinner size={13} /> : <Unlink size={13} strokeWidth={2} />}
                </button>
              ) : null}
            </div>
          );
        })}
      </div>
    </SecCard>
  );
}

// BannerCard — рекламный баннер (заполняет CRM, таблица banners). Скрыт,
// пока в базе нет активного баннера; поддерживает png/jpg/gif (анимированный
// GIF играет сам <img>), клик открывает ссылку баннера.
function BannerCard() {
  const { t } = useT();
  const [banner, setBanner] = useState(null);
  const [hidden, setHidden] = useState(false);
  useEffect(() => {
    let alive = true;
    api('GET', '/api/banner').then((r) => {
      if (!alive || !r || r.error) return;
      const placement = r.placement || 'profile';
      if (placement !== 'profile' && placement !== 'both') return;
      try {
        if (r.show_once && localStorage.getItem('zpui-profile-banner-dismissed') === String(r.id)) return;
      } catch { /* нет хранилища */ }
      setBanner(r);
    }).catch(() => {});
    return () => { alive = false; };
  }, []);
  if (!banner || hidden) return null;
  const close = () => {
    if (banner.id) {
      if (banner.show_once) {
        try { localStorage.setItem('zpui-profile-banner-dismissed', String(banner.id)); } catch { /* нет хранилища */ }
      }
    }
    setHidden(true);
  };
  const open = () => banner.link && openExternal(banner.link);
  return (
    <div className="pf-banner" style={{ position: 'relative' }}>
      <button className="sb-banner-x" aria-label={t('common.close')} onClick={close}>
        <X size={12} strokeWidth={2.4} />
      </button>
      {banner.kind === 'html' ? (
        <div className="sb-banner-html" dangerouslySetInnerHTML={{ __html: banner.html || '' }} />
      ) : banner.kind === 'text' ? (
        <div className="sb-banner-text" onClick={open} style={{ cursor: banner.link ? 'pointer' : 'default' }}>{banner.text}</div>
      ) : (
        <img
          src={banner.image}
          alt={banner.title || t('profile.bannerAlt')}
          onClick={open}
          style={{ cursor: banner.link ? 'pointer' : 'default' }}
        />
      )}
    </div>
  );
}

// SubsCard — тарифы: подписки с устройствами, описанием и ссылкой Boosty;
// оплата по-прежнему происходит на Boosty, привязку делает CRM. Скрытые
// тарифы сервера (is_hidden — системный/базовый план) в списке не показываются,
// в блоке «Текущий тариф» выше их отображение сохранено. Текущий тариф не
// помечается (решение владельца 14.11.2026): у каждого тарифа — количество
// устройств и описание.
function SubsCard() {
  const { t } = useT();
  const [subs, setSubs] = useState(null);
  useEffect(() => {
    api('GET', '/api/subscriptions').then((r) => {
      setSubs(r && !r.error && Array.isArray(r.subs) ? r.subs : []);
    }).catch(() => setSubs([]));
  }, []);
  const visible = subs ? subs.filter((s) => !(s.is_hidden === true || s.hidden === true)) : null;
  if (!visible || visible.length === 0) return null;
  return (
    <SecCard title={t('profile.secPlans')}>
      <div className="pf-subs">
        {visible.map((s) => {
          const paid = Number(s.price) > 0;
          const devN = Number(s.devices_limit) || 0;
          return (
            <div key={s.id} className="pf-sub">
              <div className="pf-sub-top">
                <span className="pf-sub-name">{s.name}</span>
                <span className={'pf-sub-price' + (paid ? '' : ' free')}>
                  {paid ? (
                    <>
                      {s.price} ₽
                      <small className="pf-sub-per">{t('profile.subsPerMonth')}</small>
                    </>
                  ) : t('profile.subsFree')}
                </span>
              </div>
              <div className="pf-sub-specs">
                <span className="pf-spec">
                  <Monitor size={11} strokeWidth={2} />
                  {devN > 0 ? t('profile.subsDevices', { n: devN }) : t('profile.subsDevicesUnlim')}
                </span>
              </div>
              {s.description && <p className="pf-sub-desc">{s.description}</p>}
              <button className="btn btn-accent pf-sub-btn" onClick={() => openExternal(s.boosty_url || BOOSTY_URL)}>
                {t('profile.boostyBtn')}
              </button>
            </div>
          );
        })}
      </div>
    </SecCard>
  );
}

// Подвкладки панели аккаунта (чип-табы, как в настройках):
// home — аккаунт-инфо + устройства, manage — безопасность/управление,
// sub — подписка, promo — промокод.
const PF_TABS = [
  { id: 'home', label: 'profile.tabHome' },
  { id: 'manage', label: 'profile.tabManage' },
  { id: 'sub', label: 'profile.tabSub' },
  { id: 'promo', label: 'profile.tabPromo' },
];

export default function ProfileDrawer({ open, onClose, me, arch, onAuthed, onLogout }) {
  const showToast = useToast();
  const { t } = useT();
  const authed = !!me;
  const dialogRef = useDialogA11y({ open, onClose, closable: true, focusDelay: 280 });
  const [tab, setTab] = useState('home');
  const [action, setAction] = useState(null); // null | nick | password | keyword | email | delete
  const [fbOpen, setFbOpen] = useState(false);

  // Панель открыли заново — всегда с «Главной», без полузавершённых форм.
  useEffect(() => {
    if (open) { setTab('home'); setAction(null); }
  }, [open]);

  const name = me ? (me.nick_display || me.display_name || me.login || '') : '';
  const initial = name ? name.charAt(0).toUpperCase() : '';

  const actionDone = useCallback((kind) => {
    setAction(null);
    if (kind === 'delete') {
      // аккаунт удалён: локальный токен уже очищен серверным вызовом —
      // выходим из состояния «вошёл»
      onLogout();
      return;
    }
    // ник/пароль меняются — досматриваем профиль без праздничного тоста
    onAuthed && onAuthed({}, { quiet: true });
  }, [onAuthed, onLogout]);

  return (
    <>
      <div className={'lg-overlay' + (open ? ' open' : '')} onClick={onClose} />
      <aside
        className={'lg-drawer pf-drawer' + (open ? ' open' : '')}
        ref={dialogRef}
        role="dialog"
        aria-label={t('profile.drawerTitle')}
      >
        <div className="lg-head">
          <div className="lg-head-icon"><UserRound size={18} strokeWidth={2} /></div>
          <span className="lg-head-title">{t('profile.drawerTitle')}</span>
          <div className="lg-spacer" />
          <button className="pf-x" onClick={onClose} aria-label={t('common.close')} data-sound="off">
            <X size={16} strokeWidth={2} />
          </button>
        </div>
        <div className="pf-body">
          {!authed ? (
            <AuthForm onAuthed={onAuthed} />
          ) : (
            <>
              <div className="pf-subtabs" role="tablist" aria-label={t('profile.subtabs')}>
                {PF_TABS.map((tb) => (
                  <button
                    key={tb.id}
                    type="button"
                    role="tab"
                    aria-selected={tab === tb.id}
                    className={'btn btn-sm pf-subtab' + (tab === tb.id ? ' active' : '')}
                    onClick={() => setTab(tb.id)}
                  >
                    {t(tb.label)}
                  </button>
                ))}
              </div>
              {/* Все панели рендерятся сразу (скрытые — display:none):
                  карточки грузят данные при открытии drawer параллельно,
                  и вкладка открывается мгновенно, без скелетонов. */}
              {PF_TABS.map((tb) => (
              <div className="pf-tabpane" key={tb.id} hidden={tab !== tb.id}>
                {tb.id === 'home' && (
                  <>
                    <div className="pf-acc">
                      <div className="pf-ava">{initial || <UserRound size={17} strokeWidth={2} />}</div>
                      <div className="pf-acc-info">
                        <div className="pf-acc-name">{name}</div>
                        {me.email && <div className="pf-acc-sub">{me.email}</div>}
                      </div>
                    </div>
                    <DevicesCard />
                    <BannerCard />
                    <button className="pf-back pf-rotate" onClick={() => setFbOpen(true)}>
                      <AlertCircle size={12} strokeWidth={2} />
                      {t('feedback.title')}
                    </button>
                  </>
                )}
                {tb.id === 'manage' && (
                  action ? (
                    <AccountAction
                      kind={action}
                      me={me}
                      onDone={actionDone}
                      onClose={() => setAction(null)}
                    />
                  ) : (
                    <>
                      <div className="pf-mgmt">
                        <div className="pf-mgmt-title">{t('profile.mgmtTitle')}</div>
                        <button className="pf-mgmt-btn" onClick={() => setAction('nick')}>
                          <UserRound size={13} strokeWidth={2} /> {t('profile.changeNickBtn')}
                        </button>
                        <button className="pf-mgmt-btn" onClick={() => setAction('password')}>
                          <KeyRound size={13} strokeWidth={2} /> {t('profile.changePassBtn')}
                        </button>
                        <button className="pf-mgmt-btn" onClick={() => setAction('email')}>
                          <Mail size={13} strokeWidth={2} /> {t('profile.changeEmailBtn')}
                        </button>
                        <button className="pf-mgmt-btn" onClick={() => setAction('keyword')}>
                          <ShieldCheck size={13} strokeWidth={2} /> {t('profile.keywordBtn')}
                        </button>
                        <button className="pf-mgmt-btn pf-mgmt-danger" onClick={() => setAction('delete')}>
                          <Trash2 size={13} strokeWidth={2} /> {t('profile.deleteBtn')}
                        </button>
                      </div>
                      <div className="pf-bottom">
                        <button className="btn btn-danger-ghost pf-logout" onClick={onLogout}>
                          <LogOut size={13} strokeWidth={2} />
                          {t('profile.logout')}
                        </button>
                      </div>
                    </>
                  )
                )}
                {tb.id === 'sub' && (
                  <>
                    <SubscriptionCard me={me} />
                    <SubsCard />
                  </>
                )}
                {tb.id === 'promo' && (
                  <PromoCard onRefresh={() => onAuthed({}, { quiet: true })} />
                )}
              </div>
              ))}
            </>
          )}
        </div>
      </aside>
      {authed && (
        <FeedbackModal open={fbOpen} onClose={() => setFbOpen(false)} userName={name} />
      )}
    </>
  );
}
