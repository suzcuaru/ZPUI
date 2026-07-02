import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { api } from './api';

function installApp(app) {
  window.go = { main: { App: app } };
}

describe('api routing', () => {
  let app;
  beforeEach(() => {
    app = {
      GetStatus: vi.fn().mockResolvedValue({ ok: true }),
      GetLogs: vi.fn().mockResolvedValue({ lines: [] }),
      ZapretStart: vi.fn().mockResolvedValue({}),
      SetProxyConfig: vi.fn().mockResolvedValue({}),
      GetDevice: vi.fn().mockResolvedValue(null),
      GetDeviceConnections: vi.fn().mockResolvedValue([]),
    };
    installApp(app);
  });
  afterEach(() => { delete window.go; });

  it('dispatches GET /api/status to GetStatus', async () => {
    const res = await api('GET', '/api/status');
    expect(app.GetStatus).toHaveBeenCalledTimes(1);
    expect(res).toEqual({ ok: true });
  });

  it('parses query params for GET /api/logs', async () => {
    await api('GET', '/api/logs?category=main&lines=50');
    expect(app.GetLogs).toHaveBeenCalledWith('main', 50);
  });

  it('applies defaults when query params are missing', async () => {
    await api('GET', '/api/logs');
    expect(app.GetLogs).toHaveBeenCalledWith('zapret', 100);
  });

  it('dispatches POST /api/zapret/start to ZapretStart', async () => {
    await api('POST', '/api/zapret/start');
    expect(app.ZapretStart).toHaveBeenCalledTimes(1);
  });

  it('passes body to POST /api/proxy/config', async () => {
    await api('POST', '/api/proxy/config', { port: 1080 });
    expect(app.SetProxyConfig).toHaveBeenCalledWith({ port: 1080 });
  });

  it('returns null for unknown routes', async () => {
    const res = await api('GET', '/api/does-not-exist');
    expect(res).toBeNull();
  });

  it('returns null when wails runtime is unavailable', async () => {
    delete window.go;
    const res = await api('GET', '/api/status');
    expect(res).toBeNull();
  });

  it('handles device sub-routes (/api/devices/:mac/connections)', async () => {
    await api('GET', '/api/devices/AA:BB:CC:DD:EE:FF/connections?limit=10');
    expect(app.GetDeviceConnections).toHaveBeenCalledWith('AA:BB:CC:DD:EE:FF', 10, 0);
  });
});
