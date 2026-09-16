import { useState, useEffect, useCallback } from 'react';
import Modal from './Modal';
import { api } from '../../api';
import { useT } from '../../i18n';
import { Download, CheckCircle2, XCircle, Package } from 'lucide-react';

/**
 * Модалка обновления компонентов с сервера ZPUI (по образцу модалки обновлений).
 * Открывается при старте, если сервер отдал компоненты новее установленных.
 * Прогресс — Wails event "server:component-progress".
 */
export default function ServerComponentsModal({ open, items, onClose }) {
  const { t } = useT();
  const [progress, setProgress] = useState({}); // name -> {pct,status,error,version}

  useEffect(() => {
    if (!window.runtime) return;
    const handler = (data) => {
      if (!data || !data.name) return;
      setProgress((p) => ({ ...p, [data.name]: data }));
    };
    window.runtime.EventsOn('server:component-progress', handler);
    return () => window.runtime.EventsOff('server:component-progress');
  }, []);

  const update = useCallback(async (item) => {
    setProgress((p) => ({ ...p, [item.name]: { pct: 0, status: 'download' } }));
    await api('POST', '/api/server/update-component', { name: item.name, url: item.url });
  }, []);

  if (!open) return null;
  const need = (items || []).filter((i) => i.has_update);

  return (
    <Modal
      title={t('srvupd.title')}
      open={open}
      onClose={onClose}
      wide
      icon={<Package size={18} strokeWidth={2} />}
      tone="accent"
      footer={
        <button className="btn btn-sm" onClick={onClose}>{t('srvupd.later')}</button>
      }
    >
      <div className="srvupd-list">
        <span className="srvupd-sub">{t('srvupd.subtitle')}</span>
        {need.map((item) => {
          const p = progress[item.name];
          return (
            <div key={item.name} className="srvupd-item">
              <span className="srvupd-icon"><Package size={15} strokeWidth={2} /></span>
              <div className="srvupd-main">
                <div className="srvupd-name">{item.name}</div>
                <div className="srvupd-vers mono">
                  {item.installed || '—'} → <b>{item.version}</b>
                </div>
                {p && !p.error && p.status !== 'done' && (
                  <div className="srvupd-bar">
                    <i style={{ width: (p.pct || 0) + '%' }} />
                  </div>
                )}
              </div>
              <div className="srvupd-action">
                {!p && (
                  <button className="btn btn-sm btn-accent" onClick={() => update(item)}>
                    <Download size={13} strokeWidth={2.2} /> {t('srvupd.update')}
                  </button>
                )}
                {p && p.status === 'done' && (
                  <span className="srvupd-ok"><CheckCircle2 size={14} strokeWidth={2.2} /> {t('srvupd.done')}</span>
                )}
                {p && p.error && (
                  <span className="srvupd-err" data-tooltip={p.error}><XCircle size={14} strokeWidth={2.2} /> {t('srvupd.error')}</span>
                )}
                {p && !p.error && p.status !== 'done' && (
                  <span className="srvupd-pct">{p.pct || 0}%</span>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </Modal>
  );
}
