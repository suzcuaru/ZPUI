import { useEffect, useRef, useState } from 'react';
import { api } from '../../api';
import { useT } from '../../i18n';

// OfflineScreen — экран «фронтенд не получает данные от бэкенда».
// reason: 'api' — биндинги есть, но /api/status не отвечает (процесс-бэкенд
// в плохом состоянии); 'bridge' — биндинги Wails недоступны вообще: фронт
// отсоединён от бэка (обычно запущена старая копия приложения — видно по
// версии в заголовке окна) либо фронт открыт вне среды Wails.
export default function OfflineScreen({ onRetry, reason }) {
  const { t } = useT();
  const [restarting, setRestarting] = useState(false);
  const retryRef = useRef(onRetry);
  retryRef.current = onRetry;
  // Автоперезапуск должен сработать один раз, даже если таймер гонки и
  // рендеры пересоздадут эффект.
  const restartFiredRef = useRef(false);

  const restartApp = () => {
    if (restartFiredRef.current) return;
    restartFiredRef.current = true;
    setRestarting(true);
    // Бэкенд в плохом состоянии может не ответить на биндинг — не ждём:
    // через 3с перезагружаем страницу, новый процесс подхватит фронт.
    try { api('POST', '/api/app/restart'); } catch {}
    setTimeout(() => { window.location.reload(); }, 3000);
  };

  // Автопроба связи: после выхода из сна сеть и бэкенд поднимаются не сразу,
  // а прошлые перезагрузки страницы к тому моменту уже исчерпаны — экран
  // сам перепроверяет бэкенд с растущим интервалом (5с → 20с), без тостов.
  useEffect(() => {
    let stopped = false;
    let timer;
    let delay = 5000;
    const tick = async () => {
      if (stopped) return;
      try { await retryRef.current?.(true); } catch {}
      if (stopped) return;
      timer = setTimeout(tick, delay);
      delay = Math.min(delay + 2500, 20000);
    };
    timer = setTimeout(tick, 5000);
    return () => { stopped = true; clearTimeout(timer); };
  }, []);

  // Бэкенд недоступен дольше 15 секунд — приложение перезапускается само.
  // Экран размонтируется при восстановлении, так что сработавший таймер
  // означает «всё ещё офлайн». Для bridge (фронт отсоединён, перезапуск
  // не поможет) автотаймер не заводим — иначе получится вечный флап.
  useEffect(() => {
    if (reason === 'bridge') return;
    const timer = setTimeout(() => restartApp(), 15000);
    return () => clearTimeout(timer);
  }, [reason]);

  const bridge = reason === 'bridge';

  return (
    <div className="offline-screen">
      <div className="offline-card">
        <div className="offline-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <line x1="1" y1="1" x2="23" y2="23"/>
            <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"/>
            <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"/>
            <path d="M10.71 5.05A16 16 0 0 1 22.56 9"/>
            <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"/>
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0"/>
            <line x1="12" y1="20" x2="12.01" y2="20"/>
          </svg>
        </div>
        <div className="offline-title">{t('offline.title')}</div>
        <div className="offline-desc">
          {bridge ? t('offline.bridgeDesc') : t('offline.desc')}<br/>
          {bridge ? t('offline.bridgeDesc2') : t('offline.desc2')}
        </div>
        {bridge && (
          <div className="offline-reasons">
            <div className="offline-reason"><b>1.</b> {t('offline.reasonOldCopy')}</div>
            <div className="offline-reason"><b>2.</b> {t('offline.reasonInstaller')}</div>
            <div className="offline-reason"><b>3.</b> {t('offline.reasonAntivirus')}</div>
          </div>
        )}
        <div className="offline-hint">{t('offline.versionHint')}</div>
        <button className="btn btn-accent offline-retry-btn" onClick={restartApp} disabled={restarting}>
          {restarting && <span className="offline-spinner" />}
          {restarting ? t('common.restarting') : t('common.restartBtn')}
        </button>
      </div>
    </div>
  );
}
