// Естественный порядок имён стратегий: general.bat первым, затем по базе
// имени и числовому варианту (ALT2 < ALT10).
export function cmpStrategiesNatural(a, b) {
  const an = a.filename || a.name;
  const bn = b.filename || b.name;
  if (an === 'general.bat') return -1;
  if (bn === 'general.bat') return 1;
  const parseStrat = (s) => {
    const m = s.match(/^(.+?)\s*\(([^)]+)\)\.bat$/i);
    if (m) return { base: m[1].toLowerCase(), variant: m[2] };
    return { base: s.replace(/\.bat$/i, '').toLowerCase(), variant: '' };
  };
  const pa = parseStrat(an);
  const pb = parseStrat(bn);
  if (pa.base !== pb.base) return pa.base.localeCompare(pb.base);
  const parseVariant = (v) => {
    const m = v.match(/^([A-Za-z ]+?)(\d+)?$/);
    if (m) return { letters: m[1].trim().toUpperCase(), num: m[2] ? parseInt(m[2], 10) : 0 };
    return { letters: v.toUpperCase(), num: 0 };
  };
  const va = parseVariant(pa.variant);
  const vb = parseVariant(pb.variant);
  if (va.letters !== vb.letters) {
    if (va.letters === '') return -1;
    if (vb.letters === '') return 1;
    return va.letters.localeCompare(vb.letters);
  }
  return va.num - vb.num;
}
