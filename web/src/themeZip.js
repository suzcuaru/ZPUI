// themeZip.js — экспорт/импорт темы стенда одним ZIP-архивом (fflate).
//
// Состав архива:
//   theme.json          — нормализованная тема (включая sounds {scheme, volume});
//   sounds/<file>.wav   — файлы звуковой схемы темы из библиотеки public/sounds;
//   bg/<basename>       — фоновая картинка, ЕСЛИ она из папки /bg/ стенда
//                         (в продукте — папка рядом с приложением, /api/bg/*).
//
// Импорт: theme.json валидируется normalizeTheme (клампы, в т.ч. sounds);
// bg/<file> заливается через существующий /api/bg/upload (POST raw) и тема
// получает ПУТЬ; звуки НЕ распаковываются в файловую систему — схема темы
// сверяется со встроенной библиотекой схем (в стенде все схемы уже есть в
// public/sounds; в продукте папка звуков лежит рядом с приложением, а
// normalizeScheme вернёт «standard» для неизвестной схемы).
// Легаси: одиночный .json по-прежнему принимается на вход (проверка magic-PK).

import { zipSync, unzipSync, strFromU8, strToU8 } from 'fflate';
import { normalizeTheme } from './themeCustom';
import { SOUND_SCHEMES } from './sound';

const BG_EXT_RE = /\.(jpe?g|png|webp|gif)$/i;

const basenameOf = (url) => decodeURIComponent(String(url).split('/').pop() || '');

export const isZipName = (name) => /\.zip$/i.test(String(name || ''));

// Blob.arrayBuffer/text есть не везде (старые WebView/jsdom) — фолбэк на FileReader.
export function blobToArrayBuffer(blob) {
  if (typeof blob.arrayBuffer === 'function') return blob.arrayBuffer();
  return new Promise((res, rej) => {
    const fr = new FileReader();
    fr.onload = () => res(fr.result);
    fr.onerror = rej;
    fr.readAsArrayBuffer(blob);
  });
}

export function blobToText(blob) {
  if (typeof blob.text === 'function') return blob.text();
  return new Promise((res, rej) => {
    const fr = new FileReader();
    fr.onload = () => res(String(fr.result));
    fr.onerror = rej;
    fr.readAsText(blob);
  });
}

// Быстрый признак ZIP: magic «PK\x03\x04».
export async function looksLikeZip(file) {
  try {
    const head = new Uint8Array(await blobToArrayBuffer(file.slice(0, 4)));
    return head[0] === 0x50 && head[1] === 0x4b && head[2] === 0x03 && head[3] === 0x04;
  } catch {
    return isZipName(file && file.name);
  }
}

const slug = (name) => String(name || 'theme').replace(/[^\w-]+/g, '_').slice(0, 32) || 'theme';
export const zipNameFor = (name) => 'zpui-theme-' + slug(name) + '.zip';

async function fetchBytes(url) {
  try {
    const r = await fetch(url);
    if (!r.ok) return null;
    return new Uint8Array(await r.arrayBuffer());
  } catch {
    return null;
  }
}

// Экспорт: slot = {name, theme} → Blob(zip). Недокачанные части просто
// пропускаются — theme.json в архиве всегда.
export async function exportThemeZip(slot) {
  const theme = normalizeTheme(slot && slot.theme);
  const files = {};
  files['theme.json'] = strToU8(JSON.stringify(theme, null, 2));

  const schemeMap = SOUND_SCHEMES[theme.sounds.scheme] || {};
  for (const file of new Set(Object.values(schemeMap))) {
    const bytes = await fetchBytes('/sounds/' + file);
    if (bytes) files['sounds/' + file] = bytes;
  }

  if (theme.bgImage.url && theme.bgImage.url.startsWith('/bg/') && BG_EXT_RE.test(theme.bgImage.url)) {
    const bytes = await fetchBytes(theme.bgImage.url);
    if (bytes) files['bg/' + basenameOf(theme.bgImage.url)] = bytes;
  }

  const zipped = zipSync(files);
  return new Blob([zipped], { type: 'application/zip' });
}

// Скачивание Blob как файл (общий хелпер для галереи и редактора).
export function downloadBlob(blob, filename) {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => { try { URL.revokeObjectURL(a.href); } catch {} }, 1000);
}

// Импорт: File(zip) → {theme} (нормализована, картинка залита в /bg/).
// Бросает исключение при отсутствии/битости theme.json.
export async function importThemeZip(file) {
  const buf = new Uint8Array(await blobToArrayBuffer(file));
  let entries;
  try {
    entries = unzipSync(buf);
  } catch {
    throw new Error('bad_zip');
  }
  let raw = entries['theme.json'];
  if (!raw) {
    const key = Object.keys(entries).find((n) => n === 'theme.json'
      || (n.endsWith('.json') && !n.includes('/'))
      || n.endsWith('/theme.json'));
    raw = key ? entries[key] : null;
  }
  if (!raw) throw new Error('no_theme_json');
  let parsed;
  try {
    parsed = JSON.parse(strFromU8(raw));
  } catch {
    throw new Error('bad_json');
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('bad_json');

  const theme = normalizeTheme(parsed);

  // Картинка из архива → /api/bg/upload (POST raw, как BgRibbon) → путь.
  const bgKey = Object.keys(entries).find((n) => n.startsWith('bg/') && BG_EXT_RE.test(n));
  if (bgKey) {
    try {
      const r = await fetch('/api/bg/upload', {
        method: 'POST',
        headers: { 'Content-Type': 'application/octet-stream', 'X-Filename': encodeURIComponent(basenameOf(bgKey)) },
        body: entries[bgKey],
      });
      const d = await r.json().catch(() => null);
      if (d && d.path) theme.bgImage.url = d.path;
    } catch {}
  }

  // Звуки: файлы архива не распаковываем — схема темы сверяется со встроенной
  // библиотекой (normalizeTheme уже клампнул scheme). Неизвестные имена файлов
  // звуков в архиве игнорируются: в продукте набор звуков — папка рядом
  // с приложением, тема хранит только имя СХЕМЫ.
  return { theme };
}
