import { openExternal } from '../../api';

export default function Toast({ toasts, onRemove }) {
  const copyError = async (msg) => {
    try { await navigator.clipboard.writeText(msg); } catch {}
  };

  const reportError = (msg) => {
    const title = encodeURIComponent('Ошибка в ZPUI');
    const body = encodeURIComponent(`**Описание ошибки:**\n\`\`\`\n${msg}\n\`\`\`\n\n**Версия:** ...\n**ОС:** Windows`);
    openExternal(`https://github.com/bol-van/zapret/issues/new?title=${title}&body=${body}`);
  };

  return (
    <div className="toast-container">
      {toasts.map(t => (
        <div key={t.id} className={'toast toast-' + (t.type || 'info')}>
          <div className="toast-msg">{t.msg}</div>
          {t.type === 'error' ? (
            <div className="toast-actions">
              <button className="toast-btn" onClick={() => copyError(t.msg)} data-tooltip="Копировать">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
              </button>
              <button className="toast-btn" onClick={() => reportError(t.msg)} data-tooltip="Сообщить об ошибке">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
              </button>
              <button className="toast-btn" onClick={() => onRemove(t.id)} data-tooltip="Закрыть">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
              </button>
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}
