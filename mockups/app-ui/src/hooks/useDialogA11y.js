import { useEffect, useRef } from 'react';

const FOCUSABLE = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

// Программный фокус при CSS zoom на предке (оболочка стенда 1.25–1.8) —
// Chromium не всегда чтит preventScroll и прокручивает контейнеры с
// overflow:hidden, чтобы показать сфокусированный элемент. Сбрасываем
// «сбежавшую» прокрутку у цепочки предков диалога и у документа.
function resetStrayScroll(root) {
  for (let el = root; el; el = el.parentElement) {
    if (el.scrollLeft !== 0) el.scrollLeft = 0;
    if (el.scrollTop !== 0) el.scrollTop = 0;
  }
  const se = document.scrollingElement;
  if (se && se.scrollLeft !== 0) se.scrollLeft = 0;
}

// useDialogA11y — общий каркас доступности диалогов (модалок и drawers):
// закрытие по Esc, ловушка Tab, первичный фокус и возврат фокуса при закрытии.
// Атрибуты role="dialog"/aria-modal вешает сам компонент; хук возвращает ref
// для корневого элемента диалога. Используется в ui/Modal и модалках вне ui/.
// focusDelay — отложить первичный фокус на N мс: диалоги со slide-анимацией
// (drawer логов) должны фокусироваться ПОСЛЕ выхода панели на место, иначе
// в момент фокуса цель стоит за translateX(100%) и zoom-квирк из resetStrayScroll
// успевает сдвинуть окно (смещение, «возвращающееся» в конце слайда).
export default function useDialogA11y({ open, onClose, closable = true, focusDelay = 0 }) {
  const ref = useRef(null);
  // Триггер запоминаем на МОМЕНТ ОТКРЫТИЯ (переход closed→open), а не в
  // эффекте первичного фокуса: onClose у коллеров часто создаётся заново на
  // каждый рендер — пере-запуск эффекта не должен ни перехватывать фокус у
  // содержимого модалки, ни портить сохранённый триггер.
  const prevFocusRef = useRef(null);
  const wasOpenRef = useRef(false);
  const onCloseRef = useRef(onClose);

  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);

  useEffect(() => {
    if (open && !wasOpenRef.current) {
      wasOpenRef.current = true;
      prevFocusRef.current = document.activeElement;
    }
    if (!open && wasOpenRef.current) {
      wasOpenRef.current = false;
      const el = prevFocusRef.current;
      prevFocusRef.current = null;
      if (el && document.contains(el) && typeof el.focus === 'function') {
        el.focus({ preventScroll: true });
      }
    }
  }, [open]);

  useEffect(() => {
    if (!open) return undefined;
    const onKeyDown = (e) => {
      if (e.key === 'Escape') {
        if (!closable) return;
        e.stopPropagation();
        onCloseRef.current?.();
        return;
      }
      if (e.key !== 'Tab') return;
      const root = ref.current;
      if (!root) return;
      const items = Array.from(root.querySelectorAll(FOCUSABLE)).filter(el => !el.disabled);
      if (items.length === 0) {
        e.preventDefault();
        root.focus({ preventScroll: true });
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = root.contains(document.activeElement) ? document.activeElement : null;
      if (e.shiftKey && (active === first || !active)) {
        e.preventDefault();
        last.focus({ preventScroll: true });
      } else if (!e.shiftKey && (active === last || !active)) {
        e.preventDefault();
        first.focus({ preventScroll: true });
      }
    };
    document.addEventListener('keydown', onKeyDown, true);
    return () => document.removeEventListener('keydown', onKeyDown, true);
  }, [open, closable]);

  useEffect(() => {
    if (!open) return undefined;
    let cancelled = false;
    let timerId = 0;
    const focusInitial = () => {
      if (cancelled) return;
      const root = ref.current;
      if (!root) return;
      const target = root.querySelector(FOCUSABLE);
      if (target) target.focus({ preventScroll: true });
      else root.focus({ preventScroll: true });
      if (focusDelay > 0) resetStrayScroll(root);
    };
    if (focusDelay > 0) {
      timerId = setTimeout(focusInitial, focusDelay);
    } else {
      focusInitial();
    }
    return () => {
      cancelled = true;
      clearTimeout(timerId);
    };
  }, [open, focusDelay]);

  return ref;
}
