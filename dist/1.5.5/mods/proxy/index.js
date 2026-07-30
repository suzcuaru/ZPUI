(function(ZPUI) {
  var h = ZPUI.h;
  var useState = ZPUI.useState;
  var useEffect = ZPUI.useEffect;
  var useCallback = ZPUI.useCallback;
  var api = ZPUI.api;

  function ProxyPage(ctx) {
    var devices = useState([]);
    var setDevices = devices[1];
    var devicesVal = devices[0];
    var config = useState({ port: 1080, username: '', password: '', auto_start: false });
    var setConfig = config[1];
    var configVal = config[0];
    var pLoading = useState(false);
    var setPLoading = pLoading[1];
    var saveTimer = useState(null);
    var setSaveTimer = saveTimer[1];

    var status = ctx.status;
    var pRun = status && status.proxy && status.proxy.running === true;
    var port = (status && status.proxy && status.proxy.port) || 1080;

    var loadData = useCallback(function() {
      Promise.all([
        api('GET', '/api/proxy/connections'),
        api('GET', '/api/proxy/config'),
      ]).then(function(results) {
        var devs = results[0];
        var cfg = results[1];
        if (devs && devs.by_client) {
          var deviceInfo = devs.device_info || {};
          var deviceList = Object.keys(devs.by_client).map(function(ip) {
            var conns = devs.by_client[ip];
            return {
              ip: ip,
              hostname: (deviceInfo[ip] && deviceInfo[ip].hostname) || ip,
              mac: (deviceInfo[ip] && deviceInfo[ip].mac) || '',
              connections: Array.isArray(conns) ? conns.length : conns,
            };
          });
          setDevices(deviceList);
        }
        if (cfg) setConfig(cfg);
      });
    }, []);

    useEffect(function() {
      loadData();
      var iv = setInterval(loadData, 5000);
      return function() { clearInterval(iv); };
    }, []);

    function toggleProxy() {
      setPLoading(true);
      var action = pRun ? 'stop' : 'start';
      api('POST', '/api/proxy/' + action).then(function() {
        api('POST', '/api/component-states');
      }).then(function() {
        setPLoading(false);
      });
    }

    function updateConfig(patch) {
      setConfig(function(prev) {
        var next = Object.assign({}, prev, patch);
        setTimeout(function() {
          api('POST', '/api/proxy/config', next);
        }, 500);
        return next;
      });
    }

    var totalConns = devicesVal.reduce(function(sum, d) { return sum + d.connections; }, 0);

    return h('div', null,
      h('div', { className: 'page-title' }, 'Прокси'),
      h('div', { className: 'section' },
        h('div', { className: 'set-row' },
          h('div', { className: 'set-row-info' },
            h('span', { className: 'set-row-title' }, 'SOCKS5 прокси'),
            h('span', { className: 'set-row-desc' },
              (pRun ? 'Работает на порту ' + port : 'Остановлен') + ' \u00B7 ' + devicesVal.length + ' устр. \u00B7 ' + totalConns + ' подкл.'
            )
          ),
          h('button', {
            className: 'btn ' + (pRun ? 'btn-danger' : 'btn-accent'),
            onClick: toggleProxy,
            disabled: pLoading[0],
          }, pLoading[0] ? '...' : pRun ? 'Остановить' : 'Запустить')
        )
      ),
      h('div', { className: 'section' },
        h('div', { className: 'section-title' }, 'Подключённые устройства'),
        devicesVal.length === 0
          ? h('div', { style: { color: 'var(--text-tertiary)', fontSize: 11, padding: 8 } }, 'Нет подключённых устройств')
          : h('div', { className: 'proxy-devices' },
              devicesVal.map(function(d) {
                return h('div', { key: d.ip, className: 'proxy-device' },
                  h('span', { className: 'pd-status' }),
                  h('div', { className: 'pd-info' },
                    h('span', { className: 'pd-host' }, d.hostname || d.ip),
                    h('span', { className: 'pd-ip' }, d.ip + (d.mac ? ' \u00B7 ' + d.mac : ''))
                  ),
                  h('div', { className: 'pd-latency' },
                    h('span', { className: 'pd-latency-val' }, String(d.connections)),
                    h('span', { className: 'pd-latency-label' }, 'подкл.')
                  )
                );
              })
            )
      ),
      h('div', { className: 'section' },
        h('div', { className: 'section-title' }, 'Настройки прокси'),
        h('div', { className: 'form-group' },
          h('label', null, 'Порт'),
          h('input', {
            type: 'number', className: 'form-input', value: configVal.port,
            min: 1, max: 65535,
            onChange: function(e) { updateConfig({ port: parseInt(e.target.value) || 1080 }); },
          })
        ),
        h('div', { className: 'form-group' },
          h('label', null, 'Имя пользователя'),
          h('input', {
            type: 'text', className: 'form-input', value: configVal.username || '',
            placeholder: '(без авторизации)',
            onChange: function(e) { updateConfig({ username: e.target.value }); },
          })
        ),
        h('div', { className: 'form-group' },
          h('label', null, 'Пароль'),
          h('input', {
            type: 'password', className: 'form-input', value: configVal.password || '',
            placeholder: '(без авторизации)',
            onChange: function(e) { updateConfig({ password: e.target.value }); },
          })
        ),
        h('div', { className: 'set-row' },
          h('div', { className: 'set-row-info' },
            h('span', { className: 'set-row-title' }, 'Автозапуск прокси'),
            h('span', { className: 'set-row-desc' }, 'Запускать при старте приложения')
          ),
          h(ZPUI.Switch, { checked: configVal.auto_start || false, onChange: function() { updateConfig({ auto_start: !configVal.auto_start }); } })
        )
      )
    );
  }

  ZPUI.registerPage('proxy', {
    label: 'Прокси',
    icon: h('svg', { viewBox: '0 0 24 24', width: '24', height: '24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.5 },
      h('rect', { x: '2', y: '6', width: '20', height: '12', rx: '2' }),
      h('circle', { cx: '7', cy: '12', r: '1.5' }),
      h('circle', { cx: '12', cy: '12', r: '1.5' }),
      h('circle', { cx: '17', cy: '12', r: '1.5' })
    ),
    order: 7,
    component: ProxyPage,
  });
})(window.ZPUI || {});
