import { useEffect, useRef } from 'react';
import { subscribeEvent } from '../events';

// Подписка на Wails-событие на время жизни компонента. Хендлер хранится в
// ref — всегда свежий, зависимостей-массивов не требует; отписка — при
// размонтировании (и при смене имени события).
export function useWailsEvent(name, handler) {
  const ref = useRef(handler);
  ref.current = handler;
  useEffect(() => subscribeEvent(name, (...args) => ref.current(...args)), [name]);
}
