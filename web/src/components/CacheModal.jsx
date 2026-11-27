import { useState, useEffect, useCallback } from 'react';
import Modal from './ui/Modal';
import { api, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { useToast } from '../context/ToastContext';
import { Check, AlertTriangle, Circle, Eraser } from 'lucide-react';
import Spinner from './ui/Spinner';

const STATUS_CFG = {
  ok:    { icon: Check,         color: 'var(--success)',  bg: 'var(--success-bg)' },
  warn:  { icon: AlertTriangle, color: 'var(--warning)',  bg: 'var(--warning-bg)' },
};

const CACHE_INFO_ROUTE = '/api/cache/info';

// CacheModal — очистка кешей: сеть (резолвер Windows), Discord, WebView2.
// Размер фиксированный, не зависит от контента.
export default function CacheModal({ open, onClose }) {
  const showToast = useToast();
  const { t } = useT();
  const [items, setItems] = useState(null);
  const [clearing, setClearing] = useState(null);
  const [lines, setLines] = useState([]);

  const load = useCallback(async (silent) => {
    const r = await api('GET', CACHE_INFO_ROUTE);
    if (r && !r.error && Array.isArray(r.items)) {
      apiCachePut(CACHE_INFO_ROUTE, r);
      setItems(r.items);
      if (!silent) setLines([]);
    }
  }, []);

  useEffect(() => {
    if (open) {
      setLines([]);
      // SWR: повторное открытие рисует кэш мгновенно, свежие размеры догоняют.
      const cached = apiCacheGet(CACHE_INFO_ROUTE);
      if (cached && Array.isArray(cached.items)) setItems(cached.items);
      load(false);
    }
  }, [open, load]);

  const handleClear = async (target) => {
    setClearing(target);
    const r = await api('POST', '/api/zapret/cache/clear', { target });
    setClearing(null);
    if (Array.isArray(r?.lines) && r.lines.length > 0) {
      setLines((prev) => [...r.lines, ...prev].slice(0, 10));
      if (showToast) showToast(t('zapret.cacheClearedOk'), 'success');
    } else {
      if (showToast) showToast(t('zapret.cacheNothing'), 'info');
    }
    // Обновляем только панели кеша, без перерисовки всей модалки.
    load(true);
  };

  const hasWarn = !!items && items.some((it) => it.status === 'warn');

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('zapret.cacheModalTitle')}
      className="sst-frame"
      icon={<Eraser size={18} strokeWidth={2} />}
      tone={!items ? 'neutral' : hasWarn ? 'warning' : 'success'}
      footer={<button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>}
    >
      <div className="cache-modal-body">
        {items === null ? (
          <div className="rp-loading"><Spinner size={16} /></div>
        ) : (
          items.map((it) => {
            const cfg = STATUS_CFG[it.status] || { icon: Circle, color: 'var(--text-tertiary)', bg: 'var(--bg-inset)' };
            const Icon = cfg.icon;
            const target = it.key === 'discord' ? 'discord' : it.key === 'webview' ? 'webview' : 'network';
            return (
              <div key={it.key} className={'diag2-cache-panel ' + (it.status || 'ok')}>
                <span className="diag2-icon" style={{ background: cfg.bg, color: cfg.color }}>
                  <Icon size={14} strokeWidth={2.8} />
                </span>
                <div className="diag2-item-body">
                  <span className="diag2-item-label">{it.label}</span>
                  {it.detail && <span className="diag2-item-detail">{it.detail}</span>}
                </div>
                <button
                  className="btn btn-xs diag2-cache-clear"
                  onClick={() => handleClear(target)}
                  disabled={!!clearing}
                >
                  {clearing === target ? <Spinner size={12} /> : t('zapret.cacheClearBtn')}
                </button>
              </div>
            );
          })
        )}
        {lines.length > 0 && (
          <div className="diag2-cleared-lines" role="log">
            {lines.map((l, i) => (
              <div key={i} className="diag2-cleared-line mono">{l}</div>
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}
