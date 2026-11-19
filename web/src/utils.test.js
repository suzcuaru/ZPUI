import { describe, it, expect } from 'vitest';
import {
  formatBytes, formatSpeed, strategyDisplayName,
  deepDiff, deepMergePatch, isNetError,
  dnsActiveState, isValidDnsAddress,
  bytesToMbps, formatRateAuto, formatMetricValue,
  dnsUiState, dnsHealthOf, formatDateSafe, isUnlimitedDate,
  tgDomainFromUrl, openTelegram, isValidDomain,
} from './utils';

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

describe('deepDiff', () => {
  it('returns only changed top-level keys', () => {
    expect(deepDiff({ a: 1, b: 2 }, { a: 1, b: 3 })).toEqual({ b: 3 });
  });

  it('returns whole value for arrays and primitives', () => {
    expect(deepDiff({ list: [1, 2], n: 1 }, { list: [1, 2, 3], n: 1 })).toEqual({ list: [1, 2, 3] });
  });

  it('recurses into nested objects and keeps unchanged branches out', () => {
    const a = { server: { telemetry_enabled: true, url: 'x' }, theme: 'dark' };
    const b = { server: { telemetry_enabled: false, url: 'x' }, theme: 'dark' };
    expect(deepDiff(a, b)).toEqual({ server: { telemetry_enabled: false } });
  });

  it('returns empty object when nothing changed', () => {
    expect(deepDiff({ a: { b: 1 } }, { a: { b: 1 } })).toEqual({});
  });

  it('includes keys missing from the base', () => {
    expect(deepDiff({ a: 1 }, { a: 1, b: 2 })).toEqual({ b: 2 });
  });

  it('returns b as-is when shapes are not plain objects', () => {
    expect(deepDiff(null, { a: 1 })).toEqual({ a: 1 });
    expect(deepDiff({ a: 1 }, [1, 2])).toEqual([1, 2]);
  });
});

describe('deepMergePatch', () => {
  it('merges nested objects without dropping siblings', () => {
    const base = { sounds: { enabled: true, volume: 60 }, theme: 'dark' };
    expect(deepMergePatch(base, { sounds: { volume: 30 } })).toEqual({
      sounds: { enabled: true, volume: 30 },
      theme: 'dark',
    });
  });

  it('replaces arrays and scalars wholesale', () => {
    expect(deepMergePatch({ list: [1], n: 1 }, { list: [2, 3], n: 5 })).toEqual({ list: [2, 3], n: 5 });
  });

  it('returns patch as-is when base is not an object', () => {
    expect(deepMergePatch(null, { a: 1 })).toEqual({ a: 1 });
  });
});

describe('isNetError', () => {
  it('treats null/undefined as network error', () => {
    expect(isNetError(null)).toBe(true);
    expect(isNetError(undefined)).toBe(true);
  });

  it('detects known network error codes', () => {
    expect(isNetError({ error: 'server_unreachable' })).toBe(true);
    expect(isNetError({ error: 'timeout' })).toBe(true);
  });

  it('does not flag application error codes', () => {
    expect(isNetError({ error: 'login_taken' })).toBe(false);
    expect(isNetError({ status: 'ok' })).toBe(false);
  });
});

describe('dnsActiveState', () => {
  const user = [{ addr: '10.0.0.1', enabled: true }, { addr: '10.0.0.2', enabled: false }];

  it('off when system has no custom DNS', () => {
    expect(dnsActiveState({ current: ['8.8.8.8'], servers: user, xboxEnabled: true })).toBe('off');
    expect(dnsActiveState({ current: [], servers: user, xboxEnabled: true })).toBe('off');
  });

  it('user when enabled user server is applied', () => {
    expect(dnsActiveState({ current: ['10.0.0.1'], servers: user, xboxEnabled: false })).toBe('user');
  });

  it('disabled user servers do not count', () => {
    expect(dnsActiveState({ current: ['10.0.0.2'], servers: user, xboxEnabled: false })).toBe('off');
  });

  it('xbox when only the preset is applied and enabled', () => {
    expect(dnsActiveState({ current: ['111.88.96.54'], servers: [], xboxEnabled: true })).toBe('xbox');
    expect(dnsActiveState({ current: ['111.88.96.55'], servers: [], xboxEnabled: false })).toBe('off');
  });

  it('mixed when both user and xbox are applied', () => {
    expect(dnsActiveState({ current: ['10.0.0.1', '111.88.96.54'], servers: user, xboxEnabled: true })).toBe('mixed');
  });

  it('is case/whitespace tolerant', () => {
    expect(dnsActiveState({ current: [' 10.0.0.1 '], servers: [{ addr: '10.0.0.1', enabled: true }] })).toBe('user');
  });
});

describe('isValidDnsAddress', () => {
  it('accepts IPv4', () => {
    expect(isValidDnsAddress('111.88.96.54')).toBe(true);
    expect(isValidDnsAddress('8.8.8.8')).toBe(true);
  });

  it('accepts IPv6 and DoH URLs and hostnames', () => {
    expect(isValidDnsAddress('2606:4700:4700::1111')).toBe(true);
    expect(isValidDnsAddress('https://dns.example.com/dns-query')).toBe(true);
    expect(isValidDnsAddress('dns.example.com')).toBe(true);
  });

  it('rejects garbage', () => {
    expect(isValidDnsAddress('')).toBe(false);
    expect(isValidDnsAddress('999.1.1.1')).toBe(false);
    expect(isValidDnsAddress('not a dns')).toBe(false);
    expect(isValidDnsAddress('http://insecure.example.com')).toBe(false);
  });
});

describe('isValidDomain', () => {
  it('accepts plain domains and subdomains', () => {
    expect(isValidDomain('example.com')).toBe(true);
    expect(isValidDomain('cdn.example.com')).toBe(true);
    expect(isValidDomain('EXAMPLE.COM')).toBe(true);
  });

  it('accepts ^ prefix (точное совпадение hostlist)', () => {
    expect(isValidDomain('^example.com')).toBe(true);
  });

  it('rejects garbage', () => {
    expect(isValidDomain('')).toBe(false);
    expect(isValidDomain('example')).toBe(false);
    expect(isValidDomain('not a domain')).toBe(false);
    expect(isValidDomain('-bad.example.com')).toBe(false);
    expect(isValidDomain('example.com/path')).toBe(false);
  });
});

describe('graph units', () => {
  it('bytesToMbps converts bytes/s ×8/1e6', () => {
    expect(bytesToMbps(0)).toBe(0);
    expect(bytesToMbps(125000)).toBeCloseTo(1, 6);
    expect(bytesToMbps(12500000)).toBeCloseTo(100, 6);
  });

  it('formatRateAuto picks KB/s / MB/s / GB/s', () => {
    expect(formatRateAuto(500)).toBe('500 B/s');
    expect(formatRateAuto(1024)).toBe('1.0 KB/s');
    expect(formatRateAuto(1048576)).toBe('1.0 MB/s');
    expect(formatRateAuto(2 * 1024 * 1024 * 1024)).toBe('2.00 GB/s');
  });

  it('formatMetricValue formats fixed metrics', () => {
    expect(formatMetricValue(87.64, 'mbps')).toBe('87.6');
    expect(formatMetricValue(42.4, 'ms')).toBe('42');
    expect(formatMetricValue(2048, 'rate')).toBe('2.0 KB/s');
  });
});

describe('dnsUiState (единый источник: status.xbox_dns)', () => {
  it('enabled=false → off, 0 активных', () => {
    expect(dnsUiState({ enabled: false, servers: ['1.1.1.1'] })).toEqual({ on: false, state: 'off', activeCount: 0 });
    expect(dnsUiState(undefined, { current: ['8.8.8.8'], servers: [] })).toEqual({ on: false, state: 'off', activeCount: 0 });
  });

  it('enabled=true: applied/active_count → user; без факта системы → off', () => {
    expect(dnsUiState({ enabled: true, applied: true, active_count: 3 }))
      .toEqual({ on: true, state: 'user', activeCount: 3 });
    expect(dnsUiState({ enabled: true, system_active: true, servers: ['1.1.1.1', '1.0.0.1'] }))
      .toEqual({ on: true, state: 'user', activeCount: 2 });
    expect(dnsUiState({ enabled: true })).toEqual({ on: false, state: 'off', activeCount: 0 });
  });

  it('нет полей статуса → фолбэк на dnsActiveState', () => {
    expect(dnsUiState(null, { current: ['10.0.0.1'], servers: [{ addr: '10.0.0.1', enabled: true }] }))
      .toEqual({ on: true, state: 'user', activeCount: 0 });
    expect(dnsUiState(null, { current: [] })).toEqual({ on: false, state: 'off', activeCount: 0 });
  });
});

describe('dnsHealthOf (per-серверная доступность)', () => {
  it('понимает health-строку и булев available', () => {
    expect(dnsHealthOf({ health: 'ok' })).toBe('ok');
    expect(dnsHealthOf({ health: 'FAIL' })).toBe('fail');
    expect(dnsHealthOf({ health: 'checking' })).toBe('checking');
    expect(dnsHealthOf({ available: true })).toBe('ok');
    expect(dnsHealthOf({ available: false })).toBe('fail');
    expect(dnsHealthOf({})).toBe('unknown');
    expect(dnsHealthOf(null)).toBe('unknown');
  });
});

describe('formatDateSafe / isUnlimitedDate', () => {
  it('NULL/0001-01-01 → «Бессрочно» при unlimited', () => {
    for (const v of [null, undefined, '0001-01-01T00:00:00Z']) {
      expect(isUnlimitedDate(v)).toBe(true);
      expect(formatDateSafe(v, { unlimited: true })).toBe('common.unlimited');
    }
  });

  it('0/пустая строка — не бессрочность: «—»', () => {
    for (const v of [0, '']) {
      expect(isUnlimitedDate(v)).toBe(false);
      expect(formatDateSafe(v)).toBe('—');
    }
  });

  it('без unlimited-контекста отсутствующее → «—»', () => {
    for (const v of [null, undefined]) {
      expect(formatDateSafe(v)).toBe('—');
      expect(isUnlimitedDate(v)).toBe(true);
    }
  });

  it('мусор → «—», валидное → локализованная дата', () => {
    expect(formatDateSafe('not a date')).toBe('—');
    expect(formatDateSafe(Number.NaN)).toBe('—');
    expect(formatDateSafe('2026-09-26', { lang: 'en' })).toMatch(/2026/);
    expect(formatDateSafe(1789000000, { seconds: true, lang: 'en' })).toMatch(/\d/);
  });
});

describe('telegram helper', () => {
  it('tgDomainFromUrl извлекает домен из t.me-ссылок и голых доменов', () => {
    expect(tgDomainFromUrl('https://t.me/zpui_app')).toBe('zpui_app');
    expect(tgDomainFromUrl('https://t.me/zpui_chat?s=1')).toBe('zpui_chat');
    expect(tgDomainFromUrl('http://telegram.me/s/zpui_app')).toBe('zpui_app');
    expect(tgDomainFromUrl('zpui_app')).toBe('zpui_app');
    expect(tgDomainFromUrl('https://example.com/x')).toBe('');
    expect(tgDomainFromUrl('')).toBe('');
  });

  it('openTelegram открывает приложение (tg://resolve) через системный обработчик', () => {
    const opened = [];
    window.runtime = { BrowserOpenURL: (u) => { opened.push(u); } };
    openTelegram('https://t.me/zpui_app');
    openTelegram('zpui_chat');
    openTelegram('tg://proxy?server=x&port=443&secret=aa');
    expect(opened).toEqual([
      'tg://resolve?domain=zpui_app',
      'tg://resolve?domain=zpui_chat',
      'tg://proxy?server=x&port=443&secret=aa',
    ]);
    delete window.runtime;
  });

  it('без runtime — фолбэк открывает веб-версию по таймауту', () => {
    const opened = [];
    const originalOpen = window.open;
    window.open = (u) => { opened.push(u); return null; };
    vi.useFakeTimers();
    openTelegram('https://t.me/zpui_app');
    vi.advanceTimersByTime(1600);
    expect(opened).toEqual(['https://t.me/zpui_app']);
    vi.useRealTimers();
    window.open = originalOpen;
  });
});
