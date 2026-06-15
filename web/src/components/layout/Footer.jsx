export default function Footer({ status }) {
  if (!status) return null;

  const strategy = status?.zapret?.strategy || '—';
  const resPct = status?.mod?.resource_pct ?? -1;
  const hostname = status?.network?.hostname || '';
  const ips = status?.network?.ips || [];

  const pctColor = resPct >= 80 ? 'var(--success)' : resPct >= 50 ? 'var(--warning)' : 'var(--danger)';

  return (
    <div className="footer">
      <div className="footer-item">
        <span style={{ color: 'var(--text-tertiary)' }}>Стратегия:</span>
        <span className="footer-mono" style={{ color: 'var(--text-secondary)' }}>{strategy}</span>
      </div>

      <span className="footer-sep" />

      {resPct >= 0 && (
        <>
          <div className="footer-item footer-pct">
            <span style={{ fontWeight: 600, color: pctColor }}>{resPct}%</span>
            <span className="footer-pct-bar">
              <span className="footer-pct-fill" style={{ width: resPct + '%', background: pctColor }} />
            </span>
            <span style={{ color: 'var(--text-tertiary)' }}>доступно</span>
          </div>
          <span className="footer-sep" />
        </>
      )}

      <div className="footer-item footer-right">
        <span className="footer-mono" style={{ color: 'var(--text-tertiary)' }}>
          {hostname && `${hostname} `}
          {ips[0] || '127.0.0.1'}
        </span>
      </div>
    </div>
  );
}
