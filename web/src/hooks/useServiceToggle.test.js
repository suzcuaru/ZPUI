import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

vi.mock('../api', () => ({
  api: vi.fn(),
  apiCall: vi.fn(async (fn) => { await fn(); return true; }),
}));

import { useServiceToggle } from './useServiceToggle';
import { api, apiCall } from '../api';

function resetApi() {
  api.mockImplementation(async () => ({}));
  apiCall.mockImplementation(async (fn) => { await fn(); return true; });
}

describe('useServiceToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetApi();
  });

  it('starts zapret when not running', async () => {
    const toast = vi.fn();
    const { result } = renderHook(() =>
      useServiceToggle('zapret', false, toast, { startMsg: 'started', stopMsg: 'stopped' })
    );

    expect(result.current.loading).toBe(false);
    await act(async () => { await result.current.toggle(); });
    expect(result.current.loading).toBe(false);

    expect(api).toHaveBeenCalledWith('POST', '/api/zapret/start');
    expect(api).toHaveBeenCalledWith('POST', '/api/component-states');
    expect(toast).toHaveBeenCalledWith('started', 'success');
  });

  it('stops proxy when running', async () => {
    const toast = vi.fn();
    const { result } = renderHook(() =>
      useServiceToggle('proxy', true, toast, { startMsg: 's', stopMsg: 'stopped' })
    );

    await act(async () => { await result.current.toggle(); });

    expect(api).toHaveBeenCalledWith('POST', '/api/proxy/stop');
    expect(toast).toHaveBeenCalledWith('stopped', 'success');
  });

  it('toggles xboxdns via config (enabled inverted)', async () => {
    api.mockImplementation(async (method, path) => {
      if (method === 'GET' && path === '/api/xbox-dns/config') {
        return { enabled: false, primary_dns: '1.1.1.1', secondary_dns: '8.8.8.8' };
      }
      return {};
    });

    const { result } = renderHook(() =>
      useServiceToggle('xboxdns', false, vi.fn(), { startMsg: 'on', stopMsg: 'off' })
    );

    await act(async () => { await result.current.toggle(); });

    expect(api).toHaveBeenCalledWith('GET', '/api/xbox-dns/config');
    expect(api).toHaveBeenCalledWith('POST', '/api/xbox-dns/config', {
      enabled: true,
      primary_dns: '1.1.1.1',
      secondary_dns: '8.8.8.8',
    });
    expect(api).toHaveBeenCalledWith('POST', '/api/component-states');
  });
});
