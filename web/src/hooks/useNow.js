import { useState, useEffect } from 'react';
import { usePageVisible } from './usePageVisible';
import { isInteractive, onWindowShownChange } from '../winVisible';
import { usePowerInterval } from './usePowerInterval';

/**
 * useNow — тикающий Date.now() для листовых компонентов («обновлено N сек
 * назад», отсчёты). Изолирует посекундные перерисовки: обновляется только
 * вызвавший компонент, не вся страница. В скрытом окне не тикает, на
 * неактивной keep-mounted странице — тоже (пока страница скрыта, значение
 * замирает; возврат на вкладку обновляет немедленно).
 *
 * Крупный интервал (например 30000) годится для «двигающегося» окна графика:
 * правый край смещается на ~0.4% ширины за шаг — визуально плавно, а тяжёлый
 * пересчёт путей SVG происходит раз в полминуты, а не каждую секунду.
 * В экономном режиме крупные интервалы (≥ 10с) идут вдвое реже (powerFlag).
 */
export function useNow(intervalMs = 1000) {
  const [now, setNow] = useState(() => Date.now());
  const pageVisible = usePageVisible();
  const effMs = usePowerInterval(intervalMs);

  useEffect(() => {
    if (!pageVisible) return undefined;
    const update = () => { if (isInteractive()) setNow(Date.now()); };
    update();
    const iv = setInterval(update, effMs);
    const onVisible = () => update();
    const offWin = onWindowShownChange(onVisible);
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      clearInterval(iv);
      offWin();
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [effMs, pageVisible]);

  return now;
}
