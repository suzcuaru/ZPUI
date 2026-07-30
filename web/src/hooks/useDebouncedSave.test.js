import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

vi.mock('../api', () => ({
  api: vi.fn(),
  apiCall: vi.fn(async (fn) => { await fn(); return true; }),
}));

import { useDebouncedSave } from './useDebouncedSave';
import { api } from '../api';

describe('useDebouncedSave', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });
  afterEach(() => vi.useRealTimers());

  it('does not save immediately — waits for delay', () => {
    const { result } = renderHook(() => useDebouncedSave('/api/c', 500));
    act(() => { result.current({ a: 1 }); });
    expect(api).not.toHaveBeenCalled();
  });

  it('saves merged config (currentConfig + patch) after delay', async () => {
    const { result } = renderHook(() => useDebouncedSave('/api/proxy/config', 500));
    act(() => { result.current({ port: 1080 }, { port: 0, username: 'a' }); });

    await act(async () => { await vi.advanceTimersByTimeAsync(500); });

    expect(api).toHaveBeenCalledTimes(1);
    expect(api).toHaveBeenCalledWith('POST', '/api/proxy/config', { port: 1080, username: 'a' });
  });

  it('debounces rapid updates — single save with last value', async () => {
    const { result } = renderHook(() => useDebouncedSave('/api/c', 500));
    act(() => { result.current({ a: 1 }); });
    act(() => { result.current({ a: 2 }); });
    act(() => { result.current({ a: 3 }); });

    await act(async () => { await vi.advanceTimersByTimeAsync(500); });

    expect(api).toHaveBeenCalledTimes(1);
    expect(api).toHaveBeenCalledWith('POST', '/api/c', { a: 3 });
  });

  it('calls onSuccess after a successful save', async () => {
    const onSuccess = vi.fn();
    const { result } = renderHook(() => useDebouncedSave('/api/c', 500, onSuccess));
    act(() => { result.current({ x: 1 }); });

    await act(async () => { await vi.advanceTimersByTimeAsync(500); });

    expect(onSuccess).toHaveBeenCalledTimes(1);
  });
});
