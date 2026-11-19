// Видимость окна приложения: Wails прячет окно (трей/close-to-tray/--hidden)
// без смены document.hidden — WebView2 продолжает рисовать кадры и тикать,
// и невидимый дашборд грузит GPU. Модуль держит явный флаг «окно скрыто»:
// его выставляют события window:shown/window:hidden от бэка и стартовый
// запрос /api/app/is-shown; на body вешается класс win-hidden (CSS ставит
// паузу всем анимациям), хуки usePolling/useNow спрашивают isInteractive().
let shown = true;
const subs = new Set();

export function setWindowShown(v) {
  v = !!v;
  if (v === shown) return;
  shown = v;
  try { document.body.classList.toggle('win-hidden', !v); } catch {}
  subs.forEach((fn) => { try { fn(v); } catch {} });
}

export function isWindowShown() {
  return shown;
}

// Интерактивность = окно видно И вкладка браузера активна (минимизация
// окна триггерит visibilitychange — покрываем обоими флагами).
export function isInteractive() {
  return shown && !document.hidden;
}

export function onWindowShownChange(fn) {
  subs.add(fn);
  return () => subs.delete(fn);
}
