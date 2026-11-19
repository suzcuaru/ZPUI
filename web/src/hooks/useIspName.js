import { useState, useEffect } from 'react';
import { api } from '../api';
import { subscribeEvent } from '../events';

// useIspName — имя текущего провайдера для экранов подбора стратегии.
// Читает /api/isp/current при активации и обновляет по operator:changed.
// null = провайдер ещё определяется.
export default function useIspName(active = true) {
  const [name, setName] = useState(null);

  useEffect(() => {
    if (!active) return undefined;
    let alive = true;
    const load = async () => {
      const cur = await api('GET', '/api/isp/current');
      if (!alive) return;
      if (cur && !cur.error && (cur.key || cur.name)) {
        setName(String(cur.name || '').trim() || null);
      } else {
        setName(null);
      }
    };
    load();
    const off = subscribeEvent('operator:changed', () => load());
    return () => {
      alive = false;
      off();
    };
  }, [active]);

  return name;
}
