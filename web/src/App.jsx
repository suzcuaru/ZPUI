import { useState, useEffect, useCallback, useRef } from 'react';
import Header from './components/layout/Header';
import Sidebar from './components/layout/Sidebar';
import Footer from './components/layout/Footer';
import Toast from './components/feedback/Toast';
import LogDrawer from './components/navigation/LogDrawer';
import OfflineScreen from './components/feedback/OfflineScreen';
import ResourceChecker from './components/ResourceChecker';
import HealthCheckModal from './components/HealthCheckModal';
import AutoSelectModal from './components/AutoSelectModal';
import StartupScreen from './components/StartupScreen';
import DashboardPage from './pages/DashboardPage';
import ZapretPage from './pages/ZapretPage';
import SettingsPage from './pages/SettingsPage';
import MonitorPage from './pages/MonitorPage';
import ProxyPage from './pages/ProxyPage';
import XboxDnsPage from './pages/XboxDnsPage';
import { api } from './api';
import { useT } from './i18n';
import './styles/index.css';

const PAGES = {
  dashboard: DashboardPage,
  zapret: ZapretPage,
  proxy: ProxyPage,
  xboxdns: XboxDnsPage,
  settings: SettingsPage,
  monitor: MonitorPage,
};

export default function App() {
  const { t } = useT();
  const [startupDone, setStartupDone] = useState(false);
  const [activePage, setActivePage] = useState('dashboard');
  const [status, setStatus] = useState(null);
  const [toasts, setToasts] = useState([]);
  const [logsOpen, setLogsOpen] = useState(false);
  const [checkerOpen, setCheckerOpen] = useState(false);
  const [autoSelectOpen, setAutoSelectOpen] = useState(false);
  const [backendOnline, setBackendOnline] = useState(true);
  const [theme, setTheme] = useState('dark');
  const [themeMode, setThemeMode] = useState('system');
  const [healthOpen, setHealthOpen] = useState(false);
  const [healthWarn, setHealthWarn] = useState(null);
  const failCountRef = useRef(0);
  const themeInitRef = useRef(false);
  const statusIntervalRef = useRef(null);

  const showToast = useCallback((msg, type) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type }]);
  }, []);

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const applyTheme = useCallback((mode) => {
    setThemeMode(mode);
    let actual = mode;
    if (mode === 'system') {
      actual = theme;
    }
    document.documentElement.setAttribute('data-theme', actual);
  }, [theme]);

  const toggleTheme = useCallback(() => {
    setTheme(prev => {
      const next = prev === 'dark' ? 'light' : 'dark';
      if (themeMode === 'system') {
        applyTheme('manual');
        document.documentElement.setAttribute('data-theme', next);
      } else {
        document.documentElement.setAttribute('data-theme', next);
        api('POST', '/api/config', { theme: next === 'dark' ? 'dark' : 'light' });
      }
      return next;
    });
  }, [themeMode, applyTheme]);

  // Theme init — run once after startup
  useEffect(() => {
    if (!startupDone || themeInitRef.current || !status?.mod) return;
    themeInitRef.current = true;
    const savedTheme = status.mod.theme || 'system';
    if (savedTheme === 'system') {
      api('GET', '/api/system-theme').then(sysTheme => {
        if (sysTheme) {
          setTheme(sysTheme === 'dark' ? 'dark' : 'light');
          document.documentElement.setAttribute('data-theme', sysTheme === 'dark' ? 'dark' : 'light');
        }
      });
    } else {
      setTheme(savedTheme === 'dark' ? 'dark' : 'light');
      setThemeMode(savedTheme);
      document.documentElement.setAttribute('data-theme', savedTheme);
    }
  }, [startupDone, status]);

  // Status polling (only after startup)
  useEffect(() => {
    if (!startupDone) return;
    let alive = true;
    const poll = async () => {
      const data = await api('GET', '/api/status');
      if (!alive) return;
      if (data) {
        if (failCountRef.current > 0) console.log('[App] Backend reconnected');
        failCountRef.current = 0;
        setBackendOnline(true);
        setStatus(data);
      } else {
        failCountRef.current++;
        if (failCountRef.current === 3) {
          setBackendOnline(false);
        }
      }
    };
    poll();
    const iv = setInterval(poll, 2000);
    statusIntervalRef.current = iv;
    return () => { alive = false; clearInterval(iv); };
  }, [startupDone]);

  // Health check polling
  useEffect(() => {
    if (!startupDone) return;
    let alive = true;
    const check = async () => {
      const h = await api('GET', '/api/health');
      if (!alive) return;
      setHealthWarn(h && h.overall && h.overall !== 'healthy' ? h : null);
    };
    check();
    const iv = setInterval(check, 30000);
    return () => { alive = false; clearInterval(iv); };
  }, [startupDone]);

  // Update available notification (from backend startup check)
  useEffect(() => {
    if (!startupDone) return;
    const unsub = window.go?.main?.App?.EventsOn
      ? null
      : null;
    const handler = (data) => {
      if (data?.component && data?.latest) {
        const name = data.component === 'ZPUI' ? 'ZPUI' : t('nav.zapret');
        showToast(t('toast.updateAvailable', { name, version: data.latest }), 'info');
      }
    };
    if (window.runtime?.EventsOn) {
      window.runtime.EventsOn('update:available', handler);
    }
    return () => {
      if (window.runtime?.EventsOff) {
        window.runtime.EventsOff('update:available');
      }
    };
  }, [startupDone, showToast]);

  const handleStartupComplete = useCallback(() => {
    setStartupDone(true);
  }, []);

  // Show startup screen when not done
  if (!startupDone) {
    return (
      <>
        <StartupScreen onComplete={handleStartupComplete} />
        <Toast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} />
      </>
    );
  }

  const PageComponent = PAGES[activePage];

  const pageContent = backendOnline ? (
    <div className="main-area page-fade" key={activePage}>
        {PageComponent ? (
         <PageComponent status={status} showToast={showToast} onNavigate={setActivePage} />
       ) : null}
    </div>
  ) : (
    <div className="main-area">
      <OfflineScreen onRetry={async () => {
        const d = await api('GET', '/api/status');
        if (d) {
          setBackendOnline(true);
          failCountRef.current = 0;
          setStatus(d);
        } else {
          showToast(t('offline.backendUnavailable'), 'error');
        }
      }} />
    </div>
  );

  return (
    <div className="app">
       <Sidebar activePage={activePage} onNavigate={setActivePage} onOpenChecker={() => setCheckerOpen(true)} onAutoSelect={() => setAutoSelectOpen(true)} onOpenHealth={() => setHealthOpen(true)} healthWarn={healthWarn} zapretRunning={status?.zapret?.status === 'running'} />
      <div className="app-body">
        <Header
          status={status}
          onOpenLogs={() => setLogsOpen(true)}
          isDark={theme === 'dark'}
          onToggleTheme={toggleTheme}
          showToast={showToast}
          onNavigate={setActivePage}
        />
        {pageContent}
        <Footer status={status} />
      </div>
      <LogDrawer open={logsOpen} onClose={() => setLogsOpen(false)} />
      {checkerOpen && <ResourceChecker onClose={() => setCheckerOpen(false)} showToast={showToast} proxyRunning={status?.proxy?.running} />}
      {healthOpen && <HealthCheckModal onClose={() => setHealthOpen(false)} />}
      <AutoSelectModal open={autoSelectOpen} onClose={() => setAutoSelectOpen(false)} showToast={showToast} />
      <Toast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} />
    </div>
  );
}
