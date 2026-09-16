import { useT } from '../../i18n';
import { Loader2, Check, RotateCw } from 'lucide-react';
import Modal from '../ui/Modal';

const STEP_ORDER = ['files', 'restart', 'reinstall', 'verify'];

function stepIcon(status) {
  if (status === 'done') return <Check size={13} />;
  if (status === 'failed') return <span className="rcv-step-fail">✕</span>;
  if (status === 'start') return <Loader2 size={13} className="spinning" />;
  return null;
}

// RecoveryModal — модальное окно процедуры восстановления Запрета.
// По центру приложения, без возможности закрыть, пока идёт процесс.
// Фазы: progress (этапы) → success (закрывается сам) | restarting (перезапуск приложения).
export default function RecoveryModal({ open, phase, steps }) {
  const { t } = useT();
  if (!open) return null;

  const ordered = [...(steps || [])].sort(
    (a, b) => STEP_ORDER.indexOf(a.key) - STEP_ORDER.indexOf(b.key)
  );

  const headIcon = phase === 'success'
    ? <Check size={18} strokeWidth={2.2} />
    : phase === 'restarting'
      ? <RotateCw size={18} strokeWidth={2} className="spinning" />
      : <Loader2 size={18} strokeWidth={2} className="spinning" />;
  const headTone = phase === 'success' ? 'success' : phase === 'restarting' ? 'accent' : 'warning';
  const headline = phase === 'success'
    ? t('recovery.success')
    : phase === 'restarting'
      ? t('recovery.restarting')
      : t('recovery.title');

  return (
    <Modal
      title={headline}
      open={open}
      closable={false}
      size="sm"
      icon={headIcon}
      tone={headTone}
    >
      <div className="rcv-modal">
        {phase === 'restarting' && (
          <div className="rcv-modal-sub">{t('recovery.restartingDesc')}</div>
        )}
        {phase === 'progress' && ordered.length > 0 && (
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
    </Modal>
  );
}
