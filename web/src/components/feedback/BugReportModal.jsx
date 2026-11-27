import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Send, ChevronDown } from 'lucide-react';
import Modal from '../ui/Modal';
import Spinner from '../ui/Spinner';
import { api } from '../../api';
import { useT } from '../../i18n';
import { useToast } from '../../context/ToastContext';
import { errorText } from '../../errors';

// BugReportModal — расширенный баг-репорт (как в 1С «Сообщить о проблеме»):
// показывается из модалки/тоста ошибки. Пользователь видит код и текст
// ошибки, может описать, что делал, и отправить. На сервер уходит обычное
// обращение (тема bug), но в сообщении — JSON с деталями: действие
// пользователя, код/текст ошибки, состояние движка, диагностика и хвост
// журнала — их собирает бэкенд (BugReportSend), логин/время проставляет
// сервер.
export default function BugReportModal({ open, onClose, code = '', error = '' }) {
  const showToast = useToast();
  const { t } = useT();
  const [action, setAction] = useState('');
  const [comment, setComment] = useState('');
  const [busy, setBusy] = useState(false);
  const [host, setHost] = useState('');
  const [accName, setAccName] = useState('');
  const [detailsOpen, setDetailsOpen] = useState(false);

  useEffect(() => {
    if (!open) return;
    setAction('');
    setComment('');
    setAccName('');
    setDetailsOpen(false);
    api('GET', '/api/status').then((s) => {
      if (s && !s.error) setHost(s?.network?.hostname || '');
    }).catch(() => {});
    api('GET', '/api/account/me').then((m) => {
      if (m && !m.error) setAccName(m.nick_display || m.display_name || m.login || '');
    }).catch(() => {});
  }, [open]);

  const errorDisplay = useMemo(() => {
    if (code) {
      const txt = errorText(t, code);
      if (txt && txt !== error) return txt;
    }
    return error || t('errors.unknown');
  }, [code, error, t]);

  const canSend = action.trim().length > 0 || comment.trim().length > 0;

  const send = async () => {
    setBusy(true);
    const r = await api('POST', '/api/feedback/bug-report', {
      code,
      error: errorDisplay,
      action: action.trim(),
      comment: comment.trim(),
    });
    setBusy(false);
    if (!r || r.error) {
      showToast(r?.error === 'auth_required' ? t('feedback.notAuthorized') : t('feedback.sendFail'), 'error');
      return;
    }
    showToast(t('feedback.sendOk'), 'success');
    onClose();
  };

  const now = useMemo(() => new Date().toLocaleString(), []);

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('bugreport.title')}
      icon={<AlertCircle size={18} strokeWidth={2} />}
      tone="danger"
      footer={(
        <>
          <button className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
          <button className="btn btn-accent" onClick={send} disabled={busy || !canSend}>
            {busy ? <Spinner size={13} /> : <Send size={13} strokeWidth={2} />}
            {t('bugreport.sendBtn')}
          </button>
        </>
      )}
    >
      <p className="pf-hint">{t('bugreport.hint')}</p>
      <div className="fb-meta">
        <span>{t('feedback.user')}: {accName || t('feedback.notAuthorized')}</span>
        {host && <span>{t('feedback.device')}: {host}</span>}
        <span>{t('feedback.date')}: {now}</span>
      </div>

      <div className="bugreport-error">
        {code && <span className="toast-error-code">{code}</span>}
        <span className="bugreport-error-text">{errorDisplay}</span>
      </div>

      <label className="dns-form-field">
        <span className="dns-form-label">{t('bugreport.actionLabel')} *</span>
        <input className="form-input" maxLength={500} value={action}
          placeholder={t('bugreport.actionPh')}
          onChange={(e) => setAction(e.target.value)} />
      </label>
      <label className="dns-form-field">
        <span className="dns-form-label">{t('bugreport.commentLabel')}</span>
        <textarea className="form-input fb-msg" rows={3} maxLength={1000} value={comment}
          placeholder={t('bugreport.commentPh')}
          onChange={(e) => setComment(e.target.value)} />
      </label>

      <button type="button" className="bugreport-details-toggle" onClick={() => setDetailsOpen((v) => !v)}>
        <ChevronDown size={12} strokeWidth={2} className={detailsOpen ? 'up' : ''} />
        {t('bugreport.detailsToggle')}
      </button>
      {detailsOpen && (
        <ul className="bugreport-details">
          <li>{t('bugreport.detail.action')}</li>
          <li>{t('bugreport.detail.error')}</li>
          <li>{t('bugreport.detail.device')}</li>
          <li>{t('bugreport.detail.engine')}</li>
          <li>{t('bugreport.detail.diagnostics')}</li>
          <li>{t('bugreport.detail.errors')}</li>
        </ul>
      )}
    </Modal>
  );
}
