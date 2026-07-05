import { useState, useEffect, useCallback, useRef } from 'react';
import Sidebar from './components/layout/Sidebar';
import Footer from './components/layout/Footer';
import Toast from './components/feedback/Toast';
import LogDrawer from './components/navigation/LogDrawer';
import OfflineScreen from './components/feedback/OfflineScreen';
import ResourceChecker from './components/ResourceChecker';
import HealthCheckModal from './components/HealthCheckModal';
import AutoSelectModal from './components/AutoSelectModal';
import SetupWizard from './components/SetupWizard';
import DashboardPage from './pages/DashboardPage';
import ZapretPage from './pages/ZapretPage';
import SettingsPage from './pages/SettingsPage';
import MonitorPage from './pages/MonitorPage';
import ProxyPage from './pages/ProxyPage';
import XboxDnsPage from './pages/XboxDnsPage';
import { api } from './api';
import { useT } from './i18n';
import { usePolling } from './hooks/usePolling';
import { useUpdateCheck, checkZpuiUpdate, checkZapretUpdate, shouldNotifyZpui, shouldNotifyZapret, setZpuiCheck, setZapretCheck } from './hooks/useUpdateCheck';
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

  const showToast = useCallback((msg, type) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type }]);
  }, []);

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  // Единая точка сохранения темы в конфиг бэкенда.
  const saveThemeConfig = useCallback((themeValue) => {
    api('POST', '/api/config', { theme: themeValue });
  }, []);

  const toggleTheme = useCallback(() => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    document.documentElement.setAttribute('data-theme', next);
    // При выходе из режима "system" переходим на явную тему.
    if (themeMode === 'system') {
      setThemeMode('manual');
    }
    saveThemeConfig(next);
  }, [theme, themeMode, saveThemeConfig]);

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
  const pollStatus = async () => {
    if (!startupDone) return;
    const data = await api('GET', '/api/status');
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
  usePolling(pollStatus, startupDone ? 2000 : 0);

  // Health check polling
  const pollHealth = async () => {
    if (!startupDone) return;
    const h = await api('GET', '/api/health');
    setHealthWarn(h && h.overall && h.overall !== 'healthy' ? h : null);
  };
  usePolling(pollHealth, startupDone ? 30000 : 0);

  // Update checks: initial delayed + hourly periodic
  useEffect(() => {
    if (!startupDone) return;
    const initId = setTimeout(() => {
      checkZpuiUpdate();
      checkZapretUpdate();
    }, 2000);
    const hourlyId = setInterval(() => {
      checkZpuiUpdate();
      checkZapretUpdate();
    }, 3600000);
    return () => { clearTimeout(initId); clearInterval(hourlyId); };
  }, [startupDone]);

  // Update available notification (dedup by version) + files-missing
  useEffect(() => {
    if (!startupDone) return;
    const handler = (data) => {
      if (!data?.latest) return;
      if (data.component === 'ZPUI') {
        setZpuiCheck({ state: 'available', current: data.current, latest: data.latest });
        if (shouldNotifyZpui(data.latest)) {
          showToast(t('toast.updateAvailable', { name: 'ZPUI', version: data.latest }), 'info');
        }
      } else if (data.component === 'zapret') {
        setZapretCheck({ state: 'available', current: data.current, latest: data.latest });
        if (shouldNotifyZapret(data.latest)) {
          showToast(t('toast.updateAvailable', { name: t('nav.zapret'), version: data.latest }), 'info');
        }
      }
    };
    const filesHandler = (data) => {
      if (data?.missing?.length > 0) {
        showToast(t('toast.filesMissing', { count: data.missing.length }), 'warning');
      }
    };
    if (window.runtime?.EventsOn) {
      window.runtime.EventsOn('update:available', handler);
      window.runtime.EventsOn('zapret:files-missing', filesHandler);
    }
    return () => {
      if (window.runtime?.EventsOff) {
        window.runtime.EventsOff('update:available');
        window.runtime.EventsOff('zapret:files-missing');
      }
    };
  }, [startupDone, showToast, t]);

  const handleStartupComplete = useCallback(() => {
    setStartupDone(true);
  }, []);

  const handleWizardCancel = useCallback(() => {
    // Exit the app - called when user chooses Cancel on third-party warning
    if (window.runtime?.WindowClose) {
      window.runtime.WindowClose();
    } else if (window.runtime?.Quit) {
      window.runtime.Quit();
    }
  }, []);

  // Show startup screen when not done
  if (!startupDone) {
    return (
      <>
        <SetupWizard onComplete={handleStartupComplete} onCancel={handleWizardCancel} />
        <Toast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} />
      </>
    );
  }

  const PageComponent = PAGES[activePage];

  const pageContent = backendOnline ? (
    <div className="main-area page-fade" key={activePage}>
        {PageComponent ? (
         <PageComponent status={status} showToast={showToast} onNavigate={setActivePage} onOpenLogs={() => setLogsOpen(true)} />
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
       <Sidebar
         activePage={activePage}
         onNavigate={setActivePage}
         onOpenChecker={() => setCheckerOpen(true)}
         onAutoSelect={() => setAutoSelectOpen(true)}
         onOpenHealth={() => setHealthOpen(true)}
         healthWarn={healthWarn}
         status={status}
         showToast={showToast}
         onOpenLogs={() => setLogsOpen(true)}
         isDark={theme === 'dark'}
         onToggleTheme={toggleTheme}
       />
      <div className="app-body">
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
