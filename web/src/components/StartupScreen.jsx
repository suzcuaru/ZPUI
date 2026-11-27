import { useState, useEffect, useRef, useCallback } from 'react';
import { AlertTriangle } from 'lucide-react';
import HvciFixButton from './HvciFixButton';
import { api, createStream } from '../api';
import { useT } from '../i18n';
import { frontlog } from '../frontlog';
import { subscribeEvent } from '../events';
import ProgressBar from './ui/ProgressBar';
import { STEP_PROGRESS } from './startup/steps';
import { setEngineFail } from '../engineFail';

// Стартовый экран: чистый прогресс-бар с процентами. Подписи «что происходит»
// не показываются; под баром — ОДНА строка, привязанная к реальному шагу
// (на 0–15% текста нет вообще — там нечего сообщать). Вопросы окружения,
// критичные проблемы службы и автодиагностики — модалками поверх бара.

// Сообщение под баром по текущему шагу; '' = текста нет. Решение владельца
// 14.11: сообщения только про то, что реально происходит.
const STEP_STATUS = {
  'check-service': 'system',
  'decide-service': 'system',
  install: 'engine',
  'install-service': 'engine',
  'auto-select': 'selecting',
  done: 'almost',
};

export default function StartupScreen({ onComplete }) {
  const { t } = useT();
  const tRef = useRef(t);
  tRef.current = t;
  const [currentStep, setCurrentStep] = useState('connect');
  const [stepDone, setStepDone] = useState(false);
  const [progress, setProgress] = useState(2);
  // Отображаемый процент: медленно ползёт к CAP между реальными чекпоинтами,
  // чтобы бар не «замирал» на долгих шагах (см. эффект creep ниже).
  const [displayed, setDisplayed] = useState(2);
  // Неблокирующая ошибка шага (шаг продолжается/пропускается) — строка
  // под баром, не модалка.
  const [softError, setSoftError] = useState(null);
  const softErrTimerRef = useRef(null);
  const [error, setError] = useState(null);
  const [errCode, setErrCode] = useState(null);
  const [errorTitle, setErrorTitle] = useState('');
  const [errorLog, setErrorLog] = useState(null);

  const [downloadProgress, setDownloadProgress] = useState(null);
  const [autoSelect, setAutoSelect] = useState(null);
  const [serviceCritical, setServiceCritical] = useState(false);
  const autoSelectCountRef = useRef(0);
  // Вопрос окружения (сторонний Запрет / VPN): {kind:'zapret'|'vpn', data}.
  const [envQuestion, setEnvQuestion] = useState(null);
  const envResolveRef = useRef(null);
  const autoSelectEsRef = useRef(null);
  const autoSelectFinishRef = useRef(null);
  // Критичные проблемы автодиагностики (после установки запрета и статуса
  // «работает»): модалка «Пропустить / Выключить запрет».
  const [criticalIssues, setCriticalIssues] = useState(null);
  const diagResolveRef = useRef(null);

  const aliveRef = useRef(true);
  const eventsCleanupRef = useRef(null);
  const installedNowRef = useRef(false);
  const installedServiceRef = useRef(false);
  // Чистая база (первый запуск): снимается с конфига ДО того, как шаг
  // auto-select перезапишет first_run_done, и уходит в onComplete.
  const firstRunRef = useRef(false);

  const go = useCallback(async (id, fn) => {
    if (id) setCurrentStep(id);
    setStepDone(false);
    try {
      await fn();
    } catch (e) {
      frontlog('startup', { step: id || '', stepResult: 'fail', code: (e && e.message) || String(e) });
      setErrorTitle(tRef.current('startup.errors.title'));
      setError(e.message || String(e));
      return false;
    }
    frontlog('startup', { level: 'info', step: id || '', stepResult: 'done' });
    setStepDone(true);
    return true;
  }, []);

  const sleep = useCallback(ms => new Promise(r => setTimeout(r, ms)), []);

  // Софт-ошибка: шаг не удался, но процесс продолжается (пропуск/ретрай).
  // Висит под баром до следующей ошибки или 12 секунд.
  const showSoftError = useCallback((msg) => {
    setSoftError(msg);
    if (softErrTimerRef.current) clearTimeout(softErrTimerRef.current);
    softErrTimerRef.current = setTimeout(() => setSoftError(null), 12000);
  }, []);

  const resolveEnv = useCallback((action) => {
    const resolve = envResolveRef.current;
    envResolveRef.current = null;
    setEnvQuestion(null);
    if (resolve) resolve(action);
  }, []);

  const askEnv = useCallback(async (question) => {
    if (envResolveRef.current) {
      const stale = envResolveRef.current;
      envResolveRef.current = null;
      stale(null);
    }
    return new Promise((resolve) => {
      envResolveRef.current = resolve;
      setEnvQuestion(question);
    });
  }, []);

  const handleRetryDiagnostics = useCallback(() => {
    window.location.reload();
  }, []);

  const handleContinueWithout = useCallback(async () => {
    setServiceCritical(false);
    await api('POST', '/api/config', { zapret_skipped: true }).catch(() => null);
    // Без флага firstRun: служба сломана — авто-подбор при входе не запускаем.
    if (aliveRef.current) onComplete();
  }, [onComplete]);

  const withTimeout = useCallback((promise, ms, errorMsg) => {
    return Promise.race([
      promise,
      new Promise((_, reject) => setTimeout(() => reject(new Error(errorMsg || 'Timeout')), ms))
    ]);
  }, []);

  // Скачивание/установка запрета (AutoInstallZapret) с подпиской на прогресс.
  const autoInstallZapret = useCallback(async () => {
    setDownloadProgress(null);
    const off = subscribeEvent('download:progress', (data) => setDownloadProgress(data));
    eventsCleanupRef.current = off;
    const r = await api('POST', '/api/zapret/auto-install');
    off();
    eventsCleanupRef.current = null;
    return r;
  }, []);

  const run = useCallback(async () => {
    // 1. Подключение к бэкенду. Стартовая панель НЕ закрывается: если фронт
    // долго не может достучаться до бэка —
    //  • биндинги ещё не вшиты (window.go нет) — тихо перезагружаем фронт,
    //    панель появляется заново (бэк — тот же процесс, его не трогаем);
    //  • биндинги есть, но вызовы висят (холодная машина/занятая БД) —
    //    просто продолжаем ждать до 2 минут, после чего показываем ошибку.
    if (!await go('connect', async () => {
      const t0 = Date.now();
      for (;;) {
        if (!aliveRef.current) return;
        if (!window.go) {
          const reloads = Number(sessionStorage.getItem('zpui-connect-reloads') || 0);
          if (reloads < 3 && Date.now() - t0 > 5000) {
            sessionStorage.setItem('zpui-connect-reloads', String(reloads + 1));
            window.location.reload();
            return;
          }
        } else {
          sessionStorage.removeItem('zpui-connect-reloads');
        }
        // биндинг может зависнуть (холодная машина) — таймаут, а не вечное ожидание
        const data = await withTimeout(api('GET', '/api/status'), 8000).catch(() => null);
        if (data) return;
        if (Date.now() - t0 > 120000) break;
        await sleep(1000);
      }
      throw new Error(tRef.current('startup.errors.backendConnect'));
    })) return;
    setProgress(STEP_PROGRESS.connect);

    const cfg = await api('GET', '/api/config').catch(() => null);
    firstRunRef.current = !!cfg && cfg.first_run_done !== true;
    if (cfg?.theme === 'dark' || cfg?.theme === 'light') {
      document.documentElement.setAttribute('data-theme', cfg.theme);
    }

    // 2. Проверка целостности
    if (!await go('health', async () => {
      const health = await withTimeout(api('GET', '/api/health'), 20000).catch(() => null);
      if (health?.warnings?.length > 0) {
        const critical = health.warnings.filter(w =>
          w.includes('не найден') || w.includes('missing') || w.includes('exe')
        );
        if (critical.length > 0) throw new Error(critical.join('\n'));
      }
    })) return;
    setProgress(STEP_PROGRESS.health);

    // 2.5 Анализ окружения: сторонний Запрет (кнопка «Удалить» / «Оставить»).
    // Вопрос про активный VPN убран — приложение стартует молча; карточка VPN
    // на вкладке «Пользователь» показывает окружение без остановки запуска.
    // Скан — рекомендательный и БЫСТРЫЙ: 12 секунд потолок, завис/не успел —
    // едем дальше (без него старт висел на 15% целых 90 секунд).
    let keptThirdPartyZapret = false;
    if (!await go('envscan', async () => {
      const env = await withTimeout(api('GET', '/api/envscan/scan'), 12000).catch(() => null);
      if (env?.zapret?.found) {
        const act = await askEnv({ kind: 'zapret', data: env });
        if (!aliveRef.current || !act) return;
        if (act === 'remove') {
          const r = await api('POST', '/api/envscan/remove-zapret');
          if (r?.error) throw new Error(r.error);
        } else if (act === 'keep') {
          keptThirdPartyZapret = true;
          await api('POST', '/api/config', { zapret_skipped: true }).catch(() => null);
        }
      }
    })) return;
    setProgress(STEP_PROGRESS.envscan);

    // 3. Проверка службы Windows
    const zapretSkipped = cfg?.zapret_skipped === true;
    let userSkipped = false;

    if (!zapretSkipped && !keptThirdPartyZapret) {
      const hasService = await api('GET', '/api/zapret/system-service');
      const hasLocal = await api('GET', '/api/zapret/local');

      if (hasService === true || hasService?.result === true) {
        if (!await go('check-service', async () => {
          setCurrentStep('check-service');
          let healthResult;
          try {
            healthResult = await withTimeout(
              api('GET', '/api/zapret/service-health'),
              180000,
              tRef.current('startup.errors.serviceCheckTimeout')
            );
          } catch (e) {
            setServiceCritical(true);
            setErrorLog([`[${new Date().toLocaleTimeString()}] ${e.message || 'Timeout'}`]);
            throw new Error(tRef.current('startup.serviceHealth.criticalDesc'));
          }
          if (healthResult?.status === 'critical') {
            if (healthResult.log) setErrorLog(healthResult.log);
            setServiceCritical(true);
            setErrCode(healthResult.errorCode || null);
            throw new Error(healthResult.message || tRef.current('startup.serviceHealth.criticalDesc'));
          }
        })) return;
      }
      setProgress(STEP_PROGRESS['check-service']);

      // 4. Решение по службе + локальной копии
      if (hasService === true || hasService?.result === true) {
        if (!(hasLocal === true || hasLocal?.result === true)) {
          setCurrentStep('decide-service');
          setStepDone(false);
          const r = await api('POST', '/api/zapret/remove-system-service');
          if (r?.error) {
            await api('POST', '/api/config', { zapret_skipped: true }).catch(() => null);
            userSkipped = true;
            showSoftError(tRef.current('startup.soft.removeService'));
          }
          setStepDone(true);
        }
      }
      setProgress(STEP_PROGRESS['decide-service']);

      if (!userSkipped) {
        // 5. Скачивание/установка локальной копии.
        const hasLocal2 = await api('GET', '/api/zapret/local');
        if (!(hasLocal2 === true || hasLocal2?.result === true)) {
          if (!await go('install', async () => {
            const r = await autoInstallZapret();
            if (r?.error) throw new Error(r.error);
            if (r?.start_error) throw new Error(tRef.current('startup.errors.zapretFailed', { error: r.start_error }));
            installedNowRef.current = true;
            setStepDone(true);
          })) return;
        }
        setProgress(STEP_PROGRESS['check-local']);

        // 6. Установка службы
        const serviceInstalledNow = await api('GET', '/api/zapret/system-service');
        if (!(serviceInstalledNow === true || serviceInstalledNow?.result === true)) {
          if (!await go('install-service', async () => {
            const def = await api('GET', '/api/zapret/default-strategy').catch(() => null);
            const strategy = def?.strategy || 'general.bat';
            const r = await api('POST', '/api/zapret/install-service-logged', { strategy });
            if (r?.error) {
              const log = await api('GET', '/api/zapret/install-log');
              setErrorLog(log?.lines || null);
              throw new Error(tRef.current('startup.errors.serviceInstallFailed', { error: r.error }));
            }
            if (r?.errors?.length) {
              const log = await api('GET', '/api/zapret/install-log');
              setErrorLog(log?.lines || null);
              throw new Error(r.errors.join('\n'));
            }
            if (r && r.success === false) {
              const log = await api('GET', '/api/zapret/install-log');
              setErrorLog(log?.lines || null);
              throw new Error(tRef.current('startup.errors.serviceInstallFailedLog'));
            }
            installedServiceRef.current = true;
          })) return;
        }
        setProgress(STEP_PROGRESS['install-service']);

        // 6.5 Попытка запуска движка (решение владельца 14.11): если движок
        // не поднялся — подбор и тесты стратегий бессмысленны и пропускаются,
        // а факт уходит в интерфейс (модалка ошибки + щит в сайдбаре).
        setCurrentStep('start-engine');
        let startErr = '';
        try {
          const r = await api('POST', '/api/zapret/start').catch(() => null);
          if (!r || r.error) throw new Error(r?.error || 'start failed');
          let running = false;
          for (let i = 0; i < 25; i++) {
            if (!aliveRef.current) return;
            await sleep(1000);
            const st = await withTimeout(api('GET', '/api/status'), 5000).catch(() => null);
            if (st?.zapret?.status === 'running') { running = true; break; }
          }
          if (!running) throw new Error('engine not running after 25s');
        } catch (e) {
          startErr = (e && e.message) || String(e);
        }
        if (startErr) {
          // Движок не запустился: собираем детали и завершаем старт —
          // модалку с рекомендацией покажет интерфейс.
          showSoftError(tRef.current('startup.errors.zapretFailed', { error: startErr }));
          const [eng, hvci, st] = await Promise.all([
            api('GET', '/api/zapret/engine').catch(() => null),
            api('GET', '/api/system/hvci').catch(() => null),
            api('GET', '/api/status').catch(() => null),
          ]);
          setEngineFail({
            engine: eng && !eng.error ? eng.engine : '',
            version: st?.zapret?.version || '',
            hvci: !!(hvci && !hvci.error && hvci.enabled),
            error: startErr,
          });
          setProgress(STEP_PROGRESS.done);
          await sleep(300);
          if (aliveRef.current) onComplete({ firstRun: firstRunRef.current });
          return;
        }

        // 7. Стартовый автоподбор — ускоренная проверка 3 лучших «по мнению
        // устройства» (бэк вернёт skipped, если данных нет вообще).
        {
          if (!await go('auto-select', async () => {
            autoSelectCountRef.current = 0;
            setAutoSelect({ current: 0, total: 0, phase: 'operator', strategy: '' });

            // Умный подбор: топ стратегий своего оператора (с сервера, при
            // недоступности — по локальным данным устройства).
            await new Promise((resolve, reject) => {
              const finish = () => {
                if (es) es.close();
                es = null;
                autoSelectEsRef.current = null;
                autoSelectFinishRef.current = null;
                resolve();
              };
              autoSelectFinishRef.current = finish;
              let es = null;
              es = createStream('/api/autoselect/stream?mode=startup');
              autoSelectEsRef.current = es;
              es.onmessage = (e) => {
                if (!aliveRef.current) { finish(); return; }
                let d;
                try { d = JSON.parse(e.data); } catch { return; }
                if (d.type === 'done') {
                  if (d.error) {
                    reject(new Error(d.error));
                    return;
                  }
                  finish();
                  return;
                }
                if (d.type === 'progress') {
                  setAutoSelect(prev => ({
                    ...prev,
                    current: d.current || 0,
                    total: d.total || 0,
                    phase: d.phase || prev.phase,
                    strategy: d.strategy ? d.strategy.replace(/\.bat$/i, '') : prev.strategy,
                  }));
                } else if (d.type === 'keyprogress') {
                  setAutoSelect(prev => ({
                    ...prev,
                    current: d.current || prev.current,
                    total: d.total || prev.total,
                    phase: 'filter',
                    strategy: d.strategy ? d.strategy.replace(/\.bat$/i, '') : prev.strategy,
                  }));
                } else if (d.type === 'result') {
                  if (d.strategy) {
                    autoSelectCountRef.current += 1;
                    setAutoSelect(prev => ({ ...prev, current: autoSelectCountRef.current }));
                  }
                } else if (d.type === 'info') {
                  setAutoSelect(prev => {
                    const total = d.total || prev.total;
                    const stageChanged = d.total > 0 && d.total !== prev.total;
                    if (stageChanged) autoSelectCountRef.current = 0;
                    return {
                      ...prev,
                      total,
                      current: stageChanged ? 0 : prev.current,
                      phase: d.phase || prev.phase,
                      strategy: d.phase && d.phase !== prev.phase ? '' : prev.strategy,
                    };
                  });
                }
              };
              es.onerror = () => {
                // Стрим оборвался (сеть/перезапуск бэка): как и раньше
                // продолжаем старт, но факт показываем под баром.
                showSoftError(tRef.current('zapret.autoSelectFailed'));
                finish();
              };
            });

            await api('POST', '/api/config', { first_run_done: true }).catch(() => null);
            const now = new Date().toISOString();
            await api('POST', '/api/config', { last_auto_select_time: now }).catch(() => null);
          })) return;
        }
        setProgress(STEP_PROGRESS['auto-select']);
      }
    } else {
      setProgress(STEP_PROGRESS['auto-select']);
    }

    // 8. Готово
    if (!await go('done', async () => {
      setProgress(STEP_PROGRESS.done);
      // Гейт готовности бэкенда: UI показываем только когда фоновый старт
      // завершён. Потолок 2.5 минуты: сломанная сеть не держит стартовый
      // экран вечно. Пока ждём — ползём чекпоинтами 90→99.
      const t0 = Date.now();
      for (;;) {
        const r = await withTimeout(api('GET', '/api/startup/ready'), 5000).catch(() => null);
        if (r?.ready) break;
        if (!aliveRef.current || Date.now() - t0 > 150000) break;
        const waited = Date.now() - t0;
        setProgress(Math.min(99, STEP_PROGRESS.done + Math.floor(waited / 30000)));
        await sleep(1500);
      }
      setProgress(100);
      await sleep(400);
    })) return;

    // Автодиагностика после установки: запрет установлен и «работает» —
    // прогоняем встроенную диагностику. Критичные проблемы показываем
    // модалкой и ждём решения; результат целиком сохраняется для отчёта.
    if (installedNowRef.current || installedServiceRef.current) {
      const diag = await api('POST', '/api/zapret/startup-diagnostics').catch(() => null);
      if (diag && !diag.error && diag.has_critical) {
        if (!aliveRef.current) return;
        await new Promise((resolve) => {
          diagResolveRef.current = resolve;
          setCriticalIssues(diag.critical || []);
        });
      }
    }

    await sleep(200);
    if (aliveRef.current) onComplete({ firstRun: firstRunRef.current });
  }, [go, sleep, onComplete, autoInstallZapret, showSoftError]);

  const resolveDiag = () => {
    setCriticalIssues(null);
    const r = diagResolveRef.current;
    diagResolveRef.current = null;
    r?.();
  };

  const stopZapretFromDiag = async () => {
    await api('POST', '/api/zapret/stop').catch(() => null);
    resolveDiag();
  };

  useEffect(() => {
    // StrictMode монтирует эффект дважды: cleanup первого прогона гасит
    // aliveRef — повторный монтаж обязан восстановить его, иначе прогон
    // сразу падает «не удалось подключиться» и экран зависает.
    aliveRef.current = true;
    applyThemeAtStart();
    // Показ окна — через бэкенд: флаг видимости и событие окна обновляются
    // одним местом (POST /api/app/request-show). При старте с --hidden
    // бэкенд окно не покажет — до открытия из трея. Вызов не блокирует
    // kickoff-таймаут ниже.
    api('POST', '/api/app/request-show').catch(() => null);
    // Сначала даём экрану отрисоваться, затем запускаем процесс — иначе
    // первый кадр теряется и выглядит как зависание.
    const kickoff = setTimeout(() => { if (aliveRef.current) run(); }, 700);

    return () => {
      aliveRef.current = false;
      if (autoSelectEsRef.current) {
        api('POST', '/api/strategy/cancel').catch(() => null);
      }
      clearTimeout(kickoff);
      if (softErrTimerRef.current) { clearTimeout(softErrTimerRef.current); softErrTimerRef.current = null; }
      if (eventsCleanupRef.current) { eventsCleanupRef.current(); eventsCleanupRef.current = null; }
    };
  }, [run]);

  // Блокирующие диалоги (вопрос окружения, критичные ошибки) — процесс
  // стоит и ждать решения: ползучий процент на паузу.
  const modalOpen = !!(envQuestion || serviceCritical || criticalIssues || error);

  // Реальный целевой процент: авто-подбор и скачивание движка
  // интерполируются внутри своих диапазонов, чтобы бар не «стоял» на
  // длинных шагах.
  const targetPct = (() => {
    if (currentStep === 'auto-select' && autoSelect && autoSelect.total > 0 && autoSelect.current > 0) {
      const base = STEP_PROGRESS['install-service'];
      const target = STEP_PROGRESS['auto-select'];
      const calc = Math.round(base + (target - base) * Math.min(autoSelect.current / autoSelect.total, 1));
      return Math.max(calc, progress);
    }
    if (currentStep === 'install' && downloadProgress && downloadProgress.total > 0) {
      const base = STEP_PROGRESS['check-local'];
      const target = STEP_PROGRESS['install-service'];
      const calc = Math.round(base + (target - base) * Math.min(downloadProgress.downloaded / downloadProgress.total, 1));
      return Math.max(calc, progress);
    }
    return Math.round(progress);
  })();

  // CAP ползучего процента: 95% пути от текущего чекпоинта до следующего
  // этапа. displayed никогда не пересекает реальное значение следующего
  // этапа — при его наступлении бар мгновенно прыгает к targetPct.
  const creepCap = (() => {
    const floor = Math.round(progress);
    const next = Object.values(STEP_PROGRESS).filter(v => v > floor).sort((a, b) => a - b)[0];
    if (next === undefined) return 99;
    return Math.floor(floor + (next - floor) * 0.95);
  })();

  const targetRef = useRef(targetPct);
  targetRef.current = targetPct;
  const capRef = useRef(creepCap);
  capRef.current = creepCap;

  // Смена этапа — мгновенный прыжок к реальному значению (вперёд).
  useEffect(() => {
    setDisplayed(prev => (targetPct > prev ? targetPct : prev));
  }, [targetPct]);

  // Ползучий процент: пока реальный прогресс не меняется, displayed раз в
  // секунду подтягивается к CAP: displayed += (cap - displayed) * k + шаг.
  // Живое значение — таймер по образцу useNow (setInterval в эффекте).
  useEffect(() => {
    if (modalOpen) return undefined;
    const iv = setInterval(() => {
      setDisplayed(prev => {
        const target = targetRef.current;
        if (target > prev) return target;
        const cap = capRef.current;
        if (prev >= cap) return prev;
        return Math.min(cap, prev + (cap - prev) * 0.08 + 0.2);
      });
    }, 1000);
    return () => clearInterval(iv);
  }, [modalOpen]);

  const pct = Math.round(displayed);

  const applyThemeAtStart = () => {
    const saved = localStorage.getItem('zpui-theme');
    if (saved === 'dark' || saved === 'light') {
      document.documentElement.setAttribute('data-theme', saved);
    } else {
      const prefersDark = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches;
      document.documentElement.setAttribute('data-theme', prefersDark ? 'dark' : 'light');
    }
  };

  const handleCopyError = useCallback(() => {
    let text = (errorTitle || t('startup.errors.title')) + '\n' + error;
    if (errorLog && errorLog.length > 0) {
      text += '\n\n— журнал установки —\n' + errorLog.join('\n');
    }
    navigator.clipboard.writeText(text).catch(() => {});
  }, [error, errorTitle, errorLog, t]);

  // Текст под баром — только про реальный шаг; на 0–15% пусто.
  const statusKey = modalOpen ? '' : (STEP_STATUS[currentStep] || '');
  const statusText = statusKey ? t('startup.rotating.' + statusKey) : '\u00A0';

  // Живая строка автоподбора: текущий этап конвейера + проверяемая
  // стратегия + счётчик. Ключи фаз — из модалки подбора.
  const AS_PHASES = ['operator', 'server', 'local', 'analyze', 'select', 'startup-last'];
  const asPhaseKey = autoSelect && AS_PHASES.includes(autoSelect.phase) ? autoSelect.phase : 'testing';
  const asPhaseText = t('zapret.autoSelectPhase.' + asPhaseKey);

  return (
    <div className="startup-overlay">
      <div className="startup-card startup-card-slim">
        <div className="startup-title">ZPUI</div>

        {/* Контейнер 1: бар + название этапа. Оба слота присутствуют всегда
            с жёсткой высотой — смена текста не меняет layout, бар и проценты
            не смещаются никогда (правило владельца). */}
        <div className="startup-progress">
          <div className="startup-brow">
            <ProgressBar value={pct} striped className="startup-bar" />
            <div className="startup-pct mono">{pct}%</div>
          </div>
          <div className="startup-status">
            <span className="startup-status-msg" key={statusKey || 'hold'}>{statusText}</span>
          </div>
        </div>

        {/* Контейнер 2: дополнительные подробности + софт-ошибка. Слоты
            зарезервированы (мин-высота под 2 строки / 1 строку) и живут
            в DOM постоянно — появление/исчезновение строк не двигает бар. */}
        <div className="startup-details">
          <div className="startup-live">
            {currentStep === 'auto-select' && autoSelect && !modalOpen && (
              <>
                <span className="startup-live-phase">{asPhaseText}</span>
                {autoSelect.strategy && <span className="startup-live-strat mono">{autoSelect.strategy}</span>}
                {autoSelect.total > 0 && (
                  <span className="startup-live-count mono">{autoSelect.current}/{autoSelect.total}</span>
                )}
              </>
            )}
          </div>
          <div className="startup-status-err" role="alert">
            {softError && (
              <>
                <AlertTriangle size={12} strokeWidth={2.2} />
                <span>{softError}</span>
              </>
            )}
          </div>
        </div>
      </div>

      {criticalIssues && (
        <div className="startup-modal-overlay">
          <div className="startup-modal">
            <strong>{t('startup.diag.title')}</strong>
            <p>{t('startup.diag.text')}</p>
            <div className="startup-modal-list">
              {criticalIssues.map(i => (
                <div key={i.key} className="startup-modal-list-item">
                  <span className="startup-modal-list-label">{i.label}</span>
                  <span className="startup-modal-list-detail">{i.detail}</span>
                </div>
              ))}
            </div>
            <div className="startup-modal-actions">
              <button className="btn btn-danger btn-sm" style={{ flex: 1 }} onClick={stopZapretFromDiag}>
                {t('startup.diag.stopZapret')}
              </button>
              <button className="btn btn-ghost btn-sm" onClick={resolveDiag}>
                {t('startup.diag.skip')}
              </button>
            </div>
          </div>
        </div>
      )}
      {envQuestion && (
        <div className="startup-modal-overlay">
          <div className="startup-modal">
            {envQuestion.kind === 'vpn' ? (
              <>
                <strong>{t('startup.thirdparty.vpnTitle')}</strong>
                <p>{t('startup.thirdparty.vpnDesc', { names: (envQuestion.data?.vpn?.names || []).join(', ') })}</p>
                <div className="startup-modal-actions">
                  <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={() => resolveEnv('disable')}>
                    {t('startup.thirdparty.vpnDisable')}
                  </button>
                  <button className="btn btn-ghost btn-sm" onClick={() => resolveEnv('skip')}>
                    {t('startup.thirdparty.vpnSkip')}
                  </button>
                </div>
                <div className="startup-modal-note">{t('startup.thirdparty.vpnSkipHint')}</div>
              </>
            ) : (
              <>
                <strong>{t('startup.thirdparty.title')}</strong>
                <p>{t('startup.thirdparty.desc')}</p>
                <div className="startup-warn">{t('startup.thirdparty.vpnWarn')}</div>
                <div className="startup-modal-actions">
                  <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={() => resolveEnv('remove')}>
                    {t('startup.thirdparty.remove')}
                  </button>
                  <button className="btn btn-ghost btn-sm" onClick={() => resolveEnv('keep')}>
                    {t('startup.thirdparty.keep')}
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {serviceCritical && (
        <div className="startup-modal-overlay">
          <div className="startup-modal startup-modal-error">
            <strong>{t('startup.serviceHealth.critical')}</strong>
            <p>{t('startup.serviceHealth.criticalDesc')}</p>
            {errorLog && errorLog.length > 0 && (
              <details className="startup-modal-log">
                <summary>{t('startup.installLog')}</summary>
                <pre>{errorLog.join('\n')}</pre>
              </details>
            )}
            <div className="startup-modal-actions">
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={handleRetryDiagnostics}>{t('startup.serviceHealth.retry')}</button>
              <button className="btn btn-ghost btn-sm" onClick={handleContinueWithout}>{t('startup.serviceHealth.continueWithout')}</button>
            </div>
          </div>
        </div>
      )}

      {error && (
        <div className="startup-modal-overlay">
          <div className="startup-modal startup-modal-error">
            <strong>{errorTitle}</strong>
            <p className="startup-modal-err-text">{error}</p>
            {errorLog && errorLog.length > 0 && (
              <details className="startup-modal-log">
                <summary>{t('startup.installLog')}</summary>
                <pre>{errorLog.join('\n')}</pre>
              </details>
            )}
            {errCode === 'R-000-HVCI' && <HvciFixButton />}
            <div className="startup-modal-actions">
              <button className="btn btn-accent btn-sm" style={{ flex: 1 }} onClick={() => window.location.reload()}>{t('common.restart')}</button>
              <button className="btn btn-ghost btn-sm" onClick={handleCopyError}>{t('common.copy')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
