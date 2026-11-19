import { useState, useEffect, Fragment } from 'react';
import {
  Rocket, ShieldCheck, Globe, Server, Activity,
  Settings, Wrench, Send,
  Lightbulb, AlertTriangle, Info, ChevronRight,
  GitBranch, Download, Bug, Smartphone, MousePointerClick,
  LayoutDashboard, Waypoints, Cpu,
} from 'lucide-react';
import { openExternal, api } from '../api';
import { useT } from '../i18n';
import { openTelegram as openTelegramShared } from '../utils';
import FeedbackModal from '../components/feedback/FeedbackModal';
import { useToast } from '../context/ToastContext';
import { AlertCircle } from 'lucide-react';

const GITHUB_URL = 'https://github.com/suzcuaru/ZPUI';
const SITE_URL = 'https://zpui-life.ru';
const TG_CHANNEL_DOMAIN = 'zpui_app';
const TG_GROUP_DOMAIN = 'zpui_chat';

// Проекты, на которых построен ZPUI (секция «Благодарности»).
const CREDITS = [
  { name: 'Zapret', url: 'https://github.com/bol-van/zapret', key: 'docs.about.credits.zapret' },
  { name: 'zapret-discord-youtube', url: 'https://github.com/Flowseal/zapret-discord-youtube', key: 'docs.about.credits.zdy' },
  { name: 'tg-ws-proxy', url: 'https://github.com/Flowseal/tg-ws-proxy', key: 'docs.about.credits.tgws' },
];

// Каналы/группы открываются в ПРИЛОЖЕНИИ Telegram (tg://resolve?domain=…),
// фолбэк — https://t.me/… (общий хелпер в utils).
const openTelegram = (domain) => openTelegramShared(domain);

const C = {
  important: { cls: 'important', Icon: Lightbulb },
  warn: { cls: 'warn', Icon: AlertTriangle },
  info: { cls: 'info', Icon: Info },
};

function Callout({ type, children }) {
  const { cls, Icon } = C[type] || C.info;
  return (
    <div className={'docs-callout ' + cls}>
      <span className="docs-callout-icon"><Icon size={15} strokeWidth={2} /></span>
      <span>{children}</span>
    </div>
  );
}

function StepList({ items }) {
  return (
    <ol className="docs-steps">
      {items.map((it, i) => (
        <li key={i}><span className="docs-step-num">{i + 1}</span><span>{it}</span></li>
      ))}
    </ol>
  );
}

function BulletList({ items }) {
  return (
    <ul className="docs-list">
      {items.map((it, i) => (
        <li key={i}><ChevronRight size={13} strokeWidth={2.5} /><span>{it}</span></li>
      ))}
    </ul>
  );
}

const GROUPS = ['start', 'tabs', 'more'];

const SECTIONS = [
  { id: 'quickstart', icon: Rocket, group: 'start' },
  { id: 'dashboard', icon: LayoutDashboard, group: 'tabs' },
  { id: 'zapret', icon: ShieldCheck, group: 'tabs' },
  { id: 'engine', icon: Cpu, group: 'tabs' },
  { id: 'proxy', icon: Globe, group: 'tabs' },
  { id: 'dns', icon: Server, group: 'tabs' },
  { id: 'monitor', icon: Activity, group: 'tabs' },
  { id: 'settings', icon: Settings, group: 'tabs' },
  { id: 'troubleshoot', icon: Wrench, group: 'more' },
  { id: 'features', icon: MousePointerClick, group: 'more' },
  { id: 'mcp', icon: Waypoints, group: 'more' },
  { id: 'about', icon: Info, group: 'more' },
];

function LinkRow({ t }) {
  return (
    <div className="docs-link-row">
      <button className="btn docs-link-btn" onClick={() => openExternal(SITE_URL)}>
        <Globe size={13} strokeWidth={2} />
        {t('docs.links.site')}
      </button>
      <button className="btn docs-link-btn" onClick={() => openTelegram(TG_CHANNEL_DOMAIN)}>
        <Send size={13} strokeWidth={2} />
        {t('docs.links.channel')}
      </button>
      <button className="btn docs-link-btn" onClick={() => openTelegram(TG_GROUP_DOMAIN)}>
        <Smartphone size={13} strokeWidth={2} />
        {t('docs.links.group')}
      </button>
      <button className="btn docs-link-btn" onClick={() => openExternal(GITHUB_URL)}>
        <GitBranch size={13} strokeWidth={2} />
        GitHub
      </button>
      <button className="btn docs-link-btn" onClick={() => openExternal(`${GITHUB_URL}/releases`)}>
        <Download size={13} strokeWidth={2} />
        {t('docs.links.releases')}
      </button>
      <button className="btn docs-link-btn" onClick={() => openExternal(`${GITHUB_URL}/issues`)}>
        <Bug size={13} strokeWidth={2} />
        {t('docs.links.issues')}
      </button>
    </div>
  );
}

function CreditsRows({ t }) {
  return (
    <div className="docs-credits">
      {CREDITS.map((c) => (
        <div className="docs-credit" key={c.name}>
          <div className="docs-credit-txt">
            <span className="docs-credit-name">{c.name}</span>
            <span className="docs-credit-desc">{t(c.key)}</span>
          </div>
          <button className="btn docs-link-btn" onClick={() => openExternal(c.url)}>
            <GitBranch size={13} strokeWidth={2} />
            GitHub
          </button>
        </div>
      ))}
    </div>
  );
}

function OptionRows({ rows, v }) {
  return (
    <div className="docs-opts">
      {rows.map((r, i) => (
        <div className="docs-opt" key={i}>
          <span className="docs-opt-k">{v(r.k)}</span>
          <span className="docs-opt-v">{v(r.v)}</span>
        </div>
      ))}
    </div>
  );
}

function DocBlock({ b, t, version }) {
  const v = (s) => String(s).replace(/\{version\}/g, version || t('docs.versionLoading'));
  if (b.lead !== undefined) return <p className="docs-lead">{v(b.lead)}</p>;
  if (b.h !== undefined) return <h3 className="docs-h3">{b.h}</h3>;
  if (b.list) return <BulletList items={b.list.map(v)} />;
  if (b.steps) return <StepList items={b.steps.map(v)} />;
  if (b.rows) return <OptionRows rows={b.rows} v={v} />;
  if (b.callout) return <Callout type={b.callout}>{v(b.text)}</Callout>;
  if (b.links) return <LinkRow t={t} />;
  if (b.credits) return <CreditsRows t={t} />;
  return null;
}

export default function DocsPage() {
  const showToast = useToast();
  const { t } = useT();
  // Deep-link секции: подсказка кладётся в sessionStorage до навигации,
  // здесь читается один раз и стирается.
  const [activeId, setActiveId] = useState(() => {
    try {
      const v = sessionStorage.getItem('zpui-docs-section');
      if (v && SECTIONS.some((s) => s.id === v)) return v;
    } catch {}
    return SECTIONS[0].id;
  });
  const [version, setVersion] = useState('');
  const [fbOpen, setFbOpen] = useState(false);
  const section = SECTIONS.find(s => s.id === activeId) || SECTIONS[0];
  const SectionIcon = section.icon;
  const blocks = t(`docs.sections.${section.id}.blocks`) || [];

  useEffect(() => {
    try { sessionStorage.removeItem('zpui-docs-section'); } catch {}
    api('GET', '/api/versions').then(d => { if (d?.zpui) setVersion(d.zpui); });
  }, []);

  return (
    <div className="docs-page">
      <div className="docs-shell">
        <aside className="docs-rail">
          {GROUPS.map(g => (
            <Fragment key={g}>
              <span className="docs-rail-label">{t(`docs.groups.${g}`)}</span>
              {SECTIONS.filter(s => s.group === g).map(s => {
                const Icon = s.icon;
                const active = s.id === activeId;
                return (
                  <button
                    key={s.id}
                    className={'docs-rail-item' + (active ? ' active' : '')}
                    onClick={() => setActiveId(s.id)}
                    aria-current={active ? 'page' : undefined}
                  >
                    <Icon size={16} strokeWidth={2} />
                    <span>{t(`docs.sections.${s.id}.title`)}</span>
                  </button>
                );
              })}
              {g !== GROUPS[GROUPS.length - 1] && <span className="docs-rail-divider" />}
            </Fragment>
          ))}
        </aside>
        <section className="docs-content" key={activeId}>
          <div className="docs-content-head">
            <span className="docs-content-icon"><SectionIcon size={22} strokeWidth={2} /></span>
            <h2 className="docs-content-title">{t(`docs.sections.${section.id}.title`)}</h2>
            <button className="pf-back pf-rotate docs-fb-btn" onClick={() => setFbOpen(true)}>
              <AlertCircle size={12} strokeWidth={2} />
              {t('feedback.title')}
            </button>
          </div>
          <div className="docs-content-body">
            {blocks.map((b, i) => <DocBlock key={i} b={b} t={t} version={version} />)}
          </div>
        </section>
      </div>
      <FeedbackModal open={fbOpen} onClose={() => setFbOpen(false)} />
    </div>
  );
}
