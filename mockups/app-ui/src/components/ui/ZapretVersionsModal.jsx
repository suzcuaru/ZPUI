import { useState, useEffect } from 'react';
import Modal from './Modal';
import { api } from '../../api';
import { useT } from '../../i18n';
import { Loader2, HardDriveDownload, Check, AlertTriangle } from 'lucide-react';

export default function ZapretVersionsModal({ open, onClose, onInstall, installBusy }) {
  const { t } = useT();
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);

  const load = () => {
    setLoading(true);
    setData(null);
    api('GET', '/api/updates/zapret-available-versions').then(d => {
      setData(d);
      setLoading(false);
    });
  };

  useEffect(() => {
    if (!open) return;
    load();
  }, [open]);

  const versions = data?.versions || [];
  const current = data?.current || '';

  const formatDate = (dateStr) => {
    if (!dateStr) return '';
    try {
      return new Date(dateStr).toLocaleDateString();
    } catch {
      return '';
    }
  };

  return (
    <Modal
      title={t('settings.zapretVersionsTitle')}
      onClose={onClose}
      open={open}
      wide
      icon={<HardDriveDownload size={18} strokeWidth={2} />}
      tone="accent"
    >
      <div className="zapret-versions">
        {loading && (
          <div className="release-loading">
            <Loader2 size={24} className="spin" />
            <span>{t('update.loadingRelease')}</span>
          </div>
        )}

        {!loading && data?.error && (
          <div className="release-error" role="alert">
            <span>{t('settings.zapretVersionsError')}</span>
            <code>{data.error}</code>
            <button className="btn btn-sm" onClick={load}>{t('common.retry')}</button>
          </div>
        )}

        {!loading && !data?.error && versions.length === 0 && (
          <div className="release-loading">
            <span>{t('settings.zapretVersionsEmpty')}</span>
          </div>
        )}

        {!loading && versions.length > 0 && (
          <div className="zapret-versions-list">
            {versions.map(v => (
              <div key={v.source + '-' + v.version} className={'zapret-ver-row' + (v.version === current ? ' current' : '')}>
                <span className="zapret-ver-name">v{v.version}</span>
                <span className={'zapret-ver-src src-' + v.source}>
                  {v.source === 'yandex' ? t('settings.versionDisk') : 'GitHub'}
                </span>
                <span className="zapret-ver-date">{formatDate(v.modified)}</span>
                {v.version === current ? (
                  <span className="zapret-ver-cur">
                    <Check size={12} strokeWidth={2.5} />
                    {t('settings.zapretVersionsCurrent')}
                  </span>
                ) : (
                  <button className="upd-btn-check" disabled={installBusy} onClick={() => onInstall(v.version, v.source)}>
                    <HardDriveDownload size={11} strokeWidth={2.2} />
                    {t('update.install')}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}

        <div className="zapret-versions-hint">
          <AlertTriangle size={12} strokeWidth={2} />
          {t('settings.zapretVersionsHint')}
        </div>
      </div>
    </Modal>
  );
}
