import { useRef, useCallback, useEffect } from 'react';
import { api, apiCall } from '../api';

// FIX: version counter prevents race condition + cleanup on unmount
export function useDebouncedSave(url, delay = 500, onSuccess = null) {
  const configRef = useRef({});
  const timerRef = useRef(null);
  const versionRef = useRef(0);
  const pendingVersionRef = useRef(0);

  const updateFn = useCallback((patch, currentConfig = null) => {
    if (currentConfig) configRef.current = { ...configRef.current, ...currentConfig };
    configRef.current = { ...configRef.current, ...patch };
    versionRef.current++;
    const v = versionRef.current;
    pendingVersionRef.current = v;
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(async () => {
      if (pendingVersionRef.current !== v) return;
      await apiCall(() => api('POST', url, configRef.current), null, null);
      if (onSuccess) onSuccess();
    }, delay);
  }, [url, delay, onSuccess]);

  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current); }, []);
  return updateFn;
}
