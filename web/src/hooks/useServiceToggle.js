import { useState, useCallback } from 'react';
import { api, apiCall } from '../api';

/**
 * useServiceToggle — хук для управления переключателем сервиса (zapret/proxy/xboxdns).
 * Заменяет 3 копии функций toggleZapret/toggleProxy/toggleXboxDns в Header.jsx.
 *
 * @param {string} service — имя сервиса: 'zapret' | 'proxy' | 'xboxdns'
 * @param {boolean} isRunning — текущее состояние
 * @param {Function} showToast — функция показа тоста
 * @param {Object} messages — { startMsg, stopMsg } (переведённые строки)
 * @returns {Object} { loading, toggle }
 *
 * Пример:
 *   const zapret = useServiceToggle('zapret', zRun, showToast, { startMsg: '...', stopMsg: '...' });
 *   <button onClick={zapret.toggle} disabled={zapret.loading} />
 */
export function useServiceToggle(service, isRunning, showToast, messages = {}) {
  const [loading, setLoading] = useState(false);

  const toggle = useCallback(async () => {
    setLoading(true);

    try {
      if (service === 'xboxdns') {
        // Xbox DNS: переключаем через конфиг
        const cfg = await api('GET', '/api/xbox-dns/config');
        if (cfg) {
          await apiCall(
            () => api('POST', '/api/xbox-dns/config', { ...cfg, enabled: !isRunning }),
            isRunning ? messages.stopMsg : messages.startMsg,
            showToast
          );
        }
      } else {
        // zapret / proxy: POST /api/<service>/start|stop
        const result = await api('POST', `/api/${service}/${isRunning ? 'stop' : 'start'}`);
        if (result?.error) {
          showToast(result.error, 'error');
        } else if (isRunning ? messages.stopMsg : messages.startMsg) {
          showToast(isRunning ? messages.stopMsg : messages.startMsg, 'success');
        }
      }

      // Сохраняем состояния компонентов
      await apiCall(() => api('POST', '/api/component-states'));
    } catch {
      if (showToast) showToast('Не удалось выполнить операцию', 'error');
    } finally {
      setLoading(false);
    }
  }, [service, isRunning, showToast, messages.startMsg, messages.stopMsg]);

  return { loading, toggle };
}