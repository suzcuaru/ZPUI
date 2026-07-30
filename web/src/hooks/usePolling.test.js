import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { usePolling } from './usePolling';

describe('usePolling', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('calls fetchFn immediately on mount', async () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 5000));
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('calls fetchFn on each interval tick', () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 1000));
    expect(fn).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(1000);
    expect(fn).toHaveBeenCalledTimes(2);

    vi.advanceTimersByTime(2000);
    expect(fn).toHaveBeenCalledTimes(4);
  });

  it('does not set an interval when intervalMs is 0', () => {
    const fn = vi.fn();
    renderHook(() => usePolling(fn, 0));
    expect(fn).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(10000);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('stops polling after unmount', () => {
    const fn = vi.fn();
    const { unmount } = renderHook(() => usePolling(fn, 1000));
    expect(fn).toHaveBeenCalledTimes(1);

    unmount();
    vi.advanceTimersByTime(5000);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('uses the latest fetchFn after rerender', () => {
    const fn1 = vi.fn();
    const fn2 = vi.fn();
    const { rerender } = renderHook(({ f }) => usePolling(f, 1000), {
      initialProps: { f: fn1 },
    });
    rerender({ f: fn2 });

    vi.advanceTimersByTime(1000);
    expect(fn2).toHaveBeenCalled();
  });
});
