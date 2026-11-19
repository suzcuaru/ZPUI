import { useEffect, useRef, useCallback } from 'react';
import { usePageVisible } from './usePageVisible';
import { isInteractive, onWindowShownChange } from '../winVisible';
import { usePowerInterval } from './usePowerInterval';

/**
 * usePolling — хук для периодического вызова функции с автоматической очисткой.
 *
 * @param {Function} fetchFn — async-функция для вызова
 * @param {number} intervalMs — интервал в миллисекундах (0 = без polling, только начальный вызов)
 * @param {Array} deps — зависимости для пересоздания fetchFn (опционально)
 * @param {boolean} enabled — вкл/выкл опроса целиком (false: ни интервала,
 *   ни начального вызова; включение даёт немедленный вызов — опционально)
 * @returns {Function} — функция refetch для принудительного ручного вызова
 *
 * Поведение:
 *  - медленный ответ не копит очередь: тик пропускается, пока прошлый вызов
 *    не завершился (refetch() при этом форсирует вызов);
 *  - скрытое окно (трей/close-to-tray/--hidden/свёрнуто) опрос не будит —
 *    по возвращении видимости данные догоняются немедленно;
 *  - неактивная keep-mounted страница (display:none) — как скрытое окно:
 *    тики пропускаются, возврат на вкладку даёт немедленный вызов;
 *  - экономный режим (powerFlag): интервалы ≥ 10с идут вдвое реже, частые
 *    поллеры (<10с) не трогаются; смена флага перезаряжает таймер (лишний
 *    немедленный вызов при переключении допустим).
 *
 * Пример:
 *   const refetch = usePolling(() => api('GET', '/api/status'), 2000);
 *   const refetchLogs = usePolling(loadLogs, 3000, [loadLogs], open);
 */
export function usePolling(fetchFn, intervalMs = 5000, deps = [], enabled = true) {
  const fnRef = useRef(fetchFn);
  fnRef.current = fetchFn;

  const busyRef = useRef(false);
  const pageVisible = usePageVisible();
  const effMs = usePowerInterval(intervalMs);

  const run = useCallback(async (force = false) => {
    if (busyRef.current && !force) return;
    busyRef.current = true;
    try {
      await fnRef.current();
    } finally {
      busyRef.current = false;
    }
  }, []);

  const refetch = useCallback(() => run(true), [run]);

  useEffect(() => {
    if (!enabled || !pageVisible) return undefined;

    let alive = true;

    const tick = () => {
      if (alive && isInteractive()) run();
    };

    tick();

    const onVisible = () => tick();
    const offWin = onWindowShownChange(onVisible);
    document.addEventListener('visibilitychange', onVisible);

    if (effMs > 0) {
      const iv = setInterval(tick, effMs);
      return () => {
        alive = false;
        clearInterval(iv);
        offWin();
        document.removeEventListener('visibilitychange', onVisible);
      };
    }

    return () => {
      alive = false;
      offWin();
      document.removeEventListener('visibilitychange', onVisible);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [effMs, enabled, pageVisible, run, ...deps]);

  return refetch;
}
