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
  const [config, setConfig] = useState({ port: 1080, bind_host: '0.0.0.0', username: '', password: '', auto_start: false });

  const pRun = status?.proxy?.running === true;
  const port = config.port || status?.proxy?.port || 1080;
  const bindHost = config.bind_host || '0.0.0.0';
  const proxyAddr = `${bindHost}:${port}`;

  const proxy = useServiceToggle('proxy', pRun, showToast, {
    startMsg: t('header.proxyStarted'),
    stopMsg: t('header.proxyStopped'),
  });
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
      <div className="page-title">{t('proxy.title')}</div>

      <div className={'proxy-hero' + (pRun ? ' running' : '')}>
        <div className="proxy-hero-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 010 20M12 2a15 15 0 000 20"/></svg>
        </div>
        <div className="proxy-hero-body">
          <span className="proxy-hero-title">{t('proxy.socks5Proxy')}</span>
          <span className="proxy-hero-status">
            {pRun ? (
              <>socks5://<span className="proxy-addr">{proxyAddr}</span></>
            ) : t('proxy.stopped')}
          </span>
        </div>
        {pRun && (
          <div className="proxy-hero-stats">
            <div className="proxy-hero-stat">
              <span className="phs-val">{devices.length}</span>
              <span className="phs-label">{t('proxy.devicesSuffix')}</span>
            </div>
            <span className="proxy-hero-sep" />
            <div className="proxy-hero-stat">
              <span className="phs-val">{totalConns}</span>
              <span className="phs-label">{t('proxy.connSuffix')}</span>
            </div>
          </div>
        )}
        <button
          className={'proxy-hero-btn ' + (pRun ? 'stop' : 'start')}
          onClick={proxy.toggle}
          disabled={proxy.loading}
        >
          {proxy.loading ? <span className="mini-spin" /> : pRun ? t('common.stop') : t('common.start')}
        </button>
      </div>

      <div className="proxy-2col">
        <div className="section proxy-col-devices">
          <div className="section-title">{t('proxy.connectedDevices')}</div>
          {devices.length === 0 ? (
            <div className="proxy-empty">{t('proxy.noDevices')}</div>
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

        <div className="section proxy-col-settings">
          <div className="section-title">{t('proxy.proxySettings')}</div>
          <div className="form-group">
            <label>{t('proxy.port')}</label>
            <input type="number" className="form-input" value={config.port} min="1" max="65535"
              onChange={e => updateConfig({ port: parseInt(e.target.value) || 1080 })} />
          </div>
          <div className="form-group">
            <label>{t('proxy.address')}</label>
            <input type="text" className="form-input" value={config.bind_host || '0.0.0.0'}
              placeholder="0.0.0.0" onChange={e => updateConfig({ bind_host: e.target.value })} />
            <span className="form-hint">{t('proxy.addressDesc')}</span>
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
      </div>
    </>
  );
}
