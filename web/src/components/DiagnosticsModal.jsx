import { useState, useEffect, useCallback } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { useT } from '../i18n';
import { Check, AlertTriangle, X, RotateCw, Circle, Zap, Wifi, Database, Eraser } from 'lucide-react';

const STATUS_CFG = {
  ok:    { icon: Check,          color: 'var(--success)',  bg: 'var(--success-bg)' },
  warn:  { icon: AlertTriangle,  color: 'var(--warning)',  bg: 'var(--warning-bg)' },
  error: { icon: X,              color: 'var(--danger)',   bg: 'var(--danger-bg)' },
};

function StatusIcon({ status, size = 14 }) {
  const cfg = STATUS_CFG[status] || { icon: Circle, color: 'var(--text-tertiary)', bg: 'var(--bg-inset)' };
  const Icon = cfg.icon;
  return (
    <span className="diag2-icon" style={{ background: cfg.bg, color: cfg.color }}>
      <Icon size={size} strokeWidth={2.8} />
    </span>
  );
}

export default function DiagnosticsModal({ open, onClose, showToast }) {
  const { t } = useT();
  const [results, setResults] = useState(null);
  const [loading, setLoading] = useState(false);
  const [clearing, setClearing] = useState(null);
  const [clearedMsg, setClearedMsg] = useState(null);

  const runDiag = useCallback(async () => {
    setLoading(true);
    const data = await api('GET', '/api/zapret/diagnostics');
    setResults(data);
    setLoading(false);
  }, []);

  useEffect(() => {
    if (open) {
      setResults(null);
      setClearedMsg(null);
      runDiag();
    }
  }, [open, runDiag]);

  const handleClear = async (target) => {
    setClearing(target);
    setClearedMsg(null);
    const r = await api('POST', '/api/zapret/cache/clear', { target });
    if (r?.status === 'ok' && r.cleared?.length > 0) {
      setClearedMsg(r.cleared.join(', '));
      if (showToast) showToast(t('zapret.cacheCleared', { items: r.cleared.join(', ') }), 'success');
    } else {
      if (showToast) showToast(t('zapret.cacheNothing'), 'info');
    }
    setClearing(null);
  };

  const checks = results ? Object.entries(results) : [];
  const okCount = checks.filter(([, v]) => v?.status === 'ok').length;
  const warnCount = checks.filter(([, v]) => v?.status === 'warn').length;
  const errorCount = checks.filter(([, v]) => v?.status === 'error').length;
  const allOk = !loading && errorCount === 0 && warnCount === 0 && okCount > 0;
  const hasErrors = errorCount > 0;

  return (
    <Modal open={open} onClose={onClose} title={t('zapret.diagnostics')} wide>
      {/* Summary banner */}
      <div className={'diag2-banner ' + (allOk ? 'ok' : hasErrors ? 'error' : 'warn')}>
        {loading ? (
          <><span className="mini-spin" /><span>{t('zapret.diagnosticsRunning')}</span></>
        ) : (
          <>
            <div className="diag2-banner-counts">
              {okCount > 0 && <span className="diag2-count ok"><Check size={12} strokeWidth={3} /> {okCount}</span>}
              {warnCount > 0 && <span className="diag2-count warn"><AlertTriangle size={12} strokeWidth={2.5} /> {warnCount}</span>}
              {errorCount > 0 && <span className="diag2-count error"><X size={12} strokeWidth={3} /> {errorCount}</span>}
            </div>
            <span className="diag2-banner-text">
              {allOk ? t('zapret.diagAllOk') : hasErrors ? t('zapret.diagHasErrors') : t('zapret.diagHasWarnings')}
            </span>
            <button className="btn btn-xs btn-ghost diag2-rerun" onClick={runDiag}>
              <RotateCw size={13} strokeWidth={2.5} />
            </button>
          </>
        )}
      </div>

      {/* Check items */}
      <div className="diag2-list">
        {checks.map(([key, val]) => (
          <div key={key} className={'diag2-item ' + (val?.status || 'ok')}>
            <StatusIcon status={val?.status} />
            <div className="diag2-item-body">
              <span className="diag2-item-label">{val?.label || key}</span>
              {val?.detail && <span className="diag2-item-detail">{val.detail}</span>}
            </div>
          </div>
        ))}
        {loading && [...Array(4)].map((_, i) => (
          <div key={i} className="diag2-skeleton" />
        ))}
      </div>

      {/* Cache clearing */}
      <div className="diag2-cache">
        <div className="diag2-cache-title">
          <Eraser size={13} strokeWidth={2.2} />
          {t('zapret.cacheClearing')}
        </div>
        <div className="diag2-cache-btns">
          <button className="btn btn-sm" onClick={() => handleClear('discord')} disabled={!!clearing}>
            {clearing === 'discord' ? <span className="mini-spin" /> : t('zapret.clearDiscord')}
          </button>
          <button className="btn btn-sm" onClick={() => handleClear('network')} disabled={!!clearing}>
            {clearing === 'network' ? <span className="mini-spin" /> : t('zapret.clearNetwork')}
          </button>
          <button className="btn btn-sm btn-accent" onClick={() => handleClear('all')} disabled={!!clearing}>
            {clearing === 'all' ? <span className="mini-spin" /> : t('zapret.clearAll')}
          </button>
        </div>
        {clearedMsg && (
          <div className="diag2-cleared">{t('zapret.cacheCleared', { items: clearedMsg })}</div>
        )}
      </div>
    </Modal>
  );
}
