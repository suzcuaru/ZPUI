// Stand-only mock of the Wails backend for the UI stand (mockups/app-ui).
// Realistic demo data, mutable service states, event-bus runtime and
// scripted autoselect/single-test streams. Installed in main.jsx.

const now = () => Math.floor(Date.now() / 1000);

const state = {
  zapret: 'running',
  zapretStrategy: 'general.bat',
  proxyRunning: true,
  xboxEnabled: false,
  tgRunning: false,
  autostart: true,
  accountSet: true,
  autoSwitch: true,
  cancelled: false,
};

const strategyFiles = [
  'general.bat', 'general (ALT).bat', 'general (discord).bat',
  'quic (ALT).bat', 'discord_voice.bat', 'youtube_unblock.bat',
];
const strategies = () => strategyFiles.map((f) => ({
  filename: f,
  name: f.replace(/\.bat$/i, ''),
  current: f === state.zapretStrategy,
}));

const status = () => ({
  mod: { version: '1.9.1', arch: 'x64', theme: config.theme },
  zapret: { status: state.zapret, strategy: state.zapretStrategy, version: '1.3.1' },
  proxy: { running: state.proxyRunning, port: 1080 },
  tgproxy: { running: state.tgRunning },
  xbox_dns: {
    enabled: state.xboxEnabled,
    primary_dns: '111.88.96.50',
    secondary_dns: '111.88.96.51',
  },
  system: {
    os: 'Windows 11 Pro (10.0.26200)',
    cpu: 'AMD Ryzen 5 7530U · 12',
  },
  monitor: {
    dl_speed_fmt: fmtSpeed(4_600_000 + jitter(400_000)),
    ul_speed_fmt: fmtSpeed(920_000 + jitter(120_000)),
    download_fmt: '18,4 GB', upload_fmt: '3,1 GB',
    download_bytes: 19756849500, upload_bytes: 3328599654,
    download_speed: 4600000, upload_speed: 920000,
  },
  network: { hostname: 'DESKTOP-ABC123', ips: ['192.168.1.42'], public_ip: '80.240.252.169' },
  power_saving: false,
  power_saving_level: 2,
});

function jitter(spread) { return Math.round((Math.random() - 0.5) * 2 * spread); }
function fmtSpeed(bps) {
  if (bps >= 1048576) return (bps / 1048576).toFixed(1).replace('.', ',') + ' MB/s';
  return Math.round(bps / 1024) + ' KB/s';
}

const config = {
  theme: 'system', first_run_done: true, zapret_skipped: false,
  auto_start_proxy: true, last_proxy_state: true, last_zapret_state: true, last_xbox_dns_state: false,
  language: 'ru', disable_updates: false, auto_update_check: true,
  power_saving: false, power_level: 2, power_threshold: 0,
  custom_theme: null,
};

// Конфиг мока живёт между перезагрузками страницы: тема/кастом восстанавливаются,
// как в продукте (config.json — источник истины).
try {
  const savedCfg = JSON.parse(localStorage.getItem('zpui-stand-config') || 'null');
  if (savedCfg && typeof savedCfg === 'object') Object.assign(config, savedCfg);
} catch {}

function persistConfig() {
  try { localStorage.setItem('zpui-stand-config', JSON.stringify(config)); } catch {}
}

// Сохранённая кастомная тема («БД» стенда): zpui-stand-custom-theme.
let customTheme = null;
try {
  customTheme = JSON.parse(localStorage.getItem('zpui-stand-custom-theme') || 'null');
} catch {}
if (customTheme && typeof customTheme === 'object') config.custom_theme = customTheme;

const resources = () => ({
  default: [
    { name: 'youtube.com', ok: true }, { name: 'discord.com', ok: true },
    { name: 'googlevideo.com', ok: true }, { name: 'twitch.tv', ok: Math.random() > 0.4 },
    { name: 'instagram.com', ok: true }, { name: 'x.com', ok: true },
  ],
  user: [{ name: 'custom.example.com', ok: true }],
  checking_now: false,
  last_check_unix: now() - 240,
});

const devices = [
  { mac: 'A4:BB:6D:3E:91:C7', ip: '192.168.1.42', hostname: 'DESKTOP-ABC123', connections: 34 },
  { mac: '8C:16:45:0A:22:F1', ip: '192.168.1.51', hostname: 'LAPTOP-K9M2', connections: 12 },
];

const snapshots = [];
for (let i = 0; i < 60; i++) {
  snapshots.push({ t: i, dl: 3_500_000 + Math.random() * 2_500_000, ul: 600_000 + Math.random() * 700_000 });
}

function availability(type) {
  const records = [];
  for (let i = 0; i < 40; i++) records.push({ t: now() - (40 - i) * 120, pct: 72 + Math.round(Math.random() * 28) });
  return { records, type };
}

// ── Аккаунт с подпиской ───────────────────────────────
// Вход = логин + пароль; инвайт-код нужен только при регистрации.
// Устройства привязываются по отпечатку системы (fingerprint), ключей нет.
const DEMO_INVITE = 'DEMO-2026';

const defaultAccount = {
  login: 'demo',
  password: 'demo123',
  invite: DEMO_INVITE,
  plan: 'tester',
  period: 'monthly',
  account_status: 'active',
  // Месячная подписка «на исходе» (осталось меньше суток) — кнопка
  // «Продлить» видна, статус аккаунта «Истекает».
  until: now() + 86400 - 3600 * 3,
  // Далёкая дата (кнопка «Продлить» скрыта, статус «Активен»):
  // until: now() + 86400 * 25,
  devices_max: 5,
  created_at: 1740900000,
};

const defaultDevices = [
  { id: 'fp-8a3f52c9', name: 'DESKTOP-ABC123', os: 'Windows 11 Pro (10.0.26200)', fingerprint: 'FP:8A3F-52C9-DE71', bound_at: now() - 86400 * 30 },
  { id: 'fp-c2d791ba', name: 'LAPTOP-K9M2', os: 'Windows 10 Pro (19045)', fingerprint: 'FP:C2D7-91BA-4406', bound_at: now() - 86400 * 12 },
];

let account = { ...defaultAccount };
let accountDevices = defaultDevices.map((d) => ({ ...d }));
let accountRegistered = true;

try {
  const saved = JSON.parse(localStorage.getItem('zpui-stand-account') || 'null');
  if (saved) {
    accountRegistered = !!saved.registered;
    if (saved.registered) {
      account = { ...defaultAccount, ...(saved.account || {}) };
      if (Array.isArray(saved.devices) && saved.devices.length) accountDevices = saved.devices;
    }
  }
} catch {}

function persistAccount() {
  try {
    if (accountRegistered) {
      localStorage.setItem('zpui-stand-account', JSON.stringify({ registered: true, account, devices: accountDevices }));
    } else {
      localStorage.setItem('zpui-stand-account', JSON.stringify({ registered: false }));
    }
  } catch {}
}

// Инвайт принимаем демо-кодом или любой строкой от 8 символов (буквы/цифры).
function inviteValid(code) {
  const c = String(code || '').trim().toUpperCase();
  if (c === DEMO_INVITE) return true;
  return c.replace(/[^A-Z0-9]/g, '').length >= 8;
}

const me = () => ({
  user: {
    login: account.login,
    status: account.account_status,
    created_at: account.created_at,
  },
  devices: accountDevices,
  subscription: {
    plan: account.plan,
    period: account.period,
    status: account.account_status,
    until: account.until,
    devices_max: account.devices_max,
  },
});

const reportText = [
  '# ZPUI Diagnostic Report',
  '',
  'Generated: ' + new Date().toLocaleString('ru-RU') + ' · Host: DESKTOP-ABC123',
  '',
  '## Diagnostic Summary',
  '',
  '| Metric              | Value                        |',
  '|---------------------|------------------------------|',
  '| ZPUI Version        | 1.9.1 (x64)                  |',
  '| Zapret Version      | 1.3.1                        |',
  '| Strategy            | general.bat                  |',
  '| Engine State        | ' + state.zapret.padEnd(28) + '|',
  '| Errors (DB)         | none                         |',
  '| SOCKS5 Proxy        | ' + (state.proxyRunning ? 'running' : 'stopped').padEnd(28) + '|',
  '| Telegram MTProto    | stopped                      |',
  '| Xbox DNS            | ' + (state.xboxEnabled ? 'on' : 'off').padEnd(28) + '|',
  '| Public IP           | 80.240.252.169               |',
  '',
  '## System Information',
  '',
  '- OS: Windows 11 Pro (build 10.0.26200)',
  '- CPU: AMD Ryzen 5 7530U, 12 cores',
  '- RAM: 7558 MB total / 908 MB available',
  '- Hostname: DESKTOP-ABC123',
  '- MAC: A4:BB:6D:3E:91:C7',
  '- Admin rights: yes (WinDivert available)',
  '',
  '## Configuration (anonymized)',
  '',
  '- AutoSwitch: enabled (threshold 60%)',
  '- Monitoring interval: 30 s',
  '- Autostart: scheduled task ZPUI (ONLOGON, RL HIGHEST)',
  '- Update channel: stable',
].join('\n');

const logLines = () => {
  const out = [];
  const lv = ['INFO', 'INFO', 'INFO', 'WARN', 'ERROR', 'DEBUG'];
  const msgs = [
    'zapret: strategy general.bat applied',
    'proxy: SOCKS5 listener on 127.0.0.1:1080',
    'monitor: availability sample 94%',
    'updater: check finished, no updates',
    'tgproxy: bridge idle',
    'zapret: winws.exe pid 4812',
    'config: saved config.json',
  ];
  const tables = ['app', 'zapret', 'updater', 'tgproxy'];
  for (let i = 0; i < 40; i++) {
    const level = lv[Math.floor(Math.random() * lv.length)];
    const msg = msgs[i % msgs.length];
    const mod = msg.split(':')[0];
    const ts = new Date(Date.now() - i * 45000);
    // Объекты, как отдаёт реальный GetAllLogs (таблица LogDrawer).
    out.push({
      timestamp: ts.toLocaleDateString('ru-RU') + ' ' + ts.toLocaleTimeString('ru-RU'),
      level,
      table: tables[i % tables.length],
      category: mod,
      message: msg.split(': ').slice(1).join(': '),
      code: '',
    });
  }
  return out;
};

// ── Runtime: event bus ────────────────────────────────

const listeners = {};
const runtime = {
  __standMock: true,
  EventsOn(name, fn) { (listeners[name] = listeners[name] || []).push(fn); },
  EventsOff(name) { delete listeners[name]; },
  EventsEmit(name, data) { (listeners[name] || []).forEach((fn) => { try { fn(data); } catch {} }); },
  WindowClose: () => {},
  Quit: () => {},
  OpenExternal: () => Promise.resolve(),
  BrowserOpenURL: () => {},
  WindowMinimise: () => {},
  WindowToggleMaximise: () => {},
};

// ── Scripted streams ──────────────────────────────────

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function runStrategyStream(prefix) {
  state.cancelled = false;
  const total = 6;
  let best = { strategy: 'general (ALT).bat', pct: 0 };
  for (let i = 1; i <= total; i++) {
    if (state.cancelled) break;
    const name = strategies[i % strategies.length];
    runtime.EventsEmit(prefix + ':event', { type: 'progress', current: i, total, strategy: name, message: '' });
    await sleep(1300);
    if (state.cancelled) break;
    const pct = 62 + Math.round(Math.random() * 34);
    runtime.EventsEmit(prefix + ':event', { type: 'result', strategy: name, pct, ok: Math.round(pct / 100 * 24), total: 24 });
    if (pct > best.pct) best = { strategy: name, pct };
  }
  if (!state.cancelled) {
    runtime.EventsEmit(prefix + ':event', { type: 'info', message: 'applying best strategy' });
    await sleep(600);
    state.zapretStrategy = best.strategy;
    runtime.EventsEmit(prefix + ':done', { applied: best.strategy, success: true });
  } else {
    runtime.EventsEmit(prefix + ':done', { cancelled: true });
  }
}

// ── Overrides ─────────────────────────────────────────

const ok = { status: 'ok' };

const overrides = {
  // status / config / theme
  GetStatus: async () => status(),
  GetConfig: async () => ({ ...config }),
  SetConfig: async (b) => { Object.assign(config, b || {}); persistConfig(); return ok; },
  GetCustomTheme: async () => ({ theme: customTheme }),
  SaveCustomTheme: async (b) => {
    customTheme = (b && b.theme) || null;
    config.custom_theme = customTheme;
    persistConfig();
    try { localStorage.setItem('zpui-stand-custom-theme', JSON.stringify(customTheme)); } catch {}
    return ok;
  },
  GetSystemTheme: async () => 'dark',
  GetNetworkInfo: async () => ({ hostname: 'DESKTOP-ABC123', ips: ['192.168.1.42'], public_ip: '80.240.252.169', gateway: '192.168.1.1' }),

  // zapret
  GetStrategies: async () => ({ strategies: strategies(), current: state.zapretStrategy, resources: [] }),
  SetupListStrategies: async () => ({ strategies: strategies(), current: state.zapretStrategy, resources: [] }),
  GetServiceStatus: async () => ({ running: state.zapret === 'running' }),
  GetGameFilter: async () => ({ mode: 'off' }),
  SetGameFilter: async () => ok,
  GetIpsetStatus: async () => ({ mode: 'off', updated_at: now() - 86400 * 3 }),
  ToggleIpset: async () => ok,
  GetAutoUpdateStatus: async () => ({ enabled: true }),
  ToggleAutoUpdate: async () => ok,
  UpdateIpset: async () => ok,
  UpdateHosts: async () => ok,
  DefaultStrategy: async () => ({ strategy: 'general.bat' }),
  GetAutoTestResults: async () => ({
    results: [
      { strategy: 'general (ALT).bat', pct: 92, ok: 22, total: 24 },
      { strategy: 'general.bat', pct: 86, ok: 21, total: 24 },
      { strategy: 'quic (ALT).bat', pct: 74, ok: 18, total: 24 },
    ],
  }),
  GetSingleTestResult: async () => ({ running: false, result: null }),
  GetSkipResources: async () => 'cdn.example.net\nstatic.example.org',
  GetLists: async () => ({ 'list-general.txt': '# youtube.com\n# discord.com', 'list-discord.txt': '# discord media' }),
  GetInstallLog: async () => ({ lines: ['[mock] install log line'] }),
  HasLocalZapret: async () => true,
  HasSystemZapretService: async () => true,
  IsServiceInstalled: async () => true,
  VerifyZapretFiles: async () => ({ ok: true, missing: [] }),
  DetectThirdPartyZapret: async () => ({ has_third_party: false, third_party_detail: '' }),
  CheckServiceHealth: async () => ({ status: 'ok', message: 'healthy' }),
  RunDiagnostics: async () => ({ status: 'ok', lines: ['[mock] diagnostics: all checks passed'] }),
  GetStrategyChangeHistory: async () => ({
    items: [
      { at: now() - 3600 * 5, from: 'general.bat', to: 'general (ALT).bat', reason: 'autoselect' },
      { at: now() - 3600 * 26, from: 'quic (ALT).bat', to: 'general.bat', reason: 'manual' },
    ],
  }),

  ZapretStart: async () => {
    state.zapret = 'starting';
    setTimeout(() => { state.zapret = 'running'; }, 2500);
    return { status: 'starting' };
  },
  ZapretStop: async () => { state.zapret = 'stopped'; return ok; },
  ZapretRestart: async () => { state.zapret = 'starting'; setTimeout(() => { state.zapret = 'running'; }, 2500); return { status: 'starting' }; },
  SetStrategy: async (f) => { state.zapretStrategy = f || state.zapretStrategy; return ok; },
  InstallService: async () => ({ success: true }),
  InstallServiceLogged: async () => ({ success: true }),
  RemoveService: async () => ({ success: true }),
  RemoveSystemZapretService: async () => ok,
  InstallZapret: async () => ok,
  AutoInstallZapret: async () => ok,
  FullReinstall: async () => ok,
  ClearCache: async () => ok,
  SaveList: async () => ok,
  SaveSkipResources: async () => ok,
  AddSkipResource: async () => ok,

  // proxy / tgproxy / xbox
  GetProxyStatus: async () => ({ running: state.proxyRunning, port: 1080 }),
  GetProxyConfig: async () => ({ enable: true, port: 1080, bind_host: '127.0.0.1' }),
  SetProxyConfig: async () => ok,
  GetProxyConnections: async () => (state.proxyRunning ? [
    { remote: '142.250.74.14:443', host: 'youtube.com', bytes: 184000, at: now() - 5 },
    { remote: '162.159.128.23:443', host: 'discord.com', bytes: 96000, at: now() - 14 },
  ] : []),
  GetProxyQRCode: async () => '',
  ProxyStart: async () => { state.proxyRunning = true; return ok; },
  ProxyStop: async () => { state.proxyRunning = false; return ok; },
  GetTgProxyStatus: async () => ({ running: state.tgRunning, state: state.tgRunning ? 'running' : 'stopped' }),
  GetTgProxyConfig: async () => ({ enable: false, port: 443, secret: 'ee1603b0b4d2f5e2a1c0' }),
  SetTgProxyConfig: async () => ok,
  GetTgProxyLink: async () => ({ link: 'https://t.me/proxy?server=127.0.0.1&port=443&secret=ee1603b0b4d2f5e2a1c0' }),
  TgProxyStart: async () => { state.tgRunning = true; return ok; },
  TgProxyStop: async () => { state.tgRunning = false; return ok; },
  TgProxyRestart: async () => ok,
  RegenerateTgProxySecret: async () => ok,
  OpenTgProxyLink: async () => ok,
  GetXboxDnsConfig: async () => ({ enabled: state.xboxEnabled, primary_dns: '111.88.96.50', secondary_dns: '111.88.96.51' }),
  SetXboxDnsConfig: async () => ok,
  ToggleXboxDns: async (b) => { state.xboxEnabled = !!b.enabled; return ok; },

  // monitor / resources
  GetMonitorDevices: async () => devices,
  GetDevice: async () => devices[0],
  GetDeviceConnections: async () => [],
  PingDevice: async () => ({ ok: true, ms: 3 }),
  DeleteDevice: async () => ok,
  GetDevices: async () => [],
  GetTrafficSnapshots: async () => snapshots,
  GetTraffic: async () => status().monitor,
  GetUpInfo: async () => null,
  GetResourceStatus: async () => resources(),
  RefreshResourceStatus: async () => resources(),
  CheckResource: async () => ({
    report: {
      Host: 'instagram.com',
      Provider: { IP: '80.240.252.169', ISP: 'Rostelecom', ASN: 'AS12389', City: 'Москва', Country: 'RU' },
      Direct: {
        Verdict: 'TLS_BLOCK', Confidence: 'HIGH',
        TCP: { Ok: true, TimeMs: 23.4 },
        TLS: { Ok: false, TimeMs: 0, Error: 'TLS handshake reset by DPI' },
        HTTP: { Ok: false, TimeMs: 0, Error: 'connection reset' },
        Notes: ['SNI домен отрезается на этапе TLS ClientHello.'],
      },
      Blocked: true,
      InUserList: false,
    },
  }),
  CheckResourceDeep: async () => ({
    report: {
      host: 'instagram.com',
      main_verdict: 'DNS_BLOCK',
      fetched: true,
      total: 7, blocked_count: 3, ok_count: 4,
      sub_resources: [
        { host: 'instagram.com', verdict: 'DNS_BLOCK', blocked: true, ok: false, latency_ms: 0, reason: 'DNS-ответ подменён' },
        { host: 'cdninstagram.com', verdict: 'TLS_BLOCK', blocked: true, ok: false, latency_ms: 12, reason: 'TLS reset' },
        { host: 'fbcdn.net', verdict: 'TCP_BLOCK', blocked: true, ok: false, latency_ms: 0, reason: 'TCP filtered' },
        { host: 'api.instagram.com', verdict: 'OK', blocked: false, ok: true, latency_ms: 84 },
        { host: 'graph.instagram.com', verdict: 'OK', blocked: false, ok: true, latency_ms: 91 },
        { host: 'scontent.cdninstagram.com', verdict: 'OK', blocked: false, ok: true, latency_ms: 77 },
        { host: 'l.instagram.com', verdict: 'OK', blocked: false, ok: true, latency_ms: 88 },
      ],
    },
  }),
  AddHostToUserList: async () => ok,
  AddHostsToUserList: async () => ok,
  GetYouTubeStatus: async () => ({ checked: true, dns_poisoned: false }),
  GetAvailabilityHistory: async () => availability(),
  GetSystemResources: async () => ({ cpu_percent: 11 + jitter(6), ram_total: 7558, ram_used: 6650, ram_available: 908, disks: [{ letter: 'C:', total: 476, free: 210 }] }),

  // logs
  GetLogs: async () => ({ lines: logLines() }),
  GetAllLogs: async () => ({ lines: logLines() }),
  GetLogStats: async () => ({ total: 1284, errors: 3, db_size: 412688, counts: { app_logs: 512, zapret_logs: 402, updater_logs: 210, tgproxy_logs: 160 } }),
  GetLogCount: async () => 42,
  GetLogTableNames: async () => ['app', 'zapret', 'updater', 'tgproxy'],
  FindLogEntryByMsg: async () => null,
  ClearLogs: async () => ok,
  ClearLogBucket: async () => ok,
  ExportLogs: async () => ({ path: 'C:\\mock\\logs.zip' }),
  GetLogDebug: async () => ({ enabled: false }),
  SetLogDebug: async () => ok,
  LogErrorCode: async () => ok,
  FrontendLogs: async () => ok,

  // updates
  CheckZPUIUpdate: async () => ({ update_needed: true, current: '1.9.1', latest: '2.0.0' }),
  CheckForUpdates: async () => ({ available: false }),
  CheckVersionsPanel: async () => ({ zpui: { current: '1.9.1', latest: '1.9.1', available: false } }),
  CheckZapretVersionsPanel: async () => ({ zapret: { current: '1.3.1', latest: '1.3.1', available: false } }),
  GetZapretAvailableVersions: async () => ({ versions: ['1.3.1', '1.3.0', '1.2.9'] }),
  GetZapretReleaseInfo: async () => ({ notes: '# zapret ' + 'release notes (mock)' }),
  GetUpdateState: async () => ({ state: 'idle' }),
  GetReleaseNotes: async () => ({ notes: '(mock) release notes' }),
  GetIgnoredVersions: async () => ({ items: [] }),
  AddIgnoredVersion: async () => ok,
  RemoveIgnoredVersion: async () => ok,
  CheckComponentUpdates: async () => ({ has_update: false, items: [] }),
  DownloadZPUISetup: async () => ok,
  ApplyZPUISetup: async () => ok,
  DownloadZapretUpdate: async () => ok,
  DownloadZapretVersion: async () => ok,
  ApplyZapretPackage: async () => ok,
  GetVersions: async () => ({ zpui: '1.9.1', report: '1.4.2', tgproxy: '1.2.0', zapret: '1.3.1' }),

  // system / autostart / health
  GetAutostartStatus: async () => ({ enabled: state.autostart }),
  EnableAutostart: async () => { state.autostart = true; return ok; },
  DisableAutostart: async () => { state.autostart = false; return ok; },
  HealthCheck: async () => ({ overall: 'healthy', warnings: [] }),
  GetCurrentOperator: async () => ({ name: 'Rostelecom', asn: 'AS12389', code: 'rostelecom' }),
  GetOperatorHistory: async () => ({ items: [{ at: now() - 86400, name: 'Rostelecom' }] }),
  GetBackups: async () => [],
  RestoreFromBackup: async () => ok,

  // autoselect / autoswitch
  RunAutoSelectStream: async () => { await runStrategyStream('autoselect'); return ok; },
  RunAutoTestStream: async () => { await runStrategyStream('strategy'); return ok; },
  StartAutoTest: async () => { runStrategyStream('strategy'); return { status: 'started' }; },
  CancelAutoTest: async () => { state.cancelled = true; return ok; },
  TestSingleStrategy: async () => { return { status: 'started' }; },
  GetAutoSelectLastCheck: async () => ({ last: now() - 3600 * 6, total: 6, best: 'general (ALT).bat' }),
  GetAutoSwitchStatus: async () => ({ enabled: state.autoSwitch, threshold: 60, interval: 30, last_switch: now() - 86400 * 2 }),
  SetAutoSwitchEnabled: async (b) => { state.autoSwitch = !!b.enabled; return ok; },
  SetAutoSwitchConfig: async () => ok,
  GetAutoSwitchHistory: async () => ({ items: [{ at: now() - 86400 * 2, from: 'general.bat', to: 'general (ALT).bat', availability: 54 }] }),

  // report / docs
  GetReport: async () => ({ content: reportText }),
  SaveReport: async () => ({ path: 'C:\\mock\\report.md' }),
  OpenReportFile: async () => ok,
  CopyReportFileToClipboard: async () => ok,

  // server / account
  GetServerStatus: async () => ({ account_set: state.accountSet, url: 'https://zpui.example.net', device_id: 'DESKTOP-ABC123', hostname: 'DESKTOP-ABC123', connected: true }),
  PingServer: async () => ({ available: true, url: 'https://zpui.example.net', latency_ms: 38 + jitter(10) }),
  GetServerComponentUpdates: async () => ({ has_update: false, items: [] }),
  ServerActivate: async () => ok,
  ServerSetMode: async () => ok,
  SetServerKey: async () => ok,
  ServerUpdateComponent: async () => ok,
  ServerUnbindDevice: async (b) => {
    accountDevices = accountDevices.filter((d) => d.id !== (b && b.device_id));
    persistAccount();
    return ok;
  },
  AccountMe: async () => (accountRegistered ? me() : { error: 'not_authorized' }),
  CheckInvite: (p) => (inviteValid(p && p.code) ? { valid: true } : { error: 'invalid_invite' }),
  ActivateInvite: async (b) => {
    if (!inviteValid(b && b.code)) return { error: 'invalid_invite' };
    const login = String((b && b.login) || '').trim();
    if (login.length < 3 || String((b && b.password) || '').length < 6) return { error: 'invalid_input' };
    account.login = login;
    account.password = String(b.password);
    accountRegistered = true;
    state.accountSet = true;
    persistAccount();
    return ok;
  },
  AccountLogin: async (login, password) => {
    if (String(login || '').trim() !== account.login || String(password || '') !== account.password) {
      return { error: 'bad_credentials' };
    }
    accountRegistered = true;
    state.accountSet = true;
    persistAccount();
    return ok;
  },
  AccountRegisterStart: async () => ok,
  AccountRegisterConfirm: async () => { accountRegistered = true; state.accountSet = true; persistAccount(); return ok; },
  AccountLogout: async () => {
    accountRegistered = false;
    persistAccount();
    return ok;
  },

  // sync / banners / misc
  GetSyncPeers: async () => ({ peers: [] }),
  GetStartupSyncSuggestion: async () => null,
  GetSyncSettings: async () => ({ enabled: false, port: 9432 }),
  SetSyncSettings: async () => ok,
  RequestSyncFromPeer: async () => ok,
  ApplySyncFromPeer: async () => ok,
  SyncNow: async () => ok,
  DisconnectPeer: async () => ok,
  ConfirmPeerRename: async () => ok,
  DismissBanner: async () => ok,
  SaveComponentStates: async () => ok,
  RestartApp: async () => { window.location.reload(); return ok; },
  RestartAppClean: async () => { window.location.reload(); return ok; },
  OpenExternal: async () => ok,
  OpenTelegramLink: async () => ok,
  SendTestNotification: async () => ok,
  SetupControlCheck: async () => ({ ok: true }),
  SetupTestStrategy: async () => ({ passed: true, pct: 88, ok: 21, total: 24 }),
  InstallOurZapret: async () => ok,
  StartOurZapret: async () => ok,
  WindowClose: async () => ok,
  SetupApplyStrategy: async () => ok,
  SetupConfigureFilters: async () => ok,
  SetupConfigureDNS: async () => ok,
  SetupConfigureProxy: async () => ok,
  SetupSkip: async () => ok,
  SetupComplete: async () => ok,
};

export function installStandMock() {
  if (typeof window === 'undefined') return;

  try {
    if (!localStorage.getItem('zpui-manual-seen')) localStorage.setItem('zpui-manual-seen', '1');
  } catch {}

  // Команды (не-GET) идут с реалистичной задержкой 300–1200 мс и редким
  // случайным отказом ~10%: тосты и смены состояния возможны ТОЛЬКО после
  // ответа мока. Запросы (Get*/Is*/Ping*/Open*/Copy*) — без отказов.
  const COMMAND_DELAY = [300, 1200];
  const COMMAND_FAIL_RATE = 0.10;
  const isCommand = (name) => /^(Set|Start|Stop|Restart|Toggle|Save|Delete|Remove|Install|Uninstall|Unbind|Run|Cancel|Apply|Activate|Bind|Register|Login|Logout|WindowClose|Setup|Complete|Skip|Reinstall|Enable|Disable|Regenerate|OpenTg|TestStr)/.test(name);
  function mockCommand(fn) {
    return async (...args) => {
      await new Promise((r) => setTimeout(r, COMMAND_DELAY[0] + Math.random() * (COMMAND_DELAY[1] - COMMAND_DELAY[0])));
      if (Math.random() < COMMAND_FAIL_RATE) {
        return { error: 'Демо-отказ: команда отклонена мини-бэком (~10% отказов)' };
      }
      return fn(...args);
    };
  }

  const app = new Proxy({}, {
    get(_t, prop) {
      if (prop in overrides) {
        const fn = overrides[prop];
        return typeof fn === 'function' && isCommand(String(prop)) ? mockCommand(fn) : fn;
      }
      return async () => {
        console.info('[stand-mock] Unhandled App method:', String(prop));
        return null;
      };
    },
  });

  window.go = { app: { App: app } };
  window.runtime = runtime;
  window.__zpuiStand = { state, strategies: strategyFiles };

  console.info('[stand-mock] Installed stand backend mock.');
}
