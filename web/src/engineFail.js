import { useEffect, useState } from 'react';

// engineFail — состояние «движок запрета не запустился» (решение владельца
// 14.11): стартовый экран ставит факт/детали, интерфейс показывает модалку
// с рекомендацией, сайдбар «зажигает» щит.
let fail = null;
const subs = new Set();

export function setEngineFail(info) {
  fail = info;
  subs.forEach((fn) => fn(fail));
}

export function clearEngineFail() {
  fail = null;
  subs.forEach((fn) => fn(null));
}

export function useEngineFail() {
  const [, setTick] = useState(0);
  useEffect(() => {
    const fn = () => setTick((n) => n + 1);
    subs.add(fn);
    return () => subs.delete(fn);
  }, []);
  return fail;
}
