import { useState } from 'react';
import { useT } from '../i18n';

const SECTIONS = [
  {
    id: 'dashboard',
    title: '📊 Дашборд',
    body: 'Главная страница с обзором системы. Показывает процент доступности ресурсов, текущий статус всех компонентов (Zapret, прокси, Xbox DNS), трафик в реальном времени и историю доступности за последние 24 часа.',
  },
  {
    id: 'zapret',
    title: '🛡 Запрет (Zapret)',
    body: 'Основной модуль обхода DPI-блокировок. Здесь выбирается стратегия обхода — разные стратегии подходят для разных провайдеров. Можно настроить игровой фильтр (для онлайн-игр) и отредактировать списки доменов. Точка на иконке в боковой панели включает/выключает обход.',
  },
  {
    id: 'proxy',
    title: '🌐 Прокси (SOCKS5)',
    body: 'Локальный SOCKS5-прокси-сервер. Нужен для приложений, которые поддерживают SOCKS5 (браузеры, мессенджеры). Адрес: socks5://127.0.0.1:1080. Точка на иконке в боковой панели включает/выключает прокси.',
  },
  {
    id: 'xboxdns',
    title: '🎮 Xbox DNS',
    body: 'Смена DNS на серверы xbox-dns.ru (111.88.96.50 / 111.88.96.51) для обхода блокировок на уровне DNS. Меняет DNS на всех активных сетевых адаптерах. Рекомендуется для игровых консолей.',
  },
  {
    id: 'monitor',
    title: '📈 Монитор',
    body: 'Мониторинг сети: текущая скорость, объём трафика, список устройств и история подключений. Помогает отследить, какие приложения используют трафик.',
  },
  {
    id: 'settings',
    title: '⚙ Настройки',
    body: 'Общие настройки: язык, тема, автозапуск, сворачивание в трей, проверка обновлений, а также статус модулей и резервные копии.',
  },
  {
    id: 'sidebar',
    title: '🔧 Боковая панель',
    body: 'Внизу боковой панели находятся кнопки: проверка ресурсов (лупа), диагностика системы (документ), логи (файл), переключатель темы (солнце/луна). Точки рядом с иконками Zapret/Прокси/Xbox — быстрое включение/выключение.',
  },
];

export default function HelpModal({ onClose }) {
  const { t } = useT();
  const [idx, setIdx] = useState(0);
  const section = SECTIONS[idx];
  const isLast = idx === SECTIONS.length - 1;

  const handleNext = () => {
    if (isLast) {
      localStorage.setItem('zpui-manual-seen', '1');
      onClose();
    } else {
      setIdx(idx + 1);
    }
  };

  const handleSkip = () => {
    localStorage.setItem('zpui-manual-seen', '1');
    onClose();
  };

  return (
    <div className="modal-overlay" onClick={handleSkip}>
      <div className="modal help-modal" onClick={e => e.stopPropagation()}>
        <div className="help-header">
          <span className="help-counter">{idx + 1} / {SECTIONS.length}</span>
          <button className="help-close" onClick={handleSkip}>✕</button>
        </div>
        <div className="help-body">
          <h2 className="help-title">{section.title}</h2>
          <p className="help-text">{section.body}</p>
        </div>
        <div className="help-dots">
          {SECTIONS.map((_, i) => (
            <span key={i} className={'help-dot' + (i === idx ? ' active' : '')} onClick={() => setIdx(i)} />
          ))}
        </div>
        <div className="help-actions">
          <button className="btn btn-ghost btn-sm" onClick={handleSkip}>{t('common.skip')}</button>
          <button className="btn btn-accent btn-sm" onClick={handleNext}>
            {isLast ? t('common.done') || 'Готово' : t('common.next')}
          </button>
        </div>
      </div>
    </div>
  );
}
