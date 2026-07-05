import { useState, useEffect } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { useT } from '../i18n';

export default function HealthCheckModal({ onClose }) {
  const { t } = useT();
  const [health, setHealth] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    (async () => {
      const d = await api('GET', '/api/health');
      if (d) setHealth(d);
      setLoading(false);
    })();
  }, []);

  const overallColor = {
    healthy: 'var(--success)',
    degraded: 'var(--warning)',
    critical: 'var(--danger)',
  };

  const overallLabel = {
    healthy: 'Все компоненты в норме',
    degraded: 'Есть предупреждения',
    critical: 'Критические проблемы',
  };

  return (
    <Modal title="Проверка системы" onClose={onClose} open={true} wide>
      {loading ? (
        <div style={{ padding: '20px', textAlign: 'center' }}>
          <span className="mini-spin" style={{ display: 'inline-block', width: '20px', height: '20px', borderWidth: '3px' }} />
        </div>
      ) : health ? (
        <div className="health-panel">
          <div className="health-overall" style={{ borderColor: overallColor[health.overall] }}>
            <span className="health-overall-dot" style={{ background: overallColor[health.overall] }} />
            <span className="health-overall-text">{overallLabel[health.overall]}</span>
            <span className="health-time">{health.timestamp}</span>
          </div>

          {health.warnings?.length > 0 && (
            <div className="health-warnings">
              {health.warnings.map((w, i) => (
                <div key={i} className="health-warn-row">{w}</div>
              ))}
            </div>
          )}

          <div className="health-section">
            <div className="health-section-title">Компоненты</div>
            <div className="health-comp-grid">
              {health.components?.map(c => (
                <div key={c.name} className={'health-comp-card ' + c.status}>
                  <span className="health-comp-dot" />
                  <div className="health-comp-info">
                    <span className="health-comp-name">{c.name}</span>
                    {c.version && <span className="health-comp-ver mono">v{c.version}</span>}
                    {c.detail && <span className="health-comp-detail">{c.detail}</span>}
                  </div>
                  <span className="health-comp-status">
                    {c.status === 'running' ? 'Работает' :
                     c.status === 'stopped' ? 'Остановлен' :
                     c.status === 'missing' ? 'Отсутствует' : c.status}
                  </span>
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

          {health.mods?.length > 0 && (
            <div className="health-section">
              <div className="health-section-title">{t('health.mods')}</div>
              <div className="health-comp-grid">
                {health.mods.map(m => (
                  <div key={m.id} className={'health-comp-card ' + m.status}>
                    <span className="health-comp-dot" />
                    <div className="health-comp-info">
                      <span className="health-comp-name">{m.name}</span>
                      <span className="health-comp-ver mono">v{m.version}</span>
                      {m.message && <span className="health-comp-detail">{m.message}</span>}
                    </div>
                    <span className="health-comp-status">
                      {m.status === 'healthy' ? 'OK' :
                       m.status === 'degraded' ? 'Проблема' :
                       m.status === 'broken' ? 'Сломан' : m.status}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      ) : (
        <div style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }}>
          Не удалось получить данные
        </div>
      )}
    </Modal>
  );
}
