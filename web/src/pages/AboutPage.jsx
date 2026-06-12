import Card from '../components/Card';
import { api } from '../api';

export default function AboutPage({ status }) {
  const z = status?.zapret || {};
  const mod = status?.mod || {};
  const zapretRepo = mod.zapret_repo || 'https://github.com/bol-van/zapret';
  const modRepo = mod.mod_repo || 'https://github.com/bol-van/zapret';

  return (
    <>
      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>О системе</h3>
          <p>ZPUI — Модуль управления Zapret</p>
        </div>
        <div className="about-desc">
          <p><strong>Zapret</strong> — утилита обхода DPI (Deep Packet Inspection) блокировок провайдеров. Работает на уровне пакетов Windows, модифицируя TCP/HTTP трафик чтобы провайдер не мог определить заблокированные ресурсы.</p>
          <p><strong>ZPUI</strong> — графический веб-интерфейс для управления Zapret. Предоставляет мониторинг, настройку стратегий, SOCKS5 прокси, диагностику и автообновление.</p>
        </div>
        <div className="about-grid">
          <div className="about-item">
            <span className="about-label">Версия модуля</span>
            <span className="about-value">{mod.version || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Версия Zapret</span>
            <span className="about-value">{z.version || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Стратегия</span>
            <span className="about-value">{z.strategy || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Веб-порт</span>
            <span className="about-value mono">{mod.web_port || 8080}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Прокси-порт</span>
            <span className="about-value mono">{status?.proxy?.port || '—'}</span>
          </div>
          <div className="about-item">
            <span className="about-label">Путь Zapret</span>
            <span className="about-value mono" style={{ fontSize: 10 }}>{z.zapretDir || '—'}</span>
          </div>
        </div>
      </Card>

      <Card className="settings-card" style={{ marginTop: 12 }}>
        <div className="settings-card-header">
          <h3>Как это работает</h3>
        </div>
        <div className="about-desc">
          <p>1. <strong>Zapret</strong> перехватывает сетевые пакеты через WinDivert и модифицирует их — разбивает TCP-сегменты, добавляет фиктивные пакеты, меняет TTL. Это не даёт провайдеру сопоставить трафик с сигнатурами заблокированных сайтов.</p>
          <p>2. <strong>Стратегии</strong> (.bat файлы) — это наборы параметров winws.exe для разных сценариев: YouTube, Discord, игры, общий обход. Можно переключать и тестировать автоматически.</p>
          <p>3. <strong>SOCKS5 прокси</strong> — позволяет делиться обходом с другими устройствами в локальной сети (телефоны, планшеты, другие ПК).</p>
        </div>
      </Card>

      <Card className="settings-card" style={{ marginTop: 12 }}>
        <div className="settings-card-header">
          <h3>Ссылки</h3>
        </div>
        <div className="about-links">
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal(zapretRepo); }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            <span>Zapret на GitHub</span>
            <span className="about-link-url mono">{zapretRepo}</span>
          </a>
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal(modRepo); }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            <span>ZPUI на GitHub</span>
            <span className="about-link-url mono">{modRepo}</span>
          </a>
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal('https://github.com/bol-van/zapret/issues'); }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/></svg>
            <span>Сообщить о проблеме</span>
            <span className="about-link-url mono">GitHub Issues</span>
          </a>
        </div>
      </Card>
    </>
  );
}

// Need to ensure openExternal is available
import { openExternal } from '../api';
