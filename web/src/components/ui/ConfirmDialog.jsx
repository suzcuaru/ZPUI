import { createContext, useContext, useState, useCallback } from 'react';
import { AlertTriangle } from 'lucide-react';
import { useT } from '../../i18n';

const ConfirmContext = createContext(null);

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

  const isDanger = state.variant === 'danger';

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {state.open && (
        <div className="modal-overlay" onClick={() => close(false)}>
          <div className="modal modal-sm cfm-modal" onClick={e => e.stopPropagation()}>
            <div className="cfm-body">
              <div className={'cfm-icon-wrap ' + (isDanger ? 'danger' : 'warn')}>
                <AlertTriangle size={22} strokeWidth={2.5} />
              </div>
              {state.title && <div className="cfm-title">{state.title}</div>}
              <div className="cfm-message">{state.message}</div>
            </div>
            <div className="cfm-actions">
              <button className="btn btn-sm btn-ghost cfm-cancel" onClick={() => close(false)}>
                {state.cancelText || t('common.cancel')}
              </button>
              <button
                className={'btn btn-sm cfm-ok ' + (isDanger ? 'btn-danger' : 'btn-accent')}
                onClick={() => close(true)}
                autoFocus
              >
                {state.confirmText || t('common.confirm')}
              </button>
            </div>
          </div>
        </div>
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
