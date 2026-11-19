import { useState, useEffect } from 'react';
import { isPowerSaving, onPowerSavingChange } from '../powerFlag';

// Граница «медленного» таймера: интервалы от 10с в экономном режиме удваиваются,
// частые поллеры (статус 2–5с, логи 3с, трафик 5с) не трогаются.
const POWER_SLOW_MS = 10000;

/**
 * usePowerInterval — эффективный интервал таймера с учётом экономного режима.
 *
 * Флаг выключен или интервал < 10с — значение 1:1 как до внедрения; флаг
 * включён и интервал ≥ 10с — ×2. Смена флага меняет возвращаемое значение,
 * поэтому effect, держащий setInterval с этим значением в deps, перезаряжает
 * таймер (немедленный лишний тик при переключении режима допустим).
 */
export function usePowerInterval(intervalMs) {
  const [saving, setSaving] = useState(isPowerSaving);
  useEffect(() => onPowerSavingChange(setSaving), []);
  return saving && intervalMs >= POWER_SLOW_MS ? intervalMs * 2 : intervalMs;
}
