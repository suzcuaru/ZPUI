import { Star, Heart } from 'lucide-react';
import Modal from './ui/Modal';
import { useT } from '../i18n';

// StarPromoModal — просьба поставить звезду на GitHub: показывается при
// открытии окна не чаще раза в неделю (решение владельца 14.11.2026).
// «Поставить» открывает репозиторий в браузере по умолчанию и больше не
// напоминает; «Пропустить» откладывает сообщение ещё на неделю. Текст
// упоминает подписку на Boosty как способ поддержать проект.
const LAST_KEY = 'zpui-star-promo-last';
const DONE_KEY = 'zpui-star-promo-done';
const WEEK_MS = 7 * 24 * 60 * 60 * 1000;
export const REPO_URL = 'https://github.com/suzcuaru/ZPUI';

export function starPromoDue() {
  try {
    if (localStorage.getItem(DONE_KEY) === '1') return false;
    const raw = localStorage.getItem(LAST_KEY);
    if (raw === null) {
      // Первый запуск: отсчёт недели начинается сейчас — сообщение приходит
      // только после недели использования (решение владельца 14.11).
      localStorage.setItem(LAST_KEY, String(Date.now()));
      return false;
    }
    return Date.now() - Number(raw) >= WEEK_MS;
  } catch {
    return false;
  }
}

export default function StarPromoModal({ onClose }) {
  const { t } = useT();
  const dismiss = () => {
    try { localStorage.setItem(LAST_KEY, String(Date.now())); } catch {}
    onClose();
  };
  const star = () => {
    try { localStorage.setItem(DONE_KEY, '1'); } catch {}
    try { window.open(REPO_URL, '_blank'); } catch {}
    onClose();
  };
  return (
    <Modal
      title={t('starPromo.title')}
      icon={<Star size={15} strokeWidth={2} />}
      tone="accent"
      open={true}
      onClose={dismiss}
      footer={(
        <>
          <button className="btn btn-sm" onClick={dismiss}>{t('starPromo.later')}</button>
          <button className="btn btn-accent" onClick={star}>
            <Star size={14} strokeWidth={2.2} />
            {t('starPromo.star')}
          </button>
        </>
      )}
    >
      <div className="star-promo">
        <p className="star-promo-text">{t('starPromo.text')}</p>
        <div className="star-promo-boosty">
          <Heart size={13} strokeWidth={2.2} />
          <span>{t('starPromo.boosty')}</span>
        </div>
      </div>
    </Modal>
  );
}
