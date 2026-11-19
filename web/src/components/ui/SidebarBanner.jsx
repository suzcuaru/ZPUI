import { useEffect, useState } from 'react';
import { api, openExternal } from '../../api';
import { useT } from '../../i18n';
import { X, Megaphone } from 'lucide-react';

// SidebarBanner — рекламный баннер в нижней части боковой панели (плашка,
// HTML или картинка/GIF). Закрытие: show_once → больше не показывать
// (localStorage), иначе — до перезапуска приложения.
// В свёрнутой панели (окна уже 1300px) плашке нет места: показывается
// иконка-мегафон, при наведении — поповер с баннером.
const DISMISS_KEY = 'zpui-sidebar-banner-dismissed';
const EXPANDED_MQ = '(min-width: 1300px)';

function useSidebarExpanded() {
  const [expanded, setExpanded] = useState(() => {
    try { return window.matchMedia(EXPANDED_MQ).matches; } catch { return true; }
  });
  useEffect(() => {
    let mq;
    try { mq = window.matchMedia(EXPANDED_MQ); } catch { return undefined; }
    const onChange = (e) => setExpanded(e.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);
  return expanded;
}

export default function SidebarBanner() {
  const { t } = useT();
  const expanded = useSidebarExpanded();
  const [banner, setBanner] = useState(null);
  const [hidden, setHidden] = useState(() => {
    try { return sessionStorage.getItem('zpui-sidebar-banner-hidden') === '1'; } catch { return false; }
  });
  const [miniOpen, setMiniOpen] = useState(false);

  useEffect(() => {
    let alive = true;
    api('GET', '/api/banner').then((r) => {
      if (!alive || !r || r.error) return;
      const placement = r.placement || 'profile';
      if (placement !== 'sidebar' && placement !== 'both') return;
      // «1 раз»: этот баннер уже закрывали — не показываем никогда
      try {
        if (r.show_once && localStorage.getItem(DISMISS_KEY) === String(r.id)) return;
      } catch { /* хранилище недоступно — показываем */ }
      setBanner(r);
    }).catch(() => {});
    return () => { alive = false; };
  }, []);

  if (!banner || hidden) return null;

  const close = () => {
    if (banner.id) {
      if (banner.show_once) {
        try { localStorage.setItem(DISMISS_KEY, String(banner.id)); } catch { /* нет хранилища */ }
      }
    }
    try { sessionStorage.setItem('zpui-sidebar-banner-hidden', '1'); } catch { /* нет хранилища */ }
    setHidden(true);
  };

  const body = (
    <>
      {banner.kind === 'html' ? (
        <div className="sb-banner-html" dangerouslySetInnerHTML={{ __html: banner.html || '' }} />
      ) : banner.kind === 'text' ? (
        <div className="sb-banner-text">{banner.text}</div>
      ) : (
        <img
          className="sb-banner-img"
          src={banner.image}
          alt={banner.title || 'banner'}
          style={banner.h > 0 ? { height: banner.h } : null}
          onClick={() => banner.link && openExternal(banner.link)}
        />
      )}
      {banner.link && banner.kind !== 'image' && (
        <a
          className="sb-banner-link"
          onClick={() => banner.link && openExternal(banner.link)}
        >
          {t('sidebar.bannerOpen')}
        </a>
      )}
    </>
  );

  // Свёрнутая панель: иконка-мегафон, баннер — в поповере по наведению.
  if (!expanded) {
    return (
      <div className="sb-mini" onMouseEnter={() => setMiniOpen(true)} onMouseLeave={() => setMiniOpen(false)}>
        <button
          className="sidebar-footer-btn sb-mini-btn"
          aria-label={banner.title || t('sidebar.bannerTip')}
          aria-expanded={miniOpen}
        >
          <Megaphone size={18} strokeWidth={2} />
        </button>
        {miniOpen && (
          <div className="sb-mini-pop">
            <div className="sb-banner" style={banner.w > 0 ? { maxWidth: banner.w } : null}>
              <button className="sb-banner-x" aria-label={t('common.close')} onClick={close}>
                <X size={12} strokeWidth={2.4} />
              </button>
              {body}
            </div>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="sb-banner" style={banner.w > 0 ? { maxWidth: banner.w } : null}>
      <button className="sb-banner-x" aria-label={t('common.close')} onClick={close}>
        <X size={12} strokeWidth={2.4} />
      </button>
      {body}
    </div>
  );
}
