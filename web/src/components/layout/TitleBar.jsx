import { useT } from '../../i18n';
import { Sun, Moon, Monitor, ArrowDownCircle, CircleUserRound } from 'lucide-react';

// Единый стиль глифов кнопок управления окном: 16px, stroke 1.5,
// скруглённые концы — в духе иконок сайдбара (тонкая линия, без заливок).
function CapGlyph({ children }) {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {children}
    </svg>
  );
}
const GLYPH_CLOSE = <path d="M6.5 6.5l11 11M17.5 6.5l-11 11" />;
const GLYPH_MINIMIZE = <path d="M5.5 12h13" />;
const GLYPH_MAXIMIZE = <rect x="5.75" y="5.75" width="12.5" height="12.5" rx="2.5" />;
// «Восстановить»: две рамки (задняя — частичная), как у Windows.
const GLYPH_RESTORE = (
  <>
    <rect x="4.75" y="8.75" width="10.5" height="10.5" rx="2" />
    <path d="M9 4.75h8A2.25 2.25 0 0 1 19.25 7v8" />
  </>
);

// user: { state: 'ok'|'offline'|'none', name, subLine }
// Клик по иконке открывает боковую панель профиля (ProfileDrawer) — вход,
// аккаунт и текущая подписка живут там, отдельного поповера больше нет.
function UserBadge({ user, onUserClick }) {
  const { t } = useT();
  const state = user?.state || 'none';
  const stateLabel = state === 'ok'
    ? t('profile.stateOk')
    : state === 'offline' ? t('profile.stateOffline') : t('profile.stateNone');

  return (
    <button
      className={'tbar-user-btn is-' + state}
      onClick={onUserClick}
      data-tooltip={stateLabel}
      data-tooltip-pos="bottom"
      aria-label={t('profile.drawerTitle')}
    >
      <CircleUserRound size={17} strokeWidth={1.8} />
    </button>
  );
}

export default function TitleBar({
  version, arch, theme, themeSetting, onToggleTheme, maximized, onToggleMaximize, onMinimize, onClose,
  updateInfo, onOpenUpdate, user, onUserClick,
}) {
  const { t } = useT();
  // Трёхпозиционный переключатель: системная / светлая / тёмная.
  // Кастомная тема показывает свою базу (первый клик возвращает на неё).
  const setting = themeSetting === 'system' || themeSetting === 'light' || themeSetting === 'dark'
    ? themeSetting
    : (theme === 'dark' ? 'dark' : 'light');
  const ThemeIcon = setting === 'system' ? Monitor : setting === 'light' ? Sun : Moon;
  const themeLabel = setting === 'system' ? t('settings.systemTheme')
    : setting === 'light' ? t('settings.lightTheme')
      : t('settings.darkTheme');
  return (
    // Безрамочное окно Wails: тайтлбар — зона перетаскивания (--wails-draggable).
    // Кнопки внутри — интерактивные (drag не наследуется на детей с кнопками).
    <div className="tbar" style={{ '--wails-draggable': 'drag' }}>
      <span className="tbar-logo">ZPUI</span>
      {updateInfo && (
        <button className="tbar-update" onClick={onOpenUpdate} aria-label={t('stand.updateAvailable', { v: updateInfo.latest })}>
          <ArrowDownCircle size={14} strokeWidth={2} />
          <span className="tbar-update-ver mono">{updateInfo.latest}</span>
          <span className="tbar-update-label">{t('stand.updateAvailableShort')}</span>
        </button>
      )}
      <div className="tbar-right">
        <span className="tbar-ver mono">{version ? `v${version}${arch ? ` (${arch})` : ''}` : ''}</span>
        <UserBadge user={user} onUserClick={onUserClick} />
        <button
          className="tbar-btn"
          onClick={onToggleTheme}
          aria-label={themeLabel}
        >
          <ThemeIcon size={14} strokeWidth={2} />
        </button>
        <span className="tbar-sep" />
        <div className="tbar-caps">
          {/* Управление окном — беззвучно (data-sound="off"): системные
              действия не должны щёлкать интерфейсом. */}
          <button className="tbar-cap" data-sound="off" aria-label={t('stand.minimize')} onClick={onMinimize}>
            <CapGlyph>{GLYPH_MINIMIZE}</CapGlyph>
          </button>
          <button className="tbar-cap" data-sound="off" onClick={onToggleMaximize} aria-label={t(maximized ? 'stand.restore' : 'stand.maximize')}>
            <CapGlyph>{maximized ? GLYPH_RESTORE : GLYPH_MAXIMIZE}</CapGlyph>
          </button>
          <button className="tbar-cap tbar-cap-close" data-sound="off" aria-label={t('stand.close')} onClick={onClose}>
            <CapGlyph>{GLYPH_CLOSE}</CapGlyph>
          </button>
        </div>
      </div>
    </div>
  );
}
