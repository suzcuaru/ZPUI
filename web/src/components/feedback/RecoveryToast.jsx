import { useT } from '../../i18n';
import { Loader, Check, AlertTriangle, X } from 'lucide-react';

const STEP_ORDER = ['files', 'restart', 'reinstall', 'verify'];

function stepIcon(status) {
  if (status === 'done') return <Check size={13} />;
  if (status === 'failed') return <AlertTriangle size={13} />;
  if (status === 'start') return <Loader size={13} className="spinning" />;
  return null;
}

export default function RecoveryToast({ open, steps, done, success, onClose }) {
  const { t } = useT();
  if (!open) return null;

  const ordered = [...steps].sort(
    (a, b) => STEP_ORDER.indexOf(a.key) - STEP_ORDER.indexOf(b.key)
  );
  const lastDone = [...ordered].reverse().find((s) => s.status === 'done' || s.status === 'failed');
  const headline = done
    ? (success ? t('recovery.success') : t('recovery.failed'))
    : (lastDone ? lastDone.message : t('recovery.title'));

  return (
    <div className={'rcv-toast' + (done ? (success ? ' done' : ' failed') : '')}>
      <div className="rcv-toast-head">
        <span className="rcv-toast-icon">
          {done ? (success ? <Check size={15} /> : <AlertTriangle size={15} />) : <Loader size={15} className="spinning" />}
        </span>
        <span className="rcv-toast-title">{headline}</span>
        {done && (
          <button className="rcv-toast-close" onClick={onClose} aria-label={t('common.close')}>
            <X size={14} />
          </button>
        )}
      </div>
      {!done && <div className="rcv-toast-sub">{t('recovery.title')}</div>}
      {ordered.length > 0 && (
        <ul className="rcv-steps">
          {ordered.map((s, i) => (
            <li key={s.key + i} className={'rcv-step is-' + s.status}>
              <span className="rcv-step-icon">{stepIcon(s.status)}</span>
              <span className="rcv-step-msg">{s.message}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
