import { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { useT } from '../../i18n';
import useDialogA11y from '../../hooks/useDialogA11y';
import { usePageVisible } from '../../hooks/usePageVisible';

// Оверлеи монтируются в .stand-win: модалки центрируются в «окне приложения»
// (в том числе в восстановленном режиме 960×640), а не в канве браузера.
// Вне оболочки (AuthGate/стартап) фолбэк — body.
export function getOverlayRoot() {
  return document.querySelector('.stand-win') || document.body;
}

export function OverlayPortal({ children }) {
  return createPortal(children, getOverlayRoot());
}

// Modal — единый каркас диалога: шапка с иконкой-контекстом (tone:
// accent/success/warning/danger/neutral), заголовок, крестик 28px,
// тело со своим скроллом и футер действий справа.
// size: 'sm' (420) | undefined (520) | 'wide' (680); wide — legacy-алиас.
// Закрытие анимированное: при open→false узел держится 260мс с .is-closing
// на оверлее (CSS MOTION LAYER), затем размонтируется.
export default function Modal({
  title,
  titleExtra,
  icon,
  tone = 'accent',
  size,
  wide,
  footer,
  onClose,
  open,
  closable = true,
  closeOnOverlay = true,
  className,
  children,
}) {
  const { t } = useT();
  // 'open' → 'closing' (анимация выхода) → 'closed' (размонтирован)
  const [state, setState] = useState(open ? 'open' : 'closed');

  useEffect(() => {
    if (open) {
      setState('open');
      return;
    }
    setState(prev => (prev === 'open' ? 'closing' : prev));
  }, [open]);

  useEffect(() => {
    if (state !== 'closing') return undefined;
    const timer = setTimeout(() => setState('closed'), 300);
    return () => clearTimeout(timer);
  }, [state]);

  const closing = state === 'closing';
  // Keep-mounted вкладки: модалка скрытой страницы не рисуется (её портал
  // уходил бы поверх активной вкладки). Состояние страницы сохраняется —
  // вернувшись на вкладку, пользователь снова увидит открытую модалку.
  const pageVisible = usePageVisible();
  const dialogA11y = useDialogA11y({ open: open && !closing && pageVisible, onClose, closable });

  // На время closing родители успевают сбросить свои состояния (open=false)
  // — рисуем последний снимок контента, чтобы тело не мигнуло пустым.
  const contentRef = useRef(null);
  const footerRef = useRef(null);
  const hadFooterRef = useRef(false);
  if (!closing) {
    contentRef.current = children;
    footerRef.current = footer;
    hadFooterRef.current = !!footer;
  }

  if (state === 'closed' || !pageVisible) return null;

  const sizeClass = size === 'sm' || size === 'wide' || size === 'xl'
    ? ' modal-' + size
    : (wide ? ' modal-wide' : '');

  return createPortal(
    <div
      className={'modal-overlay' + (closing ? ' is-closing' : '')}
      onClick={closable && closeOnOverlay && !closing ? onClose : undefined}
    >
      <div
        ref={dialogA11y}
        className={'modal' + sizeClass + (className ? ' ' + className : '')}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        onClick={e => e.stopPropagation()}
      >
        {(title || icon) && (
          <div className="modal-header">
            {icon && <span className={'modal-icon tone-' + tone}>{icon}</span>}
            {title && (
              <span className="modal-title">
                {title}
                {titleExtra && <span className="modal-title-extra">{titleExtra}</span>}
              </span>
            )}
            {closable && (
              <button className="modal-close" data-sound="off" onClick={onClose} aria-label={t('common.close')}>
                <X size={16} strokeWidth={1.5} />
              </button>
            )}
          </div>
        )}
        <div className="modal-body">{closing ? contentRef.current : children}</div>
        {hadFooterRef.current && <div className="modal-footer">{footerRef.current}</div>}
      </div>
    </div>,
    getOverlayRoot()
  );
}
