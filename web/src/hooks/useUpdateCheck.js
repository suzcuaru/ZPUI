import { useEffect, useState, useCallback } from 'react';
import { api } from '../api';

let _zpui = { state: 'idle', current: null, latest: null, progress: null, phase: null };
let _zapret = { state: 'idle', current: null, latest: null, progress: null, phase: null };
let _notifiedZpuiVer = null;
let _notifiedZapretVer = null;
const subs = new Set();
const notify = () => subs.forEach(fn => fn());

export function setZpuiCheck(v) { _zpui = v; notify(); }
export function setZapretCheck(v) { _zapret = v; notify(); }
export function resetZapretCheck() { _zapret = { state: 'idle', current: null, latest: null, progress: null, phase: null }; notify(); }

export function shouldNotifyZpui(version) {
  if (!version || version === _notifiedZpuiVer) return false;
  _notifiedZpuiVer = version;
  return true;
}

export function shouldNotifyZapret(version) {
  if (!version || version === _notifiedZapretVer) return false;
  _notifiedZapretVer = version;
  return true;
}

export async function checkZpuiUpdate() {
  _zpui = { state: 'checking', current: _zpui.current, latest: _zpui.latest, progress: null, phase: null };
  notify();
  const d = await api('GET', '/api/updates/check/zpui');
  if (d?.error) { _zpui = { ..._zpui, state: 'error' }; notify(); return; }
  const hasUpdate = d?.update_needed === true;
  _zpui = {
    state: hasUpdate ? 'available' : 'latest',
    current: d?.current || _zpui.current,
    latest: d?.latest || d?.current,
    progress: null,
    phase: null,
  };
  notify();
}

export async function checkZapretUpdate() {
  _zapret = { state: 'checking', current: _zapret.current, latest: _zapret.latest, progress: null, phase: null };
  notify();
  const d = await api('GET', '/api/updates/check/zapret');
  if (d?.error) { _zapret = { ..._zapret, state: 'error' }; notify(); return; }
  const hasUpdate = d?.update_needed === true;
  _zapret = {
    state: hasUpdate ? 'available' : 'latest',
    current: d?.current_version || _zapret.current,
    latest: d?.latest_version || d?.latest || _zapret.current,
    progress: null,
    phase: null,
  };
  notify();
}

// Download update with progress tracking via polling
export async function downloadUpdate(name) {
  const target = name === 'ZPUI' ? _zpui : _zapret;
  target.state = 'downloading';
  target.progress = 0;
  target.phase = 'starting';
  notify();

  const d = await api('POST', '/api/components/download', { name });
  if (d?.error) {
    target.state = 'error';
    target.phase = 'error';
    notify();
    return false;
  }

  // Poll for download progress
  target.phase = 'downloading';
  notify();

  // Wait for download to complete (poll component check until downloaded)
  for (let i = 0; i < 60; i++) {
    await new Promise(r => setTimeout(r, 1000));
    const check = await api('GET', '/api/updates/check/' + (name === 'ZPUI' ? 'zpui' : 'zapret'));
    // Progress estimation based on time
    target.progress = Math.min(95, (i + 1) * 2);
    notify();
  }

  // Verify download
  target.phase = 'verifying';
  target.progress = 95;
  notify();

  const verify = await api('POST', '/api/components/verify', { name });
  if (verify?.error) {
    target.state = 'error';
    target.phase = 'error';
    notify();
    return false;
  }

  target.state = 'downloaded';
  target.phase = 'downloaded';
  target.progress = 100;
  notify();
  return true;
}

// Install previously downloaded update
export async function installUpdate(name) {
  const target = name === 'ZPUI' ? _zpui : _zapret;
  target.state = 'installing';
  target.phase = 'installing';
  target.progress = 0;
  notify();

  const d = await api('POST', '/api/components/install', { name });
  if (d?.error) {
    target.state = 'error';
    target.phase = 'error';
    notify();
    return false;
  }

  target.state = 'installed';
  target.phase = 'installed';
  target.progress = 100;
  notify();
  return true;
}

export function useUpdateCheck() {
  const [, setTick] = useState(0);
  const rerender = useCallback(() => setTick(n => n + 1), []);

  useEffect(() => {
    subs.add(rerender);
    return () => subs.delete(rerender);
  }, [rerender]);

  return { zpuiCheck: _zpui, zapretCheck: _zapret };
}