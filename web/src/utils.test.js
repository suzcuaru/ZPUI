import { describe, it, expect } from 'vitest';
import { formatBytes, formatSpeed, strategyDisplayName } from './utils';

describe('formatBytes', () => {
  it('formats zero bytes', () => {
    expect(formatBytes(0)).toBe('0 B');
  });

  it('formats bytes below KB as-is', () => {
    expect(formatBytes(512)).toBe('512 B');
  });

  it('formats KB / MB / GB / TB', () => {
    expect(formatBytes(1024)).toBe('1 KB');
    expect(formatBytes(1048576)).toBe('1 MB');
    expect(formatBytes(1073741824)).toBe('1 GB');
    expect(formatBytes(1099511627776)).toBe('1 TB');
  });

  it('rounds to two decimals', () => {
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(2621440)).toBe('2.5 MB');
  });
});

describe('formatSpeed', () => {
  it('formats below 1 KB as B/s with no decimals', () => {
    expect(formatSpeed(500)).toBe('500 B/s');
  });

  it('formats KB/s and MB/s with one decimal', () => {
    expect(formatSpeed(1024)).toBe('1.0 KB/s');
    expect(formatSpeed(1048576)).toBe('1.0 MB/s');
  });
});

describe('strategyDisplayName', () => {
  it('returns dash for empty input', () => {
    expect(strategyDisplayName('')).toBe('—');
    expect(strategyDisplayName(null)).toBe('—');
    expect(strategyDisplayName(undefined)).toBe('—');
  });

  it('strips .bat suffix (case-insensitive)', () => {
    expect(strategyDisplayName('general (ALT).bat')).toBe('general (ALT)');
    expect(strategyDisplayName('general.BAT')).toBe('general');
  });

  it('normalises pure number to general<N>', () => {
    expect(strategyDisplayName('123')).toBe('general123');
  });

  it('keeps general<N> as-is', () => {
    expect(strategyDisplayName('general1')).toBe('general1');
  });

  it('keeps plain general', () => {
    expect(strategyDisplayName('general')).toBe('general');
    expect(strategyDisplayName('general.bat')).toBe('general');
  });
});
