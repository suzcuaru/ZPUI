import { useState } from 'react';
import { useT } from '../../i18n';

export default function OfflineScreen({ onRetry }) {
  const { t } = useT();
  const [loading, setLoading] = useState(false);

  const handleRetry = async () => {
    setLoading(true);
    await onRetry();
    setLoading(false);
  };

  return (
    <div className="offline-screen">
      <div className="offline-card">
        <div className="offline-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <line x1="1" y1="1" x2="23" y2="23"/>
            <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"/>
            <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"/>
            <path d="M10.71 5.05A16 16 0 0 1 22.56 9"/>
            <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"/>
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0"/>
            <line x1="12" y1="20" x2="12.01" y2="20"/>
          </svg>
        </div>
        <div className="offline-title">{t('offline.title')}</div>
        <div className="offline-desc">
          {t('offline.desc')}<br/>
          {t('offline.desc2')}
        </div>
        <button className="offline-retry-btn" onClick={handleRetry} disabled={loading}>
          {loading && <span className="offline-spinner" />}
          {loading ? t('offline.connecting') : t('offline.retry')}
        </button>
      </div>
    </div>
  );
}
