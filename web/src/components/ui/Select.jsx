import { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, Check } from 'lucide-react';
import { getOverlayRoot } from './Modal';

// Select — кастомный выпадающий список дизайн-системы: нативный <select>
// выбивается из стиля (системный фон/шрифт, белый в тёмной теме). Пилюля-триггер
// + попап; закрытие по клику мимо и по Esc. items: [{ id, label }], id сравниваются
// с value строго (числа/строки — как у вызывающего).
// Попап порталируется в .stand-win (как модалки): внутри .modal-body он
// обрезался бы скролл-контейнером. Координаты считаются в локальных единицах
// корня — CSS zoom оболочки применяется к rect'ам, делим на множитель.
export default function Select({ items, value, onChange, ariaLabel, align = 'left', className = '' }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState(null);
  const btnRef = useRef(null);
  const popRef = useRef(null);

  useEffect(() => {
    if (!open) return undefined;
    const onDoc = (e) => {
      if (btnRef.current?.contains(e.target) || popRef.current?.contains(e.target)) return;
      setOpen(false);
    };
    const onKey = (e) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('pointerdown', onDoc, true);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('pointerdown', onDoc, true);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const toggle = () => {
    if (open) { setOpen(false); return; }
    const btn = btnRef.current;
    const root = getOverlayRoot();
    if (btn && root && root !== document.body) {
      const br = btn.getBoundingClientRect();
      const wr = root.getBoundingClientRect();
      const zoomEl = root.closest('.stand-app') || root;
      const z = parseFloat(getComputedStyle(zoomEl).zoom) || 1;
      setPos({
        top: (br.bottom - wr.top) / z + 6,
        left: align === 'right' ? undefined : (br.left - wr.left) / z,
        right: align === 'right' ? (wr.right - br.right) / z : undefined,
        minWidth: br.width / z,
      });
    } else {
      setPos(null);
    }
    setOpen(true);
  };

  // Авто-флип: если попап вылезает за правый край окна — прижимаем его
  // к правому краю триггера (панель «вылезает за окно» у триггеров у края).
  useEffect(() => {
    if (!open || !pos || pos.left === undefined || !popRef.current || !btnRef.current) return;
    const root = getOverlayRoot();
    const pr = popRef.current.getBoundingClientRect();
    const rr = root.getBoundingClientRect();
    if (pr.right - rr.right <= 2) return;
    const zoomEl = root.closest('.stand-app') || root;
    const z = parseFloat(getComputedStyle(zoomEl).zoom) || 1;
    const br = btnRef.current.getBoundingClientRect();
    setPos((p) => ({ ...p, left: undefined, right: (rr.right - br.right) / z }));
  }, [open, pos]);

  const current = items.find((i) => i.id === value);
  const pop = open && (
    <div
      ref={popRef}
      className="ui-select-pop"
      role="listbox"
      aria-label={ariaLabel}
      style={pos ? {
        position: 'absolute',
        top: pos.top,
        left: pos.left,
        right: pos.right,
        minWidth: Math.max(pos.minWidth, 176),
      } : undefined}
    >
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="option"
          aria-selected={item.id === value}
          className={'ui-select-item' + (item.id === value ? ' on' : '')}
          onClick={() => { onChange(item.id); setOpen(false); }}
        >
          <span>{item.label}</span>
          {item.id === value && <Check size={13} strokeWidth={2.5} />}
        </button>
      ))}
    </div>
  );

  return (
    <div className={'ui-select' + (className ? ' ' + className : '')}>
      <button
        ref={btnRef}
        type="button"
        className={'ui-select-btn' + (open ? ' open' : '')}
        onClick={toggle}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
      >
        <span>{current ? current.label : (items[0] ? items[0].label : '')}</span>
        <ChevronDown size={12} strokeWidth={2} className={'ui-select-chev' + (open ? ' up' : '')} />
      </button>
      {pop && createPortal(pop, getOverlayRoot())}
    </div>
  );
}
