import { useState, useEffect, useRef } from 'react';
import { openExternal } from '../../api';
import { useT } from '../../i18n';

const DURATIONS = { success: 3000, info: 4000, error: 0 };
const MAX_VISIBLE = 5;

function IconSuccess() {
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>;
}
function IconError() {
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>;
}
function IconInfo() {
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>;
}
function IconClose() {
  return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>;
}
function IconClip() {
  return <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>;
}
function IconBug() {
  return <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>;
}

function ToastItem({ toast, onRemove, version }) {
  const { t } = useT();
  const [exiting, setExiting] = useState(false);
  const [progress, setProgress] = useState(100);
  const startRef = useRef(Date.now());
  const frameRef = useRef(null);

  const duration = DURATIONS[toast.type] || 3000;

  useEffect(() => {
    if (duration <= 0) return;
    const animate = () => {
      const elapsed = Date.now() - startRef.current;
      const pct = Math.max(0, 100 - (elapsed / duration) * 100);
      setProgress(pct);
      if (pct > 0) {
        frameRef.current = requestAnimationFrame(animate);
      }
    };
    frameRef.current = requestAnimationFrame(animate);
    return () => { if (frameRef.current) cancelAnimationFrame(frameRef.current); };
  }, [duration]);

  useEffect(() => {
    if (duration <= 0) return;
    const timer = setTimeout(() => {
      setExiting(true);
      setTimeout(() => onRemove(toast.id), 250);
    }, duration);
    return () => clearTimeout(timer);
  }, [duration, toast.id, onRemove]);

  const handleDismiss = () => {
    setExiting(true);
    setTimeout(() => onRemove(toast.id), 250);
  };

  const copyText = async () => {
    try { await navigator.clipboard.writeText(toast.msg); } catch {}
  };

  const reportBug = () => {
    const v = version || '—';
    const title = encodeURIComponent('Error in ZPUI');
    const body = encodeURIComponent(`**Error description:**\n\`\`\`\n${toast.msg}\n\`\`\`\n\n**Version:** ${v}\n**OS:** Windows`);
    openExternal(`https://github.com/anomalyco/zpui/issues/new?title=${title}&body=${body}`);
  };

  const icons = { success: <IconSuccess />, error: <IconError />, info: <IconInfo /> };

  return (
    <div className={'toast toast-' + (toast.type || 'info') + (exiting ? ' toast-exit' : '')}>
      <div className="toast-icon">{icons[toast.type] || icons.info}</div>
      <div className="toast-body">
        <div className="toast-msg">{toast.msg}</div>
        {toast.type === 'error' && (
          <div className="toast-actions">
            <button className="toast-btn" data-tooltip={t('common.copy')} onClick={copyText}><IconClip /></button>
            <button className="toast-btn" data-tooltip={t('toast.reportBug')} onClick={reportBug}><IconBug /></button>
          </div>
        )}
      </div>
      <button className="toast-close" onClick={handleDismiss}><IconClose /></button>
      {duration > 0 && <div className="toast-progress" style={{ width: progress + '%' }} />}
    </div>
  );
}

export default function Toast({ toasts, onRemove, version }) {
  const visible = toasts.slice(-MAX_VISIBLE);
  return (
    <div className="toast-container">
      {visible.map(t => (
        <ToastItem key={t.id} toast={t} onRemove={onRemove} version={version} />
      ))}
    </div>
  );
}
