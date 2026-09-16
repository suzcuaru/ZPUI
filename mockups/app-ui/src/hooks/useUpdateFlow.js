import { useEffect, useState, useCallback } from 'react';
import { api } from '../api';
import { resetZapretCheck, checkZapretUpdate } from './useUpdateCheck';

// Глобальное состояние модалки обновления.
// Фазы: idle (закрыто) → downloading → downloaded → installing (zapret) /
// launching (ZPUI: закрываем приложение, запускаем установщик) → done | error.
// В фазах downloading/installing/launching модалку закрыть нельзя.
let _flow = {
  open: false,
  component: null, // 'ZPUI' | 'zapret'
  phase: 'idle',
  percent: 0,
  downloaded: 0,
  total: 0,
  step: '',
  source: '',
  version: null,
  error: null,
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
    error: null,
  });
  const route = component === 'ZPUI' ? '/api/updates/download' : '/api/updates/download-zapret';
  api('POST', route).then((d) => {
    if (!d) return;
    if (d.error) set({ phase: 'error', error: d.error });
    else if (d.status === 'no_update') set({ phase: 'done', version: d.version });
    else if (d.version) set({ version: d.version, ...(d.source ? { source: d.source } : {}) });
  });
}

// openUpdateModalForVersion — модалка скачивания конкретной версии запрета
// (ручная установка/откат). Прогресс приходит теми же событиями update:download.
export function openUpdateModalForVersion(version, source) {
  if (_flow.open && busyPhase(_flow.phase)) return;
  set({
    open: true,
    component: 'zapret',
    phase: 'downloading',
    percent: 0,
    downloaded: 0,
    total: 0,
    step: '',
    error: null,
    version,
    source: source || '',
  });
  api('POST', '/api/updates/download-zapret-version', { version, source: source || '' }).then((d) => {
    if (!d) return;
    if (d.error) set({ phase: 'error', error: d.error });
    else if (d.version) set({ version: d.version });
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
  const rt = window.runtime;
  rt.EventsOn('update:download', (d) => {
    if (!d?.component) return;
    set({
      open: true,
      component: d.component,
      phase: 'downloading',
      percent: d.percent || 0,
      downloaded: d.downloaded || 0,
      total: d.total || 0,
    });
  });
  rt.EventsOn('update:download-done', (d) => {
    if (!d?.component) return;
    if (d.ok) {
      set({ phase: 'downloaded', percent: 100, version: d.version || _flow.version, source: d.source || _flow.source });
    } else {
      set({ phase: 'error', error: d.error || '' });
    }
  });
  rt.EventsOn('update:install', (d) => {
    if (!d) return;
    set({ open: true, component: d.component || 'zapret', phase: 'installing', step: d.step || '', percent: d.percent || 0 });
  });
  rt.EventsOn('update:install-done', (d) => {
    if (!d) return;
    if (d.ok) {
      set({ phase: 'done', percent: 100, version: d.version || _flow.version });
      // После установки запрета сбрасываем закэшированный «доступно обновление»,
      // иначе кнопка «Скачать» остаётся до перезапуска приложения.
      if (d.component === 'zapret') {
        resetZapretCheck();
        checkZapretUpdate().catch(() => {});
        window.dispatchEvent(new CustomEvent('zpui:zapret-installed'));
      }
    } else {
      set({ phase: 'error', error: d.error || '' });
    }
  });
  rt.EventsOn('update:open-modal', (d) => {
    openUpdateModal(d?.component || 'ZPUI');
  });
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
