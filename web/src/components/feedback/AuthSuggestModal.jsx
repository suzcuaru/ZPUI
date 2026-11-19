import { useState } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { useToast } from '../../context/ToastContext';
import Modal from '../ui/Modal';
import { UserRound } from 'lucide-react';

// AuthSuggestModal — предложение авторизоваться при запуске без учётной
// записи. «Больше не показывать» persistится в config.json
// (server.auth_prompt_dismissed) — до нового устройства/сброса конфига.
export default function AuthSuggestModal({ open, onClose, onAuth }) {
  const showToast = useToast();
  const { t } = useT();
  const [dismissing, setDismissing] = useState(false);

  const dismissForever = async () => {
    setDismissing(true);
    const r = await api('POST', '/api/auth/prompt/dismissed', { on: true });
    setDismissing(false);
    if (r?.error) {
      if (showToast) showToast(r.error, 'error');
      return;
    }
    onClose();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('auth.suggestTitle')}
      icon={<UserRound size={18} strokeWidth={2} />}
      tone="accent"
      closeOnOverlay={false}
      footer={(
        <>
          <button className="btn btn-sm" onClick={dismissForever} disabled={dismissing}>
            {t('auth.suggestNever')}
          </button>
          <button className="btn btn-sm" onClick={onClose}>
            {t('auth.suggestLater')}
          </button>
          <button className="btn btn-accent btn-sm" onClick={onAuth}>
            {t('auth.suggestAuth')}
          </button>
        </>
      )}
    >
      <p className="sweep-prompt-text">{t('auth.suggestText')}</p>
      <ul className="sweep-prompt-list">
        <li>{t('auth.suggestPoint1')}</li>
        <li>{t('auth.suggestPoint2')}</li>
      </ul>
    </Modal>
  );
}
