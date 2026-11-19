// Экономный режим для фоновых таймеров фронтенда: модуль-синглтон с подписчиками
// (по образцу winVisible.js). Источник истины — /api/status (power_saving от
// power.Active() бэка): App.jsx обновляет флаг при каждом успешном опросе.
// До первого ответа флаг false — поллинг обычный, никаких деградаций до
// подтверждения экономки бэкендом. Хуки usePolling/useNow (через
// usePowerInterval) замедляют «медленные» таймеры (интервал ≥ 10с) вдвое.
let active = false;
const subs = new Set();

export function setPowerSaving(v) {
  v = !!v;
  if (v === active) return;
  active = v;
  subs.forEach((fn) => { try { fn(v); } catch {} });
}

export function isPowerSaving() {
  return active;
}

export function onPowerSavingChange(fn) {
  subs.add(fn);
  return () => subs.delete(fn);
}
