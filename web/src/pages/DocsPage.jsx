import { useState } from 'react';
import {
  Rocket, ShieldCheck, Network, Gamepad2, Activity,
  Settings, Wrench, PanelLeft,
  Lightbulb, AlertTriangle, Info, ChevronRight,
  GitBranch, Download, Bug,
} from 'lucide-react';
import { openExternal } from '../api';

const GITHUB_URL = 'https://github.com/suzcuaru/ZPUI';

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
  {
    id: 'quickstart', icon: Rocket, title: 'Быстрый старт',
    render: () => (
      <>
        <p className="docs-lead">
          ZPUI — это программа для обхода блокировок Discord, YouTube и других сайтов.
          Всё работает автоматически: нужно только запустить и подождать.
        </p>
        <StepList items={[
          'Скачайте установщик со страницы релизов на GitHub.',
          'Запустите ZPUI от имени администратора (программа сама запросует права).',
          'При первом запуске мастер проверит систему и скачает движок обхода.',
          'Автовыбор протестирует стратегии и применит лучшую для вашего провайдера.',
          'Готово! Discord, YouTube и другие сайты снова работают.',
        ]} />
        <Callout type="warn">Если обход не заработал — нажмите иконку молнии в боковой панели для повторного автовыбора стратегии.</Callout>
        <Callout type="important">Программа бесплатная, без рекламы и телеметрии. Исходный код открыт на GitHub.</Callout>
      </>
    ),
  },
  {
    id: 'zapret', icon: ShieldCheck, title: 'Запрет (обход DPI)',
    render: () => (
      <>
        <p className="docs-lead">
          Главный модуль. Модифицирует сетевые пакеты так, чтобы оборудование провайдера
          пропускало трафик к заблокированным сайтам. Включается одной кнопкой.
        </p>
        <h3 className="docs-h3">Как включить</h3>
        <BulletList items={[
          'Нажмите на зелёную точку рядом с «Запрет» в боковой панели — обход включён.',
          'Если точка красная — обход выключен. Нажмите ещё раз.',
        ]} />
        <h3 className="docs-h3">Если обход не работает</h3>
        <StepList items={[
          'Нажмите иконку молнии в боковой панели — запустится автовыбор.',
          'Программа переберёт все стратегии и выберет лучшую.',
          'Если ни одна не помогла — откройте Диагностику (иконка стетоскопа).',
        ]} />
        <Callout type="warn">Не запускайте сторонний Zapret одновременно с ZPUI — это приведёт к конфликту и потере интернета.</Callout>
        <Callout type="info">Стратегия подбирается под вашего провайдера. То, что работает у друга, может не работать у вас.</Callout>
      </>
    ),
  },
  {
    id: 'proxy', icon: Network, title: 'Прокси (SOCKS5)',
    render: () => (
      <>
        <p className="docs-lead">
          Локальный SOCKS5-прокси для раздачи обхода на телефон, планшет или телевизор
          по домашней сети. Телефон будет ходить через компьютер, на котором работает Zapret.
        </p>
        <h3 className="docs-h3">Настройка</h3>
        <StepList items={[
          'Включите Запрет (обход должен работать на компьютере).',
          'Включите Прокси точкой в боковой панели.',
          'Узнайте IP компьютера — он указан на вкладке Монитор или Прокси.',
          'На телефоне: Настройки → Wi-Fi → прокси → вручную → IP компьютера, порт 1080.',
          'Проверьте: откройте заблокированный сайт на телефоне.',
        ]} />
        <Callout type="info">Логин и пароль можно оставить пустыми. Телефон и компьютер должны быть в одной сети (один роутер).</Callout>
      </>
    ),
  },
  {
    id: 'xboxdns', icon: Gamepad2, title: 'Xbox DNS',
    render: () => (
      <>
        <p className="docs-lead">
          Меняет DNS-серверы вашего компьютера на xbox-dns.ru (111.88.96.50 и 111.88.96.51)
          для обхода блокировок Xbox Live и других сервисов на уровне DNS.
        </p>
        <h3 className="docs-h3">Как использовать</h3>
        <StepList items={[
          'Откройте вкладку Xbox DNS.',
          'Включите переключатель — DNS применится автоматически.',
          'На самой консоли Xbox следуйте инструкции на вкладке (5 шагов).',
        ]} />
        <Callout type="important">После выключения DNS автоматически возвращается к настройкам вашего провайдера (DHCP).</Callout>
      </>
    ),
  },
  {
    id: 'monitor', icon: Activity, title: 'Монитор',
    render: () => (
      <>
        <p className="docs-lead">
          Показывает скорость интернета, историю трафика и список устройств,
          подключённых через прокси.
        </p>
        <BulletList items={[
          'Скорость — текущая загрузка и отдача.',
          'График — история трафика за последнее время.',
          'Устройства — телефоны и планшеты, подключённые через SOCKS5-прокси.',
        ]} />
      </>
    ),
  },
  {
    id: 'settings', icon: Settings, title: 'Настройки',
    render: () => (
      <>
        <p className="docs-lead">
          Тема оформления, язык, автозапуск с Windows, обновления и управление службой Zapret.
        </p>
        <BulletList items={[
          'Тема: системная, светлая или тёмная.',
          'Язык: русский или английский.',
          'Автозапуск — запускать ZPUI при включении компьютера.',
          'Сворачивать в трей — прятать окно вместо закрытия.',
          'Обновления — автоматическая проверка новых версий.',
          'Служба — запускать Zapret как системную службу (надёжнее).',
        ]} />
        <Callout type="info">«Полная перестановка» удаляет папку Zapret и скачивает заново — используйте при проблемах.</Callout>
      </>
    ),
  },
  {
    id: 'troubleshoot', icon: Wrench, title: 'Решение проблем',
    render: () => (
      <>
        <p className="docs-lead">
          Самые частые проблемы и способы их решения.
        </p>
        <h3 className="docs-h3">Интернет пропал полностью</h3>
        <Callout type="warn">Выключите Запрет в боковой панели. Если интернет не вернулся — перезагрузите компьютер. Запрет автоматически восстанавливает сетевые настройки при выключении.</Callout>
        <h3 className="docs-h3">Discord / YouTube всё ещё заблокированы</h3>
        <StepList items={[
          'Убедитесь, что Запрет включён (зелёная точка).',
          'Запустите автовыбор (иконка молнии).',
          'Откройте Диагностику — проверьте, нет ли конфликтов.',
          'Очистите кеш DNS: откройте cmd и выполните ipconfig /flushdns.',
        ]} />
        <h3 className="docs-h3">Прокси не работает на телефоне</h3>
        <BulletList items={[
          'Убедитесь, что телефон и компьютер в одной Wi-Fi сети.',
          'Проверьте, что брандмауэр Windows не блокирует порт 1080.',
          'Убедитесь, что Запрет включён на компьютере.',
        ]} />
        <h3 className="docs-h3">Программа не запускается</h3>
        <Callout type="important">Проверьте файл logs/boot.log в папке программы или %TEMP%/zpui-boot.log — там может быть описание ошибки.</Callout>
      </>
    ),
  },
  {
    id: 'sidebar', icon: PanelLeft, title: 'Боковая панель',
    render: () => (
      <>
        <p className="docs-lead">
          Навигация и быстрый доступ к инструментам. Вот что означают значки.
        </p>
        <BulletList items={[
          'Точки рядом с Запрет / Прокси / Xbox DNS — быстрое включение и выключение сервисов.',
          'Молния — автовыбор стратегии (подобрать лучшую стратегию для провайдера).',
          'Лупа — проверка доступности конкретного сайта.',
          'Документы (лист) — логи приложения с фильтром по категориям.',
          'График — генерация отчёта о работе ZPUI (можно сохранить в MD или PDF).',
          'Вопрос — эта страница справки.',
        ]} />
      </>
    ),
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
          <button className="docs-rail-item docs-rail-link" onClick={() => openExternal(GITHUB_URL)}>
            <GitBranch size={16} strokeWidth={2} />
            <span>GitHub</span>
          </button>
          <button className="docs-rail-item docs-rail-link" onClick={() => openExternal(`${GITHUB_URL}/releases`)}>
            <Download size={16} strokeWidth={2} />
            <span>Релизы</span>
          </button>
          <button className="docs-rail-item docs-rail-link" onClick={() => openExternal(`${GITHUB_URL}/issues`)}>
            <Bug size={16} strokeWidth={2} />
            <span>Сообщить о проблеме</span>
          </button>
        </aside>
        <section className="docs-content" key={activeId}>
          <div className="docs-content-head">
            <span className="docs-content-icon"><SectionIcon size={22} strokeWidth={2} /></span>
            <h2 className="docs-content-title">{section.title}</h2>
          </div>
          <div className="docs-content-body">
            {section.render()}
          </div>
        </section>
      </div>
    </div>
  );
}
