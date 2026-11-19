import { useEffect } from 'react';
import { subscribeEvent } from '../events';
import { api } from '../api';

// Recovery procedure events — модальное окно с этапами по центру.
// Провал: первый раз — чистый перезапуск приложения; если и после перезапуска
// восстановление не удалось — финальная ошибка с просьбой обратиться в поддержку.
export function useRecoveryEvents({ startupDone, setRecovery, setZapretLoading, setRecoveryReport }) {
  useEffect(() => {
    if (!startupDone) return;
    let hideTimer = null;
    const MARKER = 'zpui-recovery-restart';
    const MARKER_TTL = 10 * 60 * 1000;
    const markerAge = () => {
      const ts = Number(localStorage.getItem(MARKER) || 0);
      return ts > 0 ? Date.now() - ts : -1;
    };
    if (markerAge() > MARKER_TTL) localStorage.removeItem(MARKER);
    const upsertStep = (step) => {
      setRecovery((prev) => {
        const steps = [...prev.steps];
        const idx = steps.findIndex((s) => s.key === step.key);
        if (idx >= 0) steps[idx] = step; else steps.push(step);
        return { ...prev, steps };
      });
    };
    const closeRecovery = () => setRecovery({ open: false, steps: [], phase: 'progress' });
    const onStart = () => {
      setZapretLoading(true);
      setRecovery({ open: true, steps: [], phase: 'progress' });
    };
    const onStep = (step) => { if (step) upsertStep(step); };
    const onDone = async (data) => {
      const success = !!(data && data.success);
      // Пользователь начал деструктивное действие (стоп/перестановка) —
      // просто закрываем окно восстановления без ошибок и перезапусков.
      if (data && data.cancelled) {
        closeRecovery();
        setZapretLoading(false);
        return;
      }
      const report = data && data.report;
      setZapretLoading(false);
      if (success) {
        localStorage.removeItem(MARKER);
        setRecovery((prev) => ({ ...prev, phase: 'success' }));
        if (hideTimer) clearTimeout(hideTimer);
        hideTimer = setTimeout(closeRecovery, 2500);
        return;
      }
      const retried = markerAge() >= 0 && markerAge() <= MARKER_TTL;
      if (retried) {
        // Второй провал подряд (после перезапуска) — финальная ошибка.
        localStorage.removeItem(MARKER);
        closeRecovery();
        setRecoveryReport({ ...(report || {}), error_code: report?.error_code || data?.error_code, reason: report?.reason || data?.reason, second_failure: true });
        return;
      }
      localStorage.setItem(MARKER, String(Date.now()));
      setRecovery((prev) => ({ ...prev, phase: 'restarting' }));
      const r = await api('POST', '/api/app/restart-clean');
      if (r?.error) {
        // Перезапустить не вышло — сразу показываем финальную ошибку.
        localStorage.removeItem(MARKER);
        closeRecovery();
        setRecoveryReport({ ...(report || {}), error_code: report?.error_code || data?.error_code, reason: report?.reason || data?.reason, second_failure: true });
      }
    };
    const offs = [
      subscribeEvent('zapret:recovery:start', onStart),
      subscribeEvent('zapret:recovery:step', onStep),
      subscribeEvent('zapret:recovery:done', onDone),
    ];
    return () => {
      if (hideTimer) clearTimeout(hideTimer);
      offs.forEach((off) => off());
    };
  }, [startupDone, setRecovery, setZapretLoading, setRecoveryReport]);
}
