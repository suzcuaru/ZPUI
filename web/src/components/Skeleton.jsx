export default function Skeleton({ lines = 3, height = 14, wide }) {
  return (
    <div className={'skeleton' + (wide ? ' skeleton-wide' : '')}>
      {Array.from({ length: lines }).map((_, i) => (
        <div key={i} className="skeleton-line" style={{ height, width: i === lines - 1 ? '60%' : '100%' }}></div>
      ))}
    </div>
  );
}
