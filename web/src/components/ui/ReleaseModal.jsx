import { useState, useEffect } from 'react';
import Modal from './Modal';
import { api, openExternal } from '../../api';
import { useT } from '../../i18n';
import { ExternalLink, Loader2 } from 'lucide-react';

const ROUTES = {
  zpui: '/api/updates/release-notes',
  zapret: '/api/updates/zapret-release-notes',
};

export default function ReleaseModal({ open, onClose, currentVersion, latestVersion, target = 'zpui' }) {
  const { t } = useT();
  const [info, setInfo] = useState(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setInfo(null);
    api('GET', ROUTES[target] || ROUTES.zpui).then(d => {
      setInfo(d);
      setLoading(false);
    });
  }, [open, target]);

  const formatDate = (dateStr) => {
    if (!dateStr) return '';
    try {
      const d = new Date(dateStr);
      return d.toLocaleDateString();
    } catch {
      return dateStr;
    }
  };

  const tagName = info?.tag_name ? info.tag_name.replace(/^v/, '') : latestVersion;

  return (
    <Modal title={t('update.releaseTitle')} onClose={onClose} open={open} wide>
      <div className="release-modal">
        {loading && (
          <div className="release-loading">
            <Loader2 size={24} className="spin" />
            <span>{t('update.loadingRelease')}</span>
          </div>
        )}

        {!loading && info?.error && (
          <div className="release-error">
            <span>{t('update.releaseError')}</span>
            <code>{info.error}</code>
          </div>
        )}

        {!loading && info && !info.error && (
          <>
            <div className="release-header">
              <div className="release-version-info">
                <span className="release-tag">v{tagName}</span>
                {info.name && info.name !== info.tag_name && (
                  <span className="release-name">{info.name}</span>
                )}
              </div>
              {info.published_at && (
                <span className="release-date">{formatDate(info.published_at)}</span>
              )}
            </div>

            {currentVersion && (
              <div className="release-version-compare">
                <span className="release-ver-current">v{currentVersion}</span>
                <span className="release-arrow">→</span>
                <span className="release-ver-new">v{tagName}</span>
              </div>
            )}

            {info.body && (
              <div className="release-body">
                <pre className="release-notes">{info.body}</pre>
              </div>
            )}

            <div className="release-actions">
              {info.html_url && (
                <button className="btn btn-sm btn-ghost" onClick={() => openExternal(info.html_url)}>
                  <ExternalLink size={13} strokeWidth={2} />
                  GitHub
                </button>
              )}
              <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
