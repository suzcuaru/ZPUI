import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { I18nProvider } from './i18n';
import { useT } from './i18n';
import DashboardPage from './pages/DashboardPage';
import ru from './locales/ru.json';
import en from './locales/en.json';

const STATUS = () => ({
  zapret: { status: 'running', version: '2.1.2', strategy: 'general(1).bat', auto_test_running: false },
  proxy: { running: true, port: 1080, stats: {} },
  tg_proxy: { running: true, version: '1.0.0', status: 'running', stats: {} },
  xbox_dns: { enabled: true, xbox_enabled: false, servers: ['77.88.8.8'] },
  monitor: { download_bytes: 0, upload_bytes: 0, download_speed: 0, upload_speed: 0, download_fmt: '0 B', upload_fmt: '0 B', dl_speed_fmt: '0 B/s', ul_speed_fmt: '0 B/s' },
  mod: { version: '2.1.2', theme: 'dark', first_run_done: true },
  network: { hostname: 'PC', mac: '', ips: ['127.0.0.1'] },
  power_saving: false, power_saving_level: 2, power_saving_paused: false,
  power_battery: { percent: 100, has: false },
});

function bindings(map) {
  global.window.go = { app: { App: new Proxy(map, { get(t, p) { if (p in t) return t[p]; return async () => null; } }) } };
  global.window.runtime = { EventsOn() {}, EventsOff() {}, EventsEmit() {} };
}

const noop = () => {};

if (typeof global.ResizeObserver === 'undefined') {
  global.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
}

function Probe() {
  const { t } = useT();
  return (
    <div>
      <span id="p-running">{String(t('status.running'))}</span>
      <span id="p-stopped">{String(t('status.stopped'))}</span>
      <span id="p-nodata">{String(t('status.getDataFailed'))}</span>
      <span id="p-dnscount">{String(t('dashboard.dnsCount', { n: 2 }))}</span>
    </div>
  );
}

describe('DIAG locale keys', () => {
  it('raw values in ru/en', () => {
    console.log('ru.status =', JSON.stringify(ru.status));
    console.log('en.status =', JSON.stringify(en.status));
    console.log('ru.dashboard.dnsCount =', ru.dashboard?.dnsCount, '| en =', en.dashboard?.dnsCount);
    expect(true).toBe(true);
  });

  it('t() resolution through provider', () => {
    render(<I18nProvider><Probe /></I18nProvider>);
    console.log('running =', JSON.stringify(document.getElementById('p-running').textContent));
    console.log('stopped =', JSON.stringify(document.getElementById('p-stopped').textContent));
    console.log('getDataFailed =', JSON.stringify(document.getElementById('p-nodata').textContent));
    console.log('dnsCount =', JSON.stringify(document.getElementById('p-dnscount').textContent));
    expect(true).toBe(true);
  });
});

describe('DIAG dashboard pills', () => {
  beforeEach(() => {
    bindings({
      GetResourceStatus: async () => ({ default: [], user: [], cached: false, checking_now: false }),
      GetAvailabilityHistory: async () => ({ records: [] }),
      GetStrategyChangeHistory: async () => ({ changes: [] }),
      GetTgProxyConfig: async () => ({ port: 1443 }),
      GetYouTubeStatus: async () => ({ checked: false }),
      GetStatus: async () => STATUS(),
    });
  });

  it('status=null → что в пилюлях', () => {
    render(<I18nProvider><DashboardPage status={null} onNavigate={noop} /></I18nProvider>);
    const states = [...document.querySelectorAll('.svc-card-state')].map(e => e.textContent);
    const dots = [...document.querySelectorAll('.svc-card-dot')].map(e => e.className);
    console.log('PILLS(null) =', JSON.stringify(states));
    console.log('DOTS(null) =', JSON.stringify(dots));
    expect(true).toBe(true);
  });

  it('status=полный → что в пилюлях', async () => {
    render(<I18nProvider><DashboardPage status={STATUS()} onNavigate={noop} /></I18nProvider>);
    await waitFor(() => expect(screen.getAllByText(/Запущен|Остановлен|Нет данных/).length).toBeGreaterThan(0));
    const states = [...document.querySelectorAll('.svc-card-state')].map(e => e.textContent);
    const dots = [...document.querySelectorAll('.svc-card-dot')].map(e => e.className);
    console.log('PILLS(full) =', JSON.stringify(states));
    console.log('DOTS(full) =', JSON.stringify(dots));
    expect(true).toBe(true);
  });
});
