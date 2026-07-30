import { useState, useEffect, useCallback } from 'react';
import Switch from '../components/ui/Switch';
import { api } from '../api';
import { useT } from '../i18n';
import { Gamepad2, Copy, Check, Server, Monitor, BookOpen, Settings as SettingsIcon, AlertCircle } from 'lucide-react';

export default function XboxDnsPage({ status, showToast }) {
  const { t } = useT();
  const [cfg, setCfg] = useState(null);
  const [autoStart, setAutoStart] = useState(false);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(null);

  const load = useCallback(async () => {
    const d = await api('GET', '/api/xbox-dns/config');
    if (d) setCfg(d);
    const c = await api('GET', '/api/config');
    if (c) setAutoStart(c.auto_start_xbox_dns || false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const copyText = async (text, key) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(key);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      showToast(t('toast.requestFailed'), 'error');
    }
  };

  if (!cfg) return null;

  const enabled = status?.xbox_dns?.enabled ?? cfg.enabled ?? false;
  const primaryDns = cfg.primary_dns || '111.88.96.50';
  const secondaryDns = cfg.secondary_dns || '111.88.96.51';

  const ips = status?.network?.ips || [];
  const lanIp = ips.find(ip => ip && ip !== '127.0.0.1' && ip !== '::1' && !ip.startsWith('169.254')) || ips[0] || '—';

  const toggleMaster = async () => {
    if (busy) return;
    const on = !enabled;
    setBusy(true);
    const r = await api('POST', '/api/xbox-dns/config', { ...cfg, enabled: on });
    if (r?.error) {
      showToast(r.error, 'error');
    } else {
      showToast(on ? t('xboxdns.dnsEnabled') : t('xboxdns.dnsDisabled'), on ? 'success' : 'info');
    }
    setBusy(false);
    load();
  };

  const toggleAutoStart = async () => {
    const next = !autoStart;
    setAutoStart(next);
    const c = await api('GET', '/api/config');
    if (c) await api('POST', '/api/config', { ...c, auto_start_xbox_dns: next });
  };

  const steps = [t('xboxdns.step1'), t('xboxdns.step2'), t('xboxdns.step3'), t('xboxdns.step4'), t('xboxdns.step5')];

  return (
    <>
      <div className={'xdns-hero' + (enabled ? ' on' : '')}>
        <div className="xdns-hero-left">
          <div className={'xdns-hero-icon' + (enabled ? ' on' : '')}>
            <Gamepad2 size={26} strokeWidth={2} />
          </div>
          <div className="xdns-hero-info">
            <span className="xdns-hero-title">{t('xboxdns.internalDns')}</span>
            <span className={'xdns-hero-desc' + (enabled ? ' on' : '')}>
              {enabled ? t('xboxdns.runningDesc') : t('xboxdns.stoppedDesc')}
            </span>
          </div>
        </div>
        <Switch checked={enabled} onChange={toggleMaster} loading={busy} />
      </div>

      <div className="xdns-grid">
        <div className="section xdns-section">
          <div className="xdns-section-head">
            <Server size={14} strokeWidth={2} />
            <span className="section-title">{t('xboxdns.serverInfo')}</span>
          </div>
          <p className="xdns-section-sub">{t('xboxdns.serverInfoDesc')}</p>
          <div className="xdns-server-list">
            <div className="xdns-server-row">
              <div className="xdns-server-meta">
                <span className="xdns-server-tag">{t('xboxdns.primaryDns')}</span>
                <span className="xdns-server-ip">{primaryDns}</span>
              </div>
              <button className="xdns-copy-btn" onClick={() => copyText(primaryDns, 'primary')}>
                {copied === 'primary' ? <Check size={13} strokeWidth={2.5} /> : <Copy size={13} strokeWidth={2} />}
              </button>
            </div>
            <div className="xdns-server-row">
              <div className="xdns-server-meta">
                <span className="xdns-server-tag">{t('xboxdns.secondaryDns')}</span>
                <span className="xdns-server-ip">{secondaryDns}</span>
              </div>
              <button className="xdns-copy-btn" onClick={() => copyText(secondaryDns, 'secondary')}>
                {copied === 'secondary' ? <Check size={13} strokeWidth={2.5} /> : <Copy size={13} strokeWidth={2} />}
              </button>
            </div>
          </div>
        </div>

        <div className="section xdns-section">
          <div className="xdns-section-head">
            <Monitor size={14} strokeWidth={2} />
            <span className="section-title">{t('xboxdns.thisComputer')}</span>
          </div>
          <p className="xdns-section-sub">{t('xboxdns.lanIpDesc')}</p>
          <div className="xdns-server-row">
            <div className="xdns-server-meta">
              <span className="xdns-server-tag">{t('xboxdns.lanIp')}</span>
              <span className="xdns-server-ip">{lanIp}</span>
            </div>
            <button className="xdns-copy-btn" onClick={() => copyText(lanIp, 'lan')}>
              {copied === 'lan' ? <Check size={13} strokeWidth={2.5} /> : <Copy size={13} strokeWidth={2} />}
            </button>
          </div>
        </div>

        <div className="section xdns-section xdns-guide">
          <div className="xdns-section-head">
            <BookOpen size={14} strokeWidth={2} />
            <span className="section-title">{t('xboxdns.setupGuide')}</span>
          </div>
          <ol className="xdns-steps">
            {steps.map((step, i) => (
              <li key={i}>
                <span className="xdns-step-num">{i + 1}</span>
                <span className="xdns-step-text">{step}</span>
              </li>
            ))}
          </ol>
        </div>

        <div className="section xdns-section">
          <div className="xdns-section-head">
            <SettingsIcon size={14} strokeWidth={2} />
            <span className="section-title">{t('xboxdns.options')}</span>
          </div>
          <div className="set-row">
            <div className="set-row-info">
              <span className="set-row-title">{t('xboxdns.autoStart')}</span>
              <span className="set-row-desc">{t('xboxdns.autoStartDesc')}</span>
            </div>
            <Switch checked={autoStart} onChange={toggleAutoStart} />
          </div>
          <div className="xdns-info-box">
            <AlertCircle size={13} strokeWidth={2} className="xdns-info-icon" />
            <span>{t('xboxdns.warning')}</span>
          </div>
        </div>
      </div>
    </>
  );
}
