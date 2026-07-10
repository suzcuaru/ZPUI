import { useState } from 'react';
import {
  Sparkles, LayoutDashboard, ShieldCheck, Network, Gamepad2, Activity,
  Settings, PanelLeft, Search, Stethoscope, Zap,
  Lightbulb, AlertTriangle, Info, ArrowRight, GitBranch, ExternalLink,
} from 'lucide-react';
import { openExternal } from '../api';

const C = {
  important: { cls: 'important', Icon: Lightbulb },
  warn: { cls: 'warn', Icon: AlertTriangle },
  info: { cls: 'info', Icon: Info },
};

function Block({ b }) {
  if (b.type === 'list') {
    return (
      <ul className="docs-list">
        {b.items.map((it, i) => (
          <li key={i}><ArrowRight size={14} strokeWidth={2.5} /><span>{it}</span></li>
        ))}
      </ul>
    );
  }
  if (b.type === 'steps') {
    return (
      <ol className="docs-steps">
        {b.items.map((it, i) => (
          <li key={i}><span className="docs-step-num">{i + 1}</span><span>{it}</span></li>
        ))}
      </ol>
    );
  }
  const { cls, Icon } = C[b.type] || C.info;
  return (
    <div className={'docs-callout ' + cls}>
      <span className="docs-callout-icon"><Icon size={16} strokeWidth={2} /></span>
      <span>{b.text}</span>
    </div>
  );
}

const GITHUB_URL = 'https://github.com/suzcuaru/ZPUI';

function extLink(url) { openExternal(url); }

const SECTIONS = [
  {
    id: 'start', icon: Sparkles, title: 'Первый запуск', img: 'start',
    intro: 'ZPUI — программа для обхода блокировок Discord, YouTube и других сайтов. При первом запуске мастер автоматически проверит систему, скачает Zapret и подберёт рабочую стратегию.',
    blocks: [
      { type: 'steps', items: [
        'Скачайте установщик или портативную версию со страницы релизов.',
        'Запустите ZPUI от имени администратора.',
        'Мастер проверит систему на конфликты (сторонний Zapret, VPN).',
        'Zapret скачается автоматически, если его нет.',
        'Автовыбор протестирует стратегии и применит лучшую.',
      ]},
      { type: 'important', text: 'Без прав администратора обход работать не будет — драйвер WinDivert требует повышенных привилегий.' },
      { type: 'warn', text: 'Если на ПК уже установлен сторонний Zapret или активен VPN — мастер предложит их отключить. Иначе они будут конфликтовать.' },
    ],
  },
  {
    id: 'dashboard', icon: LayoutDashboard, title: 'Дашборд', img: 'dashboard',
    intro: 'Главная страница: обзор доступности ресурсов, статус компонентов и график за 24 часа.',
    blocks: [
      { type: 'list', items: [
        'Зелёный индикатор — ресурс доступен, жёлтый — перебои, красный — недоступен.',
        'Кнопка «Проверить сейчас» запускает внеплановую проверку всех ресурсов.',
        'График показывает историю доступности за последний час.',
      ]},
    ],
  },
  {
    id: 'zapret', icon: ShieldCheck, title: 'Запрет (Zapret)', img: 'zapret',
    intro: 'Главный модуль обхода DPI. Здесь выбирается стратегия, настраиваются фильтры и редактируются списки доменов. Включение и выключение — точкой на иконке в боковой панели.',
    blocks: [
      { type: 'steps', items: [
        'Убедитесь, что Zapret запущен (зелёная точка в строке статуса).',
        'Если обход не работает — нажмите «Автовыбор стратегии» (иконка молнии в боковой панели).',
        'Автовыбор проверит все стратегии и применит лучшую.',
      ]},
      { type: 'important', text: 'Стратегия подбирается под вашего провайдера. То, что работает у одного, может не работать у другого.' },
      { type: 'warn', text: 'Не запускайте две стратегии одновременно и не оставляйте сторонний Zapret — это приведёт к конфликту и потере сети.' },
    ],
  },
  {
    id: 'autoselect', icon: Zap, title: 'Автовыбор стратегии', img: 'autoselect',
    intro: 'Автоматически перебирает все стратегии Zapret, проверяет каждую на реальных ресурсах и выбирает лучшую.',
    blocks: [
      { type: 'list', items: [
        'Зелёный — стратегия работает отлично.',
        'Жёлтый — работает частично.',
        'Красный — не работает.',
        'Серый — ошибка запуска.',
      ]},
      { type: 'info', text: 'Тест может занять несколько минут. Не закрывайте окно до завершения.' },
    ],
  },
  {
    id: 'proxy', icon: Network, title: 'Прокси (SOCKS5)', img: 'proxy',
    intro: 'Локальный SOCKS5-прокси для раздачи обхода на другие устройства: телефон, планшет, телевизор. Адрес: socks5://IP_компьютера:1080.',
    blocks: [
      { type: 'steps', items: [
        'Включите Zapret (обход должен работать).',
        'Включите прокси точкой на иконке в боковой панели.',
        'На телефоне в настройках Wi-Fi укажите прокси: IP компьютера и порт 1080.',
        'Проверьте, что заблокированные сайты открываются на телефоне.',
      ]},
      { type: 'info', text: 'Логин и пароль можно оставить пустыми. IP компьютера можно посмотреть на вкладке «Монитор».' },
    ],
  },
  {
    id: 'xboxdns', icon: Gamepad2, title: 'Xbox DNS', img: 'xboxdns',
    intro: 'Смена DNS на серверы xbox-dns.ru для обхода блокировок на уровне DNS. Применяется ко всем активным сетевым адаптерам.',
    blocks: [
      { type: 'steps', items: [
        'Включите DNS переключателем.',
        'На консоли в настройках сети укажите IP компьютера как DNS-сервер.',
      ]},
      { type: 'important', text: 'После выключения DNS возвращаются к значениям провайдера автоматически.' },
    ],
  },
  {
    id: 'monitor', icon: Activity, title: 'Монитор', img: 'monitor',
    intro: 'Сетевой мониторинг: текущая скорость, объём трафика и список подключённых устройств.',
    blocks: [
      { type: 'list', items: [
        'Скорость вниз/вверх — текущая загрузка канала.',
        'Всего скачано/отправлено — за время работы ZPUI.',
        'Устройства — клиенты, подключённые через прокси.',
      ]},
    ],
  },
  {
    id: 'checker', icon: Search, title: 'Проверка ресурсов', img: 'checker',
    intro: 'Пошаговая проверка доступности сайта: DNS → TCP → TLS → HTTP. Показывает, на каком этапе обрыв и нужен ли обход.',
    blocks: [
      { type: 'steps', items: [
        'Введите адрес сайта (например, discord.com).',
        'Нажмите «Проверить».',
        'Посмотрите, на каком слое обрыв — это подскажет, нужен ли обход.',
      ]},
      { type: 'warn', text: 'Проверяйте и без Zapret, и с включённым обходом — так видно, помог ли он.' },
    ],
  },
  {
    id: 'diag', icon: Stethoscope, title: 'Диагностика', img: 'diag',
    intro: 'Проверка системы: статус WinDivert, службы Zapret, TCP Timestamps, правила файрвола, конфликтующие антивирусы и VPN.',
    blocks: [
      { type: 'important', text: 'Если обход не работает — начните с диагностики. Красные пункты указывают на вероятную причину.' },
    ],
  },
  {
    id: 'settings', icon: Settings, title: 'Настройки', img: 'settings',
    intro: 'Тема, язык, автозапуск, сворачивание в трей, проверка обновлений, управление службой Zapret и уведомления.',
    blocks: [
      { type: 'list', items: [
        '«Установить службу» — Zapret будет работать как системная служба.',
        '«Удалить службу» — переключение в режим процесса.',
        '«Полная переустановка» — удалить папку Zapret и скачать заново.',
        'Уведомления — настройте, о чём предупреждать.',
      ]},
    ],
  },
  {
    id: 'sidebar', icon: PanelLeft, title: 'Боковая панель', img: 'sidebar',
    intro: 'Навигация и быстрое управление. Внизу — инструменты: проверка ресурсов, диагностика, логи, документация.',
    blocks: [
      { type: 'list', items: [
        'Точки рядом с Zapret / Прокси / Xbox — быстрое включение и выключение.',
        'Иконка молнии — автовыбор стратегии.',
        'Лупа — проверка доступности конкретного ресурса.',
        'Стетоскоп — диагностика системы.',
      ]},
    ],
  },
];

export default function DocsPage() {
  const [activeId, setActiveId] = useState(SECTIONS[0].id);
  const section = SECTIONS.find(s => s.id === activeId) || SECTIONS[0];
  const SectionIcon = section.icon;

  return (
    <div className="docs-page">
      <div className="docs-shell">
        <aside className="docs-rail">
          {SECTIONS.map(s => {
            const Icon = s.icon;
            return (
              <button
                key={s.id}
                className={'docs-rail-item' + (s.id === activeId ? ' active' : '')}
                onClick={() => setActiveId(s.id)}
              >
                <Icon size={16} strokeWidth={2} />
                <span>{s.title}</span>
              </button>
            );
          })}
          <div className="docs-rail-divider" />
          <button className="docs-rail-item docs-rail-link" onClick={() => extLink(GITHUB_URL)}>
            <GitBranch size={16} strokeWidth={2} />
            <span>GitHub</span>
            <ExternalLink size={11} strokeWidth={2} className="docs-rail-ext" />
          </button>
          <button className="docs-rail-item docs-rail-link" onClick={() => extLink(`${GITHUB_URL}/releases`)}>
            <ArrowRight size={16} strokeWidth={2} />
            <span>Релизы</span>
            <ExternalLink size={11} strokeWidth={2} className="docs-rail-ext" />
          </button>
          <button className="docs-rail-item docs-rail-link" onClick={() => extLink(`${GITHUB_URL}/issues`)}>
            <AlertTriangle size={16} strokeWidth={2} />
            <span>Сообщить о проблеме</span>
            <ExternalLink size={11} strokeWidth={2} className="docs-rail-ext" />
          </button>
        </aside>
        <section className="docs-content" key={activeId}>
          <div className="docs-content-head">
            <span className="docs-content-icon"><SectionIcon size={22} strokeWidth={2} /></span>
            <h2 className="docs-content-title">{section.title}</h2>
          </div>
          <div className="docs-content-body">
            <p className="docs-lead">{section.intro}</p>
            {section.blocks?.map((b, i) => <Block key={i} b={b} />)}
            {section.img && (
              <div className="docs-screenshot">
                <img src={`docs/${section.img}.png`} alt={section.title} loading="lazy" />
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
