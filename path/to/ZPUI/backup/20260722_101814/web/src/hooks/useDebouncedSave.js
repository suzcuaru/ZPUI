import { useRef, useCallback } from 'react';
import { api, apiCall } from '../api';

/**
 * useDebouncedSave — хук для debounced-сохранения конфигурации.
 * Заменяет дублированную логику в ProxyPage (через useState) и SettingsPage (через useRef).
 *
 * @param {string} url — POST endpoint для сохранения (например, '/api/proxy/config')
 * @param {number} delay — задержка в мс (по умолчанию 500)
 * @param {Function} onSuccess — колбек при успехе (например, toast)
 * @returns {Function} updateFn(patch) — мержит patch в текущий конфиг и дебаунсит сохранение
 *
 * Пример:
 *   const [config, setConfig] = useState(null);
 *   const updateConfig = useDebouncedSave('/api/proxy/config', 500, () => showToast('Saved'));
 *   updateConfig({ port: 8080 });
 */
export function useDebouncedSave(url, delay = 500, onSuccess = null) {
  const configRef = useRef({});
  const timerRef = useRef(null);

  const updateFn = useCallback((patch, currentConfig = null) => {
    // Если передан currentConfig — используем его как базу, иначе берём из ref
    if (currentConfig) {
      configRef.current = { ...configRef.current, ...currentConfig };
    }
    configRef.current = { ...configRef.current, ...patch };

    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(async () => {
      await apiCall(
        () => api('POST', url, configRef.current),
        null,
        null
      );
      if (onSuccess) onSuccess();
    }, delay);
  }, [url, delay, onSuccess]);

  return updateFn;
}