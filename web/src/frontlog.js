import { api } from './api';

// frontlog — фронтенд-лог приложения (журнал «Диагностики», POST
// /api/logs/frontend → FrontendLogs). Ошибки auth/vpn-потоков и глобальные
// сбои окна пишутся батчем (500мс / 50 событий), никогда не бросают и не
// блокируют UI. Форма события под биндинг Go: {action, category, details}.
let queue = [];
let timer = null;

function flush() {
  timer = null;
  if (!queue.length) return;
  const events = queue;
  queue = [];
  try {
    Promise.resolve(api('POST', '/api/logs/frontend', { events })).catch(() => {});
  } catch {}
}

export function frontlog(action, details) {
  queue.push({
    action: String(action || 'event'),
    category: 'frontend',
    details: { level: 'error', ts: Date.now(), ...(details || {}) },
  });
  if (queue.length >= 50) {
    flush();
    return;
  }
  if (!timer) timer = setTimeout(flush, 500);
}

// Ошибка пользовательского потока перед показом: код + экран/действие.
export function logError(flow, code, screen) {
  frontlog(flow, { code: String(code || 'unknown'), screen: screen || '' });
}

// Ошибка, показанная тостом: пишем в журнал (logdb → журнал и отчёт) и в
// очередь на сервер через /api/logs/ui-error. Молча: сбой логирования не
// должен порождать новые ошибки; биндинга нет (старый бэк/devmock) — пропускаем.
export function logUiError(msg, opts) {
  try {
    const code = (opts && opts.code) || 'ui_error';
    Promise.resolve(api('POST', '/api/logs/ui-error', { code, msg: String(msg || '') })).catch(() => {});
  } catch {}
}
