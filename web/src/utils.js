export function formatBytes(bytes) {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return parseFloat((bytes / Math.pow(1024, i)).toFixed(2)) + ' ' + units[i];
}

export function formatSpeed(bps) {
  if (bps < 1024) return bps.toFixed(0) + ' B/s';
  if (bps < 1024 * 1024) return (bps / 1024).toFixed(1) + ' KB/s';
  return (bps / (1024 * 1024)).toFixed(1) + ' MB/s';
}

export function formatSpeedMB(bps) {
  const mb = bps / (1024 * 1024);
  return mb.toFixed(2).padStart(6, ' ') + ' MB/s';
}

export function formatBytesMB(bytes) {
  const mb = bytes / (1024 * 1024);
  if (mb < 1) return '0.00 MB';
  return mb.toFixed(2).padStart(6, ' ') + ' MB';
}

export function strategyDisplayName(raw) {
  if (!raw) return '—';
  let s = String(raw).trim();
  s = s.replace(/\.bat$/i, '');
  if (/^\d+$/.test(s)) return 'general' + s;
  if (/^general\d+$/.test(s)) return s;
  if (s === 'general' || s === 'general.bat') return 'general';
  return s;
}
