// Spinner — кольцо загрузки приложения: indeterminate-дуга в стиле Material
// (канонические keyframes Materialize/MUI: рост-сжатие дуги с ease-in-out
// поверх равномерного вращения). Сплошная белая, без точки и градиента;
// цвет — color у .spinner (по умолчанию #fff, переопределяется контекстом).
// size в px; tone оставлен для совместимости вызовов и ни на что не влияет.
export default function Spinner({ size = 16, className = '' }) {
  const stroke = size <= 12 ? 5.5 : size <= 20 ? 5 : size <= 30 ? 4.5 : 4;
  const cls = ['spinner', className].filter(Boolean).join(' ');
  return (
    <span className={cls} role="status" style={{ width: size, height: size }}>
      <svg viewBox="0 0 50 50" aria-hidden="true">
        <circle
          className="spinner-arc" cx="25" cy="25" r="20" fill="none"
          strokeWidth={stroke} strokeLinecap="round"
        />
      </svg>
    </span>
  );
}
