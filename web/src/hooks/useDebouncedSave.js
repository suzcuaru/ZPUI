import { useRef, useCallback, useEffect } from 'react';
import { api, apiCall } from '../api';
import { deepMergePatch } from '../utils';

/**
 * useDebouncedSave — debounced-сохранение конфигурации ПАТЧАМИ.
 *
 * В POST уходит только накопленный патч (deep-merge последовательных
 * изменений), а НЕ снимок полного конфига: устаревшие ветки (например,
 * server.telemetry_enabled из снимка на момент загрузки страницы) иначе
 * перезаписывали бы свежие значения бэка. Второй аргумент updateFn
 * (currentConfig) сохранён для совместимости вызовов, но НЕ включается
 * в payload.
 *
 * @param {string} url — POST endpoint (например, '/api/config')
 * @param {number} delay — задержка в мс (по умолчанию 500)
 * @param {Function} onSuccess — колбек при успехе
 * @returns {Function} updateFn(patch) — накапливает патч и дебаунсит сохранение
 */
export function useDebouncedSave(url, delay = 500, onSuccess = null) {
  const patchRef = useRef({});
  const timerRef = useRef(null);

  // Незавершённое сохранение выполняется сразу при размонтировании —
  // иначе быстрый уход со вкладки терял последнее изменение.
  useEffect(() => {
    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        apiCall(() => api('POST', url, patchRef.current), null, null);
      }
    };
  }, [url]);

  const updateFn = useCallback((patch /* , currentConfig — игнорируется */) => {
    patchRef.current = deepMergePatch(patchRef.current, patch && typeof patch === 'object' ? patch : {});

    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(async () => {
      timerRef.current = null;
      await apiCall(
        () => api('POST', url, patchRef.current),
        null,
        null
      );
      if (onSuccess) onSuccess();
    }, delay);
  }, [url, delay, onSuccess]);

  return updateFn;
}
