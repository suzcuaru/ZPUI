import { useState, useEffect, useCallback } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { useT } from '../i18n';

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

  return (
    <Modal open={open} onClose={onClose} title={t('zapret.diagnostics')} wide>
      <div className="diag-summary">
        {loading ? (
          <span className="diag-running">
            <span className="mini-spin" /> {t('zapret.diagnosticsRunning')}
          </span>
        ) : (
          <>
            {okCount > 0 && <span className="diag-pill ok">✓ {okCount}</span>}
            {warnCount > 0 && <span className="diag-pill warn">⚠ {warnCount}</span>}
            {errorCount > 0 && <span className="diag-pill error">✕ {errorCount}</span>}
            <span className="diag-spacer" />
            <button className="btn btn-xs btn-ghost" onClick={runDiag}>↻</button>
          </>
        )}
      </div>

      <div className="diag-list">
        {checks.map(([key, val]) => (
          <div key={key} className={'diag-item ' + (val?.status || 'ok')}>
            <span className="diag-item-icon">
              {val?.status === 'ok' ? '✓' : val?.status === 'warn' ? '⚠' : val?.status === 'error' ? '✕' : '○'}
            </span>
            <span className="diag-item-label">{val?.label || key}</span>
            <span className="diag-item-detail">{val?.detail || ''}</span>
          </div>
        ))}
      </div>

      <div className="diag-cache-section">
        <div className="diag-cache-title">{t('zapret.cacheClearing')}</div>
        <div className="diag-cache-buttons">
          <button
            className="btn btn-sm"
            onClick={() => handleClear('discord')}
            disabled={!!clearing}
          >
            {clearing === 'discord' ? <span className="mini-spin" /> : t('zapret.clearDiscord')}
          </button>
          <button
            className="btn btn-sm"
            onClick={() => handleClear('network')}
            disabled={!!clearing}
          >
            {clearing === 'network' ? <span className="mini-spin" /> : t('zapret.clearNetwork')}
          </button>
          <button
            className="btn btn-sm btn-accent"
            onClick={() => handleClear('all')}
            disabled={!!clearing}
          >
            {clearing === 'all' ? <span className="mini-spin" /> : t('zapret.clearAll')}
          </button>
        </div>
        {clearedMsg && (
          <div className="diag-cleared">{t('zapret.cacheCleared', { items: clearedMsg })}</div>
        )}
      </div>
    </Modal>
  );
}
