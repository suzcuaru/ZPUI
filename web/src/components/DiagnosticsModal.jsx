import { useState, useEffect, useCallback } from 'react';
import Modal from './ui/Modal';
import { api, apiCacheGet, apiCachePut } from '../api';
import { errorText } from '../errors';
import { useT } from '../i18n';
import { useToast } from '../context/ToastContext';
import { Check, AlertTriangle, X, RotateCw, Circle, Stethoscope, Trash2 } from 'lucide-react';
import Spinner from './ui/Spinner';

const STATUS_CFG = {
  ok:    { icon: Check,          color: 'var(--success)',  bg: 'var(--success-bg)' },
  warn:  { icon: AlertTriangle,  color: 'var(--warning)',  bg: 'var(--warning-bg)' },
  error: { icon: X,              color: 'var(--danger)',   bg: 'var(--danger-bg)' },
};

const DIAG_ROUTE = '/api/zapret/diagnostics';

// Кеш-проблемы чистятся прямо из списка проблем: ключ диагностики → target
// /api/zapret/cache/clear. Прочие проблемы — без кнопки (нужны действия
// вне приложения).
const CACHE_CLEAR_TARGET = {
  cache_dns: 'dns',
  cache_network: 'network',
  cache_discord: 'discord',
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

export default function DiagnosticsModal({ open, onClose }) {
  const showToast = useToast();
  const { t } = useT();
  const [results, setResults] = useState(null);
  const [loading, setLoading] = useState(false);
  const [clearing, setClearing] = useState(null);

  const runDiag = useCallback(async () => {
    setLoading(true);
    const data = await api('GET', DIAG_ROUTE);
    if (data && !data.error) apiCachePut(DIAG_ROUTE, data);
    setResults(data);
    setLoading(false);
  }, []);

  // Тихий прогон без скелета: обновить только что очищенную кеш-панель,
  // не трогая остальные проверки.
  const refreshCachePanel = useCallback(async (key) => {
    const data = await api('GET', DIAG_ROUTE);
    if (data && !data.error) {
      apiCachePut(DIAG_ROUTE, data);
      if (data[key]) setResults(prev => (prev ? { ...prev, [key]: data[key] } : prev));
    }
  }, []);

  const clearCache = useCallback(async (key) => {
    const target = CACHE_CLEAR_TARGET[key];
    if (!target || clearing) return;
    setClearing(key);
    const res = await api('POST', '/api/zapret/cache/clear', { target });
    setClearing(null);
    if (res?.error) {
      showToast(errorText(t, res.error), 'error');
      return;
    }
    showToast(t('zapret.cacheClearedOk'), 'success');
    refreshCachePanel(key);
  }, [clearing, showToast, t, refreshCachePanel]);

  useEffect(() => {
    if (open) {
      // SWR: открытие рисует прошлый прогон мгновенно, свежий стартует следом.
      const cached = apiCacheGet(DIAG_ROUTE);
      if (cached && !cached.error) setResults(cached);
      else setResults(null);
      runDiag();
    }
  }, [open, runDiag]);


  const rank = { error: 0, warn: 1, ok: 2 };
  const checks = results
    ? Object.entries(results)

        .sort((a, b) => (rank[a[1]?.status] ?? 3) - (rank[b[1]?.status] ?? 3))
    : [];
  const okCount = checks.filter(([, v]) => v?.status === 'ok').length;
  const warnCount = checks.filter(([, v]) => v?.status === 'warn').length;
  const errorCount = checks.filter(([, v]) => v?.status === 'error').length;
  const allOk = !loading && errorCount === 0 && warnCount === 0 && okCount > 0;
  const hasErrors = errorCount > 0;
  const problems = checks.filter(([, v]) => v?.status !== 'ok');
  const okChecks = checks.filter(([, v]) => v?.status === 'ok');

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('zapret.diagnostics')}
      wide
      icon={loading
        ? <Spinner size={18} />
        : <Stethoscope size={18} strokeWidth={2} />}
      tone={loading ? 'neutral' : allOk ? 'success' : hasErrors ? 'danger' : 'warning'}
      footer={
        <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
      }
    >
      {/* Summary banner */}
      <div className={'diag2-banner ' + (allOk ? 'ok' : hasErrors ? 'error' : 'warn')}>
        {loading ? (
          <><Spinner size={14} /><span>{t('zapret.diagnosticsRunning')}</span></>
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

      {loading ? (
        <div className="diag2-list">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="diag2-skeleton" />
          ))}
        </div>
      ) : (
        <>
          {problems.length > 0 && (
            <div className="diag2-group-title">{t('zapret.diagProblems')}</div>
          )}
          <div className="diag2-list">
            {problems.map(([key, val]) => {
              const clearTarget = CACHE_CLEAR_TARGET[key];
              return (
                <div
                  key={key}
                  className={'diag2-item ' + (val?.status || 'ok')}
                  data-tooltip={val ? `${val.label} — ${t('zapret.diagHelp.' + key, { defaultValue: t('zapret.diagHelpFallback') })}` : undefined}
                  data-tooltip-pos="top"
                >
                  <StatusIcon status={val?.status} />
                  <div className="diag2-item-body">
                    <span className="diag2-item-label">{val?.label || key}</span>
                    {val?.detail && <span className="diag2-item-detail">{val.detail}</span>}
                  </div>
                  {clearTarget && (
                    <button
                      className="btn btn-xs btn-ghost diag2-clear-btn"
                      disabled={clearing === key}
                      onClick={(e) => { e.stopPropagation(); clearCache(key); }}
                      data-tooltip={t('zapret.cacheClearBtn')}
                      data-tooltip-pos="top"
                    >
                      {clearing === key ? <Spinner size={12} /> : <Trash2 size={13} strokeWidth={2.2} />}
                      {t('zapret.cacheClearBtn')}
                    </button>
                  )}
                </div>
              );
            })}
          </div>
          {okChecks.length > 0 && (
            <>
              <div className="diag2-group-title">{t('zapret.diagAllGood')}</div>
              <div className="diag2-list">
                {okChecks.map(([key, val]) => (
                  <div
                    key={key}
                    className="diag2-item ok"
                    data-tooltip={val ? `${val.label} — ${t('zapret.diagHelp.' + key, { defaultValue: t('zapret.diagHelpFallback') })}` : undefined}
                    data-tooltip-pos="top"
                  >
                    <StatusIcon status={val?.status} />
                    <div className="diag2-item-body">
                      <span className="diag2-item-label">{val?.label || key}</span>
                      {val?.detail && <span className="diag2-item-detail">{val.detail}</span>}
                    </div>
                  </div>
                ))}
              </div>
            </>
          )}
        </>
      )}

    </Modal>
  );
}
