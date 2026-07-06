import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { useT } from '../i18n';

export default function ZapretPage({ showToast }) {
  const { t } = useT();
  const [state, setState] = useState(null);
  const [loading, setLoading] = useState(false);
  const [strategies, setStrategies] = useState([]);
  const [selStrategy, setSelStrategy] = useState('');

  const call = useCallback(async (command) => {
    setLoading(true);
    try {
      const res = await api('POST', '/api/modules/call', { id: 'zapret', command });
      if (res?.error) {
        showToast(res.error, 'error');
      } else if (res?.output) {
        showToast(res.output, 'success');
      }
      await refresh();
    } catch (e) {
      showToast(String(e), 'error');
    }
    setLoading(false);
  }, [showToast]);

  const refresh = useCallback(async () => {
    const mods = await api('GET', '/api/modules');
    if (Array.isArray(mods)) {
      const m = mods.find(x => x.id === 'zapret');
      if (m) setState(m);
    }
    if (!state?.zapret_state || state.zapret_state !== 'not_installed') {
      const res = await api('POST', '/api/modules/call', { id: 'zapret', command: 'strategies' });
      if (res?.output) {
        const lines = res.output.split('\n').filter(Boolean);
        setStrategies(lines);
        setSelStrategy(lines[0] || '');
      }
    }
  }, [state?.zapret_state]);

  useEffect(() => { refresh(); }, []);

  const isInstalled = state?.zapret_state && state.zapret_state !== 'not_installed';
  const isRunning = state?.zapret_state === 'running';
  const zapretVer = state?.zapret_version || '—';

  if (!state) {
    return <div className="page-status">{t('loading')}</div>;
  }

  return (
    <div className="settings-page">
      <div className="section-card">
        <div className="section-title">Zapret DPI</div>
        <div className="section-desc">Обход DPI блокировок Discord, YouTube и других сервисов</div>
      </div>

      <div className="section-card">
        <div className="section-title">{t('startup.selfCheck')}</div>
        <div className="zapret-info">
          <span className="zapret-label">Версия:</span>
          <span className="zapret-value">{zapretVer}</span>
        </div>
        <div className="zapret-info">
          <span className="zapret-label">Статус:</span>
          <span className={`zapret-badge ${isRunning ? 'badge-ok' : isInstalled ? 'badge-warn' : 'badge-off'}`}>
            {isRunning ? 'Запущен' : isInstalled ? 'Остановлен' : 'Не установлен'}
          </span>
        </div>
      </div>

      <div className="section-card">
        <div className="section-title">Действия</div>
        <div className="zapret-actions">
          {!isInstalled ? (
            <button className="btn btn-primary" onClick={() => call('install')} disabled={loading}>
              {loading ? 'Установка...' : 'Установить'}
            </button>
          ) : (
            <>
              {isRunning ? (
                <button className="btn btn-warn" onClick={() => call('stop')} disabled={loading}>
                  Остановить
                </button>
              ) : (
                <button className="btn btn-primary" onClick={() => call('start')} disabled={loading}>
                  Запустить
                </button>
              )}
              <button className="btn btn-secondary" onClick={() => call('restart')} disabled={loading}>
                Перезапустить
              </button>
              <button className="btn btn-secondary" onClick={() => call('update')} disabled={loading}>
                Обновить
              </button>
              <button className="btn btn-danger" onClick={() => call('uninstall')} disabled={loading}>
                Удалить
              </button>
            </>
          )}
        </div>
      </div>

      {isInstalled && strategies.length > 0 && (
        <div className="section-card">
          <div className="section-title">Стратегия</div>
          <select className="zapret-select" value={selStrategy} onChange={e => setSelStrategy(e.target.value)}>
            {strategies.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
        </div>
      )}
    </div>
  );
}
