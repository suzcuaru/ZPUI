// themeCustom.js — движок пользовательской темы стенда.
// Кастомная тема = база (light/dark) + переопределения токенов. Генерирует
// CSS-текст для селектора :root[data-theme="custom"] и кладёт его в один
// <style id="zpui-custom-theme-style"> в head. Картинка фона передаётся
// через --custom-bg-image / --custom-bg-dim / --custom-bg-blur (статические
// правила под них — в index.css). Снятие темы = удаление style-тега.

const STYLE_ID = 'zpui-custom-theme-style';

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

export function defaultTheme(base) {
  const b = base === 'light' ? LIGHT_TOKENS : DARK_TOKENS;
  return {
    version: 1,
    base: base === 'light' ? 'light' : 'dark',
    colors: {
      accent: b.accent, success: b.success, warning: b.warning, danger: b.danger,
      bgPage: b.bgPage, bgCard: b.bgCard, bgInset: b.bgInset,
      textPrimary: b.textPrimary, textSecondary: b.textSecondary, textTertiary: b.textTertiary,
      border: b.border, borderLight: b.borderLight,
    },
    radius: { md: 10, lg: 14 },
    contrast: 100,
    bgImage: { url: '', panelOpacity: 85, blur: 6, dim: 30 },
    effects: { glow: false, translucent: false, strength: 60 },
  };
}

export function normalizeTheme(raw) {
  const d = defaultTheme(raw && raw.base);
  if (!raw || typeof raw !== 'object') return d;
  const colors = { ...d.colors };
  if (raw.colors && typeof raw.colors === 'object') {
    for (const k of COLOR_FIELDS) colors[k] = normHex(raw.colors[k], d.colors[k]);
  }
  const radius = {
    md: clamp(Number(raw.radius?.md ?? d.radius.md), 4, 16),
    lg: clamp(Number(raw.radius?.lg ?? d.radius.lg), 4, 16),
  };
  const bg = raw.bgImage && typeof raw.bgImage === 'object' ? raw.bgImage : {};
  const fx = raw.effects && typeof raw.effects === 'object' ? raw.effects : {};
  return {
    version: 1,
    base: d.base,
    colors,
    radius,
    contrast: clamp(Number(raw.contrast ?? d.contrast), 50, 150),
    bgImage: {
      url: typeof bg.url === 'string' ? bg.url : '',
      panelOpacity: clamp(Number(bg.panelOpacity ?? d.bgImage.panelOpacity), 0, 100),
      blur: clamp(Number(bg.blur ?? d.bgImage.blur), 0, 20),
      dim: clamp(Number(bg.dim ?? d.bgImage.dim), 0, 90),
    },
    effects: {
      glow: fx.glow === true,
      translucent: fx.translucent === true,
      strength: clamp(Number(fx.strength ?? d.effects.strength), 0, 100),
    },
  };
}

export const cloneTheme = (t) => JSON.parse(JSON.stringify(t));

export const themeEquals = (a, b) => JSON.stringify(normalizeTheme(a)) === JSON.stringify(normalizeTheme(b));

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
  if (t.bgImage.url) surfaceAlpha = t.bgImage.panelOpacity / 100;
  else if (t.effects.translucent) surfaceAlpha = 1 - (t.effects.strength / 100) * 0.55;
  const surf = (hex) => (surfaceAlpha < 1 ? rgba(hex, Math.round(surfaceAlpha * 100) / 100) : hex);

  return {
    vars: {
      '--bg-page': normHex(c.bgPage, base.bgPage),
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
      '--shadow-sm': base.shadowSm,
      '--shadow-md': base.shadowMd,
      '--shadow-lg': base.shadowLg,
      '--tooltip-bg': base.tooltipBg,
      '--tooltip-text': base.tooltipText,
      '--radius-md': t.radius.md + 'px',
      '--radius-lg': t.radius.lg + 'px',
      '--custom-bg-dim': String(t.bgImage.dim / 100),
      '--custom-bg-blur': t.bgImage.blur + 'px',
      ...(t.bgImage.url ? { '--custom-bg-image': `url("${t.bgImage.url.replace(/"/g, '%22')}")` } : {}),
    },
    meta: { ...t, dark, surfaceAlpha },
  };
}

const BLUR_SELECTORS = [
  '.section', '.card', '.card-section', '.modal', '.svc-card', '.dash-card',
  '.upd-card', '.us-panel', '.us-tile', '.us-metric',
];

const GLOW_SELECTORS = [
  '.btn-accent', '.btn.active', '.sidebar-nav-item.active',
  '.switch-track[aria-checked="true"]:not(:focus-visible)',
];

export function buildCustomThemeCss(theme) {
  const { vars, meta } = resolveTokens(theme);
  const lines = Object.entries(vars).map(([k, v]) => `  ${k}:${v};`);
  let css = `:root[data-theme="custom"]{\n${lines.join('\n')}\n}`;

  const needsBlur = meta.bgImage.blur > 0 && (meta.bgImage.url || meta.effects.translucent);
  if (needsBlur) {
    const sel = BLUR_SELECTORS.map((s) => `:root[data-theme="custom"] ${s}`).join(',');
    css += `\n${sel}{-webkit-backdrop-filter:blur(var(--custom-bg-blur,0px));backdrop-filter:blur(var(--custom-bg-blur,0px));}`;
  }
  if (meta.effects.glow) {
    const px = Math.round(6 + (meta.effects.strength / 100) * 10);
    const sel = GLOW_SELECTORS.map((s) => `:root[data-theme="custom"] ${s}`).join(',');
    css += `\n${sel}{box-shadow:0 0 ${px}px color-mix(in srgb, var(--accent) 45%, transparent);}`;
  }
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

export function applyCustomTheme(theme) {
  if (typeof document === 'undefined') return;
  styleEl().textContent = buildCustomThemeCss(theme);
}

export function removeCustomTheme() {
  if (typeof document === 'undefined') return;
  const el = document.getElementById(STYLE_ID);
  if (el) el.remove();
}
