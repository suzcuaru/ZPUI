import { useState, useEffect, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import Row from '../components/ui/Row';
import { api, apiCall } from '../api';
import { useT } from '../i18n';

export default function XboxDnsPage({ status, showToast }) {
  const { t } = useT();
  const [cfg, setCfg] = useState(null);
  const [autoStart, setAutoStart] = useState(false);
  const [toggling, setToggling] = useState(false);

  const load = useCallback(async () => {
    const d = await api('GET', '/api/xbox-dns/config');
    if (d) setCfg(d);
    const c = await api('GET', '/api/config');
    if (c) setAutoStart(c.auto_start_xbox_dns || false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const copyText = async (text, label) => {
    try {
      await navigator.clipboard.writeText(text);
      showToast(t('sysinfo.copied', { label }), 'success');
    } catch {
      showToast(t('toast.requestFailed'), 'error');
    }
  };

  if (!cfg) return null;

  // Источник правды — опрашиваемый status (каждые 2с).
  // Так тумблер синхронизируется и при переключении из боковой панели.
  const enabled = status?.xbox_dns?.enabled ?? cfg.enabled ?? false;
  const primary = cfg.primary_dns || '111.88.96.50';
  const secondary = cfg.secondary_dns || '111.88.96.51';

  const toggle = async () => {
    const on = !enabled;
    setToggling(true);
    const ok = await apiCall(
      () => api('POST', '/api/xbox-dns/config', { ...cfg, enabled: on }),
      on ? t('xboxdns.dnsEnabled') : t('xboxdns.dnsDisabled'),
      showToast
    );
    if (!ok) {
      load();
    }
    setToggling(false);
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
            <span className={'xdns-status-desc' + (enabled ? ' on' : '')}>{enabled ? t('xboxdns.runningDesc') : t('xboxdns.stoppedDesc')}</span>
          </div>
        </div>
        <Switch checked={enabled} onChange={toggle} loading={toggling} />
      </div>

      <div className="xdns-2col">
        <div className="section">
          <div className="section-title">{t('xboxdns.dnsServers')}</div>
          <Row title={t('xboxdns.primaryDns')}>
            <button className="set-copy-btn mono" onClick={() => copyText(primary, t('xboxdns.primaryDns'))}>
              {primary}
              <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            </button>
          </Row>
          <Row title={t('xboxdns.secondaryDns')}>
            <button className="set-copy-btn mono" onClick={() => copyText(secondary, t('xboxdns.secondaryDns'))}>
              {secondary}
              <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            </button>
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
          <p className="xdns-explain">{t('xboxdns.explanation')}</p>
        </div>
      </div>
    </>
  );
}
