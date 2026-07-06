import { useState, useEffect, useRef, useCallback } from 'react';

export function usePolling(fn, intervalMs) {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const cb = useCallback(async () => {
    try { await fnRef.current(); } catch {}
  }, []);
  useEffect(() => {
    if (!intervalMs || intervalMs <= 0) return;
    cb();
    const id = setInterval(cb, intervalMs);
    return () => clearInterval(id);
  }, [intervalMs, cb]);
}
