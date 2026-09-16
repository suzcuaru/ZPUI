import { useState, useEffect } from 'react';
import {
  Rocket, ShieldCheck, Network, Gamepad2, Activity,
  Settings, Wrench, PanelLeft, Send,
  Lightbulb, AlertTriangle, Info, ChevronRight,
  GitBranch, Download, Bug, Smartphone, FileText, BatteryCharging,
  LayoutDashboard,
} from 'lucide-react';
import { openExternal, api } from '../api';
import { useT } from '../i18n';

const GITHUB_URL = 'https://github.com/suzcuaru/ZPUI';
const TG_CHANNEL_DOMAIN = 'zpui_app';
const TG_GROUP_DOMAIN = 'zpui_chat';

const openTelegram = (domain) => { api('POST', '/api/telegram/open', { domain }); };

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

const SECTIONS = [
  { id: 'quickstart', icon: Rocket },
  { id: 'dashboard', icon: LayoutDashboard },
  { id: 'zapret', icon: ShieldCheck },
  { id: 'proxy', icon: Network },
  { id: 'monitor', icon: Activity },
  { id: 'xboxdns', icon: Gamepad2 },
  { id: 'power', icon: BatteryCharging },
  { id: 'reports', icon: FileText },
  { id: 'settings', icon: Settings },
  { id: 'troubleshoot', icon: Wrench },
  { id: 'sidebar', icon: PanelLeft },
  { id: 'about', icon: Info },
];

function LinkRow({ t }) {
  return (
    <div className="docs-link-row">
      <button className="docs-link-btn" onClick={() => openTelegram(TG_CHANNEL_DOMAIN)}>
        <Send size={13} strokeWidth={2} />
        {t('docs.links.channel')}
      </button>
      <button className="docs-link-btn" onClick={() => openTelegram(TG_GROUP_DOMAIN)}>
        <Smartphone size={13} strokeWidth={2} />
        {t('docs.links.group')}
      </button>
      <button className="docs-link-btn" onClick={() => openExternal(GITHUB_URL)}>
        <GitBranch size={13} strokeWidth={2} />
        GitHub
      </button>
      <button className="docs-link-btn" onClick={() => openExternal(`${GITHUB_URL}/releases`)}>
        <Download size={13} strokeWidth={2} />
        {t('docs.links.releases')}
      </button>
      <button className="docs-link-btn" onClick={() => openExternal(`${GITHUB_URL}/issues`)}>
        <Bug size={13} strokeWidth={2} />
        {t('docs.links.issues')}
      </button>
    </div>
  );
}

function DocBlock({ b, t, version }) {
  const v = (s) => String(s).replace(/\{version\}/g, version || t('docs.versionLoading'));
  if (b.lead !== undefined) return <p className="docs-lead">{v(b.lead)}</p>;
  if (b.h !== undefined) return <h3 className="docs-h3">{b.h}</h3>;
  if (b.list) return <BulletList items={b.list.map(v)} />;
  if (b.steps) return <StepList items={b.steps.map(v)} />;
  if (b.callout) return <Callout type={b.callout}>{v(b.text)}</Callout>;
  if (b.links) return <LinkRow t={t} />;
  return null;
}

export default function DocsPage() {
  const { t } = useT();
  const [activeId, setActiveId] = useState(SECTIONS[0].id);
  const [version, setVersion] = useState('');
  const section = SECTIONS.find(s => s.id === activeId) || SECTIONS[0];
  const SectionIcon = section.icon;
  const blocks = t(`docs.sections.${section.id}.blocks`) || [];

  useEffect(() => {
    api('GET', '/api/versions').then(d => { if (d?.zpui) setVersion(d.zpui); });
  }, []);

  return (
    <div className="docs-page">
      <div className="docs-shell">
        <aside className="docs-rail">
          {SECTIONS.map(s => {
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
        </aside>
        <section className="docs-content" key={activeId}>
          <div className="docs-content-head">
            <span className="docs-content-icon"><SectionIcon size={22} strokeWidth={2} /></span>
            <h2 className="docs-content-title">{t(`docs.sections.${section.id}.title`)}</h2>
          </div>
          <div className="docs-content-body">
            {blocks.map((b, i) => <DocBlock key={i} b={b} t={t} version={version} />)}
          </div>
        </section>
      </div>
    </div>
  );
}
