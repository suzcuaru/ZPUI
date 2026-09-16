import { useState } from 'react';
import Modal from './ui/Modal';
import Switch from './ui/Switch';
import { api } from '../api';
import { useT } from '../i18n';
import { Check, Search, X, Globe, ShieldCheck, Layers } from 'lucide-react';

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
      <span className="rc-layer-ico">{ok ? <Check size={10} strokeWidth={3} /> : <X size={10} strokeWidth={3} />}</span>
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
  const [deep, setDeep] = useState(false);
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState(null);
  const [deepReport, setDeepReport] = useState(null);
  const [selected, setSelected] = useState(new Set());
  const [adding, setAdding] = useState(false);

  const handleCheck = async () => {
    if (!url.trim()) return;
    setLoading(true);
    setReport(null);
    setDeepReport(null);
    setSelected(new Set());

    const endpoint = deep ? '/api/resource-check-deep' : '/api/resource-check';
    const d = await api('POST', endpoint, { url: url.trim() });
    setLoading(false);

    if (!d) {
      showToast(t('checker.backendError'), 'error');
      return;
    }
    if (d?.error) {
      showToast(d.error, 'error');
      return;
    }
    if (!d?.report) {
      showToast(t('toast.requestFailed'), 'error');
      return;
    }

    if (deep) {
      setDeepReport(d.report);
      const blocked = (d.report.sub_resources || []).filter(r => r.blocked).map(r => r.host);
      setSelected(new Set(blocked));
    } else {
      setReport(d.report);
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

  const toggleSelect = (host) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(host)) next.delete(host);
      else next.add(host);
      return next;
    });
  };

  const handleAddSelected = async () => {
    const hosts = [...selected];
    if (!hosts.length) return;
    setAdding(true);
    const d = await api('POST', '/api/resource-add-batch', { hosts });
    setAdding(false);
    if (d?.status === 'ok') {
      const added = d.added || 0;
      if (added > 0) {
        showToast(t('checker.addedBatch', { count: added }), 'success');
      } else {
        showToast(t('checker.alreadyInList'), 'info');
      }
      setSelected(new Set());
      setDeepReport(prev => prev ? { ...prev, _added: true } : prev);
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

  const mainMeta = deepReport ? (VERDICT_META[deepReport.main_verdict] || VERDICT_META.UNKNOWN) : null;

  return (
    <Modal open onClose={onClose} title={t('checker.title')} wide icon={<Search size={18} strokeWidth={2} />} tone="accent">
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

        <div className="rc-options">
          <Switch
            checked={deep}
            onChange={() => {
              setDeep(v => !v);
              setReport(null);
              setDeepReport(null);
              setSelected(new Set());
            }}
            label={t('checker.deepToggle')}
            disabled={loading}
          />
          {deep && <span className="rc-options-hint">{t('checker.deepHint')}</span>}
        </div>

        {!loading && !report && !deepReport && (
          <div className="rc-empty">
            <Search size={22} strokeWidth={1.6} />
            <span className="rc-empty-title">{t('checker.emptyTitle')}</span>
            <span className="rc-empty-hint">{t('checker.emptyHint')}</span>
          </div>
        )}

        {loading && (
          <div className="rc-loading">
            <div className="loading-spinner-lg" />
            <span className="loading-sub">{deep ? t('checker.deepProgress') : t('checker.checkingLayers')}</span>
          </div>
        )}

        {!deep && report && !loading && (
          <div className="rc-results">
            <div className="rc-section">
              <div className="rc-section-title"><Globe size={12} strokeWidth={2.2} />{t('checker.provider')}</div>
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
              <div className="rc-section-title"><ShieldCheck size={12} strokeWidth={2.2} />{t('checker.checkResult')}</div>
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
              <div className="rc-section-title"><Layers size={12} strokeWidth={2.2} />{t('checker.layerDetails')}</div>
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

        {deep && deepReport && !loading && (
          <div className="rc-results">
            <div className="rc-section">
              <div className="rc-section-title"><ShieldCheck size={12} strokeWidth={2.2} />{t('checker.checkResult')}</div>
              <div className="rc-deep-main">
                <span className="rc-deep-host mono">{deepReport.host}</span>
                {mainMeta && (
                  <span className="rc-verdict-badge" style={{ background: mainMeta.color + '22', color: mainMeta.color }}>
                    {mainMeta.icon} {t(mainMeta.key)}
                  </span>
                )}
              </div>
            </div>

            {!deepReport.fetched && (
              <div className="rc-section rc-deep-error-section">
                <span className="rc-deep-error">{deepReport.fetch_error || t('checker.deepFetchFail')}</span>
              </div>
            )}

            {deepReport.fetched && (
              <>
                <div className="rc-deep-summary">
                  <span className="rc-deep-sum-item">{t('checker.deepFound', { total: deepReport.total })}</span>
                  {deepReport.blocked_count > 0 && (
                    <span className="rc-deep-sum-item danger">{t('checker.deepBlocked', { count: deepReport.blocked_count })}</span>
                  )}
                  <span className="rc-deep-sum-item success">{t('checker.deepOk', { count: deepReport.ok_count })}</span>
                </div>

                {deepReport.blocked_count === 0 && (
                  <div className="rc-add-card" style={{ background: 'var(--success-bg)' }}>
                    <span style={{ color: 'var(--success)', fontSize: 12, fontWeight: 600, display:'inline-flex', alignItems:'center', gap:4 }}>
                      <Check size={13} strokeWidth={3} /> {t('checker.deepAllOk')}
                    </span>
                  </div>
                )}

                {deepReport.blocked_count > 0 && (
                  <div className="rc-deep-actions">
                    <span className="rc-deep-sel">{t('checker.deepSelected', { count: selected.size })}</span>
                    <button className="btn btn-accent btn-sm" onClick={handleAddSelected} disabled={adding || selected.size === 0}>
                      {adding ? '...' : t('checker.addSelected')}
                    </button>
                  </div>
                )}

                {deepReport.sub_resources && deepReport.sub_resources.length > 0 && (
                  <div className="rc-deep-list">
                    {deepReport.sub_resources.map(r => {
                      const m = VERDICT_META[r.verdict] || VERDICT_META.UNKNOWN;
                      return (
                        <label
                          key={r.host}
                          className={'rc-deep-item' + (r.blocked ? ' blocked' : r.ok ? ' ok' : ' other')}
                        >
                          <input
                            type="checkbox"
                            checked={selected.has(r.host)}
                            onChange={() => toggleSelect(r.host)}
                            disabled={!r.blocked}
                          />
                          <span className="rc-deep-host mono">{r.host}</span>
                          <span className="rc-deep-badge" style={{ background: m.color + '22', color: m.color }} data-tooltip={r.reason}>
                            {m.icon}
                          </span>
                          {r.latency_ms > 0 && <span className="rc-deep-latency">{Math.round(r.latency_ms)}{t('common.ms')}</span>}
                        </label>
                      );
                    })}
                  </div>
                )}

                {deepReport._added && (
                  <div className="rc-add-card" style={{ background: 'var(--success-bg)' }}>
                    <span style={{ color: 'var(--success)', fontSize: 12, fontWeight: 600, display:'inline-flex', alignItems:'center', gap:4 }}>
                      <Check size={13} strokeWidth={3} /> {t('checker.deepAddedDone')}
                    </span>
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </Modal>
  );
}
