import { useState, useEffect } from 'react';

const DURATIONS = { success: 3000, info: 4000, error: 0 };

function IconSuccess() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>; }
function IconError() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>; }
function IconInfo() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>; }
function IconClose() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>; }

function ToastItem({ toast, onRemove }) {
  const [exiting, setExiting] = useState(false);
  const duration = DURATIONS[toast.type] || 3000;

  useEffect(() => {
    if (duration <= 0) return;
    const timer = setTimeout(() => {
      setExiting(true);
      setTimeout(() => onRemove(toast.id), 250);
    }, duration);
    return () => clearTimeout(timer);
  }, [duration, toast.id, onRemove]);

  const dismiss = () => { setExiting(true); setTimeout(() => onRemove(toast.id), 250); };
  const icons = { success: <IconSuccess />, error: <IconError />, info: <IconInfo /> };

  return (
    <div className={'toast toast-' + (toast.type || 'info') + (exiting ? ' toast-exit' : '')}>
      <div className="toast-icon">{icons[toast.type] || icons.info}</div>
      <div className="toast-msg">{toast.msg}</div>
      <button className="toast-close" onClick={dismiss}><IconClose /></button>
    </div>
  );
}

export default function Toast({ toasts, onRemove }) {
  return (
    <div className="toast-container">
      {toasts.slice(-5).map(t => (
        <ToastItem key={t.id} toast={t} onRemove={onRemove} />
      ))}
    </div>
  );
}
