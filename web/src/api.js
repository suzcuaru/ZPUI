export async function api(method, path, body) {
  const opts = { method, headers: { 'Content-Type': 'application/json' } };
  if (body) opts.body = JSON.stringify(body);
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 10000);
    opts.signal = controller.signal;
    const res = await fetch(path, opts);
    clearTimeout(timeout);
    return await res.json();
  } catch {
    return null;
  }
}

export async function apiCall(fn, successMsg, showToast) {
  try {
    const result = await fn();
    if (result?.error) {
      if (showToast) showToast(result.error, 'error');
      return false;
    }
    if (successMsg && showToast) showToast(successMsg, 'success');
    return true;
  } catch {
    if (showToast) showToast('Ошибка выполнения запроса', 'error');
    return false;
  }
}

export async function openExternal(url) {
  try {
    await fetch('/api/external', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    });
  } catch {}
}
