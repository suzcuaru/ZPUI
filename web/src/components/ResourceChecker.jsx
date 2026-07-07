import { useState } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { Check } from 'lucide-react';

const VERDICT_LABELS = {
  OK:         { label: 'Доступен',          color: 'var(--success)', icon: '✓' },
  DNS_BLOCK:  { label: 'Блокировка DNS',     color: 'var(--danger)',  icon: 'DNS' },
  TCP_BLOCK:  { label: 'Блокировка TCP',     color: 'var(--danger)',  icon: 'TCP' },
  TLS_BLOCK:  { label: 'DPI сбрасывает TLS', color: 'var(--danger)',  icon: 'TLS' },
  HTTP_STUB:  { label: 'Заглушка РКН',       color: 'var(--danger)',  icon: 'HTTP' },
  TIMEOUT:    { label: 'Таймаут',            color: 'var(--warning)', icon: '⏱' },
  DOWN:       { label: 'Недоступен',         color: 'var(--danger)',  icon: '✗' },
  UNKNOWN:    { label: 'Неизвестно',         color: 'var(--text-tertiary)', icon: '?' },
};

const CONF_LABELS = { HIGH: 'высокая', MEDIUM: 'средняя', LOW: 'низкая' };

function LayerBadge({ ok, label, detail, ms }) {
  return (
    <div className={'rc-layer' + (ok ? ' ok' : ' fail')}>
      <span className="rc-layer-dot" />
      <div className="rc-layer-info">
        <span className="rc-layer-name">{label}</span>
        {detail && <span className="rc-layer-detail">{detail}</span>}
      </div>
      {ms > 0 && <span className="rc-layer-ms">{Math.round(ms)}мс</span>}
    </div>
  );
}

export default function ResourceChecker({ onClose, showToast }) {
  const [url, setUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState(null);
  const [adding, setAdding] = useState(false);

  const handleCheck = async () => {
    if (!url.trim()) return;
    setLoading(true);
    setReport(null);
    const d = await api('POST', '/api/resource-check', { url: url.trim() });
    setLoading(false);
    if (!d) {
      showToast('Ошибка проверки: бэкенд недоступен', 'error');
      return;
    }
    if (d?.error) {
      showToast(d.error, 'error');
    } else if (d?.report) {
      setReport(d.report);
    } else {
      showToast('Неожиданный ответ от сервера', 'error');
    }
  };

  const handleAddToList = async () => {
    if (!report?.Host) return;
    setAdding(true);
    const d = await api('POST', '/api/resource-add', { host: report.Host });
    setAdding(false);
    if (d?.status === 'ok') {
      showToast(`${report.Host} добавлен в пользовательский список`, 'success');
      setReport(prev => ({ ...prev, InUserList: true }));
    } else if (d?.status === 'already_exists') {
      showToast('Уже в списке', 'info');
    } else {
      showToast(d?.error || 'Ошибка', 'error');
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') handleCheck();
  };

  const vInfo = report ? VERDICT_LABELS[report.Direct?.Verdict] || VERDICT_LABELS.UNKNOWN : null;

  return (
    <Modal open onClose={onClose} title="Проверка ресурса" wide>
      <div className="rc-container">
        <div className="rc-input-row">
          <input
            type="text"
            className="form-input"
            placeholder="Введите URL или домен (например, instagram.com)"
            value={url}
            onChange={e => setUrl(e.target.value)}
            onKeyDown={handleKeyDown}
            autoFocus
          />
          <button className="btn btn-accent" onClick={handleCheck} disabled={loading || !url.trim()}>
            {loading ? 'Проверка...' : 'Проверить'}
          </button>
        </div>

        {loading && (
          <div className="rc-loading">
            <div className="loading-spinner-lg" />
            <span className="loading-sub">Идёт проверка ресурса по слоям: TCP → TLS → HTTP</span>
          </div>
        )}

        {report && !loading && (
          <div className="rc-results">
            {/* Provider info */}
            <div className="rc-section">
              <div className="rc-section-title">Провайдер</div>
              <div className="rc-provider-grid">
                <div className="rc-provider-item">
                  <span className="rc-provider-label">IP</span>
                  <span className="rc-provider-val mono">{report.Provider?.IP || '—'}</span>
                </div>
                <div className="rc-provider-item">
                  <span className="rc-provider-label">Провайдер</span>
                  <span className="rc-provider-val">{report.Provider?.ISP || '—'}</span>
                </div>
                <div className="rc-provider-item">
                  <span className="rc-provider-label">ASN</span>
                  <span className="rc-provider-val mono">{report.Provider?.ASN || '—'}</span>
                </div>
                <div className="rc-provider-item">
                  <span className="rc-provider-label">Город</span>
                  <span className="rc-provider-val">{report.Provider?.City || '—'}, {report.Provider?.Country || '—'}</span>
                </div>
              </div>
            </div>

            {/* Verdict */}
            <div className="rc-section">
              <div className="rc-section-title">Результат проверки</div>
              <div className="rc-verdict-row">
                <div className="rc-verdict-card direct">
                  <span className="rc-verdict-label">Подключение</span>
                  <span className="rc-verdict-badge" style={{ background: vInfo.color + '22', color: vInfo.color }}>
                    {vInfo.icon} {vInfo.label}
                  </span>
                  <span className="rc-verdict-conf">Уверенность: {CONF_LABELS[report.Direct?.Confidence] || '—'}</span>
                </div>
              </div>
            </div>

            {/* Layer details */}
            <div className="rc-section">
              <div className="rc-section-title">Детали по слоям</div>
              <div className="rc-layers">
                <LayerBadge
                  ok={report.Direct?.TCP?.Ok}
                  label="TCP"
                  detail={
                    report.Direct?.TCP?.Ok
                      ? 'Соединение установлено'
                      : report.Direct?.TCP?.Error
                        ? `Ошибка: ${report.Direct.TCP.Error}`
                        : 'Не проверялось'
                  }
                  ms={report.Direct?.TCP?.TimeMs || 0}
                />
                <LayerBadge
                  ok={report.Direct?.TLS?.Ok}
                  label="TLS"
                  detail={
                    report.Direct?.TLS?.Ok
                      ? 'Handshake успешен'
                      : report.Direct?.TLS?.Error
                        ? `Ошибка: ${report.Direct.TLS.Error}`
                        : 'Не проверялось'
                  }
                  ms={report.Direct?.TLS?.TimeMs || 0}
                />
                <LayerBadge
                  ok={report.Direct?.HTTP?.Ok}
                  label="HTTP"
                  detail={
                    report.Direct?.HTTP?.StubPage
                      ? 'Заглушка РКН/TSPU обнаружена'
                      : report.Direct?.HTTP?.Status
                        ? `Статус: ${report.Direct.HTTP.Status}`
                        : report.Direct?.HTTP?.Error || 'Нет ответа'
                  }
                  ms={report.Direct?.HTTP?.TimeMs || 0}
                />
              </div>
            </div>

            {/* Notes */}
            {report.Direct?.Notes && report.Direct.Notes.length > 0 && (
              <div className="rc-section">
                <div className="rc-section-title">Заметки</div>
                <ul className="rc-notes">
                  {report.Direct.Notes.map((n, i) => <li key={i}>{n}</li>)}
                </ul>
              </div>
            )}

            {/* Add to list */}
            {report.Blocked && !report.InUserList && (
              <div className="rc-add-card">
                <div className="rc-add-info">
                  <span className="rc-add-title">Ресурс заблокирован</span>
                  <span className="rc-add-desc">{report.Host} не найден в списках обхода</span>
                </div>
                <button className="btn btn-accent" onClick={handleAddToList} disabled={adding}>
                  {adding ? '...' : 'Добавить в список'}
                </button>
              </div>
            )}
            {report.InUserList && (
              <div className="rc-add-card" style={{ background: 'var(--success-bg)' }}>
                <span style={{ color: 'var(--success)', fontSize: 12, fontWeight: 600, display:'inline-flex', alignItems:'center', gap:4 }}>
                  <Check size={13} strokeWidth={3} /> {report.Host} уже в пользовательском списке
                </span>
              </div>
            )}
          </div>
        )}
      </div>
    </Modal>
  );
}
