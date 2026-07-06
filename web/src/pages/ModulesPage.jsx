import { useT } from '../i18n';
import { api, apiCall } from '../api';

function StatusBadge({ state, t }) {
  if (state === 'running') return <span className="badge badge-success"><span className="badge-dot" />{t('modules.running')}</span>;
  if (state === 'error') return <span className="badge badge-danger"><span className="badge-dot" />{t('modules.error')}</span>;
  return <span className="badge badge-warning"><span className="badge-dot" />{t('modules.stopped')}</span>;
}

export default function ModulesPage({ modules, showToast, onChange }) {
  const { t } = useT();

  const doAction = async (action, id, successMsg) => {
    const ok = await apiCall(() => api('POST', `/api/modules/${action}`, { id }), null, showToast);
    if (ok && successMsg) showToast(successMsg, 'success');
    onChange();
  };

  if (!modules || modules.length === 0) {
    return (
      <>
        <div className="page-title">{t('modules.title')}</div>
        <div className="page-subtitle">{t('modules.subtitle')}</div>
        <div className="empty-state">
          <div className="empty-state-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/></svg>
          </div>
          <div className="empty-state-title">{t('modules.empty')}</div>
          <div className="empty-state-text">{t('modules.emptyHint')}</div>
          <button className="btn btn-accent btn-sm" onClick={() => api('POST', '/api/modules/open-folder')}>
            {t('modules.openFolder')}
          </button>
        </div>
      </>
    );
  }

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <div className="page-title" style={{ flex: 1 }}>{t('modules.title')}</div>
        <button className="btn btn-sm" onClick={onChange}>{t('modules.reload')}</button>
        <button className="btn btn-sm" onClick={() => api('POST', '/api/modules/open-folder')}>{t('modules.openFolder')}</button>
      </div>
      <div className="page-subtitle">{t('modules.subtitle')}</div>

      <div className="mod-grid">
        {modules.map(m => (
          <div key={m.id} className="mod-card">
            <div className="mod-card-head">
              <div className="mod-card-icon">{(m.name || m.id || '?').charAt(0).toUpperCase()}</div>
              <div className="mod-card-body">
                <div className="mod-card-name">
                  {m.name || m.id}
                  <StatusBadge state={m.state} t={t} />
                </div>
                <div className="mod-card-meta">
                  {t('modules.version')}: <span className="mono" style={{ fontFamily: 'var(--font-mono)' }}>{m.version || '—'}</span>
                  {m.author ? ` · ${t('modules.by', { author: m.author })}` : ''}
                  {m.autostart ? ` · ${t('modules.autostart')}` : ''}
                </div>
              </div>
            </div>
            {m.description && <div className="mod-card-desc">{m.description}</div>}
            <div className="mod-card-actions">
              {!m.entry_ok && <span className="badge badge-danger">{t('modules.noEntry')}</span>}
              <button
                className="btn btn-sm btn-success"
                disabled={!m.entry_ok || m.disabled || m.state === 'running'}
                onClick={() => doAction('start', m.id, t('modules.started'))}
              >
                {t('modules.start')}
              </button>
              <button
                className="btn btn-sm btn-danger"
                disabled={m.state !== 'running'}
                onClick={() => doAction('stop', m.id, t('modules.stopped'))}
              >
                {t('modules.stop')}
              </button>
              <button
                className="btn btn-sm"
                disabled={!m.entry_ok || m.disabled}
                onClick={() => doAction('restart', m.id)}
              >
                {t('modules.restart')}
              </button>
              <span className="mono footer-mono" style={{ marginLeft: 'auto', fontSize: 10, color: 'var(--text-tertiary)' }}>
                {m.id}
              </span>
            </div>
          </div>
        ))}
      </div>
    </>
  );
}
