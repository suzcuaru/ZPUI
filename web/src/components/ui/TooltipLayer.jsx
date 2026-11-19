import { useState, useEffect, useLayoutEffect, useRef } from 'react';
import { createPortal } from 'react-dom';

// TooltipLayer — единственный слой подсказок для всех [data-tooltip].
// Рендерится порталом в document.body: fixed-позиционирование в координатах
// вьюпорта, z-index выше модалок и AuthGate, подсказка нигде не обрезается
// (overflow:hidden контента на неё не действует) и «флипается» у краёв окна:
// сверху → снизу → сбоку, с клампингом в границы. Ориентация из
// data-tooltip-pos остаётся предпочтительной, но не обязательной.
const PAD = 8;
const GAP = 6;
const MAX_W = 300;

const OPPOSITE = { top: 'bottom', bottom: 'top', right: 'left', left: 'right' };
const ALL_PLACES = ['top', 'bottom', 'right', 'left'];

export default function TooltipLayer() {
  const [tip, setTip] = useState(null); // { text, el, prefer }
  const [pos, setPos] = useState(null); // { left, top }
  const boxRef = useRef(null);
  const curRef = useRef(null);

  useEffect(() => {
    const show = (el) => {
      const text = el.getAttribute('data-tooltip') || '';
      if (!text) { curRef.current = null; setTip(null); return; }
      curRef.current = { el, text };
      setTip({
        text,
        el,
        prefer: el.getAttribute('data-tooltip-pos') || 'top',
      });
    };
    const onOver = (e) => {
      const el = e.target && e.target.closest ? e.target.closest('[data-tooltip]') : null;
      if (!el) return;
      const cur = curRef.current;
      const text = el.getAttribute('data-tooltip') || '';
      if (cur && cur.el === el && cur.text === text) return;
      show(el);
    };
    const onOut = (e) => {
      const cur = curRef.current;
      if (!cur) return;
      const to = e.relatedTarget;
      if (to && cur.el.contains(to)) return;
      if (to && to.closest && to.closest('[data-tooltip]') === cur.el) return;
      curRef.current = null;
      setTip(null);
    };
    const clear = () => {
      if (!curRef.current) return;
      curRef.current = null;
      setTip(null);
    };
    document.addEventListener('pointerover', onOver, true);
    document.addEventListener('pointerout', onOut, true);
    window.addEventListener('scroll', clear, true);
    window.addEventListener('resize', clear);
    window.addEventListener('blur', clear);
    return () => {
      document.removeEventListener('pointerover', onOver, true);
      document.removeEventListener('pointerout', onOut, true);
      window.removeEventListener('scroll', clear, true);
      window.removeEventListener('resize', clear);
      window.removeEventListener('blur', clear);
    };
  }, []);

  useLayoutEffect(() => {
    if (!tip || !boxRef.current) { setPos(null); return; }
    const b = boxRef.current.getBoundingClientRect();
    const r = tip.el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const w = Math.min(MAX_W, Math.max(1, b.width));
    const h = Math.max(1, b.height);
    const fits = {
      top: r.top - GAP - h >= PAD,
      bottom: r.bottom + GAP + h <= vh - PAD,
      right: r.right + GAP + w <= vw - PAD,
      left: r.left - GAP - w >= PAD,
    };
    const prefer = tip.prefer === 'top-left' ? 'top' : tip.prefer;
    let place = ALL_PLACES.includes(prefer) ? prefer : 'top';
    if (!fits[place]) {
      const opp = OPPOSITE[place];
      place = fits[opp] ? opp : (ALL_PLACES.find((p) => fits[p]) || place);
    }
    let left;
    let top;
    if (place === 'top' || place === 'bottom') {
      left = r.left + r.width / 2 - w / 2;
      top = place === 'top' ? r.top - GAP - h : r.bottom + GAP;
    } else {
      top = r.top + r.height / 2 - h / 2;
      left = place === 'right' ? r.right + GAP : r.left - GAP - w;
    }
    left = Math.max(PAD, Math.min(left, vw - PAD - w));
    top = Math.max(PAD, Math.min(top, vh - PAD - h));
    setPos({ left: Math.round(left), top: Math.round(top) });
  }, [tip]);

  return createPortal(
    <div
      ref={boxRef}
      role="tooltip"
      className={'ttl' + (tip && pos ? ' show' : '')}
      style={pos ? { left: pos.left, top: pos.top } : { left: -9999, top: -9999 }}
    >
      {tip ? tip.text : ''}
    </div>,
    document.body
  );
}
