import { useT } from '../../i18n';

export default function Footer({ status, uiRegs }) {
  const { t } = useT();
  const version = status?.mod?.version || '0.0.0';
  const total = status?.app?.modules_count || 0;
  const running = status?.app?.running_count || 0;

  return (
    <div className="footer">
      {uiRegs && uiRegs.map((r, i) => (
        r.placement === 'statusbar' ? (
          <span key={r.module_id + ':' + r.comp_id} className="footer-ui-item" title={r.module_id}>
            {r.comp_type === 'dot' && <span className="footer-ui-dot" style={{ background: r.color || 'var(--success)' }} />}
            {r.comp_type === 'badge' && <span className="footer-ui-badge">{r.label || r.comp_id}</span>}
            {r.comp_type === 'button' && <span className="footer-ui-btn">{r.label || r.comp_id}</span>}
          </span>
        ) : null
      ))}
      <span className="footer-right">
        <span className="footer-item footer-mono">v{version}</span>
        <span className="footer-sep" />
        <span className="footer-item">{running}/{total}</span>
        <span className="footer-sep" />
        <span className="footer-item">{t('footer.ready')}</span>
      </span>
    </div>
  );
}
