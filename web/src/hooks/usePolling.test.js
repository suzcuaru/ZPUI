import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { usePolling } from './usePolling';
import { setPowerSaving } from '../powerFlag';

// document.hidden в jsdom — геттер без сеттера: переопределяем через defineProperty.
const setHidden = (v) => {
  Object.defineProperty(document, 'hidden', { value: v, configurable: true });
};

describe('usePolling', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    setHidden(false);
    setPowerSaving(false);
  });

  it('calls fetchFn immediately on mount', async () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 5000));
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('calls fetchFn on each interval tick', async () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 1000));
    expect(fn).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1000);
    expect(fn).toHaveBeenCalledTimes(2);

    await vi.advanceTimersByTimeAsync(2000);
    expect(fn).toHaveBeenCalledTimes(4);
  });

  it('does not set an interval when intervalMs is 0', async () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 0));
    expect(fn).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(10000);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('stops polling after unmount', async () => {
    const fn = vi.fn();
    const { unmount } = renderHook(() => usePolling(fn, 1000));
    expect(fn).toHaveBeenCalledTimes(1);

    unmount();
    await vi.advanceTimersByTimeAsync(5000);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('uses the latest fetchFn after rerender', async () => {
    const fn1 = vi.fn();
    const fn2 = vi.fn();
    const { rerender } = renderHook(({ f }) => usePolling(f, 1000), {
      initialProps: { f: fn1 },
    });
    rerender({ f: fn2 });

    await vi.advanceTimersByTimeAsync(1000);
    expect(fn2).toHaveBeenCalled();
  });

  it('skips ticks while the previous call is still running (no queue pile-up)', async () => {
    let resolveFn;
    const fn = vi.fn(() => new Promise((res) => { resolveFn = res; }));
    const { unmount } = renderHook(() => usePolling(fn, 1000));
    expect(fn).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(3000);
    expect(fn).toHaveBeenCalledTimes(1);

    resolveFn();
    await vi.advanceTimersByTimeAsync(1000);
    expect(fn).toHaveBeenCalledTimes(2);
    unmount();
  });

  it('does not poll while hidden and catches up when visible again', async () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 1000));
    expect(fn).toHaveBeenCalledTimes(1);

    setHidden(true);
    await vi.advanceTimersByTimeAsync(5000);
    expect(fn).toHaveBeenCalledTimes(1);

    setHidden(false);
    document.dispatchEvent(new Event('visibilitychange'));
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('does not call fetchFn while disabled and loads immediately when enabled', async () => {
    const fn = vi.fn();
    const { rerender } = renderHook(({ on }) => usePolling(fn, 1000, [], on), {
      initialProps: { on: false },
    });
    await vi.advanceTimersByTimeAsync(5000);
    expect(fn).not.toHaveBeenCalled();

    rerender({ on: true });
    expect(fn).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1000);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('refetch forces a call even while a poll is in flight', async () => {
    let resolveFn;
    const fn = vi.fn(() => new Promise((res) => { resolveFn = res; }));
    const { result, unmount } = renderHook(() => usePolling(fn, 30000));
    expect(fn).toHaveBeenCalledTimes(1);

    result.current();
    expect(fn).toHaveBeenCalledTimes(2);

    resolveFn();
    await vi.advanceTimersByTimeAsync(0);
    unmount();
  });

  it('doubles intervals >= 10s while power saving is on', async () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 30000));
    expect(fn).toHaveBeenCalledTimes(1);

    // смена флага перезаряжает таймер; немедленный тик перезарядки допустим,
    // но не обязателен — поэтому базу фиксируем после того, как перезарядка
    // устоялась, и проверяем расписание относительно неё
    act(() => setPowerSaving(true));
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
    const base = fn.mock.calls.length;

    await vi.advanceTimersByTimeAsync(29000); // t=30000: старый интервал снят — тика нет
    expect(fn).toHaveBeenCalledTimes(base);

    await vi.advanceTimersByTimeAsync(30000); // t=60000: новый тик, а не на 30-й секунде
    expect(fn).toHaveBeenCalledTimes(base + 1);
  });

  it('keeps short intervals unchanged while power saving is on', async () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 5000));
    expect(fn).toHaveBeenCalledTimes(1);

    act(() => setPowerSaving(true));
    // интервал < 10с: таймер не перезаряжается, тик по прежнему расписанию
    expect(fn).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(5000);
    expect(fn).toHaveBeenCalledTimes(2);
  });
});
