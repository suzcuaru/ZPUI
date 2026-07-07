import { useState } from 'react';
import {
  Sparkles, LayoutDashboard, ShieldCheck, Network, Gamepad2, Activity,
  Settings, PanelLeft, Search, Stethoscope, Zap,
  Lightbulb, AlertTriangle, Info, ArrowRight,
} from 'lucide-react';

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
  const { cls, Icon } = C[b.type] || C.info;
  return (
    <div className={'docs-callout ' + cls}>
      <span className="docs-callout-icon"><Icon size={16} strokeWidth={2} /></span>
      <span>{b.text}</span>
    </div>
  );
}

const SECTIONS = [
  {
    id: 'start', icon: Sparkles, title: 'Начало работы',
    intro: 'ZPUI — контроллер обхода DPI-блокировок на базе Zapret. Мастер первого запуска проверит целостность файлов, скачает Zapret и подберёт рабочую стратегию автоматически. Дальше всё управляется из боковой панели.',
    blocks: [
      { type: 'important', text: 'Для работы обхода нужен запуск от имени администратора — без прав WinDivert не сможет перехватывать трафик.' },
      { type: 'info', text: 'Если на ПК уже стоит сторонний Zapret или активен VPN, мастер предложит их удалить — иначе они будут конфликтовать.' },
    ],
  },
  {
    id: 'dashboard', icon: LayoutDashboard, title: 'Дашборд',
    intro: 'Главная страница с обзором системы: доступность ресурсов, статус компонентов, трафик в реальном времени и история за 24 часа.',
    blocks: [
      { type: 'info', text: 'Процент доступности считается по регулярным проверкам ключевых ресурсов. Зелёный — стабильно доступно, жёлтый — перебои, красный — недоступно.' },
    ],
  },
  {
    id: 'zapret', icon: ShieldCheck, title: 'Запрет (Zapret)',
    intro: 'Основной модуль обхода DPI. Здесь выбирается стратегия, настраивается игровой фильтр и редактируются списки доменов. Точка на иконке в боковой панели включает и выключает обход.',
    blocks: [
      { type: 'important', text: 'Стратегия подбирается под провайдера. Если обход не работает — запустите «Автовыбор стратегии», он проверит все варианты и предложит лучший.' },
      { type: 'warn', text: 'Не запускайте две стратегии одновременно и не оставляйте сторонний Zapret — это приведёт к конфликту и потере сети.' },
    ],
  },
  {
    id: 'proxy', icon: Network, title: 'Прокси (SOCKS5)',
    intro: 'Локальный SOCKS5-прокси для приложений, которые умеют работать с SOCKS5: браузеры, мессенджеры, клиенты. Адрес — socks5://127.0.0.1:1080.',
    blocks: [
      { type: 'list', items: [
        'Включается точкой на иконке в боковой панели.',
        'Работает поверх Zapret — сначала включите обход, затем прокси.',
        'Логин и пароль настраиваются в разделе прокси (можно оставить пустыми).',
      ]},
    ],
  },
  {
    id: 'xboxdns', icon: Gamepad2, title: 'Xbox DNS',
    intro: 'Смена DNS на серверы xbox-dns.ru (111.88.96.50 / 111.88.96.51) для обхода блокировок на уровне DNS. Применяется ко всем активным сетевым адаптерам.',
    blocks: [
      { type: 'important', text: 'Рекомендуется для игровых консолей. После выключения DNS возвращаются к значениям провайдера (или к DHCP).' },
    ],
  },
  {
    id: 'monitor', icon: Activity, title: 'Монитор',
    intro: 'Сетевой мониторинг: текущая скорость, объём трафика, список процессов и история. Помогает понять, какие приложения нагружают канал.',
  },
  {
    id: 'settings', icon: Settings, title: 'Настройки',
    intro: 'Общие параметры: язык, тема, автозапуск, сворачивание в трей, проверка обновлений, управление сервисом Zapret и уведомления.',
    blocks: [
      { type: 'info', text: 'Обновления ZPUI и Zapret проверяются автоматически. Если доступна новая версия — появится кнопка «Обновить».' },
    ],
  },
  {
    id: 'sidebar', icon: PanelLeft, title: 'Боковая панель',
    intro: 'Основная навигация и быстрое управление. Внизу — инструменты: проверка ресурсов, диагностика, логи, документация, тема.',
    blocks: [
      { type: 'list', items: [
        'Точки рядом с Zapret / Прокси / Xbox — быстрое включение и выключение.',
        'Иконка молнии — автовыбор стратегии (доступна, когда Zapret запущен).',
        'Лупа — проверка доступности конкретного ресурса.',
        'Стетоскоп — диагностика системы (WinDivert, службы, firewall).',
      ]},
    ],
  },
  {
    id: 'checker', icon: Search, title: 'Проверка ресурсов',
    intro: 'Пошаговая проверка доступности ресурса по слоям: DNS → TCP → TLS → HTTP. Показывает, на каком этапе обрыв и помогает понять, нужен ли обход.',
    blocks: [
      { type: 'warn', text: 'Проверку проводите и без Zapret, и с включённым обходом — так видно, помог ли он.' },
    ],
  },
  {
    id: 'diag', icon: Stethoscope, title: 'Диагностика',
    intro: 'Быстрая проверка системы: статус служб Zapret и WinDivert, TCP Timestamps, правила файрвола, конфликтующие антивирусы и VPN.',
    blocks: [
      { type: 'important', text: 'Если обход не работает — начните с диагностики. Красные пункты указывают на вероятную причину.' },
    ],
  },
  {
    id: 'autoselect', icon: Zap, title: 'Автовыбор стратегии',
    intro: 'Автоматически перебирает стратегии Zapret, проверяет каждую и выбирает лучшую по результатам теста доступности. Результаты помечаются цветом: зелёный — идеально, жёлтый — средне, красный — плохо, серый — не работает.',
    blocks: [
      { type: 'info', text: 'Тест может занять несколько минут: каждая стратегия проверяется на реальных ресурсах.' },
    ],
  },
];

export default function DocsPage() {
  const [activeId, setActiveId] = useState(SECTIONS[0].id);
  const section = SECTIONS.find(s => s.id === activeId) || SECTIONS[0];
  const SectionIcon = section.icon;

  return (
    <div className="docs-page">
      <div className="page-title">Документация</div>
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
        </aside>
        <section className="docs-content" key={activeId}>
          <div className="docs-content-head">
            <span className="docs-content-icon"><SectionIcon size={22} strokeWidth={2} /></span>
            <h2 className="docs-content-title">{section.title}</h2>
          </div>
          <div className="docs-content-body">
            <p className="docs-lead">{section.intro}</p>
            {section.blocks?.map((b, i) => <Block key={i} b={b} />)}
          </div>
        </section>
      </div>
    </div>
  );
}
