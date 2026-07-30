import { useState, useEffect, useCallback } from 'react';
import Modal from './ui/Modal';
import { api } from '../api';
import { X, Plus, Trash2 } from 'lucide-react';

// SkipListModal — модалка управления списком исключений проверки ресурсов.
// Ресурсы из списка не проверяются (полезно для всегда-недоступных CDN и т.п.).
export default function SkipListModal({ open, onClose, showToast, onChanged }) {
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [input, setInput] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    const d = await api('GET', '/api/resource-skip/list');
    setLoading(false);
    if (d?.items) {
      setItems(d.items);
    } else if (d?.error) {
      setError(d.error);
    }
  }, []);

  useEffect(() => {
    if (open) {
      setError('');
      setInput('');
      load();
    }
  }, [open, load]);

  const handleAdd = async () => {
    const host = input.trim().toLowerCase();
    if (!host) return;
    // Простая валидация — должна быть точка или http(s)://
    if (!/^https?:\/\//.test(host) && !host.includes('.')) {
      setError('Введите домен (например, example.com)');
      return;
    }
    setError('');
    const d = await api('POST', '/api/resource-skip/add', { host });
    if (d?.error) {
      setError(d.error);
      return;
    }
    setInput('');
    await load();
    onChanged?.();
    showToast?.(`Добавлено в исключения: ${host}`, 'success');
  };

  const handleRemove = async (host) => {
    const d = await api('POST', '/api/resource-skip/remove', { host });
    if (d?.error) {
      showToast?.(d.error, 'error');
      return;
    }
    await load();
    onChanged?.();
    showToast?.(`Убрано из исключений: ${host}`, 'info');
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleAdd();
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="Исключения из проверки" wide>
      <div className="sl-container">
        <div className="sl-desc">
          Ресурсы из этого списка <strong>не проверяются</strong> на доступность.
          Используйте для сайтов, которые всегда недоступны (мертвые CDN, удалённые поддомены),
          чтобы не засорять статистику и логи.
          <br />
          Поддерживается частичное совпадение: <code>google.com</code> исключит и <code>google.com</code>, и <code>drive.google.com</code>.
        </div>

        <div className="sl-input-row">
          <input
            type="text"
            className="form-input"
            placeholder="Домен (например, dead.cdn.example.com)"
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            autoFocus
          />
          <button className="btn btn-accent" onClick={handleAdd} disabled={!input.trim()}>
            <Plus size={14} strokeWidth={2.5} />
            Добавить
          </button>
        </div>

        {error && <div className="sl-error">{error}</div>}

        <div className="sl-list-section">
          <div className="sl-list-head">
            <span className="sl-list-title">Исключённые ресурсы</span>
            <span className="sl-list-count">{items.length}</span>
          </div>

          {loading ? (
            <div className="sl-empty">Загрузка...</div>
          ) : items.length === 0 ? (
            <div className="sl-empty">Список пуст. Добавьте домен выше.</div>
          ) : (
            <div className="sl-list">
              {items.map((host, i) => (
                <div key={i} className="sl-item">
                  <span className="sl-item-host mono">{host}</span>
                  <button
                    className="sl-item-remove"
                    onClick={() => handleRemove(host)}
                    data-tooltip="Убрать из исключений"
                    aria-label="Убрать"
                  >
                    <Trash2 size={13} strokeWidth={2} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="sl-footer">
          <button className="btn" onClick={onClose}>Готово</button>
        </div>
      </div>
    </Modal>
  );
}
