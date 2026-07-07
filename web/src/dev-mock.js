// Dev-only mock of the Wails backend. Lets the frontend run standalone
// (npm run dev) for visual work without the Go backend.
// Loaded only when import.meta.env.DEV is true and no real runtime present.

const mockStatus = {
  mod: { version: '1.4.7-dev', theme: 'system' },
  zapret: { status: 'running', strategy: 'general (ALT).bat', version: '1.7.6' },
  proxy: { running: true, port: 1080 },
  xbox_dns: { enabled: false, primary_dns: '111.88.96.50', secondary_dns: '111.88.96.51' },
  monitor: {
    dl_speed_fmt: '4.2 MB/s', ul_speed_fmt: '880 KB/s',
    download_fmt: '1.8 GB', upload_fmt: '320 MB',
    download_bytes: 1932735283, upload_bytes: 335544320,
  },
  network: { hostname: 'DESKTOP-ZPUI', ips: ['192.168.1.42', '10.0.0.5'] },
};

const mockConfig = {
  theme: 'system', first_run_done: false, zapret_skipped: false,
  auto_start_proxy: true, last_proxy_state: true, last_zapret_state: true, last_xbox_dns_state: false,
  language: 'ru', disable_updates: false,
};

const mockResources = {
  default: [
    { name: 'youtube.com', ok: true }, { name: 'discord.com', ok: true },
    { name: 'googlevideo.com', ok: true }, { name: 'twitch.tv', ok: false },
    { name: 'instagram.com', ok: true }, { name: 'x.com', ok: true },
  ],
  user: [{ name: 'custom.example.com', ok: true }],
};

function mockAvailability() {
  const records = [];
  for (let i = 0; i < 24; i++) records.push({ t: i, pct: 70 + Math.round(Math.random() * 30) });
  return { records };
}

const mockStrategies = [
  'general (ALT).bat', 'general (discord).bat', 'quic (ALT).bat',
  'discord_voice.bat', 'youtube_unblock.bat',
];

const mockDevices = [
  { mac: 'AA:BB:CC:DD:EE:01', ip: '192.168.1.10', hostname: 'Phone', connections: 12 },
  { mac: 'AA:BB:CC:DD:EE:02', ip: '192.168.1.11', hostname: 'Laptop', connections: 5 },
];

const mockSnapshots = [];
for (let i = 0; i < 30; i++) {
  mockSnapshots.push({ t: i, dl: Math.random() * 5000000, ul: Math.random() * 1000000 });
}

const mockVersions = { zpui: '1.4.7', wizard: '1.0.0', autoselect: '1.0.0', selfupdate: '1.0.0', zapretupdate: '1.0.0' };

const overrides = {
  GetStatus: async () => mockStatus,
  GetConfig: async () => mockConfig,
  GetSystemTheme: async () => 'dark',
  GetResourceStatus: async () => mockResources,
  GetAvailabilityHistory: async () => mockAvailability(),
  HealthCheck: async () => ({ overall: 'healthy', warnings: [] }),
  GetStrategies: async () => ({ strategies: mockStrategies, current: 'general (ALT).bat', resources: [] }),
  GetTraffic: async () => mockStatus.monitor,
  GetMonitorDevices: async () => mockDevices,
  GetTrafficSnapshots: async () => mockSnapshots,
  GetVersions: async () => mockVersions,
  GetNetworkInfo: async () => mockStatus.network,
  GetProxyStatus: async () => ({ running: true, port: 1080 }),
  GetProxyConfig: async () => ({ enable: true, port: 1080, bind_host: '127.0.0.1' }),
  GetXboxDnsConfig: async () => ({ enabled: false, primary_dns: '111.88.96.50', secondary_dns: '111.88.96.51' }),
  HasLocalZapret: async () => true,
  HasSystemZapretService: async () => true,
  GetServiceStatus: async () => ({ running: true }),
  DetectThirdPartyZapret: async () => ({ has_third_party: false, third_party_detail: '' }),
  SetupListStrategies: async () => ({ strategies: mockStrategies, current: 'general (ALT).bat', resources: [] }),
  GetAutostartStatus: async () => ({ enabled: true }),
  GetLists: async () => ({ 'list-general.txt': '# sample', 'list-discord.txt': '# sample' }),
  GetLogFiles: async () => ['main.log', 'zapret.log'],
  GetLogs: async () => ['[mock] sample log line 1', '[mock] sample log line 2'],
  CheckWizardDone: async () => true,
};

export function installDevMock() {
  if (typeof window === 'undefined') return;
  if (window.go && window.go.app && window.go.app.App) return;

  const app = new Proxy({}, {
    get(_t, prop) {
      if (prop in overrides) return overrides[prop];
      const listish = ['GetDevices', 'GetBackups', 'GetActionLogs', 'GetLogFiles', 'GetErrorSnapshots', 'GetArchiveLogs', 'GetProxyConnections'];
      if (listish.includes(prop)) return async () => [];
      return async () => null;
    },
  });

  window.go = { app: { App: app } };
  window.runtime = {
    EventsOn: () => {}, EventsOff: () => {}, EventsEmit: () => {},
    WindowClose: () => { try { window.close(); } catch {} },
    Quit: () => { try { window.close(); } catch {} },
    OpenExternal: () => Promise.resolve(),
  };

  console.info('[dev-mock] Installed mock Wails backend. Visual-only mode.');
}
