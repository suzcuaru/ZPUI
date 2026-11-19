export function formatBytes(bytes) {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return parseFloat((bytes / Math.pow(1024, i)).toFixed(2)) + ' ' + units[i];
}

export function formatSpeed(bps) {
  if (bps < 1024) return bps.toFixed(0) + ' B/s';
  if (bps < 1024 * 1024) return (bps / 1024).toFixed(1) + ' KB/s';
  return (bps / (1024 * 1024)).toFixed(1) + ' MB/s';
}

export function formatSpeedMB(bps) {
  const mb = bps / (1024 * 1024);
  return mb.toFixed(2).padStart(6, ' ') + ' MB/s';
}

export function formatBytesMB(bytes) {
  const mb = bytes / (1024 * 1024);
  if (mb < 1) return '0.00 MB';
  return mb.toFixed(2).padStart(6, ' ') + ' MB';
}

// Время последней сверки с сервером (RFC3339): сегодня — только время,
// раньше — дата и время. Пустая/битая дата → ''.
export function formatSyncTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  const opts = d.toDateString() === new Date().toDateString()
    ? { hour: '2-digit', minute: '2-digit' }
    : { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' };
  return d.toLocaleString(undefined, opts);
}

export function strategyDisplayName(raw) {
  if (!raw) return '—';
  let s = String(raw).trim();
  s = s.replace(/\.bat$/i, '');
  if (/^\d+$/.test(s)) return 'general' + s;
  if (/^general\d+$/.test(s)) return s;
  if (s === 'general' || s === 'general.bat') return 'general';
  return s;
}

// ── Санитизация полей авторизации ─────────────────────
// React экранирует вывод (XSS через рендер невозможен), поэтому защита — на
// входе: управляющие символы и ограничители разметки вырезаются, длина
// клампится ещё до отправки на бэкенд. Формат-проверки — отдельно.

// Управляющие символы (в т.ч. \n \t \0) + угловые скобки — в логине/почте им
// делать нечего ни при каком формате.
const INJECT_RE = /[\u0000-\u001f\u007f<>]/g;

export function sanitizeText(value, maxLen = 64) {
  return String(value ?? '').replace(INJECT_RE, '').trim().slice(0, maxLen);
}

// Логин: буквы/цифры/._@- (внутри), начало — буква или цифра.
export function sanitizeLogin(value, maxLen = 32) {
  return sanitizeText(value, maxLen).replace(/[^A-Za-z0-9._@-]/g, '');
}

export function isValidLogin(value) {
  return /^[A-Za-z0-9][A-Za-z0-9._@-]{2,31}$/.test(String(value || ''));
}

export function isValidEmail(value) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(String(value || ''));
}

// ── Deep-diff / deep-merge патчей конфига ─────────────
// POST /api/config уходит ТОЛЬКО с изменёнными ключами: устаревший снимок
// полного конфига (например, server.telemetry_enabled) не должен перезаписывать
// свежие значения бэка.

export function isPlainObject(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

// Патч из ключей b, отличающихся от a (рекурсивно; массивы и не-объекты —
// целиком). Ключи, которых нет в a, попадают в патч; удаление ключей не
// выражается — патчи только добавляют/меняют.
export function deepDiff(a, b) {
  const out = {};
  if (!isPlainObject(a) || !isPlainObject(b)) return b;
  for (const k of Object.keys(b)) {
    if (!(k in a)) {
      out[k] = b[k];
      continue;
    }
    const av = a[k];
    const bv = b[k];
    if (isPlainObject(av) && isPlainObject(bv)) {
      const sub = deepDiff(av, bv);
      if (Object.keys(sub).length > 0) out[k] = sub;
      continue;
    }
    if (av !== bv) out[k] = bv;
  }
  return out;
}

// Слияние патчей: объекты рекурсивно, остальные значения заменяются.
export function deepMergePatch(base, patch) {
  if (!isPlainObject(base) || !isPlainObject(patch)) return patch;
  const out = { ...base };
  for (const k of Object.keys(patch)) {
    const pv = patch[k];
    out[k] = isPlainObject(pv) ? deepMergePatch(out[k], pv) : pv;
  }
  return out;
}

// ── Сетевые ошибки сервера (AuthGate / ServerOfflineModal) ──
// null от api() — упавший Wails-биндинг (сеть/таймаут); коды — явные
// признаки недоступности сервера из ответов бэка.
export const NET_ERROR_CODES = new Set([
  'server_unreachable', 'server_unavailable', 'network_error', 'no_connection',
  'connection_refused', 'timeout', 'unreachable', 'server_error', 'offline',
]);

export function isNetError(result) {
  if (result === null || result === undefined) return true;
  return !!(result && typeof result === 'object' && NET_ERROR_CODES.has(result.error));
}

// ── Состояние DNS-тумблера (DnsPage) ──────────────────
// Пресет Xbox (xbox-dns.ru): учитывается наравне с пользовательскими записями.
export const XBOX_DNS_PRESET = ['111.88.96.54', '111.88.96.55'];

const normDnsAddr = (v) => String(v || '').trim().toLowerCase().replace(/^https?:\/\//, '').replace(/\/+$/, '');

// 'user'  — в системе прописаны включённые пользовательские DNS;
// 'xbox'  — только пресет Xbox; 'mixed' — пользовательские + Xbox;
// 'off'   — внешние/стандартные адреса (тумблер выключен).
export function dnsActiveState({ current = [], servers = [], xboxEnabled = false }) {
  const sys = (Array.isArray(current) ? current : []).map(normDnsAddr).filter(Boolean);
  if (sys.length === 0) return 'off';
  const enabledUser = (Array.isArray(servers) ? servers : [])
    .filter((s) => s && s.enabled !== false)
    .map((s) => normDnsAddr(s.addr));
  const userActive = enabledUser.some((a) => sys.includes(a));
  const xboxActive = !!xboxEnabled && XBOX_DNS_PRESET.map(normDnsAddr).some((a) => sys.includes(a));
  if (userActive && xboxActive) return 'mixed';
  if (userActive) return 'user';
  if (xboxActive) return 'xbox';
  return 'off';
}

// ── Валидация адреса DNS-сервера ──────────────────────
const IPV4_RE = /^((25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(25[0-5]|2[0-4]\d|1?\d?\d)$/;
// Грубая проверка на клиенте (строгая — net.ParseIP на бэкенде).
const IPV6_RE = /^[0-9a-fA-F:]{2,45}$/;
const HOSTNAME_RE = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i;

export function isValidDnsAddress(value) {
  const v = String(value || '').trim();
  if (!v) return false;
  if (IPV4_RE.test(v)) return true;
  if (v.includes(':') && IPV6_RE.test(v)) return true;
  if (/^https:\/\/[^\s/]+(\/\S*)?$/i.test(v)) return true;
  return HOSTNAME_RE.test(v);
}

// Строгая IP-проверка (IPv4/IPv6) — только IP можно прописать на адаптер
// (домены/URL отбрасывает и бэкенд через net.ParseIP).
export function isValidDnsIp(value) {
  const v = String(value || '').trim();
  if (!v) return false;
  if (IPV4_RE.test(v)) return true;
  return v.includes(':') && IPV6_RE.test(v);
}

// Семья адреса: true — IPv6.
export const isIPv6Addr = (value) => String(value || '').includes(':');

// Домен (для списков исключений): минимум две метки, допустим префикс «^»
// (точное совпадение без поддоменов в синтаксисе hostlist).
const DOMAIN_RE = /^(?=.{1,253}$)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i;

export function isValidDomain(value) {
  let v = String(value || '').trim();
  if (v.startsWith('^')) v = v.slice(1);
  return DOMAIN_RE.test(v);
}

// ── Единое UI-состояние DNS (сайдбар / Обзор / hero DNS-вкладки) ──
// Источник истины — /api/status → xbox_dns (Go сводит туда enabled + факт
// применения на системном адаптере). Поля нет/старый бэк — фолбэк на локальный
// вывод dnsActiveState. Все точки UI читают ОДНО поле → показывают одно и то же.
export function dnsUiState(statusDns, fallback = {}) {
  if (statusDns && typeof statusDns === 'object') {
    if (statusDns.enabled !== true) return { on: false, state: 'off', activeCount: 0 };
    const activeCount = Number(statusDns.active_count)
      || (Array.isArray(statusDns.servers) ? statusDns.servers.filter(Boolean).length : 0);
    const applied = statusDns.applied === true
      || statusDns.system_active === true
      || activeCount > 0;
    return { on: applied, state: applied ? 'user' : 'off', activeCount };
  }
  const s = dnsActiveState(fallback);
  return { on: s !== 'off', state: s, activeCount: 0 };
}

// ── Per-серверная доступность DNS (Go отдаёт health-поля) ──
// Принимаем и health ('ok'|'fail'|'unknown'|'checking'), и булев available;
// ничего не известно — 'unknown' (серая точка).
export function dnsHealthOf(s) {
  if (!s || typeof s !== 'object') return 'unknown';
  const h = String(s.health || '').toLowerCase();
  if (h === 'ok' || h === 'fail' || h === 'unknown' || h === 'checking') return h;
  if (s.available === true) return 'ok';
  if (s.available === false) return 'fail';
  return 'unknown';
}

import { tr } from './i18n';

// ── Telegram: единое открытие каналов/групп/.deep-link ──
// Вход: t.me-ссылка (https://t.me/name), голый домен или готовый tg://.
// Приоритет — ПРИЛОЖЕНИЕ Telegram (tg://resolve?domain=…); фолбэк —
// https://t.me/… (для браузера, если tg:// не сработал).
export function tgDomainFromUrl(link) {
  const raw = String(link || '').trim();
  if (/^[A-Za-z0-9_]{3,64}$/.test(raw)) return raw; // голый домен
  const m = raw.match(/^(?:https?:\/\/)?(?:t(?:elegram)?\.me)\/(?:s\/)?@?([A-Za-z0-9_]{3,64})/i);
  return m ? m[1] : '';
}

export function openTelegram(link) {
  const raw = String(link || '').trim();
  if (!raw) return;
  const isTgLink = /^tg:\/\//i.test(raw);
  const domain = tgDomainFromUrl(raw);
  const tgUrl = isTgLink ? raw : (domain ? 'tg://resolve?domain=' + domain : '');
  const webUrl = isTgLink ? '' : (domain ? 'https://t.me/' + domain : (/^https?:\/\//i.test(raw) ? raw : ''));

  // 1) Wails/WebView2: системный обработчик протоколов — откроет приложение.
  const rt = typeof window !== 'undefined' ? window.runtime : null;
  if (rt && typeof rt.BrowserOpenURL === 'function') {
    const target = tgUrl || webUrl || raw;
    try {
      rt.BrowserOpenURL(target);
      return;
    } catch {}
  }
  // 2) Браузер: пробуем tg:// через локацию; если приложение не открылось —
  // через паузу открываем веб-версию.
  if (tgUrl) {
    try { window.location.href = tgUrl; } catch {}
    setTimeout(() => {
      try {
        if (!document.hidden && webUrl) window.open(webUrl, '_blank', 'noopener');
      } catch {}
    }, 1500);
    return;
  }
  if (webUrl) {
    try { window.open(webUrl, '_blank', 'noopener'); } catch {}
  }
}

// ── Даты: никакой «Invalid Date» ──
// NULL/пусто в контексте подписки (unlimited: true) → «Бессрочно»;
// невалидное/отсутствующее → «—» (или свой noneText).
// «0001-01-01…» (zero-дата .NET) и 0 трактуется как бессрочность при unlimited.
export function formatDateSafe(value, opts = {}) {
  const unlimited = opts.unlimited === true;
  const none = opts.noneText || '—';
  if (value === undefined || value === null || value === '') {
    return unlimited ? tr('common.unlimited') : none;
  }
  if (typeof value === 'number') {
    if (!isFinite(value)) return none;
    if (value === 0) return unlimited ? tr('common.unlimited') : none;
    const d = new Date(value * (opts.seconds ? 1000 : 1));
    if (isNaN(d.getTime())) return none;
    return d.toLocaleDateString(opts.lang === 'en' ? 'en-US' : 'ru-RU');
  }
  const s = String(value);
  if (/^0001-01-01/.test(s)) return tr('common.unlimited');
  const d = new Date(s);
  if (isNaN(d.getTime())) return none;
  return d.toLocaleDateString(opts.lang === 'en' ? 'en-US' : 'ru-RU');
}

// Бессрочная подписка: NULL/отсутствие поля или zero-дата .NET.
// 0/пустая строка/мусор — не бессрочность (покажем «—»).
export function isUnlimitedDate(value) {
  if (value === undefined || value === null) return true;
  return /^0001-01-01/.test(String(value));
}

// Процент заполнения слайдера для CSS-переменной --val (класс .slider-ui).
export function rangePct(value, min, max) {
  if (max <= min) return 0;
  const pct = ((Number(value) - min) / (max - min)) * 100;
  return Math.round(Math.max(0, Math.min(100, pct)));
}

// Сдвиг центрированной подсказки (margin-left, px) в СИСТЕМЕ КООРДИНАТ
// элемента (внутри zoom-контекста .stand-app), чтобы подсказка шириной
// tipW (CSS-px, до zoom) не вылезала за [boundsLeft, boundsRight]
// (визуальные px, getBoundingClientRect). pad — визуальные поля по краям.
// Если границы уже подсказки — прижимаем к левому краю (max не меньше min).
export function tooltipShiftMl({ center, boundsLeft, boundsRight, tipW, zoom, pad }) {
  const z = Number(zoom) > 0 ? Number(zoom) : 1;
  const p = Number(pad == null ? 8 * z : pad);
  const half = (Number(tipW) * z) / 2;
  const min = Number(boundsLeft) + half + p;
  const max = Math.max(min, Number(boundsRight) - half - p);
  const clamped = Math.max(min, Math.min(max, Number(center)));
  return (clamped - Number(center)) / z;
}

// Вертикальный сдвиг подсказки (translateY, px) в системе координат элемента
// (внутри zoom-контекста), чтобы подсказка высотой tipH (CSS-px) не выходила
// за [boundsTop, boundsBottom] (визуальные px). side:
//   'top'    — подсказка НАД элементом (default/top-left): сдвиг только вниз (>=0);
//   'bottom' — подсказка ПОД элементом: сдвиг только вверх (<=0);
//   'center' — сбоку от элемента (right/left): вертикальное центрирование, сдвиг в обе стороны.
// gap — зазор между элементом и подсказкой (6px, как в CSS bottom/top:calc(100%+6px)).
export function tooltipShiftTy({ side, rectTop, rectBottom, rectHeight, boundsTop, boundsBottom, tipH, zoom, pad, gap }) {
  const z = Number(zoom) > 0 ? Number(zoom) : 1;
  const p = Number(pad == null ? 8 * z : pad);
  const g = Number(gap == null ? 6 * z : gap);
  const h = Number(tipH) * z;
  if (side === 'bottom') {
    const tipBottom = Number(rectBottom) + g + h;
    const max = Number(boundsBottom) - p;
    return tipBottom > max ? -(tipBottom - max) / z : 0;
  }
  if (side === 'center') {
    const c = Number(rectTop) + Number(rectHeight || 0) / 2;
    const min = Number(boundsTop) + h / 2 + p;
    const max = Math.max(min, Number(boundsBottom) - h / 2 - p);
    const clamped = Math.max(min, Math.min(max, c));
    return (clamped - c) / z;
  }
  const tipTop = Number(rectTop) - g - h;
  const min = Number(boundsTop) + p;
  return tipTop < min ? (min - tipTop) / z : 0;
}

// ── Единицы графиков (MonitorPage) ────────────────────

// bytes/s → Мбит/с (×8/1e6).
export function bytesToMbps(bps) {
  const n = Number(bps);
  if (!isFinite(n) || n <= 0) return 0;
  return (n * 8) / 1e6;
}

// Скорость канала: KB/s | MB/s | GB/s — авто.
export function formatRateAuto(bps) {
  const v = Number(bps);
  if (!isFinite(v) || v < 0) return '0 KB/s';
  if (v < 1024) return v.toFixed(0) + ' B/s';
  if (v < 1024 * 1024) return (v / 1024).toFixed(1) + ' KB/s';
  if (v < 1024 * 1024 * 1024) return (v / (1024 * 1024)).toFixed(1) + ' MB/s';
  return (v / (1024 * 1024 * 1024)).toFixed(2) + ' GB/s';
}

// Числа для оси/тултипа с фиксированной метрикой.
export function formatMetricValue(value, metric) {
  const v = Number(value);
  if (!isFinite(v)) return '0';
  if (metric === 'raw') return String(Math.round(v));
  if (metric === 'mbps') return (Math.round(v * 10) / 10).toFixed(1);
  if (metric === 'ms') return Math.round(v).toFixed(0);
  return formatRateAuto(v);
}
