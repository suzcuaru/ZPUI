import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { useT } from '../../i18n';
import useDialogA11y from '../../hooks/useDialogA11y';

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
export default function Modal({
  title,
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
  const modalRef = useDialogA11y({ open, onClose, closable });

  if (!open) return null;

  const sizeClass = size === 'sm' || size === 'wide'
    ? ' modal-' + size
    : (wide ? ' modal-wide' : '');

  return createPortal(
    <div className="modal-overlay" onClick={closable && closeOnOverlay ? onClose : undefined}>
      <div
        ref={modalRef}
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
            {title && <span className="modal-title">{title}</span>}
            {closable && (
              <button className="modal-close" onClick={onClose} aria-label={t('common.close')}>
                <X size={15} strokeWidth={2.5} />
              </button>
            )}
          </div>
        )}
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>,
    getOverlayRoot()
  );
}
