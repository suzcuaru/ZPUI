import { useState, useEffect } from 'react';
import { openExternal } from '../../api';
import { useT } from '../../i18n';
import { AlertTriangle, Check, X as XIcon } from 'lucide-react';

function sanitize(text) {
  if (!text) return text;
  text = String(text);
  text = text.replace(/([A-Za-z]:\\Users\\)[^\\]+/gi, '$1***');
  text = text.replace(/(%USERPROFILE%\\)[^\\]+/gi, '$1***');
  text = text.replace(/\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/g, '[IP]');
  return text;
}

function buildReportText(report, version) {
  const lines = [];
  lines.push('=== ZPUI Recovery Report ===');
  lines.push('Date: ' + new Date().toISOString());
  lines.push('ZPUI Version: ' + (version || '—'));
  lines.push('Strategy: ' + sanitize(report?.strategy || '—'));
  lines.push('Error code: ' + (report?.error_code || '—'));
  lines.push('Reason: ' + sanitize(report?.reason || '—'));
  lines.push('Final status: ' + sanitize(report?.final_status || '—'));
  lines.push('');
  lines.push('--- Steps ---');
  for (const s of report?.steps || []) {
    lines.push(sanitize(`[${s.key}] ${s.status}: ${s.message}`));
  }
  if (report?.install_log?.length) {
    lines.push('');
    lines.push('--- Install log ---');
    lines.push(sanitize(report.install_log.join('\n')));
  }
  if (report?.diagnostics) {
    lines.push('');
    lines.push('--- Diagnostics ---');
    for (const [k, v] of Object.entries(report.diagnostics)) {
      lines.push(sanitize(`${k}: ${v?.status || '?'} — ${v?.label || ''} (${v?.detail || ''})`));
    }
  }
  return lines.join('\n');
}

export default function RecoveryFailedModal({ report, version, onClose, onOpenReport, showToast }) {
  const { t } = useT();
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const onKey = (e) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  if (!report) return null;

  const diag = report.diagnostics || {};
  const diagEntries = Object.entries(diag);
  const problems = diagEntries.filter(([, v]) => v && v.status !== 'ok');

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(buildReportText(report, version));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      if (showToast) showToast(t('recovery.copyFailed'), 'error');
    }
  };

  const contactSupport = () => {
    const title = encodeURIComponent('Zapret recovery failed: ' + (report.error_code || ''));
    const body = encodeURIComponent('```\n' + buildReportText(report, version) + '\n```');
    openExternal(`https://github.com/suzcuaru/ZPUI/issues/new?title=${title}&body=${body}`);
  };

  const generateFullReport = () => {
    if (onOpenReport) onOpenReport();
  };

  return (
    <div className="rcv-fail-overlay" onClick={onClose}>
      <div className="rcv-fail-modal" onClick={(e) => e.stopPropagation()}>
        <div className="rcv-fail-head">
          <AlertTriangle size={20} className="rcv-fail-head-icon" />
          <div className="rcv-fail-head-text">
            <strong>{t('recovery.failedTitle')}</strong>
            <span>{t('recovery.failedDesc')}</span>
          </div>
          <button className="rcv-fail-close" onClick={onClose} aria-label={t('common.close')}><XIcon size={16} /></button>
        </div>

        <div className="rcv-fail-code-row">
          <span className="rcv-fail-code-label">{t('recovery.errorCode')}</span>
          <span className="rcv-fail-code">{report.error_code || '—'}</span>
        </div>

        <div className="rcv-fail-section">
          <div className="rcv-fail-section-title">{t('recovery.stepsLog')}</div>
          <ul className="rcv-fail-steps">
            {(report.steps || []).map((s, i) => (
              <li key={i} className={'is-' + s.status}>
                <span className="rcv-fail-step-mark">{s.status === 'done' ? <Check size={12} /> : s.status === 'failed' ? <XIcon size={12} /> : '•'}</span>
                <span className="rcv-fail-step-key">{s.key}</span>
                <span className="rcv-fail-step-msg">{s.message}</span>
              </li>
            ))}
          </ul>
        </div>

        {problems.length > 0 && (
          <div className="rcv-fail-section">
            <div className="rcv-fail-section-title">{t('recovery.diagnostics')} ({problems.length})</div>
            <ul className="rcv-fail-diag">
              {problems.map(([k, v]) => (
                <li key={k}><strong>{v?.label || k}</strong> — {v?.detail || v?.status}</li>
              ))}
            </ul>
          </div>
        )}

        {report.install_log?.length > 0 && (
          <div className="rcv-fail-section">
            <div className="rcv-fail-section-title">{t('recovery.installLog')}</div>
            <pre className="rcv-fail-pre">{report.install_log.join('\n')}</pre>
          </div>
        )}

        <div className="rcv-fail-actions">
          <button className="btn btn-sm" onClick={generateFullReport}>{t('recovery.generateReport')}</button>
          <button className="btn btn-sm" onClick={copy}>{copied ? t('common.copied') : t('recovery.copyReport')}</button>
          <button className="btn btn-accent btn-sm" onClick={contactSupport}>{t('recovery.contactSupport')}</button>
          <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
        </div>
      </div>
    </div>
  );
}
