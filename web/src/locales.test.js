import { describe, it, expect } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import ru from './locales/ru.json';
import en from './locales/en.json';

// Литеральные ключи t('...') / tr('...') в исходниках обязаны существовать
// в ОБОИХ локалях (ru + en). Динамические ключи (шаблонные строки) не
// проверяются — они вне формата литерала.
const SRC = path.join(__dirname, '.');

function collectFiles(dir, acc = []) {
  for (const name of fs.readdirSync(dir)) {
    if (name === 'locales' || name.startsWith('locales.test')) continue;
    const p = path.join(dir, name);
    const st = fs.statSync(p);
    if (st.isDirectory()) collectFiles(p, acc);
    else if (/\.(jsx|js)$/.test(name) && !name.endsWith('.test.js') && !name.endsWith('.test.jsx')) acc.push(p);
  }
  return acc;
}

function extractLiteralKeys(src) {
  const keys = new Set();
  const re = /\b(?:t|tr)\(\s*['"]([A-Za-z0-9_.-]+)['"]\s*[,)]/g;
  let m;
  while ((m = re.exec(src)) !== null) {
    if (m[1].includes('.')) keys.add(m[1]);
  }
  return keys;
}

function hasPath(dict, key) {
  let node = dict;
  for (const seg of key.split('.')) {
    if (node === null || typeof node !== 'object' || !(seg in node)) return false;
    node = node[seg];
  }
  return typeof node === 'string' || typeof node === 'object';
}

const files = collectFiles(SRC);
const allKeys = new Set();
for (const f of files) {
  for (const k of extractLiteralKeys(fs.readFileSync(f, 'utf8'))) allKeys.add(k);
}

describe('целостность локалей', () => {
  it('в исходниках найдены литеральные i18n-ключи (санити сканера)', () => {
    expect(allKeys.size).toBeGreaterThan(200);
  });

  it('каждый литеральный t()/tr() ключ есть в ru.json', () => {
    const missing = [...allKeys].filter((k) => !hasPath(ru, k));
    expect(missing, 'нет в ru.json: ' + missing.join(', ')).toEqual([]);
  });

  it('каждый литеральный t()/tr() ключ есть в en.json', () => {
    const missing = [...allKeys].filter((k) => !hasPath(en, k));
    expect(missing, 'нет в en.json: ' + missing.join(', ')).toEqual([]);
  });

  it('локали структурно совпадают (ключи ru.json есть в en.json)', () => {
    function paths(node, prefix = '') {
      let out = [];
      for (const [k, v] of Object.entries(node)) {
        const p = prefix ? prefix + '.' + k : k;
        if (v !== null && typeof v === 'object') out = out.concat(paths(v, p));
        else out.push(p);
      }
      return out;
    }
    const ruPaths = new Set(paths(ru));
    const enPaths = new Set(paths(en));
    const onlyRu = [...ruPaths].filter((p) => !enPaths.has(p));
    expect(onlyRu, 'ключи ru.json без пары в en.json: ' + onlyRu.slice(0, 20).join(', ')).toEqual([]);
  });

  it('локали структурно совпадают и в обратную сторону (ключи en.json есть в ru.json)', () => {
    function paths(node, prefix = '') {
      let out = [];
      for (const [k, v] of Object.entries(node)) {
        const p = prefix ? prefix + '.' + k : k;
        if (v !== null && typeof v === 'object') out = out.concat(paths(v, p));
        else out.push(p);
      }
      return out;
    }
    const ruPaths = new Set(paths(ru));
    const enPaths = new Set(paths(en));
    const onlyEn = [...enPaths].filter((p) => !ruPaths.has(p));
    expect(onlyEn, 'ключи en.json без пары в ru.json: ' + onlyEn.slice(0, 20).join(', ')).toEqual([]);
  });

  it('в локалях нет пустых значений-заглушек', () => {
    function empties(node, prefix = '') {
      let out = [];
      for (const [k, v] of Object.entries(node)) {
        const p = prefix ? prefix + '.' + k : k;
        if (v !== null && typeof v === 'object') out = out.concat(empties(v, p));
        else if (typeof v === 'string' && v.trim() === '') out.push(p);
      }
      return out;
    }
    expect(empties(ru), 'пустые значения в ru.json').toEqual([]);
    expect(empties(en), 'пустые значения в en.json').toEqual([]);
  });
});
