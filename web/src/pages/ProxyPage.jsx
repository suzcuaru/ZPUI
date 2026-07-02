import { useState, useEffect, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import { api, apiCall } from '../api';
import { useT } from '../i18n';

export default function ProxyPage({ status, showToast }) {
  const { t } = useT();
  const [devices, setDevices] = useState([]);
  const [conns, setConns] = useState(null);
  const [config, setConfig] = useState({ port: 1080, username: '', password: '', auto_start: false });
  const [pLoading, setPLoading] = useState(false);
  const [saveTimer, setSaveTimer] = useState(null);

  const pRun = status?.proxy?.running === true;
  const port = status?.proxy?.port || 1080;

  const loadData = useCallback(async () => {
    const [devs, c, cfg] = await Promise.all([
      api('GET', '/api/proxy/connections'),
      api('GET', '/api/proxy/status'),
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

  useEffect(() => {
    loadData();
    const iv = setInterval(loadData, 5000);
    return () => clearInterval(iv);
  }, [loadData]);

  const toggleProxy = async () => {
    setPLoading(true);
    await apiCall(() => api('POST', '/api/proxy/' + (pRun ? 'stop' : 'start')), null, showToast);
    await apiCall(() => api('POST', '/api/component-states'));
    setPLoading(false);
  };

  const updateConfig = (patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      clearTimeout(saveTimer);
      const timer = setTimeout(async () => {
        await apiCall(() => api('POST', '/api/proxy/config', {
          port: parseInt(next.port),
          username: next.username,
          password: next.password,
          auto_start: next.auto_start,
        }), t('proxy.settingsSaved'), showToast);
      }, 500);
      setSaveTimer(timer);
      return next;
    });
  };

  const totalConns = devices.reduce((sum, d) => sum + d.connections, 0);

  return (
    <>
      <div className="section">
        <div className="set-row">
          <div className="set-row-info">
            <span className="set-row-title">{t('proxy.socks5Proxy')}</span>
            <span className="set-row-desc">{pRun ? t('proxy.runningOnPort', { port }) : t('proxy.stopped')} · {devices.length} {t('proxy.devicesSuffix')} · {totalConns} {t('proxy.connSuffix')}</span>
          </div>
          <button className={'btn ' + (pRun ? 'btn-danger' : 'btn-accent')} onClick={toggleProxy} disabled={pLoading}>
            {pLoading ? '...' : pRun ? t('common.stop') : t('common.start')}
          </button>
        </div>
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
        <div className="set-row">
          <div className="set-row-info">
            <span className="set-row-title">{t('proxy.autoStartProxy')}</span>
            <span className="set-row-desc">{t('proxy.autoStartProxyDesc')}</span>
          </div>
          <Switch checked={config.auto_start || false} onChange={() => updateConfig({ auto_start: !config.auto_start })} />
        </div>
      </div>
    </>
  );
}
