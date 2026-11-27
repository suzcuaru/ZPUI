// Единый шаблон статус-героя вкладок (DNS / Telegram-мост / SOCKS5).
// CSS: .dns-hero-* в styles/profile.css — классы исторические, общий шаблон.
export default function StatusHero({ icon, title, desc, on, children }) {
  return (
    <div className={'dns-hero' + (on ? ' on' : '')}>
      <div className="dns-hero-left">
        <div className={'dns-hero-icon' + (on ? ' on' : '')}>{icon}</div>
        <div className="dns-hero-info">
          <span className="dns-hero-title">{title}</span>
          <span className={'dns-hero-desc' + (on ? ' on' : '')}>{desc}</span>
        </div>
      </div>
      <div className="dns-hero-state">{children}</div>
    </div>
  );
}
