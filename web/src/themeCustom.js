// themeCustom.js — движок пользовательской темы стенда.
// Кастомная тема = база (light/dark) + переопределения токенов. Генерирует
// CSS-текст для селектора :root[data-theme="custom"] и кладёт его в один
// <style id="zpui-custom-theme-style"> в head. Картинка фона передаётся через
// --custom-bg-image / -size / -repeat / -pos / -satur / -bright / -dim / -vig
// (статические правила под них — в index.css). При картинке на .stand-win
// ставится класс has-custom-bg: оболочка (тайтлбар/сайдбар/футер) и
// контент-карта становятся одним стеклом (--bg-page с альфой), зазоры
// между панелями убираются. Снятие темы = удаление style-тега + маркера.
// bgImage.url хранит ПУТЬ к файлу из отдельной папки картинок (/bg/<file>,
// в продукте — папка рядом с приложением) либо внешний URL. data:-URL —
// legacy старых тем: продолжает работать, новые темы создаются только путями.
// Старые сохранения могли записать путь БЕЗ префикса папки (/bg-test.jpg
// вместо /bg/bg-test.jpg) — normalizeTheme мигрирует такие пути, а
// resolveThemeBgUrl при применении сверяется с фактическим списком /api/bg/list.

import { normalizeScheme } from './sound';

const STYLE_ID = 'zpui-custom-theme-style';

const BG_EXT_RE = /\.(jpe?g|png|webp|gif)$/i;

// Легаси-путь: корневой путь к картинке без префикса папки (/bg-*).
export function isLegacyBgPath(url) {
  return typeof url === 'string' && url.startsWith('/') && !url.startsWith('/bg/') && BG_EXT_RE.test(url);
}

// Миграция легаси-пути: /bg-test.jpg → /bg/bg-test.jpg. Если известен список
// файлов папки ([{name, path}] из /api/bg/list) — резолвим по нему (учитывает
// кодировку имён); без списка — просто дописываем префикс /bg/.
export function migrateBgUrl(url, files) {
  if (!isLegacyBgPath(url)) return url;
  const base = decodeURIComponent(url.replace(/^\/+/, ''));
  if (Array.isArray(files) && files.length) {
    const hit = findBgFile(files, base);
    if (hit && hit.path) return hit.path;
    return url;
  }
  return '/bg/' + encodeURIComponent(base);
}

// Точный поиск файла по basename (сравниваем имя и его encode/decode-варианты —
// путь в теме мог быть записан с percent-кодированием кириллицы).
function findBgFile(files, base) {
  return files.find((f) => f && (f.name === base
    || decodeURIComponent(f.name) === base
    || encodeURIComponent(f.name) === base));
}

// Финальный резолв пути при применении/загрузке темы: ЛЮБОЙ локальный путь
// (начинается с «/» — легаси-путь без папки или уже мигрированный /bg/<file>)
// сверяется с фактическим содержимым папки картинок (/api/bg/list) по basename.
// Нашли файл — берём его ПУТЬ; файла нет — картинка просто снимается (url=''),
// применение темы не падает. Не-локальные URL (внешние ссылки, data:-легаси)
// возвращаются как есть.
export async function resolveThemeBgUrl(theme) {
  const t = normalizeTheme(theme);
  const url = t.bgImage.url;
  if (typeof url !== 'string' || !url.startsWith('/')) return t;
  const base = decodeURIComponent(url.replace(/^\/+/, '')).split('/').pop();
  let files = null;
  try {
    const r = await fetch('/api/bg/list');
    const d = await r.json();
    if (d && Array.isArray(d.files)) files = d.files;
  } catch {}
  if (!files) {
    // Список недоступен: лучшее усилие — префикс /bg/ у легаси-путей, не падаем.
    return { ...t, bgImage: { ...t.bgImage, url: migrateBgUrl(url) } };
  }
  const hit = findBgFile(files, base);
  return { ...t, bgImage: { ...t.bgImage, url: hit && hit.path ? hit.path : '' } };
}

// Базовые палитры — зеркало index.css (:root и [data-theme="dark"]).
const LIGHT_TOKENS = {
  bgPage: '#f5f6f8', bgSection: '#ffffff', bgCard: '#ffffff',
  bgHover: 'rgba(0,0,0,0.04)', bgActive: 'rgba(43,103,212,0.08)', bgInset: '#f0f1f4',
  border: '#e2e4ea', borderLight: '#eceef2',
  textPrimary: '#1a1a22', textSecondary: '#5b606e', textTertiary: '#6b7080',
  accent: '#2b67d4', accentHover: '#2258c2', accentSoft: 'rgba(43,103,212,0.10)', accentText: '#ffffff',
  success: '#127544', successBg: 'rgba(18,117,68,0.10)',
  danger: '#c02b2b', dangerBg: 'rgba(192,43,43,0.08)', dangerStrong: '#c02b2b',
  warning: '#8f6400', warningBg: 'rgba(255,184,0,0.12)',
  shadowSm: '0 1px 3px rgba(0,0,0,0.04)', shadowMd: '0 4px 12px rgba(0,0,0,0.06)', shadowLg: '0 8px 24px rgba(0,0,0,0.08)',
  tooltipBg: '#2a2a35', tooltipText: '#f0f0f5',
};

const DARK_TOKENS = {
  bgPage: '#141418', bgSection: '#1c1c24', bgCard: '#20202a',
  bgHover: 'rgba(255,255,255,0.06)', bgActive: 'rgba(107,163,255,0.10)', bgInset: '#16161e',
  border: '#2e2e38', borderLight: '#262630',
  textPrimary: '#e5e5ed', textSecondary: '#9ea3b3', textTertiary: '#8a8fa0',
  accent: '#6ba3ff', accentHover: '#82b3ff', accentSoft: 'rgba(107,163,255,0.12)', accentText: '#ffffff',
  success: '#34d058', successBg: 'rgba(52,208,88,0.12)',
  danger: '#f55e5e', dangerBg: 'rgba(245,94,94,0.10)', dangerStrong: '#c42b1c',
  warning: '#ffcc33', warningBg: 'rgba(255,204,51,0.12)',
  shadowSm: '0 1px 3px rgba(0,0,0,0.2)', shadowMd: '0 4px 12px rgba(0,0,0,0.25)', shadowLg: '0 8px 24px rgba(0,0,0,0.3)',
  tooltipBg: '#3a3a48', tooltipText: '#f0f0f5',
};

const COLOR_FIELDS = [
  'accent', 'success', 'warning', 'danger',
  'bgPage', 'bgCard', 'bgInset',
  'textPrimary', 'textSecondary', 'textTertiary',
  'border', 'borderLight',
];

const clamp = (v, min, max) => Math.min(max, Math.max(min, v));

function hexToRgb(hex) {
  let h = String(hex || '').trim().replace(/^#/, '');
  if (h.length === 3) h = h.split('').map((ch) => ch + ch).join('');
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return null;
  return { r: parseInt(h.slice(0, 2), 16), g: parseInt(h.slice(2, 4), 16), b: parseInt(h.slice(4, 6), 16) };
}

function rgbToHex(r, g, b) {
  const p = (n) => clamp(Math.round(n), 0, 255).toString(16).padStart(2, '0');
  return '#' + p(r) + p(g) + p(b);
}

function hexToHsl(hex) {
  const rgb = hexToRgb(hex);
  if (!rgb) return null;
  const r = rgb.r / 255, g = rgb.g / 255, b = rgb.b / 255;
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  const l = (max + min) / 2;
  let h = 0, s = 0;
  if (max !== min) {
    const d = max - min;
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    if (max === r) h = ((g - b) / d + (g < b ? 6 : 0));
    else if (max === g) h = (b - r) / d + 2;
    else h = (r - g) / d + 4;
    h *= 60;
  }
  return { h, s: s * 100, l: l * 100 };
}

function hslToHex(h, s, l) {
  s /= 100; l /= 100;
  const k = (n) => (n + h / 30) % 12;
  const a = s * Math.min(l, 1 - l);
  const f = (n) => l - a * Math.max(-1, Math.min(k(n) - 3, Math.min(9 - k(n), 1)));
  return rgbToHex(f(0) * 255, f(8) * 255, f(4) * 255);
}

const shiftLightness = (hex, delta) => {
  const hsl = hexToHsl(hex);
  return hsl ? hslToHex(hsl.h, hsl.s, clamp(hsl.l + delta, 0, 100)) : hex;
};

const scaleLightness = (hex, mult) => {
  const hsl = hexToHsl(hex);
  return hsl ? hslToHex(hsl.h, hsl.s, clamp(hsl.l * mult, 3, 97)) : hex;
};

function rgba(hex, a) {
  const rgb = hexToRgb(hex);
  return rgb ? `rgba(${rgb.r},${rgb.g},${rgb.b},${a})` : hex;
}

function brightness(hex) {
  const rgb = hexToRgb(hex);
  if (!rgb) return 0;
  return (0.299 * rgb.r + 0.587 * rgb.g + 0.114 * rgb.b) / 255;
}

export function normalizeHexInput(v) {
  let h = String(v || '').trim().replace(/^#/, '');
  if (h.length === 3) h = h.split('').map((ch) => ch + ch).join('');
  return /^[0-9a-fA-F]{6}$/.test(h) ? '#' + h.toLowerCase() : null;
}

const normHex = (v, fallback) => normalizeHexInput(v) || fallback;

export const BG_FITS = ['cover', 'contain', 'repeat'];

// Веб-безопасные стеки шрифтов темы: ключи — значения селекта в редакторе,
// стеки подставляются в --font/--font-mono при применении темы.
export const FONT_STACKS = {
  system: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif",
  segoe: "'Segoe UI', 'Inter', system-ui, sans-serif",
  roboto: "'Roboto', 'Segoe UI', system-ui, sans-serif",
  arial: "Arial, 'Helvetica Neue', Helvetica, sans-serif",
  georgia: "Georgia, 'Times New Roman', serif",
};
export const FONT_MONO_STACKS = {
  jetbrains: "'JetBrains Mono', 'Cascadia Code', 'Consolas', monospace",
  consolas: "'Consolas', 'Cascadia Mono', 'JetBrains Mono', monospace",
};
const FONT_FAMILY_KEYS = Object.keys(FONT_STACKS);
const FONT_MONO_KEYS = Object.keys(FONT_MONO_STACKS);
const HEADING_WEIGHTS = [500, 600, 700];

export function defaultTheme(base) {
  const b = base === 'light' ? LIGHT_TOKENS : DARK_TOKENS;
  return {
    version: 2,
    base: base === 'light' ? 'light' : 'dark',
    colors: {
      accent: b.accent, success: b.success, warning: b.warning, danger: b.danger,
      bgPage: b.bgPage, bgCard: b.bgCard, bgInset: b.bgInset,
      textPrimary: b.textPrimary, textSecondary: b.textSecondary, textTertiary: b.textTertiary,
      border: b.border, borderLight: b.borderLight,
    },
    radius: { md: 10, lg: 14, shell: 12 },
    contrast: 100,
    typography: { family: 'system', monoFamily: 'jetbrains', scale: 100, headingWeight: 600, letterSpacing: 0 },
    bgImage: {
      url: '', panelOpacity: 85, shellOpacity: 85, fit: 'cover',
      posX: 50, posY: 50, satur: 100, bright: 100, blur: 6, dim: 30,
    },
    effects: {
      glow: false, glowColor: '', translucent: false, strength: 60,
      vignette: false, vignetteStrength: 50,
      shadowStrength: 100, modalOpacity: 100, modalBlur: 0,
    },
    // Звуки темы: перекрывают scheme/volume из конфига, пока тема активна.
    sounds: { scheme: 'soft', volume: 60 },
  };
}

export function normalizeTheme(raw, bgFiles) {
  const d = defaultTheme(raw && raw.base);
  if (!raw || typeof raw !== 'object') return d;
  const colors = { ...d.colors };
  if (raw.colors && typeof raw.colors === 'object') {
    for (const k of COLOR_FIELDS) colors[k] = normHex(raw.colors[k], d.colors[k]);
  }
  const radius = {
    md: clamp(Number(raw.radius?.md ?? d.radius.md), 4, 16),
    lg: clamp(Number(raw.radius?.lg ?? d.radius.lg), 4, 16),
    shell: clamp(Number(raw.radius?.shell ?? d.radius.shell), 0, 20),
  };
  const typ = raw.typography && typeof raw.typography === 'object' ? raw.typography : {};
  const typography = {
    family: FONT_FAMILY_KEYS.includes(typ.family) ? typ.family : d.typography.family,
    monoFamily: FONT_MONO_KEYS.includes(typ.monoFamily) ? typ.monoFamily : d.typography.monoFamily,
    scale: clamp(Math.round(Number(typ.scale ?? d.typography.scale)), 90, 115),
    headingWeight: HEADING_WEIGHTS.includes(Number(typ.headingWeight)) ? Number(typ.headingWeight) : d.typography.headingWeight,
    letterSpacing: clamp(Math.round(Number(typ.letterSpacing ?? d.typography.letterSpacing) * 100) / 100, -0.2, 0.8),
  };
  const bg = raw.bgImage && typeof raw.bgImage === 'object' ? raw.bgImage : {};
  const fx = raw.effects && typeof raw.effects === 'object' ? raw.effects : {};
  const snd = raw.sounds && typeof raw.sounds === 'object' ? raw.sounds : {};
  return {
    version: 2,
    base: d.base,
    colors,
    radius,
    typography,
    contrast: clamp(Number(raw.contrast ?? d.contrast), 50, 150),
    bgImage: {
      url: migrateBgUrl(typeof bg.url === 'string' ? bg.url : '', bgFiles),
      panelOpacity: clamp(Number(bg.panelOpacity ?? d.bgImage.panelOpacity), 0, 100),
      shellOpacity: clamp(Number(bg.shellOpacity ?? d.bgImage.shellOpacity), 0, 100),
      fit: BG_FITS.includes(bg.fit) ? bg.fit : 'cover',
      posX: clamp(Number(bg.posX ?? d.bgImage.posX), 0, 100),
      posY: clamp(Number(bg.posY ?? d.bgImage.posY), 0, 100),
      satur: clamp(Number(bg.satur ?? d.bgImage.satur), 0, 200),
      bright: clamp(Number(bg.bright ?? d.bgImage.bright), 50, 150),
      blur: clamp(Number(bg.blur ?? d.bgImage.blur), 0, 20),
      dim: clamp(Number(bg.dim ?? d.bgImage.dim), 0, 90),
    },
    effects: {
      glow: fx.glow === true,
      glowColor: normalizeHexInput(fx.glowColor) || '',
      translucent: fx.translucent === true,
      strength: clamp(Number(fx.strength ?? d.effects.strength), 0, 100),
      vignette: fx.vignette === true,
      vignetteStrength: clamp(Number(fx.vignetteStrength ?? d.effects.vignetteStrength), 0, 100),
      shadowStrength: clamp(Number(fx.shadowStrength ?? d.effects.shadowStrength), 0, 200),
      modalOpacity: clamp(Number(fx.modalOpacity ?? d.effects.modalOpacity), 40, 100),
      modalBlur: clamp(Number(fx.modalBlur ?? d.effects.modalBlur), 0, 24),
    },
    sounds: {
      scheme: normalizeScheme(snd.scheme),
      volume: clamp(Number(snd.volume ?? d.sounds.volume), 0, 100),
    },
  };
}

export const cloneTheme = (t) => JSON.parse(JSON.stringify(t));

export const themeEquals = (a, b) => JSON.stringify(normalizeTheme(a)) === JSON.stringify(normalizeTheme(b));

// Множитель «Силы теней»: пересчитывает альфу каждого rgba() в строке тени.
// 0 → 'none' (тени выключены), 1 → строка без изменений.
function scaleShadow(shadow, mult) {
  if (!(mult > 0)) return 'none';
  if (mult === 1) return shadow;
  return shadow.replace(/rgba\(([^)]*)\)/g, (m, inner) => {
    const parts = inner.split(',');
    const a = parseFloat(parts[3]);
    if (parts.length < 4 || Number.isNaN(a)) return m;
    parts[3] = String(Math.round(Math.min(1, a * mult) * 1000) / 1000);
    return 'rgba(' + parts.join(',') + ')';
  });
}

// Финальные значения токенов: база + переопределения + производные.
function resolveTokens(theme) {
  const t = normalizeTheme(theme);
  const dark = t.base !== 'light';
  const base = dark ? DARK_TOKENS : LIGHT_TOKENS;
  const c = t.colors;
  const m = t.contrast / 100;

  const accent = normHex(c.accent, base.accent);
  const success = normHex(c.success, base.success);
  const warning = normHex(c.warning, base.warning);
  const danger = normHex(c.danger, base.danger);

  let surfaceAlpha = 1;
  let shellAlpha = 1;
  if (t.bgImage.url) {
    surfaceAlpha = t.bgImage.panelOpacity / 100;
    shellAlpha = t.bgImage.shellOpacity / 100;
  } else if (t.effects.translucent) {
    surfaceAlpha = 1 - (t.effects.strength / 100) * 0.55;
  }
  const surf = (hex) => (surfaceAlpha < 1 ? rgba(hex, Math.round(surfaceAlpha * 100) / 100) : hex);
  const shell = (hex) => (shellAlpha < 1 ? rgba(hex, Math.round(shellAlpha * 100) / 100) : hex);

  // Прозрачность модалок: множится на альфу поверхности, чтобы при картинке
  // дефолтные 100% давали то же стекло, что и раньше (см. --custom-modal-bg).
  const modalAlpha = Math.round((t.effects.modalOpacity / 100) * surfaceAlpha * 100) / 100;

  const hasImg = !!t.bgImage.url;
  const fitVars = !hasImg ? {} : {
    '--custom-bg-size': t.bgImage.fit === 'repeat' ? 'auto' : t.bgImage.fit,
    '--custom-bg-repeat': t.bgImage.fit === 'repeat' ? 'repeat' : 'no-repeat',
    '--custom-bg-pos': t.bgImage.posX + '% ' + t.bgImage.posY + '%',
    '--custom-bg-satur': String(Math.round(t.bgImage.satur) / 100),
    '--custom-bg-bright': String(Math.round(t.bgImage.bright) / 100),
    '--custom-bg-vig': String(t.effects.vignette ? Math.round(t.effects.vignetteStrength) / 100 : 0),
  };

  const shadowMult = (t.effects.shadowStrength ?? 100) / 100;

  return {
    vars: {
      '--bg-page': shell(normHex(c.bgPage, base.bgPage)),
      '--bg-section': surf(base.bgSection),
      '--bg-card': surf(normHex(c.bgCard, base.bgCard)),
      '--bg-inset': surf(normHex(c.bgInset, base.bgInset)),
      '--bg-hover': base.bgHover,
      '--bg-active': rgba(accent, dark ? 0.10 : 0.08),
      '--border': normHex(c.border, base.border),
      '--border-light': normHex(c.borderLight, base.borderLight),
      '--text-primary': scaleLightness(normHex(c.textPrimary, base.textPrimary), m),
      '--text-secondary': scaleLightness(normHex(c.textSecondary, base.textSecondary), m),
      '--text-tertiary': scaleLightness(normHex(c.textTertiary, base.textTertiary), m),
      '--accent': accent,
      '--accent-hover': shiftLightness(accent, dark ? 8 : -8),
      '--accent-soft': rgba(accent, dark ? 0.12 : 0.10),
      '--accent-text': brightness(accent) > 0.62 ? '#16161e' : '#ffffff',
      '--success': success,
      '--success-bg': rgba(success, dark ? 0.12 : 0.10),
      '--danger': danger,
      '--danger-bg': rgba(danger, dark ? 0.10 : 0.08),
      '--danger-strong': dark ? shiftLightness(danger, -18) : danger,
      '--warning': warning,
      '--warning-bg': rgba(warning, 0.12),
      '--shadow-sm': scaleShadow(base.shadowSm, shadowMult),
      '--shadow-md': scaleShadow(base.shadowMd, shadowMult),
      '--shadow-lg': scaleShadow(base.shadowLg, shadowMult),
      '--tooltip-bg': base.tooltipBg,
      '--tooltip-text': base.tooltipText,
      '--radius-md': t.radius.md + 'px',
      '--radius-lg': t.radius.lg + 'px',
      '--shell-radius': t.radius.shell + 'px',
      '--content-radius': Math.max(0, t.radius.shell - 2) + 'px',
      '--font': FONT_STACKS[t.typography.family] || FONT_STACKS.system,
      '--font-mono': FONT_MONO_STACKS[t.typography.monoFamily] || FONT_MONO_STACKS.jetbrains,
      '--fs-scale': String(Math.round(t.typography.scale) / 100),
      '--fw-heading': String(t.typography.headingWeight),
      '--ls-adj': (Math.round(t.typography.letterSpacing * 100) / 100) + 'px',
      '--custom-bg-dim': String(t.bgImage.dim / 100),
      /* Непрозрачный цвет страницы: база для вуалей читаемости (сама
         --bg-page при картинке полупрозрачна и для скримов не годится). */
      '--custom-page-solid': normHex(c.bgPage, base.bgPage),
      '--custom-glow-color': normHex(t.effects.glowColor, '') || accent,
      '--custom-modal-bg': rgba(normHex(c.bgCard, base.bgCard), modalAlpha),
      ...(t.effects.modalBlur > 0 ? { '--custom-modal-blur': Math.round(t.effects.modalBlur) + 'px' } : {}),
      ...(t.bgImage.url ? { '--custom-bg-image': `url("${t.bgImage.url.replace(/"/g, '%22')}")` } : {}),
      ...fitVars,
    },
    meta: { ...t, dark, surfaceAlpha, shellAlpha },
  };
}

const GLOW_SELECTORS = [
  '.btn-accent', '.btn.active', '.sidebar-nav-item.active',
  '.switch-track[aria-checked="true"]:not(:focus-visible)',
];

// backdrop-filter НЕ генерируется сознательно: в Chromium при zoom оболочки
// текст внутри замороченных элементов растрируется замыленным («применение
// кастомной темы размывает текст»). Стекло = только полупрозрачный тинт.
export function buildCustomThemeCss(theme) {
  const { vars, meta } = resolveTokens(theme);
  const lines = Object.entries(vars).map(([k, v]) => `  ${k}:${v};`);
  let css = `:root[data-theme="custom"]{\n${lines.join('\n')}\n}`;

  if (meta.effects.glow) {
    const px = Math.round(6 + (meta.effects.strength / 100) * 10);
    const sel = GLOW_SELECTORS.map((s) => `:root[data-theme="custom"] ${s}`).join(',');
    css += `\n${sel}{box-shadow:0 0 ${px}px color-mix(in srgb, var(--custom-glow-color, var(--accent)) 45%, transparent);}`;
  }
  // Модалки: прозрачность и размытие фона за ними применяются статикой в
  // index.css через var(--custom-modal-bg)/var(--custom-modal-blur) —
  // дублирующая инжекция здесь больше не нужна.
  return css;
}

function styleEl() {
  let el = document.getElementById(STYLE_ID);
  if (!el) {
    el = document.createElement('style');
    el.id = STYLE_ID;
    document.head.appendChild(el);
  }
  return el;
}

// Маркер режима фоновой картинки: на .stand-win включает правила единого
// стеклянного окна (все панели одним стеклом, без зазоров) — см. index.css.
const WIN_MARK = 'has-custom-bg';

let appliedHasImage = false;

function syncWindowMark(hasImage) {
  appliedHasImage = !!hasImage;
  document.querySelectorAll('.stand-win').forEach((el) => el.classList.toggle(WIN_MARK, appliedHasImage));
}

// Повторная расстановка маркера на текущих .stand-win: окно могло смонтироваться
// ПОСЛЕ применения темы (AuthGate/стартап поверх) — без этого фон не красился.
export function resyncCustomThemeMark() {
  document.querySelectorAll('.stand-win').forEach((el) => el.classList.toggle(WIN_MARK, appliedHasImage));
}

export function applyCustomTheme(theme) {
  if (typeof document === 'undefined') return;
  styleEl().textContent = buildCustomThemeCss(theme);
  syncWindowMark(!!normalizeTheme(theme).bgImage.url);
}

export function removeCustomTheme() {
  if (typeof document === 'undefined') return;
  const el = document.getElementById(STYLE_ID);
  if (el) el.remove();
  syncWindowMark(false);
}
