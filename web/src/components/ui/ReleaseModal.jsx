import { useState, useEffect } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { X, ExternalLink, Package } from 'lucide-react';

export default function ReleaseModal({ component, version, onClose }) {
  const { t } = useT();
  const [release, setRelease] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    (async () => {
      setLoading(true);
      const d = await api('POST', '/api/components/release-info', { name: component });
      if (d?.error) {
        setError(d.error);
        setLoading(false);
        return;
      }
      setRelease(d);
      setLoading(false);
    })();
  }, [component]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal release-modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-title">
            <Package size={16} strokeWidth={2} />
            {' '}
            {component} — v{version}
          </span>
          <button className="modal-close" onClick={onClose}>
            <X size={16} strokeWidth={2} />
          </button>
        </div>
        <div className="modal-body">
          {loading && (
            <div className="release-loading">
              <span className="mini-spin" />
              <span>{t('common.loading')}</span>
            </div>
          )}
          {error && (
            <div className="release-error">
              <span className="release-error-icon">⚠</span>
              <span>{error}</span>
            </div>
          )}
          {release && (
            <div className="release-content">
              {release.tag_name && (
                <div className="release-tag">
                  <span className="release-tag-label">{t('settings.releaseTag')}</span>
                  <span className="release-tag-value">{release.tag_name}</span>
                </div>
              )}
              {release.release_page && (
                <a className="release-link" href={release.release_page}
                  target="_blank" rel="noopener noreferrer"
                  onClick={e => { e.preventDefault(); window.open(release.release_page); }}>
                  <ExternalLink size={14} strokeWidth={2} />
                  {t('settings.openReleasePage')}
                </a>
              )}
              {release.assets && release.assets.length > 0 && (
                <div className="release-assets">
                  <span className="release-assets-title">{t('settings.releaseAssets')}</span>
                  <ul className="release-assets-list">
                    {release.assets.map((a, i) => (
                      <li key={i} className="release-asset-item">
                        <span className="release-asset-name">{a.name}</span>
                        {a.browser_download_url && (
                          <a className="release-asset-dl" href={a.browser_download_url}
                            target="_blank" rel="noopener noreferrer">
                            {t('settings.download')}
                          </a>
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}