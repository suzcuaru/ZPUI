import { useRef, useEffect } from 'react';

export default function SpeedChart({ dlHistory, ulHistory }) {
  const canvasRef = useRef(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || canvas.offsetWidth === 0) return;
    const ctx = canvas.getContext('2d');
    const MAX = 60;
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.width = canvas.offsetWidth * dpr;
    const h = canvas.height = canvas.offsetHeight * dpr;
    ctx.clearRect(0, 0, w, h);

    const maxDl = Math.max(...dlHistory, 1);
    const maxUl = Math.max(...ulHistory, 1);
    const maxVal = Math.max(maxDl, maxUl);
    const pad = 4 * dpr;
    const cw = w - pad * 2;
    const ch = h - pad * 2;

    const drawLine = (data, color, fillAlpha) => {
      if (data.length < 2) return;
      const step = cw / (MAX - 1);
      ctx.beginPath();
      ctx.strokeStyle = color;
      ctx.lineWidth = 2 * dpr;
      ctx.lineJoin = 'round';
      data.forEach((val, i) => {
        const x = pad + i * step;
        const y = h - pad - (val / maxVal) * ch;
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.stroke();
      ctx.lineTo(pad + (data.length - 1) * step, h - pad);
      ctx.lineTo(pad, h - pad);
      ctx.closePath();
      ctx.fillStyle = fillAlpha;
      ctx.fill();
    };

    drawLine(dlHistory, '#5b8def', 'rgba(91,141,239,0.08)');
    drawLine(ulHistory, '#9f7aea', 'rgba(159,122,234,0.08)');
  }, [dlHistory, ulHistory]);

  return <canvas ref={canvasRef} className="speed-chart" />;
}
