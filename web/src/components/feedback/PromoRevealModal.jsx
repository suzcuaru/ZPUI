import { useState } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { useToast } from '../../context/ToastContext';
import Modal from '../ui/Modal';
import { Gift } from 'lucide-react';

// PromoRevealModal — подарочная карточка после первой регистрации
// (решение владельца 10.11.2026): закрытая анимированная карточка, по клику
// — раскрытие (3D-флип) с промокодом и описанием ниже. Код активируется
// кнопкой (promo.activate) или копируется. Показ управляется в CRM
// (Настройки → «Регистрация»); данные приходят в ответе register/verify.
export default function PromoRevealModal({ open, promo, onClose, onActivated }) {
  const showToast = useToast();
  const { t } = useT();
  const [revealed, setRevealed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(null); // {until, sub_name}
  const [copied, setCopied] = useState(false);

  if (!promo || !promo.code) return null;
  const code = promo.code;

  const activate = async () => {
    setBusy(true);
    const r = await api('POST', '/api/account/promo/activate', { code });
    setBusy(false);
    if (r?.error) { showToast(r.error, 'error'); return; }
    setDone(r);
    showToast(t('profile.promoDoneToast', { days: r.days || promo.days }), 'success');
    if (onActivated) onActivated(r);
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch { /* нет доступа к буферу */ }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('regpromo.title')}
      icon={<Gift size={18} strokeWidth={2} />}
      tone="accent"
      closeOnOverlay={false}
      footer={(
        <>
          <button className="btn btn-sm" onClick={onClose}>{t('regpromo.later')}</button>
          {!done && revealed && (
            <button className="btn btn-accent btn-sm" disabled={busy} onClick={activate}>
              {t('regpromo.activate')}
            </button>
          )}
          {done && <button className="btn btn-accent btn-sm" onClick={onClose}>{t('common.close')}</button>}
        </>
      )}
    >
      <div className="regpromo-stage">
        <button
          className={'regpromo-card' + (revealed ? ' open' : '')}
          onClick={() => !revealed && setRevealed(true)}
          aria-label={t('regpromo.reveal')}
        >
          <span className="regpromo-face regpromo-front">
            <Gift size={34} strokeWidth={1.6} />
            <span className="regpromo-front-title">{t('regpromo.tap')}</span>
          </span>
          <span className="regpromo-face regpromo-back">
            <span className="regpromo-code mono">{code}</span>
            <button
              className="btn btn-sm"
              onClick={(e) => { e.stopPropagation(); copy(); }}
              aria-label={t('common.copy')}
            >
              {copied ? t('common.copied') : t('common.copy')}
            </button>
          </span>
        </button>
      </div>
      {promo.title && <p className="regpromo-desc-title">{promo.title}</p>}
      <p className="regpromo-desc">
        {promo.description || t('regpromo.defaultDesc', { days: promo.days })}
        {promo.sub_name ? ` · ${promo.sub_name}` : ''}
      </p>
      {done && (
        <p className="ms" style={{ color: 'var(--success)', marginTop: 8 }}>
          {t('regpromo.activated', { until: done.until || '' })}
        </p>
      )}
    </Modal>
  );
}
