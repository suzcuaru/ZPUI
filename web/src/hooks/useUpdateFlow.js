import { useEffect, useState, useCallback } from 'react';
import { api } from '../api';
import { resetZapretCheck, checkZapretUpdate } from './useUpdateCheck';
import { subscribeEvent } from '../events';

// Глобальное состояние модалки обновления.
// Фазы: idle (закрыто) → downloading → downloaded → installing (zapret) /
// launching (ZPUI: закрываем приложение, запускаем установщик) → done | error.
// В фазах downloading/installing/launching модалку закрыть нельзя.
// Плагин браузера (component "extension") идёт без фазы downloaded: скачали —
// сразу ставим, итог приходит теми же событиями update:install*.
let _flow = {
  open: false,
  component: null, // 'ZPUI' | 'zapret' | 'extension'
  phase: 'idle',
  percent: 0,
  downloaded: 0,
  total: 0,
  step: '',
  source: '',
  version: null,
  noUpdate: false,
  error: null,
  browsers: [], // extension: где поставили (имена)
};
const subs = new Set();
const notify = () => subs.forEach((fn) => fn());

function set(patch) {
  _flow = { ..._flow, ...patch };
  notify();
}

const busyPhase = (phase) => phase === 'downloading' || phase === 'installing' || phase === 'launching';

// openUpdateModal открывает модалку и сразу запускает скачивание компонента.
export function openUpdateModal(component) {
  if (component === 'extension') {
    startExtensionUpdate(lastExtIds);
    return;
  }
  if (_flow.open && busyPhase(_flow.phase)) return;
  set({
    open: true,
    component,
    phase: 'downloading',
    percent: 0,
    downloaded: 0,
    total: 0,
    step: '',
    source: '',
    version: null,
    noUpdate: false,
    error: null,
    browsers: [],
  });
  const route = component === 'ZPUI' ? '/api/updates/download' : '/api/updates/download-zapret';
  api('POST', route).then((d) => {
    if (!d) return;
    if (d.error) set({ phase: 'error', error: d.error, noUpdate: false });
    else if (d.status === 'no_update') set({ phase: 'done', noUpdate: true });
    else if (d.version) set({ version: d.version, ...(d.source ? { source: d.source } : {}) });
  });
}

// lastExtIds — браузеры, выбранные пользователем в модалке плагина;
// «Повторить» после ошибки идёт с тем же списком.
let lastExtIds = [];

// startExtensionUpdate — скачивание и установка плагина браузера в фоне
// (POST /api/extension/update); прогресс и итог — событиями update:*.
export function startExtensionUpdate(ids) {
  if (_flow.open && busyPhase(_flow.phase)) return;
  lastExtIds = Array.isArray(ids) ? ids : [];
  set({
    open: true,
    component: 'extension',
    phase: 'downloading',
    percent: 0,
    downloaded: 0,
    total: 0,
    step: '',
    source: '',
    version: null,
    noUpdate: false,
    error: null,
    browsers: [],
  });
  api('POST', '/api/extension/update', { ids: lastExtIds }).then((d) => {
    if (d && d.error) set({ phase: 'error', error: d.error, noUpdate: false });
  });
}

// showUpdateModal показывает модалку с текущим состоянием (без запуска скачивания).
export function showUpdateModal() {
  if (_flow.phase === 'idle') return;
  set({ open: true });
}

export function closeUpdateModal() {
  if (busyPhase(_flow.phase)) return;
  set({ open: false, phase: 'idle' });
}

// applyDownloadedUpdate — кнопка «Установить».
// ZPUI: бэкенд ждёт выхода приложения и запускает установщик.
// zapret: установка идёт в приложении, прогресс придёт событиями update:install.
export function applyDownloadedUpdate() {
  const { component } = _flow;
  if (component === 'ZPUI') {
    api('POST', '/api/updates/apply').then((d) => {
      if (d?.error) {
        set({ phase: 'error', error: d.error });
        return;
      }
      set({ phase: 'launching' });
    });
  } else {
    set({ phase: 'installing', percent: 0, step: '' });
    api('POST', '/api/updates/apply-zapret').then((d) => {
      if (d?.error) set({ phase: 'downloaded', error: null });
    });
  }
}

export function retryDownload() {
  openUpdateModal(_flow.component);
}

let bound = false;
// initUpdateEvents подписывается на события обновлений от бэкенда
// (в т.ч. update:open-modal из трея). Вызывается один раз из App.
export function initUpdateEvents() {
  if (bound || typeof window === 'undefined' || !window.runtime?.EventsOn) return;
  bound = true;
  subscribeEvent('update:download', (d) => {
    if (!d?.component) return;
    set({
      open: true,
      component: d.component,
      phase: 'downloading',
      percent: d.percent || 0,
      downloaded: d.downloaded || 0,
      total: d.total || 0,
      noUpdate: false,
    });
  });
  subscribeEvent('update:download-done', (d) => {
    if (!d?.component) return;
    if (d.ok) {
      // Плагин ставится сразу после скачивания — кнопки «Установить» у него нет.
      const phase = d.component === 'extension' ? 'installing' : 'downloaded';
      set({ phase, percent: d.component === 'extension' ? 0 : 100, version: d.version || _flow.version, source: d.source || _flow.source, noUpdate: false });
    } else {
      set({ phase: 'error', error: d.error || '', noUpdate: false });
    }
  });
  subscribeEvent('update:install', (d) => {
    if (!d) return;
    set({ open: true, component: d.component || 'zapret', phase: 'installing', step: d.step || '', percent: d.percent || 0, noUpdate: false });
  });
  subscribeEvent('update:install-done', (d) => {
    if (!d) return;
    if (d.ok) {
      set({ phase: 'done', percent: 100, version: d.version || _flow.version, noUpdate: false, browsers: d.browsers || [] });
      // После установки запрета сбрасываем закэшированный «доступно обновление»,
      // иначе кнопка «Скачать» остаётся до перезапуска приложения.
      if (d.component === 'zapret') {
        resetZapretCheck();
        checkZapretUpdate().catch(() => {});
        window.dispatchEvent(new CustomEvent('zpui:zapret-installed'));
      }
      // Плагин браузера: модалка плагина и карточка в настройках перечитывают инфо.
      if (d.component === 'extension') {
        window.dispatchEvent(new CustomEvent('zpui:extension-installed'));
      }
    } else {
      set({ phase: 'error', error: d.error || '', noUpdate: false });
    }
  });
  subscribeEvent('update:open-modal', (d) => {
    openUpdateModal(d?.component || 'ZPUI');
  });
}

// resetUpdateFlowForTests — сброс глобального состояния (только тесты:
// события бэкенда в тестах не приходят, модалка осталась бы «в полёте»).
export function resetUpdateFlowForTests() {
  _flow = { ..._flow, open: false, phase: 'idle', component: null };
  lastExtIds = [];
}

export function useUpdateFlow() {
  const [, setTick] = useState(0);
  const rerender = useCallback(() => setTick((n) => n + 1), []);

  useEffect(() => {
    subs.add(rerender);
    return () => subs.delete(rerender);
  }, [rerender]);

  return _flow;
}
