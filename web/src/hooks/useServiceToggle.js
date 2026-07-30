import { useState, useCallback } from 'react';
import { api, apiCall } from '../api';

const POLL_INTERVAL = 1000;
const POLL_TIMEOUT = 30000;

function delay(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export function useServiceToggle(service, isRunning, showToast) {
  const [loading, setLoading] = useState(false);

  const toggle = useCallback(async () => {
    setLoading(true);

    try {
      if (service === 'xboxdns') {
        const cfg = await api('GET', '/api/xbox-dns/config');
        if (cfg) {
          const result = await api('POST', '/api/xbox-dns/config', { ...cfg, enabled: !isRunning });
          if (result?.error) showToast(result.error, 'error');
        }
      } else if (service === 'zapret' && !isRunning) {
        const result = await api('POST', '/api/zapret/start');
        if (result?.error) {
          showToast(result.error, 'error');
          setLoading(false);
          return;
        }

        const deadline = Date.now() + POLL_TIMEOUT;
        let zapRunning = false;
        while (Date.now() < deadline) {
          await delay(POLL_INTERVAL);
          const status = await api('GET', '/api/status');
          if (status?.zapret?.status === 'running') {
            zapRunning = true;
            break;
          }
        }

        if (!zapRunning) {
          showToast('Zapret failed to start — process not responding', 'error');
        }
      } else {
        const result = await api('POST', `/api/${service}/${isRunning ? 'stop' : 'start'}`);
        if (result?.error) {
          showToast(result.error, 'error');
        }
      }

      await apiCall(() => api('POST', '/api/component-states'));
    } catch {
      if (showToast) showToast('Operation failed', 'error');
    } finally {
      setLoading(false);
    }
  }, [service, isRunning, showToast]);

  return { loading, toggle };
}
