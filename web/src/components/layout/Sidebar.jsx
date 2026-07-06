import { useT } from '../../i18n';

const ICONS = {
  sun: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.2" y1="4.2" x2="5.6" y2="5.6"/><line x1="18.4" y1="18.4" x2="19.8" y2="19.8"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.2" y1="19.8" x2="5.6" y2="18.4"/><line x1="18.4" y1="5.6" x2="19.8" y2="4.2"/></svg>,
  moon: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/></svg>,
  modules: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/></svg>,
  settings: <svg viewBox="0 0 24 24" fill="currentColor"><path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58a.49.49 0 00.12-.61l-1.92-3.32a.49.49 0 00-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54A.484.484 0 0014.12 2h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96a.49.49 0 00-.59.22L2.94 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58a.49.49 0 00-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/></svg>,
};

export default function Sidebar({ activePage, onNavigate, modules, onToggleTheme, isDark }) {
  const { t } = useT();
  const sidebarMods = (modules || []).filter(m => (m.placements || []).includes('sidebar'));

  return (
    <aside className="sidebar">
      <nav className="sidebar-nav">
        <button
          className={'sidebar-nav-item' + (activePage === 'modules' ? ' active' : '')}
          onClick={() => onNavigate('modules')}
          aria-label={t('nav.modules')}
        >
          {ICONS.modules}
        </button>
        {sidebarMods.map(m => (
          <button
            key={m.id}
            className={'sidebar-nav-item' + (activePage === 'mod:' + m.id ? ' active' : '')}
            onClick={() => onNavigate('mod:' + m.id)}
            aria-label={m.name}
            title={m.name}
            style={{ position: 'relative' }}
          >
            <span style={{ fontSize: 13, fontWeight: 700, color: 'inherit' }}>
              {(m.name || m.id || '?').charAt(0).toUpperCase()}
            </span>
            <span className={'sidebar-mod-dot' + (m.state === 'running' ? '' : ' off')} />
          </button>
        ))}
      </nav>
      <div className="sidebar-spacer" />
      <div className="sidebar-divider" />
      <button
        className={'sidebar-nav-item' + (activePage === 'settings' ? ' active' : '')}
        onClick={() => onNavigate('settings')}
        aria-label={t('nav.settings')}
      >
        {ICONS.settings}
      </button>
      <button className="sidebar-footer-btn" onClick={onToggleTheme} aria-label="theme">
        {isDark ? ICONS.sun : ICONS.moon}
      </button>
    </aside>
  );
}
