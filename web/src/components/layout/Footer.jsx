import { formatBytes } from '../../utils';
import { useT } from '../../i18n';

export default function Footer({ status }) {
  const { t } = useT();
  if (!status) return <div className="footer" />;

  const strategy = status?.zapret?.strategy || '—';
  const mon = status?.monitor || {};
  const dlSpeed = mon.dl_speed_fmt || '0 B/s';
  const ulSpeed = mon.ul_speed_fmt || '0 B/s';
  const dlTotal = formatBytes(mon.download_bytes || 0);
  const ulTotal = formatBytes(mon.upload_bytes || 0);
  const hostname = status?.network?.hostname || '';
  const ips = status?.network?.ips || [];

  // "up" / "down" - одинаковый визуальный паттерн для скорости и тоталов.
  // Слева слово up/down (моноширинно, чтобы совпадало по ширине), справа значение.
  return (
    <div className="footer">
      <div className="footer-item footer-metric">
        <span className="footer-tag down">DOWN</span>
        <span className="footer-mono footer-num spd" style={{ color: 'var(--accent)' }}>{dlSpeed}</span>
      </div>
      <div className="footer-item footer-metric">
        <span className="footer-tag up">UP</span>
        <span className="footer-mono footer-num spd" style={{ color: 'var(--success)' }}>{ulSpeed}</span>
      </div>

      <span className="footer-sep" />

      <div className="footer-item footer-metric">
        <span className="footer-tag down muted">DOWN</span>
        <span className="footer-mono footer-num total" style={{ color: 'var(--text-secondary)' }}>{dlTotal}</span>
      </div>
      <div className="footer-item footer-metric">
        <span className="footer-tag up muted">UP</span>
        <span className="footer-mono footer-num total" style={{ color: 'var(--text-secondary)' }}>{ulTotal}</span>
      </div>

      <span className="footer-sep" />

      <div className="footer-item">
        <span style={{ color: 'var(--text-tertiary)' }}>{t('footer.strategy')}</span>
        <span className="footer-mono" style={{ color: 'var(--text-secondary)' }}>{strategy}</span>
      </div>

      <div className="footer-item footer-right">
        <span className="footer-mono" style={{ color: 'var(--text-tertiary)' }}>
          {hostname && `${hostname} · `}
          {ips[0] || '127.0.0.1'}
        </span>
      </div>
    </div>
  );
}
