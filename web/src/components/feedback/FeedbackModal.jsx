import { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { AlertCircle, Send, ChevronDown, Check } from 'lucide-react';
import Modal from '../ui/Modal';
import Spinner from '../ui/Spinner';
import { api } from '../../api';
import { useT } from '../../i18n';
import { useToast } from '../../context/ToastContext';

// TopicSelect — кастомная выпадающая панель выбора темы обращения
// (нативных select в приложении нет; паттерн — CatSelect из журнала логов).
// Панель рендерится порталом в body с fixed-позицией по rect кнопки:
// overflow модалки её обрезает.
function TopicSelect({ value, items, onChange, ariaLabel }) {
  const [open, setOpen] = useState(false);
  const [rect, setRect] = useState(null);
  const btnRef = useRef(null);

  const syncRect = () => {
    const b = btnRef.current;
    if (!b) return;
    const r = b.getBoundingClientRect();
    setRect({ left: r.left, top: r.bottom + 6, width: r.width });
  };

  useEffect(() => {
    if (!open) return undefined;
    syncRect();
    const onDoc = (e) => {
      // клик по самой кнопке и по панели закрытия не теряет выбор
      if (
        btnRef.current && btnRef.current.contains(e.target)
      ) return;
      if (e.target.closest && e.target.closest('.fb-topic-pop')) return;
      setOpen(false);
    };
    const onKey = (e) => { if (e.key === 'Escape') setOpen(false); };
    const onMove = () => syncRect();
    document.addEventListener('pointerdown', onDoc, true);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', onMove);
    document.addEventListener('scroll', onMove, true);
    return () => {
      document.removeEventListener('pointerdown', onDoc, true);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onMove);
      document.removeEventListener('scroll', onMove, true);
    };
  }, [open]);

  const current = items.find((i) => i.id === value);
  return (
    <>
      <button
        ref={btnRef}
        type="button"
        className={'form-input fb-topic-btn' + (open ? ' open' : '') + (current ? '' : ' placeholder')}
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
      >
        <span>{current ? current.label : ariaLabel}</span>
        <ChevronDown size={12} strokeWidth={2} className={'fb-topic-chev' + (open ? ' up' : '')} />
      </button>
      {open && rect && createPortal(
        <div
          className="fb-topic-pop"
          role="listbox"
          aria-label={ariaLabel}
          style={{ position: 'fixed', left: rect.left, top: rect.top, width: rect.width }}
        >
          {items.map((item) => (
            <button
              key={item.id}
              type="button"
              role="option"
              aria-selected={item.id === value}
              className={'fb-topic-item' + (item.id === value ? ' on' : '')}
              onClick={() => { onChange(item.id); setOpen(false); }}
            >
              <span>{item.label}</span>
              {item.id === value && <Check size={13} strokeWidth={2.5} />}
            </button>
          ))}
        </div>,
        document.body
      )}
    </>
  );
}

// FeedbackModal — «Сообщить о проблеме»: обязательная тема (без дефолта),
// свободный текст; имя пользователя, устройство и дата подставляются сами
// (логин/устройство сервер проставляет при получении — здесь только показ).
export default function FeedbackModal({ open, onClose, userName, initialTopic = '', initialText = '' }) {
  const showToast = useToast();
  const { t } = useT();
  const [topic, setTopic] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [host, setHost] = useState('');
  const [accName, setAccName] = useState('');

  useEffect(() => {
    if (!open) return;
    setTopic(initialTopic || '');
    setMessage(initialText || '');
    setAccName('');
    api('GET', '/api/status').then((s) => {
      if (s && !s.error) setHost(s?.network?.hostname || '');
    }).catch(() => {});
    // имя берём сами: пропс userName приходит не из всех мест вызова
    api('GET', '/api/account/me').then((m) => {
      if (m && !m.error) setAccName(m.nick_display || m.display_name || m.login || '');
    }).catch(() => {});
  }, [open]);

  const displayName = accName || userName || '';

  const topics = useMemo(() => ([
    'engine', 'proxy', 'telegram', 'dns', 'account', 'updates', 'ui', 'other',
  ].map((id) => ({ id, label: t(`feedback.topic.${id}`) }))), [t]);

  const now = useMemo(() => new Date().toLocaleString(), []);

  const send = async () => {
    setBusy(true);
    const r = await api('POST', '/api/feedback/send', { topic, message });
    setBusy(false);
    if (!r || r.error) {
      showToast(t('feedback.sendFail'), 'error');
      return;
    }
    showToast(t('feedback.sendOk'), 'success');
    onClose();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('feedback.title')}
      icon={<AlertCircle size={18} strokeWidth={2} />}
      tone="accent"
      footer={(
        <>
          <button className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
          <button className="btn btn-accent" onClick={send} disabled={busy || !topic || !message.trim()}>
            {busy ? <Spinner size={13} /> : <Send size={13} strokeWidth={2} />}
            {t('feedback.sendBtn')}
          </button>
        </>
      )}
    >
      <p className="pf-hint">{t('feedback.hint')}</p>
      <div className="fb-meta">
        <span>{t('feedback.user')}: {displayName || t('feedback.notAuthorized')}</span>
        {host && <span>{t('feedback.device')}: {host}</span>}
        <span>{t('feedback.date')}: {now}</span>
      </div>
      <label className="dns-form-field">
        <span className="dns-form-label">{t('feedback.topicLabel')} *</span>
        <TopicSelect
          value={topic}
          items={topics}
          onChange={setTopic}
          ariaLabel={t('feedback.topicPlaceholder')}
        />
      </label>
      <label className="dns-form-field">
        <span className="dns-form-label">{t('feedback.messageLabel')}</span>
        <textarea className="form-input fb-msg" rows={4} maxLength={4000} value={message}
          placeholder={t('feedback.messagePh')}
          onChange={(e) => setMessage(e.target.value)} />
      </label>
    </Modal>
  );
}
