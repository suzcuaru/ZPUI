// errors.js — единая гарантия «у каждой ошибки есть текст».
// Сервер присылает либо коды (snake_case: vpn_plan_forbidden, http_429),
// либо готовые человекочитаемые сообщения. Код → словарь errors.*,
// неизвестный код → «Ошибка (код: X). Подробности в журнале»,
// готовый текст → показывается как есть. Голое «Ошибка» невозможно.
const CODE_RE = /^[a-z0-9][a-z0-9_.-]{1,63}$/i;

export function errorText(t, code) {
  if (code === null || code === undefined || code === '') return t('errors.unknown');
  const str = String(code);
  if (!CODE_RE.test(str)) return str;
  const key = 'errors.' + str;
  const v = t(key);
  if (v !== key) return v;
  return t('errors.withCode', { code: str });
}
