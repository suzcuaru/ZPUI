import { useState } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { useT } from '../i18n';
import { Check } from 'lucide-react';

const VERDICT_META = {
  OK:         { key: 'checker.available', color: 'var(--success)', icon: '✓' },
  DNS_BLOCK:  { key: 'checker.dnsBlock',  color: 'var(--danger)',  icon: 'DNS' },
  TCP_BLOCK:  { key: 'checker.dnsBlock',  color: 'var(--danger)',  icon: 'TCP' },
  TLS_BLOCK:  { key: 'checker.dpiSni',    color: 'var(--danger)',  icon: 'TLS' },
  HTTP_STUB:  { key: 'checker.rknStub',   color: 'var(--danger)',  icon: 'HTTP' },
  TIMEOUT:    { key: 'checker.timeout',   color: 'var(--warning)', icon: '⏱' },
  DOWN:       { key: 'checker.unreachable', color: 'var(--danger)', icon: '✗' },
  UNKNOWN:    { key: 'checker.unknown',   color: 'var(--text-tertiary)', icon: '?' },
};

function LayerBadge({ ok, label, detail, ms, t }) {
  return (
    <div className={'rc-layer' + (ok ? ' ok' : ' fail')}>
      <span className="rc-layer-dot" />
      <div className="rc-layer-info">
        <span className="rc-layer-name">{label}</span>
        {detail && <span className="rc-layer-detail">{detail}</span>}
      </div>
      {ms > 0 && <span className="rc-layer-ms">{Math.round(ms)}{t('common.ms')}</span>}
    </div>
  );
}

export default function ResourceChecker({ onClose, showToast, initialUrl }) {
  const { t } = useT();
  const [url, setUrl] = useState(initialUrl || '');
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState(null);
  const [adding, setAdding] = useState(false);

  const handleCheck = async () => {
    if (!url.trim()) return;
    setLoading(true);
    setReport(null);
    const d = await api('POST', '/api/resource-check', { url: url.trim() });
    setLoading(false);
    if (!d) {
      showToast(t('checker.backendError'), 'error');
      return;
    }
    if (d?.error) {
      showToast(d.error, 'error');
    } else if (d?.report) {
      setReport(d.report);
    } else {
      showToast(t('toast.requestFailed'), 'error');
    }
  };

  const handleAddToList = async () => {
    if (!report?.Host) return;
    setAdding(true);
    const d = await api('POST', '/api/resource-add', { host: report.Host });
    setAdding(false);
    if (d?.status === 'ok') {
      showToast(t('checker.addedToList', { host: report.Host }), 'success');
      setReport(prev => ({ ...prev, InUserList: true }));
    } else if (d?.status === 'already_exists') {
      showToast(t('checker.alreadyInList'), 'info');
    } else {
      showToast(d?.error || t('common.error'), 'error');
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') handleCheck();
  };

  const meta = report ? (VERDICT_META[report.Direct?.Verdict] || VERDICT_META.UNKNOWN) : null;
  const vLabel = meta ? t(meta.key) : '';
  const confMap = { HIGH: t('checker.confHigh'), MEDIUM: t('checker.confMedium'), LOW: t('checker.confLow') };

  return (
    <Modal open onClose={onClose} title={t('checker.title')} wide>
      <div className="rc-container">
        <div className="rc-input-row">
          <input
            type="text"
            className="form-input"
            placeholder={t('checker.placeholder')}
            value={url}
            onChange={e => setUrl(e.target.value)}
            onKeyDown={handleKeyDown}
            autoFocus
          />
          <button className="btn btn-accent" onClick={handleCheck} disabled={loading || !url.trim()}>
            {loading ? t('common.checking') : t('common.check')}
          </button>
        </div>

        {loading && (
          <div className="rc-loading">
            <div className="loading-spinner-lg" />
            <span className="loading-sub">{t('checker.checkingLayers')}</span>
          </div>
        )}

        {report && !loading && (
          <div className="rc-results">
            <div className="rc-section">
              <div className="rc-section-title">{t('checker.provider')}</div>
              <div className="rc-provider-grid">
                <div className="rc-provider-item">
                  <span className="rc-provider-label">IP</span>
                  <span className="rc-provider-val mono">{report.Provider?.IP || '—'}</span>
                </div>
                <div className="rc-provider-item">
                  <span className="rc-provider-label">{t('checker.provider')}</span>
                  <span className="rc-provider-val">{report.Provider?.ISP || '—'}</span>
                </div>
                <div className="rc-provider-item">
                  <span className="rc-provider-label">ASN</span>
                  <span className="rc-provider-val mono">{report.Provider?.ASN || '—'}</span>
                </div>
                <div className="rc-provider-item">
                  <span className="rc-provider-label">{t('checker.city')}</span>
                  <span className="rc-provider-val">{report.Provider?.City || '—'}, {report.Provider?.Country || '—'}</span>
                </div>
              </div>
            </div>

            <div className="rc-section">
              <div className="rc-section-title">{t('checker.checkResult')}</div>
              <div className="rc-verdict-row">
                <div className="rc-verdict-card direct">
                  <span className="rc-verdict-label">{t('checker.direct')}</span>
                  <span className="rc-verdict-badge" style={{ background: meta.color + '22', color: meta.color }}>
                    {meta.icon} {vLabel}
                  </span>
                  <span className="rc-verdict-conf">{t('checker.confidence', { level: confMap[report.Direct?.Confidence] || '—' })}</span>
                </div>
              </div>
            </div>

            <div className="rc-section">
              <div className="rc-section-title">{t('checker.layerDetails')}</div>
              <div className="rc-layers">
                <LayerBadge
                  ok={report.Direct?.TCP?.Ok}
                  label="TCP"
                  detail={
                    report.Direct?.TCP?.Ok
                      ? t('checker.tcpOk')
                      : report.Direct?.TCP?.Error
                        ? t('checker.tcpError', { error: report.Direct.TCP.Error })
                        : t('checker.notResolving')
                  }
                  ms={report.Direct?.TCP?.TimeMs || 0}
                  t={t}
                />
                <LayerBadge
                  ok={report.Direct?.TLS?.Ok}
                  label="TLS"
                  detail={
                    report.Direct?.TLS?.Ok
                      ? t('checker.cert', { cn: report.Direct?.TLS?.Cert || '' })
                      : report.Direct?.TLS?.Error
                        ? t('checker.tcpError', { error: report.Direct.TLS.Error })
                        : t('checker.notResolving')
                  }
                  ms={report.Direct?.TLS?.TimeMs || 0}
                  t={t}
                />
                <LayerBadge
                  ok={report.Direct?.HTTP?.Ok}
                  label="HTTP"
                  detail={
                    report.Direct?.HTTP?.StubPage
                      ? t('checker.rknDetected')
                      : report.Direct?.HTTP?.Status
                        ? t('checker.httpStatus', { status: report.Direct.HTTP.Status })
                        : report.Direct?.HTTP?.Error || t('checker.noResponse')
                  }
                  ms={report.Direct?.HTTP?.TimeMs || 0}
                  t={t}
                />
              </div>
            </div>

            {report.Direct?.Notes && report.Direct.Notes.length > 0 && (
              <div className="rc-section">
                <div className="rc-section-title">{t('checker.notes')}</div>
                <ul className="rc-notes">
                  {report.Direct.Notes.map((n, i) => <li key={i}>{n}</li>)}
                </ul>
              </div>
            )}

            {report.Blocked && !report.InUserList && (
              <div className="rc-add-card">
                <div className="rc-add-info">
                  <span className="rc-add-title">{t('checker.blocked')}</span>
                  <span className="rc-add-desc">{t('checker.notInLists', { host: report.Host })}</span>
                </div>
                <button className="btn btn-accent" onClick={handleAddToList} disabled={adding}>
                  {adding ? '...' : t('checker.addToList')}
                </button>
              </div>
            )}
            {report.InUserList && (
              <div className="rc-add-card" style={{ background: 'var(--success-bg)' }}>
                <span style={{ color: 'var(--success)', fontSize: 12, fontWeight: 600, display:'inline-flex', alignItems:'center', gap:4 }}>
                  <Check size={13} strokeWidth={3} /> {t('checker.inUserList', { host: report.Host })}
                </span>
              </div>
            )}
          </div>
        )}
      </div>
    </Modal>
  );
}
