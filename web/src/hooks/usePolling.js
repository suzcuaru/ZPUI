import { useEffect, useRef, useCallback } from 'react';

/**
 * usePolling — хук для периодического вызова функции с автоматической очисткой.
 *
 * @param {Function} fetchFn — async-функция для вызова
 * @param {number} intervalMs — интервал в миллисекундах (0 = без polling, только начальный вызов)
 * @param {Array} deps — зависимости для пересоздания fetchFn (опционально)
 * @returns {Function} — обёрнутая функция для ручного вызова
 *
 * Пример:
 *   const { data, refetch } = usePolling(() => api('GET', '/api/status'), 2000);
 */
export function usePolling(fetchFn, intervalMs = 5000, deps = []) {
  const fnRef = useRef(fetchFn);
  fnRef.current = fetchFn;

  const refetch = useCallback(async () => {
    await fnRef.current();
  }, []);

  useEffect(() => {
    let alive = true;

    const poll = async () => {
      await fnRef.current();
      return alive;
    };

    poll();

    if (intervalMs > 0) {
      const iv = setInterval(poll, intervalMs);
      return () => {
        alive = false;
        clearInterval(iv);
      };
    }

    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [intervalMs, ...deps]);

  return refetch;
}