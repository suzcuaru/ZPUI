import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { I18nProvider } from './i18n';
import { ToastProvider } from './context/ToastContext';
import StrategiesTab from './components/zapret/StrategiesTab';

// Регресс-тест ПКМ-проверки стратегии (репорт владельца «требует два нажатия»,
// 20–22.11.2026): контекст-меню обязано открываться с первого правого клика,
// первый клик по «Проверка» — открывать модалку и звать биндинг сразу,
// в том числе при мгновенном клике по пункту и на повторном прогоне.

function bindings(map) {
  global.window.go = { app: { App: new Proxy(map, { get(t, p) { if (p in t) return t[p]; return async () => null; } }) } };
  global.window.runtime = { EventsOn() {}, EventsOff() {}, EventsEmit() {} };
}

if (typeof global.ResizeObserver === 'undefined') {
  global.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
}

const STRATS = () => ({
  strategies: [
    { name: 'AAA', filename: 'aaa.bat', current: true, score: 80, grade: 'A' },
    { name: 'BBB', filename: 'bbb.bat', current: false, score: 60, grade: 'B' },
  ],
});

function mountTab() {
  return render(
    <I18nProvider>
      <ToastProvider value={() => {}}>
        <StrategiesTab status={{ zapret: { status: 'running' } }} zRun={true} />
      </ToastProvider>
    </I18nProvider>
  );
}

async function openMenuAndTest(screen) {
  const card = screen.getByText('BBB').closest('.strat-card');
  fireEvent.contextMenu(card, { clientX: 100, clientY: 100, bubbles: true, cancelable: true });
  const menu = document.querySelector('.strat-ctx-menu');
  const items = [...menu.querySelectorAll('.strat-ctx-item')];
  fireEvent.mouseDown(items[0], { bubbles: true });
  fireEvent.click(items[0]);
}

describe('ПКМ-цепочка стратегии: меню → «Проверка» → модалка → POST', () => {
  let calls;

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    calls = [];
    bindings({
      GetStrategies: async () => STRATS(),
      GetAutoTestResults: async () => ({ results: [{ strategy: 'aaa.bat', pct: 80, ok_count: 8, total: 10 }] }),
      GetConfig: async () => ({}),
      GetSingleTestResult: async () => ({ running: false, result: null }),
      TestSingleStrategy: async (filename, game) => { calls.push(['test', filename, game]); return { status: 'ok' }; },
      CancelAutoTest: async () => ({ status: 'cancelled' }),
    });
  });

  it('первый ПКМ открывает меню, первый клик по «Проверка» зовёт биндинг и открывает модалку', async () => {
    mountTab();
    await waitFor(() => expect(screen.getByText('AAA')).toBeTruthy());

    await openMenuAndTest(screen);

    await waitFor(() => expect(calls.length, 'биндинг вызван с первого клика').toBe(1));
    expect(calls[0]).toEqual(['test', 'bbb.bat', false]);
    await waitFor(() => expect(document.querySelector('.modal')).toBeTruthy());
  });

  it('повторный прогон после отмены первого тоже срабатывает с первого клика', async () => {
    mountTab();
    await waitFor(() => expect(screen.getByText('AAA')).toBeTruthy());

    for (let round = 1; round <= 2; round++) {
      await openMenuAndTest(screen);
      await waitFor(() => expect(calls.length).toBe(round));
      await waitFor(() => expect(document.querySelector('.modal')).toBeTruthy());
      // отменить прогон (модалка во время running не закрывается крестиком)
      const cancelBtn = document.querySelector('.asm2-cancel');
      fireEvent.click(cancelBtn);
      await waitFor(() => expect(document.querySelector('.modal-close')).toBeTruthy());
      fireEvent.click(document.querySelector('.modal-close'));
      await waitFor(() => expect(document.querySelector('.modal')).toBeFalsy());
    }
  });

  it('мгновенный клик по пункту (в пределах анимации входа меню) не теряется', async () => {
    mountTab();
    await waitFor(() => expect(screen.getByText('AAA')).toBeTruthy());

    await openMenuAndTest(screen);
    await waitFor(() => expect(calls.length).toBe(1));
  });
});
