import { useState, useEffect } from 'react';
import Switch from '../components/ui/Switch';
import Row from '../components/ui/Row';
import { api, apiCall } from '../api';
import { useT } from '../i18n';

export default function XboxDnsPage({ status, showToast }) {
  const { t } = useT();
  const [cfg, setCfg] = useState(null);
  const [autoStart, setAutoStart] = useState(false);

  useEffect(() => { load(); }, []);

  const load = async () => {
    const d = await api('GET', '/api/xbox-dns/config');
    if (d) setCfg(d);
    const c = await api('GET', '/api/config');
    if (c) setAutoStart(c.auto_start_xbox_dns || false);
  };

  if (!cfg) return null;

  const enabled = cfg.enabled;
  const primary = cfg.primary_dns || '111.88.96.50';
  const secondary = cfg.secondary_dns || '111.88.96.51';

  const toggle = async (on) => {
    setCfg(prev => ({ ...prev, enabled: on }));
    await apiCall(
      () => api('POST', '/api/xbox-dns/config', { ...cfg, enabled: on }),
      on ? t('xboxdns.dnsEnabled') : t('xboxdns.dnsDisabled'),
      showToast
    );
    load();
  };

  const toggleAutoStart = async () => {
    const next = !autoStart;
    setAutoStart(next);
    const c = await api('GET', '/api/config');
    if (c) await api('POST', '/api/config', { ...c, auto_start_xbox_dns: next });
  };

  return (
    <>
      <div className={'xdns-status-card' + (enabled ? ' on' : '')}>
        <div className="xdns-status-left">
          <div className={'xdns-status-icon' + (enabled ? ' on' : '')}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="2" y="3" width="20" height="6" rx="2"/><rect x="2" y="15" width="20" height="6" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
          </div>
          <div className="xdns-status-info">
            <span className="xdns-status-title">{t('xboxdns.internalDns')}</span>
            <span className="xdns-status-desc">{enabled ? t('xboxdns.runningDesc') : t('xboxdns.stoppedDesc')}</span>
          </div>
        </div>
        <Switch checked={enabled} onChange={toggle} />
      </div>

      <div className="section">
        <div className="section-title">{t('xboxdns.dnsServers')}</div>
        <Row title={t('xboxdns.primaryDns')}>
          <span className="set-static mono">{primary}</span>
        </Row>
        <Row title={t('xboxdns.secondaryDns')}>
          <span className="set-static mono">{secondary}</span>
        </Row>
        <Row title={t('xboxdns.provider')}>
          <span className="set-static mono">xbox-dns.ru</span>
        </Row>
      </div>

      <div className="section">
        <div className="section-title">{t('xboxdns.options')}</div>
        <Row title={t('xboxdns.autoStart')} desc={t('xboxdns.autoStartDesc')}>
          <Switch checked={autoStart} onChange={toggleAutoStart} />
        </Row>
      </div>

      <div className="section">
        <div className="section-title">{t('xboxdns.howItWorks')}</div>
        <p className="xdns-explain">{t('xboxdns.explanation')}</p>
      </div>
    </>
  );
}
