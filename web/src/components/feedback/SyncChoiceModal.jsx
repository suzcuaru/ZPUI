import { useEffect, useState } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';
import { useToast } from '../../context/ToastContext';
import Modal from '../ui/Modal';
import Spinner from '../ui/Spinner';
import { Monitor, RefreshCw } from 'lucide-react';

const fmtDate = (v) => {
  if (!v) return '—';
  const d = new Date(v);
  return isNaN(d) ? '—' : d.toLocaleString(undefined, { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
};

// SyncChoiceModal — перенос данных с другого устройства аккаунта после входа
// (решение владельца 10.11.2026): список снапшотов синхронизации, текущее
// устройство помечено, дата/время последней синхронизации у каждого.
// Синхронизируются настройки, темы и данные; авто-пуш своего снапшота —
// раз в сутки в фоне.
export default function SyncChoiceModal({ open, onClose, onRestored }) {
  const showToast = useToast();
  const { t } = useT();
  const [rows, setRows] = useState(null); // null = грузится
  const [limit, setLimit] = useState(0);
  const [busy, setBusy] = useState('');
  const [err, setErr] = useState('');

  const load = () => {
    setErr('');
    api('GET', '/api/account/sync/devices').then((r) => {
      if (r?.error) { setErr(r.error); setRows([]); return; }
      setRows(Array.isArray(r.devices) ? r.devices : []);
      setLimit(Number(r.limit) || 0);
    }).catch(() => { setErr('network'); setRows([]); });
  };
  useEffect(() => { if (open) load(); }, [open]);

  const restore = async (deviceId) => {
    setBusy(deviceId);
    const r = await api('POST', '/api/account/sync/restore', { device_id: deviceId });
    setBusy('');
    if (r?.error) { showToast(r.error, 'error'); return; }
    showToast(t('sync.restored', { what: r.applied || '' }), 'success');
    if (onRestored) onRestored(r);
    onClose();
  };

  const others = (rows || []).filter((d) => !d.current);
  const current = (rows || []).find((d) => d.current);

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('sync.choiceTitle')}
      icon={<RefreshCw size={18} strokeWidth={2} />}
      tone="accent"
      footer={(
        <>
          <button className="btn btn-sm" onClick={onClose}>{t('sync.skip')}</button>
        </>
      )}
    >
      <p className="sweep-prompt-text">{t('sync.choiceText')}</p>
      {rows === null ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: '14px 0' }}><Spinner /></div>
      ) : (
        <>
          {others.length === 0 && (
            <p className="pf-hint pf-hint-plain">{t('sync.empty')}</p>
          )}
          {others.map((d) => (
            <div key={d.device_id} className="svc-row" style={{ gap: 8 }}>
              <Monitor size={15} strokeWidth={1.8} />
              <div style={{ display: 'flex', flexDirection: 'column', minWidth: 0, flex: 1 }}>
                <span className="pf-device-name">{d.device_name || d.device_id?.slice(0, 8)}</span>
                <span className="ms muted">{t('sync.lastAt', { date: fmtDate(d.updated_at) })}</span>
              </div>
              <button className="btn btn-sm btn-accent" disabled={busy === d.device_id} onClick={() => restore(d.device_id)}>
                {busy === d.device_id ? <Spinner size={13} /> : null}
                {t('sync.restore')}
              </button>
            </div>
          ))}
          {current && (
            <p className="ms muted" style={{ marginTop: 8 }}>{t('sync.thisDevice', { date: fmtDate(current.updated_at) })}</p>
          )}
          {limit > 0 && (
            <p className="ms muted" style={{ marginTop: 4 }}>{t('sync.slots', { used: (rows || []).length, limit })}</p>
          )}
          {err && <p className="ms" style={{ color: 'var(--danger)' }}>{t('sync.loadErr')}</p>}
        </>
      )}
    </Modal>
  );
}
