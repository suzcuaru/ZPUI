import { useState, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import Row from '../components/ui/Row';
import { api } from '../api';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import { useServiceToggle } from '../hooks/useServiceToggle';
import { useDebouncedSave } from '../hooks/useDebouncedSave';

export default function ProxyPage({ status, showToast }) {
  const { t } = useT();
  const [devices, setDevices] = useState([]);
  const [config, setConfig] = useState({ port: 1080, username: '', password: '', auto_start: false });

  const pRun = status?.proxy?.running === true;
  const port = status?.proxy?.port || 1080;

  const proxy = useServiceToggle('proxy', pRun, showToast);
  const saveProxyConfig = useDebouncedSave('/api/proxy/config', 500, () => showToast(t('proxy.settingsSaved'), 'success'));

  const loadData = useCallback(async () => {
    const [devs, cfg] = await Promise.all([
      api('GET', '/api/proxy/connections'),
      api('GET', '/api/proxy/config'),
    ]);
    if (devs?.by_client) {
      const deviceInfo = devs.device_info || {};
      const deviceList = Object.entries(devs.by_client).map(([ip, conns]) => ({
        ip,
        hostname: deviceInfo[ip]?.hostname || ip,
        mac: deviceInfo[ip]?.mac || '',
        connections: Array.isArray(conns) ? conns.length : conns,
      }));
      setDevices(deviceList);
    }
    if (cfg) setConfig(cfg);
  }, []);

  usePolling(loadData, 5000);

  const updateConfig = (patch) => {
    const next = { ...config, ...patch };
    setConfig(next);
    saveProxyConfig(patch, next);
  };

  const totalConns = devices.reduce((sum, d) => sum + d.connections, 0);

  return (
    <>
      <div className="section">
        <Row
          title={t('proxy.socks5Proxy')}
          desc={`${pRun ? t('proxy.runningOnPort', { port }) : t('proxy.stopped')} · ${devices.length} ${t('proxy.devicesSuffix')} · ${totalConns} ${t('proxy.connSuffix')}`}
        >
          <button className={'btn ' + (pRun ? 'btn-danger' : 'btn-accent')} onClick={proxy.toggle} disabled={proxy.loading}>
            {proxy.loading ? '...' : pRun ? t('common.stop') : t('common.start')}
          </button>
        </Row>
      </div>

      <div className="section">
        <div className="section-title">{t('proxy.connectedDevices')}</div>
        {devices.length === 0 ? (
          <div style={{ color: 'var(--text-tertiary)', fontSize: 11, padding: 8 }}>{t('proxy.noDevices')}</div>
        ) : (
          <div className="proxy-devices">
            {devices.map(d => (
              <div key={d.ip} className="proxy-device">
                <span className="pd-status" />
                <div className="pd-info">
                  <span className="pd-host">{d.hostname || d.ip}</span>
                  <span className="pd-ip">{d.ip}{d.mac && ' · ' + d.mac}</span>
                </div>
                <div className="pd-latency">
                  <span className="pd-latency-val">{d.connections}</span>
                  <span className="pd-latency-label">{t('proxy.connSuffix')}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section">
        <div className="section-title">{t('proxy.proxySettings')}</div>
        <div className="form-group">
          <label>{t('proxy.port')}</label>
          <input type="number" className="form-input" value={config.port} min="1" max="65535"
            onChange={e => updateConfig({ port: parseInt(e.target.value) || 1080 })} />
        </div>
        <div className="form-group">
          <label>{t('proxy.username')}</label>
          <input type="text" className="form-input" value={config.username || ''} placeholder={t('proxy.noAuth')}
            onChange={e => updateConfig({ username: e.target.value })} />
        </div>
        <div className="form-group">
          <label>{t('proxy.password')}</label>
          <input type="password" className="form-input" value={config.password || ''} placeholder={t('proxy.noAuth')}
            onChange={e => updateConfig({ password: e.target.value })} />
        </div>
        <Row title={t('proxy.autoStartProxy')} desc={t('proxy.autoStartProxyDesc')}>
          <Switch checked={config.auto_start || false} onChange={() => updateConfig({ auto_start: !config.auto_start })} />
        </Row>
      </div>
    </>
  );
}
