import { createContext, useContext } from 'react';

// Тосты вместо проп-дриллинга showToast через всё дерево: App создаёт
// showToast и провайдит его, потребители берут через useToast().
const ToastContext = createContext(() => {});

export function ToastProvider({ value, children }) {
  return <ToastContext.Provider value={value}>{children}</ToastContext.Provider>;
}

export function useToast() {
  return useContext(ToastContext);
}
