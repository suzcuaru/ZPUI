import { useState, useEffect, useCallback } from 'react';
import Modal from './ui/Modal';
import { api, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { errorText } from '../errors';
import { useToast } from '../context/ToastContext';
import { HeartPulse, AlertTriangle, Check, ArrowRight } from 'lucide-react';
import Spinner from './ui/Spinner';

const HEALTH_ROUTE = '/api/health';

const OVERALL_TONE = {
  healthy: 'success',
  degraded: 'warning',
  critical: 'danger',
};

// Известные типы предупреждений: бэкенд (/api/health) присылает готовые
// строки, сопоставляем их по подстрокам с типом проблемы. Новый тип —
// запись здесь + ключи health.problem.<type>.* в локалях. Неизвестные
// строки показываются как есть, без кнопок — ничего не выдумываем.
const PROBLEM_TYPES = [
  { type: 'zapret_missing', match: (w) => /запрет/i.test(w) && /не установлен/i.test(w) },
];

// Действия по типам. kind 'navigate' — переход на вкладку (модалка
// закрывается), kind 'call' — POST-вызов с перепроверкой предупреждений.
const PROBLEM_ACTIONS = {
  zapret_missing: { kind: 'navigate', page: 'zapret', label: 'health.action.openZapret' },
};

// HealthCheckModal — «Центр здоровья»: проблемы с разбором (что случилось,
// чем грозит, что делать), состояние компонентов и спутников.
export default function HealthCheckModal({ onClose, onNavigate, onOpenDiagnostics }) {
  const showToast = useToast();
  const { t } = useT();
  const [health, setHealth] = useState(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  // Ключ проблемы, по которой сейчас выполняется действие (спиннер в кнопке).
  const [acting, setActing] = useState(null);
  // Ключи проблем, по которым действие уже успешно выполнилось.
  const [doneKeys, setDoneKeys] = useState({});

  const load = useCallback(async (silent) => {
    if (!silent) {
      setLoading(true);
      setFailed(false);
    }
    const d = await api('GET', HEALTH_ROUTE);
    if (d) {
      if (!d.error) apiCachePut(HEALTH_ROUTE, d);
      setHealth(d);
    } else if (!silent) {
      setFailed(true);
    }
    if (!silent) setLoading(false);
  }, []);

  useEffect(() => {
    // SWR: при открытии с кэшем показываем его сразу и тихо обновляем,
    // без кэша — обычная загрузка со скелетом.
    const cached = apiCacheGet(HEALTH_ROUTE);
    if (cached && !cached.error) {
      setHealth(cached);
      load(true);
    } else {
      load();
    }
  }, [load]);

  const runAction = useCallback(async (key, action) => {
    if (acting) return;
    if (action.kind === 'navigate') {
      onNavigate?.(action.page);
      onClose();
      return;
    }
    if (action.kind === 'diagnostics') {
      onOpenDiagnostics?.();
      onClose();
      return;
    }
    if (action.kind === 'call') {
      setActing(key);
      const r = await api('POST', action.route, action.body || {});
      setActing(null);
      if (r && r.error) {
        showToast(errorText(t, r.error), 'error');
        return;
      }
      setDoneKeys((m) => ({ ...m, [key]: true }));
      showToast(t('health.actionDone'), 'success');
      load();
    }
  }, [acting, onNavigate, onOpenDiagnostics, onClose, showToast, t, load]);

  const overallColor = {
    healthy: 'var(--success)',
    degraded: 'var(--warning)',
    critical: 'var(--danger)',
  };

  const overallLabel = {
    healthy: t('status.healthy'),
    degraded: t('status.degraded'),
    critical: t('status.critical'),
  };

  const compStatus = (status) => {
    if (status === 'running') return t('status.ok');
    if (status === 'stopped') return t('status.stopped');
    if (status === 'missing') return t('status.missing');
    return status;
  };

  const tone = health ? (OVERALL_TONE[health.overall] || 'neutral') : 'neutral';
  const critical = health?.overall === 'critical';

  const problems = (health?.warnings || []).map((w, i) => {
    const known = PROBLEM_TYPES.find((pt) => pt.match(w));
    return known
      ? {
          key: known.type + ':' + i,
          title: t('health.problem.' + known.type + '.title'),
          desc: t('health.problem.' + known.type + '.desc'),
          action: PROBLEM_ACTIONS[known.type] || null,
        }
      : { key: 'raw:' + i, title: w, desc: t('health.problem.unknown.desc'), action: null };
  });

  return (
    <Modal
      title={t('health.centerTitle')}
      onClose={onClose}
      open={true}
      wide
      icon={<HeartPulse size={18} strokeWidth={2} />}
      tone={tone}
      footer={
        <button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>
      }
    >
      {loading ? (
        <div className="modal-loading">
          <Spinner size={20} />
          {t('common.loading')}
        </div>
      ) : failed && !health ? (
        <div className="modal-loading" role="alert">
          <span className="modal-error-text">{t('status.getDataFailed')}</span>
          <button className="btn btn-sm" onClick={load}>{t('common.retry')}</button>
        </div>
      ) : health ? (
        <div className="health-panel">
          <div className="health-overall" style={{ borderColor: overallColor[health.overall] }}>
            <span className="health-overall-dot" style={{ background: overallColor[health.overall] }} />
            <span className="health-overall-text">{overallLabel[health.overall]}</span>
            <span className="health-time">{health.timestamp}</span>
          </div>

          {problems.length > 0 && (
            <div className="health-section">
              <div className="health-section-title">{t('health.problemsTitle')}</div>
              <div className="health-problems">
                {problems.map((p) => {
                  const isActing = acting === p.key;
                  const isDone = !!doneKeys[p.key];
                  return (
                    <div key={p.key} className={'health-problem' + (critical ? ' critical' : '')}>
                      <span className="health-problem-icon">
                        <AlertTriangle size={13} strokeWidth={2.2} />
                      </span>
                      <div className="health-problem-body">
                        <span className="health-problem-title">{p.title}</span>
                        <span className="health-problem-desc">{p.desc}</span>
                        {p.action && !isDone && (
                          <button
                            className="btn btn-sm health-problem-btn"
                            onClick={() => runAction(p.key, p.action)}
                            disabled={!!acting}
                          >
                            {isActing ? <Spinner size={12} /> : <ArrowRight size={12} strokeWidth={2.2} />}
                            {t(p.action.label)}
                          </button>
                        )}
                        {isDone && (
                          <span className="health-problem-ok">
                            <Check size={12} strokeWidth={2.6} />
                            {t('health.actionDone')}
                          </span>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          <div className="health-section">
            <div className="health-section-title">{t('health.components')}</div>
            <div className="health-comp-grid">
              {health.components?.map(c => (
                <div key={c.name} className={'health-comp-card ' + c.status}>
                  <span className="health-comp-dot" />
                  <div className="health-comp-info">
                    <span className="health-comp-name">{c.name}</span>
                    {c.version && <span className="health-comp-ver mono">v{c.version}</span>}
                    {c.detail && <span className="health-comp-detail">{c.detail}</span>}
                  </div>
                  <span className="health-comp-status">{compStatus(c.status)}</span>
                </div>
              ))}
            </div>
          </div>

          <div className="health-section">
            <div className="health-section-title">{t('health.satellites')}</div>
            <div className="health-sat-row">
              {Object.entries(health.modules || {}).map(([name, status]) => (
                <div key={name} className={'health-sat-pill ' + status}>
                  <span className="health-sat-dot" />
                  <span className="health-sat-name">{name}.exe</span>
                </div>
              ))}
            </div>
          </div>

        </div>
      ) : null}
    </Modal>
  );
}
