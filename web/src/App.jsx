import { useState, useEffect, useCallback, useRef } from 'react';
import Sidebar from './components/layout/Sidebar';
import Footer from './components/layout/Footer';
import Toast from './components/feedback/Toast';
import ModulesPage from './pages/ModulesPage';
import SettingsPage from './pages/SettingsPage';
import StartupScreen from './components/StartupScreen';
import { api } from './api';
import { useT } from './i18n';
import { usePolling } from './hooks/usePolling';
import './styles/index.css';

const BASE_W = 1280;
const BASE_H = 720;
const MAX_SCALE = 1.25;

export default function App() {
  const { t } = useT();
  const [activePage, setActivePage] = useState('modules');
  const [status, setStatus] = useState(null);
  const [modules, setModules] = useState([]);
  const [config, setConfig] = useState(null);
  const [toasts, setToasts] = useState([]);
  const [theme, setTheme] = useState('dark');
  const [startupState, setStartupState] = useState(null);
  const [uiRegs, setUiRegs] = useState([]);
  const [scale, setScale] = useState(1);
  const themeInitRef = useRef(false);

  useEffect(() => {
    const handleResize = () => {
      const w = window.innerWidth;
      const h = window.innerHeight;
      const raw = Math.min(w / BASE_W, h / BASE_H);
      setScale(Math.max(1, Math.min(raw, MAX_SCALE)));
    };
    handleResize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const showToast = useCallback((msg, type) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type }]);
  }, []);

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const loadModules = useCallback(async () => {
    const data = await api('GET', '/api/modules');
    if (Array.isArray(data)) setModules(data);
  }, []);

  const pollStatus = useCallback(async () => {
    const data = await api('GET', '/api/status');
    if (data) setStatus(data);
  }, []);
  usePolling(pollStatus, 3000);

  const pollModules = useCallback(async () => {
    await loadModules();
  }, [loadModules]);
  usePolling(pollModules, 3000);

  const pollStartup = useCallback(async () => {
    const data = await api('GET', '/api/startup/state');
    if (data) setStartupState(data);
  }, []);
  usePolling(pollStartup, 500);

  const pollUI = useCallback(async () => {
    const data = await api('GET', '/api/ui/registrations');
    if (data && data.items) setUiRegs(data.items);
  }, []);
  usePolling(pollUI, 3000);

  useEffect(() => {
    (async () => {
      const cfg = await api('GET', '/api/config');
      if (cfg) setConfig(cfg);
      await loadModules();
    })();
  }, [loadModules]);

  useEffect(() => {
    if (!status || themeInitRef.current) return;
    themeInitRef.current = true;
    const saved = status.mod?.theme || 'system';
    if (saved === 'system') {
      api('GET', '/api/system-theme').then(sysTheme => {
        const resolved = sysTheme === 'dark' ? 'dark' : 'light';
        setTheme(resolved);
        document.documentElement.setAttribute('data-theme', resolved);
      });
    } else {
      setTheme(saved);
      document.documentElement.setAttribute('data-theme', saved);
    }
  }, [status]);

  const toggleTheme = useCallback(() => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    document.documentElement.setAttribute('data-theme', next);
    api('POST', '/api/config', { theme: next });
  }, [theme]);

  const renderPage = () => {
    if (activePage === 'settings') {
      return <SettingsPage config={config} status={status} onConfigChange={(p) => setConfig(c => ({ ...c, ...p }))} showToast={showToast} />;
    }
    if (activePage.startsWith('mod:')) {
      const id = activePage.slice(4);
      const mod = modules.find(m => m.id === id);
      if (mod) {
        return (
          <>
            <div className="page-title">{mod.name || mod.id}</div>
            <ModulesPage modules={[mod]} showToast={showToast} onChange={loadModules} />
          </>
        );
      }
    }
    return <ModulesPage modules={modules} showToast={showToast} onChange={loadModules} />;
  };

  const scaled = scale > 1;
  const s = scaled ? scale : 1;

  if (startupState && startupState.stage && startupState.stage !== 'done') {
    return (
      <div className="startup-root" style={{ transform: `scale(${s})` }}>
        <StartupScreen state={startupState} />
      </div>
    );
  }

  return (
    <div className="app" style={{ transform: scaled ? `scale(${s})` : undefined }}>
      <Sidebar
        activePage={activePage}
        onNavigate={setActivePage}
        modules={modules}
        onToggleTheme={toggleTheme}
        isDark={theme === 'dark'}
      />
      <div className="app-body">
        <div className="main-area page-fade" key={activePage}>
          {renderPage()}
        </div>
        <Footer status={status} uiRegs={uiRegs} />
      </div>
      <Toast toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
