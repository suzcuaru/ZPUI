import { describe, it, expect, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { usePowerInterval } from './usePowerInterval';
import { setPowerSaving } from '../powerFlag';

describe('usePowerInterval', () => {
  afterEach(() => setPowerSaving(false));

  it('returns intervals as-is while power saving is off', () => {
    const { result } = renderHook(() => usePowerInterval(30000));
    expect(result.current).toBe(30000);
  });

  it('doubles intervals >= 10s and keeps short ones when on', () => {
    const slow = renderHook(() => usePowerInterval(30000));
    const fast = renderHook(() => usePowerInterval(5000));
    const zero = renderHook(() => usePowerInterval(0));

    act(() => setPowerSaving(true));
    expect(slow.result.current).toBe(60000);
    expect(fast.result.current).toBe(5000);
    expect(zero.result.current).toBe(0);

    act(() => setPowerSaving(false));
    expect(slow.result.current).toBe(30000);
  });
});
