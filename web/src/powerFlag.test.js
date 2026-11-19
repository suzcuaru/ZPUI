import { describe, it, expect, vi, afterEach } from 'vitest';
import { setPowerSaving, isPowerSaving, onPowerSavingChange } from './powerFlag';

describe('powerFlag', () => {
  afterEach(() => setPowerSaving(false));

  it('defaults to false and toggles', () => {
    expect(isPowerSaving()).toBe(false);
    setPowerSaving(true);
    expect(isPowerSaving()).toBe(true);
    setPowerSaving(false);
    expect(isPowerSaving()).toBe(false);
  });

  it('coerces value to boolean and skips no-op sets', () => {
    const fn = vi.fn();
    onPowerSavingChange(fn);
    setPowerSaving('yes');
    expect(isPowerSaving()).toBe(true);
    setPowerSaving(true);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('notifies subscribers and supports unsubscribe', () => {
    const fn = vi.fn();
    const off = onPowerSavingChange(fn);
    setPowerSaving(true);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenLastCalledWith(true);
    off();
    setPowerSaving(false);
    expect(fn).toHaveBeenCalledTimes(1);
  });
});
