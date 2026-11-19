// Шина подписок на Wails-события. EventsOff снимает ВСЕ слушатели имени,
// поэтому на каждое событие держится одна нативная подписка, а отписка
// вызывается только когда ушёл последний слушатель.
const refs = new Map();
const attached = new Set();

export function subscribeEvent(name, handler) {
  if (typeof window === 'undefined' || !window.runtime?.EventsOn) return () => {};
  let set = refs.get(name);
  if (!set) {
    set = new Set();
    refs.set(name, set);
  }
  set.add(handler);
  if (!attached.has(name)) {
    attached.add(name);
    window.runtime.EventsOn(name, (...args) => {
      const cur = refs.get(name);
      if (!cur) return;
      for (const h of [...cur]) {
        try { h(...args); } catch (e) { console.error('[events] ' + name, e); }
      }
    });
  }
  return () => {
    const cur = refs.get(name);
    if (!cur || !cur.has(handler)) return;
    cur.delete(handler);
    if (cur.size === 0 && attached.has(name)) {
      attached.delete(name);
      refs.delete(name);
      try { window.runtime?.EventsOff?.(name); } catch {}
    }
  };
}
