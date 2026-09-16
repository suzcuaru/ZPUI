import { useState, useEffect } from 'react';
import Modal from './Modal';
import { api } from '../../api';
import { useT } from '../../i18n';
import { Megaphone } from 'lucide-react';

export default function BannerModal() {
  const { t } = useT();
  const [queue, setQueue] = useState([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const rt = window.runtime;
    if (!rt?.EventsOn) return;
    const handler = (data) => {
      const items = Array.isArray(data) ? data : (data && data.banners) || [];
      if (items.length) setQueue((prev) => [...prev, ...items.filter((b) => b && b.id)]);
    };
    rt.EventsOn('server:banners', handler);
    return () => {
      try { rt.EventsOff('server:banners'); } catch {}
    };
  }, []);

  const current = queue[0] || null;

  const dismiss = async () => {
    if (!current || busy) return;
    setBusy(true);
    try {
      await api('POST', '/api/banners/dismiss', { id: current.id });
    } catch {}
    setQueue((prev) => prev.slice(1));
    setBusy(false);
  };

  if (!current) return null;

  return (
    <Modal
      title={current.title || t('banner.title')}
      open
      closable={false}
      onClose={dismiss}
      icon={<Megaphone size={18} strokeWidth={2} />}
      tone="accent"
      footer={
        <button className="btn btn-sm" disabled={busy} onClick={dismiss}>{t('banner.dismiss')}</button>
      }
    >
      <div className="banner-body">
        {current.text && <div className="banner-text">{current.text}</div>}
        {current.image && <img className="banner-image" src={current.image} alt="" />}
        {current.link && (
          <div className="banner-actions">
            <a
              className="banner-link"
              href={current.link}
              onClick={(e) => { e.preventDefault(); window.open(current.link, '_blank'); }}
            >{current.link_label || current.link}</a>
          </div>
        )}
      </div>
    </Modal>
  );
}
