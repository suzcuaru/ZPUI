import Modal from './ui/Modal';
import { useT } from '../i18n';
import { Download, ExternalLink } from 'lucide-react';

export default function UpdateAvailableModal({ data, onLaunch, onClose }) {
  const { t } = useT();
  if (!data) return null;

  const fmtVer = (v) => (v ? (v.startsWith('v') ? v : 'v' + v) : '—');
  const componentName = data.component === 'zapret' ? t('nav.zapret') : 'ZPUI';

  return (
    <Modal title={t('update.availableTitle')} onClose={onClose} open={true}>
      <div className="upd-modal-body">
        <div className="upd-modal-versions">
          <span className="upd-modal-comp">{componentName}</span>
          <span className="upd-modal-ver">{fmtVer(data.current)}</span>
          <span className="upd-modal-arrow">→</span>
          <span className="upd-modal-ver new">{fmtVer(data.latest)}</span>
        </div>
        <p className="upd-modal-text">{t('update.availableBody', { component: componentName, latest: fmtVer(data.latest) })}</p>
        <div className="upd-modal-actions">
          <button className="btn" onClick={onClose}>{t('update.later')}</button>
          <button className="btn btn-accent" onClick={onLaunch}>
            <ExternalLink size={13} strokeWidth={2.2} style={{ marginRight: 6 }} />
            {t('update.openCenter')}
          </button>
        </div>
      </div>
    </Modal>
  );
}
