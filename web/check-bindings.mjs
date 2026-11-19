// Проверка: каждый Go-метод, который дергает фронт через api.js, должен быть
// в сгенерированных wailsjs-биндингах. Отсутствие = вызов упадет в рантайме
// с [null]/«проблема с сетью». Запускается из build.bat после сборки:
//   node check-bindings.mjs
import { readFileSync } from 'node:fs';

const apiSrc = readFileSync(new URL('./src/api.js', import.meta.url), 'utf8');
const wailsSrc = readFileSync(new URL('./wailsjs/go/app/App.js', import.meta.url), 'utf8');

const used = new Set([...apiSrc.matchAll(/app\.([A-Z]\w+)\(/g)].map(m => m[1]));
const have = new Set([...wailsSrc.matchAll(/export function (\w+)\(/g)].map(m => m[1]));

const missing = [...used].filter(m => !have.has(m));
if (missing.length) {
  console.error('[check-bindings] FAIL — отсутствуют в биндингах:', missing.join(', '));
  console.error('[check-bindings] Перезапустите сборку БЕЗ -skipbindings или выполните: wails generate module');
  process.exit(1);
}
console.log('[check-bindings] OK —', used.size, 'биндингов покрыто');
