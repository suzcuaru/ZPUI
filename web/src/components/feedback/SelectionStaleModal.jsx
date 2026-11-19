import { useT } from '../../i18n';
import Modal from '../ui/Modal';
import { History } from 'lucide-react';

// SelectionStaleModal — данные подбора устарели (прошло больше 7 дней с
// последнего ручного прогона). Однократность показа за сессию решает App
// (state, без localStorage); «Запустить подбор» открывает AutoSelectModal
// тем же механизмом, что кнопка «Автоподбор» в сайдбаре.
export default function SelectionStaleModal({ open, onClose, onRun }) {
  const { t } = useT();
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('selectionStale.title')}
      icon={<History size={18} strokeWidth={2} />}
      tone="accent"
      footer={(
        <>
          <button className="btn btn-sm" onClick={onClose}>
            {t('selectionStale.later')}
          </button>
          <button className="btn btn-accent btn-sm" onClick={onRun}>
            {t('selectionStale.run')}
          </button>
        </>
      )}
    >
      <p className="sweep-prompt-text">{t('selectionStale.body')}</p>
    </Modal>
  );
}
