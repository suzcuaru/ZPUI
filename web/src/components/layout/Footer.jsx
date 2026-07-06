import { useT } from '../../i18n';

export default function Footer({ status }) {
  const { t } = useT();
  const version = status?.mod?.version || '0.0.0';
  const total = status?.app?.modules_count || 0;
  const running = status?.app?.running_count || 0;

  return (
    <div className="footer">
      <div className="footer-item footer-mono">{t('footer.version', { version })}</div>
      <span className="footer-sep" />
      <div className="footer-item">{t('modules.runningCount', { count: running })}</div>
      <span className="footer-sep" />
      <div className="footer-item">{t('modules.totalCount', { count: total })}</div>
      <div className="footer-item footer-right">{t('footer.ready')}</div>
    </div>
  );
}
