const lum = (hex) => {
  const c = hex.replace('#', '');
  const [r, g, b] = [0, 2, 4].map(i => parseInt(c.slice(i, i + 2), 16) / 255)
    .map(v => v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const blend = (fgHex, alpha, bgHex) => {
  const f = fgHex.replace('#', ''), b = bgHex.replace('#', '');
  const ch = (i) => Math.round(alpha * parseInt(f.slice(i, i + 2), 16) + (1 - alpha) * parseInt(b.slice(i, i + 2), 16));
  return '#' + [0, 2, 4].map(i => ch(i).toString(16).padStart(2, '0')).join('');
};
const ratio = (a, b) => {
  const [l1, l2] = [lum(a), lum(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
};
const fmt = (v) => v.toFixed(2).padStart(5);

console.log('=== LIGHT THEME ===');
const Lsurfaces = { card: '#ffffff', page: '#f5f6f8', inset: '#f0f1f4' };
const Lcands = {
  'current #e54545': '#e54545',
  '#cc3a3a': '#cc3a3a',
  '#c53030': '#c53030',
  '#c02b2b': '#c02b2b',
  '#bd2b2b': '#bd2b2b',
  '#b91c1c': '#b91c1c',
};
for (const [name, hex] of Object.entries(Lcands)) {
  const bg08card = blend(hex, 0.08, '#ffffff');
  const bg08page = blend(hex, 0.08, '#f5f6f8');
  console.log(`--danger ${name}:`);
  console.log(`  on card #fff:            ${fmt(ratio(hex, '#ffffff'))}`);
  console.log(`  on page #f5f6f8:         ${fmt(ratio(hex, '#f5f6f8'))}`);
  console.log(`  on inset #f0f1f4:        ${fmt(ratio(hex, '#f0f1f4'))}`);
  console.log(`  on danger-bg(0.08)card:  ${fmt(ratio(hex, bg08card))}  (bg ${bg08card})`);
  console.log(`  on danger-bg(0.06)card:  ${fmt(ratio(hex, blend(hex, 0.06, '#ffffff')))}  (bg ${blend(hex, 0.06, '#ffffff')})`);
  console.log(`  on danger-bg(0.08)page:  ${fmt(ratio(hex, bg08page))}  (bg ${bg08page})`);
  console.log(`  white text on it:        ${fmt(ratio('#ffffff', hex))}`);
}

console.log('\n=== DARK THEME ===');
const Dcands = {
  'current text #f55e5e': '#f55e5e',
};
for (const [name, hex] of Object.entries(Dcands)) {
  console.log(`--danger ${name}:`);
  console.log(`  on card #20202a:         ${fmt(ratio(hex, '#20202a'))}`);
  console.log(`  on section #1c1c24:      ${fmt(ratio(hex, '#1c1c24'))}`);
  console.log(`  on page #141418:         ${fmt(ratio(hex, '#141418'))}`);
  console.log(`  on inset #16161e:        ${fmt(ratio(hex, '#16161e'))}`);
  console.log(`  on danger-bg(0.10)card:  ${fmt(ratio(hex, blend(hex, 0.10, '#20202a')))}  (bg ${blend(hex, 0.10, '#20202a')})`);
  console.log(`  on danger-bg(0.10)sect:  ${fmt(ratio(hex, blend(hex, 0.10, '#1c1c24')))}  (bg ${blend(hex, 0.10, '#1c1c24')})`);
  console.log(`  white text on it:        ${fmt(ratio('#ffffff', hex))}`);
}
console.log('\n-- danger filled-button candidates (white glyph on fill), dark theme:');
for (const hex of ['#f55e5e', '#e04848', '#d64545', '#d14343', '#cc3a3a', '#c42b1c', '#c53030']) {
  console.log(`  white on ${hex}: ${fmt(ratio('#ffffff', hex))}   | as text on dark card: ${fmt(ratio(hex, '#20202a'))}`);
}
console.log('\n-- tooltip / misc checks:');
console.log(`  #fff on #1c1c26 (tooltip bg):      ${fmt(ratio('#ffffff', '#1c1c26'))}`);
console.log(`  dark tooltip text #e8e8f0 on #1c1c26: ${fmt(ratio('#e8e8f0', '#1c1c26'))}`);
