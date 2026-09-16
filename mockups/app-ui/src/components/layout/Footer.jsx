import { useState, useEffect } from 'react';
import { formatBytes } from '../../utils';
import { api } from '../../api';
import { useT } from '../../i18n';

export default function Footer({ status }) {
  const { t } = useT();
  const [serverOk, setServerOk] = useState(null);

  // Статус сервера управления — лёгкий опрос (30 сек), результат только для точки.
  useEffect(() => {
    let alive = true;
    const check = async () => {
      const s = await api('GET', '/api/server/status');
      if (alive) setServerOk(!!(s && !s.error && s.connected === true));
    };
    check();
    const iv = setInterval(check, 30000);
    return () => { alive = false; clearInterval(iv); };
  }, []);

  if (!status) return <div className="footer" />;

  const zRun = status?.zapret?.status === 'running';
  const strategy = status?.zapret?.strategy || '—';
  const mon = status?.monitor || {};
  const dlSpeed = mon.dl_speed_fmt || '0 B/s';
  const ulSpeed = mon.ul_speed_fmt || '0 B/s';
  const dlTotal = formatBytes(mon.download_bytes || 0);
  const ulTotal = formatBytes(mon.upload_bytes || 0);
  const hostname = status?.network?.hostname || '';
  const ips = status?.network?.ips || [];

  return (
    <div className="footer">
      <div className="ft-right">
        <div className={'ft-chip ft-engine'} data-tooltip={zRun ? t('footer.engineOn') : t('footer.engineOff')}>
          <span className={'ft-dot' + (zRun ? ' ok' : ' err')} />
          <span className="ft-label">{zRun ? t('footer.engineOn') : t('footer.engineOff')}</span>
        </div>

        <div className="ft-chip ft-speed">
          <span className="ft-tag dl">DL</span>
          <span className="ft-val accent">{dlSpeed}</span>
          <span className="ft-tag ul">UL</span>
          <span className="ft-val ok">{ulSpeed}</span>
        </div>

        <div className="ft-chip ft-totals">
          <span className="ft-tag muted">DL</span>
          <span className="ft-val">{dlTotal}</span>
          <span className="ft-tag muted">UL</span>
          <span className="ft-val">{ulTotal}</span>
        </div>

        <div className="ft-chip ft-strat">
          <span className="ft-key">{t('footer.strategy')}</span>
          <span className="ft-val" style={{ maxWidth: 140 }}>{strategy}</span>
        </div>

        <div className="ft-chip ft-host">
          <span className="ft-val">{hostname && `${hostname} · `}{ips[0] || '127.0.0.1'}</span>
        </div>

        <div className="ft-chip" data-tooltip={serverOk === null ? t('footer.serverChecking') : (serverOk ? t('footer.serverOk') : t('footer.serverErr'))}>
          <span className={'ft-dot' + (serverOk === null ? '' : serverOk ? ' ok' : ' err')} />
          <span className="ft-label">{t('footer.server')}</span>
        </div>
      </div>
    </div>
  );
}
