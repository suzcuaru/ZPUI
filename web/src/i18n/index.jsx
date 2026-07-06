import { createContext, useContext, useState, useCallback, useEffect } from 'react';
import ru from '../locales/ru.json';
import en from '../locales/en.json';

const dictionaries = { ru, en };
const I18nContext = createContext(null);

export function I18nProvider({ children }) {
  const [lang, setLang] = useState(() => {
    const saved = localStorage.getItem('zpui-lang');
    return saved && dictionaries[saved] ? saved : 'ru';
  });

  const t = useCallback((key, vars) => {
    const dict = dictionaries[lang] || dictionaries.ru;
    let val = dict;
    for (const p of key.split('.')) {
      if (val && typeof val === 'object' && p in val) val = val[p];
      else { val = undefined; break; }
    }
    if (val === undefined) {
      val = dictionaries.ru;
      for (const p of key.split('.')) {
        if (val && typeof val === 'object' && p in val) val = val[p];
        else { val = key; break; }
      }
    }
    if (typeof val === 'string' && vars) {
      val = val.replace(/\{(\w+)\}/g, (_, k) => vars[k] ?? '');
    }
    return val;
  }, [lang]);

  const changeLang = useCallback((newLang) => {
    if (!dictionaries[newLang]) return;
    setLang(newLang);
    localStorage.setItem('zpui-lang', newLang);
    document.documentElement.lang = newLang;
  }, []);

  useEffect(() => { document.documentElement.lang = lang; }, [lang]);

  return (
    <I18nContext.Provider value={{ lang, t, changeLang }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useT() {
  const ctx = useContext(I18nContext);
  if (!ctx) return { lang: 'ru', t: (k) => k, changeLang: () => {} };
  return ctx;
}
