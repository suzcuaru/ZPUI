import { useState, useEffect, useCallback, useRef } from 'react';
import Header from './components/layout/Header';
import Sidebar from './components/layout/Sidebar';
import Footer from './components/layout/Footer';
import Toast from './components/feedback/Toast';
import LogDrawer from './components/navigation/LogDrawer';
import OfflineScreen from './components/feedback/OfflineScreen';
import MonitorPage from './pages/MonitorPage';
import DevicesPage from './pages/DevicesPage';
import FiltersPage from './pages/FiltersPage';
import DiagnosticsPage from './pages/DiagnosticsPage';
import GeneralPage from './pages/GeneralPage';
import AboutPage from './pages/AboutPage';
import { api } from './api';
import './styles/index.css';

const PAGES = {
  monitor: MonitorPage,
  devices: DevicesPage,
  filters: FiltersPage,
  diagnostics: DiagnosticsPage,
  general: GeneralPage,
  about: AboutPage,
};

export default function App() {
  const [activePage, setActivePage] = useState('monitor');
  const [status, setStatus] = useState(null);
  const [toasts, setToasts] = useState([]);
  const [logsOpen, setLogsOpen] = useState(false);
  const [backendOnline, setBackendOnline] = useState(true);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [isDark, setIsDark] = useState(() => {
    return document.documentElement.getAttribute('data-theme') !== 'light';
  });
  const failCountRef = useRef(0);

  const showToast = useCallback((msg, type) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type }]);
    if (type !== 'error') {
      setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 3000);
    }
  }, []);

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const toggleTheme = useCallback(() => {
    const next = !isDark;
    setIsDark(next);
    document.documentElement.setAttribute('data-theme', next ? 'dark' : 'light');
  }, [isDark]);

  useEffect(() => {
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
          console.error('[App] Backend unreachable (3 failures)');
          setBackendOnline(false);
        }
      }
    };
    console.log('[App] Initializing ZPUI');
    poll();
    const iv = setInterval(poll, 2000);
    return () => { alive = false; clearInterval(iv); };
  }, []);

  const PageComponent = PAGES[activePage];

  const pageContent = backendOnline ? (
    <div className="main-area page-fade" key={activePage}>
      {PageComponent && <PageComponent status={status} showToast={showToast} />}
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
          showToast('Бэкенд по-прежнему недоступен', 'error');
        }
      }} />
    </div>
  );

  return (
    <div className="app">
      {!sidebarCollapsed && (
        <Sidebar
          activePage={activePage}
          onNavigate={setActivePage}
          onOpenLogs={() => setLogsOpen(true)}
        />
      )}
      <div className="app-body">
        <Header
          status={status}
          onOpenLogs={() => setLogsOpen(true)}
          isDark={isDark}
          onToggleTheme={toggleTheme}
          collapsed={sidebarCollapsed}
          onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
          showToast={showToast}
          onNavigate={setActivePage}
        />
        {pageContent}
        <Footer status={status} />
      </div>
      <LogDrawer open={logsOpen} onClose={() => setLogsOpen(false)} />
      <Toast toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
