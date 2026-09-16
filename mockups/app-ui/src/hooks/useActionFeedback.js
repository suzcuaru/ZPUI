import { useState, useCallback, useRef } from 'react';

// Единый поток действия: pending → ответ мока → флеш ok/err → тост.
// Никаких уведомлений до получения ответа.
export function useActionFeedback(showToast) {
  const [busy, setBusy] = useState(false);
  const [flash, setFlash] = useState('');
  const timerRef = useRef(null);

  const run = useCallback(async (fn, opts = {}) => {
    if (timerRef.current) clearTimeout(timerRef.current);
    setBusy(true);
    setFlash('');
    let res;
    try {
      res = await fn();
    } catch {
      res = { error: 'Operation failed' };
    }
    const failed = !res || res.error;
    setFlash(failed ? 'err' : 'ok');
    if (failed) {
      showToast(res?.error || opts.errorMsg || 'Ошибка', 'error');
    } else if (opts.successMsg) {
      showToast(opts.successMsg, 'success');
    }
    timerRef.current = setTimeout(() => {
      setFlash('');
      setBusy(false);
    }, failed ? 500 : 600);
    return res;
  }, [showToast]);

  return { busy, flash, run };
}
