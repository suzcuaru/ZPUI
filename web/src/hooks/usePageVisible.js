import { createContext, useContext, createElement } from 'react';

// Видимость страницы при keep-mounted вкладках: App оборачивает каждую
// посещённую страницу в провайдер; неактивная (display:none) получает false —
// usePolling/useNow не тикают, модалки страницы не рисуются. Снаружи страниц
// (аппарат App, drawer-ы, стартовый экран) — true по умолчанию.
// Спека: Документы/Задачи/2026-11-03-render-responsiveness.md (R5, R6).
const Ctx = createContext(true);

export function PageVisibleProvider({ value, children }) {
  return createElement(Ctx.Provider, { value }, children);
}

export function usePageVisible() {
  return useContext(Ctx);
}
