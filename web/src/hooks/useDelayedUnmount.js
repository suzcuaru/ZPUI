import { useEffect, useState } from 'react';

// useDelayedUnmount — задержка размонтирования для анимации выхода (MOTION LAYER):
// при open=true узел рендерится сразу; при open→false держится ещё ms мс
// (за это время проигрывается .is-closing), затем убирается.
// Возвращает [render, closing]: closing=true на время анимации выхода.
// Пример: меню `{render && <div className={'menu' + (closing ? ' is-closing' : '')}>…</div>}`.
export default function useDelayedUnmount(open, ms = 200) {
  const [state, setState] = useState(open ? 'mounted' : 'unmounted');

  useEffect(() => {
    if (open) {
      setState('mounted');
      return;
    }
    setState(prev => (prev === 'mounted' ? 'closing' : prev));
  }, [open]);

  useEffect(() => {
    if (state !== 'closing') return undefined;
    const timer = setTimeout(() => setState('unmounted'), ms);
    return () => clearTimeout(timer);
  }, [state, ms]);

  return [state !== 'unmounted', state === 'closing'];
}
