/**
 * db.js — слой данных фронтенда.
 *
 * IndexedDB полностью удалён. Исторические данные (снапшоты трафика)
 * берутся через API бекенда, который читает из SQLite (zpui.db).
 *
 * Для простого кэширования API-ответов (resource-status, strategies и т.д.)
 * используется in-memory Map — этого достаточно, так как данные всё равно
 * периодически опрашиваются.
 */

import { api } from './api';

const _cache = new Map();

/** Получить закэшированные данные по ключу (in-memory). */
export async function cacheGet(key) {
  const entry = _cache.get(key);
  return entry ? entry.data : null;
}

/** Сохранить данные в in-memory кэш. */
export async function cacheSet(key, data) {
  _cache.set(key, { data, ts: Date.now() });
}

/** Вернуть возраст кэша в мс (-1 если нет). */
export async function cacheGetAge(key) {
  const entry = _cache.get(key);
  if (!entry) return -1;
  return Date.now() - (entry.ts || 0);
}

// ── Подписка: окно «Продлить» ─────────────────────────
// Кнопка появляется, когда до конца подписки осталось меньше порога периода.
const RENEW_WINDOW_DAYS = { monthly: 1, quarterly: 14, yearly: 30 };

/** Порог появления «Продлить» (в днях) для периода тарифа. */
export function renewThresholdDays(period) {
  return RENEW_WINDOW_DAYS[period] || RENEW_WINDOW_DAYS.monthly;
}

/** Истинна, если подписка истекает в пределах порога своего периода. */
export function renewalDue(untilUnix, period) {
  if (!untilUnix) return false;
  return untilUnix * 1000 - Date.now() <= renewThresholdDays(period) * 86400000;
}

/**
 * Получить снапшоты трафика из бекенда (SQLite).
 * @param {number} sinceMs — timestamp (мс), по умолчанию последние 30 мин
 * @returns {Promise<Array>} массив снапшотов {dl_speed, ul_speed, total_dl, ...}
 */
export async function getSnapshots(sinceMs) {
  const minutes = sinceMs
    ? Math.ceil((Date.now() - sinceMs) / 60000)
    : 30;
  try {
    const data = await api('GET', `/api/monitor/snapshots?minutes=${minutes}`);
    if (!data || !data.snapshots) return [];
    // Нормализуем поля под формат, ожидаемый фронтендом
    return data.snapshots.map(s => ({
      ts: s.timestamp ? new Date(s.timestamp).getTime() : 0,
      dl: s.dl_speed || 0,
      ul: s.ul_speed || 0,
      totalDl: s.total_dl || 0,
      totalUl: s.total_ul || 0,
      conns: s.conn_count || 0,
    }));
  } catch {
    return [];
  }
}