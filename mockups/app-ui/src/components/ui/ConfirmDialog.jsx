import { createContext, useContext, useState, useCallback } from 'react';
import { AlertTriangle, Info } from 'lucide-react';
import { useT } from '../../i18n';
import useDialogA11y from '../../hooks/useDialogA11y';
import Modal, { getOverlayRoot } from './Modal';
import { createPortal } from 'react-dom';

const ConfirmContext = createContext(null);

const VARIANT_ICONS = {
  danger: { tone: 'danger', icon: <AlertTriangle size={18} strokeWidth={2} /> },
  warning: { tone: 'warning', icon: <AlertTriangle size={18} strokeWidth={2} /> },
  info: { tone: 'accent', icon: <Info size={18} strokeWidth={2} /> },
};

export function ConfirmProvider({ children }) {
  const { t } = useT();
  const [state, setState] = useState({ open: false });

  const confirm = useCallback((opts) => {
    if (typeof opts === 'string') opts = { message: opts };
    return new Promise((resolve) => {
      setState({ open: true, resolve, ...opts });
    });
  }, []);

  const close = useCallback((result) => {
    setState(prev => {
      prev.resolve?.(result);
      return { open: false };
    });
  }, []);

  const dismiss = useCallback(() => close(false), [close]);

  const variant = VARIANT_ICONS[state.variant] || VARIANT_ICONS.warning;
  const isDanger = state.variant === 'danger';

  const dialogRef = useDialogA11y({ open: state.open, onClose: dismiss, closable: true });

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {state.open && createPortal(
        <div className="modal-overlay" onClick={dismiss}>
          <div
            ref={dialogRef}
            className="modal modal-sm cfm-modal"
            role="dialog"
            aria-modal="true"
            aria-label={state.title || state.message || t('common.confirm')}
            tabIndex={-1}
            onClick={e => e.stopPropagation()}
          >
            <div className="modal-header">
              <span className={'modal-icon tone-' + variant.tone}>{variant.icon}</span>
              <span className="modal-title">{state.title || t('common.confirm')}</span>
            </div>
            <div className="modal-body">
              <div className="cfm-message" role={isDanger ? 'alert' : undefined}>{state.message}</div>
            </div>
            <div className="modal-footer">
              <button className="btn btn-sm btn-ghost" onClick={dismiss}>
                {state.cancelText || t('common.cancel')}
              </button>
              <button
                className={'btn ' + (isDanger ? 'btn-danger' : 'btn-accent')}
                onClick={() => close(true)}
                autoFocus
              >
                {state.confirmText || t('common.confirm')}
              </button>
            </div>
          </div>
        </div>,
        getOverlayRoot()
      )}
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  const fn = useContext(ConfirmContext);
  if (!fn) {
    return async () => {
      return true;
    };
  }
  return fn;
}
