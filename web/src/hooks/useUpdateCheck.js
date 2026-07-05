import { useEffect, useState, useCallback } from 'react';
import { api } from '../api';

let _zpui = { state: 'idle', current: null, latest: null };
let _zapret = { state: 'idle', current: null, latest: null };
let _notifiedZpuiVer = null;
let _notifiedZapretVer = null;
const subs = new Set();
const notify = () => subs.forEach(fn => fn());

export function setZpuiCheck(v) { _zpui = v; notify(); }
export function setZapretCheck(v) { _zapret = v; notify(); }
export function resetZapretCheck() { _zapret = { state: 'idle', current: null, latest: null }; notify(); }

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
  _zpui = { state: 'checking', current: _zpui.current, latest: _zpui.latest };
  notify();
  const d = await api('GET', '/api/updates/check/zpui');
  if (d?.error) { _zpui = { ..._zpui, state: 'error' }; notify(); return; }
  const hasUpdate = d?.update_needed === true;
  _zpui = {
    state: hasUpdate ? 'available' : 'latest',
    current: d?.current || _zpui.current,
    latest: d?.latest || d?.current,
  };
  notify();
}

export async function checkZapretUpdate() {
  _zapret = { state: 'checking', current: _zapret.current, latest: _zapret.latest };
  notify();
  const d = await api('GET', '/api/updates/check/zapret');
  if (d?.error) { _zapret = { ..._zapret, state: 'error' }; notify(); return; }
  const hasUpdate = d?.update_needed === true;
  _zapret = {
    state: hasUpdate ? 'available' : 'latest',
    current: d?.current_version || _zapret.current,
    latest: d?.latest_version || d?.latest || _zapret.current,
  };
  notify();
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
